package formalis

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The guards that hold cius_pt_rules.go to AT/eSPap's Schematron.
//
// cius_artefacts_test.go checks three things about all five vendored national rule
// sets: which identifiers each publishes, what flag each carries, and that nothing
// published is in neither the code nor the coverage table. None of those is the
// question this file asks, which is the one C37 was about: *does the rule this
// package evaluates say what the authority's rule says?* Fifteen national rules
// were transcribed from prose and got that wrong, and the only reason it was
// findable was that somebody re-derived them from the XPath.
//
// So the table below is the XPath, quoted. Each entry is one published identifier
// with the polarity of its Schematron element, the context its rule is bound to and
// the test it asserts, all three resolved out of the abstract/concrete pair the way
// a Schematron processor resolves them. A change upstream — or a mis-read here —
// fails the build with the two expressions side by side, which is the only failure
// message that is any use for this class of defect.
//
// It is not a substitute for the behavioural fixtures and it is not meant to be:
// the table says what AT wrote, ciusPTMutations says the Go says the same thing,
// and TestCIUSPTCorpus says neither over-fires on AT's own instances. Each catches
// a different mistake.

// ptAssertion is one resolved Schematron assertion: what the processor sees after
// the abstract pattern's $names have been substituted from the concrete pattern's
// <param> values.
type ptAssertion struct {
	kind    string // "assert" (fires when test is false) or "report" (fires when true)
	flag    string
	context string
	test    string
}

// ptCollapse normalises whitespace so that a reflow upstream is not a failure and a
// changed expression is.
func ptCollapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// ptResolveArtefact reads one vendored CIUS-PT version and returns every assertion
// whose identifier the filter admits, with $names resolved.
//
// It reads with an XML decoder and never with a regular expression. That is C31's
// lesson, and it bites here specifically: several of these tests contain a literal
// '>' (BR-CIUS-PT-12's `(cbc:Percent) > 0`, BR-AA-01's two `count(...) > 0`
// comparisons), which is exactly the character class that made a 57-rule KoSIT set
// survey as 54.
//
// Resolution is a longest-name-first textual substitution of `$Name`, which is what
// the Schematron abstract-pattern mechanism specifies. Longest-first matters: the
// parameter set contains both `$VATAA` and `$VATAA_Allowance`, and substituting the
// shorter one first would corrupt the longer.
func ptResolveArtefact(t *testing.T, version string, admit *regexp.Regexp) map[string]ptAssertion {
	t.Helper()
	dir := filepath.Join("testdata", "cius-pt", "schematron", version)
	var files []string
	for _, g := range []string{"*.sch", "*/*.sch"} {
		m, _ := filepath.Glob(filepath.Join(dir, g))
		files = append(files, m...)
	}
	if len(files) == 0 {
		return nil
	}
	sort.Strings(files)

	type rawAssert struct {
		pattern, kind, id, flag, context, test string
	}
	var raw []rawAssert
	// params is patternID -> name -> value, and isA is patternID -> the abstract
	// pattern it instantiates.
	params := map[string]map[string]string{}
	isA := map[string]string{}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(string(data)))
		var pattern, context string
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			attr := func(n string) string {
				for _, a := range se.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch se.Name.Local {
			case "pattern":
				pattern = attr("id")
				if v := attr("is-a"); v != "" {
					isA[pattern] = v
				}
			case "param":
				if params[pattern] == nil {
					params[pattern] = map[string]string{}
				}
				params[pattern][attr("name")] = attr("value")
			case "rule":
				context = attr("context")
			case "assert", "report":
				raw = append(raw, rawAssert{pattern, se.Name.Local, attr("id"), attr("flag"), context, attr("test")})
			}
		}
	}

	out := map[string]ptAssertion{}
	for concrete, abstract := range isA {
		p := params[concrete]
		names := make([]string, 0, len(p))
		for n := range p {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
		resolve := func(expr string) string {
			for _, n := range names {
				// A <param name> may carry stray whitespace ("Invoice_Line "), and the
				// abstract file refers to it both ways.
				expr = strings.ReplaceAll(expr, "$"+n, p[n])
				expr = strings.ReplaceAll(expr, "$"+strings.TrimSpace(n), p[n])
			}
			return ptCollapse(expr)
		}
		for _, a := range raw {
			if a.pattern != abstract || !admit.MatchString(a.id) {
				continue
			}
			out[a.id] = ptAssertion{kind: a.kind, flag: a.flag, context: resolve(a.context), test: resolve(a.test)}
		}
	}
	return out
}

// ptOwnIdentifier admits the two families AT/eSPap mints and this package evaluates.
var ptOwnIdentifier = regexp.MustCompile(`^(?:BR-CIUS-PT-|BR-AA-)`)

// TestCIUSPTFamilyHasNoPhantom is the C34/C38 check for this rule set, read out of
// the artefact rather than believed from the numbering.
//
// BR-CIUS-PT runs 01..66 with a hole: AT/eSPap publishes no BR-CIUS-PT-31, so the
// family is 65 identifiers and not 66. A coverage entry written as "24..63" claims
// a rule nobody publishes, which is what PR 22 found and corrected; this asserts the
// hole is still there rather than leaving the correction as a comment. And BR-AA is
// 01..07 plus 10 — eight of CEN's ten numbering slots, missing -08 and -09, the two
// arithmetic ones.
func TestCIUSPTFamilyHasNoPhantom(t *testing.T) {
	pub := ptResolveArtefact(t, "2.1.1", ptOwnIdentifier)
	if pub == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	var brPT, brAA []string
	for id := range pub {
		if strings.HasPrefix(id, "BR-AA-") {
			brAA = append(brAA, id)
		} else {
			brPT = append(brPT, id)
		}
	}
	sort.Strings(brPT)
	sort.Strings(brAA)
	if len(brPT) != 65 {
		t.Errorf("the BR-CIUS-PT family decodes to %d identifiers, want 65: %v", len(brPT), brPT)
	}
	if _, ok := pub["BR-CIUS-PT-31"]; ok {
		t.Error("AT/eSPap now publishes BR-CIUS-PT-31; it did not, and the coverage table and the " +
			"01..66-minus-31 reading of this family both assume it does not")
	}
	if want := "BR-AA-01 BR-AA-02 BR-AA-03 BR-AA-04 BR-AA-05 BR-AA-06 BR-AA-07 BR-AA-10"; strings.Join(brAA, " ") != want {
		t.Errorf("the BR-AA family decodes to %v, want %q — eight of CEN's ten VAT-category slots, without "+
			"the two arithmetic ones", brAA, want)
	}
	for id, a := range pub {
		if a.flag != "fatal" {
			t.Errorf("%s is flagged %q; every identifier in these two families is fatal, which is what lets "+
				"cius_pt_rules.go use the plain adder", id, a.flag)
		}
	}
	t.Logf("CIUS-PT publishes %d BR-CIUS-PT-* and %d BR-AA-* identifiers, all fatal, with no BR-CIUS-PT-31",
		len(brPT), len(brAA))
}

// TestCIUSPTRulesTranscribeTheArtefact compares every rule this package evaluates
// against the resolved Schematron, in both directions.
//
// The direction that matters most is "the artefact publishes an identifier the
// table does not name", because that is how a rule set grows underneath a
// validator that thinks it is complete. The other direction catches a rule this
// package evaluates that the artefact has stopped publishing.
func TestCIUSPTRulesTranscribeTheArtefact(t *testing.T) {
	pub := ptResolveArtefact(t, "2.1.1", ptOwnIdentifier)
	if pub == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	defer func() {
		if t.Failed() {
			t.Logf("the vendored 2.1.1 Schematron resolves to:\n\n%s\nRead the differences before pasting: a "+
				"changed expression is a changed rule, and cius_pt_rules.go has to change with it",
				ptRegenerateTable(t))
		}
	}()
	for id, want := range ptPublished211 {
		got, ok := pub[id]
		if !ok {
			t.Errorf("%s is in this package's table and the vendored Schematron does not publish it", id)
			continue
		}
		if got.kind != want.kind {
			t.Errorf("%s is a <%s> in the artefact and this table records a <%s>. The two are opposite — an "+
				"assert fires when its test is false and a report when it is true — so the rule is inverted",
				id, got.kind, want.kind)
		}
		if got.context != want.context {
			t.Errorf("%s context\n  artefact: %s\n  table   : %s", id, got.context, want.context)
		}
		if got.test != want.test {
			t.Errorf("%s test\n  artefact: %s\n  table   : %s", id, got.test, want.test)
		}
	}
	for id := range pub {
		if _, ok := ptPublished211[id]; !ok {
			t.Errorf("the vendored Schematron publishes %s and this package's table does not name it", id)
		}
	}
	if len(ptPublished211) != 73 {
		t.Fatalf("the table holds %d entries, want 73 (65 BR-CIUS-PT-* and 8 BR-AA-*)", len(ptPublished211))
	}
	t.Logf("checked %d resolved CIUS-PT assertions against the vendored 2.1.1 Schematron, both directions",
		len(ptPublished211))
}

// TestCIUSPTVersionsAgreeExceptOnTheCategoryAliases pins the 2.0.0/2.1.1 divergence.
//
// Both versions are live — phive-rules registers a validation set for each and
// ships ten sample instances for each — so "which version's condition governs" is a
// real question and not a formality. The answer this package gives is 2.1.1, and
// the reason it is defensible is measured rather than assumed: the two versions
// publish the same 73 identifiers, 67 of them with byte-identical resolved
// conditions, and the six that differ differ only by the VAT category-code aliases
// 2.1.1 added ('RED'/'INT' beside 'AA', 'NOR' beside 'S', 'ISE' beside 'E').
//
// Dispatching per document is not available in any case: all twenty vendored
// instances, including the ten filed under 2.1.1, declare BT-24 as
// `urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:1.0.0.`, so the
// specification identifier does not carry the version.
//
// If a third version widens the divergence, this fails, and it should: the choice
// would then need re-arguing rather than inheriting.
func TestCIUSPTVersionsAgreeExceptOnTheCategoryAliases(t *testing.T) {
	old := ptResolveArtefact(t, "2.0.0", ptOwnIdentifier)
	cur := ptResolveArtefact(t, "2.1.1", ptOwnIdentifier)
	if old == nil || cur == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	if len(old) != len(cur) {
		t.Errorf("2.0.0 publishes %d of these identifiers and 2.1.1 publishes %d; the two versions were the "+
			"same rule set", len(old), len(cur))
	}
	var differ []string
	for id, a := range cur {
		b, ok := old[id]
		if !ok {
			t.Errorf("2.1.1 publishes %s and 2.0.0 does not", id)
			continue
		}
		if a.kind != b.kind {
			t.Errorf("%s changed polarity between 2.0.0 (<%s>) and 2.1.1 (<%s>)", id, b.kind, a.kind)
		}
		if a.context != b.context || a.test != b.test {
			differ = append(differ, id)
		}
	}
	sort.Strings(differ)
	const want = "BR-AA-01 BR-AA-02 BR-AA-03 BR-AA-04 BR-AA-05 BR-AA-06 BR-AA-07 BR-AA-10 " +
		"BR-CIUS-PT-12 BR-CIUS-PT-14 BR-CIUS-PT-15 BR-CIUS-PT-16 BR-CIUS-PT-17 BR-CIUS-PT-18"
	if strings.Join(differ, " ") != want {
		t.Errorf("2.0.0 and 2.1.1 disagree on %v; the expected divergence is %q, and it is entirely the VAT "+
			"category-code aliases 2.1.1 added. A wider one means this package's choice of 2.1.1 has "+
			"consequences that were argued for a narrower difference", differ, want)
	}
	// And the difference really is only the alias codes. Two facts say so without
	// string surgery on somebody else's XPath: the aliases appear in 2.1.1 and in no
	// 2.0.0 expression, and the *element steps* the two versions reference are
	// identical, so 2.1.1 widened a set of string literals and did not rebind a rule
	// to a different part of the document.
	for _, id := range differ {
		a, b := cur[id], old[id]
		newer, older := a.context+" "+a.test, b.context+" "+b.test
		if !ptAliasCode.MatchString(newer) {
			t.Errorf("%s differs between the versions and 2.1.1 mentions none of RED/INT/NOR/ISE, so the "+
				"difference is not the alias widening this package assumed\n  2.0.0: %s\n  2.1.1: %s",
				id, older, newer)
		}
		if ptAliasCode.MatchString(older) {
			t.Errorf("%s already carries an alias code in 2.0.0: %s", id, older)
		}
		if o, n := ptElementSteps(older), ptElementSteps(newer); o != n {
			t.Errorf("%s references different element steps in the two versions, so 2.1.1 changed more than "+
				"the category codes\n  2.0.0: %s\n  2.1.1: %s", id, o, n)
		}
	}
	t.Logf("CIUS-PT 2.0.0 and 2.1.1 publish the same %d identifiers; %d differ, every one of them only by the "+
		"VAT category-code aliases 2.1.1 added", len(cur), len(differ))
}

// ptAliasCode matches the four category codes 2.1.1 added beside AA, S and E.
var ptAliasCode = regexp.MustCompile(`'(?:RED|INT|NOR|ISE)'`)

// ptStepRE matches a UBL element step in an XPath expression.
var ptStepRE = regexp.MustCompile(`\b(?:cbc|cac|ubl|cn):[A-Za-z]+`)

// ptElementSteps returns the sorted set of element steps an expression references,
// which is the structural half of "the two versions are the same rule".
func ptElementSteps(expr string) string {
	seen := map[string]bool{}
	for _, s := range ptStepRE.FindAllString(expr, -1) {
		seen[s] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// TestCIUSPTEveryPublishedRuleHasBothVerdicts is requirement four of this rule set's
// oracle, scoped to the two families AT publishes and this package evaluates: every
// identifier needs a fixture that makes it fire and evidence that it stays silent on
// a conforming document.
//
// TestEveryEvaluatedCIUSRuleFires gives the first half for all five authorities.
// This adds the half that is specific to a rule set with no unevaluated business
// rules left: the *published* set and the *evaluated* set are now the same set, so
// the guard can be stated over the artefact rather than over a hand-maintained
// table. A rule AT adds upstream fails this test on the day it is fetched, with no
// intervening state in which it is quietly unimplemented.
//
// The silent verdict is the conforming baseline (runCIUSSuite requires it clean) and
// the twenty AT sample instances (TestCIUSPTCorpus). Both are asserted elsewhere;
// what is asserted here is that they cover the published set.
func TestCIUSPTEveryPublishedRuleHasBothVerdicts(t *testing.T) {
	pub := ptResolveArtefact(t, "2.1.1", ptOwnIdentifier)
	if pub == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	fixtured := map[string]bool{}
	s := ciusSuites()[0]
	for _, c := range s.cases {
		fixtured[c.want] = true
	}
	for _, d := range s.extras {
		fixtured[d.want] = true
	}
	var missing []string
	for id := range pub {
		if !fixtured[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("AT/eSPap publishes %v and no fixture in this repository makes them fire. A published rule "+
			"with no firing fixture is one that could be deleted without a red build", missing)
	}
	for id := range fixtured {
		if _, ok := pub[id]; !ok {
			t.Errorf("the CIUS-PT mutation suite names %s, which the vendored Schematron does not publish", id)
		}
	}
}

// ptRegenerateTable prints ptPublished211 as Go source, resolved from whatever is
// currently vendored. It is how the table was written and how it should be updated
// after a re-fetch, and TestCIUSPTRulesTranscribeTheArtefact prints it as part of
// its failure so that the correction is in front of whoever caused it.
//
// The regenerated text still has to be *read* before it is pasted: the guard exists
// because a changed expression is a changed rule, and pasting one without reading
// it converts the guard into a rubber stamp. That is the failure mode of every
// generated fixture and it is worth naming here.
func ptRegenerateTable(t *testing.T) string {
	pub := ptResolveArtefact(t, "2.1.1", ptOwnIdentifier)
	ids := make([]string, 0, len(pub))
	for id := range pub {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("var ptPublished211 = map[string]ptAssertion{\n")
	for _, id := range ids {
		a := pub[id]
		fmt.Fprintf(&b, "\t%q: {kind: %q, context: %q, test: %q},\n", id, a.kind, a.context, a.test)
	}
	b.WriteString("}\n")
	return b.String()
}

// ptPublished211 is every BR-CIUS-PT-* and BR-AA-* assertion the vendored 2.1.1
// Schematron publishes, resolved. It is generated by ptRegenerateTable and it is
// the evidence behind every condition in cius_pt_rules.go: read an entry here and
// the rule body there side by side, and the transcription is checkable by a reader
// as well as by a test.
//
// `kind` is the polarity and it is the field to read first. An <assert> fires when
// its test is *false* and a <report> when it is *true*, so the same expression under
// the wrong element is the rule inverted — and 31 of these 73 are reports.
var ptPublished211 = map[string]ptAssertion{
	"BR-AA-01":      {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "((count(//cac:AllowanceCharge/cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']) + count(//cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'])) > 0 and count(cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']) > 0) or ((count(//cac:AllowanceCharge/cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']) + count(//cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'])) = 0 and count(cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']) = 0)"},
	"BR-AA-02":      {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]))"},
	"BR-AA-03":      {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]))"},
	"BR-AA-04":      {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]))"},
	"BR-AA-05":      {kind: "assert", context: "cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'] | cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']", test: "(cbc:Percent) > 0"},
	"BR-AA-06":      {kind: "assert", context: "cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[normalize-space(cbc:ID)='AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']", test: "(cbc:Percent) > 0"},
	"BR-AA-07":      {kind: "assert", context: "cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[normalize-space(cbc:ID)='AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']", test: "(cbc:Percent) > 0"},
	"BR-AA-10":      {kind: "assert", context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]", test: "not(cbc:TaxExemptionReason) and not(cbc:TaxExemptionReasonCode)"},
	"BR-CIUS-PT-01": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "exists(cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)"},
	"BR-CIUS-PT-02": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "(cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme/cbc:ID) = 'VAT'"},
	"BR-CIUS-PT-03": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "exists(cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)"},
	"BR-CIUS-PT-04": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "(cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme/cbc:ID) = 'VAT'"},
	"BR-CIUS-PT-05": {kind: "assert", context: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress", test: "exists(cbc:StreetName)"},
	"BR-CIUS-PT-06": {kind: "assert", context: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress", test: "exists(cbc:CityName)"},
	"BR-CIUS-PT-07": {kind: "assert", context: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress", test: "exists(cbc:PostalZone)"},
	"BR-CIUS-PT-08": {kind: "assert", context: "cac:TaxTotal/cac:TaxSubtotal", test: "exists(cac:TaxCategory/cac:TaxScheme/cbc:ID)"},
	"BR-CIUS-PT-09": {kind: "assert", context: "cac:InvoiceLine | cac:CreditNoteLine", test: "exists(cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme/cbc:ID)"},
	"BR-CIUS-PT-10": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "exists(cac:LegalMonetaryTotal)"},
	"BR-CIUS-PT-11": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "exists(cac:TaxTotal/cbc:TaxAmount)"},
	"BR-CIUS-PT-12": {kind: "assert", context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']", test: "(cbc:Percent) > 0"},
	"BR-CIUS-PT-13": {kind: "assert", context: "cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/cac:AdditionalItemProperty/cbc:Name", test: "not(starts-with(normalize-space(.),'#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#')) and not(starts-with(normalize-space(.),'#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#'))"},
	"BR-CIUS-PT-14": {kind: "assert", context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR']", test: "(cbc:Percent) > 0"},
	"BR-CIUS-PT-15": {kind: "assert", context: "cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or cac:ClassifiedTaxCategory/cbc:ID = 'NOR']/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or normalize-space(cbc:ID) = 'NOR']/cac:AdditionalItemProperty/cbc:Name", test: "not(starts-with(normalize-space(.),'#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#')) and not(starts-with(normalize-space(.),'#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#'))"},
	"BR-CIUS-PT-16": {kind: "assert", context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE']", test: "(cbc:Percent = 0)"},
	"BR-CIUS-PT-17": {kind: "assert", context: "cac:InvoiceLine/cac:Item[(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')]/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')]/cac:AdditionalItemProperty/cbc:Name", test: "$cnt17.1 > 0 or $cnt17.2 > 0"},
	"BR-CIUS-PT-18": {kind: "report", context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item", test: "((cac:ClassifiedTaxCategory/cbc:ID) = 'E' or (cac:ClassifiedTaxCategory/cbc:ID) = 'ISE') and not(cac:AdditionalItemProperty)"},
	"BR-CIUS-PT-19": {kind: "assert", context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']", test: "exists(cac:TaxCategory/cac:TaxScheme/cbc:ID)"},
	"BR-CIUS-PT-20": {kind: "assert", context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']", test: "exists(cac:TaxCategory/cac:TaxScheme/cbc:ID)"},
	"BR-CIUS-PT-21": {kind: "assert", context: "cac:Delivery/cac:DeliveryLocation/cac:Address", test: "exists(cbc:StreetName)"},
	"BR-CIUS-PT-22": {kind: "assert", context: "cac:Delivery/cac:DeliveryLocation/cac:Address", test: "exists(cbc:CityName)"},
	"BR-CIUS-PT-23": {kind: "assert", context: "cac:Delivery/cac:DeliveryLocation/cac:Address", test: "exists(cbc:PostalZone)"},
	"BR-CIUS-PT-24": {kind: "assert", context: "cac:OrderReference", test: "exists(cbc:ID) or exists(cbc:SalesOrderID)"},
	"BR-CIUS-PT-25": {kind: "report", context: "//ubl:Invoice | //cn:CreditNote", test: "exists(//cn:CreditNote) and not(cac:BillingReference)"},
	"BR-CIUS-PT-26": {kind: "assert", context: "cac:DespatchDocumentReference", test: "exists(cbc:ID)"},
	"BR-CIUS-PT-27": {kind: "assert", context: "cac:ReceiptDocumentReference", test: "exists(cbc:ID)"},
	"BR-CIUS-PT-28": {kind: "assert", context: "cac:OriginatorDocumentReference", test: "exists(cbc:ID)"},
	"BR-CIUS-PT-29": {kind: "assert", context: "cac:ContractDocumentReference", test: "exists(cbc:ID)"},
	"BR-CIUS-PT-30": {kind: "report", context: "cac:AdditionalDocumentReference", test: "exists(cac:Attachment) and (not(cac:Attachment/cbc:EmbeddedDocumentBinaryObject) and not(cac:Attachment/cac:ExternalReference/cbc:URI))"},
	"BR-CIUS-PT-32": {kind: "report", context: "cac:PayeeParty", test: "exists(cac:PartyName) and not(cac:PartyName/cbc:Name)"},
	"BR-CIUS-PT-33": {kind: "assert", context: "cac:ProjectReference", test: "exists(cbc:ID)"},
	"BR-CIUS-PT-34": {kind: "report", context: "cac:AccountingSupplierParty", test: "exists(cac:Party/cac:PartyIdentification) and not(cac:Party/cac:PartyIdentification/cbc:ID)"},
	"BR-CIUS-PT-35": {kind: "report", context: "cac:AccountingSupplierParty", test: "exists(cac:Party/cac:PartyName) and not(cac:Party/cac:PartyName/cbc:Name)"},
	"BR-CIUS-PT-36": {kind: "report", context: "cac:AccountingSupplierParty", test: "exists(cac:Party/cac:Contact) and (not(cac:Party/cac:Contact/cbc:Name) and not(cac:Party/cac:Contact/cbc:Telephone) and not(cac:Party/cac:Contact/cbc:ElectronicMail))"},
	"BR-CIUS-PT-37": {kind: "report", context: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress", test: "exists(cac:AddressLine) and not(cac:AddressLine/cbc:Line)"},
	"BR-CIUS-PT-38": {kind: "report", context: "cac:AccountingCustomerParty", test: "exists(cac:Party/cac:PartyIdentification) and not(cac:Party/cac:PartyIdentification/cbc:ID)"},
	"BR-CIUS-PT-39": {kind: "report", context: "cac:AccountingCustomerParty", test: "exists(cac:Party/cac:PartyName) and not(cac:Party/cac:PartyName/cbc:Name)"},
	"BR-CIUS-PT-40": {kind: "report", context: "cac:AccountingCustomerParty", test: "exists(cac:Party/cac:Contact) and (not(cac:Party/cac:Contact/cbc:Name) and not(cac:Party/cac:Contact/cbc:Telephone) and not(cac:Party/cac:Contact/cbc:ElectronicMail))"},
	"BR-CIUS-PT-41": {kind: "report", context: "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress", test: "exists(cac:AddressLine) and not(cac:AddressLine/cbc:Line)"},
	"BR-CIUS-PT-42": {kind: "report", context: "cac:PayeeParty", test: "exists(cac:PartyIdentification) and not(cac:PartyIdentification/cbc:ID)"},
	"BR-CIUS-PT-43": {kind: "report", context: "cac:PayeeParty", test: "exists(cac:PartyLegalEntity) and not(cac:PartyLegalEntity/cbc:CompanyID)"},
	"BR-CIUS-PT-44": {kind: "report", context: "cac:TaxRepresentativeParty/cac:PostalAddress", test: "exists(cac:AddressLine) and not(cac:AddressLine/cbc:Line)"},
	"BR-CIUS-PT-45": {kind: "report", context: "cac:Delivery/cac:DeliveryLocation/cac:Address", test: "exists(cac:AddressLine) and not(cac:AddressLine/cbc:Line)"},
	"BR-CIUS-PT-46": {kind: "report", context: "cac:Delivery", test: "(exists(cac:DeliveryParty) and not(cac:DeliveryParty/cac:PartyName)) or (exists(cac:DeliveryParty/cac:PartyName) and not(cac:DeliveryParty/cac:PartyName/cbc:Name))"},
	"BR-CIUS-PT-47": {kind: "report", context: "cac:PaymentMeans", test: "exists(cac:PayeeFinancialAccount) and (not(cac:PayeeFinancialAccount/cbc:ID) and not(cac:PayeeFinancialAccount/cbc:Name) and not(cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID))"},
	"BR-CIUS-PT-48": {kind: "report", context: "cac:PaymentMeans", test: "exists(cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch) and not(cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID)"},
	"BR-CIUS-PT-49": {kind: "report", context: "cac:PaymentMeans", test: "exists(cac:PaymentMandate) and (not(cac:PaymentMandate/cbc:ID) and not(cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID))"},
	"BR-CIUS-PT-50": {kind: "report", context: "cac:PaymentMeans", test: "exists(cac:PaymentMandate/cac:PayerFinancialAccount) and not(cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID)"},
	"BR-CIUS-PT-51": {kind: "report", context: "cac:InvoiceLine | cac:CreditNoteLine", test: "exists(cac:OrderLineReference) and not(cac:OrderLineReference/cbc:LineID)"},
	"BR-CIUS-PT-52": {kind: "report", context: "cac:InvoiceLine | cac:CreditNoteLine", test: "exists(cac:DocumentReference) and not(cac:DocumentReference/cbc:ID)"},
	"BR-CIUS-PT-53": {kind: "report", context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item", test: "exists(cac:BuyersItemIdentification) and not(cac:BuyersItemIdentification/cbc:ID)"},
	"BR-CIUS-PT-54": {kind: "report", context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item", test: "exists(cac:SellersItemIdentification) and not(cac:SellersItemIdentification/cbc:ID)"},
	"BR-CIUS-PT-55": {kind: "report", context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item", test: "exists(cac:StandardItemIdentification) and not(cac:StandardItemIdentification/cbc:ID)"},
	"BR-CIUS-PT-56": {kind: "report", context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item", test: "exists(cac:OriginCountry) and not(cac:OriginCountry/cbc:IdentificationCode)"},
	"BR-CIUS-PT-57": {kind: "report", context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item", test: "exists(cac:CommodityClassification) and not(cac:CommodityClassification/cbc:ItemClassificationCode)"},
	"BR-CIUS-PT-58": {kind: "assert", context: "cac:InvoiceLine/cac:Price | cac:CreditNoteLine/cac:Price", test: "not(cac:AllowanceCharge[cbc:ChargeIndicator='true'])"},
	"BR-CIUS-PT-59": {kind: "report", context: "cac:InvoiceLine/cac:Price | cac:CreditNoteLine/cac:Price", test: "exists(cac:AllowanceCharge[cbc:ChargeIndicator='false']) and not(cac:AllowanceCharge/cbc:Amount)"},
	"BR-CIUS-PT-60": {kind: "report", context: "cac:PaymentMeans", test: "exists(cac:CardAccount) and (not(cac:CardAccount/cbc:PrimaryAccountNumberID) or not(cac:CardAccount/cbc:NetworkID))"},
	"BR-CIUS-PT-61": {kind: "assert", context: "cac:PaymentTerms", test: "exists(cbc:Note)"},
	"BR-CIUS-PT-62": {kind: "report", context: "cac:LegalMonetaryTotal", test: "exists(/*/cac:AllowanceCharge[cbc:ChargeIndicator='false']) and not(cbc:AllowanceTotalAmount)"},
	"BR-CIUS-PT-63": {kind: "report", context: "cac:LegalMonetaryTotal", test: "exists(/*/cac:AllowanceCharge[cbc:ChargeIndicator='true']) and not(cbc:ChargeTotalAmount)"},
	"BR-CIUS-PT-64": {kind: "assert", context: "cac:Delivery", test: "exists(cbc:ActualDeliveryDate) or exists(cac:DeliveryParty) or exists(cac:DeliveryLocation/cbc:ID) or exists(cac:DeliveryLocation/cac:Address)"},
	"BR-CIUS-PT-65": {kind: "report", context: "//ubl:Invoice | //cn:CreditNote", test: "((cbc:InvoiceTypeCode = '383' or cbc:InvoiceTypeCode = 'ND') and not(cac:BillingReference))"},
	"BR-CIUS-PT-66": {kind: "assert", context: "//ubl:Invoice | //cn:CreditNote", test: "exists(cac:Delivery/cac:DeliveryLocation/cac:Address)"},
}

// ptContextSizes counts, per identifier, how many context nodes the rule would be
// evaluated against in one document.
//
// It is the second half of "does this rule work", and it is a different question
// from "does it fire". A rule that reports nothing across 1,690 documents is either
// a rule that was asked and kept answering yes — which is the desired outcome — or a
// rule bound to an element name no document contains, which is not a working rule at
// all and would look identical from the outside. The two are distinguishable only by
// counting the contexts, so this counts them.
//
// Each line is a transcription of one <param name="..."> the same way
// gatherPTNodes' fields are, and TestCIUSPTContextsAreReachable is what keeps the
// two honest: a context this function resolves differently from the implementation
// would show up as a rule with contexts and no possible finding, or the reverse.
func ptContextSizes(g *ptNodes) map[string]int {
	n := map[string]int{}
	doc := 1 // the rules bound to $Invoice, one context per document
	for _, id := range []string{"01", "02", "03", "04", "10", "11", "65", "66",
		"34", "35", "36", "38", "39", "40"} {
		n["BR-CIUS-PT-"+id] = doc
	}
	for _, id := range []string{"01", "02", "03", "04"} {
		n["BR-AA-"+id] = doc
	}
	if g.isCreditNote {
		n["BR-CIUS-PT-25"] = doc
	}
	add := func(id string, c int) { n[id] += c }
	add("BR-CIUS-PT-62", len(g.totals))
	add("BR-CIUS-PT-63", len(g.totals))
	for _, id := range []string{"32", "42", "43"} {
		add("BR-CIUS-PT-"+id, len(g.payees))
	}
	for _, id := range []string{"05", "06", "07", "37"} {
		add("BR-CIUS-PT-"+id, len(g.sellerAddr))
	}
	add("BR-CIUS-PT-41", len(g.buyerAddr))
	add("BR-CIUS-PT-44", len(g.taxRepAddr))
	add("BR-CIUS-PT-24", len(g.root.all("OrderReference")))
	add("BR-CIUS-PT-26", len(g.root.all("DespatchDocumentReference")))
	add("BR-CIUS-PT-27", len(g.root.all("ReceiptDocumentReference")))
	add("BR-CIUS-PT-28", len(g.root.all("OriginatorDocumentReference")))
	add("BR-CIUS-PT-29", len(g.root.all("ContractDocumentReference")))
	add("BR-CIUS-PT-33", len(g.root.all("ProjectReference")))
	add("BR-CIUS-PT-30", len(g.addDocRefs))
	add("BR-CIUS-PT-64", len(g.deliveries))
	add("BR-CIUS-PT-46", len(g.deliveries))
	for _, id := range []string{"21", "22", "23", "45"} {
		add("BR-CIUS-PT-"+id, len(g.deliverTo))
	}
	for _, id := range []string{"47", "48", "49", "50", "60"} {
		add("BR-CIUS-PT-"+id, len(g.payMeans))
	}
	add("BR-CIUS-PT-61", len(g.payTerms))
	for _, id := range []string{"09", "51", "52"} {
		add("BR-CIUS-PT-"+id, len(g.lines))
	}
	for _, id := range []string{"18", "53", "54", "55", "56", "57"} {
		add("BR-CIUS-PT-"+id, len(g.lineItems))
	}
	add("BR-CIUS-PT-58", len(g.linePrices))
	add("BR-CIUS-PT-59", len(g.linePrices))
	add("BR-CIUS-PT-19", len(g.docAllowance))
	add("BR-CIUS-PT-20", len(g.docCharge))
	add("BR-CIUS-PT-08", len(g.breakdowns))
	add("BR-CIUS-PT-12", len(ptCategoriesIn(g.bdCategories, ptCatLower...)))
	add("BR-CIUS-PT-14", len(ptCategoriesIn(g.bdCategories, ptCatStandard...)))
	add("BR-CIUS-PT-16", len(ptCategoriesIn(g.bdCategories, ptCatExempt...)))
	add("BR-AA-10", len(ptCategoriesIn(g.bdCategories, ptCatLower...)))
	add("BR-AA-05", len(ptCategoriesIn(ptPathFrom(g.lineItems, "ClassifiedTaxCategory"), ptCatLower...)))
	add("BR-AA-06", len(ptCategoriesIn(ptPathFrom(g.docAllowance, "TaxCategory"), ptCatLower...)))
	add("BR-AA-07", len(ptCategoriesIn(ptPathFrom(g.docCharge, "TaxCategory"), ptCatLower...)))
	// The three item-attribute contexts partition by the line item's category, the
	// way ptVATRules does.
	for _, it := range g.lineItems {
		names := len(ptPath(it, "AdditionalItemProperty", "Name"))
		cats := it.all("ClassifiedTaxCategory")
		switch {
		case len(ptCategoriesIn(cats, ptCatLowerItemAttr...)) > 0:
			add("BR-CIUS-PT-13", names)
		case len(ptCategoriesIn(cats, ptStandardRateCodes(g, it)...)) > 0:
			add("BR-CIUS-PT-15", names)
		case len(ptCategoriesIn(cats, ptCatExempt...)) > 0:
			add("BR-CIUS-PT-17", names)
		}
	}
	return n
}

// ptContextsTheCorpusNeverReaches names the rules whose context does not occur in
// the corpus, with the reason each is correctness rather than mis-wiring.
//
// The distinction is the whole point of the guard. Every entry here has to be
// argued from the artefact and from the corpus, not assumed, because "no document
// has this element" and "I bound the rule to the wrong element name" produce exactly
// the same measurement.
var ptContextsTheCorpusNeverReaches = map[string]string{
	"BR-CIUS-PT-13": "its context is an item attribute on a \"Lower rate\" (AA) line: " +
		"$VATAA_AdditionalLine = cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/" +
		"cac:AdditionalItemProperty/cbc:Name. Four of AT/eSPap's own twenty instances carry an AA-category " +
		"line and none of the four carries an item attribute on it — which is what the rule asks for, since " +
		"it forbids exactly that. No other document in the corpus uses category AA at all. Correctness, not " +
		"mis-wiring: ciusPTExtras' ptLowerRateLineWithExemption constructs the context and the rule fires.",
	"BR-AA-06": "its context is $VATAA_Allowance = cac:AllowanceCharge[cbc:ChargeIndicator='false']/" +
		"cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or 'RED' or 'INT'] — a document-level *allowance* in " +
		"the \"Lower rate\" category. Exactly two documents in the corpus put an AA category on a " +
		"document-level allowance or charge (AT/eSPap's own \"F_b\" instance, in both versions) and both " +
		"are charges, which is BR-AA-07's context and not this one. Correctness, not mis-wiring: -07 is " +
		"reached and silent, and the two rules are the same expression on the other side of the charge " +
		"indicator; ciusPTMutations' \"lower-rate document allowance at a zero rate\" builds the context " +
		"and the rule fires.",
}

// TestCIUSPTContextsAreReachable is requirement two of this rule set's oracle: for
// every rule, either the corpus reaches its context or this package says why not.
//
// A rule that reports nothing over 1,690 documents is not evidence of anything on
// its own. This separates the two readings — asked and satisfied, versus never asked
// — and forces the second into a table with a written reason, so that a rule bound
// to a misspelt element name cannot hide behind "it never fires because conforming
// documents never trip it".
//
// The exception list is also checked in the other direction: a rule that starts
// being reachable and is still listed here fails, because the reason recorded for it
// has stopped being true.
func TestCIUSPTContextsAreReachable(t *testing.T) {
	skipWithoutCorpus(t)
	pub := ptResolveArtefact(t, "2.1.1", ptOwnIdentifier)
	if pub == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	total := map[string]int{}
	files := 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil || parsed.inv.syntax != "UBL" {
			return nil
		}
		for id, c := range ptContextSizes(gatherPTNodes(parsed.root)) {
			total[id] += c
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	atLeast(t, "CIUS-PT context sweep corpus", files, minCorpusDocuments)

	reached, unreached := 0, []string{}
	for id := range pub {
		if total[id] > 0 {
			reached++
			if why, listed := ptContextsTheCorpusNeverReaches[id]; listed {
				t.Errorf("%s is recorded as unreachable in this corpus (%q) and its context now occurs %d "+
					"times; the recorded reason has stopped being true", id, why, total[id])
			}
			continue
		}
		if _, listed := ptContextsTheCorpusNeverReaches[id]; !listed {
			unreached = append(unreached, id)
		}
	}
	sort.Strings(unreached)
	if len(unreached) != 0 {
		t.Errorf("no document in the corpus reaches the context of %v, and nothing says why. A rule bound to "+
			"an element the corpus never contains is indistinguishable from a rule bound to a misspelt one: "+
			"either add a reason to ptContextsTheCorpusNeverReaches or fix the binding", unreached)
	}
	t.Logf("CIUS-PT contexts: %d of %d published rules are reached by the corpus, %d named as unreachable "+
		"with a reason", reached, len(pub), len(ptContextsTheCorpusNeverReaches))
}
