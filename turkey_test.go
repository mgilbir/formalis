package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurkishInvoiceCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/turkey/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("UBL-TR corpus not present (make cius-oracles)")
	}
	recognised := 0
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if !IsTurkishInvoice(data) {
			continue
		}
		recognised++
		if v := ValidateTurkishInvoice(context.Background(), data); len(v) != 0 {
			t.Errorf("%s: expected 0 UBL-TR violations, got %v", filepath.Base(f), v)
		}
	}
	if recognised == 0 {
		t.Error("no UBL-TR invoice recognised")
	}
}

const minimalTurkishInvoice = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>TR1.2</cbc:CustomizationID><cbc:ProfileID>TEMELFATURA</cbc:ProfileID>
<cbc:ID>INV-1</cbc:ID><cbc:UUID>3cf5ee18-ee25-4ea8-8b78-2b9d5e5f5f5f</cbc:UUID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>TRY</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">1234567890</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">0987654321</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
</Invoice>`

func TestTurkishInvoiceMutations(t *testing.T) {
	if v := ValidateTurkishInvoice(context.Background(), []byte(minimalTurkishInvoice)); len(v) != 0 {
		t.Fatalf("baseline UBL-TR not clean: %v", v)
	}
	cases := []struct{ name, from, to, want string }{
		{"no customization", "<cbc:CustomizationID>TR1.2</cbc:CustomizationID>", "<cbc:CustomizationID>XX1.2</cbc:CustomizationID>", "TR-customization"},
		{"no profile", "<cbc:ProfileID>TEMELFATURA</cbc:ProfileID>", "", "TR-profile"},
		{"no uuid", "<cbc:UUID>3cf5ee18-ee25-4ea8-8b78-2b9d5e5f5f5f</cbc:UUID>", "", "TR-uuid"},
		{"no number", "<cbc:ID>INV-1</cbc:ID>", "", "TR-number"},
		{"no date", "<cbc:IssueDate>2024-01-15</cbc:IssueDate>", "", "TR-date"},
		{"no type", "<cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>", "", "TR-type"},
		{"bad currency", "<cbc:DocumentCurrencyCode>TRY</cbc:DocumentCurrencyCode>", "<cbc:DocumentCurrencyCode>TRYY</cbc:DocumentCurrencyCode>", "TR-currency"},
		{"no seller id", "<cac:PartyIdentification><cbc:ID schemeID=\"VKN\">1234567890</cbc:ID></cac:PartyIdentification>", "", "TR-seller-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalTurkishInvoice, tc.from, tc.to, 1)
			if broken == minimalTurkishInvoice {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateTurkishInvoice(context.Background(), []byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateTurkishInvoice(context.Background(), []byte(broken)))
			}
		})
	}
}
