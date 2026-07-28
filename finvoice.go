package formalis

import (
	"context"
	"strings"
)

// This file validates the Finnish Finvoice format (finvoice.info / Finance
// Finland) — Finland's national e-invoice XML. Like the other national formats it
// is XSD-validated rather than rule-validated, so this checks the mandatory
// structure directly against the parsed tree.
//
// Rule identifiers are FI-* (this package's own). Not vendored: the Finvoice
// sample instances (phax/phive-rules) are used only as the oracle.

// IsFinvoice reports whether the XML is a Finvoice document.
func IsFinvoice(xmlData []byte) bool {
	r := newRun(nil)
	root, err := parseCII(r, xmlData)
	return err == nil && root.name == "Finvoice"
}

// ValidateFinvoice validates a Finnish Finvoice document against its mandatory
// structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateFinvoice(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	return r.finish(validateFinvoice(r, xmlData))
}

func validateFinvoice(r *run, xmlData []byte) []Violation {
	root, err := parseCII(r, xmlData)
	if err != nil {
		return syntaxViolation(err)
	}
	if root.name != "Finvoice" {
		return []Violation{{Rule: "FI-root", Message: "the document root shall be Finvoice"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// FI-seller: the seller party has an organisation name and a postal address.
	sp := root.child("SellerPartyDetails").orNil()
	if strings.TrimSpace(sp.str("SellerOrganisationName")) == "" {
		add("FI-seller-name", "the SellerPartyDetails shall contain a SellerOrganisationName")
	}
	if sp.child("SellerPostalAddressDetails") == nil {
		add("FI-seller-address", "the SellerPartyDetails shall contain a SellerPostalAddressDetails")
	}

	// FI-buyer: the buyer party has an organisation (or person) name.
	bp := root.child("BuyerPartyDetails").orNil()
	if strings.TrimSpace(bp.str("BuyerOrganisationName")) == "" &&
		strings.TrimSpace(root.str("InvoiceRecipientPartyDetails", "InvoiceRecipientOrganisationName")) == "" {
		add("FI-buyer-name", "the BuyerPartyDetails shall contain a BuyerOrganisationName")
	}

	// FI-invoice: the invoice details carry a type code, number and date.
	id := root.child("InvoiceDetails").orNil()
	if strings.TrimSpace(id.str("InvoiceTypeCode")) == "" {
		add("FI-type", "the InvoiceDetails shall contain an InvoiceTypeCode")
	}
	if strings.TrimSpace(id.str("InvoiceNumber")) == "" {
		add("FI-number", "the InvoiceDetails shall contain an InvoiceNumber")
	}
	if strings.TrimSpace(id.str("InvoiceDate")) == "" {
		add("FI-date", "the InvoiceDetails shall contain an InvoiceDate")
	}

	return out
}
