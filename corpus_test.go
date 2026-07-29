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

	// EN 16931 and the CIUS layered on it.
	minEN16931UBLInvoices = 15
	minXRechnungInstances = 86
	minPeppolExamples     = 9
	minCIUSPTInstances    = 20
	minCIUSROInstances    = 44
	minUBLBEInstances     = 36
	minSRBDTInstances     = 10

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
