package formalis

import (
	"context"
	"errors"
	"fmt"
)

// This file validates the embedded Cross Industry Order document of an Order-X
// (a.k.a. ZUGFeRD Order) — the order-document sibling of Factur-X. Order-X
// business rules differ from EN 16931 (which is invoice-specific), so this checks
// the order's structure and mandatory head terms, reusing the shared CII parser.
// The PDF container around it is validated in the pdf0 package.
//
// # Why the identifiers read ORDER-01 and not BR-O-01
//
// These five checks are this package's own, not quotations from a published
// Order-X rule set, so they are numbered in this package's own namespace the way
// FPA-*, FE-*, EB-* and ZA-* are. They used to be called BR-O-01…BR-O-05, which
// was a name CEN had already taken: BR-O-01 … BR-O-14 are EN 16931's rules for
// the "Not subject to VAT" (O) category, and the conformance corpus this package
// is tested against ships CEN's own unit tests for them under exactly those
// names (testdata/en16931-artefacts/test/Invoice-unit-UBL/BR-O-01.xml …).
// The rule engine emits them from validateVATCategories and
// validateVATIdentifiers, so one string meant two unrelated things depending on
// which validator produced it, and a caller aggregating a mailbox by rule
// identifier merged the two. Violation.Source now scopes every finding, but a
// name minted inside another authority's numbering is worth correcting on its
// own account: a reader who sees BR-O-03 in a log has no reason to check the
// scope before believing it is the CEN rule.
//
// If Order-X should later publish business rules under identifiers of its own,
// quote those and retire these.

// orderXTypeCodes is the order document type code set (UNTDID 1001): order (220),
// order change (230), order response (231).
var orderXTypeCodes = map[string]bool{"220": true, "230": true, "231": true}

// ValidateOrderXML checks an embedded Cross Industry Order's structure and
// mandatory head business terms, returning any violations.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateOrderXML(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	root, err := parseCII(r, xmlData)
	if errors.Is(err, errStopped) {
		// A run that stopped says nothing about the order. Reporting "not
		// well-formed" here would accuse a document the checker never finished
		// reading; the trip itself is already recorded on the run.
		return r.finish(nil)
	}
	if err != nil {
		return r.finish([]Violation{notAnOrder})
	}
	return r.finish(validateOrderXML(r, root))
}

// notAnOrder is the finding for XML that is not a Cross Industry Order.
var notAnOrder = Violation{
	Source:  SourceOrderX,
	Rule:    "order-xml",
	Message: "the order XML is not a well-formed Cross Industry Order (SCRDMCCBDACIOMessageStructure)",
}

func validateOrderXML(r *run, root *ciiNode) []Violation {
	var out []Violation
	add := adder(&out, SourceOrderX)

	if root.name != "SCRDMCCBDACIOMessageStructure" {
		return []Violation{notAnOrder}
	}
	doc := root.child("ExchangedDocument").orNil()
	agr := root.child("SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement").orNil()

	if doc.str("ID") == "" {
		add("ORDER-01", "an Order shall have an order number (ExchangedDocument/ID)")
	}
	if doc.str("IssueDateTime", "DateTimeString") == "" {
		add("ORDER-02", "an Order shall have an issue date (ExchangedDocument/IssueDateTime)")
	}
	if tc := doc.str("TypeCode"); tc == "" {
		add("ORDER-03", "an Order shall have a document type code (ExchangedDocument/TypeCode)")
	} else if !orderXTypeCodes[tc] {
		add("ORDER-03", fmt.Sprintf("order type code %q is not a permitted UNTDID 1001 order value (220/230/231)", tc))
	}
	if agr.str("BuyerTradeParty", "Name") == "" {
		add("ORDER-04", "an Order shall contain the Buyer name")
	}
	if agr.str("SellerTradeParty", "Name") == "" {
		add("ORDER-05", "an Order shall contain the Seller name")
	}
	return out
}
