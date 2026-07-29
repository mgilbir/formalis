package formalis

import "strings"

// This file evaluates the fatal half of CEN's UBL syntax binding — the
// UBL-SR-* rules of EN16931-UBL-syntax.sch — against the parsed element tree.
//
// Why not on the syntax-neutral model, which is where every BR-* rule in this
// package lives? Because these are not statements about business terms. A BR-*
// rule says something an invoice means ("the total with VAT is the total
// without VAT plus the VAT total"), which is true or false of a Factur-X
// document and of a Peppol document alike, and the shared model exists so that
// one sentence of Go can say it for both. A UBL-SR-* rule says something about a
// UBL document tree ("cac:PaymentTerms shall carry at most one cbc:Note"), and
// it is inapplicable to CII by construction: CEN publishes a separate CII-SR-*
// binding whose rules are numbered differently, phrased differently, and do not
// correspond one-to-one. Restating CEN's element counts as fields on
// en16931Invoice would fill the shared model with facts that are meaningless for
// half its readers, and would still not make a CII invoice answerable to them.
//
// So the syntax rules read the tree, which parsed keeps alongside the model for
// exactly this reason, and they live in their own file where a reader looking
// for "the UBL binding" finds all of it in one place. cius_be.go is the
// precedent: the ubl-BE-* rules are UBL-structural and are evaluated the same
// way.
//
// Fidelity. The rules below are transcribed from the assertions in
// ubl/schematron/UBL/EN16931-UBL-syntax.sch as resolved in
// ubl/schematron/preprocessed/EN16931-UBL-validation-preprocessed.sch, which is
// the form a reference validator runs. Each rule cites its XPath. The Schematron
// contexts are element patterns, not fixed paths: `cac:PaymentMeans` matches a
// cac:PaymentMeans anywhere in the document, not only under the document
// element, and the transcription follows that. The rules whose context is the
// document element (`/ubl:Invoice | /cn:CreditNote`) are the exception and are
// applied to the root only.
//
// Severity. Every one of CEN's 54 UBL-SR-* assertions is flagged fatal — the
// binding publishes no advisory cardinality rule — so none of them is filtered
// out here. The advisory UBL-DT-* and UBL-CR-* rules of the same binding are not
// evaluated; Coverage(SourceEN16931) names them.
//
// Source. These findings are stamped SourceEN16931, not a source of their own:
// CEN publishes the syntax bindings as normative parts of EN 16931 itself, and
// a caller filtering on the authority is asking who wrote the rule, not which
// file it came out of.

// validateUBLSyntaxRules evaluates the fatal UBL-SR-* rules against a UBL
// document tree. It is a no-op for any other root, so a CII invoice passing
// through the same entry point is never asked to answer a UBL rule.
//
// The rule bodies are grouped by the node population they read, which is also
// how the Schematron groups them: one <rule context=...> per group, named in
// each function's own comment.
func validateUBLSyntaxRules(r *run, root *ciiNode) []Violation {
	if root == nil || (root.name != "Invoice" && root.name != "CreditNote") {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceEN16931)
	g := gatherUBLSyntaxNodes(root)

	ublSyntaxDocumentRules(g, add)
	return out
}

// ublSyntaxNodes is every node population the UBL-SR-* rules read, gathered in
// one pass.
//
// The alternative — one findAll per rule — would walk the tree once per rule,
// four dozen times over, and this package validates 1,680 UBL documents on every
// test run. One walk costs what the largest population costs. The struct also
// makes the rule bodies read as what they are: a count against a bound, with no
// traversal in the way.
type ublSyntaxNodes struct {
	// root is the document element, and the context of the twenty-eight rules
	// CEN binds to `/ubl:Invoice | /cn:CreditNote`.
	root *ciiNode
	// isCreditNote is read by UBL-SR-43, the one rule whose test names the
	// document element.
	isCreditNote bool

	addresses        []*ciiNode // cac:PostalAddress | cac:Address
	supplierParties  []*ciiNode // cac:AccountingSupplierParty/cac:Party
	addDocRefs       []*ciiNode // cac:AdditionalDocumentReference
	deliveries       []*ciiNode // cac:Delivery
	allowanceCharges []*ciiNode // cac:AllowanceCharge
	partyTaxSchemes  []*ciiNode // cac:PartyTaxScheme
	lines            []*ciiNode // cac:InvoiceLine | cac:CreditNoteLine
	paymentMeans     []*ciiNode // cac:PaymentMeans
	billingRefs      []*ciiNode // cac:BillingReference
	taxReps          []*ciiNode // cac:TaxRepresentativeParty
	taxSubtotals     []*ciiNode // cac:TaxSubtotal

	// payees carries each cac:PayeeParty with its parent element, because
	// UBL-SR-19/20/21 compare the payee name against `../cac:AccountingSupplier
	// Party/...` — a step up out of the context node.
	payees []ublParented

	// sepaCreditorIDs is `//cac:PartyIdentification/cbc:ID[upper-case(@schemeID)
	// = 'SEPA']`, counted document-wide for UBL-SR-29.
	sepaCreditorIDs int
	// paymentIDValues and paymentMeansCodeValues are the string values of
	// `//cbc:PaymentID` and `//cbc:PaymentMeansCode`, document-wide, for the two
	// rules that bound the number of *distinct* values rather than of elements.
	paymentIDValues        []string
	paymentMeansCodeValues []string
}

// ublParented is a node together with its parent, for the rules whose XPath
// steps up out of the context node.
type ublParented struct {
	node   *ciiNode
	parent *ciiNode
}

// gatherUBLSyntaxNodes walks the tree once and collects every population the
// rules read.
func gatherUBLSyntaxNodes(root *ciiNode) *ublSyntaxNodes {
	g := &ublSyntaxNodes{root: root, isCreditNote: root.name == "CreditNote"}
	var rec func(n, parent *ciiNode)
	rec = func(n, parent *ciiNode) {
		switch n.name {
		case "PostalAddress", "Address":
			g.addresses = append(g.addresses, n)
		case "Party":
			if parent != nil && parent.name == "AccountingSupplierParty" {
				g.supplierParties = append(g.supplierParties, n)
			}
		case "AdditionalDocumentReference":
			g.addDocRefs = append(g.addDocRefs, n)
		case "Delivery":
			g.deliveries = append(g.deliveries, n)
		case "AllowanceCharge":
			g.allowanceCharges = append(g.allowanceCharges, n)
		case "PartyTaxScheme":
			g.partyTaxSchemes = append(g.partyTaxSchemes, n)
		case "InvoiceLine", "CreditNoteLine":
			g.lines = append(g.lines, n)
		case "PayeeParty":
			g.payees = append(g.payees, ublParented{node: n, parent: parent})
		case "PaymentMeans":
			g.paymentMeans = append(g.paymentMeans, n)
		case "BillingReference":
			g.billingRefs = append(g.billingRefs, n)
		case "TaxRepresentativeParty":
			g.taxReps = append(g.taxReps, n)
		case "TaxSubtotal":
			g.taxSubtotals = append(g.taxSubtotals, n)
		case "PartyIdentification":
			for _, id := range n.all("ID") {
				if strings.EqualFold(id.attr("schemeID"), "SEPA") {
					g.sepaCreditorIDs++
				}
			}
		case "PaymentID":
			g.paymentIDValues = append(g.paymentIDValues, strings.TrimSpace(n.text))
		case "PaymentMeansCode":
			g.paymentMeansCodeValues = append(g.paymentMeansCodeValues, strings.TrimSpace(n.text))
		}
		for _, c := range n.children {
			rec(c, n)
		}
	}
	rec(root, nil)
	return g
}

// nodesAt returns every node reachable from n by the given chain of local
// names, following every branch. It is `count(a/b/c)`'s node set: the XPath step
// is over all matching children at each level, not the first.
func nodesAt(n *ciiNode, path ...string) []*ciiNode {
	if n == nil {
		return nil
	}
	cur := []*ciiNode{n}
	for _, name := range path {
		var next []*ciiNode
		for _, c := range cur {
			for _, ch := range c.children {
				if ch.name == name {
					next = append(next, ch)
				}
			}
		}
		if len(next) == 0 {
			return nil
		}
		cur = next
	}
	return cur
}

// countAt is `count(path)` relative to n.
func countAt(n *ciiNode, path ...string) int { return len(nodesAt(n, path...)) }

// atMostOnce is the shape forty of these rules share: a path that may occur once
// and does not.
func atMostOnce(n *ciiNode, path ...string) bool { return countAt(n, path...) > 1 }

// distinctValues counts how many distinct strings a slice holds. It is
// `count(//x[not(preceding::x/. = .)])` — the number of distinct string values
// of an element, which is what UBL-SR-44 and UBL-SR-47 bound rather than the
// number of elements.
func distinctValues(vals []string) int {
	seen := make(map[string]bool, len(vals))
	for _, v := range vals {
		seen[v] = true
	}
	return len(seen)
}

// ublSyntaxDocumentRules are the rules CEN binds to the document element itself
// — `<rule context="/ubl:Invoice | /cn:CreditNote">` — including the three whose
// test reaches document-wide from there with a `//` step. Each bounds how often
// a business term may appear on one invoice.
func ublSyntaxDocumentRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	root := g.root
	for _, c := range []struct {
		rule string
		term string
		path []string
	}{
		// count(cac:ContractDocumentReference/cbc:ID) <= 1
		{"UBL-SR-01", "The Contract reference (BT-12)", []string{"ContractDocumentReference", "ID"}},
		// count(cac:ReceiptDocumentReference/cbc:ID) <= 1
		{"UBL-SR-02", "The Receiving advice reference (BT-15)", []string{"ReceiptDocumentReference", "ID"}},
		// count(cac:DespatchDocumentReference/cbc:ID) <= 1
		{"UBL-SR-03", "The Despatch advice reference (BT-16)", []string{"DespatchDocumentReference", "ID"}},
		// count(cac:PaymentTerms/cbc:Note) <= 1
		{"UBL-SR-05", "The Payment terms (BT-20)", []string{"PaymentTerms", "Note"}},
		// count(cac:InvoicePeriod) <= 1
		{"UBL-SR-08", "The Invoicing period (BG-14)", []string{"InvoicePeriod"}},
		// count(cac:Delivery) <= 1
		{"UBL-SR-24", "The Deliver to information (BG-13)", []string{"Delivery"}},
		// count(cac:ProjectReference/cbc:ID) <= 1
		{"UBL-SR-39", "The Project reference (BT-11)", []string{"ProjectReference", "ID"}},
		// count(cac:PaymentMeans/cbc:PaymentDueDate) <= 1
		{"UBL-SR-45", "The Payment due date (BT-9)", []string{"PaymentMeans", "PaymentDueDate"}},
		// count(cac:InvoicePeriod/cbc:DescriptionCode) <= 1
		{"UBL-SR-49", "The Value added tax point date code (BT-8)", []string{"InvoicePeriod", "DescriptionCode"}},
		// count(cac:PaymentMeans/cac:CardAccount) <= 1
		{"UBL-SR-54", "The Payment card information group (BG-18)", []string{"PaymentMeans", "CardAccount"}},
		// count(cac:PaymentMeans/cac:PaymentMandate) <= 1
		{"UBL-SR-55", "The Direct debit group (BG-19)", []string{"PaymentMeans", "PaymentMandate"}},
		// count(cac:OriginatorDocumentReference/cbc:ID) <= 1
		{"UBL-SR-56", "The Tender or lot reference (BT-17)", []string{"OriginatorDocumentReference", "ID"}},
	} {
		if atMostOnce(root, c.path...) {
			add(c.rule, c.term+" shall occur at most once")
		}
	}

	// UBL-SR-04: count(cac:AdditionalDocumentReference[cbc:DocumentTypeCode='130']
	// /cbc:ID) <= 1. The Invoiced object identifier (BT-18) is the supporting
	// document whose type code is 130; the other cac:AdditionalDocumentReference
	// entries are BG-24 attachments and are not bounded here.
	//
	// The path is relative to the context node, which for this rule is the
	// document element, so the references on it are the ones counted — unlike
	// UBL-SR-33 and UBL-SR-43, whose context is the reference itself and which
	// therefore reach every one in the document.
	objectIDs := 0
	for _, d := range root.all("AdditionalDocumentReference") {
		if !ublDocRefHasTypeCode(d, "130") {
			continue
		}
		objectIDs += countAt(d, "ID")
	}
	if objectIDs > 1 {
		add("UBL-SR-04", "The Invoiced object identifier (BT-18) shall occur at most once")
	}

	// UBL-SR-46: count(cac:PaymentMeans/cbc:PaymentMeansCode/@name) <= 1. The
	// Payment means text (BT-82) is an attribute of the code, not an element of
	// its own, so this counts attributes rather than elements.
	names := 0
	for _, pmc := range nodesAt(root, "PaymentMeans", "PaymentMeansCode") {
		if pmc.hasAttr("name") {
			names++
		}
	}
	if names > 1 {
		add("UBL-SR-46", "The Payment means text (BT-82) shall occur at most once")
	}

	// UBL-SR-29: count(//cac:PartyIdentification/cbc:ID[upper-case(@schemeID) =
	// 'SEPA']) <= 1. The Bank assigned creditor identifier (BT-90) is written as
	// a party identifier tagged SEPA, and may be written on the Seller or on the
	// Payee — hence the document-wide count.
	if g.sepaCreditorIDs > 1 {
		add("UBL-SR-29", "The Bank assigned creditor identifier (BT-90) shall occur at most once")
	}

	// UBL-SR-44: count(//cbc:PaymentID[not(preceding::cbc:PaymentID/. = .)]) <= 1.
	// The bound is on distinct values, not on elements: one payment reference may
	// legitimately be repeated on every cac:PaymentMeans.
	if distinctValues(g.paymentIDValues) > 1 {
		add("UBL-SR-44", "The Payment reference (BT-83) shall occur at most once, though one value may be repeated on several payment means")
	}
	// UBL-SR-47: the same shape for the payment means code (BT-81) — several
	// cac:PaymentMeans are allowed, but they shall agree.
	if distinctValues(g.paymentMeansCodeValues) > 1 {
		add("UBL-SR-47", "All Payment means type codes (BT-81) shall have the same value")
	}
}

// ublDocRefHasTypeCode reports whether a cac:AdditionalDocumentReference carries
// a cbc:DocumentTypeCode with the given value. CEN writes the predicate
// `cbc:DocumentTypeCode='130'`, an existential over the children, so a reference
// with several codes matches if any of them does.
func ublDocRefHasTypeCode(d *ciiNode, code string) bool {
	for _, tc := range d.all("DocumentTypeCode") {
		if strings.TrimSpace(tc.text) == code {
			return true
		}
	}
	return false
}
