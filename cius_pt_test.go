package formalis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func ptRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if v.Source == SourceCIUSPT {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestCIUSPTCorpus is the FP=0 oracle: every official CIUS-PT sample instance
// (phax/phive-rules, all "good" cases) must satisfy the CIUS-PT rules.
//
// It is the strongest oracle available for this rule set, and it is stronger than
// its size suggests. phive-rules' CTestFiles registers all twenty through
// PhiveTestFile.createGoodCase against the *whole* compiled Schematron — all three
// phases, all 355 published assertions, both versions — so each file is AT's own
// claim that it violates none of them. And they are not minimal: nineteen of the
// twenty optional UBL groups these rules are conditional on are present and
// complete in the samples, so 64 of the 65 BR-CIUS-PT rules are genuinely asked
// rather than skipped for want of a context. (The exception is BR-CIUS-PT-13,
// whose context is an item attribute on a "Lower rate" line; AT's four AA-category
// lines carry no item attribute, which is what the rule asks for.)
//
// It stays scoped rather than asserting "no findings at all", and PR 22 was right
// to keep it that way: the samples are synthetic templates carrying placeholder
// code-list values, and this package reports BR-CL-01/10/11/16/17/18/22/25/26 and
// UBL-SR-43 on them from the EN 16931 core. Several of those are consequences of
// CIUS-PT's own extensions — AT's 'AA' and 'NA' VAT category codes are not in
// EN 16931's restricted BT-118 list, so BR-CL-17 reports every Portuguese invoice
// that uses a reduced rate — and settling them is EN 16931 code-list work, not
// CIUS-PT work.
//
// What did change here is the scope's *shape*: it filters on Source rather than on
// the "BR-CIUS-PT-" identifier prefix, so the eight BR-AA-* rules — AT's own
// "Lower rate" family, which does not carry that prefix — are inside the oracle
// instead of beside it. An identifier-prefix scope is exactly how a family comes to
// be unwatched.
//
// Skips when the corpus is absent (run `make cius-oracles`).
func TestCIUSPTCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-pt/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("CIUS-PT corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-PT corpus", len(files), minCIUSPTInstances)
	clean := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if pt := ptRuleViolations(findings(t, context.Background(), ValidateCIUSPT, data)); len(pt) != 0 {
			t.Errorf("%s: expected 0 CIUS-PT violations on a conformant sample, got %v", filepath.Base(f), pt)
		} else {
			clean++
		}
	}
	t.Logf("CIUS-PT corpus: %d/%d instances clean (FP=0) across all 73 published rules", clean, len(files))
}

// minimalCIUSPTUBL is a CIUS-PT-conformant invoice carrying every optional UBL
// group the rule set is conditional on, each one complete, with distinct values so
// that any single one can be emptied in isolation.
//
// It is deliberately not minimal any more, and that is the point. Thirty-one of the
// sixty-five rules are Schematron <report>s of the shape "this optional group is
// present and its identifying child is not", so a minimal invoice cannot make any
// of them fire: their context does not exist. A baseline that omits the groups
// would leave half the rule set with no fixture, which is the state C27, C30 and
// C33 were all found in — a rule that could be deleted without a red build.
//
// Two omissions are deliberate. There is no cac:BillingReference, so that the
// BR-CIUS-PT-65 fixture is a one-character change to the invoice type code rather
// than two edits; and there is no "Lower rate" category anywhere, so that the
// BR-AA-* fixtures introduce one. Core values are placeholders — the assertions
// here scope to Source rather than to arithmetic.
const minimalCIUSPTUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:2.1.1</cbc:CustomizationID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cac:OrderReference><cbc:ID>PO-1</cbc:ID></cac:OrderReference>
<cac:DespatchDocumentReference><cbc:ID>DESP-1</cbc:ID></cac:DespatchDocumentReference>
<cac:ReceiptDocumentReference><cbc:ID>RCPT-1</cbc:ID></cac:ReceiptDocumentReference>
<cac:OriginatorDocumentReference><cbc:ID>TEND-1</cbc:ID></cac:OriginatorDocumentReference>
<cac:ContractDocumentReference><cbc:ID>CTR-1</cbc:ID></cac:ContractDocumentReference>
<cac:AdditionalDocumentReference><cbc:ID>ADR-1</cbc:ID><cac:Attachment><cac:ExternalReference><cbc:URI>http://example.pt/doc</cbc:URI></cac:ExternalReference></cac:Attachment></cac:AdditionalDocumentReference>
<cac:ProjectReference><cbc:ID>PRJ-1</cbc:ID></cac:ProjectReference>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PartyIdentification><cbc:ID>SELLER-ID</cbc:ID></cac:PartyIdentification>
  <cac:PartyName><cbc:Name>Seller Trading</cbc:Name></cac:PartyName>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:PostalZone>1111-001</cbc:PostalZone><cac:AddressLine><cbc:Line>SellerLine3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>PT111111111</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller Lda</cbc:RegistrationName></cac:PartyLegalEntity>
  <cac:Contact><cbc:Name>Seller Contact</cbc:Name></cac:Contact>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PartyIdentification><cbc:ID>BUYER-ID</cbc:ID></cac:PartyIdentification>
  <cac:PartyName><cbc:Name>Buyer Trading</cbc:Name></cac:PartyName>
  <cac:PostalAddress><cbc:CityName>BuyerCity</cbc:CityName><cac:AddressLine><cbc:Line>BuyerLine3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>PT222222222</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer Lda</cbc:RegistrationName></cac:PartyLegalEntity>
  <cac:Contact><cbc:Telephone>+351210000000</cbc:Telephone></cac:Contact>
</cac:Party></cac:AccountingCustomerParty>
<cac:PayeeParty>
  <cac:PartyIdentification><cbc:ID>PAYEE-ID</cbc:ID></cac:PartyIdentification>
  <cac:PartyName><cbc:Name>Payee Name</cbc:Name></cac:PartyName>
  <cac:PartyLegalEntity><cbc:CompanyID>PAYEE-LEI</cbc:CompanyID></cac:PartyLegalEntity>
</cac:PayeeParty>
<cac:TaxRepresentativeParty>
  <cac:PartyName><cbc:Name>Tax Rep</cbc:Name></cac:PartyName>
  <cac:PostalAddress><cbc:CityName>RepCity</cbc:CityName><cac:AddressLine><cbc:Line>RepLine3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
</cac:TaxRepresentativeParty>
<cac:Delivery><cbc:ActualDeliveryDate>2024-01-14</cbc:ActualDeliveryDate><cac:DeliveryLocation><cbc:ID>LOC-1</cbc:ID><cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:PostalZone>4444-002</cbc:PostalZone><cac:AddressLine><cbc:Line>DelivLine3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation><cac:DeliveryParty><cac:PartyName><cbc:Name>Deliver To Lda</cbc:Name></cac:PartyName></cac:DeliveryParty></cac:Delivery>
<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>
  <cac:CardAccount><cbc:PrimaryAccountNumberID>1234</cbc:PrimaryAccountNumberID><cbc:NetworkID>VISA</cbc:NetworkID></cac:CardAccount>
  <cac:PayeeFinancialAccount><cbc:ID>PT50000201231234567890154</cbc:ID><cbc:Name>Seller account</cbc:Name><cac:FinancialInstitutionBranch><cbc:ID>BRANCH-1</cbc:ID></cac:FinancialInstitutionBranch></cac:PayeeFinancialAccount>
  <cac:PaymentMandate><cbc:ID>MANDATE-1</cbc:ID><cac:PayerFinancialAccount><cbc:ID>PAYER-ACCT</cbc:ID></cac:PayerFinancialAccount></cac:PaymentMandate>
</cac:PaymentMeans>
<cac:PaymentTerms><cbc:Note>30 dias</cbc:Note></cac:PaymentTerms>
<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReason>Desconto</cbc:AllowanceChargeReason><cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:AllowanceChargeReason>Portes</cbc:AllowanceChargeReason><cbc:Amount currencyID="EUR">5.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:TaxTotal><cbc:TaxAmount>21.85</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>95.00</cbc:TaxableAmount><cbc:TaxAmount>21.85</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:AllowanceTotalAmount>10.00</cbc:AllowanceTotalAmount><cbc:ChargeTotalAmount>5.00</cbc:ChargeTotalAmount><cbc:TaxExclusiveAmount>95.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>116.85</cbc:TaxInclusiveAmount><cbc:PayableAmount>116.85</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount>
  <cac:OrderLineReference><cbc:LineID>1</cbc:LineID></cac:OrderLineReference>
  <cac:DocumentReference><cbc:ID>OBJ-1</cbc:ID></cac:DocumentReference>
  <cac:Item><cbc:Name>Widget</cbc:Name>
    <cac:BuyersItemIdentification><cbc:ID>BII-1</cbc:ID></cac:BuyersItemIdentification>
    <cac:SellersItemIdentification><cbc:ID>SII-1</cbc:ID></cac:SellersItemIdentification>
    <cac:StandardItemIdentification><cbc:ID>SDI-1</cbc:ID></cac:StandardItemIdentification>
    <cac:OriginCountry><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:OriginCountry>
    <cac:CommodityClassification><cbc:ItemClassificationCode>65141500</cbc:ItemClassificationCode></cac:CommodityClassification>
    <cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>
  </cac:Item>
  <cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount><cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">0.00</cbc:Amount></cac:AllowanceCharge></cac:Price>
</cac:InvoiceLine>
</Invoice>`

// The strings the mutations key on. Each is unique in the baseline, which matters
// because a mutation is one strings.Replace with a count of 1: a `from` that also
// matches an earlier element would silently break a different rule than the one the
// case names, and the case would still pass if the named rule happened to fire for
// the wrong reason. The two document-level allowance/charge groups are the case
// that forced this — they are structurally identical, so the amount is part of the
// key.
const (
	ptSellerVATScheme    = `<cbc:CompanyID>PT111111111</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme>`
	ptBuyerVATScheme     = `<cbc:CompanyID>PT222222222</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme>`
	ptBreakdownCategory  = `<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`
	ptLineCategory       = `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>`
	ptDocAllowanceCat    = `<cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`
	ptDocChargeCat       = `<cbc:Amount currencyID="EUR">5.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`
	ptDelivery           = `<cac:Delivery><cbc:ActualDeliveryDate>2024-01-14</cbc:ActualDeliveryDate><cac:DeliveryLocation><cbc:ID>LOC-1</cbc:ID><cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:PostalZone>4444-002</cbc:PostalZone><cac:AddressLine><cbc:Line>DelivLine3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation><cac:DeliveryParty><cac:PartyName><cbc:Name>Deliver To Lda</cbc:Name></cac:PartyName></cac:DeliveryParty></cac:Delivery>`
	ptDeliverToAddress   = `<cac:Address><cbc:StreetName>DelivStreet</cbc:StreetName><cbc:CityName>DelivCity</cbc:CityName><cbc:PostalZone>4444-002</cbc:PostalZone><cac:AddressLine><cbc:Line>DelivLine3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:Address>`
	ptLegalMonetaryTotal = `<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:AllowanceTotalAmount>10.00</cbc:AllowanceTotalAmount><cbc:ChargeTotalAmount>5.00</cbc:ChargeTotalAmount><cbc:TaxExclusiveAmount>95.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>116.85</cbc:TaxInclusiveAmount><cbc:PayableAmount>116.85</cbc:PayableAmount></cac:LegalMonetaryTotal>`
	ptPayeeAccount       = `<cac:PayeeFinancialAccount><cbc:ID>PT50000201231234567890154</cbc:ID><cbc:Name>Seller account</cbc:Name><cac:FinancialInstitutionBranch><cbc:ID>BRANCH-1</cbc:ID></cac:FinancialInstitutionBranch></cac:PayeeFinancialAccount>`
	ptMandate            = `<cac:PaymentMandate><cbc:ID>MANDATE-1</cbc:ID><cac:PayerFinancialAccount><cbc:ID>PAYER-ACCT</cbc:ID></cac:PayerFinancialAccount></cac:PaymentMandate>`
	ptPriceAllowance     = `<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">0.00</cbc:Amount></cac:AllowanceCharge>`
)

// ciusPTMutations is one fixture per evaluated identifier: a substitution on the
// baseline that must make exactly that rule fire.
//
// The list is the firing half of "every rule has both verdicts". The silent half is
// the baseline itself, which runCIUSSuite requires to be clean, plus the twenty
// AT sample instances TestCIUSPTCorpus reads. Six identifiers need more than one
// edit and are whole documents in ciusPTExtras instead.
var ciusPTMutations = []ciusMutation{
	// $Invoice — the parties' tax identifiers and schemes, the totals, the delivery.
	{"no seller VAT identifier (01)", `<cbc:CompanyID>PT111111111</cbc:CompanyID>`, "", "BR-CIUS-PT-01"},
	{"seller tax scheme is not VAT (02)", ptSellerVATScheme, `<cbc:CompanyID>PT111111111</cbc:CompanyID><cac:TaxScheme><cbc:ID>GST</cbc:ID></cac:TaxScheme>`, "BR-CIUS-PT-02"},
	{"no buyer VAT identifier (03)", `<cbc:CompanyID>PT222222222</cbc:CompanyID>`, "", "BR-CIUS-PT-03"},
	{"buyer tax scheme is not VAT (04)", ptBuyerVATScheme, `<cbc:CompanyID>PT222222222</cbc:CompanyID><cac:TaxScheme><cbc:ID>GST</cbc:ID></cac:TaxScheme>`, "BR-CIUS-PT-04"},
	{"no document totals (10)", ptLegalMonetaryTotal, "", "BR-CIUS-PT-10"},
	{"no total VAT amount (11)", `<cac:TaxTotal><cbc:TaxAmount>21.85</cbc:TaxAmount>`, `<cac:TaxTotal>`, "BR-CIUS-PT-11"},
	{"debit note without a preceding invoice (65)", `<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>`, `<cbc:InvoiceTypeCode>383</cbc:InvoiceTypeCode>`, "BR-CIUS-PT-65"},
	{"no deliver-to address (66)", ptDeliverToAddress, "", "BR-CIUS-PT-66"},

	// $Document_totals — the two sums that become mandatory once a document-level
	// allowance or charge is present.
	{"allowance without its document total (62)", `<cbc:AllowanceTotalAmount>10.00</cbc:AllowanceTotalAmount>`, "", "BR-CIUS-PT-62"},
	{"charge without its document total (63)", `<cbc:ChargeTotalAmount>5.00</cbc:ChargeTotalAmount>`, "", "BR-CIUS-PT-63"},

	// $Seller, $Buyer, $Payee and the three postal addresses.
	{"no seller street (05)", `<cbc:StreetName>SellerStreet</cbc:StreetName>`, "", "BR-CIUS-PT-05"},
	{"no seller city (06)", `<cbc:CityName>SellerCity</cbc:CityName>`, "", "BR-CIUS-PT-06"},
	{"no seller postcode (07)", `<cbc:PostalZone>1111-001</cbc:PostalZone>`, "", "BR-CIUS-PT-07"},
	{"empty payee name (32)", `<cac:PartyName><cbc:Name>Payee Name</cbc:Name></cac:PartyName>`, `<cac:PartyName/>`, "BR-CIUS-PT-32"},
	{"empty seller identification (34)", `<cac:PartyIdentification><cbc:ID>SELLER-ID</cbc:ID></cac:PartyIdentification>`, `<cac:PartyIdentification/>`, "BR-CIUS-PT-34"},
	{"empty seller trading name (35)", `<cac:PartyName><cbc:Name>Seller Trading</cbc:Name></cac:PartyName>`, `<cac:PartyName/>`, "BR-CIUS-PT-35"},
	{"empty seller contact (36)", `<cac:Contact><cbc:Name>Seller Contact</cbc:Name></cac:Contact>`, `<cac:Contact/>`, "BR-CIUS-PT-36"},
	{"empty seller address line (37)", `<cac:AddressLine><cbc:Line>SellerLine3</cbc:Line></cac:AddressLine>`, `<cac:AddressLine/>`, "BR-CIUS-PT-37"},
	{"empty buyer identification (38)", `<cac:PartyIdentification><cbc:ID>BUYER-ID</cbc:ID></cac:PartyIdentification>`, `<cac:PartyIdentification/>`, "BR-CIUS-PT-38"},
	{"empty buyer trading name (39)", `<cac:PartyName><cbc:Name>Buyer Trading</cbc:Name></cac:PartyName>`, `<cac:PartyName/>`, "BR-CIUS-PT-39"},
	{"empty buyer contact (40)", `<cac:Contact><cbc:Telephone>+351210000000</cbc:Telephone></cac:Contact>`, `<cac:Contact/>`, "BR-CIUS-PT-40"},
	{"empty buyer address line (41)", `<cac:AddressLine><cbc:Line>BuyerLine3</cbc:Line></cac:AddressLine>`, `<cac:AddressLine/>`, "BR-CIUS-PT-41"},
	{"empty payee identification (42)", `<cac:PartyIdentification><cbc:ID>PAYEE-ID</cbc:ID></cac:PartyIdentification>`, `<cac:PartyIdentification/>`, "BR-CIUS-PT-42"},
	{"empty payee legal entity (43)", `<cac:PartyLegalEntity><cbc:CompanyID>PAYEE-LEI</cbc:CompanyID></cac:PartyLegalEntity>`, `<cac:PartyLegalEntity/>`, "BR-CIUS-PT-43"},
	{"empty tax representative address line (44)", `<cac:AddressLine><cbc:Line>RepLine3</cbc:Line></cac:AddressLine>`, `<cac:AddressLine/>`, "BR-CIUS-PT-44"},

	// The six document-reference contexts.
	{"empty order reference (24)", `<cac:OrderReference><cbc:ID>PO-1</cbc:ID></cac:OrderReference>`, `<cac:OrderReference/>`, "BR-CIUS-PT-24"},
	{"empty despatch reference (26)", `<cac:DespatchDocumentReference><cbc:ID>DESP-1</cbc:ID></cac:DespatchDocumentReference>`, `<cac:DespatchDocumentReference/>`, "BR-CIUS-PT-26"},
	{"empty receipt reference (27)", `<cac:ReceiptDocumentReference><cbc:ID>RCPT-1</cbc:ID></cac:ReceiptDocumentReference>`, `<cac:ReceiptDocumentReference/>`, "BR-CIUS-PT-27"},
	{"empty originator reference (28)", `<cac:OriginatorDocumentReference><cbc:ID>TEND-1</cbc:ID></cac:OriginatorDocumentReference>`, `<cac:OriginatorDocumentReference/>`, "BR-CIUS-PT-28"},
	{"empty contract reference (29)", `<cac:ContractDocumentReference><cbc:ID>CTR-1</cbc:ID></cac:ContractDocumentReference>`, `<cac:ContractDocumentReference/>`, "BR-CIUS-PT-29"},
	{"attachment with neither URI nor binary (30)", `<cac:Attachment><cac:ExternalReference><cbc:URI>http://example.pt/doc</cbc:URI></cac:ExternalReference></cac:Attachment>`, `<cac:Attachment/>`, "BR-CIUS-PT-30"},
	{"empty project reference (33)", `<cac:ProjectReference><cbc:ID>PRJ-1</cbc:ID></cac:ProjectReference>`, `<cac:ProjectReference/>`, "BR-CIUS-PT-33"},

	// $Delivery and $Deliver_to_address.
	{"no deliver-to street (21)", `<cbc:StreetName>DelivStreet</cbc:StreetName>`, "", "BR-CIUS-PT-21"},
	{"no deliver-to city (22)", `<cbc:CityName>DelivCity</cbc:CityName>`, "", "BR-CIUS-PT-22"},
	{"no deliver-to postcode (23)", `<cbc:PostalZone>4444-002</cbc:PostalZone>`, "", "BR-CIUS-PT-23"},
	{"empty deliver-to address line (45)", `<cac:AddressLine><cbc:Line>DelivLine3</cbc:Line></cac:AddressLine>`, `<cac:AddressLine/>`, "BR-CIUS-PT-45"},
	{"empty deliver-to party (46)", `<cac:DeliveryParty><cac:PartyName><cbc:Name>Deliver To Lda</cbc:Name></cac:PartyName></cac:DeliveryParty>`, `<cac:DeliveryParty/>`, "BR-CIUS-PT-46"},
	// BR-CIUS-PT-64's four alternatives all absent: a cac:Delivery with no actual
	// delivery date, no DeliveryParty, no location identifier and no address. The
	// rule used to accept only two of the four, so an invoice that named the party
	// it delivered to was reported; this leaves none of them.
	{"delivery evidences nothing (64)", ptDelivery, `<cac:Delivery/>`, "BR-CIUS-PT-64"},

	// $Payment_instructions and $Payment_terms.
	{"payment account identifies itself in no way (47)", ptPayeeAccount, `<cac:PayeeFinancialAccount/>`, "BR-CIUS-PT-47"},
	{"empty financial institution branch (48)", `<cac:FinancialInstitutionBranch><cbc:ID>BRANCH-1</cbc:ID></cac:FinancialInstitutionBranch>`, `<cac:FinancialInstitutionBranch/>`, "BR-CIUS-PT-48"},
	{"mandate with neither reference nor debited account (49)", ptMandate, `<cac:PaymentMandate/>`, "BR-CIUS-PT-49"},
	{"empty payer financial account (50)", `<cac:PayerFinancialAccount><cbc:ID>PAYER-ACCT</cbc:ID></cac:PayerFinancialAccount>`, `<cac:PayerFinancialAccount/>`, "BR-CIUS-PT-50"},
	{"card account without its network (60)", `<cbc:NetworkID>VISA</cbc:NetworkID>`, "", "BR-CIUS-PT-60"},
	{"payment terms without a note (61)", `<cac:PaymentTerms><cbc:Note>30 dias</cbc:Note></cac:PaymentTerms>`, `<cac:PaymentTerms/>`, "BR-CIUS-PT-61"},

	// $Invoice_Line, $Invoice_Line_Item and $Invoice_Line_Price.
	{"line item without a tax scheme (09)", ptLineCategory, `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent></cac:ClassifiedTaxCategory>`, "BR-CIUS-PT-09"},
	{"empty order line reference (51)", `<cac:OrderLineReference><cbc:LineID>1</cbc:LineID></cac:OrderLineReference>`, `<cac:OrderLineReference/>`, "BR-CIUS-PT-51"},
	{"empty line object reference (52)", `<cac:DocumentReference><cbc:ID>OBJ-1</cbc:ID></cac:DocumentReference>`, `<cac:DocumentReference/>`, "BR-CIUS-PT-52"},
	{"empty buyer's item identifier (53)", `<cac:BuyersItemIdentification><cbc:ID>BII-1</cbc:ID></cac:BuyersItemIdentification>`, `<cac:BuyersItemIdentification/>`, "BR-CIUS-PT-53"},
	{"empty seller's item identifier (54)", `<cac:SellersItemIdentification><cbc:ID>SII-1</cbc:ID></cac:SellersItemIdentification>`, `<cac:SellersItemIdentification/>`, "BR-CIUS-PT-54"},
	{"empty standard item identifier (55)", `<cac:StandardItemIdentification><cbc:ID>SDI-1</cbc:ID></cac:StandardItemIdentification>`, `<cac:StandardItemIdentification/>`, "BR-CIUS-PT-55"},
	{"empty item origin country (56)", `<cac:OriginCountry><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:OriginCountry>`, `<cac:OriginCountry/>`, "BR-CIUS-PT-56"},
	{"empty commodity classification (57)", `<cac:CommodityClassification><cbc:ItemClassificationCode>65141500</cbc:ItemClassificationCode></cac:CommodityClassification>`, `<cac:CommodityClassification/>`, "BR-CIUS-PT-57"},
	{"charge on the price detail (58)", `<cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount>`, `<cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount><cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">1.00</cbc:Amount></cac:AllowanceCharge>`, "BR-CIUS-PT-58"},
	{"price discount without an amount (59)", ptPriceAllowance, `<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator></cac:AllowanceCharge>`, "BR-CIUS-PT-59"},

	// $Document_level_allowances and $Document_level_charges.
	{"document allowance without a tax scheme (19)", ptDocAllowanceCat, `<cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent></cac:TaxCategory>`, "BR-CIUS-PT-19"},
	{"document charge without a tax scheme (20)", ptDocChargeCat, `<cbc:Amount currencyID="EUR">5.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent></cac:TaxCategory>`, "BR-CIUS-PT-20"},

	// $VAT_breakdown and its three code-filtered subsets.
	{"breakdown without a tax scheme (08)", ptBreakdownCategory, `<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent></cac:TaxCategory></cac:TaxSubtotal>`, "BR-CIUS-PT-08"},
	{"lower-rate breakdown with no rate (12)", ptBreakdownCategory, `<cac:TaxCategory><cbc:ID>AA</cbc:ID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`, "BR-CIUS-PT-12"},
	{"standard-rated breakdown at zero (14)", ptBreakdownCategory, `<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`, "BR-CIUS-PT-14"},
	{"exempt breakdown at a non-zero rate (16)", ptBreakdownCategory, `<cac:TaxCategory><cbc:ID>E</cbc:ID><cbc:Percent>23</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`, "BR-CIUS-PT-16"},
	// -15 forbids the exemption-reason item attribute on a standard-rated line. The
	// baseline's line is standard-rated, so adding the attribute is the whole fixture.
	{"standard-rated line carrying an exemption reason (15)", `<cac:BuyersItemIdentification>`, `<cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#</cbc:Name><cbc:Value>Artigo 9</cbc:Value></cac:AdditionalItemProperty><cac:BuyersItemIdentification>`, "BR-CIUS-PT-15"},
	// -18 is the exempt line with no item attribute at all, which is what recategorising
	// the baseline's line leaves.
	{"exempt line with no exemption reason (18)", `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID>`, `<cac:ClassifiedTaxCategory><cbc:ID>E</cbc:ID>`, "BR-CIUS-PT-18"},

	// BR-AA-*, AT's own "Lower rate" family.
	{"lower-rate line with no lower-rate breakdown (BR-AA-01)", `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID>`, `<cac:ClassifiedTaxCategory><cbc:ID>AA</cbc:ID>`, "BR-AA-01"},
	{"lower-rate line at a zero rate (BR-AA-05)", ptLineCategory, `<cac:ClassifiedTaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>`, "BR-AA-05"},
	{"lower-rate document allowance at a zero rate (BR-AA-06)", ptDocAllowanceCat, `<cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`, "BR-AA-06"},
	{"lower-rate document charge at a zero rate (BR-AA-07)", ptDocChargeCat, `<cbc:Amount currencyID="EUR">5.00</cbc:Amount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`, "BR-AA-07"},
	{"lower-rate breakdown with an exemption reason (BR-AA-10)", ptBreakdownCategory, `<cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cbc:TaxExemptionReason>nao aplicavel</cbc:TaxExemptionReason><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`, "BR-AA-10"},
}

// ciusPTExtras are the six identifiers whose fixture cannot be one substitution:
// a credit note is not a mutation of an invoice, and the three BR-AA identity rules
// need both a "Lower rate" category *and* a seller with no VAT identifier.
//
// Each is a whole document rather than a diff, and each is deliberately small: what
// it has to demonstrate is that one rule fires, not that everything else is clean,
// which the baseline and the AT corpus already say.
var ciusPTExtras = []ciusDoc{
	{"credit note with no preceding invoice (25)", ptCreditNoteNoBillingRef, "BR-CIUS-PT-25"},
	{"lower-rate line carrying an exemption reason (13)", ptLowerRateLineWithExemption, "BR-CIUS-PT-13"},
	{"exempt line whose item attribute is not the exemption reason (17)", ptExemptLineWrongAttribute, "BR-CIUS-PT-17"},
	{"lower-rate line with no seller VAT identifier (BR-AA-02)", ptLowerRateNoSellerVAT, "BR-AA-02"},
	{"lower-rate document allowance with no seller VAT identifier (BR-AA-03)", ptLowerRateAllowanceNoSellerVAT, "BR-AA-03"},
	{"lower-rate document charge with no seller VAT identifier (BR-AA-04)", ptLowerRateChargeNoSellerVAT, "BR-AA-04"},
}

const ptDocHead = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>INV-2</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate><cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>`

// ptCreditNoteNoBillingRef is BR-CIUS-PT-25's context: the document element is a
// credit note and there is no cac:BillingReference. The rule reads
// `exists(//cn:CreditNote) and not(cac:BillingReference)`, so the document element
// is half of the test and no mutation of an Invoice can produce it.
const ptCreditNoteNoBillingRef = `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:ID>CN-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate><cbc:CreditNoteTypeCode>381</cbc:CreditNoteTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
</CreditNote>`

// ptLowerRateLineWithExemption is BR-CIUS-PT-13: a "Lower rate" line item carrying
// the exemption-reason item attribute the rule forbids there.
const ptLowerRateLineWithExemption = ptDocHead + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cac:Item><cbc:Name>Widget</cbc:Name>
  <cac:AdditionalItemProperty><cbc:Name>#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#</cbc:Name><cbc:Value>M99</cbc:Value></cac:AdditionalItemProperty>
  <cac:ClassifiedTaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>
</cac:Item></cac:InvoiceLine>
</Invoice>`

// ptExemptLineWrongAttribute is BR-CIUS-PT-17: an exempt line that carries item
// attributes, none of which is the exemption reason. The rule is bound to each
// cbc:Name and counts exact matches across the line, so a line with attributes but
// the wrong ones is its firing case; a line with none at all is BR-CIUS-PT-18's.
const ptExemptLineWrongAttribute = ptDocHead + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cac:Item><cbc:Name>Widget</cbc:Name>
  <cac:AdditionalItemProperty><cbc:Name>NUM_CONTRATO</cbc:Name><cbc:Value>1234</cbc:Value></cac:AdditionalItemProperty>
  <cac:ClassifiedTaxCategory><cbc:ID>E</cbc:ID><cbc:Percent>0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>
</cac:Item></cac:InvoiceLine>
</Invoice>`

// The three BR-AA identity rules: a "Lower rate" line, document-level allowance or
// document-level charge in a document whose seller carries neither a VAT identifier
// nor a tax representative with one.
const ptLowerRateNoSellerVAT = ptDocHead + `
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cac:Item><cbc:Name>Widget</cbc:Name>
  <cac:ClassifiedTaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>
</cac:Item></cac:InvoiceLine>
<cac:TaxTotal><cbc:TaxAmount>6.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>6.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
</Invoice>`

const ptLowerRateAllowanceNoSellerVAT = ptDocHead + `
<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:TaxTotal><cbc:TaxAmount>6.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>6.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
</Invoice>`

const ptLowerRateChargeNoSellerVAT = ptDocHead + `
<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount currencyID="EUR">10.00</cbc:Amount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:TaxTotal><cbc:TaxAmount>6.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>6.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>AA</cbc:ID><cbc:Percent>6</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
</Invoice>`

func TestCIUSPTMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[0])
}
