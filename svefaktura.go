package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Swedish Svefaktura format (SFTI Svefaktura 1.0) — the
// legacy Swedish e-invoice, a UBL 1.0 profile. It is XSD-validated rather than
// rule-validated, so this checks the mandatory structure against the parsed tree.
// The root element is "Invoice" (as in UBL 2.1), so it is disambiguated by the
// UBL-1.0-style SellerParty child (UBL 2.1 uses AccountingSupplierParty).
//
// Rule identifiers are SV-* (this package's own). Not vendored: the Svefaktura
// sample instances (phax/phive-rules) are used only as the oracle.

// IsSvefaktura reports whether the XML is an SFTI Svefaktura invoice.
func IsSvefaktura(xmlData []byte) bool {
	r := newRun(nil)
	root, err := parseCII(r, xmlData)
	return err == nil && root.name == "Invoice" && root.child("SellerParty") != nil
}

// ValidateSvefaktura validates a Swedish Svefaktura document against its
// mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateSvefaktura(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	return r.finish(validateSvefaktura(r, xmlData))
}

func validateSvefaktura(r *run, xmlData []byte) []Violation {
	root, err := parseCII(r, xmlData)
	if err != nil {
		return syntaxViolation(err)
	}
	if root.name != "Invoice" || root.child("SellerParty") == nil {
		return []Violation{{Rule: "SV-root", Message: "the document root shall be a Svefaktura Invoice with a SellerParty"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	if strings.TrimSpace(root.str("ID")) == "" {
		add("SV-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("SV-date", "the invoice shall contain an IssueDate")
	}
	if cur := strings.TrimSpace(root.str("InvoiceCurrencyCode")); !en16931Currencies[cur] {
		add("SV-currency", fmt.Sprintf("the InvoiceCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}
	if strings.TrimSpace(root.str("SellerParty", "Party", "PartyName", "Name")) == "" {
		add("SV-seller", "the SellerParty shall contain a party name")
	}
	if strings.TrimSpace(root.str("BuyerParty", "Party", "PartyName", "Name")) == "" {
		add("SV-buyer", "the BuyerParty shall contain a party name")
	}

	return out
}
