package formalis

// Per-CIUS condition overrides: the mechanism by which a document validated under a
// national CIUS is judged by that authority's reading of a CEN rule and not by
// CEN's.
//
// # The problem this exists for
//
// A CIUS narrows EN 16931. Most of them do it by adding rules of their own, which
// this package evaluates alongside CEN's and reports under the authority's Source.
// But five of the seven do something else as well: instead of *referencing* CEN's
// Schematron they ship a **copy** of it, and a copy can be edited. Where an
// authority has edited one, its own validator evaluates the edited condition — so a
// package that evaluates CEN's condition and reports it under a CEN identifier says
// something about the document that the authority does not.
//
// That is the audit's C40, found in PR 23, confirmed in PR 26 and deliberately left
// alone until the shape below was decided.
//
// # Telling a national reading apart from a stale vendored copy
//
// The hard half is not applying an override, it is knowing which differences are
// overrides. A copy that differs from CEN's *current* file differs for one of two
// reasons, and they call for opposite treatment:
//
//   - the authority edited it. That is a national reading and this package should
//     honour it under that CIUS.
//   - CEN changed the file after the copy was taken. That is a lag, not a rule.
//     Honouring it would mean reporting BR-51 with a four-to-six-character
//     card-number test because a directory has not been refreshed since 2018, and
//     it would silently change what a CEN identifier means for every caller — the
//     objection on which PR 23 and PR 26 declined to act at all.
//
// testdata/cius-condition-overrides/gen.py separates them by asking CEN's own git
// history whether CEN ever published what a copy carries, at any commit, for that
// identifier. A condition CEN published is CEN's whenever it was written; one CEN
// never published is the authority's. That is a decision procedure and not a
// judgement, which matters here more than usual: the differences are overwhelmingly
// of a kind that looks cosmetic, and the audit's own reading of them was wrong.
// `exists(cbc:ID)` for BR-02 — recorded in C40 as AT/eSPap being more permissive
// than CEN, and as this package's one live false positive under CIUS-PT — is CEN's
// own text, published on 2017-03-14 and changed later. So are the dropped
// `cac:TaxScheme/cbc:ID='VAT'` predicates, BR-48's missing `or category='O'` escape,
// BR-CO-19's missing DescriptionCode alternative and UBL-SR-27 counting
// cbc:InstructionNote. None of them is Portuguese.
//
// What survives that filter is small and specific. See ciusCENCopyVerdicts for the
// per-authority counts and cius_overrides_test.go for the checks that re-derive
// them.
//
// # What an override is, mechanically
//
// The authority's condition is *evaluated*, not translated. Each override set
// carries the authority's own Schematron pattern — every <rule> of it, in the
// authority's order, with its context — and the overridden assertions with the
// authority's XPath, polarity and message verbatim. It is run by the same evaluator
// the generated CIUS-PT datatype and CIUS-RO tiers use (cius_pt_datatype_eval.go).
//
// Every rule of the pattern is present even when it carries no overridden
// assertion, because under ISO Schematron a node goes to the first rule whose
// context matches it: a rule dropped for carrying nothing would hand its nodes to a
// rule below it and change which nodes the override sees. Rule order has decided a
// rule's meaning four times in this repository already (PR 14, PR 23, PR 25, PR 26).
//
// Because the authority's condition is evaluated directly, the question "is this
// difference cosmetic" never arises at the point of evaluation. It arises once, in
// the generator, and is answered against CEN's history rather than by eye.
//
// # Where an override applies, and where it does not
//
// Only on the path that asked for that CIUS. ValidateCIUSPT applies AT/eSPap's
// reading; Validate, ValidatePeppol, ValidateXRechnung and everything else keep
// CEN's, and so does ValidateCIUS when it routes to a validator with no override
// set. A caller who asked for EN 16931 gets EN 16931.
//
// And only in the syntax the authority publishes for. CIUS-PT ships a UBL binding
// and no CII one, so a Factur-X invoice through ValidateCIUSPT is judged by CEN's
// conditions throughout — the same gate PR 22 put on the BR-CIUS-PT-* rules
// themselves, for the same reason (C36).
//
// # How a caller can tell
//
// Violation.Reading. An overridden finding keeps SourceEN16931 and CEN's identifier,
// because the identifier is CEN's and re-stamping it would make one identifier mean
// two things — the collision TestNoRuleIdentifierIsClaimedByTwoSources exists to
// prevent, and the reason PR 23 rejected the obvious fix. Reading names the
// authority whose condition was evaluated, is SourceNone on every finding judged by
// its own Source, and is rendered by Violation.Error, so the distinction reaches a
// caller who only logs the finding as well as one who reads the field.

// ciusOverrides is one authority's set of re-conditioned CEN rules.
//
// rules is the identifiers overridden and the severity to report them at, which is
// the authority's own flag rather than CEN's — an authority that re-wrote the
// condition may have re-flagged it too, and the flag is read from the same file as
// the condition.
//
// patterns is the authority's Schematron, and compiled is the same thing parsed once
// at load. syntax is the document syntax the authority publishes the binding for;
// an override never applies to the other one.
type ciusOverrides struct {
	authority Source
	syntax    string
	rules     map[string]Severity
	patterns  []ptDTPattern

	compiled  []*ptDTCompiledPattern
	rootNames map[string]bool
}

// ciusCENCopyVerdict is what one authority's copy of CEN's Schematron turned out to
// be, derived from the copy and from CEN's history rather than asserted.
//
// ships is false for an authority that publishes no copy at all. same, stale and own
// partition the CEN identifiers a copy does carry: identical to CEN's current file;
// different from it but published by CEN at some release; and carrying an axis —
// context, polarity or test — that CEN never published. own maps each such
// identifier to the axes that are the authority's.
//
// applied says whether this package evaluates that authority's conditions. It is
// true for an authority with nothing of its own, and notApplied carries the reason
// when it is false. An authority whose overrides are known and not applied is a
// stated gap with its identifiers named; the alternative — leaving them out of the
// table — is how a rule set comes to have a hole nobody can see (C27, C33, C39).
type ciusCENCopyVerdict struct {
	authority  string
	source     Source
	ships      bool
	shared     int
	same       int
	stale      int
	applied    bool
	notApplied string
	own        map[string]string
}

// mustCompileOverrides parses an override set's patterns once, at load.
//
// A parse failure panics, for the reason ptDTMustCompile gives: the table is
// generated and committed, so a row this evaluator cannot read is a defect in the
// build and not anything a caller did, and the alternative — dropping the row — is
// an override that silently stops being applied while the finding it was meant to
// replace goes on being reported.
func mustCompileOverrides(o *ciusOverrides) *ciusOverrides {
	o.rootNames = map[string]bool{}
	for i := range o.patterns {
		c := ptDTMustCompile(o.patterns[i])
		o.compiled = append(o.compiled, c)
		for n := range c.rootNames {
			o.rootNames[n] = true
		}
	}
	return o
}

// ptOverrides is AT/eSPap's reading of the nine CEN identifiers CIUS-PT 2.1.1
// re-conditions. See ciusCENCopyVerdicts for how the nine were arrived at.
var ptOverrides = mustCompileOverrides(ptConditionOverrides)

// applyConditionOverrides replaces this package's verdict on the identifiers o's
// authority re-conditioned with that authority's own verdict on the same document.
//
// It is two halves and both are necessary. Dropping the package's findings without
// evaluating the authority's would turn a national reading into a silent exemption;
// evaluating the authority's without dropping the package's would report the
// identifier twice, once each way.
//
// The findings it emits keep SourceEN16931 — the identifier is CEN's — and carry
// Reading, so the substitution is visible rather than silent. They are appended
// after the core findings that survived rather than in the position the replaced
// ones held: an overridden rule fires at the nodes the *authority's* context claims,
// which is not in general the order this package's own rules ran in, and inventing a
// position for it would be a fact about nothing.
func applyConditionOverrides(r *run, o *ciusOverrides, p *parsed, out []Violation) []Violation {
	if o == nil || p.inv == nil || p.inv.syntax != o.syntax {
		return out
	}
	kept := make([]Violation, 0, len(out))
	for _, v := range out {
		if v.Source == SourceEN16931 {
			if _, ok := o.rules[v.Rule]; ok {
				continue
			}
		}
		kept = append(kept, v)
	}
	doc := &ptDTDoc{root: p.root, want: o.rootNames}
	add := func(rule, msg string) {
		kept = append(kept, Violation{
			Source:   SourceEN16931,
			Rule:     rule,
			Severity: o.rules[rule],
			Message:  msg,
			Reading:  o.authority,
		})
	}
	for _, c := range o.compiled {
		ptDTRun(c, r, p.root, doc, add)
	}
	return kept
}
