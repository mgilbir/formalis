package formalis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The six committed lean-tier invoices, and what this repository asserts about
// them.
//
// # Why they are here at all
//
// testdata/facturx/examples is weighted toward the two richest tiers — 25
// EXTENDED and 23 EN 16931 against 2 MINIMUM and 2 BASIC WL — and that matters
// more for the Factur-X rule set than it would for CEN's, because FX-DM-* is per
// tier: FX-DM-MINIMUM-0019 and FX-DM-BASIC-0107 are different rules with
// different contexts, and no document of another tier can reach either. Issue #61
// is that gap; these six are the documents that close it, and the reason they are
// committed rather than fetched is in their directory's README — they exist only
// inside PDF/A-3 containers, and opening one needs a PDF parser this package
// deliberately does not depend on.
//
// # Why the assertion is a ratchet and not FP=0
//
// Four of the six draw fatal findings, and the findings are right. Read out of
// the vendored Schematrons, unflagged and therefore fatal:
//
//   - FX-DM-MINIMUM-0019/0022 are `report true()` on the buyer's
//     ram:PostalTradeAddress and ram:SpecifiedTaxRegistration — elements MINIMUM's
//     element table marks unused. Both FNFE MINIMUM samples carry them (empty, but
//     present, which is what XPath says and what FNFE's own processor does), and
//     neither MINIMUM document already in the corpus does. The rule agrees with the
//     corpus and it is the sample that departs from the model.
//   - FX-DM-MINIMUM-0043/0044/0045 and FX-DM-BASIC-0107/0108/0182/0183/0184/0185/
//     0189/0224/0259 are `report @currencyID` on amounts whose tier forbids the
//     attribute. This is the same conclusion CEN's CII-DT-031 reached before v0.3.0
//     narrowed the binding to Factur-X's own: two authorities, arrived at
//     independently.
//   - FX-DM-BASIC-0018 is the code-database lookup on BT-24 itself. FNFE's
//     FACTUR-X_BASIC_codedb.xml enumerates exactly two values for that element, and
//     Avoir_FR_type381_BASIC declares a third —
//     "urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic", with colons where
//     both published identifiers write "#". FNFE's own sample declares an
//     identifier FNFE's own code list rejects.
//
// So the honest assertion is not "these are clean" and not "these are broken".
// It is "this is exactly what they draw", pinned per tier and per document so that
// a *new* finding on one of them is as red a build as a disappearing one. That is
// shape (1) of the three #61 offers, and it keeps the documents without claiming
// they pass.
//
// # Why they are outside TestAuthoritySamplesDrawNoFatalFinding
//
// That guard's principle is that a document *the authority accepts* must not draw
// a fatal finding here, and the operative word is accepts: for the Factur-X entry
// it is not the directory a document sits in that decides membership but FNFE's
// own valitool verdict, read out of the *_fx_validation_report.xml beside each
// example. Eight of the 59 examples have no report and are already not judged
// there for that reason.
//
// These six have no such verdict either. ZUGFeRD/corpus files them under
// `ZUGFeRDv2/correct/`, but that is a third party's classification of a PDF
// container, not FNFE saying the invoice inside passes FNFE's business rules —
// and where FNFE's data model and FNFE's sample disagree, as they do on all four
// counts above, the artefact is the authority and the sample is the thing being
// judged. So they are outside that population for the same stated reason the
// unreported examples are, and they are not thereby unjudged: this file judges
// them, harder than an FP=0 sweep could, because it fixes the number in both
// directions.
//
// The narrowing is deliberate and it is worth naming what it costs. If one of
// these four findings were ever shown to be this package's error rather than the
// sample's, this test would go on asserting it — the protection against that is
// the derivation above, re-checked against the artefact by
// facturx_datamodel_test.go, not the count below.

// fxLeanTierSample is one committed document: the tier its own BT-24 declares, and
// every fatal finding this package reports on it at that tier, in rule order with
// repeats spelled out.
//
// The rules are written out rather than counted because a count is the weaker
// half of the fact. Fourteen findings that became fourteen different findings is
// the movement most worth catching, and it is the one a total cannot see.
type fxLeanTierSample struct {
	file    string
	profile Profile
	fatal   []string
}

// fxLeanTierSamples is the record. Regenerate it by reading the failure message,
// never by copying whatever the code now produces: every line of it is a claim
// about FNFE's artefact that the comment above derives.
var fxLeanTierSamples = []fxLeanTierSample{{
	file:    "fnfe_BASIC.xml",
	profile: ProfileBasic,
	fatal: []string{
		"FX-DM-BASIC-0018",
		"FX-DM-BASIC-0107", "FX-DM-BASIC-0107",
		"FX-DM-BASIC-0108", "FX-DM-BASIC-0108",
		"FX-DM-BASIC-0182",
		"FX-DM-BASIC-0183",
		"FX-DM-BASIC-0184",
		"FX-DM-BASIC-0185",
		"FX-DM-BASIC-0189",
		"FX-DM-BASIC-0224", "FX-DM-BASIC-0224",
		"FX-DM-BASIC-0259", "FX-DM-BASIC-0259",
	},
}, {
	file:    "fnfe_MINIMUM.xml",
	profile: ProfileMinimum,
	fatal: []string{
		"FX-DM-MINIMUM-0019",
		"FX-DM-MINIMUM-0022",
		"FX-DM-MINIMUM-0043",
		"FX-DM-MINIMUM-0044",
		"FX-DM-MINIMUM-0045",
	},
}, {
	file:    "fnfe_MINIMUM_UE.xml",
	profile: ProfileMinimum,
	fatal: []string{
		"FX-DM-MINIMUM-0019",
		"FX-DM-MINIMUM-0022",
		"FX-DM-MINIMUM-0043",
		"FX-DM-MINIMUM-0044",
		"FX-DM-MINIMUM-0045",
	},
}, {
	// Clean, and the tier it is clean at is the point: it declares
	// "urn:cen.eu:en16931:2017#compliant#urn:zugferd.de:2p0:basic", the German
	// half of BASIC's two published identifiers.
	file:    "intarsys_BASIC.xml",
	profile: ProfileBasic,
}, {
	// One finding, and the same one both FNFE MINIMUM samples draw: a buyer
	// PostalTradeAddress at a tier that does not use it. Three independent
	// producers, one rule.
	file:    "intarsys_MINIMUM.xml",
	profile: ProfileMinimum,
	fatal:   []string{"FX-DM-MINIMUM-0019"},
}, {
	file:    "mustang_BASICWL_avoir.xml",
	profile: ProfileBasicWL,
}}

// TestFacturXLeanTierSamplesDrawExactlyTheseFindings is the ratchet #61 asks for,
// in both directions.
//
// It needs no corpus — the documents are tracked — so it runs in CI's corpus-less
// job, which is where a rule set that stopped firing at the lean tiers would
// otherwise be invisible: every other Factur-X oracle here skips without the
// vendored examples.
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
			"drift apart by a fetch; add the new document to fxLeanTierSamples with its findings derived "+
			"from the artefact, or remove the record with the file", committedCorpusDir, names, want)
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

		got := fatalRuleList(t, ctx, validateAtDeclaredProfile, data, s.file)
		if strings.Join(got, " ") != strings.Join(s.fatal, " ") {
			t.Errorf("%s (%s) draws\n  %v\nand this test records\n  %v\n"+
				"A finding that appeared is as interesting as one that vanished: neither is a number to "+
				"update on sight. Derive the change from the profile Schematron before touching the table.",
				s.file, string(s.profile), got, s.fatal)
		}

		// And again through ValidateCIUS, which is the entry point this package
		// tells callers to prefer and which arbitrates on the same BT-24. The
		// two doors disagreeing is C24 and C44; on these documents it was also
		// live, because a ZUGFeRD-branded identifier reached neither Factur-X
		// rule set until specIDRules learnt the second brand.
		if routed := fatalRuleList(t, ctx, ValidateCIUS, data, s.file); strings.Join(routed, " ") != strings.Join(got, " ") {
			t.Errorf("%s: ValidateCIUS reports %v and Validate at the declared %q tier reports %v; the "+
				"routing entry point must reach the same rule set the declared profile does",
				s.file, routed, string(s.profile), got)
		}

		e := byTier[s.profile]
		e.docs++
		e.findings += len(s.fatal)
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
		lines = append(lines, fmt.Sprintf("%s: %d document(s), %d fatal finding(s)", string(p), e.docs, e.findings))
	}
	t.Logf("Factur-X lean tiers, committed corpus — %s; %d fatal findings in all, every one derived from "+
		"the profile Schematron", strings.Join(lines, "; "), total)
}

// fatalRuleList is one document's fatal findings as a sorted rule list, repeats
// included. Sorted because the order findings arrive in is an implementation
// detail of the walk and not a property this test is about; repeats kept because
// two @currencyID attributes on two amounts is a different fact from one.
func fatalRuleList(t *testing.T, ctx context.Context, v validator, data []byte, name string) []string {
	t.Helper()
	r, err := v(ctx, data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := []string{}
	for _, f := range r.Fatal() {
		out = append(out, f.Rule)
	}
	sort.Strings(out)
	return out
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
