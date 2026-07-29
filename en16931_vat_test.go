package formalis

import (
	"context"
	"strings"
	"testing"
)

// allowChargeUBL is a conforming EN 16931 UBL invoice that carries both a
// document-level allowance (BG-20) and a document-level charge (BG-21) as
// itemizable entries, with the summation totals BT-107/BT-108 that account for
// them. It is the shape the BR-{fam}-08 sum's allowance/charge arm exists to
// check, and the shape the previous gate excluded outright.
//
//	line net (BT-131)                100.00   S 19%
//	allowance (BT-92)                 10.00   S 19%
//	charge (BT-99)                     5.00   S 19%
//	BT-106 sum of line net amounts   100.00
//	BT-107 sum of allowances          10.00
//	BT-108 sum of charges              5.00
//	BT-109 total without VAT          95.00 = 100 - 10 + 5
//	BT-116 VAT category taxable       95.00 = 100 - 10 + 5   <- the rule under test
//	BT-117 VAT category tax           18.05 = 95 x 19%
//	BT-110 total VAT                  18.05
//	BT-112 total with VAT            113.05
//	BT-115 amount due                113.05
const allowChargeUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2">
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
<AllowanceCharge><ChargeIndicator>false</ChargeIndicator>
  <AllowanceChargeReason>Volume discount</AllowanceChargeReason><Amount>10.00</Amount>
  <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></AllowanceCharge>
<AllowanceCharge><ChargeIndicator>true</ChargeIndicator>
  <AllowanceChargeReason>Freight</AllowanceChargeReason><Amount>5.00</Amount>
  <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></AllowanceCharge>
<TaxTotal><TaxAmount>18.05</TaxAmount>
  <TaxSubtotal><TaxableAmount>95.00</TaxableAmount><TaxAmount>18.05</TaxAmount>
    <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></TaxSubtotal>
</TaxTotal>
<LegalMonetaryTotal><LineExtensionAmount>100.00</LineExtensionAmount>
  <AllowanceTotalAmount>10.00</AllowanceTotalAmount><ChargeTotalAmount>5.00</ChargeTotalAmount>
  <TaxExclusiveAmount>95.00</TaxExclusiveAmount><TaxInclusiveAmount>113.05</TaxInclusiveAmount>
  <PayableAmount>113.05</PayableAmount></LegalMonetaryTotal>
<InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="C62">1</InvoicedQuantity>
  <LineExtensionAmount>100.00</LineExtensionAmount>
  <Item><Name>Widget</Name><ClassifiedTaxCategory><ID>S</ID><Percent>19</Percent></ClassifiedTaxCategory></Item>
  <Price><PriceAmount>100.00</PriceAmount></Price></InvoiceLine>
</Invoice>`

// TestVATTaxableSumCoversAllowancesAndCharges is the regression test for the
// BR-{fam}-08 sum's allowance/charge arm.
//
// The sum was gated on "the invoice has no document-level allowance or charge",
// which meant inv.allowCharges was empty at every call, the allowance/charge
// loop in vatSummands.total never ran, and the violation's own message —
// "the sum of matching line net amounts + charges - allowances" — described
// arithmetic the function provably could not perform. The gate now asks the
// narrower question it meant to ask: are the BG-20/21 entries present and do
// they account for BT-107/BT-108?
func TestVATTaxableSumCoversAllowancesAndCharges(t *testing.T) {
	// The conforming invoice is clean: 100 - 10 + 5 = 95.00 = BT-116.
	if v := Validate(context.Background(), []byte(allowChargeUBL), ProfileEN16931); len(v) != 0 {
		t.Fatalf("baseline allowance/charge invoice not clean: %d violation(s): %v", len(v), v)
	}

	// A breakdown that ignores one of the two entries, with BT-117, BT-110, BT-112
	// and BT-115 all carried along so BT-116 is the document's only defect and
	// BR-S-08 is the only rule with anything to say about it.
	//
	//	          BT-116   BT-117   BT-110   BT-112   BT-115
	//	allowance ignored  105.00    19.95    19.95   114.95   114.95
	//	charge ignored      90.00    17.10    17.10   112.10   112.10
	mutate := func(basis, tax, grand string) []byte {
		return []byte(strings.NewReplacer(
			"<TaxableAmount>95.00</TaxableAmount><TaxAmount>18.05</TaxAmount>",
			"<TaxableAmount>"+basis+"</TaxableAmount><TaxAmount>"+tax+"</TaxAmount>",
			"<TaxTotal><TaxAmount>18.05</TaxAmount>", "<TaxTotal><TaxAmount>"+tax+"</TaxAmount>",
			"<TaxInclusiveAmount>113.05</TaxInclusiveAmount>", "<TaxInclusiveAmount>"+grand+"</TaxInclusiveAmount>",
			"<PayableAmount>113.05</PayableAmount>", "<PayableAmount>"+grand+"</PayableAmount>",
		).Replace(allowChargeUBL))
	}

	for _, tc := range []struct{ name, basis, tax, grand string }{
		{"breakdown ignores the document allowance", "105.00", "19.95", "114.95"},
		{"breakdown ignores the document charge", "90.00", "17.10", "112.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := Validate(context.Background(), mutate(tc.basis, tc.tax, tc.grand), ProfileEN16931)
			if len(v) != 1 || v[0].Rule != "BR-S-08" {
				t.Fatalf("want exactly one violation, BR-S-08; got %v", v)
			}
			// The message now describes arithmetic the function actually performed:
			// 100 - 10 + 5. Before, the allowance/charge loop was unreachable.
			if !strings.Contains(v[0].Message, "(95.00)") {
				t.Errorf("BR-S-08 should report the computed sum 95.00 (= 100 - 10 + 5); got %q", v[0].Message)
			}
		})
	}
}

// TestVATTaxableSumSkipsUnitemizedAllowances pins the reason the gate exists.
//
// Some producers carry an allowance or charge only in the summation totals
// BT-107/BT-108, with no BG-20/21 entry to attribute to a VAT category. Summing
// the entries then understates the taxable base, so the sum must not run — the
// invoice is not wrong, this package simply cannot compute the check.
func TestVATTaxableSumSkipsUnitemizedAllowances(t *testing.T) {
	// The same invoice with the two BG-20/21 entries removed: BT-107 and BT-108
	// still claim 10.00 and 5.00, and BT-116 is still 95.00, but nothing
	// attributes those amounts to the standard-rated category.
	unitemized := allowChargeUBL
	for _, entry := range []string{
		`<AllowanceCharge><ChargeIndicator>false</ChargeIndicator>
  <AllowanceChargeReason>Volume discount</AllowanceChargeReason><Amount>10.00</Amount>
  <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></AllowanceCharge>`,
		`<AllowanceCharge><ChargeIndicator>true</ChargeIndicator>
  <AllowanceChargeReason>Freight</AllowanceChargeReason><Amount>5.00</Amount>
  <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></AllowanceCharge>`,
	} {
		next := strings.Replace(unitemized, entry, "", 1)
		if next == unitemized {
			t.Fatalf("allowance/charge entry not found in the fixture:\n%s", entry)
		}
		unitemized = next
	}
	// BR-CO-11/BR-CO-12 report the unaccounted totals; BR-S-08 must not, because
	// its operands are not all present.
	v := Validate(context.Background(), []byte(unitemized), ProfileEN16931)
	if hasFacturXRule(v, "BR-S-08") {
		t.Errorf("BR-S-08 must not fire when BT-107/BT-108 are not itemized as BG-20/21 entries; got %v", v)
	}
	if !hasFacturXRule(v, "BR-CO-11") {
		t.Errorf("BR-CO-11 should report the unaccounted allowance total; got %v", v)
	}
}

// TestDocAllowanceChargesItemized pins the gate itself, including the shape it
// must keep letting through unchanged: an invoice with neither entries nor
// totals, which is what the overwhelming majority of the corpus looks like.
func TestDocAllowanceChargesItemized(t *testing.T) {
	ac := func(amount string, charge bool) docAllowanceCharge {
		return docAllowanceCharge{amount: amount, isCharge: charge}
	}
	for _, tc := range []struct {
		name string
		inv  en16931Invoice
		want bool
	}{
		{
			name: "no entries and no totals",
			want: true,
		},
		{
			name: "no entries, an allowance total claimed",
			inv:  en16931Invoice{totals: monetaryTotals{allowanceTotal: "10.00"}},
			want: false,
		},
		{
			name: "no entries, a charge total claimed",
			inv:  en16931Invoice{totals: monetaryTotals{chargeTotal: "5.00"}},
			want: false,
		},
		{
			name: "entries that account for both totals",
			inv: en16931Invoice{
				allowCharges: []docAllowanceCharge{ac("10.00", false), ac("5.00", true)},
				totals:       monetaryTotals{allowanceTotal: "10.00", chargeTotal: "5.00"},
			},
			want: true,
		},
		{
			name: "entries summing to more than the total",
			inv: en16931Invoice{
				allowCharges: []docAllowanceCharge{ac("10.00", false), ac("2.00", false)},
				totals:       monetaryTotals{allowanceTotal: "10.00"},
			},
			want: false,
		},
		{
			name: "an entry with no amount is accounted for by nothing",
			inv: en16931Invoice{
				allowCharges: []docAllowanceCharge{ac("", false)},
				totals:       monetaryTotals{allowanceTotal: "0.00"},
			},
			want: false,
		},
		{
			name: "an entry and an absent total, which reads as zero",
			inv:  en16931Invoice{allowCharges: []docAllowanceCharge{ac("10.00", false)}},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := docAllowanceChargesItemized(&tc.inv); got != tc.want {
				t.Errorf("docAllowanceChargesItemized = %v, want %v", got, tc.want)
			}
		})
	}
}
