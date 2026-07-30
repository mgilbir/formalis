package formalis

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNLCIUSConformanceSuite uses the SimplerInvoicing SI-UBL instance test suite
// as the FP=0 oracle. Each file name encodes the rule it exercises and whether the
// instance satisfies it (…_ok_…), violates it (…_error_…) or merely uses a
// discouraged term (…_warning_…). Like the CEN unit tests, each file targets one
// rule and is not otherwise guaranteed clean, so the assertions are per-family:
//   - _error_ : at least one NLCIUS (BR-NL-*) rule must fire (the broken instance
//     is caught);
//   - _ok_ / _warning_ : no BR-NL rule may fire (no false positive; advisory
//     "not recommended" warnings are not emitted).
//
// The suite is not vendored; the test skips when testdata/nlcius is absent (run
// `make cius-oracles`).
func TestNLCIUSConformanceSuite(t *testing.T) {
	files, _ := filepath.Glob("testdata/nlcius/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("NLCIUS test suite not present (make cius-oracles)")
	}
	brNL := func(vs []Violation) []string {
		var r []string
		for _, v := range vs {
			if strings.HasPrefix(v.Rule, "BR-NL-") {
				r = append(r, v.Rule)
			}
		}
		return r
	}
	var falsePositives, missed []string
	caught, instances := 0, 0
	for _, f := range files {
		base := filepath.Base(f)
		if !strings.Contains(base, "BR-NL-") {
			continue
		}
		instances++
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		nl := brNL(findings(t, context.Background(), ValidateNLCIUS, data))
		switch {
		case strings.Contains(base, "_error"):
			if len(nl) > 0 {
				caught++
			} else {
				missed = append(missed, base)
			}
		default: // _ok_ or _warning_
			if len(nl) > 0 {
				falsePositives = append(falsePositives, base+" -> "+strings.Join(nl, ","))
			}
		}
	}
	for _, fp := range falsePositives {
		t.Errorf("NLCIUS false positive on a valid/advisory instance: %s", fp)
	}
	for _, m := range missed {
		t.Errorf("NLCIUS failed to catch a broken instance: %s", m)
	}
	// Both halves, for the reason corpus_test.go gives: "no false positive" over
	// a handful of _ok_ instances is not the same claim as over the suite, and a
	// truncation that took only the _error_ instances would leave the first
	// number looking healthy while the engine stopped catching anything.
	atLeast(t, "NLCIUS BR-NL instances", instances, minNLCIUSInstances)
	atLeast(t, "NLCIUS error instances caught", caught, minNLCIUSErrorsCaught)
	t.Logf("NLCIUS conformance: %d error instances caught, %d false positives", caught, len(falsePositives))
}

// TestNLCIUSPerRuleFixtures reads the verdict each SimplerInvoicing instance
// declares *for the rule it names*, which is the oracle PRs 19–21 built for KoSIT's
// <?xmute?> fixtures and OpenPEPPOL's unit-test directories, and which this suite
// had on disk and was not using.
//
// TestNLCIUSConformanceSuite above asks the weaker question: an _error_ instance
// must make *some* BR-NL rule fire. That passes for an engine that reports the
// wrong rule, and it passes for an engine in which one over-eager rule fires on
// everything. This one asks whether BR-NL-10's fixture makes BR-NL-10 fire, which
// is the question that tells a working rule from a coincidence — and it is what
// makes the twelve fatal identifiers individually load-bearing rather than
// collectively.
//
// One fixture is excused and the exception is derived rather than named: SI-UBL
// ships BR-NL-7_error_381_invoice.xml, whose document is an Invoice carrying type
// code 381. 381 is in BR-NL-7's permitted set, so BR-NL-7 does not fire; what fires
// is BR-NL-8, the neighbouring assertion in the same Schematron rule, which is what
// forbids a credit-note code in an Invoice. The fixture's *name* is wrong and its
// document is not. The test therefore accepts BR-NL-8 for it, and asserts that the
// document really is an Invoice with type code 381 so the excuse cannot widen.
func TestNLCIUSPerRuleFixtures(t *testing.T) {
	files, _ := filepath.Glob("testdata/nlcius/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("NLCIUS test suite not present (make cius-oracles)")
	}
	named := regexp.MustCompile(`(BR-NL-[0-9]+)_(ok|error|warning)`)
	checked, verdicts := map[string]bool{}, 0
	for _, f := range files {
		m := named.FindStringSubmatch(filepath.Base(f))
		if m == nil {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rule := m[1]
		var nl []string
		for _, v := range findings(t, context.Background(), ValidateNLCIUS, data) {
			if strings.HasPrefix(v.Rule, "BR-NL-") {
				nl = append(nl, v.Rule)
			}
		}
		verdicts++
		if m[2] != "error" {
			// _ok_ and _warning_: no fatal BR-NL rule may fire. The advisory rules
			// this package does not evaluate are why a _warning_ instance is in the
			// same bucket as an _ok_ one.
			if len(nl) != 0 {
				t.Errorf("%s: SimplerInvoicing declares this instance %s for %s, and this package reports %v",
					filepath.Base(f), m[2], rule, nl)
			}
			continue
		}
		want := rule
		if strings.Contains(filepath.Base(f), "BR-NL-7_error_381_invoice") {
			if !strings.Contains(string(data), "<cbc:InvoiceTypeCode>381<") {
				t.Errorf("%s is excused as a mis-named BR-NL-8 fixture, but it is not an Invoice with type code "+
					"381 any more; re-derive the exception rather than widening it", filepath.Base(f))
			}
			want = "BR-NL-8"
		}
		hit := false
		for _, r := range nl {
			if r == want {
				hit = true
			}
		}
		if !hit {
			t.Errorf("%s: SimplerInvoicing declares this instance invalid against %s, and this package reports %v",
				filepath.Base(f), want, nl)
			continue
		}
		checked[want] = true
	}
	atLeast(t, "NLCIUS per-rule verdicts", verdicts, minNLCIUSRuleVerdicts)
	atLeast(t, "NLCIUS rules with a fixture that fires them", len(checked), minNLCIUSRulesFired)
	t.Logf("NLCIUS per-rule fixtures: %d verdicts read, %d distinct rules made to fire by the fixture that names them",
		verdicts, len(checked))
}

// minimalNLCIUSUBL is a small SI-UBL 2.0 invoice from a Dutch supplier that
// satisfies every fatal BR-NL rule, with distinct values so each term can be
// mutated in isolation. BuyerReference and OrderReference sit adjacent so that
// BR-NL-2 — which needs *both* gone — is one substitution.
const minimalNLCIUSUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0</cbc:CustomizationID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cbc:BuyerReference>NLBUYERREF</cbc:BuyerReference><cac:OrderReference><cbc:ID>ORD-1</cbc:ID></cac:OrderReference>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:PostalZone>1011AA</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>NL123456789B01</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">11223344</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:PostalZone>2022BB</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">55667788</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Rep BV</cbc:Name></cac:PartyName><cac:PostalAddress><cbc:StreetName>RepStreet</cbc:StreetName><cbc:CityName>RepCity</cbc:CityName><cbc:PostalZone>3033CC</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:TaxRepresentativeParty>
<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>
<cac:TaxTotal><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>121.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>121.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:OrderLineReference><cbc:LineID>1</cbc:LineID></cac:OrderLineReference><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// nlciusMutations is one fixture per fatal BR-NL identifier. NLCIUS is the one CIUS
// here whose authority also ships per-rule fixtures (TestNLCIUSPerRuleFixtures
// reads them), so these are the belt to that suite's braces: they run without a
// corpus, and they pin the two rules the SI-UBL suite exercises only through a
// document that also trips something else.
var nlciusMutations = []ciusMutation{
	{"seller legal id not KVK or OIN (1)", `<cbc:CompanyID schemeID="0106">11223344</cbc:CompanyID>`, `<cbc:CompanyID schemeID="0088">11223344</cbc:CompanyID>`, "BR-NL-1"},
	{"no buyer reference and no order reference (2)", `<cbc:BuyerReference>NLBUYERREF</cbc:BuyerReference><cac:OrderReference><cbc:ID>ORD-1</cbc:ID></cac:OrderReference>`, "", "BR-NL-2"},
	{"no seller street (3)", "<cbc:StreetName>SellerStreet</cbc:StreetName>", "", "BR-NL-3"},
	{"no Dutch buyer street (4)", "<cbc:StreetName>BuyerStreet</cbc:StreetName>", "", "BR-NL-4"},
	{"no Dutch tax representative street (5)", "<cbc:StreetName>RepStreet</cbc:StreetName>", "", "BR-NL-5"},
	{"type code outside the permitted set (7)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>100</cbc:InvoiceTypeCode>", "BR-NL-7"},
	{"credit-note type code in an Invoice (8)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>381</cbc:InvoiceTypeCode>", "BR-NL-8"},
	{"corrective invoice without a preceding reference (9)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>", "BR-NL-9"},
	{"Dutch buyer legal id not KVK or OIN (10)", `<cbc:CompanyID schemeID="0106">55667788</cbc:CompanyID>`, `<cbc:CompanyID schemeID="0088">55667788</cbc:CompanyID>`, "BR-NL-10"},
	{"no means of payment (11)", `<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`, "", "BR-NL-11"},
	{"payment means code outside the permitted set (12)", "<cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>", "<cbc:PaymentMeansCode>31</cbc:PaymentMeansCode>", "BR-NL-12"},
	{"order line reference without a document order reference (13)", `<cac:OrderReference><cbc:ID>ORD-1</cbc:ID></cac:OrderReference>`, "", "BR-NL-13"},
}

func TestNLCIUSMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[4])
}
