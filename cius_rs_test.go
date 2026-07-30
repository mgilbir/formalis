package formalis

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rsRuleViolations scopes a report to the Serbian rule set.
//
// It reads Source rather than an identifier prefix, and the change is C39's lesson
// rather than a tidy-up: this rule set publishes three families (RSR-*, RSK-*,
// RSE-*) and the filter used to admit one of them, so the twenty-eight VAT-category
// findings and the three extension findings would have been invisible to the
// false-positive oracle on the day they were first emitted. A prefix scope only
// watches the identifiers its author anticipated.
func rsRuleViolations(vs []Violation) []string {
	var r []string
	for _, v := range vs {
		if v.Source == SourceSRBDT {
			r = append(r, v.Rule)
		}
	}
	return r
}

// TestSRBDTCorpus is the FP=0 oracle: every official SRBDT sample instance
// (phax/phive-rules, all good cases) must satisfy the implemented SRBDT rules.
// Skips when the corpus is absent (make cius-oracles).
func TestSRBDTCorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-rs/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("SRBDT corpus not present (make cius-oracles)")
	}
	atLeast(t, "SRBDT corpus", len(files), minSRBDTInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if rs := rsRuleViolations(findings(t, context.Background(), ValidateSRBDT, data)); len(rs) != 0 {
			t.Errorf("%s: expected 0 SRBDT violations on a conformant sample, got %v", filepath.Base(f), rs)
		}
	}
}

// minimalSRBDT is a small SRBDT-conformant invoice mirroring the Serbian party
// structure (VAT + tax-status PartyTaxScheme, 9948 endpoints, RS PIB codes, a payee
// and a tax representative), with distinct seller/buyer/payee values for isolated
// mutation.
//
// The three VAT category groups carry the same code S at three deliberately
// different precisions — 20.0 on the document charge, 20 on the breakdown, 20.00 on
// the line — so that a mutation naming one of RSR-34, RSR-35 and RSR-36 reaches the
// group that rule is bound to and not whichever comes first in the document.
const minimalSRBDT = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022</cbc:CustomizationID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RSD</cbc:DocumentCurrencyCode>
<cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>
<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>PRIOR-1</cbc:ID><cbc:IssueDate>2023-12-01</cbc:IssueDate></cac:InvoiceDocumentReference></cac:BillingReference>
<cac:AccountingSupplierParty><cac:Party>
  <cbc:EndpointID schemeID="9948">100000005</cbc:EndpointID>
  <cac:PostalAddress><cbc:CityName>SellerCity</cbc:CityName><cac:Country><cbc:IdentificationCode>RS</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>RS100000005</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyTaxScheme><cbc:CompanyID>Obveznik PDV-a</cbc:CompanyID><cac:TaxScheme><cbc:ID>RS-VAT-STATUS</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller doo</cbc:RegistrationName><cbc:CompanyID>10000000</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cbc:EndpointID schemeID="9948">222222222</cbc:EndpointID>
  <cac:PostalAddress><cbc:CityName>BuyerCity</cbc:CityName><cbc:PostalZone>11000</cbc:PostalZone><cac:Country><cbc:IdentificationCode>RS</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>RS222222222</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer doo</cbc:RegistrationName><cbc:CompanyID>20000000</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:PayeeParty><cac:PartyName><cbc:Name>Payee doo</cbc:Name></cac:PartyName><cac:PartyLegalEntity><cbc:CompanyID>30000000</cbc:CompanyID></cac:PartyLegalEntity></cac:PayeeParty>
<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Rep doo</cbc:Name></cac:PartyName><cac:PartyTaxScheme><cbc:CompanyID>RS333333333</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme></cac:TaxRepresentativeParty>
<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount>0.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20.0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:TaxTotal><cbc:TaxAmount>20.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>20.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>120.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>120.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Roba</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20.00</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// srbdtZeroRateOrphanLine uses the zero-rate category R on a line and carries no
// VAT breakdown for it, which is the half of RSK-X-01 a conforming document cannot
// reach. Its non-zero rate makes RSK-X-05 report too.
const srbdtZeroRateOrphanLine = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022</cbc:CustomizationID>
<cbc:ID>INV-R1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RSD</cbc:DocumentCurrencyCode>
<cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>
<cac:TaxTotal><cbc:TaxAmount>0.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>0.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>100.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>100.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Roba</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>R</cbc:ID><cbc:Percent>10</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// srbdtZeroRateBadBreakdown carries exactly one VAT breakdown for category R — so
// RSK-X-01 is satisfied — and breaks everything the pattern says about it: a
// non-zero rate on the allowance, on the charge and on the line, a taxable amount
// that does not balance, a non-zero tax amount, and no exemption reason at all.
const srbdtZeroRateBadBreakdown = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022</cbc:CustomizationID>
<cbc:ID>INV-R2</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RSD</cbc:DocumentCurrencyCode>
<cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>
<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:Amount>5.00</cbc:Amount><cac:TaxCategory><cbc:ID>R</cbc:ID><cbc:Percent>10.0</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount>7.00</cbc:Amount><cac:TaxCategory><cbc:ID>R</cbc:ID><cbc:Percent>10.5</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:TaxTotal><cbc:TaxAmount>3.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>999.99</cbc:TaxableAmount><cbc:TaxAmount>3.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>R</cbc:ID><cbc:Percent>10</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>103.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>103.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Roba</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>R</cbc:ID><cbc:Percent>10.00</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// srbdtExtensionDoc carries the srbdtext extension group, which nothing in the
// vendored corpus does. It is the only fixture in this repository that reaches
// RSR-02, RSE-01, RSE-02 or RSE-03 at all.
//
// RSR-02 reports for it whatever the extension contains, because the Ministry binds
// that rule to normalize-space() of the group element rather than of a
// specification identifier — see the note in cius_rs.go. The two category codes are
// outside the published set and the reduced total does not equal the invoice total
// less the prepaid amount, so RSE-01, RSE-02 and RSE-03 all report as well.
const srbdtExtensionDoc = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2" xmlns:sbt="http://mfin.gov.rs/srbdt/srbdtext">
<ext:UBLExtensions><ext:UBLExtension><ext:ExtensionContent>
  <sbt:SrbDtExt>
    <sbt:InvoicedPrepaymentAmmount><cac:TaxTotal><cac:TaxSubtotal><cac:TaxCategory><cbc:ID>XX</cbc:ID></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal></sbt:InvoicedPrepaymentAmmount>
    <sbt:ReducedTotals><cac:TaxTotal><cac:TaxSubtotal><cac:TaxCategory><cbc:ID>YY</cbc:ID></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal><cac:LegalMonetaryTotal><cbc:TaxInclusiveAmount>77.00</cbc:TaxInclusiveAmount></cac:LegalMonetaryTotal></sbt:ReducedTotals>
  </sbt:SrbDtExt>
</ext:ExtensionContent></ext:UBLExtension></ext:UBLExtensions>
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022#conformant#urn:mfin.gov.rs:srbdtext:2022</cbc:CustomizationID>
<cbc:ID>INV-E1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>386</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>RSD</cbc:DocumentCurrencyCode>
<cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>
<cac:TaxTotal><cbc:TaxAmount>20.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>20.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>120.00</cbc:TaxInclusiveAmount><cbc:PrepaidAmount>10.00</cbc:PrepaidAmount><cbc:PayableAmount>110.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Roba</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>20.00</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

var srbdtMutations = []ciusMutation{
	{"specification identifier not SRBDT (01)", "urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022", "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0", "RSR-01"},
	{"bad type code (03)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>999</cbc:InvoiceTypeCode>", "RSR-03"},
	{"has tax point date (04)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:TaxPointDate>2024-01-15</cbc:TaxPointDate>", "RSR-04"},
	{"no tax point date code (05)", "<cac:InvoicePeriod><cbc:DescriptionCode>3</cbc:DescriptionCode></cac:InvoicePeriod>", "", "RSR-05"},
	{"tax point date code outside the set (06)", "<cbc:DescriptionCode>3</cbc:DescriptionCode>", "<cbc:DescriptionCode>7</cbc:DescriptionCode>", "RSR-06"},
	{"preceding invoice without an issue date (07)", "<cbc:IssueDate>2023-12-01</cbc:IssueDate>", "", "RSR-07"},
	{"bad seller PIB (11)", "<cbc:CompanyID>RS100000005</cbc:CompanyID>", "<cbc:CompanyID>100000005</cbc:CompanyID>", "RSR-11"},
	{"seller non-VAT scheme is not RS-VAT-STATUS (12)", "<cbc:ID>RS-VAT-STATUS</cbc:ID>", "<cbc:ID>OTHER</cbc:ID>", "RSR-12"},
	{"bad seller endpoint scheme (14)", "schemeID=\"9948\">100000005", "schemeID=\"0088\">100000005", "RSR-14"},
	{"buyer registration with a scheme identifier (18)", "<cbc:CompanyID>20000000</cbc:CompanyID>", "<cbc:CompanyID schemeID=\"0088\">20000000</cbc:CompanyID>", "RSR-18"},
	{"buyer registration of the wrong length (19)", "<cbc:CompanyID>20000000</cbc:CompanyID>", "<cbc:CompanyID>2000</cbc:CompanyID>", "RSR-19"},
	{"bad buyer PIB (21)", "<cbc:CompanyID>RS222222222</cbc:CompanyID>", "<cbc:CompanyID>222222222</cbc:CompanyID>", "RSR-21"},
	{"bad buyer endpoint scheme (23)", "schemeID=\"9948\">222222222", "schemeID=\"0088\">222222222", "RSR-23"},
	{"payee without a registration identifier (27)", "<cbc:CompanyID>30000000</cbc:CompanyID>", "", "RSR-27"},
	{"payee registration of the wrong length (28)", "<cbc:CompanyID>30000000</cbc:CompanyID>", "<cbc:CompanyID>3000</cbc:CompanyID>", "RSR-28"},
	{"tax representative without a PIB (29)", "<cac:PartyTaxScheme><cbc:CompanyID>RS333333333</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>", "", "RSR-29"},
	{"bad tax representative PIB (30)", "<cbc:CompanyID>RS333333333</cbc:CompanyID>", "<cbc:CompanyID>333333333</cbc:CompanyID>", "RSR-30"},
	{"bad allowance/charge VAT category (34)", "<cbc:ID>S</cbc:ID><cbc:Percent>20.0</cbc:Percent>", "<cbc:ID>XX</cbc:ID><cbc:Percent>20.0</cbc:Percent>", "RSR-34"},
	{"bad VAT breakdown category (35)", "<cbc:ID>S</cbc:ID><cbc:Percent>20</cbc:Percent>", "<cbc:ID>XX</cbc:ID><cbc:Percent>20</cbc:Percent>", "RSR-35"},
	{"bad line VAT category (36)", "<cbc:ID>S</cbc:ID><cbc:Percent>20.00</cbc:Percent>", "<cbc:ID>XX</cbc:ID><cbc:Percent>20.00</cbc:Percent>", "RSR-36"},
}

// srbdtExtras are the rules no substitution on the baseline can reach: the four
// bound to the srbdtext extension group, and the seven of the abstract pdvcat
// pattern, which need a document that uses one of the four Serbian zero-rate VAT
// categories.
var srbdtExtras = []ciusDoc{
	{"srbdtext specification identifier (02)", srbdtExtensionDoc, "RSR-02"},
	{"prepayment VAT category code (RSE-01)", srbdtExtensionDoc, "RSE-01"},
	{"reduced-amount VAT category code (RSE-02)", srbdtExtensionDoc, "RSE-02"},
	{"reduced total does not balance (RSE-03)", srbdtExtensionDoc, "RSE-03"},
	{"zero-rate category with no breakdown (RSK-X-01)", srbdtZeroRateOrphanLine, "RSK-X-01"},
	{"zero-rate line with a non-zero rate (RSK-X-05)", srbdtZeroRateOrphanLine, "RSK-X-05"},
	{"zero-rate allowance with a non-zero rate (RSK-X-06)", srbdtZeroRateBadBreakdown, "RSK-X-06"},
	{"zero-rate charge with a non-zero rate (RSK-X-07)", srbdtZeroRateBadBreakdown, "RSK-X-07"},
	{"zero-rate breakdown that does not balance (RSK-X-08)", srbdtZeroRateBadBreakdown, "RSK-X-08"},
	{"zero-rate breakdown with a non-zero tax amount (RSK-X-09)", srbdtZeroRateBadBreakdown, "RSK-X-09"},
	{"zero-rate breakdown with no exemption reason (RSK-X-10)", srbdtZeroRateBadBreakdown, "RSK-X-10"},
}

func TestSRBDTMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[3])
}

// srbdtUnevaluable is the list Coverage(SourceSRBDT) records, in one place so the
// test below and the coverage table cannot drift apart by hand.
var srbdtUnevaluable = []string{
	"RSR-08", "RSR-09", "RSR-10", "RSR-13", "RSR-15", "RSR-16", "RSR-17",
	"RSR-20", "RSR-22", "RSR-24", "RSR-25", "RSR-26", "RSR-31", "RSR-32", "RSR-33",
}

// TestSRBDTUnevaluableRulesAreDerivedFromTheArtefact is the evidence for the
// largest single claim this PR makes: fifteen of the Ministry of Finance's
// thirty-six RSR identifiers are rules no Schematron processor reaches, so a
// validator that reports one is reporting a finding the Ministry's own validator
// cannot produce.
//
// It re-derives the whole list from EN16931-UBL-srbdt.sch rather than believing the
// prose, in both directions: every identifier the coverage table calls unevaluable
// must be shadowed in the file, and every identifier the file shadows must be in the
// table. The second direction is the one that matters on the day the Ministry splits
// its pattern — a fixed upstream turns these back into ordinary coverage gaps, and
// this is what would say so.
//
// Eight of the fifteen were being emitted by this package before the file was read
// this way (RSR-09, 10, 13, 16, 17, 20, 22, 25), which is why the claim is worth
// this much machinery.
func TestSRBDTUnevaluableRulesAreDerivedFromTheArtefact(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "cius-rs", "schematron", "*", "EN16931-UBL-srbdt.sch"))
	if len(files) == 0 {
		t.Skip("SRBDT Schematron not present; run `make cius-schematron`")
	}
	shadowed := schShadowed(t, files)
	if len(shadowed) == 0 {
		t.Fatal("the rule-order decoder found no shadowed rule in EN16931-UBL-srbdt.sch, so it is reading nothing")
	}
	claimed := map[string]bool{}
	for _, id := range srbdtUnevaluable {
		claimed[id] = true
		by, ok := shadowed[id]
		if !ok {
			t.Errorf("Coverage(SourceSRBDT) records %s as unevaluable, and every rule carrying it in "+
				"EN16931-UBL-srbdt.sch is reachable. Either the Ministry split its pattern — in which case this "+
				"is a coverage gap again and belongs in the code — or the claim was never true", id)
			continue
		}
		t.Logf("SRBDT %s: unreachable, context %q is claimed by an earlier rule of the same pattern", id, by)
	}
	for id, by := range shadowed {
		if !claimed[id] {
			t.Errorf("EN16931-UBL-srbdt.sch shadows %s (context %q) and neither Coverage(SourceSRBDT) nor this "+
				"test names it. An identifier no processor reaches has to be recorded, or a reader cannot tell "+
				"it from one this package forgot", id, by)
		}
	}
	// And the halves have to add up: what this package evaluates plus what it
	// records as unreachable is the whole published set.
	published := ciusPublished(t)
	if published == nil {
		return
	}
	for id := range published[SourceSRBDT]["UBL"] {
		_, evaluated := ciusEvaluated[SourceSRBDT][id]
		if !evaluated && !claimed[id] {
			t.Errorf("the Ministry publishes %s and this package neither evaluates it nor records it as "+
				"unreachable", id)
		}
	}
	t.Logf("SRBDT: %d published identifiers, %d evaluated, %d unreachable by ISO Schematron rule order",
		len(published[SourceSRBDT]["UBL"]), len(ciusEvaluated[SourceSRBDT]), len(srbdtUnevaluable))
}

// TestSRBDTPDVCategoriesComeFromTheArtefact reads the four instantiations of the
// abstract pdvcat pattern out of the validation schema's siblings, so rsPDVCategories
// is a quotation rather than a memory.
//
// It also pins the identifier oddity the file comment describes: each instantiating
// pattern declares a <param> per identifier mapping RSK-X-NN to RSK-<CAT>-NN, and
// ISO Schematron substitutes $NAME occurrences — of which the assertion's id
// attribute, being the bare string "RSK-X-01", contains none. So the resolved schema
// reports the id RSK-X-01 with a message reading [RSK-N-01], and this package does
// the same.
func TestSRBDTPDVCategoriesComeFromTheArtefact(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "cius-rs", "schematron", "*", "EN16931-UBL-srbdt-pdvcat-*.sch"))
	if len(files) == 0 {
		t.Skip("SRBDT Schematron not present; run `make cius-schematron`")
	}
	codes := map[string]bool{}
	aliases := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(string(data)))
		isA := false
		for {
			tok, derr := dec.Token()
			if derr != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			at := func(n string) string {
				for _, a := range se.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch se.Name.Local {
			case "pattern":
				isA = at("is-a") == "pdvcat"
			case "param":
				if !isA {
					continue
				}
				if at("name") == "PDVCATCODE" {
					codes[at("value")] = true
					continue
				}
				// An identifier alias: the param's name is the abstract identifier and
				// its value the concrete one. It rewrites the message text and not the
				// id attribute, which is the whole point.
				if strings.HasPrefix(at("name"), "RSK-X-") {
					aliases++
				}
			}
		}
	}
	for _, want := range rsPDVCategories {
		if !codes[want] {
			t.Errorf("this package instantiates the pdvcat pattern for VAT category %q and no vendored "+
				"instantiation passes it as $PDVCATCODE", want)
		}
		delete(codes, want)
	}
	for extra := range codes {
		t.Errorf("the Ministry instantiates the pdvcat pattern for VAT category %q and this package does not; "+
			"seven assertions go unevaluated for it", extra)
	}
	if aliases != 7*len(rsPDVCategories) {
		t.Errorf("the four instantiations declare %d RSK-X-* identifier aliases, want %d (seven per category). "+
			"A change here changes which identifier a Serbian validator prints", aliases, 7*len(rsPDVCategories))
	}
	t.Logf("SRBDT: the abstract pdvcat pattern is instantiated for %v, %d identifier aliases in all; the id "+
		"attribute is not parameterised, so the published identifiers stay RSK-X-01 and RSK-X-05..10",
		rsPDVCategories, aliases)
}

// TestSRBDTRuleContextsAreReachable is requirement two of this rule set's oracle.
//
// It carries an exception list, which UBL.BE's counterpart does not, and the reason
// is a fact about the corpus rather than about the rules: not one of the 1,690
// documents carries an sbt:SrbDtExt, so the four rules bound to the srbdtext
// extension have no context node anywhere. The test asserts that too, so the excuse
// cannot outlive its reason — and srbdtExtensionDoc is what gives all four a firing
// verdict in the meantime.
func TestSRBDTRuleContextsAreReachable(t *testing.T) {
	seen, files := ciusContextSweep(t, func(p *parsed, seen ruleContexts) {
		validateSRBDTRules(p.inv, p.root, seen)
	})
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "SRBDT context sweep corpus", files, minCorpusDocuments)

	const noExtension = "no document in the corpus carries an sbt:SrbDtExt extension group"
	const noZeroRateAC = "no document in the corpus carries an allowance or charge whose VAT category is one of " +
		"the four Serbian zero-rate codes under a VAT tax scheme"
	reportUnreached(t, "SRBDT", seen, keysOfSeverityMap(ciusEvaluated[SourceSRBDT]), map[string]string{
		"RSR-02":   noExtension,
		"RSE-01":   noExtension,
		"RSE-02":   noExtension,
		"RSE-03":   noExtension,
		"RSK-X-06": noZeroRateAC,
		"RSK-X-07": noZeroRateAC,
	})

	// Both excuses, verified rather than asserted. An exception list nobody
	// re-derives is the shape C34's phantoms had.
	extensions, zeroRateAC := 0, 0
	_, _ = ciusContextSweep(t, func(p *parsed, _ ruleContexts) {
		extensions += len(p.root.findAll("SrbDtExt"))
		for _, ac := range p.root.findAll("AllowanceCharge") {
			for _, tc := range ac.all("TaxCategory") {
				for _, code := range rsPDVCategories {
					if isVATScheme(tc) && normSpace(tc.str("ID")) == code {
						zeroRateAC++
					}
				}
			}
		}
	})
	if extensions != 0 {
		t.Errorf("the corpus now carries %d sbt:SrbDtExt groups, so the four extension rules are reachable and "+
			"their exception in this test is stale", extensions)
	}
	if zeroRateAC != 0 {
		t.Errorf("the corpus now carries %d zero-rate allowance or charge VAT categories, so RSK-X-06 and "+
			"RSK-X-07 are reachable and their exception in this test is stale", zeroRateAC)
	}
}
