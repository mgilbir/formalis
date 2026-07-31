package formalis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The guards on the per-CIUS condition overrides, in both directions.
//
// There are four questions, and they are separate on purpose:
//
//   - is the *classification* right — which of the differences between a CIUS's
//     copy of CEN's Schematron and CEN's own file are that authority's own?
//     TestCIUSCopiesOfCENAreClassifiedFromTheArtefacts re-derives the whole thing
//     from the artefacts and CEN's history and compares it to the committed table.
//   - is the *transcription* right — does the emitted pattern quote the authority's
//     contexts, rule order, XPath, polarity, flag and message verbatim?
//     TestConditionOverrideTableTranscribesTheArtefact.
//   - does each override *do* anything — is there a document on which the
//     authority's reading and CEN's differ observably? TestEveryConditionOverrideFires.
//     A rule that is present, reachable and inert passes every other guard here
//     (C41), and none of these nine fires differently anywhere in the 1,690-document
//     corpus, so a fixture is the only thing that can tell them apart.
//   - is it *contained* — does an override reach any path that did not ask for that
//     CIUS? TestConditionOverridesApplyOnlyUnderTheirCIUS.

// ---------------------------------------------------------------------------
// Reading a Schematron
// ---------------------------------------------------------------------------

// ccAssert is one <assert> or <report>: the authority's identifier, polarity, flag,
// XPath and message text with the leading "[rule-id]-" stripped.
type ccAssert struct {
	id, kind, flag, test, msg string
	// ctx is the context of the <rule> the assertion sits in. It is carried on the
	// assertion so that an identifier's whole (context, polarity, test) — the triple
	// the classification compares — is one value.
	ctx string
}

// ccRule is one <rule>: its context with the pattern's parameters resolved, and its
// assertions in document order.
type ccRule struct {
	ctx     string
	asserts []ccAssert
}

var ccMsgPrefix = regexp.MustCompile(`^\[[^]]*\]\s*-?\s*`)

func ccCollapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// ccParams reads the <param name= value=> of a binding pattern. A nil or empty
// binding yields an empty set, which is what a concrete pattern (CEN's code lists,
// UBL.BE's merged file) needs.
func ccParams(data []byte) map[string]string {
	out := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = latin1Reader
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "param" {
			continue
		}
		var n, v string
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "name":
				n = a.Value
			case "value":
				v = a.Value
			}
		}
		out[ccCollapse(n)] = v
	}
}

// ccDeref resolves a whole-value parameter reference, which is the only form CEN's
// abstract files and every copy of them use: `test="$BR-02"`, `context="$Invoice "`.
// A value that is not one is the expression itself.
func ccDeref(v string, params map[string]string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "$") {
		if p, ok := params[ccCollapse(v[1:])]; ok {
			return p
		}
	}
	return v
}

// ccResolve reads one (abstract, binding) pair into rules in document order,
// optionally restricted to named patterns. It is the Go half of gen.py's read_pair
// and read_flat, and the two must agree: the drift test below compares what this
// produces against what the generator wrote.
func ccResolve(abstract, binding []byte, patterns map[string]bool) []ccRule {
	params := ccParams(binding)
	var rules []ccRule
	var inWanted bool = patterns == nil
	// CEN declares cii/schematron/CII/EN16931-CII-syntax.sch ISO-8859-1, and every
	// copy of it inherits the declaration. Without a CharsetReader the decoder
	// refuses the file outright and yields no parameters at all — which does not
	// fail, it silently leaves every $CII-* reference unresolved and classifies the
	// whole CII binding as unchanged. It is the same silent-degradation shape as a
	// regex that stops matching (C31), and it was caught here only because the
	// generator and this test read the artefact independently and disagreed by one.
	dec := xml.NewDecoder(bytes.NewReader(abstract))
	dec.CharsetReader = latin1Reader
	for {
		tok, err := dec.Token()
		if err != nil {
			return rules
		}
		switch e := tok.(type) {
		case xml.StartElement:
			attr := func(n string) string {
				for _, a := range e.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			switch e.Name.Local {
			case "pattern":
				if patterns != nil {
					inWanted = patterns[attr("id")]
				}
			case "rule":
				if inWanted {
					rules = append(rules, ccRule{ctx: ccCollapse(ccDeref(attr("context"), params))})
				}
			case "assert", "report":
				if !inWanted || len(rules) == 0 {
					continue
				}
				text, _ := ptDTElementText(dec, e)
				r := &rules[len(rules)-1]
				r.asserts = append(r.asserts, ccAssert{
					id:   attr("id"),
					kind: e.Name.Local,
					flag: attr("flag"),
					test: ccCollapse(ccDeref(attr("test"), params)),
					msg:  ccMsgPrefix.ReplaceAllString(ccCollapse(text), ""),
				})
			}
		}
	}
}

func ccIndex(rules []ccRule) map[string]ccAssert {
	out := map[string]ccAssert{}
	for _, r := range rules {
		for _, a := range r.asserts {
			a.ctx = r.ctx
			out[a.id] = a
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The artefacts this file reads
// ---------------------------------------------------------------------------

const ccCENDir = "testdata/en16931-artefacts"

// ccPair is one (abstract, binding) pair of CEN's, as repository-relative paths.
// The binding is empty for a concrete pattern.
type ccPair struct{ abstract, binding string }

var ccCENUBLPairs = []ccPair{
	{"ubl/schematron/abstract/EN16931-model.sch", "ubl/schematron/UBL/EN16931-UBL-model.sch"},
	{"ubl/schematron/abstract/EN16931-syntax.sch", "ubl/schematron/UBL/EN16931-UBL-syntax.sch"},
	{"ubl/schematron/codelist/EN16931-UBL-codes.sch", ""},
}

var ccCENCIIPairs = []ccPair{
	{"cii/schematron/abstract/EN16931-CII-model.sch", "cii/schematron/CII/EN16931-CII-model.sch"},
	{"cii/schematron/abstract/EN16931-CII-syntax.sch", "cii/schematron/CII/EN16931-CII-syntax.sch"},
	{"cii/schematron/codelist/EN16931-CII-codes.sch", ""},
}

// ccCopy is one authority's copy of CEN's files, named the way gen.py names it.
// files is a list of (abstract, binding) pairs for a copy that keeps CEN's
// abstract/binding split, or one entry with patterns set for a copy that ships them
// already resolved. dir is set instead for an authority that ships no copy.
type ccCopy struct {
	authority string
	syntax    string
	pairs     []ccPair
	patterns  map[string]bool
	dir       string
}

var ccCopies = []ccCopy{
	{authority: "CIUS-PT 2.1.1 (UBL)", syntax: "UBL", pairs: []ccPair{
		{"testdata/cius-pt/schematron/2.1.1/abstract/urn_feap.gov.pt_CIUS-PT_2.1.1-model.sch",
			"testdata/cius-pt/schematron/2.1.1/UBL/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-model.sch"},
		{"testdata/cius-pt/schematron/2.1.1/abstract/urn_feap.gov.pt_CIUS-PT_2.1.1-syntax.sch",
			"testdata/cius-pt/schematron/2.1.1/UBL/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-syntax.sch"},
	}},
	{authority: "CIUS-RO 1.0.9 (UBL)", syntax: "UBL", pairs: []ccPair{
		{"testdata/cius-ro/schematron/1.0.9/abstract/EN16931-model.sch",
			"testdata/cius-ro/schematron/1.0.9/UBL/EN16931-UBL-model.sch"},
		{"testdata/cius-ro/schematron/1.0.9/abstract/EN16931-syntax.sch",
			"testdata/cius-ro/schematron/1.0.9/UBL/EN16931-UBL-syntax.sch"},
	}},
	{authority: "NLCIUS SI-UBL 2.0.3.2 (UBL)", syntax: "UBL", pairs: []ccPair{
		{"testdata/nlcius/schematron/ubl/cen/EN16931-model.sch",
			"testdata/nlcius/schematron/ubl/cen/EN16931-UBL-model.sch"},
		{"testdata/nlcius/schematron/ubl/cen/EN16931-syntax.sch",
			"testdata/nlcius/schematron/ubl/cen/EN16931-UBL-syntax.sch"},
	}},
	{authority: "NLCIUS SI-UBL G-account 1.0.2 (UBL)", syntax: "UBL", pairs: []ccPair{
		{"testdata/nlcius/schematron/ubl/cen/EN16931-syntax-modified.sch",
			"testdata/nlcius/schematron/ubl/cen/EN16931-UBL-syntax.sch"},
	}},
	{authority: "NLCIUS 1.0.3 (CII)", syntax: "CII", pairs: []ccPair{
		{"testdata/nlcius/schematron/cii/cen/abstract/EN16931-CII-model.sch",
			"testdata/nlcius/schematron/cii/cen/CII/EN16931-CII-model.sch"},
		{"testdata/nlcius/schematron/cii/cen/abstract/EN16931-CII-syntax.sch",
			"testdata/nlcius/schematron/cii/cen/CII/EN16931-CII-syntax.sch"},
	}},
	{authority: "UBL.BE v1.31 (UBL)", syntax: "UBL",
		pairs:    []ccPair{{"testdata/cius-be/schematron/v1.31/GLOBALUBL.BE.sch", ""}},
		patterns: map[string]bool{"ubl-model": true, "UBL-syntax": true, "Codesmodel": true}},
	{authority: "SRBDT 1.0.0 (UBL)", syntax: "UBL", dir: "testdata/cius-rs/schematron/1.0.0"},
}

// ---------------------------------------------------------------------------
// What CEN ever published
// ---------------------------------------------------------------------------

// ccCENPublished resolves every version of CEN's files and returns, per identifier,
// the set of (context, polarity, test) triples CEN has published at any commit.
//
// This is the whole discriminator between "the authority wrote this" and "CEN wrote
// this and changed it later", and it is the reason `make en16931-artefacts` clones
// with history rather than --depth 1. A shallow clone would answer "CEN never
// published it" for every condition CEN has since edited, which is 735 of CIUS-PT's
// 771 shared identifiers: the classification would invert.
func ccCENPublished(t *testing.T, pairs []ccPair) (map[string]map[[3]string]bool, int) {
	t.Helper()
	var revs []string
	seen := map[string]bool{}
	for _, p := range pairs {
		for _, f := range []string{p.abstract, p.binding} {
			if f == "" {
				continue
			}
			for _, c := range ccGitLines(t, "log", "--format=%H", "--", f) {
				if !seen[c] {
					seen[c] = true
					revs = append(revs, c)
				}
			}
		}
	}
	var want []string
	for _, c := range revs {
		for _, p := range pairs {
			want = append(want, c+":"+p.abstract)
			if p.binding != "" {
				want = append(want, c+":"+p.binding)
			}
		}
	}
	blobs := ccGitCatFile(t, want)

	out := map[string]map[[3]string]bool{}
	for _, c := range revs {
		for _, p := range pairs {
			ab, ok := blobs[c+":"+p.abstract]
			if !ok {
				continue
			}
			bi := blobs[c+":"+p.binding]
			for _, r := range ccResolve(ab, bi, nil) {
				for _, a := range r.asserts {
					if out[a.id] == nil {
						out[a.id] = map[[3]string]bool{}
					}
					out[a.id][[3]string{r.ctx, a.kind, a.test}] = true
				}
			}
		}
	}
	return out, len(revs)
}

func ccGitLines(t *testing.T, args ...string) []string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", ccCENDir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.Fields(string(out))
}

// ccGitCatFile reads many `commit:path` blobs with one git process. A path missing at
// a commit is simply absent from the result.
func ccGitCatFile(t *testing.T, revs []string) map[string][]byte {
	t.Helper()
	cmd := exec.Command("git", "-C", ccCENDir, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(strings.Join(revs, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git cat-file --batch: %v", err)
	}
	blobs := map[string][]byte{}
	rd := bufio.NewReader(bytes.NewReader(out))
	for i := 0; i < len(revs); i++ {
		header, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("git cat-file: short output at %d of %d", i, len(revs))
		}
		f := strings.Fields(header)
		if len(f) == 2 && f[1] == "missing" {
			continue
		}
		if len(f) != 3 {
			t.Fatalf("git cat-file: unreadable header %q", header)
		}
		n, err := strconv.Atoi(f[2])
		if err != nil {
			t.Fatalf("git cat-file: unreadable size in %q", header)
		}
		buf := make([]byte, n+1)
		if _, err := ccReadFull(rd, buf); err != nil {
			t.Fatalf("git cat-file: short body: %v", err)
		}
		blobs[revs[i]] = buf[:n]
	}
	return blobs
}

func ccReadFull(rd *bufio.Reader, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := rd.Read(buf[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// The classification, re-derived
// ---------------------------------------------------------------------------

func TestCIUSCopiesOfCENAreClassifiedFromTheArtefacts(t *testing.T) {
	if _, err := os.Stat(ccCENDir); err != nil {
		t.Skip("CEN artefacts not present (make en16931-artefacts)")
	}
	if _, err := os.Stat(filepath.Join(ccCENDir, ".git", "shallow")); err == nil {
		t.Fatalf("%s is a shallow clone. This test classifies a CIUS condition by asking whether CEN "+
			"ever published it, which is a question about the repository's history; a shallow clone "+
			"answers no to every version CEN has since changed. Run `make en16931-artefacts`, which "+
			"clones with full history for this reason", ccCENDir)
	}
	if _, err := os.Stat("testdata/cius-pt/schematron/2.1.1"); err != nil {
		t.Skip("CIUS Schematrons not present (make cius-schematron)")
	}

	nowUBL := ccIndex(ccReadPairs(t, ccCENDir, ccCENUBLPairs, nil))
	nowCII := ccIndex(ccReadPairs(t, ccCENDir, ccCENCIIPairs, nil))
	histUBL, nUBL := ccCENPublished(t, ccCENUBLPairs)
	histCII, nCII := ccCENPublished(t, ccCENCIIPairs)
	t.Logf("CEN history: %d commits touching the UBL files, %d the CII files", nUBL, nCII)

	byAuthority := map[string]ciusCENCopyVerdict{}
	for _, v := range ciusCENCopyVerdicts {
		byAuthority[v.authority] = v
	}
	if len(byAuthority) != len(ciusCENCopyVerdicts) {
		t.Fatalf("ciusCENCopyVerdicts names an authority twice")
	}

	for _, c := range ccCopies {
		want, ok := byAuthority[c.authority]
		if !ok {
			t.Errorf("%s ships a copy of CEN's files and ciusCENCopyVerdicts does not record it", c.authority)
			continue
		}
		delete(byAuthority, c.authority)

		now, hist := nowUBL, histUBL
		if c.syntax == "CII" {
			now, hist = nowCII, histCII
		}

		if c.dir != "" {
			found := ccCENIdentifiersUnder(t, c.dir, now)
			if len(found) != 0 {
				t.Errorf("%s is recorded as shipping no copy of CEN's files, but %s carries %v",
					c.authority, c.dir, found)
			}
			if want.ships {
				t.Errorf("%s is recorded as shipping a copy of CEN's files; it ships none", c.authority)
			}
			continue
		}
		if !want.ships {
			t.Errorf("%s is recorded as shipping no copy of CEN's files; it ships one", c.authority)
		}

		cius := ccIndex(ccReadPairs(t, "", c.pairs, c.patterns))
		same, stale, own := 0, 0, map[string]string{}
		for id, a := range cius {
			cn, isCEN := now[id]
			if !isCEN {
				continue
			}
			// The three axes that decide which nodes a rule claims and what it says
			// about them. The message text is not one of them: an authority that
			// reworded CEN's prose has not changed the rule.
			if a.ctx == cn.ctx && a.kind == cn.kind && a.test == cn.test {
				same++
				continue
			}
			h := hist[id]
			var axes []string
			if !ccAnyTriple(h, 0, a.ctx) {
				axes = append(axes, "context")
			}
			if !ccAnyTriple(h, 1, a.kind) {
				axes = append(axes, "polarity")
			}
			if !ccAnyTriple(h, 2, a.test) {
				axes = append(axes, "test")
			}
			if len(axes) == 0 {
				stale++
				continue
			}
			own[id] = strings.Join(axes, ", ")
		}

		if same != want.same || stale != want.stale || same+stale+len(own) != want.shared {
			t.Errorf("%s: derived shared=%d same=%d stale=%d; the table says shared=%d same=%d stale=%d",
				c.authority, same+stale+len(own), same, stale, want.shared, want.same, want.stale)
		}
		if len(own) != len(want.own) {
			t.Errorf("%s: derived %d identifiers of the authority's own, the table records %d",
				c.authority, len(own), len(want.own))
		}
		for id, axes := range own {
			if want.own[id] != axes {
				t.Errorf("%s: %s is the authority's own on %q; the table says %q",
					c.authority, id, axes, want.own[id])
			}
		}
		for id := range want.own {
			if _, ok := own[id]; !ok {
				t.Errorf("%s: the table records %s as the authority's own; the artefact and CEN's "+
					"history do not agree", c.authority, id)
			}
		}
		t.Logf("%-38s shared %4d  same %4d  stale %3d  own %d %v",
			c.authority, same+stale+len(own), same, stale, len(own), ccSortedKeys(own))
	}
	for a := range byAuthority {
		t.Errorf("ciusCENCopyVerdicts records %q and no copy is read for it", a)
	}
}

// ccAnyTriple reports whether some triple CEN published has v in field i. The
// assertion's context travels in ccAssert.msg — ccIndex puts the rule's context
// there so that an identifier carries its whole (context, polarity, test) in one
// value.
func ccAnyTriple(h map[[3]string]bool, i int, v string) bool {
	for k := range h {
		if k[i] == v {
			return true
		}
	}
	return false
}

func ccSortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ccReadPairs(t *testing.T, base string, pairs []ccPair, patterns map[string]bool) []ccRule {
	t.Helper()
	var out []ccRule
	for _, p := range pairs {
		ab, err := os.ReadFile(filepath.Join(base, p.abstract))
		if err != nil {
			t.Fatalf("%v", err)
		}
		var bi []byte
		if p.binding != "" {
			bi, err = os.ReadFile(filepath.Join(base, p.binding))
			if err != nil {
				t.Fatalf("%v", err)
			}
		}
		out = append(out, ccResolve(ab, bi, patterns)...)
	}
	return out
}

func ccCENIdentifiersUnder(t *testing.T, dir string, now map[string]ccAssert) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sch") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, r := range ccResolve(data, nil, nil) {
			for _, a := range r.asserts {
				if _, ok := now[a.id]; ok {
					found = append(found, a.id)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(found)
	return found
}

// ---------------------------------------------------------------------------
// The transcription
// ---------------------------------------------------------------------------

// TestConditionOverrideTableTranscribesTheArtefact holds the generated pattern to
// AT/eSPap's file in both directions: every rule of the authority's pattern is in
// the table, in the authority's order and with the authority's context, and every
// assertion the table carries quotes the authority's polarity, XPath and message.
//
// Both directions matter. A missing rule is a context handed to the rule below it,
// which is how a Schematron reader silently changes which nodes a rule claims; an
// extra assertion is a rule this package would report that AT does not.
func TestConditionOverrideTableTranscribesTheArtefact(t *testing.T) {
	if _, err := os.Stat("testdata/cius-pt/schematron/2.1.1"); err != nil {
		t.Skip("CIUS Schematrons not present (make cius-schematron)")
	}
	rules := ccReadPairs(t, "", []ccPair{{
		"testdata/cius-pt/schematron/2.1.1/abstract/urn_feap.gov.pt_CIUS-PT_2.1.1-model.sch",
		"testdata/cius-pt/schematron/2.1.1/UBL/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-model.sch"}}, nil)

	if len(ptConditionOverrides.patterns) != 1 {
		t.Fatalf("the CIUS-PT override set holds %d patterns, expected 1", len(ptConditionOverrides.patterns))
	}
	table := ptConditionOverrides.patterns[0]
	if len(table.rules) != len(rules) {
		t.Fatalf("the table holds %d rules, AT/eSPap's model pattern has %d. Every rule belongs in the "+
			"table whether or not it carries an overridden assertion: under ISO Schematron a node goes "+
			"to the first rule whose context matches it, so a missing rule hands its nodes to a rule "+
			"below it", len(table.rules), len(rules))
	}
	for i, want := range rules {
		got := table.rules[i]
		if got.context != want.ctx {
			t.Errorf("rule %d: the table says context %q, the artefact says %q", i, got.context, want.ctx)
		}
		var wantAsserts []ccAssert
		for _, a := range want.asserts {
			if _, ok := ptConditionOverrides.rules[a.id]; ok {
				wantAsserts = append(wantAsserts, a)
			}
		}
		if len(got.asserts) != len(wantAsserts) {
			t.Errorf("rule %d (%s): the table carries %d overridden assertions, the artefact has %d",
				i, want.ctx, len(got.asserts), len(wantAsserts))
			continue
		}
		for j, a := range wantAsserts {
			g := got.asserts[j]
			if g.id != a.id || g.kind != a.kind || g.test != a.test || g.message != a.msg {
				t.Errorf("rule %d assertion %d: the table has\n  %s %s %s\n  %s\nthe artefact has\n  %s %s %s\n  %s",
					i, j, g.id, g.kind, g.test, g.message, a.id, a.kind, a.test, a.msg)
			}
			wantSev := SeverityFatal
			if a.flag == "warning" {
				wantSev = SeverityWarning
			}
			if ptConditionOverrides.rules[a.id] != wantSev {
				t.Errorf("%s: the table reports it %s, AT/eSPap flags it %q", a.id,
					ptConditionOverrides.rules[a.id], a.flag)
			}
		}
	}
}

// TestOverriddenIdentifiersAreCENs is the guard that keeps this mechanism from
// becoming a way to invent rules. An override replaces a condition for an
// identifier CEN publishes; an identifier CEN does not publish is the authority's
// own rule and belongs in that authority's rule set under its own Source, not here.
func TestOverriddenIdentifiersAreCENs(t *testing.T) {
	if _, err := os.Stat(ccCENDir); err != nil {
		t.Skip("CEN artefacts not present (make en16931-artefacts)")
	}
	now := ccIndex(ccReadPairs(t, ccCENDir, ccCENUBLPairs, nil))
	for id := range ptConditionOverrides.rules {
		if _, ok := now[id]; !ok {
			t.Errorf("%s is overridden under CIUS-PT and CEN does not publish it. An identifier CEN "+
				"did not mint is the authority's own rule and belongs under its own Source", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Firing
// ---------------------------------------------------------------------------

// ptOverrideFixtures is one document per overridden identifier on which AT/eSPap's
// condition and CEN's disagree, and what each says about it.
//
// This is the guard C41 asks for and the only one that can be given here. The other
// three tests in this file would all pass over an override that was transcribed
// correctly, reachable, and inert — and inert is exactly what these nine are on the
// whole 1,690-document corpus: every one of them fires the same number of times
// through AT's condition as through CEN's, on every document this repository has.
// A fixture is therefore not a convenience, it is the only evidence that the
// substitution does anything at all.
//
// The two shapes of disagreement are AT's, and both are recorded in
// ciusCENCopyVerdicts as the axis CEN never published:
//
//   - the VAT category aliases. AT treats 'NOR' as a synonym of 'S' and 'ISE' of
//     'E' throughout BR-S-* and BR-E-*, which CEN does not, so a Portuguese invoice
//     using them is judged by those rules under CIUS-PT and by nothing under CEN.
//     Eight of the nine are of this kind, and each has a fixture below.
//   - BR-23's polarity. CEN asserts that a line *has* a quantity unit code, so a
//     line with no quantity element at all breaks it; AT reports a line that has a
//     quantity *without* one, so such a line does not. That difference is not
//     observable through this package, for the reason ptOverridesWithNoVisibleEffect
//     gives, and BR-23 is listed there rather than given a fixture that does not
//     fixture anything.
var ptOverrideFixtures = []struct {
	rule    string
	doc     string
	underPT bool // does AT/eSPap's condition report it?
	underEN bool // does CEN's?
}{
	{rule: "BR-S-02", underPT: true, underEN: false, doc: ptFixtureNoSellerVATLine(`
		<cac:InvoiceLine><cbc:ID>1</cbc:ID>
			<cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>
			<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
			<cac:Item><cbc:Name>Thing</cbc:Name>
				<cac:ClassifiedTaxCategory><cbc:ID>NOR</cbc:ID><cbc:Percent>23</cbc:Percent>
					<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
			<cac:Price><cbc:PriceAmount currencyID="EUR">100.00</cbc:PriceAmount></cac:Price>
		</cac:InvoiceLine>`)},

	{rule: "BR-S-03", underPT: true, underEN: false, doc: ptFixtureNoSellerVAT(`
		<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator>
			<cbc:AllowanceChargeReason>Discount</cbc:AllowanceChargeReason>
			<cbc:Amount currencyID="EUR">10.00</cbc:Amount>
			<cac:TaxCategory><cbc:ID>NOR</cbc:ID><cbc:Percent>23</cbc:Percent>
				<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>`)},

	{rule: "BR-S-04", underPT: true, underEN: false, doc: ptFixtureNoSellerVAT(`
		<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator>
			<cbc:AllowanceChargeReason>Freight</cbc:AllowanceChargeReason>
			<cbc:Amount currencyID="EUR">10.00</cbc:Amount>
			<cac:TaxCategory><cbc:ID>NOR</cbc:ID><cbc:Percent>23</cbc:Percent>
				<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>`)},

	{rule: "BR-E-02", underPT: true, underEN: false, doc: ptFixtureNoSellerVATLine(`
		<cac:InvoiceLine><cbc:ID>1</cbc:ID>
			<cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>
			<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
			<cac:Item><cbc:Name>Thing</cbc:Name>
				<cac:ClassifiedTaxCategory><cbc:ID>ISE</cbc:ID><cbc:Percent>0</cbc:Percent>
					<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
			<cac:Price><cbc:PriceAmount currencyID="EUR">100.00</cbc:PriceAmount></cac:Price>
		</cac:InvoiceLine>`)},

	{rule: "BR-E-03", underPT: true, underEN: false, doc: ptFixtureNoSellerVAT(`
		<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator>
			<cbc:AllowanceChargeReason>Discount</cbc:AllowanceChargeReason>
			<cbc:Amount currencyID="EUR">10.00</cbc:Amount>
			<cac:TaxCategory><cbc:ID>ISE</cbc:ID><cbc:Percent>0</cbc:Percent>
				<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>`)},

	{rule: "BR-E-04", underPT: true, underEN: false, doc: ptFixtureNoSellerVAT(`
		<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator>
			<cbc:AllowanceChargeReason>Freight</cbc:AllowanceChargeReason>
			<cbc:Amount currencyID="EUR">10.00</cbc:Amount>
			<cac:TaxCategory><cbc:ID>ISE</cbc:ID><cbc:Percent>0</cbc:Percent>
				<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:AllowanceCharge>`)},

	{rule: "BR-E-10", underPT: true, underEN: false, doc: ptFixtureBreakdown(`
		<cac:TaxCategory><cbc:ID>ISE</cbc:ID><cbc:Percent>0</cbc:Percent>
			<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`)},

	{rule: "BR-S-10", underPT: true, underEN: false, doc: ptFixtureBreakdown(`
		<cac:TaxCategory><cbc:ID>NOR</cbc:ID><cbc:Percent>23</cbc:Percent>
			<cbc:TaxExemptionReason>Not exempt at all</cbc:TaxExemptionReason>
			<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`)},
}

func TestEveryConditionOverrideFires(t *testing.T) {
	covered := map[string]bool{}
	for _, f := range ptOverrideFixtures {
		covered[f.rule] = true
		t.Run(f.rule, func(t *testing.T) {
			pt, err := ValidateCIUSPT(context.Background(), []byte(f.doc))
			if err != nil {
				t.Fatalf("ValidateCIUSPT: %v", err)
			}
			en, err := Validate(context.Background(), []byte(f.doc), ProfileEN16931)
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			gotPT := ccFindRule(pt.Violations, f.rule)
			gotEN := ccFindRule(en.Violations, f.rule)
			if (gotPT != nil) != f.underPT {
				t.Errorf("ValidateCIUSPT reports %s = %v, AT/eSPap's condition says %v\n  all: %v",
					f.rule, gotPT != nil, f.underPT, ccRuleNames(pt.Violations))
			}
			if (gotEN != nil) != f.underEN {
				t.Errorf("Validate reports %s = %v, CEN's condition says %v\n  all: %v",
					f.rule, gotEN != nil, f.underEN, ccRuleNames(en.Violations))
			}
			if gotPT != nil {
				if gotPT.Source != SourceEN16931 {
					t.Errorf("%s is stamped %s; the identifier is CEN's and must stay so", f.rule, gotPT.Source)
				}
				if gotPT.Reading != SourceCIUSPT {
					t.Errorf("%s was judged by AT/eSPap's condition and carries Reading %q; a caller "+
						"cannot otherwise tell it apart from the same finding under CEN's condition",
						f.rule, gotPT.Reading)
				}
				if !strings.Contains(gotPT.Error(), "as CIUS-PT reads it") {
					t.Errorf("%s renders as %q, which does not say whose condition decided it",
						f.rule, gotPT.Error())
				}
			}
		})
	}
	for id := range ptConditionOverrides.rules {
		if covered[id] {
			if _, excused := ptOverridesWithNoVisibleEffect[id]; excused {
				t.Errorf("%s has a fixture that shows AT/eSPap's condition and this package's differ, "+
					"and is also recorded as having no visible effect", id)
			}
			continue
		}
		if _, excused := ptOverridesWithNoVisibleEffect[id]; !excused {
			t.Errorf("%s is overridden and no fixture makes AT/eSPap's condition and this package's "+
				"differ on it. An override with no such fixture is indistinguishable from no override "+
				"at all: the corpus does not separate them either, because all nine agree on all 1,690 "+
				"documents. Add a fixture, or record why none exists in ptOverridesWithNoVisibleEffect", id)
		}
	}
}

// ptOverridesWithNoVisibleEffect names an override that is applied for fidelity to
// the authority's file and that no document can distinguish from what this package
// already did, with the reason. It is one entry, and the reason is a finding in its
// own right rather than an excuse.
//
// This is the shape C41 warns about — a rule that is present, reachable and inert —
// so it is recorded rather than left to be rediscovered. The difference is that here
// the inertness is *derived*: AT's condition and this package's implementation of
// BR-23 happen to be the same rule, and it is CEN's published condition that both
// depart from.
var ptOverridesWithNoVisibleEffect = map[string]string{
	"BR-23": "CEN asserts exists(cbc:InvoicedQuantity/@unitCode), which a line with no quantity element " +
		"at all breaks; AT/eSPap reports a line that has a quantity without a unit code, which such a " +
		"line does not. This package's own BR-23 (en16931_model.go: `if li.quantity == \"\" { BR-22 } " +
		"else if li.unitCode == \"\" { BR-23 }`) is AT's reading and not CEN's, so substituting AT's " +
		"condition changes nothing. The divergence that remains is between CEN's published condition and " +
		"this package's core implementation of it — a separate defect, in the direction of reporting " +
		"less than CEN does, on a line BR-22 already reports.",
}

// TestConditionOverridesApplyOnlyUnderTheirCIUS is the containment guard. A caller
// who asked for EN 16931, Peppol, XRechnung or another CIUS gets CEN's condition,
// and so does a CII document under ValidateCIUSPT — AT/eSPap publishes a UBL
// binding and no CII one.
func TestConditionOverridesApplyOnlyUnderTheirCIUS(t *testing.T) {
	doc := []byte(ptFixtureFor(t, "BR-S-02"))
	others := map[string]validator{
		"Validate":          withProfile(ProfileEN16931),
		"ValidatePeppol":    ValidatePeppol,
		"ValidateXRechnung": ValidateXRechnung,
		"ValidateNLCIUS":    ValidateNLCIUS,
		"ValidateCIUSRO":    ValidateCIUSRO,
		"ValidateUBLBE":     ValidateUBLBE,
		"ValidateSRBDT":     ValidateSRBDT,
	}
	for name, v := range others {
		rep, err := v(context.Background(), doc)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, f := range rep.Violations {
			if f.Reading != SourceNone {
				t.Errorf("%s reported %s with Reading %q; only the validator for that CIUS may "+
					"substitute its authority's condition", name, f.Rule, f.Reading)
			}
			if f.Rule == "BR-S-02" {
				t.Errorf("%s reports BR-S-02 on a document whose only standard-rated line is coded "+
					"'NOR'; that is AT/eSPap's reading and no other rule set's", name)
			}
		}
	}

	// A CII invoice through ValidateCIUSPT keeps CEN's conditions throughout.
	cii, err := ValidateCIUSPT(context.Background(), []byte(ptCIIFixture))
	if err != nil {
		t.Fatalf("ValidateCIUSPT on CII: %v", err)
	}
	for _, f := range cii.Violations {
		if f.Reading != SourceNone {
			t.Errorf("ValidateCIUSPT substituted AT/eSPap's condition for %s on a CII document; "+
				"AT/eSPap publishes a UBL binding and no CII one", f.Rule)
		}
	}
}

// TestUnappliedConditionOverridesAreNamed keeps a known-and-unapplied override from
// becoming a silent one. UBL.BE's five BR-*-08 conditions and two BR-CL-* ones are
// its own and are not evaluated; the reason is recorded beside them rather than left
// to a commit message.
func TestUnappliedConditionOverridesAreNamed(t *testing.T) {
	for _, v := range ciusCENCopyVerdicts {
		if len(v.own) == 0 {
			if !v.applied {
				t.Errorf("%s has nothing of its own and is recorded as not applied", v.authority)
			}
			if v.notApplied != "" {
				t.Errorf("%s has nothing of its own and carries a reason for not applying it", v.authority)
			}
			continue
		}
		if v.applied == (v.notApplied != "") {
			t.Errorf("%s: applied=%v and notApplied=%q disagree; an authority whose overrides are not "+
				"applied must say why, and one whose overrides are applied must not",
				v.authority, v.applied, v.notApplied)
		}
		if v.applied {
			set := overrideSetFor(v.source)
			if set == nil {
				t.Errorf("%s is recorded as applied and no override set is wired to %s",
					v.authority, v.source)
				continue
			}
			for id := range v.own {
				if _, ok := set.rules[id]; !ok {
					t.Errorf("%s: %s is the authority's own, the verdict says applied, and the "+
						"override set does not carry it", v.authority, id)
				}
			}
		}
	}
}

func overrideSetFor(s Source) *ciusOverrides {
	if s == SourceCIUSPT {
		return ptOverrides
	}
	return nil
}

// TestConditionOverridesChangeNoCorpusVerdict is the zero-false-positive sweep, and
// it asserts an equality rather than an inequality on purpose.
//
// AT/eSPap's nine conditions and CEN's agree on the *verdict* for every one of the
// 1,690 documents this repository holds: the multiset of (Source, rule, severity)
// each document reports is unchanged, so no document is newly accused, none stops
// being accused, and Conformant() moves for none of them. That is a fact about the
// corpus — nothing in it uses the 'NOR'/'ISE' aliases — and not about the overrides,
// which the fixtures above show do change the verdict on a document that does.
// Pinning it here means a change that starts moving corpus findings is visible
// rather than absorbed.
//
// What does change on 44 of those documents is the *wording*: an overridden finding
// now quotes AT/eSPap's own sentence rather than this package's paraphrase, and
// carries Reading. That is counted rather than asserted away, because it is the one
// output difference a caller can see today.
func TestConditionOverridesChangeNoCorpusVerdict(t *testing.T) {
	files, changed, reworded := 0, 0, 0
	var moved []string
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xml") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		parsed, perr := parseEN16931(newRun(context.Background()), data)
		if perr != nil {
			return nil
		}
		with := validateCIUSPT(newRun(context.Background()), parsed)
		core := validateEN16931(newRun(context.Background()), parsed, ProfileEN16931, ciiBindingCEN)
		without := append(core,
			validateCIUSPTRules(newRun(context.Background()), parsed.inv, parsed.root)...)
		if ccFingerprint(with) != ccFingerprint(without) {
			changed++
			if len(moved) < 5 {
				moved = append(moved, p)
			}
		}
		for _, v := range with {
			if v.Reading != SourceNone {
				reworded++
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "condition-override corpus sweep", files, minCorpusDocuments)
	if changed != 0 {
		t.Errorf("applying AT/eSPap's conditions changes the verdict on %d of %d documents (%v). "+
			"That may be right, but it is a change to what this package says about real invoices and "+
			"belongs in the commit message with its direction measured", changed, files, moved)
	}
	t.Logf("CIUS-PT condition overrides: %d documents swept, %d verdicts changed, %d documents carry "+
		"at least one finding decided by AT/eSPap's condition", files, changed, reworded)
}

// ccFingerprint is a document's verdict: the multiset of (authority, rule,
// severity) it reports. Reading and Message are deliberately outside it — they are
// what an override is *expected* to change, and folding them in would make the sweep
// answer "did anything change" rather than "did the verdict change".
func ccFingerprint(vs []Violation) string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, fmt.Sprintf("%s|%s|%d", v.Source, v.Rule, v.Severity))
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// ptFixtureFor is the fixture for one overridden identifier.
func ptFixtureFor(t *testing.T, rule string) string {
	t.Helper()
	for _, f := range ptOverrideFixtures {
		if f.rule == rule {
			return f.doc
		}
	}
	t.Fatalf("no fixture for %s", rule)
	return ""
}

func ccFindRule(vs []Violation, rule string) *Violation {
	for i := range vs {
		if vs[i].Rule == rule {
			return &vs[i]
		}
	}
	return nil
}

func ccRuleNames(vs []Violation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Rule)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const ptHead = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
         xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
         xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
	<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:2.1.1</cbc:CustomizationID>
	<cbc:ID>INV-1</cbc:ID>
	<cbc:IssueDate>2024-01-31</cbc:IssueDate>
	<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>
	<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
	<cac:AccountingSupplierParty><cac:Party>
		<cbc:EndpointID schemeID="9946">PT500000000</cbc:EndpointID>
		<cac:PartyName><cbc:Name>Seller</cbc:Name></cac:PartyName>
		<cac:PostalAddress><cbc:StreetName>Rua 1</cbc:StreetName><cbc:CityName>Lisboa</cbc:CityName>
			<cbc:PostalZone>1000-001</cbc:PostalZone>
			<cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
		%s
		<cac:PartyLegalEntity><cbc:RegistrationName>Seller</cbc:RegistrationName></cac:PartyLegalEntity>
	</cac:Party></cac:AccountingSupplierParty>
	<cac:AccountingCustomerParty><cac:Party>
		<cbc:EndpointID schemeID="9946">PT500000001</cbc:EndpointID>
		<cac:PostalAddress><cbc:StreetName>Rua 2</cbc:StreetName><cbc:CityName>Porto</cbc:CityName>
			<cbc:PostalZone>4000-001</cbc:PostalZone>
			<cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
		<cac:PartyTaxScheme><cbc:CompanyID>PT500000001</cbc:CompanyID>
			<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
		<cac:PartyLegalEntity><cbc:RegistrationName>Buyer</cbc:RegistrationName></cac:PartyLegalEntity>
	</cac:Party></cac:AccountingCustomerParty>
	<cac:Delivery><cbc:ActualDeliveryDate>2024-01-31</cbc:ActualDeliveryDate></cac:Delivery>
	<cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>
		<cac:PayeeFinancialAccount><cbc:ID>PT50000000000000000000000</cbc:ID></cac:PayeeFinancialAccount>
	</cac:PaymentMeans>
	%s
	<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">23.00</cbc:TaxAmount>
		<cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount>
			<cbc:TaxAmount currencyID="EUR">23.00</cbc:TaxAmount>
			%s
		</cac:TaxSubtotal>
	</cac:TaxTotal>
	<cac:LegalMonetaryTotal>
		<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
		<cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount>
		<cbc:TaxInclusiveAmount currencyID="EUR">123.00</cbc:TaxInclusiveAmount>
		<cbc:PayableAmount currencyID="EUR">123.00</cbc:PayableAmount>
	</cac:LegalMonetaryTotal>
	%s
</Invoice>`

const ptSellerVAT = `<cac:PartyTaxScheme><cbc:CompanyID>PT500000000</cbc:CompanyID>
			<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>`

const ptStandardCategory = `<cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent>
				<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory>`

const ptStandardLine = `<cac:InvoiceLine><cbc:ID>1</cbc:ID>
		<cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>
		<cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
		<cac:Item><cbc:Name>Thing</cbc:Name>
			<cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>23</cbc:Percent>
				<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
		<cac:Price><cbc:PriceAmount currencyID="EUR">100.00</cbc:PriceAmount></cac:Price>
	</cac:InvoiceLine>`

// ptFixtureNoSellerVAT omits the Seller VAT identifier, which is what BR-S-02..04
// and BR-E-02..04 are about, and adds body — a document-level allowance or charge —
// beside an ordinary standard-rated line.
func ptFixtureNoSellerVAT(body string) string {
	return fmt.Sprintf(ptHead, "", body, ptStandardCategory, ptStandardLine)
}

// ptFixtureNoSellerVATLine is the same without the standard-rated line: body is the
// document's only line. BR-S-02 and BR-E-02 are about the *line* categories, so a
// fixture that kept a category-'S' line beside the aliased one would break CEN's
// condition too and demonstrate nothing.
func ptFixtureNoSellerVATLine(line string) string {
	return fmt.Sprintf(ptHead, "", "", ptStandardCategory, line)
}

// ptFixtureBreakdown replaces the VAT breakdown's category, which is where BR-E-10
// and BR-S-10 are bound.
func ptFixtureBreakdown(category string) string {
	return fmt.Sprintf(ptHead, ptSellerVAT, "", category, ptStandardLine)
}

const ptCIIFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
	<rsm:ExchangedDocumentContext><ram:GuidelineSpecifiedDocumentContextParameter>
		<ram:ID>urn:cen.eu:en16931:2017</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter>
	</rsm:ExchangedDocumentContext>
	<rsm:ExchangedDocument><ram:ID>CII-1</ram:ID><ram:TypeCode>380</ram:TypeCode>
		<ram:IssueDateTime><udt:DateTimeString format="102">20240131</udt:DateTimeString></ram:IssueDateTime>
	</rsm:ExchangedDocument>
	<rsm:SupplyChainTradeTransaction/>
</rsm:CrossIndustryInvoice>`
