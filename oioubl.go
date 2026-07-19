package formalis

import (
	"fmt"
	"strings"
)

// This file validates the Danish OIOUBL format (oioubl.info) — Denmark's UBL 2.0
// national profile (CustomizationID "OIOUBL-2.x"). OIOUBL has an extensive
// Schematron, but it is layered across many included rule sets; this validator
// checks the OIOUBL-mandatory structure directly against the parsed UBL tree —
// the profile, the core document terms, the seller and buyer electronic
// addresses (which OIOUBL requires) and the seller name.
//
// Rule identifiers are OIO-* (this package's own). Not vendored: the OIOUBL
// sample instances (phax/phive-rules) are used only as the oracle.

// IsOIOUBL reports whether the XML is an OIOUBL Invoice.
func IsOIOUBL(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "Invoice" && strings.Contains(root.str("CustomizationID"), "OIOUBL")
}

// ValidateOIOUBL validates a Danish OIOUBL Invoice against its mandatory structure.
func ValidateOIOUBL(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	if root.name != "Invoice" {
		return []Violation{{Rule: "OIO-root", Message: "the document root shall be an Invoice"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	if !strings.Contains(root.str("CustomizationID"), "OIOUBL") {
		add("OIO-customization", "the CustomizationID shall declare an OIOUBL profile")
	}
	if strings.TrimSpace(root.str("ID")) == "" {
		add("OIO-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("OIO-date", "the invoice shall contain an IssueDate")
	}
	if strings.TrimSpace(root.str("InvoiceTypeCode")) == "" {
		add("OIO-type", "the invoice shall contain an InvoiceTypeCode")
	}
	if cur := strings.TrimSpace(root.str("DocumentCurrencyCode")); !en16931Currencies[cur] {
		add("OIO-currency", fmt.Sprintf("the DocumentCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// OIOUBL requires an electronic address (EndpointID) for both parties.
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()
	if strings.TrimSpace(seller.str("EndpointID")) == "" {
		add("OIO-seller-endpoint", "the Seller shall contain an electronic address (EndpointID)")
	}
	if strings.TrimSpace(buyer.str("EndpointID")) == "" {
		add("OIO-buyer-endpoint", "the Buyer shall contain an electronic address (EndpointID)")
	}
	if strings.TrimSpace(seller.str("PartyName", "Name")) == "" &&
		strings.TrimSpace(seller.str("PartyLegalEntity", "RegistrationName")) == "" {
		add("OIO-seller-name", "the Seller shall contain a party name")
	}

	return out
}
