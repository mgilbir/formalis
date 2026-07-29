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
