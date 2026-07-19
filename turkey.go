package formalis

import (
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
func IsTurkishInvoice(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "Invoice" && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(root.str("CustomizationID"))), "TR")
}

// ValidateTurkishInvoice validates a Turkish UBL-TR Invoice against its mandatory
// structure.
func ValidateTurkishInvoice(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	if root.name != "Invoice" {
		return []Violation{{Rule: "TR-root", Message: "the document root shall be an Invoice"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

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

	return out
}
