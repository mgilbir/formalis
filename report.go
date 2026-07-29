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
// CEN unit-test oracle reports 198/198 — implements none of the CII datatype
// bindings and five of the fifty-four fatal UBL syntax rules. None of that
// reached a caller. A Portuguese integrator could run ValidateCIUSPT, read
// len(v) == 0, file with AT, and be rejected on BR-CIUS-PT-13: a rule this
// package never evaluated and never said it had not evaluated.
//
// The consequence of confusing "unknown" with "clean" is the same in both
// cases — an invoice ships that should not have — so the two belong behind one
// question. Report.Complete is that question and Report.Conformant is the
// single predicate a caller can safely key on: it is false for a run that
// stopped early *and* for a run whose rule set has holes.
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
// The zero Report is deliberately not conformant: Complete is false, so a
// Report that was never filled in cannot pass for a clean invoice.
type Report struct {
	// Violations is every finding: the ways the document departs from the rules
	// that were evaluated, plus any statement by this checker about its own run
	// (RuleLimit, RuleSyntax, RuleProfile — see IsCheckerViolation).
	Violations []Violation

	// Complete reports whether every rule that applies to this document was
	// evaluated. It is false when either kind of gap is present:
	//
	//   - the rule set that ran does not implement some family its authority
	//     publishes, which is a static fact about this package and is named in
	//     NotEvaluated; or
	//   - this run stopped before it had seen everything — a cancelled context
	//     or a tripped resource budget — or never chose a rule set at all,
	//     which Violations reports as a finding IsCheckerViolation recognises.
	//
	// A malformed document does not make a run incomplete on its own account:
	// "this file is not well-formed XML" is a definite finding about the
	// document, and Conformant is false because of the finding, not because of
	// the doubt. (In practice a parse failure is reported by a validator whose
	// rule set is partial anyway, so Complete is false for the first reason.)
	Complete bool

	// NotEvaluated names the rule families the rule set that ran does not
	// implement, in a form a caller can look up in the authority's own
	// documentation — "BR-CIUS-PT-24..63", "BR-NL-19..35 (advisory)". A
	// validator that composes rule sets reports the union: ValidateCIUSPT runs
	// the EN 16931 core and the CIUS-PT rules, so its NotEvaluated holds both
	// sources' gaps.
	//
	// It is empty when no rule set was selected — an unknown Profile — because
	// naming the gaps of a rule set that was never chosen would say something
	// about the document, and nothing was checked. Complete is false there too,
	// through the RuleProfile finding.
	NotEvaluated []string
}

// Conformant reports whether this document may be treated as conforming to the
// rule set it was validated against.
//
// It is the conjunction the caller almost always means: no findings *and*
// nothing left unexamined. len(Violations) == 0 alone is the claim this package
// exists to stop people making, because it is equally true of a run that
// checked everything, a run that was cancelled, and a run whose rule set has
// documented holes.
//
// Being honest about that has a consequence worth stating plainly: no rule set
// in this package is complete today, so Conformant returns false for every
// document. Coverage says why for any Source, and Report.NotEvaluated says why
// for any particular run. A caller who wants the older, weaker claim writes
// len(r.Violations) == 0 and now has r.NotEvaluated sitting beside it, naming
// exactly what that claim omits.
func (r Report) Conformant() bool {
	return len(r.Violations) == 0 && r.Complete
}

// Coverage returns the rule families that src publishes and this package does
// not evaluate. It returns nil for a Source whose rule set is implemented in
// full, and for SourceChecker, which publishes no rules — its identifiers
// (RuleLimit, RuleSyntax, RuleProfile) are this package's statements about its
// own run.
//
// The result is a fresh slice: the table is package state read by every
// validator, and a caller that sorted or appended to it in place would change
// what every later Report says.
//
// It parses nothing and takes no document, so it answers before a call is made
// — "is this validator good enough for what I am about to trust it with?" —
// as well as after, through Report.NotEvaluated, which is built from this same
// table.
func Coverage(src Source) []string {
	g := notEvaluated[src]
	if len(g) == 0 {
		return nil
	}
	return append(make([]string, 0, len(g)), g...)
}

// notEvaluated is the coverage table: for each authority, the rule families
// this package does not evaluate. It is the single source of truth — Coverage
// returns from it and newReport builds Report.NotEvaluated from it, so a
// validator cannot state a coverage claim of its own that drifts from this one.
//
// The entries are derived from what the code emits, not from what the file
// comments claim. Two tests hold them to that:
// TestCoverageNamesNoRuleThePackageEmits sweeps the corpus and fails if an entry
// claims a rule that a validator does in fact report, and
// TestEN16931CoverageNamesRulesCENPublishes checks every identifier under
// SourceEN16931 against the vendored CEN Schematron so the table cannot invent
// a rule family.
//
// One writing convention, which those tests read: an entry that carves out the
// implemented part of a family says "other than" or "emits only" and then lists
// it. Those two phrases mark an entry as containing identifiers this package
// *does* report, so the over-claim test skips it. Any other phrasing is taken
// as a plain claim that everything it names is unevaluated.
//
// A Source absent from the map is claimed complete. Only SourceChecker is,
// today, and it is absent because it publishes no rules rather than because it
// implements all of them.
var notEvaluated = map[Source][]string{
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
	// The fatal half of the UBL syntax binding is no longer here. All 54 fatal
	// UBL-SR-* rules are evaluated (en16931_ubl_rules.go). The CII binding is
	// being closed the same way, family by family, in en16931_cii_rules.go; the
	// two entries below shrink as that lands and name what is still outstanding at
	// each step. Report.Conformant is false for every document throughout.
	SourceEN16931: {
		"BR-51 other than in the CII binding: EN16931-CII-model.sch flags it fatal and this package evaluates it there, while EN16931-UBL-model.sch flags it warning, so a UBL invoice carrying a full card PAN (BT-87) is not reported",
		"BR-CO-05..08 (allowance/charge reason code agrees with reason text: BT-97/98, BT-104/105, BT-139/140, BT-144/145) — CEN binds all four to true() in both syntaxes, so they are unenforceable rather than unimplemented",
		"UBL-DT-* other than UBL-DT-01/06/07 (the 21 advisory UBL datatype rules)",
		"UBL-CR-* (678 rules, all but two advisory: UBL elements outside the EN 16931 core)",
		"the 440 advisory CII-SR-* rules (of CEN's 482; all 42 fatal ones are evaluated)",
		"CII-DT-* other than the 39 fatal rules on the document element, the identifiers, the codes, the values and the document references (31 of the 70 fatal CII datatype rules remain, three of which — CII-DT-010, CII-DT-011 and CII-DT-012 — are unreachable: the EN16931-CII-Syntax pattern matches the invoice type code with //ram:TypeCode before the rule bound to it specifically, and ISO Schematron gives a node to the first matching rule only, so no reference validator reports them either; and 31 advisory ones)",
	},

	// XRechnung: the KoSIT Schematron publishes 54 identifiers; this package
	// emits BR-DE-1..11, 14..17, 19..22, 26..28 and 30/31.
	SourceXRechnung: {
		"BR-DE-18 (settlement-discount payment-terms text format)",
		"BR-DE-23-a/b, BR-DE-24-a/b, BR-DE-25-a/b (the payment-means group BG-17/18/19 must match BT-81, exclusively)",
		"BR-DEX-01..15 (the EXTENSION sub-profile). ValidateXRechnung suppresses BR-CO-16 for an EXTENSION document because BR-DEX-09 replaces it, and does not evaluate BR-DEX-09",
		"BR-DE-CVD-01..06-b (the CVD sub-profile)",
		"BR-DE-TMP-32, BR-TMP-2, BR-TMP-3, BR-TMP-CVD-01 (provisional/advisory)",
	},

	// Peppol BIS Billing 3.0: the OpenPEPPOL Schematron publishes 60
	// identifiers; this package emits P0104..P0111 and R001/003/004/005/007/
	// 010/020/061/110/111/121/130.
	SourcePeppol: {
		"PEPPOL-COMMON-R040..R053 (participant, scheme and endpoint identifier checks)",
		"PEPPOL-EN16931-CL001/002/003/006/007/008 (Peppol's own code-list restrictions)",
		"PEPPOL-EN16931-F001, P0100, P0101, P0112",
		"PEPPOL-EN16931-R002/R006/R008/R040..R046/R051/R053..R055/R080/R100/R101/R120",
	},

	// NLCIUS is the one CIUS whose fatal rule set is implemented in full
	// (BR-NL-1..5 and 7..13; there is no BR-NL-6). Its gap is advisory only.
	SourceNLCIUS: {
		"BR-NL-19..35 (advisory: NLCIUS's \"not recommended\" rules, which do not make an invoice non-conformant)",
	},

	SourceCIUSPT: {
		"BR-CIUS-PT-13/15/17/18 (Portuguese VAT-category rate rules)",
		"BR-CIUS-PT-24..63 (conditional structural completeness: \"if this optional UBL group is present, its mandatory child must be too\")",
		"any other BR-CIUS-PT rule: this package emits only 01, 03, 05, 06, 07, 10, 11, 21, 22, 23, 64, 66",
	},

	SourceCIUSRO: {
		"BR-RO-L* (per-field maximum-length limits)",
		"BR-DEC-RO-* (Romanian decimal limits)",
		"BR-RO-065/120 (allowance/charge-conditional VAT identifiers, which overlap the EN 16931 core)",
		"any other BR-RO rule: this package emits only 010, 020, 030, 081, 082, 091, 092, 100, 101, 110, 111, 140, 150, 160, 170, 180, 201, 202, 211, 212",
	},

	SourceUBLBE: {
		"ubl-BE-06 and ubl-BE-12 (the BELMText and BVERCText bilingual free-text description code lists)",
		"any other ubl-BE rule: this package emits only ubl-BE-02, 03, 05, 07, 08, 09, 10, 11, 13, 14, 15",
	},

	SourceSRBDT: {
		"RSK-X-* (the Serbian VAT-category rules)",
		"the finer identifier and endpoint cross-checks: BT-8 code values, endpoint-contains-PIB, buyer registration format",
		"any other RSR rule: this package emits only RSR-03, 04, 09, 10, 11, 13, 14, 16, 17, 20, 21, 22, 23, 25",
	},

	// The national formats below publish an XSD (and, for OIOUBL, a Schematron)
	// rather than a business-rule set this package could quote, and each
	// validator checks the mandatory structure and code lists rather than the
	// whole schema. The identifiers under these Sources were minted here, so
	// there is no published family to name: the entry says what the authority
	// checks that this package does not.
	SourceFatturaPA: {
		"the SdI FatturaPA XSD and the SdI's consistency checks, beyond the mandatory structure and Italian code lists the FPA-* rules cover",
	},
	SourceFacturae: {
		"the Facturae XSD, beyond the mandatory structure and Spanish code lists the FE-* rules cover",
	},
	SourceEbInterface: {
		"the ebInterface XSD (schema versions 3.x..6.x), beyond the version-independent mandatory structure the EB-* rules cover",
	},
	SourceKSeF: {
		"the KSeF FA XSD, beyond the mandatory structure and Polish code lists the KS-* rules cover",
	},
	SourceFinvoice: {
		"the Finvoice XSD, beyond the mandatory structure the FI-* rules cover",
	},
	SourceTEAPPS: {
		"the TEAPPSXML XSD, beyond each invoice's type and customer information the TP-* rules cover",
	},
	SourceOIOUBL: {
		"the OIOUBL Schematron, beyond the profile, core document terms, electronic addresses and seller name the OIO-* rules cover",
	},
	SourceSvefaktura: {
		"the SFTI Svefaktura 1.0 (UBL 1.0) XSD, beyond the mandatory structure the SV-* rules cover",
	},
	SourceZATCA: {
		"the ZATCA XSD and the Fatoora platform's reporting/clearance checks — including the cryptographic stamp, the QR payload and the invoice hash chain — beyond the mandatory structure the ZA-* rules cover",
	},
	SourceOSA: {
		"the NAV Online Számla XSD, beyond the mandatory structure the HU-* rules cover",
	},
	SourceUBLTR: {
		"the UBL-TR XSD, beyond the mandatory structure the TR-* rules cover; and the non-invoice UBL-TR document types (despatch advice, response), which this validator does not accept",
	},
	SourcePINT: {
		"the PINT core rule set and every jurisdiction rule set (AE, AUNZ, EU, JP, MY, OM, SG), beyond the mandatory structure every jurisdiction shares that the PINT-* rules cover",
	},
	SourceOrderX: {
		"the Order-X document rules, beyond the five mandatory head terms this package checks",
	},
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
func newReport(vs []Violation, sources ...Source) Report {
	r := Report{Violations: vs, NotEvaluated: coverageUnion(sources)}
	r.Complete = len(r.NotEvaluated) == 0 && !anyCheckerViolation(vs)
	return r
}

// coverageUnion concatenates the coverage gaps of sources, in order, dropping
// the repeats a composed rule set would otherwise produce.
func coverageUnion(sources []Source) []string {
	var out []string
	seen := map[string]bool{}
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
