package formalis

import (
	"context"
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

	runRuleCases(t, cases)
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
