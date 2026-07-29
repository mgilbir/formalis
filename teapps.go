package formalis

import "context"

// This file validates the Finnish TEAPPS format (Tieto TEAPPS) — a proprietary
// batch invoice XML (root INVOICE_CENTER, carrying a transport frame and one or
// more INVOICE documents). It is XSD-validated rather than rule-validated and is
// deeply nested, so this checks only the mandatory structure common to the
// format: each invoice's type and customer information.
//
// Rule identifiers are TP-* (this package's own). Not vendored: the TEAPPS sample
// instances (phax/phive-rules) are used only as the oracle.

// IsTEAPPS reports whether the XML is a TEAPPS batch document.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
func IsTEAPPS(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "INVOICE_CENTER", nil
}

// ValidateTEAPPS validates a Finnish TEAPPS batch against its mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateTEAPPS(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	root, err := parseCII(r, xmlData)
	if err != nil {
		return r.finish(syntaxViolation(err))
	}
	return r.finish(validateTEAPPS(r, root))
}

func validateTEAPPS(r *run, root *ciiNode) []Violation {
	if root.name != "INVOICE_CENTER" {
		return []Violation{{Source: SourceTEAPPS, Rule: "TP-root", Message: "the document root shall be INVOICE_CENTER"}}
	}
	var out []Violation
	add := adder(&out, SourceTEAPPS)

	invoices := root.findAll("INVOICE")
	if len(invoices) == 0 {
		add("TP-invoice", "the batch shall contain at least one INVOICE")
	}
	for _, inv := range invoices {
		if len(inv.findAll("INVOICE_TYPE")) == 0 {
			add("TP-type", "each INVOICE shall contain an INVOICE_TYPE")
		}
		if len(inv.findAll("CUSTOMER_INFORMATION")) == 0 {
			add("TP-customer", "each INVOICE shall contain CUSTOMER_INFORMATION")
		}
	}

	return out
}
