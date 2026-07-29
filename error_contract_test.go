package formalis

import (
	"context"
	"strings"
	"testing"
)

// The error contract, and why it is table-driven over every exported validator.
//
// limits.go states a property of the whole package: a document the checker could
// not read is RuleSyntax, a run the checker did not finish is RuleLimit, and the
// two are never confused, because "syntaxViolation draws that line, and every
// exported validator goes through it". ValidateOrderXML did not. It answered a
// parse error with a finding of its own — "order-xml: the order XML is not a
// well-formed Cross Industry Order" — for three different inputs:
//
//	<a></b>                    malformed XML, and the decoder's message, the only
//	                           thing saying where it broke, was discarded
//	""                         empty input
//	<CrossIndustryInvoice/>    well-formed, and not an order — the claim is simply
//	                           false about this document
//
// and a caller filtering on the exported RuleSyntax constant to tell "bad file"
// from "bad invoice" got nothing from it in any of the three.
//
// TestStoppedRunIsNotReportedAsBadSyntax already listed ValidateOrderXML, which
// read as coverage; it exercised only the cancelled path, which is the one case
// the hand-rolled version got right. So the suite was green and the claim in
// limits.go was false. This file is the test that closes that gap: it drives
// every exported validator over all four inputs, and it fails if a validator is
// added without declaring what it answers.
type errorContract struct {
	// source and wrongRoot are what this validator answers when handed a
	// well-formed document whose root it does not accept.
	//
	// Two shapes are legitimate, and the split is the reason this is a per
	// validator field rather than one assertion for all. The validators that read
	// the raw element tree name the refusal in their own space (FPA-root,
	// ZA-root, ORDER-root …) under their own Source, so a caller can route it.
	// The EN 16931 core and the CIUS layered on it answer RuleSyntax under
	// SourceChecker, which is that constant's documented second meaning — "or is
	// not an invoice document at all" — since they have no rule namespace of
	// their own to refuse from and no format to name. Either way the answer must
	// be distinguishable from the malformed-document answer, which is what the
	// test asserts directly and what C5 broke.
	source    Source
	wrongRoot string
	// otherRoots are well-formed documents this validator must refuse. The first
	// is a root nothing in the package accepts; the rest are shapes that are
	// specifically interesting for this validator.
	otherRoots []string
}

// unknownRoot is well-formed XML that no validator in this package accepts.
const unknownRoot = `<NotAnInvoice/>`

// errorContracts is the contract each exported validator is held to. Every
// validator in allValidators needs a row; TestErrorContractCoversEveryValidator
// fails if one is missing, so a new validator cannot be added without saying
// what it answers to input it cannot read.
var errorContracts = map[string]errorContract{
	"Validate":          {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateCIUS":      {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateXRechnung": {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidatePeppol":    {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateNLCIUS":    {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateCIUSPT":    {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateCIUSRO":    {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateUBLBE":     {source: SourceChecker, wrongRoot: RuleSyntax},
	"ValidateSRBDT":     {source: SourceChecker, wrongRoot: RuleSyntax},

	// The Order-X row is C5 itself. <CrossIndustryInvoice/> is the document the
	// old validator called "not a well-formed Cross Industry Order"; it is
	// well-formed, and it is what an Order-X entry point is most likely to be
	// handed by mistake, since Factur-X is its sibling format.
	"ValidateOrderXML": {
		source: SourceOrderX, wrongRoot: "ORDER-root",
		otherRoots: []string{`<CrossIndustryInvoice/>`, `<Invoice/>`},
	},

	"ValidateFatturaPA": {source: SourceFatturaPA, wrongRoot: "FPA-root"},
	"ValidateFacturae":  {source: SourceFacturae, wrongRoot: "FE-root"},
	// ebInterface and Svefaktura take UBL's Invoice root and are told apart from
	// it by a child, so a bare <Invoice/> has to be refused too — the root name
	// alone is not acceptance.
	"ValidateEbInterface": {
		source: SourceEbInterface, wrongRoot: "EB-root",
		otherRoots: []string{`<Invoice/>`},
	},
	"ValidateSvefaktura": {
		source: SourceSvefaktura, wrongRoot: "SV-root",
		otherRoots: []string{`<Invoice/>`},
	},
	"ValidateKSeF":           {source: SourceKSeF, wrongRoot: "KS-root"},
	"ValidateFinvoice":       {source: SourceFinvoice, wrongRoot: "FI-root"},
	"ValidateTEAPPS":         {source: SourceTEAPPS, wrongRoot: "TP-root"},
	"ValidateOIOUBL":         {source: SourceOIOUBL, wrongRoot: "OIO-root"},
	"ValidateZATCA":          {source: SourceZATCA, wrongRoot: "ZA-root"},
	"ValidateOSA":            {source: SourceOSA, wrongRoot: "HU-root"},
	"ValidateTurkishInvoice": {source: SourceUBLTR, wrongRoot: "TR-root"},
	"ValidatePINT":           {source: SourcePINT, wrongRoot: "PINT-root"},
}

// TestErrorContractCoversEveryValidator keeps the table below honest. A
// validator that is not listed is a validator whose contract nobody decided, and
// C5 is exactly what that looks like a year later.
func TestErrorContractCoversEveryValidator(t *testing.T) {
	for name := range allValidators {
		if _, ok := errorContracts[name]; !ok {
			t.Errorf("%s has no row in errorContracts: say what it answers when it cannot read the document", name)
		}
	}
	for name := range errorContracts {
		if _, ok := allValidators[name]; !ok {
			t.Errorf("errorContracts lists %s, which is not in allValidators", name)
		}
	}
}

// TestMalformedXMLIsReportedAsSyntax pins case (a): a document the decoder
// rejects is one RuleSyntax finding, from this checker, carrying the decoder's
// own message — which is the only thing that tells a human where the XML broke.
func TestMalformedXMLIsReportedAsSyntax(t *testing.T) {
	const malformed = `<a></b>`
	// The decoder's own words. Asserting on them is the point: a validator that
	// substitutes a message of its own passes "it is a RuleSyntax finding" and
	// still loses the position of the defect.
	const want = "element <a> closed by </b>"

	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			v := fn(context.Background(), []byte(malformed)).Violations
			if len(v) != 1 {
				t.Fatalf("malformed XML produced %d findings, want exactly 1: %v", len(v), v)
			}
			if v[0].Rule != RuleSyntax {
				t.Errorf("malformed XML reported as %q, want %q: %s", v[0].Rule, RuleSyntax, v[0].Message)
			}
			if v[0].Source != SourceChecker {
				t.Errorf("malformed XML reported under Source %q, want %q", v[0].Source, SourceChecker)
			}
			if !strings.Contains(v[0].Message, want) {
				t.Errorf("the decoder's message was discarded: got %q, want it to contain %q", v[0].Message, want)
			}
		})
	}
}

// TestEmptyInputIsReportedAsSyntax pins case (b). Empty input is not a defective
// invoice and not a stopped run: there is no document, and the answer must say
// so under the one identifier a caller can filter on.
func TestEmptyInputIsReportedAsSyntax(t *testing.T) {
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			v := fn(context.Background(), nil).Violations
			if len(v) != 1 {
				t.Fatalf("empty input produced %d findings, want exactly 1: %v", len(v), v)
			}
			if v[0].Rule != RuleSyntax {
				t.Errorf("empty input reported as %q, want %q: %s", v[0].Rule, RuleSyntax, v[0].Message)
			}
			if v[0].Source != SourceChecker {
				t.Errorf("empty input reported under Source %q, want %q", v[0].Source, SourceChecker)
			}
			if !strings.Contains(v[0].Message, "no root element") {
				t.Errorf("empty input reported as %q, which does not say the document had no root", v[0].Message)
			}
		})
	}
}

// TestWrongRootIsNotReportedAsMalformed pins case (c), the half of C5 that was a
// false statement rather than a lost one: a well-formed document of a root this
// validator does not accept is refused as such, and never as "not well-formed".
//
// The finding must also be distinguishable from the malformed-document finding —
// by rule identifier for the tree-reading validators, and at minimum by message
// for the EN 16931 half, which reports RuleSyntax's documented second meaning.
// A caller that cannot tell "you handed me the wrong document" from "this file
// is broken" cannot route either.
func TestWrongRootIsNotReportedAsMalformed(t *testing.T) {
	ctx := context.Background()
	for name, fn := range allValidators {
		c := errorContracts[name]
		t.Run(name, func(t *testing.T) {
			malformed := fn(ctx, []byte(`<a></b>`)).Violations
			if len(malformed) != 1 {
				t.Fatalf("malformed XML produced %d findings, want exactly 1: %v", len(malformed), malformed)
			}

			for _, doc := range append([]string{unknownRoot}, c.otherRoots...) {
				v := fn(ctx, []byte(doc)).Violations
				if len(v) != 1 {
					t.Errorf("%s produced %d findings, want exactly 1: %v", doc, len(v), v)
					continue
				}
				if v[0].Rule != c.wrongRoot {
					t.Errorf("%s reported as %q, want %q: %s", doc, v[0].Rule, c.wrongRoot, v[0].Message)
				}
				if v[0].Source != c.source {
					t.Errorf("%s reported under Source %q, want %q", doc, v[0].Source, c.source)
				}
				if v[0].Message == malformed[0].Message {
					t.Errorf("%s and malformed XML give the identical answer %q, so a caller cannot tell "+
						"a document of the wrong format from a broken file", doc, v[0].Message)
				}
				// The specific untruth C5 told. This document parsed; nothing in
				// it is not well-formed.
				if strings.Contains(v[0].Message, "not well-formed") || strings.Contains(v[0].Message, "not a well-formed") {
					t.Errorf("%s is well-formed XML, but it was reported as %q", doc, v[0].Message)
				}
			}
		})
	}
}

// TestCancelledRunReportsOnlyLimit pins case (d) over the whole exported
// surface, where TestStoppedRunIsNotReportedAsBadSyntax walks a sample. A run
// that stopped has read nothing it can testify about, so RuleLimit is the whole
// answer: not empty (which reads as valid), not RuleSyntax (which accuses a
// document the checker never finished reading), and nothing from any rule set.
func TestCancelledRunReportsOnlyLimit(t *testing.T) {
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r := fn(ctx, []byte(validCII))
			v := r.Violations
			if len(v) == 0 {
				t.Fatal("a cancelled run returned nothing, which reads as valid")
			}
			// The same fact in the form a caller is most likely to test. A
			// stopped run is one of the two ways Complete is false, and
			// Conformant is the predicate that has to get both right.
			if r.Complete {
				t.Error("a cancelled run reported Complete; the checker did not see the whole document")
			}
			if r.Conformant() {
				t.Error("a cancelled run reported Conformant, which is the exact reading RuleLimit exists to prevent")
			}
			for _, e := range v {
				if e.Rule != RuleLimit {
					t.Errorf("a cancelled run reported %q: %s", e.Rule, e.Message)
				}
				if e.Source != SourceChecker {
					t.Errorf("a cancelled run reported under Source %q, want %q", e.Source, SourceChecker)
				}
				if !IsCheckerViolation(e) {
					t.Errorf("a cancelled run produced a finding IsCheckerViolation does not recognise: %s: %s", e.Rule, e.Message)
				}
			}
		})
	}
}
