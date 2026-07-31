package formalis

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The six committed lean-tier invoices, recorded as expected failures.
//
// # Whose documents these are
//
// All six were published by the authorities themselves or by an implementer the
// authority's own corpus ships: three by FNFE-MPE as the official Factur-X
// examples, two by intarsys in the ZUGFeRD reference corpus, one by
// mustangproject. Twenty-five fatal findings across four of them, and **every one
// of the twenty-five is a defect in the published sample, not in this package**.
// FNFE's sample invoices depart from FNFE's own published data model at the
// leanest tiers, which is exactly why they are worth keeping: they are the
// evidence for #61, and editing them to make the build green would turn evidence
// into fixture and break the byte-identical provenance the directory's README
// rests on. The owner's decision is to keep them unmodified and say why.
//
// So this is not a corpus that happens to be red. It is an expected-failure
// table: each document carries the tier it declares, the findings it draws with
// the node each one fires at, and a reason a reader can check — and the reasons
// are checked, by TestFacturXLeanTierExpectedFailuresHaveTheStatedCause, against
// the documents and against the artefact rather than left as comments that rot.
//
// # The three divergences, and the design behind them
//
// **The currency is stated once.** Nineteen of the twenty-five are `report
// @currencyID` on an amount. The attribute is not optional-and-omitted at these
// tiers and it is not merely unusual: the element table excludes it, because an
// invoice is denominated once — BT-5 ram:InvoiceCurrencyCode — and a per-amount
// restatement adds nothing while letting one document carry two answers. The
// single exception is ram:TaxTotalAmount, where FNFE *constrains* @currencyID
// instead of forbidding it, keyed on whether it names BT-5 or BT-6
// ram:TaxCurrencyCode: that is the one place two amounts in two currencies
// legitimately coexist, BT-110 and BT-111. CEN reached the same design
// independently, carving ram:TaxTotalAmount out of CII-DT-031 by name — two
// authorities, arrived at separately.
// TestFacturXForbidsRestatingTheCurrencyOnEveryAmountButTaxTotalAmount reads that
// design back out of all five profile tables, and every one of the six documents
// here carries @currencyID on its ram:TaxTotalAmount and draws nothing for it.
//
// **An element the tier does not have.** Five findings are `report true()`: the
// buyer's ram:PostalTradeAddress and ram:SpecifiedTaxRegistration at MINIMUM, a
// head-only tier whose element table has no buyer address and no buyer VAT
// registration. Presence is the whole of the offence, which is what XPath says
// and what FNFE's own processor does.
//
// **An identifier its own code list rejects.** One finding, FX-DM-BASIC-0018, is
// the code-database lookup on BT-24. FACTUR-X_BASIC_codedb.xml enumerates exactly
// two permitted values and Avoir_FR_type381_BASIC declares neither.
//
// # Why they are outside TestAuthoritySamplesDrawNoFatalFinding
//
// That guard's principle is that a document *the authority accepts* must not draw
// a fatal finding here, and the operative word is accepts: for the Factur-X entry
// membership is decided not by the directory a document sits in but by FNFE's own
// valitool verdict, read out of the *_fx_validation_report.xml beside each
// example. Eight of the 59 examples have no report and are already not judged
// there for that reason.
//
// These six have no such verdict either. ZUGFeRD/corpus files them under
// `ZUGFeRDv2/correct/`, but that is a third party's classification of a PDF
// container, not FNFE saying the invoice inside passes FNFE's business rules —
// and where FNFE's data model and FNFE's sample disagree, as they do on all three
// counts above, the artefact is the authority and the sample is the thing being
// judged. They are outside that population for the same stated reason the
// unreported examples are, and they are not thereby unjudged: this file judges
// them harder than an FP=0 sweep could, because it fixes what fires and where in
// both directions.
//
// # What this table costs
//
// If one of these findings were ever shown to be this package's error rather than
// the sample's, the table would go on asserting it. The protection is not the
// list: it is that every row is tied to the artefact — the assertion's shape is
// looked up in the generated data-model table by key, and that table is re-derived
// from the vendored Schematron and compared in both directions by
// facturx_datamodel_test.go — and to the document, by decoding it independently of
// the parser that produced the finding.

// One observation, recorded and deliberately not acted on.
//
// MINIMUM's ram:TaxTotalAmount rules are selected by which currency @currencyID
// names, and one of the three variants keys on `../../ram:TaxCurrencyCode` — BT-6,
// the accounting currency — at a tier whose whole purpose is a reduced head-only
// summary. MINIMUM is the only one of the five profiles that references BT-6
// without defining it: the other four give ram:TaxCurrencyCode a rule of its own
// constraining its value against a code list, and MINIMUM gives it none, so a
// MINIMUM document carrying one is neither permitted nor forbidden nor
// value-checked. Whether the tier genuinely admits a second currency or the
// predicate is copy-paste from the richer profiles is unresolved here.
//
// No document decides it. Of the 65 Factur-X documents in this tree, exactly one
// carries a BT-6 — X07_01_Fremdwaehrung.xml, the two-currency example, GBP invoice
// and EUR tax — and it is EXTENDED. Nothing at MINIMUM exercises the variant in
// either direction.
//
// TestMinimumKeysARuleOnACurrencyItsOwnTierNeverDefines pins the asymmetry, so
// that an FNFE revision resolving it either way arrives as a failure pointing back
// at this note rather than as silence.

// fxCause is the kind of thing a document does that makes an assertion report. It
// is a small closed set rather than a free-text label because each kind carries a
// specific obligation on the checker below, and because it is matched against the
// generated data-model table: an expectation whose stated cause is not the shape
// the artefact actually publishes for that rule is a failure, which is what keeps
// the prose from drifting away from the rule it describes.
type fxCause uint8

const (
	// fxCauseCurrencyRestated: the element carries @currencyID and the tier's
	// rule for it is `report @currencyID`. The checker additionally requires the
	// attribute to restate the document's own BT-5, so "restates a currency the
	// document has already stated" is measured rather than assumed.
	fxCauseCurrencyRestated fxCause = iota
	// fxCauseElementNotInTier: the element exists and the tier's rule for it is
	// `report true()` — the element table does not carry it, so presence is the
	// whole of the offence.
	fxCauseElementNotInTier
	// fxCauseValueOutsideCodeList: the element's value is not in the code list
	// FNFE's code database binds to it.
	fxCauseValueOutsideCodeList
)

// fxExpectedFinding is one expected divergence: which rule reports, at which
// nodes, why the document draws it, and which of the three causes it is.
//
// at names every node the rule fires at, one entry per firing and repeats spelled
// out, because a count is the weaker half of the fact: thirteen findings that
// became thirteen *different* findings is the movement most worth catching and
// the one a total cannot see. The paths are the ones the finding's own message
// carries, so the expectation and the report are anchored to the same node.
type fxExpectedFinding struct {
	rule  string
	cause fxCause
	at    []string
	// why names what in this document causes it and what the rule says. It is
	// prose for a reader; the machine-checkable part of it is cause plus at.
	why string
	// spelledAs, for a code-list cause only, is the permitted value the
	// document's own differs from only in how it is written. The checker requires
	// it to be in the code list and to reduce to the same tokens as the
	// document's value, which is how "the right profile, spelled a way the code
	// list does not hold" stops being an assertion and becomes a measurement.
	spelledAs string
}

// fxLeanTierSample is one committed document as an expected failure: who
// published it, the tier its own BT-24 declares, the divergence in prose, and the
// findings that divergence produces.
//
// A document with no expectations is not an omission. Two of the six draw nothing
// at all, and saying so is a claim in its own right: they are the same tiers and
// the same rules, written the way the model asks.
type fxLeanTierSample struct {
	file      string
	profile   Profile
	publisher string
	// divergence is the paragraph a reader checks the findings against: what this
	// document does, whose document it is, and what its authority's data model
	// says about it.
	divergence string
	expect     []fxExpectedFinding
}

// Context paths, as the findings report them: local names from the document
// element, because that is the syntax-neutral spelling parseCII keys on and
// TestFacturXDataModelNamesAreUnambiguous shows nothing is lost by it.
const (
	fxAtDoc     = "/CrossIndustryInvoice"
	fxAtSpecID  = fxAtDoc + "/ExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter/ID"
	fxAtTx      = fxAtDoc + "/SupplyChainTradeTransaction"
	fxAtAgree   = fxAtTx + "/ApplicableHeaderTradeAgreement"
	fxAtSettle  = fxAtTx + "/ApplicableHeaderTradeSettlement"
	fxAtSummary = fxAtSettle + "/SpecifiedTradeSettlementHeaderMonetarySummation"
	fxAtLine    = fxAtTx + "/IncludedSupplyChainTradeLineItem"
)

// fxLeanTierSamples is the expected-failure table.
//
// Regenerate a row by reading the failure message and then re-deriving it from
// the profile Schematron, never by pasting whatever the code now produces: every
// line is a claim about FNFE's artefact and about FNFE's document, and both are
// checked below.
var fxLeanTierSamples = []fxLeanTierSample{{
	file:      "fnfe_BASIC.xml",
	profile:   ProfileBasic,
	publisher: "FNFE-MPE, Avoir_FR_type381_BASIC",
	divergence: "FNFE's own BASIC credit note, and the worst-diverging of the six. It denominates " +
		"itself once as BT-5 EUR and then restates EUR on thirteen further amounts — every amount " +
		"in the document except ram:TaxTotalAmount, which is the one BASIC allows to name a " +
		"currency. BASIC's element table excludes the attribute from all thirteen. It also " +
		"declares a BT-24 that FNFE's own FACTUR-X_BASIC_codedb.xml does not enumerate. Fourteen " +
		"findings, none of them this package's.",
	expect: []fxExpectedFinding{{
		rule:  "FX-DM-BASIC-0018",
		cause: fxCauseValueOutsideCodeList,
		at:    []string{fxAtSpecID},
		why: "BT-24 reads \"urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic\". FNFE's code " +
			"database enumerates exactly two values for this element, one per brand, and both write " +
			"the separators as \"#\" and repeat the urn: scheme before the profile stem. The sample " +
			"names the right profile and spells the identifier a way its own authority's code list " +
			"does not hold — which is also why routing still reads it as BASIC: " +
			"facturXProfileFromSpecID matches the brand stem as a substring, while the code-list " +
			"assertion is an exact lookup.",
		spelledAs: "urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic",
	}, {
		rule:  "FX-DM-BASIC-0107",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSettle + "/ApplicableTradeTax/BasisAmount", fxAtSettle + "/ApplicableTradeTax/BasisAmount"},
		why:   "Both VAT breakdown groups write currencyID=\"EUR\" on BT-116, the taxable amount.",
	}, {
		rule:  "FX-DM-BASIC-0108",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSettle + "/ApplicableTradeTax/CalculatedAmount", fxAtSettle + "/ApplicableTradeTax/CalculatedAmount"},
		why:   "Both VAT breakdown groups write it on BT-117, the tax amount, as well.",
	}, {
		rule:  "FX-DM-BASIC-0182",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/DuePayableAmount"},
		why:   "BT-115, the amount due for payment, restates EUR.",
	}, {
		rule:  "FX-DM-BASIC-0183",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/GrandTotalAmount"},
		why:   "BT-112, the total with VAT, restates EUR.",
	}, {
		rule:  "FX-DM-BASIC-0184",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/LineTotalAmount"},
		why:   "BT-106, the sum of line net amounts, restates EUR.",
	}, {
		rule:  "FX-DM-BASIC-0185",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/TaxBasisTotalAmount"},
		why:   "BT-109, the total without VAT, restates EUR.",
	}, {
		rule:  "FX-DM-BASIC-0189",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/TotalPrepaidAmount"},
		why:   "BT-113, the amount already paid, restates EUR — on a value of 0.00.",
	}, {
		rule:  "FX-DM-BASIC-0224",
		cause: fxCauseCurrencyRestated,
		at: []string{
			fxAtLine + "/SpecifiedLineTradeAgreement/NetPriceProductTradePrice/ChargeAmount",
			fxAtLine + "/SpecifiedLineTradeAgreement/NetPriceProductTradePrice/ChargeAmount",
		},
		why: "Both invoice lines write it on BT-146, the item net price.",
	}, {
		rule:  "FX-DM-BASIC-0259",
		cause: fxCauseCurrencyRestated,
		at: []string{
			fxAtLine + "/SpecifiedLineTradeSettlement/SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount",
			fxAtLine + "/SpecifiedLineTradeSettlement/SpecifiedTradeSettlementLineMonetarySummation/LineTotalAmount",
		},
		why: "Both invoice lines write it on BT-131, the line net amount, too.",
	}},
}, {
	file:      "fnfe_MINIMUM.xml",
	profile:   ProfileMinimum,
	publisher: "FNFE-MPE, Facture_FR_MINIMUM",
	divergence: "FNFE's own MINIMUM invoice. MINIMUM is the head-only tier — no lines, no VAT " +
		"breakdown, and a buyer reduced to a name and a legal registration — and this sample gives " +
		"the buyer a postal address and a VAT registration, neither of which is in MINIMUM's " +
		"element table. It then restates its own EUR on three of the four amounts in the summary. " +
		"Five findings, all of them the sample departing from FNFE's model.",
	expect: []fxExpectedFinding{{
		rule:  "FX-DM-MINIMUM-0019",
		cause: fxCauseElementNotInTier,
		at:    []string{fxAtAgree + "/BuyerTradeParty/PostalTradeAddress"},
		why: "The buyer carries a ram:PostalTradeAddress (a French country code). MINIMUM has no " +
			"buyer address at all — BT-50..BT-55 are not part of the tier — so FNFE's binding gives " +
			"the element a rule whose entire body is `report true()`: at MINIMUM, having one is the " +
			"finding, and what is inside it does not matter.",
	}, {
		rule:  "FX-DM-MINIMUM-0022",
		cause: fxCauseElementNotInTier,
		at:    []string{fxAtAgree + "/BuyerTradeParty/SpecifiedTaxRegistration"},
		why: "The same shape one element along: the buyer states a VAT identifier (BT-48). MINIMUM " +
			"carries the *seller's* tax registrations and not the buyer's, so this too is `report " +
			"true()`.",
	}, {
		rule:  "FX-DM-MINIMUM-0043",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/DuePayableAmount"},
		why:   "BT-115 restates the EUR the document has already declared as BT-5.",
	}, {
		rule:  "FX-DM-MINIMUM-0044",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/GrandTotalAmount"},
		why:   "BT-112 restates it.",
	}, {
		rule:  "FX-DM-MINIMUM-0045",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/TaxBasisTotalAmount"},
		why: "BT-109 restates it. The fourth amount in the same group, ram:TaxTotalAmount, carries " +
			"currencyID=\"EUR\" as well and draws nothing — it is the one element the tier allows to " +
			"name a currency, and this document is its own demonstration that the rule set " +
			"distinguishes the two.",
	}},
}, {
	file:      "fnfe_MINIMUM_UE.xml",
	profile:   ProfileMinimum,
	publisher: "FNFE-MPE, Facture_UE_MINIMUM",
	divergence: "The intra-EU variant of the same FNFE MINIMUM sample — a Spanish buyer, a Spanish " +
		"VAT identifier, zero VAT — and it diverges in exactly the same five ways, which is what " +
		"makes it evidence rather than a one-off: the defect is in how FNFE writes a MINIMUM " +
		"sample, not in one file.",
	expect: []fxExpectedFinding{{
		rule:  "FX-DM-MINIMUM-0019",
		cause: fxCauseElementNotInTier,
		at:    []string{fxAtAgree + "/BuyerTradeParty/PostalTradeAddress"},
		why:   "The buyer carries a ram:PostalTradeAddress (here a Spanish country code), which MINIMUM does not have.",
	}, {
		rule:  "FX-DM-MINIMUM-0022",
		cause: fxCauseElementNotInTier,
		at:    []string{fxAtAgree + "/BuyerTradeParty/SpecifiedTaxRegistration"},
		why:   "The buyer states a Spanish VAT identifier, which MINIMUM does not carry for the buyer.",
	}, {
		rule:  "FX-DM-MINIMUM-0043",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/DuePayableAmount"},
		why:   "BT-115 restates the document's own EUR.",
	}, {
		rule:  "FX-DM-MINIMUM-0044",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/GrandTotalAmount"},
		why:   "BT-112 restates it.",
	}, {
		rule:  "FX-DM-MINIMUM-0045",
		cause: fxCauseCurrencyRestated,
		at:    []string{fxAtSummary + "/TaxBasisTotalAmount"},
		why:   "BT-109 restates it; ram:TaxTotalAmount carries the attribute too and is not reported.",
	}},
}, {
	file:      "intarsys_BASIC.xml",
	profile:   ProfileBasic,
	publisher: "intarsys, zugferd_2p0_BASIC_Einfach",
	divergence: "Draws nothing, and both halves of that are worth having. It is the same BASIC tier " +
		"as fnfe_BASIC.xml with the identifier written the way the code database enumerates it — " +
		"\"urn:cen.eu:en16931:2017#compliant#urn:zugferd.de:2p0:basic\", the German brand of the " +
		"pair — so the two documents between them show FX-DM-BASIC-0018 discriminating rather than " +
		"accusing. And its only @currencyID sits on ram:TaxTotalAmount, which is the model's own " +
		"answer to the question fnfe_BASIC.xml gets wrong thirteen times.",
}, {
	file:      "intarsys_MINIMUM.xml",
	profile:   ProfileMinimum,
	publisher: "intarsys, zugferd_2p0_MINIMUM",
	divergence: "A third producer, an independent implementation, and the same buyer-address " +
		"divergence FNFE's two MINIMUM samples carry — so that finding is not one house's habit. " +
		"It gets the currency right where FNFE's samples do not: every amount is bare except " +
		"ram:TaxTotalAmount. This is also the document that found the routing defect fixed in the " +
		"commit before this one; with a ZUGFeRD-branded BT-24 it reached CEN's CII binding instead " +
		"of Factur-X's and was accused of BR-16 and BR-CO-18.",
	expect: []fxExpectedFinding{{
		rule:  "FX-DM-MINIMUM-0019",
		cause: fxCauseElementNotInTier,
		at:    []string{fxAtAgree + "/BuyerTradeParty/PostalTradeAddress"},
		why: "The buyer carries a full German ram:PostalTradeAddress — postcode, street, city, " +
			"country. MINIMUM has no buyer address, so the element's presence is the finding.",
	}},
}, {
	file:      "mustang_BASICWL_avoir.xml",
	profile:   ProfileBasicWL,
	publisher: "mustangproject, validAvoir_FR_type380_BASICWL",
	divergence: "Draws nothing. It is the tier's counterexample to fnfe_BASIC.xml: a credit note " +
		"with a full header settlement whose single @currencyID is on ram:TaxTotalAmount, which is " +
		"what BASIC WL's element table asks for.",
}}

// TestFacturXLeanTierSamplesDrawExactlyTheseFindings is the expected-failure
// ratchet, in both directions and at node granularity.
//
// A finding that appears is as interesting as one that vanishes. The vanishing
// case is the more dangerous of the two: it would mean this package stopped
// evaluating a rule that is still in the table, still reachable and now inert,
// which is C41 — the failure mode that "every identifier has an implementation"
// and "every context is reached" both pass. Comparing (rule, node) pairs rather
// than a rule list or a count is what makes fourteen findings becoming fourteen
// different findings red.
//
// It needs no corpus — the documents are tracked — so it runs in CI's corpus-less
// job, which is where a lean-tier rule set that stopped firing would otherwise be
// invisible: every other Factur-X oracle here skips without the vendored examples.
func TestFacturXLeanTierSamplesDrawExactlyTheseFindings(t *testing.T) {
	ctx := context.Background()

	// The directory listing and the table are held to each other first, so a
	// document added or removed is a red build rather than a silently smaller
	// population.
	onDisk, err := filepath.Glob(filepath.Join(committedCorpusDir, "*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range onDisk {
		names = append(names, filepath.Base(p))
	}
	sort.Strings(names)
	var want []string
	for _, s := range fxLeanTierSamples {
		want = append(want, s.file)
	}
	sort.Strings(want)
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Fatalf("%s holds %v and this test records %v. These documents are tracked, so the two cannot "+
			"drift apart by a fetch; add the new document to fxLeanTierSamples with its findings and its "+
			"reasons derived from the artefact, or remove the record with the file", committedCorpusDir, names, want)
	}

	byTier := map[Profile]struct{ docs, findings int }{}
	for _, s := range fxLeanTierSamples {
		p := filepath.Join(committedCorpusDir, s.file)
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}

		// The tier the document's own BT-24 declares, which is how every other
		// Factur-X oracle here validates one, and the door C44 came in through:
		// a document read as some other format is judged by some other binding.
		prof, src, ok := fxDeclaredProfile(data)
		if !ok || src != SourceFacturX {
			t.Errorf("%s: Detect reads this as %s and not as Factur-X, so it would be judged by another "+
				"authority's binding; it declares a Factur-X profile identifier", s.file, src)
			continue
		}
		if prof != s.profile {
			t.Errorf("%s: declares the %q tier and this test records %q", s.file, string(prof), string(s.profile))
			continue
		}

		// Two of the six draw nothing, and for those "no finding" and "the tier's
		// rule set never ran" produce the same empty list. Coverage(SourceFacturX)
		// is what separates them: a run that reached this rule set names its gaps,
		// and one that fell through to CEN's binding does not.
		r, verr := validateAtDeclaredProfile(ctx, data)
		if verr != nil {
			t.Fatalf("%s: %v", s.file, verr)
		}
		named := false
		for _, g := range r.NotEvaluated {
			if strings.Contains(g.Rules, "BR-FXEXT") || strings.Contains(g.Rules, "profile data model") {
				named = true
			}
		}
		if !named {
			t.Errorf("%s: the report names no Factur-X coverage, so the %q rule set did not run on it and a "+
				"finding list of length %d says nothing", s.file, string(s.profile), len(r.Fatal()))
		}

		want := s.expectedPairs()
		got := fxFatalPairs(t, ctx, validateAtDeclaredProfile, data, s.file)
		if strings.Join(got, "\n  ") != strings.Join(want, "\n  ") {
			t.Errorf("%s (%s, %s) draws\n  %s\nand this table records\n  %s\n"+
				"A finding that appeared is as interesting as one that vanished, and neither is a number "+
				"to update on sight: a rule that stopped firing is a rule this package may have stopped "+
				"evaluating. Derive the change from the profile Schematron, then write the reason.",
				s.file, string(s.profile), s.publisher, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}

		// And again through ValidateCIUS, which is the entry point this package
		// tells callers to prefer and which arbitrates on the same BT-24. The
		// two doors disagreeing is C24 and C44; on these documents it was also
		// live, because a ZUGFeRD-branded identifier reached neither Factur-X
		// rule set until specIDRules learnt the second brand.
		if routed := fxFatalPairs(t, ctx, ValidateCIUS, data, s.file); strings.Join(routed, " ") != strings.Join(got, " ") {
			t.Errorf("%s: ValidateCIUS reports %v and Validate at the declared %q tier reports %v; the "+
				"routing entry point must reach the same rule set the declared profile does",
				s.file, routed, string(s.profile), got)
		}

		e := byTier[s.profile]
		e.docs++
		e.findings += len(want)
		byTier[s.profile] = e
	}

	// The per-tier totals, which are what #61 asks to be visible. They are
	// derived from the table above rather than asserted separately — two records
	// of the same fact drift — and logged so the tiers this material exists to
	// cover are readable at a glance.
	var lines []string
	total := 0
	for _, p := range profiles {
		e := byTier[p]
		if e.docs == 0 {
			continue
		}
		total += e.findings
		lines = append(lines, fmt.Sprintf("%s: %d document(s), %d expected fatal finding(s)", string(p), e.docs, e.findings))
	}
	t.Logf("Factur-X lean tiers, committed corpus — %s; %d expected failures in all, every one a defect in "+
		"the published sample rather than in this package", strings.Join(lines, "; "), total)
}

// expectedPairs is the table's claim about one document as sorted "rule at node"
// lines, repeats included, which is the same shape fxFatalPairs renders a report
// into.
func (s fxLeanTierSample) expectedPairs() []string {
	out := []string{}
	for _, e := range s.expect {
		for _, at := range e.at {
			out = append(out, e.rule+" at "+at)
		}
	}
	sort.Strings(out)
	return out
}

// fxFatalPairs is one document's fatal findings as sorted "rule at node" lines.
//
// Sorted because the order findings arrive in is an implementation detail of the
// walk and not a property this test is about; repeats kept because two
// @currencyID attributes on two amounts is a different fact from one; the node
// included because a rule that moved to a different element is a change this test
// exists to see.
func fxFatalPairs(t *testing.T, ctx context.Context, v validator, data []byte, name string) []string {
	t.Helper()
	r, err := v(ctx, data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := []string{}
	for _, f := range r.Fatal() {
		out = append(out, f.Rule+" at "+fxFindingNode(t, name, f))
	}
	sort.Strings(out)
	return out
}

// fxFindingNode is the context path a data-model finding reports, taken out of
// the message facturx_datamodel.go builds: "<message> (<test> at <path>)".
//
// It fails rather than degrading if the message is not in that shape. A silent
// fallback here would turn every node comparison above into a comparison of two
// empty strings — a guard that is present, reachable and inert, which is the one
// thing this file exists to catch.
func fxFindingNode(t *testing.T, doc string, v Violation) string {
	t.Helper()
	if !fxIsDataModelRule(v.Rule) {
		t.Fatalf("%s: %s is not a %s rule; this table records only the profile data model, and a finding "+
			"from another Factur-X rule family needs its own reason before it is recorded here",
			doc, v.Rule, fxDMKeyPrefix)
	}
	i := strings.LastIndex(v.Message, " at "+fxAtDoc)
	if i < 0 || !strings.HasSuffix(v.Message, ")") {
		t.Fatalf("%s: %s reports %q, which does not end in \"( ... at <path>)\"; fxFindingNode reads the "+
			"node out of that shape and this test's node comparison is worthless without it",
			doc, v.Rule, v.Message)
	}
	return strings.TrimSuffix(v.Message[i+len(" at "):], ")")
}

// TestFacturXLeanTierExpectedFailuresHaveTheStatedCause checks the reasons.
//
// A recorded finding with a written reason is only worth having if the reason is
// held to something. Each expectation is checked twice over:
//
//   - against the artefact, by looking the rule up in the generated data-model
//     table and requiring the assertion's shape to be the cause the row claims —
//     `report @currencyID`, `report true()` or the code-list lookup. The keys are
//     positional, so an FNFE revision that inserts an assertion shifts them; this
//     is what makes that shift a failure naming the row rather than a silent
//     change of subject.
//   - against the document, by decoding it with encoding/xml — independently of
//     parseCII, which produced the findings — and requiring the node the row names
//     to be there, to carry what the row says it carries, and for the currency
//     rows to restate the document's own BT-5 rather than merely to have some
//     attribute.
//
// And once more across the whole document, which is the part a per-row check
// cannot do: every @currencyID anywhere in the file is either on ram:TaxTotalAmount
// — the single element the tier constrains instead of forbidding — or is accounted
// for by exactly one expectation. That closes the currency claim in both
// directions, and it is what lets the prose say "thirteen amounts restate a
// currency the invoice has already stated" as a measurement.
func TestFacturXLeanTierExpectedFailuresHaveTheStatedCause(t *testing.T) {
	// The three-way split the file header states in prose. It is counted here
	// rather than written there twice, so the paragraph a reader trusts cannot
	// drift from the table it describes.
	wantByCause := map[fxCause]int{
		fxCauseCurrencyRestated:     19,
		fxCauseElementNotInTier:     5,
		fxCauseValueOutsideCodeList: 1,
	}
	byCause := map[fxCause]int{}

	for _, s := range fxLeanTierSamples {
		for _, e := range s.expect {
			byCause[e.cause] += len(e.at)
		}
		t.Run(s.file, func(t *testing.T) {
			if s.divergence == "" {
				t.Errorf("%s has no divergence written down. A document kept because it disagrees with its "+
					"own authority has to say what the disagreement is, or the next reader cannot tell an "+
					"expected failure from an unnoticed one", s.file)
			}
			if s.publisher == "" {
				t.Errorf("%s names no publisher; whose sample this is is the whole point of the table", s.file)
			}
			nodes := fxDecodeCommitted(t, s.file)
			currency := fxSoleValue(t, nodes, fxAtSettle+"/InvoiceCurrencyCode", s.file)

			restated := map[string]int{}
			for _, e := range s.expect {
				if e.why == "" {
					t.Errorf("%s: %s is recorded with no reason", s.file, e.rule)
				}
				a, ok := fxDMAssertByKey(s.profile, e.rule)
				if !ok {
					t.Errorf("%s: %s is not an assertion of the %s data model. The keys are positional, so "+
						"an FNFE revision that inserts an assertion renumbers everything after it; re-derive "+
						"this row from the profile Schematron rather than renaming it",
						s.file, e.rule, string(s.profile))
					continue
				}
				if len(e.at) == 0 {
					t.Errorf("%s: %s is recorded with no node", s.file, e.rule)
					continue
				}
				switch e.cause {
				case fxCauseCurrencyRestated:
					if a.op != fxDMAttrForbidden || a.attr != "currencyID" {
						t.Errorf("%s: %s is recorded as a restated currency and the artefact publishes it as "+
							"%q, which is not `report @currencyID`", s.file, e.rule, a.test)
						continue
					}
					for at, n := range fxTally(e.at) {
						restated[at] += n
						got := fxNodesAt(nodes, at)
						if len(got) != n {
							t.Errorf("%s: %s is recorded firing %d time(s) at %s and the document has %d such "+
								"element(s)", s.file, e.rule, n, at, len(got))
							continue
						}
						for _, el := range got {
							v, has := el.attrs["currencyID"]
							switch {
							case !has:
								t.Errorf("%s: %s is recorded because %s carries @currencyID and it does not",
									s.file, e.rule, at)
							case v != currency:
								t.Errorf("%s: %s is recorded as restating the invoice currency; %s carries "+
									"@currencyID=%q and BT-5 says %q. That is a document naming two currencies, "+
									"which is a different and more serious claim than a redundant restatement — "+
									"say so in the reason before recording it", s.file, e.rule, at, v, currency)
							}
						}
					}
				case fxCauseElementNotInTier:
					if a.op != fxDMUnused {
						t.Errorf("%s: %s is recorded as an element the tier does not carry and the artefact "+
							"publishes it as %q, which is not `report true()`", s.file, e.rule, a.test)
						continue
					}
					for at, n := range fxTally(e.at) {
						if got := fxNodesAt(nodes, at); len(got) != n {
							t.Errorf("%s: %s is recorded firing %d time(s) at %s and the document has %d such "+
								"element(s); at this tier presence is the whole of the offence, so the count of "+
								"elements and the count of findings are the same number",
								s.file, e.rule, n, at, len(got))
						}
					}
				case fxCauseValueOutsideCodeList:
					if a.op != fxDMCode {
						t.Errorf("%s: %s is recorded as a code-list lookup and the artefact publishes it as %q",
							s.file, e.rule, a.test)
						continue
					}
					list := facturXCodeLists[a.list]
					if len(list) == 0 {
						t.Errorf("%s: %s binds an empty code list, so it could not report on any value",
							s.file, e.rule)
						continue
					}
					for at := range fxTally(e.at) {
						v := fxSoleValue(t, nodes, at, s.file)
						if v == "" {
							t.Errorf("%s: %s is recorded on %s and the element is empty; FNFE's own "+
								"string-length($v)=0 disjunct passes an empty value", s.file, e.rule, at)
							continue
						}
						if fxDMInList(a.list, v) {
							t.Errorf("%s: %s is recorded because %q is not in the code list FNFE binds to %s, "+
								"and the list holds it: %v", s.file, e.rule, v, at, list)
							continue
						}
						if e.spelledAs == "" {
							continue
						}
						if !fxDMInList(a.list, e.spelledAs) {
							t.Errorf("%s: %s says the document's %q is %q spelled differently, and %q is not "+
								"in the code list either: %v", s.file, e.rule, v, e.spelledAs, e.spelledAs, list)
							continue
						}
						if got, want := fxURNTokens(v), fxURNTokens(e.spelledAs); got != want {
							t.Errorf("%s: %s says %q differs from the permitted %q only in how it is written, "+
								"and they reduce to different identifiers: %q against %q. That is the document "+
								"naming another profile, which is a different finding",
								s.file, e.rule, v, e.spelledAs, got, want)
						}
					}
				default:
					t.Errorf("%s: %s carries a cause this test does not check", s.file, e.rule)
				}
				for _, at := range e.at {
					if strings.HasSuffix(at, "/TaxTotalAmount") {
						t.Errorf("%s: %s is recorded firing on ram:TaxTotalAmount, which is the one amount every "+
							"Factur-X tier constrains @currencyID on rather than forbidding it. Re-read the rule "+
							"before recording this", s.file, e.rule)
					}
				}
			}

			// The closure. Every @currencyID in the document, wherever it sits.
			exempt := 0
			loose := map[string]int{}
			for _, n := range nodes {
				v, has := n.attrs["currencyID"]
				if !has {
					continue
				}
				if strings.HasSuffix(n.path, "/TaxTotalAmount") {
					exempt++
					if v != currency {
						t.Errorf("%s: ram:TaxTotalAmount carries @currencyID=%q against a BT-5 of %q. That is "+
							"the legitimate two-currency case, BT-110 against BT-111 — and this table has no "+
							"row saying so", s.file, v, currency)
					}
					continue
				}
				loose[n.path]++
			}
			if exempt == 0 {
				t.Errorf("%s: no ram:TaxTotalAmount carries @currencyID. Every one of these six documents does, "+
					"and it is the demonstration that the rule set discriminates rather than forbidding the "+
					"attribute outright — losing it would make the whole table read as an over-strict package",
					s.file)
			}
			if diff := fxTallyDiff(loose, restated); diff != "" {
				t.Errorf("%s: the @currencyID attributes outside ram:TaxTotalAmount and the rows accounting for "+
					"them differ — %s. Every one of those attributes sits on an amount whose tier excludes it, "+
					"so the two have to be the same multiset: an attribute in the document that no row claims "+
					"is a finding this package stopped reporting, and a row claiming an attribute that is not "+
					"there is a reason that no longer describes the document", s.file, diff)
			}
			restatements := 0
			for _, n := range loose {
				restatements += n
			}
			t.Logf("%s (%s, %s): %d expected finding(s); %d @currencyID outside ram:TaxTotalAmount across %d "+
				"element path(s), %d on it, BT-5 %s", s.file, string(s.profile), s.publisher,
				len(s.expectedPairs()), restatements, len(loose), exempt, currency)
		})
	}

	names := map[fxCause]string{
		fxCauseCurrencyRestated:     "a currency restated on an amount the tier excludes it from",
		fxCauseElementNotInTier:     "an element the tier does not carry",
		fxCauseValueOutsideCodeList: "a value outside the code list its own authority binds",
	}
	total := 0
	for c, want := range wantByCause {
		total += byCause[c]
		if byCause[c] != want {
			t.Errorf("%d expected failure(s) are %s and the file header says %d. The header's arithmetic is "+
				"part of the argument these documents are kept for; correct it with the table",
				byCause[c], names[c], want)
		}
	}
	if got := len(byCause); got > len(wantByCause) {
		t.Errorf("the table uses %d causes and this test knows %d", got, len(wantByCause))
	}
	t.Logf("25 expected failures across six documents: %d %s, %d %s, %d %s — %d in all",
		byCause[fxCauseCurrencyRestated], names[fxCauseCurrencyRestated],
		byCause[fxCauseElementNotInTier], names[fxCauseElementNotInTier],
		byCause[fxCauseValueOutsideCodeList], names[fxCauseValueOutsideCodeList], total)
}

// TestFacturXForbidsRestatingTheCurrencyOnEveryAmountButTaxTotalAmount reads the
// design behind nineteen of the twenty-five expected failures out of the artefact
// itself, so the reasons above can appeal to it instead of asserting it.
//
// Across all five profile tables: of the contexts naming an element whose name
// ends in Amount, every one either marks the element unused at that tier, or
// forbids @currencyID outright — or is a ram:TaxTotalAmount variant, selected by
// which currency the attribute names, whose rule *constrains the attribute's
// value* against the currency code list rather than forbidding the attribute.
// There is no fourth case in any of the five files.
//
// That is the whole of the design: the invoice is denominated once as BT-5, no
// amount may restate it, and the single place two currencies legitimately coexist
// is the VAT total, BT-110 against BT-111. CEN carves ram:TaxTotalAmount out of
// CII-DT-031 by name for the same reason and by a different route.
func TestFacturXForbidsRestatingTheCurrencyOnEveryAmountButTaxTotalAmount(t *testing.T) {
	totalForbidden, totalExempt := 0, 0
	for _, p := range profiles {
		forbidden, unused, exempt := 0, 0, 0
		for _, r := range facturXDataModel[p] {
			last := r.steps[len(r.steps)-1].name
			if !strings.HasSuffix(last, "Amount") {
				continue
			}
			forbids, notInTier, constrains := false, false, false
			for _, a := range r.asserts {
				switch {
				case a.op == fxDMAttrForbidden && a.attr == "currencyID":
					forbids = true
				case a.op == fxDMUnused:
					notInTier = true
				case a.op == fxDMCode && a.value.attr == "currencyID":
					constrains = true
				}
			}
			switch {
			case forbids:
				forbidden++
			case notInTier:
				unused++
			case last == "TaxTotalAmount" && constrains:
				exempt++
			default:
				t.Errorf("%s: the rule at %s neither forbids @currencyID, nor marks the element unused at "+
					"this tier, nor is a ram:TaxTotalAmount whose value is constrained. A fourth case means "+
					"the design nineteen expected failures in fxLeanTierSamples appeal to is not what FNFE "+
					"publishes, and those reasons have to be rewritten before this test is relaxed",
					string(p), r.context)
			}
		}
		if exempt == 0 {
			t.Errorf("%s: no ram:TaxTotalAmount context constrains @currencyID instead of forbidding it, so "+
				"this tier admits no currency on any amount at all. That is a different data model from the "+
				"one this file describes", string(p))
		}
		totalForbidden += forbidden
		totalExempt += exempt
		t.Logf("%s: %d amount context(s) forbid @currencyID, %d exclude the element from the tier, %d are "+
			"ram:TaxTotalAmount variants that constrain the attribute's value", string(p), forbidden, unused, exempt)
	}
	t.Logf("Factur-X, all five profiles: @currencyID is forbidden on %d amount contexts and constrained on "+
		"%d, and every one of the %d is a ram:TaxTotalAmount", totalForbidden, totalExempt, totalExempt)
}

// TestMinimumKeysARuleOnACurrencyItsOwnTierNeverDefines pins the open question
// recorded at the top of this file.
//
// MINIMUM selects one of its three ram:TaxTotalAmount rules on
// `@currencyID=../../ram:TaxCurrencyCode` — BT-6, the accounting currency — and
// gives ram:TaxCurrencyCode no rule of its own, where the other four profiles all
// do. The tier therefore keys a rule on an element it never defines: a MINIMUM
// document carrying a BT-6 is neither permitted, nor forbidden, nor value-checked,
// and no document in this tree exercises it.
//
// This test does not decide whether that is intentional. It fails if the asymmetry
// moves, in either direction, so that an FNFE revision resolving it arrives as a
// question pointed back at the note rather than as silence.
func TestMinimumKeysARuleOnACurrencyItsOwnTierNeverDefines(t *testing.T) {
	const bt6 = "TaxCurrencyCode"
	refs, defs := map[Profile]int{}, map[Profile]int{}
	for _, p := range profiles {
		for _, r := range facturXDataModel[p] {
			if r.steps[len(r.steps)-1].name == bt6 {
				defs[p]++
			}
			if strings.Contains(r.context, bt6) {
				refs[p]++
			}
			for _, a := range r.asserts {
				if strings.Contains(a.test, bt6) {
					refs[p]++
				}
			}
		}
	}
	if refs[ProfileMinimum] == 0 {
		t.Errorf("MINIMUM no longer mentions ram:%s anywhere. The open question at the top of this file was "+
			"whether the tier means to admit a second currency; if FNFE has dropped the reference, it is "+
			"answered and the note should go", bt6)
	}
	if defs[ProfileMinimum] != 0 {
		t.Errorf("MINIMUM now gives ram:%s a rule of its own. The open question at the top of this file is "+
			"answered — the tier does define BT-6 — and the note should say so", bt6)
	}
	for _, p := range profiles {
		if p == ProfileMinimum {
			continue
		}
		if defs[p] == 0 {
			t.Errorf("%s no longer defines ram:%s either, so MINIMUM's silence is no longer an asymmetry and "+
				"the note at the top of this file is describing something that is not there", string(p), bt6)
		}
	}
	for _, s := range fxLeanTierSamples {
		for _, n := range fxDecodeCommitted(t, s.file) {
			if strings.HasSuffix(n.path, "/"+bt6) {
				t.Errorf("%s carries a ram:%s. No committed document did when the note at the top of this "+
					"file was written, which is why the question is open; a document that exercises it is "+
					"the evidence that would close it", s.file, bt6)
			}
		}
	}
	t.Logf("BT-6 ram:%s: MINIMUM refers to it %d time(s) and gives it no rule; the other four profiles each "+
		"give it one. No committed document carries one", bt6, refs[ProfileMinimum])
}

// ---------------------------------------------------------------------------
// Reading the documents, independently of the parser under test
// ---------------------------------------------------------------------------

// fxDocNode is one element of a committed document as encoding/xml sees it.
//
// The decode is deliberately not parseCII's: a check that a finding's stated
// cause is really in the document is worth little if the only witness is the same
// tree the finding came out of. It is also not a regular expression, for C31's
// reason — a guard that reads a normative artefact with a pattern is a guard that
// can quietly stop guarding.
type fxDocNode struct {
	// path is local names from the document element, "/CrossIndustryInvoice/...",
	// which is the spelling the findings report their context in.
	path  string
	attrs map[string]string
	text  string
}

func fxDecodeCommitted(t *testing.T, file string) []fxDocNode {
	t.Helper()
	p := filepath.Join(committedCorpusDir, file)
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	defer f.Close()

	var nodes []fxDocNode
	var open []int
	dec := xml.NewDecoder(f)
	for {
		tok, terr := dec.Token()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			t.Fatalf("%s: %v", p, terr)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			parent := ""
			if len(open) > 0 {
				parent = nodes[open[len(open)-1]].path
			}
			n := fxDocNode{path: parent + "/" + tk.Name.Local, attrs: map[string]string{}}
			for _, a := range tk.Attr {
				// Namespace declarations are not attributes of the element for
				// any purpose here, and @currencyID is unprefixed in all five
				// artefacts.
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				n.attrs[a.Name.Local] = a.Value
			}
			nodes = append(nodes, n)
			open = append(open, len(nodes)-1)
		case xml.CharData:
			if len(open) > 0 {
				nodes[open[len(open)-1]].text += string(tk)
			}
		case xml.EndElement:
			if len(open) == 0 {
				t.Fatalf("%s: end element </%s> with nothing open", p, tk.Name.Local)
			}
			open = open[:len(open)-1]
		}
	}
	if len(nodes) == 0 {
		t.Fatalf("%s: decoded no elements", p)
	}
	return nodes
}

// fxNodesAt is every element at a path, in document order.
func fxNodesAt(nodes []fxDocNode, path string) []fxDocNode {
	var out []fxDocNode
	for _, n := range nodes {
		if n.path == path {
			out = append(out, n)
		}
	}
	return out
}

// fxSoleValue is the trimmed text of the one element at a path. More than one is
// a failure rather than a choice: every path this test reads that way names an
// element its tier requires exactly once.
func fxSoleValue(t *testing.T, nodes []fxDocNode, path, doc string) string {
	t.Helper()
	got := fxNodesAt(nodes, path)
	if len(got) != 1 {
		t.Fatalf("%s: %d element(s) at %s, expected exactly one", doc, len(got), path)
	}
	return strings.TrimSpace(got[0].text)
}

// fxDMAssertByKey finds an assertion of a profile's data model by its synthetic
// identifier, so an expectation can be checked against the shape the artefact
// publishes rather than against the shape its reason claims.
func fxDMAssertByKey(p Profile, key string) (fxDMAssert, bool) {
	for _, r := range facturXDataModel[p] {
		for _, a := range r.asserts {
			if a.key == key {
				return a, true
			}
		}
	}
	return fxDMAssert{}, false
}

// fxURNTokens reduces a specification identifier to the sequence that names the
// profile, treating ":" and "#" alike and dropping the urn scheme token.
//
// It exists for one claim: that fnfe_BASIC.xml's BT-24 names the profile FNFE's
// code list permits and writes it a way the list does not hold. Without the
// reduction that is an assertion about two strings a reader has to compare by eye;
// with it, "the same identifier, spelled differently" and "a different identifier"
// are told apart by the test.
func fxURNTokens(id string) string {
	var out []string
	for _, tok := range strings.FieldsFunc(id, func(r rune) bool { return r == ':' || r == '#' }) {
		if tok == "urn" {
			continue
		}
		out = append(out, tok)
	}
	return strings.Join(out, "|")
}

// fxTally counts repeats, which is how a node that a rule fires at twice is told
// from one it fires at once.
func fxTally(paths []string) map[string]int {
	out := map[string]int{}
	for _, p := range paths {
		out[p]++
	}
	return out
}

// fxTallyDiff names where two tallies disagree, and is empty when they do not.
// It reports the difference rather than both sides because the sides are long and
// nearly identical, and the one line a reader needs is which node moved.
func fxTallyDiff(inDocument, accountedFor map[string]int) string {
	var out []string
	seen := map[string]bool{}
	for _, m := range []map[string]int{inDocument, accountedFor} {
		for k := range m {
			if seen[k] {
				continue
			}
			seen[k] = true
			if inDocument[k] != accountedFor[k] {
				out = append(out, fmt.Sprintf("%s: %d in the document, %d recorded", k, inDocument[k], accountedFor[k]))
			}
		}
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

// TestFacturXRoutingAcceptsEveryIdentifierTheAuthorityPublishes is the guard that
// makes the fix these documents forced non-recurring.
//
// Factur-X 1.0 and ZUGFeRD 2.x are one specification under two brands, and FNFE's
// own code database says so: the enumeration its Schematron looks BT-24 up in
// holds two values at every tier that names itself, one per brand. specIDRules
// matched only "factur-x.eu", so a ZUGFeRD-branded MINIMUM invoice was routed to
// CEN's EN 16931 CII binding and accused of BR-16 and BR-CO-18 — an invoice line
// and a VAT breakdown, at a head-only tier that has neither by design. That is
// C44 in the German half of the identifier space, and it was live on
// intarsys_MINIMUM.xml.
//
// The population is read out of the committed data-model table rather than
// written here, so a brand or a version FNFE adds arrives as a failure instead of
// as silence. The table is generated from the artefact and checked against it by
// facturx_datamodel_test.go, so this is the authority's own list at one remove
// and not a transcription.
func TestFacturXRoutingAcceptsEveryIdentifierTheAuthorityPublishes(t *testing.T) {
	published := facturXPublishedSpecIDs(t)
	if len(published) != len(profiles) {
		t.Fatalf("found BT-24 code lists for %d of the %d profiles; the data-model table is not being read",
			len(published), len(profiles))
	}
	for _, p := range profiles {
		ids := published[p]
		if len(ids) == 0 {
			t.Errorf("%s publishes no BT-24 value; every profile's element table constrains that element", string(p))
			continue
		}
		for _, id := range ids {
			if p == ProfileEN16931 {
				// The one tier that names no brand. Its identifier is CEN's own
				// "urn:cen.eu:en16931:2017" with nothing added, which is FNFE
				// saying the document is exactly EN 16931; routing it to
				// Factur-X would claim a tier the document does not claim. Both
				// halves are asserted so the exception cannot quietly become the
				// rule.
				if got, ok := facturXProfileFromSpecID(id); ok {
					t.Errorf("%q is CEN's own identifier and facturXProfileFromSpecID reads a Factur-X tier "+
						"(%q) out of it", id, string(got))
				}
				if src := specIDSource(id); src == SourceFacturX {
					t.Errorf("%q is CEN's own identifier and specIDRules routes it to %s", id, src)
				}
				continue
			}
			got, ok := facturXProfileFromSpecID(id)
			if !ok || got != p {
				t.Errorf("FNFE's %s code database publishes %q for BT-24 and facturXProfileFromSpecID "+
					"answers (%q, %v); a tier this package cannot read out of an identifier its authority "+
					"publishes is judged by CEN's binding instead, which is C44",
					string(p), id, string(got), ok)
			}
			if src := specIDSource(id); src != SourceFacturX {
				t.Errorf("FNFE's %s code database publishes %q for BT-24 and specIDRules routes it to %s; "+
					"ValidateCIUS would then reach another authority's rule set", string(p), id, src)
			}
		}
	}
	t.Logf("Factur-X routing: every BT-24 value the five code databases publish is routed to the tier that "+
		"publishes it (%d identifiers)", func() int {
		n := 0
		for _, ids := range published {
			n += len(ids)
		}
		return n
	}())
}

// facturXPublishedSpecIDs is, per profile, the BT-24 values FNFE's code database
// allows: the code list bound to the assertion on
// ram:GuidelineSpecifiedDocumentContextParameter/ram:ID.
func facturXPublishedSpecIDs(t *testing.T) map[Profile][]string {
	t.Helper()
	const ctxSuffix = "ram:GuidelineSpecifiedDocumentContextParameter/ram:ID"
	out := map[Profile][]string{}
	for _, p := range profiles {
		for _, r := range facturXDataModel[p] {
			if !strings.HasSuffix(r.context, ctxSuffix) {
				continue
			}
			for _, a := range r.asserts {
				if a.op != fxDMCode {
					continue
				}
				if a.list < 0 || a.list >= len(facturXCodeLists) {
					t.Fatalf("%s: %s names code list %d and there are %d", string(p), a.key, a.list, len(facturXCodeLists))
				}
				out[p] = append(out[p], facturXCodeLists[a.list]...)
			}
		}
	}
	return out
}
