package formalis

// This file validates the Finnish TEAPPS format (Tieto TEAPPS) — a proprietary
// batch invoice XML (root INVOICE_CENTER, carrying a transport frame and one or
// more INVOICE documents). It is XSD-validated rather than rule-validated and is
// deeply nested, so this checks only the mandatory structure common to the
// format: each invoice's type and customer information.
//
// Rule identifiers are TP-* (this package's own). Not vendored: the TEAPPS sample
// instances (phax/phive-rules) are used only as the oracle.

// IsTEAPPS reports whether the XML is a TEAPPS batch document.
func IsTEAPPS(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "INVOICE_CENTER"
}

// ValidateTEAPPS validates a Finnish TEAPPS batch against its mandatory structure.
func ValidateTEAPPS(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	if root.name != "INVOICE_CENTER" {
		return []Violation{{Rule: "TP-root", Message: "the document root shall be INVOICE_CENTER"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

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
