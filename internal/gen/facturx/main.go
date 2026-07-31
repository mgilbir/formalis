// Command facturx generates facturx_datamodel_table.go from the five Factur-X
// profile Schematrons and the code databases FNFE ships beside them.
//
// Run `make facturx-schematron` first: it vendors the ten files this reads into
// the gitignored testdata/facturx/schematron/. Then `make facturx-datamodel`, or
// `go run ./internal/gen/facturx`, rewrites the table.
//
// # What this emits, and why it is generated
//
// Each Factur-X profile Schematron carries, beside its named business rules, a
// generated per-profile *element-table data model*: one assertion per element of
// that profile's element table, 48 of them in MINIMUM and 1,241 in EXTENDED,
// 2,159 in all. None of them carries an identifier — FNFE names a rule by an
// "[ID]-" prefix on its message text and these have none — and between them they
// decide exactly the questions CEN's CII-SR-*/CII-DT-* binding decides, with
// per-profile answers. They are the binding Factur-X publishes in place of CEN's,
// and issue #56 is what applying CEN's to a Factur-X document costs.
//
// A tier that size does not need judgement, it needs transcribing without error,
// and a mistyped element name in a hand-written transcription is invisible: the
// rule simply never fires. So it is generated, the way CEN's 1,168 advisory
// binding rules and AT/eSPap's 291 datatype rules already are.
//
// # Where this generator lives, and why it is tracked
//
// The five existing generators in this repository sit under testdata/*-rules/ and
// are all tracked in git — testdata/ is gitignored for the *artefacts* fetched
// into it, never for the generators. This one is a Go program under internal/
// instead, for one reason: it is the largest generated tier in the package and
// `go vet ./...` and `gofmt -l .` reach it there, so the program that decides
// what 2,159 fatal assertions mean is held to the same standard as the code it
// writes.
//
// # Failing loudly
//
// Three things could be silently dropped here, and each aborts the run instead:
//
//   - an assertion whose shape is outside the six this table can express. classify
//     below returns an error naming the profile, the rule context and the XPath,
//     and main exits non-zero. There is no "skip what we do not understand" path.
//
//   - a context or a count() argument whose XPath is outside the subset xpath.go
//     parses. Same treatment, and additionally every decomposition is rendered
//     back to XPath and compared, token for token, against the artefact's own
//     string before it is emitted: a parse that lost a predicate or invented a
//     step cannot reach the table even if it parsed cleanly.
//
//   - an assertion carrying an empty message. Three exist, all in MINIMUM, and
//     all three are CEN-minted binding assertions facturx.go already evaluates
//     under their CII-SR-* identifiers. The generator identifies them by XPath
//     against BASIC WL's named copies and aborts if it finds a fourth, because an
//     unnamed, unmessaged assertion is otherwise indistinguishable from a data
//     model row.
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// The artefact
// ---------------------------------------------------------------------------

// profile is one of the five Factur-X tiers: the Go Profile constant to key the
// emitted table by, the Schematron's file name, and the token that goes into the
// synthetic rule identifiers.
type profile struct {
	goConst string
	file    string
	token   string
	varName string
}

var profiles = []profile{
	{"ProfileMinimum", "FACTUR-X_MINIMUM.sch", "MINIMUM", "fxDMMinimum"},
	{"ProfileBasicWL", "FACTUR-X_BASIC-WL.sch", "BASICWL", "fxDMBasicWL"},
	{"ProfileBasic", "FACTUR-X_BASIC.sch", "BASIC", "fxDMBasic"},
	{"ProfileEN16931", "FACTUR-X_EN16931.sch", "EN16931", "fxDMEN16931"},
	{"ProfileExtended", "FACTUR-X_EXTENDED.sch", "EXTENDED", "fxDMExtended"},
}

// codedbFile is the name of the code database a profile's assertions call
// document() on. It is derived from the .sch name rather than hard-coded, and the
// generator checks every document() call against it.
func (p profile) codedbFile() string {
	return strings.TrimSuffix(p.file, ".sch") + "_codedb.xml"
}

type schema struct {
	Patterns []pattern `xml:"pattern"`
}

type pattern struct {
	Rules []rule `xml:"rule"`
}

type rule struct {
	Context string  `xml:"context,attr"`
	Body    []child `xml:",any"`
}

// child is an element of a <rule>: an <assert>, a <report> or a <let>. They share
// one type because document order matters and because the assert/report
// distinction is a polarity the classification depends on.
type child struct {
	XMLName xml.Name
	Test    string `xml:"test,attr"`
	Flag    string `xml:"flag,attr"`
	Name    string `xml:"name,attr"`
	Value   string `xml:"value,attr"`
	Message string `xml:",chardata"`
}

func (c child) isAssertion() bool { return c.XMLName.Local == "assert" || c.XMLName.Local == "report" }

type codedb struct {
	Lists []codeList `xml:"cl"`
}

type codeList struct {
	ID      int          `xml:"id,attr"`
	Entries []codeString `xml:"enumeration"`
}

type codeString struct {
	Value string `xml:"value,attr"`
}

// idPrefix is how FNFE names a rule. It is the same expression facturx_test.go's
// fxIDPrefix uses, and the two are the definition of "named" on both sides.
var idPrefix = regexp.MustCompile(`^\s*\[([A-Za-z0-9_.-]+)\]`)

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// decode reads one XML file, tolerating the ISO-8859-1 some of CEN's artefacts
// declare. FNFE's are UTF-8, and the reader is here so that a re-encoded copy
// cannot make this generator silently read an empty document — which is exactly
// what Go's decoder does for a declared charset it has no reader for.
func decode(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "iso-8859-1", "latin1", "windows-1252":
			return &latin1{r: input}, nil
		}
		return nil, fmt.Errorf("%s: unsupported charset %q", path, charset)
	}
	return dec.Decode(v)
}

type latin1 struct {
	r   io.Reader
	buf []byte
}

func (l *latin1) Read(p []byte) (int, error) {
	if len(l.buf) == 0 {
		src := make([]byte, len(p))
		n, err := l.r.Read(src)
		if n == 0 {
			return 0, err
		}
		for _, b := range src[:n] {
			if b < 0x80 {
				l.buf = append(l.buf, b)
			} else {
				l.buf = append(l.buf, 0xc0|b>>6, 0x80|b&0x3f)
			}
		}
	}
	n := copy(p, l.buf)
	l.buf = l.buf[n:]
	return n, nil
}

// ---------------------------------------------------------------------------
// The shapes
// ---------------------------------------------------------------------------

// op mirrors the fxDMOp constants in facturx_datamodel.go. The Go names are
// emitted, not the numbers, so a reordering there is a compile error here rather
// than a silent renumbering of 2,159 assertions.
type op string

const (
	opCountEQ       op = "fxDMCountEQ"
	opCountLE       op = "fxDMCountLE"
	opCountGE       op = "fxDMCountGE"
	opUnused        op = "fxDMUnused"
	opAttrRequired  op = "fxDMAttrRequired"
	opAttrForbidden op = "fxDMAttrForbidden"
	opCode          op = "fxDMCode"
)

// assertion is one emitted data-model assertion.
type assertion struct {
	key   string
	op    op
	test  string // FNFE's own XPath, whitespace-normalised
	msg   string // FNFE's own message, whitespace-normalised
	child step   // count() only
	bound int    // count() only
	attr  string // the attribute ops
	value path   // code lists: the <let> value, empty for the context item
	list  int    // code lists: index into the deduplicated table
	clID  int    // code lists: FNFE's cl/@id, kept so the table reads against the file
}

// emittedRule is one rule of a pattern that carries at least one data-model
// assertion. Rules with no data-model assertion in such a pattern are emitted
// too, with an empty asserts list: under ISO Schematron a node goes to the first
// matching rule of a pattern, so a rule that asserts nothing can still claim a
// node away from one below it, and dropping it would over-report. (In these five
// files no such rule exists today; emitting the shape rather than the observation
// is what stops a future revision changing the answer silently.)
type emittedRule struct {
	pattern int
	context string
	steps   []step
	asserts []assertion
}

// countRE, codeRE and attrRE recognise the six shapes. They are anchored and
// whole-string: an assertion that nearly matches one is refused, not coerced.
var (
	countRE = regexp.MustCompile(`^count\((.*)\)(=|<=|>=)([0-9]+)$`)
	attrRE  = regexp.MustCompile(`^@([A-Za-z_][\w.-]*)$`)
	codeRE  = regexp.MustCompile(`^string-length\(\$([\w.-]+)\)=0 or document\('([^']+)'\)/codedb/cl\[@id=([0-9]+)\]/enumeration\[@value=\$([\w.-]+)\]$`)
)

// classify turns one assertion into an emitted row, or returns the reason it
// cannot. lets is the rule's <let> bindings, which the code-list shape resolves
// its variable through.
func classify(a child, lets map[string]string, p profile, lists *listTable, db map[int][]string) (assertion, error) {
	test := norm(a.Test)
	out := assertion{test: test, msg: norm(a.Message)}

	if a.XMLName.Local == "report" && test == "true()" {
		out.op = opUnused
		return out, nil
	}
	if m := attrRE.FindStringSubmatch(test); m != nil {
		out.attr = m[1]
		if a.XMLName.Local == "report" {
			out.op = opAttrForbidden
		} else {
			out.op = opAttrRequired
		}
		return out, nil
	}
	if m := countRE.FindStringSubmatch(test); m != nil {
		if a.XMLName.Local != "assert" {
			return out, fmt.Errorf("a cardinality shape written as <report>, whose polarity this table cannot express")
		}
		st, err := parseCountArg(m[1])
		if err != nil {
			return out, fmt.Errorf("count() argument %q: %w", m[1], err)
		}
		if got, want := canonical(renderCountArg(st)), canonical(m[1]); got != want {
			return out, fmt.Errorf("count() argument %q decomposes to %q, which is not the same expression", want, got)
		}
		out.child = st
		switch m[2] {
		case "=":
			out.op = opCountEQ
		case "<=":
			out.op = opCountLE
		case ">=":
			out.op = opCountGE
		}
		n, err := strconv.Atoi(m[3])
		if err != nil {
			return out, err
		}
		out.bound = n
		return out, nil
	}
	if m := codeRE.FindStringSubmatch(test); m != nil {
		if a.XMLName.Local != "assert" {
			return out, fmt.Errorf("a code-list shape written as <report>, whose polarity this table cannot express")
		}
		if m[1] != m[4] {
			return out, fmt.Errorf("the code-list shape tests $%s for emptiness and looks up $%s", m[1], m[4])
		}
		if m[2] != p.codedbFile() {
			return out, fmt.Errorf("document(%q): this profile's code database is %q", m[2], p.codedbFile())
		}
		raw, ok := lets[m[1]]
		if !ok {
			return out, fmt.Errorf("the code-list shape reads $%s, which its rule does not bind", m[1])
		}
		val, err := parseValuePath(raw)
		if err != nil {
			return out, fmt.Errorf("the <let> value %q: %w", raw, err)
		}
		if got, want := canonical(val.render()), canonical(raw); raw != "." && got != want {
			return out, fmt.Errorf("the <let> value %q decomposes to %q, which is not the same expression", want, got)
		}
		if len(val.steps) > 0 {
			return out, fmt.Errorf("the <let> value %q selects a descendant; this table looks up the context item or one of its attributes", raw)
		}
		id, err := strconv.Atoi(m[3])
		if err != nil {
			return out, err
		}
		vals, ok := db[id]
		if !ok {
			return out, fmt.Errorf("cl[@id=%d] is not in %s", id, p.codedbFile())
		}
		out.op = opCode
		out.value = val
		out.clID = id
		out.list = lists.intern(vals)
		return out, nil
	}
	return out, fmt.Errorf("matches none of the six data-model shapes")
}

// ---------------------------------------------------------------------------
// Code lists
// ---------------------------------------------------------------------------

// listTable deduplicates the code lists. The five databases hold 22,455
// enumerations between them and only 7,304 distinct values across 45 distinct
// lists: the country and currency lists are repeated once per profile and once
// per element that uses them.
type listTable struct {
	lists [][]string
	index map[string]int
}

func newListTable() *listTable { return &listTable{index: map[string]int{}} }

// intern returns the index of a code list, adding it if new. Values are sorted
// and deduplicated so the emitted table can be searched without building a map at
// load, and so that two lists that differ only in order are one list.
func (t *listTable) intern(vals []string) int {
	sorted := append([]string(nil), vals...)
	sort.Strings(sorted)
	out := sorted[:0]
	for i, v := range sorted {
		if i == 0 || v != sorted[i-1] {
			out = append(out, v)
		}
	}
	sorted = out
	key := strings.Join(sorted, "\x00")
	if i, ok := t.index[key]; ok {
		return i
	}
	i := len(t.lists)
	t.lists = append(t.lists, sorted)
	t.index[key] = i
	return i
}

// ---------------------------------------------------------------------------
// Reading one profile
// ---------------------------------------------------------------------------

// bindingTests are the XPath expressions of the CEN-minted binding assertions
// BASIC WL names and MINIMUM carries with an empty message. They are the one
// legitimate reason for an assertion to be unnamed and not part of the data
// model, and they are identified by expression rather than by position.
func bindingTests(dir string) (map[string]string, error) {
	var s schema
	if err := decode(filepath.Join(dir, "FACTUR-X_BASIC-WL.sch"), &s); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, pat := range s.Patterns {
		for _, r := range pat.Rules {
			for _, a := range r.Body {
				if !a.isAssertion() {
					continue
				}
				m := idPrefix.FindStringSubmatch(a.Message)
				if m != nil && strings.HasPrefix(m[1], "CII-") {
					out[norm(a.Test)] = m[1]
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("FACTUR-X_BASIC-WL.sch names no CII-* assertion; the file is short or the decoder read nothing")
	}
	return out, nil
}

// readProfile returns the emitted rules of one profile, in artefact order.
func readProfile(dir string, p profile, lists *listTable, binding map[string]string) ([]emittedRule, int, error) {
	var s schema
	if err := decode(filepath.Join(dir, p.file), &s); err != nil {
		return nil, 0, err
	}
	var cdb codedb
	if err := decode(filepath.Join(dir, p.codedbFile()), &cdb); err != nil {
		return nil, 0, err
	}
	db := map[int][]string{}
	for _, cl := range cdb.Lists {
		vals := make([]string, len(cl.Entries))
		for i, e := range cl.Entries {
			vals[i] = e.Value
		}
		db[cl.ID] = vals
	}
	if len(db) == 0 {
		return nil, 0, fmt.Errorf("%s holds no code list", p.codedbFile())
	}

	// isModel decides, for one assertion, whether it belongs to the data model.
	// Named assertions are FNFE's business rules and are implemented by hand in
	// facturx.go. An unnamed one with no message at all is a CEN-minted binding
	// assertion MINIMUM carries anonymously — and anything else with no message
	// is a shape nobody can classify, so it aborts.
	isModel := func(a child, ctx string) (bool, error) {
		if idPrefix.MatchString(a.Message) {
			return false, nil
		}
		if norm(a.Message) == "" {
			if _, ok := binding[norm(a.Test)]; ok {
				return false, nil
			}
			return false, fmt.Errorf("%s: an assertion at %s carries neither an [ID] prefix nor a message, and its test %q is not one BASIC WL names: "+
				"it can be neither classified as a data-model shape nor attributed to a CEN identifier", p.file, ctx, norm(a.Test))
		}
		return true, nil
	}

	var out []emittedRule
	n := 0
	for pi, pat := range s.Patterns {
		// Only patterns that carry data-model assertions are emitted, and from
		// those, every rule.
		carries := false
		for _, r := range pat.Rules {
			for _, a := range r.Body {
				if !a.isAssertion() {
					continue
				}
				ok, err := isModel(a, r.Context)
				if err != nil {
					return nil, 0, err
				}
				if ok {
					carries = true
				}
			}
		}
		if !carries {
			continue
		}
		for _, r := range pat.Rules {
			ctx := norm(r.Context)
			steps, err := parseContext(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("%s: rule context %q: %w", p.file, ctx, err)
			}
			if got, want := canonical(renderContext(steps)), canonical(ctx); got != want {
				return nil, 0, fmt.Errorf("%s: rule context %q decomposes to %q, which is not the same expression", p.file, want, got)
			}
			lets := map[string]string{}
			er := emittedRule{pattern: pi, context: ctx, steps: steps}
			for _, a := range r.Body {
				if a.XMLName.Local == "let" {
					lets[a.Name] = a.Value
					continue
				}
				if !a.isAssertion() {
					continue
				}
				ok, err := isModel(a, ctx)
				if err != nil {
					return nil, 0, err
				}
				if !ok {
					continue
				}
				if a.Flag != "" {
					return nil, 0, fmt.Errorf("%s: the data-model assertion %q at %s carries flag=%q. Every one of these is unflagged today and "+
						"facturx.go reads an unflagged Factur-X assertion as fatal; a flagged one is a severity this generator must not decide",
						p.file, norm(a.Test), ctx, a.Flag)
				}
				as, err := classify(a, lets, p, lists, db)
				if err != nil {
					return nil, 0, fmt.Errorf("%s: the assertion <%s test=%q> at %s %w.\n"+
						"  This is an assertion the evaluator would silently not check. Extend the shapes in\n"+
						"  internal/gen/facturx and in facturx_datamodel.go, or record it in Coverage(SourceFacturX)\n"+
						"  as an unevaluated gap. Do not drop it.",
						p.file, a.XMLName.Local, norm(a.Test), ctx, err)
				}
				n++
				as.key = fmt.Sprintf("FX-DM-%s-%04d", p.token, n)
				er.asserts = append(er.asserts, as)
			}
			out = append(out, er)
		}
	}
	return out, n, nil
}

// ---------------------------------------------------------------------------
// Canonical form
// ---------------------------------------------------------------------------

// canonical renders an expression as its token stream, so that two spellings of
// the same expression compare equal and two different expressions never do. It is
// what the round-trip checks above compare, because the artefact's own whitespace
// is incidental — `[ not(ram:ID/@schemeID="VA") and  not(...)]` — and its quoting
// is not consistent either.
func canonical(s string) string {
	toks, err := lex(s)
	if err != nil {
		return "\x00unlexable: " + s
	}
	var b strings.Builder
	for _, t := range toks {
		if t.kind == tokEOF {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		if t.kind == tokLiteral {
			b.WriteByte('\'')
			b.WriteString(t.text)
			b.WriteByte('\'')
			continue
		}
		b.WriteString(t.text)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Emitting Go
// ---------------------------------------------------------------------------

func golit(s string) string {
	return strconv.Quote(s)
}

func emitPath(p path) string {
	var parts []string
	if p.abs {
		parts = append(parts, "abs: true")
	}
	if p.up > 0 {
		parts = append(parts, "up: "+strconv.Itoa(p.up))
	}
	if len(p.steps) > 0 {
		parts = append(parts, "steps: "+emitNames(p.steps))
	}
	if p.attr != "" {
		parts = append(parts, "attr: "+golit(p.attr))
	}
	return "fxDMPath{" + strings.Join(parts, ", ") + "}"
}

// emitNames writes a step list as local names. The artefact's prefixes are
// dropped because parseCII keys the tree on local names; facturx_datamodel_test.go
// asserts that no two qualified names in these files share a local name, so the
// mapping loses nothing.
func emitNames(qnames []string) string {
	parts := make([]string, len(qnames))
	for i, q := range qnames {
		parts[i] = golit(local(q))
	}
	return "[]string{" + strings.Join(parts, ", ") + "}"
}

func local(q string) string {
	if i := strings.IndexByte(q, ':'); i >= 0 {
		return q[i+1:]
	}
	return q
}

func emitPred(p pred) string {
	if len(p.terms) == 0 {
		return "fxDMPred{}"
	}
	parts := make([]string, len(p.terms))
	for i, t := range p.terms {
		var fields []string
		if t.negs > 0 {
			fields = append(fields, "negs: "+strconv.Itoa(t.negs))
		}
		fields = append(fields, "left: "+emitPath(t.left))
		if t.eq {
			fields = append(fields, "eq: true")
			if t.isLit {
				fields = append(fields, "isLit: true", "lit: "+golit(t.lit))
			} else {
				fields = append(fields, "right: "+emitPath(t.right))
			}
		}
		parts[i] = "{" + strings.Join(fields, ", ") + "}"
	}
	return "fxDMPred{terms: []fxDMTerm{" + strings.Join(parts, ", ") + "}}"
}

// emitStep writes a step as a composite literal with its type elided, which is
// legal for a slice element and not for a struct field; emitStepValue is the
// spelled-out form the child field needs.
func emitStep(s step) string {
	if len(s.pred.terms) == 0 {
		return "{name: " + golit(local(s.name)) + "}"
	}
	return "{name: " + golit(local(s.name)) + ", pred: " + emitPred(s.pred) + "}"
}

func emitStepValue(s step) string { return "fxDMStep" + emitStep(s) }

func emitSteps(steps []step) string {
	parts := make([]string, len(steps))
	for i, s := range steps {
		parts[i] = emitStep(s)
	}
	return "[]fxDMStep{" + strings.Join(parts, ", ") + "}"
}

func emitAssert(a assertion) string {
	fields := []string{
		"key: " + golit(a.key),
		"op: " + string(a.op),
	}
	switch a.op {
	case opCountEQ, opCountLE, opCountGE:
		fields = append(fields, "child: "+emitStepValue(a.child), "bound: "+strconv.Itoa(a.bound))
	case opAttrRequired, opAttrForbidden:
		fields = append(fields, "attr: "+golit(a.attr))
	case opCode:
		fields = append(fields, "value: "+emitPath(a.value), "list: "+strconv.Itoa(a.list), "clID: "+strconv.Itoa(a.clID))
	}
	fields = append(fields, "test: "+golit(a.test), "msg: "+golit(a.msg))
	return "{" + strings.Join(fields, ", ") + "}"
}

func emitProfile(p profile, rules []emittedRule, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s is the %s profile's element-table data model: %d rules carrying %d\n", p.varName, p.token, len(rules), n)
	fmt.Fprintf(&b, "// assertions, in the artefact's own order. Every rule of every pattern that carries a\n")
	fmt.Fprintf(&b, "// data-model assertion is here, whether or not it carries one itself, because under ISO\n")
	fmt.Fprintf(&b, "// Schematron a node goes to the first matching rule of a pattern and a rule that asserts\n")
	fmt.Fprintf(&b, "// nothing can still claim a node away from one below it.\n")
	fmt.Fprintf(&b, "var %s = []fxDMRule{\n", p.varName)
	for _, r := range rules {
		fmt.Fprintf(&b, "\t{\n")
		fmt.Fprintf(&b, "\t\tpattern: %d,\n", r.pattern)
		fmt.Fprintf(&b, "\t\tcontext: %s,\n", golit(r.context))
		fmt.Fprintf(&b, "\t\tsteps:   %s,\n", emitSteps(r.steps))
		if len(r.asserts) > 0 {
			fmt.Fprintf(&b, "\t\tasserts: []fxDMAssert{\n")
			for _, a := range r.asserts {
				fmt.Fprintf(&b, "\t\t\t%s,\n", emitAssert(a))
			}
			fmt.Fprintf(&b, "\t\t},\n")
		}
		fmt.Fprintf(&b, "\t},\n")
	}
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

func emitLists(t *listTable, declared int) string {
	var b strings.Builder
	total := 0
	for _, l := range t.lists {
		total += len(l)
	}
	fmt.Fprintf(&b, "// facturXCodeLists are the code databases FNFE ships beside the five Schematrons,\n")
	fmt.Fprintf(&b, "// deduplicated: %d distinct lists holding %d values, from the %d lists the five files\n", len(t.lists), total, declared)
	fmt.Fprintf(&b, "// declare between them. Each list is sorted and free of duplicates, so a lookup is a\n")
	fmt.Fprintf(&b, "// binary search and nothing has to be built at load.\n")
	fmt.Fprintf(&b, "var facturXCodeLists = [][]string{\n")
	for i, l := range t.lists {
		fmt.Fprintf(&b, "\t%d: {", i)
		for j, v := range l {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString(golit(v))
		}
		fmt.Fprintf(&b, "},\n")
	}
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

const header = `package formalis

// Code generated from the Factur-X 1.09 / ZUGFeRD 2.5 profile Schematrons and the
// code databases beside them by internal/gen/facturx; DO NOT EDIT. Regenerate with
// ` + "`make facturx-datamodel`" + `.
//
// This is the per-profile element-table data model: one assertion per element of
// each profile's element table, in six mechanical shapes, and the binding
// Factur-X publishes in place of CEN's CII-SR-*/CII-DT-*. None of these assertions
// carries an identifier in the artefact — FNFE names a rule by an "[ID]-" prefix
// on its message and these have none — so each is keyed FX-DM-<PROFILE>-<NNNN>,
// where NNNN is its 1-based position in document order among that profile's
// data-model assertions. facturx_datamodel.go is the evaluator and documents that
// key; facturx_datamodel_test.go re-derives these tables from the Schematrons,
// asserts the committed set is exactly the published one, renders every
// decomposition back to XPath and compares it against the context and test strings
// held here, and makes every assertion report on a document mutated to break it.
//
// Each assertion's test and each rule's context is FNFE's own XPath,
// whitespace-normalised and otherwise verbatim, so this file can be read against
// the Schematron line by line.
`

func main() {
	dir := filepath.Join("testdata", "facturx", "schematron")
	if _, err := os.Stat(filepath.Join(dir, "FACTUR-X_EXTENDED.sch")); err != nil {
		fail("%s is not populated; run `make facturx-schematron` first", dir)
	}
	binding, err := bindingTests(dir)
	if err != nil {
		fail("%v", err)
	}
	lists := newListTable()
	var blocks []string
	var counts []string
	var mapRows []string
	declared := 0
	for _, p := range profiles {
		var cdb codedb
		if err := decode(filepath.Join(dir, p.codedbFile()), &cdb); err != nil {
			fail("%v", err)
		}
		declared += len(cdb.Lists)
		rules, n, err := readProfile(dir, p, lists, binding)
		if err != nil {
			fail("%v", err)
		}
		blocks = append(blocks, emitProfile(p, rules, n))
		counts = append(counts, fmt.Sprintf("  %-9s %4d rules, %4d assertions", p.token, len(rules), n))
		mapRows = append(mapRows, fmt.Sprintf("\t%s: %s,", p.goConst, p.varName))
	}
	listBlock := emitLists(lists, declared)

	out := filepath.Join("facturx_datamodel_table.go")
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString("// facturXDataModel is the five profiles' data models, keyed by the Profile a caller\n")
	b.WriteString("// names. A Profile with no entry has no data model, which no Profile is today.\n")
	b.WriteString("var facturXDataModel = map[Profile][]fxDMRule{\n")
	b.WriteString(strings.Join(mapRows, "\n"))
	b.WriteString("\n}\n\n")
	b.WriteString(strings.Join(blocks, "\n"))
	b.WriteString("\n")
	b.WriteString(listBlock)
	// Formatted here rather than by a separate `gofmt -w` in the Makefile, so
	// that a syntactically invalid emission fails the generator rather than
	// landing in the tree for the compiler to find.
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		fail("the emitted table does not parse: %v", err)
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Printf("wrote %s\n", out)
	for _, c := range counts {
		fmt.Println(c)
	}
	total := 0
	for _, l := range lists.lists {
		total += len(l)
	}
	fmt.Printf("  code lists: %d distinct, %d values, from %d declared across the five databases\n", len(lists.lists), total, declared)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen/facturx: "+format+"\n", args...)
	os.Exit(1)
}
