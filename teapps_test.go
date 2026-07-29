package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTEAPPSCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/teapps/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("TEAPPS corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		ok, err := IsTEAPPS(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			t.Errorf("%s: not recognised as TEAPPS", filepath.Base(f))
			continue
		}
		if v := ValidateTEAPPS(context.Background(), data); len(v) != 0 {
			t.Errorf("%s: expected 0 TEAPPS violations, got %v", filepath.Base(f), v)
		}
	}
}

const minimalTEAPPS = `<INVOICE_CENTER><CONTENT_FRAME><INVOICES><INVOICE>
<HEADER><INVOICE_ID>INV-1</INVOICE_ID><INVOICE_TYPE>00</INVOICE_TYPE></HEADER>
<CUSTOMER_INFORMATION><CUSTOMER_NAME>Asiakas Oy</CUSTOMER_NAME></CUSTOMER_INFORMATION>
</INVOICE></INVOICES></CONTENT_FRAME></INVOICE_CENTER>`

func TestTEAPPSMutations(t *testing.T) {
	if v := ValidateTEAPPS(context.Background(), []byte(minimalTEAPPS)); len(v) != 0 {
		t.Fatalf("baseline TEAPPS not clean: %v", v)
	}
	cases := []struct{ name, from, want string }{
		{"no invoice type", "<INVOICE_TYPE>00</INVOICE_TYPE>", "TP-type"},
		{"no customer", "<CUSTOMER_INFORMATION><CUSTOMER_NAME>Asiakas Oy</CUSTOMER_NAME></CUSTOMER_INFORMATION>", "TP-customer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalTEAPPS, tc.from, "", 1)
			if broken == minimalTEAPPS {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateTEAPPS(context.Background(), []byte(broken)), tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateTEAPPS(context.Background(), []byte(broken)))
			}
		})
	}
}
