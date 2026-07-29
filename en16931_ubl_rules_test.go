package formalis

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Per-rule tests for the 54 fatal UBL-SR-* rules of CEN's UBL syntax binding.
//
// The CEN unit-test suite is not a substitute for these. Of the 54 rules it
// ships an <error> fragment for five — UBL-SR-12, 18, 42, 44 and 47 — and a
// <success>-only fragment for one more, UBL-SR-43. The other forty-eight are
// invisible to TestEN16931ConformanceSuite in both directions: it would neither
// notice a rule that never fires nor a rule that fires on a conforming invoice.
// The corpus is a stronger oracle but a partial one — it exercises the shapes
// real producers emit, which is by construction the conforming half.
//
// So every rule gets both verdicts: a document that satisfies it, on which it
// must stay silent, and a document that breaks it, on which it must fire. The
// two together are what says the rule is a rule rather than a constant.
//
// Each case asserts about its own rule and no other, for the reason
// en16931_core_rules_test.go gives: a fixture with three party tax schemes
// breaks UBL-SR-42 and UBL-SR-13 at once, and pinning the whole finding set
// would make each case fail for the other's reasons.

// The insertion points in minimalUBL. The fixture is one clean EN 16931 invoice
// and every case below is that invoice plus one thing, so what a case is about
// is the string it inserts.
const (
	ublAtDocument = `<ID>INV-1</ID>`
	ublAtSeller   = `<PartyLegalEntity><RegistrationName>Seller Ltd</RegistrationName></PartyLegalEntity>`
	ublAtBuyer    = `<PartyLegalEntity><RegistrationName>Buyer Ltd</RegistrationName></PartyLegalEntity>`
	ublAtAddress  = `<PostalAddress>` // the first one, which is the seller's
	ublAtLine     = `<InvoiceLine><ID>1</ID>`
	ublAtItem     = `<Item><Name>Widget</Name>`
	ublAtPrice    = `<Price>`
	ublAtCategory = `<TaxCategory><ID>S</ID><Percent>19</Percent>`
)

// ublWith is minimalUBL with x inserted immediately after anchor.
func ublWith(t *testing.T, anchor, x string) string {
	t.Helper()
	return mutate(t, minimalUBL, anchor, anchor+x)
}

// ublCreditNote rewrites minimalUBL as the UBL CreditNote of the same invoice.
// Only UBL-SR-43 distinguishes the two roots, and it is the only rule that needs
// this.
func ublCreditNote(s string) string {
	for _, r := range [][2]string{
		{`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2">`,
			`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2">`},
		{`</Invoice>`, `</CreditNote>`},
		{`InvoiceTypeCode>380<`, `CreditNoteTypeCode>381<`},
		{`</InvoiceTypeCode>`, `</CreditNoteTypeCode>`},
		{`InvoiceLine>`, `CreditNoteLine>`},
		{`InvoicedQuantity`, `CreditedQuantity`},
	} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return s
}

// The party tax scheme fragments the cardinality cases repeat. A complete one
// carries both halves UBL-SR-53 requires.
const (
	ublVATScheme   = `<PartyTaxScheme><CompanyID>DE987654321</CompanyID><TaxScheme><ID>VAT</ID></TaxScheme></PartyTaxScheme>`
	ublOtherScheme = `<PartyTaxScheme><CompanyID>123/456/789</CompanyID><TaxScheme><ID>FC</ID></TaxScheme></PartyTaxScheme>`
)

// TestUBLSyntaxRules is the per-rule table. Every fatal UBL-SR-* rule appears
// twice, once with want=false and once with want=true.
func TestUBLSyntaxRules(t *testing.T) {
	// The fixture every case is built from must itself be clean, or a case that
	// expects silence would be asserting nothing.
	if v := Validate(context.Background(), []byte(minimalUBL), ProfileEN16931).Violations; len(v) != 0 {
		t.Fatalf("baseline UBL not clean: %d violations (first %s: %s)", len(v), v[0].Rule, v[0].Message)
	}

	cases := []ruleCase{
		// --- Document-element cardinality (context: /ubl:Invoice | /cn:CreditNote) ---
		{"UBL-SR-01 one contract reference", ublWith(t, ublAtDocument,
			`<ContractDocumentReference><ID>C-1</ID></ContractDocumentReference>`), "UBL-SR-01", false},
		{"UBL-SR-01 two contract references", ublWith(t, ublAtDocument,
			`<ContractDocumentReference><ID>C-1</ID></ContractDocumentReference>`+
				`<ContractDocumentReference><ID>C-2</ID></ContractDocumentReference>`), "UBL-SR-01", true},

		{"UBL-SR-02 one receipt advice reference", ublWith(t, ublAtDocument,
			`<ReceiptDocumentReference><ID>R-1</ID></ReceiptDocumentReference>`), "UBL-SR-02", false},
		{"UBL-SR-02 two receipt advice references", ublWith(t, ublAtDocument,
			`<ReceiptDocumentReference><ID>R-1</ID></ReceiptDocumentReference>`+
				`<ReceiptDocumentReference><ID>R-2</ID></ReceiptDocumentReference>`), "UBL-SR-02", true},

		{"UBL-SR-03 one despatch advice reference", ublWith(t, ublAtDocument,
			`<DespatchDocumentReference><ID>D-1</ID></DespatchDocumentReference>`), "UBL-SR-03", false},
		{"UBL-SR-03 two despatch advice references", ublWith(t, ublAtDocument,
			`<DespatchDocumentReference><ID>D-1</ID></DespatchDocumentReference>`+
				`<DespatchDocumentReference><ID>D-2</ID></DespatchDocumentReference>`), "UBL-SR-03", true},

		{"UBL-SR-04 one invoiced object identifier", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>130</DocumentTypeCode></AdditionalDocumentReference>`), "UBL-SR-04", false},
		{"UBL-SR-04 two invoiced object identifiers", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>130</DocumentTypeCode></AdditionalDocumentReference>`+
				`<AdditionalDocumentReference><ID schemeID="AAB">OBJ-2</ID><DocumentTypeCode>130</DocumentTypeCode></AdditionalDocumentReference>`), "UBL-SR-04", true},

		{"UBL-SR-05 one payment terms note", ublWith(t, ublAtDocument,
			`<PaymentTerms><Note>Net 30 days</Note></PaymentTerms>`), "UBL-SR-05", false},
		{"UBL-SR-05 two payment terms notes", ublWith(t, ublAtDocument,
			`<PaymentTerms><Note>Net 30 days</Note><Note>2% within 10 days</Note></PaymentTerms>`), "UBL-SR-05", true},

		{"UBL-SR-08 one invoicing period", ublWith(t, ublAtDocument,
			`<InvoicePeriod><StartDate>2024-01-01</StartDate><EndDate>2024-01-31</EndDate></InvoicePeriod>`), "UBL-SR-08", false},
		{"UBL-SR-08 two invoicing periods", ublWith(t, ublAtDocument,
			`<InvoicePeriod><StartDate>2024-01-01</StartDate><EndDate>2024-01-31</EndDate></InvoicePeriod>`+
				`<InvoicePeriod><StartDate>2024-02-01</StartDate><EndDate>2024-02-29</EndDate></InvoicePeriod>`), "UBL-SR-08", true},

		{"UBL-SR-24 one deliver-to group", ublWith(t, ublAtDocument,
			`<Delivery><ActualDeliveryDate>2024-01-10</ActualDeliveryDate></Delivery>`), "UBL-SR-24", false},
		{"UBL-SR-24 two deliver-to groups", ublWith(t, ublAtDocument,
			`<Delivery><ActualDeliveryDate>2024-01-10</ActualDeliveryDate></Delivery>`+
				`<Delivery><ActualDeliveryDate>2024-01-11</ActualDeliveryDate></Delivery>`), "UBL-SR-24", true},

		{"UBL-SR-29 one SEPA creditor identifier", ublWith(t, ublAtSeller,
			`<PartyIdentification><ID schemeID="SEPA">DE98ZZZ09999999999</ID></PartyIdentification>`), "UBL-SR-29", false},
		{"UBL-SR-29 a SEPA creditor identifier on the seller and on the payee", ublWith(t,
			ublAtSeller, `<PartyIdentification><ID schemeID="SEPA">DE98ZZZ09999999999</ID></PartyIdentification>`+
				`</Party></AccountingSupplierParty><PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName>`+
				`<PartyIdentification><ID schemeID="SEPA">DE98ZZZ08888888888</ID></PartyIdentification></PayeeParty>`+
				`<AccountingSupplierParty><Party>`), "UBL-SR-29", true},

		{"UBL-SR-39 one project reference", ublWith(t, ublAtDocument,
			`<ProjectReference><ID>PRJ-1</ID></ProjectReference>`), "UBL-SR-39", false},
		{"UBL-SR-39 two project references", ublWith(t, ublAtDocument,
			`<ProjectReference><ID>PRJ-1</ID></ProjectReference><ProjectReference><ID>PRJ-2</ID></ProjectReference>`), "UBL-SR-39", true},

		{"UBL-SR-44 one payment reference repeated across payment means", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentID>REF-1</PaymentID></PaymentMeans>`+
				`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentID>REF-1</PaymentID></PaymentMeans>`), "UBL-SR-44", false},
		{"UBL-SR-44 two different payment references", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentID>REF-1</PaymentID></PaymentMeans>`+
				`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentID>REF-2</PaymentID></PaymentMeans>`), "UBL-SR-44", true},

		{"UBL-SR-45 one payment due date", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentDueDate>2024-02-15</PaymentDueDate></PaymentMeans>`), "UBL-SR-45", false},
		{"UBL-SR-45 two payment due dates", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentDueDate>2024-02-15</PaymentDueDate></PaymentMeans>`+
				`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentDueDate>2024-03-15</PaymentDueDate></PaymentMeans>`), "UBL-SR-45", true},

		{"UBL-SR-46 one payment means text", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode name="Credit transfer">30</PaymentMeansCode></PaymentMeans>`), "UBL-SR-46", false},
		{"UBL-SR-46 two payment means texts", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode name="Credit transfer">30</PaymentMeansCode></PaymentMeans>`+
				`<PaymentMeans><PaymentMeansCode name="Bank transfer">30</PaymentMeansCode></PaymentMeans>`), "UBL-SR-46", true},

		{"UBL-SR-47 two payment means groups agreeing on the code", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>`+
				`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>`), "UBL-SR-47", false},
		{"UBL-SR-47 two payment means groups disagreeing on the code", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>`+
				`<PaymentMeans><PaymentMeansCode>58</PaymentMeansCode></PaymentMeans>`), "UBL-SR-47", true},

		{"UBL-SR-49 one tax point date code", ublWith(t, ublAtDocument,
			`<InvoicePeriod><DescriptionCode>3</DescriptionCode></InvoicePeriod>`), "UBL-SR-49", false},
		{"UBL-SR-49 two tax point date codes", ublWith(t, ublAtDocument,
			`<InvoicePeriod><DescriptionCode>3</DescriptionCode><DescriptionCode>35</DescriptionCode></InvoicePeriod>`), "UBL-SR-49", true},

		{"UBL-SR-54 one payment card account", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>48</PaymentMeansCode><CardAccount><PrimaryAccountNumberID>1234</PrimaryAccountNumberID></CardAccount></PaymentMeans>`), "UBL-SR-54", false},
		{"UBL-SR-54 two payment card accounts", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>48</PaymentMeansCode>`+
				`<CardAccount><PrimaryAccountNumberID>1234</PrimaryAccountNumberID></CardAccount>`+
				`<CardAccount><PrimaryAccountNumberID>5678</PrimaryAccountNumberID></CardAccount></PaymentMeans>`), "UBL-SR-54", true},

		{"UBL-SR-55 one direct debit mandate", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>59</PaymentMeansCode><PaymentMandate><ID>MND-1</ID></PaymentMandate></PaymentMeans>`), "UBL-SR-55", false},
		{"UBL-SR-55 two direct debit mandates", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>59</PaymentMeansCode>`+
				`<PaymentMandate><ID>MND-1</ID></PaymentMandate><PaymentMandate><ID>MND-2</ID></PaymentMandate></PaymentMeans>`), "UBL-SR-55", true},

		{"UBL-SR-56 one tender or lot reference", ublWith(t, ublAtDocument,
			`<OriginatorDocumentReference><ID>LOT-1</ID></OriginatorDocumentReference>`), "UBL-SR-56", false},
		{"UBL-SR-56 two tender or lot references", ublWith(t, ublAtDocument,
			`<OriginatorDocumentReference><ID>LOT-1</ID></OriginatorDocumentReference>`+
				`<OriginatorDocumentReference><ID>LOT-2</ID></OriginatorDocumentReference>`), "UBL-SR-56", true},

		// --- Parties, addresses and tax schemes ---------------------------
		{"UBL-SR-09 one seller name", minimalUBL, "UBL-SR-09", false},
		{"UBL-SR-09 two seller names", ublWith(t, ublAtSeller,
			`<PartyLegalEntity><RegistrationName>Seller Ltd (trading)</RegistrationName></PartyLegalEntity>`), "UBL-SR-09", true},

		{"UBL-SR-10 one seller trading name", ublWith(t, ublAtSeller,
			`<PartyName><Name>SellerCo</Name></PartyName>`), "UBL-SR-10", false},
		{"UBL-SR-10 two seller trading names", ublWith(t, ublAtSeller,
			`<PartyName><Name>SellerCo</Name></PartyName><PartyName><Name>SellCo</Name></PartyName>`), "UBL-SR-10", true},

		{"UBL-SR-11 one seller legal registration identifier", ublWith(t, ublAtSeller,
			`<PartyLegalEntity><CompanyID>HRB 1234</CompanyID></PartyLegalEntity>`), "UBL-SR-11", false},
		{"UBL-SR-11 two seller legal registration identifiers", ublWith(t, ublAtSeller,
			`<PartyLegalEntity><CompanyID>HRB 1234</CompanyID><CompanyID>HRB 5678</CompanyID></PartyLegalEntity>`), "UBL-SR-11", true},

		{"UBL-SR-12 one seller VAT identifier", minimalUBL, "UBL-SR-12", false},
		{"UBL-SR-12 two seller VAT identifiers", ublWith(t, ublAtSeller, ublVATScheme), "UBL-SR-12", true},

		{"UBL-SR-13 one seller tax registration", ublWith(t, ublAtSeller, ublOtherScheme), "UBL-SR-13", false},
		{"UBL-SR-13 two seller tax registrations", ublWith(t, ublAtSeller, ublOtherScheme+ublOtherScheme), "UBL-SR-13", true},

		{"UBL-SR-14 one seller legal form", ublWith(t, ublAtSeller,
			`<PartyLegalEntity><CompanyLegalForm>GmbH</CompanyLegalForm></PartyLegalEntity>`), "UBL-SR-14", false},
		{"UBL-SR-14 two seller legal forms", ublWith(t, ublAtSeller,
			`<PartyLegalEntity><CompanyLegalForm>GmbH</CompanyLegalForm><CompanyLegalForm>AG</CompanyLegalForm></PartyLegalEntity>`), "UBL-SR-14", true},

		{"UBL-SR-15 one buyer name", minimalUBL, "UBL-SR-15", false},
		{"UBL-SR-15 two buyer names", ublWith(t, ublAtBuyer,
			`<PartyLegalEntity><RegistrationName>Buyer Ltd (trading)</RegistrationName></PartyLegalEntity>`), "UBL-SR-15", true},

		{"UBL-SR-16 one buyer identifier", ublWith(t, ublAtBuyer,
			`<PartyIdentification><ID>BUY-1</ID></PartyIdentification>`), "UBL-SR-16", false},
		{"UBL-SR-16 two buyer identifiers", ublWith(t, ublAtBuyer,
			`<PartyIdentification><ID>BUY-1</ID></PartyIdentification><PartyIdentification><ID>BUY-2</ID></PartyIdentification>`), "UBL-SR-16", true},

		{"UBL-SR-17 one buyer legal registration identifier", ublWith(t, ublAtBuyer,
			`<PartyLegalEntity><CompanyID>HRB 4321</CompanyID></PartyLegalEntity>`), "UBL-SR-17", false},
		{"UBL-SR-17 two buyer legal registration identifiers", ublWith(t, ublAtBuyer,
			`<PartyLegalEntity><CompanyID>HRB 4321</CompanyID><CompanyID>HRB 8765</CompanyID></PartyLegalEntity>`), "UBL-SR-17", true},

		{"UBL-SR-18 one buyer VAT identifier", ublWith(t, ublAtBuyer, ublVATScheme), "UBL-SR-18", false},
		{"UBL-SR-18 two buyer VAT identifiers", ublWith(t, ublAtBuyer, ublVATScheme+ublVATScheme), "UBL-SR-18", true},

		{"UBL-SR-19 one payee name, different from the seller", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName></PayeeParty>`), "UBL-SR-19", false},
		{"UBL-SR-19 two payee names", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName><PartyName><Name>Factor GmbH</Name></PartyName></PayeeParty>`), "UBL-SR-19", true},
		{"UBL-SR-19 payee named exactly like the seller", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Seller Ltd</Name></PartyName></PayeeParty>`), "UBL-SR-19", true},

		{"UBL-SR-20 one payee identifier", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName><PartyIdentification><ID>PAY-1</ID></PartyIdentification></PayeeParty>`), "UBL-SR-20", false},
		{"UBL-SR-20 a payee identifier beside a SEPA creditor identifier", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName>`+
				`<PartyIdentification><ID>PAY-1</ID></PartyIdentification>`+
				`<PartyIdentification><ID schemeID="SEPA">DE98ZZZ09999999999</ID></PartyIdentification></PayeeParty>`), "UBL-SR-20", false},
		{"UBL-SR-20 two payee identifiers", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName>`+
				`<PartyIdentification><ID>PAY-1</ID></PartyIdentification>`+
				`<PartyIdentification><ID>PAY-2</ID></PartyIdentification></PayeeParty>`), "UBL-SR-20", true},

		{"UBL-SR-21 one payee legal registration identifier", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName><PartyLegalEntity><CompanyID>HRB 99</CompanyID></PartyLegalEntity></PayeeParty>`), "UBL-SR-21", false},
		{"UBL-SR-21 two payee legal registration identifiers", ublWith(t, ublAtDocument,
			`<PayeeParty><PartyName><Name>Factor Ltd</Name></PartyName>`+
				`<PartyLegalEntity><CompanyID>HRB 99</CompanyID><CompanyID>HRB 100</CompanyID></PartyLegalEntity></PayeeParty>`), "UBL-SR-21", true},

		{"UBL-SR-22 one tax representative name", ublWith(t, ublAtDocument,
			`<TaxRepresentativeParty><PartyName><Name>Rep Ltd</Name></PartyName>`+ublVATScheme+`</TaxRepresentativeParty>`), "UBL-SR-22", false},
		{"UBL-SR-22 two tax representative names", ublWith(t, ublAtDocument,
			`<TaxRepresentativeParty><PartyName><Name>Rep Ltd</Name></PartyName><PartyName><Name>Rep GmbH</Name></PartyName>`+
				ublVATScheme+`</TaxRepresentativeParty>`), "UBL-SR-22", true},

		{"UBL-SR-23 one tax representative VAT identifier", ublWith(t, ublAtDocument,
			`<TaxRepresentativeParty><PartyName><Name>Rep Ltd</Name></PartyName>`+ublVATScheme+`</TaxRepresentativeParty>`), "UBL-SR-23", false},
		{"UBL-SR-23 two tax representative VAT identifiers", ublWith(t, ublAtDocument,
			`<TaxRepresentativeParty><PartyName><Name>Rep Ltd</Name></PartyName>`+ublVATScheme+ublVATScheme+`</TaxRepresentativeParty>`), "UBL-SR-23", true},

		{"UBL-SR-25 one deliver-to party name", ublWith(t, ublAtDocument,
			`<Delivery><DeliveryParty><PartyName><Name>Warehouse A</Name></PartyName></DeliveryParty></Delivery>`), "UBL-SR-25", false},
		{"UBL-SR-25 two deliver-to party names", ublWith(t, ublAtDocument,
			`<Delivery><DeliveryParty><PartyName><Name>Warehouse A</Name></PartyName>`+
				`<PartyName><Name>Warehouse B</Name></PartyName></DeliveryParty></Delivery>`), "UBL-SR-25", true},

		{"UBL-SR-40 one buyer trading name", ublWith(t, ublAtBuyer,
			`<PartyName><Name>BuyerCo</Name></PartyName>`), "UBL-SR-40", false},
		{"UBL-SR-40 two buyer trading names", ublWith(t, ublAtBuyer,
			`<PartyName><Name>BuyerCo</Name></PartyName><PartyName><Name>BuyCo</Name></PartyName>`), "UBL-SR-40", true},

		{"UBL-SR-42 seller with two party tax schemes", ublWith(t, ublAtSeller, ublOtherScheme), "UBL-SR-42", false},
		{"UBL-SR-42 seller with three party tax schemes", ublWith(t, ublAtSeller, ublOtherScheme+ublOtherScheme), "UBL-SR-42", true},

		{"UBL-SR-51 one additional address line", ublWith(t, ublAtAddress,
			`<AddressLine><Line>Building C</Line></AddressLine>`), "UBL-SR-51", false},
		{"UBL-SR-51 two additional address lines", ublWith(t, ublAtAddress,
			`<AddressLine><Line>Building C</Line></AddressLine><AddressLine><Line>Floor 3</Line></AddressLine>`), "UBL-SR-51", true},

		{"UBL-SR-53 a complete party tax scheme", minimalUBL, "UBL-SR-53", false},
		{"UBL-SR-53 a tax scheme without a company identifier", ublWith(t, ublAtSeller,
			`<PartyTaxScheme><TaxScheme><ID>FC</ID></TaxScheme></PartyTaxScheme>`), "UBL-SR-53", true},
		{"UBL-SR-53 a company identifier without a tax scheme identifier", ublWith(t, ublAtSeller,
			`<PartyTaxScheme><CompanyID>123/456/789</CompanyID><TaxScheme></TaxScheme></PartyTaxScheme>`), "UBL-SR-53", true},

		// --- Payment instructions, allowances, charges and the VAT breakdown ---
		{"UBL-SR-26 one payment reference in a payment means group", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentID>REF-1</PaymentID></PaymentMeans>`), "UBL-SR-26", false},
		{"UBL-SR-26 two payment references in one payment means group", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentID>REF-1</PaymentID><PaymentID>REF-1</PaymentID></PaymentMeans>`), "UBL-SR-26", true},

		{"UBL-SR-27 one payment means code in a group", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>`), "UBL-SR-27", false},
		{"UBL-SR-27 two payment means codes in one group", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>`), "UBL-SR-27", true},

		{"UBL-SR-28 one mandate reference", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>59</PaymentMeansCode><PaymentMandate><ID>MND-1</ID></PaymentMandate></PaymentMeans>`), "UBL-SR-28", false},
		{"UBL-SR-28 two mandate references", ublWith(t, ublAtDocument,
			`<PaymentMeans><PaymentMeansCode>59</PaymentMeansCode><PaymentMandate><ID>MND-1</ID><ID>MND-2</ID></PaymentMandate></PaymentMeans>`), "UBL-SR-28", true},

		{"UBL-SR-30 one document allowance reason", ublWith(t, ublAtDocument,
			`<AllowanceCharge><ChargeIndicator>false</ChargeIndicator><AllowanceChargeReason>Discount</AllowanceChargeReason><Amount>5.00</Amount></AllowanceCharge>`), "UBL-SR-30", false},
		{"UBL-SR-30 two document allowance reasons", ublWith(t, ublAtDocument,
			`<AllowanceCharge><ChargeIndicator>false</ChargeIndicator><AllowanceChargeReason>Discount</AllowanceChargeReason>`+
				`<AllowanceChargeReason>Rebate</AllowanceChargeReason><Amount>5.00</Amount></AllowanceCharge>`), "UBL-SR-30", true},
		{"UBL-SR-30 stays silent on a charge", ublWith(t, ublAtDocument,
			`<AllowanceCharge><ChargeIndicator>true</ChargeIndicator><AllowanceChargeReason>Freight</AllowanceChargeReason>`+
				`<AllowanceChargeReason>Packing</AllowanceChargeReason><Amount>5.00</Amount></AllowanceCharge>`), "UBL-SR-30", false},

		{"UBL-SR-31 one document charge reason", ublWith(t, ublAtDocument,
			`<AllowanceCharge><ChargeIndicator>true</ChargeIndicator><AllowanceChargeReason>Freight</AllowanceChargeReason><Amount>5.00</Amount></AllowanceCharge>`), "UBL-SR-31", false},
		{"UBL-SR-31 two document charge reasons", ublWith(t, ublAtDocument,
			`<AllowanceCharge><ChargeIndicator>true</ChargeIndicator><AllowanceChargeReason>Freight</AllowanceChargeReason>`+
				`<AllowanceChargeReason>Packing</AllowanceChargeReason><Amount>5.00</Amount></AllowanceCharge>`), "UBL-SR-31", true},

		{"UBL-SR-32 one VAT exemption reason", ublWith(t, ublAtCategory,
			`<TaxExemptionReason>Reverse charge</TaxExemptionReason>`), "UBL-SR-32", false},
		{"UBL-SR-32 two VAT exemption reasons", ublWith(t, ublAtCategory,
			`<TaxExemptionReason>Reverse charge</TaxExemptionReason><TaxExemptionReason>Export</TaxExemptionReason>`), "UBL-SR-32", true},

		// --- Invoice and credit note lines --------------------------------
		{"UBL-SR-34 one line note", ublWith(t, ublAtLine, `<Note>Backordered</Note>`), "UBL-SR-34", false},
		{"UBL-SR-34 two line notes", ublWith(t, ublAtLine, `<Note>Backordered</Note><Note>Partial</Note>`), "UBL-SR-34", true},

		{"UBL-SR-35 one order line reference", ublWith(t, ublAtLine,
			`<OrderLineReference><LineID>10</LineID></OrderLineReference>`), "UBL-SR-35", false},
		{"UBL-SR-35 two order line references", ublWith(t, ublAtLine,
			`<OrderLineReference><LineID>10</LineID></OrderLineReference><OrderLineReference><LineID>20</LineID></OrderLineReference>`), "UBL-SR-35", true},

		{"UBL-SR-36 one line period", ublWith(t, ublAtLine,
			`<InvoicePeriod><StartDate>2024-01-01</StartDate><EndDate>2024-01-31</EndDate></InvoicePeriod>`), "UBL-SR-36", false},
		{"UBL-SR-36 two line periods", ublWith(t, ublAtLine,
			`<InvoicePeriod><StartDate>2024-01-01</StartDate><EndDate>2024-01-31</EndDate></InvoicePeriod>`+
				`<InvoicePeriod><StartDate>2024-02-01</StartDate><EndDate>2024-02-29</EndDate></InvoicePeriod>`), "UBL-SR-36", true},

		{"UBL-SR-37 one item price discount", ublWith(t, ublAtPrice,
			`<AllowanceCharge><ChargeIndicator>false</ChargeIndicator><Amount>5.00</Amount><BaseAmount>105.00</BaseAmount></AllowanceCharge>`), "UBL-SR-37", false},
		{"UBL-SR-37 two item price discounts", ublWith(t, ublAtPrice,
			`<AllowanceCharge><ChargeIndicator>false</ChargeIndicator><Amount>5.00</Amount><BaseAmount>105.00</BaseAmount></AllowanceCharge>`+
				`<AllowanceCharge><ChargeIndicator>false</ChargeIndicator><Amount>2.00</Amount><BaseAmount>107.00</BaseAmount></AllowanceCharge>`), "UBL-SR-37", true},

		{"UBL-SR-48 exactly one classified tax category", minimalUBL, "UBL-SR-48", false},
		{"UBL-SR-48 two classified tax categories", ublWith(t, ublAtItem,
			`<ClassifiedTaxCategory><ID>Z</ID><Percent>0</Percent></ClassifiedTaxCategory>`), "UBL-SR-48", true},
		{"UBL-SR-48 no classified tax category", mutate(t, minimalUBL,
			`<ClassifiedTaxCategory><ID>S</ID><Percent>19</Percent></ClassifiedTaxCategory>`, ``), "UBL-SR-48", true},

		{"UBL-SR-50 one item description", ublWith(t, ublAtItem, `<Description>A widget</Description>`), "UBL-SR-50", false},
		{"UBL-SR-50 two item descriptions", ublWith(t, ublAtItem,
			`<Description>A widget</Description><Description>Ein Widget</Description>`), "UBL-SR-50", true},

		{"UBL-SR-52 one line document reference", ublWith(t, ublAtLine,
			`<DocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>130</DocumentTypeCode></DocumentReference>`), "UBL-SR-52", false},
		{"UBL-SR-52 two line document references", ublWith(t, ublAtLine,
			`<DocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>130</DocumentTypeCode></DocumentReference>`+
				`<DocumentReference><ID schemeID="AAB">OBJ-2</ID><DocumentTypeCode>130</DocumentTypeCode></DocumentReference>`), "UBL-SR-52", true},

		// --- Preceding invoice and supporting document references ---------
		{"UBL-SR-06 one preceding invoice", ublWith(t, ublAtDocument,
			`<BillingReference><InvoiceDocumentReference><ID>PREV-1</ID></InvoiceDocumentReference></BillingReference>`), "UBL-SR-06", false},
		{"UBL-SR-06 two preceding invoices in one group", ublWith(t, ublAtDocument,
			`<BillingReference><InvoiceDocumentReference><ID>PREV-1</ID></InvoiceDocumentReference>`+
				`<InvoiceDocumentReference><ID>PREV-2</ID></InvoiceDocumentReference></BillingReference>`), "UBL-SR-06", true},

		{"UBL-SR-07 preceding invoice carries its number", ublWith(t, ublAtDocument,
			`<BillingReference><InvoiceDocumentReference><ID>PREV-1</ID></InvoiceDocumentReference></BillingReference>`), "UBL-SR-07", false},
		{"UBL-SR-07 preceding invoice without a number", ublWith(t, ublAtDocument,
			`<BillingReference><InvoiceDocumentReference><IssueDate>2023-12-01</IssueDate></InvoiceDocumentReference></BillingReference>`), "UBL-SR-07", true},

		{"UBL-SR-33 one supporting document description", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID>DOC-1</ID><DocumentDescription>Timesheet</DocumentDescription></AdditionalDocumentReference>`), "UBL-SR-33", false},
		{"UBL-SR-33 two supporting document descriptions", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID>DOC-1</ID><DocumentDescription>Timesheet</DocumentDescription>`+
				`<DocumentDescription>Delivery note</DocumentDescription></AdditionalDocumentReference>`), "UBL-SR-33", true},

		{"UBL-SR-43 supporting document with neither scheme nor type code", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID>DOC-1</ID><DocumentDescription>Timesheet</DocumentDescription></AdditionalDocumentReference>`), "UBL-SR-43", false},
		{"UBL-SR-43 invoiced object identifier with a scheme and type code 130", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>130</DocumentTypeCode></AdditionalDocumentReference>`), "UBL-SR-43", false},
		{"UBL-SR-43 credit note object identifier with type code 50", ublCreditNote(ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>50</DocumentTypeCode></AdditionalDocumentReference>`)), "UBL-SR-43", false},
		{"UBL-SR-43 supporting document carrying a scheme identifier", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID schemeID="AAB">DOC-1</ID><DocumentDescription>Timesheet</DocumentDescription></AdditionalDocumentReference>`), "UBL-SR-43", true},
		{"UBL-SR-43 type code 50 on an invoice rather than a credit note", ublWith(t, ublAtDocument,
			`<AdditionalDocumentReference><ID schemeID="AAB">OBJ-1</ID><DocumentTypeCode>50</DocumentTypeCode></AdditionalDocumentReference>`), "UBL-SR-43", true},
	}
	runRuleCases(t, cases)

	// The table above must cover the binding, not a subset of it that happens to
	// be easy: both verdicts, for every fatal rule CEN publishes.
	fires := map[string]bool{}
	silent := map[string]bool{}
	for _, c := range cases {
		if c.want {
			fires[c.rule] = true
		} else {
			silent[c.rule] = true
		}
	}
	for _, rule := range ublFatalSyntaxRules(t) {
		if !silent[rule] {
			t.Errorf("%s has no conforming case in this table: nothing says it stays silent on a document that satisfies it", rule)
		}
		if !fires[rule] {
			t.Errorf("%s has no violating case in this table: nothing says it is a rule rather than dead code", rule)
		}
	}
}

// ublFatalSyntaxRules reads the fatal UBL-SR-* identifiers out of the vendored
// CEN Schematron, so the table above is measured against what CEN publishes
// rather than against a list this package wrote down. It skips when the
// artefacts are absent, like every other oracle here.
func ublFatalSyntaxRules(t *testing.T) []string {
	t.Helper()
	dir := en16931SuiteDir()
	if dir == "" {
		t.Skip("EN 16931 artefact suite not present; run `make en16931-artefacts`")
	}
	// The abstract pattern is where the severity lives: the UBL binding file
	// supplies each rule's XPath, the abstract file its flag and its message.
	data, err := os.ReadFile(filepath.Join(dir, "ubl", "schematron", "abstract", "EN16931-syntax.sch"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\bid="(UBL-SR-\d+)"`)
	fatal := regexp.MustCompile(`flag="fatal"`)
	seen := map[string]bool{}
	var out []string
	for _, m := range regexp.MustCompile(`<assert[^>]*>`).FindAllString(string(data), -1) {
		if !fatal.MatchString(m) {
			continue
		}
		id := re.FindStringSubmatch(m)
		if id == nil || seen[id[1]] {
			continue
		}
		seen[id[1]] = true
		out = append(out, id[1])
	}
	sort.Strings(out)
	if len(out) != 54 {
		t.Fatalf("expected 54 fatal UBL-SR-* assertions in the CEN Schematron, found %d", len(out))
	}
	return out
}

// TestUBLSyntaxRulesAreNotAskedOfCII pins the half of the design that the file
// comment argues for: the UBL binding is a statement about UBL, so a CII invoice
// must never be accused under one of its identifiers.
func TestUBLSyntaxRulesAreNotAskedOfCII(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{"clean CII invoice", validCII},
		{"CII invoice with two payment means codes", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>30</TypeCode></SpecifiedTradeSettlementPaymentMeans>` +
				`<SpecifiedTradeSettlementPaymentMeans><TypeCode>58</TypeCode></SpecifiedTradeSettlementPaymentMeans>`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range Validate(context.Background(), []byte(tc.doc), ProfileEN16931).Violations {
				if strings.HasPrefix(v.Rule, "UBL-") {
					t.Errorf("CII invoice reported the UBL binding rule %s: %s", v.Rule, v.Message)
				}
			}
		})
	}
}

// TestUBLSyntaxRulesCarryTheEN16931Source pins the PR 3 decision: CEN publishes
// the syntax bindings as normative parts of EN 16931, so a binding finding is an
// EN 16931 finding and not one from a source of its own.
func TestUBLSyntaxRulesCarryTheEN16931Source(t *testing.T) {
	doc := ublWith(t, ublAtDocument,
		`<ContractDocumentReference><ID>C-1</ID></ContractDocumentReference>`+
			`<ContractDocumentReference><ID>C-2</ID></ContractDocumentReference>`)
	found := false
	for _, v := range Validate(context.Background(), []byte(doc), ProfileEN16931).Violations {
		if v.Rule != "UBL-SR-01" {
			continue
		}
		found = true
		if v.Source != SourceEN16931 {
			t.Errorf("UBL-SR-01 carries Source %q, want %q", v.Source, SourceEN16931)
		}
	}
	if !found {
		t.Fatal("UBL-SR-01 did not fire on the fixture this test is built from")
	}
}
