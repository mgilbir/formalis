package main

// The parser for the XPath subset the Factur-X profile data model is written in.
//
// It computes nothing. Its whole job is to decompose FNFE's own expressions into
// the structure facturx_datamodel.go evaluates, and to *refuse* anything outside
// the subset — loudly, naming the assertion — so that a shape this package cannot
// express can never be silently dropped from the emitted table. A generator that
// skips what it does not understand is how two fatal UBL-CR-* rules came to sit
// inside a coverage entry describing their family as advisory (C27), and how an
// entire national rule family became invisible to the guard built to find missing
// rules (C39).
//
// The subset is small because the artefact is generated. Over all five profiles
// the data model uses exactly 20 distinct predicates and 87 distinct count()
// arguments, and every one of them is built from:
//
//	path        ::= '/' qname pred? ( '/' qname pred? )*        (absolute)
//	              | ( '..' '/' )* qname ( '/' qname )* ( '/' '@' name )?
//	              | '@' name
//	pred        ::= '[' term ( 'and' term )* ']'
//	term        ::= 'not' '(' term ( 'and' term )* ')'
//	              | path ( '=' ( literal | path ) )?
//
// A bare path in boolean position is XPath's existence test, and `=` is XPath's
// general comparison: true when some node on the left has the same string value as
// some node (or the literal) on the right. Both are what the evaluator implements.

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Lexer
// ---------------------------------------------------------------------------

type tokKind int

const (
	tokEOF tokKind = iota
	tokName
	tokLiteral
	tokNumber
	tokPunct
)

type token struct {
	kind tokKind
	text string
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == ':'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lex tokenises one expression, or returns the offset it could not read.
//
// A '-' is a name character here rather than an operator: these expressions carry
// no arithmetic, and every '-' in them sits inside a QName. A '.' is a name
// character for the same reason except when it stands alone, which is the context
// item the code-list <let> bindings use.
func lex(s string) ([]token, error) {
	var out []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'' || c == '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return nil, fmt.Errorf("unterminated string literal at offset %d", i)
			}
			out = append(out, token{tokLiteral, s[i+1 : i+1+j]})
			i += j + 2
		case isDigit(c):
			j := i
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			out = append(out, token{tokNumber, s[i:j]})
			i = j
		case c == '.' && i+1 < len(s) && s[i+1] == '.':
			out = append(out, token{tokPunct, ".."})
			i += 2
		case c == '.':
			out = append(out, token{tokPunct, "."})
			i++
		case isNameStart(c):
			j := i
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			out = append(out, token{tokName, s[i:j]})
			i = j
		case strings.IndexByte("/[]()@=,", c) >= 0:
			out = append(out, token{tokPunct, string(c)})
			i++
		case c == '<' || c == '>':
			if i+1 < len(s) && s[i+1] == '=' {
				out = append(out, token{tokPunct, s[i : i+2]})
				i += 2
			} else {
				out = append(out, token{tokPunct, string(c)})
				i++
			}
		default:
			return nil, fmt.Errorf("cannot tokenise at offset %d: %q", i, s[i:])
		}
	}
	out = append(out, token{tokEOF, ""})
	return out, nil
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

type parser struct {
	toks []token
	i    int
}

func newParser(s string) (*parser, error) {
	toks, err := lex(s)
	if err != nil {
		return nil, err
	}
	return &parser{toks: toks}, nil
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) at(text string) bool {
	return p.toks[p.i].text == text && p.toks[p.i].kind != tokLiteral
}

func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }

func (p *parser) eat(text string) error {
	if !p.at(text) {
		return fmt.Errorf("expected %q, found %q", text, p.peek().text)
	}
	p.i++
	return nil
}

func (p *parser) atEOF() bool { return p.toks[p.i].kind == tokEOF }

// ---------------------------------------------------------------------------
// The parsed forms
// ---------------------------------------------------------------------------

// path is a location path: an absolute one from the document element, or one
// relative to the context node with some number of leading parent steps, ending
// optionally in an attribute.
type path struct {
	abs   bool
	up    int
	steps []string // qualified names, exactly as the artefact writes them
	attr  string
}

func (p path) render() string {
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

// term is one conjunct of a predicate: an existence test or a general comparison,
// under some number of not() wrappers. `not(A and B)` is not representable as a
// single term, and the parser refuses it rather than approximating; the artefact
// never writes one.
//
// negs is a count rather than a boolean because the artefact writes
// `not(not (@listID))`, and a boolean would have to fold the double negation —
// which is sound but would stop the decomposition rendering back to the XPath it
// came from, and that round trip is the only check that the structure emitted
// really is the expression the artefact published.
type term struct {
	negs  int
	left  path
	eq    bool
	isLit bool
	lit   string
	right path
}

func (t term) render() string {
	inner := t.left.render()
	if t.eq {
		if t.isLit {
			inner += "=\"" + t.lit + "\""
		} else {
			inner += "=" + t.right.render()
		}
	}
	for i := 0; i < t.negs; i++ {
		inner = "not(" + inner + ")"
	}
	return inner
}

// pred is a conjunction of terms — the only connective the artefact uses inside a
// predicate.
type pred struct {
	terms []term
}

func (p pred) render() string {
	if len(p.terms) == 0 {
		return ""
	}
	parts := make([]string, len(p.terms))
	for i, t := range p.terms {
		parts[i] = t.render()
	}
	return "[" + strings.Join(parts, " and ") + "]"
}

// step is one step of a rule context: a qualified name and an optional predicate.
type step struct {
	name string
	pred pred
}

// ---------------------------------------------------------------------------
// Productions
// ---------------------------------------------------------------------------

// parseContext parses a rule context, which in this artefact is always an
// absolute path from the document element with a predicate allowed on any step.
func parseContext(s string) ([]step, error) {
	p, err := newParser(s)
	if err != nil {
		return nil, err
	}
	if err := p.eat("/"); err != nil {
		return nil, fmt.Errorf("a rule context must be an absolute path: %w", err)
	}
	var steps []step
	for {
		st, err := p.step()
		if err != nil {
			return nil, err
		}
		steps = append(steps, st)
		if !p.at("/") {
			break
		}
		p.next()
	}
	if !p.atEOF() {
		return nil, fmt.Errorf("trailing input at %q", p.peek().text)
	}
	return steps, nil
}

// parseCountArg parses the argument of a count(): one child step, with an
// optional predicate.
func parseCountArg(s string) (step, error) {
	p, err := newParser(s)
	if err != nil {
		return step{}, err
	}
	st, err := p.step()
	if err != nil {
		return step{}, err
	}
	if !p.atEOF() {
		return step{}, fmt.Errorf("count() takes one child step here; trailing input at %q", p.peek().text)
	}
	return st, nil
}

// parseValuePath parses a <let> value: the context item or an attribute.
func parseValuePath(s string) (path, error) {
	s = strings.TrimSpace(s)
	if s == "." {
		return path{}, nil
	}
	p, err := newParser(s)
	if err != nil {
		return path{}, err
	}
	pa, err := p.path()
	if err != nil {
		return path{}, err
	}
	if !p.atEOF() {
		return path{}, fmt.Errorf("trailing input at %q", p.peek().text)
	}
	return pa, nil
}

func (p *parser) step() (step, error) {
	t := p.peek()
	if t.kind != tokName {
		return step{}, fmt.Errorf("expected an element name, found %q", t.text)
	}
	p.next()
	st := step{name: t.text}
	if p.at("[") {
		pr, err := p.predicate()
		if err != nil {
			return step{}, err
		}
		st.pred = pr
	}
	return st, nil
}

func (p *parser) predicate() (pred, error) {
	if err := p.eat("["); err != nil {
		return pred{}, err
	}
	terms, err := p.conjunction()
	if err != nil {
		return pred{}, err
	}
	if err := p.eat("]"); err != nil {
		return pred{}, err
	}
	return pred{terms: terms}, nil
}

func (p *parser) conjunction() ([]term, error) {
	var out []term
	for {
		t, err := p.term()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if !p.at("and") {
			return out, nil
		}
		p.next()
	}
}

func (p *parser) term() (term, error) {
	if p.at("not") {
		p.next()
		if err := p.eat("("); err != nil {
			return term{}, err
		}
		inner, err := p.conjunction()
		if err != nil {
			return term{}, err
		}
		if err := p.eat(")"); err != nil {
			return term{}, err
		}
		if len(inner) != 1 {
			// not(A and B) is representable, but as two terms it would be wrong
			// (De Morgan turns it into a disjunction, which pred cannot hold) and
			// as one term it would need a nested predicate. The artefact never
			// writes one, so this refuses rather than approximating.
			return term{}, fmt.Errorf("not() over a conjunction of %d terms is outside the subset", len(inner))
		}
		t := inner[0]
		t.negs++
		return t, nil
	}
	left, err := p.path()
	if err != nil {
		return term{}, err
	}
	t := term{left: left}
	if p.at("=") {
		p.next()
		t.eq = true
		if p.peek().kind == tokLiteral {
			t.isLit = true
			t.lit = p.next().text
			return t, nil
		}
		right, err := p.path()
		if err != nil {
			return term{}, err
		}
		t.right = right
	}
	return t, nil
}

// path parses a location path in a predicate.
func (p *parser) path() (path, error) {
	var out path
	switch {
	case p.at("@"):
		p.next()
		t := p.peek()
		if t.kind != tokName {
			return path{}, fmt.Errorf("@ must be followed by an attribute name, found %q", t.text)
		}
		p.next()
		out.attr = t.text
		return out, nil
	case p.at("/"):
		out.abs = true
		for p.at("/") {
			p.next()
			t := p.peek()
			if t.kind != tokName {
				return path{}, fmt.Errorf("expected an element name after /, found %q", t.text)
			}
			p.next()
			out.steps = append(out.steps, t.text)
		}
		return p.finishAttr(out)
	case p.at(".."):
		for p.at("..") {
			p.next()
			out.up++
			if !p.at("/") {
				return out, nil
			}
			p.next()
		}
	}
	for {
		if p.at("@") {
			return p.finishAttrAt(out)
		}
		t := p.peek()
		if t.kind != tokName {
			return path{}, fmt.Errorf("expected an element name, found %q", t.text)
		}
		p.next()
		out.steps = append(out.steps, t.text)
		if !p.at("/") {
			return out, nil
		}
		p.next()
	}
}

// finishAttr consumes a trailing /@name on an absolute path, which the artefact
// never writes but which the grammar allows.
func (p *parser) finishAttr(out path) (path, error) {
	if !p.at("@") {
		return out, nil
	}
	return p.finishAttrAt(out)
}

func (p *parser) finishAttrAt(out path) (path, error) {
	if err := p.eat("@"); err != nil {
		return path{}, err
	}
	t := p.peek()
	if t.kind != tokName {
		return path{}, fmt.Errorf("@ must be followed by an attribute name, found %q", t.text)
	}
	p.next()
	out.attr = t.text
	return out, nil
}

// renderContext renders a parsed context back to XPath. The generator compares it
// against the artefact's own string before emitting anything, so a decomposition
// that lost or invented a step cannot reach the table.
func renderContext(steps []step) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteByte('/')
		b.WriteString(s.name)
		b.WriteString(s.pred.render())
	}
	return b.String()
}

func renderCountArg(s step) string { return s.name + s.pred.render() }
