package formalis

import (
	"context"
	"strings"
)

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
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
func IsEbInterface(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Invoice" && d.hasBiller, nil
}

// ValidateEbInterface validates an Austrian ebInterface document against its
// mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateEbInterface(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	root, err := parseCII(r, xmlData)
	if err != nil {
		return r.finish(syntaxViolation(err))
	}
	return r.finish(validateEbInterface(r, root))
}

func validateEbInterface(r *run, root *ciiNode) []Violation {
	if root.name != "Invoice" || root.child("Biller") == nil {
		return []Violation{{Source: SourceEbInterface, Rule: "EB-root", Message: "the document root shall be an ebInterface Invoice with a Biller"}}
	}
	var out []Violation
	add := adder(&out, SourceEbInterface)

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
