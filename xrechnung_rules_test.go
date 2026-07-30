package formalis

import (
	"context"
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
// at once, in both bindings.
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v := findings(t, context.Background(), ValidateXRechnung, []byte(tc.doc)); len(v) != 0 {
				t.Fatalf("baseline not clean: %d violations (first %s: %s)", len(v), v[0].Rule, v[0].Message)
			}
		})
	}
}

// TestXRechnungRules is the violating half of the table, grouped as
// xrechnung_rules.go groups the rule bodies.
func TestXRechnungRules(t *testing.T) {
	var cases []xrCase
	cases = append(cases, xrSkontoCases(t)...)
	cases = append(cases, xrPaymentMeansCases(t)...)
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
