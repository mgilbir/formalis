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
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
func IsFinvoice(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Finvoice", nil
}

// ValidateFinvoice validates a Finnish Finvoice document against its mandatory
// structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateFinvoice(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	root, err := parseCII(r, xmlData)
	if err != nil {
		return r.finish(syntaxViolation(err))
	}
	return r.finish(validateFinvoice(r, root))
}

func validateFinvoice(r *run, root *ciiNode) []Violation {
	if root.name != "Finvoice" {
		return []Violation{{Source: SourceFinvoice, Rule: "FI-root", Message: "the document root shall be Finvoice"}}
	}
	var out []Violation
	add := adder(&out, SourceFinvoice)

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
