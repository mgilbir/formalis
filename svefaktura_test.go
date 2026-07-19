package formalis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSvefakturaCorpus checks FP=0 on the direct Svefaktura invoices (the
// transport-enveloped variant is not a direct Svefaktura document and is skipped).
func TestSvefakturaCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/svefaktura/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("Svefaktura corpus not present (make cius-oracles)")
	}
	recognised := 0
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if !IsSvefaktura(data) {
			continue
		}
		recognised++
		if v := ValidateSvefaktura(data); len(v) != 0 {
			t.Errorf("%s: expected 0 Svefaktura violations, got %v", filepath.Base(f), v)
		}
	}
	if recognised == 0 {
		t.Error("no Svefaktura invoice recognised in the corpus")
	}
}

const minimalSvefaktura = `<Invoice xmlns="urn:sfti:documents:BasicInvoice:1:0">
<ID>INV-1</ID><IssueDate>2024-01-15</IssueDate><InvoiceCurrencyCode>SEK</InvoiceCurrencyCode>
<BuyerParty><Party><PartyName><Name>Koepare AB</Name></PartyName></Party></BuyerParty>
<SellerParty><Party><PartyName><Name>Saeljare AB</Name></PartyName></Party></SellerParty>
</Invoice>`

func TestSvefakturaMutations(t *testing.T) {
	if v := ValidateSvefaktura([]byte(minimalSvefaktura)); len(v) != 0 {
		t.Fatalf("baseline Svefaktura not clean: %v", v)
	}
	cases := []struct{ name, from, want string }{
		{"no number", "<ID>INV-1</ID>", "SV-number"},
		{"no date", "<IssueDate>2024-01-15</IssueDate>", "SV-date"},
		{"no seller", "<Name>Saeljare AB</Name>", "SV-seller"},
		{"no buyer", "<Name>Koepare AB</Name>", "SV-buyer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalSvefaktura, tc.from, "", 1)
			if broken == minimalSvefaktura {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateSvefaktura([]byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateSvefaktura([]byte(broken)))
			}
		})
	}
}
