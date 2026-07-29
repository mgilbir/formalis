package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZATCACorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/zatca/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("ZATCA corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := IsZATCA(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			t.Errorf("%s: not recognised as ZATCA", filepath.Base(f))
			continue
		}
		if v := ValidateZATCA(context.Background(), data).Violations; len(v) != 0 {
			t.Errorf("%s: expected 0 ZATCA violations, got %d (first %s: %s)", filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

const minimalZATCA = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ProfileID>reporting:1.0</cbc:ProfileID>
<cbc:ID>INV-1</cbc:ID>
<cbc:UUID>3cf5ee18-ee25-4ea8-8b78-2b9d5e5f5f5f</cbc:UUID>
<cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode name="0100000">388</cbc:InvoiceTypeCode>
<cbc:DocumentCurrencyCode>SAR</cbc:DocumentCurrencyCode>
<cac:AdditionalDocumentReference><cbc:ID>ICV</cbc:ID><cbc:UUID>1</cbc:UUID></cac:AdditionalDocumentReference>
<cac:AdditionalDocumentReference><cbc:ID>PIH</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="text/plain">NWZ...</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>
<cac:AdditionalDocumentReference><cbc:ID>QR</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode="text/plain">AR...</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>
<cac:AccountingSupplierParty><cac:Party><cac:PartyTaxScheme><cbc:CompanyID>300000000000003</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme></cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party><cac:PostalAddress><cac:Country><cbc:IdentificationCode>SA</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:Party></cac:AccountingCustomerParty>
</Invoice>`

func TestZATCAMutations(t *testing.T) {
	if v := ValidateZATCA(context.Background(), []byte(minimalZATCA)).Violations; len(v) != 0 {
		t.Fatalf("baseline ZATCA not clean: %v", v)
	}
	cases := []struct{ name, from, want string }{
		{"no uuid", "<cbc:UUID>3cf5ee18-ee25-4ea8-8b78-2b9d5e5f5f5f</cbc:UUID>", "ZA-uuid"},
		{"no icv", "<cac:AdditionalDocumentReference><cbc:ID>ICV</cbc:ID><cbc:UUID>1</cbc:UUID></cac:AdditionalDocumentReference>", "ZA-icv"},
		{"no pih", "<cac:AdditionalDocumentReference><cbc:ID>PIH</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode=\"text/plain\">NWZ...</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>", "ZA-pih"},
		{"no qr", "<cac:AdditionalDocumentReference><cbc:ID>QR</cbc:ID><cac:Attachment><cbc:EmbeddedDocumentBinaryObject mimeCode=\"text/plain\">AR...</cbc:EmbeddedDocumentBinaryObject></cac:Attachment></cac:AdditionalDocumentReference>", "ZA-qr"},
		{"no seller vat", "<cbc:CompanyID>300000000000003</cbc:CompanyID>", "ZA-seller-vat"},
		{"no type", "<cbc:InvoiceTypeCode name=\"0100000\">388</cbc:InvoiceTypeCode>", "ZA-type"},
		{"no date", "<cbc:IssueDate>2024-01-15</cbc:IssueDate>", "ZA-date"},
		{"no number", "<cbc:ID>INV-1</cbc:ID>", "ZA-number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalZATCA, tc.from, "", 1)
			if broken == minimalZATCA {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateZATCA(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateZATCA(context.Background(), []byte(broken)).Violations)
			}
		})
	}
}
