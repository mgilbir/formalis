package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Saudi ZATCA e-invoice (Fatoora) format. ZATCA invoices
// are UBL 2.1 Invoice/CreditNote documents with Kingdom-of-Saudi-Arabia specific
// requirements, exchanged through the ZATCA reporting/clearance platform. There
// is no business-rule Schematron (XSD + platform checks), so this validates the
// KSA-mandatory structure directly against the parsed UBL tree — the UUID, the
// mandatory ICV (invoice counter), PIH (previous invoice hash) and QR additional
// document references, the seller VAT registration, and the core document terms.
//
// Rule identifiers are ZA-* (this package's own). Not vendored: the ZATCA sample
// instances (phax/phive-rules) are used only as the oracle.

// zatcaDocRef reports whether the invoice carries an AdditionalDocumentReference
// with the given cbc:ID (e.g. "ICV", "PIH", "QR").
func zatcaDocRef(root *ciiNode, id string) bool {
	for _, r := range root.findAll("AdditionalDocumentReference") {
		if strings.TrimSpace(r.str("ID")) == id {
			return true
		}
	}
	return false
}

// IsZATCA reports whether the XML is a ZATCA (Fatoora) UBL invoice, identified by
// its reporting/clearance ProfileID.
func IsZATCA(xmlData []byte) bool {
	r := newRun(nil)
	root, err := parseCII(r, xmlData)
	if err != nil || (root.name != "Invoice" && root.name != "CreditNote") {
		return false
	}
	return strings.Contains(strings.ToLower(root.str("ProfileID")), "reporting") || zatcaDocRef(root, "ICV")
}

// ValidateZATCA validates a Saudi ZATCA UBL invoice against its KSA-mandatory
// structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateZATCA(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	return r.finish(validateZATCA(r, xmlData))
}

func validateZATCA(r *run, xmlData []byte) []Violation {
	root, err := parseCII(r, xmlData)
	if err != nil {
		return syntaxViolation(err)
	}
	if root.name != "Invoice" && root.name != "CreditNote" {
		return []Violation{{Rule: "ZA-root", Message: "the document root shall be a UBL Invoice or CreditNote"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// ZA-number/date/uuid: the document id, issue date and UUID are mandatory.
	if strings.TrimSpace(root.str("ID")) == "" {
		add("ZA-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("ZA-date", "the invoice shall contain an IssueDate")
	}
	if strings.TrimSpace(root.str("UUID")) == "" {
		add("ZA-uuid", "the invoice shall contain a UUID")
	}

	// ZA-type/currency: the type code and currency are mandatory.
	if strings.TrimSpace(root.str("InvoiceTypeCode")) == "" && strings.TrimSpace(root.str("CreditNoteTypeCode")) == "" {
		add("ZA-type", "the invoice shall contain an InvoiceTypeCode")
	}
	if cur := strings.TrimSpace(root.str("DocumentCurrencyCode")); !en16931Currencies[cur] {
		add("ZA-currency", fmt.Sprintf("the DocumentCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// ZA-icv/pih/qr: the KSA additional document references are mandatory — the
	// invoice counter value (ICV), the previous invoice hash (PIH) and the QR code.
	if !zatcaDocRef(root, "ICV") {
		add("ZA-icv", "the invoice shall contain an ICV (invoice counter value) AdditionalDocumentReference")
	}
	if !zatcaDocRef(root, "PIH") {
		add("ZA-pih", "the invoice shall contain a PIH (previous invoice hash) AdditionalDocumentReference")
	}
	if !zatcaDocRef(root, "QR") {
		add("ZA-qr", "the invoice shall contain a QR AdditionalDocumentReference")
	}

	// ZA-seller-vat: the seller VAT registration (PartyTaxScheme/CompanyID) is
	// mandatory.
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	if strings.TrimSpace(seller.str("PartyTaxScheme", "CompanyID")) == "" {
		add("ZA-seller-vat", "the Seller shall contain a VAT registration (PartyTaxScheme/CompanyID)")
	}

	return out
}
