package formalis

import (
	"context"
	"regexp"
	"strings"
)

// This file validates the Serbian CIUS (SRBDT, urn:mfin.gov.rs:srbdt, the Ministry
// of Finance e-invoice profile) on top of the EN 16931 core.
//
// The rules below are transcribed from EN16931-UBL-srbdt.sch rather than from the
// specification's prose, which is what they used to be. Reading the artefact
// changed four things:
//
//   - The Ministry publishes SRBDT for **UBL only**: every context in the file is
//     "/ubl:Invoice | /cn:CreditNote" or a UBL path beneath it, and there is no CII
//     binding. This package evaluated all fourteen rules on the shared
//     syntax-neutral model — C32's eight-rule defect, again.
//   - RSR-03 is one identifier over two contexts with two different code sets:
//     cbc:InvoiceTypeCode must be 380, 383 or 386, and cbc:CreditNoteTypeCode must
//     be 381. This package tested membership of the union {380, 381, 383, 386}
//     without regard to the document element, so an *invoice* declaring the
//     credit-note code 381 passed a rule the Ministry's validator fails it on.
//   - RSR-11 and RSR-21 apply upper-case() before matching the PIB, so "rs123456789"
//     is a conforming identifier. This package matched case-sensitively and reported
//     it.
//   - RSR-16, RSR-17, RSR-20 and RSR-25 are exists() tests. This package required
//     non-empty text for the first, third and fourth, so an invoice carrying
//     <cbc:CityName/> was reported as missing its city; and RSR-20 binds to
//     cac:PartyTaxScheme with no scheme condition, where this package required the
//     VAT scheme, so a buyer identified under another scheme was reported as
//     missing its PIB.
//
// All 46 published identifiers — RSR-01..36, RSK-X-01/05..10 and RSE-01..03 — are
// flagged fatal, so the plain adder is right and the coverage table's fail-safe
// fatal turned out to be the Ministry's own flag. cius_artefacts_test.go checks
// both directions.
//
// Not evaluated: the Serbian VAT-category rules (RSK-X-*), the extension rules
// (RSE-*) and the rest of the RSR set. See Coverage(SourceSRBDT).

// rsInvoiceTypeCodes and rsCreditNoteTypeCodes are RSR-03's two code sets, one per
// document element, quoted from its test.
var (
	rsInvoiceTypeCodes    = map[string]bool{"380": true, "383": true, "386": true}
	rsCreditNoteTypeCodes = map[string]bool{"381": true}
)

// rsPIB matches a Serbian tax identifier (PIB) as RSR-11/21 test it:
// matches(normalize-space(upper-case(.)), '^RS[0-9]+$') with 9 or 13 digits after
// the prefix. The caller upper-cases first, which the artefact does and this
// package did not.
var rsPIB = regexp.MustCompile(`^RS(\d{9}|\d{13})$`)

// ValidateSRBDT validates an invoice XML against the Serbian CIUS (SRBDT): the
// EN 16931 core plus the SRBDT mandatory-term and format rules.
//
// The EN 16931 core accepts either syntax. The SRBDT rules are evaluated for a UBL
// document only, because that is the only binding the Ministry publishes: a CII
// invoice is validated against the core and reported as carrying no SRBDT finding,
// which is what a reference SRBDT validator says about it too.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice or credit note.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. It is a
// statement about the file rather than about the document, and the Report
// returned with it is the zero Report, so a caller who ignores the error cannot
// read the value as clean. See ErrMalformedXML.
//
// The Report names the rule families neither rule set evaluates — the union of
// Coverage(SourceEN16931) and Coverage(SourceSRBDT).
func ValidateSRBDT(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceSRBDT}, validateSRBDT)
}

func validateSRBDT(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931)
	return append(out, validateSRBDTRules(p.inv, p.root)...)
}

// anyChildWith reports whether any direct child of n named group has a descendant
// path leaf — the shape "exists(cac:X/cbc:Y)" takes when X may repeat.
//
// ciiNode.child follows the *first* match at every step, so
// child("PartyTaxScheme", "CompanyID") answers nil for a party whose first tax
// scheme carries no CompanyID and whose second does. An XPath existence test does
// not: exists(cac:PartyTaxScheme/cbc:CompanyID) is true if any of them has one.
//
// It lived in cius_pt.go until the Portuguese rules grew a general location-path
// helper (ptPath) that subsumes it; SRBDT is its only remaining caller, so it moved
// here rather than being generalised for one user.
func anyChildWith(n *ciiNode, group string, leaf ...string) bool {
	for _, g := range n.all(group) {
		if g.child(leaf...) != nil {
			return true
		}
	}
	return false
}

// vatSchemeCompanyIDs returns the PartyTaxScheme/cbc:CompanyID nodes of a UBL party
// whose tax scheme is VAT (vat true) or is not (vat false), which is how RSR-09,
// RSR-10 and RSR-11 partition the same element.
func vatSchemeCompanyIDs(p *ciiNode, vat bool) []*ciiNode {
	var out []*ciiNode
	for _, pts := range p.all("PartyTaxScheme") {
		isVAT := strings.EqualFold(strings.TrimSpace(pts.str("TaxScheme", "ID")), "VAT")
		if isVAT != vat {
			continue
		}
		if id := pts.child("CompanyID"); id != nil {
			out = append(out, id)
		}
	}
	return out
}

// validateSRBDTRules applies the SRBDT rules to a UBL document. It reads the tree
// for the same reason validateCIUSPTRules does: half of these are exists() tests,
// and the syntax-neutral model cannot tell an absent element from an empty one.
func validateSRBDTRules(inv *en16931Invoice, root *ciiNode) []Violation {
	if inv.syntax != "UBL" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceSRBDT)

	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()

	// RSR-03, context cbc:InvoiceTypeCode | cbc:CreditNoteTypeCode, with the
	// permitted set chosen by self::. Two sets, not one union: 381 is a credit-note
	// code and nothing else, and 380/383/386 are invoice codes and nothing else.
	if tc := root.child("InvoiceTypeCode"); tc != nil && !rsInvoiceTypeCodes[strings.TrimSpace(tc.text)] {
		add("RSR-03", "the Invoice type code (BT-3) shall be one of 380, 383, 386")
	}
	if tc := root.child("CreditNoteTypeCode"); tc != nil && !rsCreditNoteTypeCodes[strings.TrimSpace(tc.text)] {
		add("RSR-03", "the Credit note type code (BT-3) shall be 381")
	}

	// RSR-04, context /ubl:Invoice | /cn:CreditNote: not(exists(cbc:TaxPointDate)).
	if root.child("TaxPointDate") != nil {
		add("RSR-04", "the invoice shall not contain a tax point date (BT-7); use the tax point date code (BT-8)")
	}

	// RSR-09/10, context /ubl:Invoice | /cn:CreditNote. The Seller's VAT-scheme
	// company identifier (BT-31) and its non-VAT one (BT-32) are separate
	// existence tests over the same repeated element.
	sellerVAT := vatSchemeCompanyIDs(seller, true)
	if len(sellerVAT) == 0 {
		add("RSR-09", "the invoice shall contain the Seller VAT identifier / PIB (BT-31)")
	}
	if len(vatSchemeCompanyIDs(seller, false)) == 0 {
		add("RSR-10", "the invoice shall contain the Seller tax registration identifier (BT-32)")
	}
	// RSR-11, context
	// cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme[VAT]/cbc:CompanyID —
	// evaluated per element, and case-insensitively.
	for _, id := range sellerVAT {
		if !rsPIB.MatchString(strings.ToUpper(strings.TrimSpace(id.text))) {
			add("RSR-11", "the Seller PIB (BT-31) shall be 'RS' followed by 9 or 13 digits")
			break
		}
	}

	// RSR-13/14: the Seller electronic address (BT-34) exists, with scheme 9948.
	if ep := seller.child("EndpointID"); ep == nil {
		add("RSR-13", "the invoice shall contain the Seller electronic address (BT-34)")
	} else if strings.TrimSpace(ep.attr("schemeID")) != "9948" {
		add("RSR-14", "the Seller electronic address (BT-34) shall use scheme identifier '9948'")
	}
	// RSR-16: exists(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName).
	if seller.child("PostalAddress", "CityName") == nil {
		add("RSR-16", "the invoice shall contain the Seller city (BT-37)")
	}

	// RSR-17: exists(cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID).
	if !anyChildWith(buyer, "PartyLegalEntity", "CompanyID") {
		add("RSR-17", "the invoice shall contain the Buyer registration identifier (BT-47)")
	}
	// RSR-20: exists(cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)
	// — any tax scheme, unlike the Seller's RSR-09, which names the VAT one.
	if !anyChildWith(buyer, "PartyTaxScheme", "CompanyID") {
		add("RSR-20", "the invoice shall contain the Buyer VAT identifier / PIB (BT-48)")
	}
	// RSR-21, context
	// cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID — every
	// one of them, again with no scheme condition.
	for _, pts := range buyer.all("PartyTaxScheme") {
		id := pts.child("CompanyID")
		if id == nil {
			continue
		}
		if !rsPIB.MatchString(strings.ToUpper(strings.TrimSpace(id.text))) {
			add("RSR-21", "the Buyer PIB (BT-48) shall be 'RS' followed by 9 or 13 digits")
			break
		}
	}

	// RSR-22/23: the Buyer electronic address (BT-49) exists, with scheme 9948.
	if ep := buyer.child("EndpointID"); ep == nil {
		add("RSR-22", "the invoice shall contain the Buyer electronic address (BT-49)")
	} else if strings.TrimSpace(ep.attr("schemeID")) != "9948" {
		add("RSR-23", "the Buyer electronic address (BT-49) shall use scheme identifier '9948'")
	}
	// RSR-25: exists(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CityName).
	if buyer.child("PostalAddress", "CityName") == nil {
		add("RSR-25", "the invoice shall contain the Buyer city (BT-52)")
	}

	return out
}
