package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOIOUBLCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/oioubl/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("OIOUBL corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		ok, err := IsOIOUBL(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			continue
		}
		if v := ValidateOIOUBL(context.Background(), data); len(v) != 0 {
			t.Errorf("%s: expected 0 OIOUBL violations, got %v", filepath.Base(f), v)
		}
	}
}

const minimalOIOUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>OIOUBL-2.02</cbc:CustomizationID><cbc:ProfileID>Procurement-BilSim-1.0</cbc:ProfileID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>DKK</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party><cbc:EndpointID schemeID="DK:CVR">DK12345678</cbc:EndpointID><cac:PartyName><cbc:Name>Saelger A/S</cbc:Name></cac:PartyName></cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party><cbc:EndpointID schemeID="DK:CVR">DK87654321</cbc:EndpointID><cac:PartyName><cbc:Name>Koeber A/S</cbc:Name></cac:PartyName></cac:Party></cac:AccountingCustomerParty>
</Invoice>`

func TestOIOUBLMutations(t *testing.T) {
	if v := ValidateOIOUBL(context.Background(), []byte(minimalOIOUBL)); len(v) != 0 {
		t.Fatalf("baseline OIOUBL not clean: %v", v)
	}
	cases := []struct{ name, from, to, want string }{
		{"no customization", "<cbc:CustomizationID>OIOUBL-2.02</cbc:CustomizationID>", "<cbc:CustomizationID>FOO</cbc:CustomizationID>", "OIO-customization"},
		{"no number", "<cbc:ID>INV-1</cbc:ID>", "", "OIO-number"},
		{"no date", "<cbc:IssueDate>2024-01-15</cbc:IssueDate>", "", "OIO-date"},
		{"no type", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "", "OIO-type"},
		{"bad currency", "<cbc:DocumentCurrencyCode>DKK</cbc:DocumentCurrencyCode>", "<cbc:DocumentCurrencyCode>DKKK</cbc:DocumentCurrencyCode>", "OIO-currency"},
		{"no seller endpoint", "<cbc:EndpointID schemeID=\"DK:CVR\">DK12345678</cbc:EndpointID>", "", "OIO-seller-endpoint"},
		{"no buyer endpoint", "<cbc:EndpointID schemeID=\"DK:CVR\">DK87654321</cbc:EndpointID>", "", "OIO-buyer-endpoint"},
		{"no seller name", "<cbc:Name>Saelger A/S</cbc:Name>", "", "OIO-seller-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalOIOUBL, tc.from, tc.to, 1)
			if broken == minimalOIOUBL {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateOIOUBL(context.Background(), []byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateOIOUBL(context.Background(), []byte(broken)))
			}
		})
	}
}
