package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func beRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "ubl-BE-") {
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
<cac:AccountingSupplierParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>BE</cbc:IdentificationCode></cac:Country></cac:PostalAddress><cac:PartyLegalEntity><cbc:RegistrationName>Seller NV</cbc:RegistrationName></cac:PartyLegalEntity></cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>BE</cbc:IdentificationCode></cac:Country></cac:PostalAddress><cac:PartyLegalEntity><cbc:RegistrationName>Buyer SA</cbc:RegistrationName></cac:PartyLegalEntity></cac:Party></cac:AccountingCustomerParty>
<cac:Delivery><cac:DeliveryTerms><cbc:ID>BELM-001</cbc:ID></cac:DeliveryTerms></cac:Delivery>
<cac:PaymentTerms><cbc:SettlementDiscountPercent>2</cbc:SettlementDiscountPercent><cbc:Amount>2.00</cbc:Amount><cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate></cac:PaymentTerms>
<cac:TaxTotal><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Name>03</cbc:Name><cbc:TaxExemptionReasonCode>BETE-45</cbc:TaxExemptionReasonCode><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>121.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>121.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount>
<cac:TaxTotal><cbc:TaxAmount>21.01</cbc:TaxAmount></cac:TaxTotal>
<cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Name>45</cbc:Name><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
<cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

var ublBEMutations = []ciusMutation{
	{"no document type (02)", "<cbc:DocumentDescription>CommercialInvoice</cbc:DocumentDescription>", "<cbc:DocumentDescription>Foo</cbc:DocumentDescription>", "ubl-BE-02"},
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
