package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Per-rule tests for the XRechnung rule set, and the self-checks that measure the
// table against what KoSIT publishes rather than against a list this package
// wrote down.
//
// Neither of this repository's two general oracles reaches these rules. The CEN
// unit-test suite knows nothing about a German CIUS, and the 86-instance KoSIT
// business-case corpus is conforming by construction, so it can catch a rule that
// fires when it should not and is silent about every rule that never fires at
// all. That is the gap PRs 14 and 16 were held to, and it is the one where "the
// rule is right" and "the rule is bound to an element name nothing in the corpus
// contains" look identical from outside.
//
// TestXRechnungRules is the hand-written table that closes it, one violating case
// per rule, and TestXRechnungBaselinesAreClean is the conforming verdict for the
// whole rule set at once. The self-checks that measure the table against KoSIT's
// published identifiers arrive with the last of the rules.

// minimalXRechnungCII is minimalXRechnungUBL's counterpart in the other binding:
// a conforming XRechnung CII invoice carrying every term XRechnung makes
// mandatory. Nine of KoSIT's identifiers exist in one binding only and several
// more test a different thing in each — BR-DE-30 and BR-DE-31 do — so a table with
// one fixture would leave half the rule set unexercised.
const minimalXRechnungCII = `<CrossIndustryInvoice>
  <ExchangedDocumentContext><GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0</ID></GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext>
  <ExchangedDocument><ID>INV-1</ID><TypeCode>380</TypeCode><IssueDateTime><DateTimeString>20240101</DateTimeString></IssueDateTime></ExchangedDocument>
  <SupplyChainTradeTransaction>
    <IncludedSupplyChainTradeLineItem>
      <AssociatedDocumentLineDocument><LineID>1</LineID></AssociatedDocumentLineDocument>
      <SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>
      <SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>
      <SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>
      <SpecifiedLineTradeSettlement><ApplicableTradeTax><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax><SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>100.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation></SpecifiedLineTradeSettlement>
    </IncludedSupplyChainTradeLineItem>
    <ApplicableHeaderTradeAgreement>
      <BuyerReference>04011000-12345-03</BuyerReference>
      <SellerTradeParty><Name>Seller Co</Name>
        <DefinedTradeContact><PersonName>Tim Tester</PersonName>
          <TelephoneUniversalCommunication><CompleteNumber>012 3456789</CompleteNumber></TelephoneUniversalCommunication>
          <EmailURIUniversalCommunication><URIID>tim@test.de</URIID></EmailURIUniversalCommunication></DefinedTradeContact>
        <PostalTradeAddress><PostcodeCode>10115</PostcodeCode><CityName>Berlin</CityName><CountryID>DE</CountryID></PostalTradeAddress>
        <SpecifiedTaxRegistration><ID schemeID="VA">DE123456789</ID></SpecifiedTaxRegistration></SellerTradeParty>
      <BuyerTradeParty><Name>Buyer Co</Name>
        <PostalTradeAddress><PostcodeCode>53113</PostcodeCode><CityName>Bonn</CityName><CountryID>DE</CountryID></PostalTradeAddress></BuyerTradeParty>
    </ApplicableHeaderTradeAgreement>
    <ApplicableHeaderTradeDelivery>
      <ActualDeliverySupplyChainEvent><OccurrenceDateTime><DateTimeString>20240101</DateTimeString></OccurrenceDateTime></ActualDeliverySupplyChainEvent>
    </ApplicableHeaderTradeDelivery>
    <ApplicableHeaderTradeSettlement>
      <InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>
      <SpecifiedTradeSettlementPaymentMeans><TypeCode>58</TypeCode>
        <PayeePartyCreditorFinancialAccount><IBANID>DE75512108001245126199</IBANID></PayeePartyCreditorFinancialAccount></SpecifiedTradeSettlementPaymentMeans>
      <ApplicableTradeTax><CalculatedAmount>20.00</CalculatedAmount><BasisAmount>100.00</BasisAmount><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>
      <SpecifiedTradeSettlementHeaderMonetarySummation>
        <LineTotalAmount>100.00</LineTotalAmount>
        <TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>
        <TaxTotalAmount>20.00</TaxTotalAmount>
        <GrandTotalAmount>120.00</GrandTotalAmount>
        <DuePayableAmount>120.00</DuePayableAmount>
      </SpecifiedTradeSettlementHeaderMonetarySummation>
    </ApplicableHeaderTradeSettlement>
  </SupplyChainTradeTransaction>
</CrossIndustryInvoice>`

// The three specification identifiers of common.sch. The sub-profile fixtures are
// the plain ones with BT-24 swapped, which is also the only thing that puts a
// document into a sub-profile.
const (
	xrCIUSID      = "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0"
	xrExtensionID = xrCIUSID + "#conformant#urn:xeinkauf.de:kosit:extension:xrechnung_3.0"
	xrCVDID       = xrCIUSID + "#compliant#urn:xeinkauf.de:kosit:xrechnung:cvd_0.9"
)

// asExtension and asCVD move a fixture into a sub-profile.
func asExtension(doc string) string { return strings.Replace(doc, xrCIUSID+"<", xrExtensionID+"<", 1) }
func asCVD(doc string) string       { return strings.Replace(doc, xrCIUSID+"<", xrCVDID+"<", 1) }

// The insertion points the cases below build from.
const (
	xrUBLAtBody    = "<cac:TaxTotal>" // a direct child of the UBL document element
	xrUBLAtItem    = "<cbc:Name>Widget</cbc:Name>"
	xrUBLAtContact = "<cbc:Name>Tim Tester</cbc:Name>"
	xrUBLAtSeller  = "<cac:AccountingSupplierParty><cac:Party>"
	// The delivery group the fixtures carry for BR-DE-TMP-32's sake, which is also
	// where a case adds BG-15: a second cac:Delivery would trip UBL-SR-24 and leave
	// the mapper reading the first.
	xrUBLAtDelivery = "<cac:Delivery>"
	xrUBLDelivery   = `<cac:Delivery><cbc:ActualDeliveryDate>2024-01-10</cbc:ActualDeliveryDate></cac:Delivery>`
	xrCIIDelivery   = `<ActualDeliverySupplyChainEvent><OccurrenceDateTime><DateTimeString>20240101</DateTimeString></OccurrenceDateTime></ActualDeliverySupplyChainEvent>`
	xrCIIAtSettle   = "<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>"
	xrCIIAtAgree    = "<BuyerReference>04011000-12345-03</BuyerReference>"
	xrCIIAtProduct  = "<SpecifiedTradeProduct><Name>Widget</Name>"
	xrCIIAtContact  = "<PersonName>Tim Tester</PersonName>"
	xrCIIContact    = `<DefinedTradeContact><PersonName>Tim Tester</PersonName>
          <TelephoneUniversalCommunication><CompleteNumber>012 3456789</CompleteNumber></TelephoneUniversalCommunication>
          <EmailURIUniversalCommunication><URIID>tim@test.de</URIID></EmailURIUniversalCommunication></DefinedTradeContact>`
	xrUBLPaymentMns = `<cac:PaymentMeans><cbc:PaymentMeansCode>58</cbc:PaymentMeansCode>
  <cac:PayeeFinancialAccount><cbc:ID>DE75512108001245126199</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`
	xrCIIPaymentMns = `<SpecifiedTradeSettlementPaymentMeans><TypeCode>58</TypeCode>
        <PayeePartyCreditorFinancialAccount><IBANID>DE75512108001245126199</IBANID></PayeePartyCreditorFinancialAccount></SpecifiedTradeSettlementPaymentMeans>`
)

// xrWith is doc with x inserted immediately after anchor, and xrBefore is doc
// with x inserted immediately before it.
//
// Both exist because the anchor decides the depth: xrUBLAtBody is the first
// element of the UBL document that a case's insertion must sit *alongside*, so
// inserting after it makes the new element that anchor's child instead of the
// document element's — and a rule whose XPath is a direct step
// (cac:PaymentMeans, cac:PrepaidPayment, cac:Delivery) then never sees it, while
// one written // sees it either way. That is the shape of a test that passes for
// the wrong reason, so the two are named apart.
func xrWith(t *testing.T, doc, anchor, x string) string {
	t.Helper()
	return mutate(t, doc, anchor, anchor+x)
}

func xrBefore(t *testing.T, doc, anchor, x string) string {
	t.Helper()
	return mutate(t, doc, anchor, x+anchor)
}

// xrCase is ruleCase for a rule whose severity is also under test. want=false
// means the rule must not fire; sev is checked only when want is true.
type xrCase struct {
	name string
	xml  string
	rule string
	want bool
	sev  Severity
}

func runXRechnungCases(t *testing.T, cases []xrCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs := findings(t, context.Background(), ValidateXRechnung, []byte(tc.xml))
			var got *Violation
			for i, v := range vs {
				if v.Rule == tc.rule && v.Source == SourceXRechnung {
					got = &vs[i]
				}
			}
			switch {
			case tc.want && got == nil:
				t.Errorf("expected %s to fire; got %v", tc.rule, vs)
			case !tc.want && got != nil:
				t.Errorf("%s fired on a document that satisfies it: %s", tc.rule, got.Message)
			case tc.want && got.Severity != tc.sev:
				t.Errorf("%s was reported as %s; KoSIT flags it %s", tc.rule, got.Severity, tc.sev)
			}
		})
	}
}

// TestXRechnungBaselinesAreClean is the conforming verdict for the whole rule set
// at once, in both bindings and all three sub-profiles.
//
// It is what lets the table below hold only violating cases: a rule that fires on
// one of these documents is over-firing, and there is no cheaper way to say that
// about a whole rule set. It asserts no finding at all rather than no fatal
// finding, advisory rules included, because these fixtures are this package's own
// and there is no reason for one of them to carry UBL the EN 16931 core subset
// leaves out.
func TestXRechnungBaselinesAreClean(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{"UBL CIUS", minimalXRechnungUBL},
		{"CII CIUS", minimalXRechnungCII},
		{"UBL EXTENSION", asExtension(minimalXRechnungUBL)},
		{"CII EXTENSION", asExtension(minimalXRechnungCII)},
		{"UBL CVD", xrCVDUBL(t)},
		{"CII CVD", xrCVDCII(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v := findings(t, context.Background(), ValidateXRechnung, []byte(tc.doc)); len(v) != 0 {
				t.Fatalf("baseline not clean: %d violations (first %s: %s)", len(v), v[0].Rule, v[0].Message)
			}
		})
	}
}

// xrCVDUBL and xrCVDCII are the CVD sub-profile's conforming fixtures: BT-24 for
// the sub-profile, the contract reference (BT-12) and tender reference (BT-17) it
// makes mandatory, and one line carrying a vehicle category and a clean-vehicle
// attribute.
func xrCVDUBL(t *testing.T) string {
	t.Helper()
	doc := asCVD(minimalXRechnungUBL)
	doc = mutate(t, doc, xrUBLAtBody,
		`<cac:ContractDocumentReference><cbc:ID>C-1</cbc:ID></cac:ContractDocumentReference>`+
			`<cac:OriginatorDocumentReference><cbc:ID>T-1</cbc:ID></cac:OriginatorDocumentReference>`+xrUBLAtBody)
	return xrWith(t, doc, xrUBLAtItem, xrUBLCVDItem("M1", "clean"))
}

// xrUBLCVDItem is the pair of item elements the CVD rules are about.
func xrUBLCVDItem(category, cva string) string {
	return `<cac:CommodityClassification><cbc:ItemClassificationCode listID="CVD">` + category +
		`</cbc:ItemClassificationCode></cac:CommodityClassification>` +
		`<cac:AdditionalItemProperty><cbc:Name>cva</cbc:Name><cbc:Value>` + cva + `</cbc:Value></cac:AdditionalItemProperty>`
}

func xrCVDCII(t *testing.T) string {
	t.Helper()
	doc := asCVD(minimalXRechnungCII)
	doc = xrWith(t, doc, xrCIIAtAgree,
		`<ContractReferencedDocument><IssuerAssignedID>C-1</IssuerAssignedID></ContractReferencedDocument>`+
			`<AdditionalReferencedDocument><IssuerAssignedID>T-1</IssuerAssignedID><TypeCode>50</TypeCode></AdditionalReferencedDocument>`)
	return xrWith(t, doc, xrCIIAtProduct, xrCIICVDItem("M1", "clean"))
}

func xrCIICVDItem(category, cva string) string {
	return `<DesignatedProductClassification><ClassCode listID="CVD">` + category + `</ClassCode></DesignatedProductClassification>` +
		`<ApplicableProductCharacteristic><Description>cva</Description><Value>` + cva + `</Value></ApplicableProductCharacteristic>`
}

// TestXRechnungRules is the violating half of the table, grouped as
// xrechnung_rules.go groups the rule bodies.
func TestXRechnungRules(t *testing.T) {
	var cases []xrCase
	cases = append(cases, xrSkontoCases(t)...)
	cases = append(cases, xrPaymentMeansCases(t)...)
	cases = append(cases, xrExtensionCases(t)...)
	cases = append(cases, xrCVDCases(t)...)
	cases = append(cases, xrProvisionalCases(t)...)
	runXRechnungCases(t, cases)
}

// xrSkontoCases is BR-DE-18, whose subject is the text of BT-20 rather than any
// element. The regular expression is the one place in this rule set where a
// conforming and a violating document differ by one character, so the cases are
// written as the payment-terms note and nothing else.
func xrSkontoCases(t *testing.T) []xrCase {
	ubl := func(note string) string {
		return mutate(t, minimalXRechnungUBL, xrUBLAtBody,
			"<cac:PaymentTerms><cbc:Note>"+note+"</cbc:Note></cac:PaymentTerms>"+xrUBLAtBody)
	}
	cii := func(note string) string {
		return xrWith(t, minimalXRechnungCII, xrCIIAtSettle,
			"<SpecifiedTradePaymentTerms><Description>"+note+"</Description></SpecifiedTradePaymentTerms>")
	}
	const good = "#SKONTO#TAGE=14#PROZENT=2.00#&#10;"
	var out []xrCase
	for _, b := range []struct {
		syntax string
		fn     func(string) string
	}{{"UBL", ubl}, {"CII", cii}} {
		out = append(out,
			// A payment term with no discount line at all: the quantifier is empty and
			// the rule is vacuously satisfied, which is the common case.
			xrCase{b.syntax + " plain payment terms (BR-DE-18)", b.fn("Zahlbar sofort ohne Abzug."), "BR-DE-18", false, 0},
			xrCase{b.syntax + " correct skonto (BR-DE-18)", b.fn(good), "BR-DE-18", false, 0},
			xrCase{b.syntax + " skonto with a base amount (BR-DE-18)",
				b.fn("#SKONTO#TAGE=14#PROZENT=2.00#BASISBETRAG=100.00#&#10;"), "BR-DE-18", false, 0},
			// The five ways KoSIT's own fixtures break it.
			xrCase{b.syntax + " skonto with no trailing newline (BR-DE-18)",
				b.fn("#SKONTO#TAGE=14#PROZENT=2.00#"), "BR-DE-18", true, SeverityFatal},
			xrCase{b.syntax + " skonto in lower case (BR-DE-18)",
				b.fn("#Skonto#TAGE=14#PROZENT=2.00#&#10;"), "BR-DE-18", true, SeverityFatal},
			xrCase{b.syntax + " skonto with one decimal (BR-DE-18)",
				b.fn("#SKONTO#TAGE=14#PROZENT=2.0#&#10;"), "BR-DE-18", true, SeverityFatal},
			xrCase{b.syntax + " skonto with a comma separator (BR-DE-18)",
				b.fn("#SKONTO#TAGE=14#PROZENT=2,00#&#10;"), "BR-DE-18", true, SeverityFatal},
			xrCase{b.syntax + " skonto with extra whitespace (BR-DE-18)",
				b.fn("#SKONTO# TAGE=14#PROZENT=2.00#&#10;"), "BR-DE-18", true, SeverityFatal},
		)
	}
	return out
}

// xrPaymentMeansCases covers the three payment-means groups (BR-DE-23/24/25), the
// two IBAN rules (BR-DE-19/20) and the direct-debit pair (BR-DE-30/31).
func xrPaymentMeansCases(t *testing.T) []xrCase {
	ublPM := func(pm string) string { return mutate(t, minimalXRechnungUBL, xrUBLPaymentMns, pm) }
	ciiPM := func(pm string) string { return mutate(t, minimalXRechnungCII, xrCIIPaymentMns, pm) }
	const ublCard = `<cac:CardAccount><cbc:PrimaryAccountNumberID>1234</cbc:PrimaryAccountNumberID><cbc:NetworkID>n</cbc:NetworkID></cac:CardAccount>`
	const ublAccount = `<cac:PayeeFinancialAccount><cbc:ID>DE75512108001245126199</cbc:ID></cac:PayeeFinancialAccount>`
	const ublMandate = `<cac:PaymentMandate><cbc:ID>M-1</cbc:ID><cac:PayerFinancialAccount><cbc:ID>DE75512108001245126199</cbc:ID></cac:PayerFinancialAccount></cac:PaymentMandate>`
	const ublSEPA = `<cac:PartyIdentification><cbc:ID schemeID="SEPA">DE98ZZZ09999999999</cbc:ID></cac:PartyIdentification>`
	ublMeans := func(code, body string) string {
		return ublPM(`<cac:PaymentMeans><cbc:PaymentMeansCode>` + code + `</cbc:PaymentMeansCode>` + body + `</cac:PaymentMeans>`)
	}
	const ciiCard = `<ApplicableTradeSettlementFinancialCard><ID>1234</ID></ApplicableTradeSettlementFinancialCard>`
	const ciiAccount = `<PayeePartyCreditorFinancialAccount><IBANID>DE75512108001245126199</IBANID></PayeePartyCreditorFinancialAccount>`
	const ciiDebtor = `<PayerPartyDebtorFinancialAccount><IBANID>DE75512108001245126199</IBANID></PayerPartyDebtorFinancialAccount>`
	ciiMeans := func(code, body string) string {
		return ciiPM(`<SpecifiedTradeSettlementPaymentMeans><TypeCode>` + code + `</TypeCode>` + body + `</SpecifiedTradeSettlementPaymentMeans>`)
	}
	// BT-90 in CII is ram:CreditorReferenceID, a sibling of the payment means.
	ciiWithCreditor := func(doc string) string {
		return xrWith(t, doc, xrCIIAtSettle, "<CreditorReferenceID>DE98ZZZ09999999999</CreditorReferenceID>")
	}

	return []xrCase{
		// BR-DE-23: a credit-transfer code requires BG-17 and forbids BG-18/BG-19.
		{"UBL credit transfer without BG-17 (BR-DE-23-a)", ublMeans("58", ""), "BR-DE-23-a", true, SeverityFatal},
		{"UBL credit transfer with BG-17 (BR-DE-23-a)", ublMeans("58", ublAccount), "BR-DE-23-a", false, 0},
		{"UBL code 30 without BG-17 (BR-DE-23-a)", ublMeans("30", ""), "BR-DE-23-a", true, SeverityFatal},
		{"UBL credit transfer with a card (BR-DE-23-b)", ublMeans("58", ublAccount+ublCard), "BR-DE-23-b", true, SeverityFatal},
		{"UBL credit transfer without a card (BR-DE-23-b)", ublMeans("58", ublAccount), "BR-DE-23-b", false, 0},
		{"CII credit transfer without BG-17 (BR-DE-23-a)", ciiMeans("58", ""), "BR-DE-23-a", true, SeverityFatal},
		{"CII credit transfer with BG-17 (BR-DE-23-a)", ciiMeans("58", ciiAccount), "BR-DE-23-a", false, 0},
		{"CII credit transfer with a card (BR-DE-23-b)", ciiMeans("58", ciiAccount+ciiCard), "BR-DE-23-b", true, SeverityFatal},
		{"CII credit transfer without a card (BR-DE-23-b)", ciiMeans("58", ciiAccount), "BR-DE-23-b", false, 0},

		// BR-DE-24: a card code requires BG-18 and forbids BG-17/BG-19.
		{"UBL card payment without BG-18 (BR-DE-24-a)", ublMeans("48", ""), "BR-DE-24-a", true, SeverityFatal},
		{"UBL card payment with BG-18 (BR-DE-24-a)", ublMeans("48", ublCard), "BR-DE-24-a", false, 0},
		{"UBL card payment with an account (BR-DE-24-b)", ublMeans("54", ublCard+ublAccount), "BR-DE-24-b", true, SeverityFatal},
		{"UBL card payment without an account (BR-DE-24-b)", ublMeans("55", ublCard), "BR-DE-24-b", false, 0},
		{"CII card payment without BG-18 (BR-DE-24-a)", ciiMeans("48", ""), "BR-DE-24-a", true, SeverityFatal},
		{"CII card payment with BG-18 (BR-DE-24-a)", ciiMeans("48", ciiCard), "BR-DE-24-a", false, 0},
		{"CII card payment with an account (BR-DE-24-b)", ciiMeans("54", ciiCard+ciiAccount), "BR-DE-24-b", true, SeverityFatal},
		{"CII card payment without an account (BR-DE-24-b)", ciiMeans("55", ciiCard), "BR-DE-24-b", false, 0},

		// BR-DE-25: a direct-debit code requires BG-19 and forbids BG-17/BG-18. In
		// CII BG-19 is the reconstructed group, so the conforming case needs BT-90
		// alongside BT-91 or BR-DE-30 answers instead.
		{"UBL direct debit without BG-19 (BR-DE-25-a)", ublMeans("59", ""), "BR-DE-25-a", true, SeverityFatal},
		{"UBL direct debit with BG-19 (BR-DE-25-a)",
			xrWith(t, ublMeans("59", ublMandate), xrUBLAtSeller, ublSEPA), "BR-DE-25-a", false, 0},
		{"UBL direct debit with an account (BR-DE-25-b)",
			xrWith(t, ublMeans("59", ublMandate+ublAccount), xrUBLAtSeller, ublSEPA), "BR-DE-25-b", true, SeverityFatal},
		{"UBL direct debit without an account (BR-DE-25-b)",
			xrWith(t, ublMeans("59", ublMandate), xrUBLAtSeller, ublSEPA), "BR-DE-25-b", false, 0},
		{"CII direct debit without BG-19 (BR-DE-25-a)", ciiMeans("59", ""), "BR-DE-25-a", true, SeverityFatal},
		{"CII direct debit with BG-19 (BR-DE-25-a)", ciiWithCreditor(ciiMeans("59", ciiDebtor)), "BR-DE-25-a", false, 0},
		{"CII direct debit with an account (BR-DE-25-b)",
			ciiWithCreditor(ciiMeans("59", ciiDebtor+ciiAccount)), "BR-DE-25-b", true, SeverityFatal},
		{"CII direct debit with a financial institution (BR-DE-25-b)",
			ciiWithCreditor(ciiMeans("59", ciiDebtor+`<PayeeSpecifiedCreditorFinancialInstitution><BICID>DEUTDEFF</BICID></PayeeSpecifiedCreditorFinancialInstitution>`)),
			"BR-DE-25-b", true, SeverityFatal},
		{"CII direct debit without an account (BR-DE-25-b)", ciiWithCreditor(ciiMeans("59", ciiDebtor)), "BR-DE-25-b", false, 0},

		// BR-DE-19/20, the two IBAN rules KoSIT flags warning. The IBAN below has a
		// wrong check digit and matches the shape, which is the case KoSIT's own
		// fixture was added for.
		{"UBL bad credit-transfer IBAN (BR-DE-19)",
			ublMeans("58", `<cac:PayeeFinancialAccount><cbc:ID>DE65512108001245126199</cbc:ID></cac:PayeeFinancialAccount>`),
			"BR-DE-19", true, SeverityWarning},
		{"UBL good credit-transfer IBAN (BR-DE-19)", ublMeans("58", ublAccount), "BR-DE-19", false, 0},
		// BR-DE-20 reads the mandate's account and not the payee's, which is the
		// distinction KoSIT's github issue #31 fixture pins.
		{"UBL bad direct-debit IBAN (BR-DE-20)",
			xrWith(t, ublMeans("59", `<cac:PaymentMandate><cbc:ID>M-1</cbc:ID><cac:PayerFinancialAccount><cbc:ID>DE65512108001245126199</cbc:ID></cac:PayerFinancialAccount></cac:PaymentMandate>`),
				xrUBLAtSeller, ublSEPA),
			"BR-DE-20", true, SeverityWarning},
		{"UBL good direct-debit IBAN beside a bad payee one (BR-DE-20)",
			xrWith(t, ublMeans("59", `<cac:PayeeFinancialAccount><cbc:ID>DE65512108001245126199</cbc:ID></cac:PayeeFinancialAccount>`+ublMandate),
				xrUBLAtSeller, ublSEPA),
			"BR-DE-20", false, 0},
		{"CII bad credit-transfer IBAN (BR-DE-19)",
			ciiMeans("58", `<PayeePartyCreditorFinancialAccount><IBANID>DE65512108001245126199</IBANID></PayeePartyCreditorFinancialAccount>`),
			"BR-DE-19", true, SeverityWarning},
		{"CII bad direct-debit IBAN (BR-DE-20)",
			ciiWithCreditor(ciiMeans("59", `<PayerPartyDebtorFinancialAccount><IBANID>DE65512108001245126199</IBANID></PayerPartyDebtorFinancialAccount>`)),
			"BR-DE-20", true, SeverityWarning},

		// BR-DE-30/31. BT-90 in UBL is a party identifier with the SEPA scheme, not
		// the mandate's own identifier, so a mandate with a mandate reference and no
		// SEPA party identifier is the violating case.
		{"UBL mandate without a SEPA creditor identifier (BR-DE-30)", ublMeans("59", ublMandate), "BR-DE-30", true, SeverityFatal},
		{"UBL mandate with a SEPA creditor identifier (BR-DE-30)",
			xrWith(t, ublMeans("59", ublMandate), xrUBLAtSeller, ublSEPA), "BR-DE-30", false, 0},
		{"UBL mandate without a debited account (BR-DE-31)",
			xrWith(t, ublMeans("59", `<cac:PaymentMandate><cbc:ID>M-1</cbc:ID></cac:PaymentMandate>`), xrUBLAtSeller, ublSEPA),
			"BR-DE-31", true, SeverityFatal},
		{"UBL mandate with a debited account (BR-DE-31)",
			xrWith(t, ublMeans("59", ublMandate), xrUBLAtSeller, ublSEPA), "BR-DE-31", false, 0},
		// The CII case the model could not express: BT-90 alone is a BG-19, and it is
		// missing BT-91.
		// BT-90 alone is a BG-19 that is missing BT-91, and it also trips BR-DE-30 —
		// whose message names BT-90 while its XPath is
		// "((BT-89 or BT-91) and BT-90) or no BG-19", so a creditor identifier with
		// neither a mandate nor a debited account fails the rule that asks for it.
		// The XPath is the rule.
		{"CII creditor identifier alone (BR-DE-31)", ciiWithCreditor(minimalXRechnungCII), "BR-DE-31", true, SeverityFatal},
		{"CII creditor identifier alone (BR-DE-30)", ciiWithCreditor(minimalXRechnungCII), "BR-DE-30", true, SeverityFatal},
		{"CII debited account without a creditor identifier (BR-DE-30)",
			ciiMeans("59", ciiDebtor), "BR-DE-30", true, SeverityFatal},
	}
}

// xrMandatoryTermCases covers the rules whose severity this change corrected, and
// the three mandatory-term rules the existing table in xrechnung_test.go does not
// reach.
func xrMandatoryTermCases(t *testing.T) []xrCase {
	ubl := func(from, to string) string { return mutate(t, minimalXRechnungUBL, from, to) }
	cii := func(from, to string) string { return mutate(t, minimalXRechnungCII, from, to) }
	ublDeliverTo := func(address string) string {
		return xrWith(t, minimalXRechnungUBL, xrUBLAtDelivery,
			`<cac:DeliveryLocation><cac:Address>`+address+
				`<cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation>`)
	}
	const attach = `<cac:AdditionalDocumentReference><cbc:ID>A</cbc:ID><cac:Attachment>` +
		`<cbc:EmbeddedDocumentBinaryObject mimeCode="application/pdf" filename="a.pdf">eA==</cbc:EmbeddedDocumentBinaryObject>` +
		`</cac:Attachment></cac:AdditionalDocumentReference>`
	return []xrCase{
		// The seven KoSIT flags warning and this package reported fatal.
		{"UBL bad type code (BR-DE-17)", ubl("<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>",
			"<cbc:InvoiceTypeCode>325</cbc:InvoiceTypeCode>"), "BR-DE-17", true, SeverityWarning},
		{"CII bad type code (BR-DE-17)", cii("<TypeCode>380</TypeCode>", "<TypeCode>325</TypeCode>"), "BR-DE-17", true, SeverityWarning},
		{"UBL foreign specification identifier (BR-DE-21)", ubl(xrCIUSID, "urn:cen.eu:en16931:2017"), "BR-DE-21", true, SeverityWarning},
		{"CII foreign specification identifier (BR-DE-21)", cii(xrCIUSID, "urn:cen.eu:en16931:2017"), "BR-DE-21", true, SeverityWarning},
		{"UBL corrected invoice without BG-3 (BR-DE-26)", ubl("<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>",
			"<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>"), "BR-DE-26", true, SeverityWarning},
		{"UBL telephone with two digits (BR-DE-27)", ubl("<cbc:Telephone>012 3456789</cbc:Telephone>",
			"<cbc:Telephone>ab 12</cbc:Telephone>"), "BR-DE-27", true, SeverityWarning},
		{"UBL email with two @ (BR-DE-28)", ubl("<cbc:ElectronicMail>tim@test.de</cbc:ElectronicMail>",
			"<cbc:ElectronicMail>tim@@test.de</cbc:ElectronicMail>"), "BR-DE-28", true, SeverityWarning},
		{"CII telephone with two digits (BR-DE-27)", cii("<CompleteNumber>012 3456789</CompleteNumber>",
			"<CompleteNumber>ab 12</CompleteNumber>"), "BR-DE-27", true, SeverityWarning},
		{"CII email with two @ (BR-DE-28)", cii("<URIID>tim@test.de</URIID>", "<URIID>tim@@test.de</URIID>"), "BR-DE-28", true, SeverityWarning},

		// BR-DE-22, and the two mandatory terms the UBL fixture's shape reaches only
		// in the other binding.
		{"UBL two attachments with one file name (BR-DE-22)",
			xrBefore(t, minimalXRechnungUBL, xrUBLAtBody, attach+attach), "BR-DE-22", true, SeverityFatal},
		{"UBL one attachment (BR-DE-22)", xrBefore(t, minimalXRechnungUBL, xrUBLAtBody, attach), "BR-DE-22", false, 0},
		{"CII no buyer reference (BR-DE-15)", cii(xrCIIAtAgree, ""), "BR-DE-15", true, SeverityFatal},
		{"CII no payment instructions (BR-DE-1)", cii(xrCIIPaymentMns, ""), "BR-DE-1", true, SeverityFatal},
		{"CII no seller contact (BR-DE-2)", cii(xrCIIContact, ""), "BR-DE-2", true, SeverityFatal},
		{"CII no seller city (BR-DE-3)", cii("<CityName>Berlin</CityName>", ""), "BR-DE-3", true, SeverityFatal},
		{"CII no seller post code (BR-DE-4)", cii("<PostcodeCode>10115</PostcodeCode>", ""), "BR-DE-4", true, SeverityFatal},
		{"CII no seller contact point (BR-DE-5)", cii(xrCIIAtContact, ""), "BR-DE-5", true, SeverityFatal},
		{"CII no seller telephone (BR-DE-6)",
			cii("<TelephoneUniversalCommunication><CompleteNumber>012 3456789</CompleteNumber></TelephoneUniversalCommunication>", ""),
			"BR-DE-6", true, SeverityFatal},
		{"CII no seller email (BR-DE-7)",
			cii("<EmailURIUniversalCommunication><URIID>tim@test.de</URIID></EmailURIUniversalCommunication>", ""),
			"BR-DE-7", true, SeverityFatal},
		{"CII no buyer city (BR-DE-8)", cii("<CityName>Bonn</CityName>", ""), "BR-DE-8", true, SeverityFatal},
		{"CII no buyer post code (BR-DE-9)", cii("<PostcodeCode>53113</PostcodeCode>", ""), "BR-DE-9", true, SeverityFatal},
		// BG-15 goes inside the cac:Delivery the fixture already carries: a second
		// one would trip UBL-SR-24 and leave the mapper reading the first.
		{"UBL deliver-to without a city (BR-DE-10)", ublDeliverTo(`<cbc:PostalZone>50667</cbc:PostalZone>`), "BR-DE-10", true, SeverityFatal},
		{"UBL deliver-to without a post code (BR-DE-11)", ublDeliverTo(`<cbc:CityName>Koeln</cbc:CityName>`), "BR-DE-11", true, SeverityFatal},
		{"UBL deliver-to with both (BR-DE-10)",
			ublDeliverTo(`<cbc:CityName>Koeln</cbc:CityName><cbc:PostalZone>50667</cbc:PostalZone>`), "BR-DE-10", false, 0},
		{"UBL VAT breakdown without a rate (BR-DE-14)",
			ubl(`<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`,
				`<cac:TaxCategory><cbc:ID>S</cbc:ID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`),
			"BR-DE-14", true, SeverityFatal},
		{"UBL taxed category without a seller VAT identifier (BR-DE-16)",
			ubl(`<cac:PartyTaxScheme><cbc:CompanyID>DE123456789</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>`, ""),
			"BR-DE-16", true, SeverityFatal},
	}
}

// xrExtensionCases covers the fifteen BR-DEX-* rules.
func xrExtensionCases(t *testing.T) []xrCase {
	ublExt := asExtension(minimalXRechnungUBL)
	ciiExt := asExtension(minimalXRechnungCII)
	// A third-party payment group, which is what BG-DEX-09 is and what BR-DEX-09
	// adds to the amount-due formula. 119.00 + 10.00 = 129.00.
	prepaid := func(id, amount, currency, descr string) string {
		return `<cac:PrepaidPayment><cbc:ID>` + id + `</cbc:ID><cbc:PaidAmount currencyID="` + currency + `">` + amount +
			`</cbc:PaidAmount><cbc:InstructionID>` + descr + `</cbc:InstructionID></cac:PrepaidPayment>`
	}
	// mutate is no use here: half these cases leave BT-115 at its baseline value,
	// and a replacement that changes nothing is a failure to that helper rather
	// than the identity it is here.
	withPrepaid := func(pp, payable string) string {
		const from = "<cbc:PayableAmount>119.00</cbc:PayableAmount>"
		if !strings.Contains(ublExt, from) {
			t.Fatalf("fixture does not contain %q", from)
		}
		doc := strings.Replace(ublExt, from, "<cbc:PayableAmount>"+payable+"</cbc:PayableAmount>", 1)
		return xrBefore(t, doc, xrUBLAtBody, pp)
	}
	// Two sub invoice lines that add up to their parent, each with the one VAT
	// group BR-DEX-03 requires.
	subLine := func(id, amount, taxCategory string) string {
		return `<cac:SubInvoiceLine><cbc:ID>` + id + `</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>` +
			`<cbc:LineExtensionAmount>` + amount + `</cbc:LineExtensionAmount>` +
			`<cac:Item><cbc:Name>Part</cbc:Name>` + taxCategory + `</cac:Item>` +
			`<cac:Price><cbc:PriceAmount>` + amount + `</cbc:PriceAmount></cac:Price></cac:SubInvoiceLine>`
	}
	const subTax = `<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory>`
	withSubLines := func(a, b, tax string) string {
		return mutate(t, ublExt, `<cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price>`,
			`<cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price>`+subLine("1.1", a, tax)+subLine("1.2", b, tax))
	}
	ublAttach := func(mime string) string {
		return xrBefore(t, ublExt, xrUBLAtBody, `<cac:AdditionalDocumentReference><cbc:ID>A</cbc:ID><cac:Attachment>`+
			`<cbc:EmbeddedDocumentBinaryObject mimeCode="`+mime+`" filename="a.bin">eA==</cbc:EmbeddedDocumentBinaryObject>`+
			`</cac:Attachment></cac:AdditionalDocumentReference>`)
	}
	ciiAttach := func(mime string) string {
		return xrWith(t, ciiExt, xrCIIAtAgree, `<AdditionalReferencedDocument><IssuerAssignedID>A</IssuerAssignedID>`+
			`<TypeCode>916</TypeCode><AttachmentBinaryObject mimeCode="`+mime+`" filename="a.bin">eA==</AttachmentBinaryObject>`+
			`</AdditionalReferencedDocument>`)
	}
	ublScheme := func(scheme string) string {
		return xrWith(t, ublExt, xrUBLAtSeller, `<cac:PartyIdentification><cbc:ID schemeID="`+scheme+`">X</cbc:ID></cac:PartyIdentification>`)
	}
	ciiGlobalID := func(scheme string) string {
		return xrWith(t, ciiExt, `<SellerTradeParty><Name>Seller Co</Name>`,
			`<GlobalID schemeID="`+scheme+`">X</GlobalID>`)
	}
	ublEndpoint := func(scheme string) string {
		return xrWith(t, ublExt, xrUBLAtSeller, `<cbc:EndpointID schemeID="`+scheme+`">x@y.de</cbc:EndpointID>`)
	}
	ublItemScheme := func(scheme string) string {
		return xrWith(t, ublExt, xrUBLAtItem,
			`<cac:StandardItemIdentification><cbc:ID schemeID="`+scheme+`">X</cbc:ID></cac:StandardItemIdentification>`)
	}
	ublDeliverTo := func(scheme string) string {
		return xrBefore(t, ublExt, xrUBLAtBody, `<cac:Delivery><cac:DeliveryLocation><cbc:ID schemeID="`+scheme+`">L</cbc:ID>`+
			`<cac:Address><cbc:CityName>Koeln</cbc:CityName><cbc:PostalZone>50667</cbc:PostalZone>`+
			`<cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>`)
	}
	ublLegal := func(scheme string) string {
		return mutate(t, ublExt, `<cac:PartyLegalEntity><cbc:RegistrationName>Seller Ltd</cbc:RegistrationName></cac:PartyLegalEntity>`,
			`<cac:PartyLegalEntity><cbc:RegistrationName>Seller Ltd</cbc:RegistrationName><cbc:CompanyID schemeID="`+scheme+`">L</cbc:CompanyID></cac:PartyLegalEntity>`)
	}

	return []xrCase{
		// BR-DEX-01: the EXTENSION's MIME list is the EN 16931 one plus
		// application/xml, and BR-CL-24 is suppressed in its favour.
		{"UBL extension attachment as XML (BR-DEX-01)", ublAttach("application/xml"), "BR-DEX-01", false, 0},
		{"UBL extension attachment with a refused MIME code (BR-DEX-01)", ublAttach("application/zip"), "BR-DEX-01", true, SeverityFatal},
		{"CII extension attachment as XML (BR-DEX-01)", ciiAttach("application/xml"), "BR-DEX-01", false, 0},
		{"CII extension attachment with a refused MIME code (BR-DEX-01)", ciiAttach("application/zip"), "BR-DEX-01", true, SeverityFatal},

		// BR-DEX-02 and BR-DEX-03, the sub-invoice-line rules. Both are UBL-only and
		// their context names ubl:Invoice, so neither can reach a credit note.
		{"UBL sub invoice lines that sum (BR-DEX-02)", withSubLines("60.00", "40.00", subTax), "BR-DEX-02", false, 0},
		{"UBL sub invoice lines that do not sum (BR-DEX-02)", withSubLines("60.00", "30.00", subTax), "BR-DEX-02", true, SeverityWarning},
		{"UBL sub invoice line with one VAT group (BR-DEX-03)", withSubLines("60.00", "40.00", subTax), "BR-DEX-03", false, 0},
		{"UBL sub invoice line with no VAT group (BR-DEX-03)", withSubLines("60.00", "40.00", ""), "BR-DEX-03", true, SeverityFatal},

		// BR-DEX-04..08, the five code lists the EXTENSION widens with XR01..XR03.
		{"UBL extension party scheme XR01 (BR-DEX-04)", ublScheme("XR01"), "BR-DEX-04", false, 0},
		{"UBL extension party scheme 0088 (BR-DEX-04)", ublScheme("0088"), "BR-DEX-04", false, 0},
		{"UBL extension party scheme XR11 (BR-DEX-04)", ublScheme("XR11"), "BR-DEX-04", true, SeverityFatal},
		{"UBL extension SEPA creditor identifier (BR-DEX-04)", ublScheme("SEPA"), "BR-DEX-04", false, 0},
		{"CII extension party scheme XR01 (BR-DEX-04)", ciiGlobalID("XR01"), "BR-DEX-04", false, 0},
		{"CII extension party scheme 0321 (BR-DEX-04)", ciiGlobalID("0321"), "BR-DEX-04", true, SeverityFatal},
		// The CII binding has no SEPA arm, so the scheme UBL permits on a party
		// identifier is refused here. That asymmetry is KoSIT's.
		{"CII extension SEPA scheme (BR-DEX-04)", ciiGlobalID("SEPA"), "BR-DEX-04", true, SeverityFatal},
		{"UBL extension legal scheme XR02 (BR-DEX-05)", ublLegal("XR02"), "BR-DEX-05", false, 0},
		{"UBL extension legal scheme XR22 (BR-DEX-05)", ublLegal("XR22"), "BR-DEX-05", true, SeverityFatal},
		{"UBL extension item scheme XR03 (BR-DEX-06)", ublItemScheme("XR03"), "BR-DEX-06", false, 0},
		{"UBL extension item scheme XR33 (BR-DEX-06)", ublItemScheme("XR33"), "BR-DEX-06", true, SeverityFatal},
		{"UBL extension endpoint scheme XR01 (BR-DEX-07)", ublEndpoint("XR01"), "BR-DEX-07", false, 0},
		// 0219 is in KoSIT's copy of the CEF EAS list and not in the one CEN's
		// BR-CL-25 draws on, and this is the case that says so.
		{"UBL extension endpoint scheme 0219 (BR-DEX-07)", ublEndpoint("0219"), "BR-DEX-07", false, 0},
		{"UBL extension endpoint scheme 0000 (BR-DEX-07)", ublEndpoint("0000"), "BR-DEX-07", true, SeverityFatal},
		{"UBL extension deliver-to scheme XR01 (BR-DEX-08)", ublDeliverTo("XR01"), "BR-DEX-08", false, 0},
		{"UBL extension deliver-to scheme XR88 (BR-DEX-08)", ublDeliverTo("XR88"), "BR-DEX-08", true, SeverityFatal},

		// BR-DEX-09..14, the third-party payment group. BR-DEX-09 replaces BR-CO-16
		// and adds the sum: 119.00 + 10.00 = 129.00.
		{"UBL third-party payment in the amount due (BR-DEX-09)",
			withPrepaid(prepaid("card", "10.00", "EUR", "Mobiles Bezahlen"), "129.00"), "BR-DEX-09", false, 0},
		{"UBL third-party payment left out of the amount due (BR-DEX-09)",
			withPrepaid(prepaid("card", "10.00", "EUR", "Mobiles Bezahlen"), "119.00"), "BR-DEX-09", true, SeverityFatal},
		{"UBL third-party payment without a type (BR-DEX-10)",
			withPrepaid(`<cac:PrepaidPayment><cbc:PaidAmount currencyID="EUR">10.00</cbc:PaidAmount><cbc:InstructionID>d</cbc:InstructionID></cac:PrepaidPayment>`, "129.00"),
			"BR-DEX-10", true, SeverityFatal},
		{"UBL third-party payment with a type (BR-DEX-10)",
			withPrepaid(prepaid("card", "10.00", "EUR", "d"), "129.00"), "BR-DEX-10", false, 0},
		{"UBL third-party payment without an amount (BR-DEX-11)",
			withPrepaid(`<cac:PrepaidPayment><cbc:ID>card</cbc:ID><cbc:InstructionID>d</cbc:InstructionID></cac:PrepaidPayment>`, "119.00"),
			"BR-DEX-11", true, SeverityFatal},
		{"UBL third-party payment with an amount (BR-DEX-11)",
			withPrepaid(prepaid("card", "10.00", "EUR", "d"), "129.00"), "BR-DEX-11", false, 0},
		{"UBL third-party payment without a description (BR-DEX-12)",
			withPrepaid(`<cac:PrepaidPayment><cbc:ID>card</cbc:ID><cbc:PaidAmount currencyID="EUR">10.00</cbc:PaidAmount></cac:PrepaidPayment>`, "129.00"),
			"BR-DEX-12", true, SeverityFatal},
		{"UBL third-party payment with a description (BR-DEX-12)",
			withPrepaid(prepaid("card", "10.00", "EUR", "d"), "129.00"), "BR-DEX-12", false, 0},
		{"UBL third-party payment with three decimals (BR-DEX-13)",
			withPrepaid(prepaid("card", "10.000", "EUR", "d"), "129.00"), "BR-DEX-13", true, SeverityFatal},
		{"UBL third-party payment with two decimals (BR-DEX-13)",
			withPrepaid(prepaid("card", "10.00", "EUR", "d"), "129.00"), "BR-DEX-13", false, 0},
		{"UBL third-party payment in another currency (BR-DEX-14)",
			withPrepaid(prepaid("card", "10.00", "USD", "d"), "129.00"), "BR-DEX-14", true, SeverityFatal},
		{"UBL third-party payment in the invoice currency (BR-DEX-14)",
			withPrepaid(prepaid("card", "10.00", "EUR", "d"), "129.00"), "BR-DEX-14", false, 0},

		// BR-DEX-15: CII has no sub invoice lines, so a ram:ParentLineID in an
		// EXTENSION document is reported. KoSIT flags it warning.
		{"CII extension with a parent line reference (BR-DEX-15)",
			xrWith(t, ciiExt, `<AssociatedDocumentLineDocument><LineID>1</LineID>`, "<ParentLineID>0</ParentLineID>"),
			"BR-DEX-15", true, SeverityWarning},
		{"CII extension without a parent line reference (BR-DEX-15)", ciiExt, "BR-DEX-15", false, 0},
	}
}

// xrCVDCases covers the seven BR-DE-CVD-* rules and BR-TMP-CVD-01.
func xrCVDCases(t *testing.T) []xrCase {
	ubl, cii := xrCVDUBL(t), xrCVDCII(t)
	dropUBL := func(from string) string { return mutate(t, ubl, from, "") }
	dropCII := func(from string) string { return mutate(t, cii, from, "") }
	return []xrCase{
		{"UBL CVD without a contract reference (BR-DE-CVD-01)",
			dropUBL(`<cac:ContractDocumentReference><cbc:ID>C-1</cbc:ID></cac:ContractDocumentReference>`), "BR-DE-CVD-01", true, SeverityFatal},
		{"UBL CVD without a tender reference (BR-DE-CVD-02)",
			dropUBL(`<cac:OriginatorDocumentReference><cbc:ID>T-1</cbc:ID></cac:OriginatorDocumentReference>`), "BR-DE-CVD-02", true, SeverityFatal},
		{"CII CVD without a contract reference (BR-DE-CVD-01)",
			dropCII(`<ContractReferencedDocument><IssuerAssignedID>C-1</IssuerAssignedID></ContractReferencedDocument>`), "BR-DE-CVD-01", true, SeverityFatal},
		{"CII CVD without a tender reference (BR-DE-CVD-02)",
			dropCII(`<AdditionalReferencedDocument><IssuerAssignedID>T-1</IssuerAssignedID><TypeCode>50</TypeCode></AdditionalReferencedDocument>`),
			"BR-DE-CVD-02", true, SeverityFatal},

		// BR-DE-CVD-03: no line carries the pair at all. Removing the attribute takes
		// BR-DE-CVD-06-a with it, which is why each case names its own rule.
		{"UBL CVD with no clean-vehicle line (BR-DE-CVD-03)",
			mutate(t, ubl, xrUBLCVDItem("M1", "clean"), ""), "BR-DE-CVD-03", true, SeverityFatal},
		{"CII CVD with no clean-vehicle line (BR-DE-CVD-03)",
			mutate(t, cii, xrCIICVDItem("M1", "clean"), ""), "BR-DE-CVD-03", true, SeverityFatal},

		// BR-DE-CVD-04 and BR-TMP-CVD-01 share the classification element: the first
		// is about its value, the second about its scheme.
		{"UBL CVD with an unknown vehicle category (BR-DE-CVD-04)",
			mutate(t, ubl, ">M1<", ">M9<"), "BR-DE-CVD-04", true, SeverityFatal},
		{"CII CVD with an unknown vehicle category (BR-DE-CVD-04)",
			mutate(t, cii, ">M1<", ">M9<"), "BR-DE-CVD-04", true, SeverityFatal},
		{"UBL CVD with a classification scheme outside UNTDID 7143 (BR-TMP-CVD-01)",
			mutate(t, ubl, `listID="CVD"`, `listID="ZZ9"`), "BR-TMP-CVD-01", true, SeverityFatal},
		{"UBL CVD with a UNTDID 7143 classification scheme (BR-TMP-CVD-01)",
			mutate(t, ubl, `listID="CVD"`, `listID="MP"`), "BR-TMP-CVD-01", false, 0},
		{"CII CVD with a classification scheme outside UNTDID 7143 (BR-TMP-CVD-01)",
			mutate(t, cii, `listID="CVD"`, `listID="ZZ9"`), "BR-TMP-CVD-01", true, SeverityFatal},

		// BR-DE-CVD-05: the attribute value is one of three codes.
		{"UBL CVD with an unknown clean-vehicle value (BR-DE-CVD-05)",
			mutate(t, ubl, "<cbc:Value>clean</cbc:Value>", "<cbc:Value>dirty</cbc:Value>"), "BR-DE-CVD-05", true, SeverityFatal},
		{"CII CVD with an unknown clean-vehicle value (BR-DE-CVD-05)",
			mutate(t, cii, "<Value>clean</Value>", "<Value>dirty</Value>"), "BR-DE-CVD-05", true, SeverityFatal},

		// BR-DE-CVD-06-a and -06-b are the two halves of "one 'CVD' scheme and one
		// 'cva' attribute per line, or neither".
		{"UBL CVD classification without the attribute (BR-DE-CVD-06-a)",
			mutate(t, ubl, `<cac:AdditionalItemProperty><cbc:Name>cva</cbc:Name><cbc:Value>clean</cbc:Value></cac:AdditionalItemProperty>`, ""),
			"BR-DE-CVD-06-a", true, SeverityFatal},
		{"UBL CVD attribute without the classification (BR-DE-CVD-06-b)",
			mutate(t, ubl, `<cac:CommodityClassification><cbc:ItemClassificationCode listID="CVD">M1</cbc:ItemClassificationCode></cac:CommodityClassification>`, ""),
			"BR-DE-CVD-06-b", true, SeverityFatal},
		{"CII CVD classification without the attribute (BR-DE-CVD-06-a)",
			mutate(t, cii, `<ApplicableProductCharacteristic><Description>cva</Description><Value>clean</Value></ApplicableProductCharacteristic>`, ""),
			"BR-DE-CVD-06-a", true, SeverityFatal},
		{"CII CVD attribute without the classification (BR-DE-CVD-06-b)",
			mutate(t, cii, `<DesignatedProductClassification><ClassCode listID="CVD">M1</ClassCode></DesignatedProductClassification>`, ""),
			"BR-DE-CVD-06-b", true, SeverityFatal},
	}
}

// xrProvisionalCases covers BR-DE-TMP-32, BR-TMP-2 and BR-TMP-3.
func xrProvisionalCases(t *testing.T) []xrCase {
	noUBLDate := mutate(t, minimalXRechnungUBL, xrUBLDelivery, "")
	noCIIDate := mutate(t, minimalXRechnungCII, xrCIIDelivery, "")
	ublURI := func(uri string) string {
		return xrBefore(t, minimalXRechnungUBL, xrUBLAtBody,
			`<cac:AdditionalDocumentReference><cbc:ID>A</cbc:ID><cac:Attachment><cac:ExternalReference>`+
				`<cbc:URI>`+uri+`</cbc:URI></cac:ExternalReference></cac:Attachment></cac:AdditionalDocumentReference>`)
	}
	ciiURI := func(uri string) string {
		return xrWith(t, minimalXRechnungCII, xrCIIAtAgree,
			`<AdditionalReferencedDocument><IssuerAssignedID>A</IssuerAssignedID><TypeCode>916</TypeCode>`+
				`<URIID>`+uri+`</URIID></AdditionalReferencedDocument>`)
	}
	ciiPrices := func(gross, net string) string {
		return mutate(t, minimalXRechnungCII,
			`<SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>`,
			`<SpecifiedLineTradeAgreement>`+
				`<GrossPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount>`+gross+`</GrossPriceProductTradePrice>`+
				`<NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount>`+net+`</NetPriceProductTradePrice>`+
				`</SpecifiedLineTradeAgreement>`)
	}
	return []xrCase{
		// BR-DE-TMP-32, flag="information": one of BT-72, BG-14 or a period on every
		// line. The three arms are separate cases because the rule is a disjunction
		// and a fixture that satisfies two of them would not say which one worked.
		{"UBL with no delivery date or period (BR-DE-TMP-32)", noUBLDate, "BR-DE-TMP-32", true, SeverityWarning},
		{"UBL with a delivery date (BR-DE-TMP-32)", minimalXRechnungUBL, "BR-DE-TMP-32", false, 0},
		{"UBL with an invoicing period (BR-DE-TMP-32)",
			xrBefore(t, noUBLDate, xrUBLAtBody, `<cac:InvoicePeriod><cbc:StartDate>2024-01-01</cbc:StartDate><cbc:EndDate>2024-01-31</cbc:EndDate></cac:InvoicePeriod>`),
			"BR-DE-TMP-32", false, 0},
		{"UBL with a period on every line (BR-DE-TMP-32)",
			xrWith(t, noUBLDate, `<cac:InvoiceLine><cbc:ID>1</cbc:ID>`,
				`<cac:InvoicePeriod><cbc:StartDate>2024-01-01</cbc:StartDate><cbc:EndDate>2024-01-31</cbc:EndDate></cac:InvoicePeriod>`),
			"BR-DE-TMP-32", false, 0},
		{"CII with no delivery date or period (BR-DE-TMP-32)", noCIIDate, "BR-DE-TMP-32", true, SeverityWarning},
		{"CII with a delivery date (BR-DE-TMP-32)", minimalXRechnungCII, "BR-DE-TMP-32", false, 0},
		{"CII with a billing period (BR-DE-TMP-32)",
			xrWith(t, noCIIDate, xrCIIAtSettle, `<BillingSpecifiedPeriod><StartDateTime><DateTimeString>20240101</DateTimeString></StartDateTime></BillingSpecifiedPeriod>`),
			"BR-DE-TMP-32", false, 0},

		// BR-TMP-2, flag="warning": BT-124 is an absolute URL with a scheme.
		{"UBL external document location without a scheme (BR-TMP-2)", ublURI("www.example.de"), "BR-TMP-2", true, SeverityWarning},
		{"UBL external document location with a scheme (BR-TMP-2)", ublURI("https://www.example.de/a.pdf"), "BR-TMP-2", false, 0},
		{"CII external document location without a scheme (BR-TMP-2)", ciiURI("www.example.de"), "BR-TMP-2", true, SeverityWarning},
		{"CII external document location with a scheme (BR-TMP-2)", ciiURI("https://www.example.de/a.pdf"), "BR-TMP-2", false, 0},

		// BR-TMP-3, CII only and fatal: BT-149 given twice must agree, and so must
		// BT-150 when both paths carry it.
		{"CII price base quantity on one path only (BR-TMP-3)",
			ciiPrices(`<BasisQuantity unitCode="C62">1</BasisQuantity>`, ""), "BR-TMP-3", false, 0},
		{"CII price base quantity equal on both paths (BR-TMP-3)",
			ciiPrices(`<BasisQuantity unitCode="C62">1</BasisQuantity>`, `<BasisQuantity unitCode="C62">1</BasisQuantity>`), "BR-TMP-3", false, 0},
		{"CII price base quantity differing (BR-TMP-3)",
			ciiPrices(`<BasisQuantity unitCode="C62">2</BasisQuantity>`, `<BasisQuantity unitCode="C62">1</BasisQuantity>`), "BR-TMP-3", true, SeverityFatal},
		{"CII price base quantity with differing units (BR-TMP-3)",
			ciiPrices(`<BasisQuantity unitCode="C62">1</BasisQuantity>`, `<BasisQuantity unitCode="NAR">1</BasisQuantity>`), "BR-TMP-3", true, SeverityFatal},
		{"CII price base quantity with a unit on one path only (BR-TMP-3)",
			ciiPrices(`<BasisQuantity unitCode="C62">1</BasisQuantity>`, `<BasisQuantity>1</BasisQuantity>`), "BR-TMP-3", false, 0},
	}
}

// TestEveryExtensionSuppressionHasAReplacement is the invariant that makes the
// sub-profile overrides a swap and not a discount: for every EN 16931 rule
// validateXRechnung stops applying to a sub-profile document, KoSIT publishes the
// rule that takes its place, and this package evaluates it.
//
// Without it, "the EXTENSION relaxes BR-CL-21" and "the EXTENSION is not checked
// for item identifier schemes" are the same line of code. That is not
// hypothetical: BR-CO-16 was suppressed for an EXTENSION document in favour of a
// BR-DEX-09 that was never evaluated, so an EXTENSION invoice's amount due was
// checked by neither rule and the coverage table said so in prose nothing read.
func TestEveryExtensionSuppressionHasAReplacement(t *testing.T) {
	published := kositExtensionRules(t)
	for _, swap := range []map[string]string{xrechnungSuppressedForExtension, xrechnungSuppressedForCVD} {
		for suppressed, replacement := range swap {
			if !published[replacement] {
				t.Errorf("%s is suppressed in favour of %s, which KoSIT's Schematron does not publish", suppressed, replacement)
			}
			if !xrEvaluated[replacement] {
				t.Errorf("%s is suppressed in favour of %s, which this package does not evaluate", suppressed, replacement)
			}
		}
	}
	// BR-CO-16 is the one suppression that is not in the map, because it is
	// conditional on the binding: BR-DEX-09 exists in KoSIT's UBL Schematron and not
	// in its CII one, so a CII EXTENSION invoice keeps CEN's rule. If that ever
	// changes upstream, the condition in validateXRechnung is the wrong way round.
	ubl, cii := kositBindings(t, "BR-DEX-09")
	if !ubl {
		t.Error("BR-DEX-09 is not in KoSIT's UBL Schematron, so suppressing BR-CO-16 for a UBL EXTENSION invoice " +
			"replaces it with nothing")
	}
	if cii {
		t.Error("BR-DEX-09 is in KoSIT's CII Schematron now, so a CII EXTENSION invoice should have BR-CO-16 " +
			"suppressed too and validateXRechnung does not")
	}
}

// xrEvaluated is the set of replacement identifiers the case table above gives a
// violating verdict, which is the operational meaning of "this package evaluates
// it". Only the test above reads it.
var xrEvaluated = map[string]bool{
	"BR-DEX-01": true, "BR-DEX-04": true, "BR-DEX-05": true,
	"BR-DEX-06": true, "BR-DEX-07": true, "BR-DEX-08": true, "BR-DEX-09": true,
	"BR-TMP-CVD-01": true,
}

// kositExtensionRules and kositBindings read the vendored XRechnung Schematron.
// The full reader, which also carries each rule's flag, arrives with the last of
// the rules.
func kositExtensionRules(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, name := range kositSchematronFiles(t) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for id := range assertFlags(t, name, data) {
			out[id] = true
		}
	}
	return out
}

func kositBindings(t *testing.T, id string) (ubl, cii bool) {
	t.Helper()
	for _, name := range kositSchematronFiles(t) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := assertFlags(t, name, data)[id]; !ok {
			continue
		}
		if strings.Contains(name, string(filepath.Separator)+"ubl"+string(filepath.Separator)) {
			ubl = true
		} else {
			cii = true
		}
	}
	return ubl, cii
}

func kositSchematronFiles(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("testdata", "xrechnung", "schematron", "src", "validation", "schematron")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("KoSIT Schematron not present (make cius-oracles)")
	}
	return []string{
		filepath.Join(dir, "ubl", "XRechnung-UBL-validation.sch"),
		filepath.Join(dir, "cii", "XRechnung-CII-validation.sch"),
	}
}

// TestXRechnungCoverageNamesTheImportedRules is the honesty check on the one entry
// left in Coverage(SourceXRechnung): the identifiers it names have to be the ones
// KoSIT's whitelist names, and no others.
//
// The whitelist is a vendored file — src/xsl/rule-list.xml — and reading it here is
// what stops the entry from being prose that drifts. It is also the answer to
// "why is a Peppol identifier an XRechnung gap": because that file puts it in
// XRechnung's released Schematron.
func TestXRechnungCoverageNamesTheImportedRules(t *testing.T) {
	path := filepath.Join("testdata", "xrechnung", "schematron", "src", "xsl", "rule-list.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("KoSIT Schematron not present (make cius-oracles)")
	}
	// The file lists every candidate and comments out the ones not taken, so the
	// decoder's dropping of comments is what makes this read the live set.
	want := map[string]bool{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "rule" {
			continue
		}
		var id string
		if err := dec.DecodeElement(&id, &se); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		want[strings.TrimSpace(id)] = true
	}
	if len(want) < 21 {
		t.Fatalf("read %d whitelisted Peppol rules from %s, want at least 21", len(want), path)
	}
	got := map[string]bool{}
	for _, e := range Coverage(SourceXRechnung) {
		for _, id := range coverageIdentifiers(e.Rules) {
			if strings.HasPrefix(id, "PEPPOL-") {
				got[id] = true
			}
		}
	}
	for id := range want {
		if !got[id] {
			t.Errorf("KoSIT's whitelist merges %s into the XRechnung Schematron and Coverage(SourceXRechnung) does not name it", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("Coverage(SourceXRechnung) names %s as an imported rule, and KoSIT's whitelist does not merge it in", id)
		}
	}
	t.Logf("XRechnung imports %d Peppol rules; the coverage entry names all of them", len(want))
}
