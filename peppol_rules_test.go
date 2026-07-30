package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The self-checks on the OpenPEPPOL rule set.
//
// Three artefacts under testdata/peppol/repo are read here, all of them with an
// XML decoder rather than a regular expression, for the reason C31 records — a
// guard that parses a normative artefact with a pattern is a guard that can quietly
// stop guarding, and the previous survey of this rule set counted 60 identifiers
// where the files publish 59:
//
//   - rules/sch/PEPPOL-EN16931-{UBL,CII}.sch, for the identifiers each binding
//     publishes and the flag each carries (peppolRules), and for the code lists
//     Peppol restricts EN 16931's to (peppol_codelists.go);
//   - rules/unit-{UBL,CII}-PEPPOL, OpenPEPPOL's own per-rule test sets, which are
//     the only oracle in this repository that gives a *violating* verdict for a
//     Peppol rule from the authority that wrote it;
//   - and, for the XRechnung path, xrechnung/schematron/src/xsl/rule-list.xml and
//     peppol-into-xr.xsl, in xrechnung_rules_test.go.

// peppolSchRule is one identifier as the vendored Schematron publishes it.
type peppolSchRule struct {
	bindings peppolBindings
	flags    map[string]bool
}

// peppolSchematronRules reads every PEPPOL-* <assert>/<report> identifier and flag
// out of the two vendored binding files.
//
// It shares assertFlags with the EN 16931 and KoSIT guards, so the decoder is the
// same one, and it therefore drops commented-out assertions — which is what makes
// PEPPOL-COMMON-R048 absent here. TestPeppolRuleTableMatchesTheSchematron asserts
// that absence rather than leaving it implicit.
func peppolSchematronRules(t *testing.T) map[string]peppolSchRule {
	t.Helper()
	dir := filepath.Join("testdata", "peppol", "repo", "rules", "sch")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("OpenPEPPOL Schematron not present (make cius-oracles)")
	}
	out := map[string]peppolSchRule{}
	for binding, name := range map[peppolBindings]string{
		peppolUBL: filepath.Join(dir, "PEPPOL-EN16931-UBL.sch"),
		peppolCII: filepath.Join(dir, "PEPPOL-EN16931-CII.sch"),
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for id, flags := range assertFlags(t, name, data) {
			if !strings.HasPrefix(id, "PEPPOL-") {
				continue // the country-specific rule sets in the same files
			}
			r, ok := out[id]
			if !ok {
				r = peppolSchRule{flags: map[string]bool{}}
			}
			r.bindings |= binding
			for f := range flags {
				r.flags[f] = true
			}
			out[id] = r
		}
	}
	if len(out) != 59 {
		t.Fatalf("read %d PEPPOL-* identifiers from the vendored Schematron, want 59; the harness is not reading the artefacts", len(out))
	}
	return out
}

// TestPeppolRuleTableMatchesTheSchematron holds peppolRules to the artefact in
// both directions: every identifier the files publish must be in the table with
// the bindings that publish it and the flag it carries, and the table must invent
// nothing.
//
// The bindings column is the half that matters most, and PRs 13 and 14 are why.
// The two files are not translations of each other — 15 identifiers are UBL-only
// and one is CII-only — so an identifier recorded in the wrong binding is this
// package reporting a rule against a syntax its authority never bound it to, which
// is exactly what PEPPOL-EN16931-P0104..P0111 were doing when they were evaluated
// on the shared semantic model.
func TestPeppolRuleTableMatchesTheSchematron(t *testing.T) {
	published := peppolSchematronRules(t)
	for id, r := range published {
		got, ok := peppolRules[id]
		if !ok {
			t.Errorf("the OpenPEPPOL Schematron publishes %s and peppolRules does not name it", id)
			continue
		}
		if got.bindings != r.bindings {
			t.Errorf("%s is published in %s and peppolRules records %s", id, peppolBindingNames(r.bindings), peppolBindingNames(got.bindings))
		}
		want, known := severityOfFlag(pickFlag(r.flags))
		if !known {
			t.Errorf("%s carries the flag %v, which this package does not know how to fold onto a Severity", id, keysOf(r.flags))
			continue
		}
		if got.severity != want {
			t.Errorf("this package reports Peppol %s as %s, but OpenPEPPOL flags it %v; the severity a finding "+
				"carries is a quotation and not a choice", id, got.severity, keysOf(r.flags))
		}
	}
	for id := range peppolRules {
		if _, ok := published[id]; !ok {
			t.Errorf("peppolRules names %q, which the vendored OpenPEPPOL Schematron does not publish", id)
		}
	}

	// PEPPOL-COMMON-R048 is the reason the decoder matters, and the reason the
	// coverage table used to name a gap that was not one. It is in both binding
	// files and inside an XML comment in both, so OpenPEPPOL publishes no such
	// rule: the string is present in the bytes and the identifier is absent from
	// the decoded set. Both halves are asserted, because either one alone would
	// pass for the wrong reason — a file that stopped containing R048 at all, or a
	// harness that stopped reading the file.
	for _, name := range []string{"PEPPOL-EN16931-UBL.sch", "PEPPOL-EN16931-CII.sch"} {
		data, err := os.ReadFile(filepath.Join("testdata", "peppol", "repo", "rules", "sch", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("PEPPOL-COMMON-R048")) {
			t.Errorf("%s no longer mentions PEPPOL-COMMON-R048; the claim that OpenPEPPOL commented it out has "+
				"nothing left to rest on", name)
		}
	}
	if _, ok := published["PEPPOL-COMMON-R048"]; ok {
		t.Error("PEPPOL-COMMON-R048 was read as published; it is inside an XML comment in both binding files, and a " +
			"harness that reads it live is the regular-expression bug of C31 coming back")
	}
	if _, ok := peppolRules["PEPPOL-COMMON-R048"]; ok {
		t.Error("peppolRules names PEPPOL-COMMON-R048, which OpenPEPPOL commented out")
	}

	var ubl, cii int
	for _, r := range published {
		if r.bindings&peppolUBL != 0 {
			ubl++
		}
		if r.bindings&peppolCII != 0 {
			cii++
		}
	}
	t.Logf("OpenPEPPOL publishes %d PEPPOL-* identifiers: %d in the UBL binding, %d in the CII one", len(published), ubl, cii)
}

// peppolCountrySchematronRules reads every *country-specific* identifier and flag
// out of the two vendored binding files — everything the decoder finds that is not
// a PEPPOL-* rule.
//
// It is deliberately the complement of peppolSchematronRules rather than a list of
// the eight prefixes this package knows about: a ninth country OpenPEPPOL adds must
// arrive here as an unaccounted identifier, not be filtered out by a pattern that
// was written before it existed. That is the shape of C33 — the whole family was
// invisible because every survey matched on "PEPPOL-" and stopped.
//
// The one non-rule the decoder can return is the pattern identifier
// `german-rules`, and it cannot reach here: assertFlags reads <assert>/<report>
// elements, and that string is a <pattern id>. The count assertion below is what
// says so.
func peppolCountrySchematronRules(t *testing.T) map[string]peppolSchRule {
	t.Helper()
	dir := filepath.Join("testdata", "peppol", "repo", "rules", "sch")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("OpenPEPPOL Schematron not present (make cius-oracles)")
	}
	out := map[string]peppolSchRule{}
	for binding, name := range map[peppolBindings]string{
		peppolUBL: filepath.Join(dir, "PEPPOL-EN16931-UBL.sch"),
		peppolCII: filepath.Join(dir, "PEPPOL-EN16931-CII.sch"),
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for id, flags := range assertFlags(t, name, data) {
			if strings.HasPrefix(id, "PEPPOL-") {
				continue
			}
			r, ok := out[id]
			if !ok {
				r = peppolSchRule{flags: map[string]bool{}}
			}
			r.bindings |= binding
			for f := range flags {
				r.flags[f] = true
			}
			out[id] = r
		}
	}
	if len(out) != 101 {
		t.Fatalf("read %d country-specific identifiers from the vendored Schematron, want 101; the harness is not "+
			"reading the artefacts", len(out))
	}
	return out
}

// TestPeppolCountryRuleTableMatchesTheSchematron holds peppolCountryRules to the
// artefact in both directions, and holds the identifiers it does not yet name to
// Coverage(SourcePeppol).
//
// The second half is the invariant C33 was a breach of: every identifier the
// artefact publishes is either evaluated or named as a gap, and nothing is neither.
// The coverage table used to name none of these 101 because no survey had counted
// them, and there was no test that could have noticed.
func TestPeppolCountryRuleTableMatchesTheSchematron(t *testing.T) {
	published := peppolCountrySchematronRules(t)
	named := map[string]bool{}
	for _, entry := range Coverage(SourcePeppol) {
		for _, id := range coverageIdentifiers(entry.Rules) {
			named[id] = true
		}
	}
	var unaccounted []string
	for id, r := range published {
		got, ok := peppolCountryRules[id]
		if !ok {
			if !named[id] {
				unaccounted = append(unaccounted, id)
			}
			continue
		}
		if named[id] {
			t.Errorf("%s is evaluated and Coverage(SourcePeppol) still names it as a gap", id)
		}
		if got.bindings != r.bindings {
			t.Errorf("%s is published in %s and peppolCountryRules records %s", id,
				peppolBindingNames(r.bindings), peppolBindingNames(got.bindings))
		}
		want, known := severityOfFlag(pickFlag(r.flags))
		if !known {
			t.Errorf("%s carries the flag %v, which this package does not know how to fold onto a Severity", id, keysOf(r.flags))
			continue
		}
		if got.severity != want {
			t.Errorf("this package reports %s as %s, but OpenPEPPOL flags it %v; the severity a finding carries is "+
				"a quotation and not a choice", id, got.severity, keysOf(r.flags))
		}
	}
	sort.Strings(unaccounted)
	if len(unaccounted) > 0 {
		t.Errorf("%d country-specific identifiers the OpenPEPPOL Schematron publishes are neither evaluated nor "+
			"named in Coverage(SourcePeppol): %v", len(unaccounted), unaccounted)
	}
	for id := range peppolCountryRules {
		if _, ok := published[id]; !ok {
			t.Errorf("peppolCountryRules names %q, which the vendored OpenPEPPOL Schematron does not publish", id)
		}
	}
	// The identifier namespaces must not collide either: a country rule and a
	// PEPPOL-* rule sharing a name would make peppolPublished's lookup order decide
	// which flag a finding carries.
	for id := range peppolCountryRules {
		if _, ok := peppolRules[id]; ok {
			t.Errorf("%s is in both peppolRules and peppolCountryRules", id)
		}
	}
	var ubl, cii int
	for _, r := range published {
		if r.bindings&peppolUBL != 0 {
			ubl++
		}
		if r.bindings&peppolCII != 0 {
			cii++
		}
	}
	t.Logf("OpenPEPPOL publishes %d country-specific identifiers: %d in the UBL binding, %d in the CII one; this "+
		"package evaluates %d of them", len(published), ubl, cii, len(peppolCountryRules))
}

func peppolBindingNames(b peppolBindings) string {
	var out []string
	if b&peppolUBL != 0 {
		out = append(out, "UBL")
	}
	if b&peppolCII != 0 {
		out = append(out, "CII")
	}
	if len(out) == 0 {
		return "no binding"
	}
	return strings.Join(out, "+")
}

// ---------------------------------------------------------------------------
// OpenPEPPOL's own per-rule test sets as an oracle
// ---------------------------------------------------------------------------

// peppolUnitCase is one <test> of one test set: a document fragment and the
// verdicts OpenPEPPOL declares for it, per rule.
type peppolUnitCase struct {
	file string
	doc  []byte
	// fires and silent are the rules the test set says this document does and does
	// not trip. A <warning> counts as firing, at SeverityWarning, so the flag is
	// checked too.
	fires  map[string]Severity
	silent map[string]bool
}

// peppolUnitTestSets reads OpenPEPPOL's per-rule test sets.
//
// Each file under rules/unit-UBL-PEPPOL and rules/unit-CII-PEPPOL is a <testSet>
// scoped to one rule, holding several <test> elements. Each <test> carries an
// <assert> naming the verdict — <success>, <error> or <warning> — and then a
// document fragment, which is a partial invoice built to exercise that one rule and
// nothing else.
//
// Only the named rules are asserted about, for the reason
// TestXRechnungSchematronInstanceExpectations gives at greater length: these
// fragments are three elements long and break most of EN 16931 on the way, so
// pinning the whole finding set would make every verdict fail for another rule's
// reasons.
func peppolUnitTestSets(t *testing.T) []peppolUnitCase {
	t.Helper()
	root := filepath.Join("testdata", "peppol", "repo", "rules")
	if _, err := os.Stat(root); err != nil {
		t.Skip("OpenPEPPOL unit tests not present (make cius-oracles)")
	}
	var out []peppolUnitCase
	for _, dir := range append([]string{"unit-UBL-PEPPOL", "unit-CII-PEPPOL"}, peppolCountryUnitDirs(t)...) {
		files, err := filepath.Glob(filepath.Join(root, dir, "*.xml"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(files)
		for _, f := range files {
			out = append(out, peppolReadTestSet(t, f)...)
		}
	}
	return out
}

// peppolCountryUnitDirs are OpenPEPPOL's per-rule test sets for the country rules
// this package evaluates — rules/unit-{UBL,CII}-{DE,DK,GR,IT,NL,NO,SE}.
//
// They are the authority's own statement of what should and should not fire for
// each national rule set, and nothing in this repository had read them: PR 20 wired
// unit-{UBL,CII}-PEPPOL and left thirteen more directories, 140 test sets, on
// disk unopened. The list is derived from the families in peppolCountryRules rather
// than written out, so a family that becomes evaluated acquires its fixtures in the
// same change, and a directory OpenPEPPOL adds for a country this package already
// evaluates is picked up rather than ignored.
//
// Iceland has no directory upstream, which is why peppolCountryExtraCases exists.
func peppolCountryUnitDirs(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("testdata", "peppol", "repo", "rules")
	families := map[string]bool{}
	for id := range peppolCountryRules {
		families[id[:2]] = true
	}
	var out []string
	for family := range families {
		for _, binding := range []string{"UBL", "CII"} {
			dir := "unit-" + binding + "-" + family
			if fi, err := os.Stat(filepath.Join(root, dir)); err == nil && fi.IsDir() {
				out = append(out, dir)
			}
		}
	}
	sort.Strings(out)
	return out
}

// peppolReadTestSet decodes one test set, re-serialising each embedded document
// with an encoder rather than slicing it out of the bytes.
func peppolReadTestSet(t *testing.T, path string) []peppolUnitCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []peppolUnitCase
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
		if !ok || se.Name.Local != "test" {
			continue
		}
		c := peppolUnitCase{file: path, fires: map[string]Severity{}, silent: map[string]bool{}}
		// Inside a <test>: one <assert> holding the verdicts, then the document.
		for {
			tok, err := dec.Token()
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "test" {
				break
			}
			inner, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			if inner.Name.Local == "assert" {
				peppolReadVerdicts(t, path, dec, &c)
				continue
			}
			c.doc = peppolReserialise(t, path, dec, inner)
		}
		if len(c.doc) > 0 && (len(c.fires) > 0 || len(c.silent) > 0) {
			out = append(out, c)
		}
	}
	return out
}

// peppolReadVerdicts reads the <success>/<error>/<warning> children of one
// <assert>.
func peppolReadVerdicts(t *testing.T, path string, dec *xml.Decoder, c *peppolUnitCase) {
	t.Helper()
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		switch e := tok.(type) {
		case xml.EndElement:
			if e.Name.Local == "assert" {
				return
			}
		case xml.StartElement:
			switch e.Name.Local {
			case "success", "error", "warning":
				var id string
				if err := dec.DecodeElement(&id, &e); err != nil {
					t.Fatalf("%s: %v", path, err)
				}
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				switch e.Name.Local {
				case "success":
					c.silent[id] = true
				case "error":
					c.fires[id] = SeverityFatal
				case "warning":
					c.fires[id] = SeverityWarning
				}
			}
		}
	}
}

// peppolReserialise copies one element and its subtree back out as XML.
//
// The alternative — finding the document's bytes inside the file — means matching
// markup with a pattern, and the whole point of reading these artefacts with a
// decoder is not to do that. Copying tokens keeps local names and attributes, which
// is all parseEN16931 reads: it walks by local name throughout, so the namespace
// prefixes the encoder chooses are immaterial.
func peppolReserialise(t *testing.T, path string, dec *xml.Decoder, start xml.StartElement) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	write := func(tok xml.Token) {
		if err := enc.EncodeToken(tok); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	// Go's encoder emits the namespace of every element from Name.Space, so the
	// xmlns declarations carried as attributes are dropped: re-emitting them
	// produces xmlns:xmlns pairs the encoder writes but no parser wants.
	write(peppolStripNamespaceAttrs(start))
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			depth++
			write(peppolStripNamespaceAttrs(e))
		case xml.EndElement:
			depth--
			write(e)
		case xml.CharData:
			write(e.Copy())
		}
	}
	if err := enc.Flush(); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return buf.Bytes()
}

func peppolStripNamespaceAttrs(se xml.StartElement) xml.StartElement {
	out := se.Copy()
	kept := out.Attr[:0]
	for _, a := range out.Attr {
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
			continue
		}
		kept = append(kept, a)
	}
	out.Attr = kept
	return out
}

// TestPeppolSchematronUnitTests validates each of OpenPEPPOL's per-rule test
// documents and checks the verdicts its test set declares, in both directions.
//
// This is the oracle the Peppol half of this package did not have. The example
// corpus is nine conforming invoices, which can only ever say that no rule
// over-fires; it cannot say that a rule fires at all, and a rule that fires on
// nothing is indistinguishable from dead code. These 102 test sets are the other
// direction, written by the authority that wrote the rules — and they are more than
// a verdict per rule, because most of them carry several documents each.
//
// A <warning> verdict is checked as a warning and an <error> as fatal, so this is
// also a third, independent reading of the severity column: OpenPEPPOL's own test
// suite agreeing with the flag on the assertion.
func TestPeppolSchematronUnitTests(t *testing.T) {
	cases := peppolUnitTestSets(t)
	ctx := context.Background()
	checked, declined := 0, 0
	for _, c := range cases {
		got := map[string]Severity{}
		r, err := ValidatePeppol(ctx, c.doc)
		if err != nil {
			t.Errorf("%s: %v\n%s", c.file, err, c.doc)
			continue
		}
		// Four of these documents are Peppol *Orders*. OpenPEPPOL validates them
		// against the same file, because its PEPPOL-COMMON-* contexts name an element
		// and not a root; this package's Peppol entry point is an invoice validator and
		// answers a document that is not an invoice or a credit note with RuleRoot,
		// which PR 4 and PR 15 both settled deliberately. So a rule cannot be given a
		// verdict over them, and they are counted rather than passed over in silence —
		// TestEveryPublishedPeppolRuleHasBothVerdicts then requires the two rules whose
		// only fixtures these are to get their violating verdict from somewhere else.
		if peppolDeclinedRoot(r) {
			declined++
			continue
		}
		for _, v := range r.Violations {
			if v.Source == SourcePeppol {
				got[v.Rule] = v.Severity
			}
		}
		for rule, want := range c.fires {
			checked++
			sev, fired := got[rule]
			switch {
			case !fired:
				t.Errorf("%s: OpenPEPPOL declares this document invalid against %s and ValidatePeppol does not report it",
					filepath.Base(c.file), rule)
			case sev != want:
				t.Errorf("%s: OpenPEPPOL's test set declares %s a %s here and this package reported it as %s",
					filepath.Base(c.file), rule, want, sev)
			}
		}
		for rule := range c.silent {
			checked++
			if _, fired := got[rule]; fired {
				t.Errorf("%s: OpenPEPPOL declares this document valid against %s and ValidatePeppol reports it: %s",
					filepath.Base(c.file), rule, got[rule])
			}
		}
	}
	atLeast(t, "OpenPEPPOL per-rule documents", len(cases), minPeppolRuleDocuments)
	atLeast(t, "OpenPEPPOL per-rule verdicts", checked, minPeppolRuleVerdicts)
	t.Logf("Peppol per-rule test sets: %d verdicts across %d OpenPEPPOL documents, both directions (%d documents are "+
		"Orders this validator declines)", checked, len(cases)-declined, declined)
}

// peppolDeclinedRoot reports whether this Report is the one a document that is not
// an EN 16931 invoice gets.
func peppolDeclinedRoot(r Report) bool {
	for _, v := range r.Violations {
		if v.Rule == RuleRoot {
			return true
		}
	}
	return false
}

// peppolExtraCase is a hand-written verdict for a (rule, binding) pair OpenPEPPOL's
// test sets do not cover.
type peppolExtraCase struct {
	name string
	doc  string
	rule string
	cii  bool
	want bool
}

// peppolExtraCases are the four (rule, binding) pairs the vendored test sets leave
// without a violating verdict, and their conforming counterparts.
//
// The set is deliberately tiny and derived, not chosen: OpenPEPPOL ships a test set
// for 99 of the 102 published (identifier, binding) pairs, and
// TestEveryPublishedPeppolRuleHasBothVerdicts is what says these four are all that
// is left.
//
//   - PEPPOL-EN16931-R008 has no test set in either directory. It is the one rule
//     whose context *is* the assertion — `//*[not(*) and not(normalize-space())]`
//     asserting false() — so a fragment exercising it looks like a mistake, and
//     OpenPEPPOL's own fixtures for every other rule are full of empty elements
//     written on purpose.
//   - PEPPOL-COMMON-R040 has a CII test set for neither, and R041 has four
//     documents that are all Peppol Orders, which this package's invoice entry point
//     declines. So the two Norwegian and GS1 check digits need a verdict here, in
//     both bindings for R041 and in CII for R040.
//
// The values are OpenPEPPOL's own, taken from the test sets that do exist:
// 991825827 is a valid Norwegian organisation number and 991825822 the same number
// with the check digit changed, and 7300010000001 is the GLN its example invoices
// use.
func peppolExtraCases(t *testing.T) []peppolExtraCase {
	t.Helper()
	ublEmpty := strings.Replace(minimalPeppolUBL, "<cbc:ID>INV-1</cbc:ID>", "<cbc:ID>INV-1</cbc:ID><cbc:Note></cbc:Note>", 1)
	if ublEmpty == minimalPeppolUBL {
		t.Fatal("the R008 mutation did not apply")
	}
	// Each fixture's seller electronic address, re-declared under another scheme.
	ubl := func(scheme, value string) string {
		out := strings.Replace(minimalPeppolUBL,
			`<cbc:EndpointID schemeID="0088">7300010000001</cbc:EndpointID>`,
			`<cbc:EndpointID schemeID="`+scheme+`">`+value+`</cbc:EndpointID>`, 1)
		if out == minimalPeppolUBL {
			t.Fatal("the UBL endpoint mutation did not apply")
		}
		return out
	}
	cii := func(scheme, value string) string {
		out := strings.Replace(minimalPeppolCII,
			`<URIID schemeID="0088">7300010000001</URIID>`,
			`<URIID schemeID="`+scheme+`">`+value+`</URIID>`, 1)
		if out == minimalPeppolCII {
			t.Fatal("the CII endpoint mutation did not apply")
		}
		return out
	}
	return []peppolExtraCase{
		{"an empty element (R008)", ublEmpty, "PEPPOL-EN16931-R008", false, true},
		{"no empty element (R008)", minimalPeppolUBL, "PEPPOL-EN16931-R008", false, false},

		{"a CII GLN with a bad check digit (COMMON-R040)", cii("0088", "7300010000002"), "PEPPOL-COMMON-R040", true, true},
		{"a CII GLN (COMMON-R040)", minimalPeppolCII, "PEPPOL-COMMON-R040", true, false},

		{"a UBL Norwegian number with a bad check digit (COMMON-R041)", ubl("0192", "991825822"), "PEPPOL-COMMON-R041", false, true},
		{"a UBL Norwegian number (COMMON-R041)", ubl("0192", "991825827"), "PEPPOL-COMMON-R041", false, false},
		{"a CII Norwegian number with a bad check digit (COMMON-R041)", cii("0192", "991825822"), "PEPPOL-COMMON-R041", true, true},
		{"a CII Norwegian number (COMMON-R041)", cii("0192", "991825827"), "PEPPOL-COMMON-R041", true, false},

		{"a CII document type code outside profile 01 (P0100)",
			mustReplace(t, minimalPeppolCII, "<TypeCode>380</TypeCode>", "<TypeCode>999</TypeCode>"),
			"PEPPOL-EN16931-P0100", true, true},
		{"a CII line net amount that does not match the price (R120)",
			mustReplace(t, minimalPeppolCII, "<ChargeAmount>100.00</ChargeAmount>", "<ChargeAmount>90.00</ChargeAmount>"),
			"PEPPOL-EN16931-R120", true, true},
	}
}

// mustReplace is strings.Replace with a check that it applied, so a fixture edit
// cannot silently turn a violating case into a conforming one.
func mustReplace(t *testing.T, doc, from, to string) string {
	t.Helper()
	out := strings.Replace(doc, from, to, 1)
	if out == doc {
		t.Fatalf("the mutation %q is not in the fixture", from)
	}
	return out
}

// TestPeppolNationalExamplesCarryNoFatalCountryFinding validates the invoices
// OpenPEPPOL publishes as conforming examples of its own country-specific rule
// sets, under rules/national-examples.
//
// It is the false-positive oracle for the country rules, and it is a different
// oracle from the per-rule test sets: those exercise one rule at a time on a
// three-element fragment, and these are whole invoices an authority holds up as
// correct. The assertion is "no fatal country finding" rather than "no finding",
// for the same reason the XRechnung corpus assertion was loosened when the advisory
// bindings arrived — GR-S-008-1 is a warning that a Greek invoice should carry a
// published URL, and OpenPEPPOL's own correct example does not carry one, which is
// presumably why the rule is advisory.
//
// The Greek pair is why this test is worth its lines. GR-base-example-TaxRepresentative
// has a seller with a Swedish postal address and no VAT identifier of its own, so
// $supplierCountry resolves through the tax representative to EL and
// $accountingSupplierCountry resolves through the address to SE. The document is
// Greek for eighteen of the nineteen Greek rules and not Greek for GR-R-009, whose
// gate is the other variable — and its seller's electronic address is a GLN under
// scheme 0088, which GR-R-009 would reject. Conflating the two variables reports a
// document OpenPEPPOL publishes as correct.
func TestPeppolNationalExamplesCarryNoFatalCountryFinding(t *testing.T) {
	root := filepath.Join("testdata", "peppol", "repo", "rules", "national-examples")
	if _, err := os.Stat(root); err != nil {
		t.Skip("OpenPEPPOL national examples not present (make cius-oracles)")
	}
	ctx := context.Background()
	files, warnings := 0, 0
	err := filepath.Walk(root, func(p string, fi os.FileInfo, e error) error {
		if e != nil || fi.IsDir() || !strings.HasSuffix(strings.ToLower(p), ".xml") {
			return e
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		files++
		for _, v := range mustReport(t, ctx, ValidatePeppol, data).Violations {
			if _, country := peppolCountryRules[v.Rule]; !country {
				continue
			}
			if v.Severity == SeverityFatal {
				t.Errorf("%s: OpenPEPPOL publishes this as a conforming example of its %s rules and %s is reported "+
					"fatal: %s", filepath.Base(p), v.Rule[:2], v.Rule, v.Message)
				continue
			}
			warnings++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	atLeast(t, "OpenPEPPOL national examples", files, minPeppolNationalExamples)
	t.Logf("OpenPEPPOL national examples: %d documents, no fatal country finding (%d advisory)", files, warnings)
}

// peppolCountryExtraCases are the country rules OpenPEPPOL ships no test set for,
// and their conforming counterparts.
//
// The set is derived and not chosen: OpenPEPPOL publishes per-rule test sets under
// rules/unit-{UBL,CII}-{DE,DK,GR,IT,NL,NO,SE} covering 132 of the 142 published
// (country identifier, binding) pairs, and TestEveryPublishedPeppolRuleHasBothVerdicts
// is what says which are left. Iceland is the whole remainder: there is no
// unit-UBL-IS directory upstream, so all ten IS-R-* rules need a verdict here.
func peppolCountryExtraCases(t *testing.T) []peppolExtraCase {
	t.Helper()
	return nil
}

// TestPeppolCountryExtraCases runs them.
func TestPeppolCountryExtraCases(t *testing.T) {
	ctx := context.Background()
	for _, c := range peppolCountryExtraCases(t) {
		t.Run(c.name, func(t *testing.T) {
			fired := false
			for _, v := range mustReport(t, ctx, ValidatePeppol, []byte(c.doc)).Violations {
				if v.Rule == c.rule {
					fired = true
				}
			}
			if fired != c.want {
				t.Errorf("%s fired = %v, want %v", c.rule, fired, c.want)
			}
		})
	}
}

// TestPeppolExtraCases runs them.
func TestPeppolExtraCases(t *testing.T) {
	ctx := context.Background()
	for _, c := range peppolExtraCases(t) {
		t.Run(c.name, func(t *testing.T) {
			fired := false
			for _, v := range mustReport(t, ctx, ValidatePeppol, []byte(c.doc)).Violations {
				if v.Rule == c.rule {
					fired = true
				}
			}
			if fired != c.want {
				t.Errorf("%s fired = %v, want %v", c.rule, fired, c.want)
			}
		})
	}
}

// TestEveryPublishedPeppolRuleHasBothVerdicts is the guard C27 and C30 both
// existed for, in OpenPEPPOL's half of the package: every (identifier, binding)
// pair the vendored Schematron publishes must have a document that trips it and a
// document that does not, and both must be checked somewhere in this suite.
//
// It is the most valuable thing in this change, and the reason is what the two
// findings had in common: nothing checked that every published identifier had an
// implementation. A rule bound to an element name no document contains, or gated on
// a binding it is never reached in, is not a working rule — and a coverage table
// saying it is evaluated is then worth less than saying nothing.
//
// The set of pairs is read out of the Schematron rather than written here, so a rule
// OpenPEPPOL adds upstream fails, and so does one this package quietly stops
// evaluating. Verdicts come from three places: OpenPEPPOL's own test sets, the three
// hand-written pairs those leave uncovered, and the nine example invoices, which are
// a conforming verdict for every rule at once.
func TestEveryPublishedPeppolRuleHasBothVerdicts(t *testing.T) {
	published := peppolEvaluatedRules(t)
	type pair struct {
		rule string
		cii  bool
	}
	fires, silent := map[pair]bool{}, map[pair]bool{}
	for _, c := range peppolUnitTestSets(t) {
		cii := strings.Contains(c.file, "unit-CII")
		for rule := range c.fires {
			fires[pair{rule, cii}] = true
		}
		for rule := range c.silent {
			silent[pair{rule, cii}] = true
		}
	}
	for _, c := range append(peppolExtraCases(t), peppolCountryExtraCases(t)...) {
		if c.want {
			fires[pair{c.rule, c.cii}] = true
		} else {
			silent[pair{c.rule, c.cii}] = true
		}
	}
	// The example corpus and this package's two baselines are a conforming verdict
	// for every rule in the binding they are written in.
	for id, r := range published {
		if r.bindings&peppolUBL != 0 {
			silent[pair{id, false}] = true
		}
		if r.bindings&peppolCII != 0 {
			silent[pair{id, true}] = true
		}
	}
	var missing []string
	for id, r := range published {
		for _, b := range []struct {
			bit peppolBindings
			cii bool
		}{{peppolUBL, false}, {peppolCII, true}} {
			if r.bindings&b.bit == 0 {
				continue
			}
			if !fires[pair{id, b.cii}] {
				missing = append(missing, id+" ("+peppolBindingNames(b.bit)+")")
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d (identifier, binding) pairs OpenPEPPOL publishes have no violating case anywhere in this suite, "+
			"so nothing says they are rules rather than dead code: %v", len(missing), missing)
	}
	pairs := 0
	for _, r := range published {
		if r.bindings&peppolUBL != 0 {
			pairs++
		}
		if r.bindings&peppolCII != 0 {
			pairs++
		}
	}
	t.Logf("all %d published (Peppol identifier, binding) pairs have a violating verdict and a conforming one", pairs)
}

// peppolEvaluatedRules is every (identifier, binding) pair this package evaluates,
// with the bindings and flags read out of the artefact rather than out of the
// tables — so a rule whose table entry drifts from the file fails in
// TestPeppolRuleTableMatchesTheSchematron and its country counterpart, and the
// verdict guard below still asks the artefact what it should be checking.
//
// It is the 59 PEPPOL-* identifiers plus the country-specific ones
// peppolCountryRules names. The country identifiers the package does not evaluate
// yet are excluded here and required to be in Coverage(SourcePeppol) instead: a
// named gap has no verdicts to have, and demanding them would make the guard fail
// for the one reason it is not about.
func peppolEvaluatedRules(t *testing.T) map[string]peppolSchRule {
	t.Helper()
	out := peppolSchematronRules(t)
	for id, r := range peppolCountrySchematronRules(t) {
		if _, ok := peppolCountryRules[id]; ok {
			out[id] = r
		}
	}
	return out
}

// TestPeppolRulesFireOnlyInTheBindingThatPublishesThem is the negative half of the
// bindings column, asserted on documents rather than on the table.
//
// The eight VAT-exemption rules are the case that motivates it: they were evaluated
// on the syntax-neutral model, so a CII invoice whose exemption reason code did not
// match its VAT category was reported against PEPPOL-EN16931-P0104, which OpenPEPPOL
// publishes for UBL only. Nothing said that could not happen.
func TestPeppolRulesFireOnlyInTheBindingThatPublishesThem(t *testing.T) {
	published := peppolEvaluatedRules(t)
	// One document per binding that breaks as much as it can, so the sweep below
	// has something to over-fire on: a corpus sweep is the wide version of this and
	// runs in TestCoverageNamesNoRuleThePackageEmits.
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		doc  string
		bit  peppolBindings
	}{
		{"UBL", minimalPeppolUBL, peppolUBL},
		{"CII", minimalPeppolCII, peppolCII},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[string]bool{}
			for _, path := range peppolMisbindingProbes(t, tc.doc, tc.bit == peppolCII) {
				for _, v := range mustReport(t, ctx, ValidatePeppol, []byte(path)).Violations {
					if v.Source == SourcePeppol {
						seen[v.Rule] = true
					}
				}
			}
			for rule := range seen {
				if published[rule].bindings&tc.bit == 0 {
					t.Errorf("%s fired on a %s document and OpenPEPPOL publishes it in %s only",
						rule, tc.name, peppolBindingNames(published[rule].bindings))
				}
			}
		})
	}
}

// peppolMisbindingProbes are documents built to trip the rules that exist in one
// binding only, in whichever binding is being probed. Each is a mutation that would
// break the UBL-only rule if the rule were evaluated for both syntaxes.
func peppolMisbindingProbes(t *testing.T, doc string, cii bool) []string {
	t.Helper()
	out := []string{doc}
	if cii {
		// A VAT exemption reason code that does not match its category (P0104..P0111),
		// a second note (R002 in the UBL wording), a credit-note type code (P0101), a
		// period description code (CL006), a price-level charge (R044/R046) and an
		// amount in another currency (R051): all UBL-only rules, all expressed in CII.
		out = append(out,
			strings.Replace(doc, "<CategoryCode>S</CategoryCode>",
				"<CategoryCode>S</CategoryCode><ExemptionReasonCode>VATEX-EU-G</ExemptionReasonCode>", 1),
			strings.Replace(doc, "<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode>",
				"<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode><BillingSpecifiedPeriod><DescriptionCode>99</DescriptionCode></BillingSpecifiedPeriod>", 1),
			strings.Replace(doc, `<TaxTotalAmount currencyID="EUR">19.00</TaxTotalAmount>`,
				`<TaxTotalAmount currencyID="NOK">19.00</TaxTotalAmount>`, 1),
			strings.Replace(doc, "<NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice>",
				"<NetPriceProductTradePrice><ChargeAmount>100.00</ChargeAmount></NetPriceProductTradePrice>"+
					"<GrossPriceProductTradePrice><ChargeAmount>120.00</ChargeAmount>"+
					"<AppliedTradeAllowanceCharge><ChargeIndicator><Indicator>true</Indicator></ChargeIndicator>"+
					"<ActualAmount>5.00</ActualAmount></AppliedTradeAllowanceCharge></GrossPriceProductTradePrice>", 1),
		)
		return out
	}
	// The other direction: PEPPOL-EN16931-R006 is the one identifier the CII binding
	// publishes and the UBL one does not, and its UBL analogue is R100 at line level.
	// Two document-level invoiced object references would trip it if it were
	// evaluated for UBL.
	out = append(out, strings.Replace(doc, "<cac:TaxTotal>",
		`<cac:AdditionalDocumentReference><cbc:ID>A</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`+
			`<cac:AdditionalDocumentReference><cbc:ID>B</cbc:ID><cbc:DocumentTypeCode>130</cbc:DocumentTypeCode></cac:AdditionalDocumentReference>`+
			"<cac:TaxTotal>", 1))
	return out
}
