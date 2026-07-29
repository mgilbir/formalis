package formalis

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
	SourceEN16931, SourceXRechnung, SourcePeppol, SourceNLCIUS, SourceCIUSPT,
	SourceCIUSRO, SourceUBLBE, SourceSRBDT, SourceFatturaPA, SourceFacturae,
	SourceEbInterface, SourceKSeF, SourceFinvoice, SourceTEAPPS, SourceOIOUBL,
	SourceSvefaktura, SourceZATCA, SourceOSA, SourceUBLTR, SourcePINT,
	SourceOrderX, SourceChecker,
}

// completeSources are the Sources whose rule sets this package does not claim
// to have gaps in. SourceChecker is the only one, and only because it publishes
// no rules at all: RuleLimit, RuleSyntax and RuleProfile are this package's
// statements about its own run.
//
// If a Source is ever moved here it means someone finished implementing an
// authority's rule set, which is exactly the change that should be hard to make
// by accident.
var completeSources = map[Source]bool{SourceChecker: true}

// TestConformantIsFalseForACancelledRun is the first of the five. It is the
// case limits.go already solved with RuleLimit; Complete has to keep solving
// it, because the whole point of the type is that one predicate answers both
// kinds of doubt.
func TestConformantIsFalseForACancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Validate(ctx, []byte(validCII), ProfileEN16931)
	if r.Complete {
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
	r := Validate(context.Background(), flatUBLBE(maxNodes+1), ProfileEN16931)
	if r.Complete {
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
	r := Validate(context.Background(), []byte(validCII), Profile("EN16931"))
	if r.Complete {
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

// TestConformantIsFalseForAMalformedDocument is the fourth. Here Conformant is
// false because of the finding, not because of doubt: "this file is not
// well-formed XML" is a definite statement about the document. The assertion is
// on Conformant rather than on Complete for exactly that reason.
func TestConformantIsFalseForAMalformedDocument(t *testing.T) {
	r := Validate(context.Background(), []byte(`<a></b>`), ProfileEN16931)
	if r.Conformant() {
		t.Error("a malformed document reported Conformant")
	}
	if len(r.Violations) != 1 || r.Violations[0].Rule != RuleSyntax {
		t.Fatalf("a malformed document reported %v, want exactly one %q finding", r.Violations, RuleSyntax)
	}
	// The coverage claim must not depend on whether the document parsed: a
	// caller comparing two Reports would otherwise read a shorter NotEvaluated
	// as better coverage when it only means the parse failed earlier.
	clean := Validate(context.Background(), []byte(validCII), ProfileEN16931)
	if !reflect.DeepEqual(r.NotEvaluated, clean.NotEvaluated) {
		t.Errorf("a malformed document reported different coverage from a readable one:\n  malformed %v\n  readable  %v",
			r.NotEvaluated, clean.NotEvaluated)
	}
}

// TestConformantIsFalseForACleanDocumentUnderAPartialRuleSet is the fifth, and
// the one this whole file was written for. It is C12's failure scenario as a
// test: a document that produces no findings at all from ValidateCIUSPT is not
// a document that passed CIUS-PT, because BR-CIUS-PT-13/15/17/18 and 24..63
// were never run. Before Report there was nothing a caller could read that said
// so.
func TestConformantIsFalseForACleanDocumentUnderAPartialRuleSet(t *testing.T) {
	r := ValidateCIUSPT(context.Background(), []byte(minimalCIUSPTUBL))
	if len(ptRuleViolations(r.Violations)) != 0 {
		t.Fatalf("the fixture is no longer clean under the CIUS-PT rules, so this test proves nothing: %v", r.Violations)
	}
	if r.Complete {
		t.Error("ValidateCIUSPT reported Complete; it implements twelve of the CIUS-PT rules")
	}
	if r.Conformant() {
		t.Error("ValidateCIUSPT reported Conformant on a document whose Portuguese VAT-category rules were never checked")
	}
	// The caller has to be able to find out *which* rules, not merely that some
	// exist, or the report is no more actionable than the file comment was.
	var found bool
	for _, g := range r.NotEvaluated {
		if strings.Contains(g, "BR-CIUS-PT-13") {
			found = true
		}
	}
	if !found {
		t.Errorf("NotEvaluated does not name the rule family the integrator would be rejected on: %v", r.NotEvaluated)
	}
}

// TestNoRuleSetIsCompleteToday records the state of the package, so that
// closing the last gap in an authority's rule set is a deliberate act rather
// than a side effect.
//
// It is a strange test to write and it is the honest one. Conformant returns
// false for every document this package can be handed, because every rule set
// it implements is a subset — including the EN 16931 core, which looked
// complete only because the CEN unit-test oracle has error fragments for 198
// rules and says nothing about the rest. When someone finishes a rule set this
// test fails and asks them to move the Source to completeSources, at which
// point Conformant starts returning true for documents validated by it, which
// is a claim that deserves to be made on purpose.
func TestNoRuleSetIsCompleteToday(t *testing.T) {
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
	first := Coverage(SourceCIUSPT)
	if len(first) == 0 {
		t.Fatal("Coverage(SourceCIUSPT) is empty, so this test proves nothing")
	}
	first[0] = "clobbered"
	second := Coverage(SourceCIUSPT)
	if second[0] == "clobbered" {
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
	r := ValidateCIUSPT(context.Background(), []byte(minimalCIUSPTUBL))
	want := append(Coverage(SourceEN16931), Coverage(SourceCIUSPT)...)
	if !reflect.DeepEqual(r.NotEvaluated, want) {
		t.Errorf("ValidateCIUSPT NotEvaluated =\n  %v\nwant the union of the core and CIUS-PT gaps:\n  %v", r.NotEvaluated, want)
	}
	seen := map[string]int{}
	for _, g := range r.NotEvaluated {
		seen[g]++
	}
	for g, n := range seen {
		if n > 1 {
			t.Errorf("NotEvaluated repeats %q %d times; the union must dedupe", g, n)
		}
	}
}

// TestValidateCIUSReportsTheCoverageOfTheRuleSetItRan is the dispatcher's half.
// ValidateCIUS chooses a rule set from the document's BT-24, so its coverage
// claim has to follow the document: an XRechnung invoice must come back with
// the XRechnung gaps and not, say, the CIUS-PT ones, and a document declaring
// no recognised CIUS must come back with the core's gaps alone.
func TestValidateCIUSReportsTheCoverageOfTheRuleSetItRan(t *testing.T) {
	ctx := context.Background()

	xr := ValidateCIUS(ctx, []byte(minimalXRechnungUBL))
	wantXR := append(Coverage(SourceEN16931), Coverage(SourceXRechnung)...)
	if !reflect.DeepEqual(xr.NotEvaluated, wantXR) {
		t.Errorf("an XRechnung document routed through ValidateCIUS reported\n  %v\nwant the XRechnung union\n  %v", xr.NotEvaluated, wantXR)
	}
	if reflect.DeepEqual(xr.NotEvaluated, Coverage(SourceEN16931)) {
		t.Error("ValidateCIUS reported only the core's gaps for a document it validated against XRechnung too")
	}

	// A document that declares no recognised CIUS is validated against the core
	// alone, so naming any CIUS's gaps would be a claim about a rule set that
	// never ran.
	core := ValidateCIUS(ctx, []byte(minimalUBL))
	if !reflect.DeepEqual(core.NotEvaluated, Coverage(SourceEN16931)) {
		t.Errorf("a document with no recognised CIUS reported\n  %v\nwant the core's gaps alone\n  %v", core.NotEvaluated, Coverage(SourceEN16931))
	}
}

// TestEveryValidatorReportsItsCoverage sweeps the exported surface. Every
// validator here runs a partial rule set, so every one of them must say so on a
// document it can read; a validator that returns an empty NotEvaluated has
// either become complete (see TestNoRuleSetIsCompleteToday) or forgotten to
// pass its Sources to newReport, and the second is silent.
func TestEveryValidatorReportsItsCoverage(t *testing.T) {
	ctx := context.Background()
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			// Any well-formed document will do: coverage is a property of the
			// rule set, not of the document, so it is reported even for one
			// this validator refuses.
			r := fn(ctx, []byte(unknownRoot))
			if len(r.NotEvaluated) == 0 {
				t.Fatalf("%s reported no coverage gaps at all", name)
			}
			if r.Complete {
				t.Errorf("%s reported Complete while naming %d unevaluated rule families", name, len(r.NotEvaluated))
			}
			if r.Conformant() {
				t.Errorf("%s reported Conformant on a rule set with gaps", name)
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
	inTable := map[string]bool{}
	for _, gaps := range notEvaluated {
		for _, g := range gaps {
			inTable[g] = true
		}
	}
	ctx := context.Background()
	for name, fn := range allValidators {
		for _, g := range fn(ctx, []byte(unknownRoot)).NotEvaluated {
			if !inTable[g] {
				t.Errorf("%s reported the unevaluated family %q, which is not in the coverage table", name, g)
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
	return strings.Contains(entry, "other than") || strings.Contains(entry, "emits only")
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
			if carveOut(entry) {
				continue
			}
			for rule := range emitted[src] {
				if regexp.MustCompile(`\b` + regexp.QuoteMeta(rule) + `\b`).MatchString(entry) {
					t.Errorf("Coverage(%q) claims %q is not evaluated, but a validator reported it: %q", src, rule, entry)
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

	for _, entry := range Coverage(SourceEN16931) {
		for _, id := range ruleIDRE.FindAllString(entry, -1) {
			if !strings.HasPrefix(id, "BR-") && !strings.HasPrefix(id, "UBL-") && !strings.HasPrefix(id, "CII-") {
				continue
			}
			if !published[id] {
				t.Errorf("Coverage(SourceEN16931) names %q, which the CEN Schematron does not define: %q", id, entry)
			}
		}
	}
}

// TestZeroReportIsNotConformant pins the value a caller gets from a variable
// nobody filled in, or from a decoded JSON object with the field missing. It
// must not read as a clean invoice.
func TestZeroReportIsNotConformant(t *testing.T) {
	var r Report
	if r.Conformant() {
		t.Error("the zero Report is Conformant; a Report nobody filled in reads as a valid invoice")
	}
}
