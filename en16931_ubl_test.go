package formalis

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateUBLCorpus is the FP=0 oracle for the UBL syntax: every conforming
// EN 16931 UBL example (CEN TC 434 and Peppol BIS) must validate with no core
// business-rule violations. The corpus is not vendored; the test skips when
// testdata/en16931-ubl is absent (run `make en16931-ubl`).
func TestValidateUBLCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/en16931-ubl/*.xml")
	if len(files) == 0 {
		t.Skip("EN 16931 UBL corpus not present (make en16931-ubl)")
	}
	atLeast(t, "EN 16931 UBL corpus", len(files), minEN16931UBLInvoices)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		v := findings(t, context.Background(), ValidateEN16931, data)
		if len(v) != 0 {
			t.Errorf("%s: expected 0 violations on a conforming UBL invoice, got %d (first: %s: %s)",
				filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

// TestValidateUBLDetectsSyntax checks both UBL document types and the rejection
// of a non-invoice root.
func TestValidateUBLDetectsSyntax(t *testing.T) {
	inv, err := parseEN16931(newRun(nil), []byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`))
	if err != nil || inv == nil {
		t.Fatalf("UBL Invoice root not recognised: %v", err)
	}
	cn, err := parseEN16931(newRun(nil), []byte(`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`))
	if err != nil || cn == nil {
		t.Fatalf("UBL CreditNote root not recognised: %v", err)
	}
	if _, err := parseEN16931(newRun(nil), []byte(`<PurchaseOrder/>`)); err == nil {
		t.Error("a non-invoice root must be rejected")
	}
}

// minimalUBL is a small but complete, conforming EN 16931 UBL invoice used to
// verify the rules fire when a mandatory term is removed.
const minimalUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2">
<CustomizationID>urn:cen.eu:en16931:2017</CustomizationID>
<ID>INV-1</ID><IssueDate>2024-01-15</IssueDate>
<InvoiceTypeCode>380</InvoiceTypeCode><DocumentCurrencyCode>EUR</DocumentCurrencyCode>
<AccountingSupplierParty><Party>
  <PostalAddress><Country><IdentificationCode>DE</IdentificationCode></Country></PostalAddress>
  <PartyTaxScheme><CompanyID>DE123456789</CompanyID><TaxScheme><ID>VAT</ID></TaxScheme></PartyTaxScheme>
  <PartyLegalEntity><RegistrationName>Seller Ltd</RegistrationName></PartyLegalEntity>
</Party></AccountingSupplierParty>
<AccountingCustomerParty><Party>
  <PostalAddress><Country><IdentificationCode>DE</IdentificationCode></Country></PostalAddress>
  <PartyLegalEntity><RegistrationName>Buyer Ltd</RegistrationName></PartyLegalEntity>
</Party></AccountingCustomerParty>
<TaxTotal><TaxAmount>19.00</TaxAmount>
  <TaxSubtotal><TaxableAmount>100.00</TaxableAmount><TaxAmount>19.00</TaxAmount>
    <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></TaxSubtotal>
</TaxTotal>
<LegalMonetaryTotal><LineExtensionAmount>100.00</LineExtensionAmount>
  <TaxExclusiveAmount>100.00</TaxExclusiveAmount><TaxInclusiveAmount>119.00</TaxInclusiveAmount>
  <PayableAmount>119.00</PayableAmount></LegalMonetaryTotal>
<InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="C62">1</InvoicedQuantity>
  <LineExtensionAmount>100.00</LineExtensionAmount>
  <Item><Name>Widget</Name><ClassifiedTaxCategory><ID>S</ID><Percent>19</Percent></ClassifiedTaxCategory></Item>
  <Price><PriceAmount>100.00</PriceAmount></Price></InvoiceLine>
</Invoice>`

func TestValidateUBLMutations(t *testing.T) {
	// Baseline: the minimal invoice is clean.
	if v := findings(t, context.Background(), ValidateEN16931, []byte(minimalUBL)); len(v) != 0 {
		t.Fatalf("baseline UBL not clean: %d violations (first %s: %s)", len(v), v[0].Rule, v[0].Message)
	}
	cases := []struct {
		name, remove, wantRule string
	}{
		{"no currency (BR-05)", "<DocumentCurrencyCode>EUR</DocumentCurrencyCode>", "BR-05"},
		{"no invoice number (BR-02)", "<ID>INV-1</ID>", "BR-02"},
		{"no seller name (BR-06)", "<RegistrationName>Seller Ltd</RegistrationName>", "BR-06"},
		{"no seller country (BR-09)", "<IdentificationCode>DE</IdentificationCode>", "BR-09"},
		{"no line item name (BR-25)", "<Name>Widget</Name>", "BR-25"},
		{"no grand total (BR-14)", "<TaxInclusiveAmount>119.00</TaxInclusiveAmount>", "BR-14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalUBL, tc.remove, "", 1)
			if broken == minimalUBL {
				t.Fatalf("mutation string %q not found", tc.remove)
			}
			v := findings(t, context.Background(), ValidateEN16931, []byte(broken))
			found := false
			for _, x := range v {
				if x.Rule == tc.wantRule {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s to fire; got %v", tc.wantRule, v)
			}
		})
	}
}

// The two cac:TaxTotal elements of an invoice that declares a VAT accounting
// currency (BT-6): one in the document currency carrying BT-110 and the BG-23
// breakdown, one in the accounting currency carrying BT-111 and no subtotals.
const (
	ublDocCurrencyTaxTotal = `<TaxTotal><TaxAmount currencyID="EUR">19.00</TaxAmount>
  <TaxSubtotal><TaxableAmount currencyID="EUR">100.00</TaxableAmount><TaxAmount currencyID="EUR">19.00</TaxAmount>
    <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></TaxSubtotal>
</TaxTotal>`
	ublAccountingCurrencyTaxTotal = `<TaxTotal><TaxAmount currencyID="SEK">219.00</TaxAmount></TaxTotal>`
	// The single-TaxTotal block minimalUBL carries, replaced by the pair above.
	ublSingleTaxTotal = `<TaxTotal><TaxAmount>19.00</TaxAmount>
  <TaxSubtotal><TaxableAmount>100.00</TaxableAmount><TaxAmount>19.00</TaxAmount>
    <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></TaxSubtotal>
</TaxTotal>`
)

// twoTaxTotalUBL builds a conforming invoice with a EUR document currency and a
// SEK VAT accounting currency, emitting the two cac:TaxTotal elements in the
// given order.
func twoTaxTotalUBL(first, second string) string {
	withBT6 := strings.Replace(minimalUBL,
		"<DocumentCurrencyCode>EUR</DocumentCurrencyCode>",
		"<DocumentCurrencyCode>EUR</DocumentCurrencyCode><TaxCurrencyCode>SEK</TaxCurrencyCode>", 1)
	out := strings.Replace(withBT6, ublSingleTaxTotal, first+"\n"+second, 1)
	if out == withBT6 {
		panic("the minimalUBL TaxTotal block moved; update ublSingleTaxTotal")
	}
	return out
}

// TestTaxTotalSelectedByCurrency is the regression test for the document-currency
// cac:TaxTotal selection.
//
// An invoice using a VAT accounting currency (BT-6) carries two cac:TaxTotal —
// one in the document currency holding BT-110 and the BG-23 groups, one in the
// accounting currency holding BT-111. Neither EN 16931, the UBL binding nor
// Peppol BIS (R053/R054 constrain count and subtotal-presence, not sequence)
// fixes their order, so the mapper must select by currency rather than by
// document order. Reading the first one made a conforming invoice report
// BR-CO-18 ("shall at least have one VAT breakdown group"), BR-CO-15 against the
// SEK figure, and BR-S-01 — three fabricated findings — whenever a producer
// wrote the accounting-currency element first.
func TestTaxTotalSelectedByCurrency(t *testing.T) {
	docFirst := twoTaxTotalUBL(ublDocCurrencyTaxTotal, ublAccountingCurrencyTaxTotal)
	accFirst := twoTaxTotalUBL(ublAccountingCurrencyTaxTotal, ublDocCurrencyTaxTotal)

	for _, tc := range []struct{ name, xml string }{
		{"document-currency TaxTotal first", docFirst},
		{"accounting-currency TaxTotal first", accFirst},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v := findings(t, context.Background(), ValidateEN16931, []byte(tc.xml)); len(v) != 0 {
				t.Errorf("a conforming two-TaxTotal invoice reported %d violation(s): %v", len(v), v)
			}
		})
	}

	// Order must not be observable at all: the same document in either order is
	// the same invoice and must produce the same verdict.
	a := findings(t, context.Background(), ValidateEN16931, []byte(docFirst))
	b := findings(t, context.Background(), ValidateEN16931, []byte(accFirst))
	if len(a) != len(b) {
		t.Errorf("TaxTotal order changed the verdict: %d violation(s) one way, %d the other (%v / %v)",
			len(a), len(b), a, b)
	}

	// And the amount actually mapped as BT-110 is the document-currency one, not
	// the SEK figure: pin it through a rule that prints the value it read.
	brokenGrand := strings.Replace(accFirst,
		"<TaxInclusiveAmount>119.00</TaxInclusiveAmount>",
		"<TaxInclusiveAmount>999.00</TaxInclusiveAmount>", 1)
	var co15 string
	for _, v := range findings(t, context.Background(), ValidateEN16931, []byte(brokenGrand)) {
		if v.Rule == "BR-CO-15" {
			co15 = v.Message
		}
	}
	if co15 == "" {
		t.Fatal("expected BR-CO-15 to fire on an inconsistent grand total")
	}
	if !strings.Contains(co15, "BT-110=19.00") {
		t.Errorf("BR-CO-15 read the wrong VAT total: want the document-currency 19.00, got %q", co15)
	}
}

// TestTaxTotalDegenerateSelection pins the selection's behaviour on the shapes
// that are not the two-currency pair: none, one (the overwhelmingly common case,
// which must be returned whatever it is tagged with), an ambiguous pair, and a
// pair naming neither the document currency.
func TestTaxTotalDegenerateSelection(t *testing.T) {
	root := func(x string) *ciiNode {
		n, err := parseCII(newRun(nil), []byte(x))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return n
	}
	subtotal := `<TaxSubtotal><TaxableAmount>100.00</TaxableAmount></TaxSubtotal>`

	cases := []struct {
		name, xml, currency, wantTaxAmount string
		wantSubtotals                      int
	}{
		{
			name: "no TaxTotal", currency: "EUR",
			xml:           `<Invoice><DocumentCurrencyCode>EUR</DocumentCurrencyCode></Invoice>`,
			wantTaxAmount: "", wantSubtotals: 0,
		},
		{
			// The single-element path must never consult the currency: a producer
			// that mis-tags its only TaxTotal still has exactly one breakdown.
			name: "one TaxTotal tagged with a foreign currency", currency: "EUR",
			xml:           `<Invoice><TaxTotal><TaxAmount currencyID="SEK">7</TaxAmount>` + subtotal + `</TaxTotal></Invoice>`,
			wantTaxAmount: "7", wantSubtotals: 1,
		},
		{
			name: "two, both untagged: the one with subtotals wins", currency: "EUR",
			xml: `<Invoice><TaxTotal><TaxAmount>1</TaxAmount></TaxTotal>` +
				`<TaxTotal><TaxAmount>2</TaxAmount>` + subtotal + `</TaxTotal></Invoice>`,
			wantTaxAmount: "2", wantSubtotals: 1,
		},
		{
			name: "two with the same currencyID: the one with subtotals wins", currency: "EUR",
			xml: `<Invoice><TaxTotal><TaxAmount currencyID="EUR">1</TaxAmount></TaxTotal>` +
				`<TaxTotal><TaxAmount currencyID="EUR">2</TaxAmount>` + subtotal + `</TaxTotal></Invoice>`,
			wantTaxAmount: "2", wantSubtotals: 1,
		},
		{
			name: "neither names the document currency: fall back to the breakdown", currency: "EUR",
			xml: `<Invoice><TaxTotal><TaxAmount currencyID="SEK">1</TaxAmount></TaxTotal>` +
				`<TaxTotal><TaxAmount currencyID="NOK">2</TaxAmount>` + subtotal + `</TaxTotal></Invoice>`,
			wantTaxAmount: "2", wantSubtotals: 1,
		},
		{
			name: "neither names it and neither has a breakdown: document order", currency: "EUR",
			xml: `<Invoice><TaxTotal><TaxAmount currencyID="SEK">1</TaxAmount></TaxTotal>` +
				`<TaxTotal><TaxAmount currencyID="NOK">2</TaxAmount></TaxTotal></Invoice>`,
			wantTaxAmount: "1", wantSubtotals: 0,
		},
		{
			// Malformed: the breakdown sits in the accounting-currency element. The
			// explicit currency tag is the stronger signal and wins, so the invoice
			// is reported as missing its breakdown rather than accused of arithmetic
			// carried out across two currencies.
			name: "subtotals in the accounting-currency element", currency: "EUR",
			xml: `<Invoice><TaxTotal><TaxAmount currencyID="SEK">1</TaxAmount>` + subtotal + `</TaxTotal>` +
				`<TaxTotal><TaxAmount currencyID="EUR">2</TaxAmount></TaxTotal></Invoice>`,
			wantTaxAmount: "2", wantSubtotals: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := ublDocumentTaxTotal(root(tc.xml), tc.currency).orNil()
			if got := tt.str("TaxAmount"); got != tc.wantTaxAmount {
				t.Errorf("BT-110: got %q, want %q", got, tc.wantTaxAmount)
			}
			if got := len(tt.all("TaxSubtotal")); got != tc.wantSubtotals {
				t.Errorf("breakdown groups: got %d, want %d", got, tc.wantSubtotals)
			}
		})
	}
}

// TestNormDate pins what this package will and will not read as a calendar date.
// Both bindings' spellings normalise to the same fixed-width YYYYMMDD, a legal
// xs:date timezone offset is read and discarded, and anything that is not a date
// says so rather than yielding a value that would compare as if it were one.
func TestNormDate(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"2013-06-01", "20130601", true},   // UBL xs:date
		{"20130601", "20130601", true},     // CII UDT format 102
		{" 2013-06-01 ", "20130601", true}, // element text arrives untrimmed
		{"2013-06-01Z", "20130601", true},  // UTC designator
		{"2013-06-01+02:00", "20130601", true},
		{"2013-06-01-05:00", "20130601", true},
		{"20130601+02:00", "20130601", true},
		{"2013-06-01T09:30:00", "20130601", true}, // xs:dateTime in an xs:date slot
		{"", "", false},
		{"201306", "", false},      // too short to be a date
		{"201306011", "", false},   // a nine-digit run is not a date
		{"13-06-01", "", false},    // two-digit year
		{"2013/06/01", "", false},  // not a binding's separator
		{"1 June 2013", "", false}, // free text
		{"not-a-date", "", false},
	} {
		got, ok := normDate(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("normDate(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestInvoicingPeriodOrdersCalendarDays is the regression test for the period
// ordering rules BR-29 (BG-14) and BR-30 (BG-26).
//
// A legal xs:date may carry a timezone offset. Reducing a date to its digits
// turned "2024-02-01+02:00" into twelve of them, and against the eight of
// "20240201" the shorter string is a prefix of the longer and so compares LESS —
// so an invoicing period that starts and ends on the same calendar day was
// reported as ending before it began. The same-day case is precisely the one
// that must never fire.
func TestInvoicingPeriodOrdersCalendarDays(t *testing.T) {
	withPeriod := func(elem, start, end string) []byte {
		p := "<" + elem + "><StartDate>" + start + "</StartDate><EndDate>" + end + "</EndDate></" + elem + ">"
		return []byte(strings.Replace(minimalUBL,
			"<DocumentCurrencyCode>EUR</DocumentCurrencyCode>",
			"<DocumentCurrencyCode>EUR</DocumentCurrencyCode>"+p, 1))
	}
	for _, tc := range []struct {
		name, start, end string
		wantViolation    bool
	}{
		{"same day, offset on the start", "2024-02-01+02:00", "2024-02-01", false},
		{"same day, offset on the end", "2024-02-01", "2024-02-01+02:00", false},
		{"same day, negative offset on the start", "2024-02-01-05:00", "2024-02-01", false},
		{"same day, UTC designator on the end", "2024-02-01", "2024-02-01Z", false},
		{"same day, offsets on both", "2024-02-01+02:00", "2024-02-01-05:00", false},
		{"in order", "2024-02-01", "2024-03-01", false},
		{"in order across offsets", "2024-02-01+02:00", "2024-03-01", false},
		{"out of order", "2024-03-01", "2024-02-01", true},
		{"out of order across offsets", "2024-03-01+02:00", "2024-02-01", true},
		{"out of order, end offset", "2024-03-01", "2024-02-01-05:00", true},
		// Neither side was read as a date, so neither side may be ordered.
		{"unreadable start", "1 February 2024", "2024-02-01", false},
		{"unreadable end", "2024-02-01", "sometime in March", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := hasFacturXRule(findings(t, context.Background(), ValidateEN16931, withPeriod("InvoicePeriod", tc.start, tc.end)), "BR-29")
			if got != tc.wantViolation {
				t.Errorf("BR-29 for period %s..%s: got %v, want %v", tc.start, tc.end, got, tc.wantViolation)
			}
		})
	}

	// BR-30 is the same rule on the line period (BG-26) and takes the same path.
	sameDay := []byte(strings.Replace(minimalUBL, "<Item>",
		"<InvoicePeriod><StartDate>2024-02-01+02:00</StartDate><EndDate>2024-02-01</EndDate></InvoicePeriod><Item>", 1))
	if hasFacturXRule(findings(t, context.Background(), ValidateEN16931, sameDay), "BR-30") {
		t.Error("BR-30 must not fire for a line period that starts and ends on the same calendar day")
	}
	outOfOrder := []byte(strings.Replace(minimalUBL, "<Item>",
		"<InvoicePeriod><StartDate>2024-03-01+02:00</StartDate><EndDate>2024-02-01</EndDate></InvoicePeriod><Item>", 1))
	if !hasFacturXRule(findings(t, context.Background(), ValidateEN16931, outOfOrder), "BR-30") {
		t.Error("BR-30 should still fire for a line period that genuinely ends before it starts")
	}
}

// TestVATIdentifierCountryPrefix pins BR-CO-09 across the lengths a VAT
// identifier can have.
//
// The country-prefix extraction was guarded by len(v) >= 2, so a one-character
// identifier produced no finding at all while "XX123", "123456789" and "de123"
// were all correctly reported. A single character is not a country-prefixed VAT
// identifier under any reading of the rule; an absent one is other rules'
// business and stays a no-op here.
func TestVATIdentifierCountryPrefix(t *testing.T) {
	withSellerVATID := func(id string) []byte {
		return []byte(strings.Replace(minimalUBL,
			"<CompanyID>DE123456789</CompanyID>", "<CompanyID>"+id+"</CompanyID>", 1))
	}
	for _, tc := range []struct {
		id            string
		wantViolation bool
	}{
		{"DE123456789", false}, // an ISO 3166-1 alpha-2 prefix
		{"EL123456789", false}, // Greece's VAT prefix, which is not its country code
		{"D", true},            // too short to carry a prefix
		{"X", true},
		{"1", true},
		{"XX123", true},     // two characters, but not a country
		{"123456789", true}, // no prefix at all
		{"de123", true},     // lowercase is not the code
	} {
		t.Run(tc.id, func(t *testing.T) {
			got := hasFacturXRule(findings(t, context.Background(), ValidateEN16931, withSellerVATID(tc.id)), "BR-CO-09")
			if got != tc.wantViolation {
				t.Errorf("BR-CO-09 for seller VAT identifier %q: got %v, want %v", tc.id, got, tc.wantViolation)
			}
		})
	}

	// An absent identifier is not this rule's concern: BR-CO-26 reports that the
	// seller cannot be identified, and BR-CO-09 says nothing.
	noVATID := []byte(strings.Replace(minimalUBL,
		"<PartyTaxScheme><CompanyID>DE123456789</CompanyID><TaxScheme><ID>VAT</ID></TaxScheme></PartyTaxScheme>", "", 1))
	if hasFacturXRule(findings(t, context.Background(), ValidateEN16931, noVATID), "BR-CO-09") {
		t.Error("BR-CO-09 must not fire when there is no VAT identifier to prefix")
	}
}

// TestVATAmountTolerance pins the EN 16931 ±1 tolerance of the VAT-breakdown
// amount check (BR-CO-17): per-line rounding drift within one currency unit is
// accepted, a larger drift is flagged.
func TestVATAmountTolerance(t *testing.T) {
	// Exact tax is 100.00 * 19% = 19.00.
	within := strings.Replace(minimalUBL,
		"<TaxableAmount>100.00</TaxableAmount><TaxAmount>19.00</TaxAmount>",
		"<TaxableAmount>100.00</TaxableAmount><TaxAmount>19.60</TaxAmount>", 1)
	if hasFacturXRule(findings(t, context.Background(), ValidateEN16931, []byte(within)), "BR-CO-17") {
		t.Error("BR-CO-17 must not fire for a 0.60 rounding drift (within the ±1 tolerance)")
	}
	beyond := strings.Replace(minimalUBL,
		"<TaxableAmount>100.00</TaxableAmount><TaxAmount>19.00</TaxAmount>",
		"<TaxableAmount>100.00</TaxableAmount><TaxAmount>21.00</TaxAmount>", 1)
	if !hasFacturXRule(findings(t, context.Background(), ValidateEN16931, []byte(beyond)), "BR-CO-17") {
		t.Error("BR-CO-17 should fire for a 2.00 drift (beyond the ±1 tolerance)")
	}
}

// TestBindingRuleIDsPerSyntax verifies that a binding-specific rule is reported
// with the identifier of the invoice's own syntax: the same defect (inconsistent
// payment means codes) is CII-SR-467 on a CII invoice and UBL-SR-47 on a UBL one,
// and neither reports the other syntax's identifier.
func TestBindingRuleIDsPerSyntax(t *testing.T) {
	cii := strings.Replace(validCII, "<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>",
		"<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>"+
			"<SpecifiedTradeSettlementPaymentMeans><TypeCode>30</TypeCode></SpecifiedTradeSettlementPaymentMeans>"+
			"<SpecifiedTradeSettlementPaymentMeans><TypeCode>58</TypeCode></SpecifiedTradeSettlementPaymentMeans>", 1)
	v := findings(t, context.Background(), ValidateEN16931, []byte(cii))
	if !hasFacturXRule(v, "CII-SR-467") {
		t.Errorf("CII invoice should report CII-SR-467; got %v", v)
	}
	if hasFacturXRule(v, "UBL-SR-47") {
		t.Error("CII invoice must not report the UBL identifier UBL-SR-47")
	}

	ubl := strings.Replace(minimalUBL, "</Invoice>",
		"<PaymentMeans><PaymentMeansCode>30</PaymentMeansCode></PaymentMeans>"+
			"<PaymentMeans><PaymentMeansCode>58</PaymentMeansCode></PaymentMeans></Invoice>", 1)
	v = findings(t, context.Background(), ValidateEN16931, []byte(ubl))
	if !hasFacturXRule(v, "UBL-SR-47") {
		t.Errorf("UBL invoice should report UBL-SR-47; got %v", v)
	}
	if hasFacturXRule(v, "CII-SR-467") {
		t.Error("UBL invoice must not report the CII identifier CII-SR-467")
	}
}

// TestValidateUBLCalcMutation confirms a total-consistency rule fires when a
// document total is made inconsistent.
func TestValidateUBLCalcMutation(t *testing.T) {
	broken := bytes.Replace([]byte(minimalUBL),
		[]byte("<TaxInclusiveAmount>119.00</TaxInclusiveAmount>"),
		[]byte("<TaxInclusiveAmount>999.00</TaxInclusiveAmount>"), 1)
	v := findings(t, context.Background(), ValidateEN16931, broken)
	found := false
	for _, x := range v {
		if x.Rule == "BR-CO-15" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected BR-CO-15 (total with VAT = without + VAT) to fire; got %v", v)
	}
}
