package formalis

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// These tests pin the properties limits.go promises: that a stopped run says so
// rather than returning nothing, that the guards fire before the process does,
// and that the two shapes of input that used to be quadratic are now linear.

// bigCIIInvoice returns a well-formed CII invoice with n line items.
func bigCIIInvoice(n int) []byte {
	const line = `<IncludedSupplyChainTradeLineItem>` +
		`<AssociatedDocumentLineDocument><LineID>1</LineID></AssociatedDocumentLineDocument>` +
		`<SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>` +
		`<SpecifiedLineTradeSettlement>` +
		`<ApplicableTradeTax><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>` +
		`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>1.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>` +
		`</SpecifiedLineTradeSettlement></IncludedSupplyChainTradeLineItem>`
	var b strings.Builder
	b.WriteString(`<CrossIndustryInvoice><SupplyChainTradeTransaction>`)
	for i := 0; i < n; i++ {
		b.WriteString(line)
	}
	b.WriteString(`</SupplyChainTradeTransaction></CrossIndustryInvoice>`)
	return []byte(b.String())
}

// checkerCount reports how many of v are checker violations.
func checkerCount(v []Violation) int {
	n := 0
	for _, e := range v {
		if IsCheckerViolation(e) {
			n++
		}
	}
	return n
}

// TestCancelledRunIsNeverClean is the property that matters most: a caller
// testing len(v) == 0 for "valid" must not get that answer from a run that
// stopped before it had looked.
func TestCancelledRunIsNeverClean(t *testing.T) {
	// validCII is clean, so an uncancelled run really does return nothing —
	// which is what makes the cancelled run's non-empty result meaningful.
	if v := Validate(context.Background(), []byte(validCII), ProfileEN16931).Violations; len(v) != 0 {
		t.Fatalf("fixture is not clean: %v", v)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v := Validate(ctx, []byte(validCII), ProfileEN16931).Violations
	if len(v) == 0 {
		t.Fatal("a cancelled run returned no violations, which is indistinguishable from a valid invoice")
	}
	if checkerCount(v) != len(v) {
		t.Errorf("a cancelled run reported a non-checker violation: %v", v)
	}
	if !strings.Contains(v[0].Message, "context-canceled") {
		t.Errorf("the trip does not name its guard: %q", v[0].Message)
	}
	if v[0].Rule != RuleLimit {
		t.Errorf("Rule = %q, want %q", v[0].Rule, RuleLimit)
	}
}

// TestCancelledRunIsPrompt measures the latency rather than merely asserting the
// call returned: cancellation that takes seconds to take effect is the defect,
// not the absence of cancellation.
func TestCancelledRunIsPrompt(t *testing.T) {
	xml := bigCIIInvoice(60000)

	t0 := time.Now()
	Validate(context.Background(), xml, ProfileEN16931)
	full := time.Since(t0)
	if full < 50*time.Millisecond {
		t.Skipf("fixture validates in %v, too fast to measure cancellation against", full)
	}

	const cancelAfter = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
	defer cancel()
	t1 := time.Now()
	v := Validate(ctx, xml, ProfileEN16931).Violations
	latency := time.Since(t1) - cancelAfter

	if latency > full/4 {
		t.Errorf("cancellation took %v to take effect (full run %v); the checks are too coarse", latency, full)
	}
	if checkerCount(v) == 0 {
		t.Errorf("a cancelled run reported no checker violation: %v", v)
	}
	t.Logf("full run %v; cancelled after %v, returned %v later", full, cancelAfter, latency)
}

// TestDeepNestingIsGuarded pins the guard that replaced a fatal stack overflow.
// The recursive tree walks would exhaust the goroutine stack on a document this
// deep, and a stack overflow is not recoverable — no defer/recover anywhere up
// the stack, including pdf0's around ValidateFacturX, can turn it back into a
// finding. So this test failing means a crash, not a failed assertion.
func TestDeepNestingIsGuarded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<CrossIndustryInvoice>`)
	const depth = maxDepth * 4
	for i := 0; i < depth; i++ {
		b.WriteString("<a>")
	}
	for i := 0; i < depth; i++ {
		b.WriteString("</a>")
	}
	b.WriteString(`</CrossIndustryInvoice>`)

	v := Validate(context.Background(), []byte(b.String()), ProfileEN16931).Violations
	if len(v) == 0 {
		t.Fatal("a document nested past the cap returned no violations")
	}
	if checkerCount(v) != len(v) {
		t.Errorf("the depth guard reported a violation about the document: %v", v)
	}
	if !strings.Contains(v[0].Message, "xml-depth") {
		t.Errorf("the trip does not name its guard: %q", v[0].Message)
	}
	// A document just inside the cap must still validate normally, or the guard
	// would be rejecting legitimate invoices.
	if strings.Contains(fmt.Sprint(Validate(context.Background(), []byte(validCII), ProfileEN16931).Violations), "xml-depth") {
		t.Error("the depth guard fired on an ordinary invoice")
	}
}

// TestParseScalesLinearly pins the fix for the character-data accumulator. Text
// split into many tokens by comments used to be recopied per token: 6.4 MB took
// 33 s, and the growth was quadratic. Doubling the input must roughly double the
// time, not quadruple it.
func TestParseScalesLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	split := func(n int) []byte {
		return []byte(`<CrossIndustryInvoice><SupplyChainTradeTransaction>` +
			`<IncludedSupplyChainTradeLineItem><SpecifiedTradeProduct><Name>` +
			strings.Repeat("x<!---->", n) +
			`</Name></SpecifiedTradeProduct></IncludedSupplyChainTradeLineItem>` +
			`</SupplyChainTradeTransaction></CrossIndustryInvoice>`)
	}
	assertLinear(t, func(n int) []byte { return split(n) }, 100000)
}

// TestVATSumsScaleLinearly pins the fix for validateVATTaxableSums, which
// re-parsed every line amount for every VAT breakdown: 7.3 MB took 1.7 s and
// grew as the square.
func TestVATSumsScaleLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	cross := func(n int) []byte {
		const line = `<IncludedSupplyChainTradeLineItem>` +
			`<AssociatedDocumentLineDocument><LineID>1</LineID></AssociatedDocumentLineDocument>` +
			`<SpecifiedTradeProduct><Name>W</Name></SpecifiedTradeProduct>` +
			`<SpecifiedLineTradeSettlement>` +
			`<ApplicableTradeTax><CategoryCode>S</CategoryCode><RateApplicablePercent>19</RateApplicablePercent></ApplicableTradeTax>` +
			`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>1.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>` +
			`</SpecifiedLineTradeSettlement></IncludedSupplyChainTradeLineItem>`
		var b strings.Builder
		b.WriteString(`<CrossIndustryInvoice><SupplyChainTradeTransaction>`)
		for i := 0; i < n; i++ {
			b.WriteString(line)
		}
		b.WriteString(`<ApplicableHeaderTradeSettlement>`)
		for i := 0; i < n; i++ {
			// Distinct rates, so the per-(category,rate) memo cannot absorb them
			// and the scan itself has to be linear.
			fmt.Fprintf(&b, `<ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode>`+
				`<RateApplicablePercent>%d</RateApplicablePercent><BasisAmount>1.00</BasisAmount>`+
				`<CalculatedAmount>0.19</CalculatedAmount></ApplicableTradeTax>`, i%90+1)
		}
		b.WriteString(`</ApplicableHeaderTradeSettlement></SupplyChainTradeTransaction></CrossIndustryInvoice>`)
		return []byte(b.String())
	}
	assertLinear(t, cross, 2000)
}

// assertLinear times build(n) and build(4n) and fails if the larger took more
// than 8x the smaller. A linear implementation costs about 4x; the quadratics
// this guards against cost 16x, so the threshold separates them with room for
// the noise of a shared machine.
func assertLinear(t *testing.T, build func(int) []byte, n int) {
	t.Helper()
	time1 := timeValidate(t, build(n))
	time4 := timeValidate(t, build(4*n))
	if time1 < time.Millisecond {
		t.Skipf("baseline %v is too small to measure a ratio against", time1)
	}
	ratio := float64(time4) / float64(time1)
	t.Logf("n=%d took %v, n=%d took %v (ratio %.1fx, linear is ~4x)", n, time1, 4*n, time4, ratio)
	if ratio > 8 {
		t.Errorf("quadratic growth: 4x the input took %.1fx the time (%v -> %v)", ratio, time1, time4)
	}
}

func timeValidate(t *testing.T, xml []byte) time.Duration {
	t.Helper()
	best := time.Duration(1<<63 - 1)
	for i := 0; i < 3; i++ {
		t0 := time.Now()
		Validate(context.Background(), xml, ProfileEN16931)
		if d := time.Since(t0); d < best {
			best = d
		}
	}
	return best
}

// TestNilContextDoesNotPanic covers the caller who reaches an exported entry
// point with a nil context. That is a mistake, but it must not become a crash
// inside a validator running on untrusted input.
func TestNilContextDoesNotPanic(t *testing.T) {
	//lint:ignore SA1012 deliberately testing the nil case
	if v := Validate(nil, []byte(validCII), ProfileEN16931).Violations; len(v) != 0 {
		t.Errorf("a nil context changed the result: %v", v)
	}
}

// TestVATSumBudgetDeclinesRatherThanAccuses pins the reporting contract for a
// tripped budget: the check that could not be completed reports the trip, and
// must not report BR-*-08 on the strength of a partial sum.
func TestVATSumBudgetDeclinesRatherThanAccuses(t *testing.T) {
	inv := &en16931Invoice{}
	for i := 0; i < 100; i++ {
		inv.lines = append(inv.lines, invoiceLine{vatCategory: "S", vatRate: "20", netAmount: "1.00"})
	}
	// A breakdown whose basis disagrees with the sum: with budget it would fire
	// BR-S-08.
	inv.vatBreakdowns = append(inv.vatBreakdowns, vatBreakdown{
		category: "S", rate: "20", basis: "999.00", calc: "199.80",
	})

	var fired []Violation
	add := adder(&fired, SourceEN16931)

	r := newRun(context.Background())
	r.vatWork = 0 // exhausted
	validateVATTaxableSums(r, inv, add)

	if len(fired) != 0 {
		t.Errorf("an exhausted budget still accused the invoice: %v", fired)
	}
	if len(r.trips) == 0 {
		t.Fatal("an exhausted budget recorded no trip")
	}
	if !strings.Contains(r.trips[0].Message, "vat-sum-work") {
		t.Errorf("the trip does not name its guard: %q", r.trips[0].Message)
	}

	// With budget, the same input does fire — so the test above is really
	// measuring the guard and not a fixture that never violates.
	fired = nil
	validateVATTaxableSums(newRun(context.Background()), inv, add)
	if len(fired) == 0 {
		t.Error("with budget the fixture reported nothing; the guard test proves nothing")
	}
}

// flatUBLBE builds a well-formed UBL.BE invoice of n padding elements: enough
// to weigh against the element budget, small enough to parse in well under a
// second. The CustomizationID is what makes ValidateCIUS route it to the
// UBL.BE validator, which is the longest dispatch path in the package.
func flatUBLBE(n int) []byte {
	var b strings.Builder
	b.Grow(n*4 + 256)
	b.WriteString(`<?xml version="1.0"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2">`)
	b.WriteString(`<CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:UBL.BE:1.0.0.20180214</CustomizationID>`)
	// The CustomizationID costs one element, so n padding siblings under the
	// root make n+2 in all.
	for i := 0; i < n-2; i++ {
		b.WriteString(`<a/>`)
	}
	b.WriteString(`</Invoice>`)
	return []byte(b.String())
}

// TestNodeBudgetIsPerDocumentNotPerEntryPoint pins what maxNodes means.
//
// The budget belongs to the document, not to the number of layers a call
// happened to pass through. It used to belong to the parse: run.nodes is spent
// one element at a time by parseCII, every validator took raw bytes and parsed
// them itself, and the dispatchers parsed more than once against the same run —
// ValidateCIUS read the document once only to learn its CIUS from BT-24,
// discarded the result and handed the bytes to a validator that read them
// again, and ValidateUBLBE read them a second time purely to recover the tree
// for its ubl-BE-* rules. So the effective ceiling was maxNodes, maxNodes/2 or
// maxNodes/3 depending on the entry point, and the same bytes were "readable"
// through ValidateUBLBE and "too large" through ValidateCIUS.
//
// That is the failure this test exists to prevent, and it is worse than a
// performance bug: the guard exists so a hostile document cannot exhaust
// memory, and it was firing on a benign one because the package chose to read
// it three times.
func TestNodeBudgetIsPerDocumentNotPerEntryPoint(t *testing.T) {
	entryPoints := map[string]func(context.Context, []byte) Report{
		"Validate":      func(c context.Context, b []byte) Report { return Validate(c, b, ProfileEN16931) },
		"ValidateUBLBE": ValidateUBLBE,
		"ValidateCIUS":  ValidateCIUS,
	}

	// Comfortably inside the budget, but more than a third of it: a document
	// this size trips nothing when read once and trips when read three times.
	inside := flatUBLBE(400_000)
	for name, fn := range entryPoints {
		v := fn(context.Background(), inside).Violations
		if got := checkerCount(v); got != 0 {
			t.Errorf("%s reported %d limit violations on a 400000-element document (budget %d): %v",
				name, got, maxNodes, v)
		}
		// The findings a stopped run would have skipped are the point of not
		// stopping, so check the document was really validated rather than
		// merely not refused.
		if len(v) == 0 {
			t.Errorf("%s reported nothing at all on a document that is not a conforming invoice", name)
		}
	}

	// One element past the budget trips exactly once, through every entry
	// point, and never as a statement about the XML — which is well-formed.
	over := flatUBLBE(maxNodes + 1)
	for name, fn := range entryPoints {
		rep := fn(context.Background(), over)
		v := rep.Violations
		// A tripped budget is the other stopped-run case Complete has to
		// answer for, and the one a caller is least likely to think about.
		if rep.Complete || rep.Conformant() {
			t.Errorf("%s reported an over-budget run as Complete=%v Conformant=%v; the checks that had not "+
				"run were skipped", name, rep.Complete, rep.Conformant())
		}
		if got := checkerCount(v); got != 1 {
			t.Errorf("%s reported %d limit violations on a %d-element document, want exactly 1: %v",
				name, got, maxNodes+1, v)
		}
		for _, e := range v {
			if e.Rule == RuleSyntax {
				t.Errorf("%s reported a well-formed over-budget document as malformed: %s", name, e.Message)
			}
		}
	}
}

// TestStoppedRunIsNotReportedAsBadSyntax pins the other half of the honesty
// contract: a run that stopped must not be reported as a malformed document.
// Every exported validator routes its parse error through syntaxViolation, so
// this walks a representative set of them rather than only the one that had the
// bug (ValidateOrderXML reported "not a well-formed Cross Industry Order").
func TestStoppedRunIsNotReportedAsBadSyntax(t *testing.T) {
	cases := map[string]func(ctx context.Context, b []byte) Report{
		"Validate":          func(c context.Context, b []byte) Report { return Validate(c, b, ProfileEN16931) },
		"ValidateOrderXML":  ValidateOrderXML,
		"ValidateCIUS":      ValidateCIUS,
		"ValidateXRechnung": ValidateXRechnung,
		"ValidatePeppol":    ValidatePeppol,
		"ValidateZATCA":     ValidateZATCA,
		"ValidateFinvoice":  ValidateFinvoice,
		"ValidateFatturaPA": ValidateFatturaPA,
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			v := fn(ctx, []byte(validCII)).Violations
			if len(v) == 0 {
				t.Fatal("a cancelled run returned nothing, which reads as valid")
			}
			for _, e := range v {
				if e.Rule == RuleSyntax {
					t.Errorf("a cancelled run reported the document as malformed: %s", e.Message)
				}
				if !IsCheckerViolation(e) {
					t.Errorf("a cancelled run reported a document violation: %s: %s", e.Rule, e.Message)
				}
			}
		})
	}
}
