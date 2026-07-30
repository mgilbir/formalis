package formalis

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The five national rule sets whose Schematron `make cius-schematron` vendors, and
// the guards that hold this package's reading of them to the artefact.
//
// Until those files were fetched, CIUS-PT, CIUS-RO, UBL.BE, SRBDT and NLCIUS were
// the five rule sets in this package written from prose. Nothing checked which
// identifiers their authorities publish, what flag each carries, which syntax
// binding publishes it, or whether an implemented rule's condition is the
// authority's condition — and the coverage table's severity column for them was a
// fail-safe guess rather than a quotation (C35).
//
// Everything below reads the artefacts with an XML decoder, never a regular
// expression. That is C31's lesson and it is not a style preference: the character
// class in `<(?:sch:)?(?:assert|report)\s([^>]*)>` stops at the first '>', an
// XPath expression may contain one, and three of KoSIT's assertions did — which is
// how a 57-rule set was surveyed as 54 for several PRs. A decoder also declines to
// see a commented-out assertion, which is what makes a phantom (C34) visible.
//
// What the derivation found, recorded here because the numbers are the point:
//
//   - Severity: nothing to correct. All 355 CIUS-PT, 125 CIUS-RO, 15 UBL.BE and 46
//     SRBDT identifiers are flagged fatal by their authorities, and NLCIUS's split
//     is 12 fatal / 22 advisory in the UBL binding and 11 / 22 in the CII one,
//     which is what the table already claimed. Unlike C29 and C32, the severity
//     guesses here were right.
//   - Binding scope: four of the five publish for UBL only, and this package
//     evaluated all four on CII as well. That is C32's eight-rule defect again, in
//     a larger size.
//   - Transcription: fifteen implemented rules did not say what their published
//     XPath says. See the per-authority commits.
//   - One rule, ubl-BE-13, is bound to an expression that cannot be false.

// ciusArtefact describes one authority's vendored rule set: where the files are,
// which identifiers in them are the authority's own, and which syntax binding
// publishes them.
//
// The prefix filter is load-bearing rather than tidy. Three of these files are
// *merged* artefacts that carry other authorities' rules as dependencies:
// GLOBALUBL.BE.sch holds CEN's 300-odd, OpenPEPPOL's 48 and five of OpenPEPPOL's
// country sets alongside the 15 ubl-BE-* rules, and CIUS-PT's abstract files are a
// modified copy of CEN's in which UBL-SR-19/20/21 are flagged warning where CEN
// flags them fatal. Reading a file wholesale and calling the result "the Belgian
// rule set" is how a survey comes to attribute CEN's rules to Belgium — and, worse
// here, how a modified copy of CEN's flags would come to overrule CEN's own.
type ciusArtefact struct {
	source  Source
	binding string   // "UBL" or "CII"
	globs   []string // relative to testdata/
	own     *regexp.Regexp
	// minIDs is the ratchet. A short or empty .sch decodes to nothing and would
	// otherwise leave every guard below vacuously green, which is the failure mode
	// corpus_test.go's floors exist for; the same reasoning applies to an artefact.
	minIDs int
}

var ciusArtefacts = []ciusArtefact{{
	source:  SourceCIUSPT,
	binding: "UBL",
	globs:   []string{"cius-pt/schematron/*/*.sch", "cius-pt/schematron/*/*/*.sch"},
	// BR-AA-* is AT's, not CEN's, and the filter has to say so. CEN publishes a
	// rule family per VAT category code in EN 16931's restricted BT-118 list and
	// 'AA' ("Lower rate") is not in that list, so there is no CEN BR-AA-* family —
	// verified by decoding every vendored EN 16931 Schematron and by grep over their
	// bytes. AT wrote these eight by cloning CEN's BR-S-* template for the reduced
	// rates Portugal levies.
	//
	// Until this filter named them, they were invisible to every guard below: the
	// severity check skips an identifier the table does not evaluate, and
	// TestEveryPublishedCIUSRuleIsEvaluatedOrDisclaimed only sees identifiers this
	// regexp admits. Eight fatal published rules were therefore in neither the code
	// nor the record of what the code does not do — C38's shape, one prefix further
	// out than the DT-CIUS-PT-* family that finding was about.
	own:    regexp.MustCompile(`^(?:BR-AA|(?:BR|DT)-CIUS-PT)-`),
	minIDs: 363,
}, {
	// Only cius-ro/RO16931-rules.sch is fetched, so the UBL/, abstract/ and
	// codelist/ siblings that are CEN's cannot be read by accident. BR-27 is in
	// this file too — CIUS-RO re-publishes one CEN identifier verbatim — and the
	// prefix filter excludes it, because an identifier CEN minted stays CEN's.
	//
	// The glob names **one release**, and that is the substantive part. ANAF ships
	// four and they are not the same rule set; the union of them is 125 identifiers
	// and no release publishes 125. Reading all four made the guard below —
	// "every published rule is evaluated or disclaimed" — ask this package to
	// account for four identifiers ANAF withdrew (BR-RO-020, BR-RO-A999,
	// BR-RO-L0301 and BR-RO-L0309), and a withdrawn rule is not a coverage gap; it
	// also let a family pass that guard by *prefix accident*, because "BR-RO-A051"
	// in a coverage entry contains the string "BR-RO". Scoping to the release this
	// package evaluates makes the question exact, and TestCIUSROVersionsDiffer pins
	// what all four publish and names each withdrawn rule's successor — which is
	// more information than the union count was, not less.
	source:  SourceCIUSRO,
	binding: "UBL",
	globs:   []string{"cius-ro/schematron/1.0.9/cius-ro/*.sch"},
	own:     regexp.MustCompile(`^BR-(?:RO|DEC-RO)-`),
	minIDs:  121,
}, {
	source:  SourceUBLBE,
	binding: "UBL",
	globs:   []string{"cius-be/schematron/*/*.sch"},
	own:     regexp.MustCompile(`^ubl-BE-`),
	minIDs:  15,
}, {
	source:  SourceSRBDT,
	binding: "UBL",
	globs:   []string{"cius-rs/schematron/*/*.sch"},
	own:     regexp.MustCompile(`^RS[REK]-`),
	minIDs:  46,
}, {
	source:  SourceNLCIUS,
	binding: "UBL",
	globs:   []string{"nlcius/schematron/ubl/*.sch"},
	own:     regexp.MustCompile(`^BR-NL-`),
	minIDs:  34,
}, {
	source:  SourceNLCIUS,
	binding: "CII",
	globs:   []string{"nlcius/schematron/cii/*.sch"},
	own:     regexp.MustCompile(`^BR-NL-`),
	minIDs:  33,
}}

// ciusPublished reads the vendored national Schematrons and returns, per Source
// and per binding, the flag set each published identifier carries.
//
// It returns nil when the artefacts are absent, so every caller skips rather than
// passing on an empty map: a guard that reads no artefact must say so, not report
// that it found no disagreement.
func ciusPublished(t *testing.T) map[Source]map[string]map[string]map[string]bool {
	t.Helper()
	out := map[Source]map[string]map[string]map[string]bool{}
	for _, a := range ciusArtefacts {
		var files []string
		for _, g := range a.globs {
			m, _ := filepath.Glob(filepath.Join("testdata", g))
			files = append(files, m...)
		}
		if len(files) == 0 {
			return nil
		}
		ids := map[string]map[string]bool{}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for id, flags := range assertFlags(t, f, data) {
				if !a.own.MatchString(id) {
					continue
				}
				if ids[id] == nil {
					ids[id] = map[string]bool{}
				}
				for fl := range flags {
					ids[id][fl] = true
				}
			}
		}
		if len(ids) < a.minIDs {
			t.Fatalf("%s/%s: decoded %d of its own identifiers from %d vendored files, want at least %d — "+
				"the artefact is truncated or the prefix filter stopped matching, and either way every guard "+
				"over this rule set is now weaker than it reads; re-fetch with "+
				"`make clean-cius-oracles cius-oracles`", a.source, a.binding, len(ids), len(files), a.minIDs)
		}
		if out[a.source] == nil {
			out[a.source] = map[string]map[string]map[string]bool{}
		}
		out[a.source][a.binding] = ids
	}
	return out
}

// ciusFlagsBySource folds ciusPublished's per-binding view onto one flag set per
// identifier, the way schematronFlags does for the three rule sets it reads. A rule
// published in both bindings contributes both flags, and pickFlag resolves the
// disagreement the same fail-safe way.
func ciusFlagsBySource(t *testing.T) map[Source]map[string]map[string]bool {
	t.Helper()
	pub := ciusPublished(t)
	if pub == nil {
		return nil
	}
	out := map[Source]map[string]map[string]bool{}
	for src, bindings := range pub {
		merged := map[string]map[string]bool{}
		for _, ids := range bindings {
			for id, flags := range ids {
				if merged[id] == nil {
					merged[id] = map[string]bool{}
				}
				for fl := range flags {
					merged[id][fl] = true
				}
			}
		}
		out[src] = merged
	}
	return out
}

// ciusEvaluated is what this package claims to evaluate, per Source, with the flag
// the authority publishes for each identifier.
//
// It exists for three reasons, and the third is the one that makes it worth
// maintaining by hand:
//
//   - so "every rule this package emits carries its authority's flag" is a
//     statement about a set rather than about whatever the corpus happened to trip
//     (TestCIUSSeveritiesQuoteTheirAuthority reads it in both directions);
//   - so a rule that stops being emitted fails the build instead of quietly
//     disappearing, which is what C27, C30 and C33 all were
//     (TestEveryEvaluatedCIUSRuleFires); and
//   - so an identifier this package invents is visible. It is how BR-RO-020 was
//     found: CIUS-RO publishes BR-RO-020_1 and BR-RO-020_2 and no BR-RO-020, so a
//     finding under that name named a rule no authority had written.
//
// Every value is SeverityFatal, which is a derived fact and not a convention:
// these five authorities flag every identifier this package evaluates fatal, so
// the plain adder is the right one. The table is still per-identifier because the
// alternative is to assert "all fatal" once and have nothing to check it against —
// NLCIUS publishes 22 advisory identifiers in each binding, so the rule set this
// package draws from is demonstrably not uniformly fatal.
var ciusEvaluated = map[Source]map[string]Severity{
	// CIUS-PT is the one rule set here that is complete: all 65 published
	// BR-CIUS-PT-* identifiers, all 8 BR-AA-*, and — merged in by the init function
	// in cius_pt_datatype_test.go rather than pasted here — all 290 DT-CIUS-PT-*
	// identifiers of the generated datatype tier. The absence of BR-CIUS-PT-31 from
	// this list is not an omission: AT publishes no such identifier, and
	// TestCIUSPTFamilyHasNoPhantom says so out of the file.
	SourceCIUSPT: {
		"BR-CIUS-PT-01": SeverityFatal, "BR-CIUS-PT-02": SeverityFatal,
		"BR-CIUS-PT-03": SeverityFatal, "BR-CIUS-PT-04": SeverityFatal,
		"BR-CIUS-PT-05": SeverityFatal, "BR-CIUS-PT-06": SeverityFatal,
		"BR-CIUS-PT-07": SeverityFatal, "BR-CIUS-PT-08": SeverityFatal,
		"BR-CIUS-PT-09": SeverityFatal, "BR-CIUS-PT-10": SeverityFatal,
		"BR-CIUS-PT-11": SeverityFatal, "BR-CIUS-PT-12": SeverityFatal,
		"BR-CIUS-PT-13": SeverityFatal, "BR-CIUS-PT-14": SeverityFatal,
		"BR-CIUS-PT-15": SeverityFatal, "BR-CIUS-PT-16": SeverityFatal,
		"BR-CIUS-PT-17": SeverityFatal, "BR-CIUS-PT-18": SeverityFatal,
		"BR-CIUS-PT-19": SeverityFatal, "BR-CIUS-PT-20": SeverityFatal,
		"BR-CIUS-PT-21": SeverityFatal, "BR-CIUS-PT-22": SeverityFatal,
		"BR-CIUS-PT-23": SeverityFatal, "BR-CIUS-PT-24": SeverityFatal,
		"BR-CIUS-PT-25": SeverityFatal, "BR-CIUS-PT-26": SeverityFatal,
		"BR-CIUS-PT-27": SeverityFatal, "BR-CIUS-PT-28": SeverityFatal,
		"BR-CIUS-PT-29": SeverityFatal, "BR-CIUS-PT-30": SeverityFatal,
		"BR-CIUS-PT-32": SeverityFatal, "BR-CIUS-PT-33": SeverityFatal,
		"BR-CIUS-PT-34": SeverityFatal, "BR-CIUS-PT-35": SeverityFatal,
		"BR-CIUS-PT-36": SeverityFatal, "BR-CIUS-PT-37": SeverityFatal,
		"BR-CIUS-PT-38": SeverityFatal, "BR-CIUS-PT-39": SeverityFatal,
		"BR-CIUS-PT-40": SeverityFatal, "BR-CIUS-PT-41": SeverityFatal,
		"BR-CIUS-PT-42": SeverityFatal, "BR-CIUS-PT-43": SeverityFatal,
		"BR-CIUS-PT-44": SeverityFatal, "BR-CIUS-PT-45": SeverityFatal,
		"BR-CIUS-PT-46": SeverityFatal, "BR-CIUS-PT-47": SeverityFatal,
		"BR-CIUS-PT-48": SeverityFatal, "BR-CIUS-PT-49": SeverityFatal,
		"BR-CIUS-PT-50": SeverityFatal, "BR-CIUS-PT-51": SeverityFatal,
		"BR-CIUS-PT-52": SeverityFatal, "BR-CIUS-PT-53": SeverityFatal,
		"BR-CIUS-PT-54": SeverityFatal, "BR-CIUS-PT-55": SeverityFatal,
		"BR-CIUS-PT-56": SeverityFatal, "BR-CIUS-PT-57": SeverityFatal,
		"BR-CIUS-PT-58": SeverityFatal, "BR-CIUS-PT-59": SeverityFatal,
		"BR-CIUS-PT-60": SeverityFatal, "BR-CIUS-PT-61": SeverityFatal,
		"BR-CIUS-PT-62": SeverityFatal, "BR-CIUS-PT-63": SeverityFatal,
		"BR-CIUS-PT-64": SeverityFatal, "BR-CIUS-PT-65": SeverityFatal,
		"BR-CIUS-PT-66": SeverityFatal,
		"BR-AA-01":      SeverityFatal, "BR-AA-02": SeverityFatal,
		"BR-AA-03": SeverityFatal, "BR-AA-04": SeverityFatal,
		"BR-AA-05": SeverityFatal, "BR-AA-06": SeverityFatal,
		"BR-AA-07": SeverityFatal, "BR-AA-10": SeverityFatal,
	},
	// CIUS-RO is the second complete rule set here: the 25 BR-RO-NNN business rules
	// below by hand in cius_ro.go, and — merged in by the init function in
	// cius_ro_rules_test.go rather than pasted here — the 90 evaluable assertions of
	// the generated length, decimal, date-format and occurrence tier. The six ANAF
	// publishes that no processor can report are in Coverage(SourceCIUSRO) with
	// Unevaluable set, and TestCIUSROUnevaluableAssertsAreDerivedFromTheArtefact is
	// the evidence.
	SourceCIUSRO: {
		"BR-RO-001": SeverityFatal, "BR-RO-010": SeverityFatal, "BR-RO-020_1": SeverityFatal,
		"BR-RO-020_2": SeverityFatal, "BR-RO-030": SeverityFatal, "BR-RO-040": SeverityFatal,
		"BR-RO-065": SeverityFatal, "BR-RO-081": SeverityFatal, "BR-RO-082": SeverityFatal,
		"BR-RO-091": SeverityFatal, "BR-RO-092": SeverityFatal, "BR-RO-100": SeverityFatal,
		"BR-RO-101": SeverityFatal, "BR-RO-110": SeverityFatal, "BR-RO-111": SeverityFatal,
		"BR-RO-120": SeverityFatal, "BR-RO-140": SeverityFatal, "BR-RO-150": SeverityFatal,
		"BR-RO-160": SeverityFatal, "BR-RO-170": SeverityFatal, "BR-RO-180": SeverityFatal,
		"BR-RO-201": SeverityFatal, "BR-RO-202": SeverityFatal, "BR-RO-211": SeverityFatal,
		"BR-RO-212": SeverityFatal,
	},
	SourceUBLBE: {
		"ubl-BE-01": SeverityFatal, "ubl-BE-02": SeverityFatal, "ubl-BE-03": SeverityFatal,
		"ubl-BE-04": SeverityFatal, "ubl-BE-05": SeverityFatal, "ubl-BE-06": SeverityFatal,
		"ubl-BE-07": SeverityFatal, "ubl-BE-08": SeverityFatal, "ubl-BE-09": SeverityFatal,
		"ubl-BE-10": SeverityFatal, "ubl-BE-11": SeverityFatal, "ubl-BE-12": SeverityFatal,
		"ubl-BE-14": SeverityFatal, "ubl-BE-15": SeverityFatal,
		// ubl-BE-13 is absent on purpose: the authority binds it to an expression
		// that cannot be false, so it is Unevaluable rather than evaluated. See
		// ublBE13Reason and TestUBLBE13IsBoundToATautology.
	},
	SourceSRBDT: {
		"RSR-03": SeverityFatal, "RSR-04": SeverityFatal, "RSR-09": SeverityFatal,
		"RSR-10": SeverityFatal, "RSR-11": SeverityFatal, "RSR-13": SeverityFatal,
		"RSR-14": SeverityFatal, "RSR-16": SeverityFatal, "RSR-17": SeverityFatal,
		"RSR-20": SeverityFatal, "RSR-21": SeverityFatal, "RSR-22": SeverityFatal,
		"RSR-23": SeverityFatal, "RSR-25": SeverityFatal,
	},
	SourceNLCIUS: {
		"BR-NL-1": SeverityFatal, "BR-NL-2": SeverityFatal, "BR-NL-3": SeverityFatal,
		"BR-NL-4": SeverityFatal, "BR-NL-5": SeverityFatal, "BR-NL-7": SeverityFatal,
		"BR-NL-8": SeverityFatal, "BR-NL-9": SeverityFatal, "BR-NL-10": SeverityFatal,
		"BR-NL-11": SeverityFatal, "BR-NL-12": SeverityFatal, "BR-NL-13": SeverityFatal,
	},
}

// TestCIUSSeveritiesQuoteTheirAuthority is the C29 check for these five rule sets:
// for every identifier this package evaluates, the severity it reports is the flag
// the authority published, compared in both directions against the artefact.
//
// The direction that matters is the one C29 was: an advisory rule reported fatal
// makes this package refuse an invoice its own authority accepts, and no
// false-positive oracle can catch it, because the corpora are conforming documents
// and such a rule only fires on a document that departs from an advisory rule.
// KoSIT flagged seven that way and this package reported all seven fatal.
//
// It also fails on an identifier the artefact does not publish, which is the C34
// check. ciusEvaluated is a set of claims about somebody else's file; an entry the
// file does not carry is not a rule.
func TestCIUSSeveritiesQuoteTheirAuthority(t *testing.T) {
	flags := ciusFlagsBySource(t)
	if flags == nil {
		t.Skip("national CIUS Schematron not present; run `make cius-schematron`")
	}
	checked := 0
	for src, evaluated := range ciusEvaluated {
		published := flags[src]
		if published == nil {
			t.Fatalf("no vendored artefact was read for %q, so this test checked none of its %d rules",
				src, len(evaluated))
		}
		for id, sev := range evaluated {
			got, ok := published[id]
			if !ok {
				t.Errorf("this package evaluates %s/%s, which its authority's Schematron does not publish. An "+
					"identifier no artefact carries is not a rule: either it is spelled the way the prose spells "+
					"it rather than the way the artefact does, or it does not exist", src, id)
				continue
			}
			want, known := severityOfFlag(pickFlag(got))
			if !known {
				t.Errorf("%s/%s carries the flag %v, which this package cannot fold onto a Severity", src, id, keysOf(got))
				continue
			}
			checked++
			if sev != want {
				t.Errorf("this package reports %s/%s as %s, but its authority flags it %v; the severity a finding "+
					"carries is a quotation and not a choice", src, id, sev, keysOf(got))
			}
		}
	}
	if checked < 130 {
		t.Fatalf("checked only %d evaluated identifiers against their authorities' flags; the harness is not "+
			"reading the artefacts", checked)
	}
	t.Logf("checked %d evaluated CIUS identifiers against the flags their authorities publish, both directions, "+
		"with no exceptions", checked)
}

// TestCIUSPublishedInventory pins what the five artefacts publish, per binding.
//
// It is the survey C35 says nobody had done, written down so it cannot drift
// silently. The counts matter twice over: they are what the coverage table's claims
// are measured against, and the fatal/advisory split is the fact that made NLCIUS's
// advisory entry checkable at all.
//
// Floors rather than equalities, because upstream adds rules — but the fatal counts
// are equalities for the four rule sets that publish no advisory tier, since a
// *fall* there would mean a rule was withdrawn and that belongs in a commit
// message.
func TestCIUSPublishedInventory(t *testing.T) {
	pub := ciusPublished(t)
	if pub == nil {
		t.Skip("national CIUS Schematron not present; run `make cius-schematron`")
	}
	for _, want := range []struct {
		src              Source
		binding          string
		ids, fatal, warn int
	}{
		// 363, not the 355 PR 22 recorded: the survey's identifier filter stopped at
		// the BR-CIUS-PT and DT-CIUS-PT prefixes and did not admit AT's own eight
		// BR-AA-* rules.
		{SourceCIUSPT, "UBL", 363, 363, 0},
		// 121, not the 125 PR 22 recorded: 125 is the union of ANAF's four vendored
		// releases and no release publishes it. This row is release 1.0.9, the one
		// this package evaluates; TestCIUSROVersionsDiffer holds all four to
		// 112/112/113/121 and names the four withdrawn identifiers.
		{SourceCIUSRO, "UBL", 121, 121, 0},
		{SourceUBLBE, "UBL", 15, 15, 0},
		{SourceSRBDT, "UBL", 46, 46, 0},
		{SourceNLCIUS, "UBL", 34, 12, 22},
		{SourceNLCIUS, "CII", 33, 11, 22},
	} {
		ids := pub[want.src][want.binding]
		if ids == nil {
			t.Errorf("no %s binding was read for %q", want.binding, want.src)
			continue
		}
		fatal, warn := 0, 0
		for id := range ids {
			if pickFlag(ids[id]) == "fatal" {
				fatal++
			} else {
				warn++
			}
		}
		if len(ids) < want.ids || fatal < want.fatal || warn < want.warn {
			t.Errorf("%s/%s publishes %d identifiers (%d fatal, %d advisory), want at least %d (%d, %d) — a "+
				"count that fell means a rule was withdrawn upstream or the reader stopped seeing it",
				want.src, want.binding, len(ids), fatal, warn, want.ids, want.fatal, want.warn)
		}
		t.Logf("%s/%s: %d published identifiers, %d fatal, %d advisory", want.src, want.binding, len(ids), fatal, warn)
	}
	// The bindings are not the same rule set, and this is the fact that decides
	// which findings may be reported for a CII document. NLCIUS is the only one of
	// the five that publishes a CII binding at all, and even there the two sets
	// differ: BR-NL-8 is UBL-only (it asserts that the type code agrees with the
	// UBL document element, a question CII does not have), and BR-NL-22/23 are
	// CII-only.
	ublOnly, ciiOnly := diffIDs(pub[SourceNLCIUS]["UBL"], pub[SourceNLCIUS]["CII"])
	if strings.Join(ublOnly, " ") != "BR-NL-32-1 BR-NL-32-2 BR-NL-32-3 BR-NL-8" {
		t.Errorf("NLCIUS publishes %v in UBL and not in CII; the expected difference is BR-NL-8 and the three "+
			"BR-NL-32-N, and a change here changes which findings a CII document may carry", ublOnly)
	}
	if strings.Join(ciiOnly, " ") != "BR-NL-22 BR-NL-23 BR-NL-32-and-34" {
		t.Errorf("NLCIUS publishes %v in CII and not in UBL; the expected difference is BR-NL-22, BR-NL-23 and "+
			"BR-NL-32-and-34", ciiOnly)
	}
}

// diffIDs returns the sorted identifiers in a but not b, and in b but not a.
func diffIDs(a, b map[string]map[string]bool) (onlyA, onlyB []string) {
	for id := range a {
		if _, ok := b[id]; !ok {
			onlyA = append(onlyA, id)
		}
	}
	for id := range b {
		if _, ok := a[id]; !ok {
			onlyB = append(onlyB, id)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return onlyA, onlyB
}

// TestCIUSFindingsStayInsideTheEvaluatedSet is the other half of ciusEvaluated: the
// table is a claim about what this package emits, and the whole corpus is what
// checks it.
//
// Without this, ciusEvaluated could name a rule the code stopped emitting (a gap
// with no symptom) or the code could emit one the table does not name (a rule whose
// severity and existence nothing checks). Both have happened in this repository
// under other names — C27 is the first and C30 the second.
func TestCIUSFindingsStayInsideTheEvaluatedSet(t *testing.T) {
	s := corpusSweep()
	for src, evaluated := range ciusEvaluated {
		for rule := range s.bySeverity[src] {
			if _, ok := evaluated[rule]; !ok {
				t.Errorf("the sweep saw %s/%s reported, and ciusEvaluated does not name it. Either the table lost "+
					"an entry or the rule is emitted under a name its authority does not publish", src, rule)
			}
		}
	}
	if s.files > 0 {
		atLeast(t, "CIUS artefact sweep corpus", s.files, minCorpusDocuments)
	}
}

// TestCoverageNamesOnlyRulesTheseAuthoritiesPublish is
// TestEN16931CoverageNamesRulesCENPublishes for the five national rule sets, and it
// is what a vendored artefact buys the coverage table.
//
// A coverage entry is a claim that an authority publishes a rule this package does
// not evaluate. Before the artefacts were here, half of that claim was
// uncheckable, and two of the entries turn out to have been naming identifiers
// nobody publishes — the same shape as the two Peppol phantoms in C34, which could
// not be caught for the same reason: the severity guard skips an identifier the
// artefact does not carry, so a phantom is invisible to it by construction.
func TestCoverageNamesOnlyRulesTheseAuthoritiesPublish(t *testing.T) {
	flags := ciusFlagsBySource(t)
	if flags == nil {
		t.Skip("national CIUS Schematron not present; run `make cius-schematron`")
	}
	checked := 0
	for src, published := range flags {
		for _, entry := range Coverage(src) {
			for _, text := range []string{entry.Rules, entry.Reason} {
				for _, id := range coverageIdentifiers(text) {
					// A family written with a wildcard ("BR-RO-L*") expands to
					// nothing here and is checked by the residue test below.
					if !ciusOwnIdentifier(src, id) {
						continue
					}
					checked++
					if _, ok := published[id]; !ok {
						t.Errorf("Coverage(%q) names %q, which the vendored Schematron does not publish: %q",
							src, id, text)
					}
				}
			}
		}
	}
	if checked < 20 {
		t.Fatalf("resolved only %d coverage identifiers for the five national rule sets; the harness is not "+
			"reading the table", checked)
	}
	t.Logf("checked %d coverage identifiers against the five vendored national Schematrons", checked)
}

// ciusOwnIdentifier reports whether id looks like one of src's authority's own
// identifiers, so that a coverage entry mentioning a CEN or Peppol rule in passing
// is not held against the national artefact.
func ciusOwnIdentifier(src Source, id string) bool {
	for _, a := range ciusArtefacts {
		if a.source == src {
			return a.own.MatchString(id)
		}
	}
	return false
}

// TestEveryPublishedCIUSRuleIsEvaluatedOrDisclaimed is the inventory guard PRs
// 19–21 built for KoSIT and OpenPEPPOL, in the form these five authorities allow.
//
// The property is the one that matters: every identifier the authority publishes is
// either evaluated by this package or named as a gap by its coverage table. A rule
// that is neither is unaccounted for — present in a normative file, absent from the
// code, and absent from the record of what the code does not do. That is exactly
// what C27, C30 and C33 were, three times, and nothing in this suite could see any
// of them.
//
// It is checked family by family rather than identifier by identifier, because the
// coverage table names families ("BR-RO-L*", "BR-RO-DT*") and expanding a wildcard
// into 64 identifiers to compare them one at a time would only make the failure
// message longer. A family with no entry is what this catches, and DT-CIUS-PT-* —
// 290 fatal Portuguese datatype rules that no coverage entry named — is what it
// caught. Those are evaluated now, so the family no longer needs an entry; the
// guard is unchanged and it is ciusEvaluated that grew.
func TestEveryPublishedCIUSRuleIsEvaluatedOrDisclaimed(t *testing.T) {
	flags := ciusFlagsBySource(t)
	if flags == nil {
		t.Skip("national CIUS Schematron not present; run `make cius-schematron`")
	}
	// famRE splits an identifier into the longest leading run of non-digit,
	// non-underscore characters — "BR-RO-L" from "BR-RO-L0201", "DT-CIUS-PT-" from
	// "DT-CIUS-PT-157" — which is the granularity the coverage entries are written
	// at.
	famRE := regexp.MustCompile(`^([^0-9]*[^0-9-])`)
	for src, published := range flags {
		covered := coverageText(src)
		families := map[string][]string{}
		for id := range published {
			if _, ok := ciusEvaluated[src][id]; ok {
				continue
			}
			fam := id
			if m := famRE.FindStringSubmatch(id); m != nil {
				fam = m[1]
			}
			families[fam] = append(families[fam], id)
		}
		for fam, ids := range families {
			sort.Strings(ids)
			// An entry may name the family with a wildcard, with a trailing dash,
			// or by naming one of its identifiers in a range.
			named := strings.Contains(covered, fam)
			if !named {
				for _, id := range ids {
					if strings.Contains(covered, id) {
						named = true
						break
					}
				}
			}
			if !named {
				t.Errorf("%s publishes %d %s* identifiers this package does not evaluate (%v) and "+
					"Coverage(%q) does not name them. A rule that is in neither the code nor the coverage table "+
					"is unaccounted for", src, len(ids), fam, ids[:min(len(ids), 6)], src)
			}
		}
	}
}

// TestEveryEvaluatedCIUSRuleFires is the "both verdicts" half of the oracle for the
// four rule sets whose authority ships no per-rule fixtures.
//
// PRs 19–21 could point each rule at its authority's own: KoSIT ships 242 documents
// carrying <?xmute?> instructions that declare a verdict per rule, and OpenPEPPOL
// ships 13 unit-test directories declaring 885. Of the five authorities here, one
// does the same — SimplerInvoicing names every NLCIUS instance for the rule it
// exercises and the verdict it expects, and TestNLCIUSPerRuleFixtures below reads
// those verdicts per rule rather than per family. The other four ship whole
// conforming invoices with no per-rule annotation, under either test-files/ or
// rule-source/, and their own repositories publish no unit-test suite of that kind.
//
// So for those four the firing verdict comes from this repository's own mutation
// fixtures, and this test is what makes them cover the evaluated set rather than
// whatever was convenient to write. The silent verdict is each suite's baseline
// plus that authority's sample corpus, which the per-authority FP=0 tests assert.
//
// A rule with no fixture that fires it is a rule that could be deleted without a
// red build, and that is the state C27, C30 and C33 were all found in. It needs no
// corpus, so it does not skip.
func TestEveryEvaluatedCIUSRuleFires(t *testing.T) {
	fired := map[Source]map[string]bool{}
	for _, s := range ciusSuites() {
		fired[s.source] = map[string]bool{}
		record := func(doc string) {
			for _, v := range findings(t, context.Background(), s.validate, []byte(doc)) {
				if v.Source == s.source {
					fired[s.source][v.Rule] = true
				}
			}
		}
		for _, tc := range s.cases {
			broken := strings.Replace(s.baseline, tc.from, tc.to, 1)
			if broken == s.baseline {
				t.Errorf("%s: mutation %q does not apply to the baseline", s.source, tc.name)
				continue
			}
			record(broken)
		}
		for _, d := range s.extras {
			record(d.xml)
		}
	}
	// The 290 generated DT-CIUS-PT-* rules are covered by fixtures built from each
	// rule's own context rather than by one mutation each; see
	// cius_pt_datatype_fixtures_test.go for why 290 hand-written mutations would be
	// weaker rather than stronger evidence. The property asserted is the same one.
	for id := range ptDTBuiltFixtures() {
		fired[SourceCIUSPT][id] = true
	}
	// And CIUS-RO's 90 generated ones, for the same reason and by the same means;
	// see cius_ro_rules_test.go.
	for id := range roBuiltFixtures() {
		fired[SourceCIUSRO][id] = true
	}
	for src, evaluated := range ciusEvaluated {
		var missing []string
		for id := range evaluated {
			if !fired[src][id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		if len(missing) != 0 {
			t.Errorf("%s: no fixture in this repository makes %v fire, so nothing would notice if the rule "+
				"stopped being evaluated. Add a case to this authority's mutation suite", src, missing)
		}
	}
}

// TestUBLBE13IsBoundToATautology checks the one unevaluable claim this PR adds, the
// way TestUnevaluableFamiliesNameTheirEvidence checks CEN's true() bindings: by
// reading the binding out of the artefact rather than by believing the prose.
//
// The general test can verify a Reason that says "true()", because that string is a
// literal in the file. This one is not a literal — it is a variable whose fallback
// makes an inequality unfalsifiable — so the claim has three parts and each is
// asserted separately: the let is declared with the fallback -1, the assertion's
// test is abs($TaxAmount) >= 0, and the flag is fatal (an unevaluable family still
// quotes the published flag; that is D10's whole point).
//
// If UBL.BE ever fixes the binding, this fails, and it should: the rule would become
// evaluable and the coverage entry would become a gap rather than a fact about the
// artefact.
func TestUBLBE13IsBoundToATautology(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "cius-be", "schematron", "*", "GLOBALUBL.BE.sch"))
	if len(files) == 0 {
		t.Skip("UBL.BE Schematron not present; run `make cius-schematron`")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `<let name="TaxAmount" value="if (cbc:TaxAmount) then xs:decimal(cbc:TaxAmount) else -1"/>`) {
			t.Errorf("%s no longer declares $TaxAmount with the -1 fallback that makes ubl-BE-13 unfalsifiable; "+
				"Coverage(SourceUBLBE) says it does", filepath.Base(f))
		}
		found := false
		for id, flags := range assertFlags(t, f, data) {
			if id != "ubl-BE-13" {
				continue
			}
			found = true
			if !flags["fatal"] {
				t.Errorf("%s flags ubl-BE-13 %v; Coverage(SourceUBLBE) records it fatal, and an unevaluable "+
					"family quotes the published flag like any other", filepath.Base(f), keysOf(flags))
			}
		}
		if !found {
			t.Errorf("%s no longer publishes ubl-BE-13 at all", filepath.Base(f))
		}
		// The test itself, read out of the same element. A decoder would give it
		// back through assertFlags if that helper carried tests; it does not, and
		// widening it for one rule would be worse than this.
		if !strings.Contains(text, `abs($TaxAmount) &gt;=0`) && !strings.Contains(text, `abs($TaxAmount) >=0`) {
			t.Errorf("%s no longer binds ubl-BE-13 to abs($TaxAmount) >= 0; the rule may have become evaluable, "+
				"in which case Coverage(SourceUBLBE) is claiming something that stopped being true", filepath.Base(f))
		}
	}
}
