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
	//
	// 1,690 rather than 1,680: PR 28 added the ten instances SimplerInvoicing ships
	// for the G-account extension, which live in an upstream directory of their own
	// and which no fetch here had ever touched.
	// It stays at 1,690 although the tree now holds 1,805: the 59 Factur-X
	// examples and FNFE's 51 validation reports beside them are vendored rather
	// than fetched, so a checkout that ran every make target and copied no bundle
	// has 1,695 and is not truncated. minFacturXExamples and minFacturXReports are
	// the floors on that material, where they can be skipped as a unit. (1,695
	// rather than 1,690 because `make facturx-schematron` now also vendors the five
	// FACTUR-X_<PROFILE>_codedb.xml the data-model code-list assertions look values
	// up in, and they are .xml files under testdata like any other.)
	minCorpusDocuments = 1690

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

	// The 290 generated DT-CIUS-PT-* rules, over every UBL document in the corpus.
	// They are the same kind of ratchet as minAdvisoryRulesFiring above and exist
	// for the same reason: every FP=0 oracle in this suite asserts the *absence* of
	// findings, so a rule set that silently stopped firing would leave all of them
	// green. Most of the documents counted here are not Portuguese invoices — a
	// Peppol or XRechnung invoice asked AT's datatype questions answers no to a
	// great many of them — and that is what makes the number large enough to move
	// when something breaks.
	minCIUSPTDatatypeRulesFiring = 111
	minCIUSPTDatatypeFindings    = 13600

	// What ValidateCIUSPT reports that a reference CIUS-PT validator cannot, split
	// by why. AT/eSPap vendored CEN's validation-1.1.0 in June 2018 and has not
	// refreshed it, so 192 CEN identifiers this package evaluates are absent from
	// the Portuguese rule set — 114 that CEN had published by then and AT left out,
	// 78 that CEN has added since.
	//
	// Both are ratcheted, and separately, because they are opposite kinds of fact.
	// A `postdates` count that fell would mean this package stopped evaluating a
	// rule CEN has since added, which is a false negative against the standard the
	// CIUS narrows. A `dropped` count that fell would mean a suppression landed —
	// which may one day be right, but only with the replacement argument made and
	// the ground shown to be still covered, and it must never happen quietly. See
	// cius_omissions_test.go for the derivation and TestNoDroppedCENIdentifierIsSuppressed
	// for the decision these numbers pin.
	minCIUSPTDroppedFindings   = 1200
	minCIUSPTPostdatesFindings = 520

	minCIUSROInstances = 44

	// The CIUS-RO samples that write a Bucharest sector where ISO 3166-2:RO expects
	// a county code, and the BR-RO-110/111/170 findings ANAF's own Schematron
	// therefore reports on them. They are ratcheted for the reason the NLCIUS pair
	// above is: the FP=0 oracle for this corpus permits exactly these findings, so
	// a number that fell would mean either that the corpus lost the documents the
	// exception is scoped to — leaving a permission with nothing to permit — or
	// that BR-RO-110/111/170 stopped firing, which is the false negative that
	// implementing them faithfully was meant to close.
	minCIUSROSectorDocs     = 7
	minCIUSROSectorFindings = 11

	// The CIUS-RO samples that declare the superseded RO_CIUS 1.0.0 identifier in
	// BT-24 — the whole of the 1.0.3 and 1.0.4 sample sets — and therefore report
	// BR-RO-001 against the 1.0.9 rule set this package evaluates. Ratcheted for
	// the same reason as the pair above: the oracle permits exactly this finding on
	// exactly these documents, so a number that fell would mean either that the
	// corpus lost them or that BR-RO-001 stopped firing.
	minCIUSROSupersededDocs = 22

	// The 90 generated CIUS-RO length, decimal, date-format and occurrence rules,
	// over every UBL document in the corpus. Same kind of ratchet as
	// minCIUSPTDatatypeRulesFiring: every FP=0 oracle asserts the absence of
	// findings, so a rule set that silently stopped firing would leave all of them
	// green. Most of the documents counted here are not Romanian invoices.
	minCIUSRORulesFiring  = 9
	minCIUSRORuleFindings = 112

	// The context-node ratchet for the same 90 rules, and the stronger of the two:
	// a rule that stops being *asked* is invisible to a findings count, because a
	// rule set that never runs reports nothing and a rule set that runs and passes
	// reports nothing too. This counts the (context node, assertion) pairs the
	// corpus produces, which is a large number that moves when a context stops
	// matching. TestCIUSRORuleContextsAreReachable also asserts that every one of
	// the 90 is reached at least once.
	minCIUSROContextNodes = 62000
	minUBLBEInstances     = 36
	minSRBDTInstances     = 10

	// NLCIUS is the one suite that carries both verdicts: the instances named
	// for a BR-NL rule, and the subset of those that are deliberately broken and
	// must be caught. A truncation that removed only the broken half would leave
	// the false-positive assertion looking healthy, so both are ratcheted.
	minNLCIUSInstances    = 74
	minNLCIUSErrorsCaught = 27
	// The third verdict the same suite carries, and the one nothing read until the
	// advisory tier was implemented: the instances SimplerInvoicing ships as
	// conformant documents that a validator warns about. It is the compensating
	// floor for the advisory half of this rule set, in the same sense as
	// minAdvisoryRulesFiring above — every other NLCIUS assertion here is about the
	// *absence* of findings, so a tier that silently stopped firing would leave them
	// all green.
	minNLCIUSWarningsReported = 25

	// The same suite read per rule rather than per family
	// (TestNLCIUSPerRuleFixtures). SimplerInvoicing is the one of these five
	// authorities that declares a verdict for a *named* rule, and this repository
	// had those 95 files on disk since the corpus was first fetched without ever
	// asking whether the rule a fixture names is the rule that fires — the same
	// shape as the 242 KoSIT and 885 OpenPEPPOL verdicts PRs 19-21 found unread.
	// The second number is the one that would fall if a rule stopped being
	// evaluated while its neighbours covered for it.
	// The second number went from 12 to 30 when the advisory tier was implemented:
	// the twelve fatal identifiers, and the eighteen advisory ones SimplerInvoicing
	// ships a _warning_ instance for.
	minNLCIUSRuleVerdicts = 73
	minNLCIUSRulesFired   = 30

	// The G-account extension's instances, which are a *separate* upstream directory
	// from the 95 above and which nothing here fetched until PR 28. That is why none
	// of those 95 exercises the extension: they are SI-UBL 2.0 documents and these are
	// SI-UBL 2.0 G-account documents, and phive registers the two as separate
	// validation executor sets.
	//
	// Ten instances, nine of them broken against one of the eight published rules and
	// one conforming. Seven distinct rules are exercised — BR-GA-1 and BR-GA-2 get two
	// instances each, and BR-GA-0 gets none, which is what nlciusGAccountExtras is
	// for. Both are floors for the usual reason: a fetch that returned only the
	// conforming sample would leave the false-positive assertion looking healthy while
	// nothing checked that a rule fires at all.
	minNLCIUSGAccountInstances  = 10
	minNLCIUSGAccountRulesFired = 7

	// The empty-element rule, per binding and in total
	// (TestNLCIUSEmptyElementFindingsAreEmptyElements). It is the one rule in this
	// package whose context is `//*`, so it is the one whose finding count is a
	// property of the corpus rather than of a handful of documents: 739 findings over
	// 233 of the 974 documents parseEN16931 accepts, 615 under SI-UBL-2 and 124 under
	// empty-element-check. It is also the one NLCIUS rule that is evaluated outside
	// the $si gate, so almost all of those are documents no other NLCIUS assertion
	// says anything about — which is exactly why it needs a floor of its own. Every
	// other NLCIUS number here would stay green if this rule stopped being emitted.
	//
	// Both bindings are counted, because the two identifiers are reached by disjoint
	// halves of the corpus and one of them going silent must not be covered by the
	// other.
	minNLCIUSEmptyElementFindings = 700
	minNLCIUSEmptyElementsUBL     = 590
	minNLCIUSEmptyElementsCII     = 115

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

	// Factur-X, in two halves that arrive by different routes.
	//
	// The five profile Schematrons are fetchable — `make facturx-schematron`
	// takes them from ZUGFeRD/mustangproject, which vendors FNFE's own files
	// identifier for identifier — so their floor is a per-profile rule count and
	// a missing one is a red build rather than a vacuously green guard. These are
	// rule counts and not assertion counts because a rule with no assertion still
	// claims its nodes away from every rule below it, and the ordering guard
	// reads them all.
	//
	// The examples and FNFE's own validation reports are not. FNFE-MPE publishes
	// them inside a ~33 MB specification bundle that is not individually
	// addressable, and ZUGFeRD/corpus on GitHub carries only the EN 16931 subset
	// — 3 EXTENDED documents against the bundle's 25, and EXTENDED is the tier the
	// two rule sets actually disagree about. So they are vendored, the oracles
	// skip without them the way every other corpus-backed test here does, and
	// these floors are what stops a half-copied directory reading as a clean
	// sweep.
	minFacturXExamples = 59
	minFacturXReports  = 51
	// Of the 59 examples, 32 declare a Factur-X profile in BT-24. The other 27
	// declare CEN's own identifier (23, which is what the EN 16931 tier declares)
	// or an XRechnung one (4), and are validated by the rule set they name. This
	// floor is on the first group, because it is the population the scoping
	// change is measured over.
	minFacturXProfiled = 32
	// The 51 BR-FXEXT-* identifiers: 50 in EXTENDED and one more in MINIMUM.
	minFacturXExtensionIDs = 51
	// The EXTENDED tier alone, which is where the BR-FXEXT-* rules live, and how
	// many of the nine this package evaluates FNFE's own EXTENDED examples reach.
	minFacturXExtendedExamples = 25
	// Six of the nine: FNFE's EXTENDED examples exercise BR-FXEXT-01, -03, -06,
	// -08, -11 and -12, and reach neither BR-FXEXT-02 (a line note with a subject
	// code), BR-FXEXT-04 (an item attribute type code) nor BR-FXEXT-CII-DT-097a (a
	// date declaring format qualifier 205). Those three have firing fixtures and
	// nothing else, which is a weaker position and is why this number is logged
	// beside the rules rather than only asserted.
	minFacturXContextsReached = 6
)

// minFacturXRules is the per-profile floor on the Factur-X Schematrons, in
// rules. It is a map rather than five constants because every guard in
// facturx_test.go reads it through fxDecode, which is where a short file has to
// stop the run.
var minFacturXRules = map[Profile]int{
	ProfileMinimum:  55,
	ProfileBasicWL:  178,
	ProfileBasic:    248,
	ProfileEN16931:  373,
	ProfileExtended: 983,
}

// minFacturXDataModel is the per-profile floor on the *committed* data-model
// table, in assertions. It is a different kind of ratchet from every other one in
// this file: those hold a fetched corpus to its size and are skipped when the
// corpus is absent, and this one holds generated Go source, so it runs on a
// checkout with no artefacts at all. That is the point. Every guard in
// facturx_datamodel_test.go that compares the table against the Schematrons skips
// without them, which would leave a table somebody emptied looking green on
// exactly the checkout — CI's corpus-less job — where nothing else could notice.
var minFacturXDataModel = map[Profile]int{
	ProfileMinimum:  48,
	ProfileBasicWL:  196,
	ProfileBasic:    262,
	ProfileEN16931:  412,
	ProfileExtended: 1241,
}

// minFacturXCodeDBLists is the per-profile floor on the code databases
// `make facturx-schematron` fetches beside the five Schematrons, in <cl>
// elements. A truncated one decodes to nothing and would make the code-list
// fidelity test compare 366 assertions against an empty map.
var minFacturXCodeDBLists = map[Profile]int{
	ProfileMinimum:  8,
	ProfileBasicWL:  21,
	ProfileBasic:    25,
	ProfileEN16931:  33,
	ProfileExtended: 53,
}

const (
	// minFacturXCodeLists and minFacturXCodeValues hold the deduplicated code
	// lists in the committed table to their size: 44 distinct lists, 7,191 values,
	// out of the 140 lists the five databases declare between them.
	minFacturXCodeLists  = 44
	minFacturXCodeValues = 7191
	// The distinct element paths the five Schematrons' rule contexts name, which
	// is the population TestFacturXDataModelPathsAreUnambiguous checks the
	// local-name reduction over.
	minFacturXContextPaths = 900
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
