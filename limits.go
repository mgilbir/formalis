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
// The property that matters: a stopped run never returns an empty Violations
// slice. A caller testing len(r.Violations) == 0 for "valid" gets "not valid";
// a caller filtering with IsCheckerViolation gets "unknown". Neither gets a
// clean bill of health from a run that did not look.
//
// Report.Complete is where this generalises. A stopped run is one of two ways a
// validator can fail to have seen everything — the other is a rule set that
// does not implement every rule its authority publishes — and Complete is false
// for both, so Report.Conformant is the one predicate that is safe to key on.
// The RuleLimit finding stays exactly as it is: Complete reads it, rather than
// replacing it, so a caller that already routes on RuleLimit keeps working and
// pdf0's own container guards keep the same meaning inside a mixed slice.
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
// rather than a defect in the invoice. Such a finding carries SourceChecker,
// because it is a statement by this package and not by any rule authority.
//
// It matches the identifier pdf0 uses for the same event, so a caller that
// already separates "the file is bad" from "the checker could not finish" needs
// only one name for the second.
const RuleLimit = "limit"

// RuleSyntax is the rule identifier for XML that is not well-formed or is not
// an invoice document at all. Unlike RuleLimit this *is* a statement about the
// file, but it is still this checker's statement rather than a rule authority's,
// so it too carries SourceChecker.
const RuleSyntax = "syntax"

// RuleProfile is the rule identifier carried by a Violation that reports the
// caller naming a Profile this package does not implement. Like RuleLimit and
// RuleSyntax it carries SourceChecker, because it is a statement by this
// checker; unlike either it is a statement about the *request*, not about the
// document, which is innocent and was not examined.
//
// It exists because the alternatives are all worse. Reporting it as RuleSyntax
// would accuse a document that may be perfectly conformant. Reporting it as
// RuleLimit would overload an identifier documented as a resource-budget or
// cancellation event and shared verbatim with pdf0, so a caller that routes
// "the checker ran out of room, retry it smaller" would retry forever on input
// no retry can fix. And returning no findings at all would be the one outcome
// this package refuses everywhere else: a caller testing len(v) == 0 for
// "valid" would get a clean bill of health from a run that never chose a rule
// set. So a run that rejects the profile validates nothing and returns exactly
// this one finding — never mixed with document findings, because there are
// none to mix it with.
//
// It is a reserved word, like RuleLimit, rather than an identifier in anyone's
// numbering scheme, and IsCheckerViolation recognises it: "I did not judge this
// document" is exactly what that predicate exists to keep separate from
// "conformant".
const RuleProfile = "profile"

// IsCheckerViolation reports whether v describes the checker not having judged
// the document, rather than a way in which the invoice departs from the rules.
//
// A cancelled context, a tripped resource budget (both RuleLimit) and a Profile
// this package does not implement (RuleProfile) all produce one. Treat it as
// "unknown", never as "conformant" and never as "non-conformant".
//
// RuleSyntax is deliberately *not* one of them: "this file is not well-formed
// XML" is a finding about the document, and a definite one.
//
// It tests Rule alone, deliberately, even though every finding this package
// emits now carries a Source and the pair a caller should think in is
// (SourceChecker, RuleLimit). Two reasons. Both identifiers are reserved words
// rather than identifiers in anyone's numbering scheme — no rule authority
// mints a rule called "limit" or "profile" — so there is nothing for the scope
// to disambiguate here. And RuleLimit is shared with pdf0, which constructs
// that same finding for its own container guards and hands it back in one mixed
// slice; requiring SourceChecker would silently reclassify every one of those as
// a business-rule violation the moment this package added the field. A caller
// that wants the strict pair can still write it — the Source is there — but the
// predicate that exists to keep "unknown" from being read as "conformant" must
// not start returning false for a finding it has always covered.
//
// Widening it to RuleProfile is safe in the direction that mattered there:
// pdf0 never emits RuleProfile, so no finding that exists today changes
// classification. What would not be safe is the reverse — leaving it out, so
// that a caller filtering with this predicate to count document defects counted
// its own bad argument as one.
func IsCheckerViolation(v Violation) bool {
	return v.Rule == RuleLimit || v.Rule == RuleProfile
}

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
//
// The budget is a property of the *document*, not of the entry point the caller
// reached for. That has to be arranged rather than assumed: run.nodes is spent
// one element at a time by parseCII, so a call that read the same bytes twice
// would halve the ceiling. Every exported validator therefore parses exactly
// once, at its own boundary, and threads the parsed artefacts down — see parsed
// in facturx_en16931.go — rather than the raw bytes. Before that, ValidateCIUS
// (which reads BT-24 to choose a validator) reached UBL.BE through three parses
// and refused, as too large, documents that ValidateUBLBE validated and
// Validate validated: a third of the number below. Nothing about the document
// decided that; only how many layers the call passed through did.
// TestNodeBudgetIsPerDocumentNotPerEntryPoint pins the property.
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
	// nodes is the remaining element budget for the tree parseCII builds. One
	// call reads its document once, so this is the document's element count and
	// not a per-parse allowance; see maxNodes.
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
		Source:  SourceChecker,
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
