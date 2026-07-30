package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rsRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "RSR-") {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestSRBDTCorpus is the FP=0 oracle: every official SRBDT sample instance
// (phax/phive-rules, all good cases) must satisfy the implemented SRBDT rules,
// scoped to the RSR-* rules. Skips when the corpus is absent (make cius-oracles).
func TestSRBDTCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-rs/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("SRBDT corpus not present (make cius-oracles)")
	}
	atLeast(t, "SRBDT corpus", len(files), minSRBDTInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if rs := rsRuleViolations(findings(t, context.Background(), ValidateSRBDT, data)); len(rs) != 0 {
			t.Errorf("%s: expected 0 SRBDT violations on a conformant sample, got %v", filepath.Base(f), rs)
		}
	}
}

// minimalSRBDT is a small SRBDT-conformant invoice mirroring the Serbian party
// structure (VAT + tax-status PartyTaxScheme, 9948 endpoints, RS PIB codes), with
// distinct seller/buyer values for isolated mutation.
const minimalSRBDT = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RSD</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party>
  <cbc:EndpointID schemeID="9948">100000005</cbc:EndpointID>
  <cac:PostalAddress><cbc:CityName>SellerCity</cbc:CityName><cac:Country><cbc:IdentificationCode>RS</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>RS100000005</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyTaxScheme><cbc:CompanyID>Obveznik PDV-a</cbc:CompanyID><cac:TaxScheme><cbc:ID>RS-VAT-STATUS</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller doo</cbc:RegistrationName><cbc:CompanyID>10000000</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cbc:EndpointID schemeID="9948">222222222</cbc:EndpointID>
  <cac:PostalAddress><cbc:CityName>BuyerCity</cbc:CityName><cac:Country><cbc:IdentificationCode>RS</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>RS222222222</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer doo</cbc:RegistrationName><cbc:CompanyID>20000000</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxTotal><cbc:TaxAmount>20.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>20.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>120.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>120.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Roba</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

var srbdtMutations = []ciusMutation{
	{"bad type code (03)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>999</cbc:InvoiceTypeCode>", "RSR-03"},
	{"has tax point date (04)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:TaxPointDate>2024-01-15</cbc:TaxPointDate>", "RSR-04"},
	{"no seller VAT (09)", "<cbc:CompanyID>RS100000005</cbc:CompanyID>", "", "RSR-09"},
	{"bad seller PIB (11)", "<cbc:CompanyID>RS100000005</cbc:CompanyID>", "<cbc:CompanyID>100000005</cbc:CompanyID>", "RSR-11"},
	{"no seller tax reg (10)", "<cac:PartyTaxScheme><cbc:CompanyID>Obveznik PDV-a</cbc:CompanyID><cac:TaxScheme><cbc:ID>RS-VAT-STATUS</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>", "", "RSR-10"},
	{"no seller endpoint (13)", "<cbc:EndpointID schemeID=\"9948\">100000005</cbc:EndpointID>", "", "RSR-13"},
	{"bad seller endpoint scheme (14)", "schemeID=\"9948\">100000005", "schemeID=\"0088\">100000005", "RSR-14"},
	{"no seller city (16)", "<cbc:CityName>SellerCity</cbc:CityName>", "", "RSR-16"},
	{"no buyer registration (17)", "<cbc:CompanyID>20000000</cbc:CompanyID>", "", "RSR-17"},
	{"no buyer VAT (20)", "<cbc:CompanyID>RS222222222</cbc:CompanyID>", "", "RSR-20"},
	{"bad buyer PIB (21)", "<cbc:CompanyID>RS222222222</cbc:CompanyID>", "<cbc:CompanyID>222222222</cbc:CompanyID>", "RSR-21"},
	{"no buyer endpoint (22)", "<cbc:EndpointID schemeID=\"9948\">222222222</cbc:EndpointID>", "", "RSR-22"},
	{"bad buyer endpoint scheme (23)", "schemeID=\"9948\">222222222", "schemeID=\"0088\">222222222", "RSR-23"},
	{"no buyer city (25)", "<cbc:CityName>BuyerCity</cbc:CityName>", "", "RSR-25"},
}

func TestSRBDTMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[3])
}
