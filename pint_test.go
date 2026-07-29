package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPINTCorpus is the FP=0 oracle across every PINT jurisdiction in the sample
// set (phax/phive-rules).
func TestPINTCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/pint/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("PINT corpus not present (make cius-oracles)")
	}
	seen := map[string]bool{}
	recognised := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
			continue
		}
		ok, err := IsPINT(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			continue
		}
		recognised++
		root, _ := parseCII(newRun(nil), data)
		seen[DetectPINTJurisdiction(root.str("CustomizationID"))] = true
		if v := ValidatePINT(context.Background(), data).Violations; len(v) != 0 {
			t.Errorf("%s: expected 0 PINT violations, got %v", filepath.Base(f), v)
		}
	}
	atLeast(t, "PINT corpus", recognised, minPINTInstances)
	if len(seen) < 2 {
		t.Errorf("expected multiple PINT jurisdictions in the corpus, saw %v", seen)
	}
}

func TestDetectPINTJurisdiction(t *testing.T) {
	for id, want := range map[string]string{
		"urn:peppol:pint:billing-1@eu-1":   "eu",
		"urn:peppol:pint:billing-1@ae-1":   "ae",
		"urn:peppol:pint:billing-1@jp-1":   "jp",
		"urn:peppol:pint:billing-1@aunz-1": "aunz",
		"urn:cen.eu:en16931:2017":          "",
	} {
		if got := DetectPINTJurisdiction(id); got != want {
			t.Errorf("DetectPINTJurisdiction(%q) = %q, want %q", id, got, want)
		}
	}
}

const minimalPINT = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:peppol:pint:billing-1@eu-1</cbc:CustomizationID><cbc:ProfileID>urn:peppol:bis:billing</cbc:ProfileID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party><cbc:EndpointID schemeID="0088">1234567890123</cbc:EndpointID><cac:PartyLegalEntity><cbc:RegistrationName>Seller Ltd</cbc:RegistrationName></cac:PartyLegalEntity></cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party><cbc:EndpointID schemeID="0088">3210987654321</cbc:EndpointID><cac:PartyLegalEntity><cbc:RegistrationName>Buyer Ltd</cbc:RegistrationName></cac:PartyLegalEntity></cac:Party></cac:AccountingCustomerParty>
</Invoice>`

func TestPINTMutations(t *testing.T) {
	if v := ValidatePINT(context.Background(), []byte(minimalPINT)).Violations; len(v) != 0 {
		t.Fatalf("baseline PINT not clean: %v", v)
	}
	cases := []struct{ name, from, to, want string }{
		{"no customization", "<cbc:CustomizationID>urn:peppol:pint:billing-1@eu-1</cbc:CustomizationID>", "<cbc:CustomizationID>foo</cbc:CustomizationID>", "PINT-customization"},
		{"no profile", "<cbc:ProfileID>urn:peppol:bis:billing</cbc:ProfileID>", "", "PINT-profile"},
		{"no number", "<cbc:ID>INV-1</cbc:ID>", "", "PINT-number"},
		{"no date", "<cbc:IssueDate>2024-01-15</cbc:IssueDate>", "", "PINT-date"},
		{"no type", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "", "PINT-type"},
		{"bad currency", "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>", "<cbc:DocumentCurrencyCode>EURO</cbc:DocumentCurrencyCode>", "PINT-currency"},
		{"no seller endpoint", "<cbc:EndpointID schemeID=\"0088\">1234567890123</cbc:EndpointID>", "", "PINT-seller-endpoint"},
		{"no buyer endpoint", "<cbc:EndpointID schemeID=\"0088\">3210987654321</cbc:EndpointID>", "", "PINT-buyer-endpoint"},
		{"no seller name", "<cbc:RegistrationName>Seller Ltd</cbc:RegistrationName>", "", "PINT-seller-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalPINT, tc.from, tc.to, 1)
			if broken == minimalPINT {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidatePINT(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidatePINT(context.Background(), []byte(broken)).Violations)
			}
		})
	}
}
