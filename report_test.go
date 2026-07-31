package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Report exists to keep three different runs from producing the same answer:
// one that checked everything and found nothing, one that stopped before it had
// seen everything, and one whose rule set does not implement every rule its
// authority publishes. The first is the only one a caller may act on, and
// Conformant is the only predicate that tells it apart.
//
// The tests below are ordered as the argument is: first the five runs that must
// not be conformant, then the properties of the coverage table that make the
// third case machine-readable rather than a file comment.

// allSources is every Source constant that names an authority — all of them but
// SourceNone, which names the absence of one. A new authority belongs here, and
// TestEverySourceIsAccountedForInTheCoverageTable then forces a decision about
// whether its rule set is implemented in full.
var allSources = []Source{
	SourceEN16931, SourceFacturX, SourceXRechnung, SourcePeppol, SourceNLCIUS, SourceCIUSPT,
	SourceCIUSRO, SourceUBLBE, SourceSRBDT, SourceFatturaPA, SourceFacturae,
	SourceEbInterface, SourceKSeF, SourceFinvoice, SourceTEAPPS, SourceOIOUBL,
	SourceSvefaktura, SourceZATCA, SourceOSA, SourceUBLTR, SourcePINT,
	SourceOrderX, SourceChecker,
}

// completeSources are the Sources whose rule sets this package does not claim
// to have gaps in.
//
// SourceChecker is here because it publishes no rules at all: RuleLimit,
// RuleProfile and RuleRoot are this package's statements about its own run and
// about the file it was handed.
//
// SourceXRechnung is here because its rule set is finished. The Schematron a
// German buyer validates against is 78 identifiers — KoSIT's own 57 and the 21
// Peppol BIS Billing rules src/xsl/rule-list.xml whitelists — and all 78 are
// evaluated. The 21 arrive as findings under SourcePeppol, since Source names the
// authority that wrote the rule. See the comment on notEvaluated, and
// TestXRechnungImportsExactlyKoSITsWhitelist for the gate.
//
// SourcePeppol is here because the two OpenPEPPOL binding files are finished: the
// 59 PEPPOL-* identifiers and the 101 country-specific ones under "National
// rules", 244 (identifier, binding) pairs, each evaluated in the binding that
// publishes it. TestEveryPublishedPeppolRuleHasBothVerdicts requires a document
// that trips each pair and one that does not, so this entry rests on verdicts
// rather than on a count.
//
// SourceCIUSPT is here because the whole of AT/eSPap's published inventory is
// evaluated: the 65 BR-CIUS-PT-* identifiers and AT's own 8 BR-AA-* by hand in
// cius_pt_rules.go, and the 290 DT-CIUS-PT-* identifiers over the 291 assertions
// that carry them, generated from the Schematron into cius_pt_datatype_table.go.
// TestCIUSPTDatatypeTableHoldsThePublishedSet compares the generated set against
// the artefact in both directions, TestCIUSPTDatatypeContextsAreReachable says
// every one of them is asked of a real context node in the corpus, and
// TestEveryCIUSPTDatatypeRuleFires says every one of them has a document that
// makes it fire. It is the first CIUS whose *datatype* tier is implemented at all.
//
// If a Source is ever moved here it means someone finished implementing an
// authority's rule set, which is exactly the change that should be hard to make
// by accident.
var completeSources = map[Source]bool{SourceChecker: true, SourceXRechnung: true, SourcePeppol: true, SourceCIUSPT: true}

// TestConformantIsFalseForACancelledRun is the first of the five. It is the
// case limits.go already solved with RuleLimit; Complete has to keep solving
// it, because the whole point of the type is that one predicate answers both
// kinds of doubt.
func TestConformantIsFalseForACancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := mustReport(t, ctx, withProfile(ProfileEN16931), []byte(validCII))
	if r.Complete() {
		t.Error("a cancelled run reported Complete")
	}
	if r.Conformant() {
		t.Error("a cancelled run reported Conformant")
	}
	if len(r.Violations) == 0 {
		t.Fatal("a cancelled run returned no findings, which reads as valid")
	}
}

// TestConformantIsFalseForAnOverBudgetRun is the second: a run stopped by a
// resource budget rather than by the caller. It reaches Complete by the same
// route (a RuleLimit finding), and it is the case a caller is least likely to
// have thought about, since nothing the caller did caused it.
func TestConformantIsFalseForAnOverBudgetRun(t *testing.T) {
	r := mustReport(t, context.Background(), withProfile(ProfileEN16931), flatUBLBE(maxNodes+1))
	if r.Complete() {
		t.Error("an over-budget run reported Complete")
	}
	if r.Conformant() {
		t.Error("an over-budget run reported Conformant")
	}
	if !anyCheckerViolation(r.Violations) {
		t.Fatalf("an over-budget run reported no checker violation: %v", r.Violations)
	}
}

// TestConformantIsFalseForAnUnknownProfile is the third, and the one where the
// two halves of Complete come apart: no rule set was chosen, so there is no
// coverage to report, and Complete is false purely because of the RuleProfile
// finding. NotEvaluated is empty on purpose — naming the gaps of a rule set
// that never ran would suggest something was checked.
func TestConformantIsFalseForAnUnknownProfile(t *testing.T) {
	r := mustReport(t, context.Background(), withProfile(Profile("EN16931")), []byte(validCII))
	if r.Complete() {
		t.Error("an unknown Profile reported Complete; no rule set was chosen")
	}
	if r.Conformant() {
		t.Error("an unknown Profile reported Conformant")
	}
	if len(r.NotEvaluated) != 0 {
		t.Errorf("an unknown Profile named %d unevaluated rule families; no rule set was selected, so there are none to name: %v",
			len(r.NotEvaluated), r.NotEvaluated)
	}
	if len(r.Violations) != 1 || r.Violations[0].Rule != RuleProfile {
		t.Fatalf("an unknown Profile reported %v, want exactly one %q finding", r.Violations, RuleProfile)
	}
}

// TestConformantIsFalseForADocumentThatIsNotAnInvoice is the fourth. Here
// Conformant is false because of the finding, not because of doubt: "this is not
// an EN 16931 invoice" is a definite statement about a document that was read.
// The assertion is on Conformant rather than on Complete for exactly that reason.
//
// It was written against a malformed document, which is now an error rather than
// a Report and is covered by TestMalformedXMLIsAnError and by
// TestReportFromAnIgnoredErrorIsNotConformant below. The surviving half of that
// case — a document this package read and will not judge — is what this now
// exercises, and it is the half where the coverage assertion still means
// something: a Report exists on both sides, so the two can be compared.
func TestConformantIsFalseForADocumentThatIsNotAnInvoice(t *testing.T) {
	r := mustReport(t, context.Background(), withProfile(ProfileEN16931), []byte(unknownRoot))
	if r.Conformant() {
		t.Error("a document that is not an invoice reported Conformant")
	}
	if len(r.Violations) != 1 || r.Violations[0].Rule != RuleRoot {
		t.Fatalf("a document that is not an invoice reported %v, want exactly one %q finding", r.Violations, RuleRoot)
	}
	// The coverage claim must not depend on what the document turned out to be: a
	// caller comparing two Reports would otherwise read a shorter NotEvaluated
	// as better coverage when it only means the run gave up earlier.
	clean := mustReport(t, context.Background(), withProfile(ProfileEN16931), []byte(validCII))
	if !reflect.DeepEqual(r.NotEvaluated, clean.NotEvaluated) {
		t.Errorf("a document that is not an invoice reported different coverage from a readable one:\n  refused  %v\n  readable %v",
			r.NotEvaluated, clean.NotEvaluated)
	}
}

// TestReportFromAnIgnoredErrorIsNotConformant is the case the error return
// created, and the reason Report.ran has to exist rather than being an
// implementation detail nobody would miss.
//
// A caller who writes `r, _ := formalis.Validate(...)` — which Go makes easy, and
// which some caller will write — must not get a value that reads as a clean
// invoice. The Report beside an error is the zero Report, so it does not.
func TestReportFromAnIgnoredErrorIsNotConformant(t *testing.T) {
	r, _ := Validate(context.Background(), []byte(`<a></b>`), ProfileEN16931)
	if r.Conformant() {
		t.Error("the Report beside an error is Conformant; ignoring the error reads as a valid invoice")
	}
	if r.Complete() {
		t.Error("the Report beside an error is Complete")
	}
	if len(r.Violations) != 0 || len(r.NotEvaluated) != 0 {
		t.Errorf("the Report beside an error carries content: %+v", r)
	}
}

// TestConformantIsFalseForACleanDocumentUnderAPartialRuleSet is the fifth, and
// the one this whole file was written for. It is C12's failure scenario as a
// test: a document that produces no findings at all from a validator whose rule
// set is partial is not a document that passed that rule set. Before Report there
// was nothing a caller could read that said so.
//
// This test has now moved four times, and where it has moved to is the interesting
// part. It was asked of ValidateCIUSPT, whose fatal gap was first
// BR-CIUS-PT-13/15/17/18 and 24..63 and then the 290 DT-CIUS-PT-* datatype rules;
// then of ValidateCIUSRO, whose coverage named four Romanian families ANAF
// publishes; then of ValidateSRBDT, whose coverage named three Serbian ones. All
// three rule sets are finished, and what each has left is Unevaluable and therefore
// free — which is precisely the state this test must *not* be demonstrated on.
//
// With the last CIUS closed there is no CIUS left to ask, so it is asked of
// ValidateFatturaPA. That is not a downgrade: the thirteen tree-reading national
// validators are the *most* partial rule sets in this package (C12 said so, and the
// notEvaluated comment still does), each checking the mandatory structure and code
// lists of a format whose authority publishes a whole XSD, and the failure scenario
// is the same one — a caller reads len(Violations) == 0 from a validator that never
// ran the check the authority will reject the document on.
//
// The property is that a named, evaluable, fatal gap keeps Conformant false, and
// the rule set it is demonstrated on is incidental. If FatturaPA's gap is ever
// closed this moves again rather than being deleted:
// TestValidatorsWithAFatalGapAreTheOnesWeThinkTheyAre is what keeps the choice of
// validator honest, because it fails the moment the one named here stops having a
// fatal gap. The *positive* half of what CIUS-PT, CIUS-RO, SRBDT and UBL.BE became
// is asserted directly below, so nothing that was checked here stopped being
// checked.
func TestConformantIsFalseForACleanDocumentUnderAPartialRuleSet(t *testing.T) {
	r := mustReport(t, context.Background(), ValidateFatturaPA, []byte(minimalFatturaPA))
	for _, v := range r.Violations {
		if v.Source == SourceFatturaPA {
			t.Fatalf("the fixture is no longer clean under the FatturaPA rules, so this test proves nothing: %v", r.Violations)
		}
	}
	if r.Complete() {
		t.Error("ValidateFatturaPA reported Complete; it does not evaluate the SdI FatturaPA XSD")
	}
	if r.Conformant() {
		t.Error("ValidateFatturaPA reported Conformant on a document whose SdI schema checks were never run")
	}
	// The caller has to be able to find out *which* rules, not merely that some
	// exist, or the report is no more actionable than a file comment.
	var found bool
	for _, g := range r.NotEvaluated {
		if strings.Contains(g.Rules, "FatturaPA") {
			found = true
			if g.Severity != SeverityFatal {
				t.Errorf("the family the integrator would be rejected on is reported as %s: %+v", g.Severity, g)
			}
			if g.Unevaluable {
				t.Errorf("the family this test rests on is marked Unevaluable, which makes it free and makes "+
					"this test prove nothing: %+v", g)
			}
		}
	}
	if !found {
		t.Errorf("NotEvaluated does not name the rule family the integrator would be rejected on: %v", r.NotEvaluated)
	}
}

// TestCIUSROReportsConformantAndCompleteForACleanInvoice is the other half of that
// move, and the observable consequence of generating ANAF's mechanical tier.
//
// A Romanian invoice that breaks none of the 121 assertions CIUS-RO 1.0.9 publishes
// now reports Conformant() == true and Complete() == true. Before, it reported
// false for every document, whatever it contained, because four Romanian families
// were named as unevaluated.
//
// It is asserted on ANAF's own sample instances as well as on this package's
// baseline, because a fixture written to be clean under the rules this package
// implements is not evidence about the rules it implements. Conformant is asserted
// for the baseline and for the twenty-two instances that declare the current
// RO_CIUS identifier; the twenty-two older ones report BR-RO-001 against the
// release this package evaluates, which TestCIUSROCorpus derives from their BT-24.
func TestCIUSROReportsConformantAndCompleteForACleanInvoice(t *testing.T) {
	ctx := context.Background()
	docs := map[string][]byte{"baseline": []byte(minimalCIUSROUBL)}
	files, _ := filepath.Glob("testdata/cius-ro/testsuite/*.xml")
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs[filepath.Base(f)] = data
	}
	conformant := 0
	for name, data := range docs {
		r := mustReport(t, ctx, ValidateCIUSRO, data)
		if !r.Complete() {
			var evaluable []string
			for _, g := range r.NotEvaluated {
				if !g.Unevaluable {
					evaluable = append(evaluable, g.Rules)
				}
			}
			t.Errorf("%s: ValidateCIUSRO is not Complete; the gaps it names that a validator could evaluate: %v",
				name, evaluable)
		}
		if r.Conformant() {
			conformant++
		}
	}
	if !mustReport(t, ctx, ValidateCIUSRO, []byte(minimalCIUSROUBL)).Conformant() {
		t.Errorf("a clean Romanian invoice is not Conformant: %v",
			mustReport(t, ctx, ValidateCIUSRO, []byte(minimalCIUSROUBL)).Violations)
	}
	t.Logf("CIUS-RO: %d documents report Complete and %d of them Conformant", len(docs), conformant)
}

// TestCIUSPTReportsConformantAndCompleteForACleanInvoice is what the test above
// used to assert the negative of, and it is the observable consequence of
// generating AT/eSPap's datatype tier.
//
// A Portuguese invoice that breaks none of the 363 published CIUS-PT assertions
// now reports Conformant() == true and Complete() == true. Before, it reported
// false for every document, whatever it contained, because 290 fatal rules were
// named as unevaluated — the trap D10 describes, and the reason a caller could not
// use ValidateCIUSPT as a release gate at all.
//
// It is asserted on AT/eSPap's own sample instances rather than only on this
// package's baseline, because a fixture written to be clean under the rules this
// package implements is not evidence about the rules it implements.
func TestCIUSPTReportsConformantAndCompleteForACleanInvoice(t *testing.T) {
	ctx := context.Background()
	docs := map[string][]byte{"baseline": []byte(minimalCIUSPTUBL)}
	files, _ := filepath.Glob("testdata/cius-pt/testsuite/*.xml")
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs[filepath.Base(f)] = data
	}
	for name, data := range docs {
		r := mustReport(t, ctx, ValidateCIUSPT, data)
		if !r.Complete() {
			var evaluable []string
			for _, g := range r.NotEvaluated {
				if !g.Unevaluable {
					evaluable = append(evaluable, g.Rules)
				}
			}
			t.Errorf("%s: ValidateCIUSPT is not Complete; the gaps it names that a validator could evaluate: %v",
				name, evaluable)
		}
		// Conformant is asserted for the baseline alone. The AT instances trip
		// EN 16931 code-list rules from the core — AT's own 'AA' and 'NA' VAT
		// category codes are not in CEN's restricted BT-118 list — and settling
		// those is EN 16931 work rather than CIUS-PT work, as TestCIUSPTCorpus
		// records.
		if name == "baseline" && !r.Conformant() {
			t.Errorf("a clean Portuguese invoice is not Conformant: %v", r.Violations)
		}
	}
	t.Logf("CIUS-PT: %d documents report Complete, and a clean Portuguese invoice reports Conformant", len(docs))
}

// TestNLCIUSReportsCompleteForEveryDocument is the same kind of assertion for the
// Dutch path, and it is the one that has been true, then false, then true again.
//
// It became true when the "not recommended" tier was implemented: NLCIUS was the
// first rule set here whose last gap was advisory. It became false when the guard
// that enumerates published identifiers — rather than matching them against
// ^BR-NL- — found the one rule each binding names differently, SI-UBL-2 and
// empty-element-check, which nothing had counted. It is true again now that both
// are evaluated, and this time over an inventory that was enumerated: what is left
// in Coverage(SourceNLCIUS) is four assertions SimplerInvoicing publishes that no
// validator, its own included, ever reaches.
//
// Complete is the whole claim here, and Conformant is not asserted on any one
// document, because the fixture this rule set's mutation suite is built on is
// deliberately clean under the *NLCIUS* rules alone — it carries a tax
// representative without a VAT identifier, which BR-56 of the core reports. What is
// asserted instead is the property that decides Conformant for NLCIUS: that this
// rule set contributes no gap a validator could close, so its share of the verdict
// is the findings and nothing else. TestValidatorsWithAFatalGapAreTheOnesWeThinkThey-
// Are is the counterpart for the fatal half.
//
// Asserted over SimplerInvoicing's own instances as well as this package's
// baseline, for the reason the two tests above give: a fixture written to be clean
// under the rules this package implements is not evidence about them.
func TestNLCIUSReportsCompleteForEveryDocument(t *testing.T) {
	ctx := context.Background()
	docs := map[string][]byte{"baseline": []byte(minimalNLCIUSUBL)}
	for _, pat := range []string{"testdata/nlcius/testsuite/*.xml", "testdata/nlcius/gaccount/*.xml"} {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			docs[filepath.Base(f)] = data
		}
	}
	conformant := 0
	for name, data := range docs {
		r := mustReport(t, ctx, ValidateNLCIUS, data)
		if !r.Complete() {
			var evaluable []string
			for _, g := range r.NotEvaluated {
				if !g.Unevaluable {
					evaluable = append(evaluable, g.Rules)
				}
			}
			t.Errorf("%s: ValidateNLCIUS is not Complete; the gaps it names that a validator could evaluate: %v",
				name, evaluable)
		}
		if r.Conformant() {
			conformant++
		}
	}
	for _, g := range Coverage(SourceNLCIUS) {
		if !g.Unevaluable {
			t.Errorf("Coverage(SourceNLCIUS) names %q, which a validator could evaluate. NLCIUS's rule set is "+
				"claimed finished; an entry that is not Unevaluable un-does that claim", g.Rules)
		}
	}
	var got []string
	for _, v := range mustReport(t, ctx, ValidateNLCIUS, []byte(minimalNLCIUSUBL)).Violations {
		if v.Source == SourceNLCIUS {
			got = append(got, v.Rule)
		}
	}
	if len(got) != 0 {
		t.Errorf("the clean Dutch baseline reports %v", got)
	}
	t.Logf("NLCIUS: %d documents report Complete and %d of them Conformant", len(docs), conformant)
}

// TestNoAuthoritysRuleSetIsImplementedInFull records the state of the package, so
// that emptying an authority's coverage entry is a deliberate act rather than a
// side effect.
//
// It is about the *table*, and that is a stricter question than Report.Complete
// and no longer the same one. Every authority here publishes at least one rule
// this package does not evaluate, so every coverage entry is non-empty — and for
// the EN 16931 core the rules left are seven CEN itself cannot honour, so
// Report.Complete is true for a clean document while the entry is still, correctly,
// not empty. Those two facts only look contradictory until you read
// RuleFamily.Unevaluable: the table records what was not evaluated, Complete asks
// what could have been.
//
// So this test is not "Complete is false everywhere" (it was, and it is not any
// more). It is "nobody has quietly deleted a coverage entry". When someone really
// does finish an authority's rule set, this fails and asks them to move the Source
// to completeSources, which is a claim that deserves to be made on purpose.
// TestValidatorsWithAFatalGapAreTheOnesWeThinkTheyAre is the same kind of record
// for the fatal half, and
// TestTheCoreReportsConformantAndCompleteForACleanInvoice is the one that pins
// what a caller actually gets.
func TestNoAuthoritysRuleSetIsImplementedInFull(t *testing.T) {
	for _, src := range allSources {
		if completeSources[src] {
			continue
		}
		if len(Coverage(src)) == 0 {
			t.Errorf("Coverage(%q) is empty, so this package now claims to implement that authority's rule set in full. "+
				"If that is true, add it to completeSources and say so in the commit; if it is not, the table lost an entry", src)
		}
	}
}

// TestEverySourceIsAccountedForInTheCoverageTable makes the table total over
// the Source type. A Source with no entry and no place in completeSources is a
// rule set nobody decided about, which is precisely the state C12 described:
// the decision existed, in a file comment, and the API could not see it.
func TestEverySourceIsAccountedForInTheCoverageTable(t *testing.T) {
	for _, src := range allSources {
		_, listed := notEvaluated[src]
		if !listed && !completeSources[src] {
			t.Errorf("Source %q is in neither the coverage table nor completeSources: say whether its rule set is implemented in full", src)
		}
		if listed && completeSources[src] {
			t.Errorf("Source %q is claimed complete and also has coverage gaps listed", src)
		}
	}
	for src := range notEvaluated {
		var known bool
		for _, s := range allSources {
			if s == src {
				known = true
			}
		}
		if !known {
			t.Errorf("the coverage table lists %q, which is not a Source constant", src)
		}
	}
}

// TestCoverageReturnsACopy keeps the table from being editable through the
// accessor. It is package state that every Report is built from, so a caller
// that sorted the slice it got back would change what every later Report says.
func TestCoverageReturnsACopy(t *testing.T) {
	first := Coverage(SourceCIUSRO)
	if len(first) == 0 {
		t.Fatal("Coverage(SourceCIUSRO) is empty, so this test proves nothing")
	}
	first[0] = RuleFamily{Rules: "clobbered"}
	second := Coverage(SourceCIUSRO)
	if second[0].Rules == "clobbered" {
		t.Error("Coverage returns the table's own slice; a caller can rewrite every later Report")
	}
	if Coverage(SourceChecker) != nil {
		t.Error("Coverage(SourceChecker) is not nil; the checker publishes no rules")
	}
}

// TestComposedValidatorReportsTheUnionOfItsRuleSets pins the composition rule.
// ValidateCIUSPT runs the EN 16931 core *and* the CIUS-PT rules, so a caller
// reading its NotEvaluated has to see both sources' gaps; reporting only the
// CIUS half would say the core was complete, which is the same overstatement in
// a smaller place.
func TestComposedValidatorReportsTheUnionOfItsRuleSets(t *testing.T) {
	r := mustReport(t, context.Background(), ValidateCIUSPT, []byte(minimalCIUSPTUBL))
	want := append(Coverage(SourceEN16931), Coverage(SourceCIUSPT)...)
	if !reflect.DeepEqual(r.NotEvaluated, want) {
		t.Errorf("ValidateCIUSPT NotEvaluated =\n  %v\nwant the union of the core and CIUS-PT gaps:\n  %v", r.NotEvaluated, want)
	}
	seen := map[RuleFamily]int{}
	for _, g := range r.NotEvaluated {
		seen[g]++
	}
	for g, n := range seen {
		if n > 1 {
			t.Errorf("NotEvaluated repeats %q %d times; the union must dedupe", g.Rules, n)
		}
	}
}

// TestValidateCIUSReportsTheCoverageOfTheRuleSetItRan is the dispatcher's half.
// ValidateCIUS chooses a rule set from the document's BT-24, so its coverage
// claim has to follow the document: a Peppol invoice must come back with the
// Peppol gaps and not, say, the CIUS-PT ones, and a document declaring no
// recognised CIUS must come back with the core's gaps alone.
//
// It used to make that point with an XRechnung document and a second assertion
// that the result differed from the core's gaps alone. Both had to change, and the
// second is the interesting one: with KoSIT's 57 rules and the 21 it imports all
// evaluated, an XRechnung document routed here *does* come back with the core's
// gaps alone, because that is now the truth. The assertion encoded the state of
// the rule set rather than the property being tested, so the case moved to Peppol,
// whose rule set still names a gap, and the XRechnung case stayed as the union it
// is — which for XRechnung is the core's list.
func TestValidateCIUSReportsTheCoverageOfTheRuleSetItRan(t *testing.T) {
	ctx := context.Background()

	xr := mustReport(t, ctx, ValidateCIUS, []byte(minimalXRechnungUBL))
	wantXR := append(Coverage(SourceEN16931), Coverage(SourceXRechnung)...)
	if !reflect.DeepEqual(xr.NotEvaluated, wantXR) {
		t.Errorf("an XRechnung document routed through ValidateCIUS reported\n  %v\nwant the XRechnung union\n  %v", xr.NotEvaluated, wantXR)
	}

	pp := mustReport(t, ctx, ValidateCIUS, []byte(minimalPeppolUBL))
	wantPP := append(Coverage(SourceEN16931), Coverage(SourcePeppol)...)
	if !reflect.DeepEqual(pp.NotEvaluated, wantPP) {
		t.Errorf("a Peppol document routed through ValidateCIUS reported\n  %v\nwant the Peppol union\n  %v", pp.NotEvaluated, wantPP)
	}

	// The union has to be shown to be a union rather than a coincidence, and that
	// takes a CIUS whose own table is non-empty. It used to be XRechnung, then Peppol;
	// both rule sets have since been finished, so for both of them the union with the
	// core's gaps *is* the core's gaps, and asserting otherwise would be asserting
	// that a rule set is unfinished. UBL-BE is the case now — its Schematron is not
	// vendored, so its gaps are named rather than closed — and if its table is ever
	// emptied too this assertion has to move again rather than be deleted.
	be := mustReport(t, ctx, ValidateCIUS, []byte(minimalUBLBE))
	wantBE := append(Coverage(SourceEN16931), Coverage(SourceUBLBE)...)
	if !reflect.DeepEqual(be.NotEvaluated, wantBE) {
		t.Errorf("a UBL-BE document routed through ValidateCIUS reported\n  %v\nwant the UBL-BE union\n  %v", be.NotEvaluated, wantBE)
	}
	if reflect.DeepEqual(be.NotEvaluated, Coverage(SourceEN16931)) {
		t.Error("ValidateCIUS reported only the core's gaps for a document it validated against UBL-BE too")
	}

	// A document that declares no recognised CIUS is validated against the core
	// alone, so naming any CIUS's gaps would be a claim about a rule set that
	// never ran.
	core := mustReport(t, ctx, ValidateCIUS, []byte(minimalUBL))
	if !reflect.DeepEqual(core.NotEvaluated, Coverage(SourceEN16931)) {
		t.Errorf("a document with no recognised CIUS reported\n  %v\nwant the core's gaps alone\n  %v", core.NotEvaluated, Coverage(SourceEN16931))
	}
}

// TestEveryValidatorReportsItsCoverage sweeps the exported surface. Every
// validator here runs a partial rule set, so every one of them must say so on a
// document it can read; a validator that returns an empty NotEvaluated has
// either become complete (see TestNoAuthoritysRuleSetIsImplementedInFull) or forgotten to
// pass its Sources to newReport, and the second is silent.
func TestEveryValidatorReportsItsCoverage(t *testing.T) {
	ctx := context.Background()
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			// Any well-formed document will do: coverage is a property of the
			// rule set, not of the document, so it is reported even for one
			// this validator refuses.
			r := mustReport(t, ctx, fn, []byte(unknownRoot))
			if len(r.NotEvaluated) == 0 {
				t.Fatalf("%s reported no coverage gaps at all", name)
			}
			// Complete is an equality here rather than a one-sided assertion, which
			// is what it became when Unevaluable arrived. It used to read "a
			// validator naming any gap is not Complete", which was true only because
			// no rule set could reach Complete at all; the EN 16931 core reaches it
			// now, and pinning both directions is stronger than relaxing the old
			// claim to fit. A gap this package could close and did not must make
			// Complete false, and a table holding nothing but rules CEN cannot
			// honour must not.
			evaluable := 0
			for _, f := range r.NotEvaluated {
				if !f.Unevaluable {
					evaluable++
				}
			}
			if got := r.Complete(); got != (evaluable == 0) {
				t.Errorf("%s reported Complete() == %v while naming %d unevaluated rule families, %d of them evaluable",
					name, got, len(r.NotEvaluated), evaluable)
			}
			// unknownRoot is not an invoice in any format, so every validator reports
			// its own wrong-root finding, which is fatal. Conformant is therefore
			// false for every validator here whatever its coverage, including the one
			// whose coverage no longer costs a verdict.
			if r.Conformant() {
				t.Errorf("%s reported Conformant on a document that is not an invoice", name)
			}
		})
	}
}

// TestNotEvaluatedComesOnlyFromTheCoverageTable is the single-source-of-truth
// property. Every string a validator reports has to be a string the table
// holds, or there is a second list of coverage claims somewhere and the two
// will drift — which is how the file comments got out of date in the first
// place.
func TestNotEvaluatedComesOnlyFromTheCoverageTable(t *testing.T) {
	inTable := map[RuleFamily]bool{}
	for _, gaps := range notEvaluated {
		for _, g := range gaps {
			inTable[g] = true
		}
	}
	ctx := context.Background()
	for name, fn := range allValidators {
		for _, g := range mustReport(t, ctx, fn, []byte(unknownRoot)).NotEvaluated {
			if !inTable[g] {
				t.Errorf("%s reported the unevaluated family %q, which is not in the coverage table", name, g.Rules)
			}
		}
	}
}

// ruleIDRE matches a rule identifier written the way an authority writes it:
// upper-case family segments and then a numbered one, so "BR-CL-08",
// "UBL-SR-12" and "BR-DE-23-a" are identifiers and the family wildcard
// "UBL-SR-*" is not. It is used to read identifiers back out of the table's
// prose.
var ruleIDRE = regexp.MustCompile(`\b[A-Z]{2,7}(?:-[A-Z]{1,4})*-[0-9][A-Za-z0-9-]*\b`)

// carveOut reports whether an entry names the implemented part of a family
// rather than only unimplemented ones. See the convention documented on
// notEvaluated.
func carveOut(entry string) bool {
	return strings.Contains(entry, "other than") ||
		strings.Contains(entry, "emits only") ||
		strings.Contains(entry, "binding only")
}

// TestCoverageNamesNoRuleThePackageEmits is the over-claim guard, and the
// direction the file comments were wrong in when this table was derived: a
// comment that says a rule family is not emitted, when the code emits it,
// understates the package and sends a caller to re-implement work already done.
//
// It sweeps the whole corpus through every validator, collects the (Source,
// Rule) pairs actually reported, and fails if any of them is named in that
// Source's coverage entries. Entries that carve out the implemented part of a
// family are skipped, since naming it is their whole purpose.
func TestCoverageNamesNoRuleThePackageEmits(t *testing.T) {
	emitted := corpusSweep().byRule

	for src, gaps := range notEvaluated {
		for _, entry := range gaps {
			if carveOut(entry.Rules) {
				continue
			}
			// Both fields, so the split into Rules and Reason cannot weaken this
			// guard by moving a claim into the prose. The carve-out test above is
			// per entry rather than per field for the same reason: an entry that
			// names the implemented part of a family does so in Rules and then
			// explains it in Reason, and skipping only the first half would fail
			// on the explanation.
			for _, text := range []string{entry.Rules, entry.Reason} {
				for rule := range emitted[src] {
					if regexp.MustCompile(`\b` + regexp.QuoteMeta(rule) + `\b`).MatchString(text) {
						t.Errorf("Coverage(%q) claims %q is not evaluated, but a validator reported it: %q", src, rule, text)
					}
				}
			}
		}
	}
}

// TestEN16931CoverageNamesRulesCENPublishes checks the one Source for which
// ground truth is vendored. Every BR-*, UBL-* and CII-* identifier written into
// the EN 16931 entries must be an identifier the CEN Schematron actually
// defines, so the table cannot disclaim a rule family that does not exist —
// which would be a different kind of dishonesty from the one it fixes, and
// harder to notice.
func TestEN16931CoverageNamesRulesCENPublishes(t *testing.T) {
	dir := en16931SuiteDir()
	if dir == "" {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	published := map[string]bool{}
	for _, pat := range []string{
		filepath.Join(dir, "ubl", "schematron", "*", "*.sch"),
		filepath.Join(dir, "cii", "schematron", "*", "*.sch"),
	} {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			if strings.Contains(f, "preprocessed") {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(string(data), -1) {
				published[m[1]] = true
			}
		}
	}
	if len(published) < 1000 {
		t.Fatalf("read only %d rule identifiers from the CEN Schematron; the harness is not reading the artefacts", len(published))
	}

	// Both fields are read here, unlike the over-claim guard above, which reads
	// Rules alone. The two ask opposite questions: "does this identifier exist"
	// is safe to ask of prose, because an entry that mentions a rule CEN does not
	// publish is wrong wherever it mentions it, while "is this identifier
	// disclaimed" must be asked only of the field that does the disclaiming.
	for _, entry := range Coverage(SourceEN16931) {
		for _, text := range []string{entry.Rules, entry.Reason} {
			for _, id := range ruleIDRE.FindAllString(text, -1) {
				if !strings.HasPrefix(id, "BR-") && !strings.HasPrefix(id, "UBL-") && !strings.HasPrefix(id, "CII-") {
					continue
				}
				if !published[id] {
					t.Errorf("Coverage(SourceEN16931) names %q, which the CEN Schematron does not define: %q", id, text)
				}
			}
		}
	}
}

// schematronFlags reads every vendored Schematron this repository holds and
// returns, per rule identifier, the set of flags the authority put on it.
//
// A rule can carry two — BR-51 is fatal in the CII binding and a warning in the
// UBL one — which is the fact that put Severity on the finding rather than in a
// table keyed by identifier, and the reason this returns a set rather than one
// value.
func schematronFlags(t *testing.T) map[string]map[string]bool {
	t.Helper()
	dir := en16931SuiteDir()
	if dir == "" {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	pats := []string{
		filepath.Join(dir, "ubl", "schematron", "*", "*.sch"),
		filepath.Join(dir, "cii", "schematron", "*", "*.sch"),
		filepath.Join("testdata", "xrechnung", "schematron", "src", "validation", "schematron", "*", "*.sch"),
		filepath.Join("testdata", "peppol", "repo", "rules", "sch", "*.sch"),
	}
	flags := map[string]map[string]bool{}
	for _, pat := range pats {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			if strings.Contains(f, "preprocessed") {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for id, flag := range assertFlags(t, f, data) {
				if flags[id] == nil {
					flags[id] = map[string]bool{}
				}
				for fl := range flag {
					flags[id][fl] = true
				}
			}
		}
	}
	if len(flags) < 1780 {
		t.Fatalf("read flags for only %d rules from the vendored Schematron; the harness is not reading the artefacts", len(flags))
	}
	return flags
}

// assertFlags reads one Schematron file's <assert>/<report> identifiers and flags
// with an XML decoder rather than a regular expression.
//
// It was a regular expression, `<(?:sch:)?(?:assert|report)\s([^>]*)>`, and that
// is a bug with a measurable size: the character class stops at the first '>',
// and a Schematron test is an XPath expression that may contain one. Across the
// four vendored rule sets it lost exactly three assertions, all KoSIT's —
// BR-DE-19 and BR-DE-20, whose IBAN check-digit arithmetic reads "if($cp > 64)",
// and BR-DEX-02, which counts "cac:SubInvoiceLine) > 0". Nothing noticed, because
// a rule the harness cannot see has no flag to disagree with: the coverage table
// filed BR-DEX-02 as fatal where KoSIT flags it warning and the severity test had
// no opinion, and the count of KoSIT's published identifiers read 54 instead of
// 57 everywhere it was quoted, including in a coverage-entry comment.
//
// A decoder also drops commented-out assertions, which the regular expression
// read as live. One exists: PEPPOL-COMMON-R048 is inside an XML comment in both
// Peppol binding files, so Coverage(SourcePeppol) names as an advisory gap a rule
// OpenPEPPOL has switched off. That is a Peppol entry and out of this change's
// scope, but it is the reason the floor above is a floor and not an equality.
func assertFlags(t *testing.T, name string, data []byte) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	// CEN ships one of the two binding files declared ISO-8859-1, and the decoder
	// refuses a non-UTF-8 declaration outright without this. readSchPattern in
	// en16931_syntax_advisory_test.go makes the same conversion and argues it: in
	// Latin-1 the byte value is the code point, so widening cannot be ambiguous.
	dec.CharsetReader = latin1Reader
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || (se.Name.Local != "assert" && se.Name.Local != "report") {
			continue
		}
		id, flag := "", "none"
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "id":
				id = a.Value
			case "flag":
				flag = a.Value
			}
		}
		if id == "" {
			continue
		}
		if out[id] == nil {
			out[id] = map[string]bool{}
		}
		out[id][flag] = true
	}
	return out
}

// severityOfFlag folds an authority's flag onto this package's two values. CEN
// and OpenPEPPOL write fatal and warning; KoSIT adds information, which is
// advisory under another name.
//
// "none" is assertFlags's marker for an assertion that carries no flag attribute,
// and it folds onto fatal. That is a derivation and not a fail-safe guess: phive
// runs ph-schematron, whose DefaultSVRLErrorLevelDeterminator maps a flag it cannot
// recognise — and null is one — onto DEFAULT_ERROR_LEVEL, declared as
// EErrorLevel.ERROR. A reference validation therefore rejects the document.
//
// Exactly one assertion in every artefact this repository vendors is written that
// way: BR-GA-6, in the G-account extension. TestGAccountSeveritiesAreThePublishedFlags
// pins both halves — that the artefact carries no flag on it, and that this package
// reports it fatal — so the day SimplerInvoicing adds one, whichever it adds, the
// build says so.
func severityOfFlag(flag string) (Severity, bool) {
	switch flag {
	case "fatal", "none":
		return SeverityFatal, true
	case "warning", "information":
		return SeverityWarning, true
	}
	return SeverityFatal, false
}

// severityTables are the tables that decide, per rule identifier, which severity
// a finding may carry. A rule is allowed to arrive as a warning only because one
// of these says an authority flagged it that way, and the check below is over the
// tables themselves rather than over a pattern on the identifier, so a rule cannot
// become excusable by being named like one.
//
//   - advisoryRuleIDs() is the generated EN 16931 syntax-binding table: 1,168
//     rules CEN flags warning.
//   - xrechnungFlags is KoSIT's flag for every XRechnung identifier this package
//     evaluates. Eleven are not fatal — seven flagged warning, one information,
//     and three more (BR-DE-19, BR-DE-20, BR-DEX-02) that a regular-expression
//     harness could not see at all.
//   - peppolRules is OpenPEPPOL's flag for every identifier it publishes, and
//     peppolXRFlags is KoSIT's where the merge into XRechnung changes it.
//   - nlciusAdvisoryRuleIDs() is the "not recommended" tier of SimplerInvoicing's
//     two bindings: the identifiers nlcius.go emits through advisoryAdder, read
//     out of the two tables that drive it rather than listed again here.
//     ciusEvaluated in cius_artefacts_test.go is what holds those identifiers to
//     the flag SimplerInvoicing publishes for each, against the artefact and in
//     both directions, so this entry does not have to repeat that claim — it only
//     has to say which identifiers a warning is expected from.
//
// The value is a *set* of severities rather than one severity, and that is the one
// widening this test has taken. It is not an excuse list: it exists because a rule
// can genuinely carry two published flags, and one does. OpenPEPPOL flags
// PEPPOL-EN16931-R120 fatal and KoSIT's merge re-flags it warning, so the same
// identifier is a non-conformance on the Peppol path and a warning on the
// XRechnung path — which is the fact that put Severity on the Violation rather
// than in a table keyed by identifier (see schematronFlags, where BR-51 is the
// same shape). Where a rule has one published flag both directions are still
// pinned exactly as before, and the per-path reading of the one that has two is
// asserted on its own in TestR120IsAdvisoryOnlyWhereKoSITSaysSo.
func severityTables() map[Source]map[string]map[Severity]bool {
	out := map[Source]map[string]map[Severity]bool{
		SourceEN16931:   {},
		SourceXRechnung: {},
		SourcePeppol:    {},
		SourceNLCIUS:    {},
		SourceFacturX:   {},
	}
	record := func(src Source, rule string, sev Severity) {
		if out[src][rule] == nil {
			out[src][rule] = map[Severity]bool{}
		}
		out[src][rule][sev] = true
	}
	for rule := range advisoryRuleIDs() {
		record(SourceEN16931, rule, SeverityWarning)
	}
	for rule, sev := range xrechnungFlags {
		record(SourceXRechnung, rule, sev)
	}
	for rule, r := range peppolRules {
		record(SourcePeppol, rule, r.severity)
	}
	// The country-specific rules of the same two OpenPEPPOL files. They are a
	// separate table because they are a separate rule set, and both are quotations
	// of the same artefact.
	for rule, r := range peppolCountryRules {
		record(SourcePeppol, rule, r.severity)
	}
	for rule, sev := range peppolXRFlags {
		record(SourcePeppol, rule, sev)
	}
	for rule := range nlciusAdvisoryRuleIDs() {
		record(SourceNLCIUS, rule, SeverityWarning)
	}
	// Factur-X. The severity of a BR-FXEXT-* rule is decided at the emission site
	// by facturXExtensionSeverity, so that is the table read here; every
	// identifier absent from it is fatal, which is what an unflagged assertion is
	// in an artefact that flags 21 of them warning explicitly.
	// TestFacturXExtensionSeveritiesMatchTheArtefact reads the flags back out of
	// FNFE's own files, which is what makes this a quotation rather than a
	// restatement.
	for _, rule := range facturXExtensionRules {
		record(SourceFacturX, rule, facturXExtensionSeverity[rule])
	}
	return out
}

// nlciusAdvisoryRuleIDs is the set of identifiers nlcius.go emits as warnings,
// gathered from the tables that drive it rather than written out a second time.
// A list maintained beside the one it describes is a list that drifts.
func nlciusAdvisoryRuleIDs() map[string]bool {
	out := map[string]bool{}
	for _, table := range [][]nlciusDiscouraged{nlciusUBLDiscouraged, nlciusCIIDiscouraged} {
		for _, d := range table {
			out[d.id] = true
		}
	}
	// The four that test something rather than merely being reached, and so are not
	// in either forbidden-path table. BR-NL-31 is UBL-only: the CII binding's copy
	// is unevaluable.
	for _, id := range []string{"BR-NL-25", "BR-NL-29", "BR-NL-31", "BR-NL-33"} {
		out[id] = true
	}
	// The empty-element rule, which is neither a forbidden path nor a test but a
	// context: one identifier per binding, flag="warning" in both artefacts.
	for _, id := range []string{"SI-UBL-2", "empty-element-check"} {
		out[id] = true
	}
	return out
}

// TestOnlySeveritiesAnAuthorityPublishedAreEmittedAsWarnings verifies the claim
// that lets Severity have a zero value at all: a finding arrives as a warning
// only where its authority flagged the rule that way, so the fail-safe default is
// the correct answer everywhere else and no emission site is relying on the
// default to mean something it did not decide.
//
// It sweeps the whole corpus through every validator — the same sweep the
// identifier-collision guard uses — and checks each rule's severity against the
// table that decided it, in both directions: a rule an authority flags advisory
// must never arrive fatal, and one it flags fatal must never arrive as a warning.
//
// This was TestOnlyTheAdvisoryBindingsAreEmittedAsWarnings, and before that
// TestEveryEmittedFindingIsFatalToday. Each rewrite has been the same correction
// in a smaller form: the test asserted that the *only* non-fatal rules in the
// package were the ones implemented most recently, which was true when written
// and stopped being true as soon as another authority's advisory tier arrived.
// KoSIT flags eleven of its fifty-seven rules warning or information, and this
// package reported seven of them fatal — so a document KoSIT accepts with a
// warning about its telephone number was refused here, and this assertion was
// what said that had to be so.
func TestOnlySeveritiesAnAuthorityPublishedAreEmittedAsWarnings(t *testing.T) {
	tables := severityTables()
	s := corpusSweep()
	for src, rules := range s.bySeverity {
		for rule, sevs := range rules {
			published, listed := tables[src][rule]
			for sev := range sevs {
				switch {
				case !listed && sev != SeverityFatal:
					t.Errorf("%s/%s was reported as %s, and no table in this package records its authority flagging "+
						"it that way. A severity is a quotation: either the table is missing an entry or the "+
						"emission site chose one", src, rule, sev)
				case listed && !published[sev]:
					t.Errorf("%s/%s was reported as %s, and the flags its authorities publish for it are %v. A "+
						"severity is a quotation and not a choice", src, rule, sev, keysOfSeverity(published))
				}
			}
		}
	}
	// corpusFetched and not s.files > 0, for the reason corpus_test.go gives: the
	// sweep's own count includes hand-written and committed documents and is
	// therefore non-zero on a checkout that fetched nothing.
	if corpusFetched() {
		atLeast(t, "severity sweep corpus", s.files, minCorpusDocuments)
	}
}

// keysOfSeverity renders a severity set for an error message.
func keysOfSeverity(set map[Severity]bool) []string {
	var out []string
	for sev := range set {
		out = append(out, sev.String())
	}
	sort.Strings(out)
	return out
}

// TestEveryEmittedEN16931RuleCarriesCENsFlag is the same claim checked against
// ground truth rather than against itself, for the one authority whose rule set
// this repository vendors.
//
// The test above only proves the package is self-consistent: it would pass if
// every finding were stamped from the right table and the tables themselves were
// wrong about CEN. This one reads the flag off the assertion CEN published, for
// every rule the sweep saw emitted, and compares it with the severity the finding
// carried. It is what makes "this package reports the severity its authority
// published" a checked statement in both directions — a fatal rule mislabelled
// advisory would let a real non-conformance past Report.Conformant, and an
// advisory rule mislabelled fatal would fail a conforming invoice.
//
// It was TestEveryEmittedEN16931RuleIsFatalInCENsSchematron, which could only
// express one of those two directions because only one was possible.
func TestEveryEmittedEN16931RuleCarriesCENsFlag(t *testing.T) {
	flags := schematronFlags(t)
	emitted := corpusSweep().bySeverity[SourceEN16931]
	if len(emitted) < 100 {
		t.Fatalf("the sweep saw only %d EN 16931 rules; the corpus is not present, so this proves nothing", len(emitted))
	}
	var unpublished []string
	for rule, sevs := range emitted {
		got, ok := flags[rule]
		if !ok {
			// The VAT-category families are generated from one template per
			// category (BR-S-08, BR-Z-08, ...) and CEN publishes only some of
			// the cells; a rule the Schematron does not carry has no flag to
			// compare against and is reported rather than silently skipped.
			unpublished = append(unpublished, rule)
			continue
		}
		want, known := severityOfFlag(pickFlag(got))
		if !known {
			t.Errorf("%s carries the flag %v, which this package does not know how to fold onto a Severity", rule, keysOf(got))
			continue
		}
		for sev := range sevs {
			if sev != want {
				t.Errorf("this package reports EN 16931 %s as %s, but CEN flags it %v; the severity a finding carries "+
					"is a quotation and not a choice", rule, sev, keysOf(got))
			}
		}
	}
	sort.Strings(unpublished)
	t.Logf("checked %d emitted EN 16931 rules against CEN's flags; %d are not in the vendored Schematron at all: %v",
		len(emitted)-len(unpublished), len(unpublished), unpublished)
}

// TestCoverageSeveritiesMatchThePublishedFlag holds the coverage table's
// severity column to the same standard as the findings: for every family whose
// identifiers this repository can look up, the severity must be the one the
// authority published. Unconditionally — there is no excused set.
//
// There was one, and deleting it is the evidence D10 was the right change. Seven
// identifiers were listed here as allowed to diverge from their published flag,
// because six of them are rules CEN flags fatal and cannot itself honour, and
// recording them at SeverityWarning was the only way to keep Report.Conformant
// from being false for every document forever. That made this test a
// hand-maintained list that had to be edited every time such a rule was found —
// the same failure mode as a coverage claim written in a file comment — and it
// made RuleFamily.Severity two things at once: a quotation for most entries and
// an estimate of cost for those seven.
//
// RuleFamily.Unevaluable carries the second fact now, so the column is a
// quotation everywhere and this test needs no exceptions. If an exception ever
// looks necessary again, the model is wrong rather than the entry: either the
// severity is misquoted, or the family is unevaluable and should say so in the
// field built for it.
func TestCoverageSeveritiesMatchThePublishedFlag(t *testing.T) {
	flags := schematronFlags(t)

	checked := 0
	for _, src := range []Source{SourceEN16931, SourceXRechnung, SourcePeppol} {
		for _, entry := range Coverage(src) {
			if carveOut(entry.Rules) {
				continue
			}
			for _, id := range coverageIdentifiers(entry.Rules) {
				got, ok := flags[id]
				if !ok {
					continue // a range endpoint the authority does not publish
				}
				want, known := severityOfFlag(pickFlag(got))
				if !known {
					t.Errorf("%s carries the flag %v, which this package does not know how to fold onto a Severity", id, keysOf(got))
					continue
				}
				checked++
				if entry.Severity != want {
					t.Errorf("Coverage(%q) records the family %q as %s, but %s is flagged %v. Severity is the "+
						"authority's flag quoted and nothing else; if this family cannot be evaluated by anyone, "+
						"that is RuleFamily.Unevaluable's job: %q",
						src, entry.Rules, entry.Severity, id, keysOf(got), entry.Reason)
				}
			}
		}
	}
	// The floor is on the helper below, not on a corpus: it is what stops
	// coverageIdentifiers from silently regressing to reading only the identifiers
	// written out in full. Two bugs in that helper — see numTailRE and fullIDRE —
	// meant that of the identifiers this table names and the artefacts publish, 44
	// were skipped without a word, and the 23 left cleared the previous floor of 30
	// only while the XRechnung entries were long.
	//
	// A number that has to come down means either the helper broke or a rule set
	// finished, and the second belongs in a commit message: this test's population
	// is the *gaps*, so it shrinks as rules are implemented. It went from 71 to 52
	// while KoSIT's rules were being implemented, back to 67 when the twenty-one
	// Peppol rules XRechnung imports were recorded, to 108 when those were
	// implemented and Coverage(SourcePeppol) came to name the 101 country-specific
	// rules in the same OpenPEPPOL files instead, and down again as those are
	// implemented in turn.
	//
	// Which is why the floor is no longer the only thing holding the helper up.
	// The population it measures is on its way to seven — the three unevaluable
	// EN 16931 families — and a floor of seven guards nothing, so
	// TestCoverageIdentifiersExpandsFamilies pins the helper directly, on literal
	// inputs including the two shapes those bugs got wrong. The floor stays as the
	// cheaper end-to-end signal that the artefacts are being read at all.
	if checked < 7 {
		t.Fatalf("only %d coverage identifiers could be looked up; the harness is reading the wrong artefacts", checked)
	}
	t.Logf("checked the severity of %d coverage identifiers against the flags their authorities publish, with no exceptions", checked)
}

// TestCoverageIdentifiersExpandsFamilies pins coverageIdentifiers on literal
// inputs, so the helper's correctness stops depending on how many gaps happen to
// be left in the table.
//
// The first two cases are the two bugs C31 records, written as inputs: an
// identifier whose family name contains digits (fullIDRE rejected it, so no
// Peppol prefix was ever carried) and one whose first digit run is not its number
// (numTailRE split "PEPPOL-EN16931-R001" into "PEPPOL-EN" + "16931"). Both would
// pass silently again if the helper regressed, because a helper that resolves
// nothing makes this test's caller check nothing.
func TestCoverageIdentifiersExpandsFamilies(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"PEPPOL-EN16931-R040..R044, R046", []string{
			"PEPPOL-EN16931-R040", "PEPPOL-EN16931-R041", "PEPPOL-EN16931-R042",
			"PEPPOL-EN16931-R043", "PEPPOL-EN16931-R044", "PEPPOL-EN16931-R046"}},
		{"BR-CO-05..08", []string{"BR-CO-05", "BR-CO-06", "BR-CO-07", "BR-CO-08"}},
		{"CII-DT-010, CII-DT-011, CII-DT-012", []string{"CII-DT-010", "CII-DT-011", "CII-DT-012"}},
		{"DK-R-002, DK-R-004..006", []string{"DK-R-002", "DK-R-004", "DK-R-005", "DK-R-006"}},
		{"GR-R-001-1, GR-S-008-1", []string{"GR-R-001-1", "GR-S-008-1"}},
	} {
		got := coverageIdentifiers(tc.in)
		sort.Strings(got)
		want := append([]string(nil), tc.want...)
		sort.Strings(want)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("coverageIdentifiers(%q) = %v, want %v", tc.in, got, want)
		}
	}
}

// coverageIdentifiers expands one entry's Rules field into every identifier it
// names.
//
// The table writes families the way the authority does — "BR-DEX-01..14",
// "PEPPOL-EN16931-R002/R006/R008/R040..R046/R051" — where all but the first
// identifier is abbreviated to its numeric tail and a range is two endpoints.
// Reading only the identifiers written out in full checks about a fifth of them,
// which is thin enough that a wrong severity could hide in the abbreviation; this
// carries the prefix of the last full identifier across the tail and expands the
// ranges, and the caller's own floor on the number looked up is what keeps this
// helper from silently regressing to that fifth.
var (
	// numTailRE splits an identifier at its *last* digit run, which the trailing
	// \D* is what enforces. It was `^(.*?)(\d+)(.*)$`, whose non-greedy head takes
	// the first digit run instead: "PEPPOL-EN16931-R001" came apart as
	// "PEPPOL-EN" + "16931" + "-R001", so the prefix carried across the following
	// tails was "PEPPOL-EN" and every abbreviated Peppol identifier resolved to a
	// string no authority publishes. Every rule set whose family names hold no
	// digits was unaffected, which is why it looked right.
	numTailRE = regexp.MustCompile(`^(.*?)(\d+)(\D*)$`)
	tokenSep  = regexp.MustCompile(`[\s,;()]+`)
	// fullIDRE recognises a token that is an identifier written out in full,
	// rather than the numeric tail of one ("R049", "14", "06-b").
	//
	// It replaces ruleIDRE here, which was doing a job it cannot do: ruleIDRE
	// requires every segment before the numbered one to be letters only, and
	// "PEPPOL-EN16931-R001" has a segment that is not. So no Peppol identifier
	// resolved, no Peppol prefix was ever carried, and every identifier in
	// Coverage(SourcePeppol) — 46 of them across seven entries — was skipped by
	// the severity check silently. The floor the caller asserts did not catch it
	// because the XRechnung entries alone cleared it.
	fullIDRE = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Za-z0-9]+)+$`)
)

func coverageIdentifiers(rules string) []string {
	var out []string
	prefix := "" // everything before the digits of the last full identifier
	// resolve turns one token into a full identifier, using the carried prefix
	// when the token is only a numeric tail ("R049", "35", "06-b").
	resolve := func(tok string) string {
		tok = strings.Trim(tok, ".")
		if tok == "" {
			return ""
		}
		if fullIDRE.MatchString(tok) {
			if m := numTailRE.FindStringSubmatch(tok); m != nil {
				prefix = m[1]
			}
			return tok
		}
		if prefix == "" {
			return ""
		}
		if numTailRE.FindStringSubmatch(tok) == nil {
			return ""
		}
		// A tail may name its own letter segment — "R049" under the prefix
		// "PEPPOL-EN16931-R", "P0100" under "PEPPOL-EN16931-F" — in which case the
		// prefix's trailing letters are the ones being replaced and not repeated.
		// This was a HasSuffix test on the first character, which handled the
		// repeated case and turned the replaced one into "PEPPOL-EN16931-FP0100".
		if tok[0] >= 'A' && tok[0] <= 'Z' {
			return strings.TrimRight(prefix, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") + tok
		}
		return prefix + tok
	}
	for _, tok := range tokenSep.Split(rules, -1) {
		for _, part := range strings.Split(tok, "/") {
			lo, hi, isRange := strings.Cut(part, "..")
			from := resolve(lo)
			if from == "" {
				continue
			}
			out = append(out, from)
			if !isRange {
				continue
			}
			to := resolve(hi)
			if to == "" {
				continue
			}
			// Expand the range over the numeric part the two endpoints share,
			// keeping the width so "01..14" yields "01" and not "1".
			a, b := numTailRE.FindStringSubmatch(from), numTailRE.FindStringSubmatch(to)
			if a == nil || b == nil || a[1] != b[1] {
				continue
			}
			lastN, errLast := strconv.Atoi(b[2])
			firstN, errFirst := strconv.Atoi(a[2])
			if errLast != nil || errFirst != nil {
				continue
			}
			for n := firstN + 1; n <= lastN; n++ {
				out = append(out, fmt.Sprintf("%s%0*d%s", a[1], len(a[2]), n, b[3]))
			}
		}
	}
	return out
}

// pickFlag reduces a rule's flag set to the one that decides its severity: fatal
// if any binding flags it fatal. That is the same fail-safe direction Severity's
// zero value takes, and it is how this package already treats BR-51 — evaluated
// in the binding that makes it fatal.
func pickFlag(got map[string]bool) string {
	if got["fatal"] {
		return "fatal"
	}
	for f := range got {
		return f
	}
	return "none"
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestZeroReportIsNotConformant pins the value a caller gets from a variable
// nobody filled in, or from a decoded JSON object with no such field. It must not
// read as a clean invoice.
//
// This is the guard that Complete-as-a-method put at risk. While Complete was a
// field it was false in the zero value for free, and Conformant was the
// conjunction of "no findings" and it. Computed from NotEvaluated instead, a
// Report with no findings and no gaps answers true to both — and that is exactly
// the zero Report. Report.ran is what restores the property; these two tests are
// what keep it, because nothing else in the suite would notice its removal.
func TestZeroReportIsNotConformant(t *testing.T) {
	var r Report
	if r.Conformant() {
		t.Error("the zero Report is Conformant; a Report nobody filled in reads as a valid invoice")
	}
	if r.Complete() {
		t.Error("the zero Report is Complete")
	}
	if len(r.Fatal()) != 0 || len(r.Warnings()) != 0 {
		t.Error("the zero Report has findings")
	}

	// A copy is as good as the original, and a Report assembled by a caller out
	// of parts is not: the field is unexported, so a value built outside this
	// package cannot claim to have been validated by it.
	real := mustReport(t, context.Background(), ValidateCIUSPT, []byte(minimalCIUSPTUBL))
	copied := real
	if copied.Conformant() != real.Conformant() || copied.Complete() != real.Complete() {
		t.Error("copying a Report changed what it claims")
	}
	assembled := Report{Violations: nil, NotEvaluated: nil}
	if assembled.Conformant() || assembled.Complete() {
		t.Error("a Report a caller assembled reports Conformant or Complete; only a validation can make that claim")
	}
}

// TestConformantIgnoresAdvisoryGapsButNotFatalOnes is the property D7 exists for.
// A rule set with only advisory gaps left has to be able to report Conformant, or
// implementing an authority's advisory tier would be a way of making the verdict
// permanently unavailable rather than of making the report better.
//
// It is stated over hand-built Reports because it is the predicate that is under
// test and not the table: these rows cover combinations no rule set in this
// package produces, including the ones RuleFamily.Unevaluable introduced, and a
// truth table is the only way to pin a predicate's behaviour on inputs the corpus
// does not generate. TestTheCoreReportsConformantAndCompleteForACleanInvoice is
// the same properties asserted on a real document.
func TestConformantIgnoresAdvisoryGapsButNotFatalOnes(t *testing.T) {
	advisoryGap := RuleFamily{Rules: "UBL-CR-*", Severity: SeverityWarning, Reason: "advisory"}
	fatalGap := RuleFamily{Rules: "UBL-CR-666", Severity: SeverityFatal, Reason: "fatal"}
	// The two shapes D10 added: a family the authority flags fatal that nobody can
	// evaluate, and an advisory one likewise. Neither may cost a verdict and
	// neither may make a run incomplete, because there is no work either could
	// represent.
	fatalUnevaluable := RuleFamily{Rules: "BR-CO-05", Severity: SeverityFatal, Unevaluable: true, Reason: "bound to true()"}
	advisoryUnevaluable := RuleFamily{Rules: "BR-51", Severity: SeverityWarning, Unevaluable: true, Reason: "a length test a masked PAN trips"}
	warning := Violation{Source: SourceEN16931, Rule: "UBL-CR-001", Severity: SeverityWarning, Message: "advisory"}
	fatal := Violation{Source: SourceEN16931, Rule: "BR-01", Severity: SeverityFatal, Message: "fatal"}
	limit := Violation{Source: SourceChecker, Rule: RuleLimit, Severity: SeverityFatal, Message: "stopped"}

	for _, tc := range []struct {
		name       string
		vs         []Violation
		gaps       []RuleFamily
		conformant bool
		complete   bool
	}{
		{"nothing at all", nil, nil, true, true},
		{"an advisory gap", nil, []RuleFamily{advisoryGap}, true, false},
		{"a fatal gap", nil, []RuleFamily{fatalGap}, false, false},
		{"a warning", []Violation{warning}, nil, true, true},
		{"a fatal finding", []Violation{fatal}, nil, false, true},
		{"a warning and an advisory gap", []Violation{warning}, []RuleFamily{advisoryGap}, true, false},

		// The D10 rows. A fatal-but-unevaluable family costs nothing: it is a rule
		// the authority published and cannot itself honour, so there is no work it
		// stands for and no verdict it can put in doubt. This is the row that would
		// have needed the old excuse list to express, and it is the reason Complete
		// is reachable at all.
		{"a fatal unevaluable gap", nil, []RuleFamily{fatalUnevaluable}, true, true},
		{"an advisory unevaluable gap", nil, []RuleFamily{advisoryUnevaluable}, true, true},
		{"both unevaluable gaps", nil, []RuleFamily{fatalUnevaluable, advisoryUnevaluable}, true, true},
		// Unevaluable is per family and not per table: one evaluable gap alongside
		// them still makes the run incomplete, and a fatal one still costs the
		// verdict. A predicate that short-circuited on "any unevaluable entry"
		// would pass this row wrongly, which is why it is here.
		{"an unevaluable gap and an advisory one", nil, []RuleFamily{fatalUnevaluable, advisoryGap}, true, false},
		{"an unevaluable gap and a fatal one", nil, []RuleFamily{fatalUnevaluable, fatalGap}, false, false},
		{"a fatal finding beside an unevaluable gap", []Violation{fatal}, []RuleFamily{fatalUnevaluable}, false, true},

		// A stopped run is not conformant however light its findings are, and
		// Conformant tests IsCheckerViolation rather than the severity so that
		// this stays true if the severity is ever reclassified.
		{"a stopped run", []Violation{limit}, nil, false, false},
		{"a stopped run with an advisory severity", []Violation{{Source: SourceChecker, Rule: RuleLimit, Severity: SeverityWarning, Message: "stopped"}}, nil, false, false},
		// A stopped run is incomplete whatever the table says. Unevaluable answers
		// "was there work left undone"; a stopped run means the work that was
		// attempted did not finish, which is the other half of Complete and is not
		// reachable through NotEvaluated at all.
		{"a stopped run with only unevaluable gaps", []Violation{limit}, []RuleFamily{fatalUnevaluable}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReport(tc.vs)
			r.NotEvaluated = tc.gaps
			if got := r.Conformant(); got != tc.conformant {
				t.Errorf("Conformant() = %v, want %v", got, tc.conformant)
			}
			if got := r.Complete(); got != tc.complete {
				t.Errorf("Complete() = %v, want %v", got, tc.complete)
			}
		})
	}
}

// TestZeroReportIsNotComplete is TestZeroReportIsNotConformant's other half,
// separated out because Complete stopped being unreachable.
//
// While no rule set could report Complete, the zero Report's Complete() == false
// was true for two independent reasons and nobody had to know which: the ran
// guard, and the fact that no combination of inputs made it true anyway. Now that
// the EN 16931 core reaches Complete, the ran guard is the only thing left, and
// Complete is the *stronger* of the two claims a Report makes — so a Report nobody
// filled in reading as Complete would be worse than one reading as Conformant.
// This asserts it over every route into a zero Report, and it asserts the positive
// direction too, because a guard that also blocks the real thing is not a guard.
func TestZeroReportIsNotComplete(t *testing.T) {
	var zero Report
	if zero.Complete() {
		t.Error("the zero Report is Complete")
	}
	if (Report{}).Complete() {
		t.Error("a freshly composed empty Report is Complete")
	}
	// The shape a caller who decoded a Report from JSON gets: fields populated,
	// ran absent because it is unexported.
	decoded := Report{Violations: []Violation{}, NotEvaluated: []RuleFamily{}}
	if decoded.Complete() {
		t.Error("a Report assembled outside this package is Complete; only a validation can make that claim")
	}
	// And the real thing, which must be Complete, or the guard above is
	// indistinguishable from Complete being unreachable — which is exactly the
	// state this test was written to stop being confused with.
	real := mustReport(t, context.Background(), ValidateEN16931, []byte(validCII))
	if !real.Complete() {
		t.Fatalf("a clean EN 16931 document is not Complete, so this test proves nothing about the ran guard: %v",
			real.NotEvaluated)
	}
	if copied := real; !copied.Complete() {
		t.Error("copying a Complete Report lost the claim")
	}
}

// sourcesWithUnevaluableFamilies is where a Source has to be named before its
// coverage table may carry an Unevaluable family.
//
// It is a list rather than a rule because the claim "no validator can evaluate
// this" is a strong one about somebody else's artefact, and the field it sets
// makes Report.Complete true — so it is the one field in RuleFamily that pays for
// itself by being wrong. A Source arriving here should be a sentence in a commit
// message, not a side effect of adding a coverage entry.
//
// CEN was the only entry for as long as CEN's was the only rule set this
// repository vendored, and the comment here said PRs adding CIUS coverage should
// expect to leave it alone. That was the right instruction under the wrong reason:
// the bar is evidence, not authorship. With the five national Schematrons
// vendored, UBL.BE clears it — GLOBALUBL.BE.sch binds ubl-BE-13 to
// abs($TaxAmount) >= 0 over a variable that falls back to -1, which is a tautology
// in the same sense as CEN's true(), and TestUBLBE13IsBoundToATautology reads that
// binding back out of the file. The instruction stands for everything else: a rule
// an authority publishes and this package has not implemented is unimplemented,
// whatever the reason.
var sourcesWithUnevaluableFamilies = map[Source]bool{
	SourceEN16931: true, SourceUBLBE: true,
	// SRBDT joined with fifteen identifiers and one cause, read out of
	// EN16931-UBL-srbdt.sch rather than argued: that file is a single pattern in
	// which eleven rules repeat the context `/ubl:Invoice | /cn:CreditNote` and four
	// more repeat three other contexts, and ISO Schematron gives a node to the first
	// matching rule of a pattern only. Eight of the fifteen were being reported by
	// this package before it was read, so recording them is a false-positive fix.
	// TestSRBDTUnevaluableRulesAreDerivedFromTheArtefact re-derives all fifteen.
	SourceSRBDT: true,
	// NLCIUS joined with four, and it is the first Source whose unevaluable claim is
	// per *binding*: BR-NL-9 and BR-NL-31 are reachable in the UBL binding and not in
	// the CII one, and BR-NL-32-2/-3 are published only in the UBL binding and are
	// unreachable there. Same cause, same evidence, read by
	// TestNLCIUSUnevaluableRulesAreDerivedFromTheArtefacts.
	SourceNLCIUS: true,
	// CIUS-RO joined with six identifiers and two distinct reasons, both read out
	// of cius-ro/RO16931-rules.sch rather than argued: three rules whose context is
	// claimed by an earlier rule of the same pattern, which under ISO Schematron
	// means no processor ever reaches them (CEN's CII-DT-010/011/012 are
	// unevaluable for exactly this reason), and two bound to `count(.) <= 50`,
	// which counts the context node and cannot be false. Neither is "hard" or "not
	// yet": TestCIUSROUnevaluableAssertsAreDerivedFromTheArtefact re-derives all
	// six from the file, in both directions, so a fixed upstream turns the entry
	// back into a gap on the day it is fetched.
	SourceCIUSRO: true,
	// Factur-X joined with one identifier, and it is the only entry here whose
	// cause is neither rule ordering nor a true() binding: FNFE rewrote CEN's
	// CII-SR-464 into (A or B) or (not(A) and not(B)), which is a tautology over
	// any A and B and therefore an assertion no processor can report from these
	// files. TestFacturXInertBindingIsStillInert re-derives the XPath from all
	// four profile Schematrons that carry it, so the entry turns back into a gap
	// the day FNFE fixes it.
	SourceFacturX: true,
}

// sourcesWithVendoredRuleArtefacts are the authorities whose published rule set
// this repository actually holds, so that a claim about the artefact can be
// checked against it. It is the same list TestCoverageSeveritiesMatchThePublishedFlag
// reads the flags from.
var sourcesWithVendoredRuleArtefacts = map[Source]bool{
	SourceEN16931: true, SourceXRechnung: true, SourcePeppol: true,
	// Factur-X, from `make facturx-schematron`: the five profile Schematrons
	// FNFE-MPE publishes inside the specification bundle, fetched from
	// ZUGFeRD/mustangproject, which vendors the same files identifier for
	// identifier.
	SourceFacturX: true,
	// The five `make cius-schematron` added. Before it, this map was the reason
	// these Sources could not carry an unevaluable family however plain the case
	// (C35); now the claim is checkable for them like any other.
	SourceCIUSPT: true, SourceCIUSRO: true, SourceUBLBE: true,
	SourceSRBDT: true, SourceNLCIUS: true,
}

// TestOnlyEN16931HasUnevaluableFamilies is the guard against RuleFamily.Unevaluable
// becoming a dumping ground, which is the one way this change could make the
// package less honest than it was.
//
// Unevaluable makes Report.Complete true and makes a fatal gap free. If it widens
// to mean "hard", "low value" or "not yet", Complete becomes exactly the lie the
// old Complete *field* was — a package claiming it saw everything when it did not.
// Two properties keep the boundary where D10 put it:
//
//   - a Source must be named in sourcesWithUnevaluableFamilies, so the first
//     entry under a new authority is a deliberate act; and
//   - that Source's authority must publish a rule artefact this repository
//     vendors. This is the hard half. "No validator can evaluate this rule" is a
//     claim about a published artefact, and it cannot be checked — by a reviewer or
//     by this suite — for an authority whose artefact is not here. It is why the
//     four CIUS with no vendored Schematron (CIUS-PT, CIUS-RO, UBL.BE, SRBDT) can
//     never carry the field however awkward their rules turn out to be: those are
//     unimplemented, and the table already has a way to say so.
func TestOnlyEN16931HasUnevaluableFamilies(t *testing.T) {
	for _, src := range allSources {
		var unevaluable []string
		for _, f := range Coverage(src) {
			if f.Unevaluable {
				unevaluable = append(unevaluable, f.Rules)
			}
		}
		if len(unevaluable) == 0 {
			continue
		}
		if !sourcesWithUnevaluableFamilies[src] {
			t.Errorf("Coverage(%q) marks %v unevaluable, but %q is not in sourcesWithUnevaluableFamilies. "+
				"Unevaluable means the authority published a rule no validator can evaluate — not that this "+
				"package has not implemented it. If it is really the former, add the Source and say why in the "+
				"commit; if it is the latter, drop the field and leave the gap where it belongs", src, unevaluable, src)
		}
		if !sourcesWithVendoredRuleArtefacts[src] {
			t.Errorf("Coverage(%q) marks %v unevaluable, but this repository vendors no rule artefact for %q, so "+
				"neither a reviewer nor this suite can check the claim. An unevaluable family has to point at a "+
				"file that is here", src, unevaluable, src)
		}
	}
	// The last of the four CIUS this loop used to name. It named UBL.BE and CIUS-RO
	// too, on the ground that this repository vendored no Schematron for any of them
	// and the claim was therefore uncheckable — which stopped being true when
	// `make cius-schematron` landed — and it named SRBDT until the rule order of
	// EN16931-UBL-srbdt.sch was read. Every Source that left this list did so with a
	// test that re-derives its claim from the artefact, named beside it in
	// sourcesWithUnevaluableFamilies.
	//
	// CIUS-PT still has none, and its rules being among the hardest in the package
	// to implement was never a reason to acquire one. "Hard" is the thing
	// Unevaluable does not mean, and the whole of AT/eSPap's inventory is evaluated
	// anyway.
	for _, src := range []Source{SourceCIUSPT} {
		for _, f := range Coverage(src) {
			if f.Unevaluable {
				t.Errorf("Coverage(%q) marks %q unevaluable. This CIUS is unimplemented where it is incomplete, "+
					"not unevaluable: its authority publishes rules that a validator can check", src, f.Rules)
			}
		}
	}
}

// TestUnevaluableFamiliesNameTheirEvidence checks the Reason of every unevaluable
// family against the artefact it claims, rather than against a house style.
//
// An unevaluable family is a claim about somebody else's published file, and a
// claim a reviewer cannot check is not much better than no claim. So each Reason
// has to name a vendored file that exists, and where the Reason says a rule is
// bound to the XPath expression true(), this test reads that binding out of the
// Schematron and asserts it really is. That last part is what makes this a test
// rather than a lint: it is the difference between "the prose mentions true()" and
// "CEN binds BR-CO-05 to true()".
//
// The other two claims are checked elsewhere by construction, and the Reason names
// where: CII-DT-010/011/012's unreachability is
// TestAdvisoryRulesCENCannotReportAreNotReported, and BR-51's UBL binding is
// quoted with the length bound it uses so the arithmetic can be followed.
func TestUnevaluableFamiliesNameTheirEvidence(t *testing.T) {
	dir := en16931SuiteDir()
	if dir == "" {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	// Every .sch or .xslt filename mentioned in a Reason.
	fileRE := regexp.MustCompile(`[A-Za-z0-9_./-]+\.(?:sch|xslt)`)
	// A param binding a rule to a literal true(), as the two binding files write it.
	paramRE := func(id string) *regexp.Regexp {
		return regexp.MustCompile(`<param\s+name="` + regexp.QuoteMeta(id) + `"\s+value="true\(\)"\s*/>`)
	}

	vendored := map[string]bool{}
	for _, pat := range []string{
		filepath.Join(dir, "ubl", "schematron", "*", "*.sch"),
		filepath.Join(dir, "cii", "schematron", "*", "*.sch"),
		filepath.Join(dir, "ubl", "xslt", "*.xslt"),
		filepath.Join(dir, "cii", "xslt", "*.xslt"),
		// The five national rule sets. An unevaluable family under a national
		// Source has to name one of these, for the same reason CEN's has to name
		// one of CEN's: the claim is about a file, and the file has to be here.
		filepath.Join("testdata", "cius-be", "schematron", "*", "*.sch"),
		filepath.Join("testdata", "cius-pt", "schematron", "*", "*", "*.sch"),
		filepath.Join("testdata", "cius-ro", "schematron", "*", "*", "*.sch"),
		filepath.Join("testdata", "cius-rs", "schematron", "*", "*.sch"),
		filepath.Join("testdata", "nlcius", "schematron", "*", "*.sch"),
		// And Factur-X's five profile Schematrons, which are the artefact behind
		// the one unevaluable family under SourceFacturX.
		filepath.Join("testdata", "facturx", "schematron", "*.sch"),
	} {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			vendored[filepath.Base(f)] = true
		}
	}
	if len(vendored) < 8 {
		t.Fatalf("found only %d vendored rule artefacts; the harness is reading the wrong directory", len(vendored))
	}

	checkedTrue, checkedFiles := 0, 0
	for _, src := range allSources {
		for _, f := range Coverage(src) {
			if !f.Unevaluable {
				continue
			}
			named := fileRE.FindAllString(f.Reason, -1)
			if len(named) == 0 {
				t.Errorf("Coverage(%q) marks %q unevaluable without naming the artefact that makes it so; a "+
					"reviewer has to be able to check the claim: %q", src, f.Rules, f.Reason)
			}
			for _, n := range named {
				checkedFiles++
				if !vendored[filepath.Base(n)] {
					t.Errorf("Coverage(%q) entry %q names the artefact %q, which this repository does not vendor",
						src, f.Rules, n)
				}
			}
			if !strings.Contains(f.Reason, "true()") {
				continue
			}
			// The claim is mechanical, so check it mechanically: read the binding
			// out of both syntaxes and assert every identifier the entry names is
			// bound to a literal true().
			for _, id := range coverageIdentifiers(f.Rules) {
				if !strings.HasPrefix(id, "BR-") {
					continue
				}
				for _, bind := range []string{
					filepath.Join(dir, "ubl", "schematron", "UBL", "EN16931-UBL-model.sch"),
					filepath.Join(dir, "cii", "schematron", "CII", "EN16931-CII-model.sch"),
				} {
					data, err := os.ReadFile(bind)
					if err != nil {
						t.Fatalf("%s: %v", bind, err)
					}
					if !paramRE(id).Match(data) {
						t.Errorf("Coverage(%q) entry %q says CEN binds these to true(), but %s does not bind %s to "+
							"true(). Either the Reason is wrong or CEN changed the binding, and in the second case "+
							"the family may no longer be unevaluable", src, f.Rules, filepath.Base(bind), id)
						continue
					}
					checkedTrue++
				}
			}
		}
	}
	if checkedFiles == 0 {
		t.Error("no unevaluable family named an artefact; this test verified nothing")
	}
	if checkedTrue != 8 {
		t.Errorf("verified %d true() bindings against the Schematron, want 8 (BR-CO-05..08 in each of the two "+
			"syntax bindings); a change in that number means the set of unenforceable model rules moved", checkedTrue)
	}
	t.Logf("checked %d artefact references and %d true() bindings behind the unevaluable families", checkedFiles, checkedTrue)
}

// TestCoverageDoesNotHandBackTheTable pins the defensive copy Coverage documents.
//
// The table is package state read by every validator, so a caller that sorted,
// truncated or edited the returned slice in place would change what every later
// Report says — and would do it silently, since Coverage cannot fail. The property
// held before this test existed; it is written down now because RuleFamily grew a
// field, and a struct gaining fields is when "the copy is deep enough" stops being
// obvious. It is deep as long as every field is a value type, which the assertion
// on the mutated field is what checks.
func TestCoverageDoesNotHandBackTheTable(t *testing.T) {
	got := Coverage(SourceEN16931)
	if len(got) == 0 {
		t.Fatal("Coverage(SourceEN16931) is empty, so this test checks nothing")
	}
	if !got[0].Unevaluable {
		t.Fatalf("the first EN 16931 entry is not unevaluable, so the mutation below would not be visible: %+v", got[0])
	}
	got[0].Rules = "MUTATED"
	got[0].Severity = SeverityWarning
	got[0].Unevaluable = false
	got[0].Reason = "MUTATED"
	got = append(got, RuleFamily{Rules: "APPENDED"})

	again := Coverage(SourceEN16931)
	if again[0].Rules == "MUTATED" || again[0].Reason == "MUTATED" || !again[0].Unevaluable {
		t.Errorf("Coverage handed back an alias of the table; a caller has rewritten this package's coverage claim: %+v", again[0])
	}
	if len(again) == len(got) {
		t.Error("appending to the slice Coverage returned changed the table")
	}
	// And through a Report, which builds NotEvaluated from the same table.
	r := mustReport(t, context.Background(), ValidateEN16931, []byte(validCII))
	r.NotEvaluated[0].Unevaluable = false
	if !Coverage(SourceEN16931)[0].Unevaluable {
		t.Error("editing Report.NotEvaluated changed the coverage table")
	}
	if !mustReport(t, context.Background(), ValidateEN16931, []byte(validCII)).Complete() {
		t.Error("editing one Report's NotEvaluated changed what a later validation claims")
	}
}

// TestFatalAndWarningsPartitionTheFindings keeps the two accessors total. A
// caller that handles both has handled every finding; if a third severity ever
// arrives, this fails rather than letting findings fall out of both slices.
func TestFatalAndWarningsPartitionTheFindings(t *testing.T) {
	r := newReport([]Violation{
		{Source: SourceEN16931, Rule: "BR-01", Severity: SeverityFatal, Message: "a"},
		{Source: SourceEN16931, Rule: "UBL-CR-001", Severity: SeverityWarning, Message: "b"},
		{Source: SourceEN16931, Rule: "BR-02", Severity: SeverityFatal, Message: "c"},
	}, SourceEN16931)
	if got := len(r.Fatal()) + len(r.Warnings()); got != len(r.Violations) {
		t.Errorf("Fatal (%d) + Warnings (%d) = %d, want all %d findings",
			len(r.Fatal()), len(r.Warnings()), got, len(r.Violations))
	}
	// Neither may alias Violations: a caller sorting one would reorder the other.
	if f := r.Fatal(); len(f) > 0 {
		f[0] = Violation{}
		if r.Violations[0].Rule != "BR-01" {
			t.Error("Fatal aliases Violations; a caller can rewrite the Report it came from")
		}
	}
}

// coverageText flattens one Source's coverage entries into one searchable
// string, for the tests that ask whether the table still mentions a rule at all.
// It joins both fields: a rule named only in a Reason is still named.
func coverageText(src Source) string {
	var b strings.Builder
	for _, f := range Coverage(src) {
		b.WriteString(f.Rules)
		b.WriteString(" — ")
		b.WriteString(f.Reason)
		b.WriteString("\n")
	}
	return b.String()
}

// validatorsWithNoFatalGap are the validators whose rule sets have no fatal
// coverage gap left, so Conformant for them is decided by the findings alone.
//
// This set was empty until the two fatal UBL-CR-* rules were implemented (C27).
// They were the last fatal gap in the EN 16931 core, and closing them changed the
// answer to the question this package exists to ask: a clean UBL or CII invoice
// validated against the core now reports Conformant() == true, where before it
// reported false with "UBL-CR-666, UBL-CR-673" as the reason. Every validator
// here reaches that state through the core:
//
//   - Validate runs the core and Factur-X's own rule set, neither of which has a
//     fatal evaluable gap left;
//   - ValidateNLCIUS runs the core and NLCIUS, whose own rule set has no gap left
//     at all: what Coverage(SourceNLCIUS) still names is four assertions no
//     processor reaches;
//   - ValidateCIUS on a document that declares no recognised CIUS routes to the
//     core alone, which is the fixture below.
//
// Every other validator in allValidators still names a fatal gap, and the test
// asserts that too. A list is the point rather than a relaxation: "every validator
// names a fatal gap" was the property before, and it held for the CIUS and
// national rule sets because their fatal halves really are partial, but it held
// for the EN 16931 core only because of a defect. Naming the exceptions keeps both
// directions guarded — a rule set that quietly stops naming a fatal gap fails
// here, and so does one that acquires a new fatal gap after finishing its fatal
// half.
// ValidateXRechnung joined the list when the 21 Peppol rules KoSIT's released
// Schematron merges in were evaluated (C30). It was the one validator here whose
// only fatal gap was a rule set it imported rather than one of its own, and closing
// it changed the answer for the whole German public-sector path: a clean XRechnung
// invoice now reports Conformant() == true, where before it reported false with
// twenty-one PEPPOL-EN16931-* identifiers as the reason.
// ValidatePeppol joined it when the 101 country-specific rules in the same two
// OpenPEPPOL binding files were evaluated (C33). Its only fatal gap had been that
// family, so a clean Peppol invoice now reports Conformant() == true where before
// it reported false for every document — the trap D10 describes, one rule set
// later.
// ValidateCIUSPT joined it when the 290 DT-CIUS-PT-* datatype and arithmetic rules
// were generated from AT/eSPap's Schematron. That family was four fifths of the
// Portuguese rule set by count and its only fatal gap, so a clean Portuguese
// invoice now reports Conformant() == true and Complete() == true where before it
// reported false for every document. It is the first *CIUS* whose datatype tier is
// implemented at all.
// ValidateCIUSRO joined it when the four Romanian families ANAF publishes beside
// its business rules — 64 BR-RO-L* length limits, 21 BR-DEC-RO-* decimal limits, 7
// BR-RO-DT* date formats and 4 BR-RO-A* occurrence limits — were generated from
// RO16931-rules.sch. The six of those ninety-six that no Schematron processor can
// report are still in Coverage(SourceCIUSRO), carrying Unevaluable, which is what
// makes them free: they are a defect in ANAF's artefact rather than a gap here.
// ValidateUBLBE joined it when ubl-BE-01, -04, -06 and -12 were implemented — the
// two rules on the AdditionalDocumentReference group that no file comment had ever
// mentioned, and the two bilingual free-text code lists that had been called "not
// enforced". Its only remaining entry is ubl-BE-13, which the authority binds to a
// tautology.
// ValidateSRBDT joined it when the Ministry of Finance's whole inventory was
// accounted for: the 21 RSR rules a Schematron processor reaches, the 3 RSE
// extension rules and the 7 assertions of the abstract pdvcat pattern, plus the 15
// that no processor reaches. It is the validator this list's counterpart test used
// to be demonstrated on, which is why that test moved for the third time.
// Validate joined it when the 24 fatal BR-FXEXT-* restatements were evaluated, and
// its route into the list is the longest here because it left twice. It was on
// this list, as "Validate with ProfileEN16931 runs the core and nothing else",
// until a Profile began selecting Factur-X's binding — which is what issue #56
// asked for and which correctly made Conformant false for every Validate call,
// because Factur-X's own rule set was then almost entirely unevaluated. It came
// back in two steps: the 2,159 assertions of the per-profile data model, and then
// these 24. A clean Factur-X document reports Conformant() == true again at every
// tier, and the entry is now a claim about Factur-X's rule set rather than about
// the core reached through it.
var validatorsWithNoFatalGap = map[string]bool{
	"Validate":          true,
	"ValidateEN16931":   true,
	"ValidateNLCIUS":    true,
	"ValidateCIUS":      true,
	"ValidateXRechnung": true,
	"ValidatePeppol":    true,
	"ValidateCIUSPT":    true,
	"ValidateCIUSRO":    true,
	"ValidateUBLBE":     true,
	"ValidateSRBDT":     true,
}

// TestValidatorsWithAFatalGapAreTheOnesWeThinkTheyAre is where the fatal half of
// each rule set is recorded, because it is what decides Conformant.
//
// Conformant is false for a clean document only when the rule set that ran names
// a gap its authority flags fatal *and a validator could have evaluated*. The
// second half of that is the D10 clause, and it is why the loop below counts
// fatal-and-evaluable rather than fatal: the EN 16931 core names three families
// CEN flags fatal — BR-CO-05..08 and CII-DT-010/011/012 — and CEN's own reference
// implementation cannot report any of them. Before D10 that was expressed by
// recording them at SeverityWarning, contradicting the published flag; the flag is
// quoted honestly now and the exception lives in the predicate, where it can be
// read.
//
// Both directions fail here, and both are moments that deserve to be deliberate:
// a rule set finishing its fatal half is the moment Conformant starts returning
// true for documents it validated, and a rule set that had finished acquiring a
// new fatal gap is a regression of exactly that claim.
func TestValidatorsWithAFatalGapAreTheOnesWeThinkTheyAre(t *testing.T) {
	ctx := context.Background()
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			var fatal []string
			for _, g := range mustReport(t, ctx, fn, []byte(unknownRoot)).NotEvaluated {
				if g.Severity == SeverityFatal && !g.Unevaluable {
					fatal = append(fatal, g.Rules)
				}
			}
			switch {
			case len(fatal) == 0 && !validatorsWithNoFatalGap[name]:
				t.Errorf("%s names no fatal coverage gap it could close, so Conformant now depends only on the findings. "+
					"If a rule set's fatal half is finished, add it to validatorsWithNoFatalGap and say so in the commit", name)
			case len(fatal) > 0 && validatorsWithNoFatalGap[name]:
				t.Errorf("%s is listed as having no fatal coverage gap, but names %v. A rule set whose fatal half was "+
					"finished has acquired a new fatal gap, which un-does the Conformant claim made when it was added to that list", name, fatal)
			}
		})
	}
}

// TestTheCoreReportsConformantAndCompleteForACleanInvoice is the other side of
// that list, asserted on a document rather than on the table: what a caller
// actually gets.
//
// It is the claim the coverage machinery was built to make and could not make
// until C27 was closed, and it is asserted in both syntaxes because the fatal
// gap that used to block it was in the UBL binding — a CII invoice was held
// non-conformant by two rules that could never have applied to it, which is
// itself a reason the gap was worth closing rather than describing.
//
// The Complete assertion is inverted from what it was, and that inversion is the
// point of D10 rather than a concession to it. It read "the core must not be
// Complete, because the advisory binding rules are unevaluated"; the advisory
// binding rules are evaluated now, and what is left in Coverage(SourceEN16931) is
// three families CEN made unevaluable. Leaving the old assertion in place would
// mean asserting that the strongest claim this package can make must stay
// unreachable. This is the first and so far only rule set for which Complete is
// true, so the assertion is also the guard on that fact: it fails if a family a
// validator *could* evaluate is ever added to the core's coverage without being
// implemented.
func TestTheCoreReportsConformantAndCompleteForACleanInvoice(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, doc string }{
		{"UBL", minimalUBL},
		{"CII", validCII},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := mustReport(t, ctx, ValidateEN16931, []byte(tc.doc))
			if len(r.Violations) != 0 {
				t.Fatalf("the fixture is not clean: %v", r.Violations)
			}
			if !r.Conformant() {
				var fatal []string
				for _, g := range r.NotEvaluated {
					if g.Severity == SeverityFatal && !g.Unevaluable {
						fatal = append(fatal, g.Rules)
					}
				}
				t.Errorf("a clean %s invoice is not Conformant against the EN 16931 core; fatal gaps it could close: %v", tc.name, fatal)
			}
			if !r.Complete() {
				var evaluable []string
				for _, g := range r.NotEvaluated {
					if !g.Unevaluable {
						evaluable = append(evaluable, g.Rules)
					}
				}
				t.Errorf("a clean %s invoice is not Complete against the EN 16931 core; the gaps it names that a "+
					"validator could evaluate: %v", tc.name, evaluable)
			}
			// The coverage it does still name has to be there, or this test would
			// pass just as well against an empty table — which would make Complete
			// true for the wrong reason.
			if len(r.NotEvaluated) == 0 {
				t.Error("the core named no coverage gaps at all; Complete is then true because the table is empty " +
					"rather than because everything unevaluated is unevaluable")
			}
		})
	}
}
