package formalis

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// beRuleViolations scopes a report to the Belgian rule set, by Source rather than
// by the "ubl-BE-" prefix. The two agree for this authority — every identifier it
// publishes carries that prefix — and reading Source is still the right habit: a
// prefix scope is how a whole published family came to be unwatched in C39, and this
// file's own history is that four of its fifteen rules were unimplemented and
// unmentioned for six PRs.
func beRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if v.Source == SourceUBLBE {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestUBLBECorpus is the FP=0 oracle: every official UBL.BE sample instance
// (phax/phive-rules, all "good" cases) must satisfy the implemented ubl-BE rules.
// Scoped to the ubl-BE rules. Skips when the corpus is absent (make cius-oracles).
func TestUBLBECorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-be/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("UBL.BE corpus not present (make cius-oracles)")
	}
	atLeast(t, "UBL.BE corpus", len(files), minUBLBEInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if be := beRuleViolations(findings(t, context.Background(), ValidateUBLBE, data)); len(be) != 0 {
			t.Errorf("%s: expected 0 UBL.BE violations on a conformant sample, got %v", filepath.Base(f), be)
		}
	}
}

// minimalUBLBE is a small UBL.BE-conformant invoice carrying the profile markers
// and the optional groups (delivery terms, a settlement discount, an exemption
// code) so each ubl-BE rule can be exercised. Distinct values allow isolated
// mutation.
const minimalUBLBE = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#conformant#urn:UBL.BE:1.0.0</cbc:CustomizationID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cac:AdditionalDocumentReference><cbc:ID>UBL.BE</cbc:ID><cbc:DocumentDescription>CommercialInvoice</cbc:DocumentDescription></cac:AdditionalDocumentReference>
<cac:AdditionalDocumentReference><cbc:ID>REF-2</cbc:ID><cbc:DocumentDescription>Annex</cbc:DocumentDescription></cac:AdditionalDocumentReference>
<cac:AccountingSupplierParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>BE</cbc:IdentificationCode></cac:Country></cac:PostalAddress><cac:PartyLegalEntity><cbc:RegistrationName>Seller NV</cbc:RegistrationName></cac:PartyLegalEntity></cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>BE</cbc:IdentificationCode></cac:Country></cac:PostalAddress><cac:PartyLegalEntity><cbc:RegistrationName>Buyer SA</cbc:RegistrationName></cac:PartyLegalEntity></cac:Party></cac:AccountingCustomerParty>
<cac:Delivery><cac:DeliveryTerms><cbc:ID>BELM-001</cbc:ID><cbc:SpecialTerms>Special ruling - travelagencies</cbc:SpecialTerms></cac:DeliveryTerms></cac:Delivery>
<cac:PaymentTerms><cbc:SettlementDiscountPercent>2</cbc:SettlementDiscountPercent><cbc:Amount>2.00</cbc:Amount><cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate></cac:PaymentTerms>
<cac:TaxTotal><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Name>03</cbc:Name><cbc:TaxExemptionReasonCode>BETE-45</cbc:TaxExemptionReasonCode><cbc:TaxExemptionReason>Exempt</cbc:TaxExemptionReason><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>121.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>121.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount>
<cac:TaxTotal><cbc:TaxAmount>21.01</cbc:TaxAmount></cac:TaxTotal>
<cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Name>45</cbc:Name><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
<cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

var ublBEMutations = []ciusMutation{
	{"only one document reference (01)", "<cac:AdditionalDocumentReference><cbc:ID>REF-2</cbc:ID><cbc:DocumentDescription>Annex</cbc:DocumentDescription></cac:AdditionalDocumentReference>", "", "ubl-BE-01"},
	{"no document type (02)", "<cbc:DocumentDescription>CommercialInvoice</cbc:DocumentDescription>", "<cbc:DocumentDescription>Foo</cbc:DocumentDescription>", "ubl-BE-02"},
	{"document reference without a description (04)", "<cbc:ID>REF-2</cbc:ID><cbc:DocumentDescription>Annex</cbc:DocumentDescription>", "<cbc:ID>REF-2</cbc:ID>", "ubl-BE-04"},
	{"delivery terms with no special terms (06)", "<cbc:SpecialTerms>Special ruling - travelagencies</cbc:SpecialTerms>", "", "ubl-BE-06"},
	{"exemption reason outside BVERCText (12)", "<cbc:TaxExemptionReason>Exempt</cbc:TaxExemptionReason>", "<cbc:TaxExemptionReason>Because we say so</cbc:TaxExemptionReason>", "ubl-BE-12"},
	{"no UBL.BE marker (03)", "<cbc:ID>UBL.BE</cbc:ID>", "", "ubl-BE-03"},
	{"bad delivery terms (05)", "<cbc:ID>BELM-001</cbc:ID>", "<cbc:ID>BELM-999</cbc:ID>", "ubl-BE-05"},
	{"bad tax category name (10)", "<cbc:Name>03</cbc:Name>", "<cbc:Name>99</cbc:Name>", "ubl-BE-10"},
	{"bad exemption code (11)", "<cbc:TaxExemptionReasonCode>BETE-45</cbc:TaxExemptionReasonCode>", "<cbc:TaxExemptionReasonCode>BETE-XX</cbc:TaxExemptionReasonCode>", "ubl-BE-11"},
	{"bad settlement percent (07)", "<cbc:SettlementDiscountPercent>2</cbc:SettlementDiscountPercent>", "<cbc:SettlementDiscountPercent>150</cbc:SettlementDiscountPercent>", "ubl-BE-07"},
	{"settlement without amount (08)", "<cbc:Amount>2.00</cbc:Amount>", "", "ubl-BE-08"},
	{"settlement bad due date (09)", "<cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate>", "<cbc:PaymentDueDate>15/02/2024</cbc:PaymentDueDate>", "ubl-BE-09"},
	{"line without tax total (14)", "<cac:TaxTotal><cbc:TaxAmount>21.01</cbc:TaxAmount></cac:TaxTotal>", "", "ubl-BE-14"},
	{"classified category with no name (15)", "<cbc:Name>45</cbc:Name>", "", "ubl-BE-15"},
}

func TestUBLBEMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[2])
}

// TestUBLBECodeListsQuoteTheArtefact reads all four of this rule set's code lists
// back out of GLOBALUBL.BE.sch, in both directions.
//
// The two free-text ones are why it exists. BELMText and BVERCText had been left
// unimplemented with the note that they are "exact-match lists of sentences", which
// describes the rule rather than gives a reason, and a list of sentences transcribed
// by hand into Go is exactly the artefact that rots silently: a missing entry
// accuses a conforming invoice and no oracle in this repository would notice unless
// a corpus document happened to use that sentence.
//
// It reads the tokenize() calls with a decoder for C31's reason, and it applies XML
// attribute-value normalisation itself — the one place the transcription is not a
// quotation is a tab inside $BELMText, and the whole point of this test is to be the
// thing that notices if that ever changes. See beSpecialTermsArtObjects.
func TestUBLBECodeListsQuoteTheArtefact(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "cius-be", "schematron", "*", "GLOBALUBL.BE.sch"))
	if len(files) == 0 {
		t.Skip("UBL.BE Schematron not present; run `make cius-schematron`")
	}
	// The four lists, and the separator each tokenize() call uses. $BELM is split on
	// whitespace and the other three on a semicolon, which is itself a fact worth
	// pinning: a list split the other way would silently become one entry.
	want := []struct {
		let, sep string
		got      map[string]bool
	}{
		{"BELM", `\s`, beDeliveryTerms},
		{"BELMText", ";", beDeliverySpecialTerms},
		{"BTCC", ";", beTaxCategoryNames},
		{"BVERCText", ";", beExemptionReasons},
		{"BVERC", ";", beExemptionReasonCodes},
	}
	for _, f := range files {
		lets := ublBELets(t, f)
		for _, w := range want {
			raw, ok := lets[w.let]
			if !ok {
				t.Errorf("%s no longer declares $%s", filepath.Base(f), w.let)
				continue
			}
			published, sep, ok := ublBETokenize(raw)
			if !ok {
				t.Errorf("%s declares $%s as %q, which is not a tokenize() call this test can read", filepath.Base(f), w.let, raw)
				continue
			}
			if sep != w.sep {
				t.Errorf("%s splits $%s on %q, and this package transcribed it as split on %q", filepath.Base(f), w.let, sep, w.sep)
			}
			for _, code := range published {
				if !w.got[code] {
					t.Errorf("%s publishes %q in $%s and this package's list does not carry it, so a conforming "+
						"invoice using it is accused", filepath.Base(f), code, w.let)
				}
			}
			// The other direction, which is what stops the list quietly growing an
			// entry the authority does not publish. beSpecialTermsArtObjects is the
			// one permitted extra and it is named rather than pattern-matched.
			set := map[string]bool{}
			for _, code := range published {
				set[code] = true
			}
			for code := range w.got {
				if set[code] {
					continue
				}
				if w.let == "BELMText" && code == beSpecialTermsArtObjects {
					continue
				}
				t.Errorf("this package accepts %q for $%s and %s does not publish it", code, w.let, filepath.Base(f))
			}
		}
	}
}

// ublBELets returns the value of every <let> in the ubl-model-BE pattern, with XML
// attribute-value normalisation applied — Go's encoding/xml does not perform it and
// an XSLT-based Schematron processor's parser does, which is the difference the tab
// in $BELMText turns on.
func ublBELets(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	inBE := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		at := func(n string) string {
			for _, a := range se.Attr {
				if a.Name.Local == n {
					return a.Value
				}
			}
			return ""
		}
		switch se.Name.Local {
		case "pattern":
			inBE = at("id") == "ubl-model-BE"
		case "let":
			if inBE {
				out[at("name")] = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(at("value"))
			}
		}
	}
	return out
}

// ublBETokenize reads a tokenize('a;b;c', ';') expression, returning its members and
// its separator.
func ublBETokenize(expr string) ([]string, string, bool) {
	const pre = "tokenize('"
	if !strings.HasPrefix(expr, pre) || !strings.HasSuffix(expr, "')") {
		return nil, "", false
	}
	body := expr[len(pre) : len(expr)-2]
	i := strings.LastIndex(body, "', '")
	if i < 0 {
		return nil, "", false
	}
	list, sep := body[:i], body[i+len("', '"):]
	switch sep {
	case `\s`:
		return strings.Fields(list), sep, true
	case ";":
		return strings.Split(list, ";"), sep, true
	}
	return nil, sep, false
}

// TestUBLBEShipsAModifiedOlderCopyOfCENsRules records C40's answer for this
// authority, and records it as a measurement rather than as a sentence.
//
// GLOBALUBL.BE.sch is a merged artefact: alongside the fifteen ubl-BE-* rules it
// carries flattened copies of CEN's model, syntax and code-list patterns and of
// OpenPEPPOL's, so a survey that read the file wholesale would attribute nine
// hundred of CEN's rules to Belgium. It is *both* shapes C40 distinguishes at once —
// an older CEN release (it still publishes BR-66, BR-67, BR-IG-* and BR-IP-*, and
// has none of BR-AF-*, BR-AG-* or BR-B-*) and a modified one (dozens of shared
// identifiers carry a different condition, including BR-51, where CEN's UBL binding
// tests a length of at most ten characters and this file tests four to six).
//
// Deliberately not acted on, for the reason PR 23 gave for AT/eSPap's 196: these are
// CEN identifiers, this package reports them under SourceEN16931 with CEN's
// condition, and honouring one authority's modified copy would silently change what
// BR-02 means for every caller. Recorded here so the decision is visible, and
// measured here so a future release that stops diverging is visible too.
func TestUBLBEShipsAModifiedOlderCopyOfCENsRules(t *testing.T) {
	beFiles, _ := filepath.Glob(filepath.Join("testdata", "cius-be", "schematron", "*", "GLOBALUBL.BE.sch"))
	cen := filepath.Join("testdata", "en16931-artefacts", "ubl", "schematron", "preprocessed",
		"EN16931-UBL-validation-preprocessed.sch")
	if len(beFiles) == 0 {
		t.Skip("UBL.BE Schematron not present; run `make cius-schematron`")
	}
	if _, err := os.Stat(cen); err != nil {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	cenTests := schAssertTests(t, cen, nil)
	beTests := schAssertTests(t, beFiles[0], map[string]bool{"ubl-model": true, "UBL-syntax": true, "Codesmodel": true})
	var shared, differing, beOnly, cenOnly int
	for id, want := range cenTests {
		got, ok := beTests[id]
		if !ok {
			cenOnly++
			continue
		}
		shared++
		if !sameStringSet(want, got) {
			differing++
		}
	}
	for id := range beTests {
		if _, ok := cenTests[id]; !ok {
			beOnly++
		}
	}
	if shared < 900 {
		t.Fatalf("only %d CEN identifiers are shared with GLOBALUBL.BE.sch; the reader has stopped seeing the "+
			"merged CEN patterns and this measurement is about nothing", shared)
	}
	if differing == 0 {
		t.Errorf("GLOBALUBL.BE.sch now agrees with CEN's condition on all %d shared identifiers. That is a change "+
			"worth a commit message rather than a silent pass: this package reports CEN's condition on the "+
			"ground that the Belgian copy is a modified one", shared)
	}
	if beOnly == 0 || cenOnly == 0 {
		t.Errorf("GLOBALUBL.BE.sch and CEN's release now publish the same identifiers (%d only in Belgium's copy, "+
			"%d only in CEN's), so the copy is no longer an older release and the reasoning recorded here "+
			"needs revisiting", beOnly, cenOnly)
	}
	t.Logf("UBL.BE ships %d CEN identifiers beside its own 15: %d shared with CEN's current release, of which %d "+
		"carry a different condition; %d it publishes that CEN's release does not and %d the other way. "+
		"Reported under SourceEN16931 with CEN's condition, deliberately (C40)",
		len(beTests), shared, differing, beOnly, cenOnly)
}

// schAssertTests returns, per identifier, the set of test expressions the named
// Schematron file binds it to, optionally restricted to the given pattern ids.
// Identifiers are upper-cased because GLOBALUBL.BE.sch spells CEN's UBL-CR-* family
// "ubl-CR-*".
func schAssertTests(t *testing.T, path string, patterns map[string]bool) map[string]map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]bool{}
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	inPattern := patterns == nil
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		at := func(n string) string {
			for _, a := range se.Attr {
				if a.Name.Local == n {
					return a.Value
				}
			}
			return ""
		}
		switch se.Name.Local {
		case "pattern":
			inPattern = patterns == nil || patterns[at("id")]
		case "assert", "report":
			id := strings.ToUpper(at("id"))
			if !inPattern || id == "" {
				continue
			}
			if out[id] == nil {
				out[id] = map[string]bool{}
			}
			out[id][normSpace(at("test"))] = true
		}
	}
	return out
}

func sameStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// TestUBLBERuleContextsAreReachable is requirement two of this rule set's oracle:
// for every identifier this package evaluates, some document in the corpus reaches
// the node the rule is bound to.
//
// There is no exception list. Every one of the fourteen is reached, which is what a
// rule set bound to ordinary UBL structure should look like.
func TestUBLBERuleContextsAreReachable(t *testing.T) {
	seen, files := ciusContextSweep(t, func(p *parsed, seen ruleContexts) {
		if p.inv.syntax == "UBL" {
			validateUBLBERules(p.root, seen)
		}
	})
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "UBL.BE context sweep corpus", files, minCorpusDocuments)
	reportUnreached(t, "UBL.BE", seen, keysOfSeverityMap(ciusEvaluated[SourceUBLBE]), nil)
}

// keysOfSeverityMap is the sorted identifier list of one Source's entry in
// ciusEvaluated, so the context sweeps are held to the same set the firing and
// severity guards are rather than to a list of their own.
func keysOfSeverityMap(m map[string]Severity) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
