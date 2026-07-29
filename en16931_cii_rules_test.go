package formalis

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Per-rule tests for the fatal CII-SR-* and CII-DT-* rules of CEN's CII syntax
// binding.
//
// The CEN unit-test suite is no oracle at all here, which is a stronger
// statement than the one en16931_ubl_rules_test.go could make about UBL. That
// suite ships an <error> fragment for five of the 54 fatal UBL-SR-* rules; for
// the CII binding it ships none — its per-rule testSets are UBL fragments
// (test/Invoice-unit-UBL, test/CreditNote-unit-UBL) and the only two CII files
// in it, under test/cii, are named for BR-CO-15 and BR-CO-17. So
// TestEN16931ConformanceSuite would neither notice a rule here that never fires
// nor one that fires on a conforming invoice, and the 198-rule ratchet cannot
// move for this work.
//
// The corpus is a real but partial oracle: 181 of its 1,680 documents have a
// CrossIndustryInvoice root, and being the shapes real producers emit they are
// by construction the conforming half. Two rules fire on it — CII-SR-461 on
// eleven KoSIT per-rule fixtures that repeat BT-7 in two VAT breakdown groups,
// CII-SR-470 on one that declares a credit transfer with no account — and the
// other hundred and seven are silent there because nothing in the corpus does
// the thing they forbid.
//
// That is exactly the situation in which "the rule is right" and "the rule is
// wired to an element name that never occurs" look identical from the outside.
// Three things tell them apart, and all three are checked rather than asserted:
//
//   - every rule gets both verdicts below, a document that satisfies it and a
//     document that breaks it, so a rule that cannot fire fails this file;
//   - the table at the end reads the fatal identifiers out of the vendored
//     Schematron, so a rule CEN publishes and this package forgot fails too;
//   - the element names are real. Each one appears in the vendored CII D16B
//     schema as a child of the type its rule's context has, with one exception,
//     called out where it lives on CII-SR-471.
//
// Each case asserts about its own rule and no other, for the reason
// en16931_core_rules_test.go gives: a fixture with two ApplicableTradeTax groups
// breaks CII-SR-461 and several VAT rules at once, and pinning the whole finding
// set would make each case fail for the other's reasons.

// The insertion points in validCII. The fixture is one clean EN 16931 invoice
// and almost every case below is that invoice plus one thing, so what a case is
// about is the string it inserts.
const (
	ciiAtContext       = `<ExchangedDocumentContext>`
	ciiAtExchangedDoc  = `<ExchangedDocument>`
	ciiAtProduct       = `<SpecifiedTradeProduct>`
	ciiAtLineAgreement = `<SpecifiedLineTradeAgreement>`
	ciiAtHeaderAgree   = `<ApplicableHeaderTradeAgreement>`
	ciiAtSeller        = `<SellerTradeParty>`
	ciiAtBuyer         = `<BuyerTradeParty>`
	ciiAtSummation     = `<SpecifiedTradeSettlementHeaderMonetarySummation>`
	ciiAtAddress       = `<PostalTradeAddress>` // the first one, which is the seller's
)

// ciiWith is validCII with x inserted immediately after anchor.
func ciiWith(t *testing.T, anchor, x string) string {
	t.Helper()
	return mutate(t, validCII, anchor, anchor+x)
}

// TestCIISyntaxRules is the per-rule table. Every fatal CII-SR-* rule and every
// evaluated fatal CII-DT-* rule appears twice, once with want=false and once
// with want=true. The cases are grouped into functions by the Schematron rule
// they come from, which is also how en16931_cii_rules.go groups the rule bodies.
func TestCIISyntaxRules(t *testing.T) {
	// The fixture every case is built from must itself be clean, or a case that
	// expects silence would be asserting nothing.
	if v := Validate(context.Background(), []byte(validCII), ProfileEN16931).Violations; len(v) != 0 {
		t.Fatalf("baseline CII not clean: %d violations (first %s: %s)", len(v), v[0].Rule, v[0].Message)
	}

	var cases []ruleCase
	cases = append(cases, ciiDocumentCases(t)...)
	cases = append(cases, ciiLineCases(t)...)
	cases = append(cases, ciiAllowanceCases(t)...)
	cases = append(cases, ciiHeaderCases(t)...)
	cases = append(cases, ciiTotalsCases(t)...)
	cases = append(cases, ciiIdentifierAttrCases(t)...)
	cases = append(cases, ciiCodeAttrCases(t)...)
	cases = append(cases, ciiAmountAttrCases(t)...)
	cases = append(cases, ciiQuantityCases(t)...)
	cases = append(cases, ciiTaxTypeAndDateCases(t)...)

	runRuleCases(t, cases)
}

// ciiUnreachableRules are the fatal identifiers CEN publishes that no reference
// validator can report, because an earlier rule in the same Schematron pattern
// claims their context node first. See ciiDatatypeIdentifierRules for the
// derivation and Coverage(SourceEN16931) for the statement a caller reads.
var ciiUnreachableRules = map[string]bool{
	"CII-DT-010": true,
	"CII-DT-011": true,
	"CII-DT-012": true,
}

// ciiDocumentCases are the rules CEN binds to the document element and to the
// two head groups under it.
func ciiDocumentCases(t *testing.T) []ruleCase {
	t.Helper()
	return []ruleCase{
		{"CII-SR-009 one guideline parameter group", validCII, "CII-SR-009", false},
		{"CII-SR-009 two guideline parameter groups", ciiWith(t, ciiAtContext,
			`<GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017</ID></GuidelineSpecifiedDocumentContextParameter>`),
			"CII-SR-009", true},

		{"CII-SR-010 one specification identifier", validCII, "CII-SR-010", false},
		{"CII-SR-010 two specification identifiers", mutate(t, validCII,
			`<ID>urn:cen.eu:en16931:2017</ID>`,
			`<ID>urn:cen.eu:en16931:2017</ID><ID>urn:cen.eu:en16931:2017</ID>`), "CII-SR-010", true},

		{"CII-SR-014 one invoice type code", validCII, "CII-SR-014", false},
		{"CII-SR-014 two invoice type codes", ciiWith(t, ciiAtExchangedDoc,
			`<TypeCode>380</TypeCode>`), "CII-SR-014", true},

		{"CII-SR-467 two payment means agreeing on the code", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode></SpecifiedTradeSettlementPaymentMeans>` +
				`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode></SpecifiedTradeSettlementPaymentMeans>`),
			"CII-SR-467", false},
		{"CII-SR-467 two payment means disagreeing on the code", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode></SpecifiedTradeSettlementPaymentMeans>` +
				`<SpecifiedTradeSettlementPaymentMeans><TypeCode>48</TypeCode></SpecifiedTradeSettlementPaymentMeans>`),
			"CII-SR-467", true},

		{"CII-SR-468 two payment means agreeing on the text", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode><Information>Cash</Information></SpecifiedTradeSettlementPaymentMeans>` +
				`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode><Information>Cash</Information></SpecifiedTradeSettlementPaymentMeans>`),
			"CII-SR-468", false},
		{"CII-SR-468 two payment means disagreeing on the text", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode><Information>Cash</Information></SpecifiedTradeSettlementPaymentMeans>` +
				`<SpecifiedTradeSettlementPaymentMeans><TypeCode>10</TypeCode><Information>Bar</Information></SpecifiedTradeSettlementPaymentMeans>`),
			"CII-SR-468", true},

		// The CII binding counts payment reference *elements*, where UBL-SR-44
		// counts distinct values, so two copies of one reference is a violation
		// here and is not one in UBL. The violating case is deliberately two equal
		// values, because that is the case the two bindings disagree about.
		{"CII-SR-469 one payment reference", withCIISettlement(
			`<PaymentReference>REF-1</PaymentReference>`), "CII-SR-469", false},
		{"CII-SR-469 the same payment reference twice", withCIISettlement(
			`<PaymentReference>REF-1</PaymentReference><PaymentReference>REF-1</PaymentReference>`),
			"CII-SR-469", true},

		{"CII-DT-013 no languageID on the document element", validCII, "CII-DT-013", false},
		{"CII-DT-013 languageID on the document element", mutate(t, validCII,
			`<CrossIndustryInvoice>`, `<CrossIndustryInvoice languageID="en">`), "CII-DT-013", true},

		{"CII-DT-014 no languageLocaleID on the document element", validCII, "CII-DT-014", false},
		{"CII-DT-014 languageLocaleID on the document element", mutate(t, validCII,
			`<CrossIndustryInvoice>`, `<CrossIndustryInvoice languageLocaleID="en-GB">`), "CII-DT-014", true},
	}
}

// ciiLineCases are the rules whose context is an invoice line or a group inside
// one: the product, its attributes, and the line's price agreement.
func ciiLineCases(t *testing.T) []ruleCase {
	t.Helper()
	return []ruleCase{
		{"CII-SR-046 item standard identifier with a scheme", ciiWith(t, ciiAtProduct,
			`<GlobalID schemeID="0160">04012345678901</GlobalID>`), "CII-SR-046", false},
		{"CII-SR-046 item standard identifier without a scheme", ciiWith(t, ciiAtProduct,
			`<GlobalID>04012345678901</GlobalID>`), "CII-SR-046", true},

		{"CII-SR-090 one country of origin", ciiWith(t, ciiAtProduct,
			`<OriginTradeCountry><ID>FR</ID></OriginTradeCountry>`), "CII-SR-090", false},
		{"CII-SR-090 two countries of origin", ciiWith(t, ciiAtProduct,
			`<OriginTradeCountry><ID>FR</ID><ID>DE</ID></OriginTradeCountry>`), "CII-SR-090", true},

		{"CII-SR-069 one item attribute name", ciiWith(t, ciiAtProduct,
			`<ApplicableProductCharacteristic><Description>Colour</Description><Value>Red</Value></ApplicableProductCharacteristic>`),
			"CII-SR-069", false},
		{"CII-SR-069 two item attribute names", ciiWith(t, ciiAtProduct,
			`<ApplicableProductCharacteristic><Description>Colour</Description><Description>Farbe</Description><Value>Red</Value></ApplicableProductCharacteristic>`),
			"CII-SR-069", true},

		{"CII-SR-072 one item attribute value", ciiWith(t, ciiAtProduct,
			`<ApplicableProductCharacteristic><Description>Colour</Description><Value>Red</Value></ApplicableProductCharacteristic>`),
			"CII-SR-072", false},
		{"CII-SR-072 no item attribute value", ciiWith(t, ciiAtProduct,
			`<ApplicableProductCharacteristic><Description>Colour</Description></ApplicableProductCharacteristic>`),
			"CII-SR-072", true},

		{"CII-SR-439 one item net price", validCII, "CII-SR-439", false},
		{"CII-SR-439 no item net price", mutate(t, validCII,
			`<NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice>`,
			`<NetPriceProductTradePrice></NetPriceProductTradePrice>`), "CII-SR-439", true},

		{"CII-SR-441 one item net price", validCII, "CII-SR-441", false},
		{"CII-SR-441 two item net prices", mutate(t, validCII,
			`<ChargeAmount>100.00</ChargeAmount>`,
			`<ChargeAmount>100.00</ChargeAmount><ChargeAmount>100.00</ChargeAmount>`), "CII-SR-441", true},
	}
}

// ciiAllowance wraps an allowance/charge group's children. Its amount is zero so
// the invoice totals stay consistent and the VAT rules stay quiet.
func ciiAllowance(children string) string {
	return withCIISettlement(`<SpecifiedTradeAllowanceCharge>` + children + `</SpecifiedTradeAllowanceCharge>`)
}

// ciiAllowanceCases are the rules whose context is an allowance or charge group,
// at document level or on a line, plus the item price discount.
func ciiAllowanceCases(t *testing.T) []ruleCase {
	t.Helper()
	const indicator = `<ChargeIndicator><Indicator>false</Indicator></ChargeIndicator>`
	return []ruleCase{
		{"CII-SR-463 allowance with a charge indicator", ciiAllowance(
			indicator + `<ActualAmount>0.00</ActualAmount>`), "CII-SR-463", false},
		{"CII-SR-463 allowance without a charge indicator", ciiAllowance(
			`<ActualAmount>0.00</ActualAmount>`), "CII-SR-463", true},

		// CII-SR-471 counts ram:RateApplicablePercent as a direct child of the
		// allowance group. The CII D16B schema's TradeAllowanceChargeType has no
		// such child — the percentage lives in ram:CategoryTradeTax, and across the
		// 181 CII documents in the corpus it is never anywhere else — so on a
		// schema-valid document this assertion cannot fail. It is transcribed
		// because CEN publishes it fatal, and the violating case below is
		// schema-invalid by necessity: no other document trips it.
		{"CII-SR-471 one applicable rate percentage", ciiAllowance(
			indicator + `<RateApplicablePercent>20.00</RateApplicablePercent>`), "CII-SR-471", false},
		{"CII-SR-471 two applicable rate percentages", ciiAllowance(
			indicator + `<RateApplicablePercent>20.00</RateApplicablePercent><RateApplicablePercent>10.00</RateApplicablePercent>`),
			"CII-SR-471", true},

		{"CII-SR-472 one VAT category group", ciiAllowance(
			indicator + `<CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode></CategoryTradeTax>`),
			"CII-SR-472", false},
		{"CII-SR-472 two VAT category groups", ciiAllowance(
			indicator + `<CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode></CategoryTradeTax>` +
				`<CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>Z</CategoryCode></CategoryTradeTax>`),
			"CII-SR-472", true},

		{"CII-SR-473 one allowance amount", ciiAllowance(
			indicator + `<ActualAmount>0.00</ActualAmount>`), "CII-SR-473", false},
		{"CII-SR-473 two allowance amounts", ciiAllowance(
			indicator + `<ActualAmount>0.00</ActualAmount><ActualAmount>0.00</ActualAmount>`),
			"CII-SR-473", true},

		{"CII-SR-440 one item price discount", ciiWith(t, ciiAtLineAgreement,
			`<GrossPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount><AppliedTradeAllowanceCharge><ActualAmount>0.00</ActualAmount></AppliedTradeAllowanceCharge></GrossPriceProductTradePrice>`),
			"CII-SR-440", false},
		{"CII-SR-440 two item price discounts", ciiWith(t, ciiAtLineAgreement,
			`<GrossPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount><AppliedTradeAllowanceCharge><ActualAmount>0.00</ActualAmount><ActualAmount>0.00</ActualAmount></AppliedTradeAllowanceCharge></GrossPriceProductTradePrice>`),
			"CII-SR-440", true},
	}
}

// ciiHeaderCases are the rules whose context is one of the two header groups.
func ciiHeaderCases(t *testing.T) []ruleCase {
	t.Helper()
	contact := func(n string) string {
		return `<DefinedTradeContact><PersonName>` + n + `</PersonName></DefinedTradeContact>`
	}
	email := func(a string) string {
		return `<URIUniversalCommunication><URIID schemeID="EM">` + a + `</URIID></URIUniversalCommunication>`
	}
	return []ruleCase{
		{"CII-SR-455 one seller contact", ciiWith(t, ciiAtSeller, contact("Ann")), "CII-SR-455", false},
		{"CII-SR-455 two seller contacts", ciiWith(t, ciiAtSeller, contact("Ann")+contact("Bob")), "CII-SR-455", true},

		{"CII-SR-456 one buyer contact", ciiWith(t, ciiAtBuyer, contact("Ann")), "CII-SR-456", false},
		{"CII-SR-456 two buyer contacts", ciiWith(t, ciiAtBuyer, contact("Ann")+contact("Bob")), "CII-SR-456", true},

		{"CII-SR-459 one seller electronic address", ciiWith(t, ciiAtSeller,
			email("s@example.com")), "CII-SR-459", false},
		{"CII-SR-459 two seller electronic addresses", ciiWith(t, ciiAtSeller,
			email("s@example.com")+email("t@example.com")), "CII-SR-459", true},

		{"CII-SR-460 one buyer electronic address", ciiWith(t, ciiAtBuyer,
			email("b@example.com")), "CII-SR-460", false},
		{"CII-SR-460 two buyer electronic addresses", ciiWith(t, ciiAtBuyer,
			email("b@example.com")+email("c@example.com")), "CII-SR-460", true},

		{"CII-SR-461 one tax point date", mutate(t, validCII,
			`<RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>
      <SpecifiedTradeSettlementHeaderMonetarySummation>`,
			`<RateApplicablePercent>20.00</RateApplicablePercent><TaxPointDate><DateString format="102">20240101</DateString></TaxPointDate></ApplicableTradeTax>
      <SpecifiedTradeSettlementHeaderMonetarySummation>`), "CII-SR-461", false},
		{"CII-SR-461 a tax point date in each VAT breakdown", withCIISettlement(
			`<ApplicableTradeTax><TaxPointDate><DateString format="102">20240101</DateString></TaxPointDate></ApplicableTradeTax>` +
				`<ApplicableTradeTax><TaxPointDate><DateString format="102">20240101</DateString></TaxPointDate></ApplicableTradeTax>`),
			"CII-SR-461", true},

		{"CII-SR-462 the same tax point date code twice", withCIISettlement(
			`<ApplicableTradeTax><DueDateTypeCode>5</DueDateTypeCode></ApplicableTradeTax>` +
				`<ApplicableTradeTax><DueDateTypeCode>5</DueDateTypeCode></ApplicableTradeTax>`),
			"CII-SR-462", false},
		{"CII-SR-462 two different tax point date codes", withCIISettlement(
			`<ApplicableTradeTax><DueDateTypeCode>5</DueDateTypeCode></ApplicableTradeTax>` +
				`<ApplicableTradeTax><DueDateTypeCode>72</DueDateTypeCode></ApplicableTradeTax>`),
			"CII-SR-462", true},

		{"CII-SR-470 credit transfer with an IBAN", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>30</TypeCode><PayeePartyCreditorFinancialAccount><IBANID>FR7630006000011234567890189</IBANID></PayeePartyCreditorFinancialAccount></SpecifiedTradeSettlementPaymentMeans>`),
			"CII-SR-470", false},
		{"CII-SR-470 credit transfer with no account", withCIISettlement(
			`<SpecifiedTradeSettlementPaymentMeans><TypeCode>30</TypeCode></SpecifiedTradeSettlementPaymentMeans>`),
			"CII-SR-470", true},
	}
}

// ciiTotalsCases are the eighteen document-total cardinality rules. Each gets an
// invoice carrying the amount once and an invoice carrying it twice, which is
// the whole content of the rule.
//
// Four of the eighteen amounts are in the baseline already — EN 16931 makes
// BT-106, BT-109, BT-112 and BT-115 mandatory — so for those the conforming case
// is the baseline and one insertion is the violation. For the other fourteen the
// conforming case has to insert the amount, or it would only be saying that an
// absent element occurs at most once.
func ciiTotalsCases(t *testing.T) []ruleCase {
	t.Helper()
	var out []ruleCase
	for _, c := range []struct {
		rule, elem string
		mandatory  bool
	}{
		{"CII-SR-477", "LineTotalAmount", true},
		{"CII-SR-478", "ChargeTotalAmount", false},
		{"CII-SR-479", "AllowanceTotalAmount", false},
		{"CII-SR-480", "TaxBasisTotalAmount", true},
		{"CII-SR-481", "RoundingAmount", false},
		{"CII-SR-482", "GrandTotalAmount", true},
		{"CII-SR-483", "InformationAmount", false},
		{"CII-SR-484", "TotalPrepaidAmount", false},
		{"CII-SR-485", "TotalDiscountAmount", false},
		{"CII-SR-486", "TotalAllowanceChargeAmount", false},
		{"CII-SR-487", "DuePayableAmount", true},
		{"CII-SR-488", "RetailValueExcludingTaxInformationAmount", false},
		{"CII-SR-489", "TotalDepositFeeInformationAmount", false},
		{"CII-SR-490", "ProductValueExcludingTobaccoTaxInformationAmount", false},
		{"CII-SR-491", "TotalRetailValueInformationAmount", false},
		{"CII-SR-492", "GrossLineTotalAmount", false},
		{"CII-SR-493", "NetLineTotalAmount", false},
		{"CII-SR-494", "NetIncludingTaxesLineTotalAmount", false},
	} {
		one := fmt.Sprintf("<%s>0.00</%s>", c.elem, c.elem)
		clean, broken := ciiWith(t, ciiAtSummation, one), ciiWith(t, ciiAtSummation, one+one)
		if c.mandatory {
			clean, broken = validCII, ciiWith(t, ciiAtSummation, one)
		}
		out = append(out,
			ruleCase{c.rule + " one " + c.elem, clean, c.rule, false},
			ruleCase{c.rule + " two " + c.elem, broken, c.rule, true})
	}
	return out
}

// ciiIdentifierAttrCases are the eleven attribute rules on identifiers: the
// seven CEN forbids on the four identifiers it gives a rule of their own, and
// the four it forbids on every other identifier in the document.
//
// The two families are exercised on two different elements on purpose. The
// invoice number (ram:ID under rsm:ExchangedDocument) is one of the four, so it
// answers to CII-DT-001..007; the seller's VAT registration identifier is not,
// so it answers to CII-DT-101..104. Putting @schemeName on the first and
// expecting CII-DT-001 rather than CII-DT-101 is the Schematron's rule order
// made a test — TestCIIDatatypeRuleOrderFollowsTheSchematron states the negative
// half of it.
func ciiIdentifierAttrCases(t *testing.T) []ruleCase {
	t.Helper()
	var out []ruleCase
	for _, c := range []struct{ rule, attr string }{
		{"CII-DT-001", "schemeName"},
		{"CII-DT-002", "schemeAgencyName"},
		{"CII-DT-003", "schemeDataURI"},
		{"CII-DT-004", "schemeURI"},
		{"CII-DT-005", "schemeID"},
		{"CII-DT-006", "schemeAgencyID"},
		{"CII-DT-007", "schemeVersionID"},
	} {
		out = append(out,
			ruleCase{c.rule + " a bare invoice number", validCII, c.rule, false},
			ruleCase{c.rule + " an invoice number carrying @" + c.attr,
				mutate(t, validCII, `<ID>INV-1</ID>`,
					fmt.Sprintf(`<ID %s="x">INV-1</ID>`, c.attr)), c.rule, true})
	}
	for _, c := range []struct{ rule, attr string }{
		{"CII-DT-101", "schemeName"},
		{"CII-DT-102", "schemeAgencyName"},
		{"CII-DT-103", "schemeDataURI"},
		{"CII-DT-104", "schemeURI"},
	} {
		out = append(out,
			ruleCase{c.rule + " a VAT registration identifier with only @schemeID", validCII, c.rule, false},
			ruleCase{c.rule + " a VAT registration identifier carrying @" + c.attr,
				mutate(t, validCII, `<ID schemeID="VA">FR12345678</ID>`,
					fmt.Sprintf(`<ID schemeID="VA" %s="x">FR12345678</ID>`, c.attr)), c.rule, true})
	}
	return out
}

// ciiCodeAttrCases are the two attributes CEN forbids on every code in the
// document.
func ciiCodeAttrCases(t *testing.T) []ruleCase {
	t.Helper()
	return []ruleCase{
		{"CII-DT-008 no @name on a code", validCII, "CII-DT-008", false},
		{"CII-DT-008 @name on a code", mutate(t, validCII,
			`<TypeCode>380</TypeCode>`, `<TypeCode name="Commercial invoice">380</TypeCode>`), "CII-DT-008", true},

		{"CII-DT-009 no @listURI on a code", validCII, "CII-DT-009", false},
		{"CII-DT-009 @listURI on a code", mutate(t, validCII,
			`<TypeCode>380</TypeCode>`, `<TypeCode listURI="http://example.com/1001">380</TypeCode>`), "CII-DT-009", true},
	}
}

// ciiAmountAttrCases are the two currency attributes CEN forbids on every amount
// but ram:TaxTotalAmount. The conforming case for CII-DT-031 is the carve-out
// itself: the VAT total may name its currency, because an invoice with a VAT
// accounting currency (BT-6) carries that one amount twice, in two currencies.
// It is the case that matters — @currencyID occurs 210 times in the corpus and
// every one of them is on a ram:TaxTotalAmount, so a rule that had lost the
// exclusion would fire 210 times rather than none.
func ciiAmountAttrCases(t *testing.T) []ruleCase {
	t.Helper()
	return []ruleCase{
		{"CII-DT-031 a currency on the VAT total", mutate(t, validCII,
			`<TaxTotalAmount>20.00</TaxTotalAmount>`, `<TaxTotalAmount currencyID="EUR">20.00</TaxTotalAmount>`),
			"CII-DT-031", false},
		{"CII-DT-031 a currency on the grand total", mutate(t, validCII,
			`<GrandTotalAmount>120.00</GrandTotalAmount>`, `<GrandTotalAmount currencyID="EUR">120.00</GrandTotalAmount>`),
			"CII-DT-031", true},

		{"CII-DT-032 no currency code list version on an amount", validCII, "CII-DT-032", false},
		{"CII-DT-032 a currency code list version on an amount", mutate(t, validCII,
			`<GrandTotalAmount>120.00</GrandTotalAmount>`,
			`<GrandTotalAmount currencyCodeListVersionID="2016">120.00</GrandTotalAmount>`), "CII-DT-032", true},
	}
}

// ciiQuantityCases are the four rules on a quantity: the one that permits a unit
// code only once some line has stated the unit it invoiced in, and the three
// code-list attributes CEN forbids outright.
func ciiQuantityCases(t *testing.T) []ruleCase {
	t.Helper()
	const withBasis = `<GrossPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount>` +
		`<BasisQuantity unitCode="C62">1</BasisQuantity></GrossPriceProductTradePrice>`
	out := []ruleCase{
		{"CII-DT-033 a unit code where the line states its unit",
			ciiWith(t, ciiAtLineAgreement, withBasis), "CII-DT-033", false},
		{"CII-DT-033 a unit code where no line states its unit", mutate(t,
			ciiWith(t, ciiAtLineAgreement, withBasis),
			`<BilledQuantity unitCode="C62">1</BilledQuantity>`, `<BilledQuantity>1</BilledQuantity>`),
			"CII-DT-033", true},
	}
	for _, c := range []struct{ rule, attr string }{
		{"CII-DT-034", "unitCodeListID"},
		{"CII-DT-035", "unitCodeListAgencyID"},
		{"CII-DT-036", "unitCodeListAgencyName"},
	} {
		out = append(out,
			ruleCase{c.rule + " a billed quantity with only @unitCode", validCII, c.rule, false},
			ruleCase{c.rule + " a billed quantity carrying @" + c.attr,
				mutate(t, validCII, `<BilledQuantity unitCode="C62">1</BilledQuantity>`,
					fmt.Sprintf(`<BilledQuantity unitCode="C62" %s="x">1</BilledQuantity>`, c.attr)), c.rule, true})
	}
	return out
}

// ciiTaxTypeAndDateCases are the two remaining value rules: the tax type a tax
// group may name, and the form a date declaring UN/EDIFACT format 102 must take.
func ciiTaxTypeAndDateCases(t *testing.T) []ruleCase {
	t.Helper()
	return []ruleCase{
		{"CII-DT-037 a VAT tax group", mutate(t, validCII,
			`<ApplicableTradeTax><CalculatedAmount>`, `<ApplicableTradeTax><TypeCode>VAT</TypeCode><CalculatedAmount>`),
			"CII-DT-037", false},
		{"CII-DT-037 a non-VAT tax group", mutate(t, validCII,
			`<ApplicableTradeTax><CalculatedAmount>`, `<ApplicableTradeTax><TypeCode>AAA</TypeCode><CalculatedAmount>`),
			"CII-DT-037", true},

		{"CII-DT-097 a format-102 date written YYYYMMDD", mutate(t, validCII,
			`<DateTimeString>20240101</DateTimeString>`, `<DateTimeString format="102">20240101</DateTimeString>`),
			"CII-DT-097", false},
		{"CII-DT-097 a format-102 date written with separators", mutate(t, validCII,
			`<DateTimeString>20240101</DateTimeString>`, `<DateTimeString format="102">2024-01-01</DateTimeString>`),
			"CII-DT-097", true},
	}
}

// TestCIISyntaxRulesAreNotAskedOfUBL pins the half of the design the file
// comment argues for, and is the converse of
// TestUBLSyntaxRulesAreNotAskedOfCII: the CII binding is a statement about CII,
// so a UBL invoice must never be accused under one of its identifiers.
func TestCIISyntaxRulesAreNotAskedOfUBL(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{"clean UBL invoice", minimalUBL},
		{"UBL invoice with two payment means codes", withUBLBody(
			`<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>` +
				`<PaymentMeans><PaymentMeansCode>58</PaymentMeansCode></PaymentMeans>`)},
		{"UBL credit note", ublCreditNote(minimalUBL)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range Validate(context.Background(), []byte(tc.doc), ProfileEN16931).Violations {
				if strings.HasPrefix(v.Rule, "CII-") {
					t.Errorf("UBL invoice reported the CII binding rule %s: %s", v.Rule, v.Message)
				}
			}
		})
	}
}

// TestCIIDatatypeRuleOrderFollowsTheSchematron states the negative half of ISO
// Schematron's first-matching-rule semantics, which is the one thing about this
// binding a transcription can get subtly and silently wrong.
//
// Two contexts in the EN16931-CII-Syntax pattern overlap, and in both cases the
// earlier rule wins:
//
//   - the invoice number is one of the four identifiers CEN gives a rule of
//     their own, so @schemeName on it is CII-DT-001 and never CII-DT-101;
//   - the invoice type code is matched by `//ram:TypeCode` before the rule bound
//     to it specifically, so @listID on it is reported by nobody — CII-DT-010,
//     CII-DT-011 and CII-DT-012 cannot fire, which is why Coverage names them.
func TestCIIDatatypeRuleOrderFollowsTheSchematron(t *testing.T) {
	scoped := mutate(t, validCII, `<ID>INV-1</ID>`, `<ID schemeName="x">INV-1</ID>`)
	v := Validate(context.Background(), []byte(scoped), ProfileEN16931).Violations
	if !reports(v, "CII-DT-001") {
		t.Errorf("@schemeName on the invoice number should be CII-DT-001; got %v", v)
	}
	if reports(v, "CII-DT-101") {
		t.Errorf("@schemeName on the invoice number reported CII-DT-101, which an earlier rule in the same pattern claims: %v", v)
	}

	for _, attr := range []string{"listID", "listAgencyID", "listVersionID"} {
		doc := mutate(t, validCII, `<TypeCode>380</TypeCode>`,
			fmt.Sprintf(`<TypeCode %s="x">380</TypeCode>`, attr))
		for _, x := range Validate(context.Background(), []byte(doc), ProfileEN16931).Violations {
			if ciiUnreachableRules[x.Rule] {
				t.Errorf("@%s on the invoice type code reported %s, which no reference validator can reach", attr, x.Rule)
			}
		}
	}
}

// TestCIISyntaxRulesCarryTheEN16931Source pins the PR 3 decision: CEN publishes
// the syntax bindings as normative parts of EN 16931, so a binding finding is an
// EN 16931 finding and not one from a source of its own.
func TestCIISyntaxRulesCarryTheEN16931Source(t *testing.T) {
	doc := ciiWith(t, ciiAtExchangedDoc, `<TypeCode>380</TypeCode>`)
	found := false
	for _, v := range Validate(context.Background(), []byte(doc), ProfileEN16931).Violations {
		if v.Rule != "CII-SR-014" {
			continue
		}
		found = true
		if v.Source != SourceEN16931 {
			t.Errorf("CII-SR-014 carries Source %q, want %q", v.Source, SourceEN16931)
		}
	}
	if !found {
		t.Fatal("CII-SR-014 did not fire on the fixture this test is built from")
	}
}
