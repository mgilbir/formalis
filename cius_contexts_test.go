package formalis

import (
	"context"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The two guards this PR needed that no earlier one had: a corpus-wide *context*
// count for a hand-written rule set, and a decoder that reads ISO Schematron's rule
// order out of an artefact.
//
// # Why a context count
//
// Requirement two of every rule set's oracle in this repository is that a clean
// sweep is not evidence. A rule that reports nothing over 1,680 documents is either
// a rule that was asked and answered "conforms" or a rule that was never asked —
// bound to an element name the mapper never produces, or behind a gate that never
// opens — and no findings count can tell the two apart. The generated CIUS-PT and
// CIUS-RO tiers get this for free because their contexts are data
// (TestCIUSPTDatatypeContextsAreReachable, TestCIUSRORuleContextsAreReachable). A
// hand-written rule body has no such structure, so ruleContexts takes the count at
// the point the body reaches a node the rule is about.
//
// # Why a rule-order decoder
//
// ISO Schematron 6.5: a node is processed by the first rule in a pattern whose
// context matches it, and by no other. Three of this repository's rule sets have
// identifiers that no processor can report because of it — CEN's CII-DT-010/011/012,
// three of CIUS-RO's, and now fifteen of SRBDT's and four of NLCIUS's — and the
// claim is checkable, so it is checked rather than asserted. Both authorities'
// artefacts are read with a decoder for C31's reason: the previous version of this
// analysis, done by eye over a printed listing, missed the two NLCIUS-CII rules
// entirely because they sit eleven lines apart in a 200-line file.

// ciusContextSweep runs one rule set's body over every document in testdata that
// parseEN16931 accepts, and returns how many context nodes each identifier was asked
// about, together with the number of files read.
//
// It returns 0 files when there is no corpus, which is the caller's cue to skip: a
// guard that read nothing must say so rather than report that it found no gap.
func ciusContextSweep(t *testing.T, body func(*parsed, ruleContexts)) (ruleContexts, int) {
	t.Helper()
	seen, files := ruleContexts{}, 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil {
			return nil
		}
		body(parsed, seen)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return seen, files
}

// reportUnreached fails for any identifier in want that no context node in the
// corpus reached, unless it is excused, and fails for an excuse that turns out to be
// unnecessary. The second direction is what stops an exception list outliving its
// reason.
func reportUnreached(t *testing.T, what string, seen ruleContexts, want []string, excused map[string]string) {
	t.Helper()
	var unreached []string
	nodes := 0
	for _, id := range want {
		nodes += seen[id]
		if seen[id] == 0 {
			if _, ok := excused[id]; !ok {
				unreached = append(unreached, id)
			}
			continue
		}
		if why, ok := excused[id]; ok {
			t.Errorf("%s/%s is excused from the context sweep (%q) and the corpus reaches it %d times. An "+
				"exception that is no longer needed is an exception nobody will re-check", what, id, why, seen[id])
		}
	}
	sort.Strings(unreached)
	if len(unreached) != 0 {
		t.Errorf("no document in the corpus reaches the context of %s %v. A rule bound to an element the corpus "+
			"never contains is indistinguishable from a rule bound to a misspelt one; either add a corpus "+
			"document or excuse it here with the reason", what, unreached)
	}
	t.Logf("%s: %d of %d identifiers reached by the corpus, %d context nodes in all, %d excused",
		what, len(want)-len(unreached)-len(excused), len(want), nodes, len(excused))
}

// schOrderRule is one <rule> of a Schematron pattern: its context and the identifiers of
// the assertions inside it.
type schOrderRule struct {
	context string
	ids     []string
}

// schShadowed decodes the named Schematron files and returns, for every identifier
// every one of whose rules is unreachable, the context that claims it.
//
// "Unreachable" is decided per rule and then per identifier: a rule is unreachable
// when every alternative of its context is matched by an earlier rule of the same
// pattern, and an identifier is unreachable when every rule carrying it is. The
// second step matters — si-ubl-2.0-nlcius.sch carries BR-NL-32-1 in two rules, one
// reachable and one not, and the identifier is reported by the first.
func schShadowed(t *testing.T, files []string) map[string]string {
	t.Helper()
	out := schShadowedRules(t, files)
	for id := range schLiveRules(t, files) {
		delete(out, id)
	}
	return out
}

// schShadowedRules is schShadowed without the per-identifier fold: an identifier
// appears here as soon as *one* rule carrying it is unreachable, even if another
// rule carrying the same identifier is reached. NLCIUS's UBL binding is the only
// artefact here where the two answers differ, and the difference is the whole
// BR-NL-34 story.
func schShadowedRules(t *testing.T, files []string) map[string]string {
	t.Helper()
	out, _ := schRuleOrder(t, files)
	return out
}

// schLiveRules is the complement: the identifiers at least one reachable rule
// carries.
func schLiveRules(t *testing.T, files []string) map[string]bool {
	t.Helper()
	_, live := schRuleOrder(t, files)
	return live
}

func schRuleOrder(t *testing.T, files []string) (map[string]string, map[string]bool) {
	t.Helper()
	out := map[string]string{}
	live := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(string(data)))
		var rules []schOrderRule
		flush := func() {
			for i, r := range rules {
				dead := 0
				var by string
				for _, alt := range schAlternatives(r.context) {
					// Every alternative of every earlier rule, not the earlier rule's
					// context as one string: eleven SRBDT rules write "/ubl:Invoice |
					// /cn:CreditNote" and each alternative has to be compared on its own.
					for j := 0; j < i && by != alt; j++ {
						for _, earlier := range schAlternatives(rules[j].context) {
							if schCovers(earlier, alt) {
								dead++
								by = alt
								break
							}
						}
					}
				}
				for _, id := range r.ids {
					if dead == len(schAlternatives(r.context)) && dead > 0 {
						if _, ok := out[id]; !ok {
							out[id] = by
						}
					} else {
						live[id] = true
					}
				}
			}
			rules = nil
		}
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			at := func(n string) string {
				for _, a := range se.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch se.Name.Local {
			case "pattern":
				flush()
			case "rule":
				rules = append(rules, schOrderRule{context: normSpace(at("context"))})
			case "assert", "report":
				if id := at("id"); id != "" && len(rules) > 0 {
					rules[len(rules)-1].ids = append(rules[len(rules)-1].ids, id)
				}
			}
		}
		flush()
	}
	return out, live
}

// schAlternatives splits a rule context on "|", which is how these files write a
// rule that applies to an Invoice and a CreditNote.
func schAlternatives(ctx string) []string {
	var out []string
	for _, a := range strings.Split(ctx, "|") {
		if a = normSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// schCovers reports whether match pattern a selects every node b selects, for the
// shapes these artefacts use.
//
// Two cases, and the second is the one that is easy to read past. Identical patterns
// cover each other, which is how eleven SRBDT rules come to repeat "/ubl:Invoice |
// /cn:CreditNote". And a *relative* pattern covers any longer path that ends in it,
// because an XSLT match pattern is anchored at its last step and matched leftwards:
// "cac:AllowanceCharge/cbc:AllowanceChargeReasonCode" selects a reason code under
// any allowance or charge at any depth, the line-level ones included, so it covers
// "cac:InvoiceLine/cac:AllowanceCharge/cbc:AllowanceChargeReasonCode".
//
// A trailing [$s] or [$si] predicate is carried along: the two must agree, because a
// pattern with a predicate the other lacks selects fewer nodes. Any other predicate
// makes this decline to answer, which is the conservative direction — a claim that a
// rule is unreachable has to be safe to act on.
func schCovers(a, b string) bool {
	pa, qa := schSplitPredicate(a)
	pb, qb := schSplitPredicate(b)
	if qa != "" && qa != qb {
		return false
	}
	if pa == pb {
		return true
	}
	if strings.HasPrefix(pa, "/") || strings.HasPrefix(pb, "/") {
		return false
	}
	if strings.ContainsAny(pa, "[@()") {
		return false
	}
	return strings.HasSuffix(pb, "/"+pa)
}

// schSplitPredicate peels a trailing gate predicate — "[$s]", "[$si]" — off a match
// pattern, returning the path and the predicate.
func schSplitPredicate(a string) (string, string) {
	if !strings.HasSuffix(a, "]") {
		return a, ""
	}
	i := strings.LastIndex(a, "[$")
	if i < 0 || strings.ContainsAny(a[i+2:len(a)-1], "[]/ ") {
		return a, ""
	}
	return a[:i], a[i:]
}
