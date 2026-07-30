package formalis

import "context"

// This file validates the Portuguese CIUS-PT (urn:feap.gov.pt:CIUS-PT) on top of
// the EN 16931 core. CIUS-PT is the AT/eSPap public-sector profile; it makes
// several EN 16931-optional terms mandatory — the parties' tax-scheme identifiers,
// the Seller and Deliver-to postal addresses, the document totals and total VAT
// amount, and a delivery.
//
// The rules below are transcribed from the vendored Schematron rather than from
// the specification's prose, which is what they used to be. Reading the artefact
// corrected four of the twelve and moved all twelve off the syntax-neutral model:
//
//   - AT/eSPap publishes CIUS-PT for **UBL only**. urn_feap.gov.pt_CIUS-PT_*.sch
//     ships an abstract half and a UBL binding and no CII binding at all, and every
//     context in the abstract model resolves through a UBL <param>. This package
//     evaluated all twelve rules on the shared model, so every Factur-X/ZUGFeRD
//     invoice was liable to be accused of a Portuguese rule that does not exist for
//     its syntax — C32's eight-rule defect, again.
//   - BR-CIUS-PT-01 and -03 bind to exists(…/cac:PartyTaxScheme/cbc:CompanyID),
//     over *any* tax scheme. This package required the VAT scheme, which is
//     BR-CIUS-PT-02 and -04's separate job, so a seller identified under a non-VAT
//     scheme was reported as missing an identifier it had.
//   - BR-CIUS-PT-05/06/07/11/21/22/23 bind to exists(). This package tested for
//     non-empty text, so a conforming-by-the-artefact invoice carrying
//     <cbc:CityName/> was reported as missing its city.
//   - BR-CIUS-PT-64 permits four things, not two: an actual delivery date (BT-72),
//     a Deliver-to party (BT-70), a Deliver-to location identifier (BT-71) *or* a
//     Deliver-to address (BG-15). This package accepted only the first and the
//     last, so an invoice naming the party it delivered to was accused.
//
// Every published identifier is flagged fatal — 65 BR-CIUS-PT-* and 290
// DT-CIUS-PT-* — so the plain adder is right, and the coverage table's fail-safe
// fatal turned out to be the authority's own flag. cius_artefacts_test.go checks
// both directions.
//
// Not evaluated: the Portuguese VAT-category rate rules, the conditional
// structural-completeness rules, and the 290 DT-CIUS-PT-* datatype rules. See
// Coverage(SourceCIUSPT).

// ValidateCIUSPT validates an invoice XML against the Portuguese CIUS-PT: the
// EN 16931 core plus the CIUS-PT mandatory-term rules.
//
// The EN 16931 core accepts either syntax. The CIUS-PT rules are evaluated for a
// UBL document only, because that is the only binding AT/eSPap publishes: a CII
// invoice is validated against the core and reported as carrying no CIUS-PT
// finding, which is what a reference CIUS-PT validator says about it too.
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
// Coverage(SourceEN16931) and Coverage(SourceCIUSPT). This is the call the
// coverage machinery was built for: a document with no findings is not a
// document that passed CIUS-PT, because BR-CIUS-PT-13/15/17/18, 24..63 and the
// 290 DT-CIUS-PT-* rules were never run, and Report.Conformant says so.
func ValidateCIUSPT(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceCIUSPT}, validateCIUSPT)
}

func validateCIUSPT(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931)
	return append(out, validateCIUSPTRules(p.inv, p.root)...)
}

// anyChildWith reports whether any direct child of n named group has a descendant
// path leaf — the shape "exists(cac:X/cbc:Y)" takes when X may repeat.
//
// ciiNode.child follows the *first* match at every step, so
// child("PartyTaxScheme", "CompanyID") answers nil for a party whose first tax
// scheme carries no CompanyID and whose second does. An XPath existence test does
// not: exists(cac:PartyTaxScheme/cbc:CompanyID) is true if any of them has one.
func anyChildWith(n *ciiNode, group string, leaf ...string) bool {
	for _, g := range n.all(group) {
		if g.child(leaf...) != nil {
			return true
		}
	}
	return false
}

// validateCIUSPTRules applies the CIUS-PT mandatory-term rules to a UBL document.
//
// It reads the tree rather than the syntax-neutral model, and that is not a style
// choice: seven of these rules are exists() tests, and the model carries a trimmed
// string for each term, in which an element that is present and empty is
// indistinguishable from one that is absent. Transcribing an exists() test onto a
// non-empty string test is how four Peppol rules came to report invoices
// OpenPEPPOL's own fixtures hold up as conforming (C32).
func validateCIUSPTRules(inv *en16931Invoice, root *ciiNode) []Violation {
	if inv.syntax != "UBL" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceCIUSPT)

	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()

	// BR-CIUS-PT-01/03, context $Invoice:
	//   exists(cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)
	//   exists(cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)
	// Any tax scheme: that the scheme is VAT is BR-CIUS-PT-02/04, which this
	// package does not evaluate.
	if !anyChildWith(seller, "PartyTaxScheme", "CompanyID") {
		add("BR-CIUS-PT-01", "the Invoice shall contain the Seller VAT identifier (BT-31)")
	}
	if !anyChildWith(buyer, "PartyTaxScheme", "CompanyID") {
		add("BR-CIUS-PT-03", "the Invoice shall contain the Buyer VAT identifier (BT-48)")
	}

	// BR-CIUS-PT-05/06/07, context
	// $Seller_postal_address = cac:AccountingSupplierParty/cac:Party/cac:PostalAddress:
	//   exists(cbc:StreetName) / exists(cbc:CityName) / exists(cbc:PostalZone)
	// The context is the address, so an invoice with no Seller postal address at all
	// trips BR-08 in the core and none of these three.
	if addr := seller.child("PostalAddress"); addr != nil {
		if addr.child("StreetName") == nil {
			add("BR-CIUS-PT-05", "the Seller postal address (BG-5) shall contain a Seller address line 1 (BT-35)")
		}
		if addr.child("CityName") == nil {
			add("BR-CIUS-PT-06", "the Seller postal address (BG-5) shall contain a Seller city (BT-37)")
		}
		if addr.child("PostalZone") == nil {
			add("BR-CIUS-PT-07", "the Seller postal address (BG-5) shall contain a Seller post code (BT-38)")
		}
	}

	// BR-CIUS-PT-10/11, context $Invoice:
	//   exists(cac:LegalMonetaryTotal)
	//   exists(cac:TaxTotal/cbc:TaxAmount)
	if root.child("LegalMonetaryTotal") == nil {
		add("BR-CIUS-PT-10", "the Invoice shall contain the Document totals (BG-22)")
	}
	if !anyChildWith(root, "TaxTotal", "TaxAmount") {
		add("BR-CIUS-PT-11", "the Invoice shall contain the Total VAT amount (BT-110)")
	}

	// BR-CIUS-PT-66, context $Invoice:
	//   exists(cac:Delivery/cac:DeliveryLocation/cac:Address)
	deliverTo := root.child("Delivery", "DeliveryLocation", "Address")
	if deliverTo == nil {
		add("BR-CIUS-PT-66", "the Invoice shall contain at least one Deliver to address (BG-15)")
	}

	// BR-CIUS-PT-64, context $Delivery = cac:Delivery:
	//   exists(cbc:ActualDeliveryDate) or exists(cac:DeliveryParty)
	//     or exists(cac:DeliveryLocation/cbc:ID) or exists(cac:DeliveryLocation/cac:Address)
	// Four alternatives, and the rule only applies where a cac:Delivery exists.
	for _, d := range root.all("Delivery") {
		if d.child("ActualDeliveryDate") == nil && d.child("DeliveryParty") == nil &&
			d.child("DeliveryLocation", "ID") == nil && d.child("DeliveryLocation", "Address") == nil {
			add("BR-CIUS-PT-64", "the Actual delivery date (BT-72), the Deliver to party name (BT-70), the Deliver "+
				"to location identifier (BT-71) or the Deliver to address (BG-15) shall be present")
		}
	}

	// BR-CIUS-PT-21/22/23, context
	// $Deliver_to_address = cac:Delivery/cac:DeliveryLocation/cac:Address:
	//   exists(cbc:StreetName) / exists(cbc:CityName) / exists(cbc:PostalZone)
	if deliverTo != nil {
		if deliverTo.child("StreetName") == nil {
			add("BR-CIUS-PT-21", "each Deliver to address (BG-15) shall contain an address line 1 (BT-75)")
		}
		if deliverTo.child("CityName") == nil {
			add("BR-CIUS-PT-22", "each Deliver to address (BG-15) shall contain a city (BT-77)")
		}
		if deliverTo.child("PostalZone") == nil {
			add("BR-CIUS-PT-23", "each Deliver to address (BG-15) shall contain a post code (BT-78)")
		}
	}

	return out
}
