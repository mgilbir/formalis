package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinvoiceCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/finvoice/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("Finvoice corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := IsFinvoice(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			t.Errorf("%s: not recognised as Finvoice", filepath.Base(f))
			continue
		}
		if v := ValidateFinvoice(context.Background(), data).Violations; len(v) != 0 {
			t.Errorf("%s: expected 0 Finvoice violations, got %d (first %s: %s)", filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

const minimalFinvoice = `<Finvoice Version="3.0">
<SellerPartyDetails><SellerOrganisationName>Myyja Oy</SellerOrganisationName><SellerPostalAddressDetails><SellerStreetName>Katu 1</SellerStreetName></SellerPostalAddressDetails></SellerPartyDetails>
<BuyerPartyDetails><BuyerOrganisationName>Ostaja Oy</BuyerOrganisationName></BuyerPartyDetails>
<InvoiceDetails><InvoiceTypeCode>INV01</InvoiceTypeCode><InvoiceNumber>INV-1</InvoiceNumber><InvoiceDate Format="CCYYMMDD">20240115</InvoiceDate></InvoiceDetails>
</Finvoice>`

func TestFinvoiceMutations(t *testing.T) {
	if v := ValidateFinvoice(context.Background(), []byte(minimalFinvoice)).Violations; len(v) != 0 {
		t.Fatalf("baseline Finvoice not clean: %v", v)
	}
	cases := []struct{ name, from, want string }{
		{"no seller name", "<SellerOrganisationName>Myyja Oy</SellerOrganisationName>", "FI-seller-name"},
		{"no seller address", "<SellerPostalAddressDetails><SellerStreetName>Katu 1</SellerStreetName></SellerPostalAddressDetails>", "FI-seller-address"},
		{"no buyer name", "<BuyerOrganisationName>Ostaja Oy</BuyerOrganisationName>", "FI-buyer-name"},
		{"no type", "<InvoiceTypeCode>INV01</InvoiceTypeCode>", "FI-type"},
		{"no number", "<InvoiceNumber>INV-1</InvoiceNumber>", "FI-number"},
		{"no date", "<InvoiceDate Format=\"CCYYMMDD\">20240115</InvoiceDate>", "FI-date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalFinvoice, tc.from, "", 1)
			if broken == minimalFinvoice {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateFinvoice(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateFinvoice(context.Background(), []byte(broken)).Violations)
			}
		})
	}
}
