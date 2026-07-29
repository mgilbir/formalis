package formalis

import (
	"context"
	"strings"
	"testing"
)

// This file pins what Profile means: which values Validate accepts, what it does
// when it is handed one it does not implement, and — for the three values that
// genuinely change the rule set — exactly which rules each one moves.
//
// The second half exists because the first is not enough on its own. Before C4,
// ProfileXRechnung, ProfileBasic, ProfileEN16931 and ProfileExtended produced an
// identical finding set on every document in testdata, and nothing said whether
// that was the design or a defect. Naming each surviving difference against a
// document that exhibits it means a future change cannot flatten one the way
// ProfileXRechnung was flat, silently.

// TestUnknownProfileIsRefusedNotAssumed is the C4 regression: a Profile this
// package does not implement must be reported, not read as EN 16931.
func TestUnknownProfileIsRefusedNotAssumed(t *testing.T) {
	cases := []struct {
		name    string
		profile Profile
	}{
		{"garbage", Profile("GARBAGE")},
		{"empty", Profile("")},
		// The near miss the rejection exists for: one space away from
		// ProfileEN16931, and the exact spelling ProfileFor takes as input.
		{"missing space", Profile("EN16931")},
		{"wrong case", Profile("en 16931")},
		// Removed from the enum by C4/D4. A caller passing the string it used to
		// carry must be told, not quietly given the EN 16931 core with no BR-DE-*.
		{"xrechnung", Profile("XRECHNUNG")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := findings(t, context.Background(), withProfile(tc.profile), []byte(validCII))
			// The property that matters most: never an empty slice, so a caller
			// testing len(v) == 0 for "valid" cannot get a clean bill of health
			// from a run that chose no rule set. validCII is conformant, so
			// without the rejection this returns zero findings.
			if len(v) != 1 {
				t.Fatalf("Validate(%q).Violations returned %d violations, want exactly 1: %v", string(tc.profile), len(v), v)
			}
			if v[0].Rule != RuleProfile || v[0].Source != SourceChecker {
				t.Errorf("Validate(%q).Violations reported %s/%s, want %s/%s", string(tc.profile), v[0].Source, v[0].Rule, SourceChecker, RuleProfile)
			}
			// It is not an accusation against the document, which is conformant.
			if v[0].Rule == RuleRoot {
				t.Errorf("Validate(%q).Violations blamed the document with %s", string(tc.profile), RuleRoot)
			}
			// A caller separating "unknown" from "non-conformant" must land on
			// "unknown"; counting this as a document defect would blame the
			// invoice for the caller's argument.
			if !IsCheckerViolation(v[0]) {
				t.Errorf("IsCheckerViolation(%v) = false, want true", v[0])
			}
			// The message has to be actionable: the accepted values, because the
			// failure is a near miss, and the CIUS route, because the other near
			// miss is reaching for a national rule set Profile never offered.
			for _, want := range []string{string(tc.profile), string(ProfileEN16931), "ValidateCIUS"} {
				if !strings.Contains(v[0].Message, want) {
					t.Errorf("message %q does not mention %q", v[0].Message, want)
				}
			}
		})
	}
}

// TestUnknownProfileIsRefusedBeforeTheDocumentIsRead pins that the rejection is
// about the request. A malformed document handed to an unknown profile reports
// the profile, not the syntax: the checker never got as far as reading it, and
// saying otherwise would claim a finding it did not make.
func TestUnknownProfileIsRefusedBeforeTheDocumentIsRead(t *testing.T) {
	v := findings(t, context.Background(), withProfile(Profile("GARBAGE")), []byte(`<a></b>`))
	if len(v) != 1 || v[0].Rule != RuleProfile {
		t.Fatalf("malformed XML with an unknown profile reported %v, want one %s finding", v, RuleProfile)
	}
}

// TestKnownProfilesAreAccepted pins the other half: a caller passing a real
// profile sees no trace of the new check.
func TestKnownProfilesAreAccepted(t *testing.T) {
	for _, p := range profiles {
		if !knownProfile(p) {
			t.Errorf("knownProfile(%q) = false for a declared profile", string(p))
		}
		v := findings(t, context.Background(), withProfile(p), []byte(validCII))
		if len(v) != 0 {
			t.Errorf("Validate(%q).Violations on a conformant invoice reported %d violation(s): %v", string(p), len(v), v)
		}
	}
}

// TestProfileEnumHasNoCIUSInIt guards D4's boundary directly: Profile carries
// data-richness tiers only. XRechnung is a CIUS, reachable through CIUSFor,
// DetectCIUS, CIUSXRechnung and ValidateXRechnung — never through Profile.
func TestProfileEnumHasNoCIUSInIt(t *testing.T) {
	if len(profiles) != 5 {
		t.Fatalf("Profile has %d values, want the 5 Factur-X tiers: %v", len(profiles), profiles)
	}
	for _, p := range profiles {
		if strings.Contains(strings.ToUpper(string(p)), "XRECHNUNG") {
			t.Errorf("Profile %q names a CIUS; CIUS values belong in the CIUS type", string(p))
		}
	}
	if p, ok := ProfileFor("XRECHNUNG"); ok {
		t.Errorf(`ProfileFor("XRECHNUNG") = (%q, true), want a CIUS rather than a profile`, string(p))
	}
	if c, ok := CIUSFor("XRECHNUNG"); !ok || c != CIUSXRechnung {
		t.Errorf(`CIUSFor("XRECHNUNG") = (%q, %v), want (%q, true)`, string(c), ok, string(CIUSXRechnung))
	}
}

// TestProfileForAndCIUSForPartitionTheLevels pins that the two functions cover
// the XMP ConformanceLevel space between them without overlapping, so a caller
// reading a PDF's metadata loses nothing by asking both.
func TestProfileForAndCIUSForPartitionTheLevels(t *testing.T) {
	for _, level := range []string{"MINIMUM", "BASIC WL", "BASICWL", "BASIC", "EN 16931", "EN16931", "en16931", "EXTENDED", "XRECHNUNG", "xrechnung", "NOT A LEVEL"} {
		p, isProfile := ProfileFor(level)
		c, isCIUS := CIUSFor(level)
		if isProfile && isCIUS {
			t.Errorf("%q is read as both profile %q and CIUS %q", level, string(p), string(c))
		}
		if isProfile && !knownProfile(p) {
			t.Errorf("ProfileFor(%q) returned %q, which Validate would refuse", level, string(p))
		}
	}
	if _, ok := ProfileFor("NOT A LEVEL"); ok {
		t.Error(`ProfileFor("NOT A LEVEL") reported true`)
	}
	if _, ok := CIUSFor("EN 16931"); ok {
		t.Error(`CIUSFor("EN 16931") reported true for a data-richness profile`)
	}
}

// TestProfilesThatDifferStillDiffer pins every rule the profile argument
// actually moves, each against a document that exhibits it. Removing a gate, or
// adding a profile that silently behaves like EN 16931, fails here.
func TestProfilesThatDifferStillDiffer(t *testing.T) {
	// No buyer postal address: BR-10/BR-11 everywhere except MINIMUM, whose
	// reduced CIUS does not carry BG-8.
	noBuyerAddress := strings.Replace(validCII,
		"<BuyerTradeParty><Name>Buyer Co</Name><PostalTradeAddress><CountryID>FR</CountryID></PostalTradeAddress></BuyerTradeParty>",
		"<BuyerTradeParty><Name>Buyer Co</Name></BuyerTradeParty>", 1)

	// No invoice line and no line-net total: BR-16/BR-12 for the profiles that
	// carry lines, silence for the head-only MINIMUM and BASIC WL.
	headOnly := strings.Replace(validCII,
		"<IncludedSupplyChainTradeLineItem>", "<Dropped>", 1)
	headOnly = strings.Replace(headOnly,
		"</IncludedSupplyChainTradeLineItem>", "</Dropped>", 1)
	headOnly = strings.Replace(headOnly,
		"<SpecifiedTradeSettlementHeaderMonetarySummation>\n        <LineTotalAmount>100.00</LineTotalAmount>",
		"<SpecifiedTradeSettlementHeaderMonetarySummation>", 1)

	// No VAT breakdown group: BR-CO-18 except under MINIMUM, which carries only
	// totals.
	noBreakdown := strings.Replace(validCII,
		"<ApplicableTradeTax><CalculatedAmount>20.00</CalculatedAmount><BasisAmount>100.00</BasisAmount><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>",
		"", 1)

	// Amount due that does not follow from the totals: BR-CO-16 except under
	// MINIMUM, whose reduced summation omits the paid amount.
	dueMismatch := strings.Replace(validCII,
		"<DuePayableAmount>120.00</DuePayableAmount>",
		"<DuePayableAmount>90.00</DuePayableAmount>", 1)

	// Allowance and charge totals with no itemized BG-20/21 behind them:
	// BR-CO-11/BR-CO-12 except under EXTENDED, which permits unitemized amounts.
	unitemizedAC := strings.Replace(validCII,
		"<TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>",
		"<AllowanceTotalAmount>10.00</AllowanceTotalAmount><ChargeTotalAmount>5.00</ChargeTotalAmount><TaxBasisTotalAmount>100.00</TaxBasisTotalAmount>", 1)

	cases := []struct {
		name string
		doc  string
		rule string
		// exempt lists the profiles that must NOT report rule; every other
		// profile must.
		exempt []Profile
	}{
		{"buyer address group", noBuyerAddress, "BR-10", []Profile{ProfileMinimum}},
		{"buyer country code", noBuyerAddress, "BR-11", []Profile{ProfileMinimum}},
		{"at least one line", headOnly, "BR-16", []Profile{ProfileMinimum, ProfileBasicWL}},
		{"line net total", headOnly, "BR-12", []Profile{ProfileMinimum, ProfileBasicWL}},
		{"a VAT breakdown group", noBreakdown, "BR-CO-18", []Profile{ProfileMinimum}},
		{"amount due summation", dueMismatch, "BR-CO-16", []Profile{ProfileMinimum}},
		{"allowance total summation", unitemizedAC, "BR-CO-11", []Profile{ProfileExtended}},
		{"charge total summation", unitemizedAC, "BR-CO-12", []Profile{ProfileExtended}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.doc == validCII {
				t.Fatal("the mutation did not apply; the document is unchanged")
			}
			exempt := map[Profile]bool{}
			for _, p := range tc.exempt {
				exempt[p] = true
			}
			for _, p := range profiles {
				got := hasFacturXRule(findings(t, context.Background(), withProfile(p), []byte(tc.doc)), tc.rule)
				if want := !exempt[p]; got != want {
					t.Errorf("profile %q: %s reported = %v, want %v", string(p), tc.rule, got, want)
				}
			}
		})
	}
}

// TestBasicEN16931AndExtendedDifferOnlyInBRCO1112 records the flatness that is
// by design, so it is not mistaken for the flatness C4 found. These three tiers
// differ from each other in one place and no other; if that ever stops being
// true the doc comment on Profile — which enumerates the differences — has to
// change with it.
func TestBasicEN16931AndExtendedDifferOnlyInBRCO1112(t *testing.T) {
	docs := map[string]string{"conformant": validCII}
	// A document that trips a broad spread of rules, to make the comparison
	// mean something more than "both were clean".
	broken := strings.Replace(validCII, "<ID>INV-1</ID>", "", 1)
	broken = strings.Replace(broken, "<CountryID>FR</CountryID>", "", 1)
	broken = strings.Replace(broken, "<GrandTotalAmount>120.00</GrandTotalAmount>", "<GrandTotalAmount>999.00</GrandTotalAmount>", 1)
	docs["broken"] = broken

	for name, doc := range docs {
		base := ruleSet(findings(t, context.Background(), withProfile(ProfileEN16931), []byte(doc)))
		if name == "broken" && len(base) < 3 {
			t.Fatalf("the broken document reported only %d rules (%v); it is not exercising enough", len(base), base)
		}
		for _, p := range []Profile{ProfileBasic, ProfileExtended} {
			got := ruleSet(findings(t, context.Background(), withProfile(p), []byte(doc)))
			for rule := range got {
				if !base[rule] {
					t.Errorf("%s: profile %q reports %s, which EN 16931 does not", name, string(p), rule)
				}
			}
			for rule := range base {
				if got[rule] {
					continue
				}
				if p == ProfileExtended && (rule == "BR-CO-11" || rule == "BR-CO-12") {
					continue // the one documented difference
				}
				t.Errorf("%s: profile %q is silent on %s, which EN 16931 reports", name, string(p), rule)
			}
		}
	}
}

func ruleSet(vs []Violation) map[string]bool {
	m := map[string]bool{}
	for _, v := range vs {
		m[v.Rule] = true
	}
	return m
}
