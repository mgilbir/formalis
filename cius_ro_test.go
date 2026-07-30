package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func roRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "BR-RO-") {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestCIUSROCorpus is the FP=0 oracle: every official CIUS-RO sample instance
// (phax/phive-rules, all "good" cases) must satisfy the implemented CIUS-RO
// rules. Like CIUS-PT the samples carry placeholder code-list values, so the
// check is scoped to the BR-RO-* rules. Skips when the corpus is absent
// (run `make cius-oracles`).
func TestCIUSROCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-ro/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("CIUS-RO corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-RO corpus", len(files), minCIUSROInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if ro := roRuleViolations(findings(t, context.Background(), ValidateCIUSRO, data)); len(ro) != 0 {
			t.Errorf("%s: expected 0 CIUS-RO violations on a conformant sample, got %v", filepath.Base(f), ro)
		}
	}
}

// minimalCIUSROUBL is a small CIUS-RO-conformant invoice carrying the mandatory
// address terms (seller, buyer, tax representative and delivery), in RON, with
// distinct values so each can be removed in isolation.
const minimalCIUSROUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RON</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:CountrySubentity>RO-CJ</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller SRL</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:CountrySubentity>RO-TM</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer SRL</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxRepresentativeParty><cac:PostalAddress><cbc:StreetName>RepStreet</cbc:StreetName><cbc:CityName>RepCity</cbc:CityName><cbc:CountrySubentity>RO-IS</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:TaxRepresentativeParty>
<cac:Delivery><cac:DeliveryLocation><cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:CountrySubentity>RO-BV</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>
<cac:TaxTotal><cbc:TaxAmount>19.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>19.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>119.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>119.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

var ciusROMutations = []ciusMutation{
	{"number without digit (010)", "<cbc:ID>INV-1</cbc:ID>", "<cbc:ID>INVOICE</cbc:ID>", "BR-RO-010"},
	{"bad type code (020)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>100</cbc:InvoiceTypeCode>", "BR-RO-020"},
	{"non-RON without RON tax currency (030)", "<cbc:DocumentCurrencyCode>RON</cbc:DocumentCurrencyCode>", "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>", "BR-RO-030"},
	{"no seller street (081)", "<cbc:StreetName>SellerStreet</cbc:StreetName>", "", "BR-RO-081"},
	{"no seller city (091)", "<cbc:CityName>SellerCity</cbc:CityName>", "", "BR-RO-091"},
	{"no buyer street (082)", "<cbc:StreetName>BuyerStreet</cbc:StreetName>", "", "BR-RO-082"},
	{"no buyer city (092)", "<cbc:CityName>BuyerCity</cbc:CityName>", "", "BR-RO-092"},
	{"no tax rep street (140)", "<cbc:StreetName>RepStreet</cbc:StreetName>", "", "BR-RO-140"},
	{"no tax rep city (150)", "<cbc:CityName>RepCity</cbc:CityName>", "", "BR-RO-150"},
	{"no delivery street (180)", "<cbc:StreetName>DelivStreet</cbc:StreetName>", "", "BR-RO-180"},
	{"no delivery city (201)", "<cbc:CityName>DelivCity</cbc:CityName>", "", "BR-RO-201"},
	{"no seller subdivision (110)", "<cbc:CountrySubentity>RO-CJ</cbc:CountrySubentity>", "", "BR-RO-110"},
	{"no buyer subdivision (111)", "<cbc:CountrySubentity>RO-TM</cbc:CountrySubentity>", "", "BR-RO-111"},
	{"invalid tax rep subdivision (170)", "<cbc:CountrySubentity>RO-IS</cbc:CountrySubentity>", "<cbc:CountrySubentity>XX</cbc:CountrySubentity>", "BR-RO-170"},
	{"no delivery subdivision (211)", "<cbc:CountrySubentity>RO-BV</cbc:CountrySubentity>", "", "BR-RO-211"},
	{"invalid delivery subdivision (212)", "<cbc:CountrySubentity>RO-BV</cbc:CountrySubentity>", "<cbc:CountrySubentity>XX</cbc:CountrySubentity>", "BR-RO-212"},
}

func TestCIUSROMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[1])
}
