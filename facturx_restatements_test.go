package formalis

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The guards on the 24 fatal BR-FXEXT-* restatements: the table against the
// artefact in both directions, a firing verdict on every one of them, and the
// severity read back out of the files.
//
// Everything here reads the Schematrons with the XML decoder facturx_test.go
// sets up (C31), and everything is derived from the files rather than asserted
// against a transcription, which is the shape the rest of this package's Factur-X
// guards already have.

// TestFacturXRestatementsMatchTheArtefact holds facturXRestatementRules to the
// five Schematrons in both directions.
//
// Forward: every one of the 24 rows names an identifier the profile it claims
// publishes, that profile only, with no flag — which within this artefact is what
// makes it fatal — and restates a CEN identifier facturXCENOmissions records as
// dropped with that replacement.
//
// Backward: every unflagged BR-FXEXT-* identifier any profile publishes is either
// one of Factur-X's own nine or one of these 24. Nothing may be published and
// accounted for nowhere, which is the C39/C43 failure mode: a guard that
// enumerates through a pattern only finds what its author anticipated, so this
// one enumerates the files and classifies afterwards.
func TestFacturXRestatementsMatchTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	// identifier -> the profiles that publish it, with the flag each carries.
	published := map[string]map[Profile]Severity{}
	for _, p := range profiles {
		for id, x := range fxNamed(fxDecode(t, dir, p)) {
			if !strings.HasPrefix(id, "BR-FXEXT") {
				continue
			}
			if published[id] == nil {
				published[id] = map[Profile]Severity{}
			}
			published[id][p] = x.a.severity()
		}
	}
	if len(published) < minFacturXExtensionIDs {
		t.Fatalf("read %d BR-FXEXT-* identifiers, want at least %d", len(published), minFacturXExtensionIDs)
	}

	omission := map[string]string{} // replacement -> the CEN identifier it restates
	for _, o := range facturXCENOmissions {
		if o.replacedBy != "" {
			omission[o.replacedBy] = o.cen
		}
	}

	table := map[string]facturXRestatement{}
	for _, rs := range facturXRestatementRules {
		if _, dup := table[rs.id]; dup {
			t.Errorf("facturXRestatementRules names %s twice", rs.id)
		}
		table[rs.id] = rs

		where, ok := published[rs.id]
		if !ok {
			t.Errorf("facturXRestatementRules names %s and no Factur-X Schematron publishes it", rs.id)
			continue
		}
		if len(where) != 1 {
			var ps []string
			for p := range where {
				ps = append(ps, string(p))
			}
			sort.Strings(ps)
			t.Errorf("%s is published by %v and facturXRestatementRules gives it one profile; the profile field is a "+
				"single value because FNFE publishes each of these in exactly one tier", rs.id, ps)
			continue
		}
		sev, inProfile := where[rs.profile]
		if !inProfile {
			t.Errorf("facturXRestatementRules puts %s in the %q profile and FNFE publishes it in another", rs.id, string(rs.profile))
			continue
		}
		if sev != SeverityFatal {
			t.Errorf("%s is evaluated as fatal and FNFE flags it %s", rs.id, sev)
		}
		// BR-FXEXT-G-08 is the one row whose identifier facturXCENOmissions does
		// not carry as a replacedBy: that table records what the *EXTENDED*
		// profile drops, and this rule is MINIMUM's, published beside CEN's own
		// BR-G-08 rather than in place of it. The CEN identifier it restates is
		// still checked.
		if want, ok := omission[rs.id]; ok && want != rs.cen {
			t.Errorf("facturXRestatementRules says %s restates %s and facturXCENOmissions says %s", rs.id, rs.cen, want)
		}
		if rs.id == "BR-FXEXT-G-08" {
			if _, ok := fxNamed(fxDecode(t, dir, ProfileMinimum))[rs.cen]; !ok {
				t.Errorf("BR-FXEXT-G-08 is recorded as restating %s and the MINIMUM Schematron does not publish %s "+
					"beside it, which is the whole reason this row is not in facturXCENOmissions", rs.cen, rs.cen)
			}
		}
	}

	own := map[string]bool{}
	for _, id := range facturXExtensionRules {
		own[id] = true
	}
	var unaccounted []string
	for id, where := range published {
		fatal := false
		for _, sev := range where {
			if sev == SeverityFatal {
				fatal = true
			}
		}
		if !fatal || own[id] {
			continue
		}
		if _, ok := table[id]; !ok {
			unaccounted = append(unaccounted, id)
		}
	}
	sort.Strings(unaccounted)
	for _, id := range unaccounted {
		t.Errorf("FNFE publishes %s unflagged, which in these files means fatal, and it is neither one of Factur-X's "+
			"own nine nor a row of facturXRestatementRules", id)
	}
	if n := len(facturXRestatementRules); n != 24 {
		t.Errorf("facturXRestatementRules holds %d rows and Coverage(SourceFacturX) and facturx_restatements.go say 24", n)
	}
	t.Logf("Factur-X restatements: %d rows, %d in EXTENDED and %d in MINIMUM, all fatal in the artefact",
		len(facturXRestatementRules), len(facturXRestatementRules)-1, 1)
}

// TestEveryFacturXRestatementFires is the firing verdict C41 asks for, per rule.
//
// A rule present in the table, reachable in the tree and inert passes every other
// guard here: the identifier is in the coverage accounting, the context is
// reached, the corpus stays clean. PR 24 shipped exactly that — a whole rule
// shape that always answered true — and the only thing that catches it is a
// document per rule that makes it report and a second that does not.
//
// The pairs are deliberately minimal edits of a document that is otherwise clean
// under the profile, so "fired" and "did not fire" are attributable to the one
// term that moved.
func TestEveryFacturXRestatementFires(t *testing.T) {
	ctx := context.Background()
	for _, tc := range facturXRestatementFixtures() {
		t.Run(tc.rule, func(t *testing.T) {
			if !fxRestatementReports(t, ctx, tc.bad, tc.rule, tc.profile) {
				t.Errorf("%s did not fire on the document written to break it; a rule that never reports is a rule "+
					"that is present and inert, which every other guard here passes", tc.rule)
			}
			if fxRestatementReports(t, ctx, tc.good, tc.rule, tc.profile) {
				t.Errorf("%s fired on the document written to satisfy it", tc.rule)
			}
			// And it is published by one profile, so no other may report it —
			// including on the document that breaks it.
			for _, p := range profiles {
				if p == tc.profile {
					continue
				}
				r, err := Validate(ctx, []byte(tc.bad), p)
				if err != nil {
					t.Fatalf("%v", err)
				}
				for _, v := range r.Violations {
					if v.Rule == tc.rule {
						t.Errorf("%s was reported at profile %q, which does not publish it", tc.rule, string(p))
					}
				}
			}
		})
	}
	fixtures := map[string]bool{}
	for _, tc := range facturXRestatementFixtures() {
		fixtures[tc.rule] = true
	}
	for _, rs := range facturXRestatementRules {
		if !fixtures[rs.id] {
			t.Errorf("%s is evaluated and has no firing fixture; add one rather than trusting the rule body", rs.id)
		}
	}
}

// fxRestatementReports reports whether validating doc at the profile that
// publishes the rule yields it, and checks the Source while it is there.
func fxRestatementReports(t *testing.T, ctx context.Context, doc, rule string, p Profile) bool {
	t.Helper()
	if strings.HasPrefix(doc, "MISSING ANCHOR") {
		t.Fatal(doc)
	}
	r, err := Validate(ctx, []byte(doc), p)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, v := range r.Violations {
		if v.Rule != rule {
			continue
		}
		if v.Source != SourceFacturX {
			t.Errorf("%s came back under Source %q, want %q", rule, v.Source, SourceFacturX)
		}
		if v.Severity != SeverityFatal {
			t.Errorf("%s came back %s and FNFE leaves it unflagged, which facturx.go reads as fatal", rule, v.Severity)
		}
		return true
	}
	return false
}

// facturXRestatementFixture is one rule's pair.
type facturXRestatementFixture struct {
	rule      string
	profile   Profile
	bad, good string
}

// fxExtCII is validCII declaring the EXTENDED profile and carrying the sub-line
// subtype on its single line, which is the smallest document that is clean under
// the EXTENDED tier and reaches every context in this file. The line is marked
// DETAIL explicitly so that a fixture can move it to GROUP and watch a rule go
// quiet.
var fxExtCII = strings.Replace(
	strings.Replace(validCII,
		"<ID>urn:cen.eu:en16931:2017</ID>",
		"<ID>urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended</ID>", 1),
	"<AssociatedDocumentLineDocument><LineID>1</LineID></AssociatedDocumentLineDocument>",
	"<AssociatedDocumentLineDocument><LineID>1</LineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode></AssociatedDocumentLineDocument>", 1)

// fxExt edits fxExtCII, failing loudly if the anchor moved.
func fxExt(anchor, replacement string) string {
	out := strings.Replace(fxExtCII, anchor, replacement, 1)
	if out == fxExtCII {
		return "MISSING ANCHOR: " + anchor
	}
	return out
}

// fxMinCII is a MINIMUM-tier document carrying a category-G VAT breakdown, which
// is the only shape BR-FXEXT-G-08 has a context in. MINIMUM's own data model
// admits the breakdown; the line items are absent, so FNFE's summation is over an
// empty set and the taxable amount has to be the charges less the allowances,
// which here is zero.
const fxMinCII = `<CrossIndustryInvoice>
  <ExchangedDocumentContext><GuidelineSpecifiedDocumentContextParameter><ID>urn:factur-x.eu:1p0:minimum</ID></GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext>
  <ExchangedDocument><ID>INV-3</ID><TypeCode>380</TypeCode><IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime></ExchangedDocument>
  <SupplyChainTradeTransaction>
    <ApplicableHeaderTradeAgreement>
      <SellerTradeParty><Name>Seller Co</Name><PostalTradeAddress><CountryID>FR</CountryID></PostalTradeAddress><SpecifiedTaxRegistration><ID schemeID="VA">FR12345678</ID></SpecifiedTaxRegistration></SellerTradeParty>
      <BuyerTradeParty><Name>Buyer Co</Name></BuyerTradeParty>
    </ApplicableHeaderTradeAgreement>
    <ApplicableHeaderTradeSettlement>
      <InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>
      <ApplicableTradeTax><TypeCode>VAT</TypeCode><CalculatedAmount>0.00</CalculatedAmount><BasisAmount>0.00</BasisAmount><CategoryCode>G</CategoryCode><RateApplicablePercent>0.00</RateApplicablePercent></ApplicableTradeTax>
      <SpecifiedTradeSettlementHeaderMonetarySummation>
        <TaxBasisTotalAmount>0.00</TaxBasisTotalAmount>
        <TaxTotalAmount currencyID="EUR">0.00</TaxTotalAmount>
        <GrandTotalAmount>0.00</GrandTotalAmount>
        <DuePayableAmount>0.00</DuePayableAmount>
      </SpecifiedTradeSettlementHeaderMonetarySummation>
    </ApplicableHeaderTradeSettlement>
  </SupplyChainTradeTransaction>
</CrossIndustryInvoice>`

// fxVATCII is an EXTENDED document whose single line, single document allowance,
// single document charge and single logistics service charge all carry the same
// VAT category and rate, so one edit to the breakdown's taxable amount breaks the
// summation for whichever category the fixture rewrites it to.
//
// The arithmetic: line 100,00 + charge 10,00 + logistics 5,00 - allowance 15,00 =
// 100,00, which is the breakdown's BT-116 and the invoice's BT-109.
const fxVATCII = `<CrossIndustryInvoice>
  <ExchangedDocumentContext><GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended</ID></GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext>
  <ExchangedDocument><ID>INV-4</ID><TypeCode>380</TypeCode><IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime></ExchangedDocument>
  <SupplyChainTradeTransaction>
    <IncludedSupplyChainTradeLineItem>
      <AssociatedDocumentLineDocument><LineID>1</LineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode></AssociatedDocumentLineDocument>
      <SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>
      <SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>
      <SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>
      <SpecifiedLineTradeSettlement><ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax><SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>100.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation></SpecifiedLineTradeSettlement>
    </IncludedSupplyChainTradeLineItem>
    <ApplicableHeaderTradeAgreement>
      <SellerTradeParty><Name>Seller Co</Name><PostalTradeAddress><CountryID>FR</CountryID></PostalTradeAddress><SpecifiedTaxRegistration><ID schemeID="VA">FR12345678</ID></SpecifiedTaxRegistration></SellerTradeParty>
      <BuyerTradeParty><Name>Buyer Co</Name><PostalTradeAddress><CountryID>FR</CountryID></PostalTradeAddress></BuyerTradeParty>
    </ApplicableHeaderTradeAgreement>
    <ApplicableHeaderTradeSettlement>
      <InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>
      <SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>false</Indicator></ChargeIndicator><ActualAmount>15.00</ActualAmount><Reason>Discount</Reason><CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax></SpecifiedTradeAllowanceCharge>
      <SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>true</Indicator></ChargeIndicator><ActualAmount>10.00</ActualAmount><Reason>Packing</Reason><CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax></SpecifiedTradeAllowanceCharge>
      <SpecifiedLogisticsServiceCharge><Description>Freight</Description><AppliedAmount>5.00</AppliedAmount><AppliedTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></AppliedTradeTax></SpecifiedLogisticsServiceCharge>
      <ApplicableTradeTax><TypeCode>VAT</TypeCode><CalculatedAmount>20.00</CalculatedAmount><BasisAmount>100.00</BasisAmount><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>
      <SpecifiedTradeSettlementHeaderMonetarySummation>
        <LineTotalAmount>100.00</LineTotalAmount>
        <AllowanceTotalAmount>15.00</AllowanceTotalAmount>
        <ChargeTotalAmount>15.00</ChargeTotalAmount>
        <TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>
        <TaxTotalAmount currencyID="EUR">20.00</TaxTotalAmount>
        <GrandTotalAmount>120.00</GrandTotalAmount>
        <DuePayableAmount>120.00</DuePayableAmount>
      </SpecifiedTradeSettlementHeaderMonetarySummation>
    </ApplicableHeaderTradeSettlement>
  </SupplyChainTradeTransaction>
</CrossIndustryInvoice>`

// fxVAT edits fxVATCII, failing loudly if the anchor moved.
func fxVAT(anchor, replacement string) string {
	out := strings.Replace(fxVATCII, anchor, replacement, 1)
	if out == fxVATCII {
		return "MISSING ANCHOR: " + anchor
	}
	return out
}

// fxVATCategory rewrites fxVATCII from the standard category to another one, so
// the same arithmetic can be broken under each of the nine -08b identifiers.
// The rate goes to zero for the categories whose rate must be zero, and the VAT
// total with it, keeping the document otherwise conformant.
func fxVATCategory(category string) string {
	doc := strings.ReplaceAll(fxVATCII, "<CategoryCode>S</CategoryCode>", "<CategoryCode>"+category+"</CategoryCode>")
	if category == "L" || category == "M" {
		// AF and AG keep a non-zero rate: they are the Canary Islands and Ceuta /
		// Melilla categories and carry a real one.
		return doc
	}
	doc = strings.ReplaceAll(doc, "<RateApplicablePercent>20.00</RateApplicablePercent>", "<RateApplicablePercent>0.00</RateApplicablePercent>")
	doc = strings.Replace(doc, "<CalculatedAmount>20.00</CalculatedAmount>", "<CalculatedAmount>0.00</CalculatedAmount>", 1)
	doc = strings.Replace(doc, "<TaxTotalAmount currencyID=\"EUR\">20.00</TaxTotalAmount>", "<TaxTotalAmount currencyID=\"EUR\">0.00</TaxTotalAmount>", 1)
	doc = strings.Replace(doc, "<GrandTotalAmount>120.00</GrandTotalAmount>", "<GrandTotalAmount>100.00</GrandTotalAmount>", 1)
	doc = strings.Replace(doc, "<DuePayableAmount>120.00</DuePayableAmount>", "<DuePayableAmount>100.00</DuePayableAmount>", 1)
	if category == "O" {
		// Category O carries no rate at all, which is also the one -08b shape
		// that binds no $rate variable.
		doc = strings.ReplaceAll(doc, "<RateApplicablePercent>0.00</RateApplicablePercent>", "")
	}
	return doc
}

// fxBreakTaxableAmount moves a category's taxable amount far enough off the sum
// to clear FNFE's 0,01 x operand-count tolerance, which for these documents is
// 0,04.
func fxBreakTaxableAmount(doc string) string {
	out := strings.Replace(doc, "<BasisAmount>100.00</BasisAmount>", "<BasisAmount>90.00</BasisAmount>", 1)
	if out == doc {
		return "MISSING ANCHOR: <BasisAmount>100.00</BasisAmount>"
	}
	return out
}

// facturXRestatementFixtures is the pair for each of the 24. It is a function
// rather than a var so the severity guard can walk the same documents.
func facturXRestatementFixtures() []facturXRestatementFixture {
	ext := ProfileExtended
	out := []facturXRestatementFixture{{
		rule:    "BR-FXEXT-BR-22",
		profile: ext,
		bad:     fxExt(`<SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>`, `<SpecifiedLineTradeDelivery/>`),
		good:    fxExtCII,
	}, {
		rule:    "BR-FXEXT-BR-23",
		profile: ext,
		bad:     fxExt(`<BilledQuantity unitCode="C62">1</BilledQuantity>`, `<BilledQuantity>1</BilledQuantity>`),
		good:    fxExtCII,
	}, {
		rule:    "BR-FXEXT-BR-24",
		profile: ext,
		bad:     fxExt(`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>100.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>`, `<SpecifiedTradeSettlementLineMonetarySummation/>`),
		good:    fxExtCII,
	}, {
		rule:    "BR-FXEXT-BR-26",
		profile: ext,
		bad:     fxExt(`<SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>`, `<SpecifiedLineTradeAgreement/>`),
		good:    fxExtCII,
	}, {
		rule:    "BR-FXEXT-CO-04",
		profile: ext,
		bad:     fxExt(`<ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax><SpecifiedTradeSettlementLineMonetarySummation>`, `<ApplicableTradeTax><TypeCode>VAT</TypeCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax><SpecifiedTradeSettlementLineMonetarySummation>`),
		good:    fxExtCII,
	}, {
		// A document-level charge with neither a reason nor a reason code. The
		// zero amount keeps every summation intact, so only the reason moved.
		rule:    "BR-FXEXT-BR-38",
		profile: ext,
		bad:     fxExt(`<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>`, `<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode><SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>true</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount><CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax></SpecifiedTradeAllowanceCharge>`),
		good:    fxExt(`<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>`, `<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode><SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>true</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount><Reason>Packing</Reason><CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax></SpecifiedTradeAllowanceCharge>`),
	}, {
		rule:    "BR-FXEXT-BR-44",
		profile: ext,
		bad:     fxExt(`<SpecifiedLineTradeSettlement><ApplicableTradeTax>`, `<SpecifiedLineTradeSettlement><SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>true</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount></SpecifiedTradeAllowanceCharge><ApplicableTradeTax>`),
		good:    fxExt(`<SpecifiedLineTradeSettlement><ApplicableTradeTax>`, `<SpecifiedLineTradeSettlement><SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>true</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount><ReasonCode>ZZZ</ReasonCode></SpecifiedTradeAllowanceCharge><ApplicableTradeTax>`),
	}, {
		rule:    "BR-FXEXT-CO-10",
		profile: ext,
		bad:     fxVAT(`<LineTotalAmount>100.00</LineTotalAmount>`+"\n        <AllowanceTotalAmount>", `<LineTotalAmount>90.00</LineTotalAmount>`+"\n        <AllowanceTotalAmount>"),
		good:    fxVATCII,
	}, {
		// BT-107 present and wrong. This is the rule nothing checked before:
		// en16931_model.go skips CEN's BR-CO-11 at EXTENDED outright.
		rule:    "BR-FXEXT-CO-11",
		profile: ext,
		bad:     fxVAT(`<AllowanceTotalAmount>15.00</AllowanceTotalAmount>`, `<AllowanceTotalAmount>25.00</AllowanceTotalAmount>`),
		good:    fxVATCII,
	}, {
		// BT-108 counting the BG-21 charge and ignoring the logistics service
		// charge, which is the document CEN's BR-CO-12 accepts and FNFE's does
		// not.
		rule:    "BR-FXEXT-CO-12",
		profile: ext,
		bad:     fxVAT(`<ChargeTotalAmount>15.00</ChargeTotalAmount>`, `<ChargeTotalAmount>10.00</ChargeTotalAmount>`),
		good:    fxVATCII,
	}, {
		rule:    "BR-FXEXT-CO-13",
		profile: ext,
		bad:     fxVAT(`<TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>`, `<TaxBasisTotalAmount>90.00</TaxBasisTotalAmount>`),
		good:    fxVATCII,
	}, {
		rule:    "BR-FXEXT-CO-15",
		profile: ext,
		bad:     fxVAT(`<GrandTotalAmount>120.00</GrandTotalAmount>`, `<GrandTotalAmount>130.00</GrandTotalAmount>`),
		good:    fxVATCII,
	}, {
		// A third-party charge (BT-179) the amount due ignores: CEN's BR-CO-16
		// accepts it and FNFE's does not, which is the carve-out this rule is.
		rule:    "BR-FXEXT-CO-16",
		profile: ext,
		bad:     fxVAT(`<SpecifiedTradeSettlementHeaderMonetarySummation>`, `<SpecifiedFinancialAdjustment><ActualAmount>7.00</ActualAmount></SpecifiedFinancialAdjustment><SpecifiedTradeSettlementHeaderMonetarySummation>`),
		good:    fxVATCII,
	}, {
		rule:    "BR-FXEXT-G-08",
		profile: ProfileMinimum,
		bad:     strings.Replace(fxMinCII, `<BasisAmount>0.00</BasisAmount>`, `<BasisAmount>10.00</BasisAmount>`, 1),
		good:    fxMinCII,
	}}

	// The nine -08b summations, one per VAT category, and BR-FXEXT-S-09b.
	for _, spec := range facturXVATSummationRules {
		doc := fxVATCategory(spec.category)
		out = append(out, facturXRestatementFixture{
			rule:    spec.id,
			profile: ext,
			bad:     fxBreakTaxableAmount(doc),
			good:    doc,
		})
	}
	out = append(out, facturXRestatementFixture{
		rule:    "BR-FXEXT-S-09b",
		profile: ext,
		// BT-117 off by 5,00 against 20 % of 100,00, which clears the 0,04
		// tolerance these documents give it.
		bad:  fxVAT(`<CalculatedAmount>20.00</CalculatedAmount>`, `<CalculatedAmount>15.00</CalculatedAmount>`),
		good: fxVATCII,
	})
	return out
}

// TestFacturXRestatementSeveritiesAreTheArtefactsFlag is the severity half, in
// both directions and with no excuse list, over the documents the firing guard
// uses to make each rule report.
//
// It is a separate test from TestFacturXExtensionSeveritiesMatchTheArtefact for
// the same reason the two rule tables are separate: these 24 come from a
// different half of the artefact and are published by two different profiles, so
// the lookup has to be per profile rather than over a flattened map.
func TestFacturXRestatementSeveritiesAreTheArtefactsFlag(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	perProfile := map[Profile]map[string]Severity{}
	for _, p := range profiles {
		perProfile[p] = map[string]Severity{}
		for id, x := range fxNamed(fxDecode(t, dir, p)) {
			perProfile[p][id] = x.a.severity()
		}
	}
	ctx := context.Background()
	restatement := map[string]bool{}
	for _, rs := range facturXRestatementRules {
		restatement[rs.id] = true
	}
	checked := 0
	for _, tc := range facturXRestatementFixtures() {
		r, err := Validate(ctx, []byte(tc.bad), tc.profile)
		if err != nil {
			t.Fatalf("%s: %v", tc.rule, err)
		}
		for _, v := range r.Violations {
			if v.Source != SourceFacturX || !restatement[v.Rule] {
				continue
			}
			want, ok := perProfile[tc.profile][v.Rule]
			if !ok {
				t.Errorf("this package reported %s at profile %q and that Schematron does not publish it",
					v.Rule, string(tc.profile))
				continue
			}
			checked++
			if v.Severity != want {
				t.Errorf("%s was reported %s and FNFE flags it %s", v.Rule, v.Severity, want)
			}
		}
	}
	if checked < len(facturXRestatementRules) {
		t.Errorf("only %d restatement findings had their severity checked against the artefact, want at least %d; "+
			"a severity guard that reaches fewer rules than there are rules is not guarding them",
			checked, len(facturXRestatementRules))
	}
}

// TestFacturXRestatementsDuplicateTheirCENOriginal pins the duplicate-reporting
// decision facturx_restatements.go argues, rather than leaving it to be
// discovered by a caller counting findings.
//
// This package reports the union of the two rule sets: CEN's identifier under
// SourceEN16931 and FNFE's under SourceFacturX. A Factur-X processor prints only
// the second, because FACTUR-X_EXTENDED.sch does not carry the first at all. The
// test asserts the union is what comes back and that Source separates the two, so
// a caller who wants exactly FNFE's verdict has a filter that works.
func TestFacturXRestatementsDuplicateTheirCENOriginal(t *testing.T) {
	// A taxable amount that is wrong under both readings of the category-S
	// summation, which is the clearest case: CEN's BR-S-08 and FNFE's
	// BR-FXEXT-S08b are about the same defect.
	//
	// The document is the plain one-line fixture and not fxVATCII, and the reason
	// is itself worth pinning: en16931_vat.go gates CEN's BR-{fam}-08 on the
	// BG-20/21 entries accounting for BT-107 and BT-108, and fxVATCII's BT-108
	// includes a logistics service charge that is not a BG-21 entry — so on that
	// document CEN's rule is switched off and only FNFE's reports. That is one of
	// the gaps facturx_restatements.go names, and a duplicate-reporting test built
	// on it would have been testing the wrong thing.
	doc := strings.Replace(fxExtCII, `<BasisAmount>100.00</BasisAmount>`, `<BasisAmount>90.00</BasisAmount>`, 1)
	if doc == fxExtCII {
		t.Fatal("the mutation did not apply; the fixture changed")
	}
	r, err := Validate(context.Background(), []byte(doc), ProfileExtended)
	if err != nil {
		t.Fatalf("%v", err)
	}
	bySource := map[Source]map[string]bool{}
	for _, v := range r.Violations {
		if bySource[v.Source] == nil {
			bySource[v.Source] = map[string]bool{}
		}
		bySource[v.Source][v.Rule] = true
	}
	if !bySource[SourceFacturX]["BR-FXEXT-S08b"] {
		t.Errorf("the restatement did not report; the rest of this test says nothing without it")
	}
	if !bySource[SourceEN16931]["BR-S-08"] {
		t.Errorf("CEN's BR-S-08 did not report on a document that breaks it, and facturXCENOmissions records the "+
			"decision to keep evaluating CEN's original for every identifier EXTENDED drops; got %v",
			bySource[SourceEN16931])
	}
	// And filtering on Source gives a caller exactly one of the two.
	only := 0
	for _, v := range r.Violations {
		if v.Source == SourceFacturX && v.Rule == "BR-FXEXT-S08b" {
			only++
		}
	}
	if only != 1 {
		t.Errorf("BR-FXEXT-S08b was reported %d times for one breakdown, want 1", only)
	}
}

// TestFacturXRestatementContextsAreReachedInTheCorpus measures rather than
// asserts, the way its BR-FXEXT-* sibling does: how much of FNFE's own EXTENDED
// corpus goes past each of these 24 rule bodies. It is the number that says
// whether the FP=0 sweep over those documents means anything for a given rule.
func TestFacturXRestatementContextsAreReachedInTheCorpus(t *testing.T) {
	files := fxExamples(t)
	if len(files) == 0 {
		t.Skip("no Factur-X examples found; see `make facturx-examples`")
	}
	atLeast(t, "Factur-X example invoices", len(files), minFacturXExamples)

	seen, docs := ruleContexts{}, map[Profile]int{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		p, _, ok := fxDeclaredProfile(data)
		if !ok || (p != ProfileExtended && p != ProfileMinimum) {
			continue
		}
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil {
			t.Fatalf("%s: %v", f, perr)
		}
		docs[p]++
		validateFacturXRestatements(r, parsed, p, seen)
	}
	atLeast(t, "Factur-X EXTENDED examples", docs[ProfileExtended], minFacturXExtendedExamples)
	var reached, unreached []string
	for _, rs := range facturXRestatementRules {
		if seen[rs.id] > 0 {
			reached = append(reached, fmt.Sprintf("%s=%d", rs.id, seen[rs.id]))
		} else {
			unreached = append(unreached, rs.id)
		}
	}
	if len(reached) < minFacturXRestatementContexts {
		t.Errorf("only %d of the %d restatements were asked about a context node anywhere in FNFE's own examples, "+
			"want at least %d; a rule nothing in the corpus reaches is a rule the FP=0 sweep says nothing about",
			len(reached), len(facturXRestatementRules), minFacturXRestatementContexts)
	}
	t.Logf("Factur-X restatement contexts over %d EXTENDED and %d MINIMUM examples: reached %v; not reached %v",
		docs[ProfileExtended], docs[ProfileMinimum], reached, unreached)
}

// TestFacturXRestatementsBreakNoDocumentOnTheirOwn is the false-positive sweep,
// wider than the oracle and in the shape PR 58 established for the data model.
//
// Two populations. Every corpus document whose own BT-24 declares a Factur-X
// profile, validated at that profile — the same 32 the corpus oracle holds to
// FP=0 — must report no restatement finding at all. And every document in the
// corpus, forced to ProfileExtended regardless of what it declares, must not have
// a restatement as its *only* fatal finding: an XRechnung or CEN CII invoice
// asked whether it is a Factur-X EXTENDED invoice may well fail one of these
// summations, and that is a correct answer to a question nobody asks it, but a
// document these rules alone condemn would be one to look at.
func TestFacturXRestatementsBreakNoDocumentOnTheirOwn(t *testing.T) {
	ctx := context.Background()
	restatement := map[string]bool{}
	for _, rs := range facturXRestatementRules {
		restatement[rs.id] = true
	}
	// The 32 profile-declaring documents are all in testdata/facturx/examples,
	// which is vendored rather than fetchable, so that half of this sweep is
	// conditional on their presence exactly as the three other tests that read
	// them are. The forced-EXTENDED half runs against whatever corpus is there.
	haveExamples := len(fxExamples(t)) > 0
	files, declared, conformant, forced, only := 0, 0, 0, 0, 0
	seen := map[string]int{}
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Errorf("%s: %v", p, rerr)
			return nil
		}
		files++
		if prof, _, ok := fxDeclaredProfile(data); ok && haveExamples {
			declared++
			r, verr := Validate(ctx, data, prof)
			if verr == nil {
				if r.Conformant() {
					conformant++
				}
				for _, v := range r.Violations {
					if restatement[v.Rule] {
						t.Errorf("%s declares the %q profile and this package reports %s on it: %s",
							p, string(prof), v.Rule, v.Message)
					}
				}
			}
		}
		r, verr := Validate(ctx, data, ProfileExtended)
		if verr != nil {
			return nil
		}
		other, mine := 0, 0
		for _, v := range r.Fatal() {
			if restatement[v.Rule] {
				mine++
				seen[v.Rule]++
			} else {
				other++
			}
		}
		forced += mine
		if mine > 0 && other == 0 {
			only++
			t.Errorf("%s has no fatal finding at EXTENDED except the restatements this change adds; that is a "+
				"document the package called clean and now condemns on the strength of these rules alone", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if files == 0 {
		t.Skip("no corpus present")
	}
	if haveExamples {
		atLeast(t, "restatement sweep corpus", files, minCorpusDocuments)
		atLeast(t, "Factur-X profile-declaring corpus documents", declared, minFacturXProfiled)
		if conformant != declared {
			t.Errorf("%d of the %d profile-declaring corpus documents report Conformant(); every one of them should",
				conformant, declared)
		}
	}
	t.Logf("Factur-X restatements: 0 findings on %d profile-declaring documents at their own profile, all %d Conformant(); "+
		"%d findings from %d rules over %d documents forced to EXTENDED, %d of which the restatements condemn alone",
		declared, conformant, forced, len(seen), files, only)
}
