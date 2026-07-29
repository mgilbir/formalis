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
	// The three identifier tests are written out here rather than calling
	// declaresSpecID, so that this stays a reference the implementation is
	// checked against rather than a copy of it. They are the marks of the
	// OIOUBL, PINT and UBL-TR entries of specIDRules, matched the way that table
	// says: lower-cased and trimmed.
	"IsOIOUBL": func(r *ciiNode) bool {
		return r.name == "Invoice" && strings.Contains(strings.ToLower(r.str("CustomizationID")), "oioubl")
	},
	"IsPINT": func(r *ciiNode) bool {
		if r.name != "Invoice" && r.name != "CreditNote" {
			return false
		}
		id := strings.ToLower(r.str("CustomizationID"))
		return strings.Contains(id, "peppol:pint") || strings.Contains(id, "fdc:peppol:jp:billing")
	},
	"IsTurkishInvoice": func(r *ciiNode) bool {
		return r.name == "Invoice" &&
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.str("CustomizationID"))), "tr")
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

// treeShapeFacts is the tree reference for every string scanShape retains, as
// opposed to every predicate that reads one.
//
// The distinction started to matter when the scan grew a capture the twelve
// predicates do not use: the CII Specification identifier, which sits three
// elements below the root rather than directly under it, and which Detect needs
// so a caller can ask which CIUS a document declares. A field no predicate reads
// is a field the predicate parity check cannot see, so it gets a reference of
// its own here and the check below holds the scan to it over the whole corpus.
var treeShapeFacts = map[string]func(*ciiNode) string{
	"root":            func(r *ciiNode) string { return r.name },
	"CustomizationID": func(r *ciiNode) string { return r.str("CustomizationID") },
	"ProfileID":       func(r *ciiNode) string { return r.str("ProfileID") },
	"CII specification identifier": func(r *ciiNode) string {
		return r.str("ExchangedDocumentContext", "GuidelineSpecifiedDocumentContextParameter", "ID")
	},
	"BT-24": treeSpecID,
}

// scanShapeFacts is the same list read off the streaming scan.
var scanShapeFacts = map[string]func(*docShape) string{
	"root":                         func(d *docShape) string { return d.root },
	"CustomizationID":              func(d *docShape) string { return d.str("CustomizationID") },
	"ProfileID":                    func(d *docShape) string { return d.str("ProfileID") },
	"CII specification identifier": func(d *docShape) string { return strings.TrimSpace(d.ciiSpecID) },
	"BT-24":                        (*docShape).specID,
}

// treeSpecID is the Specification identifier the semantic mappers themselves
// produce from the tree — en16931Invoice.specID, the exact string ValidateCIUS
// dispatches on.
//
// It is deliberately not a re-implementation of the path: pinning
// docShape.specID to what mapCII and mapUBL actually read is what makes
// Detection.SpecID the answer the dispatcher would have got, rather than a
// second reading of the document that could drift from it.
func treeSpecID(root *ciiNode) string {
	switch root.name {
	case "CrossIndustryInvoice":
		return mapCII(root).specID
	case "Invoice", "CreditNote":
		return mapUBL(root).specID
	}
	return ""
}

// checkParity runs every predicate against the tree reference for one document
// and reports any disagreement, including on whether the document is readable
// at all. It then does the same for every string the scan retains, and for
// Detect's readability answer.
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

	d, scanErr := scanShape(data)
	if (treeErr != nil) != (scanErr != nil) {
		t.Errorf("%s: scanShape readability differs: tree err=%v, scan err=%v", label, treeErr, scanErr)
		return
	}
	if _, detErr := Detect(data); (treeErr != nil) != (detErr != nil) {
		t.Errorf("%s: Detect readability differs: tree err=%v, Detect err=%v", label, treeErr, detErr)
	}
	if treeErr != nil {
		return
	}
	for name, fromTree := range treeShapeFacts {
		if got, want := scanShapeFacts[name](d), fromTree(root); got != want {
			t.Errorf("%s: %s = %q, tree says %q", label, name, got, want)
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
			// Skipping a document silently would shrink the population this
			// parity claim is made over without saying so.
			t.Errorf("%s: %v", p, err)
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
	atLeast(t, "scan/tree parity corpus", files, minCorpusDocuments)
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

		// The CII Specification identifier: three elements down, and reached by
		// child() at every step, so a first match that leads nowhere is not
		// backtracked out of.
		"cii spec id": ciiContext(`<GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017</ID>` +
			`</GuidelineSpecifiedDocumentContextParameter>`),
		"cii spec id after the process parameter": ciiContext(
			`<BusinessProcessSpecifiedDocumentContextParameter><ID>A1</ID></BusinessProcessSpecifiedDocumentContextParameter>` +
				`<GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017</ID>` +
				`</GuidelineSpecifiedDocumentContextParameter>`),
		"cii two guideline parameters, first empty": ciiContext(
			`<GuidelineSpecifiedDocumentContextParameter/>` +
				`<GuidelineSpecifiedDocumentContextParameter><ID>urn:x</ID></GuidelineSpecifiedDocumentContextParameter>`),
		"cii two ids under the parameter": ciiContext(
			`<GuidelineSpecifiedDocumentContextParameter><ID>first</ID><ID>second</ID>` +
				`</GuidelineSpecifiedDocumentContextParameter>`),
		"cii id nested deeper": ciiContext(
			`<GuidelineSpecifiedDocumentContextParameter><Wrap><ID>urn:x</ID></Wrap>` +
				`</GuidelineSpecifiedDocumentContextParameter>`),
		"cii two contexts, first empty": `<CrossIndustryInvoice><ExchangedDocumentContext/>` +
			`<ExchangedDocumentContext><GuidelineSpecifiedDocumentContextParameter><ID>urn:x</ID>` +
			`</GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext></CrossIndustryInvoice>`,
		"cii parameter directly under the root": `<CrossIndustryInvoice>` +
			`<GuidelineSpecifiedDocumentContextParameter><ID>urn:x</ID>` +
			`</GuidelineSpecifiedDocumentContextParameter></CrossIndustryInvoice>`,
		"cii context nested deeper": `<CrossIndustryInvoice><Wrap>` + ciiContextBody(
			`<GuidelineSpecifiedDocumentContextParameter><ID>urn:x</ID></GuidelineSpecifiedDocumentContextParameter>`) +
			`</Wrap></CrossIndustryInvoice>`,
		"cii spec id unclosed": `<CrossIndustryInvoice><ExchangedDocumentContext>` +
			`<GuidelineSpecifiedDocumentContextParameter><ID>urn:cen.eu:en16931:2017`,
		// The same path under a UBL root: the scan captures it, but BT-24 for an
		// Invoice root is the CustomizationID, and mapUBL never looks here.
		"cii path under a ubl root": `<Invoice><ExchangedDocumentContext>` +
			`<GuidelineSpecifiedDocumentContextParameter><ID>urn:x</ID>` +
			`</GuidelineSpecifiedDocumentContextParameter></ExchangedDocumentContext></Invoice>`,
		// A CII root carrying a CustomizationID: mapCII does not read one.
		"cii root with a customization id": `<CrossIndustryInvoice>` +
			`<CustomizationID>urn:OIOUBL:2.02</CustomizationID></CrossIndustryInvoice>`,

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

// ciiContextBody wraps body in an ExchangedDocumentContext; ciiContext wraps
// that in a CrossIndustryInvoice root. They keep the cases above readable.
func ciiContextBody(body string) string {
	return `<ExchangedDocumentContext>` + body + `</ExchangedDocumentContext>`
}

func ciiContext(body string) string {
	return `<CrossIndustryInvoice>` + ciiContextBody(body) + `</CrossIndustryInvoice>`
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
//
// Detect is measured on the same terms. It is the entry point a router reaches
// for first, on bytes nothing has vetted yet, so it is the one that must not be
// the cheapest way to allocate a gigabyte; a Detect that parsed a tree to answer
// would give back everything scanShape was written to save.
func TestDetectionMemoryDoesNotScaleWithInput(t *testing.T) {
	retained := func(n int, read func([]byte) any) uint64 {
		data := flatDoc(n)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		got := read(data)
		runtime.GC()
		runtime.ReadMemStats(&after)
		// Keep the result reachable across the measurement, exactly as the
		// tree-returning version kept its root reachable.
		runtime.KeepAlive(got)
		runtime.KeepAlive(data)
		if after.HeapAlloc < before.HeapAlloc {
			return 0
		}
		return after.HeapAlloc - before.HeapAlloc
	}

	readers := map[string]func([]byte) any{
		"scanShape": func(b []byte) any {
			d, err := scanShape(b)
			if err != nil {
				t.Fatalf("scanShape: %v", err)
			}
			return d
		},
		"Detect": func(b []byte) any {
			d, err := Detect(b)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			return d
		},
	}

	const small, large = 50_000, 400_000
	for name, read := range readers {
		// The input itself is reachable in both measurements and is 4 bytes per
		// element, so subtract nothing and simply require that the retained total
		// does not grow with the element count beyond the input's own contribution.
		smallRetained := retained(small, read)
		largeRetained := retained(large, read)

		// A tree costs ~170 B/element; the scan costs none. Allow a generous
		// absolute slack for the input buffer and measurement noise, then require
		// the large case to stay within it too.
		const slack = 8 << 20 // 8 MiB, far below the ~68 MiB a 400k-element tree needs
		t.Logf("%s retained: %d elements -> %d B, %d elements -> %d B", name, small, smallRetained, large, largeRetained)
		if largeRetained > slack {
			t.Errorf("%s on a %d-element document retained %d B; a streaming scan should retain almost nothing",
				name, large, largeRetained)
		}
		if smallRetained > slack {
			t.Errorf("%s on a %d-element document retained %d B", name, small, smallRetained)
		}
	}
}

// TestNodeBudgetStopsTheTree pins the second half of the fix: maxDepth bounds
// nesting, and this bounds breadth. A flat document of millions of siblings has
// a depth of 2, so nothing maxDepth does engages, and every sibling still
// becomes a node.
func TestNodeBudgetStopsTheTree(t *testing.T) {
	data := flatDoc(maxNodes + 1)

	v := findings(t, context.Background(), withProfile(ProfileEN16931), data)

	// A stopped run never returns an empty result, or a caller keying on
	// len(v) == 0 reads it as a clean invoice.
	if len(v) == 0 {
		t.Fatal("a document over the node budget returned no violations at all")
	}
	var limits, rootRefusals int
	for _, x := range v {
		switch {
		case IsCheckerViolation(x):
			limits++
		case x.Rule == RuleRoot:
			rootRefusals++
		}
	}
	if limits != 1 {
		t.Errorf("got %d %q violations, want exactly 1: %v", limits, RuleLimit, v)
	}
	// The document is well-formed, and it is a UBL Invoice. Refusing it as
	// unreadable or as the wrong kind of document would be the false accusation
	// readFailure exists to prevent.
	if rootRefusals != 0 {
		t.Errorf("a well-formed UBL invoice over the budget was refused as the wrong kind of document %d times: %v", rootRefusals, v)
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
	worst, worstPath, files := 0, "", 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		files++
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
	// Where it is, the margin is a claim about the whole corpus and has to be
	// measured over the whole corpus: "no document here comes near the budget"
	// is worth nothing if "here" is three documents.
	if worst == 0 {
		t.Log("no corpus present (make cius-oracles); the margin was not re-derived")
	} else {
		atLeast(t, "documents measured for the node budget margin", files, minCorpusDocuments)
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
// by every predicate — and by Detect — without difficulty.
//
// Detect is the case with consequences. Routing exists so a caller can decide
// what to do with a document *before* paying to validate it, and a router that
// could not name a document too large to parse would have nothing to say about
// exactly the input the decision matters most for.
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

	det, err := Detect(data)
	if err != nil {
		t.Fatalf("Detect on a %d-element document: %v; detection builds no tree and has nothing to trip",
			maxNodes+1, err)
	}
	// A UBL root declaring no national profile: step 5 of the order.
	if det.Source != SourceEN16931 {
		t.Errorf("Detect on a %d-element Invoice = %q, want %q", maxNodes+1, det.Source, SourceEN16931)
	}
	if det.Validator() == nil {
		t.Error("a document too large for the parser routed to no validator at all")
	}

	// The same document through the validator the routing chose does trip the
	// budget, which is the asymmetry worth having: the router answers, and the
	// checker refuses to guess.
	v := mustReport(t, context.Background(), det.Validator(), data)
	if v.Conformant() {
		t.Error("a document over the element budget came back conformant")
	}
	stopped := false
	for _, x := range v.Violations {
		if IsCheckerViolation(x) {
			stopped = true
		}
	}
	if !stopped {
		t.Errorf("validating a %d-element document reported no checker violation: %v", maxNodes+1, v.Violations)
	}
}

// corpusFormat maps a testdata corpus to the Source every document in it that
// Detect recognises must come back as.
//
// Three corpora are deliberately absent because they are mixed by construction
// and a single expected answer would be wrong for them: en16931-artefacts and
// en16931-ubl carry both plain EN 16931 and Peppol instances (and, in the CEN
// artefacts, several hundred Schematron and build files that are not invoices);
// and nlcius ships two error samples whose whole defect is a missing or
// whitespace-only BT-24, which is exactly the document that has no CIUS to
// declare.
//
// The pint corpus was the fourth until C24 was fixed. It was excluded because
// its eight pre-release Japanese samples declare "urn:fdc:peppol:jp:billing:3.0"
// and never say "pint" — but the other 56 were not routing to PINT either, and
// the exclusion is what kept that from being visible here. All 64 now route to
// SourcePINT: the pre-release identifier is a documented entry of specIDRules.
var corpusFormat = map[string]Source{
	"cius-be":     SourceUBLBE,
	"cius-pt":     SourceCIUSPT,
	"cius-ro":     SourceCIUSRO,
	"cius-rs":     SourceSRBDT,
	"ebinterface": SourceEbInterface,
	"facturae":    SourceFacturae,
	"fatturapa":   SourceFatturaPA,
	"finvoice":    SourceFinvoice,
	"ksef":        SourceKSeF,
	"oioubl":      SourceOIOUBL,
	"osa":         SourceOSA,
	"pint":        SourcePINT,
	"svefaktura":  SourceSvefaktura,
	"teapps":      SourceTEAPPS,
	"turkey":      SourceUBLTR,
	"xrechnung":   SourceXRechnung,
	"zatca":       SourceZATCA,
}

// TestDetectRoutesTheCorpus is the evidence that the precedence is right about
// documents rather than only about the constructed overlaps.
//
// The overlapping documents in detect_test.go are contrived, which is the honest
// thing to say about them: no producer emits an invoice carrying both a Biller
// and a SellerParty. What makes the order load-bearing is that it has to leave
// every real document where it was — a Turkish invoice must not become OIOUBL
// because its identifier starts "TR", an XRechnung CII document must not fall
// through to the EN 16931 core because its identifier is three elements down,
// and a ZATCA invoice must not become Peppol. This sweeps every conformance
// corpus whose documents share one format and asserts exactly that.
func TestDetectRoutesTheCorpus(t *testing.T) {
	routed, unrecognised := 0, 0
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		parts := strings.Split(p, string(filepath.Separator))
		if len(parts) < 2 {
			return nil
		}
		want, listed := corpusFormat[parts[1]]
		if !listed {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		det, derr := Detect(data)
		if derr != nil {
			// A corpus of deliberately broken instances still contains no
			// unreadable XML; if that changes, say so rather than skipping it.
			t.Errorf("%s: Detect could not read a corpus document: %v", p, derr)
			return nil
		}
		if !det.Recognised() {
			// Several corpora ship Schematron, build files and other document
			// types alongside the invoices. Those are not this test's business.
			unrecognised++
			return nil
		}
		if det.Source != want {
			t.Errorf("%s: Detect = %q, want %q (root %q, BT-24 %q)", p, det.Source, want, det.Root, det.SpecID)
			return nil
		}
		routed++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Not vendored, so the sweep can only run where the corpus is.
	if routed == 0 && unrecognised == 0 {
		t.Skip("no corpus present (make cius-oracles / make en16931-artefacts)")
	}
	// The ratchet, from corpus_test.go: a test that only asserted "every
	// document I saw routed correctly" would pass loudly on three files.
	atLeast(t, "documents routed", routed, minRoutedDocuments)
	t.Logf("routed %d corpus documents to the format their corpus publishes (%d other document types skipped)",
		routed, unrecognised)
}

// TestScanRetainsOnlyWhatItCaptures is the precise version of the claim
// detect.go opens with.
//
// The scan's saving is that nothing accumulates *per element*: a document of
// millions of siblings costs it nothing, which is what
// TestDetectionMemoryDoesNotScaleWithInput measures. What it is not is free of
// the document's size, because the handful of elements whose text it does keep
// are kept whole — and detect.go used to claim otherwise ("the cost is set by
// the nesting rather than by the size", "a few short strings"). Nothing makes
// those strings short.
//
// So both halves are pinned here: an element the scan ignores costs it nothing
// however large, and an element it captures costs about one copy. If a cap on
// the capture is ever added, the second half is where that decision surfaces —
// and it must be weighed against TestScanMatchesTreeDetection, since the
// predicates match substrings and a truncated capture could route a document
// differently from the tree.
func TestScanRetainsOnlyWhatItCaptures(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~40 MB per case")
	}
	const size = 20 << 20
	doc := func(elem string) []byte {
		var b strings.Builder
		b.Grow(size + 128)
		b.WriteString(`<Invoice><` + elem + `>`)
		b.WriteString(strings.Repeat("x", size))
		b.WriteString(`</` + elem + `></Invoice>`)
		return []byte(b.String())
	}

	// An element no predicate consults. The text streams past and is dropped.
	ignored := doc("Note")
	_, retained := scanCost(t, ignored)
	t.Logf("uncaptured element, %d bytes of text: scan retains %.3fx the input", size, retained)
	if retained > 0.01 {
		t.Errorf("the scan retained %.3fx the input from an element it does not capture; "+
			"nothing accumulates for an element whose text is discarded", retained)
	}

	// A captured one. This is the part of the claim that was wrong.
	captured := doc("CustomizationID")
	allocated, retained := scanCost(t, captured)
	t.Logf("captured element, %d bytes of text: scan retains %.2fx the input, allocates %.2fx", size, retained, allocated)
	if retained < 0.9 {
		t.Errorf("the scan retained only %.2fx the input from a captured element; it keeps the text whole, "+
			"and a test that stops seeing that means the capture was silently bounded", retained)
	}
	if retained > 1.5 {
		t.Errorf("the scan retained %.2fx the input from one captured element; it keeps one copy, not more", retained)
	}

	// And the answer still has to be the one the tree gives, which is the
	// property any future cap would have to preserve.
	d, err := scanShape(captured)
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseCII(newRun(context.Background()), captured)
	if err != nil {
		t.Fatal(err)
	}
	if d.str("CustomizationID") != root.str("CustomizationID") {
		t.Errorf("the scan captured %d bytes and the tree %d; they must agree",
			len(d.str("CustomizationID")), len(root.str("CustomizationID")))
	}
}
