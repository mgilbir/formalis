package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKSeFCorpus is the FP=0 oracle: every official KSeF FA sample
// (phax/phive-rules) must validate with no violations.
func TestKSeFCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/ksef/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("KSeF corpus not present (make cius-oracles)")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := IsKSeF(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			t.Errorf("%s: not recognised as KSeF", filepath.Base(f))
			continue
		}
		if v := ValidateKSeF(context.Background(), data).Violations; len(v) != 0 {
			t.Errorf("%s: expected 0 KSeF violations, got %d (first %s: %s)", filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

const minimalKSeF = `<Faktura xmlns="http://crd.gov.pl/wzor/2023/06/29/12648/">
<Naglowek><KodFormularza kodSystemowy="FA (3)" wersjaSchemy="1-0E">FA</KodFormularza><WariantFormularza>3</WariantFormularza><DataWytworzeniaFa>2024-01-15T10:00:00Z</DataWytworzeniaFa></Naglowek>
<Podmiot1><DaneIdentyfikacyjne><NIP>1234567890</NIP><Nazwa>Sprzedawca sp. z o.o.</Nazwa></DaneIdentyfikacyjne><Adres><KodKraju>PL</KodKraju><AdresL1>ul. Prosta 1</AdresL1></Adres></Podmiot1>
<Podmiot2><DaneIdentyfikacyjne><NIP>0987654321</NIP><Nazwa>Nabywca S.A.</Nazwa></DaneIdentyfikacyjne><Adres><KodKraju>PL</KodKraju><AdresL1>ul. Krzywa 2</AdresL1></Adres></Podmiot2>
<Fa><KodWaluty>PLN</KodWaluty><P_1>2024-01-15</P_1><P_2>FV/1/2024</P_2><P_15>123.00</P_15><RodzajFaktury>VAT</RodzajFaktury>
<FaWiersz><NrWierszaFa>1</NrWierszaFa><P_7>Towar</P_7><P_11>100.00</P_11></FaWiersz></Fa>
</Faktura>`

func TestKSeFMutations(t *testing.T) {
	if v := ValidateKSeF(context.Background(), []byte(minimalKSeF)).Violations; len(v) != 0 {
		t.Fatalf("baseline KSeF not clean: %v", v)
	}
	cases := []struct{ name, from, to, want string }{
		{"no form code", "<KodFormularza kodSystemowy=\"FA (3)\" wersjaSchemy=\"1-0E\">FA</KodFormularza>", "", "KS-header"},
		{"no seller nip", "<NIP>1234567890</NIP>", "", "KS-seller-nip"},
		{"no seller name", "<Nazwa>Sprzedawca sp. z o.o.</Nazwa>", "", "KS-seller-name"},
		{"no buyer", "<Podmiot2><DaneIdentyfikacyjne><NIP>0987654321</NIP><Nazwa>Nabywca S.A.</Nazwa></DaneIdentyfikacyjne><Adres><KodKraju>PL</KodKraju><AdresL1>ul. Krzywa 2</AdresL1></Adres></Podmiot2>", "", "KS-buyer"},
		{"bad currency", "<KodWaluty>PLN</KodWaluty>", "<KodWaluty>PLNN</KodWaluty>", "KS-currency"},
		{"no date", "<P_1>2024-01-15</P_1>", "", "KS-date"},
		{"no number", "<P_2>FV/1/2024</P_2>", "", "KS-number"},
		{"bad type", "<RodzajFaktury>VAT</RodzajFaktury>", "<RodzajFaktury>XYZ</RodzajFaktury>", "KS-type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalKSeF, tc.from, tc.to, 1)
			if broken == minimalKSeF {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateKSeF(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateKSeF(context.Background(), []byte(broken)).Violations)
			}
		})
	}
}
