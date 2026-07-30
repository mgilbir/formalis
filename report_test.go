package formalis

import (
	"context"
	"fmt"
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
	SourceEN16931, SourceXRechnung, SourcePeppol, SourceNLCIUS, SourceCIUSPT,
	SourceCIUSRO, SourceUBLBE, SourceSRBDT, SourceFatturaPA, SourceFacturae,
	SourceEbInterface, SourceKSeF, SourceFinvoice, SourceTEAPPS, SourceOIOUBL,
	SourceSvefaktura, SourceZATCA, SourceOSA, SourceUBLTR, SourcePINT,
	SourceOrderX, SourceChecker,
}

// completeSources are the Sources whose rule sets this package does not claim
// to have gaps in. SourceChecker is the only one, and only because it publishes
// no rules at all: RuleLimit, RuleProfile and RuleRoot are this package's
// statements about its own run and about the file it was handed.
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
// test: a document that produces no findings at all from ValidateCIUSPT is not
// a document that passed CIUS-PT, because BR-CIUS-PT-13/15/17/18 and 24..63
// were never run. Before Report there was nothing a caller could read that said
// so.
func TestConformantIsFalseForACleanDocumentUnderAPartialRuleSet(t *testing.T) {
	r := mustReport(t, context.Background(), ValidateCIUSPT, []byte(minimalCIUSPTUBL))
	if len(ptRuleViolations(r.Violations)) != 0 {
		t.Fatalf("the fixture is no longer clean under the CIUS-PT rules, so this test proves nothing: %v", r.Violations)
	}
	if r.Complete() {
		t.Error("ValidateCIUSPT reported Complete; it implements twelve of the CIUS-PT rules")
	}
	if r.Conformant() {
		t.Error("ValidateCIUSPT reported Conformant on a document whose Portuguese VAT-category rules were never checked")
	}
	// The caller has to be able to find out *which* rules, not merely that some
	// exist, or the report is no more actionable than the file comment was.
	var found bool
	for _, g := range r.NotEvaluated {
		if strings.Contains(g.Rules, "BR-CIUS-PT-13") {
			found = true
			if g.Severity != SeverityFatal {
				t.Errorf("the family the integrator would be rejected on is reported as %s: %+v", g.Severity, g)
			}
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
// It is a strange test to write and it is the honest one. Complete returns false
// for every document this package can be handed, because every rule set it
// implements is a subset — including the EN 16931 core, which looked complete
// only because the CEN unit-test oracle has error fragments for 198 rules and
// says nothing about the rest. When someone finishes a rule set this test fails
// and asks them to move the Source to completeSources, at which point Complete
// starts returning true for documents validated by it, which is a claim that
// deserves to be made on purpose.
//
// It is about Complete and not Conformant, and the two have parted company: the
// EN 16931 core's *fatal* half is finished, so its clean documents are already
// Conformant, while its 1,168 advisory binding rules keep it out of
// completeSources. TestValidatorsWithAFatalGapAreTheOnesWeThinkTheyAre is the
// same kind of record for the fatal half.
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
	first[0] = RuleFamily{Rules: "clobbered"}
	second := Coverage(SourceCIUSPT)
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
// claim has to follow the document: an XRechnung invoice must come back with
// the XRechnung gaps and not, say, the CIUS-PT ones, and a document declaring
// no recognised CIUS must come back with the core's gaps alone.
func TestValidateCIUSReportsTheCoverageOfTheRuleSetItRan(t *testing.T) {
	ctx := context.Background()

	xr := mustReport(t, ctx, ValidateCIUS, []byte(minimalXRechnungUBL))
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
	core := mustReport(t, ctx, ValidateCIUS, []byte(minimalUBL))
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
			r := mustReport(t, ctx, fn, []byte(unknownRoot))
			if len(r.NotEvaluated) == 0 {
				t.Fatalf("%s reported no coverage gaps at all", name)
			}
			if r.Complete() {
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
	// An <assert> or <report> element with its attribute list. The flag and the
	// id are attributes of the same element, so they have to be read together:
	// scanning for id="..." across a whole file, as the test above does, cannot
	// tell which flag belongs to which rule.
	elemRE := regexp.MustCompile(`(?s)<(?:sch:)?(?:assert|report)\s([^>]*)>`)
	idRE := regexp.MustCompile(`\bid="([^"]+)"`)
	flagRE := regexp.MustCompile(`\bflag="([^"]+)"`)

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
			for _, m := range elemRE.FindAllStringSubmatch(string(data), -1) {
				id := idRE.FindStringSubmatch(m[1])
				if id == nil {
					continue
				}
				flag := "none"
				if fl := flagRE.FindStringSubmatch(m[1]); fl != nil {
					flag = fl[1]
				}
				if flags[id[1]] == nil {
					flags[id[1]] = map[string]bool{}
				}
				flags[id[1]][flag] = true
			}
		}
	}
	if len(flags) < 1500 {
		t.Fatalf("read flags for only %d rules from the vendored Schematron; the harness is not reading the artefacts", len(flags))
	}
	return flags
}

// severityOfFlag folds an authority's flag onto this package's two values. CEN
// and OpenPEPPOL write fatal and warning; KoSIT adds information, which is
// advisory under another name.
func severityOfFlag(flag string) (Severity, bool) {
	switch flag {
	case "fatal":
		return SeverityFatal, true
	case "warning", "information":
		return SeverityWarning, true
	}
	return SeverityFatal, false
}

// TestEveryEmittedFindingIsFatalToday verifies the claim that lets Severity have
// a zero value at all: every rule this package implements is one its authority
// rejects a document for, so the fail-safe default is also the correct answer
// everywhere, and no emission site is relying on the default to mean something it
// did not decide.
//
// It sweeps the whole corpus through every validator — the same sweep the
// identifier-collision guard uses — and fails on any finding that is not fatal.
// That makes it a ratchet in the other direction from most of this suite: when
// the advisory binding rules are implemented and start arriving as warnings, this
// test fails and asks whoever did it to say so here. That is the point. A package
// that silently began emitting advisory findings would change what
// len(r.Violations) == 0 means for every existing caller.
func TestEveryEmittedFindingIsFatalToday(t *testing.T) {
	s := corpusSweep()
	for src, rules := range s.bySeverity {
		for rule, sevs := range rules {
			for sev := range sevs {
				if sev != SeverityFatal {
					t.Errorf("%s/%s was reported as %s; every rule this package implements today is one its authority "+
						"flags fatal. If that has changed, this test is the place to record it", src, rule, sev)
				}
			}
		}
	}
	if s.files > 0 {
		atLeast(t, "severity sweep corpus", s.files, minCorpusDocuments)
	}
}

// TestEveryEmittedEN16931RuleIsFatalInCENsSchematron is the same claim checked
// against ground truth rather than against itself, for the one authority whose
// rule set this repository vendors.
//
// The test above only proves the package is self-consistent: it would pass if
// every finding were stamped fatal and half of them were advisory rules CEN
// flags warning. This one reads the flag off the assertion CEN published. It is
// what makes "this package reports what an authority makes fatal" a checked
// statement, and it is what would catch an advisory rule implemented by accident
// and stamped fatal by the zero value.
func TestEveryEmittedEN16931RuleIsFatalInCENsSchematron(t *testing.T) {
	flags := schematronFlags(t)
	emitted := corpusSweep().byRule[SourceEN16931]
	if len(emitted) < 100 {
		t.Fatalf("the sweep saw only %d EN 16931 rules; the corpus is not present, so this proves nothing", len(emitted))
	}
	var unpublished []string
	for rule := range emitted {
		got, ok := flags[rule]
		if !ok {
			// The VAT-category families are generated from one template per
			// category (BR-S-08, BR-Z-08, ...) and CEN publishes only some of
			// the cells; a rule the Schematron does not carry has no flag to
			// compare against and is reported rather than silently skipped.
			unpublished = append(unpublished, rule)
			continue
		}
		if !got["fatal"] {
			t.Errorf("this package reports EN 16931 %s as fatal, but CEN flags it %v; a rule an authority does not "+
				"reject a document for must be emitted with SeverityWarning", rule, keysOf(got))
		}
	}
	sort.Strings(unpublished)
	t.Logf("checked %d emitted EN 16931 rules against CEN's flags; %d are not in the vendored Schematron at all: %v",
		len(emitted)-len(unpublished), len(unpublished), unpublished)
}

// TestCoverageSeveritiesMatchThePublishedFlag holds the coverage table's
// severity column to the same standard as the findings: for every family whose
// identifiers this repository can look up, the severity must be the one the
// authority published.
//
// Two families deliberately do not match, and they are named here rather than
// skipped by a pattern, because they are the whole reason RuleFamily.Severity is
// documented as what the gap costs rather than as a copy of the flag. CEN flags
// BR-CO-05..08 and CII-DT-010/011/012 fatal and binds them to expressions no
// conforming validator can ever report, so not evaluating them cannot change a
// verdict. Anything else diverging is a mistake in the table.
func TestCoverageSeveritiesMatchThePublishedFlag(t *testing.T) {
	flags := schematronFlags(t)

	// The families whose recorded severity is deliberately not the published
	// flag, with the identifier that makes each recognisable.
	unenforceable := map[string]string{
		"BR-CO-05":   "CEN binds all four to true() in both syntaxes",
		"BR-CO-06":   "CEN binds all four to true() in both syntaxes",
		"BR-CO-07":   "CEN binds all four to true() in both syntaxes",
		"BR-CO-08":   "CEN binds all four to true() in both syntaxes",
		"CII-DT-010": "an earlier Schematron rule matches the node first, so no processor reaches it",
		"CII-DT-011": "an earlier Schematron rule matches the node first, so no processor reaches it",
		"CII-DT-012": "an earlier Schematron rule matches the node first, so no processor reaches it",
	}

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
				if entry.Severity == want {
					continue
				}
				if why, excused := unenforceable[id]; excused && entry.Severity == SeverityWarning {
					if !strings.Contains(entry.Reason, "true()") && !strings.Contains(entry.Reason, "first matching rule") {
						t.Errorf("%s is recorded advisory against a %s flag (%s) without the Reason saying why", id, want, why)
					}
					continue
				}
				t.Errorf("Coverage(%q) records the family %q as %s, but %s is flagged %v: %q",
					src, entry.Rules, entry.Severity, id, keysOf(got), entry.Reason)
			}
		}
	}
	if checked < 30 {
		t.Fatalf("only %d coverage identifiers could be looked up; the harness is reading the wrong artefacts", checked)
	}
	t.Logf("checked the severity of %d coverage identifiers against the flags their authorities publish", checked)
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
	numTailRE = regexp.MustCompile(`^(.*?)(\d+)(.*)$`)
	tokenSep  = regexp.MustCompile(`[\s,;()]+`)
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
		if ruleIDRE.MatchString(tok) && strings.Contains(tok, "-") {
			if m := numTailRE.FindStringSubmatch(tok); m != nil {
				prefix = m[1]
			}
			return tok
		}
		if prefix == "" {
			return ""
		}
		// A tail may repeat the letter the prefix already ends with ("R049"
		// under the prefix "PEPPOL-EN16931-R").
		if len(tok) > 1 && strings.HasSuffix(prefix, tok[:1]) {
			tok = tok[1:]
		}
		if numTailRE.FindStringSubmatch(tok) == nil {
			return ""
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
// It is stated over hand-built Reports because no rule set in this package is in
// that state yet — every one still has fatal gaps, which is what
// TestNoRuleSetIsCompleteToday records. The predicate is what is under test here,
// not the table.
func TestConformantIgnoresAdvisoryGapsButNotFatalOnes(t *testing.T) {
	advisoryGap := RuleFamily{Rules: "UBL-CR-*", Severity: SeverityWarning, Reason: "advisory"}
	fatalGap := RuleFamily{Rules: "UBL-CR-666", Severity: SeverityFatal, Reason: "fatal"}
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
		// A stopped run is not conformant however light its findings are, and
		// Conformant tests IsCheckerViolation rather than the severity so that
		// this stays true if the severity is ever reclassified.
		{"a stopped run", []Violation{limit}, nil, false, false},
		{"a stopped run with an advisory severity", []Violation{{Source: SourceChecker, Rule: RuleLimit, Severity: SeverityWarning, Message: "stopped"}}, nil, false, false},
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
//   - Validate with ProfileEN16931 runs the core and nothing else;
//   - ValidateNLCIUS runs the core and NLCIUS, and NLCIUS is the one CIUS whose
//     own gap (BR-NL-19..35, "not recommended") is advisory;
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
var validatorsWithNoFatalGap = map[string]bool{
	"Validate":       true,
	"ValidateNLCIUS": true,
	"ValidateCIUS":   true,
}

// TestValidatorsWithAFatalGapAreTheOnesWeThinkTheyAre is where the fatal half of
// each rule set is recorded, because it is what decides Conformant.
//
// Complete being false everywhere follows from the table being non-empty, which
// TestNoRuleSetIsCompleteToday records. Conformant needs more than that, because
// it passes over advisory gaps: it is false for a clean document only when the
// rule set that ran names a gap its authority flags fatal.
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
				if g.Severity == SeverityFatal {
					fatal = append(fatal, g.Rules)
				}
			}
			switch {
			case len(fatal) == 0 && !validatorsWithNoFatalGap[name]:
				t.Errorf("%s names no fatal coverage gap, so Conformant now depends only on the findings. "+
					"If a rule set's fatal half is finished, add it to validatorsWithNoFatalGap and say so in the commit", name)
			case len(fatal) > 0 && validatorsWithNoFatalGap[name]:
				t.Errorf("%s is listed as having no fatal coverage gap, but names %v. A rule set whose fatal half was "+
					"finished has acquired a new fatal gap, which un-does the Conformant claim made when it was added to that list", name, fatal)
			}
		})
	}
}

// TestTheCoreReportsConformantForACleanInvoice is the other side of that list,
// asserted on a document rather than on the table: what a caller actually gets.
//
// It is the claim the coverage machinery was built to make and could not make
// until C27 was closed, and it is asserted in both syntaxes because the fatal
// gap that used to block it was in the UBL binding — a CII invoice was held
// non-conformant by two rules that could never have applied to it, which is
// itself a reason the gap was worth closing rather than describing.
func TestTheCoreReportsConformantForACleanInvoice(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, doc string }{
		{"UBL", minimalUBL},
		{"CII", validCII},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := mustReport(t, ctx, withProfile(ProfileEN16931), []byte(tc.doc))
			if len(r.Violations) != 0 {
				t.Fatalf("the fixture is not clean: %v", r.Violations)
			}
			if !r.Conformant() {
				var fatal []string
				for _, g := range r.NotEvaluated {
					if g.Severity == SeverityFatal {
						fatal = append(fatal, g.Rules)
					}
				}
				t.Errorf("a clean %s invoice is not Conformant against the EN 16931 core; fatal gaps: %v", tc.name, fatal)
			}
			// Still not Complete: the advisory binding rules are unevaluated, and
			// that is the distinction D7 put severity on the family for.
			if r.Complete() {
				t.Errorf("%s reported Complete while %d advisory rule families are unevaluated", tc.name, len(r.NotEvaluated))
			}
		})
	}
}
