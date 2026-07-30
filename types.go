// Package formalis validates electronic invoices against the EN 16931 semantic
// model, the Core Invoice Usage Specifications (CIUS) layered on top of it, and
// the national invoice formats that stand outside EN 16931 altogether. It uses
// nothing but the Go standard library.
//
// One syntax-neutral rule engine (fed by parseEN16931) serves both EN 16931
// syntaxes — UN/CEFACT Cross Industry Invoice (CII, used by Factur-X/ZUGFeRD)
// and OASIS UBL (Peppol BIS, XRechnung UBL, NLCIUS) — and each CIUS adds its own
// rule layer in its own file (xrechnung.go, peppol.go, nlcius.go, cius_pt.go,
// cius_ro.go, cius_be.go, cius_rs.go). The formats that are not EN 16931
// profiles at all — FatturaPA, Facturae, ebInterface, KSeF, Finvoice, TEAPPS,
// OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT — and the Order-X order
// document are checked against their own mandatory structure and code lists by
// validators of their own. Source names every one of these authorities, and a
// rule is identified by the pair (Source, Rule) rather than by Rule alone.
//
// # Entry points
//
// Every exported validator takes a context and the document's bytes and returns
// a Report and an error. Which one to call:
//
//   - Detect routes. It reports which format a document is in, in one streaming
//     pass that builds no tree, and Detection.Validator returns the entry point
//     that checks it.
//   - ValidateCIUS applies that same arbitration and validates in one call,
//     falling back to the EN 16931 core for a document that declares nothing.
//   - Validate checks the EN 16931 core against a named Factur-X Profile, for a
//     caller who has that data-richness metadata from a container the invoice
//     itself does not carry.
//   - The format-specific validators (ValidateXRechnung, ValidateZATCA, …) are
//     the direct route when the format is already known.
//
// The twelve Is* predicates answer "is this document format X?" for one format
// each. They are independent tests and not a partition — more than one can be
// true of the same bytes — so route with Detect, which arbitrates between them
// in an order it documents.
//
// # The four answers a validator can give
//
// The distinctions this package exists for are all in how a call can end, so they
// are worth reading once:
//
//   - An error, with the zero Report: the input could not be read at all —
//     malformed XML, an encoding this package does not implement. A statement
//     about the file. See ErrMalformedXML and ErrUnsupportedEncoding.
//   - A Report holding a RuleLimit finding: the run stopped before it had seen
//     everything, so nothing can be concluded. A statement about the run, and
//     never an empty Violations slice. See RuleLimit and IsCheckerViolation.
//   - A Report holding findings: the document departs from rules that were
//     evaluated. Each carries the Severity its authority gave the rule, so
//     Report.Fatal and Report.Warnings separate "reject this" from "note this".
//   - A Report holding no findings: everything that was evaluated passed. What was
//     evaluated is the other half of the answer, and Report.NotEvaluated is where
//     it is written down.
//
// No rule set in this package evaluates everything its authority publishes: each
// is a documented subset. Coverage names the gaps for any Source, with the
// severity of each and whether anyone could evaluate it at all, and it takes no
// document, so a caller can ask what a validator will not look at before deciding
// to call it. Report.NotEvaluated repeats those gaps for the run that just
// happened; Report.Conformant is false whenever a rule that could have rejected
// this document was one a validator could have evaluated and this package did not,
// and Report.Complete whenever any evaluable rule went unevaluated. Today the EN
// 16931 core is the one rule set with no unevaluated fatal rule and the one whose
// clean documents report both Conformant and Complete; every CIUS and national
// validator still names a fatal gap it could close and so reports false whatever
// the document.
//
// The distinction the third field carries is worth one sentence here, because it
// is what makes Complete answerable rather than permanently false: CEN publishes
// seven rules that no validator can evaluate — four bound to the XPath expression
// true(), three unreachable in CEN's own Schematron rule ordering — and a rule
// nobody can check is not a rule this package skipped. See RuleFamily.Unevaluable,
// which documents how narrow that is, Report and Coverage.
//
// Three rule sets report warnings, and every other finding in this package is
// fatal. CEN flags 1,168 of its two syntax bindings' assertions warning rather
// than fatal — the UBL-CR-*, UBL-DT-*, CII-SR-* and CII-DT-* rules that hold a
// document down to the EN 16931 core subset of UBL and CII — and this package
// evaluates all of them from tables generated out of CEN's own Schematron. KoSIT
// flags eleven of XRechnung's fifty-seven rules warning or information: the
// invoice type code, the specification identifier, the two IBAN checks, the
// telephone and email formats and five more. OpenPEPPOL flags six of its
// fifty-nine warning — the Italian, Danish and Swedish participant-identifier
// format checks — and one more, PEPPOL-EN16931-R120, is fatal in OpenPEPPOL's own
// Schematron and warning in the XRechnung artefact that merges it, so the same
// rule is a non-conformance on one path and advisory on the other. None of these
// is a verdict: a document whose only findings are these is Conformant, and a
// caller gating on Report.Fatal never sees them. See Severity and
// Report.Warnings.
//
// # Concurrency
//
// There is no global mutable state. The code-list tables, the compiled regexps
// and the generated syntax-binding tables (parsed once at load by
// en16931_syntax_advisory.go)
// are package-level values initialised once at load and only read afterwards,
// and every per-call artefact — the run, the parsed tree, the semantic model —
// is allocated inside the call. Every exported function may therefore be called
// from any number of goroutines at once; TestValidatorsAreSafeForConcurrentUse
// pins that under -race.
//
// # A note on pdf0, which this package's comments refer to
//
// This package reads XML and has no PDF dependency. pdf0
// (github.com/mgilbir/pdf0) is a separate, public sibling module that wraps it
// for the Factur-X container: pdf0 opens the PDF, extracts the attached invoice
// XML, and calls in here. The dependency runs one way — pdf0 requires formalis,
// and nothing in this module imports or needs pdf0 — so the references to it in
// limits.go, orderx.go and facturx_en16931.go are design constraints, not
// dependencies.
// They record conventions the two modules deliberately share, RuleLimit above
// all, so that a caller draining one mixed slice of container and invoice
// findings has one name to look for rather than two. A reader with no interest
// in PDF containers can disregard every one of them; nothing here changes
// behaviour because of pdf0.
package formalis

import (
	"fmt"
	"strings"
)

// Profile is a Factur-X/ZUGFeRD conformance profile: how much of the EN 16931
// semantic model a document undertakes to carry, in increasing data richness.
// The five values below are the whole set, and Profile means only this. It is
// not a rule set chosen by nationality: a Core Invoice Usage Specification is a
// separate concept carried by CIUS, reached through ValidateCIUS or a
// CIUS-specific validator such as ValidateXRechnung.
//
// Keeping the two apart is what makes the type checkable. While XRechnung was
// also a Profile constant it was accepted by Validate and applied no BR-DE-*
// rule, so the call that looked most like "validate this as XRechnung" was the
// one that validated it least; and because any string was accepted, a
// mistyped profile was silently read as EN 16931. Validate now reports a
// Profile it does not implement — see RuleProfile.
//
// Only three of the five change what Validate checks, and only in these ways:
// MINIMUM and BASIC WL are head-only, so the invoice-line rules (BR-12, BR-16)
// are not applied to them; MINIMUM additionally omits the buyer postal address
// (BR-10, BR-11), the VAT breakdown (BR-CO-18) and the amount-due summation
// (BR-CO-16); and EXTENDED is exempt from the allowance/charge total
// summations (BR-CO-11, BR-CO-12), whose operands it may carry unitemized.
// BASIC, EN 16931 and EXTENDED are otherwise checked alike.
// TestProfilesThatDifferStillDiffer pins each of those differences.
type Profile string

const (
	ProfileMinimum  Profile = "MINIMUM"
	ProfileBasicWL  Profile = "BASIC WL"
	ProfileBasic    Profile = "BASIC"
	ProfileEN16931  Profile = "EN 16931"
	ProfileExtended Profile = "EXTENDED"
)

// profiles is every Profile this package implements, in the order Profile
// documents them. It is the single list that knownProfile, the message
// unknownProfile writes, and TestProfileForRoundTrips all read.
var profiles = []Profile{ProfileMinimum, ProfileBasicWL, ProfileBasic, ProfileEN16931, ProfileExtended}

// knownProfile reports whether p is one of the five profiles this package
// implements. Validate consults it before doing any work; see RuleProfile.
func knownProfile(p Profile) bool {
	for _, k := range profiles {
		if p == k {
			return true
		}
	}
	return false
}

// conformanceKey folds an XMP ConformanceLevel string for matching: producers
// write both "EN 16931" and "EN16931", and "BASIC WL" and "BASICWL".
func conformanceKey(level string) string {
	return strings.ToUpper(strings.ReplaceAll(level, " ", ""))
}

// ProfileFor maps an XMP ConformanceLevel string to the Factur-X profile it
// names. The value is matched case- and space-insensitively.
//
// It reports false for "XRECHNUNG", which is a level a ZUGFeRD 2.x producer
// really does write but which names the German CIUS rather than a data-richness
// profile; CIUSFor maps that one. A caller reading a PDF's XMP therefore asks
// both, and gets from the pair the two facts the metadata actually carries —
// how rich the data claims to be, and which national rule set it claims to
// follow — instead of one value that conflates them. Neither returning
// ("", false) for the level (which loses it) nor returning a Profile that no
// validator honours (which was the bug) would do that.
func ProfileFor(level string) (Profile, bool) {
	switch conformanceKey(level) {
	case "MINIMUM":
		return ProfileMinimum, true
	case "BASICWL":
		return ProfileBasicWL, true
	case "BASIC":
		return ProfileBasic, true
	case "EN16931":
		return ProfileEN16931, true
	case "EXTENDED":
		return ProfileExtended, true
	}
	return "", false
}

// CIUSFor maps an XMP ConformanceLevel string to the CIUS it names, for the
// levels that name one rather than a Factur-X profile. It is the companion to
// ProfileFor over the same input, matched the same way; exactly one of the two
// reports true for any level either recognises.
//
// Today that is only "XRECHNUNG" (ZUGFeRD 2.x). The CIUS it returns is the one
// ValidateCIUS routes on and ValidateXRechnung implements, so a caller that
// reaches here reaches a validator that actually applies the BR-DE-* rules.
//
// This says what the container's metadata claims. DetectCIUS says what the
// invoice itself declares in BT-24, and that is the more reliable of the two:
// prefer it, and treat a disagreement as the container and its attachment
// describing different documents.
func CIUSFor(level string) (CIUS, bool) {
	switch conformanceKey(level) {
	case "XRECHNUNG":
		return CIUSXRechnung, true
	}
	return CIUSNone, false
}

// Source identifies the authority that defines a rule, so that a rule identifier
// is unique within it. Two authorities may mint the same string; (Source, Rule)
// is what identifies a rule, and Rule alone is not an identity.
//
// The distinction is not decorative. This package reports identifiers minted by
// CEN, by seven national bodies, and by itself, in one flat string field, and
// they have already collided: the Order-X validator once emitted "BR-O-01" for
// "an Order shall have an order number" while the EN 16931 rule engine emitted
// "BR-O-01" for the "Not subject to VAT" category family. A caller aggregating
// findings across a mailbox keyed by Rule merged two unrelated defects, and a
// suppression list ("we accept BR-O-03 from this supplier") suppressed the wrong
// thing in the other document type. Scoping every finding by its author makes
// that class of mistake impossible to express.
//
// Three judgement calls are recorded here because they are not obvious:
//
//   - The UBL and CII syntax-binding rules (UBL-DT-*, UBL-SR-*, CII-SR-*,
//     CII-DT-*) carry
//     SourceEN16931, not a Source of their own. Source names the *authority*, and
//     CEN publishes the bindings (EN 16931-3-2, EN 16931-3-3) as normative parts
//     of the same standard as the semantic model (EN 16931-1); they arrive in the
//     same conformance artefacts this package is tested against, and the FP=0
//     oracle counts them among the rules it must catch. Splitting them would also
//     make one finding's Source depend on the invoice's syntax, since the same
//     defect — two disagreeing payment means codes — is UBL-SR-47 on a UBL
//     invoice and CII-SR-467 on a CII one, which is a distinction the caller did
//     not ask for. A caller that does want
//     "syntax binding" separately has the prefix, which is already disjoint from
//     the core BR-* space.
//
//   - RuleLimit, RuleProfile and RuleRoot carry SourceChecker. They are statements
//     by this checker — "I stopped early", "you named a profile I do not
//     implement", "this is not an invoice" — rather than by any rule authority, so
//     attributing them to CEN would be a lie, and leaving them unattributed would
//     make Source unusable as a filter. See IsCheckerViolation for why that
//     predicate still tests Rule alone. A document this package could not read at
//     all produces no finding under any Source: it is an error, because there is
//     no document to attribute anything about.
//
//   - Most national formats below (FatturaPA, Facturae, ebInterface, KSeF,
//     Finvoice, TEAPPS, OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT,
//     Order-X) do not publish a rule identifier this package could quote, so the
//     identifiers under those Sources — "FPA-number", "ZA-uuid", "ORDER-01" —
//     were invented here. The Source is still the format they judge the document
//     against, which is what a caller routing or suppressing by format needs; it
//     is not a claim that the format's own documentation uses these names. The
//     Sources whose identifiers *are* quoted from a published rule set are
//     EN 16931, XRechnung, Peppol, NLCIUS, CIUS-PT, CIUS-RO, UBL.BE and SRBDT.
type Source string

const (
	// SourceNone is the absent authority — no rule set. It is the zero Source,
	// and it is what Detect reports for a document it read and recognised as no
	// format this package validates; Detection.Recognised tests for it. No
	// Violation ever carries it: a finding with no Source is an emission site
	// that did not decide whose rule it reports, which
	// TestNoRuleIdentifierIsClaimedByTwoSources fails on.
	SourceNone Source = ""
	// SourceEN16931 is CEN's EN 16931 — the semantic model's core business rules
	// (BR-*, BR-CO-*, BR-CL-*, BR-DEC-*, BR-IC-*, and the VAT category families)
	// together with the UBL and CII syntax bindings (UBL-DT-*, UBL-SR-*, CII-SR-*,
	// CII-DT-*).
	SourceEN16931 Source = "EN 16931"
	// SourceXRechnung is the German KoSIT XRechnung CIUS (BR-DE-*).
	SourceXRechnung Source = "XRechnung"
	// SourcePeppol is OpenPEPPOL BIS Billing 3.0: PEPPOL-EN16931-* and
	// PEPPOL-COMMON-*, and the country-specific rules the same two Schematron files
	// publish under a comment reading "National rules" — DE-R-*, DK-R-*,
	// GR-R-*/GR-S-*, IS-R-*, IT-R-*, NL-R-*, NO-R-* and SE-R-*.
	//
	// Those last are OpenPEPPOL's own national rule sets and not the CIUS of the same
	// countries: NL-R-* is distinct from the BR-NL-* of SourceNLCIUS, and DE-R-* from
	// the BR-DE-* of SourceXRechnung, which is why they carry this Source and not
	// those.
	//
	// It is the one Source a validator for another authority emits: the released
	// XRechnung Schematron merges twenty-one PEPPOL-EN16931-* rules in, so
	// ValidateXRechnung reports them — under OpenPEPPOL's Source, because Source
	// names the authority that wrote the rule. It imports none of the country rules;
	// see the comment on the coverage table in report.go.
	SourcePeppol Source = "Peppol"
	// SourceNLCIUS is the Dutch SimplerInvoicing NLCIUS (BR-NL-*).
	SourceNLCIUS Source = "NLCIUS"
	// SourceCIUSPT is the Portuguese AT/eSPap CIUS-PT: BR-CIUS-PT-*, and AT's own
	// BR-AA-* — eight rules for the "Lower rate" (AA) VAT category, written by
	// cloning CEN's BR-S-* template for a category code EN 16931 leaves out of
	// BT-118's restricted list. CEN publishes no BR-AA-* family, so the identifier
	// looks like CEN's and is not, and this Source is where it belongs.
	SourceCIUSPT Source = "CIUS-PT"
	// SourceCIUSRO is the Romanian ANAF RO e-Factura CIUS (BR-RO-*).
	SourceCIUSRO Source = "CIUS-RO"
	// SourceUBLBE is the Belgian UBL.BE CIUS (ubl-BE-*).
	SourceUBLBE Source = "UBL.BE"
	// SourceSRBDT is the Serbian SRBDT CIUS (RSR-*).
	SourceSRBDT Source = "SRBDT"
	// SourceFatturaPA is the Italian SdI FatturaPA format (FPA-*).
	SourceFatturaPA Source = "FatturaPA"
	// SourceFacturae is the Spanish Facturae format (FE-*).
	SourceFacturae Source = "Facturae"
	// SourceEbInterface is the Austrian ebInterface format (EB-*).
	SourceEbInterface Source = "ebInterface"
	// SourceKSeF is the Polish KSeF FA(2) format (KS-*).
	SourceKSeF Source = "KSeF"
	// SourceFinvoice is the Finnish Finvoice format (FI-*).
	SourceFinvoice Source = "Finvoice"
	// SourceTEAPPS is the Finnish Tieto TEAPPSXML format (TP-*).
	SourceTEAPPS Source = "TEAPPS"
	// SourceOIOUBL is the Danish OIOUBL profile (OIO-*).
	SourceOIOUBL Source = "OIOUBL"
	// SourceSvefaktura is the Swedish Svefaktura format (SV-*).
	SourceSvefaktura Source = "Svefaktura"
	// SourceZATCA is the Saudi ZATCA e-invoicing profile (ZA-*).
	SourceZATCA Source = "ZATCA"
	// SourceOSA is the Hungarian NAV Online Számla format (HU-*).
	SourceOSA Source = "NAV OSA"
	// SourceUBLTR is the Turkish UBL-TR e-Fatura profile (TR-*).
	SourceUBLTR Source = "UBL-TR"
	// SourcePINT is the Peppol International (PINT) billing model (PINT-*).
	SourcePINT Source = "PINT"
	// SourceOrderX is the Franco-German Order-X order document (ORDER-*).
	SourceOrderX Source = "Order-X"
	// SourceChecker is this package speaking about its own run, or about the file
	// it was handed, rather than about any rule: RuleLimit, RuleProfile and
	// RuleRoot.
	SourceChecker Source = "checker"
)

// Severity is how much weight the authority that wrote a rule puts on breaking
// it: enough to reject the document, or not.
//
// It is a property of the rule and not of the run or of the document, and it is
// not this package's opinion. CEN writes it into the Schematron as
// flag="fatal" or flag="warning" on every assertion, and the national
// authorities quoted here do the same (KoSIT adds flag="information", which is
// advisory under another name). This type carries that flag folded onto two
// values, because two is what a caller can act on: a fatal finding is a reason
// to refuse an invoice and a warning is not.
//
// The same assertion can carry different flags in the two EN 16931 syntax
// bindings — BR-51 is fatal in the CII binding and a warning in the UBL one —
// so severity belongs on the finding, where the binding that produced it is
// known, rather than on a table keyed by rule identifier.
//
// # Why SeverityFatal is the zero value
//
// An unstamped Severity reads as fatal, deliberately. The two orderings are not
// symmetric. With SeverityWarning at zero, an emission site that forgot the
// field would report a genuine non-conformance as advisory: Report.Conformant
// would pass over it, Report.Fatal would omit it, and an invoice would ship on
// the strength of a field nobody set. With SeverityFatal at zero the same
// omission over-reports — a caller sees a blocking finding for an advisory rule,
// notices, and the stamp gets fixed. This package exists to keep silence from
// being read as a clean invoice, so the default has to lean the other way.
//
// The default is not a substitute for deciding, and it is not left unchecked:
// TestOnlyTheAdvisoryBindingsAreEmittedAsWarnings sweeps the whole corpus and
// fails on any finding whose severity does not match the half of the package it
// came from, and TestEveryEmittedEN16931RuleCarriesCENsFlag holds the one Source
// with vendored ground truth to the flag CEN publishes — in both directions, so
// an advisory rule stamped fatal by the zero value fails as loudly as a fatal one
// stamped advisory.
//
// # This package's own rule identifiers
//
// Most national formats publish no rule identifier this package could quote, so
// the identifiers under those Sources — FPA-*, FE-*, EB-*, KS-*, FI-*, ZA-*,
// SV-*, TP-*, OIO-*, TR-*, HU-*, PINT-*, ORDER-* — were minted here (see
// Source). No authority has flagged them, so their severity is a decision rather
// than a quotation, and the decision is that all of them are fatal.
//
// The reason is what those rules check rather than who wrote them down. Each is a
// mandatory element of the format's own schema or a value outside its own code
// list, so a document that breaks one is a document the authority's gateway
// rejects at the border: the SdI refuses a FatturaPA with no invoice number, and
// Fatoora refuses a ZATCA invoice with no UUID. That is precisely what fatal
// means here. If one of these formats is later found to publish a genuinely
// advisory expectation and this package implements it, it belongs at
// SeverityWarning and TestOnlyTheAdvisoryBindingsAreEmittedAsWarnings is where
// the change gets recorded.
//
// # SourceChecker findings
//
// RuleLimit, RuleProfile and RuleRoot are this package's statements about its
// own run or about the file it was handed, so no authority has flagged them and
// severity is this package's classification rather than a quotation. They are
// fatal, for the reason above rather than by analogy: a caller who writes
// len(r.Fatal()) == 0 as their release gate must not have a cancelled run, a
// rejected Profile or a document that is not an invoice slip through it. A third
// value for "not a rule" was considered and rejected — it would make Fatal and
// Warnings no longer a partition of Violations, and every caller who switched on
// severity would have to invent a policy for the third case, most of them
// choosing the unsafe one. The distinction those callers actually need is
// already exported, precisely and tested: IsCheckerViolation.
type Severity int

const (
	// SeverityFatal is a rule its authority rejects a document for breaking.
	SeverityFatal Severity = iota
	// SeverityWarning is a rule its authority reports without rejecting: CEN's
	// flag="warning", KoSIT's flag="information", NLCIUS's "not recommended".
	// A document with warnings and no fatal finding is conformant.
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityFatal:
		return "fatal"
	case SeverityWarning:
		return "warning"
	}
	return fmt.Sprintf("Severity(%d)", int(s))
}

// Violation reports one way in which a document departs from a rule set. Source
// names the authority that defines the rule and Rule is that authority's
// identifier for it (e.g. SourceEN16931/"BR-CO-15", SourceNLCIUS/"BR-NL-1");
// neither is an identity on its own. Severity is whether that authority rejects
// a document for it.
type Violation struct {
	Source   Source
	Rule     string
	Severity Severity
	Message  string
}

// Error renders the finding, severity included. The severity is in the string
// rather than only in the field because this type satisfies error: a caller that
// logs a Violation would otherwise present an advisory finding in exactly the
// same words as a blocking one, which is the confusion Severity exists to end.
func (v Violation) Error() string {
	return fmt.Sprintf("%s %s (%s): %s", v.Source, v.Rule, v.Severity, v.Message)
}

// adder returns the emission helper a rule set uses to append its findings,
// stamping src on each one.
//
// Every rule set builds its own, so the Source is fixed at the point of
// emission rather than painted on afterwards. That matters because several
// validators emit under more than one authority in a single call — the
// XRechnung validator reports EN 16931 core findings alongside BR-DE-*, as do
// the CIUS-PT, CIUS-RO, NLCIUS, Peppol, SRBDT and UBL.BE validators — so
// stamping one Source over a returned slice would misattribute the core half.
//
// The severity is written out rather than left to Severity's zero value, and
// the helpers do not take a Severity argument for the same reason: which
// severity a rule carries is a fact about the rule, so it belongs where the rule
// set decides it and not in an argument a call could get wrong by omission.
//
// There are four of them, one per way that fact is known. adder is for a rule set
// whose rules are all fatal, which is most of them. advisoryAdder is for the
// generated EN 16931 syntax bindings, where the whole table is advisory. xrAdder,
// in xrechnung_rules.go, is for a rule set that is neither: KoSIT flags eleven of
// its fifty-seven identifiers warning or information and the rest fatal, so its
// severity is a per-identifier lookup with a test that reads the flags back out of
// the Schematron. peppolEval.add, in peppol_rules.go, is the fourth and the only one
// that needs to know *which artefact* is being quoted, because one Peppol rule
// carries two published flags: fatal in OpenPEPPOL's own Schematron and warning in
// the XRechnung build that merges it.
func adder(out *[]Violation, src Source) func(rule, msg string) {
	return func(rule, msg string) {
		*out = append(*out, Violation{Source: src, Rule: rule, Severity: SeverityFatal, Message: msg})
	}
}

// advisoryAdder is adder for a rule its authority reports without rejecting the
// document: CEN's flag="warning". Its findings are what Report.Warnings returns,
// they are absent from Report.Fatal, and they do not move Report.Conformant.
//
// The generated EN 16931 syntax-binding tables use it — see
// en16931_syntax_advisory.go, which is also where the argument for reporting them
// at all is made. A national format's minted identifier does not belong here: no
// authority has flagged those, so this package had to decide, and it decided they
// are fatal for the reason Severity gives.
func advisoryAdder(out *[]Violation, src Source) func(rule, msg string) {
	return func(rule, msg string) {
		*out = append(*out, Violation{Source: src, Rule: rule, Severity: SeverityWarning, Message: msg})
	}
}
