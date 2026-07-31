package formalis

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The governing principle, and the guards on it.
//
// # The principle
//
// A document the authority that governs it accepts must not draw a fatal finding
// from this package. Reporting *more* than the authority is a defect of the same
// kind as reporting the wrong rule: a caller who gates on "no fatal finding"
// then refuses an invoice the authority's own validator passes.
//
// It has been the source of four of this repository's confirmed findings. C29:
// seven XRechnung rules emitted fatal that KoSIT flags advisory, so German
// invoices Germany accepts were refused. C32: eight Peppol rules evaluated on a
// syntax OpenPEPPOL does not publish them for, and four existence rules tested
// for non-empty text where OpenPEPPOL's own fixtures write the element empty.
// C36: four national rule sets applied to CII when their authorities publish UBL
// only, 2,070 findings against documents those rule sets do not cover. C42: eight
// Serbian rules reported that ISO Schematron rule order makes unreachable in the
// artefact, 2,081 findings the Ministry's own validator cannot produce.
//
// Every one of those was invisible to the FP=0 corpus oracles as they stood,
// because those oracles ask "is a conforming document clean under *this* rule
// set" one rule set at a time, and each of these defects was a rule set answering
// for a document it does not govern, or answering more strictly than it publishes.
//
// # The guard
//
// TestAuthoritySamplesDrawNoFatalFinding states the principle once, over every
// authority in the tree that ships its own conformant sample set, with a floor on
// each population so a short fetch cannot make it vacuous. An authority added
// without such a set is visible by its absence from the table.
//
// It overlaps the per-corpus FP=0 tests deliberately — this is the invariant they
// are each an instance of — and it adds two things they do not have. It asserts
// the same documents are also clean through ValidateCIUS, the routing entry point
// this package tells callers to prefer and the door C24 and C44 came in through;
// and it reads FNFE's verdicts out of valitool's reports rather than assuming the
// vendored examples are conformant, so "the authority accepts it" is a fact taken
// from the authority.
//
// # What is not in the table, and why
//
// Three of this repository's corpora are conformant *against their own rule set*
// and not against the EN 16931 core, so "no fatal finding at all" is the wrong
// assertion for them and their per-corpus oracles correctly scope to their own
// identifiers instead:
//
//   - CIUS-PT's 20 instances are AT's syntax-model illustrations. Measured, all
//     20 report BR-CL-10: they write a party identifier scheme of "0001", which
//     is not in CEN's own ICD.gc — 486 values, starting at 0002. That is CEN's
//     code list reported faithfully on a document CEN would also reject.
//   - CIUS-RO's and UBL.BE's sample sets are asserted clean of BR-RO-* and
//     ubl-BE-* respectively, which is what their upstreams publish them as.
//   - NLCIUS's testsuite is a mixed set: 27 of its instances are deliberately
//     broken and must produce findings, which is a different oracle
//     (TestNLCIUSPerRuleFixtures) and a stronger one.
//
// KoSIT's and OpenPEPPOL's per-rule fixtures — the <?xmute mutator="identity"?>
// instructions and the 350 unit-test documents — say more than this test can,
// because they name a rule per document rather than judging the whole of it.
// They are read by xrechnung_rules_test.go and peppol_rules_test.go and are not
// duplicated here.

// authoritySampleSet is one authority's own conformant sample set: the documents
// it publishes as passing, the validator this package offers for that authority,
// and the floor on the population.
type authoritySampleSet struct {
	authority string
	// dir is walked for *.xml; only files whose root is an invoice or credit note
	// are judged, because several of these corpora ship Schematron and build files
	// beside the instances.
	dir string
	// validate is the authority's own entry point here.
	validate validator
	// atLeast is the population floor. See corpus_test.go on why every oracle
	// backed by a fetched corpus needs one.
	atLeast int
	// conformant, when set, decides per document whether the authority passes it.
	// Only Factur-X has this: FNFE ships valitool's verdict beside each example,
	// so the set is read from the authority rather than assumed. Everywhere else
	// the corpus *is* the authority's conforming sample set, which is what its
	// own upstream README says it is.
	conformant func(t *testing.T, path string, data []byte) bool
}

func authoritySampleSets() []authoritySampleSet {
	return []authoritySampleSet{{
		authority: "FNFE-MPE (Factur-X)",
		dir:       filepath.Join("testdata", "facturx", "examples"),
		validate:  validateAtDeclaredProfile,
		atLeast:   minFacturXReports,
		conformant: func(t *testing.T, path string, data []byte) bool {
			return fxValitoolPasses(t, path)
		},
	}, {
		authority: "KoSIT (XRechnung)",
		dir:       filepath.Join("testdata", "xrechnung", "testsuite", "src", "test"),
		validate:  ValidateXRechnung,
		atLeast:   minXRechnungInstances,
	}, {
		authority: "OpenPEPPOL (BIS Billing 3.0)",
		dir:       filepath.Join("testdata", "peppol", "repo", "rules", "examples"),
		validate:  ValidatePeppol,
		atLeast:   minPeppolExamples,
	}}
}

// validateAtDeclaredProfile is the Factur-X entry point in the terms the
// authority's own examples are published in: at the profile the document's BT-24
// declares. It is what reportFatal does and what TestValidateFacturXCorpus does.
func validateAtDeclaredProfile(ctx context.Context, data []byte) (Report, error) {
	if p, _, ok := fxDeclaredProfile(data); ok {
		return Validate(ctx, data, p)
	}
	return ValidateCIUS(ctx, data)
}

// fxValitoolPasses reports whether FNFE's own validator passes this example on
// business rules, read out of the *_fx_validation_report.xml beside it.
//
// A document with no report is not in the authority's conformant set — 8 of the
// 59 examples have none — and is not judged here.
func fxValitoolPasses(t *testing.T, doc string) bool {
	t.Helper()
	base := strings.TrimSuffix(filepath.Base(doc), ".xml")
	report := filepath.Join(filepath.Dir(filepath.Dir(doc)), "reports", base+"_fx_validation_report.xml")
	data, err := os.ReadFile(report)
	if err != nil {
		return false
	}
	var rep fxReport
	if err := xml.Unmarshal(data, &rep); err != nil {
		t.Fatalf("%s: %v", report, err)
	}
	if rep.ValidationDetailsXML.IsValidBusinessRules == "" {
		t.Fatalf("%s: no isValidBusinessRules; the report schema is not being read", report)
	}
	return rep.ValidationDetailsXML.IsValidBusinessRules == "true"
}

// cxiRoot matches a CII document root, which is the syntax Factur-X publishes a
// binding for.
var cxiRoot = regexp.MustCompile(`(?s)<([\w.]+:)?CrossIndustryInvoice[\s>]`)

var authorityIsInvoice = regexp.MustCompile(`(?s)<([\w.]+:)?(CrossIndustryInvoice|Invoice|CreditNote)[\s>]`)

// TestAuthoritySamplesDrawNoFatalFinding is the principle, asserted.
func TestAuthoritySamplesDrawNoFatalFinding(t *testing.T) {
	ctx := context.Background()
	ran := 0
	for _, set := range authoritySampleSets() {
		set := set
		t.Run(set.authority, func(t *testing.T) {
			if _, err := os.Stat(set.dir); err != nil {
				t.Skipf("%s samples not present (make cius-oracles / make facturx-examples)", set.authority)
			}
			var judged, clean, routed int
			err := filepath.Walk(set.dir, func(p string, fi os.FileInfo, e error) error {
				if e != nil || fi.IsDir() || !strings.HasSuffix(strings.ToLower(p), ".xml") {
					return nil
				}
				data, err := os.ReadFile(p)
				if err != nil {
					t.Errorf("%s: %v", p, err)
					return nil
				}
				if !authorityIsInvoice.Match(data) {
					return nil
				}
				if set.conformant != nil && !set.conformant(t, p, data) {
					return nil
				}
				judged++
				if v := mustReport(t, ctx, set.validate, data).Fatal(); len(v) != 0 {
					t.Errorf("%s: %s passes this document and this package reports %d fatal finding(s) (first: %s/%s: %s)",
						filepath.Base(p), set.authority, len(v), v[0].Source, v[0].Rule, v[0].Message)
				} else {
					clean++
				}
				// And again through the routing entry point, which arbitrates on
				// the document's own BT-24. A rule set reached by the wrong door
				// is C24 and C44, and neither moved a per-corpus FP=0 number.
				if v := mustReport(t, ctx, ValidateCIUS, data).Fatal(); len(v) != 0 {
					t.Errorf("%s: %s passes this document and ValidateCIUS reports %d fatal finding(s) (first: %s/%s: %s)",
						filepath.Base(p), set.authority, len(v), v[0].Source, v[0].Rule, v[0].Message)
				} else {
					routed++
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			// The directory can exist and hold no judgeable document — a clean
			// checkout keeps the download scripts under testdata and nothing else,
			// and the Factur-X entry needs the reports beside the examples. That is
			// the corpus-absent case rather than a truncated one.
			if judged == 0 {
				t.Skipf("%s samples not present (make cius-oracles / make facturx-examples)", set.authority)
			}
			atLeast(t, set.authority+" conformant samples", judged, set.atLeast)
			t.Logf("authority parity: %s — %d/%d of its own conformant samples draw no fatal finding, %d/%d through ValidateCIUS",
				set.authority, clean, judged, routed, judged)
			ran++
		})
	}
	if ran > 0 {
		t.Logf("authority parity: %d of %d authority sample sets present", ran, len(authoritySampleSets()))
	}
}

// TestFacturXSupersessionMatchesTheOmissionTable holds facturXSuperseded — the
// map facturXAuthorityParity consults — against the two tables it is derived
// from and against the artefact.
//
// Three things have to hold for an entry to be legitimate, and none of them is a
// property of the Go code alone:
//
//   - the profile's own Schematron must not publish the CEN identifier, or there
//     is nothing to supersede and this package would be dropping a finding the
//     authority makes;
//   - the profile's Schematron must publish the restatement, unflagged, or this
//     package would be deferring to a rule the authority does not make fatal;
//   - facturXCENOmissions must record the pair, so the coverage story and the
//     suppression agree about what replaced what.
func TestFacturXSupersessionMatchesTheOmissionTable(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	replaced := map[string]string{} // CEN identifier -> its replacement
	for _, o := range facturXCENOmissions {
		if o.replacedBy != "" {
			replaced[o.cen] = o.replacedBy
		}
	}
	if len(facturXSuperseded) != 1 {
		t.Errorf("facturXSuperseded covers %d profiles; only EXTENDED drops CEN identifiers", len(facturXSuperseded))
	}
	for profile, m := range facturXSuperseded {
		published := fxNamed(fxDecode(t, dir, profile))
		for cen, id := range m {
			if _, ok := published[cen]; ok {
				t.Errorf("profile %q publishes %s, so nothing may supersede it", string(profile), cen)
			}
			x, ok := published[id]
			if !ok {
				t.Errorf("profile %q does not publish %s, which facturXSuperseded puts in front of %s", string(profile), id, cen)
				continue
			}
			if got := x.a.severity(); got != SeverityFatal {
				t.Errorf("%s is %v in profile %q; only a fatal restatement may supersede a fatal CEN rule", id, got, string(profile))
			}
			switch want := replaced[cen]; {
			case want == "":
				t.Errorf("facturXCENOmissions does not record %s as replaced, and facturXSuperseded puts %s in front of it", cen, id)
			case want != id && !(cen == "BR-G-08" && id == "BR-FXEXT-G-08b"):
				t.Errorf("facturXCENOmissions records %s replaced by %s; facturXSuperseded uses %s", cen, want, id)
			}
		}
	}
	// The two restatements that must never supersede, with the artefact's reason:
	// their test is CEN's, character for character, so there is no document one
	// reports and the other does not.
	for _, rs := range facturXRestatementRules {
		if rs.id != "BR-FXEXT-BR-38" && rs.id != "BR-FXEXT-BR-44" && rs.profile != ProfileMinimum {
			continue
		}
		if facturXSuperseded[rs.profile][rs.cen] != "" {
			t.Errorf("%s supersedes %s and must not: %s", rs.id, rs.cen, supersessionReason(rs.id))
		}
	}
	var ids []string
	for _, m := range facturXSuperseded {
		for cen := range m {
			ids = append(ids, cen)
		}
	}
	sort.Strings(ids)
	t.Logf("authority parity: %d CEN identifiers yield to a Factur-X restatement at EXTENDED: %s", len(ids), strings.Join(ids, " "))
}

func supersessionReason(id string) string {
	if id == "BR-FXEXT-BR-38" || id == "BR-FXEXT-BR-44" {
		return "its context and test are CEN's character for character"
	}
	return "FACTUR-X_MINIMUM.sch publishes it beside CEN's rule rather than in place of it"
}

// TestFacturXAuthorityParityIsPerRule is the mechanism's firing verdict (C41),
// over all 24 restatements and both directions of each.
//
// A suppression pass is exactly the kind of code that can be present, reachable
// and wrong without any oracle noticing: suppress nothing and the package stays
// stricter than the authority, suppress everything and 21 fatal rules go quiet.
// So each row is exercised four ways — the CEN finding alone, the CEN finding
// beside its restatement, the same at a profile that does not supersede, and the
// same on a UBL document — against violation slices built here rather than
// against documents, so the assertion is about the pass and not about which
// fixture happens to trip which rule.
func TestFacturXAuthorityParityIsPerRule(t *testing.T) {
	cii := parseForParity(t, `<CrossIndustryInvoice/>`)
	ubl := parseForParity(t, `<Invoice/>`)
	rules := func(vs []Violation) []string {
		var out []string
		for _, v := range vs {
			out = append(out, string(v.Source)+"/"+v.Rule)
		}
		return out
	}
	for _, rs := range facturXRestatementRules {
		rs := rs
		t.Run(rs.id, func(t *testing.T) {
			cen := Violation{Source: SourceEN16931, Rule: rs.cen, Severity: SeverityFatal, Message: "x"}
			fx := Violation{Source: SourceFacturX, Rule: rs.id, Severity: SeverityFatal, Message: "y"}

			alone := facturXAuthorityParity(newRun(context.Background()), cii, rs.profile, []Violation{cen})
			if rs.weaker {
				if len(alone) != 0 {
					t.Errorf("%s is satisfied and %s still reports: %v", rs.id, rs.cen, rules(alone))
				}
			} else if len(alone) != 1 {
				t.Errorf("%s does not supersede %s, and %s was dropped: %v", rs.id, rs.cen, rs.cen, rules(alone))
			}

			// Both fire: both stand. This is the duplicate-reporting decision PR 33
			// argued, kept for every document that fails either way.
			both := facturXAuthorityParity(newRun(context.Background()), cii, rs.profile, []Violation{cen, fx})
			if len(both) != 2 {
				t.Errorf("%s and %s both fired and the pass kept %v", rs.cen, rs.id, rules(both))
			}

			// At a profile that publishes the CEN identifier, nothing is dropped.
			for _, p := range profiles {
				if p == rs.profile {
					continue
				}
				got := facturXAuthorityParity(newRun(context.Background()), cii, p, []Violation{cen})
				if len(got) != 1 && facturXSuperseded[p][rs.cen] == "" {
					t.Errorf("profile %q does not supersede %s and dropped it", string(p), rs.cen)
				}
			}

			// A UBL document is not governed by a rule set with no UBL binding.
			if got := facturXAuthorityParity(newRun(context.Background()), ubl, rs.profile, []Violation{cen}); len(got) != 1 {
				t.Errorf("%s was dropped from a UBL document, which Factur-X publishes no binding for", rs.cen)
			}

			// A stopped run may not turn a finding into silence: the restatement
			// that would have superseded it may never have been reached.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			stopped := newRun(ctx)
			if !stopped.stopped() {
				t.Fatal("a run on a cancelled context did not report stopped")
			}
			if got := facturXAuthorityParity(stopped, cii, rs.profile, []Violation{cen}); len(got) != 1 {
				t.Errorf("%s was dropped from a run that stopped before the restatements could be evaluated", rs.cen)
			}
		})
	}
}

func parseForParity(t *testing.T, doc string) *parsed {
	t.Helper()
	p, err := parseEN16931(newRun(context.Background()), []byte(doc))
	if err != nil {
		t.Fatalf("%v", err)
	}
	return p
}

// TestSupersededCENRulesNeverStandAloneInTheCorpus is the corpus half of the
// same property, and it is the one that would catch a restatement that stopped
// being evaluated: over every document in the tree, at EXTENDED, a superseded CEN
// identifier may only be reported beside the Factur-X rule that replaced it.
func TestSupersededCENRulesNeverStandAloneInTheCorpus(t *testing.T) {
	skipWithoutCorpus(t)
	superseded := facturXSuperseded[ProfileExtended]
	if len(superseded) == 0 {
		t.Fatal("facturXSuperseded is empty at EXTENDED; the pass cannot be doing anything")
	}
	ctx := context.Background()
	docs, cii, paired := 0, 0, 0
	err := filepath.Walk("testdata", func(p string, fi os.FileInfo, e error) error {
		if e != nil || fi.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rep, err := Validate(ctx, data, ProfileExtended)
		if err != nil {
			return nil
		}
		docs++
		// Factur-X publishes no UBL binding, so no restatement was evaluated for a
		// UBL document and CEN's rule is the only reading there is. That is the
		// root test facturXAuthorityParity makes, restated here so this sweep
		// asserts the property where it holds rather than everywhere.
		if !cxiRoot.Match(data) {
			return nil
		}
		cii++
		fired := map[string]bool{}
		for _, v := range rep.Violations {
			if v.Source == SourceFacturX {
				fired[v.Rule] = true
			}
		}
		for _, v := range rep.Fatal() {
			if v.Source != SourceEN16931 {
				continue
			}
			id, ok := superseded[v.Rule]
			if !ok {
				continue
			}
			if !fired[id] {
				t.Errorf("%s: %s is reported at EXTENDED and %s, which supersedes it, is not", p, v.Rule, id)
				continue
			}
			paired++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A clean checkout has a testdata directory — the download scripts, their
	// manifests and the six committed Factur-X documents are tracked — so
	// neither the presence of the directory nor the presence of documents in it
	// is the test for "the corpus is here". skipWithoutCorpus asks the fetch
	// stamps; finding some documents but not all is what atLeast reports,
	// because a green verdict over a fraction of the corpus is a claim about the
	// wrong population.
	atLeast(t, "corpus documents", docs, minCorpusDocuments)
	t.Logf("authority parity: over %d corpus documents forced to EXTENDED, %d of them CII, %d superseded CEN findings stand and every one is beside the Factur-X rule that replaced it",
		docs, cii, paired)
}

// fxParityCII builds a CII EXTENDED invoice whose second line is marked GROUP:
// a line FNFE's summation rules do not count and CEN's do.
//
// It is the shape every one of the summation divergences turns on, and it is
// deliberately not a sub-line structure. A GROUP line with no children is
// top-level by every reading, so it is an operand of CEN's sums under any
// interpretation of the roll-up and of FNFE's under none — which isolates the
// one difference being tested. tail is appended inside the settlement, before
// the monetary summation, for the fixtures that need a third-party charge.
func fxParityCII(category, rate, extraLine, tail string, basis, tax, grand, due string) string {
	rateEl := ""
	if rate != "" {
		rateEl = "<RateApplicablePercent>" + rate + "</RateApplicablePercent>"
	}
	lineTax := "<ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>" + category + "</CategoryCode>" + rateEl + "</ApplicableTradeTax>"
	line := func(id, amount, subtype string) string {
		st := ""
		if subtype != "" {
			st = "<LineStatusReasonCode>" + subtype + "</LineStatusReasonCode>"
		}
		return `<IncludedSupplyChainTradeLineItem>` +
			`<AssociatedDocumentLineDocument><LineID>` + id + `</LineID>` + st + `</AssociatedDocumentLineDocument>` +
			`<SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>` +
			`<SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>` + amount + `</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>` +
			`<SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>` +
			`<SpecifiedLineTradeSettlement>` + lineTax +
			`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>` + amount + `</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>` +
			`</SpecifiedLineTradeSettlement></IncludedSupplyChainTradeLineItem>`
	}
	return `<CrossIndustryInvoice>
  <ExchangedDocumentContext><GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended</ID></GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext>
  <ExchangedDocument><ID>INV-P</ID><TypeCode>380</TypeCode><IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime></ExchangedDocument>
  <SupplyChainTradeTransaction>
    ` + line("1", "100.00", "DETAIL") + extraLine + `
    <ApplicableHeaderTradeAgreement>
      <SellerTradeParty><Name>Seller Co</Name><PostalTradeAddress><CountryID>FR</CountryID></PostalTradeAddress><SpecifiedTaxRegistration><ID schemeID="VA">FR12345678</ID></SpecifiedTaxRegistration></SellerTradeParty>
      <BuyerTradeParty><Name>Buyer Co</Name><PostalTradeAddress><CountryID>FR</CountryID></PostalTradeAddress></BuyerTradeParty>
    </ApplicableHeaderTradeAgreement>
    <ApplicableHeaderTradeSettlement>
      <InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>
      <ApplicableTradeTax><TypeCode>VAT</TypeCode><CalculatedAmount>` + tax + `</CalculatedAmount><BasisAmount>` + basis + `</BasisAmount><CategoryCode>` + category + `</CategoryCode>` + rateEl + `<ExemptionReason>Reason</ExemptionReason></ApplicableTradeTax>` + tail + `
      <SpecifiedTradeSettlementHeaderMonetarySummation>
        <LineTotalAmount>100.00</LineTotalAmount>
        <TaxBasisTotalAmount>` + basis + `</TaxBasisTotalAmount>
        <TaxTotalAmount currencyID="EUR">` + tax + `</TaxTotalAmount>
        <GrandTotalAmount>` + grand + `</GrandTotalAmount>
        <DuePayableAmount>` + due + `</DuePayableAmount>
      </SpecifiedTradeSettlementHeaderMonetarySummation>
    </ApplicableHeaderTradeSettlement>
  </SupplyChainTradeTransaction>
</CrossIndustryInvoice>`
}

// fxParityGroupLine is the GROUP line CEN counts and FNFE does not, at a
// category and rate.
func fxParityGroupLine(category, rate string) string {
	rateEl := ""
	if rate != "" {
		rateEl = "<RateApplicablePercent>" + rate + "</RateApplicablePercent>"
	}
	return `<IncludedSupplyChainTradeLineItem>` +
		`<AssociatedDocumentLineDocument><LineID>2</LineID><LineStatusReasonCode>GROUP</LineStatusReasonCode></AssociatedDocumentLineDocument>` +
		`<SpecifiedTradeProduct><Name>Bundle</Name></SpecifiedTradeProduct>` +
		`<SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>50.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>` +
		`<SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>` +
		`<SpecifiedLineTradeSettlement><ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>` + category + `</CategoryCode>` + rateEl + `</ApplicableTradeTax>` +
		`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>50.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>` +
		`</SpecifiedLineTradeSettlement></IncludedSupplyChainTradeLineItem>`
}

// fxParityManyLines is 100 standard-rated lines of 1,00, the population that
// makes FNFE's 0,01 x operand-count tolerance wider than CEN's flat ±1 on
// BR-S-09. Below 100 operands FNFE is the stricter of the two.
func fxParityManyLines() string {
	var b strings.Builder
	for i := 2; i <= 100; i++ {
		b.WriteString(`<IncludedSupplyChainTradeLineItem>` +
			`<AssociatedDocumentLineDocument><LineID>` + itoa(i) + `</LineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode></AssociatedDocumentLineDocument>` +
			`<SpecifiedTradeProduct><Name>Widget</Name></SpecifiedTradeProduct>` +
			`<SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>0.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>` +
			`<SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>` +
			`<SpecifiedLineTradeSettlement><ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>` +
			`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>0.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>` +
			`</SpecifiedLineTradeSettlement></IncludedSupplyChainTradeLineItem>`)
	}
	return b.String()
}

func itoa(i int) string { return strconv.Itoa(i) }

// TestCENRulesYieldToFacturXOnDocumentsFNFEPasses is the end-to-end half of the
// parity pass, one document per divergence.
//
// Each row is a document CEN's rule reports and a Factur-X processor does not,
// and each is asserted three ways: ValidateEN16931 reports the CEN identifier,
// so the rule is live and the fixture really does break it; Validate at EXTENDED
// reports neither the CEN identifier nor the restatement, so the pass fired and
// FNFE is genuinely silent rather than merely absent; and the document is
// Conformant() at EXTENDED wherever the divergence is its only defect.
func TestCENRulesYieldToFacturXOnDocumentsFNFEPasses(t *testing.T) {
	ctx := context.Background()
	// The nine VAT categories, with a rate that satisfies each family's own rate
	// constraint. FNFE writes nine copies of one rule; these are nine copies of
	// one document.
	type catRow struct{ code, rate, restatement string }
	cats := []catRow{
		{"S", "20.00", "BR-FXEXT-S08b"},
		{"Z", "0.00", "BR-FXEXT-Z-08b"},
		{"E", "0.00", "BR-FXEXT-E-08b"},
		{"AE", "0.00", "BR-FXEXT-AE-08b"},
		{"K", "0.00", "BR-FXEXT-IC-08b"},
		{"G", "0.00", "BR-FXEXT-G-08b"},
		{"L", "8.00", "BR-FXEXT-AF-08b"},
		{"M", "8.00", "BR-FXEXT-AG-08b"},
		{"O", "", "BR-FXEXT-O-08b"},
	}
	type row struct {
		name        string
		cen         string
		restatement string
		doc         string
		why         string
	}
	rows := []row{{
		name: "BR-CO-12 logistics service charge folded into BT-108",
		cen:  "BR-CO-12", restatement: "BR-FXEXT-CO-12",
		doc: fxVATCII,
		why: "BT-108 counts a ram:SpecifiedLogisticsServiceCharge (BT-X-272) that CEN's rule has never heard of; " +
			"this is the shape X11_01_Kostenrechnung.xml and X19_01_Warenrechnung.xml are in",
	}, {
		name: "BR-CO-16 third-party charge folded into BT-115",
		cen:  "BR-CO-16", restatement: "BR-FXEXT-CO-16",
		doc: strings.Replace(
			fxVAT(`<DuePayableAmount>120.00</DuePayableAmount>`, `<DuePayableAmount>127.00</DuePayableAmount>`),
			`<SpecifiedTradeSettlementHeaderMonetarySummation>`,
			`<SpecifiedFinancialAdjustment><ActualAmount>7.00</ActualAmount></SpecifiedFinancialAdjustment><SpecifiedTradeSettlementHeaderMonetarySummation>`, 1),
		why: "C46: BT-115 includes Σ BT-179, which FNFE's BR-FXEXT-CO-16 adds to the expected amount and CEN's does not",
	}, {
		name: "BR-CO-11 inside FNFE's 0,01 x count tolerance",
		cen:  "BR-CO-11", restatement: "BR-FXEXT-CO-11",
		doc: fxVAT(`<AllowanceTotalAmount>15.00</AllowanceTotalAmount>`, `<AllowanceTotalAmount>15.01</AllowanceTotalAmount>`),
		why: "one cent over one allowance: outside CEN's half-cent equality and inside FNFE's 0,01 x 1",
	}, {
		name: "BR-CO-10 GROUP line CEN counts and FNFE does not",
		cen:  "BR-CO-10", restatement: "BR-FXEXT-CO-10",
		doc: fxParityCII("S", "20.00", fxParityGroupLine("S", "20.00"), "", "100.00", "20.00", "120.00", "120.00"),
		why: "CEN sums every ram:IncludedSupplyChainTradeLineItem; FNFE sums the DETAIL ones",
	}, {
		name: "BR-CO-13 inside FNFE's tolerance",
		cen:  "BR-CO-13", restatement: "BR-FXEXT-CO-13",
		doc: fxVAT(`<TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>`, `<TaxBasisTotalAmount>100.01</TaxBasisTotalAmount>`),
		why: "one cent on BT-109 against four operands: CEN's equality is exact, FNFE's tolerance is 0,04",
	}, {
		name: "BR-CO-15 inside FNFE's tolerance",
		cen:  "BR-CO-15", restatement: "BR-FXEXT-CO-15",
		doc: fxVAT(`<GrandTotalAmount>120.00</GrandTotalAmount>`, `<GrandTotalAmount>120.01</GrandTotalAmount>`),
		why: "the direction PR 33 recorded as absent: FACTUR-X_EXTENDED.sch gives BR-FXEXT-CO-15 a " +
			"0,01 x (DETAIL lines + allowances + charges + logistics) tolerance and CEN's rule is exact",
	}, {
		name: "BR-S-09 inside FNFE's tolerance",
		cen:  "BR-S-09", restatement: "BR-FXEXT-S-09b",
		doc: fxParityCII("S", "20.00", fxParityManyLines(), "", "100.00", "21.00", "121.00", "121.00"),
		why: "100 operands make FNFE's 0,01 x count exactly CEN's ±1, and CEN reports at the boundary",
	}}
	// The five line-existence rules, on one document: a GROUP line carrying none
	// of the terms CEN requires of every line.
	bare := fxVAT(`<ApplicableHeaderTradeAgreement>`,
		`<IncludedSupplyChainTradeLineItem><AssociatedDocumentLineDocument><LineID>2</LineID><LineStatusReasonCode>GROUP</LineStatusReasonCode></AssociatedDocumentLineDocument><SpecifiedTradeProduct><Name>Bundle</Name></SpecifiedTradeProduct></IncludedSupplyChainTradeLineItem><ApplicableHeaderTradeAgreement>`)
	for _, r := range []struct{ cen, id string }{
		{"BR-22", "BR-FXEXT-BR-22"}, {"BR-24", "BR-FXEXT-BR-24"},
		{"BR-26", "BR-FXEXT-BR-26"}, {"BR-CO-04", "BR-FXEXT-CO-04"},
	} {
		rows = append(rows, row{
			name: r.cen + " on a GROUP line", cen: r.cen, restatement: r.id, doc: bare,
			why: "FNFE's context is the BG-25 document group restricted to DETAIL or no subtype, so a GROUP line is not reached",
		})
	}
	// BR-23 needs its own document: this package reports BR-22 and BR-23 as a
	// chain — no quantity is BR-22 and nothing else — where CEN's binding writes
	// them as two independent asserts. So the GROUP line here carries a quantity
	// and no unit code.
	rows = append(rows, row{
		name: "BR-23 on a GROUP line", cen: "BR-23", restatement: "BR-FXEXT-BR-23",
		doc: strings.Replace(bare,
			`<SpecifiedTradeProduct><Name>Bundle</Name></SpecifiedTradeProduct></IncludedSupplyChainTradeLineItem>`,
			`<SpecifiedTradeProduct><Name>Bundle</Name></SpecifiedTradeProduct><SpecifiedLineTradeDelivery><BilledQuantity>1</BilledQuantity></SpecifiedLineTradeDelivery></IncludedSupplyChainTradeLineItem>`, 1),
		why: "FNFE's context is the BG-25 document group restricted to DETAIL or no subtype, so a GROUP line is not reached",
	})
	for _, c := range cats {
		rows = append(rows, row{
			name: "BR-" + vatCatSpecs[c.code].fam + "-08 GROUP line CEN counts and FNFE does not",
			cen:  "BR-" + vatCatSpecs[c.code].fam + "-08", restatement: c.restatement,
			doc: fxParityCII(c.code, c.rate, fxParityGroupLine(c.code, c.rate), "", "100.00", "0.00", "100.00", "100.00"),
			why: "the taxable-amount sum over CEN's line set against FNFE's DETAIL-only one",
		})
	}
	// Category S's own row needs a non-zero tax amount to stay consistent with
	// its rate, which the generic row above cannot carry.
	for i := range rows {
		if rows[i].cen == "BR-S-08" {
			rows[i].doc = fxParityCII("S", "20.00", fxParityGroupLine("S", "20.00"), "", "100.00", "20.00", "120.00", "120.00")
		}
	}

	seen := map[string]bool{}
	for _, tc := range rows {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if strings.HasPrefix(tc.doc, "MISSING ANCHOR") {
				t.Fatal(tc.doc)
			}
			seen[tc.cen] = true
			if !hasFacturXRule(findings(t, ctx, ValidateEN16931, []byte(tc.doc)), tc.cen) {
				t.Fatalf("%s does not report on this document under CEN's own binding, so the fixture does not "+
					"exercise the divergence (%s)", tc.cen, tc.why)
			}
			got := findings(t, ctx, withProfile(ProfileExtended), []byte(tc.doc))
			if hasFacturXRule(got, tc.restatement) {
				t.Errorf("%s reports on this document, so FNFE is not silent here and it is not a parity case", tc.restatement)
			}
			if hasFacturXRule(got, tc.cen) {
				t.Errorf("%s still reports at EXTENDED on a document Factur-X accepts: %s", tc.cen, tc.why)
			}
		})
	}
	// Every superseded identifier needs a document, or the pass has rows nothing
	// exercises end to end.
	for cen := range facturXSuperseded[ProfileExtended] {
		if !seen[cen] {
			t.Errorf("%s is superseded at EXTENDED and no fixture here shows a document where that matters", cen)
		}
	}
	t.Logf("authority parity: %d of the %d superseded CEN identifiers have a document CEN reports and Factur-X accepts",
		len(seen), len(facturXSuperseded[ProfileExtended]))
}

// TestAllowanceAndChargeTotalsAreCheckedAtEveryProfile is C45's first gate,
// asserted gone.
//
// BR-CO-11 and BR-CO-12 were skipped outright at ProfileExtended, so an EXTENDED
// document's BT-107 and BT-108 were checked against its BG-20/21 entries by
// nothing. The rules do not move with the profile any more, and this is the
// firing verdict on that: a document whose totals no entry accounts for reports
// both, at all five tiers.
//
// It is the document TestProfilesThatDifferStillDiffer used to carry as a
// profile *difference*, moved here because it stopped being one.
func TestAllowanceAndChargeTotalsAreCheckedAtEveryProfile(t *testing.T) {
	unitemized := strings.Replace(validCII,
		"<TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>",
		"<AllowanceTotalAmount>10.00</AllowanceTotalAmount><ChargeTotalAmount>5.00</ChargeTotalAmount><TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>", 1)
	if unitemized == validCII {
		t.Fatal("the mutation did not apply; the document is unchanged")
	}
	for _, rule := range []string{"BR-CO-11", "BR-CO-12"} {
		for _, p := range profiles {
			got := findings(t, context.Background(), withProfile(p), []byte(unitemized))
			if !hasFacturXRule(got, rule) {
				t.Errorf("profile %q: %s is not reported for a total no BG-20/21 entry accounts for", string(p), rule)
			}
			// And FNFE's restatement reports it too at EXTENDED, which is why the
			// parity pass leaves CEN's finding standing here.
			if p == ProfileExtended {
				id := facturXSuperseded[ProfileExtended][rule]
				if !hasFacturXRule(got, id) {
					t.Errorf("%s did not report at EXTENDED; CEN's %s should not be standing alone", id, rule)
				}
			}
		}
	}
}

// TestVATTaxableSumsAreCheckedOnSubLineDocuments is C45's second gate, asserted
// gone, in both directions.
//
// BR-{fam}-08 was skipped for any document carrying a sub-line structure, so an
// EXTENDED invoice with sub-lines had no VAT taxable-amount check at all. The
// firing half is a sub-line document whose BT-116 is wrong; the silent half is
// the same document with BT-116 right, which is what the gate was protecting.
func TestVATTaxableSumsAreCheckedOnSubLineDocuments(t *testing.T) {
	// A parent line and a sub-line that names it. CEN sums both — they are
	// siblings in CII — so the taxable amount is 140,00.
	sub := `<IncludedSupplyChainTradeLineItem>` +
		`<AssociatedDocumentLineDocument><LineID>1.1</LineID><ParentLineID>1</ParentLineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode></AssociatedDocumentLineDocument>` +
		`<SpecifiedTradeProduct><Name>Part</Name></SpecifiedTradeProduct>` +
		`<SpecifiedLineTradeAgreement><NetPriceProductTradePrice><ChargeAmount>40.00</ChargeAmount></NetPriceProductTradePrice></SpecifiedLineTradeAgreement>` +
		`<SpecifiedLineTradeDelivery><BilledQuantity unitCode="C62">1</BilledQuantity></SpecifiedLineTradeDelivery>` +
		`<SpecifiedLineTradeSettlement><ApplicableTradeTax><TypeCode>VAT</TypeCode><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>` +
		`<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>40.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>` +
		`</SpecifiedLineTradeSettlement></IncludedSupplyChainTradeLineItem>`
	ctx := context.Background()

	wrong := fxParityCII("S", "20.00", sub, "", "130.00", "26.00", "156.00", "156.00")
	if !hasFacturXRule(findings(t, ctx, ValidateEN16931, []byte(wrong)), "BR-S-08") {
		t.Error("BR-S-08 did not report on a sub-line document whose BT-116 is 130,00 against a line sum of 140,00; " +
			"the rule is inert on the shape C45 named")
	}
	right := fxParityCII("S", "20.00", sub, "", "140.00", "28.00", "168.00", "168.00")
	if hasFacturXRule(findings(t, ctx, ValidateEN16931, []byte(right)), "BR-S-08") {
		t.Error("BR-S-08 reported on a sub-line document whose BT-116 is the sum of its lines")
	}
	// And the document the old gate was protecting: a producer whose GROUP line
	// rolls its children up, which is how FNFE's own X02/X17/X18/X20 examples are
	// written. CEN counts the amount twice and reports; a Factur-X processor does
	// not, and at EXTENDED neither does this package.
	rollup := strings.Replace(
		fxParityCII("S", "20.00",
			strings.Replace(sub, `<LineTotalAmount>40.00</LineTotalAmount>`, `<LineTotalAmount>100.00</LineTotalAmount>`, 1),
			"", "100.00", "20.00", "120.00", "120.00"),
		`<LineID>1</LineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode>`,
		`<LineID>1</LineID><LineStatusReasonCode>GROUP</LineStatusReasonCode>`, 1)
	if !hasFacturXRule(findings(t, ctx, ValidateEN16931, []byte(rollup)), "BR-S-08") {
		t.Error("CEN's BR-S-08 is silent on a rolled-up sub-line document; the fixture does not exercise the divergence")
	}
	if got := findings(t, ctx, withProfile(ProfileExtended), []byte(rollup)); hasFacturXRule(got, "BR-S-08") {
		t.Error("BR-S-08 reports at EXTENDED on a rolled-up sub-line document, which every Factur-X processor accepts")
	}
}
