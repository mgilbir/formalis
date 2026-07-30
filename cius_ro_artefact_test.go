package formalis

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The guards that hold this package's reading of ANAF's CIUS-RO Schematron to the
// artefact, for the half of it that is written by hand.
//
// cius_ro_rules_test.go does the same for the generated half. The two are split the
// way the rule set is: 25 business rules that need judgement, and 96 length,
// decimal, date-format and occurrence limits that need transcribing without error.

// roAssertion is one published assertion, decoded.
type roAssertion struct {
	kind    string // "assert" or "report"
	context string
	test    string
	flag    string
}

// roResolveArtefact decodes one vendored release of RO16931-rules.sch.
//
// There is no resolution step to speak of — unlike CIUS-PT, ANAF publishes one
// concrete pattern with no abstract half and no <param> — but the reader is still a
// decoder rather than a regular expression, for C31's reason: three of these tests
// contain a '>' and a character class that stops at the first one would not see
// them.
func roResolveArtefact(t *testing.T, version string) map[string]roAssertion {
	t.Helper()
	path := filepath.Join("testdata", "cius-ro", "schematron", version, "cius-ro", "RO16931-rules.sch")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pattern struct {
		Rules []struct {
			Context string `xml:"context,attr"`
			Asserts []struct {
				XMLName xml.Name
				ID      string `xml:"id,attr"`
				Flag    string `xml:"flag,attr"`
				Test    string `xml:"test,attr"`
			} `xml:",any"`
		} `xml:"rule"`
	}
	if err := xml.Unmarshal(data, &pattern); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := map[string]roAssertion{}
	for _, r := range pattern.Rules {
		for _, a := range r.Asserts {
			if a.XMLName.Local != "assert" && a.XMLName.Local != "report" {
				continue
			}
			if _, dup := out[a.ID]; dup {
				t.Errorf("%s publishes %s twice", version, a.ID)
			}
			out[a.ID] = roAssertion{
				kind:    a.XMLName.Local,
				context: normalizeSpace(r.Context),
				test:    normalizeSpace(a.Test),
				flag:    a.Flag,
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s decoded to no assertion at all", path)
	}
	return out
}

// roOwnIdentifier is the filter for ANAF's own identifiers. BR-27 is in the same
// file and is CEN's; see C40 and roCENIdentifiers.
func roOwnIdentifier(id string) bool {
	return strings.HasPrefix(id, "BR-RO-") || strings.HasPrefix(id, "BR-DEC-RO-")
}

// roCENIdentifiers are the CEN identifiers ANAF re-publishes inside its own national
// file. There is one, and this package does not evaluate it under SourceCIUSRO:
// BR-27 is CEN's rule, this package reports it under SourceEN16931 with CEN's own
// condition, and honouring a national copy of a CEN identifier would silently change
// what BR-27 means for every caller. That is C40's finding for CIUS-PT and PR 23's
// recommendation, applied here.
var roCENIdentifiers = map[string]string{
	"BR-27": "cbc:PriceAmount / (.) >=0 — CEN's Item net price rule, re-published verbatim",
}

// TestCIUSROArtefactCarriesExactlyOneCENIdentifier is the C40 check for this rule
// set, stated so that a second one cannot arrive unnoticed.
//
// ANAF's copy of CEN's abstract, UBL and codelist files is *not* vendored — the
// Makefile fetches cius-ro/RO16931-rules.sch and the wrapper beside it and nothing
// else — precisely so that a survey of "the Romanian rule set" cannot come to count
// CEN's three hundred rules as Romanian. The one CEN identifier that ANAF wrote into
// its own file would defeat that, so it is named here rather than filtered by a
// prefix nobody reads.
func TestCIUSROArtefactCarriesExactlyOneCENIdentifier(t *testing.T) {
	pub := roResolveArtefact(t, roVersion)
	if pub == nil {
		t.Skip("CIUS-RO Schematron not present; run `make cius-schematron`")
	}
	var foreign []string
	for id := range pub {
		if !roOwnIdentifier(id) {
			foreign = append(foreign, id)
		}
	}
	sort.Strings(foreign)
	want := make([]string, 0, len(roCENIdentifiers))
	for id := range roCENIdentifiers {
		want = append(want, id)
	}
	sort.Strings(want)
	if strings.Join(foreign, " ") != strings.Join(want, " ") {
		t.Errorf("RO16931-rules.sch carries the non-Romanian identifiers %v; this package accounts for %v. "+
			"An identifier another authority minted is reported under that authority's Source with that "+
			"authority's condition, and a new one is a decision rather than a detail", foreign, want)
	}
	if got := pub["BR-27"].test; got != "(.) >=0" {
		t.Errorf("ANAF's copy of BR-27 tests %q; this package reports CEN's BR-27 with CEN's own condition "+
			"and records the Romanian copy as %q. A changed copy is worth reading before it is ignored",
			got, "(.) >=0")
	}
}

// roVersion is the release this package evaluates. See cius_ro.go on why the latest
// rather than a per-document choice.
const roVersion = "1.0.9"

// TestCIUSRORulesTranscribeTheArtefact compares every hand-written rule against the
// resolved Schematron, in both directions.
//
// The direction that matters most is "the artefact publishes an identifier the
// table does not name", because that is how a rule set grows underneath a validator
// that thinks it is complete.
func TestCIUSRORulesTranscribeTheArtefact(t *testing.T) {
	pub := roResolveArtefact(t, roVersion)
	if pub == nil {
		t.Skip("CIUS-RO Schematron not present; run `make cius-schematron`")
	}
	defer func() {
		if t.Failed() {
			t.Logf("the vendored %s Schematron resolves to:\n\n%s\nRead the differences before pasting: a "+
				"changed expression is a changed rule, and cius_ro.go has to change with it",
				roVersion, roRegenerateTable(t))
		}
	}()
	for id, want := range roPublished109 {
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
	for id := range roPublished109 {
		if _, ok := ciusEvaluated[SourceCIUSRO][id]; !ok {
			t.Errorf("%s is in the hand-written table and ciusEvaluated does not name it", id)
		}
	}
	if len(roPublished109) != 25 {
		t.Fatalf("the table holds %d entries, want the 25 hand-written BR-RO-NNN rules", len(roPublished109))
	}
	t.Logf("checked %d hand-written CIUS-RO assertions against the vendored %s Schematron, both directions",
		len(roPublished109), roVersion)
}

// roRegenerateTable prints roPublished109 as Go source, resolved from whatever is
// currently vendored. It is how the table was written and how it should be updated
// after a re-fetch, and the test above prints it as part of its failure so that the
// correction is in front of whoever caused it.
//
// The regenerated text still has to be *read* before it is pasted: the guard exists
// because a changed expression is a changed rule, and pasting one without reading it
// converts the guard into a rubber stamp.
func roRegenerateTable(t *testing.T) string {
	pub := roResolveArtefact(t, roVersion)
	ids := make([]string, 0, len(pub))
	for id := range pub {
		if _, ok := roPublished109[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("var roPublished109 = map[string]roAssertion{\n")
	for _, id := range ids {
		a := pub[id]
		fmt.Fprintf(&b, "\t%q: {kind: %q, context: %q, test: %q},\n", id, a.kind, a.context, a.test)
	}
	b.WriteString("}\n")
	return b.String()
}

// TestCIUSROVersionsDiffer pins what each of the four vendored releases publishes.
//
// This is the guard behind cius_ro.go's choice of 1.0.9. The four are not the same
// rule set — 1.0.9 adds seven date rules, 1.0.8 withdrew three identifiers and split
// a fourth, 1.0.4 added the credit-note branch to twenty contexts — and "evaluate
// the latest" is only defensible if what the older ones publish is written down.
//
// It also fixes the four identifiers this package does not evaluate *and does not
// name as a gap*: BR-RO-020, BR-RO-A999, BR-RO-L0301 and BR-RO-L0309 are published
// by 1.0.3 and 1.0.4 and withdrawn by ANAF, and a withdrawn rule is not a coverage
// gap (C34's reasoning: "a withdrawn or non-existent rule is not published"). Each
// has a successor in 1.0.9 and the successor is named here, so the claim is
// checkable rather than asserted.
func TestCIUSROVersionsDiffer(t *testing.T) {
	byVersion := map[string]map[string]roAssertion{}
	for _, v := range []string{"1.0.3", "1.0.4", "1.0.8", "1.0.9"} {
		pub := roResolveArtefact(t, v)
		if pub == nil {
			t.Skip("CIUS-RO Schematron not present; run `make cius-schematron`")
		}
		byVersion[v] = pub
	}
	// The published inventory of each release, ANAF's own identifiers only.
	for _, want := range []struct {
		version string
		ids     int
	}{{"1.0.3", 112}, {"1.0.4", 112}, {"1.0.8", 113}, {"1.0.9", 121}} {
		n := 0
		for id, a := range byVersion[want.version] {
			if !roOwnIdentifier(id) {
				continue
			}
			n++
			if a.flag != "fatal" {
				t.Errorf("%s flags %s %q; every CIUS-RO identifier is fatal and the coverage table says so",
					want.version, id, a.flag)
			}
		}
		if n != want.ids {
			t.Errorf("CIUS-RO %s publishes %d of ANAF's own identifiers, want %d — a count that moved means "+
				"upstream changed the rule set, and which release this package evaluates is a decision that "+
				"has to be re-taken rather than inherited", want.version, n, want.ids)
		}
	}
	// The identifiers 1.0.9 does not publish, with the successor that replaced each.
	withdrawn := map[string]string{
		"BR-RO-020":   "BR-RO-020_1 and BR-RO-020_2, one per document element",
		"BR-RO-A999":  "withdrawn outright: ANAF's 1.0.8 changelog says \"eliminate rules BR-RO-L030 and BR-RO-A999\"",
		"BR-RO-L0301": "BR-RO-L155, which raised BT-1's limit from 30 characters to 200",
		"BR-RO-L0309": "BR-RO-L156, which raised BT-25's limit from 30 characters to 200",
	}
	gone := map[string]bool{}
	for _, v := range []string{"1.0.3", "1.0.4"} {
		for id := range byVersion[v] {
			if !roOwnIdentifier(id) {
				continue
			}
			if _, ok := byVersion["1.0.9"][id]; !ok {
				gone[id] = true
			}
		}
	}
	var got, want []string
	for id := range gone {
		got = append(got, id)
	}
	for id := range withdrawn {
		want = append(want, id)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the 1.0.3/1.0.4 releases publish %v that 1.0.9 does not; this package accounts for %v as "+
			"withdrawn. An identifier that disappears without a successor is a rule this package silently "+
			"stopped evaluating", got, want)
	}
	// And the successor really is in 1.0.9.
	for _, successor := range []string{"BR-RO-020_1", "BR-RO-020_2", "BR-RO-L155", "BR-RO-L156"} {
		if _, ok := byVersion["1.0.9"][successor]; !ok {
			t.Errorf("%s is named as the successor of a withdrawn 1.0.3 rule and 1.0.9 does not publish it",
				successor)
		}
	}
	// The seven date rules are 1.0.9's alone, which is the largest single change
	// between the releases and the reason evaluating 1.0.8 would have been a
	// materially smaller rule set.
	dt := 0
	for id := range byVersion["1.0.9"] {
		if strings.HasPrefix(id, "BR-RO-DT") {
			dt++
			if _, ok := byVersion["1.0.8"][id]; ok {
				t.Errorf("1.0.8 publishes %s; the BR-RO-DT* family is recorded as new in 1.0.9", id)
			}
		}
	}
	if dt != 7 {
		t.Errorf("1.0.9 publishes %d BR-RO-DT* identifiers, want 7", dt)
	}
	// The specification identifier BR-RO-001 requires, which is the one place a
	// version literal appears in the rule set and the reason twenty-two of ANAF's
	// own samples report it. 1.0.3/1.0.4 bind 1.0.0 and 1.0.8/1.0.9 bind 1.0.1, so
	// BT-24 tells the two *document* generations apart and cannot tell 1.0.8 from
	// 1.0.9 — which is why per-version dispatch is not warranted. See cius_ro.go.
	for _, v := range []struct{ version, want string }{
		{"1.0.3", "1.0.0"}, {"1.0.4", "1.0.0"}, {"1.0.8", "1.0.1"}, {"1.0.9", "1.0.1"},
	} {
		if got := roDeclaredVersion(t, v.version); got != v.want {
			t.Errorf("CIUS-RO %s binds $RO-MAJOR-MINOR-PATCH-VERSION to %q, want %q", v.version, got, v.want)
		}
	}
	if !strings.HasSuffix(roCustomizationID, ":"+roDeclaredVersion(t, roVersion)) {
		t.Errorf("roCustomizationID is %q and %s declares version %q; BR-RO-001 is checking for an "+
			"identifier the release this package evaluates does not require",
			roCustomizationID, roVersion, roDeclaredVersion(t, roVersion))
	}
	t.Logf("CIUS-RO publishes 112/112/113/121 of ANAF's own identifiers in 1.0.3/1.0.4/1.0.8/1.0.9, all "+
		"fatal; %v are withdrawn and each has a named successor; the seven BR-RO-DT* are new in 1.0.9", want)
}

// roDeclaredVersion reads $RO-MAJOR-MINOR-PATCH-VERSION out of one release.
func roDeclaredVersion(t *testing.T, version string) string {
	t.Helper()
	path := filepath.Join("testdata", "cius-ro", "schematron", version, "cius-ro", "RO16931-rules.sch")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pattern struct {
		Lets []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"let"`
	}
	if err := xml.Unmarshal(data, &pattern); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for _, l := range pattern.Lets {
		if l.Name == "RO-MAJOR-MINOR-PATCH-VERSION" {
			return strings.Trim(l.Value, "'")
		}
	}
	t.Errorf("%s declares no $RO-MAJOR-MINOR-PATCH-VERSION", path)
	return ""
}

// TestCIUSROEveryPublishedRuleHasBothVerdicts is requirement four of this rule
// set's oracle, stated over the artefact rather than over a hand-maintained table:
// every identifier ANAF publishes in the release this package evaluates needs a
// fixture that makes it fire, or a recorded reason why no document can.
//
// A rule ANAF adds upstream fails this test on the day it is fetched, with no
// intervening state in which it is quietly unimplemented. The silent verdict is the
// conforming baseline (runCIUSSuite requires it clean) and ANAF's forty-four sample
// instances (TestCIUSROCorpus); both are asserted elsewhere, and what is asserted
// here is that they cover the published set.
func TestCIUSROEveryPublishedRuleHasBothVerdicts(t *testing.T) {
	pub := roResolveArtefact(t, roVersion)
	if pub == nil {
		t.Skip("CIUS-RO Schematron not present; run `make cius-schematron`")
	}
	fixtured := map[string]bool{}
	s := ciusSuites()[1]
	for _, c := range s.cases {
		fixtured[c.want] = true
	}
	for _, d := range s.extras {
		fixtured[d.want] = true
	}
	for id := range roBuiltFixtures() {
		fixtured[id] = true
	}
	var missing []string
	published, unevaluable := 0, 0
	for id := range pub {
		if !roOwnIdentifier(id) {
			continue // BR-27 is CEN's; see roCENIdentifiers.
		}
		published++
		switch {
		case roUnevaluableAsserts[id] != "":
			unevaluable++
		case fixtured[id]:
		default:
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("ANAF publishes %v and no fixture in this repository makes them fire, and none of them is "+
			"recorded as unevaluable. A published rule with no firing fixture is one that could be deleted "+
			"without a red build", missing)
	}
	for id := range fixtured {
		if _, ok := pub[id]; !ok && roOwnIdentifier(id) {
			t.Errorf("a CIUS-RO fixture names %s, which the vendored Schematron does not publish", id)
		}
	}
	t.Logf("CIUS-RO %s: %d published identifiers, %d with a firing fixture, %d recorded unevaluable",
		roVersion, published, published-unevaluable, unevaluable)
}

// roPublished109 is every BR-RO-NNN assertion the vendored 1.0.9 Schematron
// publishes, decoded. It is generated by roRegenerateTable and it is the evidence
// behind every condition in cius_ro.go: read an entry here and the rule body there
// side by side, and the transcription is checkable by a reader as well as by a test.
//
// `kind` is the polarity and it is the field to read first. An <assert> fires when
// its test is *false* and a <report> when it is *true*, so the same expression under
// the wrong element is the rule inverted — and 6 of these 25 are reports.
var roPublished109 = map[string]roAssertion{
	"BR-RO-001":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "cbc:CustomizationID = $RO-CIUS-ID"},
	"BR-RO-010":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "matches(normalize-space(cbc:ID), '([0-9])')"},
	"BR-RO-020_1": {kind: "assert", context: "cbc:InvoiceTypeCode", test: "(. and ((not(contains(normalize-space(.), ' ')) and contains(' 380 384 389 751 ', concat(' ', normalize-space(.), ' ')))))"},
	"BR-RO-020_2": {kind: "assert", context: "cbc:CreditNoteTypeCode", test: "(. and ((not(contains(normalize-space(.), ' ')) and contains(' 381 ', concat(' ', normalize-space(.), ' ')))))"},
	"BR-RO-030":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "(normalize-space(cbc:TaxCurrencyCode) = 'RON' and normalize-space(cbc:DocumentCurrencyCode) != 'RON') or (normalize-space(cbc:TaxCurrencyCode) = 'RON' and normalize-space(cbc:DocumentCurrencyCode) = 'RON') or (normalize-space(cbc:TaxCurrencyCode) != 'RON' and normalize-space(cbc:DocumentCurrencyCode) = 'RON') or (not(exists (cbc:TaxCurrencyCode)) and normalize-space(cbc:DocumentCurrencyCode) = 'RON')"},
	"BR-RO-040":   {kind: "assert", context: "cac:InvoicePeriod/cbc:DescriptionCode", test: "((not(contains(normalize-space(.), ' ')) and contains(' 3 35 432 ', concat(' ', normalize-space(.), ' '))))"},
	"BR-RO-065":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "not((cac:AllowanceCharge/cac:TaxCategory/cbc:ID[ancestor::cac:AllowanceCharge/cbc:ChargeIndicator = 'false' and following-sibling::cac:TaxScheme/cbc:ID = 'VAT'] = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M')) or (cac:AllowanceCharge/cac:TaxCategory/cbc:ID[ancestor::cac:AllowanceCharge/cbc:ChargeIndicator = 'true'] = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M')) or (cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cbc:ID = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M'))) or (cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID, cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID[boolean(normalize-space(.))])"},
	"BR-RO-081":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:StreetName[boolean(normalize-space(.))]"},
	"BR-RO-082":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:StreetName[boolean(normalize-space(.))]"},
	"BR-RO-091":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName[boolean(normalize-space(.))]"},
	"BR-RO-092":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CityName[boolean(normalize-space(.))]"},
	"BR-RO-100":   {kind: "report", context: "/ubl:Invoice | /cn:CreditNote", test: "normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode) = 'RO' and normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity) = 'RO-B' and not(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName) = $SECTOR-RO-CODES)"},
	"BR-RO-101":   {kind: "report", context: "/ubl:Invoice | /cn:CreditNote", test: "normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode) = 'RO' and normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity) = 'RO-B' and not(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CityName) = $SECTOR-RO-CODES)"},
	"BR-RO-110":   {kind: "report", context: "/ubl:Invoice | /cn:CreditNote", test: "normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode) = 'RO' and not(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity) = $ISO-3166-RO-CODES)"},
	"BR-RO-111":   {kind: "report", context: "/ubl:Invoice | /cn:CreditNote", test: "normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode) = 'RO' and not(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity) = $ISO-3166-RO-CODES)"},
	"BR-RO-120":   {kind: "assert", context: "/ubl:Invoice | /cn:CreditNote", test: "not((cac:AllowanceCharge/cac:TaxCategory/cbc:ID[ancestor::cac:AllowanceCharge/cbc:ChargeIndicator = 'false' and following-sibling::cac:TaxScheme/cbc:ID = 'VAT'] = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M')) or (cac:AllowanceCharge/cac:TaxCategory/cbc:ID[ancestor::cac:AllowanceCharge/cbc:ChargeIndicator = 'true'] = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M')) or (cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cbc:ID = ('S', 'Z', 'E', 'AE', 'K', 'G', 'L', 'M'))) or (cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID, cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID[boolean(normalize-space(.))])"},
	"BR-RO-140":   {kind: "assert", context: "/ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress | /ubl:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress", test: "cbc:StreetName[boolean(normalize-space(.))]"},
	"BR-RO-150":   {kind: "assert", context: "/ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress | /ubl:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress", test: "cbc:CityName[boolean(normalize-space(.))]"},
	"BR-RO-160":   {kind: "report", context: "/ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress | /ubl:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress", test: "normalize-space(cac:Country/cbc:IdentificationCode) = 'RO' and normalize-space(cbc:CountrySubentity) = 'RO-B' and not(normalize-space(cbc:CityName) = $SECTOR-RO-CODES)"},
	"BR-RO-170":   {kind: "report", context: "/ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress | /ubl:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress", test: "normalize-space(cac:Country/cbc:IdentificationCode) = 'RO' and not(normalize-space(cbc:CountrySubentity) = $ISO-3166-RO-CODES)"},
	"BR-RO-180":   {kind: "assert", context: "/ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address | /ubl:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address", test: "cbc:StreetName[boolean(normalize-space(.))]"},
	"BR-RO-201":   {kind: "assert", context: "/ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address | /ubl:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address", test: "cbc:CityName[boolean(normalize-space(.))]"},
	"BR-RO-202":   {kind: "report", context: "/ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address | /ubl:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address", test: "normalize-space(cac:Country/cbc:IdentificationCode) = 'RO' and normalize-space(cbc:CountrySubentity) = 'RO-B' and not(normalize-space(cbc:CityName) = $SECTOR-RO-CODES)"},
	"BR-RO-211":   {kind: "assert", context: "/ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address | /ubl:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address", test: "cbc:CountrySubentity[boolean(normalize-space(.))]"},
	"BR-RO-212":   {kind: "report", context: "/ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address | /ubl:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address", test: "normalize-space(cac:Country/cbc:IdentificationCode) = 'RO' and not(normalize-space(cbc:CountrySubentity) = $ISO-3166-RO-CODES)"},
}
