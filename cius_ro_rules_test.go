package formalis

import (
	"context"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The guards over the generated half of ANAF's rule set: the drift test against the
// Schematron, the reachability survey, the firing fixtures, and the evidence behind
// every Unevaluable claim.

// roDecodePattern re-derives roRulesPattern from the vendored Schematron, the way
// gen.py does, so the committed table can be compared against the artefact rather
// than believed. It returns the rules in pattern order with every assertion, so the
// omissions are visible here too.
func roDecodePattern(t *testing.T) []struct {
	Context string
	Asserts []struct{ ID, Kind, Test string }
} {
	t.Helper()
	path := filepath.Join("testdata", "cius-ro", "schematron", roVersion, "cius-ro", "RO16931-rules.sch")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pattern struct {
		Rules []struct {
			Context string `xml:"context,attr"`
			Items   []struct {
				XMLName xml.Name
				ID      string `xml:"id,attr"`
				Test    string `xml:"test,attr"`
			} `xml:",any"`
		} `xml:"rule"`
	}
	if err := xml.Unmarshal(data, &pattern); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var out []struct {
		Context string
		Asserts []struct{ ID, Kind, Test string }
	}
	for _, r := range pattern.Rules {
		var rule struct {
			Context string
			Asserts []struct{ ID, Kind, Test string }
		}
		rule.Context = normalizeSpace(r.Context)
		for _, a := range r.Items {
			if a.XMLName.Local != "assert" && a.XMLName.Local != "report" {
				continue
			}
			rule.Asserts = append(rule.Asserts, struct{ ID, Kind, Test string }{
				a.ID, a.XMLName.Local, normalizeSpace(a.Test),
			})
		}
		out = append(out, rule)
	}
	return out
}

// TestCIUSRORulesTableMatchesTheArtefact is the drift test. It compares the
// committed table against the Schematron rule by rule and assertion by assertion,
// in pattern order and in both directions.
//
// The comparison is a string comparison because the table holds ANAF's own XPath
// verbatim: there is no translation step in which a rule could change meaning, so
// there is nothing to argue about here — either the bytes agree or the table is
// stale.
func TestCIUSRORulesTableMatchesTheArtefact(t *testing.T) {
	pub := roDecodePattern(t)
	if pub == nil {
		t.Skip("CIUS-RO Schematron not present; run `make cius-schematron`")
	}
	if len(pub) != len(roRulesPattern.rules) {
		t.Fatalf("the artefact has %d rules and the table has %d. Every <rule> is emitted, including one "+
			"carrying no assertion this generator writes, because a rule with none still claims its nodes "+
			"away from the rules below it — so a count that differs is a shadowing relation that changed. "+
			"Regenerate with `make cius-ro-rules`", len(pub), len(roRulesPattern.rules))
	}
	handled := map[string]bool{}
	for id := range roPublished109 {
		handled[id] = true
	}
	for id := range roCENIdentifiers {
		handled[id] = true
	}
	for i, want := range pub {
		got := roRulesPattern.rules[i]
		if got.context != want.Context {
			t.Errorf("rule %d context\n  artefact: %s\n  table   : %s", i, want.Context, got.context)
			continue
		}
		var wantAsserts []string
		for _, a := range want.Asserts {
			if handled[a.ID] || roUnevaluableAsserts[a.ID] != "" {
				continue
			}
			wantAsserts = append(wantAsserts, a.ID+" "+a.Kind+" "+a.Test)
		}
		var gotAsserts []string
		for _, a := range got.asserts {
			kind := "assert"
			if a.kind == "report" {
				kind = "report"
			}
			gotAsserts = append(gotAsserts, a.id+" "+kind+" "+a.test)
		}
		if strings.Join(wantAsserts, "\n") != strings.Join(gotAsserts, "\n") {
			t.Errorf("rule %d (%s) assertions differ\n  artefact:\n    %s\n  table:\n    %s",
				i, want.Context, strings.Join(wantAsserts, "\n    "), strings.Join(gotAsserts, "\n    "))
		}
	}
	t.Logf("CIUS-RO: %d rules and %d generated assertions match the vendored %s Schematron verbatim",
		len(roRulesPattern.rules), roRules.asserts, roVersion)
}

// TestCIUSRORulesTableIsReadable parses every committed row again and checks the
// pattern this package actually runs holds what the file says it holds.
//
// ptDTMustCompile has already parsed the table at load, so a row this evaluator
// cannot read is a panic before any test runs. What this adds is the count: a table
// that lost half its rows would compile.
func TestCIUSRORulesTableIsReadable(t *testing.T) {
	if roRules.asserts != 90 {
		t.Errorf("the compiled CIUS-RO pattern holds %d assertions, want 90 — 63 BR-RO-L*, 18 BR-DEC-RO-*, "+
			"7 BR-RO-DT* and 2 BR-RO-A*, which is the 96 ANAF publishes less the 6 no processor can report",
			roRules.asserts)
	}
	byFamily := map[string]int{}
	for _, r := range roRulesPattern.rules {
		for _, a := range r.asserts {
			switch {
			case strings.HasPrefix(a.id, "BR-RO-L"):
				byFamily["BR-RO-L"]++
			case strings.HasPrefix(a.id, "BR-DEC-RO-"):
				byFamily["BR-DEC-RO"]++
			case strings.HasPrefix(a.id, "BR-RO-DT"):
				byFamily["BR-RO-DT"]++
			case strings.HasPrefix(a.id, "BR-RO-A"):
				byFamily["BR-RO-A"]++
			default:
				t.Errorf("the generated table holds %s, which is in none of the four families it owns", a.id)
			}
			if _, err := ptDTParse(a.test); err != nil {
				t.Errorf("%s: %s: %v", a.id, a.test, err)
			}
		}
	}
	for fam, want := range map[string]int{"BR-RO-L": 63, "BR-DEC-RO": 18, "BR-RO-DT": 7, "BR-RO-A": 2} {
		if byFamily[fam] != want {
			t.Errorf("the generated table holds %d %s* assertions, want %d", byFamily[fam], fam, want)
		}
	}
	// Every floating branch has at least one step, and every anchored one names a
	// document element this package can meet. A branch that says neither would claim
	// every element in the document.
	for _, r := range roRulesPattern.rules {
		for _, p := range r.paths {
			switch p.root {
			case ptDTFloating:
				if len(p.steps) == 0 {
					t.Errorf("%s has a floating branch with no step", r.context)
				}
			case "Invoice", "CreditNote", "":
			default:
				t.Errorf("%s has a branch anchored at %q, which is not a UBL document element", r.context, p.root)
			}
		}
	}
}

// TestCIUSROUnevaluableAssertsAreDerivedFromTheArtefact is the evidence behind the
// six Unevaluable entries in Coverage(SourceCIUSRO), re-derived from the Schematron
// rather than trusted — the shape TestUBLBE13IsBoundToATautology has for UBL.BE's
// one such rule.
//
// Two kinds, and both are ANAF's slip rather than this package's gap:
//
//   - three rules whose context is claimed by an earlier rule of the same pattern.
//     ISO Schematron gives a node to the first matching rule only, so no conforming
//     processor evaluates them. CEN's CII-DT-010/011/012 are unevaluable for exactly
//     this reason (D10), and PR 14 found three CEN rules in the same state.
//   - two assertions bound to count(.), which counts the context node and is
//     therefore 1. `count(.) <= 50` cannot be false.
//
// If ANAF ever fixes either, this fails, and it should: the rule would become
// evaluable and the coverage entry would turn from a fact about the artefact into a
// gap in this package.
func TestCIUSROUnevaluableAssertsAreDerivedFromTheArtefact(t *testing.T) {
	pub := roDecodePattern(t)
	if pub == nil {
		t.Skip("CIUS-RO Schematron not present; run `make cius-schematron`")
	}
	byID := map[string]struct {
		rule       int
		kind, test string
	}{}
	for i, r := range pub {
		for _, a := range r.Asserts {
			byID[a.ID] = struct {
				rule       int
				kind, test string
			}{i, a.Kind, a.Test}
		}
	}
	// The tautologies, read out of the file.
	for _, id := range []string{"BR-RO-A051", "BR-RO-A052"} {
		a, ok := byID[id]
		if !ok {
			t.Errorf("%s is recorded unevaluable and %s no longer publishes it", id, roVersion)
			continue
		}
		if a.test != "count(.) <= 50" {
			t.Errorf("%s is bound to %q; it is recorded unevaluable because count(.) counts the context node "+
				"and is always 1. A changed binding may have made it evaluable", id, a.test)
		}
	}
	// The shadowed ones, with the earlier rule that claims their nodes. The pairs
	// are stated rather than recomputed so that a reader can check them against the
	// file, and the contexts are compared against the artefact so they cannot rot.
	for _, want := range []struct {
		id                  string
		shadowed, claimedBy int
	}{
		{"BR-DEC-RO-13", 20, 10},
		{"BR-DEC-RO-15", 20, 10},
		{"BR-DEC-RO-23", 22, 15},
		{"BR-RO-L1019", 28, 21},
	} {
		a, ok := byID[want.id]
		if !ok {
			t.Errorf("%s is recorded unevaluable and %s no longer publishes it", want.id, roVersion)
			continue
		}
		if a.rule != want.shadowed {
			t.Errorf("%s is in rule %d of the pattern and this package records it in rule %d; the shadowing "+
				"relation is stated in terms of rule order and the order has changed", want.id, a.rule, want.shadowed)
			continue
		}
		if want.claimedBy >= want.shadowed {
			t.Fatalf("%s: the claiming rule %d is not earlier than %d", want.id, want.claimedBy, want.shadowed)
		}
		outer, inner := pub[want.claimedBy].Context, pub[want.shadowed].Context
		t.Logf("%s: rule %d %q is claimed by rule %d %q", want.id, want.shadowed, inner, want.claimedBy, outer)
	}
	// And the set is exactly the six.
	var got []string
	for id := range roUnevaluableAsserts {
		got = append(got, id)
	}
	sort.Strings(got)
	const want = "BR-DEC-RO-13 BR-DEC-RO-15 BR-DEC-RO-23 BR-RO-A051 BR-RO-A052 BR-RO-L1019"
	if strings.Join(got, " ") != want {
		t.Errorf("the generator recorded %v as unevaluable; the six this package accounts for are %q. A "+
			"seventh means ANAF published a rule nobody can report and nothing said so", got, want)
	}
	// Each one is named in the coverage table, with Unevaluable set.
	named := map[string]bool{}
	for _, f := range Coverage(SourceCIUSRO) {
		if !f.Unevaluable {
			continue
		}
		for id := range roUnevaluableAsserts {
			if strings.Contains(f.Rules, id) {
				named[id] = true
			}
		}
	}
	for id := range roUnevaluableAsserts {
		if !named[id] {
			t.Errorf("%s is unevaluable and no Unevaluable entry of Coverage(SourceCIUSRO) names it; a rule "+
				"the authority published that nobody can report has to be in the record", id)
		}
	}
}

// TestCIUSRORuleContextsAreReachable is requirement two of this rule set's oracle,
// for its generated half: for every assertion, either the corpus reaches its context
// or this package says why not.
//
// A rule that reports nothing over 1,690 documents is not evidence of anything on
// its own. This separates the two readings — asked and answered "yes", versus never
// asked — and it is the check that would catch a context bound to a misspelt element
// name, which a clean sweep cannot.
//
// There is no exception list, because there is nothing to except: every one of the
// ninety is reached.
func TestCIUSRORuleContextsAreReachable(t *testing.T) {
	total := map[string]int{}
	dynamicErrors := 0
	files := 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
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
		doc := &ptDTDoc{root: parsed.root, want: roRules.rootNames}
		for _, e := range ptDTGather(roRules, parsed.root, doc) {
			cr := &roRules.rules[e.rule]
			c := &ptDTCtx{item: ptDTItem{kind: ptKNode, node: e.node}, ancestors: e.ancestors, doc: doc, lets: cr.lets}
			for i := range cr.asserts {
				total[cr.asserts[i].id]++
				if _, aerr := ptDTEvalBool(cr.asserts[i].expr, c); aerr != nil {
					dynamicErrors++
				}
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
	atLeast(t, "CIUS-RO context sweep corpus", files, minCorpusDocuments)

	var unreached []string
	nodes := 0
	for _, r := range roRulesPattern.rules {
		for _, a := range r.asserts {
			nodes += total[a.id]
			if total[a.id] == 0 {
				unreached = append(unreached, a.id)
			}
		}
	}
	sort.Strings(unreached)
	if len(unreached) != 0 {
		t.Errorf("no document in the corpus reaches the context of %v. A rule bound to an element the corpus "+
			"never contains is indistinguishable from a rule bound to a misspelt one", unreached)
	}
	atLeast(t, "CIUS-RO context nodes over the corpus", nodes, minCIUSROContextNodes)
	// The dynamic errors are ANAF's XPath meeting a document it was not written for:
	// normalize-space() over a sequence of more than one node is a type error in
	// XPath 2.0, a reference processor aborts on it, and ptDTRun declines to report
	// the assertion rather than inventing a verdict. Two documents in the whole
	// corpus do it, both NLCIUS fixtures that carry two cbc:RegistrationName
	// elements on purpose.
	if dynamicErrors > 2 {
		t.Errorf("%d assertion evaluations raised an XPath dynamic error over the corpus, want at most 2. "+
			"An error is not a finding — the assertion is skipped — so a rise here is a rule that quietly "+
			"stopped being asked", dynamicErrors)
	}
	t.Logf("CIUS-RO contexts: all %d generated assertions are reached by the corpus, %d context evaluations "+
		"in total, %d dynamic errors", roRules.asserts, nodes, dynamicErrors)
}

// TestCIUSRORulesFireAcrossTheCorpus is the compensating floor. Every FP=0 oracle in
// this suite asserts the *absence* of findings, so a rule set that silently stopped
// firing would leave all of them green.
//
// The number is small compared with CIUS-PT's, and that is the rule set's shape
// rather than a weakness: ANAF's limits are 100 to 1000 characters and two decimal
// places, which real invoices respect. What the corpus does exercise is the one
// limit documents genuinely exceed — BR-RO-L302, the 300-character cap on a free-text
// note. The firing evidence for the other eighty-nine is in roBuiltFixtures.
func TestCIUSRORulesFireAcrossTheCorpus(t *testing.T) {
	fired := map[string]int{}
	findings, files := 0, 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
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
		var vs []Violation
		roValidateRules(r, parsed.root, adder(&vs, SourceCIUSRO))
		for _, v := range vs {
			fired[v.Rule]++
			findings++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-RO firing sweep corpus", files, minCorpusDocuments)
	atLeast(t, "CIUS-RO generated rules firing over the corpus", len(fired), minCIUSRORulesFiring)
	atLeast(t, "CIUS-RO generated findings over the corpus", findings, minCIUSRORuleFindings)
	var ids []string
	for id := range fired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	t.Logf("CIUS-RO generated rules: %d of %d fire somewhere in the corpus (%v), for %d findings",
		len(fired), roRules.asserts, ids, findings)
}

// TestEveryCIUSRORuleFires is requirement three's firing half for the generated
// family, and the reason roBuiltFixtures exists.
func TestEveryCIUSRORuleFires(t *testing.T) {
	fired := roBuiltFixtures()
	var missing []string
	for _, r := range roRulesPattern.rules {
		for _, a := range r.asserts {
			if !fired[a.id] {
				missing = append(missing, a.id)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("no fixture in this repository makes %v fire, so nothing would notice if the rule stopped "+
			"being evaluated. Either the rule cannot be falsified — in which case it belongs in "+
			"roUnevaluableAsserts with the evidence — or the builder needs a case for it", missing)
	}
	t.Logf("all %d generated CIUS-RO assertions have a document that makes them fire", roRules.asserts)
}

// ---------------------------------------------------------------------------
// The firing fixtures
// ---------------------------------------------------------------------------
//
// Built rather than written, for the reason cius_pt_datatype_fixtures_test.go gives
// for CIUS-PT's 290: ninety hand-written mutations would be ninety chances to write
// a fixture that trips a *neighbouring* rule and calls it evidence, and a reviewer
// of such a list cannot check it.
//
// The builder synthesises, for each rule, the smallest document that reaches its
// context, and puts a hostile value at every element and attribute path the rule's
// assertions read — which it takes from the *parsed* expression rather than from the
// XPath text, so a path the evaluator would walk is a path the fixture builds.
//
// What that proves and what it does not:
//
//   - it proves each rule is reachable and falsifiable: its context selects a node
//     in a document built from the context alone, and some value makes the assertion
//     fail. A rule bound to a misspelt element name fails this, and so does one whose
//     test can never be false — which is how BR-RO-A051 and BR-RO-A052 were found.
//   - it does not prove the rule says what ANAF meant. That claim rests on
//     TestCIUSRORulesTableMatchesTheArtefact, where the table holds ANAF's XPath
//     verbatim.

// roBuiltFixtures returns the set of identifiers this repository's synthesised
// documents make fire. Memoised: two tests need it.
var roBuiltFixtures = sync.OnceValue(func() map[string]bool {
	fired := map[string]bool{}
	record := func(doc string) {
		rep, err := ValidateCIUSRO(context.Background(), []byte(doc))
		if err != nil {
			return
		}
		for _, v := range rep.Violations {
			if v.Source == SourceCIUSRO {
				fired[v.Rule] = true
			}
		}
	}
	for _, f := range roHandFixtures {
		record(f.xml)
	}
	for i := range roRulesPattern.rules {
		for _, doc := range roFiringDocs(i) {
			record(doc)
		}
	}
	return fired
})

// roValues is the value battery: values that are too long for every length limit
// ANAF sets, one with three decimal places, and several that are not dates.
var roValues = []string{
	"", strings.Repeat("x", 1001), "1.234", "2024-02-31", "20240115", "2024-01-15+02:00",
}

// roFiringDocs synthesises the candidate documents for one rule.
func roFiringDocs(rule int) []string {
	src := roRulesPattern.rules[rule]
	if len(src.asserts) == 0 {
		return nil
	}
	// The relative paths the rule's assertions read, taken from the compiled
	// expressions. "" stands for the context node itself, which is what `.` and
	// text() select.
	targets := map[string]bool{}
	for i := range roRules.rules[rule].asserts {
		roCollectTargets(roRules.rules[rule].asserts[i].expr, targets)
	}
	var docs []string
	for _, p := range src.paths {
		for target := range targets {
			for _, v := range roValues {
				docs = append(docs, roBuildDoc(p, target, v))
			}
		}
	}
	return docs
}

// roCollectTargets walks a parsed assertion and records every relative element path
// it reads, as "A/B" or "A/B@attr", plus "" for the context node itself.
func roCollectTargets(e *ptDTExpr, into map[string]bool) {
	if e == nil {
		return
	}
	if e.op == ptOpPath && !e.path.fromRoot && e.path.up == 0 {
		var names []string
		ok := true
		for i, s := range e.path.steps {
			switch {
			case s.self && i == 0 && len(e.path.steps) == 1:
				into[""] = true
				return
			case s.attr != "" && i == len(e.path.steps)-1:
				names = append(names, "@"+s.attr)
			case s.name != "":
				names = append(names, s.name)
			default:
				ok = false
			}
		}
		if ok && len(names) > 0 {
			into[strings.Join(names, "/")] = true
		}
	}
	if e.op == ptOpPath && e.path != nil {
		for _, s := range e.path.steps {
			roCollectTargets(s.call, into)
			for _, pr := range s.preds {
				roCollectTargets(pr, into)
			}
		}
	}
	for _, a := range e.args {
		roCollectTargets(a, into)
	}
}

// roBuildDoc writes the smallest document whose only content is the path to the
// rule's context, with target (a path relative to it, possibly empty, possibly
// ending in an attribute) carrying value.
func roBuildDoc(p ptDTCtxPath, target, value string) string {
	root := p.root
	if root == "" || root == ptDTFloating {
		root = "Invoice"
	}
	ns := "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	if root == "CreditNote" {
		ns = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	}
	var b strings.Builder
	b.WriteString(`<` + root + ` xmlns="` + ns + `" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">`)
	for _, st := range p.steps {
		b.WriteString("<" + st.name + ">" + roPredContent(st.pred))
	}
	b.WriteString(roLeaf(target, value))
	for i := len(p.steps) - 1; i >= 0; i-- {
		b.WriteString("</" + p.steps[i].name + ">")
	}
	b.WriteString(`</` + root + `>`)
	return b.String()
}

// roLeaf writes the target path under the context element.
func roLeaf(target, value string) string {
	if target == "" {
		return roEscape(value)
	}
	names := strings.Split(target, "/")
	attr := ""
	if last := names[len(names)-1]; strings.HasPrefix(last, "@") {
		attr = last[1:]
		names = names[:len(names)-1]
		if len(names) == 0 {
			return ""
		}
	}
	var b strings.Builder
	for i, n := range names {
		b.WriteString("<" + n)
		if i == len(names)-1 && attr != "" {
			b.WriteString(` ` + attr + `="` + roEscape(value) + `"`)
		}
		b.WriteString(">")
		if i == len(names)-1 && attr == "" {
			b.WriteString(roEscape(value))
		}
	}
	for i := len(names) - 1; i >= 0; i-- {
		b.WriteString("</" + names[i] + ">")
	}
	return b.String()
}

// roPredContent writes a child that satisfies a context predicate. ANAF uses two,
// `cbc:ChargeIndicator = false()` and its true() twin.
func roPredContent(pred string) string {
	switch {
	case pred == "":
		return ""
	case strings.Contains(pred, "ChargeIndicator") && strings.Contains(pred, "false"):
		return "<ChargeIndicator>false</ChargeIndicator>"
	case strings.Contains(pred, "ChargeIndicator") && strings.Contains(pred, "true"):
		return "<ChargeIndicator>true</ChargeIndicator>"
	}
	return ""
}

func roEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// roHandFixtures are the documents the builder cannot infer: the two occurrence
// limits, whose firing case is a *count* of children rather than a hostile value in
// one of them.
var roHandFixtures = []struct{ want, xml string }{
	// BR-RO-A020: count(cbc:Note) <= 20 at the document element.
	{"BR-RO-A020", roHead() + strings.Repeat("<cbc:Note>n</cbc:Note>", 21) + `</Invoice>`},
	// BR-RO-A500: count(cac:InvoiceDocumentReference) <= 500 inside one
	// cac:BillingReference. UBL's own schema allows at most one, so no schema-valid
	// document reaches it — but Schematron does not read the schema, and a rule that
	// can be broken by a well-formed document is evaluable.
	{"BR-RO-A500", roHead() + `<cac:BillingReference>` +
		strings.Repeat(`<cac:InvoiceDocumentReference><cbc:ID>1</cbc:ID></cac:InvoiceDocumentReference>`, 501) +
		`</cac:BillingReference></Invoice>`},
}

// The generated family joins ciusEvaluated, which is what puts it inside
// TestCIUSSeveritiesQuoteTheirAuthority (every emitted severity compared against
// the artefact's flag, both directions), TestCIUSFindingsStayInsideTheEvaluatedSet
// (nothing emitted that the table does not name) and
// TestEveryPublishedCIUSRuleIsEvaluatedOrDisclaimed.
//
// Merged rather than pasted, for the reason cius_pt_datatype_test.go gives: a
// second copy of a generated list is a copy that can disagree with the first, which
// is the state C34's two phantom Peppol entries were in. Package-level variables are
// initialised before any init function runs, so ciusEvaluated is populated by the
// time this executes.
func init() {
	for _, r := range roRulesPattern.rules {
		for _, a := range r.asserts {
			// Every identifier ANAF publishes is flagged fatal, which
			// TestCIUSROVersionsDiffer checks against the artefact for all four
			// releases rather than assuming here.
			ciusEvaluated[SourceCIUSRO][a.id] = SeverityFatal
		}
	}
}

func roHead() string {
	return `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" ` +
		`xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" ` +
		`xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">`
}
