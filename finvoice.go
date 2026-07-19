package formalis

import "strings"

// This file validates the Finnish Finvoice format (finvoice.info / Finance
// Finland) — Finland's national e-invoice XML. Like the other national formats it
// is XSD-validated rather than rule-validated, so this checks the mandatory
// structure directly against the parsed tree.
//
// Rule identifiers are FI-* (this package's own). Not vendored: the Finvoice
// sample instances (phax/phive-rules) are used only as the oracle.

// IsFinvoice reports whether the XML is a Finvoice document.
func IsFinvoice(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "Finvoice"
}

// ValidateFinvoice validates a Finnish Finvoice document against its mandatory
// structure.
func ValidateFinvoice(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
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
