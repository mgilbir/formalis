package formalis

import (
	"context"
	"testing"
)

// TestCheckerFindingsCarryTheCheckerSource pins the attribution of the two
// reserved identifiers. They are this package's statements about its own run,
// not any authority's rules, and IsCheckerViolation must keep recognising the
// stopped-run one — it is what stops "unknown" being read as "conformant".
func TestCheckerFindingsCarryTheCheckerSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v := Validate(ctx, []byte(validCII), ProfileEN16931)
	if len(v) == 0 {
		t.Fatal("a cancelled run returned nothing, which reads as valid")
	}
	for _, e := range v {
		if e.Source != SourceChecker {
			t.Errorf("a cancelled run reported %q under Source %q, want %q", e.Rule, e.Source, SourceChecker)
		}
		if !IsCheckerViolation(e) {
			t.Errorf("IsCheckerViolation no longer recognises %q", e.Rule)
		}
	}

	// Malformed XML: a statement about the file, still made by the checker.
	bad := Validate(context.Background(), []byte(`<a></b>`), ProfileEN16931)
	if len(bad) != 1 {
		t.Fatalf("malformed XML reported %d findings, want 1: %v", len(bad), bad)
	}
	if bad[0].Source != SourceChecker || bad[0].Rule != RuleSyntax {
		t.Errorf("malformed XML reported %q/%q, want %q/%q",
			bad[0].Source, bad[0].Rule, SourceChecker, RuleSyntax)
	}
	// RuleSyntax is a defect in the document, not the checker giving up, so the
	// predicate must not claim it.
	if IsCheckerViolation(bad[0]) {
		t.Error("IsCheckerViolation claimed a malformed-document finding; it means 'the checker stopped'")
	}
}

// TestViolationErrorNamesItsSource keeps the string form unambiguous: printing a
// bare rule identifier is exactly the habit that let two of them be confused.
func TestViolationErrorNamesItsSource(t *testing.T) {
	got := Violation{Source: SourceNLCIUS, Rule: "BR-NL-1", Message: "the supplier shall be identified"}.Error()
	want := "NLCIUS BR-NL-1: the supplier shall be identified"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
