package formalis

import (
	"context"
	"strings"
	"testing"
)

// The mutation fixtures behind the five national rule sets, in one place so that a
// guard can read all of them.
//
// Each suite is a conforming baseline document and a list of substitutions, one per
// rule, each of which must make exactly the rule it names fire. They live beside
// the baselines they mutate in the per-authority test files; the types and the
// runner are here because TestEveryEvaluatedCIUSRuleFires in cius_artefacts_test.go
// reads every suite to check that no evaluated identifier is left without a fixture
// that exercises it.

// ciusMutation is one fixture: a substitution on a conforming baseline that must
// make exactly one named rule fire.
type ciusMutation struct{ name, from, to, want string }

// ciusSuite is one authority's mutation suite. The four suites live in the
// per-authority test files beside the baseline document each mutates; this type is
// what lets one guard read all of them.
type ciusSuite struct {
	source   Source
	validate func(context.Context, []byte) (Report, error)
	baseline string
	prefix   string // the identifier prefix this suite's FP assertions scope to
	cases    []ciusMutation
	// extras are whole documents rather than mutations of the baseline, for a rule
	// the baseline cannot reach: a rule bound to cbc:CreditNoteTypeCode needs a
	// credit note, and turning an Invoice into one is not a substitution.
	extras []ciusDoc
}

// ciusDoc is a whole fixture document and the identifier it must make fire.
type ciusDoc struct{ name, xml, want string }

func ciusSuites() []ciusSuite {
	return []ciusSuite{
		{source: SourceCIUSPT, validate: ValidateCIUSPT, baseline: minimalCIUSPTUBL, prefix: "BR-CIUS-PT-", cases: ciusPTMutations},
		{source: SourceCIUSRO, validate: ValidateCIUSRO, baseline: minimalCIUSROUBL, prefix: "BR-RO-", cases: ciusROMutations},
		{source: SourceUBLBE, validate: ValidateUBLBE, baseline: minimalUBLBE, prefix: "ubl-BE-", cases: ublBEMutations},
		{source: SourceSRBDT, validate: ValidateSRBDT, baseline: minimalSRBDT, prefix: "RSR-", cases: srbdtMutations},
		{source: SourceNLCIUS, validate: ValidateNLCIUS, baseline: minimalNLCIUSUBL, prefix: "BR-NL-", cases: nlciusMutations},
	}
}

// runCIUSSuite is the body the five per-authority mutation tests share: the
// baseline must be clean of its authority's rules, and each mutation must make the
// rule it names fire.
func runCIUSSuite(t *testing.T, s ciusSuite) {
	t.Helper()
	scoped := func(vs []Violation) []string {
		var r []string
		for _, v := range vs {
			if strings.HasPrefix(v.Rule, s.prefix) {
				r = append(r, v.Rule)
			}
		}
		return r
	}
	if got := scoped(findings(t, context.Background(), s.validate, []byte(s.baseline))); len(got) != 0 {
		t.Fatalf("baseline %s invoice not clean: %v", s.source, got)
	}
	for _, tc := range s.cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(s.baseline, tc.from, tc.to, 1)
			if broken == s.baseline {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			got := findings(t, context.Background(), s.validate, []byte(broken))
			if !hasFacturXRule(got, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, scoped(got))
			}
		})
	}
	for _, d := range s.extras {
		t.Run(d.name, func(t *testing.T) {
			got := findings(t, context.Background(), s.validate, []byte(d.xml))
			if !hasFacturXRule(got, d.want) {
				t.Errorf("expected %s to fire; got %v", d.want, scoped(got))
			}
		})
	}
}
