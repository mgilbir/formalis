package formalis

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// allValidators is every exported entry point that reports a Report, keyed by
// the name it is called by. It is the surface the Source invariants below are
// checked over — and, since the return type became Report, the surface the
// coverage invariants in report_test.go are checked over too; a new validator
// belongs here.
var allValidators = map[string]func(context.Context, []byte) Report{
	"Validate":               func(c context.Context, b []byte) Report { return Validate(c, b, ProfileEN16931) },
	"ValidateCIUS":           ValidateCIUS,
	"ValidateXRechnung":      ValidateXRechnung,
	"ValidatePeppol":         ValidatePeppol,
	"ValidateNLCIUS":         ValidateNLCIUS,
	"ValidateCIUSPT":         ValidateCIUSPT,
	"ValidateCIUSRO":         ValidateCIUSRO,
	"ValidateUBLBE":          ValidateUBLBE,
	"ValidateSRBDT":          ValidateSRBDT,
	"ValidateOrderXML":       ValidateOrderXML,
	"ValidateFatturaPA":      ValidateFatturaPA,
	"ValidateFacturae":       ValidateFacturae,
	"ValidateEbInterface":    ValidateEbInterface,
	"ValidateKSeF":           ValidateKSeF,
	"ValidateFinvoice":       ValidateFinvoice,
	"ValidateTEAPPS":         ValidateTEAPPS,
	"ValidateOIOUBL":         ValidateOIOUBL,
	"ValidateSvefaktura":     ValidateSvefaktura,
	"ValidateZATCA":          ValidateZATCA,
	"ValidateOSA":            ValidateOSA,
	"ValidateTurkishInvoice": ValidateTurkishInvoice,
	"ValidatePINT":           ValidatePINT,
}

// claim is one sighting of a rule identifier: which Source stamped it, which
// validator produced it, and what it said. The message is kept so a collision
// report shows the two unrelated meanings side by side, which is the thing that
// makes the failure legible.
type claim struct {
	validator string
	message   string
}

// claims accumulates, per rule identifier, the Sources seen claiming it.
type claims map[string]map[Source]claim

// record folds one validator's findings in, returning a complaint for any
// finding that cannot be keyed at all.
func (c claims) record(validator string, vs []Violation) []string {
	var defects []string
	for _, v := range vs {
		// A Source that never got stamped is worse than a wrong one: it means an
		// emission site was added without deciding whose rule it reports, and ""
		// collides with every other unstamped rule in the package.
		if v.Source == "" {
			defects = append(defects, validator+" emitted "+v.Rule+" with no Source")
			continue
		}
		if v.Rule == "" {
			defects = append(defects, validator+" emitted a "+string(v.Source)+" finding with no Rule: "+v.Message)
			continue
		}
		if c[v.Rule] == nil {
			c[v.Rule] = map[Source]claim{}
		}
		if _, seen := c[v.Rule][v.Source]; !seen {
			c[v.Rule][v.Source] = claim{validator: validator, message: v.Message}
		}
	}
	return defects
}

// sweep is the result of running every exported validator over every document
// available: the identifier -> Source map, the rules each Source was seen to
// report, the severities each of those rules was reported at, and how many
// corpus files were read.
type sweep struct {
	claims claims
	byRule map[Source]map[string]bool
	// bySeverity is (Source, Rule) -> the set of severities the sweep saw it
	// emitted at. It is a set rather than one value because a rule with two
	// severities is a real thing — CEN flags BR-51 fatal in one binding and a
	// warning in the other — so a package that emitted one rule both ways would
	// be reporting a fact, and one that emitted it inconsistently would be
	// reporting a bug. Either way the set is what makes the difference visible.
	bySeverity map[Source]map[string]map[Severity]bool
	files      int
	defects    []string
}

// corpusSweep runs the full cross product once per test binary and memoises it.
//
// Two tests need it — the identifier-collision guard in this file and the
// coverage over-claim guard in report_test.go — and it is the most expensive
// thing the suite does: 22 validators over 1613 documents. Running it twice
// would add half again to the whole suite's runtime to learn nothing new, so
// the sweep gathers data and each test states its own assertions over it.
var corpusSweep = sync.OnceValue(func() *sweep {
	s := &sweep{
		claims:     claims{},
		byRule:     map[Source]map[string]bool{},
		bySeverity: map[Source]map[string]map[Severity]bool{},
	}
	ctx := context.Background()

	add := func(vname string, r Report) {
		s.defects = append(s.defects, s.claims.record(vname, r.Violations)...)
		for _, v := range r.Violations {
			if s.byRule[v.Source] == nil {
				s.byRule[v.Source] = map[string]bool{}
			}
			s.byRule[v.Source][v.Rule] = true
			if s.bySeverity[v.Source] == nil {
				s.bySeverity[v.Source] = map[string]map[Severity]bool{}
			}
			if s.bySeverity[v.Source][v.Rule] == nil {
				s.bySeverity[v.Source][v.Rule] = map[Severity]bool{}
			}
			s.bySeverity[v.Source][v.Rule][v.Severity] = true
		}
	}

	for _, doc := range collidingDocs {
		for vname, fn := range allValidators {
			add(vname, fn(ctx, []byte(doc)))
		}
	}
	_ = filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			// Dropping the document silently would shrink the population the
			// identifier map is built from without changing the count it
			// reports, which is the same class of quiet degradation the
			// ratchets exist to catch.
			s.defects = append(s.defects, p+": "+err.Error())
			return nil
		}
		s.files++
		for vname, fn := range allValidators {
			add(vname, fn(ctx, data))
		}
		return nil
	})
	return s
})

// check fails for any identifier two Sources both claim.
func (c claims) check(t *testing.T) {
	t.Helper()
	for rule, bySource := range c {
		if len(bySource) < 2 {
			continue
		}
		t.Errorf("rule identifier %q is claimed by %d Sources; (Source, Rule) is the identity, but a "+
			"caller aggregating or suppressing by Rule alone would merge these:", rule, len(bySource))
		for src, cl := range bySource {
			t.Errorf("    %-12s (from %s): %s", src, cl.validator, cl.message)
		}
	}
}

// collidingDocs are documents chosen to make a validator emit as much of its
// rule set as it can, so the sweep below sees identifiers the corpus may not
// exercise. The Order-X order is the one that matters: no Order-X document
// exists in testdata, which is precisely why the BR-O-01 collision survived
// until an audit constructed the cross product by hand.
var collidingDocs = map[string]string{
	// Empty of every mandatory head term: fires ORDER-01 … ORDER-05.
	"order-x, nothing filled in": `<SCRDMCCBDACIOMessageStructure/>`,
	// A type code outside UNTDID 1001's order values: the other ORDER-03 arm.
	"order-x, bad type code": `<SCRDMCCBDACIOMessageStructure><ExchangedDocument>` +
		`<ID>ORD-1</ID><TypeCode>380</TypeCode></ExchangedDocument>` +
		`</SCRDMCCBDACIOMessageStructure>`,
	// A "Not subject to VAT" line whose breakdown group carries a different
	// category: fires the EN 16931 BR-O-01 that orderx.go used to shadow.
	"en16931, O line without an O breakdown": strings.Replace(
		notSubjectToVATUBL, "<TaxCategory><ID>O</ID>", "<TaxCategory><ID>E</ID>", 1),
	// Roots no validator recognises, to reach each format's *-root finding.
	"unrecognised root": `<NotAnInvoice/>`,
	"bare ubl invoice":  `<Invoice/>`,
	"bare cii invoice":  `<CrossIndustryInvoice/>`,
}

// TestNoRuleIdentifierIsClaimedByTwoSources is the guard against the defect
// Violation.Source exists to make expressible, and that the Order-X rename
// removed the last instance of.
//
// A rule identifier is minted by one authority. When two authorities mint the
// same string — as orderx.go's invented BR-O-01 ("an Order shall have an order
// number") and CEN's BR-O-01 ("an Invoice with a Not subject to VAT line ...
// shall contain a VAT breakdown with that category") did — every caller that
// keys findings by Rule silently merges two unrelated defects, and a suppression
// list written for one takes effect on the other.
//
// The sweep runs every exported validator over every document available: the
// whole conformance corpus when it is present, plus the hand-written documents
// above, which cover the shapes the corpus does not contain at all. It then
// asserts that the identifier -> Source map it observed is single-valued.
//
// It is the full cross product on purpose, and it costs about as much as the
// rest of the suite. Restricting each document to the validators that plausibly
// own it is what a reasonable person would do, and it is exactly the assumption
// under which the collision hid: no single validator ever emitted both BR-O-01s.
// Sampling the corpus is no cheaper in the way that matters either — a quarter
// of the documents reaches 207 of the 226 identifiers the whole corpus does, so
// the discount is paid for in coverage of the very map being checked.
func TestNoRuleIdentifierIsClaimedByTwoSources(t *testing.T) {
	s := corpusSweep()
	for _, d := range s.defects {
		t.Error(d)
	}
	s.claims.check(t)
	// The hand-written documents carry this test on a corpus-less checkout, so a
	// zero here is the "corpus absent" case and not a truncation. Anything above
	// zero is a claim about the corpus and is held to the corpus's size.
	if s.files > 0 {
		atLeast(t, "identifier-collision sweep corpus", s.files, minCorpusDocuments)
	}
	t.Logf("checked %d rule identifiers across %d Sources, over %d corpus documents and %d hand-written ones",
		len(s.claims), countSources(s.claims), s.files, len(collidingDocs))
}

func countSources(c claims) int {
	srcs := map[Source]bool{}
	for _, bySource := range c {
		for s := range bySource {
			srcs[s] = true
		}
	}
	return len(srcs)
}

// TestOrderXAndVATCategoryOAreDistinguishable pins the specific collision the
// sweep above generalises, so the intent survives even if the corpus does not.
//
// Both halves are reproduced: the Order-X head-term findings, and the EN 16931
// "Not subject to VAT" findings. They must share no rule identifier, and the
// order validator must not mint anything inside CEN's BR-* space at all.
func TestOrderXAndVATCategoryOAreDistinguishable(t *testing.T) {
	ctx := context.Background()

	order := ValidateOrderXML(ctx, []byte(`<SCRDMCCBDACIOMessageStructure/>`)).Violations
	orderRules := map[string]bool{}
	for _, v := range order {
		if v.Source != SourceOrderX {
			t.Errorf("ValidateOrderXML emitted %q under Source %q, want %q", v.Rule, v.Source, SourceOrderX)
		}
		if strings.HasPrefix(v.Rule, "BR-") {
			t.Errorf("ValidateOrderXML minted %q inside CEN's BR-* namespace; this package numbers its own "+
				"rules in its own space (ORDER-*, like FPA-*/FE-*/ZA-*)", v.Rule)
		}
		orderRules[v.Rule] = true
	}
	for _, want := range []string{"ORDER-01", "ORDER-02", "ORDER-03", "ORDER-04", "ORDER-05"} {
		if !orderRules[want] {
			t.Errorf("an order with no head terms did not report %s; got %v", want, order)
		}
	}

	// An O line whose only breakdown group is categorised E: BR-O-01 fires.
	oInvoice := strings.Replace(notSubjectToVATUBL, "<TaxCategory><ID>O</ID>", "<TaxCategory><ID>E</ID>", 1)
	vat := Validate(ctx, []byte(oInvoice), ProfileEN16931).Violations
	foundBRO01 := false
	for _, v := range vat {
		if v.Rule != "BR-O-01" {
			continue
		}
		foundBRO01 = true
		if v.Source != SourceEN16931 {
			t.Errorf("BR-O-01 came back under Source %q, want %q", v.Source, SourceEN16931)
		}
		if orderRules[v.Rule] {
			t.Errorf("BR-O-01 is reported by both the order validator and the EN 16931 rule engine")
		}
	}
	if !foundBRO01 {
		t.Fatalf("the fixture no longer fires BR-O-01, so this test proves nothing; got %v", vat)
	}
}

// TestCheckerFindingsCarryTheCheckerSource pins the attribution of the two
// reserved identifiers. They are this package's statements about its own run,
// not any authority's rules, and IsCheckerViolation must keep recognising the
// stopped-run one — it is what stops "unknown" being read as "conformant".
func TestCheckerFindingsCarryTheCheckerSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v := Validate(ctx, []byte(validCII), ProfileEN16931).Violations
	if len(v) == 0 {
		t.Fatal("a cancelled run returned nothing, which reads as valid")
	}
	for _, e := range v {
		if e.Source != SourceChecker {
			t.Errorf("a cancelled run reported %q under Source %q, want %q", e.Rule, e.Source, SourceChecker)
		}
		if !IsCheckerViolation(e) {
			t.Errorf("IsCheckerViolation no longer recognises %q", e.Rule)
		}
	}

	// Malformed XML: a statement about the file, still made by the checker.
	bad := Validate(context.Background(), []byte(`<a></b>`), ProfileEN16931).Violations
	if len(bad) != 1 {
		t.Fatalf("malformed XML reported %d findings, want 1: %v", len(bad), bad)
	}
	if bad[0].Source != SourceChecker || bad[0].Rule != RuleSyntax {
		t.Errorf("malformed XML reported %q/%q, want %q/%q",
			bad[0].Source, bad[0].Rule, SourceChecker, RuleSyntax)
	}
	// RuleSyntax is a defect in the document, not the checker giving up, so the
	// predicate must not claim it.
	if IsCheckerViolation(bad[0]) {
		t.Error("IsCheckerViolation claimed a malformed-document finding; it means 'the checker stopped'")
	}
}

// TestViolationErrorNamesItsSource keeps the string form unambiguous: printing a
// bare rule identifier is exactly the habit that let two of them be confused.
//
// The severity is in the string for the same reason the Source is. Violation
// satisfies error, so it gets logged, and a log line that reads the same for an
// advisory finding as for a blocking one recreates the confusion one field
// along.
func TestViolationErrorNamesItsSource(t *testing.T) {
	got := Violation{Source: SourceNLCIUS, Rule: "BR-NL-1", Message: "the supplier shall be identified"}.Error()
	want := "NLCIUS BR-NL-1 (fatal): the supplier shall be identified"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
