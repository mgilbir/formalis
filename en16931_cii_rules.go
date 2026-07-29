package formalis

import (
	"fmt"
	"strings"
)

// This file evaluates the fatal half of CEN's CII syntax binding — the CII-SR-*
// cardinality rules and the CII-DT-* datatype rules of EN16931-CII-syntax.sch —
// against the parsed element tree. It is the mirror of en16931_ubl_rules.go,
// which argues the design at length; the short form is that a CII-SR-* or
// CII-DT-* rule is a statement about a CrossIndustryInvoice document tree
// ("ram:BillingSpecifiedPeriod shall carry no ram:Description", "an amount other
// than ram:TaxTotalAmount shall carry no @currencyID"), inapplicable to UBL by
// construction, and restating a hundred element and attribute counts as fields
// on en16931Invoice would fill the shared semantic model with facts meaningless
// to half its readers.
//
// Fidelity. Every rule below is transcribed from the assertion in
// cii/schematron/CII/EN16931-CII-syntax.sch as resolved in
// cii/schematron/preprocessed/EN16931-CII-validation-preprocessed.sch, which is
// the form the vendored EN16931-CII-validation.xslt is generated from and the
// form a reference validator runs. Each rule cites its XPath, because several of
// CEN's rule *titles* describe a different rule than the XPath does — CII-SR-471
// is titled "shall contain a VAT category code (BT-95)" and tests
// `count(ram:RateApplicablePercent) <= 1`, and CII-SR-472 is titled "should
// contain a VAT rate (BT-96)" and tests `count(ram:CategoryTradeTax) <= 1`. The
// XPath is the rule.
//
// Severity. Of the 482 CII-SR-* assertions CEN publishes, 42 are flagged fatal
// and 440 advisory; of the 101 CII-DT-* assertions, 70 are fatal and 31
// advisory. Only the fatal ones are evaluated here, on the policy this package
// has followed since NLCIUS's BR-NL-19..35: report what an authority makes fatal
// and name its advisory families in Coverage.

//
// Source. These findings are stamped SourceEN16931, not a source of their own:
// CEN publishes the syntax bindings as normative parts of EN 16931 itself.

// validateCIISyntaxRules evaluates the fatal CII-SR-* and CII-DT-* rules against
// a CII document tree. It is a no-op for any other root, so a UBL invoice
// passing through the same entry point is never asked to answer a CII rule.
//
// The rule bodies are grouped by the node population they read, which is also
// how the Schematron groups them: one <rule context=...> per group, named in
// each function's own comment.
func validateCIISyntaxRules(r *run, root *ciiNode) []Violation {
	if root == nil || root.name != "CrossIndustryInvoice" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceEN16931)
	g := gatherCIISyntaxNodes(root)

	ciiSyntaxDocumentRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxLineRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxAllowanceRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxHeaderRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxTotalsRules(g, add)
	return out
}

// ciiSyntaxNodes is every node population the fatal CII rules read, gathered in
// one pass.
//
// The alternative — one findAll per rule — would walk the tree once per rule,
// a hundred and ten times over, on every CII document this package validates.
// One walk costs what the largest population costs. The struct also makes the
// rule bodies read as what they are: a count, or an attribute test, against a
// bound, with no traversal in the way.
//
// Two kinds of population live here and they are not interchangeable. The
// path-anchored ones (contexts, exchanged, lines, agreements, settlements,
// summations) are the contexts CEN writes as an absolute path from the document
// element, and are collected by following that path. The rest are the contexts
// CEN writes with a `//` step or a name predicate, and are collected by the
// walk, which matches CEN's own "anywhere in the document" reading.
type ciiSyntaxNodes struct {
	// root is the document element, and the context of the five rules CEN binds
	// to `/rsm:CrossIndustryInvoice`.
	root *ciiNode

	contexts    []*ciiNode // /rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext
	exchanged   []*ciiNode // /rsm:CrossIndustryInvoice/rsm:ExchangedDocument
	lines       []*ciiNode // .../ram:IncludedSupplyChainTradeLineItem
	agreements  []*ciiNode // .../ram:ApplicableHeaderTradeAgreement
	settlements []*ciiNode // .../ram:ApplicableHeaderTradeSettlement
	summations  []*ciiNode // .../ram:SpecifiedTradeSettlementHeaderMonetarySummation

	allowanceCharges []*ciiNode // //ram:SpecifiedTradeAllowanceCharge
	priceAllowances  []*ciiNode // //ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge

	// dueDateTypeCodes is `//ram:ApplicableTradeTax/ram:DueDateTypeCode`, read
	// document-wide because CII-SR-462's test reaches out of its own context
	// with a `//` step.
	dueDateTypeCodes []string
	// paymentMeansCodes and paymentMeansTexts are the values CII-SR-467 and
	// CII-SR-468 require to agree across every payment instruction.
	paymentMeansCodes []string
	paymentMeansTexts []string
	// paymentReferences is `count(//ram:ApplicableHeaderTradeSettlement/
	// ram:PaymentReference)` — elements, not distinct values, which is where the
	// CII binding differs from UBL-SR-44.
	paymentReferences int

	// scopedIDs are the four identifiers CEN gives a rule of their own
	// (CII-DT-001..007); otherIDs is every other ram element whose name ends in
	// "ID", which is what is left for the wildcard rule (CII-DT-101..104) once
	// the earlier rule has claimed the four.
	scopedIDs []*ciiNode
	otherIDs  []*ciiNode

	typeCodes  []*ciiNode // //ram:TypeCode
	refDocs    []*ciiNode // //ram:*[ends-with(name(), 'ReferencedDocument')]
	amounts    []*ciiNode // //ram:*[ends-with(name(), 'Amount') and not(self::ram:TaxTotalAmount)]
	quantities []*ciiNode // //ram:*[ends-with(name(), 'Quantity')]
	tradeTaxes []*ciiNode // //ram:*[ends-with(name(), 'TradeTax')]
	periods    []*ciiNode // //ram:BillingSpecifiedPeriod
	addresses  []*ciiNode // //ram:PostalTradeAddress
	dates102   []*ciiNode // //udt:DateTimeString[@format = '102']

	// billedQuantityUnitCode is the second operand of CII-DT-033, which permits
	// a @unitCode anywhere in the document as soon as the line's billed quantity
	// carries one.
	billedQuantityUnitCode bool
}

// gatherCIISyntaxNodes walks the tree once and collects every population the
// rules read.
func gatherCIISyntaxNodes(root *ciiNode) *ciiSyntaxNodes {
	g := &ciiSyntaxNodes{root: root}

	// The path-anchored contexts first, because the identity of the four scoped
	// identifiers is defined by their path and the walk below has to be able to
	// tell them from every other element whose name ends in "ID".
	g.contexts = root.all("ExchangedDocumentContext")
	g.exchanged = root.all("ExchangedDocument")
	for _, tx := range root.all("SupplyChainTradeTransaction") {
		g.lines = append(g.lines, tx.all("IncludedSupplyChainTradeLineItem")...)
		g.agreements = append(g.agreements, tx.all("ApplicableHeaderTradeAgreement")...)
		g.settlements = append(g.settlements, tx.all("ApplicableHeaderTradeSettlement")...)
	}
	for _, st := range g.settlements {
		g.summations = append(g.summations, st.all("SpecifiedTradeSettlementHeaderMonetarySummation")...)
	}

	// scoped is the walk's exclusion set; g.scopedIDs is the same nodes in the
	// order they were claimed, because a map's iteration order would make the
	// order of the findings vary from run to run.
	scoped := make(map[*ciiNode]bool)
	claim := func(ns []*ciiNode) {
		for _, n := range ns {
			if scoped[n] {
				continue
			}
			scoped[n] = true
			g.scopedIDs = append(g.scopedIDs, n)
		}
	}
	for _, c := range g.contexts {
		claim(nodesAt(c, "GuidelineSpecifiedDocumentContextParameter", "ID"))
	}
	for _, e := range g.exchanged {
		claim(nodesAt(e, "ID"))
	}
	for _, ln := range g.lines {
		claim(nodesAt(ln, "AssociatedDocumentLineDocument", "LineID"))
		claim(nodesAt(ln, "SpecifiedTradeProduct", "SellerAssignedID"))
		for _, d := range nodesAt(ln, "SpecifiedLineTradeDelivery", "BilledQuantity") {
			if d.hasAttr("unitCode") {
				g.billedQuantityUnitCode = true
			}
		}
	}

	var rec func(n, parent *ciiNode)
	rec = func(n, parent *ciiNode) {
		// The contexts CEN names by an exact element name.
		switch n.name {
		case "SpecifiedTradeAllowanceCharge":
			g.allowanceCharges = append(g.allowanceCharges, n)
		case "AppliedTradeAllowanceCharge":
			if parent != nil && parent.name == "GrossPriceProductTradePrice" {
				g.priceAllowances = append(g.priceAllowances, n)
			}
		case "ApplicableTradeTax":
			for _, d := range n.all("DueDateTypeCode") {
				g.dueDateTypeCodes = append(g.dueDateTypeCodes, normalizeSpace(d.text))
			}
		case "SpecifiedTradeSettlementPaymentMeans":
			for _, tc := range n.all("TypeCode") {
				g.paymentMeansCodes = append(g.paymentMeansCodes, normalizeSpace(tc.text))
			}
			for _, i := range n.all("Information") {
				g.paymentMeansTexts = append(g.paymentMeansTexts, normalizeSpace(i.text))
			}
		case "ApplicableHeaderTradeSettlement":
			g.paymentReferences += len(n.all("PaymentReference"))
		case "TypeCode":
			g.typeCodes = append(g.typeCodes, n)
		case "BillingSpecifiedPeriod":
			g.periods = append(g.periods, n)
		case "PostalTradeAddress":
			g.addresses = append(g.addresses, n)
		case "DateTimeString":
			// CEN's context is `//udt:DateTimeString`, and this tree is keyed by
			// local name, so the namespace has to be recovered from the position.
			// The CII schema puts a qdt:DateTimeString in exactly one place —
			// under ram:FormattedIssueDateTime, whose type is the qualified
			// FormattedDateTimeType — and a udt:DateTimeString everywhere else.
			// Excluding that one parent is what makes this rule the udt-only rule
			// CEN wrote rather than a broader one this package invented.
			if n.attr("format") == "102" && (parent == nil || parent.name != "FormattedIssueDateTime") {
				g.dates102 = append(g.dates102, n)
			}
		}
		// The contexts CEN names by a suffix of the element name. These are
		// disjoint from each other, and the switch above is a separate statement
		// because an element can be in both — ram:ApplicableTradeTax is read for
		// CII-SR-462 and is also a `//ram:*[ends-with(name(), 'TradeTax')]`.
		//
		// CEN restricts each to the ram namespace, which this tree does not carry.
		// It costs nothing here: across the 181 CII documents in the corpus, every
		// element whose local name ends in ID, Amount, Quantity, TypeCode,
		// ReferencedDocument or TradeTax is in the ram namespace, and the CII
		// schema puts nothing else in rsm, udt or qdt that could end in one of
		// them.
		switch {
		case strings.HasSuffix(n.name, "ID"):
			if !scoped[n] {
				g.otherIDs = append(g.otherIDs, n)
			}
		case strings.HasSuffix(n.name, "ReferencedDocument"):
			g.refDocs = append(g.refDocs, n)
		case n.name != "TaxTotalAmount" && strings.HasSuffix(n.name, "Amount"):
			g.amounts = append(g.amounts, n)
		case strings.HasSuffix(n.name, "Quantity"):
			g.quantities = append(g.quantities, n)
		case strings.HasSuffix(n.name, "TradeTax"):
			g.tradeTaxes = append(g.tradeTaxes, n)
		}
		for _, c := range n.children {
			rec(c, n)
		}
	}
	rec(root, nil)
	return g
}

// normalizeSpace is XPath's normalize-space(): leading and trailing whitespace
// removed and every internal run collapsed to one space. Three of these rules
// compare element values through it rather than raw, and a value that differs
// only in indentation is the same value to them.
func normalizeSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// allNormalizeSpaceEqual is the shape CII-SR-467 and CII-SR-468 share:
//
//	count(//X[normalize-space(.) != normalize-space((//X)[1])]) = 0
//
// — every occurrence of a term, document-wide, agrees with the first. Vacuously
// true for none or one.
func allNormalizeSpaceEqual(vals []string) bool {
	for _, v := range vals {
		if v != vals[0] {
			return false
		}
	}
	return true
}

// anyHasAttr reports whether any node in the set carries the named attribute. It
// is how CEN's existence tests on an attribute node read when the step before
// them selects a sequence: `ram:GlobalID/@schemeID` is true when *some*
// ram:GlobalID has one.
func anyHasAttr(ns []*ciiNode, name string) bool {
	for _, n := range ns {
		if n.hasAttr(name) {
			return true
		}
	}
	return false
}

// hasChild is `(ram:X)` — an existence test on a child element, whatever its
// content.
func hasChild(n *ciiNode, name string) bool { return len(n.all(name)) > 0 }

// ciiSyntaxDocumentRules are the rules CEN binds to the document element and to
// the two head groups under it — `/rsm:CrossIndustryInvoice`,
// `/rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext` and
// `/rsm:CrossIndustryInvoice/rsm:ExchangedDocument`.
//
// CII-DT-013 and CII-DT-014 share the document element's rule with CII-SR-467/
// 468/469 rather than living with the other datatype rules, and are stated here
// for the same reason the file is grouped this way at all: the group is the
// Schematron's, so a reader holding the .sch open finds the same five assertions
// in the same place.
func ciiSyntaxDocumentRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, c := range g.contexts {
		// count(ram:GuidelineSpecifiedDocumentContextParameter) = 1
		if n := countAt(c, "GuidelineSpecifiedDocumentContextParameter"); n != 1 {
			add("CII-SR-009", fmt.Sprintf("The document context shall carry exactly one guideline parameter group (BT-24), not %d", n))
		}
		// count(ram:GuidelineSpecifiedDocumentContextParameter/ram:ID) = 1
		if n := countAt(c, "GuidelineSpecifiedDocumentContextParameter", "ID"); n != 1 {
			add("CII-SR-010", fmt.Sprintf("The Specification identifier (BT-24) shall occur exactly once, not %d times", n))
		}
	}
	for _, e := range g.exchanged {
		// count(ram:TypeCode) = 1
		if n := countAt(e, "TypeCode"); n != 1 {
			add("CII-SR-014", fmt.Sprintf("The Invoice type code (BT-3) shall occur exactly once, not %d times", n))
		}
	}

	// CII-SR-467/468: every payment instruction shall agree on the payment means
	// type code (BT-81) and on the payment means text (BT-82). CEN writes both as
	// "no occurrence differs from the first", compared through normalize-space.
	if !allNormalizeSpaceEqual(g.paymentMeansCodes) {
		add("CII-SR-467", "All Payment means type codes (BT-81) shall have the same value")
	}
	if !allNormalizeSpaceEqual(g.paymentMeansTexts) {
		add("CII-SR-468", "All Payment means texts (BT-82) shall have the same value")
	}
	// CII-SR-469: count(//ram:ApplicableHeaderTradeSettlement/ram:PaymentReference)
	// <= 1. Elements, not distinct values: the CII binding gives the payment
	// reference (BT-83) one home, so repeating it is a defect even when the two
	// copies agree. UBL-SR-44 bounds the same term by distinct value, because the
	// UBL binding writes it once per cac:PaymentMeans.
	if g.paymentReferences > 1 {
		add("CII-SR-469", "The Payment reference (BT-83) shall occur at most once")
	}

	// CII-DT-013/014, on the document element.
	if g.root.hasAttr("languageID") {
		add("CII-DT-013", "The document element shall not carry a languageID attribute")
	}
	if g.root.hasAttr("languageLocaleID") {
		add("CII-DT-014", "The document element shall not carry a languageLocaleID attribute")
	}
}

// ciiSyntaxLineRules are the rules whose context is an invoice line or a group
// inside one: ram:SpecifiedTradeProduct, its ram:ApplicableProductCharacteristic
// and ram:SpecifiedLineTradeAgreement. All three contexts are absolute paths
// through ram:IncludedSupplyChainTradeLineItem, so a product group written
// somewhere else in the tree is not asked to answer them.
func ciiSyntaxLineRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ln := range g.lines {
		for _, p := range ln.all("SpecifiedTradeProduct") {
			// not(ram:GlobalID) or (ram:GlobalID/@schemeID)
			if ids := p.all("GlobalID"); len(ids) > 0 && !anyHasAttr(ids, "schemeID") {
				add("CII-SR-046", "The Item standard identifier (BT-157) shall carry a scheme identifier (BT-157-1)")
			}
			// not(ram:OriginTradeCountry) or (count(ram:OriginTradeCountry/ram:ID) = 1)
			if hasChild(p, "OriginTradeCountry") {
				if n := countAt(p, "OriginTradeCountry", "ID"); n != 1 {
					add("CII-SR-090", fmt.Sprintf("An item origin country group shall carry exactly one Item country of origin (BT-159), not %d", n))
				}
			}
			for _, ch := range p.all("ApplicableProductCharacteristic") {
				// count(ram:Description) = 1
				if n := countAt(ch, "Description"); n != 1 {
					add("CII-SR-069", fmt.Sprintf("An item attribute (BG-32) shall carry exactly one Item attribute name (BT-160), not %d", n))
				}
				// count(ram:Value) = 1
				if n := countAt(ch, "Value"); n != 1 {
					add("CII-SR-072", fmt.Sprintf("An item attribute (BG-32) shall carry exactly one Item attribute value (BT-161), not %d", n))
				}
			}
		}
		for _, ag := range ln.all("SpecifiedLineTradeAgreement") {
			// CEN publishes two assertions over one count, in one rule: CII-SR-439
			// requires it to be exactly one and CII-SR-441 requires it to be at
			// most one. The second is implied by the first and both are fatal, so a
			// line with two net prices is reported twice; that is what a reference
			// validator does and this transcribes it rather than deduplicating.
			n := countAt(ag, "NetPriceProductTradePrice", "ChargeAmount")
			if n != 1 {
				add("CII-SR-439", fmt.Sprintf("An invoice line shall carry exactly one Item net price (BT-146), not %d", n))
			}
			if n > 1 {
				add("CII-SR-441", "The Item net price (BT-146) shall occur at most once per invoice line")
			}
		}
	}
}

// ciiSyntaxAllowanceRules are the rules whose context is an allowance or charge
// group: `//ram:SpecifiedTradeAllowanceCharge`, which matches the document-level
// groups (BG-20/21) and the line-level ones (BG-27/28) alike, and
// `//ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge`, which is
// the item price discount (BT-147).
func ciiSyntaxAllowanceRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ac := range g.allowanceCharges {
		// (ram:ChargeIndicator) — an existence test. Without it nothing in the
		// document says whether the group is an allowance or a charge.
		if !hasChild(ac, "ChargeIndicator") {
			add("CII-SR-463", "An allowance or charge group (BG-20/21, BG-27/28) shall carry a charge indicator")
		}
		// count(ram:RateApplicablePercent) <= 1. CEN's title for this assertion
		// names the VAT category code (BT-95); its XPath counts a percentage
		// element, and the XPath is the rule.
		if atMostOnce(ac, "RateApplicablePercent") {
			add("CII-SR-471", "An allowance or charge group shall carry at most one applicable rate percentage")
		}
		// count(ram:CategoryTradeTax) <= 1
		if atMostOnce(ac, "CategoryTradeTax") {
			add("CII-SR-472", "An allowance or charge group shall carry at most one VAT category group (BT-95/96, BT-102/103)")
		}
		// count(ram:ActualAmount) <= 1
		if atMostOnce(ac, "ActualAmount") {
			add("CII-SR-473", "An allowance or charge group shall carry at most one amount (BT-92/99, BT-136/141)")
		}
	}
	for _, ap := range g.priceAllowances {
		// count(ram:ActualAmount) <= 1
		if atMostOnce(ap, "ActualAmount") {
			add("CII-SR-440", "The Item price discount (BT-147) shall occur at most once")
		}
	}
}

// ciiSyntaxHeaderRules are the rules whose context is one of the two header
// groups — ram:ApplicableHeaderTradeAgreement and
// ram:ApplicableHeaderTradeSettlement — reached by their absolute path from the
// document element.
func ciiSyntaxHeaderRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ag := range g.agreements {
		for _, c := range []struct {
			rule string
			term string
			path []string
		}{
			// count(ram:SellerTradeParty/ram:DefinedTradeContact) <= 1
			{"CII-SR-455", "The Seller contact group (BG-6)", []string{"SellerTradeParty", "DefinedTradeContact"}},
			// count(ram:BuyerTradeParty/ram:DefinedTradeContact) <= 1
			{"CII-SR-456", "The Buyer contact group (BG-9)", []string{"BuyerTradeParty", "DefinedTradeContact"}},
			// count(ram:SellerTradeParty/ram:URIUniversalCommunication) <= 1
			{"CII-SR-459", "The Seller electronic address (BT-34)", []string{"SellerTradeParty", "URIUniversalCommunication"}},
			// count(ram:BuyerTradeParty/ram:URIUniversalCommunication) <= 1
			{"CII-SR-460", "The Buyer electronic address (BT-49)", []string{"BuyerTradeParty", "URIUniversalCommunication"}},
		} {
			if atMostOnce(ag, c.path...) {
				add(c.rule, c.term+" shall occur at most once")
			}
		}
	}

	for _, st := range g.settlements {
		// count(ram:ApplicableTradeTax/ram:TaxPointDate) <= 1. BT-7 is one
		// document-level term, and the CII binding writes it inside a VAT breakdown
		// group; an invoice with several breakdowns therefore has to choose one to
		// carry it rather than repeating it in each.
		if n := countAt(st, "ApplicableTradeTax", "TaxPointDate"); n > 1 {
			add("CII-SR-461", fmt.Sprintf("The Value added tax point date (BT-7) shall occur at most once, not %d times", n))
		}
		// count(//ram:ApplicableTradeTax/ram:DueDateTypeCode) = 0 or
		// count(distinct-values(//ram:ApplicableTradeTax/ram:DueDateTypeCode)) = 1.
		// The counterpart of CII-SR-461 for BT-8, and CEN bounds it differently:
		// the code may be repeated on every breakdown as long as they agree.
		if len(g.dueDateTypeCodes) > 0 && distinctValues(g.dueDateTypeCodes) > 1 {
			add("CII-SR-462", "All Value added tax point date codes (BT-8) shall have the same value")
		}
		// count(ram:SpecifiedTradeSettlementPaymentMeans[(normalize-space(
		// ram:TypeCode) = '30' or normalize-space(ram:TypeCode) = '58') and
		// not(ram:PayeePartyCreditorFinancialAccount/ram:IBANID or
		// ram:PayeePartyCreditorFinancialAccount/ram:ProprietaryID)]) = 0.
		//
		// The count is of offending groups and the bound is zero, so the finding is
		// one per settlement however many payment instructions offend.
		for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
			if !ciiIsCreditTransfer(pm) || ciiHasAccountIdentifier(pm) {
				continue
			}
			add("CII-SR-470", "A credit transfer (BG-16, BT-81 = 30 or 58) shall carry a Payment account identifier (BT-84) as an IBAN or a proprietary identifier")
			break
		}
	}
}

// ciiIsCreditTransfer is `normalize-space(ram:TypeCode) = '30' or
// normalize-space(ram:TypeCode) = '58'` — a general comparison over the
// children, so a group with several codes matches if any of them is a credit
// transfer.
func ciiIsCreditTransfer(pm *ciiNode) bool {
	for _, tc := range pm.all("TypeCode") {
		switch normalizeSpace(tc.text) {
		case "30", "58":
			return true
		}
	}
	return false
}

// ciiHasAccountIdentifier is `ram:PayeePartyCreditorFinancialAccount/ram:IBANID
// or ram:PayeePartyCreditorFinancialAccount/ram:ProprietaryID` — an existence
// test, so an empty element satisfies it. BR-50 is the rule about the value.
func ciiHasAccountIdentifier(pm *ciiNode) bool {
	for _, acc := range pm.all("PayeePartyCreditorFinancialAccount") {
		if hasChild(acc, "IBANID") || hasChild(acc, "ProprietaryID") {
			return true
		}
	}
	return false
}

// ciiSyntaxTotalsRules are the eighteen rules whose context is
// ram:SpecifiedTradeSettlementHeaderMonetarySummation. They bound every amount
// the CII summation group can carry at one occurrence each, including the eleven
// the EN 16931 core does not model — a document total this package has no
// business term for is still a document total, and two of them is still
// ambiguous.
func ciiSyntaxTotalsRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ms := range g.summations {
		for _, c := range []struct {
			rule string
			elem string
			term string
		}{
			{"CII-SR-477", "LineTotalAmount", "The Sum of Invoice line net amount (BT-106)"},
			{"CII-SR-478", "ChargeTotalAmount", "The Sum of charges on document level (BT-108)"},
			{"CII-SR-479", "AllowanceTotalAmount", "The Sum of allowances on document level (BT-107)"},
			{"CII-SR-480", "TaxBasisTotalAmount", "The Invoice total amount without VAT (BT-109)"},
			{"CII-SR-481", "RoundingAmount", "The Rounding amount (BT-114)"},
			{"CII-SR-482", "GrandTotalAmount", "The Invoice total amount with VAT (BT-112)"},
			{"CII-SR-483", "InformationAmount", "The information amount"},
			{"CII-SR-484", "TotalPrepaidAmount", "The Paid amount (BT-113)"},
			{"CII-SR-485", "TotalDiscountAmount", "The total discount amount"},
			{"CII-SR-486", "TotalAllowanceChargeAmount", "The total allowance/charge amount"},
			{"CII-SR-487", "DuePayableAmount", "The Amount due for payment (BT-115)"},
			{"CII-SR-488", "RetailValueExcludingTaxInformationAmount", "The retail value excluding tax"},
			{"CII-SR-489", "TotalDepositFeeInformationAmount", "The total deposit fee"},
			{"CII-SR-490", "ProductValueExcludingTobaccoTaxInformationAmount", "The product value excluding tobacco tax"},
			{"CII-SR-491", "TotalRetailValueInformationAmount", "The total retail value"},
			{"CII-SR-492", "GrossLineTotalAmount", "The gross line total"},
			{"CII-SR-493", "NetLineTotalAmount", "The net line total"},
			{"CII-SR-494", "NetIncludingTaxesLineTotalAmount", "The net line total including taxes"},
		} {
			// count(ram:X) <= 1
			if atMostOnce(ms, c.elem) {
				add(c.rule, c.term+" shall occur at most once in the document totals (BG-22)")
			}
		}
	}
}
