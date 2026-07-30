package formalis

import (
	"context"
	"regexp"
	"strings"
	"sync"
)

// The firing fixtures for the 290 generated DT-CIUS-PT-* identifiers.
//
// # Why they are built rather than written
//
// TestEveryEvaluatedCIUSRuleFires states the property that matters for a rule set
// with no per-rule fixtures from its authority: a rule with nothing that makes it
// fire is a rule that could be deleted without a red build, and C27, C30 and C33
// were all found in exactly that state. The four hand-written CIUS rule sets satisfy
// it with one mutation per identifier, which is the right shape for sixty-five rules
// that each need judgement.
//
// It is the wrong shape for 290 generated ones. Two hundred and ninety hand-written
// mutations would be two hundred and ninety chances to write a fixture that trips a
// *neighbouring* rule and calls it evidence, and the reviewer of such a list cannot
// check it — which is the same argument that made the rule table generated in the
// first place.
//
// So the fixtures are built from each rule's own context: ptDTFiringDocs
// synthesises the smallest document that reaches it, and drives a battery of
// hostile values through the element and through every attribute the rule's
// assertions mention. What that proves and what it does not is worth stating
// exactly:
//
//   - It proves the rule is reachable and falsifiable — that its context selects a
//     node in a document built from the context alone, and that some value makes the
//     assertion fail. A rule bound to a misspelt element name, or one whose test can
//     never be false, fails this.
//   - It does not prove the rule says what AT meant, because the battery is derived
//     partly from the rule's own string literals. That claim rests on the drift test
//     against the Schematron, where the table holds AT's XPath verbatim.
//
// The residue is in ptDTHandFixtures: rules whose firing case needs a document the
// builder cannot infer — a currency code the arithmetic tier quantifies over, a
// second invoice line, an exemption reason that has to match a breakdown. Those are
// written out in full.

// ptDTBuiltFixtures returns the set of identifiers this repository's fixtures make
// fire. It is memoised: two tests need it and it validates a few thousand
// synthesised documents.
var ptDTBuiltFixtures = sync.OnceValue(func() map[string]bool {
	fired := map[string]bool{}
	record := func(doc string) {
		rep, err := ValidateCIUSPT(context.Background(), []byte(doc))
		if err != nil {
			return
		}
		for _, v := range rep.Violations {
			if v.Source == SourceCIUSPT {
				fired[v.Rule] = true
			}
		}
	}
	for _, f := range ptDTHandFixtures {
		record(f.xml)
	}
	record(ptDTOversizedAttachment())
	for _, pat := range []*ptDTCompiledPattern{ptDatatype, ptCondition} {
		for i := range pat.rules {
			for _, doc := range ptDTFiringDocs(pat, i) {
				record(doc)
			}
		}
	}
	return fired
})

// ptDTAttrRE finds the attribute names a rule's assertions read, which are the
// other place a hostile value has to be put.
var ptDTAttrRE = regexp.MustCompile(`@([A-Za-z][\w]*)`)

// ptDTLiteralRE finds the string literals a rule's assertions carry. AT's
// free-text conventions — the "#SOURCECURRENCYCODE#…#" notes, the
// "#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-nnn#" series — are only reachable through
// the prefix the rule itself names, so the literals are part of the battery.
var ptDTLiteralRE = regexp.MustCompile(`'([^']{0,60})'`)

// ptDTFiringDocs synthesises the candidate documents for one rule.
func ptDTFiringDocs(pat *ptDTCompiledPattern, rule int) []string {
	r := &pat.rules[rule]
	if len(r.asserts) == 0 {
		return nil
	}
	src := ptDatatypePattern
	if pat == ptCondition {
		src = ptConditionPattern
	}
	paths := src.rules[rule].paths

	attrs := map[string]bool{}
	values := map[string]bool{}
	for _, a := range r.asserts {
		for _, m := range ptDTAttrRE.FindAllStringSubmatch(a.test, -1) {
			attrs[m[1]] = true
		}
		for _, m := range ptDTLiteralRE.FindAllStringSubmatch(a.test, -1) {
			if lit := m[1]; lit != "" && !strings.Contains(lit, " ") {
				values[lit] = true
				values[lit+"x"] = true
				values[lit+"999#x#"] = true
				values[lit+"x#"] = true
			}
		}
	}
	for _, v := range ptDTBaseValues {
		values[v] = true
	}

	var docs []string
	for _, p := range paths {
		for v := range values {
			docs = append(docs, ptDTBuildDoc(p, v, "", "", false))
			docs = append(docs, ptDTBuildDoc(p, v, "", "", true))
			for a := range attrs {
				docs = append(docs, ptDTBuildDoc(p, v, a, v, false))
			}
		}
	}
	return docs
}

// ptDTBaseValues is the value battery: an empty element, values that are too long
// for every length limit AT sets, values that are not dates, not decimals and not in
// any code list, and decimals outside the ranges the percentage rules bound.
var ptDTBaseValues = []string{
	"", " ", "x", "0", "1", "-1",
	strings.Repeat("x", 6), strings.Repeat("x", 21), strings.Repeat("x", 51),
	strings.Repeat("x", 151), strings.Repeat("x", 201), strings.Repeat("x", 501),
	strings.Repeat("x", 1001),
	"2024-13-99", "20240115", "not a date",
	"1.5", "1.234", "-1.00", "1000.00", "0.001", "999999999999999999.99",
	"101.00", "-1.50", "12345678901234.12",
	"ZZ", "ZZZZZ", "ZZZZZZZZZZ", "A B", "text/zzz", "9 9",
}

// ptDTBuildDoc writes the smallest document whose only content is the path to the
// rule's context, with value at the leaf.
//
// dup writes the leaf twice, which is what the rules counting their own occurrences
// (a second "#CALCULATIONRATE#" note, two invoice lines with the same identifier)
// need.
func ptDTBuildDoc(p ptDTCtxPath, value, attr, attrValue string, dup bool) string {
	root := p.root
	if root == "" {
		root = "Invoice"
	}
	ns := "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	if root == "CreditNote" {
		ns = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	}
	var b strings.Builder
	b.WriteString(`<` + root + ` xmlns="` + ns + `" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">`)
	leaf := func() string {
		var s strings.Builder
		for i, st := range p.steps {
			s.WriteString("<" + st.name)
			if i == len(p.steps)-1 && attr != "" {
				s.WriteString(` ` + attr + `="` + ptDTEscape(attrValue) + `"`)
			}
			s.WriteString(">")
			if i == len(p.steps)-1 {
				s.WriteString(ptDTEscape(value))
			}
			s.WriteString(ptDTPredContent(st.pred))
		}
		for i := len(p.steps) - 1; i >= 0; i-- {
			s.WriteString("</" + p.steps[i].name + ">")
		}
		return s.String()
	}
	b.WriteString(leaf())
	if dup {
		b.WriteString(leaf())
	}
	b.WriteString(`</` + root + `>`)
	return b.String()
}

// ptDTPredEqRE reads the two context-predicate shapes AT uses —
// `cbc:ChargeIndicator = 'false'` and `normalize-space(cbc:ID) = 'AA' or …` — well
// enough to write a child that satisfies them.
var ptDTPredEqRE = regexp.MustCompile(`(?:normalize-space\()?(?:cbc|cac):(\w+)\)?\s*=\s*'([^']*)'`)

func ptDTPredContent(pred string) string {
	if pred == "" {
		return ""
	}
	m := ptDTPredEqRE.FindStringSubmatch(pred)
	if m == nil {
		return ""
	}
	return "<" + m[1] + ">" + ptDTEscape(m[2]) + "</" + m[1] + ">"
}

func ptDTEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// ptDTHandFixtures are the documents the builder cannot infer: fifteen identifiers
// whose firing case needs more than one hostile value in one place.
//
// Each is the smallest document that reaches the rule and breaks it, and each names
// the identifiers it is for. They are here rather than in ciusPTMutations because
// they are fixtures for generated rules and belong beside the generator's own
// guard, and because none of them is a substitution on the CIUS-PT baseline —
// several need a second invoice line, a tax currency the document totals disagree
// with, or a credit note.
var ptDTHandFixtures = []struct{ want, xml string }{
	// DT-CIUS-PT-098.2 and 104.2: with a tax accounting currency (BT-6) declared,
	// both VAT amounts must be stated in *that* currency, not in BT-5's.
	{"DT-CIUS-PT-098.2/104.2", ptDTHead("Invoice") + `
<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode><cbc:TaxCurrencyCode>USD</cbc:TaxCurrencyCode>
<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">1.00</cbc:TaxAmount>
  <cac:TaxSubtotal><cbc:TaxAmount currencyID="EUR">1.00</cbc:TaxAmount></cac:TaxSubtotal>
</cac:TaxTotal></Invoice>`},

	// DT-CIUS-PT-098.3 and 104.3: with no BT-6, both must be stated in BT-5's.
	{"DT-CIUS-PT-098.3/104.3", ptDTHead("Invoice") + `
<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cac:TaxTotal><cbc:TaxAmount currencyID="USD">1.00</cbc:TaxAmount>
  <cac:TaxSubtotal><cbc:TaxAmount currencyID="USD">1.00</cbc:TaxAmount></cac:TaxSubtotal>
</cac:TaxTotal></Invoice>`},

	// DT-CIUS-PT-139_2, _3 and _5: AT's three "this item attribute may appear once
	// per line" conventions, each written twice on a single line.
	{"DT-CIUS-PT-139_2/_3/_5", ptDTHead("Invoice") + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cac:Item>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#</cbc:Name></cac:AdditionalItemProperty>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#</cbc:Name></cac:AdditionalItemProperty>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#</cbc:Name></cac:AdditionalItemProperty>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#</cbc:Name></cac:AdditionalItemProperty>
  <cac:AdditionalItemProperty><cbc:Name>#LINEID@COMMITMENTLINEREFERENCE#</cbc:Name></cac:AdditionalItemProperty>
  <cac:AdditionalItemProperty><cbc:Name>#LINEID@COMMITMENTLINEREFERENCE#</cbc:Name></cac:AdditionalItemProperty>
</cac:Item></cac:InvoiceLine></Invoice>`},

	// DT-CIUS-PT-158 and 159: a document-level allowance and charge whose amount is
	// not its base amount times its percentage. 100.00 x 10% is 10.00, not 50.00.
	{"DT-CIUS-PT-158/159", ptDTHead("Invoice") + `
<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator>
  <cbc:MultiplierFactorNumeric>10.00</cbc:MultiplierFactorNumeric>
  <cbc:BaseAmount currencyID="EUR">100.00</cbc:BaseAmount>
  <cbc:Amount currencyID="EUR">50.00</cbc:Amount></cac:AllowanceCharge>
<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator>
  <cbc:MultiplierFactorNumeric>10.00</cbc:MultiplierFactorNumeric>
  <cbc:BaseAmount currencyID="EUR">100.00</cbc:BaseAmount>
  <cbc:Amount currencyID="EUR">50.00</cbc:Amount></cac:AllowanceCharge></Invoice>`},

	// DT-CIUS-PT-168 and 169: the same two rules one level down, on an invoice line.
	{"DT-CIUS-PT-168/169", ptDTHead("Invoice") + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID>
  <cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator>
    <cbc:MultiplierFactorNumeric>10.00</cbc:MultiplierFactorNumeric>
    <cbc:BaseAmount currencyID="EUR">100.00</cbc:BaseAmount>
    <cbc:Amount currencyID="EUR">50.00</cbc:Amount></cac:AllowanceCharge>
  <cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator>
    <cbc:MultiplierFactorNumeric>10.00</cbc:MultiplierFactorNumeric>
    <cbc:BaseAmount currencyID="EUR">100.00</cbc:BaseAmount>
    <cbc:Amount currencyID="EUR">50.00</cbc:Amount></cac:AllowanceCharge>
</cac:InvoiceLine></Invoice>`},

	// DT-CIUS-PT-171 and 173: the per-rate taxable-amount summations for the "Lower
	// rate" and "Standard rated" breakdowns. Each breakdown claims 500.00 taxable
	// where its lines carry 100.00.
	{"DT-CIUS-PT-171/173", ptDTHead("Invoice") + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
  <cac:Item><cac:ClassifiedTaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6.00</cbc:Percent></cac:ClassifiedTaxCategory></cac:Item></cac:InvoiceLine>
<cac:InvoiceLine><cbc:ID>2</cbc:ID><cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
  <cac:Item><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23.00</cbc:Percent></cac:ClassifiedTaxCategory></cac:Item></cac:InvoiceLine>
<cac:TaxTotal>
  <cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">500.00</cbc:TaxableAmount>
    <cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6.00</cbc:Percent></cac:TaxCategory></cac:TaxSubtotal>
  <cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">500.00</cbc:TaxableAmount>
    <cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23.00</cbc:Percent></cac:TaxCategory></cac:TaxSubtotal>
</cac:TaxTotal></Invoice>`},

	// DT-CIUS-PT-176, the invoice branch: an exempt line naming an exemption reason
	// code that no VAT breakdown carries.
	{"DT-CIUS-PT-176 (invoice)", ptDTHead("Invoice") + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cac:Item>
  <cac:ClassifiedTaxCategory><cbc:ID>E</cbc:ID></cac:ClassifiedTaxCategory>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#</cbc:Name><cbc:Value>M99</cbc:Value></cac:AdditionalItemProperty>
</cac:Item></cac:InvoiceLine></Invoice>`},

	// DT-CIUS-PT-176, the credit-note branch. AT publishes the rule twice under one
	// identifier, once per document element, and a credit note is not a mutation of
	// an invoice.
	{"DT-CIUS-PT-176 (credit note)", ptDTHead("CreditNote") + `
<cac:CreditNoteLine><cbc:ID>1</cbc:ID><cac:Item>
  <cac:ClassifiedTaxCategory><cbc:ID>E</cbc:ID></cac:ClassifiedTaxCategory>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#</cbc:Name><cbc:Value>M99</cbc:Value></cac:AdditionalItemProperty>
</cac:Item></cac:CreditNoteLine></CreditNote>`},
}

// ptDTHead opens a document with the namespaces the CIUS-PT rules are written
// against.
func ptDTHead(root string) string {
	ns := "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	if root == "CreditNote" {
		ns = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	}
	return `<` + root + ` xmlns="` + ns + `" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">`
}

// ptDTOversizedAttachment is DT-CIUS-PT-111.2's fixture and the one place a
// fixture has to be large: AT caps an embedded attachment at 5 MB and writes the
// cap as `((string-length(.) * 0.75) div 1024) < 5000`, base64's 4:3 expansion
// spelled out. The only document that breaks it is one carrying more than
// 6,826,666 characters, so it is built rather than written.
func ptDTOversizedAttachment() string {
	return ptDTHead("Invoice") + `
<cac:AdditionalDocumentReference><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="application/pdf" filename="a.pdf">` +
		strings.Repeat("A", 6826667) +
		`</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference></Invoice>`
}
