package formalis

import "testing"

// The corpus ratchets, in one place.
//
// Every oracle in this suite is backed by a corpus this repository does not
// vendor: `make cius-oracles`, `make en16931-artefacts` and `make en16931-ubl`
// fetch some 1,600 documents over the network. An oracle written as "for every
// file I found, assert it is clean" therefore has a failure mode with no
// symptom. When the fetch brings back three XRechnung instances instead of
// eighty-six, the test passes, logs "3/3 instances clean (FP=0)", and the claim
// it prints is now backed by three documents. A change that regressed forty
// instances would land green.
//
// That is not hypothetical, and it was not only a hypothetical for this
// repository. The fetch is ~600 downloads driven by `gh api`; it used to run
// `curl` without -f (which writes the server's error body into the .xml file and
// exits 0) under a shell with no -e or pipefail, and it pasted repository paths
// into URLs without encoding them. Making it strict revealed that it had been
// dropping documents all along: ZATCA arrived with 18 of its 45 samples,
// Svefaktura with 2 of 6, OIOUBL with 18 of 52, Finvoice with 12 of 13. Every
// FP=0 claim over those corpora was a claim about a fraction of them, and
// nothing said so. The numbers below are from a fetch that finishes.
//
// The Makefile now fails loudly on a partial fetch; these constants are the
// second half, because a corpus can also be truncated by an interrupted run, a
// stale directory, or an upstream that moved its files.
//
// Each number is what the corpus held when the ratchet was written. They are
// floors, not equalities — upstream adds samples and the oracles simply get
// stronger. A number that has to be *lowered* is either an upstream corpus that
// genuinely shrank, which belongs in the commit message, or a fetch that did not
// finish, which is the case these exist to catch.
//
// They live here rather than beside their tests so the whole population is
// readable at once, and so an oracle added without a floor is visible by its
// absence.
const (
	// Whole-corpus sweeps: every .xml under testdata/, from every source.
	minCorpusDocuments = 1680

	// The two arbitration sweeps count different populations and are not
	// comparable with each other: minRoutedDocuments is the documents in a
	// corpus that publishes exactly one format (detect_scan_test.go), and
	// minDispatchedDocuments is the documents parseEN16931 accepts, which is
	// what ValidateCIUS can be asked to route (routing_test.go).
	minRoutedDocuments     = 884
	minDispatchedDocuments = 964

	// The CEN/TC 434 EN 16931 per-rule unit-test suite. The file count guards
	// the clone; minEN16931RulesCaught is the coverage ratchet over it, and it
	// is the one number in this package that measures the rule engine rather
	// than the corpus.
	//
	// It measures it over the population the suite can see, which is smaller than
	// the rule set. The ratio is computed over the rules for which the suite ships
	// an <error> fragment, so a rule with no failing fragment is invisible to it
	// in both directions: implementing one does not move the number, and losing
	// one would not either. Of the semantic-model rules added since this baseline
	// was set — BR-CL-08, BR-CL-26, BR-DEC-02/06/15/25/28 — the suite ships a
	// fragment for none, and the one it does ship for BR-51 is tagged <warning>,
	// which the harness does not score. So 198 standing still is the expected
	// result of that work rather than evidence there was none;
	// en16931_core_rules_test.go is where those rules are stated, and
	// Coverage(SourceEN16931) is where the remaining gap is.
	minEN16931UnitTestFiles = 277
	minEN16931RulesCaught   = 198

	// The advisory halves of CEN's two syntax bindings — 1,168 generated rules,
	// reported as warnings. These two are a different kind of ratchet from every
	// other number here: the rest guard against a corpus that arrived short, and
	// these guard against a *rule set* that stopped firing.
	//
	// They exist because the assertions that would otherwise have caught that were
	// weakened on purpose. An FP=0 oracle reading "a conforming document produces
	// no finding" could not survive rules whose whole job is to report legal-but-
	// non-core UBL, so the XRechnung oracle now reads "no fatal finding" — and a
	// change that silently stopped emitting four hundred advisory rules would have
	// left every oracle in this suite green. These are the compensating floor.
	// TestAdvisoryBindingsFireAcrossTheCorpus measures both over the whole corpus;
	// minXRechnungAdvisory is the same guard scoped to the one corpus whose
	// assertion was loosened.
	//
	// They are floors, so implementing more rules or fetching more documents only
	// makes them stronger. A number that has to be lowered means a rule genuinely
	// stopped applying, and that belongs in a commit message.
	minAdvisoryRulesFiring = 158
	minAdvisoryFindings    = 3157
	// minXRechnungAdvisory counts every warning ValidateXRechnung reports over the
	// 86 KoSIT business cases, and it is no longer only CEN's binding rules: KoSIT
	// flags eleven of its own fifty-seven identifiers warning or information, and
	// BR-DE-TMP-32 — "an invoice should state its delivery date" — applies to a
	// conforming invoice that simply does not. It went from 10 to 46 when those
	// eleven were implemented and their severity corrected, and 36 of the 46 are
	// BR-DE-TMP-32.
	minXRechnungAdvisory = 46

	// EN 16931 and the CIUS layered on it.
	minEN16931UBLInvoices = 15
	minXRechnungInstances = 86

	// The KoSIT Schematron's own per-rule fixtures, and the verdicts they declare
	// for themselves in <?xmute mutator="identity"?> instructions. They are a
	// different kind of oracle from every other number here: not documents that
	// must be clean, but documents an authority says are invalid against one named
	// rule and valid against another, which is the only oracle in this repository
	// that gives a *violating* verdict for a CIUS rule. Both are floors — upstream
	// adds fixtures — and a number that falls means either a short fetch or a rule
	// that stopped being exercised.
	minXRechnungRuleInstances = 242
	minXRechnungRuleVerdicts  = 362
	minPeppolExamples         = 9

	// OpenPEPPOL's own per-rule test sets, under rules/unit-{UBL,CII}-PEPPOL and the
	// thirteen country directories beside them. They are the same kind of oracle as
	// KoSIT's fixtures above and the only one that gives a *violating* verdict for a
	// Peppol rule from the authority that wrote it: each file is a test set scoped to
	// one identifier and holding several documents, every one of which declares
	// whether OpenPEPPOL considers it valid or invalid against that rule.
	//
	// The example corpus is nine conforming invoices, so it could only ever say that
	// no rule over-fires. These say that each rule fires at all, which is what tells
	// a working rule from one bound to an element name no document contains.
	//
	// They went from 354/350 to 876/885 when the country directories were read:
	// unit-{UBL,CII}-{DE,DK,GR,IT,NL,NO,SE} had been on disk since the corpus was
	// first fetched and nothing had opened them, which is the same shape as C33
	// itself — 140 test sets for 101 rules nobody had counted.
	minPeppolRuleDocuments = 876
	minPeppolRuleVerdicts  = 885

	// rules/national-examples: the invoices OpenPEPPOL publishes as *conforming
	// examples of its country-specific rule sets*. Three documents, and they are the
	// only oracle in this repository that says a national rule does not over-fire on
	// a document its own authority holds up as correct. The Greek pair in particular
	// is what distinguishes $supplierCountry from $accountingSupplierCountry: the
	// tax-representative example's seller has a Swedish postal address and no VAT
	// identifier of its own, so it is Greek by one variable and not by the other, and
	// a validator that conflated them reports GR-R-009 against it.
	minPeppolNationalExamples = 3
	minCIUSPTInstances        = 20
	minCIUSROInstances        = 44
	minUBLBEInstances         = 36
	minSRBDTInstances         = 10

	// NLCIUS is the one suite that carries both verdicts: the instances named
	// for a BR-NL rule, and the subset of those that are deliberately broken and
	// must be caught. A truncation that removed only the broken half would leave
	// the false-positive assertion looking healthy, so both are ratcheted.
	minNLCIUSInstances    = 73
	minNLCIUSErrorsCaught = 27

	// National formats. Most corpora hold one document kind and the oracle runs
	// on all of them. Three do not: the OIOUBL fetch already content-filters,
	// and the Svefaktura and UBL-TR sets mix in documents their predicate
	// declines, so those carry a file floor (what the fetch must deliver) and a
	// recognised floor (what the FP=0 assertion actually runs on). One number
	// would hide a truncation in whichever half it did not count.
	minEbInterfaceInstances = 38
	minFacturaeInstances    = 3
	minFatturaPAInstances   = 18
	minFinvoiceInstances    = 13
	minKSeFInstances        = 25
	minOIOUBLInstances      = 52
	minOSAInstances         = 54
	minPINTInstances        = 64
	minTEAPPSInstances      = 5
	minZATCAInstances       = 45

	minSvefakturaFiles      = 6
	minSvefakturaRecognised = 5
	minUBLTRFiles           = 39
	minUBLTRRecognised      = 23
)

// atLeast is the ratchet. It reports rather than skips: an oracle that found
// *some* of its corpus is not in the "corpus absent" case the skips cover, it is
// in the case where a green result would be a claim about the wrong population.
func atLeast(t *testing.T, what string, got, want int) {
	t.Helper()
	if got < want {
		t.Errorf("%s: %d, want at least %d — the corpus is truncated, so a clean verdict here is "+
			"evidence about %d documents rather than about the corpus; re-fetch with "+
			"`make clean-cius-oracles cius-oracles`, and if upstream really did shrink, lower the "+
			"constant in corpus_test.go deliberately", what, got, want, got)
	}
}
