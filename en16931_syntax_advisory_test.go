package formalis

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The guards on the generated advisory binding tables. There are four kinds, and
// they answer four different questions:
//
//   - can the package read what was generated (TestAdvisorySyntaxTableParses);
//   - is what was generated still what CEN publishes
//     (TestAdvisorySyntaxTableFaithfulToTheSchematron), which is the drift test
//     en16931_codelists_test.go is the precedent for;
//   - does the evaluator get the awkward expressions right
//     (TestAdvisoryExpressions and its neighbours), since 27 of the 1,168 are not
//     a plain forbidden path and every one of them is a place a transcription
//     could quietly invert a rule;
//   - do the rules actually fire on real documents
//     (TestAdvisoryBindingsFireAcrossTheCorpus), because a table wired to nothing
//     passes every test above.

// advisoryRuleIDs is every rule identifier in the two generated tables. It is
// read by the severity guards in report_test.go, so that "which rules are
// advisory" is answered by the table rather than by a pattern on the identifier.
func advisoryRuleIDs() map[string]bool {
	out := map[string]bool{}
	for _, p := range []*advisoryPattern{advisoryUBL, advisoryCII} {
		for _, r := range p.rules {
			for _, a := range r.asserts {
				out[a.id] = true
			}
		}
	}
	return out
}

// TestAdvisorySyntaxTableParses is the load-time contract stated as a test: every
// assertion in the committed tables is inside the XPath subset the evaluator
// implements, so the package-level compilation that panics on anything else
// cannot panic in a caller's process.
//
// It also pins the counts. They are the numbers D9 scoped this work with — 676
// advisory UBL-CR-*, 21 UBL-DT-*, 440 CII-SR-*, 31 CII-DT-* — and they are here
// rather than only in the generator's output because a regeneration against a
// newer Schematron that dropped half a family would otherwise be a silent
// shrink.
func TestAdvisorySyntaxTableParses(t *testing.T) {
	byFamily := map[string]int{}
	seen := map[string]string{}
	for name, p := range map[string]*advisoryPattern{"UBL": advisoryUBL, "CII": advisoryCII} {
		for _, r := range p.rules {
			for _, a := range r.asserts {
				if _, err := advisorySyntaxParse(a.test); err != nil {
					t.Errorf("%s: %s: %v", a.id, a.test, err)
				}
				if a.message == "" {
					t.Errorf("%s has no message", a.id)
				}
				if where, dup := seen[a.id]; dup {
					t.Errorf("%s appears twice, in %s and %s; a rule evaluated twice reports twice", a.id, where, name)
				}
				seen[a.id] = name
				family := a.id[:strings.LastIndex(a.id, "-")]
				byFamily[family]++
			}
		}
	}
	want := map[string]int{"UBL-CR": 676, "UBL-DT": 21, "CII-SR": 440, "CII-DT": 31}
	for family, n := range want {
		if byFamily[family] != n {
			t.Errorf("the generated tables hold %d advisory %s-* rules, want %d; regenerate with "+
				"`make en16931-syntax-rules` and, if CEN really did change the family, move this number "+
				"deliberately", byFamily[family], family, n)
		}
	}
	for family, n := range byFamily {
		if _, ok := want[family]; !ok {
			t.Errorf("the generated tables hold %d rules of the unexpected family %s-*", n, family)
		}
	}
	t.Logf("advisory binding tables: %d UBL and %d CII assertions across %d and %d pattern rules",
		advisoryUBL.asserts, advisoryCII.asserts, len(advisoryUBL.rules), len(advisoryCII.rules))
}

// --- the drift test ------------------------------------------------------

// schAssert and schRule are the shape the abstract syntax patterns are read back
// in, for the drift test below.
type schAssert struct {
	ID   string `xml:"id,attr"`
	Flag string `xml:"flag,attr"`
	Test string `xml:"test,attr"`
	Text string `xml:",chardata"`
}

type schRule struct {
	Context string      `xml:"context,attr"`
	Asserts []schAssert `xml:"assert"`
}

type schPattern struct {
	Rules []schRule `xml:"rule"`
	Param []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"param"`
}

var advisoryMsgPrefix = regexp.MustCompile(`^\[[^]]*\]\s*-?\s*`)

// advisoryNorm is the whitespace normalisation gen.py applies to CEN's strings
// and this test applies to the same strings before comparing them. CEN's files
// carry incidental whitespace — a trailing space on a context, a double space
// inside an assertion — and the comparison should not turn on it.
func advisoryNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// readSchPattern reads one Schematron pattern file.
//
// It goes through a Decoder rather than xml.Unmarshal for one reason: CEN ships
// cii/schematron/CII/EN16931-CII-syntax.sch declared ISO-8859-1, and the standard
// decoder refuses a non-UTF-8 declaration outright unless a CharsetReader is
// supplied. Latin-1 is the one encoding where the byte value is the code point,
// so the conversion is a byte-to-rune widening and nothing about it can be
// ambiguous.
func readSchPattern(t *testing.T, path string) schPattern {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	d.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "iso-8859-1", "latin1", "iso88591", "windows-1252":
			raw, err := io.ReadAll(input)
			if err != nil {
				return nil, err
			}
			var sb strings.Builder
			for _, b := range raw {
				sb.WriteRune(rune(b))
			}
			return strings.NewReader(sb.String()), nil
		}
		return nil, fmt.Errorf("%s: unhandled charset %q", path, charset)
	}
	var p schPattern
	if err := d.Decode(&p); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return p
}

// TestAdvisorySyntaxTableFaithfulToTheSchematron re-derives both tables from the
// vendored CEN Schematron and asserts the committed ones match, rule for rule and
// assertion for assertion. It is the guard that makes generation safe to trust:
// without it the tables are a snapshot nobody can check, and the only thing
// standing between the package and a silently edited rule is whoever reads the
// diff.
//
// It compares the whole triple — identifier, XPath, text — and the rule order and
// context of each. The XPath is CEN's verbatim, which is what makes this a string
// comparison rather than an argument about a transformation.
func TestAdvisorySyntaxTableFaithfulToTheSchematron(t *testing.T) {
	dir := en16931SuiteDir()
	if dir == "" {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	cases := []struct {
		name     string
		table    []advisorySyntaxRule
		abstract string
		binding  []string
	}{
		{"UBL", advisoryUBLPattern,
			filepath.Join(dir, "ubl", "schematron", "abstract", "EN16931-syntax.sch"),
			[]string{"ubl", "schematron", "UBL", "EN16931-UBL-syntax.sch"}},
		{"CII", advisoryCIIPattern,
			filepath.Join(dir, "cii", "schematron", "abstract", "EN16931-CII-syntax.sch"),
			[]string{"cii", "schematron", "CII", "EN16931-CII-syntax.sch"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			abstract := readSchPattern(t, tc.abstract)
			binding := readSchPattern(t, filepath.Join(append([]string{dir}, tc.binding...)...))
			params := map[string]string{}
			for _, p := range binding.Param {
				params[p.Name] = p.Value
			}
			resolve := func(v string) string {
				v = strings.TrimSpace(v)
				if !strings.HasPrefix(v, "$") {
					return advisoryNorm(v)
				}
				got, ok := params[v[1:]]
				if !ok {
					t.Fatalf("the binding does not define the parameter %s", v)
				}
				return advisoryNorm(got)
			}

			if len(abstract.Rules) != len(tc.table) {
				t.Fatalf("CEN's pattern has %d rules and the committed table has %d; regenerate with "+
					"`make en16931-syntax-rules`", len(abstract.Rules), len(tc.table))
			}
			for i, r := range abstract.Rules {
				have := tc.table[i]
				if ctx := resolve(r.Context); ctx != have.context {
					t.Errorf("rule %d: CEN's context is %q and the table holds %q; the order of the rules is "+
						"load-bearing, because a node goes to the first one that matches it", i, ctx, have.context)
				}
				var want []advisorySyntaxAssert
				for _, a := range r.Asserts {
					if a.Flag != "warning" {
						continue
					}
					want = append(want, advisorySyntaxAssert{
						id:      a.ID,
						test:    resolve(a.Test),
						message: advisoryMsgPrefix.ReplaceAllString(advisoryNorm(a.Text), ""),
					})
				}
				if len(want) != len(have.asserts) {
					t.Errorf("rule %d (%s): CEN flags %d assertions warning and the table holds %d",
						i, have.context, len(want), len(have.asserts))
					continue
				}
				for j := range want {
					if want[j] != have.asserts[j] {
						t.Errorf("rule %d assertion %d: CEN publishes %+v and the table holds %+v",
							i, j, want[j], have.asserts[j])
					}
				}
			}
		})
	}
}

// TestAdvisoryTableClaimsEveryWarningCENPublishes is the same fidelity question
// asked from the other side, and it is the one that catches a rule dropped
// wholesale rather than altered. The test above walks CEN's rules and checks each
// against the table; this one counts every flag="warning" assertion in the four
// families across the two abstract patterns and asserts the tables hold exactly
// that set.
//
// It matters because a generator that skipped a rule it could not express would
// still pass every per-rule comparison. That is the defect C27 was — two fatal
// UBL-CR-* rules unimplemented inside a coverage entry describing their family as
// advisory — reintroduced at fifty times the scale.
func TestAdvisoryTableClaimsEveryWarningCENPublishes(t *testing.T) {
	dir := en16931SuiteDir()
	if dir == "" {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	families := regexp.MustCompile(`^(UBL-CR|UBL-DT|CII-SR|CII-DT)-`)
	published := map[string]bool{}
	for _, f := range []string{
		filepath.Join(dir, "ubl", "schematron", "abstract", "EN16931-syntax.sch"),
		filepath.Join(dir, "cii", "schematron", "abstract", "EN16931-CII-syntax.sch"),
	} {
		for _, r := range readSchPattern(t, f).Rules {
			for _, a := range r.Asserts {
				if a.Flag == "warning" && families.MatchString(a.ID) {
					published[a.ID] = true
				}
			}
		}
	}
	if len(published) < 1000 {
		t.Fatalf("read only %d advisory identifiers from CEN's abstract patterns; the harness is not reading "+
			"the artefacts", len(published))
	}
	have := advisoryRuleIDs()
	var missing, extra []string
	for id := range published {
		if !have[id] {
			missing = append(missing, id)
		}
	}
	for id := range have {
		if !published[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 {
		t.Errorf("CEN flags %d rules warning that the generated tables do not hold: %v", len(missing), missing)
	}
	if len(extra) != 0 {
		t.Errorf("the generated tables hold %d rules CEN does not flag warning in the four families: %v", len(extra), extra)
	}
	t.Logf("the generated tables hold all %d advisory UBL-CR/UBL-DT/CII-SR/CII-DT rules CEN publishes", len(published))
}

// --- the evaluator ------------------------------------------------------

// advisoryFor reports the advisory rule identifiers a document produces, sorted.
func advisoryFor(t *testing.T, doc string) []string {
	t.Helper()
	r := mustReport(t, context.Background(), withProfile(ProfileEN16931), []byte(doc))
	var out []string
	for _, v := range r.Warnings() {
		out = append(out, v.Rule)
	}
	sort.Strings(out)
	return out
}

func hasRule(ids []string, rule string) bool {
	for _, id := range ids {
		if id == rule {
			return true
		}
	}
	return false
}

// TestAdvisoryExpressions covers every assertion in the two tables that is not a
// plain forbidden path — the twenty-seven that use a comparison, a count, an
// arithmetic difference, a union head, a step out of the context node, or an
// axis. They are here one at a time because each is a place where a
// transcription can invert a rule rather than merely mistype it, and because the
// XPath constructs they use are the ones PRs 13 and 14 got wrong first: `!=` is
// false when either side is empty, and `= false()` casts rather than tests for
// existence.
func TestAdvisoryExpressions(t *testing.T) {
	// wrap puts children inside a UBL document element. The surrounding invoice
	// is deliberately incomplete: the core rules will report their own fatal
	// findings, and only the advisory identifiers are examined.
	wrapUBL := func(root, children string) string {
		return "<" + root + ">" + children + "</" + root + ">"
	}
	wrapCII := func(children string) string {
		return "<CrossIndustryInvoice><SupplyChainTradeTransaction>" + children +
			"</SupplyChainTradeTransaction></CrossIndustryInvoice>"
	}
	line := func(settlement string) string {
		return "<IncludedSupplyChainTradeLineItem>" + settlement + "</IncludedSupplyChainTradeLineItem>"
	}

	cases := []struct {
		name string
		doc  string
		rule string
		want bool
	}{
		// not(cbc:UBLVersionID) or cbc:UBLVersionID = '2.1'. All three arms: the
		// element absent (silent), present and equal (silent), present and
		// different (reported). A transcription that dropped the second arm would
		// accuse every Peppol invoice that declares its UBL version correctly.
		{"UBL-CR-002 absent", wrapUBL("Invoice", "<ID>1</ID>"), "UBL-CR-002", false},
		{"UBL-CR-002 is 2.1", wrapUBL("Invoice", "<UBLVersionID>2.1</UBLVersionID>"), "UBL-CR-002", false},
		{"UBL-CR-002 is 2.0", wrapUBL("Invoice", "<UBLVersionID>2.0</UBLVersionID>"), "UBL-CR-002", true},
		{"UBL-CR-002 is empty", wrapUBL("Invoice", "<UBLVersionID></UBLVersionID>"), "UBL-CR-002", true},

		// not(cac:PaymentMeans/cbc:PaymentDueDate) or ../cn:CreditNote. The second
		// arm is a step out of the document element into the document node, which
		// is only satisfiable on a credit note.
		{"UBL-CR-412 on an invoice", wrapUBL("Invoice",
			"<PaymentMeans><PaymentDueDate>2024-01-01</PaymentDueDate></PaymentMeans>"), "UBL-CR-412", true},
		{"UBL-CR-412 on a credit note", wrapUBL("CreditNote",
			"<PaymentMeans><PaymentDueDate>2024-01-01</PaymentDueDate></PaymentMeans>"), "UBL-CR-412", false},
		{"UBL-CR-412 absent", wrapUBL("Invoice", "<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>"),
			"UBL-CR-412", false},

		// not(//cac:AdditionalDocumentReference[cbc:DocumentTypeCode != '130' or
		// not(cbc:DocumentTypeCode)]/cbc:ID/@schemeID). The predicate is the
		// interesting half: a scheme identifier is permitted on a BT-18 invoiced
		// object reference (type code 130) and on nothing else, and `!=` on an
		// absent code is false, which is why CEN spells the absent case out
		// separately.
		{"UBL-CR-665 scheme on a 130 reference", wrapUBL("Invoice",
			"<AdditionalDocumentReference><DocumentTypeCode>130</DocumentTypeCode>"+
				"<ID schemeID=\"AAA\">x</ID></AdditionalDocumentReference>"), "UBL-CR-665", false},
		{"UBL-CR-665 scheme on a 916 reference", wrapUBL("Invoice",
			"<AdditionalDocumentReference><DocumentTypeCode>916</DocumentTypeCode>"+
				"<ID schemeID=\"AAA\">x</ID></AdditionalDocumentReference>"), "UBL-CR-665", true},
		{"UBL-CR-665 scheme on a reference with no code", wrapUBL("Invoice",
			"<AdditionalDocumentReference><ID schemeID=\"AAA\">x</ID></AdditionalDocumentReference>"), "UBL-CR-665", true},
		{"UBL-CR-665 no scheme", wrapUBL("Invoice",
			"<AdditionalDocumentReference><ID>x</ID></AdditionalDocumentReference>"), "UBL-CR-665", false},

		// count(//@name) - count(//cbc:PaymentMeansCode/@name) <= 0: the only @name
		// in a conforming invoice is the one on the payment means code (BT-82).
		{"UBL-DT-18 name only on the payment means code", wrapUBL("Invoice",
			"<PaymentMeans><PaymentMeansCode name=\"Credit transfer\">30</PaymentMeansCode></PaymentMeans>"),
			"UBL-DT-18", false},
		{"UBL-DT-18 name elsewhere", wrapUBL("Invoice",
			"<PaymentMeans><PaymentMeansCode name=\"Credit transfer\">30</PaymentMeansCode></PaymentMeans>"+
				"<TaxTotal><TaxSubtotal><TaxCategory><ID name=\"S\">S</ID></TaxCategory></TaxSubtotal></TaxTotal>"),
			"UBL-DT-18", true},
		{"UBL-DT-18 no name at all", wrapUBL("Invoice", "<ID>1</ID>"), "UBL-DT-18", false},

		// A union at the head of a path: not((cac:InvoiceLine|cac:CreditNoteLine)/
		// cac:SubInvoiceLine). Both alternatives have to be followed, and only from
		// the document element.
		{"UBL-CR-646 on an invoice line", wrapUBL("Invoice",
			"<InvoiceLine><SubInvoiceLine><ID>1.1</ID></SubInvoiceLine></InvoiceLine>"), "UBL-CR-646", true},
		{"UBL-CR-646 on a credit note line", wrapUBL("CreditNote",
			"<CreditNoteLine><SubInvoiceLine><ID>1.1</ID></SubInvoiceLine></CreditNoteLine>"), "UBL-CR-646", true},
		{"UBL-CR-646 absent", wrapUBL("Invoice", "<InvoiceLine><ID>1</ID></InvoiceLine>"), "UBL-CR-646", false},

		// count(ram:ApplicableTradeTax) = 1 — the one advisory assertion that
		// bounds a term from below as well as above, so an empty line settlement
		// breaks it. A `<= 1` transcription would be silent here.
		{"CII-SR-454 exactly one", wrapCII(line(
			"<SpecifiedLineTradeSettlement><ApplicableTradeTax><CategoryCode>S</CategoryCode>" +
				"</ApplicableTradeTax></SpecifiedLineTradeSettlement>")), "CII-SR-454", false},
		{"CII-SR-454 none", wrapCII(line("<SpecifiedLineTradeSettlement/>")), "CII-SR-454", true},
		{"CII-SR-454 two", wrapCII(line(
			"<SpecifiedLineTradeSettlement><ApplicableTradeTax/><ApplicableTradeTax/>" +
				"</SpecifiedLineTradeSettlement>")), "CII-SR-454", true},

		// count(ram:AdditionalReferencedDocument[normalize-space(ram:TypeCode) =
		// '916']/ram:Name) <= 1: a predicate selecting references by code, and a
		// count of a child of the selected ones.
		{"CII-SR-475 one 916 name", wrapCII(
			"<ApplicableHeaderTradeAgreement><AdditionalReferencedDocument><TypeCode>916</TypeCode>" +
				"<Name>a</Name></AdditionalReferencedDocument></ApplicableHeaderTradeAgreement>"),
			"CII-SR-475", false},
		{"CII-SR-475 two 916 names", wrapCII(
			"<ApplicableHeaderTradeAgreement><AdditionalReferencedDocument><TypeCode> 916 </TypeCode>" +
				"<Name>a</Name></AdditionalReferencedDocument><AdditionalReferencedDocument>" +
				"<TypeCode>916</TypeCode><Name>b</Name></AdditionalReferencedDocument>" +
				"</ApplicableHeaderTradeAgreement>"), "CII-SR-475", true},
		{"CII-SR-475 two names under another code", wrapCII(
			"<ApplicableHeaderTradeAgreement><AdditionalReferencedDocument><TypeCode>130</TypeCode>" +
				"<Name>a</Name></AdditionalReferencedDocument><AdditionalReferencedDocument>" +
				"<TypeCode>130</TypeCode><Name>b</Name></AdditionalReferencedDocument>" +
				"</ApplicableHeaderTradeAgreement>"), "CII-SR-475", false},

		// not(A and B): a contact may carry a person name or a department name, not
		// both. `and` between two paths, each in boolean context.
		{"CII-SR-465 person name only", wrapCII(
			"<ApplicableHeaderTradeAgreement><SellerTradeParty><DefinedTradeContact>" +
				"<PersonName>A</PersonName></DefinedTradeContact></SellerTradeParty>" +
				"</ApplicableHeaderTradeAgreement>"), "CII-SR-465", false},
		{"CII-SR-465 both names", wrapCII(
			"<ApplicableHeaderTradeAgreement><SellerTradeParty><DefinedTradeContact>" +
				"<PersonName>A</PersonName><DepartmentName>B</DepartmentName></DefinedTradeContact>" +
				"</SellerTradeParty></ApplicableHeaderTradeAgreement>"), "CII-SR-465", true},

		// A three-armed exclusive choice: at most one of ram:ID and ram:GlobalID.
		{"CII-SR-450 buyer id only", wrapCII(
			"<ApplicableHeaderTradeAgreement><BuyerTradeParty><ID>A</ID></BuyerTradeParty>" +
				"</ApplicableHeaderTradeAgreement>"), "CII-SR-450", false},
		{"CII-SR-450 buyer global id only", wrapCII(
			"<ApplicableHeaderTradeAgreement><BuyerTradeParty><GlobalID>A</GlobalID></BuyerTradeParty>" +
				"</ApplicableHeaderTradeAgreement>"), "CII-SR-450", false},
		{"CII-SR-450 neither", wrapCII(
			"<ApplicableHeaderTradeAgreement><BuyerTradeParty><Name>A</Name></BuyerTradeParty>" +
				"</ApplicableHeaderTradeAgreement>"), "CII-SR-450", false},
		{"CII-SR-450 both", wrapCII(
			"<ApplicableHeaderTradeAgreement><BuyerTradeParty><ID>A</ID><GlobalID>B</GlobalID>" +
				"</BuyerTradeParty></ApplicableHeaderTradeAgreement>"), "CII-SR-450", true},

		// `[udt:Indicator=false()]` is an xs:boolean cast, not an existence test.
		// A price-level allowance is legal; a price-level charge is not, and CEN
		// writes that as "the indicator says false and there is an amount, or
		// neither is there".
		{"CII-SR-119 price allowance", wrapCII(line(
			"<SpecifiedLineTradeAgreement><GrossPriceProductTradePrice><AppliedTradeAllowanceCharge>" +
				"<ChargeIndicator><Indicator>false</Indicator></ChargeIndicator><ActualAmount>1.00</ActualAmount>" +
				"</AppliedTradeAllowanceCharge></GrossPriceProductTradePrice></SpecifiedLineTradeAgreement>")),
			"CII-SR-119", false},
		{"CII-SR-119 price charge", wrapCII(line(
			"<SpecifiedLineTradeAgreement><GrossPriceProductTradePrice><AppliedTradeAllowanceCharge>" +
				"<ChargeIndicator><Indicator>true</Indicator></ChargeIndicator><ActualAmount>1.00</ActualAmount>" +
				"</AppliedTradeAllowanceCharge></GrossPriceProductTradePrice></SpecifiedLineTradeAgreement>")),
			"CII-SR-119", true},
		{"CII-SR-119 nothing at all", wrapCII(line("<SpecifiedLineTradeAgreement/>")), "CII-SR-119", false},

		// not(ram:BasisAmount) or (ancestor::ram:ApplicableHeaderTradeSettlement):
		// the trade-tax datatype rules exempt the header-level VAT breakdown, where
		// a basis amount is the whole point, and apply to a line-level one.
		{"CII-DT-041 header trade tax", wrapCII(
			"<ApplicableHeaderTradeSettlement><ApplicableTradeTax><BasisAmount>100.00</BasisAmount>" +
				"</ApplicableTradeTax></ApplicableHeaderTradeSettlement>"), "CII-DT-041", false},
		{"CII-DT-041 line trade tax", wrapCII(line(
			"<SpecifiedLineTradeSettlement><ApplicableTradeTax><BasisAmount>100.00</BasisAmount>" +
				"</ApplicableTradeTax></SpecifiedLineTradeSettlement>")), "CII-DT-041", true},

		// not(ram:ExemptionReasonCode) or self::ram:ApplicableTradeTax: the same
		// context, restricted with the self axis instead.
		{"CII-DT-052 on an applicable trade tax", wrapCII(
			"<ApplicableHeaderTradeSettlement><ApplicableTradeTax>" +
				"<ExemptionReasonCode>VATEX-EU-O</ExemptionReasonCode></ApplicableTradeTax>" +
				"</ApplicableHeaderTradeSettlement>"), "CII-DT-052", false},
		{"CII-DT-052 on a category trade tax", wrapCII(
			"<ApplicableHeaderTradeSettlement><SpecifiedTradeAllowanceCharge><CategoryTradeTax>" +
				"<ExemptionReasonCode>VATEX-EU-O</ExemptionReasonCode></CategoryTradeTax>" +
				"</SpecifiedTradeAllowanceCharge></ApplicableHeaderTradeSettlement>"), "CII-DT-052", true},

		// An attribute of the context node itself, on a line's VAT category code.
		{"CII-DT-045 listID on the line category code", wrapCII(line(
			"<SpecifiedLineTradeSettlement><ApplicableTradeTax><CategoryCode listID=\"UNCL5305\">S</CategoryCode>" +
				"</ApplicableTradeTax></SpecifiedLineTradeSettlement>")), "CII-DT-045", true},
		{"CII-DT-045 bare line category code", wrapCII(line(
			"<SpecifiedLineTradeSettlement><ApplicableTradeTax><CategoryCode>S</CategoryCode>" +
				"</ApplicableTradeTax></SpecifiedLineTradeSettlement>")), "CII-DT-045", false},

		// A document-wide attribute test read from the document element.
		{"UBL-DT-19 languageID anywhere", wrapUBL("Invoice",
			"<InvoiceLine><Item><Description languageID=\"en\">x</Description></Item></InvoiceLine>"),
			"UBL-DT-19", true},
		{"UBL-DT-19 no languageID", wrapUBL("Invoice",
			"<InvoiceLine><Item><Description>x</Description></Item></InvoiceLine>"), "UBL-DT-19", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := advisoryFor(t, tc.doc)
			if hasRule(got, tc.rule) != tc.want {
				t.Errorf("%s reported=%v, want %v; advisory findings were %v",
					tc.rule, !tc.want, tc.want, got)
			}
		})
	}
}

// TestAdvisoryRulesCENCannotReportAreNotReported pins the consequence of
// modelling ISO Schematron's rule order rather than assuming it away.
//
// CEN flags CII-DT-010/011/012 fatal and binds them to
// /rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:TypeCode, which the earlier
// //ram:TypeCode rule (CII-DT-008/009) has already claimed, so no conforming
// processor ever evaluates them. The document below carries exactly the attributes
// all five rules are about: CII-DT-008/009 must fire (they are fatal and
// en16931_cii_rules.go evaluates them) and CII-DT-010/011/012 must not, from
// either half of the package.
//
// It is here rather than only in Coverage(SourceEN16931) because "we do not report
// this" was a sentence in a table until the ordering became something the code
// carries.
func TestAdvisoryRulesCENCannotReportAreNotReported(t *testing.T) {
	doc := `<CrossIndustryInvoice><ExchangedDocument><ID>1</ID>` +
		`<TypeCode name="Commercial invoice" listURI="urn:x" ` +
		`listID="UNTDID1001" listAgencyID="6" listVersionID="D16B">380</TypeCode>` +
		`</ExchangedDocument></CrossIndustryInvoice>`
	r := mustReport(t, context.Background(), withProfile(ProfileEN16931), []byte(doc))
	seen := map[string]bool{}
	for _, v := range r.Violations {
		seen[v.Rule] = true
	}
	for _, rule := range []string{"CII-DT-010", "CII-DT-011", "CII-DT-012"} {
		if seen[rule] {
			t.Errorf("%s was reported; //ram:TypeCode claims the node first, so no conforming processor reaches "+
				"this rule and reporting it is a false positive", rule)
		}
	}
	// The rules that do claim the node have to be firing, or the fixture proves
	// nothing about ordering: it would pass just as well against a table wired to
	// nothing at all.
	for _, rule := range []string{"CII-DT-008", "CII-DT-009"} {
		if !seen[rule] {
			t.Errorf("%s did not fire on a type code carrying every forbidden attribute; the fixture is not "+
				"exercising the rule it is about", rule)
		}
	}
}

// TestAdvisoryContextsAgreeWithTheHandWrittenGatherers cross-checks the generated
// context matchers against the populations the hand-written fatal rules gather.
//
// The two halves of the binding read the same document and were written from the
// same Schematron by different means — one by hand, one from a matcher table — so
// where their contexts coincide they must select the same nodes. A matcher that
// is too broad silently suppresses the advisory rules of every later context; one
// that is too narrow reports rules a reference validator does not. Neither shows
// up as a failure anywhere else, because the corpus is conforming and most of
// these rules are silent on it either way.
//
// Only the contexts that genuinely coincide are compared: a context that an
// earlier rule partly claims would differ for a correct reason, and the CEN
// contexts listed below have no earlier rule matching the same element name.
func TestAdvisoryContextsAgreeWithTheHandWrittenGatherers(t *testing.T) {
	cases := []struct {
		context string
		gather  func(*ublSyntaxNodes) []*ciiNode
	}{
		{"cac:PaymentMeans", func(g *ublSyntaxNodes) []*ciiNode { return g.paymentMeans }},
		{"cac:PartyTaxScheme", func(g *ublSyntaxNodes) []*ciiNode { return g.partyTaxSchemes }},
		{"cac:TaxSubtotal", func(g *ublSyntaxNodes) []*ciiNode { return g.taxSubtotals }},
		{"cac:BillingReference", func(g *ublSyntaxNodes) []*ciiNode { return g.billingRefs }},
		{"cac:TaxRepresentativeParty", func(g *ublSyntaxNodes) []*ciiNode { return g.taxReps }},
		{"cac:Delivery", func(g *ublSyntaxNodes) []*ciiNode { return g.deliveries }},
		{"cac:AdditionalDocumentReference", func(g *ublSyntaxNodes) []*ciiNode { return g.addDocRefs }},
		{"//cac:PostalAddress | //cac:Address", func(g *ublSyntaxNodes) []*ciiNode { return g.addresses }},
		{"cac:InvoiceLine | cac:CreditNoteLine", func(g *ublSyntaxNodes) []*ciiNode { return g.lines }},
	}
	index := map[string]int{}
	for i, r := range advisoryUBL.rules {
		index[r.context] = i
	}
	for _, tc := range cases {
		if _, ok := index[tc.context]; !ok {
			t.Fatalf("the generated UBL table has no rule with context %q; the cross-check is comparing "+
				"against nothing", tc.context)
		}
	}

	files, compared := 0, 0
	_ = filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		files++
		parsed, perr := parseCII(newRun(context.Background()), data)
		if perr != nil || parsed == nil || (parsed.name != "Invoice" && parsed.name != "CreditNote") {
			return nil
		}
		compared++
		g := gatherUBLSyntaxNodes(parsed)
		// ruleFor rather than gatherAdvisoryNodes, because the walk drops the nodes
		// of a rule with no advisory assertion — it has nothing to ask them — and
		// four of the contexts below are exactly that. The question here is which
		// rule ISO Schematron would give each element to, and ruleFor is the whole
		// of the answer.
		claimed := map[int][]*ciiNode{}
		var stack []*ciiNode
		var walk func(n *ciiNode)
		walk = func(n *ciiNode) {
			stack = append(stack, n)
			if i := advisoryUBL.ruleFor(n, stack); i >= 0 {
				claimed[i] = append(claimed[i], n)
			}
			for _, ch := range n.children {
				walk(ch)
			}
			stack = stack[:len(stack)-1]
		}
		walk(parsed)
		for _, tc := range cases {
			want, got := tc.gather(g), claimed[index[tc.context]]
			if len(want) != len(got) {
				t.Errorf("%s: the context %q claims %d nodes and the hand-written gatherer found %d",
					p, tc.context, len(got), len(want))
				continue
			}
			for i := range want {
				if want[i] != got[i] {
					t.Errorf("%s: the context %q claims a different node %d than the hand-written gatherer",
						p, tc.context, i)
					break
				}
			}
		}
		return nil
	})
	if files > 0 {
		atLeast(t, "advisory context cross-check corpus", files, minCorpusDocuments)
	}
	t.Logf("cross-checked %d context matchers against the hand-written gatherers over %d UBL documents",
		len(cases), compared)
}

// TestAdvisoryBindingsFireAcrossTheCorpus is the ratchet, and the answer to "is
// this table wired to anything".
//
// Every test above would pass against a table that was compiled, parsed, checked
// against CEN and then never consulted. This one sweeps the whole corpus and
// ratchets both how many distinct advisory rules were seen to fire and how many
// findings they produced, so a change that stopped emitting four hundred of them
// — a matcher that claims too much, an evaluator short-circuit that skips too
// eagerly, a wiring change in validateEN16931 — is a red build rather than a
// quieter report.
//
// It is a floor and not an equality, for the reason corpus_test.go gives: a
// number that has to be lowered is either a corpus that shrank or a fetch that
// did not finish.
func TestAdvisoryBindingsFireAcrossTheCorpus(t *testing.T) {
	advisory := advisoryRuleIDs()
	ctx := context.Background()
	seen := map[string]int{}
	files, findings, docsWithWarnings := 0, 0, 0
	_ = filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		files++
		r, verr := Validate(ctx, data, ProfileEN16931)
		if verr != nil {
			return nil
		}
		ws := r.Warnings()
		if len(ws) > 0 {
			docsWithWarnings++
		}
		for _, v := range ws {
			if v.Source != SourceEN16931 || !advisory[v.Rule] {
				t.Errorf("%s: the warning %s/%s is not one of the generated advisory binding rules; every "+
					"warning this package emits should be", p, v.Source, v.Rule)
				continue
			}
			findings++
			seen[v.Rule]++
		}
		return nil
	})
	if files == 0 {
		t.Skip("no corpus present")
	}
	atLeast(t, "advisory sweep corpus", files, minCorpusDocuments)
	atLeast(t, "distinct advisory binding rules seen to fire", len(seen), minAdvisoryRulesFiring)
	atLeast(t, "advisory binding findings over the corpus", findings, minAdvisoryFindings)
	t.Logf("advisory bindings: %d findings from %d distinct rules over %d documents (%d documents carry at least one)",
		findings, len(seen), files, docsWithWarnings)
}

// TestAdvisoryFindingsDoNotMoveTheVerdict is the property that makes emitting
// 1,168 new rules safe to do at all: they add information and change no verdict.
// A document whose only findings are advisory is conformant, and a caller keyed on
// Report.Fatal sees nothing new.
func TestAdvisoryFindingsDoNotMoveTheVerdict(t *testing.T) {
	// A UBL invoice that is complete by the semantic model and carries three
	// elements the EN 16931 core subset leaves out.
	doc := strings.Replace(minimalUBL, "<ID>INV-1</ID>",
		"<UBLVersionID>2.0</UBLVersionID><UUID>u</UUID><CopyIndicator>false</CopyIndicator><ID>INV-1</ID>", 1)
	if doc == minimalUBL {
		t.Fatal("the fixture was not modified; minimalUBL no longer holds the anchor this test edits")
	}
	r := mustReport(t, context.Background(), withProfile(ProfileEN16931), []byte(doc))
	if v := r.Fatal(); len(v) != 0 {
		t.Fatalf("the base invoice is not clean, so this test measures nothing: %v", v)
	}
	want := map[string]bool{"UBL-CR-002": true, "UBL-CR-005": true, "UBL-CR-004": true}
	got := map[string]bool{}
	for _, v := range r.Warnings() {
		got[v.Rule] = true
		if v.Severity != SeverityWarning {
			t.Errorf("%s came back at %s", v.Rule, v.Severity)
		}
	}
	if fmt.Sprint(sortedKeys(want)) != fmt.Sprint(sortedKeys(got)) {
		t.Errorf("advisory findings were %v, want %v", sortedKeys(got), sortedKeys(want))
	}
	if !r.Conformant() {
		t.Error("a document whose only findings are advisory must still be Conformant; that is what puts " +
			"Severity on the finding")
	}
	// And Complete, which it was not until RuleFamily.Unevaluable existed. This
	// assertion was its own inverse one commit ago: what was left in
	// Coverage(SourceEN16931) after these bindings landed was three families CEN
	// itself cannot evaluate, and the table had no way to say so, so Complete was
	// permanently false. Advisory *findings* still do not move it either way —
	// Complete is about what was evaluated, not about what was found — and this
	// document has three of them.
	if !r.Complete() {
		t.Error("a document whose only findings are advisory must be Complete: every rule left in " +
			"Coverage(SourceEN16931) is one CEN published and no validator can evaluate")
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
