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
//
// "No trace" is now the whole of the claim, and the rest of it had to go. It used
// to assert that validCII produced no finding at any of the five profiles, which
// stopped being a statement about the Profile check the moment each tier acquired
// its own element-table data model: validCII declares CEN's own identifier
// (urn:cen.eu:en16931:2017), which is the EN 16931 tier's, and it carries a buyer
// postal address, which MINIMUM does not use. It is a conformant document at the
// tier it claims and it is not one at MINIMUM, and that is the design rather than
// a regression — see TestFacturXTiersDifferInTheirDataModel, which measures it.
//
// So the assertion here is the one the test was written to make: every profile is
// accepted, and no profile produces a checker finding of any kind. The clean-run
// half is asserted at the tier the fixture actually declares.
func TestKnownProfilesAreAccepted(t *testing.T) {
	for _, p := range profiles {
		if !knownProfile(p) {
			t.Errorf("knownProfile(%q) = false for a declared profile", string(p))
		}
		for _, v := range findings(t, context.Background(), withProfile(p), []byte(validCII)) {
			if IsCheckerViolation(v) {
				t.Errorf("Validate(%q) reported %s/%s on a document it should have judged: %s",
					string(p), v.Source, v.Rule, v.Message)
			}
		}
	}
	if v := findings(t, context.Background(), withProfile(ProfileEN16931), []byte(validCII)); len(v) != 0 {
		t.Errorf("Validate(%q).Violations on a document conformant at that tier reported %d violation(s): %v",
			string(ProfileEN16931), len(v), v)
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
		"<ApplicableTradeTax><TypeCode>VAT</TypeCode><CalculatedAmount>20.00</CalculatedAmount><BasisAmount>100.00</BasisAmount><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></ApplicableTradeTax>",
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

// TestBasicEN16931AndExtendedDifferOnlyInTheRulesEXTENDEDPublishes records the
// flatness that is by design, so it is not mistaken for the flatness C4 found.
// Across the *named* rules these three tiers differ from each other in exactly
// two ways and no other; if that ever stops being true the doc comment on
// Profile — which enumerates the differences — has to change with it.
//
// The two ways are:
//
//   - EXTENDED is silent on BR-CO-11 and BR-CO-12, whose operands it may carry
//     unitemized. That is the excuse list.
//   - EXTENDED reports BR-FXEXT-* identifiers no other tier publishes. Those are
//     the rules Factur-X adds at that tier, and the assertion is not "EXTENDED
//     may report anything extra": the extra rule has to be one of the
//     identifiers this package evaluates for that profile, so a stray finding
//     from any other family still fails.
//
// The second arm is what changed when the 24 fatal restatements landed. It is a
// tightening rather than a weakening — before, the test asserted the two sets
// were equal and could not have distinguished "EXTENDED reports a BR-FXEXT-*
// rule it publishes" from "EXTENDED reports a rule nobody publishes", because
// neither happened.
//
// The scope to named rules is unchanged and is also not a weakening. The
// data-model assertions are keyed by a synthetic identifier per profile, so
// FX-DM-BASIC-0104 and FX-DM-EN16931-0204 are the same assertion under two names
// and could not be compared as strings at all. What they say is compared instead,
// by TestFacturXTiersDifferInTheirDataModel below, which asserts the tiers differ
// there rather than that they do not.
func TestBasicEN16931AndExtendedDifferOnlyInTheRulesEXTENDEDPublishes(t *testing.T) {
	docs := map[string]string{"conformant": validCII}
	// A document that trips a broad spread of rules, to make the comparison
	// mean something more than "both were clean".
	broken := strings.Replace(validCII, "<ID>INV-1</ID>", "", 1)
	broken = strings.Replace(broken, "<CountryID>FR</CountryID>", "", 1)
	broken = strings.Replace(broken, "<GrandTotalAmount>120.00</GrandTotalAmount>", "<GrandTotalAmount>999.00</GrandTotalAmount>", 1)
	docs["broken"] = broken

	// The rules Factur-X publishes at EXTENDED and nowhere else: the nine that
	// are its own new ground and the 24 restatements. Read from the two tables
	// rather than listed, so a rule added to either is admitted here and a rule
	// from any other family still fails.
	extendedOnly := map[string]bool{}
	for _, id := range facturXExtensionRules {
		extendedOnly[id] = true
	}
	for _, rs := range facturXRestatementRules {
		if rs.profile == ProfileExtended {
			extendedOnly[rs.id] = true
		}
	}

	for name, doc := range docs {
		base := namedRuleSet(findings(t, context.Background(), withProfile(ProfileEN16931), []byte(doc)))
		if name == "broken" && len(base) < 3 {
			t.Fatalf("the broken document reported only %d rules (%v); it is not exercising enough", len(base), base)
		}
		for _, p := range []Profile{ProfileBasic, ProfileExtended} {
			got := namedRuleSet(findings(t, context.Background(), withProfile(p), []byte(doc)))
			for rule := range got {
				if base[rule] {
					continue
				}
				if p == ProfileExtended && extendedOnly[rule] {
					continue // a rule the EXTENDED Schematron publishes and no other does
				}
				t.Errorf("%s: profile %q reports %s, which EN 16931 does not", name, string(p), rule)
			}
			for rule := range base {
				if got[rule] {
					continue
				}
				if p == ProfileExtended && (rule == "BR-CO-11" || rule == "BR-CO-12") {
					continue // the profile gate C45 names, removed in its own change
				}
				// A CEN identifier this tier's Schematron drops, whose replacement
				// is satisfied on this document, yields to the authority that
				// governs it. Anything else going quiet is a rule that stopped
				// firing.
				if id := facturXSuperseded[p][rule]; id != "" && !got[id] {
					continue
				}
				t.Errorf("%s: profile %q is silent on %s, which EN 16931 reports", name, string(p), rule)
			}
		}
	}
}

// TestFacturXTiersDifferInTheirDataModel is the other side of that, and it is the
// assertion issue #56 turns on: naming a Profile has to change the rule set, and
// the element-table data model is where it changes most.
//
// The document is validCII, which is conformant at the tier it declares. What the
// tiers say about it differs because their element tables differ — MINIMUM does
// not use the buyer postal address, the leaner tiers do not carry the line-level
// tax group, and only the EN 16931 tier accepts CEN's own specification
// identifier in BT-24. A change that flattened the tiers back to one rule set
// would leave every finding-count assertion in this file green and fail here.
func TestFacturXTiersDifferInTheirDataModel(t *testing.T) {
	// Each tier's findings, reduced to the assertion text rather than the
	// identifier: FX-DM-BASIC-0104 and FX-DM-EN16931-0204 are the same assertion
	// under two synthetic names, and comparing the names would report a difference
	// between every pair of tiers whatever they said.
	said := map[Profile]map[string]bool{}
	for _, p := range profiles {
		said[p] = map[string]bool{}
		for _, v := range findings(t, context.Background(), withProfile(p), []byte(validCII)) {
			if fxIsDataModelRule(v.Rule) {
				said[p][v.Message] = true
			}
		}
		t.Logf("profile %q: %d data-model findings on a document conformant at EN 16931", string(p), len(said[p]))
	}
	if len(said[ProfileEN16931]) != 0 {
		t.Errorf("the EN 16931 tier reports %d data-model findings on a document that declares its own identifier and "+
			"carries its whole element table: %v", len(said[ProfileEN16931]), said[ProfileEN16931])
	}
	for _, p := range []Profile{ProfileMinimum, ProfileBasicWL, ProfileBasic, ProfileExtended} {
		if len(said[p]) == 0 {
			t.Errorf("profile %q reports the same data-model findings as the EN 16931 tier on a document written for "+
				"that tier; the five element tables are not the same table and a Profile that does not select one is "+
				"the flatness C4 found", string(p))
		}
	}
	// And the specific difference the tiers are named for: MINIMUM does not use
	// the buyer postal address, which every other tier requires.
	want := "Element 'ram:PostalTradeAddress' is marked as not used in the given context."
	found := false
	for msg := range said[ProfileMinimum] {
		if strings.HasPrefix(msg, want) {
			found = true
		}
	}
	if !found {
		t.Errorf("MINIMUM did not report %q on a document carrying a buyer postal address: %v", want, said[ProfileMinimum])
	}
	for _, p := range []Profile{ProfileBasicWL, ProfileBasic, ProfileEN16931, ProfileExtended} {
		for msg := range said[p] {
			if strings.HasPrefix(msg, want) {
				t.Errorf("profile %q reported %q, and only MINIMUM drops the buyer postal address", string(p), msg)
			}
		}
	}
}

// namedRuleSet is ruleSet without the data-model assertions, whose identifiers
// this package mints per profile and which therefore cannot be compared across
// profiles by name.
func namedRuleSet(vs []Violation) map[string]bool {
	m := map[string]bool{}
	for _, v := range vs {
		if !fxIsDataModelRule(v.Rule) {
			m[v.Rule] = true
		}
	}
	return m
}

func ruleSet(vs []Violation) map[string]bool {
	m := map[string]bool{}
	for _, v := range vs {
		m[v.Rule] = true
	}
	return m
}
