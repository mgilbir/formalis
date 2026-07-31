package formalis

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// nlciusFindings splits a report into the NLCIUS findings at each severity. It
// scopes on Source rather than on the "BR-NL-" prefix for the reason
// rsRuleViolations does, and the split is what lets the SimplerInvoicing suite's
// three verdicts be read as three verdicts rather than as two.
func nlciusFindings(vs []Violation) (fatal, advisory []string) {
	for _, v := range vs {
		if v.Source != SourceNLCIUS {
			continue
		}
		if v.Severity == SeverityFatal {
			fatal = append(fatal, v.Rule)
		} else {
			advisory = append(advisory, v.Rule)
		}
	}
	return fatal, advisory
}

// TestNLCIUSConformanceSuite uses the SimplerInvoicing SI-UBL instance test suite
// as the FP=0 oracle. Each file name encodes the rule it exercises and whether the
// instance satisfies it (…_ok_…), violates it (…_error_…) or merely uses a
// discouraged term (…_warning_…). Like the CEN unit tests, each file targets one
// rule and is not otherwise guaranteed clean, so the assertions are per-family:
//
//   - _error_ : at least one *fatal* BR-NL rule must fire (the broken instance is
//     caught);
//   - _warning_ : no fatal BR-NL rule may fire, and at least one advisory one must
//     — the instance is conformant and uses a term NLCIUS discourages;
//   - _ok_ : no BR-NL rule may fire at all, at either severity.
//
// The middle case is the half this oracle could not state until the advisory tier
// was implemented. It read "_ok_ and _warning_ are the same bucket: no BR-NL rule
// may fire", which was true of a package that did not evaluate the advisory rules
// and hid the fact that twenty-four of the suite's instances were saying something
// it was not listening to.
//
// The suite is not vendored; the test skips when testdata/nlcius is absent (run
// `make cius-oracles`).
func TestNLCIUSConformanceSuite(t *testing.T) {
	files, _ := filepath.Glob("testdata/nlcius/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("NLCIUS test suite not present (make cius-oracles)")
	}
	var falsePositives, missed, silent []string
	caught, warned, instances := 0, 0, 0
	for _, f := range files {
		base := filepath.Base(f)
		// The instances that exercise a rule of *this* rule set. That is the BR-NL
		// family plus one more: SI-UBL-2.0_warning_empty_elements.xml, which names no
		// rule in its file name because SimplerInvoicing's empty-element rule is not
		// in the BR-NL numbering. It is the authority's own firing verdict for
		// SI-UBL-2 and it went unread here for the same reason the rule itself did.
		// The rest of the suite — UBL-SR-*, UBL-CR-*, BR-S-08, no_customizationid —
		// exercises CEN's rules or the wrapper's, not SimplerInvoicing's.
		if !strings.Contains(base, "BR-NL-") && !strings.Contains(base, "_empty_elements") {
			continue
		}
		instances++
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fatal, advisory := nlciusFindings(findings(t, context.Background(), ValidateNLCIUS, data))
		switch {
		case strings.Contains(base, "_error"):
			if len(fatal) > 0 {
				caught++
			} else {
				missed = append(missed, base)
			}
		case strings.Contains(base, "_warning"):
			if len(fatal) > 0 {
				falsePositives = append(falsePositives, base+" -> "+strings.Join(fatal, ","))
			}
			if len(advisory) > 0 {
				warned++
			} else {
				silent = append(silent, base)
			}
		default: // _ok_
			if all := append(fatal, advisory...); len(all) > 0 {
				falsePositives = append(falsePositives, base+" -> "+strings.Join(all, ","))
			}
		}
	}
	for _, fp := range falsePositives {
		t.Errorf("NLCIUS false positive on a valid/advisory instance: %s", fp)
	}
	for _, m := range missed {
		t.Errorf("NLCIUS failed to catch a broken instance: %s", m)
	}
	for _, s := range silent {
		t.Errorf("NLCIUS reported nothing for %s, which SimplerInvoicing ships as an instance a validator warns "+
			"about; an advisory rule that never fires is indistinguishable from one that is not there", s)
	}
	// Three halves now, for the reason corpus_test.go gives: "no false positive"
	// over a handful of _ok_ instances is not the same claim as over the suite, a
	// truncation that took only the _error_ instances would leave the first number
	// looking healthy while the engine stopped catching anything, and the third
	// would fall silently if the advisory tier stopped being emitted.
	atLeast(t, "NLCIUS BR-NL instances", instances, minNLCIUSInstances)
	atLeast(t, "NLCIUS error instances caught", caught, minNLCIUSErrorsCaught)
	atLeast(t, "NLCIUS warning instances reported", warned, minNLCIUSWarningsReported)
	t.Logf("NLCIUS conformance: %d error instances caught, %d warning instances reported, %d false positives",
		caught, warned, len(falsePositives))
}

// TestNLCIUSPerRuleFixtures reads the verdict each SimplerInvoicing instance
// declares *for the rule it names*, which is the oracle PRs 19–21 built for KoSIT's
// <?xmute?> fixtures and OpenPEPPOL's unit-test directories, and which this suite
// had on disk and was not using.
//
// TestNLCIUSConformanceSuite above asks the weaker question: an _error_ instance
// must make *some* BR-NL rule fire. That passes for an engine that reports the
// wrong rule, and it passes for an engine in which one over-eager rule fires on
// everything. This one asks whether BR-NL-10's fixture makes BR-NL-10 fire, which
// is the question that tells a working rule from a coincidence — and it is what
// makes the twelve fatal identifiers individually load-bearing rather than
// collectively.
//
// Two exceptions, and both are derived rather than asserted.
//
// SI-UBL ships BR-NL-7_error_381_invoice.xml, whose document is an Invoice carrying
// type code 381. 381 is in BR-NL-7's permitted set, so BR-NL-7 does not fire; what
// fires is BR-NL-8, the neighbouring assertion in the same Schematron rule, which is
// what forbids a credit-note code in an Invoice. The fixture's *name* is wrong and
// its document is not. The test accepts BR-NL-8 for it, and asserts that the
// document really is an Invoice with type code 381 so the excuse cannot widen.
//
// The second is the BR-NL-34 pair, and it is what the identifier confusion PR 22
// recorded comes to in practice. SI-UBL's BR-NL-34 fixtures use a charge reason
// code, and the assertions worded "[BR-NL-34]" in si-ubl-2.0-nlcius.sch carry the
// identifiers BR-NL-32-1/2/3 and sit in rules an earlier rule of the same pattern
// has already claimed. So the identifier a conforming validator reports for those
// documents is BR-NL-32-1, and nlciusFixtureIdentifiers says so.
func TestNLCIUSPerRuleFixtures(t *testing.T) {
	files, _ := filepath.Glob("testdata/nlcius/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("NLCIUS test suite not present (make cius-oracles)")
	}
	named := regexp.MustCompile(`(BR-NL-[0-9]+)_(ok|error|warning)`)
	checked, verdicts := map[string]bool{}, 0
	for _, f := range files {
		base := filepath.Base(f)
		m := named.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rule, verdict := m[1], m[2]
		fatal, advisory := nlciusFindings(findings(t, context.Background(), ValidateNLCIUS, data))
		verdicts++
		switch verdict {
		case "ok":
			// No BR-NL rule at all: SimplerInvoicing ships these as documents a
			// conforming validator says nothing whatever about.
			if all := append(append([]string{}, fatal...), advisory...); len(all) != 0 {
				t.Errorf("%s: SimplerInvoicing declares this instance ok for %s, and this package reports %v",
					base, rule, all)
			}
			continue
		case "warning":
			// The advisory half of the verdict, which this suite carried unread until
			// the "not recommended" tier was implemented: the named rule must report,
			// at SeverityWarning, and no fatal rule may.
			if len(fatal) != 0 {
				t.Errorf("%s: SimplerInvoicing declares this instance a warning for %s, and this package reports "+
					"the fatal findings %v", base, rule, fatal)
			}
			hit := nlciusFixtureHit(rule, advisory)
			if hit == "" {
				t.Errorf("%s: SimplerInvoicing declares this instance a warning against %s, and this package "+
					"reports %v", base, rule, advisory)
				continue
			}
			checked[hit] = true
			continue
		}
		want := rule
		if strings.Contains(base, "BR-NL-7_error_381_invoice") {
			if !strings.Contains(string(data), "<cbc:InvoiceTypeCode>381<") {
				t.Errorf("%s is excused as a mis-named BR-NL-8 fixture, but it is not an Invoice with type code "+
					"381 any more; re-derive the exception rather than widening it", base)
			}
			want = "BR-NL-8"
		}
		hit := false
		for _, r := range fatal {
			if r == want {
				hit = true
			}
		}
		if !hit {
			t.Errorf("%s: SimplerInvoicing declares this instance invalid against %s, and this package reports %v",
				base, want, fatal)
			continue
		}
		checked[want] = true
	}
	atLeast(t, "NLCIUS per-rule verdicts", verdicts, minNLCIUSRuleVerdicts)
	atLeast(t, "NLCIUS rules with a fixture that fires them", len(checked), minNLCIUSRulesFired)
	t.Logf("NLCIUS per-rule fixtures: %d verdicts read, %d distinct rules made to fire by the fixture that names them",
		verdicts, len(checked))
}

// nlciusFixtureHit returns the reported identifier that answers a fixture named for
// rule, or "".
//
// A fixture is named for the rule as SimplerInvoicing's documentation numbers it,
// and three of the advisory rules are published under sub-identifiers instead:
// BR-NL-27 is BR-NL-27-1..4, one per address, BR-NL-28 likewise, and BR-NL-32 is
// BR-NL-32-1..3. So "the identifier itself, or one of its numbered children" is the
// match, plus the one aliasing the artefact itself performs — see the test's own
// comment on BR-NL-34.
func nlciusFixtureHit(rule string, reported []string) string {
	want := rule
	if rule == "BR-NL-34" {
		want = "BR-NL-32"
	}
	for _, r := range reported {
		if r == want || strings.HasPrefix(r, want+"-") {
			return r
		}
	}
	return ""
}

// minimalNLCIUSUBL is a small SI-UBL 2.0 invoice from a Dutch supplier that
// satisfies every fatal BR-NL rule, with distinct values so each term can be
// mutated in isolation. BuyerReference and OrderReference sit adjacent so that
// BR-NL-2 — which needs *both* gone — is one substitution.
const minimalNLCIUSUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0</cbc:CustomizationID>
<cbc:ID>INV-1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cbc:BuyerReference>NLBUYERREF</cbc:BuyerReference><cac:OrderReference><cbc:ID>ORD-1</cbc:ID></cac:OrderReference>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:PostalZone>1011AA</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>NL123456789B01</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">11223344</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:PostalZone>2022BB</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">55667788</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Rep BV</cbc:Name></cac:PartyName><cac:PostalAddress><cbc:StreetName>RepStreet</cbc:StreetName><cbc:CityName>RepCity</cbc:CityName><cbc:PostalZone>3033CC</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:TaxRepresentativeParty>
<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>
<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">21.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>121.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>121.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:OrderLineReference><cbc:LineID>1</cbc:LineID></cac:OrderLineReference><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// nlciusMutations is one fixture per fatal BR-NL identifier. NLCIUS is the one CIUS
// here whose authority also ships per-rule fixtures (TestNLCIUSPerRuleFixtures
// reads them), so these are the belt to that suite's braces: they run without a
// corpus, and they pin the two rules the SI-UBL suite exercises only through a
// document that also trips something else.
var nlciusMutations = []ciusMutation{
	{"seller legal id not KVK or OIN (1)", `<cbc:CompanyID schemeID="0106">11223344</cbc:CompanyID>`, `<cbc:CompanyID schemeID="0088">11223344</cbc:CompanyID>`, "BR-NL-1"},
	{"no buyer reference and no order reference (2)", `<cbc:BuyerReference>NLBUYERREF</cbc:BuyerReference><cac:OrderReference><cbc:ID>ORD-1</cbc:ID></cac:OrderReference>`, "", "BR-NL-2"},
	{"no seller street (3)", "<cbc:StreetName>SellerStreet</cbc:StreetName>", "", "BR-NL-3"},
	{"no Dutch buyer street (4)", "<cbc:StreetName>BuyerStreet</cbc:StreetName>", "", "BR-NL-4"},
	{"no Dutch tax representative street (5)", "<cbc:StreetName>RepStreet</cbc:StreetName>", "", "BR-NL-5"},
	{"type code outside the permitted set (7)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>100</cbc:InvoiceTypeCode>", "BR-NL-7"},
	{"credit-note type code in an Invoice (8)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>381</cbc:InvoiceTypeCode>", "BR-NL-8"},
	{"corrective invoice without a preceding reference (9)", "<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>", "<cbc:InvoiceTypeCode>384</cbc:InvoiceTypeCode>", "BR-NL-9"},
	{"Dutch buyer legal id not KVK or OIN (10)", `<cbc:CompanyID schemeID="0106">55667788</cbc:CompanyID>`, `<cbc:CompanyID schemeID="0088">55667788</cbc:CompanyID>`, "BR-NL-10"},
	{"no means of payment (11)", `<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`, "", "BR-NL-11"},
	{"payment means code outside the permitted set (12)", "<cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>", "<cbc:PaymentMeansCode>31</cbc:PaymentMeansCode>", "BR-NL-12"},
	{"order line reference without a document order reference (13)", `<cac:OrderReference><cbc:ID>ORD-1</cbc:ID></cac:OrderReference>`, "", "BR-NL-13"},
}

func TestNLCIUSMutations(t *testing.T) {
	runCIUSSuite(t, ciusSuites()[4])
}

// nlciusUBLDiscouragedDoc is a Dutch SI-UBL invoice that conforms to every fatal
// BR-NL rule and uses every term the UBL binding's advisory tier discourages. It is
// the corpus-free half of the firing verdict for those twenty rules: the
// SimplerInvoicing suite covers eighteen of them and skips this repository entirely
// when testdata/ is absent, which is the state a clean checkout and CI are both in.
//
// One document rather than twenty, because these rules are forbidden *paths* and a
// document that walks all of them at once demonstrates each independently — there is
// no interaction between "the seller address has a third line" and "the payment
// account has a name". TestEveryEvaluatedCIUSRuleFires reads it once per identifier.
const nlciusUBLDiscouragedDoc = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0</cbc:CustomizationID>
<cbc:ID>INV-W1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cbc:TaxCurrencyCode>SEK</cbc:TaxCurrencyCode>
<cbc:TaxPointDate>2024-01-14</cbc:TaxPointDate>
<cbc:BuyerReference>NLBUYERREF</cbc:BuyerReference>
<cac:InvoicePeriod><cbc:DescriptionCode>35</cbc:DescriptionCode></cac:InvoicePeriod>
<cac:BillingReference><cac:InvoiceDocumentReference><cbc:ID>PRIOR-1</cbc:ID><cbc:IssueDate>2023-12-01</cbc:IssueDate></cac:InvoiceDocumentReference></cac:BillingReference>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:PostalZone>1011AA</cbc:PostalZone><cbc:CountrySubentity>Noord-Holland</cbc:CountrySubentity><cac:AddressLine><cbc:Line>Seller line 3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>NL123456789B01</cbc:CompanyID><cac:TaxScheme><cbc:ID>LOCAL</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">11223344</cbc:CompanyID><cbc:CompanyLegalForm>Besloten vennootschap</cbc:CompanyLegalForm></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:PostalZone>2022BB</cbc:PostalZone><cbc:CountrySubentity>Zuid-Holland</cbc:CountrySubentity><cac:AddressLine><cbc:Line>Buyer line 3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">55667788</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:TaxRepresentativeParty><cac:PartyName><cbc:Name>Rep BV</cbc:Name></cac:PartyName><cac:PostalAddress><cbc:StreetName>RepStreet</cbc:StreetName><cbc:CityName>RepCity</cbc:CityName><cbc:PostalZone>3033CC</cbc:PostalZone><cbc:CountrySubentity>Utrecht</cbc:CountrySubentity><cac:AddressLine><cbc:Line>Rep line 3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress></cac:TaxRepresentativeParty>
<cac:Delivery><cac:DeliveryLocation><cac:Address><cbc:StreetName>DeliveryStreet</cbc:StreetName><cbc:CountrySubentity>Gelderland</cbc:CountrySubentity><cac:AddressLine><cbc:Line>Delivery line 3</cbc:Line></cac:AddressLine><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:Address></cac:DeliveryLocation></cac:Delivery>
<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>95</cbc:AllowanceChargeReasonCode><cbc:Amount>0.00</cbc:Amount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>
<cac:PaymentMeans><cbc:PaymentMeansCode name="SEPA credit transfer">58</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID><cbc:Name>Seller current account</cbc:Name><cac:FinancialInstitutionBranch><cbc:ID>ABNANL2A</cbc:ID></cac:FinancialInstitutionBranch></cac:PayeeFinancialAccount></cac:PaymentMeans>
<cac:TaxTotal><cbc:TaxAmount currencyID="SEK">231.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>E</cbc:ID><cbc:Percent>0</cbc:Percent><cbc:TaxExemptionReasonCode>VATEX-EU-132</cbc:TaxExemptionReasonCode><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>121.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>121.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// nlciusCIIDiscouragedDoc is the same idea in the other binding, and it is the only
// fixture in this repository that reaches the three identifiers NLCIUS-CII publishes
// and SI-UBL does not: BR-NL-22, BR-NL-23 and BR-NL-32-and-34. SimplerInvoicing's
// instance suite is UBL only, so nothing else in this repository or in its corpus
// exercises the CII advisory tier at all.
const nlciusCIIDiscouragedDoc = `<CrossIndustryInvoice>
<ExchangedDocumentContext>
  <BusinessProcessSpecifiedDocumentContextParameter><ID>urn:fdc:peppol.eu:2017:poacc:billing:01:1.0</ID></BusinessProcessSpecifiedDocumentContextParameter>
  <GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0</ID></GuidelineSpecifiedDocumentContextParameter>
</ExchangedDocumentContext>
<ExchangedDocument><ID>INV-C1</ID><TypeCode>380</TypeCode><IssueDateTime><DateTimeString format="102">20240115</DateTimeString></IssueDateTime>
  <IncludedNote><Content>Note</Content><SubjectCode>AAI</SubjectCode></IncludedNote>
</ExchangedDocument>
<SupplyChainTradeTransaction>
  <IncludedSupplyChainTradeLineItem><AssociatedDocumentLineDocument><LineID>1</LineID></AssociatedDocumentLineDocument>
    <SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>
    <SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>
    <SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>
    <SpecifiedLineTradeSettlement><ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>21</RateApplicablePercent></ApplicableTradeTax>
      <SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>100.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation></SpecifiedLineTradeSettlement>
  </IncludedSupplyChainTradeLineItem>
  <ApplicableHeaderTradeAgreement><BuyerReference>NLBUYERREF</BuyerReference>
    <SellerTradeParty><Name>Seller BV</Name><Description>Besloten vennootschap</Description>
      <SpecifiedLegalOrganization><ID schemeID="0106">11223344</ID></SpecifiedLegalOrganization>
      <PostalTradeAddress><PostcodeCode>1011AA</PostcodeCode><LineOne>SellerStreet</LineOne><LineThree>Seller line 3</LineThree><CityName>SellerCity</CityName><CountryID>NL</CountryID><CountrySubDivisionName>Noord-Holland</CountrySubDivisionName></PostalTradeAddress>
      <SpecifiedTaxRegistration><ID schemeID="FC">NL123456789B01</ID></SpecifiedTaxRegistration></SellerTradeParty>
    <BuyerTradeParty><Name>Buyer BV</Name><SpecifiedLegalOrganization><ID schemeID="0106">55667788</ID></SpecifiedLegalOrganization>
      <PostalTradeAddress><PostcodeCode>2022BB</PostcodeCode><LineOne>BuyerStreet</LineOne><LineThree>Buyer line 3</LineThree><CityName>BuyerCity</CityName><CountryID>NL</CountryID><CountrySubDivisionName>Zuid-Holland</CountrySubDivisionName></PostalTradeAddress></BuyerTradeParty>
  </ApplicableHeaderTradeAgreement>
  <ApplicableHeaderTradeDelivery>
    <ShipToTradeParty><Name>Delivery</Name><PostalTradeAddress><LineOne>DeliveryStreet</LineOne><LineThree>Delivery line 3</LineThree><CountryID>NL</CountryID><CountrySubDivisionName>Gelderland</CountrySubDivisionName></PostalTradeAddress></ShipToTradeParty>
    <ActualDeliverySupplyChainEvent><OccurrenceDateTime><DateTimeString format="102">20240115</DateTimeString></OccurrenceDateTime></ActualDeliverySupplyChainEvent>
  </ApplicableHeaderTradeDelivery>
  <ApplicableHeaderTradeSettlement><InvoiceCurrencyCode>EUR</InvoiceCurrencyCode><TaxCurrencyCode>SEK</TaxCurrencyCode>
    <SellerTaxRepresentativeTradeParty><Name>Rep BV</Name><PostalTradeAddress><PostcodeCode>3033CC</PostcodeCode><LineOne>RepStreet</LineOne><LineThree>Rep line 3</LineThree><CityName>RepCity</CityName><CountryID>NL</CountryID><CountrySubDivisionName>Utrecht</CountrySubDivisionName></PostalTradeAddress></SellerTaxRepresentativeTradeParty>
    <SpecifiedTradeSettlementPaymentMeans><TypeCode>58</TypeCode><Information>SEPA credit transfer</Information>
      <PayeePartyCreditorFinancialAccount><IBANID>NL02ABNA0123456789</IBANID><AccountName>Seller current account</AccountName></PayeePartyCreditorFinancialAccount>
      <PayeeSpecifiedCreditorFinancialInstitution><BICID>ABNANL2A</BICID></PayeeSpecifiedCreditorFinancialInstitution></SpecifiedTradeSettlementPaymentMeans>
    <ApplicableTradeTax><CalculatedAmount>21.00</CalculatedAmount><TypeCode>VAT</TypeCode><BasisAmount>100.00</BasisAmount><CategoryCode>E</CategoryCode><ExemptionReasonCode>VATEX-EU-132</ExemptionReasonCode><RateApplicablePercent>0</RateApplicablePercent>
      <TaxPointDate><DateString format="102">20240114</DateString></TaxPointDate><DueDateTypeCode>5</DueDateTypeCode></ApplicableTradeTax>
    <SpecifiedTradeAllowanceCharge><ChargeIndicator><Indicator>false</Indicator></ChargeIndicator><ActualAmount>0.00</ActualAmount><ReasonCode>95</ReasonCode>
      <CategoryTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>21</RateApplicablePercent></CategoryTradeTax></SpecifiedTradeAllowanceCharge>
    <InvoiceReferencedDocument><IssuerAssignedID>PRIOR-1</IssuerAssignedID><FormattedIssueDateTime><DateTimeString format="102">20231201</DateTimeString></FormattedIssueDateTime></InvoiceReferencedDocument>
    <SpecifiedTradeSettlementHeaderMonetarySummation><LineTotalAmount>100.00</LineTotalAmount><TaxBasisTotalAmount>100.00</TaxBasisTotalAmount><TaxTotalAmount currencyID="SEK">231.00</TaxTotalAmount><GrandTotalAmount>121.00</GrandTotalAmount><DuePayableAmount>121.00</DuePayableAmount></SpecifiedTradeSettlementHeaderMonetarySummation>
  </ApplicableHeaderTradeSettlement>
</SupplyChainTradeTransaction>
</CrossIndustryInvoice>`

// nlciusUBLEmptyElementDoc and nlciusCIIEmptyElementDoc are the firing verdict for
// the empty-element rule, and each carries *two* empty elements rather than one,
// because the rule has two halves to demonstrate.
//
// The first empty element is one no earlier rule of the nlcius pattern claims, so
// the rule reports it: cbc:Note in UBL, ram:PaymentReference in CII.
//
// The second is cbc:TaxCurrencyCode / ram:TaxCurrencyCode, which BR-NL-19's rule
// claims — earlier in the same pattern — and which the empty-element rule therefore
// never sees. Both documents are inside $s, so the claim is live: a conforming
// processor reports BR-NL-19 for that element and *not* SI-UBL-2. That is the
// rule-order arithmetic nlciusEmptyElements does, and
// TestNLCIUSEmptyElementYieldsTheClaimedNode is what checks it, because no document
// in the corpus exercises it — SimplerInvoicing's own empty-element instance leaves
// both its empty elements unclaimed.
var nlciusUBLEmptyElementDoc = strings.Replace(minimalNLCIUSUBL,
	"<cbc:ID>INV-1</cbc:ID>",
	"<cbc:ID>INV-1</cbc:ID><cbc:Note></cbc:Note><cbc:TaxCurrencyCode></cbc:TaxCurrencyCode>", 1)

var nlciusCIIEmptyElementDoc = strings.Replace(nlciusCIIDiscouragedDoc,
	"<TaxCurrencyCode>SEK</TaxCurrencyCode>",
	"<TaxCurrencyCode></TaxCurrencyCode><PaymentReference></PaymentReference>", 1)

// nlciusExtras is one entry per advisory identifier, pointing at whichever of the
// two documents above belongs to the binding that publishes it. It is the
// corpus-free firing verdict C41 asks for: a rule that no fixture makes report is a
// rule that could be deleted without a red build, and the SimplerInvoicing suite —
// which covers eighteen of these and is the stronger oracle where it applies —
// skips on a checkout with no testdata/.
var nlciusExtras = []ciusDoc{
	{"VAT accounting currency code (19)", nlciusUBLDiscouragedDoc, "BR-NL-19"},
	{"tax point date (20)", nlciusUBLDiscouragedDoc, "BR-NL-20"},
	{"tax point date code (21)", nlciusUBLDiscouragedDoc, "BR-NL-21"},
	{"invoice note subject code, CII only (22)", nlciusCIIDiscouragedDoc, "BR-NL-22"},
	{"business process identifier, CII only (23)", nlciusCIIDiscouragedDoc, "BR-NL-23"},
	{"preceding invoice issue date (24)", nlciusUBLDiscouragedDoc, "BR-NL-24"},
	{"seller tax registration outside VAT (25)", nlciusUBLDiscouragedDoc, "BR-NL-25"},
	{"seller additional legal information (26)", nlciusUBLDiscouragedDoc, "BR-NL-26"},
	{"seller address line 3 (27-1)", nlciusUBLDiscouragedDoc, "BR-NL-27-1"},
	{"buyer address line 3 (27-2)", nlciusUBLDiscouragedDoc, "BR-NL-27-2"},
	{"tax representative address line 3 (27-3)", nlciusUBLDiscouragedDoc, "BR-NL-27-3"},
	{"delivery address line 3 (27-4)", nlciusUBLDiscouragedDoc, "BR-NL-27-4"},
	{"seller country subdivision (28-1)", nlciusUBLDiscouragedDoc, "BR-NL-28-1"},
	{"buyer country subdivision (28-2)", nlciusUBLDiscouragedDoc, "BR-NL-28-2"},
	{"tax representative country subdivision (28-3)", nlciusUBLDiscouragedDoc, "BR-NL-28-3"},
	{"delivery country subdivision (28-4)", nlciusUBLDiscouragedDoc, "BR-NL-28-4"},
	{"payment means text (29)", nlciusUBLDiscouragedDoc, "BR-NL-29"},
	{"payment account name (30)", nlciusUBLDiscouragedDoc, "BR-NL-30"},
	{"payment service provider on a SEPA payment (31)", nlciusUBLDiscouragedDoc, "BR-NL-31"},
	{"allowance or charge reason code (32-1)", nlciusUBLDiscouragedDoc, "BR-NL-32-1"},
	{"allowance or charge reason code, CII (32-and-34)", nlciusCIIDiscouragedDoc, "BR-NL-32-and-34"},
	{"VAT total in the accounting currency (33)", nlciusUBLDiscouragedDoc, "BR-NL-33"},
	{"VAT exemption reason code (35)", nlciusUBLDiscouragedDoc, "BR-NL-35"},
	{"empty element, UBL binding (SI-UBL-2)", nlciusUBLEmptyElementDoc, "SI-UBL-2"},
	{"empty element, CII binding (empty-element-check)", nlciusCIIEmptyElementDoc, "empty-element-check"},
}

// nlciusUnevaluable is what Coverage(SourceNLCIUS) records, per binding, in one
// place so the table and the test below cannot drift apart by hand.
var nlciusUnevaluable = map[string][]string{
	"ubl": {"BR-NL-32-1", "BR-NL-32-2", "BR-NL-32-3"},
	"cii": {"BR-NL-9", "BR-NL-31"},
}

// TestNLCIUSUnevaluableRulesAreDerivedFromTheArtefacts re-derives, from the two
// binding files, every assertion whose Schematron rule an earlier rule of the same
// pattern has already claimed.
//
// It is per binding because NLCIUS is the one rule set here whose two artefacts
// disagree about which rules are reachable, and the disagreement was a live false
// positive: BR-NL-9 has a rule of its own in NLCIUS-CII-validation.sch, against a
// context the rule carrying BR-NL-7 already holds, so no CII invoice can be reported
// for it — and this package reported it for CII until this was read.
//
// The UBL list is three identifiers rather than two, and that is the resolution of
// the BR-NL-34 curiosity. BR-NL-32-1 appears in *two* rules: the first is reached and
// is what this package emits, the second — the one whose message text reads
// "[BR-NL-34]" — is not. So the identifier is shadowed *somewhere* while remaining
// evaluable, which is why this test reads the shadowing per rule and
// nlciusEvaluableAnyway records the one identifier that is both.
func TestNLCIUSUnevaluableRulesAreDerivedFromTheArtefacts(t *testing.T) {
	for _, b := range []struct{ dir, glob string }{
		{"ubl", "si-ubl-2.0-nlcius.sch"},
		{"cii", "NLCIUS-CII-validation.sch"},
	} {
		files, _ := filepath.Glob(filepath.Join("testdata", "nlcius", "schematron", b.dir, b.glob))
		if len(files) == 0 {
			t.Skip("NLCIUS Schematron not present; run `make cius-schematron`")
		}
		shadowed := schShadowedPerRule(t, files)
		want := map[string]bool{}
		for _, id := range nlciusUnevaluable[b.dir] {
			want[id] = true
			if _, ok := shadowed[id]; !ok {
				t.Errorf("%s: this package records %s as having a rule no processor reaches, and every rule "+
					"carrying it is reachable. Either SimplerInvoicing split its pattern — in which case the "+
					"rule is evaluable and belongs in the code — or the claim was never true", b.glob, id)
				continue
			}
			t.Logf("NLCIUS/%s %s: context %q is claimed by an earlier rule of the same pattern",
				strings.ToUpper(b.dir), id, shadowed[id])
		}
		for id, by := range shadowed {
			if !want[id] {
				t.Errorf("%s shadows %s (context %q) and nothing in this package records it", b.glob, id, by)
			}
		}
	}
	// The identifier that is shadowed in one rule and reached in another. It is the
	// only one, and stating it separately is what stops the list above being read as
	// "these three are not evaluated".
	if _, ok := ciusEvaluated[SourceNLCIUS]["BR-NL-32-1"]; !ok {
		t.Error("BR-NL-32-1 is not in ciusEvaluated, so this package emits nothing for an allowance or charge " +
			"reason code — which is the one advisory identifier SI-UBL's reachable rule carries")
	}
	for _, id := range []string{"BR-NL-32-2", "BR-NL-32-3"} {
		if _, ok := ciusEvaluated[SourceNLCIUS][id]; ok {
			t.Errorf("%s is in ciusEvaluated, and no SI-UBL rule carrying it is reachable", id)
		}
	}
}

// schShadowedPerRule is schShadowed without the per-identifier fold: it reports an
// identifier whose *rule* is unreachable even when another rule carries it too. The
// NLCIUS UBL binding needs the distinction and no other artefact here does.
func schShadowedPerRule(t *testing.T, files []string) map[string]string {
	t.Helper()
	return schShadowedRules(t, files)
}

// TestNLCIUSRuleContextsAreReachable is requirement two of this rule set's oracle,
// over both bindings at once — the sweep runs the rule body over every document
// testdata holds, and the body picks its binding from the document.
//
// The exception list is what the $si gate costs. Every BR-NL rule is inside "this
// document declares the NLCIUS customization identifier", and the corpus's only
// NLCIUS documents are SimplerInvoicing's 95 UBL instances: nothing in it is an
// NLCIUS *CII* invoice, so the three identifiers only the CII binding publishes have
// no context node anywhere. The test verifies that rather than asserting it, and
// nlciusCIIDiscouragedDoc is what gives those three a firing verdict meanwhile.
func TestNLCIUSRuleContextsAreReachable(t *testing.T) {
	seen, files := ciusContextSweep(t, func(p *parsed, seen ruleContexts) {
		validateNLCIUSRules(p, seen)
	})
	atLeast(t, "NLCIUS context sweep corpus", files, minCorpusDocuments)

	const noCII = "the corpus holds no CII document declaring the NLCIUS customization identifier, and these " +
		"three are published in the CII binding only"
	reportUnreached(t, "NLCIUS", seen, keysOfSeverityMap(ciusEvaluated[SourceNLCIUS]), map[string]string{
		"BR-NL-22":        noCII,
		"BR-NL-23":        noCII,
		"BR-NL-32-and-34": noCII,
	})

	// The excuse, verified rather than asserted.
	ciiNLCIUS := 0
	_, _ = ciusContextSweep(t, func(p *parsed, _ ruleContexts) {
		if p.inv.syntax == "CII" && nlciusApplies(p.inv) {
			ciiNLCIUS++
		}
	})
	if ciiNLCIUS != 0 {
		t.Errorf("the corpus now holds %d NLCIUS CII documents, so the CII-only identifiers are reachable and "+
			"their exception in this test is stale", ciiNLCIUS)
	}
}

// nlciusBindings is the two artefact files this rule set is read out of, with the
// identifier each gives the empty-element rule.
var nlciusBindings = []struct{ dir, file, id string }{
	{"ubl", "si-ubl-2.0-nlcius.sch", "SI-UBL-2"},
	{"cii", "NLCIUS-CII-validation.sch", "empty-element-check"},
}

// nlciusBindingPattern reads one binding's single pattern, or skips.
func nlciusBindingPattern(t *testing.T, dir, file string) schPattern {
	t.Helper()
	path := filepath.Join("testdata", "nlcius", "schematron", dir, file)
	if _, err := os.Stat(path); err != nil {
		t.Skip("NLCIUS Schematron not present; run `make cius-schematron`")
	}
	return readSchPattern(t, path)
}

// TestNLCIUSEmptyElementRuleIsPublishedUngatedAndLast reads out of both artefacts
// the two facts the implementation rests on, rather than believing either.
//
// The first is that the rule carries no gate. Every other rule in both patterns has
// a context ending in [$s] or [$si] — "the supplier is Dutch and this document
// declares the NLCIUS specification identifier", or just the second — and this one
// has neither, so SimplerInvoicing's own validator reports empty elements in a
// document that is not an NLCIUS invoice. ValidateNLCIUS does the same, and this is
// what says it is entitled to: reporting less than the authority reports, under a
// gate this package invented, would be a departure from the artefact in the same
// way that reporting more is.
//
// The second is that it is *last* in its pattern. ISO Schematron gives a node to the
// first matching rule and no other, so a rule with the broadest context in the file
// would suppress everything after it if it came first. Coming last it suppresses
// nothing, and is itself suppressed for any node an earlier rule claims — which is
// the arithmetic nlciusUBLPatternContexts and nlciusCIIPatternContexts encode.
func TestNLCIUSEmptyElementRuleIsPublishedUngatedAndLast(t *testing.T) {
	const wantContext = "//*[not(*) and not(normalize-space())]"
	for _, b := range nlciusBindings {
		p := nlciusBindingPattern(t, b.dir, b.file)
		last := p.Rules[len(p.Rules)-1]
		if got := advisoryNorm(last.Context); got != wantContext {
			t.Errorf("%s: the last rule of the pattern has context %q, want %q. The empty-element rule has "+
				"moved or gone, and everything nlciusEmptyElements does about rule order depends on where it is",
				b.file, got, wantContext)
			continue
		}
		if len(last.Asserts) != 1 {
			t.Fatalf("%s: the empty-element rule carries %d assertions, want 1", b.file, len(last.Asserts))
		}
		a := last.Asserts[0]
		if a.ID != b.id {
			t.Errorf("%s: the empty-element assertion is published as %q and this package emits %q. Each binding's "+
				"own identifier is what a caller compares against a reference validator", b.file, a.ID, b.id)
		}
		if a.Flag != "warning" {
			t.Errorf("%s/%s carries flag=%q; this package emits it as SeverityWarning, and a severity is a "+
				"quotation", b.file, a.ID, a.Flag)
		}
		if a.Test != "false()" {
			t.Errorf("%s/%s has test=%q; the implementation treats the context as the rule, which is only right "+
				"while the assertion cannot pass", b.file, a.ID, a.Test)
		}
		if got := advisoryNorm(a.Text); got != "Document should not contain empty elements." {
			t.Errorf("%s/%s says %q", b.file, a.ID, got)
		}
		// Ungated, and every other rule in the pattern gated: the two halves of the
		// claim, so neither "it lost its gate" nor "they all lost theirs" reads as a
		// pass.
		if _, gate := schSplitPredicate(advisoryNorm(last.Context)); gate != "" {
			t.Errorf("%s: the empty-element rule now carries the gate %q, so evaluating it ungated reports on "+
				"documents its authority does not", b.file, gate)
		}
		for i, r := range p.Rules[:len(p.Rules)-1] {
			for _, alt := range schAlternatives(advisoryNorm(r.Context)) {
				if _, gate := schSplitPredicate(alt); gate == "" {
					t.Errorf("%s: rule %d, context %q, carries no $s or $si gate. A second ungated rule would "+
						"mean the file's shape has changed and the reasoning in nlciusEmptyElements needs "+
						"re-reading", b.file, i+1, alt)
				}
			}
		}
	}
}

// TestNLCIUSEmptyElementShadowsAreDerivedFromTheArtefacts re-derives
// nlciusUBLPatternContexts and nlciusCIIPatternContexts from the two files, in both
// directions.
//
// Those two tables are a claim about somebody else's Schematron: "these are the
// contexts that come before the empty-element rule in its pattern, and so these are
// the nodes it never sees". A stale entry makes this package suppress a finding the
// authority reports; a missing one makes it report a finding the authority does not.
// Neither shows up as a failure anywhere else, because no document in the corpus
// exercises the interaction at all — which is exactly the state C42's eight Serbian
// false positives lived in.
func TestNLCIUSEmptyElementShadowsAreDerivedFromTheArtefacts(t *testing.T) {
	for _, b := range nlciusBindings {
		p := nlciusBindingPattern(t, b.dir, b.file)
		want := map[string]bool{}
		for _, r := range p.Rules[:len(p.Rules)-1] {
			for _, alt := range schAlternatives(advisoryNorm(r.Context)) {
				want[nlciusContextKey(nlciusParseContext(t, b.file, alt))] = true
			}
		}
		got := map[string]bool{}
		table := nlciusUBLPatternContexts
		if b.dir == "cii" {
			table = nlciusCIIPatternContexts
		}
		for _, c := range table {
			got[nlciusContextKey(c)] = true
		}
		// The comparison is on the *set*, because several rules repeat a context and
		// the tables hold each once: a context claims the same nodes however many
		// rules are bound to it.
		for k := range want {
			if !got[k] {
				t.Errorf("%s: the pattern holds the context %q before the empty-element rule and neither "+
					"nlciusUBLPatternContexts nor nlciusCIIPatternContexts does. This package will report an "+
					"empty element for a node its authority gives to that rule", b.file, k)
			}
		}
		for k := range got {
			if !want[k] {
				t.Errorf("%s: this package suppresses empty-element findings for the context %q and no rule of "+
					"the pattern claims it. That is a finding the authority reports and this package does not", b.file, k)
			}
		}
		t.Logf("NLCIUS/%s: %d distinct contexts precede the empty-element rule, over %d rules",
			strings.ToUpper(b.dir), len(want), len(p.Rules)-1)
	}
}

// nlciusParseContext reads one rule context out of an artefact into the shape
// nlciusUBLPatternContexts is written in: local element names, an absolute flag for
// the two contexts anchored at "/*", and the gate.
//
// It fails rather than guesses. A context this package has never seen a shape of —
// a predicate other than [$s] or [$si], an axis, a wildcard step — must not be
// silently read as something narrower, because a context read too narrowly makes
// this package report an empty element the authority gives to another rule.
func nlciusParseContext(t *testing.T, file, alt string) nlciusPatternContext {
	t.Helper()
	path, gate := schSplitPredicate(alt)
	switch gate {
	case "[$s]":
	case "[$si]":
	default:
		t.Errorf("%s: the context %q carries the gate %q, and nlciusEmptyElements knows only [$s] and [$si]",
			file, alt, gate)
	}
	c := nlciusPatternContext{si: gate == "[$si]"}
	if strings.HasPrefix(path, "/*") {
		c.absolute = true
		path = strings.TrimPrefix(strings.TrimPrefix(path, "/*"), "/")
	}
	if path != "" {
		for _, step := range strings.Split(path, "/") {
			if strings.ContainsAny(step, "[@()* ") {
				t.Errorf("%s: the context step %q in %q is not a plain element name", file, step, alt)
			}
			if i := strings.Index(step, ":"); i >= 0 {
				step = step[i+1:]
			}
			c.path = append(c.path, step)
		}
	}
	return c
}

// nlciusContextKey is the canonical form both sides of the comparison are reduced
// to.
func nlciusContextKey(c nlciusPatternContext) string {
	k := strings.Join(c.path, "/")
	if c.absolute {
		k = "/" + k
	}
	if c.si {
		return k + "[$si]"
	}
	return k + "[$s]"
}

// TestNLCIUSEmptyElementYieldsTheClaimedNode is the behavioural half of the rule
// order: a document with two empty elements, one of which BR-NL-19's rule claims,
// must produce one empty-element finding and one BR-NL-19.
//
// It is a test rather than a note because the corpus cannot see it. Every empty
// element in all 1,690 documents falls outside every earlier context — the only
// NLCIUS instances with empty elements are SimplerInvoicing's three, and theirs are
// cbc:Note, cbc:BuyerReference and cbc:CompanyID — so an implementation that ignored
// rule order entirely would sweep the corpus with an identical answer.
func TestNLCIUSEmptyElementYieldsTheClaimedNode(t *testing.T) {
	for _, tc := range []struct{ name, doc, id, reported, claimed string }{
		{"UBL", nlciusUBLEmptyElementDoc, "SI-UBL-2", "Note", "TaxCurrencyCode"},
		{"CII", nlciusCIIEmptyElementDoc, "empty-element-check", "PaymentReference", "TaxCurrencyCode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var empties, br19 []string
			for _, v := range findings(t, context.Background(), ValidateNLCIUS, []byte(tc.doc)) {
				switch {
				case v.Rule == tc.id:
					if v.Severity != SeverityWarning {
						t.Errorf("%s arrived as %s; both artefacts flag it warning", v.Rule, v.Severity)
					}
					empties = append(empties, v.Message)
				case v.Rule == "BR-NL-19":
					br19 = append(br19, v.Message)
				}
			}
			if len(empties) != 1 || !strings.Contains(empties[0], `"`+tc.reported+`"`) {
				t.Errorf("%s reported %v; want exactly one finding, naming %q. The other empty element in this "+
					"document is claimed by the rule carrying BR-NL-19, which is earlier in the same pattern",
					tc.id, empties, tc.reported)
			}
			for _, m := range empties {
				if strings.Contains(m, `"`+tc.claimed+`"`) {
					t.Errorf("%s was reported for %q, which BR-NL-19's rule claims. No NLCIUS validator reports "+
						"both identifiers for one node", tc.id, tc.claimed)
				}
			}
			if len(br19) != 1 {
				t.Errorf("BR-NL-19 fired %d times for a document with one (empty) %s; the claim that suppresses "+
					"the empty-element finding is that this rule takes the node instead", len(br19), tc.claimed)
			}
		})
	}
}

// TestNLCIUSEmptyElementIsReportedOutsideTheGate is the other decision, asserted on
// a document: an invoice that declares no NLCIUS specification identifier at all
// still collects the empty-element finding, and collects nothing else from this rule
// set.
//
// That is a deliberate widening of what ValidateNLCIUS says. Every BR-NL rule is
// silent for such a document — nlciusApplies is where that is argued — and this one
// is not, because the artefact does not gate it. The second half of the assertion is
// what keeps the widening honest: no *other* NLCIUS identifier may leak out with it.
func TestNLCIUSEmptyElementIsReportedOutsideTheGate(t *testing.T) {
	doc := strings.Replace(nlciusUBLEmptyElementDoc,
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0",
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0", 1)
	if doc == nlciusUBLEmptyElementDoc {
		t.Fatal("the customization identifier was not replaced")
	}
	var got []string
	for _, v := range findings(t, context.Background(), ValidateNLCIUS, []byte(doc)) {
		if v.Source == SourceNLCIUS {
			got = append(got, v.Rule)
		}
	}
	// Two empty elements now, because BR-NL-19's rule is behind $s and no longer
	// claims cbc:TaxCurrencyCode.
	want := []string{"SI-UBL-2", "SI-UBL-2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("a Peppol invoice validated as NLCIUS reported %v; want %v — the empty-element rule outside "+
			"the gate and nothing else, because outside $si no other rule of the pattern matches anything", got, want)
	}
}

// TestNLCIUSEmptyElementFindingsAreEmptyElements is the FP=0 half, over the whole
// corpus and in the only sense this rule admits: every finding must name an element
// that really has no child element and no non-whitespace content.
//
// It also ratchets the volume. The rule has a `//*` context, so it is the one rule
// in this package whose finding count is a property of the corpus rather than of a
// handful of documents, and a change that stopped emitting it would otherwise leave
// every other NLCIUS assertion green.
func TestNLCIUSEmptyElementFindingsAreEmptyElements(t *testing.T) {
	skipWithoutCorpus(t)
	docs, reported := 0, 0
	perID := map[string]int{}
	worst, worstFile := 0, ""
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		docs++
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil {
			return nil
		}
		// The elements that genuinely qualify, counted independently of the rule.
		empty := map[string]int{}
		var walk func(*ciiNode)
		walk = func(n *ciiNode) {
			if len(n.children) == 0 && strings.TrimSpace(n.text) == "" {
				empty[n.name]++
			}
			for _, c := range n.children {
				walk(c)
			}
		}
		walk(parsed.root)

		here := 0
		for _, v := range nlciusEmptyElements(parsed, nil) {
			perID[v.Rule]++
			reported++
			here++
			name := strings.SplitN(v.Message, `"`, 3)[1]
			if empty[name] == 0 {
				t.Errorf("%s: %s names %q, which is not an element with no children and no non-whitespace "+
					"content — or names one more of them than the document holds", p, v.Rule, name)
			}
			empty[name]--
		}
		if here > worst {
			worst, worstFile = here, p
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	atLeast(t, "empty-element sweep corpus", docs, minCorpusDocuments)
	atLeast(t, "NLCIUS empty-element findings", reported, minNLCIUSEmptyElementFindings)
	atLeast(t, "NLCIUS SI-UBL-2 findings", perID["SI-UBL-2"], minNLCIUSEmptyElementsUBL)
	atLeast(t, "NLCIUS empty-element-check findings", perID["empty-element-check"], minNLCIUSEmptyElementsCII)
	t.Logf("NLCIUS empty elements: %d findings over %d documents (%v); the worst single document is %s with %d",
		reported, docs, perID, worstFile, worst)
}
