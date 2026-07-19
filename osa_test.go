package formalis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSACorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/osa/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("OSA corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if !IsOSA(data) {
			t.Errorf("%s: not recognised as OSA", filepath.Base(f))
			continue
		}
		if v := ValidateOSA(data); len(v) != 0 {
			t.Errorf("%s: expected 0 OSA violations, got %v", filepath.Base(f), v)
		}
	}
}

const minimalOSA = `<InvoiceData xmlns="http://schemas.nav.gov.hu/OSA/3.0/data">
<invoiceNumber>INV-1</invoiceNumber><invoiceIssueDate>2024-01-15</invoiceIssueDate>
<invoiceMain><invoice><invoiceHead>
<supplierInfo><supplierTaxNumber><taxpayerId>12345678</taxpayerId></supplierTaxNumber><supplierName>Elado Kft</supplierName></supplierInfo>
<customerInfo><customerName>Vevo Kft</customerName></customerInfo>
</invoiceHead></invoice></invoiceMain>
</InvoiceData>`

func TestOSAMutations(t *testing.T) {
	if v := ValidateOSA([]byte(minimalOSA)); len(v) != 0 {
		t.Fatalf("baseline OSA not clean: %v", v)
	}
	cases := []struct{ name, from, want string }{
		{"no number", "<invoiceNumber>INV-1</invoiceNumber>", "HU-number"},
		{"no date", "<invoiceIssueDate>2024-01-15</invoiceIssueDate>", "HU-date"},
		{"no supplier name", "<supplierName>Elado Kft</supplierName>", "HU-supplier-name"},
		{"no supplier tax", "<taxpayerId>12345678</taxpayerId>", "HU-supplier-tax"},
		{"no customer", "<customerInfo><customerName>Vevo Kft</customerName></customerInfo>", "HU-customer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalOSA, tc.from, "", 1)
			if broken == minimalOSA {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateOSA([]byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateOSA([]byte(broken)))
			}
		})
	}
}
