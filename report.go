package formalis

// Coverage: what this package checked, and what it did not.
//
// # Why a validator has to say what it did not check
//
// This package already draws one epistemic line with unusual care. limits.go
// spends eighty lines arguing that "the checker stopped" must never look like
// "the invoice is clean", and it backs the argument with RuleLimit,
// IsCheckerViolation and tests that pin the property. But there is a third
// state, and until this file existed it was folded into "clean": *the checks
// that exist all passed, and the rule set they come from is a documented
// subset*.
//
// Every rule set in this package is such a subset. Four CIUS say so in a file
// comment; the twelve national formats check "the mandatory structure and code
// lists" rather than the XSD or Schematron their authority publishes; and the
// EN 16931 core itself — the one rule set that looked complete, because the
// CEN unit-test oracle reports 198/198 — implemented, when this table was
// written, none of the CII datatype bindings and five of the fifty-four fatal
// UBL syntax rules. None of that reached a caller. Both bindings' fatal halves
// are evaluated now, and the table below is how that became a statement someone
// can check rather than a claim in a commit message. A Portuguese integrator
// could run ValidateCIUSPT, read
// len(v) == 0, file with AT, and be rejected on BR-CIUS-PT-13: a rule this
// package never evaluated and never said it had not evaluated.
//
// The consequence of confusing "unknown" with "clean" is the same in both
// cases — an invoice ships that should not have — so the two belong behind one
// question. Report.Complete is that question and Report.Conformant is the
// single predicate a caller can safely key on: it is false for a run that
// stopped early *and* for a run whose rule set has holes an authority would
// reject an invoice over.
//
// The severity on each family is what keeps the second half usable. A rule set
// with no fatal gaps left can be conformant while still not complete, so
// implementing an authority's advisory tier improves the report without being
// the price of a verdict, and a fatal gap can never be filed inside an entry
// whose prose reads as advisory — which had already happened once (C27).
//
// # Why the table is static
//
// Coverage takes a Source and reads a table. It parses nothing, allocates one
// copy of a slice of constants, and cannot fail, because what a rule set omits
// is a property of this package rather than of any document. That is what lets
// a caller ask the question *before* deciding to trust an answer, and it is
// what lets the table be the single source of truth: newReport builds
// Report.NotEvaluated from it, so the strings a caller reads are the strings
// the table holds and there is no second list to drift.
//
// # Why Source and not CIUS
//
// The obvious key is CIUS, since the CIUS validators are where the problem was
// first noticed. It does not work: CIUS names seven rule sets, and the thirteen
// tree-reading validators (FatturaPA, Facturae, ebInterface, KSeF, Finvoice,
// TEAPPS, OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT, Order-X) are not
// CIUS and are the *most* partial rule sets in the package — CIUS would have
// left every one of them unable to declare its coverage. Source already names
// every rule-authoring authority here, the seven CIUS among them, so it is the
// key that makes the table total.

// RuleFamily names a group of rules an authority publishes and this package does
// not evaluate, in a form a caller can look up in that authority's own
// documentation.
//
// Severity is what the gap costs a conformance claim: fatal means an authority
// could reject a document over a rule that was not checked here, so
// Report.Conformant must be false; a warning gap leaves the conformance question
// answerable and only makes the report less informative than a reference
// validator's. For every family in the table but two that is the authority's own
// flag, quoted. The two exceptions are families an authority flags fatal and its
// own reference implementation cannot report — CEN binds BR-CO-05..08 to the
// XPath expression true(), and three CII datatype rules sit behind an earlier
// matching Schematron rule — and there the flag and the cost part company: no
// conforming validator anywhere reports them, so not evaluating them cannot
// change any verdict. Both entries say so in Reason, with the published flag
// named, so a reader who disagrees can see exactly what was decided.
//
// This is why Severity is on the family rather than on the table as a whole. A
// residue of advisory gaps that kept Conformant false forever would make the one
// predicate a caller can safely key on useless, and the fix is not to hide the
// advisory gaps but to say which kind each one is.
type RuleFamily struct {
	// Rules is the identifier or range the authority uses — "UBL-CR-666,
	// UBL-CR-673", "BR-CIUS-PT-24..63", "BR-NL-19..35". It is the machine-ish
	// half: the tests that hold this table to the published Schematron read
	// identifiers out of this field, and the guard against claiming a rule the
	// package does in fact emit reads this field alone.
	Rules string

	// Severity is what not evaluating this family costs a conformance claim:
	// fatal when an authority could reject a document over one of these rules,
	// advisory when it could not. See the type's own comment for the two entries
	// where that is not simply the authority's published flag, and why.
	Severity Severity

	// Reason is why it is not evaluated, in prose. It is where a judgement gets
	// written down — that a rule is unenforceable by construction, that a
	// sub-profile is out of scope, that an authority publishes a schema rather
	// than a rule set.
	Reason string
}

// Report is the outcome of one validation: what the checker found, and whether
// it was in a position to find everything.
//
// The second half is the point. A bare []Violation can say "here is what is
// wrong" and "here is nothing", and a caller reasonably reads the second as
// "this invoice is fine" — but "nothing" is produced by four quite different
// runs: one that checked everything and found nothing; one that was cancelled;
// one that hit a resource budget; and one that ran a rule set which does not
// implement every rule its authority publishes. Complete separates the first
// from the other three, and Conformant is the predicate that gets it right.
//
// The zero Report is deliberately neither Conformant nor Complete, so a Report
// that was never filled in — a var nobody assigned, a struct decoded from JSON
// that had no such field, the value returned alongside an error a caller chose
// to ignore — cannot pass for a clean invoice. The unexported ran field is what
// holds that: see its comment, because the guard is easy to lose.
type Report struct {
	// Violations is every finding: the ways the document departs from the rules
	// that were evaluated, plus any statement by this checker about its own run
	// (RuleLimit, RuleProfile — see IsCheckerViolation) or about the file it was
	// handed (RuleRoot).
	Violations []Violation

	// NotEvaluated names the rule families the rule set that ran does not
	// implement. A validator that composes rule sets reports the union:
	// ValidateCIUSPT runs the EN 16931 core and the CIUS-PT rules, so its
	// NotEvaluated holds both sources' gaps.
	//
	// It is empty when no rule set was selected — an unknown Profile — because
	// naming the gaps of a rule set that was never chosen would say something
	// about the document, and nothing was checked. Complete is false there too,
	// through the RuleProfile finding.
	NotEvaluated []RuleFamily

	// ran records that this Report came out of a validation in this package.
	//
	// It exists for one reason, and the reason is not obvious enough to leave
	// implicit. Complete and Conformant are computed from the two fields above,
	// so a Report with no findings and no gaps answers true to both — and the
	// zero Report is exactly that. Every earlier shape of this type made the
	// zero value non-conformant (a Complete field defaulting to false did it for
	// free), and the contract that a Report nobody filled in cannot read as a
	// clean invoice is load-bearing: it is what makes it safe for a caller to
	// ignore an error return, decode a Report from a wire format, or copy one
	// out of a zero-valued struct field. newReport is the only thing that sets
	// this, so every one of those paths yields false and TestZeroReportIsNotConformant
	// and TestReportFromAnIgnoredErrorIsNotConformant pin both directions.
	ran bool
}

// Conformant reports whether this document may be treated as conforming to the
// rule set it was validated against.
//
// It is the conjunction the caller almost always means: nothing found that an
// authority rejects a document for, and nothing left unexamined that could have
// been. len(Violations) == 0 alone is the claim this package exists to stop
// people making, because it is equally true of a run that checked everything, a
// run that was cancelled, and a run whose rule set has fatal holes.
//
// Three things make it false:
//
//   - a fatal finding — the document breaks a rule its authority rejects for;
//   - a finding IsCheckerViolation recognises — the run was cut short, or never
//     chose a rule set, so the answer is "unknown" rather than "conformant";
//   - a fatal family in NotEvaluated — a rule that could have rejected this
//     document was never evaluated.
//
// Warnings do not: an advisory finding, and an advisory gap in the rule set, are
// both things a reference validator would report and no authority would reject
// an invoice for. Read them with Warnings and Complete respectively.
//
// The checker's own findings are tested for by IsCheckerViolation and not merely
// by their severity, deliberately. They are fatal — see Severity — but a
// stopped run must be non-conformant because the checker did not look, which is
// a different fact from the finding's weight, and this predicate should not
// quietly start depending on a severity someone could reclassify.
//
// A consequence worth stating plainly: every rule set in this package has gaps
// today, and until the fatal ones are closed Conformant returns false for every
// document. Coverage says why for any Source, and Report.NotEvaluated says why
// for any particular run. A caller who wants the older, weaker claim writes
// len(r.Fatal()) == 0 and now has r.NotEvaluated sitting beside it, naming
// exactly what that claim omits.
func (r Report) Conformant() bool {
	if !r.ran {
		return false
	}
	for _, v := range r.Violations {
		if IsCheckerViolation(v) || v.Severity == SeverityFatal {
			return false
		}
	}
	for _, f := range r.NotEvaluated {
		if f.Severity == SeverityFatal {
			return false
		}
	}
	return true
}

// Complete reports whether every rule that applies to this document was
// evaluated, advisory rules included. It is the stricter of the two questions,
// and it is false when either kind of gap is present:
//
//   - the rule set that ran does not implement some family its authority
//     publishes, which is a static fact about this package and is named in
//     NotEvaluated, whatever that family's severity; or
//   - this run stopped before it had seen everything — a cancelled context or a
//     tripped resource budget — or never chose a rule set at all, which
//     Violations reports as a finding IsCheckerViolation recognises.
//
// It was a field until severity arrived and is a method now, because with
// severity on the family the two questions genuinely differ: Conformant asks
// whether the verdict is trustworthy and passes over advisory gaps, Complete
// asks whether this package saw as much as a reference validator would. A single
// boolean could answer only one of them, and it answered the stricter one, which
// is why a residue of advisory families would have kept Conformant false forever.
//
// A document that is not an invoice at all (RuleRoot) does not make a run
// incomplete on its own account: that is a definite finding about the file, and
// Conformant is false because of the finding rather than because of any doubt.
func (r Report) Complete() bool {
	if !r.ran {
		return false
	}
	return len(r.NotEvaluated) == 0 && !anyCheckerViolation(r.Violations)
}

// Fatal returns the findings whose rules their authority rejects a document for,
// plus this checker's own findings, which are fatal for the reason Severity
// gives. It returns nil when there are none.
//
// Fatal and Warnings partition Violations, so a caller that handles both handles
// everything. Neither is a substitute for Conformant: a document can have no
// fatal findings and still not be conformant, because a rule that would have
// rejected it was never evaluated.
func (r Report) Fatal() []Violation { return r.bySeverity(SeverityFatal) }

// Warnings returns the findings whose rules their authority reports without
// rejecting the document — CEN's flag="warning" and its equivalents. It returns
// nil when there are none.
//
// These are information, not a verdict. A document whose only findings are
// warnings is conformant to the rule set that ran.
func (r Report) Warnings() []Violation { return r.bySeverity(SeverityWarning) }

// bySeverity returns a fresh slice of the findings at one severity, so that
// neither accessor hands back an alias of Violations that a caller could sort or
// truncate under the Report it came from.
func (r Report) bySeverity(s Severity) []Violation {
	var out []Violation
	for _, v := range r.Violations {
		if v.Severity == s {
			out = append(out, v)
		}
	}
	return out
}

// Coverage returns the rule families that src publishes and this package does
// not evaluate, each with what its absence costs a conformance claim. It returns
// nil for a Source whose rule set is implemented in full, and for SourceChecker,
// which publishes no rules — its identifiers (RuleLimit, RuleProfile, RuleRoot)
// are this package's statements about its own run and about the file.
//
// The result is a fresh slice: the table is package state read by every
// validator, and a caller that sorted or appended to it in place would change
// what every later Report says.
//
// It parses nothing and takes no document, so it answers before a call is made
// — "is this validator good enough for what I am about to trust it with?" —
// as well as after, through Report.NotEvaluated, which is built from this same
// table.
func Coverage(src Source) []RuleFamily {
	g := notEvaluated[src]
	if len(g) == 0 {
		return nil
	}
	return append(make([]RuleFamily, 0, len(g)), g...)
}

// notEvaluated is the coverage table: for each authority, the rule families
// this package does not evaluate. It is the single source of truth — Coverage
// returns from it and newReport builds Report.NotEvaluated from it, so a
// validator cannot state a coverage claim of its own that drifts from this one.
//
// The entries are derived from what the code emits, not from what the file
// comments claim. Three tests hold them to that:
// TestCoverageNamesNoRuleThePackageEmits sweeps the corpus and fails if an entry
// claims a rule that a validator does in fact report;
// TestEN16931CoverageNamesRulesCENPublishes checks every identifier under
// SourceEN16931 against the vendored CEN Schematron so the table cannot invent
// a rule family; and TestCoverageSeveritiesMatchThePublishedFlag checks the
// severity of every entry whose authority ships a Schematron this repository
// vendors — EN 16931, XRechnung and Peppol — against the flag on the assertion
// itself.
//
// Two writing conventions, which those tests read:
//
//   - The Rules field is the claim and the Reason field is prose. Both are read
//     by both guards, so moving a claim into the prose cannot dodge either; what
//     the split buys is that the severity and the identifiers sit where a caller
//     can use them without parsing a sentence.
//   - An entry that carves out the implemented part of a family says "other
//     than" or "emits only" in Rules and then lists it. Those two phrases mark
//     the *entry* as containing identifiers this package does report, so the
//     over-claim test skips both its fields — a Reason that explains such a
//     carve-out has to be able to name the evaluated half (BR-51 in the CII
//     binding is the example). Any other phrasing is taken as a plain claim that
//     everything the entry names is unevaluated.
//
// A family whose members do not share one flag is split into one entry per
// severity rather than recorded at the stronger of the two. That is not
// cosmetic: the two fatal UBL-CR rules sat inside an entry describing 678 rules
// as "all but two advisory" (C27), where a fatal gap was accounted for in a line
// a reader would skim as advisory, and four XRechnung provisional rules were
// filed together as "provisional/advisory" when KoSIT flags two of them fatal.
// Splitting is how the severity column stays a fact rather than a rounding.
//
// A Source absent from the map is claimed complete. Only SourceChecker is,
// today, and it is absent because it publishes no rules rather than because it
// implements all of them.
var notEvaluated = map[Source][]RuleFamily{
	// EN 16931 is the rule set that most looked complete and is not. The CEN
	// unit-test suite has error fragments for 198 rules and this package catches
	// all 198 (TestEN16931ConformanceSuite), but the suite exercises the
	// semantic model and eight of the syntax bindings; it says nothing about the
	// remaining bindings, and it has no error fragment at all for thirty model
	// rules. Reading the published Schematron instead of the oracle gives the
	// list below.
	//
	// BR-CO-05..08 are unenforceable by construction rather than unimplemented.
	// CEN binds all four to the XPath expression true() in both syntaxes, so no
	// reference validator reports them and the unit-test suite ships no fragment
	// for them. "The reason code and the free-text reason indicate the same type
	// of allowance" is a judgement about prose in an arbitrary language; any
	// mechanical stand-in would accuse conforming invoices, which is the one
	// thing this table exists to keep the package from doing.
	//
	// BR-51 is one assertion in the abstract model with two severities in the two
	// bindings: EN16931-CII-model.sch flags it fatal and EN16931-UBL-model.sch
	// flags it warning. This package reports what an authority makes fatal and
	// names its advisory rules here instead — NLCIUS's BR-NL-19..35 are the
	// precedent — so the entry carves out the half that is evaluated rather than
	// claiming the whole rule, which would send a caller to re-implement a check
	// that already runs on every Factur-X document.
	//
	// Neither binding's fatal half is here any more. All 54 fatal UBL-SR-* rules
	// are evaluated (en16931_ubl_rules.go) and so are all 42 fatal CII-SR-* and
	// 67 of the 70 fatal CII-DT-* rules (en16931_cii_rules.go). What is left
	// under EN 16931 is advisory — rules CEN flags warning, which a reference
	// validator reports and no authority rejects an invoice for — plus the three
	// fatal CII datatype rules no reference validator can reach.
	//
	// Report.Conformant is still false for every document, and deliberately so:
	// this table is not empty, and a caller keying on Conformant is asking
	// whether the rule set had holes rather than whether the holes mattered.
	SourceEN16931: {
		{
			Rules:    "BR-51 other than in the CII binding",
			Severity: SeverityWarning,
			Reason: "one assertion in the abstract model with two flags in the two bindings. EN16931-CII-model.sch flags it fatal and this package " +
				"evaluates it there; EN16931-UBL-model.sch flags it warning, so a UBL invoice carrying a full card PAN (BT-87) is not reported",
		},
		{
			Rules:    "BR-CO-05..08 (allowance/charge reason code agrees with reason text: BT-97/98, BT-104/105, BT-139/140, BT-144/145)",
			Severity: SeverityWarning,
			Reason: "CEN flags all four fatal and binds all four to the XPath expression true() in both syntaxes, so no conforming validator can " +
				"report them and the CEN unit-test suite ships no fragment for them. The gap is therefore recorded as advisory: what an authority " +
				"cannot reject a document for cannot put a verdict in doubt. \"The reason code and the free-text reason indicate the same type of " +
				"allowance\" is a judgement about prose in an arbitrary language, and any mechanical stand-in would accuse conforming invoices",
		},
		{
			Rules:    "UBL-DT-* other than UBL-DT-01/06/07",
			Severity: SeverityWarning,
			Reason:   "the 21 advisory UBL datatype rules: attribute-level restrictions CEN flags warning",
		},
		{
			Rules:    "UBL-CR-666, UBL-CR-673",
			Severity: SeverityFatal,
			Reason: "the two fatal rules of the 678-rule UBL-CR-* family — an invoice shall not include an AdditionalDocumentReference " +
				"simultaneously referring to an Invoice Object identifier and other things. They are a fatal gap and are listed apart from the " +
				"676 advisory rules of the same family for that reason: while the two sat inside one entry describing the family as \"all but two " +
				"advisory\", a fatal hole was accounted for in a line a reader would skim as advisory",
		},
		{
			Rules:    "the other 676 UBL-CR-* rules",
			Severity: SeverityWarning,
			Reason:   "advisory: UBL elements outside the EN 16931 core, which CEN flags warning to keep a document within the core subset",
		},
		{
			Rules:    "the 440 advisory CII-SR-* rules",
			Severity: SeverityWarning,
			Reason:   "of CEN's 482; all 42 fatal ones are evaluated",
		},
		{
			Rules:    "the 31 advisory CII-DT-* rules",
			Severity: SeverityWarning,
			Reason:   "of CEN's 101",
		},
		{
			Rules:    "CII-DT-010, CII-DT-011, CII-DT-012",
			Severity: SeverityWarning,
			Reason: "CEN flags these three fatal, and no reference validator reports them: the EN16931-CII-Syntax pattern matches the invoice type " +
				"code with //ram:TypeCode before the rule bound to it specifically, and ISO Schematron gives a node to the first matching rule only. " +
				"Recorded as advisory for the same reason as BR-CO-05..08 — a rule nothing can report is a rule no authority rejects for",
		},
	},

	// XRechnung: the KoSIT Schematron publishes 54 identifiers; this package
	// emits BR-DE-1..11, 14..17, 19..22, 26..28 and 30/31.
	SourceXRechnung: {
		{
			Rules:    "BR-DE-18",
			Severity: SeverityFatal,
			Reason:   "the settlement-discount payment-terms text format (a regular expression over BT-20)",
		},
		{
			Rules:    "BR-DE-23-a/b, BR-DE-24-a/b, BR-DE-25-a/b",
			Severity: SeverityFatal,
			Reason:   "the payment-means group BG-17/18/19 must match BT-81, exclusively",
		},
		{
			Rules:    "BR-DEX-01..14",
			Severity: SeverityFatal,
			Reason: "the fatal rules of the EXTENSION sub-profile. ValidateXRechnung suppresses BR-CO-16 for an EXTENSION document because " +
				"BR-DEX-09 replaces it, and does not evaluate BR-DEX-09, so that document's amount-due summation is checked by neither rule",
		},
		{
			Rules:    "BR-DEX-15",
			Severity: SeverityWarning,
			Reason:   "the one advisory rule of the EXTENSION sub-profile",
		},
		{
			Rules:    "BR-DE-CVD-01..06-b",
			Severity: SeverityFatal,
			Reason:   "the CVD sub-profile, all seven rules fatal",
		},
		{
			Rules:    "BR-TMP-3, BR-TMP-CVD-01",
			Severity: SeverityFatal,
			Reason: "provisional rules KoSIT nonetheless flags fatal. They were filed with the two advisory ones below while this table carried no " +
				"severity, which described two fatal gaps as advisory",
		},
		{
			Rules:    "BR-DE-TMP-32, BR-TMP-2",
			Severity: SeverityWarning,
			Reason:   "provisional and advisory: KoSIT flags BR-DE-TMP-32 information and BR-TMP-2 warning",
		},
	},

	// Peppol BIS Billing 3.0: the OpenPEPPOL Schematron publishes 60
	// identifiers; this package emits P0104..P0111 and R001/003/004/005/007/
	// 010/020/061/110/111/121/130.
	SourcePeppol: {
		{
			Rules:    "PEPPOL-COMMON-R040..R043, R049, R050",
			Severity: SeverityFatal,
			Reason:   "the fatal half of the participant, scheme and endpoint identifier checks",
		},
		{
			Rules:    "PEPPOL-COMMON-R044..R048, R052, R053",
			Severity: SeverityWarning,
			Reason:   "the advisory half of the same family: scheme-deprecation and recommendation warnings",
		},
		{
			Rules:    "PEPPOL-EN16931-CL001/002/003/006/007/008",
			Severity: SeverityFatal,
			Reason:   "Peppol's own code-list restrictions, narrower than the EN 16931 lists this package checks",
		},
		{
			Rules:    "PEPPOL-EN16931-F001, P0100, P0101, P0112",
			Severity: SeverityFatal,
			Reason:   "the syntax-binding and profile-identifier assertions",
		},
		{
			Rules:    "PEPPOL-EN16931-R002/R006/R008/R040..R046/R051/R053..R055/R080/R100/R101/R120",
			Severity: SeverityFatal,
			Reason:   "field-length, structural and accounting-currency assertions Peppol adds to the core",
		},
	},

	// NLCIUS is the one CIUS whose fatal rule set is implemented in full
	// (BR-NL-1..5 and 7..13; there is no BR-NL-6). Its gap is advisory only.
	SourceNLCIUS: {
		{
			Rules:    "BR-NL-19..35",
			Severity: SeverityWarning,
			Reason:   "NLCIUS's \"not recommended\" rules, which do not make an invoice non-conformant",
		},
	},

	// The four CIUS below publish their rule sets as prose or as a Schematron
	// this repository does not vendor, so there is no flag to quote and the
	// severities are this package's fail-safe reading: a gap it cannot show to be
	// advisory is recorded fatal. Lowering one of these to a warning needs
	// evidence from the authority, not an argument from plausibility.
	SourceCIUSPT: {
		{
			Rules:    "BR-CIUS-PT-13/15/17/18",
			Severity: SeverityFatal,
			Reason:   "the Portuguese VAT-category rate rules, which encode AT's rates rather than EN 16931's structure",
		},
		{
			Rules:    "BR-CIUS-PT-24..63",
			Severity: SeverityFatal,
			Reason:   "conditional structural completeness: \"if this optional UBL group is present, its mandatory child must be too\"",
		},
		{
			Rules:    "any other BR-CIUS-PT rule: this package emits only 01, 03, 05, 06, 07, 10, 11, 21, 22, 23, 64, 66",
			Severity: SeverityFatal,
			Reason:   "the rest of the published set",
		},
	},

	SourceCIUSRO: {
		{
			Rules:    "BR-RO-L*",
			Severity: SeverityFatal,
			Reason:   "per-field maximum-length limits",
		},
		{
			Rules:    "BR-DEC-RO-*",
			Severity: SeverityFatal,
			Reason:   "Romanian decimal limits",
		},
		{
			Rules:    "BR-RO-065/120",
			Severity: SeverityFatal,
			Reason:   "allowance/charge-conditional VAT identifiers, which overlap the EN 16931 core",
		},
		{
			Rules:    "any other BR-RO rule: this package emits only 010, 020, 030, 081, 082, 091, 092, 100, 101, 110, 111, 140, 150, 160, 170, 180, 201, 202, 211, 212",
			Severity: SeverityFatal,
			Reason:   "the rest of the published set",
		},
	},

	SourceUBLBE: {
		{
			Rules:    "ubl-BE-06 and ubl-BE-12",
			Severity: SeverityFatal,
			Reason:   "the BELMText and BVERCText bilingual free-text description code lists",
		},
		{
			Rules:    "any other ubl-BE rule: this package emits only ubl-BE-02, 03, 05, 07, 08, 09, 10, 11, 13, 14, 15",
			Severity: SeverityFatal,
			Reason:   "the rest of the published set",
		},
	},

	SourceSRBDT: {
		{
			Rules:    "RSK-X-*",
			Severity: SeverityFatal,
			Reason:   "the Serbian VAT-category rules",
		},
		{
			Rules:    "the finer identifier and endpoint cross-checks",
			Severity: SeverityFatal,
			Reason:   "BT-8 code values, endpoint-contains-PIB, buyer registration format",
		},
		{
			Rules:    "any other RSR rule: this package emits only RSR-03, 04, 09, 10, 11, 13, 14, 16, 17, 20, 21, 22, 23, 25",
			Severity: SeverityFatal,
			Reason:   "the rest of the published set",
		},
	},

	// The national formats below publish an XSD (and, for OIOUBL, a Schematron)
	// rather than a business-rule set this package could quote, and each
	// validator checks the mandatory structure and code lists rather than the
	// whole schema. The identifiers under these Sources were minted here, so
	// there is no published family to name: the entry says what the authority
	// checks that this package does not.
	//
	// Every one of these gaps is fatal, and that is the least interesting
	// severity in the table rather than a default nobody thought about. What is
	// unevaluated here is a schema: a document that violates it is rejected at
	// the border by the authority's own gateway, which is the definition of fatal
	// this table uses. None of these authorities publishes an advisory tier for
	// its schema, so there is no warning half to split off.
	SourceFatturaPA: {{
		Rules:    "the SdI FatturaPA XSD and the SdI's consistency checks",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure and Italian code lists the FPA-* rules cover",
	}},
	SourceFacturae: {{
		Rules:    "the Facturae XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure and Spanish code lists the FE-* rules cover",
	}},
	SourceEbInterface: {{
		Rules:    "the ebInterface XSD (schema versions 3.x..6.x)",
		Severity: SeverityFatal,
		Reason:   "beyond the version-independent mandatory structure the EB-* rules cover",
	}},
	SourceKSeF: {{
		Rules:    "the KSeF FA XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure and Polish code lists the KS-* rules cover",
	}},
	SourceFinvoice: {{
		Rules:    "the Finvoice XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the FI-* rules cover",
	}},
	SourceTEAPPS: {{
		Rules:    "the TEAPPSXML XSD",
		Severity: SeverityFatal,
		Reason:   "beyond each invoice's type and customer information the TP-* rules cover",
	}},
	SourceOIOUBL: {{
		Rules:    "the OIOUBL Schematron",
		Severity: SeverityFatal,
		Reason:   "beyond the profile, core document terms, electronic addresses and seller name the OIO-* rules cover",
	}},
	SourceSvefaktura: {{
		Rules:    "the SFTI Svefaktura 1.0 (UBL 1.0) XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the SV-* rules cover",
	}},
	SourceZATCA: {{
		Rules:    "the ZATCA XSD and the Fatoora platform's reporting/clearance checks",
		Severity: SeverityFatal,
		Reason: "including the cryptographic stamp, the QR payload and the invoice hash chain — beyond the mandatory structure the ZA-* rules " +
			"cover",
	}},
	SourceOSA: {{
		Rules:    "the NAV Online Számla XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the HU-* rules cover",
	}},
	SourceUBLTR: {{
		Rules:    "the UBL-TR XSD, and the non-invoice UBL-TR document types (despatch advice, response)",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the TR-* rules cover; the other document types this validator does not accept at all",
	}},
	SourcePINT: {{
		Rules:    "the PINT core rule set and every jurisdiction rule set (AE, AUNZ, EU, JP, MY, OM, SG)",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure every jurisdiction shares that the PINT-* rules cover",
	}},
	SourceOrderX: {{
		Rules:    "the Order-X document rules",
		Severity: SeverityFatal,
		Reason:   "beyond the five mandatory head terms this package checks",
	}},
}

// newReport assembles one validator's answer: the findings it gathered, and the
// coverage of the rule sets it ran.
//
// sources is the authorities whose rules were applied, in the order they were
// applied, and it is what makes the coverage claim follow the call rather than
// the entry point: ValidateCIUSPT passes SourceEN16931 and SourceCIUSPT because
// it runs both, and ValidateCIUS passes whatever the arbitration routed the
// document to — a pair for a CIUS layered on the core, one Source for a national
// format with a rule set of its own. Passing none says no rule set was chosen —
// the unknown-Profile
// case — and yields an empty NotEvaluated with Complete still false, because
// the RuleProfile finding is a checker violation.
// It is also the only thing that sets Report.ran, which is what keeps a Report
// nobody produced from reading as a clean invoice. Every Report a caller can
// obtain from this package comes from here or is the zero value, and the zero
// value answers false to both questions.
func newReport(vs []Violation, sources ...Source) Report {
	return Report{Violations: vs, NotEvaluated: coverageUnion(sources), ran: true}
}

// coverageUnion concatenates the coverage gaps of sources, in order, dropping
// the repeats a composed rule set would otherwise produce. Families are deduped
// on the whole value, so two authorities that happened to word one gap alike
// would still both be reported unless they agree on its severity and reason too.
func coverageUnion(sources []Source) []RuleFamily {
	var out []RuleFamily
	seen := map[RuleFamily]bool{}
	for _, src := range sources {
		for _, g := range notEvaluated[src] {
			if seen[g] {
				continue
			}
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
}

// anyCheckerViolation reports whether any finding is this checker speaking about
// its own run rather than about the document — the stopped-run half of Complete.
func anyCheckerViolation(vs []Violation) bool {
	for _, v := range vs {
		if IsCheckerViolation(v) {
			return true
		}
	}
	return false
}
