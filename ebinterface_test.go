package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEbInterfaceCorpus is the FP=0 oracle across every ebInterface schema
// version in the official sample set (phax/phive-rules).
func TestEbInterfaceCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/ebinterface/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("ebInterface corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !IsEbInterface(data) {
			t.Errorf("%s: not recognised as ebInterface", filepath.Base(f))
			continue
		}
		if v := ValidateEbInterface(context.Background(), data); len(v) != 0 {
			t.Errorf("%s: expected 0 ebInterface violations, got %d (first %s: %s)", filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

const minimalEbInterface = `<eb:Invoice xmlns:eb="http://www.ebinterface.at/schema/6p1/">
<eb:InvoiceNumber>INV-1</eb:InvoiceNumber><eb:InvoiceDate>2024-01-15</eb:InvoiceDate>
<eb:Biller><eb:VATIdentificationNumber>ATU12345678</eb:VATIdentificationNumber><eb:Address><eb:Name>Verkaeufer GmbH</eb:Name><eb:Street>Hauptstrasse 1</eb:Street><eb:Town>Wien</eb:Town><eb:ZIP>1010</eb:ZIP><eb:Country>AT</eb:Country></eb:Address></eb:Biller>
<eb:InvoiceRecipient><eb:VATIdentificationNumber>ATU87654321</eb:VATIdentificationNumber><eb:Address><eb:Name>Kaeufer GmbH</eb:Name><eb:Street>Nebenstrasse 2</eb:Street><eb:Town>Graz</eb:Town><eb:ZIP>8010</eb:ZIP><eb:Country>AT</eb:Country></eb:Address></eb:InvoiceRecipient>
</eb:Invoice>`

func TestEbInterfaceMutations(t *testing.T) {
	if v := ValidateEbInterface(context.Background(), []byte(minimalEbInterface)); len(v) != 0 {
		t.Fatalf("baseline ebInterface not clean: %v", v)
	}
	cases := []struct{ name, from, to, want string }{
		{"no number", "<eb:InvoiceNumber>INV-1</eb:InvoiceNumber>", "", "EB-number"},
		{"no date", "<eb:InvoiceDate>2024-01-15</eb:InvoiceDate>", "", "EB-date"},
		{"no biller vat", "<eb:VATIdentificationNumber>ATU12345678</eb:VATIdentificationNumber>", "", "EB-biller-vat"},
		{"no biller name", "<eb:Name>Verkaeufer GmbH</eb:Name>", "", "EB-biller-name"},
		{"no recipient name", "<eb:Name>Kaeufer GmbH</eb:Name>", "", "EB-recipient-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalEbInterface, tc.from, tc.to, 1)
			if broken == minimalEbInterface {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateEbInterface(context.Background(), []byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateEbInterface(context.Background(), []byte(broken)))
			}
		})
	}
}
