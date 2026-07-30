package formalis

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPeppolCodeListsMatchTheSchematron is the fidelity check on
// peppol_codelists.go. Peppol ships its code lists as XPath `tokenize('A B C',
// '\s')` sequences inside the Schematron rather than as genericode, so the
// artefact this repository already vendors is the source of truth and the tables
// are a transcription of it.
//
// It also checks the three lists this package deliberately does *not* copy —
// Peppol's MIME, UNCL 5189 and UNCL 7161 lists are the same sets CEN publishes, so
// CL001/CL002/CL003 read en16931MIME, en16931AllowanceReasons and
// en16931ChargeReasons. That reuse is a claim about two authorities agreeing, and
// it is the kind of claim that stops being true without anyone noticing.
//
// And it checks that the two bindings carry identical lists, which is why one set
// of tables can serve both.
func TestPeppolCodeListsMatchTheSchematron(t *testing.T) {
	ubl := peppolSchematronLists(t, "PEPPOL-EN16931-UBL.sch")
	cii := peppolSchematronLists(t, "PEPPOL-EN16931-CII.sch")
	for name, want := range ubl {
		if got, ok := cii[name]; ok && !peppolSameSet(got, want) {
			t.Errorf("the two bindings declare different %s lists, so one Go table cannot serve both", name)
		}
	}
	for _, tc := range []struct {
		list  string
		table map[string]bool
		what  string
	}{
		{"ISO4217", peppolCurrencies, "peppolCurrencies"},
		{"eaid", peppolEAS, "peppolEAS"},
		{"UNCL2005", peppolPeriodDescCodes, "peppolPeriodDescCodes"},
		{"MIMECODE", en16931MIME, "en16931MIME, reused for PEPPOL-EN16931-CL001"},
		{"UNCL5189", en16931AllowanceReasons, "en16931AllowanceReasons, reused for PEPPOL-EN16931-CL002"},
		{"UNCL7161", en16931ChargeReasons, "en16931ChargeReasons, reused for PEPPOL-EN16931-CL003"},
	} {
		want, ok := ubl[tc.list]
		if !ok {
			t.Errorf("the UBL binding no longer declares a $%s variable", tc.list)
			continue
		}
		peppolDiffSets(t, tc.what, tc.list, tc.table, want)
	}
	// The two invoice type code lists are not <let> variables; they are inline
	// tokenize() arguments in the P0100/P0101 assertions, so they are read out of
	// the assertion's own test attribute.
	for _, tc := range []struct {
		file, rule string
		table      map[string]bool
		what       string
	}{
		{"PEPPOL-EN16931-UBL.sch", "PEPPOL-EN16931-P0100", peppolInvoiceTypeCodes, "peppolInvoiceTypeCodes"},
		{"PEPPOL-EN16931-UBL.sch", "PEPPOL-EN16931-P0101", peppolCreditNoteTypeCodes, "peppolCreditNoteTypeCodes"},
		{"PEPPOL-EN16931-CII.sch", "PEPPOL-EN16931-P0100", peppolCIITypeCodes, "peppolCIITypeCodes"},
	} {
		want := peppolAssertionTokens(t, tc.file, tc.rule)
		if want == nil {
			t.Errorf("%s does not declare an inline code list on %s", tc.file, tc.rule)
			continue
		}
		peppolDiffSets(t, tc.what, tc.rule+" in "+tc.file, tc.table, want)
	}
}

func peppolDiffSets(t *testing.T, what, source string, got, want map[string]bool) {
	t.Helper()
	for code := range want {
		if !got[code] {
			t.Errorf("%s is missing %q, which %s declares", what, code, source)
		}
	}
	for code := range got {
		if !want[code] {
			t.Errorf("%s holds %q, which %s does not declare", what, code, source)
		}
	}
}

func peppolSameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// peppolSchematronLists reads the `<let name="X" value="tokenize('...', '\s')"/>`
// declarations of one binding file.
func peppolSchematronLists(t *testing.T, file string) map[string]map[string]bool {
	t.Helper()
	path := filepath.Join("testdata", "peppol", "repo", "rules", "sch", file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("OpenPEPPOL Schematron not present (make cius-oracles)")
	}
	out := map[string]map[string]bool{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "let" {
			continue
		}
		var name, value string
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "name":
				name = a.Value
			case "value":
				value = a.Value
			}
		}
		if codes := peppolTokenizeArgument(value); codes != nil {
			out[name] = codes
		}
	}
	return out
}

// peppolAssertionTokens reads the inline tokenize() argument of one assertion.
func peppolAssertionTokens(t *testing.T, file, rule string) map[string]bool {
	t.Helper()
	path := filepath.Join("testdata", "peppol", "repo", "rules", "sch", file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("OpenPEPPOL Schematron not present (make cius-oracles)")
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "assert" {
			continue
		}
		id, test := "", ""
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "id":
				id = a.Value
			case "test":
				test = a.Value
			}
		}
		if id != rule {
			continue
		}
		if codes := peppolTokenizeArgument(test); codes != nil {
			return codes
		}
	}
}

// peppolTokenizeArgument extracts the whitespace-separated codes of the first
// `tokenize('...', '\s')` in an XPath expression. It is deliberately not a regular
// expression over the artefact: the input here is one attribute value a decoder
// has already handed over, entity references resolved, which is the distinction
// C31 turns on.
func peppolTokenizeArgument(expr string) map[string]bool {
	const open = "tokenize('"
	i := strings.Index(expr, open)
	if i < 0 {
		return nil
	}
	rest := expr[i+len(open):]
	j := strings.Index(rest, "'")
	if j < 0 {
		return nil
	}
	fields := strings.Fields(rest[:j])
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]bool, len(fields))
	for _, f := range fields {
		out[f] = true
	}
	return out
}
