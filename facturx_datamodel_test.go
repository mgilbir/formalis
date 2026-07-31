package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The guards on the Factur-X profile data model.
//
// There are four of them and they close four different holes, which is worth
// stating because the previous generated tier in this package had three of the
// four and shipped a rule shape that passed for every document (C41):
//
//   - TestFacturXDataModelMatchesTheArtefact re-derives the whole table from the
//     five Schematrons and compares it *as a sequence*, in both directions. A row
//     the artefact publishes and the table omits fails, and so does a row the
//     table invents. Because it compares sequences rather than looking each row
//     up, a wholesale drop fails even though every surviving row still matches.
//
//   - TestFacturXDataModelRoundTripsToItsXPath renders every decomposed context,
//     count() argument and code-list value back to XPath and compares it, token
//     for token, against the artefact's own string held beside it. The first test
//     proves the table holds FNFE's strings; this one proves the structure the
//     evaluator actually reads is those strings and not something adjacent to
//     them.
//
//   - TestEveryFacturXDataModelAssertionFires is the answer to C41. For each of
//     the 2,159 assertions it builds two synthetic documents from the table
//     itself — one that satisfies the assertion and one that breaks it — and
//     requires the evaluator to stay silent on the first and report on the
//     second. "Every identifier has an implementation" and "every context is
//     reached" both passed for the rule shape C41 found; a rule that is present,
//     reachable and inert fails this one on the second document, and a rule that
//     reports unconditionally fails it on the first.
//
//   - TestFacturXDataModelIsRatcheted holds the committed table to a floor
//     without reading the artefact at all, so a checkout with no Schematrons
//     cannot silently ship a table somebody emptied.

// fxDMProfileToken is the token that goes into a profile's synthetic
// identifiers. It is the artefact's own profile name with the space removed,
// which is what internal/gen/facturx emits.
var fxDMProfileToken = map[Profile]string{
	ProfileMinimum:  "MINIMUM",
	ProfileBasicWL:  "BASICWL",
	ProfileBasic:    "BASIC",
	ProfileEN16931:  "EN16931",
	ProfileExtended: "EXTENDED",
}

// fxDMRow is one data-model assertion reduced to what both sides can state: the
// key, the pattern it sits in, its rule's context, its test and its message. It
// is what the fidelity test compares, and it is deliberately all strings — a
// comparison of decomposed structures would be comparing the generator against
// itself.
type fxDMRow struct {
	key     string
	pattern int
	context string
	test    string
	msg     string
}

func (r fxDMRow) String() string {
	return fmt.Sprintf("%s pattern=%d context=%q test=%q msg=%q", r.key, r.pattern, r.context, r.test, r.msg)
}

// fxDMFromTable reads the committed table back as rows.
func fxDMFromTable(p Profile) []fxDMRow {
	var out []fxDMRow
	for _, r := range facturXDataModel[p] {
		for _, a := range r.asserts {
			out = append(out, fxDMRow{a.key, r.pattern, r.context, a.test, a.msg})
		}
	}
	return out
}

// fxDMFromArtefact re-derives the same rows from the Schematron, using the same
// definition of "data model" the generator uses: an assertion with no [ID] prefix
// on its message and a message to report. The three CEN-minted binding assertions
// MINIMUM carries with an empty message are excluded by XPath, the way
// facturx.go's facturXBinding identifies them.
func fxDMFromArtefact(t *testing.T, dir string, p Profile) []fxDMRow {
	t.Helper()
	binding := map[string]bool{}
	for id, x := range fxNamed(fxDecode(t, dir, ProfileBasicWL)) {
		if strings.HasPrefix(id, "CII-") {
			binding[normalizeSpace(x.a.Test)] = true
		}
	}
	var out []fxDMRow
	n := 0
	for _, x := range fxAssertions(fxDecode(t, dir, p)) {
		if x.a.identifier() != "" {
			continue
		}
		msg := normalizeSpace(x.a.Message)
		if msg == "" {
			if !binding[normalizeSpace(x.a.Test)] {
				t.Errorf("%s carries an assertion at %s with neither an [ID] prefix nor a message and a test %q "+
					"BASIC WL does not name; it can be attributed to neither the data model nor a CEN identifier",
					string(p), x.context, normalizeSpace(x.a.Test))
			}
			continue
		}
		n++
		out = append(out, fxDMRow{
			key:     fmt.Sprintf("FX-DM-%s-%04d", fxDMProfileToken[p], n),
			pattern: x.pattern,
			context: normalizeSpace(x.context),
			test:    normalizeSpace(x.a.Test),
			msg:     msg,
		})
	}
	return out
}

// TestFacturXDataModelMatchesTheArtefact is the fidelity guard: the committed
// table is exactly what the five Schematrons publish, in their order, with no row
// added, dropped or edited.
func TestFacturXDataModelMatchesTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	total := 0
	for _, p := range profiles {
		want := fxDMFromArtefact(t, dir, p)
		got := fxDMFromTable(p)
		total += len(want)
		if len(got) != len(want) {
			t.Errorf("%s: the committed table holds %d data-model assertions and %s publishes %d. "+
				"Regenerate with `make facturx-datamodel`", string(p), len(got), fxProfileFiles[p], len(want))
		}
		for i := 0; i < len(want) && i < len(got); i++ {
			if got[i] != want[i] {
				t.Errorf("%s: assertion %d differs\n  table:    %v\n  artefact: %v", string(p), i, got[i], want[i])
				if i > 3 {
					t.Fatalf("%s: stopping after the fourth difference", string(p))
				}
			}
		}
		// And the rules, including the ones that carry no assertion: under ISO
		// Schematron a rule that asserts nothing still claims its nodes away from
		// every rule below it in the same pattern, so a missing one over-reports.
		wantRules := fxDMArtefactRules(t, dir, p, want)
		var gotRules []string
		for _, r := range facturXDataModel[p] {
			gotRules = append(gotRules, fmt.Sprintf("%d %s", r.pattern, r.context))
		}
		if len(gotRules) != len(wantRules) {
			t.Errorf("%s: the committed table holds %d rules and the patterns that carry data-model assertions hold %d",
				string(p), len(gotRules), len(wantRules))
		}
		for i := 0; i < len(wantRules) && i < len(gotRules); i++ {
			if gotRules[i] != wantRules[i] {
				t.Fatalf("%s: rule %d is %q in the table and %q in the artefact", string(p), i, gotRules[i], wantRules[i])
			}
		}
		t.Logf("Factur-X %s data model: %d assertions over %d rules", string(p), len(want), len(gotRules))
	}
	if total != fxDMPublishedTotal {
		t.Errorf("the five Schematrons publish %d data-model assertions and this suite is written for %d", total, fxDMPublishedTotal)
	}
}

// fxDMArtefactRules is every rule of every pattern that carries at least one
// data-model assertion, keyed the way the table keys them.
func fxDMArtefactRules(t *testing.T, dir string, p Profile, rows []fxDMRow) []string {
	t.Helper()
	carries := map[int]bool{}
	for _, r := range rows {
		carries[r.pattern] = true
	}
	var out []string
	s := fxDecode(t, dir, p)
	for pi, pat := range s.Patterns {
		if !carries[pi] {
			continue
		}
		for _, r := range pat.Rules {
			out = append(out, fmt.Sprintf("%d %s", pi, normalizeSpace(r.Context)))
		}
	}
	return out
}

// fxDMPublishedTotal is the number of data-model assertions the five Schematrons
// publish between them. It is a claim about the artefact rather than about this
// package, which is why it is asserted rather than computed.
const fxDMPublishedTotal = 2159

// ---------------------------------------------------------------------------
// The round trip
// ---------------------------------------------------------------------------

// fxDMCanonical renders an XPath expression as its token stream, with element
// prefixes stripped, so that two spellings of the same expression compare equal
// and two different expressions never do.
//
// The stripping is what lets a table of local names be compared against an
// artefact of qualified ones. TestFacturXDataModelNamesAreUnambiguous is what
// makes it sound: no two qualified names anywhere in these five files share a
// local name, so nothing is being conflated.
func fxDMCanonical(s string) string {
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'' || c == '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return "\x00unterminated literal: " + s
			}
			toks = append(toks, "'"+s[i+1:i+1+j]+"'")
			i += j + 2
		case c == '.' && i+1 < len(s) && s[i+1] == '.':
			toks = append(toks, "..")
			i += 2
		case fxDMIsNameStart(c):
			j := i
			for j < len(s) && fxDMIsNameChar(s[j]) {
				j++
			}
			name := s[i:j]
			if k := strings.IndexByte(name, ':'); k >= 0 {
				name = name[k+1:]
			}
			toks = append(toks, name)
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		default:
			if (c == '<' || c == '>') && i+1 < len(s) && s[i+1] == '=' {
				toks = append(toks, s[i:i+2])
				i += 2
				continue
			}
			toks = append(toks, string(c))
			i++
		}
	}
	return strings.Join(toks, " ")
}

func fxDMIsNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func fxDMIsNameChar(c byte) bool {
	return fxDMIsNameStart(c) || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == ':'
}

// fxDMRenderPath, fxDMRenderPred and fxDMRenderStep are the inverse of the
// decomposition internal/gen/facturx performs. They are written here, from the
// struct definitions, rather than shared with the generator: a renderer the
// generator also used would be checking the generator against itself.
func fxDMRenderPath(p fxDMPath) string {
	var b strings.Builder
	if p.abs {
		for _, s := range p.steps {
			b.WriteByte('/')
			b.WriteString(s)
		}
	} else {
		for i := 0; i < p.up; i++ {
			if i > 0 {
				b.WriteByte('/')
			}
			b.WriteString("..")
		}
		for i, s := range p.steps {
			if i > 0 || p.up > 0 {
				b.WriteByte('/')
			}
			b.WriteString(s)
		}
	}
	if p.attr != "" {
		if p.abs || p.up > 0 || len(p.steps) > 0 {
			b.WriteByte('/')
		}
		b.WriteByte('@')
		b.WriteString(p.attr)
	}
	return b.String()
}

func fxDMRenderPred(p fxDMPred) string {
	if len(p.terms) == 0 {
		return ""
	}
	parts := make([]string, len(p.terms))
	for i, t := range p.terms {
		inner := fxDMRenderPath(t.left)
		if t.eq {
			if t.isLit {
				inner += "=\"" + t.lit + "\""
			} else {
				inner += "=" + fxDMRenderPath(t.right)
			}
		}
		for n := 0; n < t.negs; n++ {
			inner = "not(" + inner + ")"
		}
		parts[i] = inner
	}
	return "[" + strings.Join(parts, " and ") + "]"
}

func fxDMRenderStep(s fxDMStep) string { return s.name + fxDMRenderPred(s.pred) }

// TestFacturXDataModelRoundTripsToItsXPath re-renders every decomposition in the
// committed table and compares it against the XPath the table holds beside it.
//
// This is the guard the previous generated tier did not have. A table can hold
// the authority's own expression as a string, pass a fidelity test that compares
// strings, and be evaluated from a decomposition that lost a predicate or points
// at the wrong node — which is C41 exactly. The comparison is on the token
// stream, because the artefact's whitespace and quoting are incidental
// (`[ not(ram:ID/@schemeID="VA") and  not(...)]`) and its meaning is not.
func TestFacturXDataModelRoundTripsToItsXPath(t *testing.T) {
	checked := 0
	for _, p := range profiles {
		for _, r := range facturXDataModel[p] {
			var b strings.Builder
			for _, s := range r.steps {
				b.WriteByte('/')
				b.WriteString(fxDMRenderStep(s))
			}
			if got, want := fxDMCanonical(b.String()), fxDMCanonical(r.context); got != want {
				t.Errorf("%s: the context %q decomposes to %q, which is a different expression", string(p), want, got)
			}
			checked++
			for _, a := range r.asserts {
				var rendered string
				switch a.op {
				case fxDMCountEQ:
					rendered = "count(" + fxDMRenderStep(a.child) + ")=" + fmt.Sprint(a.bound)
				case fxDMCountLE:
					rendered = "count(" + fxDMRenderStep(a.child) + ")<=" + fmt.Sprint(a.bound)
				case fxDMCountGE:
					rendered = "count(" + fxDMRenderStep(a.child) + ")>=" + fmt.Sprint(a.bound)
				case fxDMUnused:
					rendered = "true()"
				case fxDMAttrRequired, fxDMAttrForbidden:
					rendered = "@" + a.attr
				case fxDMCode:
					// The value path is the <let> binding rather than the test, so
					// what is checked here is that the lookup reads the element or
					// the attribute the test names. The rest of the expression is
					// fixed by the shape and is checked by the fidelity test.
					v := fxDMRenderPath(a.value)
					if v == "" {
						v = "."
					}
					if !strings.Contains(a.test, fmt.Sprintf("cl[@id=%d]", a.clID)) {
						t.Errorf("%s: %s looks up code list id %d and its test reads %q", string(p), a.key, a.clID, a.test)
					}
					if a.value.attr != "" && !strings.Contains(a.msg, "'@"+a.value.attr+"'") {
						t.Errorf("%s: %s reads @%s and FNFE's message names something else: %q", string(p), a.key, a.value.attr, a.msg)
					}
					checked++
					continue
				default:
					t.Fatalf("%s: %s carries an op no renderer here knows", string(p), a.key)
				}
				if got, want := fxDMCanonical(rendered), fxDMCanonical(a.test); got != want {
					t.Errorf("%s: %s decomposes to %q and its test is %q", string(p), a.key, got, want)
				}
				checked++
			}
		}
	}
	t.Logf("re-rendered %d Factur-X data-model contexts and assertions from their decompositions", checked)
}

// TestFacturXDataModelPathsAreUnambiguous is what licenses the table to hold
// local names where the artefact writes qualified ones.
//
// parseCII keys the tree on local names, so `udt:DateTimeString` and
// `qdt:DateTimeString` are one element to this package — and these files write
// both. The question is therefore not whether a local name is ever ambiguous
// (it is) but whether it is ambiguous *at a path*: the artefact writes
// `ram:IssueDateTime/udt:DateTimeString` and
// `ram:FormattedIssueDateTime/qdt:DateTimeString`, and the parent step already
// separates them, so reducing to local names conflates nothing.
//
// This asserts that, over every rule context in all five files: the map from a
// context's local-name path to its qualified path is single-valued. A revision
// that put two differently-namespaced elements at the same path would fail here
// rather than silently make one rule claim the other's nodes.
func TestFacturXDataModelPathsAreUnambiguous(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	byLocal := map[string]string{}
	prefixes := map[string]bool{}
	for _, p := range profiles {
		for _, pat := range fxDecode(t, dir, p).Patterns {
			for _, r := range pat.Rules {
				q := fxDMStripPredicates(normalizeSpace(r.Context))
				var local []string
				for _, s := range strings.Split(strings.TrimPrefix(q, "/"), "/") {
					if i := strings.IndexByte(s, ':'); i >= 0 {
						prefixes[s[:i]] = true
						s = s[i+1:]
					}
					local = append(local, s)
					// Every proper prefix of a context is itself a path some other
					// context may end at, so the map is keyed on prefixes too:
					// `A/ram:B` and `A/qdt:B/C` have to collide even though neither
					// context is the other.
					lp := strings.Join(local, "/")
					qp := fxDMPrefixOf(q, len(local))
					if prev, ok := byLocal[lp]; ok && prev != qp {
						t.Errorf("the path %q is written both %q and %q; the data-model table holds local names "+
							"because parseCII does, and these two would be one element to it", lp, prev, qp)
						continue
					}
					byLocal[lp] = qp
				}
			}
		}
	}
	t.Logf("checked %d distinct element paths over the five Schematrons, using the prefixes %v", len(byLocal), fxDMSortedSet(prefixes))
	atLeast(t, "Factur-X context paths", len(byLocal), minFacturXContextPaths)
}

// fxDMStripPredicates removes the predicates from a context, leaving the step
// names. It is the same reduction fxStripPredicates in facturx_test.go performs
// and is repeated here rather than shared because that one is about rule order.
func fxDMStripPredicates(ctx string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(ctx); i++ {
		switch ctx[i] {
		case '[':
			depth++
		case ']':
			depth--
		default:
			if depth == 0 {
				b.WriteByte(ctx[i])
			}
		}
	}
	return b.String()
}

// fxDMPrefixOf returns the first n steps of a qualified path.
func fxDMPrefixOf(q string, n int) string {
	steps := strings.Split(strings.TrimPrefix(q, "/"), "/")
	if n > len(steps) {
		n = len(steps)
	}
	return "/" + strings.Join(steps[:n], "/")
}

func fxDMSortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// The code lists
// ---------------------------------------------------------------------------

// TestFacturXCodeListsMatchTheArtefact checks every code-list assertion against
// the code database FNFE ships beside the Schematron: the list the table points
// it at holds exactly the values that profile's file declares under that cl id.
//
// The lists are deduplicated in the table — 44 distinct lists across 140 declared
// — so this is also the guard on the deduplication: two lists interned as one
// when they differ would fail here for one of the two.
func TestFacturXCodeListsMatchTheArtefact(t *testing.T) {
	dir := fxSchematronDir()
	if dir == "" {
		t.Skip("Factur-X Schematrons not present; run `make facturx-schematron`")
	}
	checked, values := 0, 0
	for _, p := range profiles {
		db := fxCodeDB(t, dir, p)
		for _, r := range facturXDataModel[p] {
			for _, a := range r.asserts {
				if a.op != fxDMCode {
					continue
				}
				want, ok := db[a.clID]
				if !ok {
					t.Errorf("%s: %s reads cl[@id=%d], which %s does not declare", string(p), a.key, a.clID, fxCodeDBFile(p))
					continue
				}
				got := facturXCodeLists[a.list]
				if !fxDMSameSet(got, want) {
					t.Errorf("%s: %s reads a %d-value list and cl[@id=%d] in %s declares %d values",
						string(p), a.key, len(got), a.clID, fxCodeDBFile(p), len(want))
					continue
				}
				checked++
				values += len(got)
			}
		}
	}
	if checked != fxDMPublishedCodeAssertions {
		t.Errorf("checked %d code-list assertions against the code databases, and the five Schematrons carry %d",
			checked, fxDMPublishedCodeAssertions)
	}
	t.Logf("checked %d Factur-X code-list assertions against %d declared code values", checked, values)
}

// fxDMPublishedCodeAssertions is how many of the 2,159 data-model assertions look
// a value up in a code database. PR 57 recorded these as unimplementable because
// the code databases were not separately fetchable; they are, from the same
// mustangproject directory the .sch come from, which is what closes them.
const fxDMPublishedCodeAssertions = 366

func fxCodeDBFile(p Profile) string {
	return strings.TrimSuffix(fxProfileFiles[p], ".sch") + "_codedb.xml"
}

// fxCodeDB reads one profile's code database, the file FNFE's own assertions call
// document() on. `make facturx-schematron` fetches it from the same
// ZUGFeRD/mustangproject directory as the .sch, which is what makes the 366
// code-list assertions evaluable at all.
func fxCodeDB(t *testing.T, dir string, p Profile) map[int][]string {
	t.Helper()
	path := filepath.Join(dir, fxCodeDBFile(p))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run `make facturx-schematron`", err)
	}
	var db struct {
		Lists []struct {
			ID      int `xml:"id,attr"`
			Entries []struct {
				Value string `xml:"value,attr"`
			} `xml:"enumeration"`
		} `xml:"cl"`
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = latin1Reader
	if err := dec.Decode(&db); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(db.Lists) < minFacturXCodeDBLists[p] {
		t.Fatalf("%s decoded to %d code lists, want at least %d; the file is short or the decoder is reading the wrong elements",
			path, len(db.Lists), minFacturXCodeDBLists[p])
	}
	out := map[int][]string{}
	for _, cl := range db.Lists {
		vals := make([]string, len(cl.Entries))
		for i, e := range cl.Entries {
			vals[i] = e.Value
		}
		out[cl.ID] = vals
	}
	return out
}

func fxDMSameSet(got, want []string) bool {
	if len(got) == 0 || len(want) == 0 {
		return false
	}
	w := append([]string(nil), want...)
	sort.Strings(w)
	out := w[:0]
	for i, v := range w {
		if i == 0 || v != w[i-1] {
			out = append(out, v)
		}
	}
	w = out
	if len(w) != len(got) {
		return false
	}
	for i := range w {
		if w[i] != got[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// The ratchets
// ---------------------------------------------------------------------------

// TestFacturXDataModelIsRatcheted holds the committed table to a floor without
// reading the artefact, so that a checkout with no Schematrons — CI's own
// corpus-less job, and anybody who has not run `make facturx-schematron` — still
// fails on a table somebody emptied. Every other guard here skips without the
// artefact, which is precisely the condition C8 and C26 describe.
func TestFacturXDataModelIsRatcheted(t *testing.T) {
	total, code := 0, 0
	for _, p := range profiles {
		n := 0
		for _, r := range facturXDataModel[p] {
			for _, a := range r.asserts {
				n++
				if a.op == fxDMCode {
					code++
				}
			}
		}
		atLeast(t, "Factur-X "+string(p)+" data-model assertions", n, minFacturXDataModel[p])
		total += n
	}
	if total != fxDMPublishedTotal {
		t.Errorf("the committed table holds %d data-model assertions and the artefact publishes %d", total, fxDMPublishedTotal)
	}
	if code != fxDMPublishedCodeAssertions {
		t.Errorf("the committed table holds %d code-list assertions and the artefact publishes %d", code, fxDMPublishedCodeAssertions)
	}
	atLeast(t, "Factur-X code lists", len(facturXCodeLists), minFacturXCodeLists)
	values := 0
	for _, l := range facturXCodeLists {
		values += len(l)
		if !sort.StringsAreSorted(l) {
			t.Errorf("a code list is not sorted; fxDMInList binary-searches them")
		}
	}
	atLeast(t, "Factur-X code values", values, minFacturXCodeValues)
	t.Logf("Factur-X data model: %d assertions, %d of them code lookups, over %d lists holding %d values",
		total, code, len(facturXCodeLists), values)
}

// ---------------------------------------------------------------------------
// The firing verdict
// ---------------------------------------------------------------------------

// fxSynth is a synthetic CII document under construction: the tree the firing
// guard builds from one table row.
type fxSynth struct {
	name  string
	attrs map[string]string
	text  string
	kids  []*fxSynth
	// protected marks the node the assertion under test is about, so that
	// satisfying an *ancestor's* predicate writes into a fresh sibling instead of
	// undoing the very mutation the document exists to make.
	//
	// This is not a convenience. Half of these contexts put the predicate on the
	// parent — `ram:SpecifiedTaxRegistration[ram:ID/@schemeID="VA"]/ram:ID` — where
	// the predicate is a general comparison over *all* the ID children and the
	// assertion is about each of them separately. A builder that satisfies the
	// predicate by writing into the context node itself can never break the
	// assertion, and would conclude that 23 perfectly reportable rules are inert.
	protected bool
}

func (n *fxSynth) child(name string) *fxSynth {
	for _, k := range n.kids {
		if k.name == name && !k.protected {
			return k
		}
	}
	k := &fxSynth{name: name}
	n.kids = append(n.kids, k)
	return k
}

func (n *fxSynth) add(name string) *fxSynth {
	k := &fxSynth{name: name}
	n.kids = append(n.kids, k)
	return k
}

func (n *fxSynth) setAttr(name, value string) {
	if n.attrs == nil {
		n.attrs = map[string]string{}
	}
	n.attrs[name] = value
}

// xml renders the tree. The prefixes are dropped along with the namespace
// declarations: parseCII keys on local names, and a synthetic document that
// declared namespaces would be testing encoding/xml rather than the table.
func (n *fxSynth) xml(b *strings.Builder) {
	b.WriteByte('<')
	b.WriteString(n.name)
	for _, k := range fxSortedKeys(n.attrs) {
		b.WriteString(" " + k + "=\"" + fxEscape(n.attrs[k]) + "\"")
	}
	b.WriteByte('>')
	b.WriteString(fxEscape(n.text))
	for _, k := range n.kids {
		k.xml(b)
	}
	b.WriteString("</" + n.name + ">")
}

func fxSortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fxEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

// fxSynthValue is the filler this builder writes wherever the artefact does not
// constrain a value. It is deliberately not a plausible code, so that a code-list
// assertion this builder did not mean to break cannot pass by accident.
const fxSynthValue = "SYNTHETIC"

// fxDMBuild builds a document that reaches the rule's context and either
// satisfies (violate == false) or breaks (violate == true) the assertion.
//
// The order is the whole trick, and it is three steps:
//
//  1. write the assertion's own condition at the context node;
//  2. satisfy that node's *own* predicate, adapting to what step 1 wrote — which
//     is what makes `ram:TaxTotalAmount[@currencyID=../../ram:InvoiceCurrencyCode]`
//     work, where the predicate has to agree with the very attribute the
//     code-list assertion is about;
//  3. satisfy the *ancestors'* predicates with the context node protected, so
//     that a predicate over a sibling set — `[ram:ID/@schemeID="VA"]` above a
//     rule whose context is `.../ram:ID` — is satisfied by a fresh sibling rather
//     than by repairing the node under test.
func fxDMBuild(r fxDMRule, a fxDMAssert, violate bool) *fxSynth {
	root := &fxSynth{name: r.steps[0].name}
	chain := []*fxSynth{root}
	for _, s := range r.steps[1:] {
		chain = append(chain, chain[len(chain)-1].child(s.name))
	}
	leaf := chain[len(chain)-1]

	switch a.op {
	case fxDMCountEQ, fxDMCountLE, fxDMCountGE:
		want := a.bound
		if violate {
			if a.op == fxDMCountGE {
				want = a.bound - 1
			} else {
				want = a.bound + 1
			}
		}
		for i := 0; i < want; i++ {
			c := leaf.add(a.child.name)
			c.text = fxSynthValue
			fxDMSatisfy(a.child.pred, append(chain, c))
		}
	case fxDMUnused:
		// The context node existing is the finding, so the satisfying document is
		// the one without it. Removing the leaf from its parent is what "this
		// element is not used in this context" means.
		if !violate && len(chain) > 1 {
			parent := chain[len(chain)-2]
			for i, k := range parent.kids {
				if k == leaf {
					parent.kids = append(parent.kids[:i], parent.kids[i+1:]...)
					break
				}
			}
			chain = chain[:len(chain)-1]
		}
	case fxDMAttrRequired:
		if !violate {
			leaf.setAttr(a.attr, fxSynthValue)
		}
	case fxDMAttrForbidden:
		if violate {
			leaf.setAttr(a.attr, fxSynthValue)
		}
	case fxDMCode:
		v := "§NOT-A-CODE"
		if !violate {
			v = facturXCodeLists[a.list][0]
		}
		if a.value.attr != "" {
			leaf.setAttr(a.value.attr, v)
		} else {
			leaf.text = v
		}
	}

	if len(chain) == len(r.steps) {
		fxDMSatisfy(r.steps[len(r.steps)-1].pred, chain)
		leaf.protected = true
	}
	for i := 0; i+1 < len(r.steps) && i+1 < len(chain); i++ {
		fxDMSatisfy(r.steps[i].pred, chain[:i+1])
	}
	leaf.protected = false
	return root
}

// fxDMSatisfy writes whatever the predicate needs to hold at the node at the end
// of chain. A negated term is satisfied by leaving its path absent, which is the
// state the builder starts in; only the positive ones write anything.
func fxDMSatisfy(p fxDMPred, chain []*fxSynth) {
	for _, t := range p.terms {
		if t.negs&1 == 1 {
			continue
		}
		switch {
		case !t.eq:
			fxDMPut(t.left, chain, fxSynthValue)
		case t.isLit:
			fxDMPut(t.left, chain, t.lit)
		default:
			// A comparison between two node sets. If the left side already holds a
			// value — which is the case when the assertion under test wrote it —
			// the right side is made to agree with it rather than the other way
			// round, so that breaking the assertion does not also break the
			// context that reaches it.
			v, ok := fxDMPeek(t.left, chain)
			if !ok || v == "" {
				v = fxSynthValue
				fxDMPut(t.left, chain, v)
			}
			fxDMPut(t.right, chain, v)
		}
	}
}

// fxDMResolve walks a path, creating what is missing, and returns the node it
// ends at. It returns nil for a path that reaches above the document element.
func fxDMResolve(p fxDMPath, chain []*fxSynth) *fxSynth {
	var cur *fxSynth
	steps := p.steps
	switch {
	case p.abs:
		cur = chain[0]
		if len(steps) == 0 || steps[0] != cur.name {
			return nil
		}
		steps = steps[1:]
	default:
		i := len(chain) - 1 - p.up
		if i < 0 {
			return nil
		}
		cur = chain[i]
	}
	for _, s := range steps {
		cur = cur.child(s)
	}
	return cur
}

func fxDMPut(p fxDMPath, chain []*fxSynth, v string) {
	n := fxDMResolve(p, chain)
	if n == nil {
		return
	}
	if p.attr != "" {
		n.setAttr(p.attr, v)
		return
	}
	n.text = v
}

// fxDMPeek reads the value a path already holds, without creating anything.
func fxDMPeek(p fxDMPath, chain []*fxSynth) (string, bool) {
	var cur *fxSynth
	steps := p.steps
	switch {
	case p.abs:
		cur = chain[0]
		if len(steps) == 0 || steps[0] != cur.name {
			return "", false
		}
		steps = steps[1:]
	default:
		i := len(chain) - 1 - p.up
		if i < 0 {
			return "", false
		}
		cur = chain[i]
	}
	for _, s := range steps {
		var next *fxSynth
		for _, k := range cur.kids {
			if k.name == s {
				next = k
				break
			}
		}
		if next == nil {
			return "", false
		}
		cur = next
	}
	if p.attr != "" {
		v, ok := cur.attrs[p.attr]
		return v, ok
	}
	return cur.text, true
}

// TestEveryFacturXDataModelAssertionFires is the answer to C41, and the reason
// this tier could be added at all.
//
// C41 was a generated evaluator in which one whole rule shape evaluated against
// the wrong node and therefore passed for every document. The guards in place at
// the time — "every published identifier has an implementation", "every context is
// reached" — both passed on it. What caught it, one PR later, was a per-rule
// firing verdict.
//
// 2,159 assertions cannot be given 2,159 hand-written fixtures, so the fixtures
// are generated from the table itself: for each assertion, two synthetic
// documents built by walking its own context and its own condition, one written
// to satisfy it and one written to break it. The evaluator must be silent on the
// first and must name that assertion on the second. That is strictly stronger
// than a hand-written fixture per rule in one respect — it covers every rule, not
// the ones somebody thought to write — and weaker in another, which is worth
// stating: the documents are built from the same decomposition the evaluator
// reads, so a decomposition that is wrong in the *same way* on both sides would
// satisfy it. That is what the round-trip test above closes, by comparing the
// decomposition against FNFE's own XPath rather than against itself.
func TestEveryFacturXDataModelAssertionFires(t *testing.T) {
	ctx := context.Background()
	fired, inert, noisy, contradictory := 0, 0, 0, 0
	for _, p := range profiles {
		for _, r := range facturXDataModel[p] {
			for _, a := range r.asserts {
				if fxDMContradictory(r, a) {
					// The assertion's condition and its own context predicate are
					// the same test at opposite polarities, so no document can
					// reach it and break it. It is not this package skipping a
					// rule; it is a rule FNFE published that no processor can
					// report, and it is recorded that way in
					// Coverage(SourceFacturX). The firing verdict is inverted for
					// these: they must *not* report, or the contradiction is not
					// what this test thinks it is.
					contradictory++
					var b strings.Builder
					fxDMBuild(r, a, true).xml(&b)
					if fxDMReports(t, ctx, b.String(), p, a.key) {
						t.Errorf("%s: %s is derived to be unreportable — its condition %q contradicts its context %q — "+
							"and it reported", string(p), a.key, a.test, r.context)
					}
					continue
				}
				var b strings.Builder
				fxDMBuild(r, a, true).xml(&b)
				if !fxDMReports(t, ctx, b.String(), p, a.key) {
					inert++
					if inert <= 5 {
						t.Errorf("%s: %s does not report on a document built to break it. An assertion that is present, "+
							"reachable and inert is C41.\n  test:    %s\n  context: %s\n  document: %s",
							string(p), a.key, a.test, r.context, b.String())
					}
					continue
				}
				fired++
				var c strings.Builder
				fxDMBuild(r, a, false).xml(&c)
				if fxDMReports(t, ctx, c.String(), p, a.key) {
					noisy++
					if noisy <= 5 {
						t.Errorf("%s: %s reports on a document built to satisfy it, so it does not depend on the "+
							"condition it names.\n  test:    %s\n  context: %s\n  document: %s",
							string(p), a.key, a.test, r.context, c.String())
					}
				}
			}
		}
	}
	if contradictory != fxDMUnreportableAssertions {
		t.Errorf("derived %d unreportable data-model assertions and Coverage(SourceFacturX) records %d",
			contradictory, fxDMUnreportableAssertions)
	}
	if fired+contradictory != fxDMPublishedTotal {
		t.Errorf("%d of %d data-model assertions were made to report and %d are unreportable by construction; "+
			"%d stayed silent on a document built to break them",
			fired, fxDMPublishedTotal, contradictory, inert)
	}
	t.Logf("Factur-X data model: %d/%d assertions report on a document built to break them and stay silent on one "+
		"built to satisfy them; %d cannot be reported by any processor",
		fired, fxDMPublishedTotal, contradictory)
}

// fxDMUnreportableAssertions is how many data-model assertions FNFE publishes
// that no processor can report, derived by fxDMContradictory and recorded in
// Coverage(SourceFacturX) as an unevaluable family.
const fxDMUnreportableAssertions = 3

// fxDMContradictory reports whether an assertion's condition is the negation of
// its own context predicate, which makes it unreportable: the nodes that would
// break it are exactly the nodes the rule does not claim.
//
// The three in these files are `report @listID` on the context
// `ram:ReasonCode[not (@listID)]` — an attribute FNFE forbids on a node it
// selects by that attribute being absent. This is the same kind of fact as CEN
// binding BR-CO-05..08 to true(), and D10 is why it is recorded rather than
// counted as an implementation gap.
//
// It is derived syntactically rather than listed, because a list is a claim that
// stops being checked. The derivation is deliberately narrow: an existence test
// on one attribute, at the context node itself, at the opposite polarity to what
// the assertion needs. Anything subtler is not recognised, and would show up as
// an inert rule above — which is the failure this test exists to produce.
func fxDMContradictory(r fxDMRule, a fxDMAssert) bool {
	var want int // the parity of not() wrappers that contradicts the assertion
	switch a.op {
	case fxDMAttrForbidden:
		// Fires when the attribute is present; contradicted by not(@a).
		want = 1
	case fxDMAttrRequired:
		// Fires when the attribute is absent; contradicted by @a.
		want = 0
	default:
		return false
	}
	for _, t := range r.steps[len(r.steps)-1].pred.terms {
		if t.eq || len(t.left.steps) > 0 || t.left.abs || t.left.up > 0 {
			continue
		}
		if t.left.attr == a.attr && t.negs&1 == want {
			return true
		}
	}
	return false
}

// fxDMReports runs the data model alone — not Validate — over one synthetic
// document and reports whether the named assertion fired. Running the data model
// alone is what makes the verdict about this table rather than about the sixty
// other rules a full validation would apply to a document this small.
func fxDMReports(t *testing.T, ctx context.Context, doc string, p Profile, key string) bool {
	t.Helper()
	r := newRun(ctx)
	root, err := parseCII(r, []byte(doc))
	if err != nil {
		t.Fatalf("the synthetic document for %s is not well-formed: %v\n%s", key, err, doc)
	}
	for _, v := range facturXDataModelRules(r, root, p) {
		if v.Rule == key {
			return true
		}
	}
	return false
}
