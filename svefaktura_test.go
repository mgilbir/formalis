package formalis

import (
	"context"
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
	atLeast(t, "Svefaktura corpus", len(files), minSvefakturaFiles)
	recognised := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
			continue
		}
		ok, err := IsSvefaktura(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			continue
		}
		recognised++
		if v := findings(t, context.Background(), ValidateSvefaktura, data); len(v) != 0 {
			t.Errorf("%s: expected 0 Svefaktura violations, got %v", filepath.Base(f), v)
		}
	}
	// The transport-enveloped sample is not a direct Svefaktura document, so the
	// recognised half is smaller than the file count and is ratcheted separately.
	atLeast(t, "Svefaktura invoices recognised", recognised, minSvefakturaRecognised)
}

const minimalSvefaktura = `<Invoice xmlns="urn:sfti:documents:BasicInvoice:1:0">
<ID>INV-1</ID><IssueDate>2024-01-15</IssueDate><InvoiceCurrencyCode>SEK</InvoiceCurrencyCode>
<BuyerParty><Party><PartyName><Name>Koepare AB</Name></PartyName></Party></BuyerParty>
<SellerParty><Party><PartyName><Name>Saeljare AB</Name></PartyName></Party></SellerParty>
</Invoice>`

func TestSvefakturaMutations(t *testing.T) {
	if v := findings(t, context.Background(), ValidateSvefaktura, []byte(minimalSvefaktura)); len(v) != 0 {
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
			if !hasFacturXRule(findings(t, context.Background(), ValidateSvefaktura, []byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, findings(t, context.Background(), ValidateSvefaktura, []byte(broken)))
			}
		})
	}
}
