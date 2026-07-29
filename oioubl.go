package formalis

import (
	"context"
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
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
func IsOIOUBL(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Invoice" && strings.Contains(d.str("CustomizationID"), "OIOUBL"), nil
}

// ValidateOIOUBL validates a Danish OIOUBL Invoice against its mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateOIOUBL(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	root, err := parseCII(r, xmlData)
	if err != nil {
		return r.finish(syntaxViolation(err))
	}
	return r.finish(validateOIOUBL(r, root))
}

func validateOIOUBL(r *run, root *ciiNode) []Violation {
	if root.name != "Invoice" {
		return []Violation{{Source: SourceOIOUBL, Rule: "OIO-root", Message: "the document root shall be an Invoice"}}
	}
	var out []Violation
	add := adder(&out, SourceOIOUBL)

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
