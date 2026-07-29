package formalis

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// treeDetectors is what the twelve predicates did before scanShape: parse the
// whole document into a ciiNode tree and read a field or two off the root.
//
// It is kept here as the reference the streaming scan is checked against. The
// point of the change was to stop building this tree, not to answer differently
// — including on documents nobody would call well-formed — so any divergence is
// a regression, and TestScanMatchesTreeDetection below is what says so.
var treeDetectors = map[string]func(*ciiNode) bool{
	"IsFacturae":    func(r *ciiNode) bool { return r.name == "Facturae" },
	"IsFatturaPA":   func(r *ciiNode) bool { return r.name == "FatturaElettronica" },
	"IsOSA":         func(r *ciiNode) bool { return r.name == "InvoiceData" },
	"IsFinvoice":    func(r *ciiNode) bool { return r.name == "Finvoice" },
	"IsTEAPPS":      func(r *ciiNode) bool { return r.name == "INVOICE_CENTER" },
	"IsKSeF":        func(r *ciiNode) bool { return r.name == "Faktura" && r.child("Naglowek") != nil },
	"IsEbInterface": func(r *ciiNode) bool { return r.name == "Invoice" && r.child("Biller") != nil },
	"IsSvefaktura":  func(r *ciiNode) bool { return r.name == "Invoice" && r.child("SellerParty") != nil },
	"IsOIOUBL": func(r *ciiNode) bool {
		return r.name == "Invoice" && strings.Contains(r.str("CustomizationID"), "OIOUBL")
	},
	"IsPINT": func(r *ciiNode) bool {
		if r.name != "Invoice" && r.name != "CreditNote" {
			return false
		}
		return strings.Contains(r.str("CustomizationID"), "peppol:pint")
	},
	"IsTurkishInvoice": func(r *ciiNode) bool {
		return r.name == "Invoice" &&
			strings.HasPrefix(strings.ToUpper(strings.TrimSpace(r.str("CustomizationID"))), "TR")
	},
	"IsZATCA": func(r *ciiNode) bool {
		if r.name != "Invoice" && r.name != "CreditNote" {
			return false
		}
		return strings.Contains(strings.ToLower(r.str("ProfileID")), "reporting") || zatcaDocRef(r, "ICV")
	},
}

var exportedDetectors = map[string]func([]byte) (bool, error){
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

// checkParity runs every predicate against the tree reference for one document
// and reports any disagreement, including on whether the document is readable
// at all.
func checkParity(t *testing.T, label string, data []byte) {
	t.Helper()
	root, treeErr := parseCII(newRun(nil), data)
	for name, fn := range exportedDetectors {
		got, gotErr := fn(data)
		if (treeErr != nil) != (gotErr != nil) {
			t.Errorf("%s: %s readability differs: tree err=%v, scan err=%v", label, name, treeErr, gotErr)
			continue
		}
		if treeErr != nil {
			continue
		}
		if want := treeDetectors[name](root); got != want {
			t.Errorf("%s: %s = %v, tree says %v", label, name, got, want)
		}
	}
}

// TestScanMatchesTreeDetection is the safety net for replacing the tree with a
// streaming scan: over every document in testdata, all twelve predicates must
// give the answer the tree gave, and must agree with it on whether the document
// could be read.
func TestScanMatchesTreeDetection(t *testing.T) {
	files := 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		files++
		checkParity(t, p, data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The corpora are not vendored, so this is a skip rather than a failure —
	// the same convention as every other corpus-backed test here. The awkward
	// input below carries the contract when the corpus is absent.
	if files == 0 {
		t.Skip("no corpus present (make cius-oracles / make en16931-artefacts)")
	}
	t.Logf("checked %d documents against the tree reference", files)
}

// TestScanMatchesTreeOnAwkwardInput covers the shapes the corpus does not: the
// leniencies of Go's XML decoder, and the places the scan has to reproduce an
// accident of how parseCII built its tree rather than an intended rule.
func TestScanMatchesTreeOnAwkwardInput(t *testing.T) {
	docs := map[string]string{
		"plain facturae":  `<?xml version="1.0"?><Facturae><FileHeader/></Facturae>`,
		"unrelated root":  `<?xml version="1.0"?><SomeUnrelatedRoot/>`,
		"ksef with head":  `<Faktura><Naglowek><KodFormularza>FA</KodFormularza></Naglowek></Faktura>`,
		"ksef no head":    `<Faktura><Inne/></Faktura>`,
		"ebinterface":     `<Invoice><Biller><VATIdentificationNumber>ATU1</VATIdentificationNumber></Biller></Invoice>`,
		"svefaktura":      `<Invoice><SellerParty/></Invoice>`,
		"oioubl":          `<Invoice><CustomizationID>urn:OIOUBL:2.02</CustomizationID></Invoice>`,
		"pint":            `<Invoice><CustomizationID>urn:peppol:pint:billing-1</CustomizationID></Invoice>`,
		"pint creditnote": `<CreditNote><CustomizationID>urn:peppol:pint:billing-1</CustomizationID></CreditNote>`,
		"turkish":         `<Invoice><CustomizationID> tr1.2 </CustomizationID></Invoice>`,
		"zatca profile":   `<Invoice><ProfileID>reporting:1.0</ProfileID></Invoice>`,
		"zatca icv":       `<Invoice><AdditionalDocumentReference><ID>ICV</ID></AdditionalDocumentReference></Invoice>`,

		// The distinguishing child is only a *direct* child of the root, so a
		// deeper one must not count.
		"biller nested deeper": `<Invoice><Delivery><Biller/></Delivery></Invoice>`,
		"customization nested": `<Invoice><X><CustomizationID>urn:OIOUBL:2.02</CustomizationID></X></Invoice>`,

		// child() and str() take the first match; a second must be ignored.
		"two customization ids": `<Invoice><CustomizationID>urn:peppol:pint:x</CustomizationID>` +
			`<CustomizationID>urn:OIOUBL:2.02</CustomizationID></Invoice>`,
		"two profile ids": `<Invoice><ProfileID>other</ProfileID><ProfileID>reporting</ProfileID></Invoice>`,
		"two doc ref ids": `<Invoice><AdditionalDocumentReference><ID>QR</ID><ID>ICV</ID>` +
			`</AdditionalDocumentReference></Invoice>`,

		// zatcaDocRef searched the whole tree, so a nested reference counts...
		"nested doc ref": `<Invoice><Wrapper><AdditionalDocumentReference><ID>ICV</ID>` +
			`</AdditionalDocumentReference></Wrapper></Invoice>`,
		// ...but the ID has to be its direct child.
		"doc ref id deeper": `<Invoice><AdditionalDocumentReference><Sub><ID>ICV</ID></Sub>` +
			`</AdditionalDocumentReference></Invoice>`,
		"second doc ref matches": `<Invoice><AdditionalDocumentReference><ID>QR</ID></AdditionalDocumentReference>` +
			`<AdditionalDocumentReference><ID>ICV</ID></AdditionalDocumentReference></Invoice>`,

		// Text split by markup, and text belonging to a descendant rather than
		// to the element itself.
		"split by comment":  `<Invoice><CustomizationID>urn:OIO<!--x-->UBL:2.02</CustomizationID></Invoice>`,
		"split by cdata":    `<Invoice><CustomizationID>urn:<![CDATA[OIOUBL]]>:2.02</CustomizationID></Invoice>`,
		"whitespace around": "<Invoice><CustomizationID>\n\t urn:OIOUBL:2.02 \n</CustomizationID></Invoice>",
		"text in descendant": `<Invoice><CustomizationID><Inner>urn:OIOUBL:2.02</Inner></CustomizationID>` +
			`</Invoice>`,

		// Multiple top-level elements: the decoder allows them and parseCII kept
		// the last, so detection must too — including discarding what it
		// gathered from the earlier one.
		"two roots":              `<Facturae/><Invoice/>`,
		"two roots reversed":     `<Invoice/><Facturae/>`,
		"first root had biller":  `<Invoice><Biller/></Invoice><Invoice/>`,
		"second root has biller": `<Invoice/><Invoice><Biller/></Invoice>`,
		"first root had icv": `<Invoice><AdditionalDocumentReference><ID>ICV</ID>` +
			`</AdditionalDocumentReference></Invoice><Invoice/>`,

		// Unclosed elements never see an EndElement; parseCII materialised
		// their text anyway.
		"unclosed with text": `<Invoice><CustomizationID>urn:OIOUBL:2.02`,
		"unclosed root":      `<Invoice><Biller/>`,

		// Unreadable in various ways.
		"truncated":       `<?xml version="1.0"?><Facturae><FileHeader><Schema`,
		"mismatched end":  `<?xml version="1.0"?><Facturae><a></b></Facturae>`,
		"empty":           ``,
		"not xml":         "%PDF-1.7\x00binary",
		"only a comment":  `<!-- nothing here -->`,
		"bad charset":     `<?xml version="1.0" encoding="EBCDIC-CP-BE"?><Facturae/>`,
		"latin-1 charset": `<?xml version="1.0" encoding="ISO-8859-1"?><Invoice><Biller/></Invoice>`,
	}
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) { checkParity(t, name, []byte(doc)) })
	}
}

// flatDoc builds a well-formed document of n sibling elements under an Invoice
// root — the shape that costs about 170 bytes per four input bytes once it is
// turned into a tree.
func flatDoc(n int) []byte {
	var b strings.Builder
	b.Grow(n*4 + 64)
	b.WriteString(`<?xml version="1.0"?><Invoice>`)
	for i := 0; i < n; i++ {
		b.WriteString(`<a/>`)
	}
	b.WriteString(`</Invoice>`)
	return []byte(b.String())
}

// TestDetectionMemoryDoesNotScaleWithInput is the teeth for the O(1) probe.
//
// The predicates used to hand back the root of a fully materialised tree, so
// the memory still live when detection returned was proportional to the element
// count — which is what made IsFacturae the cheapest way to reach a
// multi-gigabyte allocation. scanShape retains only the open elements and a few
// strings, so growing the document eightfold must not grow what it retains.
//
// Reverting scanShape to `parseCII` fails this: the tree for the larger
// document is eight times the smaller one and both stay reachable through the
// returned root.
func TestDetectionMemoryDoesNotScaleWithInput(t *testing.T) {
	retained := func(n int) uint64 {
		data := flatDoc(n)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		d, err := scanShape(data)
		if err != nil {
			t.Fatalf("scanShape: %v", err)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		// Keep the result reachable across the measurement, exactly as the
		// tree-returning version kept its root reachable.
		runtime.KeepAlive(d)
		runtime.KeepAlive(data)
		if after.HeapAlloc < before.HeapAlloc {
			return 0
		}
		return after.HeapAlloc - before.HeapAlloc
	}

	const small, large = 50_000, 400_000
	// The input itself is reachable in both measurements and is 4 bytes per
	// element, so subtract nothing and simply require that the retained total
	// does not grow with the element count beyond the input's own contribution.
	smallRetained := retained(small)
	largeRetained := retained(large)

	// A tree costs ~170 B/element; the scan costs none. Allow a generous
	// absolute slack for the input buffer and measurement noise, then require
	// the large case to stay within it too.
	const slack = 8 << 20 // 8 MiB, far below the ~68 MiB a 400k-element tree needs
	t.Logf("retained: %d elements -> %d B, %d elements -> %d B", small, smallRetained, large, largeRetained)
	if largeRetained > slack {
		t.Errorf("detecting a %d-element document retained %d B; a streaming scan should retain almost nothing",
			large, largeRetained)
	}
	if smallRetained > slack {
		t.Errorf("detecting a %d-element document retained %d B", small, smallRetained)
	}
}

// TestNodeBudgetStopsTheTree pins the second half of the fix: maxDepth bounds
// nesting, and this bounds breadth. A flat document of millions of siblings has
// a depth of 2, so nothing maxDepth does engages, and every sibling still
// becomes a node.
func TestNodeBudgetStopsTheTree(t *testing.T) {
	data := flatDoc(maxNodes + 1)

	v := Validate(context.Background(), data, ProfileEN16931).Violations

	// A stopped run never returns an empty result, or a caller keying on
	// len(v) == 0 reads it as a clean invoice.
	if len(v) == 0 {
		t.Fatal("a document over the node budget returned no violations at all")
	}
	var limits, syntax int
	for _, x := range v {
		switch {
		case IsCheckerViolation(x):
			limits++
		case x.Rule == RuleSyntax:
			syntax++
		}
	}
	if limits != 1 {
		t.Errorf("got %d %q violations, want exactly 1: %v", limits, RuleLimit, v)
	}
	// The document is well-formed. Reporting it as bad XML would be the false
	// accusation syntaxViolation exists to prevent.
	if syntax != 0 {
		t.Errorf("a well-formed document over the budget was reported as %d syntax violations: %v", syntax, v)
	}
	found := false
	for _, x := range v {
		if IsCheckerViolation(x) && strings.Contains(x.Message, "xml-node-count") {
			found = true
		}
	}
	if !found {
		t.Errorf("the limit violation does not name the guard that tripped: %v", v)
	}
}

// TestNodeBudgetLeavesRealInvoicesAlone checks the margin is real rather than
// asserted in a comment: no document in the corpus comes near the budget, and
// a document just under it still validates without tripping.
func TestNodeBudgetLeavesRealInvoicesAlone(t *testing.T) {
	worst, worstPath := 0, ""
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		r := newRun(nil)
		if _, err := parseCII(r, data); err != nil {
			return nil
		}
		if used := maxNodes - r.nodes; used > worst {
			worst, worstPath = used, p
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Not vendored, so the margin can only be re-derived where the corpus is.
	if worst == 0 {
		t.Log("no corpus present (make cius-oracles); the margin was not re-derived")
	} else {
		t.Logf("largest corpus document: %d elements (%s), budget %d — %.0fx margin",
			worst, worstPath, maxNodes, float64(maxNodes)/float64(worst))
		if worst*20 > maxNodes {
			t.Errorf("the largest corpus document uses %d of the %d element budget; the margin is too thin",
				worst, maxNodes)
		}
	}

	// A document just inside the budget parses cleanly. This needs no corpus.
	r := newRun(nil)
	if _, err := parseCII(r, flatDoc(maxNodes-2)); err != nil {
		t.Errorf("a document inside the budget failed to parse: %v", err)
	}
	if len(r.trips) != 0 {
		t.Errorf("a document inside the budget tripped a guard: %v", r.trips)
	}
}

// TestDetectionHasNoBudgetToTrip records why the scan needs no node budget: it
// builds no tree, so the document that exhausts the parser's budget is answered
// by every predicate without difficulty.
func TestDetectionHasNoBudgetToTrip(t *testing.T) {
	data := flatDoc(maxNodes + 1)
	for name, fn := range exportedDetectors {
		got, err := fn(data)
		if err != nil {
			t.Errorf("%s on a %d-element document: %v; detection builds no tree and has nothing to trip",
				name, maxNodes+1, err)
		}
		// The root is Invoice, so only the root-name-only predicates are false
		// for an uninteresting reason; none should be true here.
		if got {
			t.Errorf("%s = true on a document of empty siblings", name)
		}
	}
}
