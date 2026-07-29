package formalis

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// This file validates the Peppol PINT (Peppol International) billing model — the
// global evolution of Peppol BIS, a UBL 2.1 profile with a shared core and
// jurisdiction-aligned rule sets (AE, AUNZ, EU, JP, MY, OM, SG …). The
// jurisdiction is declared in the CustomizationID as "…@<jur>-1". This validates
// the PINT-mandatory structure that every jurisdiction shares — the profile, the
// core document terms, the parties' electronic addresses and the seller name;
// DetectPINTJurisdiction exposes the declared jurisdiction for callers that apply
// jurisdiction-specific handling.
//
// Rule identifiers are PINT-* (this package's own). Not vendored: the PINT sample
// instances (phax/phive-rules) are used only as the oracle.

var pintJurisdiction = regexp.MustCompile(`@([a-z]{2,4})-`)

// DetectPINTJurisdiction returns the jurisdiction code declared in a PINT
// CustomizationID (e.g. "eu", "ae", "jp", "my", "sg", "om", "aunz"), or "".
func DetectPINTJurisdiction(customizationID string) string {
	if m := pintJurisdiction.FindStringSubmatch(customizationID); m != nil {
		return m[1]
	}
	return ""
}

// IsPINT reports whether the XML is a Peppol PINT invoice or credit note.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
func IsPINT(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	if d.root != "Invoice" && d.root != "CreditNote" {
		return false, nil
	}
	return strings.Contains(d.str("CustomizationID"), "peppol:pint"), nil
}

// ValidatePINT validates a Peppol PINT document against the mandatory structure
// shared by every jurisdiction.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidatePINT(ctx context.Context, xmlData []byte) []Violation {
	return pintValidator.validate(ctx, xmlData)
}

var pintValidator = treeValidator{
	source:   SourcePINT,
	rootRule: "PINT-root",
	rootMsg:  "the document root shall be a UBL Invoice or CreditNote",
	accepts:  rootNamed("Invoice", "CreditNote"),
	check:    checkPINT,
}

func checkPINT(root *ciiNode, add func(rule, msg string)) {
	if !strings.Contains(root.str("CustomizationID"), "peppol:pint") {
		add("PINT-customization", "the CustomizationID shall declare a Peppol PINT profile")
	}
	if strings.TrimSpace(root.str("ProfileID")) == "" {
		add("PINT-profile", "the document shall contain a ProfileID")
	}
	if strings.TrimSpace(root.str("ID")) == "" {
		add("PINT-number", "the document shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("PINT-date", "the document shall contain an IssueDate")
	}
	if strings.TrimSpace(root.str("InvoiceTypeCode")) == "" && strings.TrimSpace(root.str("CreditNoteTypeCode")) == "" {
		add("PINT-type", "the document shall contain an invoice/credit note type code")
	}
	if cur := strings.TrimSpace(root.str("DocumentCurrencyCode")); !en16931Currencies[cur] {
		add("PINT-currency", fmt.Sprintf("the DocumentCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// Both parties carry an electronic address (EndpointID); the seller carries a
	// name.
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()
	if strings.TrimSpace(seller.str("EndpointID")) == "" {
		add("PINT-seller-endpoint", "the Seller shall contain an electronic address (EndpointID)")
	}
	if strings.TrimSpace(buyer.str("EndpointID")) == "" {
		add("PINT-buyer-endpoint", "the Buyer shall contain an electronic address (EndpointID)")
	}
	if strings.TrimSpace(seller.str("PartyLegalEntity", "RegistrationName")) == "" &&
		strings.TrimSpace(seller.str("PartyName", "Name")) == "" {
		add("PINT-seller-name", "the Seller shall contain a name")
	}
}
