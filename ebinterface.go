package formalis

import "strings"

// This file validates the Austrian ebInterface format (ebinterface.at) — the
// Austrian national e-invoice XML, across its schema versions (3.x … 6.x). Like
// the other national formats it is XSD-validated rather than rule-validated, so
// this checks the mandatory structure directly against the parsed tree. The
// element set below (invoice number and date, biller and recipient with a VAT
// identifier and an address name) is common to every ebInterface version.
//
// Rule identifiers are EB-* (this package's own). Not vendored: the ebInterface
// sample instances (phax/phive-rules) are used only as the oracle.

// IsEbInterface reports whether the XML is an ebInterface document. The root
// element is "Invoice" (as in UBL), so it is disambiguated by the ebInterface-
// specific Biller element.
func IsEbInterface(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "Invoice" && root.child("Biller") != nil
}

// ValidateEbInterface validates an Austrian ebInterface document against its
// mandatory structure.
func ValidateEbInterface(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	if root.name != "Invoice" || root.child("Biller") == nil {
		return []Violation{{Rule: "EB-root", Message: "the document root shall be an ebInterface Invoice with a Biller"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// EB-number/EB-date: the invoice number and date elements are mandatory. The
	// number is checked for presence only (the ebInterface XSD, and thus the
	// official validation, accepts an empty element).
	if root.child("InvoiceNumber") == nil {
		add("EB-number", "the invoice shall contain an InvoiceNumber")
	}
	if strings.TrimSpace(root.str("InvoiceDate")) == "" {
		add("EB-date", "the invoice shall contain an InvoiceDate")
	}

	// EB-biller: the Biller has a VAT identifier and an address with a name.
	biller := root.child("Biller").orNil()
	if strings.TrimSpace(biller.str("VATIdentificationNumber")) == "" {
		add("EB-biller-vat", "the Biller shall contain a VATIdentificationNumber")
	}
	if strings.TrimSpace(biller.str("Address", "Name")) == "" {
		add("EB-biller-name", "the Biller address shall contain a Name")
	}

	// EB-recipient: the InvoiceRecipient is present with an address name.
	rec := root.child("InvoiceRecipient").orNil()
	if rec.name == "" {
		add("EB-recipient", "the invoice shall contain an InvoiceRecipient")
	} else if strings.TrimSpace(rec.str("Address", "Name")) == "" {
		add("EB-recipient-name", "the InvoiceRecipient address shall contain a Name")
	}

	return out
}
