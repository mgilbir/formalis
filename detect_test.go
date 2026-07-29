package formalis

import (
	"context"
	"strings"
	"testing"
)

// TestDetectorsSeparateNotFromCannotTell pins the reason the Is* predicates
// return an error at all.
//
// They used to return a bare bool, which collapsed five different situations
// into one false: a well-formed document of another format, a truncated
// document of *this* format, malformed nesting, empty input, and bytes that are
// not XML. Only the first is "no". The rest are "I could not read this", and a
// caller dispatching on the answer routes a broken invoice to the wrong
// validator — which then reports another format's rules against it.
func TestDetectorsSeparateNotFromCannotTell(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		want    bool
		wantErr bool
	}{
		{"a real Facturae root", `<?xml version="1.0"?><Facturae><FileHeader/></Facturae>`, true, false},
		{"well-formed, another format", `<?xml version="1.0"?><Invoice><ID>1</ID></Invoice>`, false, false},

		// Each of these is a "could not tell", not a "no".
		{"truncated mid-element", `<?xml version="1.0"?><Facturae><FileHeader><Schema`, false, true},
		{"mismatched end tag", `<?xml version="1.0"?><Facturae><a></b></Facturae>`, false, true},
		{"empty input", ``, false, true},
		{"not XML at all", "%PDF-1.7\x00binary", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := IsFacturae([]byte(c.data))
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, want error: %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestUnsupportedCharsetIsNotSilentlyMisread pins the other half of the same
// overloading, on the true side.
//
// xmlCharsetReader used to pass any encoding it did not implement straight
// through, so a UTF-16 or EBCDIC document was read as if it were UTF-8. The
// predicate then answered true on the strength of mangled bytes, and the
// validators reported business-rule violations against text the sender never
// wrote. An encoding this package cannot decode is now a parse error.
func TestUnsupportedCharsetIsNotSilentlyMisread(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="EBCDIC-CP-BE"?><Facturae><FileHeader/></Facturae>`

	got, err := IsFacturae([]byte(doc))
	if err == nil {
		t.Fatalf("an unimplemented encoding was accepted: got %v, want an error", got)
	}
	if got {
		t.Error("the bool must be false when the document could not be read")
	}
	if !strings.Contains(err.Error(), "EBCDIC-CP-BE") {
		t.Errorf("error %q does not name the offending encoding", err)
	}

	// The same document through a validator is a syntax finding about the
	// file, not a list of business-rule violations derived from mangled text.
	v := ValidateFacturae(context.Background(), []byte(doc)).Violations
	if len(v) != 1 || v[0].Rule != RuleSyntax {
		t.Fatalf("got %v, want exactly one %q violation", v, RuleSyntax)
	}

	// The encodings real invoices declare still work.
	for _, enc := range []string{"UTF-8", "utf-8", "us-ascii", "ISO-8859-1", "ISO-8859-15", "windows-1252"} {
		body := `<?xml version="1.0" encoding="` + enc + `"?><Facturae><FileHeader/></Facturae>`
		if ok, err := IsFacturae([]byte(body)); err != nil || !ok {
			t.Errorf("encoding %s: got (%v, %v), want (true, nil)", enc, ok, err)
		}
	}
}

// TestDetectorsAgreeOnCannotTell checks the contract is uniform: every
// predicate reports unreadable input as an error rather than as a plain false,
// so a caller can rely on it without knowing which format it asked about.
func TestDetectorsAgreeOnCannotTell(t *testing.T) {
	predicates := map[string]func([]byte) (bool, error){
		"IsFacturae":       IsFacturae,
		"IsFatturaPA":      IsFatturaPA,
		"IsOSA":            IsOSA,
		"IsFinvoice":       IsFinvoice,
		"IsTEAPPS":         IsTEAPPS,
		"IsKSeF":           IsKSeF,
		"IsEbInterface":    IsEbInterface,
		"IsSvefaktura":     IsSvefaktura,
		"IsOIOUBL":         IsOIOUBL,
		"IsZATCA":          IsZATCA,
		"IsPINT":           IsPINT,
		"IsTurkishInvoice": IsTurkishInvoice,
	}
	if len(predicates) != 12 {
		t.Fatalf("the table covers %d predicates, want all 12", len(predicates))
	}
	const unreadable = `<?xml version="1.0"?><Invoice><a></b></Invoice>`
	const readable = `<?xml version="1.0"?><SomeUnrelatedRoot/>`
	for name, fn := range predicates {
		if ok, err := fn([]byte(unreadable)); err == nil || ok {
			t.Errorf("%s on malformed XML: got (%v, %v), want (false, error)", name, ok, err)
		}
		// A document it can read but does not recognise is a plain false.
		if ok, err := fn([]byte(readable)); err != nil || ok {
			t.Errorf("%s on an unrelated root: got (%v, %v), want (false, nil)", name, ok, err)
		}
	}
}

// The tests below cover the ordered entry point rather than the twelve
// predicates: that it arbitrates where they overlap, that it keeps their
// three-way answer, that it can now be asked which CIUS a document declares,
// and that its answer routes.

// overlapDocs are the documents that show the twelve Is* predicates are not a
// partition. Each is read by two predicates as their own format, and each
// resolves under Detect's documented order to exactly one.
//
// The rows carry the *whole* predicate answer, not just the pair that overlaps,
// because the point of the test is that both contracts hold at once: the
// per-predicate answer is unchanged and still correct on its own terms, and the
// routing answer is single. Pinning only the overlap would let a future
// predicate widen — the way IsTurkishInvoice already matches any identifier
// beginning "TR" — and silently take over a route.
var overlapDocs = []struct {
	name string
	doc  string
	// true is the predicates that report true; every other predicate must
	// report (false, nil).
	true []string
	// want is the one answer Detect gives, and why the order gives it.
	want Source
	why  string
}{
	{
		name: "a distinguishing child of each of two formats",
		doc:  `<Invoice><Biller/><SellerParty/></Invoice>`,
		true: []string{"IsEbInterface", "IsSvefaktura"},
		want: SourceEbInterface,
		why:  "step 4: Biller belongs to ebInterface's own vocabulary, SellerParty is an ordinary UBL 1.0 party role",
	},
	{
		name: "a specification identifier naming two profiles",
		doc:  `<Invoice><CustomizationID>TR-OIOUBL-2.02</CustomizationID></Invoice>`,
		true: []string{"IsOIOUBL", "IsTurkishInvoice"},
		want: SourceOIOUBL,
		why:  "step 2b: a brand name inside the identifier beats a two-character prefix over a shared namespace",
	},
	{
		name: "a PINT identifier with a ZATCA profile identifier",
		doc: `<Invoice><CustomizationID>urn:peppol:pint:x</CustomizationID>` +
			`<ProfileID>reporting:1.0</ProfileID></Invoice>`,
		true: []string{"IsZATCA", "IsPINT"},
		want: SourcePINT,
		why:  "steps 2a and 3: BT-24 is a claim about the rule set, a profile identifier is not",
	},
}

// TestDetectArbitratesWhereThePredicatesOverlap holds both halves of the
// contract on the same three documents.
//
// The overlap is not a defect in the predicates — IsOIOUBL means "the
// specification identifier says OIOUBL", which is true of "TR-OIOUBL-2.02" —
// it is a missing arbitration. Before Detect the package shipped twelve
// independent tests and a README example that switched on them one at a time,
// so the answer a caller got depended on the order they happened to ask in, and
// every caller invented a different order. This test makes the ambiguity
// explicit rather than hiding it: the predicates keep answering as documented,
// and the routing answer is single and fixed.
func TestDetectArbitratesWhereThePredicatesOverlap(t *testing.T) {
	for _, tc := range overlapDocs {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.doc)

			wantTrue := map[string]bool{}
			for _, n := range tc.true {
				wantTrue[n] = true
			}
			if len(wantTrue) < 2 {
				t.Fatalf("this row records %d overlapping predicates; it proves nothing about arbitration", len(wantTrue))
			}
			for name, fn := range exportedDetectors {
				got, err := fn(data)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if got != wantTrue[name] {
					t.Errorf("%s = %v, want %v; the per-predicate contract is that each is an independent test",
						name, got, wantTrue[name])
				}
			}

			det, err := Detect(data)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if det.Source != tc.want {
				t.Errorf("Detect = %q, want %q (%s)", det.Source, tc.want, tc.why)
			}
			if !det.Recognised() {
				t.Error("Detect recognised nothing in a document two predicates both claim")
			}
			if det.Validator() == nil {
				t.Errorf("Detect returned %q, which routes to no validator", det.Source)
			}
		})
	}
}

// TestDetectSourceAndCIUSAgreeOnPINT is C24 pinned at the smallest scale.
//
// This test used to assert the opposite of its second claim — that a PINT
// invoice reports CIUSPeppol — and recorded the disagreement between Detect and
// the BT-24 dispatch as a documented property. It was not a property. A PINT
// identifier contains the substring "peppol", the CIUS dispatch had no case for
// PINT, and so every PINT document reaching ValidateCIUS was validated against
// Peppol BIS Billing 3.0: a different rule set, whose first rule requires the
// BT-24 a PINT invoice by definition does not carry. The two answers are one
// answer now, and the assertion says so.
func TestDetectSourceAndCIUSAgreeOnPINT(t *testing.T) {
	const doc = `<Invoice><CustomizationID>urn:peppol:pint:billing-1@my-1</CustomizationID></Invoice>`
	det, err := Detect([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if det.Source != SourcePINT {
		t.Errorf("Source = %q, want %q", det.Source, SourcePINT)
	}
	if det.CIUS != CIUSPINT {
		t.Errorf("CIUS = %q, want %q: an identifier that names PINT must not report the CIUS of a rule set it does not follow",
			det.CIUS, CIUSPINT)
	}
	if det.CIUS != DetectCIUS(det.SpecID) {
		t.Errorf("CIUS = %q but DetectCIUS(SpecID) = %q; the field is documented as the second",
			det.CIUS, DetectCIUS(det.SpecID))
	}
	if got := ciusSource(det.CIUS); got != det.Source {
		t.Errorf("ciusSource(CIUS) = %q but Source = %q; the CIUS and the rule set must name the same authority", got, det.Source)
	}
}

// TestDetectSeparatesUnrecognisedFromUnreadable keeps Detect on the discipline
// every Is* predicate already obeys. There are three answers, not two: a format,
// a document that was read and is no format this package validates, and a
// document that could not be read at all. Folding the middle one into the error
// would say the file is broken when it is merely foreign; folding it the other
// way would route a truncated invoice somewhere.
func TestDetectSeparatesUnrecognisedFromUnreadable(t *testing.T) {
	unreadable := map[string]string{
		"truncated mid-element": `<?xml version="1.0"?><Facturae><FileHeader><Schema`,
		"mismatched end tag":    `<?xml version="1.0"?><Facturae><a></b></Facturae>`,
		"empty input":           ``,
		"not XML at all":        "%PDF-1.7\x00binary",
		"unimplemented charset": `<?xml version="1.0" encoding="EBCDIC-CP-BE"?><Facturae/>`,
	}
	for name, doc := range unreadable {
		det, err := Detect([]byte(doc))
		if err == nil {
			t.Errorf("%s: Detect = %+v, nil; unreadable input must be an error", name, det)
		}
		if det != (Detection{}) {
			t.Errorf("%s: Detect returned %+v alongside an error; the value must mean nothing", name, det)
		}
	}

	// Read, and no format this package validates. Root is still reported: it is
	// the evidence the answer rests on, and a caller logging a rejection wants it.
	unrecognised := map[string]string{
		"an unrelated root":       `<?xml version="1.0"?><SomeUnrelatedRoot/>`,
		"a UBL despatch advice":   `<DespatchAdvice><ID>1</ID></DespatchAdvice>`,
		"the KSeF root, no head":  `<Faktura><Inne/></Faktura>`,
		"a document with no root": `<!-- nothing here -->`,
	}
	for name, doc := range unrecognised {
		det, err := Detect([]byte(doc))
		if name == "a document with no root" {
			// No root element at all is unreadable, not unrecognised: parseCII
			// says so too, and detection must not diverge from it.
			if err == nil {
				t.Errorf("%s: Detect = %+v, nil; a document with no root element cannot be read", name, det)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v; the document is well-formed and must be read, not refused", name, err)
			continue
		}
		if det.Recognised() {
			t.Errorf("%s: Detect = %q, want no format", name, det.Source)
		}
		if det.Source != SourceNone {
			t.Errorf("%s: Source = %q, want %q", name, det.Source, SourceNone)
		}
		if det.Validator() != nil {
			t.Errorf("%s: an unrecognised document routed to a validator", name)
		}
		if det.Root == "" {
			t.Errorf("%s: Detect dropped the root element name, which is what the answer rests on", name)
		}
	}
}

// TestDetectExtractsTheSpecificationIdentifier is the affordance that was
// missing: DetectCIUS took a BT-24 string and nothing exported could produce
// one from XML, so a caller who wanted to route on the CIUS — to log it, meter
// it, pick a downstream schema, or reject before paying for a validation — had
// to re-implement namespace-agnostic parsing of two different syntaxes.
//
// The identifier is checked against mustSpecID, which reads it through the
// semantic mappers ValidateCIUS itself dispatches on, so this pins the two
// readings together rather than asserting a literal twice.
func TestDetectExtractsTheSpecificationIdentifier(t *testing.T) {
	// The CII case is the one the streaming scan had to grow a capture for: the
	// identifier is three elements below the root, not directly under it.
	ciiXRechnung := strings.Replace(validCII, "urn:cen.eu:en16931:2017",
		"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", 1)
	if ciiXRechnung == validCII {
		t.Fatal("the CII fixture no longer carries the identifier this test rewrites")
	}

	cases := []struct {
		name       string
		doc        string
		wantSpecID string
		wantCIUS   CIUS
		wantSource Source
	}{
		{"CII, EN 16931 core", validCII, "urn:cen.eu:en16931:2017", CIUSNone, SourceEN16931},
		{"CII, XRechnung", ciiXRechnung,
			"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", CIUSXRechnung, SourceXRechnung},
		{"UBL, XRechnung", minimalXRechnungUBL,
			"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0", CIUSXRechnung, SourceXRechnung},
		{"UBL, Peppol", minimalPeppolUBL,
			"urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0", CIUSPeppol, SourcePeppol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			det, err := Detect([]byte(tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			if det.SpecID != tc.wantSpecID {
				t.Errorf("SpecID = %q, want %q", det.SpecID, tc.wantSpecID)
			}
			// The dispatcher's own reading of BT-24, through the mappers.
			if want := mustSpecID(t, tc.doc); det.SpecID != want {
				t.Errorf("SpecID = %q, but ValidateCIUS routes on %q", det.SpecID, want)
			}
			if det.CIUS != tc.wantCIUS {
				t.Errorf("CIUS = %q, want %q", det.CIUS, tc.wantCIUS)
			}
			if det.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", det.Source, tc.wantSource)
			}
		})
	}

	// A root that is neither syntax carries no BT-24 this package could quote,
	// and guessing one from a same-named element would be worse than silence.
	det, err := Detect([]byte(`<Facturae><CustomizationID>urn:cen.eu:en16931:2017</CustomizationID></Facturae>`))
	if err != nil {
		t.Fatal(err)
	}
	if det.SpecID != "" || det.CIUS != CIUSNone {
		t.Errorf("a Facturae root reported SpecID %q / CIUS %q; neither mapper reads that element",
			det.SpecID, det.CIUS)
	}
}

// TestDetectRoutesEveryFormatToAValidator makes the routing total. An order
// nobody can act on without re-deriving a format-to-validator table of their own
// has only moved the problem, so every Source Detect can return must name an
// entry point, and the two that are not formats must name none.
func TestDetectRoutesEveryFormatToAValidator(t *testing.T) {
	for _, src := range allSources {
		got := Detection{Source: src}.Validator()
		if src == SourceChecker {
			if got != nil {
				t.Errorf("Source %q routes to a validator; it is this package speaking about its own run, not a format", src)
			}
			continue
		}
		if got == nil {
			t.Errorf("Source %q routes to no validator, so a caller detecting it has nowhere to go", src)
		}
	}
	if (Detection{}).Validator() != nil {
		t.Error("the zero Detection routes somewhere; recognising nothing must route nowhere")
	}
}

// TestDetectedValidatorRunsTheRuleSetItNamed closes the loop end to end: the
// document is detected, the validator the detection names is called, and the
// Report it returns declares the coverage of the Source that was detected. A
// routing that reached a different rule set would show up here as a coverage
// claim that does not contain the detected Source's gaps.
func TestDetectedValidatorRunsTheRuleSetItNamed(t *testing.T) {
	cases := map[string]struct {
		doc  string
		want Source
	}{
		"UBL XRechnung":     {minimalXRechnungUBL, SourceXRechnung},
		"UBL Peppol":        {minimalPeppolUBL, SourcePeppol},
		"CII EN 16931":      {validCII, SourceEN16931},
		"OIOUBL":            {`<Invoice><CustomizationID>OIOUBL-2.1</CustomizationID></Invoice>`, SourceOIOUBL},
		"UBL-TR":            {`<Invoice><CustomizationID>TR1.2</CustomizationID></Invoice>`, SourceUBLTR},
		"PINT":              {`<Invoice><CustomizationID>urn:peppol:pint:billing-1@sg-1</CustomizationID></Invoice>`, SourcePINT},
		"ZATCA":             {`<Invoice><ProfileID>reporting:1.0</ProfileID></Invoice>`, SourceZATCA},
		"ebInterface":       {`<Invoice><Biller/></Invoice>`, SourceEbInterface},
		"Svefaktura":        {`<Invoice><SellerParty/></Invoice>`, SourceSvefaktura},
		"Facturae":          {`<Facturae><FileHeader/></Facturae>`, SourceFacturae},
		"FatturaPA":         {`<FatturaElettronica/>`, SourceFatturaPA},
		"NAV OSA":           {`<InvoiceData/>`, SourceOSA},
		"Finvoice":          {`<Finvoice/>`, SourceFinvoice},
		"TEAPPS":            {`<INVOICE_CENTER/>`, SourceTEAPPS},
		"KSeF":              {`<Faktura><Naglowek/></Faktura>`, SourceKSeF},
		"Order-X":           {`<SCRDMCCBDACIOMessageStructure/>`, SourceOrderX},
		"NLCIUS":            {`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0</CustomizationID></Invoice>`, SourceNLCIUS},
		"CIUS-PT":           {`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:2.1.1</CustomizationID></Invoice>`, SourceCIUSPT},
		"CIUS-RO":           {`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:efactura.mfinante.ro:CIUS-RO:1.0.0</CustomizationID></Invoice>`, SourceCIUSRO},
		"UBL.BE":            {`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#conformant#urn:UBL.BE:1.0.0.20180214</CustomizationID></Invoice>`, SourceUBLBE},
		"SRBDT":             {`<Invoice><CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022</CustomizationID></Invoice>`, SourceSRBDT},
		"unrecognised root": {`<SomeUnrelatedRoot/>`, SourceNone},
	}

	// Every format this package validates has a row, so a new Source cannot be
	// added without deciding how Detect reaches it.
	covered := map[Source]bool{}
	for _, tc := range cases {
		covered[tc.want] = true
	}
	for _, src := range allSources {
		if src != SourceChecker && !covered[src] {
			t.Errorf("no document in this table detects as %q", src)
		}
	}

	ctx := context.Background()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			det, err := Detect([]byte(tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			if det.Source != tc.want {
				t.Fatalf("Detect = %q, want %q", det.Source, tc.want)
			}
			v := det.Validator()
			if tc.want == SourceNone {
				if v != nil {
					t.Error("an unrecognised document routed to a validator")
				}
				return
			}
			if v == nil {
				t.Fatalf("%q routes to no validator", det.Source)
			}
			rep := v(ctx, []byte(tc.doc))
			gaps := map[string]bool{}
			for _, g := range rep.NotEvaluated {
				gaps[g] = true
			}
			for _, g := range Coverage(det.Source) {
				if !gaps[g] {
					t.Errorf("the validator Detect chose for %q did not declare that Source's coverage gap %q; "+
						"the routing reached a different rule set", det.Source, g)
				}
			}
		})
	}
}
