package formalis

import (
	"context"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The guards that hold cius_pt_datatype_table.go to AT/eSPap's Schematron.
//
// The table is generated, so the question these ask is not "was it transcribed
// correctly" — nothing was transcribed — but the four questions a generated table
// actually raises:
//
//   - does it hold *exactly* what the artefact publishes, so that a wholesale drop
//     fails even though every per-rule comparison would pass;
//   - is every row's XPath byte-for-byte AT's, so the fidelity claim is a string
//     comparison and not an argument about a transformation;
//   - can this package read every row it ships, so a rule cannot be silently
//     unevaluated; and
//   - does every rule reach a context and fire on something, so a rule bound to an
//     element name no document contains is told apart from one that is simply
//     satisfied.
//
// Everything below reads the artefact with an XML decoder and never with a regular
// expression. That is C31's lesson and it bites here harder than anywhere: 40 of
// these 291 tests contain a literal '>', so the character class in
// `<(?:sch:)?assert\s([^>]*)>` would make the family survey as 251.

// ---------------------------------------------------------------------------
// Re-deriving the artefact
// ---------------------------------------------------------------------------

// ptDTArtefactRule is one <rule> as decoded from the vendored Schematron.
type ptDTArtefactRule struct {
	context string
	lets    [][2]string
	asserts []ptDTAssertSrc
	flags   []string
}

// ptDTMsgPrefix is the "[DT-CIUS-PT-nnn]-" AT writes in front of every message.
var ptDTMsgPrefix = regexp.MustCompile(`^\[[^]]*\]\s*-?\s*`)

// ptDTOwn admits the family this table owns. The BR-* assertions that share the
// condition pattern's <rule> elements are cius_pt_rules.go's.
var ptDTOwn = regexp.MustCompile(`^DT-CIUS-PT-`)

// ptDTReadDatatype decodes the concrete UBL-datatype pattern of one vendored
// version. It returns nil when the artefact is absent.
func ptDTReadDatatype(t *testing.T, version string) ([][2]string, []ptDTArtefactRule) {
	t.Helper()
	path := filepath.Join("testdata", "cius-pt", "schematron", version, "datatype",
		"urn_feap.gov.pt_CIUS-PT_"+version+"-UBL-datatype.sch")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var patternLets [][2]string
	var rules []ptDTArtefactRule
	depth := 0
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, terr := dec.Token()
		if terr != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			depth++
			attr := func(n string) string {
				for _, a := range e.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch e.Name.Local {
			case "rule":
				rules = append(rules, ptDTArtefactRule{context: ptCollapse(attr("context"))})
			case "let":
				if depth == 2 {
					patternLets = append(patternLets, [2]string{attr("name"), ptCollapse(attr("value"))})
				} else {
					r := &rules[len(rules)-1]
					r.lets = append(r.lets, [2]string{attr("name"), ptCollapse(attr("value"))})
				}
			case "assert", "report":
				text, _ := ptDTElementText(dec, e)
				r := &rules[len(rules)-1]
				r.flags = append(r.flags, attr("flag"))
				if !ptDTOwn.MatchString(attr("id")) {
					continue
				}
				r.asserts = append(r.asserts, ptDTAssertSrc{
					id:      attr("id"),
					kind:    e.Name.Local,
					test:    ptCollapse(attr("test")),
					message: ptDTMsgPrefix.ReplaceAllString(ptCollapse(text), ""),
				})
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
	return patternLets, rules
}

// ptDTElementText reads an element's character data to its end tag.
func ptDTElementText(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String(), err
		}
		switch e := tok.(type) {
		case xml.CharData:
			b.Write(e)
		case xml.EndElement:
			if e.Name.Local == start.Name.Local {
				return b.String(), nil
			}
		}
	}
}

// ptDTReadCondition decodes the DT-CIUS-PT-* half of the abstract condition
// pattern, resolved through the UBL binding the way a Schematron processor
// resolves it. It reuses ptResolveArtefact's reading of the abstract/concrete pair
// so the two guards cannot disagree about what "resolved" means.
func ptDTReadCondition(t *testing.T, version string) []ptDTArtefactRule {
	t.Helper()
	abstract := filepath.Join("testdata", "cius-pt", "schematron", version, "abstract",
		"urn_feap.gov.pt_CIUS-PT_"+version+"-condition.sch")
	binding := filepath.Join("testdata", "cius-pt", "schematron", version, "UBL",
		"urn_feap.gov.pt_CIUS-PT_"+version+"-UBL-condition.sch")
	adata, err := os.ReadFile(abstract)
	if err != nil {
		return nil
	}
	bdata, err := os.ReadFile(binding)
	if err != nil {
		return nil
	}

	params := map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(string(bdata)))
	for {
		tok, terr := dec.Token()
		if terr != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "param" {
			var n, v string
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "name":
					n = a.Value
				case "value":
					v = a.Value
				}
			}
			params[n] = v
		}
	}
	names := make([]string, 0, len(params))
	for n := range params {
		names = append(names, n)
	}
	// Longest first: the parameter set holds both $VATAA and $VATAA_Allowance, and
	// substituting the shorter one first would corrupt the longer.
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	resolve := func(expr string) string {
		for _, n := range names {
			expr = strings.ReplaceAll(expr, "$"+n, params[n])
			expr = strings.ReplaceAll(expr, "$"+strings.TrimSpace(n), params[n])
		}
		return ptCollapse(expr)
	}

	var rules []ptDTArtefactRule
	dec = xml.NewDecoder(strings.NewReader(string(adata)))
	for {
		tok, terr := dec.Token()
		if terr != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := func(n string) string {
			for _, a := range se.Attr {
				if a.Name.Local == n {
					return a.Value
				}
			}
			return ""
		}
		switch se.Name.Local {
		case "rule":
			rules = append(rules, ptDTArtefactRule{context: resolve(attr("context"))})
		case "assert", "report":
			text, _ := ptDTElementText(dec, se)
			r := &rules[len(rules)-1]
			r.flags = append(r.flags, attr("flag"))
			if !ptDTOwn.MatchString(attr("id")) {
				continue
			}
			r.asserts = append(r.asserts, ptDTAssertSrc{
				id:      attr("id"),
				kind:    se.Name.Local,
				test:    resolve(attr("test")),
				message: ptDTMsgPrefix.ReplaceAllString(ptCollapse(text), ""),
			})
		}
	}
	return rules
}

// ---------------------------------------------------------------------------
// Drift
// ---------------------------------------------------------------------------

// TestCIUSPTDatatypeTableMatchesTheArtefact re-derives both generated tables from
// the vendored Schematron and compares them row by row.
//
// It compares rule *order* as well as content, because order is meaning here: under
// ISO Schematron a node is processed by the first rule of a pattern whose context
// matches it, so two rules exchanged is two rules whose assertions may stop being
// reachable. It compares the whole rule list and not only the rules carrying an
// assertion, for the same reason.
func TestCIUSPTDatatypeTableMatchesTheArtefact(t *testing.T) {
	patternLets, dtRules := ptDTReadDatatype(t, "2.1.1")
	condRules := ptDTReadCondition(t, "2.1.1")
	if dtRules == nil || condRules == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	ptDTCompareTable(t, "ptDatatypePattern", ptDatatypePattern, patternLets, dtRules)
	ptDTCompareTable(t, "ptConditionPattern", ptConditionPattern, nil, condRules)
	t.Logf("CIUS-PT datatype tables: %d + %d rules re-derived from the 2.1.1 Schematron and identical",
		len(ptDatatypePattern.rules), len(ptConditionPattern.rules))
}

func ptDTCompareTable(t *testing.T, name string, got ptDTPattern, lets [][2]string, want []ptDTArtefactRule) {
	t.Helper()
	if len(got.lets) != len(lets) {
		t.Errorf("%s holds %d pattern-level <let>s and the artefact declares %d", name, len(got.lets), len(lets))
	}
	for i := range lets {
		if i >= len(got.lets) {
			break
		}
		if got.lets[i].name != lets[i][0] || got.lets[i].value != lets[i][1] {
			t.Errorf("%s pattern <let> %d\n  artefact: $%s = %s\n  table   : $%s = %s",
				name, i, lets[i][0], lets[i][1], got.lets[i].name, got.lets[i].value)
		}
	}
	if len(got.rules) != len(want) {
		t.Fatalf("%s holds %d rules and the artefact publishes %d. A rule dropped wholesale is invisible to "+
			"every per-rule comparison below", name, len(got.rules), len(want))
	}
	for i, w := range want {
		g := got.rules[i]
		if g.context != w.context {
			t.Errorf("%s rule %d context\n  artefact: %s\n  table   : %s", name, i, w.context, g.context)
			continue
		}
		if len(g.lets) != len(w.lets) {
			t.Errorf("%s rule %d (%s) holds %d <let>s and the artefact declares %d",
				name, i, w.context, len(g.lets), len(w.lets))
		} else {
			for j := range w.lets {
				if g.lets[j].name != w.lets[j][0] || g.lets[j].value != w.lets[j][1] {
					t.Errorf("%s rule %d <let> %d\n  artefact: $%s = %s\n  table   : $%s = %s",
						name, i, j, w.lets[j][0], w.lets[j][1], g.lets[j].name, g.lets[j].value)
				}
			}
		}
		if len(g.asserts) != len(w.asserts) {
			t.Errorf("%s rule %d (%s) holds %d assertions and the artefact publishes %d",
				name, i, w.context, len(g.asserts), len(w.asserts))
			continue
		}
		for j, wa := range w.asserts {
			ga := g.asserts[j]
			if ga.id != wa.id {
				t.Errorf("%s rule %d assertion %d is %s in the table and %s in the artefact", name, i, j, ga.id, wa.id)
				continue
			}
			if ga.kind != wa.kind {
				t.Errorf("%s is a <%s> in the artefact and the table records a <%s>. The two are opposite — an "+
					"assert fires when its test is false and a report when it is true — so the rule is inverted",
					ga.id, wa.kind, ga.kind)
			}
			if ga.test != wa.test {
				t.Errorf("%s test\n  artefact: %s\n  table   : %s", ga.id, wa.test, ga.test)
			}
			if ga.message != wa.message {
				t.Errorf("%s message\n  artefact: %s\n  table   : %s", ga.id, wa.message, ga.message)
			}
		}
	}
}

// TestCIUSPTDatatypeTableHoldsThePublishedSet is the wholesale-drop guard: the two
// tables together hold *exactly* the DT-CIUS-PT-* set the artefact publishes, all of
// it flagged fatal.
//
// The drift test above compares rule for rule and would pass on a table that had
// lost a whole pattern, because it compares what is there. This one compares the
// sets, which is the check C39 was missing when eight published BR-AA-* rules were
// in neither the code nor the coverage table.
func TestCIUSPTDatatypeTableHoldsThePublishedSet(t *testing.T) {
	published := ptDTPublishedIDs(t, "2.1.1")
	if published == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	inTable := map[string]int{}
	for _, p := range []ptDTPattern{ptDatatypePattern, ptConditionPattern} {
		for _, r := range p.rules {
			for _, a := range r.asserts {
				inTable[a.id]++
			}
		}
	}
	for id := range published {
		if inTable[id] == 0 {
			t.Errorf("AT/eSPap publishes %s and neither generated table holds it", id)
		}
	}
	for id := range inTable {
		if _, ok := published[id]; !ok {
			t.Errorf("the generated tables hold %s, which the vendored Schematron does not publish", id)
		}
	}
	// 290 identifiers over 291 assertion elements: AT publishes DT-CIUS-PT-176
	// twice under one identifier, once for the invoice branch and once for the
	// credit-note branch.
	if len(inTable) != 290 {
		t.Errorf("the tables hold %d distinct DT-CIUS-PT-* identifiers, want 290", len(inTable))
	}
	total := 0
	for _, n := range inTable {
		total += n
	}
	if total != 291 {
		t.Errorf("the tables hold %d DT-CIUS-PT-* assertions, want 291", total)
	}
	if ptDatatype.asserts != 269 || ptCondition.asserts != 22 {
		t.Errorf("the compiled patterns hold %d + %d assertions, want 269 + 22",
			ptDatatype.asserts, ptCondition.asserts)
	}
	t.Logf("CIUS-PT datatype: %d published identifiers over %d assertions, all fatal, all in the tables",
		len(inTable), total)
}

// ptDTPublishedIDs decodes every DT-CIUS-PT-* identifier one vendored version
// publishes, from every .sch in its tree, and fails on one that is not fatal.
func ptDTPublishedIDs(t *testing.T, version string) map[string]string {
	t.Helper()
	dir := filepath.Join("testdata", "cius-pt", "schematron", version)
	var files []string
	for _, g := range []string{"*.sch", "*/*.sch"} {
		m, _ := filepath.Glob(filepath.Join(dir, g))
		files = append(files, m...)
	}
	if len(files) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			tok, terr := dec.Token()
			if terr != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok || (se.Name.Local != "assert" && se.Name.Local != "report") {
				continue
			}
			var id, flag string
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "id":
					id = a.Value
				case "flag":
					flag = a.Value
				}
			}
			if !ptDTOwn.MatchString(id) {
				continue
			}
			if flag != "fatal" {
				t.Errorf("%s is flagged %q in %s; every DT-CIUS-PT-* assertion is fatal, which is what lets "+
					"the plain adder report them", id, flag, filepath.Base(f))
			}
			out[id] = flag
		}
	}
	return out
}

// TestCIUSPTDatatypeTableIsReadable parses every committed row again and checks the
// context paths against the contexts they were derived from.
//
// The package already panics at load on a row it cannot parse, so the first half is
// belt and braces. The second half is not: `paths` is the only field in the table
// that is a *transformation* of AT's text rather than a quotation of it, so it is
// the one place a generated table could be wrong in a way the drift test cannot
// see. Re-deriving it here from the quoted context closes that.
func TestCIUSPTDatatypeTableIsReadable(t *testing.T) {
	for _, p := range []ptDTPattern{ptDatatypePattern, ptConditionPattern} {
		for _, r := range p.rules {
			for _, a := range r.asserts {
				e, err := ptDTParse(a.test)
				if err != nil {
					t.Errorf("%s: %s: %v", a.id, a.test, err)
					continue
				}
				if err := ptDTCheck(e); err != nil {
					t.Errorf("%s: %s: %v", a.id, a.test, err)
				}
			}
			got := ptDTFormatPaths(r.paths)
			want := ptDTFormatPaths(ptDTParseContext(t, r.context))
			if got != want {
				t.Errorf("the context %q resolves to %s and the table records %s", r.context, want, got)
			}
		}
	}
}

// ptDTParseContext re-derives a context's root-anchored paths, the way gen.py does.
func ptDTParseContext(t *testing.T, ctx string) []ptDTCtxPath {
	t.Helper()
	var out []ptDTCtxPath
	for _, branch := range ptDTSplitTop(ctx, '|') {
		branch = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(branch), "//"))
		var p ptDTCtxPath
		for i, raw := range ptDTSplitTop(branch, '/') {
			raw = strings.TrimSpace(raw)
			name, pred := raw, ""
			if j := strings.IndexByte(raw, '['); j >= 0 {
				if !strings.HasSuffix(raw, "]") {
					t.Fatalf("cannot read the context step %q of %q", raw, ctx)
				}
				name, pred = raw[:j], ptCollapse(raw[j+1:len(raw)-1])
			}
			name = ptDTLocal(name)
			if i == 0 && pred == "" && (name == "Invoice" || name == "CreditNote") {
				p.root = name
				continue
			}
			p.steps = append(p.steps, ptDTCtxStep{name: name, pred: pred})
		}
		out = append(out, p)
	}
	return out
}

func ptDTSplitTop(s string, sep byte) []string {
	var parts []string
	depth, quote, start := 0, byte(0), 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

func ptDTFormatPaths(ps []ptDTCtxPath) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString("{" + p.root)
		for _, s := range p.steps {
			b.WriteString("/" + s.name)
			if s.pred != "" {
				b.WriteString("[" + s.pred + "]")
			}
		}
		b.WriteString("}")
	}
	return b.String()
}

// TestCIUSPTDatatypeLengthMatchersAgreeWithRegexp holds ptDTLengthMatcher to the
// general engine.
//
// It is the one place in this rule set where a regular expression is compiled by
// shape rather than handed to regexp: `^(.{1,N})$` is 149 of the 192 matches()
// calls, and DT-CIUS-PT-111.1's bound of 6,826,666 is above the repetition limit Go
// accepts at all. So every such pattern whose bound Go *does* accept is compiled
// both ways and the two are required to agree on a battery of inputs, including the
// newline that is the only case where the shape matters.
func TestCIUSPTDatatypeLengthMatchersAgreeWithRegexp(t *testing.T) {
	inputs := []string{"", " ", "x", "xx", strings.Repeat("x", 5), strings.Repeat("x", 20),
		strings.Repeat("x", 21), strings.Repeat("x", 50), strings.Repeat("x", 51),
		strings.Repeat("x", 200), strings.Repeat("x", 201), "a\nb", "\n", "a\rb", "é", "ééé"}
	checked := 0
	for pattern, matcher := range ptDTPatternCache {
		flags, src, _ := strings.Cut(pattern, "\x00")
		lm, ok := matcher.(ptDTLengthMatcher)
		if !ok {
			continue
		}
		if lm.max > 1000 {
			continue // Go's regexp refuses the bound; that is why the shape is compiled
		}
		goSrc := src
		if strings.Contains(flags, "s") {
			goSrc = "(?s)" + goSrc
		}
		re := regexp.MustCompile(goSrc)
		checked++
		for _, in := range inputs {
			if re.MatchString(in) != lm.match(in) {
				t.Errorf("%q on %q: regexp says %v and ptDTLengthMatcher says %v",
					src, in, re.MatchString(in), lm.match(in))
			}
		}
	}
	// Seven distinct patterns after the 6,826,666-character one is skipped: AT
	// writes the same handful of length limits over and over, which is exactly why
	// compiling them by shape is worth doing.
	if checked < 7 {
		t.Fatalf("checked only %d length matchers; the cache is not being read", checked)
	}
	t.Logf("checked %d anchored-length matchers against Go's regexp on %d inputs each", checked, len(inputs))
}

// ---------------------------------------------------------------------------
// The two versions
// ---------------------------------------------------------------------------

// TestCIUSPTDatatypeVersionsDiffer pins the 2.0.0/2.1.1 divergence for this family,
// the way TestCIUSPTVersionsAgreeExceptOnTheCategoryAliases does for the business
// rules.
//
// Both versions are live — phive-rules registers a validation set for each and ships
// ten instances for each — so "which version's condition governs" is a real question.
// This package evaluates 2.1.1, and what that costs is measured rather than assumed:
// 2.1.1 adds three identifiers over four assertions and changes nothing else.
func TestCIUSPTDatatypeVersionsDiffer(t *testing.T) {
	old := ptDTPublishedIDs(t, "2.0.0")
	cur := ptDTPublishedIDs(t, "2.1.1")
	if old == nil || cur == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	var added, removed []string
	for id := range cur {
		if _, ok := old[id]; !ok {
			added = append(added, id)
		}
	}
	for id := range old {
		if _, ok := cur[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	const want = "DT-CIUS-PT-175 DT-CIUS-PT-176 DT-CIUS-PT-177"
	if strings.Join(added, " ") != want {
		t.Errorf("2.1.1 adds %v over 2.0.0; the expected addition is %q — the exempt-category taxable-amount "+
			"summation, the one-breakdown-per-exemption-reason rule and the positive base quantity", added, want)
	}
	if len(removed) != 0 {
		t.Errorf("2.0.0 publishes %v and 2.1.1 does not; a withdrawn rule belongs in a commit message", removed)
	}
	// And the rules both versions publish otherwise say the same thing. The
	// datatype pattern is concrete in both, so this is a straight comparison of AT's
	// own XPath — no parameter resolution, no room for the comparison to be about
	// this package's reading rather than about the artefact.
	_, oldRules := ptDTReadDatatype(t, "2.0.0")
	_, curRules := ptDTReadDatatype(t, "2.1.1")
	oldTests := map[string]string{}
	for _, r := range oldRules {
		for _, a := range r.asserts {
			oldTests[a.id] = a.kind + " " + r.context + " :: " + a.test
		}
	}
	var differ []string
	for _, r := range curRules {
		for _, a := range r.asserts {
			if o, ok := oldTests[a.id]; ok && o != a.kind+" "+r.context+" :: "+a.test {
				differ = append(differ, a.id)
			}
		}
	}
	sort.Strings(differ)
	var expected []string
	for id := range ptDTVersionChanges {
		expected = append(expected, id)
	}
	sort.Strings(expected)
	if strings.Join(differ, " ") != strings.Join(expected, " ") {
		t.Errorf("2.0.0 and 2.1.1 disagree on %v; the recorded divergence is %v. A wider one means this "+
			"package's choice of 2.1.1 has consequences that were argued for a narrower difference",
			differ, expected)
	}
	t.Logf("CIUS-PT datatype: 2.0.0 publishes %d identifiers and 2.1.1 publishes %d; 2.1.1 adds %v and widens "+
		"%d of the ones both carry, each in the direction of accepting more", len(old), len(cur), added, len(differ))
}

// ptDTVersionChanges records what 2.1.1 changed in the rules 2.0.0 already
// published, and it is the evidence behind this package evaluating 2.1.1.
//
// Every one of the seven is a *widening*: 2.1.1 accepts documents 2.0.0 refused, and
// none of them refuses a document 2.0.0 accepted. That makes the choice of version
// the conservative one for a false-positive-free validator — evaluating 2.0.0 would
// report Portuguese invoices that AT's current release accepts — and it is a
// measured fact rather than a preference, which is why the list is here rather than
// in a comment.
var ptDTVersionChanges = map[string]string{
	"DT-CIUS-PT-020": "2.1.1 adds matches()'s 's' flag to the 500-character note limit, so a note containing " +
		"a newline is no longer refused for that alone",
	"DT-CIUS-PT-027.2": "2.1.1 folds 'SEPA' into the ISO 6523 scheme list and drops the ancestor-qualified " +
		"alternative that admitted it only under the Seller or the Payee",
	"DT-CIUS-PT-044.2": "the same change on the Buyer's party identifier, where the effect is a widening: " +
		"2.0.0's ancestor test could not be satisfied under cac:AccountingCustomerParty at all",
	"DT-CIUS-PT-058.2": "the same change on the Payee's party identifier",
	"DT-CIUS-PT-112":   "2.1.1 adds 'text/plain' to the attachment mime-code list",
	"DT-CIUS-PT-148":   "2.1.1 adds 'ADF' and 'ST' to the line-level charge reason code list",
	"DT-CIUS-PT-150":   "2.1.1 adds 'ST' to the document-level charge reason code list",
}

// ---------------------------------------------------------------------------
// Reachability, cleanliness and firing
// ---------------------------------------------------------------------------

// ptDTContextCounts counts, per identifier, how many context nodes each rule is
// evaluated against in one document — the instrument PR 23 built for the business
// rules, applied to the generated family.
//
// It is a different question from "does it fire". A rule that reports nothing across
// 1,680 documents is either a rule that was asked and kept answering yes — the
// desired outcome — or a rule bound to an element name no document contains, which
// is not a working rule at all and looks identical from the outside.
func ptDTContextCounts(root *ciiNode, into map[string]int) {
	doc := &ptDTDoc{root: root}
	for _, p := range []*ptDTCompiledPattern{ptDatatype, ptCondition} {
		for _, e := range ptDTGather(p, root, doc) {
			for _, a := range p.rules[e.rule].asserts {
				into[a.id]++
			}
		}
	}
}

// TestCIUSPTDatatypeContextsAreReachable is requirement two of this family's
// oracle: every one of the 290 identifiers is evaluated against a real context node
// somewhere in the corpus, or this package says which are not and why.
//
// The exception table is empty, and that is the result rather than the design: every
// published identifier is reached. If one stops being reached, this fails and the
// reason has to be written down rather than assumed.
func TestCIUSPTDatatypeContextsAreReachable(t *testing.T) {
	published := ptDTPublishedIDs(t, "2.1.1")
	if published == nil {
		t.Skip("CIUS-PT Schematron not present; run `make cius-schematron`")
	}
	total := map[string]int{}
	files, ubl := 0, 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil || parsed.inv.syntax != "UBL" {
			return nil
		}
		ubl++
		ptDTContextCounts(parsed.root, total)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-PT datatype context sweep corpus", files, minCorpusDocuments)

	var unreached []string
	for id := range published {
		if total[id] == 0 {
			unreached = append(unreached, id)
		}
	}
	sort.Strings(unreached)
	if len(unreached) != 0 {
		t.Errorf("no document in the corpus reaches the context of %v, and nothing says why. A rule bound to an "+
			"element the corpus never contains is indistinguishable from a rule bound to a misspelt one: either "+
			"record a reason or fix the binding", unreached)
	}
	nodes := 0
	for _, n := range total {
		nodes += n
	}
	t.Logf("CIUS-PT datatype contexts: %d of %d published identifiers reached, %d context nodes over %d UBL "+
		"documents, none unreachable", len(published)-len(unreached), len(published), nodes, ubl)
}

// TestCIUSPTDatatypeCorpusIsClean is the FP=0 oracle for this family, and it is the
// same twenty documents PR 23's business-rule oracle rests on: phive-rules registers
// all twenty through PhiveTestFile.createGoodCase against the *whole* compiled
// Schematron — every phase, both versions — so each is AT's own claim that it
// violates none of the 363 published assertions, these 290 included.
//
// It also counts the XPath dynamic errors the family raises, because those are the
// one place this evaluator declines to give a verdict, and a rule set that quietly
// stopped answering would look exactly like a clean one.
func TestCIUSPTDatatypeCorpusIsClean(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-pt/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("CIUS-PT corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-PT datatype corpus", len(files), minCIUSPTInstances)
	errs, clean := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil {
			t.Fatalf("%s: %v", f, perr)
		}
		var got []string
		errs += ptDTValidate(r, parsed.root, func(rule, msg string) { got = append(got, rule) })
		if len(got) != 0 {
			sort.Strings(got)
			t.Errorf("%s: expected 0 DT-CIUS-PT findings on a conformant sample, got %v", filepath.Base(f), got)
		} else {
			clean++
		}
	}
	if errs != 0 {
		t.Errorf("AT/eSPap's own instances raised %d XPath dynamic errors, so that many assertions gave no "+
			"verdict at all. On the authority's own conforming documents every assertion should evaluate", errs)
	}
	t.Logf("CIUS-PT datatype corpus: %d/%d instances clean (FP=0) across all 290 published DT-CIUS-PT-* "+
		"identifiers, 0 dynamic errors", clean, len(files))
}

// TestCIUSPTDatatypeFiresAcrossTheCorpus is the compensating floor PR 17's advisory
// ratchets are: a family this large that silently stopped firing would leave every
// FP=0 oracle in this suite green, because they all assert the *absence* of findings.
//
// The population is every UBL document in the corpus put through ValidateCIUSPT.
// Most of them are not Portuguese invoices, and that is the point: a Peppol or
// XRechnung invoice asked Portuguese questions answers no to a great many of them,
// which is what makes the number large enough to be a meaningful ratchet.
func TestCIUSPTDatatypeFiresAcrossTheCorpus(t *testing.T) {
	byRule := map[string]int{}
	files, findings, errs := 0, 0, 0
	_ = filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		files++
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil || parsed.inv.syntax != "UBL" {
			return nil
		}
		errs += ptDTValidate(r, parsed.root, func(rule, msg string) {
			byRule[rule]++
			findings++
		})
		return nil
	})
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-PT datatype sweep corpus", files, minCorpusDocuments)
	atLeast(t, "CIUS-PT datatype rules firing", len(byRule), minCIUSPTDatatypeRulesFiring)
	atLeast(t, "CIUS-PT datatype findings", findings, minCIUSPTDatatypeFindings)
	t.Logf("CIUS-PT datatype bindings: %d findings from %d distinct rules over %d documents, %d dynamic errors",
		findings, len(byRule), files, errs)
}

// TestCIUSPTDatatypeFastPathsAgreeWithTheGeneralEvaluator holds the three
// allocation optimisations in cius_pt_datatype_eval.go to the semantics they claim
// to preserve.
//
// ptDTEvalString, ptDTEvalNum and the numeric comparison exist because this pass
// runs over 1,680 documents on every test run and the naive shape allocated a
// sequence per operator. Every one of them is a shortcut around the general
// evaluator, and a shortcut around an evaluator is exactly the kind of change that
// alters a rule without anyone noticing — the empty-sequence propagation AT's
// arithmetic tier is built on is one sign flip away from being wrong.
//
// So the whole corpus goes through both, and the findings have to be identical in
// content and in order.
func TestCIUSPTDatatypeFastPathsAgreeWithTheGeneralEvaluator(t *testing.T) {
	run := func() ([]string, int) {
		var out []string
		errs := 0
		_ = filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
				return nil
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			r := newRun(context.Background())
			parsed, perr := parseEN16931(r, data)
			if perr != nil || parsed.inv.syntax != "UBL" {
				return nil
			}
			errs += ptDTValidate(r, parsed.root, func(rule, msg string) {
				out = append(out, filepath.Base(p)+" "+rule)
			})
			return nil
		})
		return out, errs
	}
	fast, fastErrs := run()
	if len(fast) == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	ptDTFastPaths = false
	general, generalErrs := run()
	ptDTFastPaths = true

	if fastErrs != generalErrs {
		t.Errorf("the fast paths raise %d dynamic errors and the general evaluator raises %d", fastErrs, generalErrs)
	}
	if len(fast) != len(general) {
		t.Fatalf("the fast paths report %d findings and the general evaluator reports %d", len(fast), len(general))
	}
	for i := range fast {
		if fast[i] != general[i] {
			t.Fatalf("finding %d differs: fast %q, general %q", i, fast[i], general[i])
		}
	}
	t.Logf("CIUS-PT datatype fast paths: %d findings and %d dynamic errors, identical to the general evaluator",
		len(fast), fastErrs)
}

// TestEveryCIUSPTDatatypeRuleFires is requirement three's firing half for the
// generated family: every one of the 290 published identifiers has a document in
// this repository that makes it fire.
//
// The silent half is TestCIUSPTDatatypeCorpusIsClean's twenty AT instances and the
// CIUS-PT baseline, which runCIUSSuite requires clean. Together they are the "both
// verdicts" property stated over the artefact rather than over a hand-maintained
// list, so a rule AT adds upstream fails the build on the day it is fetched.
func TestEveryCIUSPTDatatypeRuleFires(t *testing.T) {
	fired := ptDTBuiltFixtures()
	var missing []string
	for _, p := range []ptDTPattern{ptDatatypePattern, ptConditionPattern} {
		for _, r := range p.rules {
			for _, a := range r.asserts {
				if !fired[a.id] {
					missing = append(missing, a.id)
				}
			}
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("no fixture in this repository makes %v fire, so nothing would notice if the rule stopped "+
			"being evaluated. Add a document to ptDTHandFixtures", missing)
	}
	for id := range fired {
		if !strings.HasPrefix(id, "DT-CIUS-PT-") {
			continue
		}
		if _, ok := ptDTEvaluatedIdentifiers()[id]; !ok {
			t.Errorf("a fixture made %s fire and the generated tables do not hold it", id)
		}
	}
	t.Logf("CIUS-PT datatype: all %d published identifiers have a document that makes them fire",
		len(ptDTEvaluatedIdentifiers()))
}

// ptDTEvaluatedIdentifiers is the set the generated tables evaluate, derived from
// the tables rather than written down beside them.
func ptDTEvaluatedIdentifiers() map[string]Severity {
	out := map[string]Severity{}
	for _, p := range []ptDTPattern{ptDatatypePattern, ptConditionPattern} {
		for _, r := range p.rules {
			for _, a := range r.asserts {
				// Every DT-CIUS-PT-* assertion AT publishes is flagged fatal, which
				// TestCIUSPTDatatypeTableHoldsThePublishedSet checks against the
				// artefact in both directions rather than assuming here.
				out[a.id] = SeverityFatal
			}
		}
	}
	return out
}

// The generated family joins ciusEvaluated, which is what puts it inside
// TestCIUSSeveritiesQuoteTheirAuthority (every emitted severity compared against the
// artefact's flag, both directions), TestCIUSFindingsStayInsideTheEvaluatedSet
// (nothing emitted that the table does not name) and
// TestEveryPublishedCIUSRuleIsEvaluatedOrDisclaimed.
//
// It is merged rather than pasted because the list is 290 entries long and derived
// from a generated table: pasting it would create a second copy that can disagree
// with the first, which is the state C34's two phantom Peppol entries were in.
// Package-level variables are initialised before any init function runs, so
// ciusEvaluated is populated by the time this executes.
func init() {
	for id, sev := range ptDTEvaluatedIdentifiers() {
		ciusEvaluated[SourceCIUSPT][id] = sev
	}
}
