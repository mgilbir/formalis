package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ptRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "BR-CIUS-PT-") {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestCIUSPTCorpus is the FP=0 oracle: every official CIUS-PT sample instance
// (phax/phive-rules, all "good" cases) must satisfy the CIUS-PT rules. The
// samples are synthetic templates that carry placeholder code-list values (e.g.
// scheme "0001"), so they legitimately trip EN 16931 code-list rules; the check
// is therefore scoped to the CIUS-PT rules. Skips when the corpus is absent
// (run `make cius-oracles`).
func TestCIUSPTCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-pt/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("CIUS-PT corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-PT corpus", len(files), minCIUSPTInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if pt := ptRuleViolations(ValidateCIUSPT(context.Background(), data).Violations); len(pt) != 0 {
			t.Errorf("%s: expected 0 CIUS-PT violations on a conformant sample, got %v", filepath.Base(f), pt)
		}
	}
}

// minimalCIUSPTUBL is a small CIUS-PT-conformant invoice: it carries every term
// the mandatory-term rules require, with distinct values so each can be removed
// in isolation. Placeholder core values are irrelevant — the tests scope to the
// BR-CIUS-PT rules.
const minimalCIUSPTUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:PostalZone>1111-001</cbc:PostalZone><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>PT111111111</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller Lda</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:CityName>BuyerCity</cbc:CityName><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>PT222222222</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer Lda</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:Delivery><cac:DeliveryLocation><cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:PostalZone>4444-002</cbc:PostalZone><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>
<cac:TaxTotal><cbc:TaxAmount>23.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>23.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>123.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>123.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

func TestCIUSPTMutations(t *testing.T) {
	if pt := ptRuleViolations(ValidateCIUSPT(context.Background(), []byte(minimalCIUSPTUBL)).Violations); len(pt) != 0 {
		t.Fatalf("baseline CIUS-PT invoice not clean: %v", pt)
	}
	cases := []struct{ name, remove, want string }{
		{"no seller VAT (01)", "<cbc:CompanyID>PT111111111</cbc:CompanyID>", "BR-CIUS-PT-01"},
		{"no buyer VAT (03)", "<cbc:CompanyID>PT222222222</cbc:CompanyID>", "BR-CIUS-PT-03"},
		{"no seller street (05)", "<cbc:StreetName>SellerStreet</cbc:StreetName>", "BR-CIUS-PT-05"},
		{"no seller city (06)", "<cbc:CityName>SellerCity</cbc:CityName>", "BR-CIUS-PT-06"},
		{"no seller postcode (07)", "<cbc:PostalZone>1111-001</cbc:PostalZone>", "BR-CIUS-PT-07"},
		{"no totals (10)", "<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>123.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>123.00</cbc:PayableAmount></cac:LegalMonetaryTotal>", "BR-CIUS-PT-10"},
		{"no total VAT (11)", "<cbc:TaxAmount>23.00</cbc:TaxAmount>", "BR-CIUS-PT-11"},
		{"no delivery (66)", "<cac:Delivery><cac:DeliveryLocation><cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:PostalZone>4444-002</cbc:PostalZone><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>", "BR-CIUS-PT-66"},
		{"no deliver-to street (21)", "<cbc:StreetName>DelivStreet</cbc:StreetName>", "BR-CIUS-PT-21"},
		{"no deliver-to city (22)", "<cbc:CityName>DelivCity</cbc:CityName>", "BR-CIUS-PT-22"},
		{"no deliver-to postcode (23)", "<cbc:PostalZone>4444-002</cbc:PostalZone>", "BR-CIUS-PT-23"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalCIUSPTUBL, tc.remove, "", 1)
			if broken == minimalCIUSPTUBL {
				t.Fatalf("mutation string not found: %q", tc.remove)
			}
			if !hasFacturXRule(ValidateCIUSPT(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ptRuleViolations(ValidateCIUSPT(context.Background(), []byte(broken)).Violations))
			}
		})
	}
}
