package formalis

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The tests in this file exist to make C24 unrepeatable, and to make the whole
// class it belongs to unrepeatable with it.
//
// C24 was not one wrong case. It was two orderings — DetectCIUS's switch over
// BT-24 and Detect's arbitration — kept in different files, each individually
// defensible, silently disagreeing about 64 real documents. The fix was to leave
// one ordering; these tests are what stops a second one growing back, and what
// stops a new profile being added whose discriminator another profile's
// identifier already contains.

// canonicalSpecIDs are Specification identifiers as the authorities publish
// them, one or more per rule set, taken from the conformance corpora and from
// the specifications themselves.
//
// SourceEN16931 lists identifiers that name no profile at all: the bare CEN one
// and a Factur-X vintage. They belong here because "matches nothing" is an
// answer the table has to keep giving — a discriminator broad enough to swallow
// them would route every plain EN 16931 invoice into a national rule set.
var canonicalSpecIDs = map[Source][]string{
	SourceXRechnung: {
		"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0",
		"urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_2.2",
		"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0#conformant#urn:xeinkauf.de:kosit:extension:xrechnung_3.0",
	},
	SourcePeppol: {
		"urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0",
	},
	SourcePINT: {
		"urn:peppol:pint:billing-1@ae-1",
		"urn:peppol:pint:billing-1@aunz-1",
		"urn:peppol:pint:billing-1@eu-1",
		"urn:peppol:pint:billing-1@jp-1",
		"urn:peppol:pint:billing-1@my-1",
		"urn:peppol:pint:billing-1@om-1",
		"urn:peppol:pint:billing-1@sg-1",
		"urn:peppol:pint:nontaxinvoice-1@jp-1",
		// The pre-release JP identifier, kept here so the exception is stated in
		// the test data as well as in the table it is written into.
		"urn:fdc:peppol:jp:billing:3.0",
	},
	SourceNLCIUS: {"urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0"},
	SourceCIUSPT: {
		"urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:1.0.0.",
		"urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:2.1.1",
	},
	SourceCIUSRO: {
		"urn:cen.eu:en16931:2017#compliant#urn:efactura.mfinante.ro:CIUS-RO:1.0.0",
		"urn:cen.eu:en16931:2017#compliant#urn:efactura.mfinante.ro:CIUS-RO:1.0.1",
	},
	SourceUBLBE:   {"urn:cen.eu:en16931:2017#conformant#urn:UBL.BE:1.0.0.20180214"},
	SourceSRBDT:   {"urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022"},
	SourceOIOUBL:  {"OIOUBL-2.01", "OIOUBL-2.1"},
	SourceUBLTR:   {"TR1.2", "TR1.2.1"},
	SourceEN16931: {"urn:cen.eu:en16931:2017", "urn:ferd:CrossIndustryDocument:invoice:1p0:comfort", ""},
}

// wantSpecIDSource is the answer specIDSource must give for a canonical
// identifier: the Source itself, except that "names no profile" is SourceNone.
func wantSpecIDSource(src Source) Source {
	if src == SourceEN16931 {
		return SourceNone
	}
	return src
}

// TestEveryPublishedIdentifierRoutesToItsOwnRuleSet is the direct regression
// test for C24, generalised to every identifier the package claims to know.
//
// Before the fix, "urn:peppol:pint:billing-1@my-1" answered SourcePeppol here,
// because the bare "peppol" case was the last arm of a switch that had no PINT
// case at all.
func TestEveryPublishedIdentifierRoutesToItsOwnRuleSet(t *testing.T) {
	for src, ids := range canonicalSpecIDs {
		for _, id := range ids {
			if got := specIDSource(id); got != wantSpecIDSource(src) {
				t.Errorf("specIDSource(%q) = %q, want %q", id, got, wantSpecIDSource(src))
			}
			// Case and surrounding whitespace are not part of an identifier's
			// meaning. They were part of its routing while three of the tests
			// were case-sensitive and the rest were not.
			for _, v := range []string{strings.ToUpper(id), strings.ToLower(id), "  " + id + "\n"} {
				if got := specIDSource(v); got != wantSpecIDSource(src) {
					t.Errorf("specIDSource(%q) = %q, want %q; case and whitespace must not change the routing",
						v, got, wantSpecIDSource(src))
				}
			}
		}
	}
}

// TestNoDiscriminatorIsSwallowedByAnother is the guard on the bug class rather
// than on the bug.
//
// A discriminator that another profile's identifier also contains is not by
// itself a defect — "urn:peppol:pint:billing-1@my-1" contains "peppol" and
// always will, because PINT is a Peppol specification. The defect is such a pair
// with no ordered rule saying which wins. So: for every published identifier,
// collect every entry of specIDRules whose marks match it, and require that when
// more than one does, the one the table reaches first is the profile that
// published the identifier. A new profile whose discriminator is broad enough to
// claim someone else's document fails here, naming both.
func TestNoDiscriminatorIsSwallowedByAnother(t *testing.T) {
	for src, ids := range canonicalSpecIDs {
		for _, id := range ids {
			norm := normSpecID(id)
			var matched []string
			for _, r := range specIDRules {
				if r.matches(norm) {
					matched = append(matched, string(r.src))
				}
			}
			if len(matched) == 0 {
				if wantSpecIDSource(src) != SourceNone {
					t.Errorf("%q (%s) matches no entry of specIDRules", id, src)
				}
				continue
			}
			if wantSpecIDSource(src) == SourceNone {
				t.Errorf("%q names no profile, but specIDRules claims it for %s", id, strings.Join(matched, ", "))
				continue
			}
			if matched[0] != string(src) {
				t.Errorf("%q is published by %s, but the first entry of specIDRules that matches it is %s (all matches: %s)",
					id, src, matched[0], strings.Join(matched, ", "))
			}
			if len(matched) > 1 {
				t.Logf("%s wins %q over %s by position in specIDRules", src, id, strings.Join(matched[1:], ", "))
			}
		}
	}
}

// TestSpecIDRulesAreOrderedMostSpecificFirst checks the property that makes the
// table above answerable at all, straight off the table and without any
// identifier to test it on: if one profile's discriminator contains another's,
// the containing one must be tested first, because everywhere it matches the
// contained one matches too. "peppol:pint" contains "peppol"; PINT is therefore
// ahead of Peppol, and a future entry that gets this backwards fails here rather
// than in a corpus sweep six profiles later.
func TestSpecIDRulesAreOrderedMostSpecificFirst(t *testing.T) {
	seen := map[Source]bool{}
	for i, r := range specIDRules {
		if seen[r.src] {
			t.Errorf("specIDRules lists %s twice; declaresSpecID reads the first entry only", r.src)
		}
		seen[r.src] = true
		if len(r.marks) == 0 {
			t.Errorf("specIDRules[%d] (%s) has no discriminator, so it can never match", i, r.src)
		}
		for _, m := range r.marks {
			if m != normSpecID(m) {
				t.Errorf("specIDRules[%d] (%s) discriminator %q is not lower-case and trimmed; it is matched "+
					"against a normalised identifier and would never fire", i, r.src, m)
			}
		}
	}

	for i, a := range specIDRules {
		for j, b := range specIDRules {
			if i == j || a.src == b.src {
				continue
			}
			for _, ma := range a.marks {
				for _, mb := range b.marks {
					if ma == mb {
						t.Errorf("%s and %s share the discriminator %q, so no order can tell them apart", a.src, b.src, ma)
						continue
					}
					if !strings.Contains(ma, mb) {
						continue
					}
					// Every identifier ma matches, mb matches too. If b came
					// first it would take all of a's documents.
					if j < i {
						t.Errorf("%s is tested before %s, but %s's discriminator %q contains %s's %q: "+
							"every identifier naming %s would be routed to %s",
							b.src, a.src, a.src, ma, b.src, mb, a.src, b.src)
					}
				}
			}
		}
	}
}

// TestRoutingAndArbitrationAgreeOnHandWrittenDocuments checks the two paths
// against each other on documents small enough to read, including the ones the
// corpus does not contain.
//
// The two paths are not the same code and cannot be: Detect scans, because it
// must route documents the parser would refuse, and ValidateCIUS reads the tree
// it already parsed, because reading the bytes twice would spend one document's
// element budget twice. What they share is route, and what this test asserts is
// that sharing it is enough — that the facts each collects lead to the same
// answer.
func TestRoutingAndArbitrationAgreeOnHandWrittenDocuments(t *testing.T) {
	docs := map[string]struct {
		doc  string
		want Source
		cius CIUS
	}{
		"pint, malaysian": {
			`<Invoice><CustomizationID>urn:peppol:pint:billing-1@my-1</CustomizationID></Invoice>`,
			SourcePINT, CIUSPINT,
		},
		"pint, pre-release japanese": {
			`<Invoice><CustomizationID>urn:fdc:peppol:jp:billing:3.0</CustomizationID></Invoice>`,
			SourcePINT, CIUSPINT,
		},
		"pint credit note": {
			`<CreditNote><CustomizationID>urn:peppol:pint:billing-1@sg-1</CustomizationID></CreditNote>`,
			SourcePINT, CIUSPINT,
		},
		"peppol bis billing 3.0": {
			`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0</CustomizationID></Invoice>`,
			SourcePeppol, CIUSPeppol,
		},
		"xrechnung": {
			`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0</CustomizationID></Invoice>`,
			SourceXRechnung, CIUSXRechnung,
		},
		"oioubl": {
			`<Invoice><CustomizationID>OIOUBL-2.1</CustomizationID></Invoice>`,
			SourceOIOUBL, CIUSNone,
		},
		// The audit's own overlap: a brand name beats a two-character prefix.
		"tr-oioubl": {
			`<Invoice><CustomizationID>TR-OIOUBL-2.02</CustomizationID></Invoice>`,
			SourceOIOUBL, CIUSNone,
		},
		"turkish": {
			`<Invoice><CustomizationID>TR1.2</CustomizationID></Invoice>`,
			SourceUBLTR, CIUSNone,
		},
		// A PINT identifier alongside a ZATCA profile identifier: BT-24 is a
		// claim about the rule set, a profile identifier is not.
		"pint over zatca": {
			`<Invoice><CustomizationID>urn:peppol:pint:billing-1@om-1</CustomizationID>` +
				`<ProfileID>reporting:1.0</ProfileID></Invoice>`,
			SourcePINT, CIUSPINT,
		},
		"zatca, no identifier": {
			`<Invoice><ProfileID>reporting:1.0</ProfileID></Invoice>`,
			SourceZATCA, CIUSNone,
		},
		"ebinterface": {
			`<Invoice><Biller><VATIdentificationNumber>ATU1</VATIdentificationNumber></Biller></Invoice>`,
			SourceEbInterface, CIUSNone,
		},
		"svefaktura": {
			`<Invoice><SellerParty/></Invoice>`,
			SourceSvefaktura, CIUSNone,
		},
		"plain en 16931 ubl": {
			`<Invoice><CustomizationID>urn:cen.eu:en16931:2017</CustomizationID></Invoice>`,
			SourceEN16931, CIUSNone,
		},
		"cii, no profile": {
			`<CrossIndustryInvoice><ExchangedDocumentContext>` +
				`<GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017</ID>` +
				`</GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext></CrossIndustryInvoice>`,
			SourceEN16931, CIUSNone,
		},
		"cii xrechnung": {
			`<CrossIndustryInvoice><ExchangedDocumentContext>` +
				`<GuidelineSpecifiedDocumentContextParameter>` +
				`<ID>urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0</ID>` +
				`</GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext></CrossIndustryInvoice>`,
			SourceXRechnung, CIUSXRechnung,
		},
	}
	for name, tc := range docs {
		t.Run(name, func(t *testing.T) {
			data := []byte(tc.doc)
			det, err := Detect(data)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if det.Source != tc.want {
				t.Errorf("Detect = %q, want %q", det.Source, tc.want)
			}
			if det.CIUS != tc.cius {
				t.Errorf("Detect CIUS = %q, want %q", det.CIUS, tc.cius)
			}
			p, err := parseEN16931(newRun(nil), data)
			if err != nil {
				t.Fatalf("parseEN16931: %v", err)
			}
			if got := route(p.facts()); got != det.Source {
				t.Errorf("ValidateCIUS routes to %q but Detect says %q; the two must read the same arbitration",
					got, det.Source)
			}
		})
	}
}

// TestRoutingAndArbitrationAgreeOverTheCorpus is the same claim over real
// documents, and the one that would have caught C24 on the day it was written:
// 64 PINT instances routed to Peppol BIS through ValidateCIUS while Detect
// reported PINT, and 154 corpus documents in all were validated against a rule
// set Detect disagreed with.
//
// It compares the routing rather than the findings so it can cover every
// document cheaply. TestValidateCIUSRunsWhatDetectNamed below closes the loop
// on the findings themselves.
//
// Its ratchet is minDispatchedDocuments (corpus_test.go), which counts a
// different population from detect_scan_test.go's: every corpus document the
// EN 16931 parser accepts, which is what ValidateCIUS can be asked to route,
// rather than every document whose corpus publishes one format.
func TestRoutingAndArbitrationAgreeOverTheCorpus(t *testing.T) {
	checked, disagreed := 0, 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		// Only documents ValidateCIUS can reach the routing on: it is the
		// dispatcher for the roots parseEN16931 accepts, and a document it
		// refuses never gets as far as being routed.
		doc, perr := parseEN16931(newRun(nil), data)
		if perr != nil {
			return nil
		}
		det, derr := Detect(data)
		if derr != nil {
			t.Errorf("%s: the parser read this document but Detect could not: %v", p, derr)
			return nil
		}
		checked++
		if got := route(doc.facts()); got != det.Source {
			disagreed++
			if disagreed <= 20 {
				t.Errorf("%s: ValidateCIUS routes to %q, Detect says %q (root %q, BT-24 %q)",
					p, got, det.Source, det.Root, det.SpecID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Skip("no corpus present (make cius-oracles / make en16931-artefacts)")
	}
	// The same ratchet the other corpus sweeps carry: a test that agreed with
	// three files would agree with anything.
	atLeast(t, "documents dispatched", checked, minDispatchedDocuments)
	t.Logf("routing and arbitration agree on %d corpus documents (%d disagreements)", checked, disagreed)
}

// TestValidateCIUSRunsWhatDetectNamed closes the loop the routing tests leave
// open. Agreeing on a Source is worth nothing if the dispatcher then runs
// something else, so this compares the findings themselves: for a document of
// every format the dispatcher can reach, ValidateCIUS must return exactly what
// the validator Detect names returns.
//
// It is restricted to documents parseEN16931 accepts, which is what ValidateCIUS
// is: hand a Facturae invoice to a dispatcher for the UBL and CII syntaxes and
// it answers with a syntax finding, correctly, while ValidateFacturae validates
// it.
func TestValidateCIUSRunsWhatDetectNamed(t *testing.T) {
	docs := map[string]string{
		"pint": `<Invoice><CustomizationID>urn:peppol:pint:billing-1@my-1</CustomizationID></Invoice>`,
		"pint, pre-release japanese": `<Invoice><CustomizationID>urn:fdc:peppol:jp:billing:3.0</CustomizationID>` +
			`<ProfileID>x</ProfileID></Invoice>`,
		"peppol":      `<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0</CustomizationID></Invoice>`,
		"xrechnung":   minimalXRechnungUBL,
		"oioubl":      `<Invoice><CustomizationID>OIOUBL-2.1</CustomizationID></Invoice>`,
		"turkish":     `<Invoice><CustomizationID>TR1.2</CustomizationID></Invoice>`,
		"zatca":       `<Invoice><ProfileID>reporting:1.0</ProfileID></Invoice>`,
		"ebinterface": `<Invoice><Biller/></Invoice>`,
		"svefaktura":  `<Invoice><SellerParty/></Invoice>`,
		"plain ubl":   minimalUBL,
	}
	ctx := context.Background()
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			data := []byte(doc)
			det, err := Detect(data)
			if err != nil {
				t.Fatal(err)
			}
			v := det.Validator()
			if v == nil {
				t.Fatalf("Detect = %q, which routes to no validator", det.Source)
			}
			got := mustReport(t, ctx, ValidateCIUS, data)
			want := mustReport(t, ctx, v, data)
			if !reflect.DeepEqual(got.Violations, want.Violations) {
				t.Errorf("ValidateCIUS reported\n  %v\nbut Detect named %q, whose validator reported\n  %v",
					got.Violations, det.Source, want.Violations)
			}
			if !reflect.DeepEqual(got.NotEvaluated, want.NotEvaluated) {
				t.Errorf("ValidateCIUS claims the coverage\n  %v\nbut the validator Detect named claims\n  %v",
					got.NotEvaluated, want.NotEvaluated)
			}
		})
	}
}

// TestEveryRoutedFormatHasAnInRunValidator keeps the dispatcher's table and
// Detection.Validator's from drifting apart, which is the same failure as C24
// one level up: two lists of the same mapping, maintained separately.
func TestEveryRoutedFormatHasAnInRunValidator(t *testing.T) {
	var missing []string
	for _, src := range allSources {
		if src == SourceChecker || src == SourceNone || src == SourceEN16931 {
			continue
		}
		if (Detection{Source: src}).Validator() == nil {
			continue // not a format Detect returns; nothing to route
		}
		_, model := modelValidators[src]
		_, tree := treeValidators[src]
		if !model && !tree {
			missing = append(missing, string(src))
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("Detect can route to %s, and each has an exported validator, but ValidateCIUS has no in-run "+
			"validator for them; a document declaring one would silently fall back to the EN 16931 core",
			strings.Join(missing, ", "))
	}
	for src := range modelValidators {
		if _, dup := treeValidators[src]; dup {
			t.Errorf("%s appears in both validator tables; which one runs is then an accident of the lookup order", src)
		}
	}
}
