package formalis

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// minimalPeppolUBL is a small but complete, conforming Peppol BIS Billing 3.0
// (UBL) invoice carrying the terms Peppol requires on top of EN 16931.
//
// It is a German *domestic* invoice, and that is deliberate: OpenPEPPOL's
// german-rules pattern applies to a document whose seller and buyer postal
// addresses are both DE, so this fixture exercises all thirty DE-R-* rules in the
// silent direction on every test that uses it. Meeting them is what the seller
// contact group, the payment means and the credit-transfer account are here for —
// DE-R-001/002/005/006/007/023-1 each require one of them, and the baseline
// asserting no findings is what says they are satisfiable together.
const minimalPeppolUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0</cbc:CustomizationID>
<cbc:ProfileID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</cbc:ProfileID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cbc:BuyerReference>abc123</cbc:BuyerReference>
<cac:AccountingSupplierParty><cac:Party>
  <cbc:EndpointID schemeID="0088">7300010000001</cbc:EndpointID>
  <cac:PostalAddress><cbc:CityName>Berlin</cbc:CityName><cbc:PostalZone>10115</cbc:PostalZone>
    <cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>DE123456789</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller Ltd</cbc:RegistrationName></cac:PartyLegalEntity>
  <cac:Contact><cbc:Name>Sales</cbc:Name><cbc:Telephone>+49 30 123456</cbc:Telephone>
    <cbc:ElectronicMail>sales@seller.example</cbc:ElectronicMail></cac:Contact>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cbc:EndpointID schemeID="0088">7300010000018</cbc:EndpointID>
  <cac:PostalAddress><cbc:CityName>Bonn</cbc:CityName><cbc:PostalZone>53113</cbc:PostalZone>
    <cac:Country><cbc:IdentificationCode>DE</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer Ltd</cbc:RegistrationName></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>
  <cac:PayeeFinancialAccount><cbc:ID>DE02120300000000202051</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>
<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
  <cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
    <cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
  <cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount currencyID="EUR">119.00</cbc:TaxInclusiveAmount>
  <cbc:PayableAmount currencyID="EUR">119.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>
  <cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
  <cac:Item><cbc:Name>Widget</cbc:Name>
    <cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>19</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
  <cac:Price><cbc:PriceAmount currencyID="EUR">100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// minimalPeppolCII is minimalPeppolUBL's counterpart in the other binding.
//
// The two Peppol binding files are not translations of each other — 15 identifiers
// are UBL-only and one is CII-only, and several more test a different thing in each
// — so a suite with one fixture would leave a third of the rule set unexercised and
// could not tell a rule bound to the wrong binding from one bound to the right
// one. It also carries what the CII binding is stricter about: a @format on every
// udt:DateTimeString (F001) and a @currencyID on the VAT total (R053, CL007).
const minimalPeppolCII = `<CrossIndustryInvoice>
  <ExchangedDocumentContext>
    <BusinessProcessSpecifiedDocumentContextParameter><ID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</ID></BusinessProcessSpecifiedDocumentContextParameter>
    <GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0</ID></GuidelineSpecifiedDocumentContextParameter>
  </ExchangedDocumentContext>
  <ExchangedDocument><ID>INV-1</ID><TypeCode>380</TypeCode><IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime></ExchangedDocument>
  <SupplyChainTradeTransaction>
    <IncludedSupplyChainTradeLineItem>
      <AssociatedDocumentLineDocument><LineID>1</LineID></AssociatedDocumentLineDocument>
      <SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>
      <SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>
      <SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>
      <SpecifiedLineTradeSettlement><ApplicableTradeTax><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax><SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>100.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation></SpecifiedLineTradeSettlement>
    </IncludedSupplyChainTradeLineItem>
    <ApplicableHeaderTradeAgreement>
      <BuyerReference>abc123</BuyerReference>
      <SellerTradeParty><Name>Seller Co</Name>
        <URIUniversalCommunication><URIID schemeID="0088">7300010000001</URIID></URIUniversalCommunication>
        <PostalTradeAddress><PostcodeCode>10115</PostcodeCode><CityName>Berlin</CityName><CountryID>DE</CountryID></PostalTradeAddress>
        <SpecifiedTaxRegistration><ID schemeID="VA">DE123456789</ID></SpecifiedTaxRegistration></SellerTradeParty>
      <BuyerTradeParty><Name>Buyer Co</Name>
        <URIUniversalCommunication><URIID schemeID="0088">7300010000018</URIID></URIUniversalCommunication>
        <PostalTradeAddress><PostcodeCode>53113</PostcodeCode><CityName>Bonn</CityName><CountryID>DE</CountryID></PostalTradeAddress></BuyerTradeParty>
    </ApplicableHeaderTradeAgreement>
    <ApplicableHeaderTradeDelivery>
      <ActualDeliverySupplyChainEvent><OccurrenceDateTime><DateTimeString format="102">20240101</DateTimeString></OccurrenceDateTime></ActualDeliverySupplyChainEvent>
    </ApplicableHeaderTradeDelivery>
    <ApplicableHeaderTradeSettlement>
      <InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>
      <SpecifiedTradeSettlementPaymentMeans><TypeCode>58</TypeCode>
        <PayeePartyCreditorFinancialAccount><IBANID>DE75512108001245126199</IBANID></PayeePartyCreditorFinancialAccount></SpecifiedTradeSettlementPaymentMeans>
      <ApplicableTradeTax><CalculatedAmount>20.00</CalculatedAmount><BasisAmount>100.00</BasisAmount><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>
      <SpecifiedTradeSettlementHeaderMonetarySummation>
        <LineTotalAmount>100.00</LineTotalAmount>
        <TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>
        <TaxTotalAmount currencyID="EUR">20.00</TaxTotalAmount>
        <GrandTotalAmount>120.00</GrandTotalAmount>
        <DuePayableAmount>120.00</DuePayableAmount>
      </SpecifiedTradeSettlementHeaderMonetarySummation>
    </ApplicableHeaderTradeSettlement>
  </SupplyChainTradeTransaction>
</CrossIndustryInvoice>`

func TestValidatePeppolBaseline(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{"UBL", minimalPeppolUBL},
		{"CII", minimalPeppolCII},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v := findings(t, context.Background(), ValidatePeppol, []byte(tc.doc)); len(v) != 0 {
				t.Fatalf("baseline Peppol not clean: %d violations (first %s: %s)", len(v), v[0].Rule, v[0].Message)
			}
		})
	}
}

func TestValidatePeppolRules(t *testing.T) {
	cases := []struct{ name, from, to, rule string }{
		{"no profile id (R001)", "<cbc:ProfileID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</cbc:ProfileID>", "", "PEPPOL-EN16931-R001"},
		{"bad profile id (R007)", "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0", "urn:fdc:peppol.eu:2017:poacc:billing:bad", "PEPPOL-EN16931-R007"},
		{"wrong spec id (R004)", "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0", "urn:cen.eu:en16931:2017", "PEPPOL-EN16931-R004"},
		{"no buyer ref or order (R003)", "<cbc:BuyerReference>abc123</cbc:BuyerReference>", "", "PEPPOL-EN16931-R003"},
		{"no seller endpoint (R020)", `<cbc:EndpointID schemeID="0088">7300010000001</cbc:EndpointID>`, "", "PEPPOL-EN16931-R020"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalPeppolUBL, tc.from, tc.to, 1)
			if broken == minimalPeppolUBL {
				t.Fatalf("mutation %q not found", tc.from)
			}
			v := findings(t, context.Background(), ValidatePeppol, []byte(broken))
			found := false
			for _, x := range v {
				if x.Rule == tc.rule {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s to fire; got %v", tc.rule, v)
			}
		})
	}
}

// TestPeppolLinePeriodOrdersCalendarDays is the regression test for
// PEPPOL-EN16931-R110/R111, which contain the Invoice line period (BG-26) within
// the Invoicing period (BG-14).
//
// A legal xs:date may carry a timezone offset, and the old digit-stripping
// normalisation made "2024-02-01+02:00" — twelve digits — compare less than
// "2024-02-01" — eight — so a line period starting on the very same calendar day
// as the invoicing period was reported as falling outside it.
func TestPeppolLinePeriodOrdersCalendarDays(t *testing.T) {
	build := func(docStart, docEnd, lineStart, lineEnd string) []byte {
		doc := "<cac:InvoicePeriod><cbc:StartDate>" + docStart + "</cbc:StartDate>" +
			"<cbc:EndDate>" + docEnd + "</cbc:EndDate></cac:InvoicePeriod>"
		line := "<cac:InvoicePeriod><cbc:StartDate>" + lineStart + "</cbc:StartDate>" +
			"<cbc:EndDate>" + lineEnd + "</cbc:EndDate></cac:InvoicePeriod>"
		x := strings.Replace(minimalPeppolUBL,
			"<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>",
			"<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>"+doc, 1)
		x = strings.Replace(x, "<cac:Item>", line+"<cac:Item>", 1)
		return []byte(x)
	}
	for _, tc := range []struct {
		name               string
		docStart, docEnd   string
		lineStart, lineEnd string
		wantR110, wantR111 bool
	}{
		{
			name:     "line period on the same calendar days, offset on the document period",
			docStart: "2024-02-01+02:00", docEnd: "2024-02-29+02:00",
			lineStart: "2024-02-01", lineEnd: "2024-02-29",
		},
		{
			name:     "line period on the same calendar days, offset on the line period",
			docStart: "2024-02-01", docEnd: "2024-02-29",
			lineStart: "2024-02-01-05:00", lineEnd: "2024-02-29Z",
		},
		{
			name:     "line period genuinely starts before the invoicing period",
			docStart: "2024-02-01+02:00", docEnd: "2024-02-29",
			lineStart: "2024-01-15", lineEnd: "2024-02-29",
			wantR110: true,
		},
		{
			name:     "line period genuinely ends after the invoicing period",
			docStart: "2024-02-01", docEnd: "2024-02-29+02:00",
			lineStart: "2024-02-01", lineEnd: "2024-03-15",
			wantR111: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := findings(t, context.Background(), ValidatePeppol, build(tc.docStart, tc.docEnd, tc.lineStart, tc.lineEnd))
			if got := hasFacturXRule(v, "PEPPOL-EN16931-R110"); got != tc.wantR110 {
				t.Errorf("R110: got %v, want %v (violations: %v)", got, tc.wantR110, v)
			}
			if got := hasFacturXRule(v, "PEPPOL-EN16931-R111"); got != tc.wantR111 {
				t.Errorf("R111: got %v, want %v (violations: %v)", got, tc.wantR111, v)
			}
		})
	}
}

// TestValidatePeppolCorpus is the FP=0 oracle: every OpenPEPPOL example invoice
// must validate with no violations. The oracle is not vendored; the test skips
// when it is absent (run `make cius-oracles`).
func TestValidatePeppolCorpus(t *testing.T) {
	root := filepath.Join("testdata", "peppol", "repo", "rules", "examples")
	if _, err := os.Stat(root); err != nil {
		t.Skip("Peppol examples not present (make cius-oracles)")
	}
	isInvoice := regexp.MustCompile(`(?s)<([\w.]+:)?(Invoice|CreditNote)[\s>]`)
	var files, clean int
	filepath.Walk(root, func(p string, fi os.FileInfo, e error) error {
		if e != nil || !strings.HasSuffix(strings.ToLower(p), ".xml") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			// A read error used to leave data nil, which fails the regexp below
			// and dropped the document from the count without a word.
			t.Errorf("%s: %v", p, err)
			return nil
		}
		if !isInvoice.Match(data) {
			return nil
		}
		files++
		if v := findings(t, context.Background(), ValidatePeppol, data); len(v) != 0 {
			t.Errorf("%s: conforming Peppol reported %d violations (first: %s: %s)",
				filepath.Base(p), len(v), v[0].Rule, v[0].Message)
		} else {
			clean++
		}
		return nil
	})
	// The directory exists, so this is no longer the "corpus absent" case the
	// skip above covers: finding nothing under it, or finding less than the
	// clone carries, is a broken fetch and must say so.
	atLeast(t, "Peppol corpus", files, minPeppolExamples)
	t.Logf("Peppol corpus: %d/%d examples clean (FP=0)", clean, files)
}
