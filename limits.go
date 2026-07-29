package formalis

import (
	"context"
	"fmt"
)

// Cancellation and resource limits.
//
// This package validates invoice XML that arrives from outside — a Factur-X
// attachment pulled out of a PDF, a file posted to an endpoint — so the cost of
// a call is set by the document, not by the caller. Two separate mechanisms
// bound that cost, and the distinction is the same one pdf0 draws in
// docs/limits.md: a *limit* says how much this document may cost, a *context*
// says how long this operation may take.
//
// # Why both, and why the limits came first
//
// The limits are not a fallback for the context; they answer a question the
// context cannot. Measuring validation time against invoice size turned up
// three defects that no amount of cancellation would have fixed:
//
//   - parseCII accumulated an element's character data with `text += string(t)`,
//     one reallocation per token. 6.4 MB of well-formed XML took 33 s and grew
//     quadratically; the same shape at this repository's practical 100 MB
//     ceiling would have run for hours. Fixed in parseCII by appending to a
//     []byte (there is no budget for it — it is simply linear now).
//   - validateVATTaxableSums re-scanned and re-parsed every invoice line for
//     every VAT breakdown. 7.3 MB took 1.7 s, also quadratic. Fixed by parsing
//     each operand once and memoising per distinct (category, rate), with
//     maxVATSumWork as the backstop for input that defeats the memoisation.
//   - The tree walks (findAll, collectAttr, mapCII) recurse once per level, and
//     a deeply nested document overflowed the goroutine stack at around 90 MB.
//     A stack overflow is a *fatal* runtime error: it is not a panic, so no
//     recover() anywhere up the stack — including the one pdf0 wraps
//     ValidateFacturX in — can turn it back into a finding. maxDepth stops the
//     parse before the tree can get that deep, which protects every walk at one
//     point rather than each of them separately.
//
// A caller with no deadline at all still pays those costs, which is what makes
// them limits rather than latency.
//
// # What a stopped run reports
//
// A cancelled run, and a run that trips a budget, have the same problem: the
// checker stopped before it had seen everything, so "no violations" would be a
// lie. Both are therefore reported as a Violation under the reserved rule
// identifier RuleLimit, and IsCheckerViolation tells those apart from a real
// non-conformance.
//
// This is deliberately the same convention pdf0 uses for its own guards and for
// cancellation — the same rule name, the same meaning, the same predicate shape
// — so that a caller draining ValidateFacturX's mixed slice of container and
// invoice findings has one rule to look for and not two.
//
// The property that matters: a stopped run never returns an empty slice. A
// caller testing len(v) == 0 for "valid" gets "not valid"; a caller filtering
// with IsCheckerViolation gets "unknown". Neither gets a clean bill of health
// from a run that did not look.
//
// This is also why a parse failure has to be told apart from a stopped parse.
// Returning RuleSyntax ("not well-formed") when the run was merely cancelled
// would be the same lie in the other direction — accusing a document the checker
// never finished reading. syntaxViolation draws that line, and every exported
// validator goes through it.
//
// # Why the Is* predicates take no context
//
// The exported detection predicates (IsZATCA, IsFinvoice, ...) report whether
// they could read the document, but take no context. There is nothing to
// cancel: they do not build a tree, so there is no budget for them to trip and
// no long-running phase for a deadline to interrupt. scanShape reads the
// document once, retaining only the open elements and a few short strings, so
// their cost is a single linear pass in memory set by the nesting rather than
// by the size. See detect.go.
//
// Their error therefore reports what the document is — malformed XML, an
// encoding this package does not implement, nesting past the cap — and never
// "the checker gave up", which is the distinction RuleLimit exists to carry on
// the validation side.

// RuleLimit is the rule identifier carried by a Violation that reports the
// checker stopping early — a cancelled context or a tripped resource budget —
// rather than a defect in the invoice.
//
// It matches the identifier pdf0 uses for the same event, so a caller that
// already separates "the file is bad" from "the checker could not finish" needs
// only one name for the second.
const RuleLimit = "limit"

// RuleSyntax is the rule identifier for XML that is not well-formed or is not
// an invoice document at all. Unlike RuleLimit this *is* a statement about the
// file.
const RuleSyntax = "syntax"

// IsCheckerViolation reports whether v describes the checker stopping early
// rather than a way in which the invoice departs from the rules.
//
// A cancelled context and a tripped resource budget both produce one. Treat it
// as "unknown", never as "conformant" and never as "non-conformant".
func IsCheckerViolation(v Violation) bool { return v.Rule == RuleLimit }

// maxDepth is the deepest element nesting the parser will build.
//
// Real invoices in every syntax this package reads nest around a dozen levels;
// the deepest document in the oracle suites is far below this. The cap exists
// for the fatal-stack-overflow case described above, so it is set generously
// enough that no genuine invoice can reach it and low enough that the recursive
// walks stay far from the 1 GB goroutine stack limit.
const maxDepth = 1000

// maxNodes is the largest number of elements the parser will build a tree from.
//
// maxDepth bounds how *deep* a document nests; it says nothing about how *many*
// elements it has, and the two failure shapes are unrelated. A document that is
// millions of shallow siblings has a depth of 2, so maxDepth never engages,
// while every one of those siblings becomes a ciiNode — a name string, a
// children slice, a text string and an accumulator. That is about 105 bytes
// retained per element, and around 165 bytes of peak RSS, for an element
// written as four (`<a/>`): a 60 MB document of them reached 2.5 GB, and the
// 100 MB this package can be handed projected to roughly 4.2 GB. Like the stack
// overflow maxDepth exists for, an OOM kill is a process death rather than a
// finding the caller can report, which is what makes a bound necessary rather
// than merely tidy.
//
// A context does not substitute for it. parseCII polls cancellation every
// cancelParseTokens tokens, so a deadline does stop the parse — but only after
// whatever has already been allocated, so a short deadline still admits
// gigabytes.
//
// The number is set the way maxDepth was: measure real documents and leave a
// wide margin. Across the 1613 documents in testdata the largest is 8300
// elements, and that is a UN/ECE code list rather than an invoice; the largest
// actual invoice is 1803. The largest XML pdf0 has found embedded in a PDF is
// 1.3 MB. A million elements is over a hundred times the largest document here,
// and it holds the whole call flat: validating 15, 30 and 60 MB of siblings now
// peaks at 167, 203 and 236 MB rather than 610 MB, 1.24 GB and 2.56 GB, and
// reports one RuleLimit finding instead of eighteen invented business-rule
// violations.
const maxNodes = 1_000_000

// maxVATSumWork bounds the (breakdown x operand) pairs validateVATTaxableSums
// may examine across one invoice.
//
// After memoisation a real invoice uses a few thousand: the pair count is
// (distinct VAT category/rate combinations) x (lines + allowances/charges), and
// an invoice has single-digit combinations. The budget is three orders of
// magnitude above that, so it is reached only by a document built to reach it.
const maxVATSumWork = 10_000_000

// canceler is one call's cancellation signal.
//
// The zero value never cancels, which is what lets internal helpers take one
// unconditionally instead of branching around a missing context.
//
// ctx.Done() is hoisted into a field at construction because the parser's token
// loop tests it: Context.Done and Context.Err both take the context's mutex on a
// cancellable context, while a receive-with-default on an already-obtained
// channel does not. A context that can never be cancelled (context.Background)
// has a nil Done channel, and a receive on a nil channel blocks, so the default
// case is taken — the zero value and a background context behave alike, for
// free.
type canceler struct {
	ctx  context.Context
	done <-chan struct{}
}

func newCanceler(ctx context.Context) canceler {
	if ctx == nil {
		return canceler{}
	}
	return canceler{ctx: ctx, done: ctx.Done()}
}

// stopped reports whether the call should stop now.
func (c canceler) stopped() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c canceler) err() error {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}

// run is the per-call state of one validation: the caller's cancellation signal
// and the resource budgets. It is threaded explicitly rather than stored
// anywhere, because its lifetime is exactly one call.
type run struct {
	cancel canceler
	// vatWork is the remaining validateVATTaxableSums pair budget.
	vatWork int
	// nodes is the remaining element budget for the tree parseCII builds.
	nodes int
	// trips accumulates the RuleLimit findings this run has to report. A run
	// records at most one trip per distinct cause, since repeating "the checker
	// stopped" tells the caller nothing new.
	trips []Violation
	seen  map[string]bool
}

func newRun(ctx context.Context) *run {
	return &run{cancel: newCanceler(ctx), vatWork: maxVATSumWork, nodes: maxNodes}
}

// note records a limit trip once per guard.
func (r *run) note(guard, msg string) {
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if r.seen[guard] {
		return
	}
	r.seen[guard] = true
	r.trips = append(r.trips, Violation{
		Rule:    RuleLimit,
		Message: fmt.Sprintf("%s (%s); the checks that had not yet run were skipped, so this invoice is neither confirmed valid nor invalid", msg, guard),
	})
}

// stopped reports whether the caller's context has ended, recording the trip the
// first time it has. Every loop that can run long consults it.
func (r *run) stopped() bool {
	if r == nil || !r.cancel.stopped() {
		return false
	}
	err := r.cancel.err()
	if err == nil {
		err = context.Canceled
	}
	r.note("context-canceled", "the run was cancelled before it finished: "+err.Error())
	return true
}

// spendNode draws one element from the tree budget, reporting whether the parse
// may proceed.
func (r *run) spendNode() bool {
	if r == nil {
		return true
	}
	if r.nodes <= 0 {
		r.note("xml-node-count", fmt.Sprintf("the invoice XML has more than %d elements", maxNodes))
		return false
	}
	r.nodes--
	return true
}

// spendVAT draws n pairs from the VAT summation budget, reporting whether the
// work may proceed.
func (r *run) spendVAT(n int) bool {
	if r == nil {
		return true
	}
	if r.vatWork <= 0 {
		r.note("vat-sum-work", "the VAT breakdown taxable-amount checks (BR-*-08) exceeded their work budget and were not completed")
		return false
	}
	r.vatWork -= n
	return true
}

// finish appends this run's limit trips to the findings it gathered.
//
// It is the single place a stopped run is turned into output, so no exported
// validator can forget it — and because the trips are appended rather than
// replacing the findings, the checks that did complete are still reported. They
// are true, just incomplete.
func (r *run) finish(out []Violation) []Violation {
	if r == nil || len(r.trips) == 0 {
		return out
	}
	return append(out, r.trips...)
}
