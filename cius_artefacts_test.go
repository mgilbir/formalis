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
	// SimplerInvoicing mints four identifier shapes and this filter used to admit
	// one. ^BR-NL- alone hid the eight BR-GA-* rules of the G-account extension —
	// PR 26 left them out on the reading that the extension is "not NLCIUS", and no
	// guard here could have contradicted it — and it hid SI-UBL-2 and
	// empty-element-check, which are one rule under two names that no survey of this
	// rule set in this repository had ever counted. That is C39 in a second
	// authority's artefact, and TestEveryPublishedCIUSIdentifierIsClassified is what
	// makes the next one a red build rather than a reading.
	source:  SourceNLCIUS,
	binding: "UBL",
	globs:   []string{"nlcius/schematron/ubl/*.sch"},
	own:     nlciusOwnIDs,
	minIDs:  43,
}, {
	source:  SourceNLCIUS,
	binding: "CII",
	globs:   []string{"nlcius/schematron/cii/*.sch"},
	own:     nlciusOwnIDs,
	minIDs:  34,
}}

// nlciusOwnIDs is every identifier shape SimplerInvoicing mints across its two
// bindings and the G-account extension: BR-NL-* in both, BR-GA-* in the UBL
// extension file, and the empty-element rule, which is SI-UBL-2 in the UBL binding
// and empty-element-check in the CII one.
var nlciusOwnIDs = regexp.MustCompile(`^(?:BR-NL-|BR-GA-|SI-UBL-)|^empty-element-check$`)

// ciusForeignResidue is the last of the four dispositions
// TestEveryPublishedCIUSIdentifierIsClassified admits: an identifier a national
// artefact carries that another authority minted, and that the vendored copy of
// *that* authority's rule set no longer publishes.
//
// Every one is a rule CEN or OpenPEPPOL published in an earlier release and
// withdrew, sitting in a national file that vendors an older copy. They are listed
// individually rather than by prefix, because the whole point of the guard is that a
// prefix is what let a family hide: "BR-IG-*" written as a pattern would also admit
// a future BR-IG rule some authority invents, and there would be nothing to notice
// it.
//
// The reason column is the evidence for the attribution. Each is checkable against
// the vendored artefact that does carry the identifier's family, or against the
// release note that renamed it.
var ciusForeignResidue = map[string]string{
	// CEN renamed the two intra-community VAT category families between EN 16931
	// releases: BR-IG-* became BR-AF-* and BR-IP-* became BR-AG-*. UBL.BE and SI-UBL
	// both vendor a copy from before the rename, and CEN's current tree publishes
	// the new names only.
	"BR-IG-01": "CEN, renamed BR-AF-01", "BR-IG-02": "CEN, renamed BR-AF-02",
	"BR-IG-03": "CEN, renamed BR-AF-03", "BR-IG-04": "CEN, renamed BR-AF-04",
	"BR-IP-01": "CEN, renamed BR-AG-01", "BR-IP-02": "CEN, renamed BR-AG-02",
	"BR-IP-03": "CEN, renamed BR-AG-03", "BR-IP-04": "CEN, renamed BR-AG-04",
	// CEN identifiers the copies carry and CEN's current files do not.
	"BR-66":      "CEN, withdrawn after the release GLOBALUBL.BE.sch vendors",
	"BR-67":      "CEN, withdrawn after the release GLOBALUBL.BE.sch vendors",
	"UBL-CR-423": "CEN, withdrawn after the release CIUS-PT vendors",
	"UBL-CR-631": "CEN, withdrawn after the release CIUS-PT and UBL.BE vendor",
	"ubl-CR-631": "CEN, withdrawn after the release UBL.BE vendors; see ciusLowercasedCEN",
	"UBL-SR-38":  "CEN, withdrawn after the release CIUS-PT and UBL.BE vendor",
	"DK-R-015":   "OpenPEPPOL, withdrawn from its Danish rule set after the release GLOBALUBL.BE.sch vendors",
	"GR-R-007-1": "OpenPEPPOL, its Greek GR-R-007 split into sub-identifiers in the release GLOBALUBL.BE.sch vendors",
	"GR-R-007-2": "OpenPEPPOL, as GR-R-007-1",
	"GR-R-007-3": "OpenPEPPOL, as GR-R-007-1",
}

// ciusLowercasedCEN is the one artefact quirk the classifier folds case for:
// GLOBALUBL.BE.sch re-publishes CEN's UBL-CR-* and UBL-DT-* binding rules with a
// lower-case first segment. 671 of the 672 resolve to a CEN identifier that way and
// the one that does not is in ciusForeignResidue above, which is what makes this a
// spelling difference rather than a family of Belgian rules.
//
// It is scoped to that one prefix on purpose. A blanket case-insensitive lookup
// would be a second pattern deciding membership, which is the defect this guard
// exists to remove.
var ciusLowercasedCEN = regexp.MustCompile(`^ubl-(?:CR|DT)-`)

// TestEveryPublishedCIUSIdentifierIsClassified is the general form of C39, and the
// reason this PR is worth more than the eight rules it implements.
//
// Every guard over these five rule sets asks its question about the identifiers
// ciusArtefact.own admits. That is a pattern, and a pattern only enumerates what its
// author anticipated: ^(?:BR|DT)-CIUS-PT- could not see AT/eSPap's eight BR-AA-*
// rules (C39), and ^BR-NL- could not see SimplerInvoicing's eight BR-GA-*, its
// SI-UBL-2 or its empty-element-check. In both cases the family was in neither the
// code, nor the coverage table, nor the record of what the code does not do — and
// nothing could have said so, because the survey that would have noticed was
// filtering on the same pattern.
//
// So this test does not filter. It decodes **every** assertion identifier out of
// every vendored national Schematron and requires each to fall into exactly one of
// four dispositions:
//
//   - its authority's own, by ciusArtefact.own — then the guards below take over,
//     and it is either evaluated or named in that Source's coverage table;
//   - published by another authority whose artefact this repository also vendors
//     (CEN's EN 16931 files, KoSIT's, OpenPEPPOL's), which is what a merged artefact
//     like GLOBALUBL.BE.sch is full of. This is a lookup and not a pattern: the
//     identifier has to actually be in that authority's file;
//   - the same, spelt with a lower-case first segment (ciusLowercasedCEN);
//   - or in ciusForeignResidue, which names the authority and the release.
//
// An identifier in none of them fails the build, and the failure message says what
// to do with it. That is the property: a new family cannot arrive in one of these
// files and be invisible, whatever it is called, because being unrecognised is now
// the thing that fails rather than the thing that hides.
//
// Two-directional, like the severity guard: an entry in ciusForeignResidue that no
// artefact carries any more is an excuse with nothing to excuse, and it is removed
// by the same failure.
func TestEveryPublishedCIUSIdentifierIsClassified(t *testing.T) {
	files := map[Source][]string{}
	for _, a := range ciusArtefacts {
		for _, g := range a.globs {
			m, _ := filepath.Glob(filepath.Join("testdata", g))
			files[a.source] = append(files[a.source], m...)
		}
	}
	total := 0
	for _, fs := range files {
		total += len(fs)
	}
	if total == 0 {
		t.Skip("national CIUS Schematron not present; run `make cius-schematron`")
	}
	// schematronFlags is every identifier CEN, KoSIT and OpenPEPPOL publish in the
	// artefacts this repository vendors. It skips when the EN 16931 suite is absent,
	// which is the right behaviour here too: a classifier that cannot look an
	// identifier up must not conclude that nobody publishes it.
	foreign := schematronFlags(t)

	used := map[string]bool{}
	classified, unclassified := 0, map[string][]string{}
	for _, a := range ciusArtefacts {
		for _, f := range files[a.source] {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for id := range assertFlags(t, f, data) {
				switch {
				case a.own.MatchString(id):
				case foreign[id] != nil:
				case ciusLowercasedCEN.MatchString(id) && foreign[strings.ToUpper(id[:3])+id[3:]] != nil:
				case ciusForeignResidue[id] != "":
					used[id] = true
				default:
					unclassified[filepath.Base(f)] = append(unclassified[filepath.Base(f)], id)
					continue
				}
				classified++
			}
		}
	}
	for f, ids := range unclassified {
		sort.Strings(ids)
		t.Errorf("%s publishes %d identifiers (%v) that are neither %s's own nor published by any authority whose "+
			"artefact this repository vendors. Decide which: widen that artefact's `own` pattern and evaluate or "+
			"disclaim them, or add them to ciusForeignResidue with the authority and the release. A published "+
			"identifier nothing accounts for is exactly how BR-AA-* and BR-GA-* stayed invisible",
			f, len(ids), ids[:min(len(ids), 8)], ciusSourceOfFile(f))
	}
	for id, why := range ciusForeignResidue {
		if !used[id] {
			t.Errorf("ciusForeignResidue excuses %s (%q) and no vendored national artefact carries it any more; "+
				"an excuse with nothing to excuse is an excuse nobody will re-check", id, why)
		}
	}
	// The floor is the same kind as ciusArtefact.minIDs and guards the same failure:
	// a classifier that reads nothing classifies everything.
	if classified < 2400 {
		t.Fatalf("classified only %d published identifiers across the five national rule sets; the harness is "+
			"not reading the artefacts", classified)
	}
	t.Logf("classified %d published identifiers across %d vendored national Schematron files, %d of them by the "+
		"withdrawn-identifier list", classified, total, len(ciusForeignResidue))
}

// ciusSourceOfFile names the authority a vendored file belongs to, for the failure
// message above.
func ciusSourceOfFile(base string) Source {
	for _, a := range ciusArtefacts {
		for _, g := range a.globs {
			m, _ := filepath.Glob(filepath.Join("testdata", g))
			for _, f := range m {
				if filepath.Base(f) == base {
					return a.source
				}
			}
		}
	}
	return SourceNone
}

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
	// SRBDT is the third complete rule set here: the 21 reachable RSR business
	// rules, the 3 RSE extension rules and the 7 assertions of the abstract pdvcat
	// pattern (evaluated once per Serbian zero-rate VAT category, four times over).
	// The fifteen the Ministry publishes that no processor reaches are in
	// Coverage(SourceSRBDT) with Unevaluable set, and
	// TestSRBDTUnevaluableRulesAreDerivedFromTheArtefact is the evidence.
	SourceSRBDT: {
		"RSR-01": SeverityFatal, "RSR-02": SeverityFatal, "RSR-03": SeverityFatal,
		"RSR-04": SeverityFatal, "RSR-05": SeverityFatal, "RSR-06": SeverityFatal,
		"RSR-07": SeverityFatal, "RSR-11": SeverityFatal, "RSR-12": SeverityFatal,
		"RSR-14": SeverityFatal, "RSR-18": SeverityFatal, "RSR-19": SeverityFatal,
		"RSR-21": SeverityFatal, "RSR-23": SeverityFatal, "RSR-27": SeverityFatal,
		"RSR-28": SeverityFatal, "RSR-29": SeverityFatal, "RSR-30": SeverityFatal,
		"RSR-34": SeverityFatal, "RSR-35": SeverityFatal, "RSR-36": SeverityFatal,
		"RSE-01": SeverityFatal, "RSE-02": SeverityFatal, "RSE-03": SeverityFatal,
		"RSK-X-01": SeverityFatal, "RSK-X-05": SeverityFatal, "RSK-X-06": SeverityFatal,
		"RSK-X-07": SeverityFatal, "RSK-X-08": SeverityFatal, "RSK-X-09": SeverityFatal,
		"RSK-X-10": SeverityFatal,
	},
	// NLCIUS is the fourth complete rule set here, and the only one with two
	// bindings: 12 fatal identifiers and 22 advisory ones in UBL, 11 and 22 in CII.
	// This is the one place in the table where the value is not SeverityFatal, and
	// it is the reason the table is per identifier rather than an assertion that
	// everything is fatal.
	//
	// BR-NL-9 appears here at SeverityFatal because the UBL binding publishes it
	// reachably; the CII binding's copy is unevaluable and is in
	// Coverage(SourceNLCIUS). BR-NL-31 is the mirror image, advisory rather than
	// fatal. BR-NL-32-2 and BR-NL-32-3 are absent for the same reason ubl-BE-13 is:
	// no processor reaches them.
	SourceNLCIUS: {
		"BR-NL-1": SeverityFatal, "BR-NL-2": SeverityFatal, "BR-NL-3": SeverityFatal,
		"BR-NL-4": SeverityFatal, "BR-NL-5": SeverityFatal, "BR-NL-7": SeverityFatal,
		"BR-NL-8": SeverityFatal, "BR-NL-9": SeverityFatal, "BR-NL-10": SeverityFatal,
		"BR-NL-11": SeverityFatal, "BR-NL-12": SeverityFatal, "BR-NL-13": SeverityFatal,
		"BR-NL-19": SeverityWarning, "BR-NL-20": SeverityWarning, "BR-NL-21": SeverityWarning,
		"BR-NL-22": SeverityWarning, "BR-NL-23": SeverityWarning, "BR-NL-24": SeverityWarning,
		"BR-NL-25": SeverityWarning, "BR-NL-26": SeverityWarning,
		"BR-NL-27-1": SeverityWarning, "BR-NL-27-2": SeverityWarning,
		"BR-NL-27-3": SeverityWarning, "BR-NL-27-4": SeverityWarning,
		"BR-NL-28-1": SeverityWarning, "BR-NL-28-2": SeverityWarning,
		"BR-NL-28-3": SeverityWarning, "BR-NL-28-4": SeverityWarning,
		"BR-NL-29": SeverityWarning, "BR-NL-30": SeverityWarning, "BR-NL-31": SeverityWarning,
		"BR-NL-32-1": SeverityWarning, "BR-NL-32-and-34": SeverityWarning,
		"BR-NL-33": SeverityWarning, "BR-NL-35": SeverityWarning,
		// The G-account extension, UBL binding only. BR-GA-6 is the one assertion in
		// any of these artefacts that carries no flag attribute at all, and it is
		// fatal here because that is what a conforming processor makes of it: phive
		// runs ph-schematron, whose DefaultSVRLErrorLevelDeterminator folds an absent
		// or unrecognised flag onto DEFAULT_ERROR_LEVEL, and that constant is
		// EErrorLevel.ERROR. See TestGAccountSeveritiesAreThePublishedFlags.
		"BR-GA-0": SeverityFatal, "BR-GA-1": SeverityFatal, "BR-GA-2": SeverityFatal,
		"BR-GA-3": SeverityFatal, "BR-GA-4": SeverityFatal, "BR-GA-5": SeverityFatal,
		"BR-GA-6": SeverityFatal, "BR-GA-7": SeverityFatal,
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
		// 43 and 34, not the 34 and 33 PR 22 recorded: the survey's identifier filter
		// stopped at ^BR-NL- and did not admit the eight BR-GA-* rules of the
		// G-account extension, nor the empty-element rule that each binding publishes
		// under a name of its own (SI-UBL-2, empty-element-check).
		{SourceNLCIUS, "UBL", 43, 20, 23},
		{SourceNLCIUS, "CII", 34, 11, 23},
	} {
		ids := pub[want.src][want.binding]
		if ids == nil {
			t.Errorf("no %s binding was read for %q", want.binding, want.src)
			continue
		}
		// Split by Severity rather than by the literal flag string, because one
		// published assertion carries no flag at all — BR-GA-6 — and "the string is
		// not 'fatal'" would file it as advisory, which is neither what
		// ph-schematron makes of it nor what this package reports. severityOfFlag is
		// the one place that fold is decided.
		fatal, warn := 0, 0
		for id := range ids {
			if sev, known := severityOfFlag(pickFlag(ids[id])); !known {
				t.Errorf("%s/%s publishes %s with the flag %v, which this package cannot fold onto a Severity",
					want.src, want.binding, id, keysOf(ids[id]))
			} else if sev == SeverityFatal {
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
	if strings.Join(ublOnly, " ") != "BR-GA-0 BR-GA-1 BR-GA-2 BR-GA-3 BR-GA-4 BR-GA-5 BR-GA-6 BR-GA-7 "+
		"BR-NL-32-1 BR-NL-32-2 BR-NL-32-3 BR-NL-8 SI-UBL-2" {
		t.Errorf("NLCIUS publishes %v in UBL and not in CII; the expected difference is the eight BR-GA-*, "+
			"BR-NL-8, the three BR-NL-32-N and SI-UBL-2, and a change here changes which findings a CII document "+
			"may carry", ublOnly)
	}
	if strings.Join(ciiOnly, " ") != "BR-NL-22 BR-NL-23 BR-NL-32-and-34 empty-element-check" {
		t.Errorf("NLCIUS publishes %v in CII and not in UBL; the expected difference is BR-NL-22, BR-NL-23, "+
			"BR-NL-32-and-34 and empty-element-check", ciiOnly)
	}
	// The G-account extension is UBL's alone, and that is the fact that decides
	// whether a CII invoice declaring the extension's specification identifier may
	// carry a BR-GA finding. It may not: SimplerInvoicing publishes no CII G-account
	// Schematron, and NLCIUS-CII-validation.sch recognises the identifier in its $si
	// and $s gates without publishing a single rule of the extension. This is the
	// same question C32 and C36 got wrong twenty times between them.
	for id := range pub[SourceNLCIUS]["CII"] {
		if strings.HasPrefix(id, "BR-GA-") {
			t.Errorf("the NLCIUS CII binding now publishes %s; nlciusGAccountApplies gates the extension on a UBL "+
				"Invoice root and would have to change", id)
		}
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
					got, ok := published[id]
					if !ok {
						t.Errorf("Coverage(%q) names %q, which the vendored Schematron does not publish: %q",
							src, id, text)
						continue
					}
					// And the severity column, which had no guard for these five
					// Sources at all: TestCoverageSeveritiesMatchThePublishedFlag reads
					// EN 16931, XRechnung and Peppol only, because until PR 22 these
					// authorities' artefacts were not here to read. A coverage entry's
					// Severity is a quotation like a finding's (D10), and an entry that
					// misquotes it tells a caller a fatal gap is advisory.
					if want, known := severityOfFlag(pickFlag(got)); !known {
						t.Errorf("%s/%s carries the flag %v, which this package cannot fold onto a Severity",
							src, id, keysOf(got))
					} else if entry.Severity != want {
						t.Errorf("Coverage(%q) records the family %q as %s, and its authority flags %s %v. A "+
							"coverage entry's severity is the authority's flag quoted and nothing else",
							src, entry.Rules, entry.Severity, id, keysOf(got))
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
