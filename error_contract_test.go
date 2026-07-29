package formalis

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The error contract, and why it is table-driven over every exported validator.
//
// limits.go states a property of the whole package: a document the checker could
// not read is one answer, a run the checker did not finish is another, and the two
// are never confused, because one function draws that line and every exported
// validator goes through it. ValidateOrderXML did not. It answered a parse error
// with a finding of its own — "order-xml: the order XML is not a well-formed Cross
// Industry Order" — for three different inputs:
//
//	<a></b>                    malformed XML, and the decoder's message, the only
//	                           thing saying where it broke, was discarded
//	""                         empty input
//	<CrossIndustryInvoice/>    well-formed, and not an order — the claim is simply
//	                           false about this document
//
// and a caller filtering on the exported constant for a bad file to tell it from a
// bad invoice got nothing from it in any of the three.
//
// TestStoppedRunIsNotReportedAsBadSyntax already listed ValidateOrderXML, which
// read as coverage; it exercised only the cancelled path, which is the one case
// the hand-rolled version got right. So the suite was green and the claim in
// limits.go was false. This file is the test that closes that gap: it drives
// every exported validator over all four inputs, and it fails if a validator is
// added without declaring what it answers.
//
// Since D8 the first two of those inputs are an error rather than a finding, so
// the four cases are now split across two kinds of return. That does not weaken
// the contract, it sharpens it: the assertions below check that the error is the
// only thing an unreadable document produces, that the Report beside it is the
// zero Report, and that a well-formed document of the wrong root is still a
// finding — which is the half of C5 that was a false statement rather than a lost
// one.
type errorContract struct {
	// source and wrongRoot are what this validator answers when handed a
	// well-formed document whose root it does not accept.
	//
	// Two shapes are legitimate, and the split is the reason this is a per
	// validator field rather than one assertion for all. The validators that read
	// the raw element tree name the refusal in their own space (FPA-root,
	// ZA-root, ORDER-root …) under their own Source. The EN 16931 core and the
	// CIUS layered on it answer RuleRoot under SourceChecker, since they have no
	// rule namespace of their own to refuse from and no single format to name.
	// Either way it must be a finding and not an error: the document was read.
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
	"Validate":          {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateCIUS":      {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateXRechnung": {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidatePeppol":    {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateNLCIUS":    {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateCIUSPT":    {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateCIUSRO":    {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateUBLBE":     {source: SourceChecker, wrongRoot: RuleRoot},
	"ValidateSRBDT":     {source: SourceChecker, wrongRoot: RuleRoot},

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

// TestMalformedXMLIsAnError pins case (a): a document the decoder rejects comes
// back as an error wrapping ErrMalformedXML, carrying the decoder's own message —
// which is the only thing that tells a human where the XML broke — and with no
// findings at all, because there is no document to have findings about.
//
// It replaces the assertion that this was a RuleSyntax finding. The claim is
// strictly stronger in the direction that matters: it was previously enough to
// return one finding with the right rule name, and it is now required that the
// Report carry nothing a caller could mistake for a verdict.
func TestMalformedXMLIsAnError(t *testing.T) {
	const malformed = `<a></b>`
	// The decoder's own words. Asserting on them is the point: a validator that
	// substitutes a message of its own passes "it is an ErrMalformedXML" and still
	// loses the position of the defect.
	const want = "element <a> closed by </b>"

	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			r, err := fn(context.Background(), []byte(malformed))
			assertUnreadable(t, r, err, ErrMalformedXML, want)
		})
	}
}

// TestEmptyInputIsAnError pins case (b). Empty input is not a defective invoice
// and not a stopped run: there is no document, and XML requires exactly one root
// element, so "there was nothing to read" and "what was there could not be read"
// are one answer.
func TestEmptyInputIsAnError(t *testing.T) {
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			r, err := fn(context.Background(), nil)
			assertUnreadable(t, r, err, ErrMalformedXML, "no root element")
		})
	}
}

// TestUnsupportedEncodingIsADistinctError pins the discrimination the sentinels
// exist for. "The sender's file is corrupt" and "this producer emits UTF-16" are
// different operational answers, and a caller must be able to tell them apart
// without matching on a message.
func TestUnsupportedEncodingIsADistinctError(t *testing.T) {
	const utf16 = `<?xml version="1.0" encoding="UTF-16"?><Invoice/>`
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			r, err := fn(context.Background(), []byte(utf16))
			assertUnreadable(t, r, err, ErrUnsupportedEncoding, "UTF-16")
			if errors.Is(err, ErrMalformedXML) {
				t.Error("an encoding refusal also matches ErrMalformedXML, so the two cannot be told apart")
			}
		})
	}
}

// assertUnreadable is the whole shape of the unreadable-input contract: the
// error names the right sentinel and keeps the detail, and the Report beside it is
// the zero Report — no findings, and neither Conformant nor Complete, so a caller
// who ignored the error still cannot read it as clean.
func assertUnreadable(t *testing.T, r Report, err error, want error, detail string) {
	t.Helper()
	if err == nil {
		t.Fatalf("input this package cannot read returned no error; Report = %+v", r)
	}
	if !errors.Is(err, want) {
		t.Errorf("error %q does not match %v", err, want)
	}
	if !strings.Contains(err.Error(), detail) {
		t.Errorf("the underlying detail was discarded: got %q, want it to contain %q", err, detail)
	}
	if len(r.Violations) != 0 {
		t.Errorf("an unreadable document also produced %d findings: %v", len(r.Violations), r.Violations)
	}
	if r.Conformant() {
		t.Error("the Report returned with an error is Conformant, so ignoring the error reads as a clean invoice")
	}
	if r.Complete() {
		t.Error("the Report returned with an error is Complete")
	}
}

// TestWrongRootIsNotReportedAsMalformed pins case (c), the half of C5 that was a
// false statement rather than a lost one: a well-formed document of a root this
// validator does not accept is refused as such, and never as "not well-formed".
//
// With malformed input now an error, the distinction the test asserts is sharper
// than it was: a document of the wrong root must produce a *finding and no error*,
// so the two answers are no longer even the same kind of value. The finding must
// still name the refusal in a way a caller can route on — the format's own
// identifier for the tree-reading validators, RuleRoot for the EN 16931 half.
func TestWrongRootIsNotReportedAsMalformed(t *testing.T) {
	ctx := context.Background()
	for name, fn := range allValidators {
		c := errorContracts[name]
		t.Run(name, func(t *testing.T) {
			for _, doc := range append([]string{unknownRoot}, c.otherRoots...) {
				r, err := fn(ctx, []byte(doc))
				// The document parsed. Answering with an error would say this
				// package could not read a file it read perfectly well.
				if err != nil {
					t.Errorf("%s is well-formed XML, but it came back as an error: %v", doc, err)
					continue
				}
				v := r.Violations
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
				if v[0].Severity != SeverityFatal {
					t.Errorf("%s was refused as %s; being handed the wrong document is not advisory", doc, v[0].Severity)
				}
				// The specific untruth C5 told. This document parsed; nothing in
				// it is not well-formed.
				if strings.Contains(v[0].Message, "not well-formed") || strings.Contains(v[0].Message, "not a well-formed") {
					t.Errorf("%s is well-formed XML, but it was reported as %q", doc, v[0].Message)
				}
				// It must also not be mistaken for the checker having given up.
				if IsCheckerViolation(v[0]) {
					t.Errorf("%s was reported as a checker violation, which means \"I did not judge this\"; "+
						"this document was judged, and refused", doc)
				}
			}
		})
	}
}

// TestCancelledRunReportsOnlyLimit pins case (d) over the whole exported
// surface, where TestStoppedRunIsNotReportedAsBadSyntax walks a sample. A run
// that stopped has read nothing it can testify about, so RuleLimit is the whole
// answer: not empty (which reads as valid), not an error and not RuleRoot (either
// of which says something about a document the checker never finished reading),
// and nothing from any rule set.
func TestCancelledRunReportsOnlyLimit(t *testing.T) {
	for name, fn := range allValidators {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r, err := fn(ctx, []byte(validCII))
			// A stopped run is emphatically not an error. It says nothing about
			// the document, so returning one would discard the findings that did
			// complete and would make pdf0 learn a second mechanism for an event
			// it already drains from one slice by rule name.
			if err != nil {
				t.Fatalf("a cancelled run returned an error, which reads as \"this file is unreadable\": %v", err)
			}
			v := r.Violations
			if len(v) == 0 {
				t.Fatal("a cancelled run returned nothing, which reads as valid")
			}
			// The same fact in the form a caller is most likely to test. A
			// stopped run is one of the two ways Complete is false, and
			// Conformant is the predicate that has to get both right.
			if r.Complete() {
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

// The rest of this suite calls validators through the two helpers below.
//
// The two-value return cannot be threaded into an assertion expression, and
// spelling the error check out at each of the hundred and fifty call sites would
// bury the assertions in ceremony that is the same every time. These keep each
// site one expression and turn an unreadable document into a failure that names
// its test: every fixture here is well-formed XML, and every corpus document is
// too — TestDetectRoutesTheCorpus asserts that over the whole corpus — so an
// error at any of those sites is a broken fixture rather than a result, and the
// assertion around it would be measuring nothing.
//
// The validator is passed rather than its result because Go allows a multi-value
// call as an argument list only when it is the whole argument list, and these
// helpers need the *testing.T as well.
type validator = func(context.Context, []byte) (Report, error)

// findings is the Violations of a validation whose input must be readable.
func findings(t *testing.T, ctx context.Context, fn validator, data []byte) []Violation {
	t.Helper()
	return mustReport(t, ctx, fn, data).Violations
}

// mustReport is findings for the assertions that are about the whole Report.
func mustReport(t *testing.T, ctx context.Context, fn validator, data []byte) Report {
	t.Helper()
	r, err := fn(ctx, data)
	if err != nil {
		t.Fatalf("the document could not be read, so the assertion on this call is about nothing: %v", err)
	}
	return r
}

// withProfile adapts Validate to the one-document signature every other
// validator already has.
func withProfile(p Profile) validator {
	return func(ctx context.Context, data []byte) (Report, error) { return Validate(ctx, data, p) }
}
