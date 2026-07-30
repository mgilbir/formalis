package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func roRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "BR-RO-") {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestCIUSROCorpus is the FP=0 oracle over ANAF's own sample instances, with one
// documented exception that the artefact forced and that this test re-derives
// rather than trusts.
//
// The oracle used to read "0 BR-RO violations on a conformant sample" and it
// passed, because BR-RO-110/111/170/212 were implemented more permissively than
// ANAF publishes them: roValidSubdivision accepted a Bucharest sector ("Sector 5")
// where ISO 3166-2:RO expects a county code, on the argument that real Romanian
// invoices are written that way. They are — but ANAF's own Schematron reports
// BR-RO-111 for exactly that, so the assertion was pinning a false negative in
// place. It encoded a bug.
//
// Seven of the eleven samples, in the releases that carry them, write a sector in
// the buyer's BT-54 and four also in the tax representative's BT-68. ANAF fixed
// the same file upstream between 1.0.4 and 1.0.8 — example0's buyer went from
// "Sector 5" to "RO-B"/"SECTOR1" — which is the authority agreeing that the sector
// form was the defect.
//
// So the exception is narrow and checked: only BR-RO-110/111/170 may be reported,
// and only for a document that really does write a sector where the code belongs.
// Every other BR-RO finding is still a failure, and both counts are ratcheted, so
// neither a new false positive nor a rule that stopped firing can hide here.
func TestCIUSROCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-ro/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("CIUS-RO corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-RO corpus", len(files), minCIUSROInstances)
	sectorRule := map[string]bool{"BR-RO-110": true, "BR-RO-111": true, "BR-RO-170": true}
	sectorFindings, sectorDocs := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Re-derive the excuse from the document: does it write a Bucharest sector
		// in a country-subdivision element at all? A file that does not gets no
		// exception.
		writesSector := false
		for _, v := range subdivisionValues(t, data) {
			if roSectorSubdivision(v) {
				writesSector = true
			}
		}
		var unexpected []string
		hit := 0
		for _, r := range roRuleViolations(findings(t, context.Background(), ValidateCIUSRO, data)) {
			if writesSector && sectorRule[r] {
				hit++
				continue
			}
			unexpected = append(unexpected, r)
		}
		if len(unexpected) != 0 {
			t.Errorf("%s: expected 0 CIUS-RO violations on a conformant sample, got %v", filepath.Base(f), unexpected)
		}
		sectorFindings += hit
		if hit > 0 {
			sectorDocs++
		}
	}
	atLeast(t, "CIUS-RO samples writing a Bucharest sector as the subdivision", sectorDocs, minCIUSROSectorDocs)
	atLeast(t, "CIUS-RO subdivision findings on those samples", sectorFindings, minCIUSROSectorFindings)
	t.Logf("CIUS-RO: %d of %d samples write a Bucharest sector where ISO 3166-2:RO belongs, for %d findings; "+
		"every other sample is clean of BR-RO rules", sectorDocs, len(files), sectorFindings)
}

// subdivisionValues returns every cbc:CountrySubentity value in a UBL document, so
// the exception above is derived from the file rather than from a list of names.
func subdivisionValues(t *testing.T, data []byte) []string {
	t.Helper()
	root, err := parseCII(newRun(context.Background()), data)
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range root.findAll("CountrySubentity") {
		out = append(out, strings.TrimSpace(n.text))
	}
	return out
}

// minimalCIUSROUBL is a small CIUS-RO-conformant invoice carrying the mandatory
// address terms (seller, buyer, tax representative and delivery), in RON, with
// distinct values so each can be removed in isolation.
const minimalCIUSROUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RON</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:CountrySubentity>RO-CJ</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller SRL</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:CountrySubentity>RO-TM</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer SRL</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxRepresentativeParty><cac:PostalAddress><cbc:StreetName>RepStreet</cbc:StreetName><cbc:CityName>RepCity</cbc:CityName><cbc:CountrySubentity>RO-IS</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:TaxRepresentativeParty>
<cac:Delivery><cac:DeliveryLocation><cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:CountrySubentity>RO-BV</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>
<cac:TaxTotal><cbc:TaxAmount>19.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>19.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>119.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>119.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

var ciusROMutations = []ciusMutation{
	{"number without digit (010)", "<cbc:ID>INV-1</cbc:ID>", "<cbc:ID>INVOICE</cbc:ID>", "BR-RO-010"},
	{"bad invoice type code (020_1)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>100</cbc:InvoiceTypeCode>", "BR-RO-020_1"},
	{"non-RON without RON tax currency (030)", "<cbc:DocumentCurrencyCode>RON</cbc:DocumentCurrencyCode>", "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>", "BR-RO-030"},
	{"no seller street (081)", "<cbc:StreetName>SellerStreet</cbc:StreetName>", "", "BR-RO-081"},
	{"no seller city (091)", "<cbc:CityName>SellerCity</cbc:CityName>", "", "BR-RO-091"},
	{"no buyer street (082)", "<cbc:StreetName>BuyerStreet</cbc:StreetName>", "", "BR-RO-082"},
	{"no buyer city (092)", "<cbc:CityName>BuyerCity</cbc:CityName>", "", "BR-RO-092"},
	{"no tax rep street (140)", "<cbc:StreetName>RepStreet</cbc:StreetName>", "", "BR-RO-140"},
	{"no tax rep city (150)", "<cbc:CityName>RepCity</cbc:CityName>", "", "BR-RO-150"},
	{"no delivery street (180)", "<cbc:StreetName>DelivStreet</cbc:StreetName>", "", "BR-RO-180"},
	{"no delivery city (201)", "<cbc:CityName>DelivCity</cbc:CityName>", "", "BR-RO-201"},
	{"no seller subdivision (110)", "<cbc:CountrySubentity>RO-CJ</cbc:CountrySubentity>", "", "BR-RO-110"},
	{"no buyer subdivision (111)", "<cbc:CountrySubentity>RO-TM</cbc:CountrySubentity>", "", "BR-RO-111"},
	{"invalid tax rep subdivision (170)", "<cbc:CountrySubentity>RO-IS</cbc:CountrySubentity>", "<cbc:CountrySubentity>XX</cbc:CountrySubentity>", "BR-RO-170"},
	{"no delivery subdivision (211)", "<cbc:CountrySubentity>RO-BV</cbc:CountrySubentity>", "", "BR-RO-211"},
	{"invalid delivery subdivision (212)", "<cbc:CountrySubentity>RO-BV</cbc:CountrySubentity>", "<cbc:CountrySubentity>XX</cbc:CountrySubentity>", "BR-RO-212"},
	// The four Bucharest rules. RO-B is a valid ISO 3166-2:RO code, so moving a
	// party to it satisfies BR-RO-110/111/170/212 and leaves only the sector-city
	// requirement to fail — which is what isolates these four.
	{"Bucharest seller without a sector city (100)", "<cbc:CountrySubentity>RO-CJ</cbc:CountrySubentity>", "<cbc:CountrySubentity>RO-B</cbc:CountrySubentity>", "BR-RO-100"},
	{"Bucharest buyer without a sector city (101)", "<cbc:CountrySubentity>RO-TM</cbc:CountrySubentity>", "<cbc:CountrySubentity>RO-B</cbc:CountrySubentity>", "BR-RO-101"},
	{"Bucharest tax representative without a sector city (160)", "<cbc:CountrySubentity>RO-IS</cbc:CountrySubentity>", "<cbc:CountrySubentity>RO-B</cbc:CountrySubentity>", "BR-RO-160"},
	{"Bucharest delivery without a sector city (202)", "<cbc:CountrySubentity>RO-BV</cbc:CountrySubentity>", "<cbc:CountrySubentity>RO-B</cbc:CountrySubentity>", "BR-RO-202"},
}

func TestCIUSROMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[1])
}

// minimalCIUSROCreditNote is the credit-note half of the type-code rule. ANAF binds
// BR-RO-020_1 to cbc:InvoiceTypeCode and BR-RO-020_2 to cbc:CreditNoteTypeCode, so
// the second is unreachable from an Invoice baseline however it is mutated — which
// is also why "BR-RO-020" could stand for both for as long as it did.
const minimalCIUSROCreditNote = `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>CN-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:CreditNoteTypeCode>380</cbc:CreditNoteTypeCode><cbc:DocumentCurrencyCode>RON</cbc:DocumentCurrencyCode>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:CountrySubentity>RO-CJ</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller SRL</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:CountrySubentity>RO-TM</cbc:CountrySubentity><cac:Country><cbc:IdentificationCode>RO</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer SRL</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxTotal><cbc:TaxAmount>19.00</cbc:TaxAmount></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:PayableAmount>119.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
</CreditNote>`

var ciusROExtras = []ciusDoc{
	{"credit note with an invoice type code (020_2)", minimalCIUSROCreditNote, "BR-RO-020_2"},
}
