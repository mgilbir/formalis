package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The Factur-X guards: this package's reading of FNFE-MPE's five profile
// Schematrons, held to the files themselves.
//
// Everything below reads the artefact with an XML decoder rather than a regular
// expression, for C31's reason, and everything below runs in both directions: an
// identifier FNFE publishes and this package neither evaluates nor discloses
// fails as loudly as one this package claims and FNFE does not.
//
// Two traps are specific to these files and both are pinned by tests here rather
// than left to a reader's care:
//
//   - an assertion carries no id attribute, so an identifier has to be read off
//     an "[ID]-" prefix on the message text (fxIdentifier);
//   - most assertions have no identifier at all, and they are not noise but the
//     profile data model. fxDataModelShapes classifies every one of them and
//     TestFacturXDataModelIsSixShapes asserts the six shapes are the whole of it.
//     The data model itself — the table, the evaluator and the firing verdict on
//     each of its 2,159 assertions — is facturx_datamodel_test.go.

// fxSchematronDir is where `make facturx-schematron` puts the five profile
// Schematrons, or "" when they are not present.
func fxSchematronDir() string {
	dir := filepath.Join("testdata", "facturx", "schematron")
	if _, err := os.Stat(filepath.Join(dir, "FACTUR-X_EXTENDED.sch")); err != nil {
		return ""
	}
	return dir
}

// fxSchema, fxPattern, fxRule and fxAssert are as much of ISO Schematron as
// these files use. The assertion keeps its element name because assert and
// report are opposite polarities and the classification below depends on which
// it is.
type fxSchema struct {
	Patterns []fxPattern `xml:"pattern"`
}

type fxPattern struct {
	Rules []fxRule `xml:"rule"`
}

type fxRule struct {
	Context string     `xml:"context,attr"`
	Body    []fxAssert `xml:",any"`
}

type fxAssert struct {
	XMLName xml.Name
	Test    string `xml:"test,attr"`
	Flag    string `xml:"flag,attr"`
	Message string `xml:",chardata"`
}

// isAssertion tells an assert or report from the <let> bindings that share the
// rule element.
func (a fxAssert) isAssertion() bool {
	return a.XMLName.Local == "assert" || a.XMLName.Local == "report"
}

// fxIDPrefix is how FNFE names a rule: a bracketed identifier at the head of the
// message text, followed by a hyphen or a space. There is no id attribute
// anywhere in these files.
var fxIDPrefix = regexp.MustCompile(`^\s*\[([A-Za-z0-9_.-]+)\]`)

// identifier is the [ID] prefix on the message, or "" for the unnamed
// assertions of the profile data model.
func (a fxAssert) identifier() string {
	if m := fxIDPrefix.FindStringSubmatch(a.Message); m != nil {
		return m[1]
	}
	return ""
}

// severity is the flag FNFE published, folded onto this package's two values.
// The artefact uses flag="warning" on 21 assertions and leaves the rest unset,
// so within it the absence of the attribute is evidence rather than a default —
// which is the argument facturx.go makes for reporting an unflagged BR-FXEXT-*
// rule as fatal.
func (a fxAssert) severity() Severity {
	if a.Flag == "warning" || a.Flag == "information" {
		return SeverityWarning
	}
	return SeverityFatal
}

// fxProfileFiles maps each Profile onto its Schematron's file name.
var fxProfileFiles = map[Profile]string{
	ProfileMinimum:  "FACTUR-X_MINIMUM.sch",
	ProfileBasicWL:  "FACTUR-X_BASIC-WL.sch",
	ProfileBasic:    "FACTUR-X_BASIC.sch",
	ProfileEN16931:  "FACTUR-X_EN16931.sch",
	ProfileExtended: "FACTUR-X_EXTENDED.sch",
}

// fxDecode reads one profile Schematron.
func fxDecode(t *testing.T, dir string, p Profile) fxSchema {
	t.Helper()
	path := filepath.Join(dir, fxProfileFiles[p])
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	var s fxSchema
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = latin1Reader
	if err := dec.Decode(&s); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	// The ratchet, for the reason corpus_test.go gives about every other oracle
	// here: a truncated or empty .sch decodes to nothing and would leave every
	// both-directions guard below vacuously green.
	n := 0
	for _, pat := range s.Patterns {
		n += len(pat.Rules)
	}
	if n < minFacturXRules[p] {
		t.Fatalf("%s decoded to %d rules, want at least %d; the file is short or the decoder is reading the wrong elements",
			path, n, minFacturXRules[p])
	}
	return s
}

// fxAssertions is every assert and report in one profile, in document order,
// with the rule and pattern it belongs to.
type fxAssertion struct {
	pattern, rule int
	context       string
	a             fxAssert
}

func fxAssertions(s fxSchema) []fxAssertion {
	var out []fxAssertion
	for pi, p := range s.Patterns {
		for ri, r := range p.Rules {
			for _, a := range r.Body {
				if a.isAssertion() {
					out = append(out, fxAssertion{pattern: pi, rule: ri, context: r.Context, a: a})
				}
			}
		}
	}
	return out
}

// fxNamed is every identifier one profile publishes, mapped to its assertion.
func fxNamed(s fxSchema) map[string]fxAssertion {
	out := map[string]fxAssertion{}
	for _, x := range fxAssertions(s) {
		if id := x.a.identifier(); id != "" {
			out[id] = x
		}
	}
	return out
}

// TestFacturXPublishedInventory is the first fact everything else rests on, and
// it is here because the numbers in facturx.go, in Coverage(SourceFacturX) and in
// issue #56 are all claims about it.
func TestFacturXPublishedInventory(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	for _, p := range profiles {
		s := fxDecode(t, dir, p)
		all := fxAssertions(s)
		named := 0
		for _, x := range all {
			if x.a.identifier() != "" {
				named++
			}
		}
		t.Logf("Factur-X %s: %d assertions, %d named identifiers, %d unnamed", string(p), len(all), named, len(all)-named)
		if named == 0 {
			t.Errorf("%s published no named identifier; the [ID] prefix convention is not being read", string(p))
		}
	}
}

// TestFacturXBindingMatchesTheArtefact holds facturXBinding to the files in both
// directions: every CEN-minted CII binding identifier a profile Schematron
// carries is in the table with that profile, and every (identifier, profile) pair
// the table claims is in the file.
//
// The three MINIMUM entries are the reason this cannot be written as "read the
// [ID] prefixes and compare". MINIMUM carries CII-SR-463, CII-SR-465 and
// CII-SR-466 with an empty message and therefore with no identifier at all, so
// they are matched by their XPath against the profile that does name them.
func TestFacturXBindingMatchesTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	// The XPath of each binding identifier, taken from a profile that names it.
	tests := map[string]string{}
	for _, x := range fxAssertions(fxDecode(t, dir, ProfileExtended)) {
		if id := x.a.identifier(); strings.HasPrefix(id, "CII-") {
			tests[id] = normalizeSpace(x.a.Test)
		}
	}
	if len(tests) < 5 {
		t.Fatalf("EXTENDED named only %d CII-* identifiers, want at least 5", len(tests))
	}

	table := map[string]facturXBindingRule{}
	for _, b := range facturXBinding {
		table[b.id] = b
	}
	for _, p := range profiles {
		s := fxDecode(t, dir, p)
		published := map[string]bool{}
		for _, x := range fxAssertions(s) {
			id := x.a.identifier()
			if id == "" {
				// An unnamed assertion whose XPath is one of the binding rules'
				// is that binding rule under another guise. Only MINIMUM has any.
				for cand, want := range tests {
					if normalizeSpace(x.a.Test) == want {
						id = cand
						if !hasProfile(table[cand].anonymousIn, p) {
							t.Errorf("%s carries %s with no [ID] prefix and facturXBinding does not say so", string(p), cand)
						}
					}
				}
			}
			if strings.HasPrefix(id, "CII-") {
				published[id] = true
			}
		}
		for id := range published {
			b, ok := table[id]
			if !ok {
				t.Errorf("%s publishes the CEN binding identifier %s and facturXBinding does not name it", string(p), id)
				continue
			}
			if !b.carries(p) {
				t.Errorf("%s publishes %s and facturXBinding does not list that profile", string(p), id)
			}
		}
		for _, b := range facturXBinding {
			if b.carries(p) && !published[b.id] {
				t.Errorf("facturXBinding says %s carries %s and the artefact does not", string(p), b.id)
			}
			if !b.carries(p) && published[b.id] {
				t.Errorf("facturXBinding says %s does not carry %s and the artefact does", string(p), b.id)
			}
		}
	}
}

func hasProfile(ps []Profile, p Profile) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}

// TestFacturXQuotesCENsConditionVerbatim is the licence for reporting these four
// under SourceEN16931 and at CEN's published flag rather than at Factur-X's
// absent one. If FNFE ever edits one of these conditions the way it edited
// CII-SR-464's, this fails and the decision has to be made again.
func TestFacturXQuotesCENsConditionVerbatim(t *testing.T) {
	dir := fxSchematronDir()
	cen := en16931SuiteDir()
	if dir == "" || cen == "" {
		t.Skip("Factur-X or CEN artefacts not present; run `make facturx-schematron en16931-artefacts`")
	}
	params := fxCENParams(t, filepath.Join(cen, "cii", "schematron", "CII", "EN16931-CII-syntax.sch"))
	named := fxNamed(fxDecode(t, dir, ProfileExtended))
	checked := 0
	for _, b := range facturXBinding {
		if !b.evaluated {
			continue
		}
		x, ok := named[b.id]
		if !ok {
			t.Errorf("EXTENDED does not name %s, so its condition cannot be compared", b.id)
			continue
		}
		want, ok := params[b.id]
		if !ok {
			t.Errorf("CEN's EN16931-CII-syntax.sch binds no param named %s", b.id)
			continue
		}
		if normalizeSpace(x.a.Test) != normalizeSpace(want) {
			t.Errorf("%s: Factur-X binds %q and CEN binds %q; this package reports it under CEN's Source and at CEN's "+
				"flag on the strength of them being the same rule", b.id, normalizeSpace(x.a.Test), normalizeSpace(want))
		}
		checked++
	}
	if checked != 4 {
		t.Errorf("compared %d conditions, want 4", checked)
	}
}

// fxCENParams reads CEN's <param name= value=> bindings, which is where the CII
// binding's conditions live before the abstract pattern is resolved.
//
// The decoder gets a CharsetReader because CEN declares this file ISO-8859-1 and
// Go's decoder without one returns no attributes at all for it rather than an
// error — the trap recorded in the C40 correction, which made a survey of the
// whole CII binding come back empty.
func fxCENParams(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = latin1Reader
	out := map[string]string{}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "param" {
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
		if name != "" {
			out[name] = value
		}
	}
	if len(out) < 500 {
		t.Fatalf("%s yielded %d params; the decoder is not reading the file (CEN declares it ISO-8859-1)", path, len(out))
	}
	return out
}

// TestFacturXInertBindingIsStillInert re-derives the tautology behind the one
// Unevaluable entry under SourceFacturX.
//
// It checks the shape mechanically rather than matching the string: the test is a
// disjunction whose second half is the negation of the first, which is true for
// every document. That is the same class of fact as CEN binding BR-CO-05..08 to
// true(), and the entry stops being legitimate the moment FNFE writes something
// that can fail.
func TestFacturXInertBindingIsStillInert(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	checked := 0
	for _, p := range profiles {
		named := fxNamed(fxDecode(t, dir, p))
		x, ok := named[facturXInertBinding]
		if !ok {
			continue
		}
		got := normalizeSpace(x.a.Test)
		const want = "(ram:PayeeSpecifiedCreditorFinancialInstitution or ram:PayerSpecifiedDebtorFinancialInstitution) or " +
			"(not(ram:PayeeSpecifiedCreditorFinancialInstitution) and not(ram:PayerSpecifiedDebtorFinancialInstitution))"
		if got != want {
			t.Errorf("%s binds %s to %q; the Unevaluable entry in Coverage(SourceFacturX) rests on it being the "+
				"tautology %q, so either FNFE fixed it — in which case implement it — or it is a different rule now",
				string(p), facturXInertBinding, got, want)
		}
		checked++
	}
	if checked != 4 {
		t.Errorf("found %s in %d profiles, want 4", facturXInertBinding, checked)
	}
}

// TestFacturXExtensionRulesMatchTheArtefact accounts for every BR-FXEXT-*
// identifier FNFE publishes, in both directions and in both halves of the split
// facturx.go describes: implemented, or a restatement of a CEN identifier the
// profile drops and named in facturXCENOmissions.
func TestFacturXExtensionRulesMatchTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	published := map[string]Severity{}
	for _, p := range profiles {
		for id, x := range fxNamed(fxDecode(t, dir, p)) {
			if strings.HasPrefix(id, "BR-FXEXT") {
				published[id] = x.a.severity()
			}
		}
	}
	if len(published) < minFacturXExtensionIDs {
		t.Fatalf("read %d BR-FXEXT-* identifiers, want at least %d", len(published), minFacturXExtensionIDs)
	}

	implemented := map[string]bool{}
	for _, id := range facturXExtensionRules {
		implemented[id] = true
	}
	for _, rs := range facturXRestatementRules {
		implemented[rs.id] = true
	}
	restatement := map[string]string{}
	for _, o := range facturXCENOmissions {
		if o.replacedBy != "" {
			restatement[o.replacedBy] = o.cen
		}
	}
	// The -08b/-08ini/-08rev and S08b/S-09b variants all restate the same CEN
	// identifier, and facturXCENOmissions names one of each family. Group by the
	// family prefix so the accounting is per identifier without the table having
	// to list three rows per VAT category.
	family := func(id string) string {
		for _, suffix := range []string{"-08b", "-08ini", "-08rev", "-09b", "08b", "-08"} {
			if strings.HasSuffix(id, suffix) {
				return strings.TrimSuffix(id, suffix)
			}
		}
		return id
	}
	families := map[string]bool{}
	for r := range restatement {
		families[family(r)] = true
	}

	var unaccounted, fatalGap, advisoryGap []string
	for id, sev := range published {
		switch {
		case implemented[id]:
		case restatement[id] != "" || families[family(id)]:
			if sev == SeverityFatal {
				fatalGap = append(fatalGap, id)
			} else {
				advisoryGap = append(advisoryGap, id)
			}
		default:
			unaccounted = append(unaccounted, id)
		}
	}
	sort.Strings(unaccounted)
	for _, id := range unaccounted {
		t.Errorf("FNFE publishes %s and this package neither evaluates it nor names it in facturXCENOmissions as a "+
			"restatement of a CEN identifier; a rule accounted for nowhere is a rule nobody decided about", id)
	}
	for _, id := range facturXExtensionRules {
		if _, ok := published[id]; !ok {
			t.Errorf("facturXExtensionRules names %s and no Factur-X Schematron publishes it", id)
		}
	}
	// The coverage entry quotes these counts, so they are pinned here rather than
	// only in prose. The fatal half is zero now: the 24 fatal restatements are
	// evaluated, and what is left under this Source is the 18 FNFE flags warning.
	if len(fatalGap) != 0 || len(advisoryGap) != 18 {
		sort.Strings(fatalGap)
		sort.Strings(advisoryGap)
		t.Errorf("the unimplemented BR-FXEXT-* split is %d fatal / %d advisory, and Coverage(SourceFacturX) says 0 / 18\n"+
			"fatal: %v\nadvisory: %v", len(fatalGap), len(advisoryGap), fatalGap, advisoryGap)
	}
	t.Logf("Factur-X extension rules: %d published, %d evaluated (%d own ground, %d restatements), %d fatal gap, %d advisory restatements",
		len(published), len(facturXExtensionRules)+len(facturXRestatementRules), len(facturXExtensionRules),
		len(facturXRestatementRules), len(fatalGap), len(advisoryGap))
}

// TestFacturXOmissionsMatchTheArtefact re-derives facturXCENOmissions: the CEN
// identifiers the EXTENDED profile's Schematron does not carry although the
// EN 16931 profile's does.
//
// It is the per-rule half of the decision facturx.go records — keep CEN's
// stricter original — and it is derived rather than asserted because a table of
// 41 identifiers written by hand is a table that goes stale silently.
func TestFacturXOmissionsMatchTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	en := fxNamed(fxDecode(t, dir, ProfileEN16931))
	ext := fxNamed(fxDecode(t, dir, ProfileExtended))

	var dropped []string
	for id := range en {
		if _, ok := ext[id]; !ok {
			dropped = append(dropped, id)
		}
	}
	sort.Strings(dropped)

	table := map[string]string{}
	for _, o := range facturXCENOmissions {
		table[o.cen] = o.replacedBy
	}
	for _, id := range dropped {
		if _, ok := table[id]; !ok {
			t.Errorf("the EXTENDED profile drops %s and facturXCENOmissions does not name it; this package still "+
				"evaluates CEN's rule, and that has to be a recorded decision rather than an oversight", id)
		}
		delete(table, id)
	}
	var extra []string
	for id := range table {
		extra = append(extra, id)
	}
	sort.Strings(extra)
	for _, id := range extra {
		t.Errorf("facturXCENOmissions says the EXTENDED profile drops %s and it does not", id)
	}
	// And the replacements it names have to be identifiers EXTENDED publishes.
	replaced := 0
	for _, o := range facturXCENOmissions {
		if o.replacedBy == "" {
			continue
		}
		if _, ok := ext[o.replacedBy]; !ok {
			t.Errorf("facturXCENOmissions says %s replaces %s and EXTENDED publishes no such identifier", o.replacedBy, o.cen)
		}
		replaced++
	}
	t.Logf("Factur-X EXTENDED drops %d CEN identifiers, %d with a BR-FXEXT-* replacement and %d with none",
		len(dropped), replaced, len(dropped)-replaced)
	if len(dropped) != 41 {
		t.Errorf("EXTENDED drops %d CEN identifiers and Coverage(SourceFacturX) says 41", len(dropped))
	}
}

// fxDataModelShapes is the classification of the unnamed half of these files:
// the six mechanical shapes facturx.go lists, keyed by a name the coverage entry
// uses.
//
// It exists so that "the unnamed assertions are the profile data model" is a
// checked statement and not a reading. Anything that falls outside these shapes
// is reported, which is what would happen if FNFE started writing real rules
// without identifiers.
func fxDataModelShapes(x fxAssertion) string {
	test := normalizeSpace(x.a.Test)
	switch {
	case x.a.XMLName.Local == "report" && test == "true()":
		return "element-not-used"
	case x.a.XMLName.Local == "report" && strings.HasPrefix(test, "@"):
		return "attribute-not-used"
	case x.a.XMLName.Local == "assert" && strings.HasPrefix(test, "@"):
		return "attribute-required"
	case strings.HasPrefix(test, "count(") && (strings.Contains(test, ")=") || strings.Contains(test, ")<=") || strings.Contains(test, ")>=")):
		return "cardinality"
	case strings.Contains(test, "codedb") && strings.Contains(test, "enumeration"):
		return "code-list"
	}
	return ""
}

// TestFacturXDataModelIsSixShapes classifies every unnamed assertion in every
// profile and asserts that the six mechanical shapes account for all of them, and
// that each profile carries the number this package is written for.
//
// It was the guard on the coverage entry that named this layer as a gap. The
// layer is implemented now — facturx_datamodel.go, and the whole of
// facturx_datamodel_test.go beside it — so what it guards is different and
// narrower: the shapes are a *closed* set, and an assertion outside them is FNFE
// writing a real rule without an identifier, which internal/gen/facturx would
// refuse to emit and which this says out loud rather than leaving to the
// generator's exit status. The counts are held to minFacturXDataModel by
// TestFacturXDataModelIsRatcheted and to the artefact, row by row, by
// TestFacturXDataModelMatchesTheArtefact.
func TestFacturXDataModelIsSixShapes(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	// The counts this package is written for, stated here rather than read from
	// minFacturXDataModel so that the two are checked against each other.
	want := map[Profile]int{
		ProfileMinimum:  48,
		ProfileBasicWL:  196,
		ProfileBasic:    262,
		ProfileEN16931:  412,
		ProfileExtended: 1241,
	}
	// The three MINIMUM assertions that are CII-SR-463/465/466 without their
	// message, which are binding rules rather than data model and are subtracted
	// from the count for that reason.
	bindingTests := map[string]bool{}
	for id, x := range fxNamed(fxDecode(t, dir, ProfileBasicWL)) {
		if strings.HasPrefix(id, "CII-") {
			bindingTests[normalizeSpace(x.a.Test)] = true
		}
	}

	for _, p := range profiles {
		byShape := map[string]int{}
		other, model := 0, 0
		for _, x := range fxAssertions(fxDecode(t, dir, p)) {
			if x.a.identifier() != "" || bindingTests[normalizeSpace(x.a.Test)] {
				continue
			}
			model++
			shape := fxDataModelShapes(x)
			if shape == "" {
				other++
				if other <= 3 {
					t.Errorf("%s: the unnamed assertion %s test=%q at %s matches none of the six data-model shapes; "+
						"Coverage(SourceFacturX) describes this layer as those six", string(p), x.a.XMLName.Local,
						normalizeSpace(x.a.Test), x.context)
				}
				continue
			}
			byShape[shape]++
		}
		t.Logf("Factur-X %s data model: %d assertions %v", string(p), model, byShape)
		if model != want[p] {
			t.Errorf("%s carries %d data-model assertions and this package is written for %d", string(p), model, want[p])
		}
		if model != minFacturXDataModel[p] {
			t.Errorf("%s carries %d data-model assertions and the committed table's floor is %d", string(p), model, minFacturXDataModel[p])
		}
	}
}

// fxCoverageEntry finds the Coverage(SourceFacturX) entry whose Rules contains
// the given text, so a test can assert about one entry without indexing a slice
// whose order is prose.
func fxCoverageEntry(t *testing.T, contains string) RuleFamily {
	t.Helper()
	for _, f := range Coverage(SourceFacturX) {
		if strings.Contains(f.Rules, contains) {
			return f
		}
	}
	t.Fatalf("Coverage(SourceFacturX) has no entry whose Rules contains %q", contains)
	return RuleFamily{}
}

// TestFacturXRulesAreReachableInTheirPattern models the ISO Schematron rule
// order for the assertions this package evaluates. C42 is the third time an
// unreachable rule was reported in this repository and it is checked here before
// the fourth.
//
// Under ISO Schematron a node goes to the first matching rule of a pattern. Every
// context in these files is an absolute path from /rsm:CrossIndustryInvoice or a
// `//` step, and the only contexts that can compete are those with the same path
// once predicates are stripped. So for each rule carrying an assertion this
// package evaluates, no earlier rule in the same pattern may share its stripped
// path.
func TestFacturXRulesAreReachableInTheirPattern(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	evaluated := map[string]bool{}
	for _, b := range facturXBinding {
		if b.evaluated {
			evaluated[b.id] = true
		}
	}
	for _, id := range facturXExtensionRules {
		evaluated[id] = true
	}
	for _, rs := range facturXRestatementRules {
		evaluated[rs.id] = true
	}
	checked := 0
	for _, p := range profiles {
		s := fxDecode(t, dir, p)
		for pi, pat := range s.Patterns {
			for ri, r := range pat.Rules {
				var mine []string
				for _, a := range r.Body {
					if a.isAssertion() && evaluated[a.identifier()] {
						mine = append(mine, a.identifier())
					}
				}
				if len(mine) == 0 {
					continue
				}
				checked++
				for j := 0; j < ri; j++ {
					if fxStripPredicates(pat.Rules[j].Context) == fxStripPredicates(r.Context) {
						t.Errorf("%s pattern %d: the rule at %q carries %v, and the earlier rule at %q claims the same "+
							"nodes; under ISO Schematron no processor reaches it", string(p), pi, r.Context, mine,
							pat.Rules[j].Context)
					}
				}
			}
		}
	}
	if checked < 10 {
		t.Errorf("checked the ordering of only %d rules; the identifiers are not being matched", checked)
	}
}

// fxStripPredicates removes every [...] from an XPath, leaving the path steps.
// Two contexts can select the same node only if these agree.
func fxStripPredicates(ctx string) string {
	var b strings.Builder
	depth := 0
	for _, r := range ctx {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return normalizeSpace(b.String())
}

// ---------------------------------------------------------------------------
// The corpus: FNFE's own published examples, and its own verdict on each
// ---------------------------------------------------------------------------

// fxExamples are the bare CII invoice XMLs FNFE-MPE ships with the Factur-X 1.09
// / ZUGFeRD 2.5 specification bundle, or nil when they are not vendored.
//
// They are not fetchable individually — GitHub carries the EN 16931 subset only,
// 3 EXTENDED documents against the bundle's 25 — so `make facturx-examples`
// reports on them rather than downloading them, and these tests skip on a clean
// checkout the way every other corpus-backed test here does. What they must not
// do is pass quietly on a subset, which is what minFacturXExamples is for.
func fxExamples(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join("testdata", "facturx", "examples", "*.xml"))
	sort.Strings(files)
	return files
}

// fxDeclaredProfile is the tier a document's own BT-24 claims, which is how each
// example is validated: at the profile it says it is.
//
// A document declaring an XRechnung identifier is not a Factur-X profile document
// at all — four of the examples are XRechnung CII — and is reported as such, so
// the sweep can hold those to XRechnung's rule set instead of guessing a tier.
func fxDeclaredProfile(data []byte) (Profile, Source, bool) {
	det, err := Detect(data)
	if err != nil {
		return "", SourceNone, false
	}
	if det.Source != SourceFacturX {
		return "", det.Source, false
	}
	p, _ := facturXProfileFromSpecID(det.SpecID)
	return p, SourceFacturX, true
}

// TestValidateFacturXCorpus is the oracle issue #56 asked for: FNFE's own
// published examples, each validated at the profile its own BT-24 declares, with
// no fatal finding permitted.
//
// Before Profile scoped the binding this reported 76 fatal findings on 13 of the
// 25 EXTENDED examples — CII-DT-027 seventy times, CII-DT-021 three, CII-DT-018
// twice and CII-SR-439 once — every one of them a rule from CEN's CII binding
// that Factur-X does not adopt. The assertion is FP=0 and not "fewer than
// before", because these are the documents the authority publishes as correct.
func TestValidateFacturXCorpus(t *testing.T) {
	files := fxExamples(t)
	if len(files) == 0 {
		t.Skip("no Factur-X examples found; see `make facturx-examples`")
	}
	atLeast(t, "Factur-X example invoices", len(files), minFacturXExamples)

	ctx := context.Background()
	byProfile := map[Profile]int{}
	byRule := map[string]int{}
	clean, judged, conformant := 0, 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		p, src, ok := fxDeclaredProfile(data)
		if !ok {
			// The twenty-seven examples that do not declare a Factur-X profile,
			// validated by the rule set they do declare, which is what ValidateCIUS
			// does. Twenty-three declare CEN's own "urn:cen.eu:en16931:2017" — the
			// identifier the EN 16931 tier writes — and are held to FP=0 like every
			// other conforming document in this suite.
			//
			// The other four declare XRechnung, and they are logged rather than
			// asserted on. They are FNFE's illustrations of a German invoice in the
			// CII syntax, the specification bundle ships no validation report for
			// any of them, and they are not documents about Factur-X's rule set at
			// all. One of them really is non-conformant to XRechnung 3.0 and this
			// package says so correctly: XRECHNUNG_Betriebskostenabrechnung.xml
			// carries no ram:BusinessProcessSpecifiedDocumentContextParameter
			// (BT-23) and no ram:URIUniversalCommunication on either party (BT-34,
			// BT-49) — its two ram:EmailURIUniversalCommunication are the contact
			// email BT-43/BT-58, a different term — so PEPPOL-EN16931-R001, R010
			// and R020 are true positives. Asserting FP=0 over them would mean
			// either suppressing three correct findings or pretending the sample is
			// conformant.
			r, err := ValidateCIUS(ctx, data)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			for _, v := range r.Fatal() {
				if src == SourceEN16931 {
					t.Errorf("%s (declares %s): %s", filepath.Base(f), src, v.Error())
					continue
				}
				t.Logf("%s (declares %s, no FNFE validation report): %s", filepath.Base(f), src, v.Error())
			}
			continue
		}
		judged++
		byProfile[p]++
		r, err := Validate(ctx, data, p)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		// Conformant is the whole answer and not a second opinion: a document with
		// no fatal finding is only conformant if this package also evaluated every
		// fatal rule that applies. It was false for every one of these documents
		// from the moment Profile began selecting Factur-X's binding until the
		// last fatal gap under SourceFacturX closed, which is why it is asserted
		// here rather than left to Coverage.
		if r.Conformant() {
			conformant++
		}
		fatal := r.Fatal()
		if len(fatal) == 0 {
			clean++
			continue
		}
		for _, v := range fatal {
			byRule[string(v.Source)+"/"+v.Rule]++
			t.Errorf("%s (%s): %s", filepath.Base(f), string(p), v.Error())
		}
	}
	if conformant != judged {
		var gaps []string
		for _, f := range Coverage(SourceFacturX) {
			if f.Severity == SeverityFatal && !f.Unevaluable {
				gaps = append(gaps, f.Rules)
			}
		}
		t.Errorf("%d of %d profile-declaring examples report Conformant(); every one of them should, and the fatal "+
			"evaluable gaps left under SourceFacturX are %v", conformant, judged, gaps)
	}
	if judged < minFacturXProfiled {
		t.Errorf("only %d of %d examples declare a Factur-X profile in BT-24, want at least %d; the routing is not "+
			"recognising them and this sweep is measuring something else", judged, len(files), minFacturXProfiled)
	}
	t.Logf("Factur-X corpus: %d/%d profile-declaring examples clean (FP=0) over %d files, %d/%d Conformant(), by tier %v",
		clean, judged, len(files), conformant, judged, fxProfileCounts(byProfile))
}

func fxProfileCounts(m map[Profile]int) string {
	var parts []string
	for _, p := range profiles {
		if m[p] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", string(p), m[p]))
		}
	}
	return strings.Join(parts, " ")
}

// fxReport is as much of valitool's validation report as this oracle reads.
type fxReport struct {
	IsValid              string `xml:"isValid"`
	ValidationDetailsXML struct {
		IsValidXML           string `xml:"isValidXML"`
		IsValidSchema        string `xml:"isValidSchema"`
		IsValidCodelists     string `xml:"isValidCodelists"`
		IsValidBusinessRules string `xml:"isValidBusinessRules"`
		Details              []struct {
			ErrorType        string `xml:"errorType"`
			ErrorCode        string `xml:"errorCode"`
			ErrorDescription string `xml:"errorDescription"`
		} `xml:"details"`
	} `xml:"validationDetailsXML"`
	Embedding struct {
		Variant string `xml:"variantFromXML"`
	} `xml:"validationDetailsEmbedding"`
}

// TestFacturXValidationReportsAgree wires FNFE's own verdicts.
//
// The specification bundle ships a *_fx_validation_report.xml beside 51 of the
// examples: the output of valitool, the validator FNFE-MPE publishes the samples
// with. It is the same class of oracle as KoSIT's <?xmute?> fixtures and
// OpenPEPPOL's per-rule unit tests, both of which caught real defects here, and it
// answers a question the examples alone cannot: not "does this package report
// anything" but "does the authority".
//
// What the reports assert, read out of all 51: isValidBusinessRules is true for
// every one, and so are isValidSchema, isValidCodelists, isValidNamespaces and
// isValidEncoding. Three carry isValid=false and all three fail on
// VD-Valitool-176, the German Leitweg-ID checksum, which is valitool's own rule
// and appears in no Factur-X Schematron. The only rule identifier from any of the
// three authorities this package quotes that appears anywhere in the 51 is
// CII-SR-450, twice, at valitool's errorType 1 — a warning — which is CEN's
// advisory CII binding and which this package also reports as a warning, under
// ValidateEN16931.
//
// So the assertion is the strongest one the reports support: for every document
// the authority's own validator passes on business rules, this package reports no
// fatal finding.
func TestFacturXValidationReportsAgree(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "facturx", "reports", "*_fx_validation_report.xml"))
	sort.Strings(files)
	if len(files) == 0 {
		t.Skip("no Factur-X validation reports found; see `make facturx-examples`")
	}
	atLeast(t, "Factur-X validation reports", len(files), minFacturXReports)

	ctx := context.Background()
	matched, passed := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var rep fxReport
		if err := xml.Unmarshal(data, &rep); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if rep.ValidationDetailsXML.IsValidBusinessRules == "" {
			t.Fatalf("%s: no isValidBusinessRules; the report schema is not being read", f)
		}
		base := strings.TrimSuffix(filepath.Base(f), "_fx_validation_report.xml")
		doc := filepath.Join("testdata", "facturx", "examples", base+".xml")
		invoice, err := os.ReadFile(doc)
		if err != nil {
			t.Errorf("%s has a validation report and no example XML beside it; the corpus is half vendored", base)
			continue
		}
		matched++
		if rep.ValidationDetailsXML.IsValidBusinessRules != "true" {
			// None does today. If one ever does, it is a document the authority
			// rejects, and this package reporting nothing would be the defect —
			// so it is asserted the other way round rather than skipped.
			if len(reportFatal(t, ctx, invoice)) == 0 {
				t.Errorf("%s: valitool reports isValidBusinessRules=%q and this package reports no fatal finding",
					base, rep.ValidationDetailsXML.IsValidBusinessRules)
			}
			continue
		}
		passed++
		for _, v := range reportFatal(t, ctx, invoice) {
			t.Errorf("%s: valitool passes it on business rules (variant %s) and this package reports %s",
				base, rep.Embedding.Variant, v.Error())
		}
	}
	if matched != len(files) {
		t.Errorf("matched %d of %d reports to an example; every report needs its document", matched, len(files))
	}
	t.Logf("Factur-X reports: %d/%d documents FNFE's own validator passes on business rules and this package "+
		"reports no fatal finding on", passed, matched)
}

// reportFatal validates one example the way TestValidateFacturXCorpus does: at
// the profile its BT-24 declares, or through ValidateCIUS when it declares
// something else.
func reportFatal(t *testing.T, ctx context.Context, data []byte) []Violation {
	t.Helper()
	var r Report
	var err error
	if p, _, ok := fxDeclaredProfile(data); ok {
		r, err = Validate(ctx, data, p)
	} else {
		r, err = ValidateCIUS(ctx, data)
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	return r.Fatal()
}

// TestProfileSelectsTheBinding is the property issue #56 is about, asserted on
// one document rather than on a corpus: the same bytes, validated at two
// profiles, are judged by two different syntax bindings.
//
// It was false before this change — the issue records confirming that the same
// document yields identical CII-DT-* findings at every profile from MINIMUM to
// EXTENDED — and a regression would make it false again without moving any
// corpus number, because the corpus documents are conforming.
func TestProfileSelectsTheBinding(t *testing.T) {
	// A document reference carrying an issue date, which is what CEN's CII-DT-027
	// forbids anywhere but a preceding invoice reference and what the EXTENDED
	// profile's data model permits on an additional document reference.
	doc := strings.Replace(validCII,
		"<ExchangedDocument>",
		"<ExchangedDocument>", 1)
	doc = strings.Replace(doc, "<ApplicableHeaderTradeAgreement>",
		"<ApplicableHeaderTradeAgreement><AdditionalReferencedDocument><IssuerAssignedID>X</IssuerAssignedID>"+
			"<TypeCode>916</TypeCode><FormattedIssueDateTime><DateTimeString format=\"102\">20240101</DateTimeString>"+
			"</FormattedIssueDateTime></AdditionalReferencedDocument>", 1)
	if doc == validCII {
		t.Fatal("the fixture was not modified; validCII no longer holds the anchor this test edits")
	}
	ctx := context.Background()

	cen := ruleSet(fatalFindings(t, ctx, ValidateEN16931, []byte(doc)))
	if !cen["CII-DT-027"] {
		t.Fatalf("CEN's binding does not report CII-DT-027 on this document, so the test measures nothing: %v", cen)
	}
	for _, p := range profiles {
		got := ruleSet(fatalFindings(t, ctx, withProfile(p), []byte(doc)))
		if got["CII-DT-027"] {
			t.Errorf("profile %q still reports CII-DT-027, which no Factur-X Schematron publishes", string(p))
		}
	}
	// And the four the Schematrons do publish are not simply switched off with it.
	// CII-SR-463 is carried by every profile, so an allowance with no charge
	// indicator has to be reported at every one of them.
	bad := withAllowanceCharge(`<SpecifiedTradeAllowanceCharge><ActualAmount>0.00</ActualAmount>` +
		`<CategoryTradeTax><CategoryCode>S</CategoryCode><RateApplicablePercent>20.00</RateApplicablePercent></CategoryTradeTax>` +
		`<Reason>r</Reason></SpecifiedTradeAllowanceCharge>`)
	for _, p := range profiles {
		if !ruleSet(fatalFindings(t, ctx, withProfile(p), []byte(bad)))["CII-SR-463"] {
			t.Errorf("profile %q does not report CII-SR-463, which its own Schematron carries", string(p))
		}
	}
}

// TestEveryFacturXExtensionRuleFires is the firing verdict C41 asks for. A rule
// present in the table, reachable in the tree and inert passes every other guard
// here; PR 24 shipped exactly that defect, and the only thing that catches it is a
// document per rule that makes it report and one that does not.
func TestEveryFacturXExtensionRuleFires(t *testing.T) {
	ctx := context.Background()
	// Each case is a pair: the document that must trip the rule, and the
	// modification of it that must not.
	cases := []struct {
		rule      string
		bad, good string
	}{{
		rule: "BR-FXEXT-01",
		bad:  fxWith(`<ExchangedDocument><ID>INV-1</ID>`, `<ExchangedDocument><IncludedNote><SubjectCode>AAI</SubjectCode></IncludedNote><ID>INV-1</ID>`),
		good: fxWith(`<ExchangedDocument><ID>INV-1</ID>`, `<ExchangedDocument><IncludedNote><SubjectCode>AAI</SubjectCode><Content>text</Content></IncludedNote><ID>INV-1</ID>`),
	}, {
		rule: "BR-FXEXT-02",
		bad:  fxWith(`<AssociatedDocumentLineDocument><LineID>1</LineID>`, `<AssociatedDocumentLineDocument><LineID>1</LineID><IncludedNote><SubjectCode>AAI</SubjectCode></IncludedNote>`),
		good: fxWith(`<AssociatedDocumentLineDocument><LineID>1</LineID>`, `<AssociatedDocumentLineDocument><LineID>1</LineID><IncludedNote><SubjectCode>AAI</SubjectCode><ContentCode>C</ContentCode></IncludedNote>`),
	}, {
		rule: "BR-FXEXT-03",
		bad:  fxWith(`<BuyerTradeParty><Name>Buyer Co</Name>`, `<BuyerTradeParty><Name>Buyer Co</Name><SpecifiedTaxRegistration><ID schemeID="FC">123</ID></SpecifiedTaxRegistration>`),
		good: fxWith(`<BuyerTradeParty><Name>Buyer Co</Name>`, `<BuyerTradeParty><Name>Buyer Co</Name><SpecifiedTaxRegistration><ID schemeID="VA">FR9</ID></SpecifiedTaxRegistration>`),
	}, {
		rule: "BR-FXEXT-04",
		bad:  fxWith(`<SpecifiedTradeProduct><Name>Widget</Name>`, `<SpecifiedTradeProduct><Name>Widget</Name><ApplicableProductCharacteristic><TypeCode>NOT_A_CODE</TypeCode><Description>d</Description><Value>v</Value></ApplicableProductCharacteristic>`),
		good: fxWith(`<SpecifiedTradeProduct><Name>Widget</Name>`, `<SpecifiedTradeProduct><Name>Widget</Name><ApplicableProductCharacteristic><TypeCode>MATERIAL</TypeCode><Description>d</Description><Value>v</Value></ApplicableProductCharacteristic>`),
	}, {
		rule: "BR-FXEXT-06",
		bad:  fxWith(`<AssociatedDocumentLineDocument><LineID>1</LineID>`, `<AssociatedDocumentLineDocument><LineID>1</LineID><ParentLineID>1</ParentLineID>`),
		good: fxWith(`<AssociatedDocumentLineDocument><LineID>1</LineID>`, `<AssociatedDocumentLineDocument><LineID>1</LineID><ParentLineID>1</ParentLineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode>`),
	}, {
		rule: "BR-FXEXT-08",
		bad:  strings.Replace(subLineCII, "<LineTotalAmount>60.00</LineTotalAmount>", "<LineTotalAmount>59.00</LineTotalAmount>", 1),
		good: subLineCII,
	}, {
		rule: "BR-FXEXT-11",
		bad:  strings.Replace(subLineCII, "<ParentLineID>1</ParentLineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode></AssociatedDocumentLineDocument>\n      <SpecifiedTradeProduct><Name>Child A</Name>", "<ParentLineID>99</ParentLineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode></AssociatedDocumentLineDocument>\n      <SpecifiedTradeProduct><Name>Child A</Name>", 1),
		good: subLineCII,
	}, {
		rule: "BR-FXEXT-12",
		// A subtotal line under a subtotal line that carries a net amount, with no
		// net amount of its own — which is what the rule forbids, and which
		// promoting a DETAIL line to GROUP with its BT-131 removed produces.
		bad: strings.Replace(strings.Replace(subLineCII,
			"<LineID>11</LineID><ParentLineID>1</ParentLineID><LineStatusReasonCode>DETAIL</LineStatusReasonCode>",
			"<LineID>11</LineID><ParentLineID>1</ParentLineID><LineStatusReasonCode>GROUP</LineStatusReasonCode>", 1),
			"<SpecifiedTradeSettlementLineMonetarySummation><LineTotalAmount>60.00</LineTotalAmount></SpecifiedTradeSettlementLineMonetarySummation>",
			"<SpecifiedTradeSettlementLineMonetarySummation/>", 1),
		good: subLineCII,
	}, {
		rule: "BR-FXEXT-CII-DT-097a",
		bad:  fxWith(`<IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime>`, `<IssueDateTime><DateTimeString format="205">2024-01-01</DateTimeString></IssueDateTime>`),
		good: fxWith(`<IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime>`, `<IssueDateTime><DateTimeString format="205">202401011200</DateTimeString></IssueDateTime>`),
	}}

	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			seen[tc.rule] = true
			if !fxReports(t, ctx, tc.bad, tc.rule) {
				t.Errorf("%s did not fire on the document written to break it; a rule that never reports is a rule "+
					"that is present and inert, which every other guard here passes", tc.rule)
			}
			if fxReports(t, ctx, tc.good, tc.rule) {
				t.Errorf("%s fired on the document written to satisfy it", tc.rule)
			}
			// And it is Factur-X's rule only: no other profile publishes it.
			for _, p := range profiles {
				if p == ProfileExtended {
					continue
				}
				r, err := Validate(ctx, []byte(tc.bad), p)
				if err != nil {
					t.Fatalf("%v", err)
				}
				for _, v := range r.Violations {
					if v.Rule == tc.rule {
						t.Errorf("%s was reported at profile %q, which does not publish it", tc.rule, string(p))
					}
				}
			}
		})
	}
	for _, id := range facturXExtensionRules {
		if !seen[id] {
			t.Errorf("%s is evaluated and has no firing fixture; add one rather than trusting the rule body", id)
		}
	}
}

// fxWith edits validCII, failing loudly if the anchor moved.
func fxWith(anchor, replacement string) string {
	out := strings.Replace(validCII, anchor, replacement, 1)
	if out == validCII {
		return "MISSING ANCHOR: " + anchor
	}
	return out
}

// fxReports reports whether validating doc at EXTENDED yields the rule, at any
// severity — a rule flagged warning has to fire too.
func fxReports(t *testing.T, ctx context.Context, doc, rule string) bool {
	t.Helper()
	if strings.HasPrefix(doc, "MISSING ANCHOR") {
		t.Fatal(doc)
	}
	r, err := Validate(ctx, []byte(doc), ProfileExtended)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, v := range r.Violations {
		if v.Rule == rule {
			if v.Source != SourceFacturX {
				t.Errorf("%s came back under Source %q, want %q", rule, v.Source, SourceFacturX)
			}
			return true
		}
	}
	return false
}

// TestFacturXExtensionSeveritiesMatchTheArtefact is the severity half, in both
// directions and with no excuse list, over the rules this package evaluates.
func TestFacturXExtensionSeveritiesMatchTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	published := map[string]Severity{}
	for _, p := range profiles {
		for id, x := range fxNamed(fxDecode(t, dir, p)) {
			published[id] = x.a.severity()
		}
	}
	ctx := context.Background()
	// The firing fixtures above are what makes this checkable: a rule that never
	// reports has no severity to compare.
	for _, doc := range fxFiringDocuments() {
		r, err := Validate(ctx, []byte(doc), ProfileExtended)
		if err != nil {
			t.Fatalf("%v", err)
		}
		for _, v := range r.Violations {
			if v.Source != SourceFacturX {
				continue
			}
			if fxIsDataModelRule(v.Rule) {
				// The profile data model's assertions carry no identifier in the
				// artefact — the key is minted by this package — so there is
				// nothing to look up. Their severity is still checkable and is
				// checked: every one of the 2,159 is unflagged, which within this
				// artefact means fatal, and internal/gen/facturx refuses to emit
				// one that carries a flag rather than deciding a severity for it.
				if v.Severity != SeverityFatal {
					t.Errorf("%s was reported %s and every Factur-X data-model assertion is unflagged, which facturx.go reads as fatal",
						v.Rule, v.Severity)
				}
				continue
			}
			want, ok := published[v.Rule]
			if !ok {
				t.Errorf("this package reported %s under %q and no Factur-X Schematron publishes it", v.Rule, SourceFacturX)
				continue
			}
			if v.Severity != want {
				t.Errorf("%s was reported %s and FNFE flags it %s", v.Rule, v.Severity, want)
			}
		}
	}
}

// fxFiringDocuments is every document TestEveryFacturXExtensionRuleFires uses to
// make a rule report, so the severity guard runs over the same population rather
// than over a second set that could drift from it.
func fxFiringDocuments() []string {
	return []string{
		fxWith(`<ExchangedDocument><ID>INV-1</ID>`, `<ExchangedDocument><IncludedNote><SubjectCode>AAI</SubjectCode></IncludedNote><ID>INV-1</ID>`),
		fxWith(`<AssociatedDocumentLineDocument><LineID>1</LineID>`, `<AssociatedDocumentLineDocument><LineID>1</LineID><IncludedNote><SubjectCode>AAI</SubjectCode></IncludedNote>`),
		fxWith(`<BuyerTradeParty><Name>Buyer Co</Name>`, `<BuyerTradeParty><Name>Buyer Co</Name><SpecifiedTaxRegistration><ID schemeID="FC">123</ID></SpecifiedTaxRegistration>`),
		fxWith(`<SpecifiedTradeProduct><Name>Widget</Name>`, `<SpecifiedTradeProduct><Name>Widget</Name><ApplicableProductCharacteristic><TypeCode>NOT_A_CODE</TypeCode><Description>d</Description><Value>v</Value></ApplicableProductCharacteristic>`),
		fxWith(`<AssociatedDocumentLineDocument><LineID>1</LineID>`, `<AssociatedDocumentLineDocument><LineID>1</LineID><ParentLineID>1</ParentLineID>`),
		strings.Replace(subLineCII, "<LineTotalAmount>60.00</LineTotalAmount>", "<LineTotalAmount>59.00</LineTotalAmount>", 1),
		fxWith(`<IssueDateTime><DateTimeString format="102">20240101</DateTimeString></IssueDateTime>`, `<IssueDateTime><DateTimeString format="205">2024-01-01</DateTimeString></IssueDateTime>`),
	}
}

// TestFacturXRoutingReachesTheFacturXRuleSet closes the second door issue #56
// left open. ValidateCIUS is the entry point this package tells a caller to
// prefer; before this change an identifier reading
// "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended" matched no
// routing rule and fell through to the EN 16931 core with CEN's CII binding,
// which is the same defect reached by a different call.
func TestFacturXRoutingReachesTheFacturXRuleSet(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want Profile
	}{
		{"urn:factur-x.eu:1p0:minimum", ProfileMinimum},
		{"urn:factur-x.eu:1p0:basicwl", ProfileBasicWL},
		{"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic", ProfileBasic},
		{"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended", ProfileExtended},

		// The same four tiers under the German brand. FNFE's own code database
		// enumerates both identifiers for every tier that names itself, and
		// TestFacturXRoutingAcceptsEveryIdentifierTheAuthorityPublishes reads
		// that list out of the artefact rather than repeating it; these four are
		// here because this test asserts something the other one does not — that
		// the rule set is actually *run*, not merely named.
		{"urn:zugferd.de:2p0:minimum", ProfileMinimum},
		{"urn:zugferd.de:2p0:basicwl", ProfileBasicWL},
		{"urn:cen.eu:en16931:2017#compliant#urn:zugferd.de:2p0:basic", ProfileBasic},
		{"urn:cen.eu:en16931:2017#conformant#urn:zugferd.de:2p0:extended", ProfileExtended},
	} {
		t.Run(tc.id, func(t *testing.T) {
			got, ok := facturXProfileFromSpecID(tc.id)
			if !ok || got != tc.want {
				t.Errorf("facturXProfileFromSpecID(%q) = %q, %v; want %q, true", tc.id, string(got), ok, string(tc.want))
			}
			doc := strings.Replace(validCII, "<ID>urn:cen.eu:en16931:2017</ID>", "<ID>"+tc.id+"</ID>", 1)
			if doc == validCII {
				t.Fatal("the fixture was not modified")
			}
			det, err := Detect([]byte(doc))
			if err != nil {
				t.Fatalf("%v", err)
			}
			if det.Source != SourceFacturX {
				t.Errorf("Detect reported %q, want %q", det.Source, SourceFacturX)
			}
			r, err := ValidateCIUS(context.Background(), []byte(doc))
			if err != nil {
				t.Fatalf("%v", err)
			}
			named := false
			for _, g := range r.NotEvaluated {
				if strings.Contains(g.Rules, "BR-FXEXT") || strings.Contains(g.Rules, "profile data model") {
					named = true
				}
			}
			if !named {
				t.Errorf("ValidateCIUS on a %s document names no Factur-X coverage, so it did not run that rule set",
					string(tc.want))
			}
		})
	}
	// The EN 16931 tier declares CEN's own identifier and must stay CEN's, or a
	// plain EN 16931 CII invoice would silently acquire a Factur-X reading.
	det, err := Detect([]byte(validCII))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if det.Source != SourceEN16931 {
		t.Errorf("a document declaring urn:cen.eu:en16931:2017 detected as %q; the EN 16931 tier of Factur-X declares "+
			"exactly that identifier and is indistinguishable from a plain EN 16931 invoice by design", det.Source)
	}
}

// TestFacturXExtensionContextsAreReachedInTheCorpus answers the question the
// firing fixtures cannot: does anything FNFE publishes actually exercise these
// rules, or are they only reached by documents this suite wrote.
//
// It is the weaker of the two guards and it is kept for what it measures rather
// than for what it asserts. TestEveryFacturXExtensionRuleFires is the one that
// separates "implemented" from "implemented and inert" (C41); this one says how
// much of the authority's own corpus goes past each rule body, which is the
// number that tells a reader whether an FP=0 sweep over those 25 EXTENDED
// documents means anything for a given rule.
func TestFacturXExtensionContextsAreReachedInTheCorpus(t *testing.T) {
	files := fxExamples(t)
	if len(files) == 0 {
		t.Skip("no Factur-X examples found; see `make facturx-examples`")
	}
	atLeast(t, "Factur-X example invoices", len(files), minFacturXExamples)

	seen, docs := ruleContexts{}, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		p, _, ok := fxDeclaredProfile(data)
		if !ok || p != ProfileExtended {
			continue
		}
		parsed, perr := parseEN16931(newRun(context.Background()), data)
		if perr != nil {
			t.Fatalf("%s: %v", f, perr)
		}
		docs++
		validateFacturXExtensionRules(parsed, ProfileExtended, seen)
	}
	atLeast(t, "Factur-X EXTENDED examples", docs, minFacturXExtendedExamples)
	var reached []string
	for _, id := range facturXExtensionRules {
		if seen[id] > 0 {
			reached = append(reached, fmt.Sprintf("%s=%d", id, seen[id]))
		}
	}
	if len(reached) < minFacturXContextsReached {
		t.Errorf("only %d of the %d evaluated BR-FXEXT-* rules were asked about a context node anywhere in FNFE's own "+
			"%d EXTENDED examples, want at least %d; a rule nothing in the corpus reaches is a rule the FP=0 sweep says "+
			"nothing about", len(reached), len(facturXExtensionRules), docs, minFacturXContextsReached)
	}
	t.Logf("Factur-X extension contexts over %d EXTENDED examples: %v", docs, reached)
}

// TestFacturXCoverageSeveritiesMatchTheArtefact is the severity half of
// Coverage(SourceFacturX), and it is a test of its own rather than another
// Source in TestCoverageSeveritiesMatchThePublishedFlag for one mechanical
// reason: that test reads flags through schematronFlags, which keys on the id
// attribute, and no assertion in these five files has one. Adding Factur-X there
// would have made every entry silently unlookupable, which is the failure C31
// records — a guard that quietly stops guarding.
//
// Two of the five entries name a literal identifier and both are checked here,
// each against the authority whose flag this package quotes for it. The other
// three are the two BR-FXEXT-* carve-outs, whose 24/18 split
// TestFacturXExtensionRulesMatchTheArtefact derives from the artefact, and the
// data-model entry, whose assertions have no identifier to look up at all.
func TestFacturXCoverageSeveritiesMatchTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	cen := en16931SuiteDir()
	if dir == "" || cen == "" {
		t.Skip("Factur-X or CEN artefacts not present; run `make facturx-schematron en16931-artefacts`")
	}
	// PEPPOL-EN16931-R008 is FNFE's to flag here: OpenPEPPOL minted the identifier,
	// but the assertion this entry is about is the one FNFE merged into its own
	// files, and FNFE flags it warning explicitly.
	fx := map[string]Severity{}
	for _, p := range profiles {
		for id, x := range fxNamed(fxDecode(t, dir, p)) {
			fx[id] = x.a.severity()
		}
	}
	if got, want := fxCoverageEntry(t, "PEPPOL-EN16931-R008").Severity, fx["PEPPOL-EN16931-R008"]; got != want {
		t.Errorf("Coverage(SourceFacturX) records PEPPOL-EN16931-R008 as %s and FNFE flags it %s", got, want)
	}

	// CII-SR-464 is CEN's to flag, and that is the decision facturx.go argues:
	// CEN minted the identifier and the wording, FNFE sets no flag at all on it,
	// and reading an absent attribute as a severity would be this package deciding
	// one. So the entry quotes EN16931-CII-syntax.sch.
	flags := schematronFlags(t)
	got, ok := flags[facturXInertBinding]
	if !ok {
		t.Fatalf("CEN publishes no flag for %s; the harness is reading the wrong artefacts", facturXInertBinding)
	}
	want, known := severityOfFlag(pickFlag(got))
	if !known {
		t.Fatalf("%s carries the flag %v, which this package does not know how to fold onto a Severity",
			facturXInertBinding, keysOf(got))
	}
	if e := fxCoverageEntry(t, facturXInertBinding); e.Severity != want {
		t.Errorf("Coverage(SourceFacturX) records %s as %s and CEN flags it %v; this package quotes CEN's flag for the "+
			"identifiers Factur-X republishes from CEN, and that has to be the flag in the table too",
			facturXInertBinding, e.Severity, keysOf(got))
	}
}
