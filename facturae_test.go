package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFacturaeCorpus is the FP=0 oracle: every official Facturae sample
// (phax/phive-rules) must validate with no violations.
func TestFacturaeCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/facturae/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("Facturae corpus not present (make cius-oracles)")
	}
	atLeast(t, "Facturae corpus", len(files), minFacturaeInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := IsFacturae(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			t.Errorf("%s: not recognised as Facturae", filepath.Base(f))
			continue
		}
		if v := ValidateFacturae(context.Background(), data).Violations; len(v) != 0 {
			t.Errorf("%s: expected 0 Facturae violations, got %d (first %s: %s)", filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

const minimalFacturae = `<fe:Facturae xmlns:fe="http://www.facturae.es/Facturae/2014/v3.2.1/Facturae">
<FileHeader><SchemaVersion>3.2.1</SchemaVersion><Modality>I</Modality><InvoiceIssuerType>EM</InvoiceIssuerType>
 <Batch><BatchIdentifier>B1</BatchIdentifier><InvoicesCount>1</InvoicesCount><TotalInvoicesAmount><TotalAmount>122.00</TotalAmount></TotalInvoicesAmount><TotalOutstandingAmount><TotalAmount>122.00</TotalAmount></TotalOutstandingAmount><TotalExecutableAmount><TotalAmount>122.00</TotalAmount></TotalExecutableAmount><InvoiceCurrencyCode>EUR</InvoiceCurrencyCode></Batch>
</FileHeader>
<Parties>
 <SellerParty><TaxIdentification><PersonTypeCode>J</PersonTypeCode><ResidenceTypeCode>R</ResidenceTypeCode><TaxIdentificationNumber>A12345678</TaxIdentificationNumber></TaxIdentification><LegalEntity><CorporateName>Vendedor SL</CorporateName><AddressInSpain><Address>Calle Uno 1</Address><PostCode>28001</PostCode><Town>Madrid</Town><Province>Madrid</Province><CountryCode>ESP</CountryCode></AddressInSpain></LegalEntity></SellerParty>
 <BuyerParty><TaxIdentification><PersonTypeCode>J</PersonTypeCode><ResidenceTypeCode>R</ResidenceTypeCode><TaxIdentificationNumber>B87654321</TaxIdentificationNumber></TaxIdentification><LegalEntity><CorporateName>Comprador SL</CorporateName><AddressInSpain><Address>Calle Dos 2</Address><PostCode>08001</PostCode><Town>Barcelona</Town><Province>Barcelona</Province><CountryCode>ESP</CountryCode></AddressInSpain></LegalEntity></BuyerParty>
</Parties>
<Invoices><Invoice><InvoiceHeader><InvoiceNumber>INV-1</InvoiceNumber><InvoiceDocumentType>FC</InvoiceDocumentType><InvoiceClass>OO</InvoiceClass></InvoiceHeader><InvoiceIssueData><IssueDate>2024-01-15</IssueDate></InvoiceIssueData></Invoice></Invoices>
</fe:Facturae>`

func TestFacturaeMutations(t *testing.T) {
	if v := ValidateFacturae(context.Background(), []byte(minimalFacturae)).Violations; len(v) != 0 {
		t.Fatalf("baseline Facturae not clean: %d (first %s: %s)", len(v), v[0].Rule, v[0].Message)
	}
	cases := []struct{ name, from, to, want string }{
		{"bad modality", "<Modality>I</Modality>", "<Modality>X</Modality>", "FE-header"},
		{"bad currency", "<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>", "<InvoiceCurrencyCode>EURO</InvoiceCurrencyCode>", "FE-currency"},
		{"no seller id", "<TaxIdentificationNumber>A12345678</TaxIdentificationNumber>", "", "FE-seller-id"},
		{"no seller name", "<CorporateName>Vendedor SL</CorporateName>", "", "FE-seller-name"},
		{"no seller address", "<Address>Calle Uno 1</Address>", "", "FE-seller-address"},
		{"no buyer id", "<TaxIdentificationNumber>B87654321</TaxIdentificationNumber>", "", "FE-buyer-id"},
		{"bad invoice type", "<InvoiceDocumentType>FC</InvoiceDocumentType>", "<InvoiceDocumentType>FZ</InvoiceDocumentType>", "FE-invoice-type"},
		{"bad invoice class", "<InvoiceClass>OO</InvoiceClass>", "<InvoiceClass>ZZ</InvoiceClass>", "FE-invoice-class"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalFacturae, tc.from, tc.to, 1)
			if broken == minimalFacturae {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateFacturae(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateFacturae(context.Background(), []byte(broken)).Violations)
			}
		})
	}
}
