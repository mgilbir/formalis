package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Turkish UBL-TR invoice format (the GİB e-Fatura /
// e-Arşiv profile, CustomizationID "TR1.x"). UBL-TR is a UBL 2.1 profile; this
// checks the UBL-TR-mandatory structure directly against the parsed UBL tree —
// the UUID and profile, the core document terms and the seller party. Only the
// invoice document type is validated (the UBL-TR family also covers despatch
// advices and responses, which have other roots).
//
// Rule identifiers are TR-* (this package's own). Not vendored: the UBL-TR sample
// instances (phax/phive-rules) are used only as the oracle.

// IsTurkishInvoice reports whether the XML is a UBL-TR Invoice.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
func IsTurkishInvoice(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Invoice" &&
		strings.HasPrefix(strings.ToUpper(strings.TrimSpace(d.str("CustomizationID"))), "TR"), nil
}

// ValidateTurkishInvoice validates a Turkish UBL-TR Invoice against its mandatory
// structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateTurkishInvoice(ctx context.Context, xmlData []byte) []Violation {
	return turkishValidator.validate(ctx, xmlData)
}

var turkishValidator = treeValidator{
	source:   SourceUBLTR,
	rootRule: "TR-root",
	rootMsg:  "the document root shall be an Invoice",
	accepts:  rootNamed("Invoice"),
	check:    checkTurkishInvoice,
}

func checkTurkishInvoice(root *ciiNode, add func(rule, msg string)) {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(root.str("CustomizationID"))), "TR") {
		add("TR-customization", "the CustomizationID shall declare a UBL-TR profile (TR1.x)")
	}
	if strings.TrimSpace(root.str("ProfileID")) == "" {
		add("TR-profile", "the invoice shall contain a ProfileID")
	}
	if strings.TrimSpace(root.str("UUID")) == "" {
		add("TR-uuid", "the invoice shall contain a UUID")
	}
	if strings.TrimSpace(root.str("ID")) == "" {
		add("TR-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("TR-date", "the invoice shall contain an IssueDate")
	}
	if strings.TrimSpace(root.str("InvoiceTypeCode")) == "" {
		add("TR-type", "the invoice shall contain an InvoiceTypeCode")
	}
	if cur := strings.TrimSpace(root.str("DocumentCurrencyCode")); !en16931Currencies[cur] {
		add("TR-currency", fmt.Sprintf("the DocumentCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// The seller party (AccountingSupplierParty) shall carry a tax identifier
	// (PartyIdentification/ID — a VKN company or TCKN citizen number).
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	if strings.TrimSpace(seller.str("PartyIdentification", "ID")) == "" {
		add("TR-seller-id", "the Seller (AccountingSupplierParty) shall contain a tax identifier (VKN/TCKN)")
	}
}
