package formalis

import (
	"context"
	"strings"
	"testing"
)

// Per-rule tests for the EN 16931 semantic-model rules added on top of the CEN
// unit-test oracle.
//
// The oracle cannot stand in for these. Of the twelve rules this file's subject
// matter covers, the CEN suite ships fragments for exactly one (BR-51), and even
// there the failing fragment is tagged <warning>, which
// TestEN16931ConformanceSuite does not score at all. Every other rule here is
// invisible to the oracle in both directions: it would neither catch a rule that
// never fires nor catch one that fires on a conforming invoice. So each rule
// gets a conforming case and a violating case, in both syntaxes, stating what
// the rule is for rather than only that it exists.
//
// Each case asserts about *its own* rule and no other. A mutation that breaks a
// decimal limit may well trip a datatype rule too, and pinning the whole finding
// set would make these tests fail for reasons that have nothing to do with what
// they are about.

// reports says whether vs contains a finding for rule.
func reports(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// mutate applies a single textual substitution and fails if it matched nothing,
// so a fixture edit cannot silently turn a violating case into the baseline.
func mutate(t *testing.T, doc, from, to string) string {
	t.Helper()
	out := strings.Replace(doc, from, to, 1)
	if out == doc {
		t.Fatalf("fixture does not contain %q", from)
	}
	return out
}

// ruleCase is one document and the verdict expected for one rule.
type ruleCase struct {
	name string
	xml  string
	rule string
	want bool // true: the rule must fire; false: it must not
}

func runRuleCases(t *testing.T, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs := Validate(context.Background(), []byte(tc.xml), ProfileEN16931).Violations
			if got := reports(vs, tc.rule); got != tc.want {
				if tc.want {
					t.Errorf("expected %s to fire; got %v", tc.rule, vs)
				} else {
					t.Errorf("%s fired on a document that satisfies it: %v", tc.rule, vs)
				}
			}
		})
	}
}

// withUBLBody injects XML immediately before the UBL cac:TaxTotal.
func withUBLBody(x string) string {
	return strings.Replace(minimalUBL, "<TaxTotal>", x+"<TaxTotal>", 1)
}

// TestBRCL08NoteSubjectCode covers the Invoice note subject code (BT-21) against
// UNTDID 4451, and the very different ways the two syntaxes carry it.
//
// CII has an element, ram:SubjectCode. UBL has none: the binding writes the code
// into the note text as a "#CODE#" prefix, and the CEN rule reads it back out
// under exactly that convention — a '#' followed by three characters and another
// '#'. The negative cases below are the ones that convention exists to protect:
// a note that merely mentions a '#' is a note without a subject code, not a note
// with an invalid one.
func TestBRCL08NoteSubjectCode(t *testing.T) {
	ciiNote := func(code string) string {
		return mutate(t, validCII, "<ID>INV-1</ID>",
			"<ID>INV-1</ID><IncludedNote><Content>a note</Content><SubjectCode>"+code+"</SubjectCode></IncludedNote>")
	}
	ublNote := func(text string) string {
		return mutate(t, minimalUBL, "<ID>INV-1</ID>", "<ID>INV-1</ID><Note>"+text+"</Note>")
	}
	runRuleCases(t, []ruleCase{
		{"CII listed code", ciiNote("AAI"), "BR-CL-08", false},
		{"CII code the stale UBL copy omits", ciiNote("BAT"), "BR-CL-08", false},
		{"CII unlisted code", ciiNote("XXX"), "BR-CL-08", true},
		{"UBL prefixed listed code", ublNote("#AAI#General information"), "BR-CL-08", false},
		{"UBL prefixed unlisted code", ublNote("#XXX#General information"), "BR-CL-08", true},
		{"UBL note with no prefix", ublNote("General information"), "BR-CL-08", false},
		{"UBL note that merely mentions a hash", ublNote("see #1 above"), "BR-CL-08", false},
		{"UBL prefix of the wrong length is not a code", ublNote("#GENERAL#information"), "BR-CL-08", false},
		// BT-127, the Invoice line note, has no subject code: the CEN rule's
		// context is the document element's cbc:Note only.
		{"UBL line note carries no subject code", mutate(t, minimalUBL,
			"<InvoiceLine><ID>1</ID>", "<InvoiceLine><ID>1</ID><Note>#XXX#line note</Note>"), "BR-CL-08", false},
	})
}

// TestBRCL26DeliverToLocationScheme covers the Deliver-to location identifier
// scheme (BT-71) against the ISO 6523 ICD list — the same list BR-CL-10/11/21
// use, which is why 0001 is not in it (ISO withdrew it).
//
// The identifier is placed without a postal address on purpose: BG-15's
// presence is what BR-57 keys on, and this rule is about the identifier, not
// about the address group.
func TestBRCL26DeliverToLocationScheme(t *testing.T) {
	ciiShipTo := func(scheme string) string {
		return mutate(t, validCII, "<ApplicableHeaderTradeAgreement>",
			`<ApplicableHeaderTradeDelivery><ShipToTradeParty><GlobalID schemeID="`+scheme+
				`">7300010000001</GlobalID></ShipToTradeParty></ApplicableHeaderTradeDelivery>`+
				"<ApplicableHeaderTradeAgreement>")
	}
	ublLocation := func(scheme string) string {
		return withUBLBody(`<Delivery><DeliveryLocation><ID schemeID="` + scheme +
			`">7300010000001</ID></DeliveryLocation></Delivery>`)
	}
	runRuleCases(t, []ruleCase{
		{"CII listed scheme", ciiShipTo("0088"), "BR-CL-26", false},
		{"CII unlisted scheme", ciiShipTo("XR01"), "BR-CL-26", true},
		{"UBL listed scheme", ublLocation("0088"), "BR-CL-26", false},
		{"UBL withdrawn scheme 0001", ublLocation("0001"), "BR-CL-26", true},
		// An identifier with no scheme at all is BT-71 without BT-71-1; this rule
		// constrains the scheme when there is one.
		{"UBL identifier with no scheme", withUBLBody(
			`<Delivery><DeliveryLocation><ID>7300010000001</ID></DeliveryLocation></Delivery>`), "BR-CL-26", false},
	})
}
