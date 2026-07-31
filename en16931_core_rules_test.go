package formalis

import (
	"context"
	"strings"
	"testing"
)

// Per-rule tests for the EN 16931 semantic-model rules added on top of the CEN
// unit-test oracle.
//
// The oracle cannot stand in for these. Of the twelve rules this file's subject
// matter covers, the CEN suite ships fragments for exactly one (BR-51), and even
// there the failing fragment is tagged <warning>, which
// TestEN16931ConformanceSuite does not score at all. Every other rule here is
// invisible to the oracle in both directions: it would neither catch a rule that
// never fires nor catch one that fires on a conforming invoice. So each rule
// gets a conforming case and a violating case, in both syntaxes, stating what
// the rule is for rather than only that it exists.
//
// Each case asserts about *its own* rule and no other. A mutation that breaks a
// decimal limit may well trip a datatype rule too, and pinning the whole finding
// set would make these tests fail for reasons that have nothing to do with what
// they are about.

// reports says whether vs contains a finding for rule.
func reports(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// mutate applies a single textual substitution and fails if it matched nothing,
// so a fixture edit cannot silently turn a violating case into the baseline.
func mutate(t *testing.T, doc, from, to string) string {
	t.Helper()
	out := strings.Replace(doc, from, to, 1)
	if out == doc {
		t.Fatalf("fixture does not contain %q", from)
	}
	return out
}

// ruleCase is one document and the verdict expected for one rule.
type ruleCase struct {
	name string
	xml  string
	rule string
	want bool // true: the rule must fire; false: it must not
}

func runRuleCases(t *testing.T, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs := findings(t, context.Background(), ValidateEN16931, []byte(tc.xml))
			if got := reports(vs, tc.rule); got != tc.want {
				if tc.want {
					t.Errorf("expected %s to fire; got %v", tc.rule, vs)
				} else {
					t.Errorf("%s fired on a document that satisfies it: %v", tc.rule, vs)
				}
			}
		})
	}
}

// withCIISettlement injects XML immediately after the CII invoice currency code,
// i.e. into ApplicableHeaderTradeSettlement.
func withCIISettlement(x string) string {
	return strings.Replace(validCII, "<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>",
		"<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>"+x, 1)
}

// withUBLBody injects XML immediately before the UBL cac:TaxTotal.
func withUBLBody(x string) string {
	return strings.Replace(minimalUBL, "<TaxTotal>", x+"<TaxTotal>", 1)
}

// ciiCard wraps a payment card primary account number (BT-87) in the CII payment
// means group. Type code 48 is "bank card", so the group does not additionally
// trip BR-49 or BR-61.
func ciiCard(pan string) string {
	return withCIISettlement(`<SpecifiedTradeSettlementPaymentMeans><TypeCode>48</TypeCode>` +
		`<ApplicableTradeSettlementFinancialCard><ID>` + pan + `</ID></ApplicableTradeSettlementFinancialCard>` +
		`</SpecifiedTradeSettlementPaymentMeans>`)
}

// ublCard is the UBL spelling of the same thing.
func ublCard(pan string) string {
	return withUBLBody(`<PaymentMeans><PaymentMeansCode>48</PaymentMeansCode>` +
		`<CardAccount><PrimaryAccountNumberID>` + pan + `</PrimaryAccountNumberID></CardAccount></PaymentMeans>`)
}

// TestBR51CardNumber covers the card-number length rule, including the part of
// it this package deliberately does not report.
//
// BR-51 is one assertion in the abstract EN 16931 model with two severities in
// the two bindings: EN16931-CII-model.sch flags it fatal and EN16931-UBL-model.sch
// flags it warning, which is why the CEN suite's failing fragment for it is
// tagged <warning> rather than <error>. This package reports what an authority
// makes fatal, so the CII half is checked and the UBL half is named in
// Coverage(SourceEN16931).
//
// The last case is why that distinction is not pedantry. A PAN masked the way
// PCI DSS asks for it — twelve X's and the last four digits — is sixteen
// characters long, so CEN's length test reports it. On a UBL invoice that is a
// warning about a document that did the right thing; reporting it as a violation
// would be an accusation the reference validator does not make.
func TestBR51CardNumber(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{"CII truncated PAN", ciiCard("123456"), "BR-51", false},
		{"CII ten characters is the limit", ciiCard("1234567890"), "BR-51", false},
		{"CII eleven characters is a full PAN", ciiCard("12345678901"), "BR-51", true},
		// The CEN test normalises whitespace before measuring, so a grouped PAN
		// is measured on its collapsed form (13 characters here, not 15).
		{"CII grouped digits are normalised, not stripped", ciiCard(" 1234 5678 901 "), "BR-51", true},
		{"CII padded short PAN normalises to six", ciiCard("   123456   "), "BR-51", false},
		{"UBL full PAN is advisory there, so not reported", ublCard("12345678901"), "BR-51", false},
		{"UBL PCI-masked PAN is not reported", ublCard("XXXXXXXXXXXX1234"), "BR-51", false},
	})
}

// TestBRCL08NoteSubjectCode covers the Invoice note subject code (BT-21) against
// UNTDID 4451, and the very different ways the two syntaxes carry it.
//
// CII has an element, ram:SubjectCode. UBL has none: the binding writes the code
// into the note text as a "#CODE#" prefix, and the CEN rule reads it back out
// under exactly that convention — a '#' followed by three characters and another
// '#'. The negative cases below are the ones that convention exists to protect:
// a note that merely mentions a '#' is a note without a subject code, not a note
// with an invalid one.
func TestBRCL08NoteSubjectCode(t *testing.T) {
	ciiNote := func(code string) string {
		return mutate(t, validCII, "<ID>INV-1</ID>",
			"<ID>INV-1</ID><IncludedNote><Content>a note</Content><SubjectCode>"+code+"</SubjectCode></IncludedNote>")
	}
	ublNote := func(text string) string {
		return mutate(t, minimalUBL, "<ID>INV-1</ID>", "<ID>INV-1</ID><Note>"+text+"</Note>")
	}
	runRuleCases(t, []ruleCase{
		{"CII listed code", ciiNote("AAI"), "BR-CL-08", false},
		{"CII code the stale UBL copy omits", ciiNote("BAT"), "BR-CL-08", false},
		{"CII unlisted code", ciiNote("XXX"), "BR-CL-08", true},
		{"UBL prefixed listed code", ublNote("#AAI#General information"), "BR-CL-08", false},
		{"UBL prefixed unlisted code", ublNote("#XXX#General information"), "BR-CL-08", true},
		{"UBL note with no prefix", ublNote("General information"), "BR-CL-08", false},
		{"UBL note that merely mentions a hash", ublNote("see #1 above"), "BR-CL-08", false},
		{"UBL prefix of the wrong length is not a code", ublNote("#GENERAL#information"), "BR-CL-08", false},
		// BT-127, the Invoice line note, has no subject code: the CEN rule's
		// context is the document element's cbc:Note only.
		{"UBL line note carries no subject code", mutate(t, minimalUBL,
			"<InvoiceLine><ID>1</ID>", "<InvoiceLine><ID>1</ID><Note>#XXX#line note</Note>"), "BR-CL-08", false},
	})
}

// TestBRCL26DeliverToLocationScheme covers the Deliver-to location identifier
// scheme (BT-71) against the ISO 6523 ICD list — the same list BR-CL-10/11/21
// use, which is why 0001 is not in it (ISO withdrew it).
//
// The identifier is placed without a postal address on purpose: BG-15's
// presence is what BR-57 keys on, and this rule is about the identifier, not
// about the address group.
func TestBRCL26DeliverToLocationScheme(t *testing.T) {
	ciiShipTo := func(scheme string) string {
		return mutate(t, validCII, "<ApplicableHeaderTradeAgreement>",
			`<ApplicableHeaderTradeDelivery><ShipToTradeParty><GlobalID schemeID="`+scheme+
				`">7300010000001</GlobalID></ShipToTradeParty></ApplicableHeaderTradeDelivery>`+
				"<ApplicableHeaderTradeAgreement>")
	}
	ublLocation := func(scheme string) string {
		return withUBLBody(`<Delivery><DeliveryLocation><ID schemeID="` + scheme +
			`">7300010000001</ID></DeliveryLocation></Delivery>`)
	}
	runRuleCases(t, []ruleCase{
		{"CII listed scheme", ciiShipTo("0088"), "BR-CL-26", false},
		{"CII unlisted scheme", ciiShipTo("XR01"), "BR-CL-26", true},
		{"UBL listed scheme", ublLocation("0088"), "BR-CL-26", false},
		{"UBL withdrawn scheme 0001", ublLocation("0001"), "BR-CL-26", true},
		// An identifier with no scheme at all is BT-71 without BT-71-1; this rule
		// constrains the scheme when there is one.
		{"UBL identifier with no scheme", withUBLBody(
			`<Delivery><DeliveryLocation><ID>7300010000001</ID></DeliveryLocation></Delivery>`), "BR-CL-26", false},
	})
}

// ciiDocAllowanceCharge builds a document-level allowance or charge carrying a
// base amount (BT-93 / BT-100). The actual amount is zero so the invoice totals
// stay consistent.
func ciiDocAllowanceCharge(charge bool, base string) string {
	ind := "false"
	if charge {
		ind = "true"
	}
	return withCIISettlement(`<SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>` + ind +
		`</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount><BasisAmount>` + base +
		`</BasisAmount><Reason>r</Reason><CategoryTradeTax><CategoryCode>S</CategoryCode>` +
		`<RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax></SpecifiedTradeAllowanceCharge>`)
}

// ublDocAllowanceCharge is the UBL spelling of the same thing.
func ublDocAllowanceCharge(charge bool, base string) string {
	ind := "false"
	if charge {
		ind = "true"
	}
	return withUBLBody(`<AllowanceCharge><ChargeIndicator>` + ind +
		`</ChargeIndicator><AllowanceChargeReason>r</AllowanceChargeReason><Amount>0.00</Amount><BaseAmount>` +
		base + `</BaseAmount><TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></AllowanceCharge>`)
}

// TestBRDEC02And06DocumentAllowanceChargeBase covers the two-decimal limits on
// the document level allowance base amount (BT-93) and charge base amount
// (BT-100). The base amount is the figure a percentage allowance is computed
// from, so it is a separate term from the allowance amount itself (BT-92/99,
// BR-DEC-01/05) and has its own limit.
func TestBRDEC02And06DocumentAllowanceChargeBase(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{"CII allowance base with two decimals", ciiDocAllowanceCharge(false, "1000.00"), "BR-DEC-02", false},
		{"CII allowance base with three decimals", ciiDocAllowanceCharge(false, "999.375"), "BR-DEC-02", true},
		{"UBL allowance base with two decimals", ublDocAllowanceCharge(false, "1000.00"), "BR-DEC-02", false},
		{"UBL allowance base with three decimals", ublDocAllowanceCharge(false, "999.375"), "BR-DEC-02", true},
		// The allowance rule and the charge rule are distinct identifiers over the
		// same element, told apart only by the charge indicator.
		{"CII allowance base does not report the charge rule", ciiDocAllowanceCharge(false, "999.375"), "BR-DEC-06", false},
		{"CII charge base with two decimals", ciiDocAllowanceCharge(true, "1000.00"), "BR-DEC-06", false},
		{"CII charge base with three decimals", ciiDocAllowanceCharge(true, "999.375"), "BR-DEC-06", true},
		{"UBL charge base with three decimals", ublDocAllowanceCharge(true, "999.375"), "BR-DEC-06", true},
		{"UBL charge base does not report the allowance rule", ublDocAllowanceCharge(true, "999.375"), "BR-DEC-02", false},
	})
}

// ciiLineAllowanceCharge puts an allowance or charge with a base amount
// (BT-137 / BT-142) on the CII invoice line.
func ciiLineAllowanceCharge(charge bool, base string) string {
	ind := "false"
	if charge {
		ind = "true"
	}
	return strings.Replace(validCII, "<SpecifiedTradeSettlementLineMonetarySummation>",
		`<SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>`+ind+
			`</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount><BasisAmount>`+base+
			`</BasisAmount><Reason>r</Reason></SpecifiedTradeAllowanceCharge>`+
			"<SpecifiedTradeSettlementLineMonetarySummation>", 1)
}

// ublLineAllowanceCharge is the UBL spelling of the same thing.
func ublLineAllowanceCharge(charge bool, base string) string {
	ind := "false"
	if charge {
		ind = "true"
	}
	return strings.Replace(minimalUBL, "<Item><Name>Widget</Name>",
		`<AllowanceCharge><ChargeIndicator>`+ind+
			`</ChargeIndicator><AllowanceChargeReason>r</AllowanceChargeReason><Amount>0.00</Amount><BaseAmount>`+
			base+`</BaseAmount></AllowanceCharge>`+"<Item><Name>Widget</Name>", 1)
}

// TestBRDEC25And28LineAllowanceChargeBase covers the two-decimal limits on the
// invoice line allowance base amount (BT-137) and charge base amount (BT-142).
func TestBRDEC25And28LineAllowanceChargeBase(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{"CII line allowance base with two decimals", ciiLineAllowanceCharge(false, "9800.00"), "BR-DEC-25", false},
		{"CII line allowance base with three decimals", ciiLineAllowanceCharge(false, "9800.000"), "BR-DEC-25", true},
		{"UBL line allowance base with two decimals", ublLineAllowanceCharge(false, "9800.00"), "BR-DEC-25", false},
		{"UBL line allowance base with three decimals", ublLineAllowanceCharge(false, "9800.000"), "BR-DEC-25", true},
		{"CII line charge base with two decimals", ciiLineAllowanceCharge(true, "9800.00"), "BR-DEC-28", false},
		{"CII line charge base with three decimals", ciiLineAllowanceCharge(true, "9800.000"), "BR-DEC-28", true},
		{"UBL line charge base with three decimals", ublLineAllowanceCharge(true, "9800.000"), "BR-DEC-28", true},
		{"UBL line charge base does not report the allowance rule", ublLineAllowanceCharge(true, "9800.000"), "BR-DEC-25", false},
	})
}

// TestBRDEC15VATTotalInAccountingCurrency covers the two-decimal limit on the
// Invoice total VAT amount in accounting currency (BT-111) — the one BR-DEC-*
// term that is not simply "an amount somewhere in the document" but "the amount
// tagged with the VAT accounting currency (BT-6)". An invoice carrying two VAT
// totals in two currencies must have this rule read the right one, so the
// document-currency total is deliberately given three decimals in the last case
// and must not be reported here (BR-DEC-13 is that rule).
func TestBRDEC15VATTotalInAccountingCurrency(t *testing.T) {
	cii := func(inTaxCurrency string) string {
		return mutate(t, validCII, `<TaxTotalAmount currencyID="EUR">20.00</TaxTotalAmount>`,
			`<TaxTotalAmount currencyID="EUR">20.00</TaxTotalAmount>`+
				`<TaxTotalAmount currencyID="SEK">`+inTaxCurrency+`</TaxTotalAmount>`)
	}
	ciiWithTaxCurrency := func(inTaxCurrency string) string {
		return mutate(t, cii(inTaxCurrency), "<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>",
			"<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode><TaxCurrencyCode>SEK</TaxCurrencyCode>")
	}
	ubl := func(inTaxCurrency string) string {
		return mutate(t, minimalUBL, "<DocumentCurrencyCode>EUR</DocumentCurrencyCode>",
			"<DocumentCurrencyCode>EUR</DocumentCurrencyCode><TaxCurrencyCode>SEK</TaxCurrencyCode>"+
				`<TaxTotal><TaxAmount currencyID="SEK">`+inTaxCurrency+`</TaxAmount></TaxTotal>`)
	}
	runRuleCases(t, []ruleCase{
		{"CII BT-111 with two decimals", ciiWithTaxCurrency("210.00"), "BR-DEC-15", false},
		{"CII BT-111 with three decimals", ciiWithTaxCurrency("210.001"), "BR-DEC-15", true},
		{"UBL BT-111 with two decimals", ubl("210.00"), "BR-DEC-15", false},
		{"UBL BT-111 with three decimals", ubl("210.001"), "BR-DEC-15", true},
		// No VAT accounting currency: there is no BT-111 to bound, whatever the
		// second amount is tagged with.
		{"CII no accounting currency declared", cii("210.001"), "BR-DEC-15", false},
		// The document-currency VAT total is BT-110, and BR-DEC-13 bounds it.
		{"UBL document-currency total is not BT-111", mutate(t, ubl("210.00"),
			"<TaxTotal><TaxAmount>19.00</TaxAmount>", "<TaxTotal><TaxAmount>19.001</TaxAmount>"), "BR-DEC-15", false},
		{"UBL document-currency total is BR-DEC-13", mutate(t, ubl("210.00"),
			"<TaxTotal><TaxAmount>19.00</TaxAmount>", "<TaxTotal><TaxAmount>19.001</TaxAmount>"), "BR-DEC-13", true},
	})
}

// TestBRCO05To08AreNotReported states the one decision in this group that is a
// deliberate absence rather than an implementation.
//
// BR-CO-05..08 ask whether an allowance or charge reason *code* and its
// free-text *reason* "indicate the same type". CEN binds all four to the XPath
// expression true() in both the UBL and the CII Schematron, so no reference
// validator reports them and the unit-test suite ships no fragment for them.
// This test pins that this package does not invent an answer: a document whose
// reason text plainly contradicts its reason code is not reported, and the
// Coverage table says why.
func TestBRCO05To08AreNotReported(t *testing.T) {
	contradictory := withCIISettlement(
		`<SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>false</Indicator></ChargeIndicator>` +
			`<ActualAmount>0.00</ActualAmount><ReasonCode>95</ReasonCode>` + // 95 = Discount
			`<Reason>Insurance</Reason>` + // plainly not a discount
			`<CategoryTradeTax><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax>` +
			`</SpecifiedTradeAllowanceCharge>`)
	vs := findings(t, context.Background(), ValidateEN16931, []byte(contradictory))
	for _, rule := range []string{"BR-CO-05", "BR-CO-06", "BR-CO-07", "BR-CO-08"} {
		if reports(vs, rule) {
			t.Errorf("%s was reported; CEN binds it to true() in both syntaxes, so reporting it "+
				"is an accusation no reference validator makes", rule)
		}
	}
}

// TestCoverageDropsTheRulesThisPRImplements is the other half of implementing a
// rule: an implemented rule that still appears in NotEvaluated is a lie in the
// direction the Coverage table was built to prevent, just pointing the other
// way. It sends a caller to re-implement work already done.
//
// The over-claim sweep in report_test.go catches this only for rules that some
// corpus document happens to trip; four of these fire on nothing in the corpus,
// which is exactly the case that needs stating here instead.
func TestCoverageDropsTheRulesThisPRImplements(t *testing.T) {
	gaps := coverageText(SourceEN16931)
	for _, rule := range []string{
		"BR-CL-08", "BR-CL-26", "BR-DEC-02", "BR-DEC-06", "BR-DEC-15", "BR-DEC-25", "BR-DEC-28",
	} {
		if strings.Contains(gaps, rule) {
			t.Errorf("Coverage(SourceEN16931) still names %s, which this package evaluates", rule)
		}
	}
	// BR-51 stays, because half of it does: the entry has to say which half.
	if !strings.Contains(gaps, "BR-51 other than in the CII binding") {
		t.Error("Coverage(SourceEN16931) no longer names the advisory UBL half of BR-51, which is not evaluated")
	}
	if !strings.Contains(gaps, "BR-CO-05..08") {
		t.Error("Coverage(SourceEN16931) no longer names BR-CO-05..08, which CEN binds to true()")
	}
}
