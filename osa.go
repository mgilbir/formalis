package formalis

import "strings"

// This file validates the Hungarian NAV Online Számla (OSA) invoice-data format —
// the real-time invoice report submitted to the Hungarian tax authority (root
// InvoiceData). It is XSD-validated rather than rule-validated, so this checks the
// mandatory structure directly against the parsed tree.
//
// Rule identifiers are HU-* (this package's own). Not vendored: the OSA sample
// instances (phax/phive-rules) are used only as the oracle.

// firstNonEmptyText returns the first non-empty trimmed text among the nodes.
func firstNonEmptyText(ns []*ciiNode) string {
	for _, n := range ns {
		if t := strings.TrimSpace(n.text); t != "" {
			return t
		}
	}
	return ""
}

// IsOSA reports whether the XML is a Hungarian OSA InvoiceData document.
func IsOSA(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "InvoiceData"
}

// ValidateOSA validates a Hungarian OSA invoice-data document against its
// mandatory structure.
func ValidateOSA(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	if root.name != "InvoiceData" {
		return []Violation{{Rule: "HU-root", Message: "the document root shall be InvoiceData"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	if strings.TrimSpace(root.str("invoiceNumber")) == "" {
		add("HU-number", "the document shall contain an invoiceNumber")
	}
	if strings.TrimSpace(root.str("invoiceIssueDate")) == "" {
		add("HU-date", "the document shall contain an invoiceIssueDate")
	}
	if firstNonEmptyText(root.findAll("supplierName")) == "" {
		add("HU-supplier-name", "the supplier information shall contain a supplierName")
	}
	if firstNonEmptyText(root.findAll("taxpayerId")) == "" {
		add("HU-supplier-tax", "the supplier information shall contain a tax number (supplierTaxNumber/taxpayerId)")
	}
	if len(root.findAll("customerInfo")) == 0 {
		add("HU-customer", "the invoice shall contain customerInfo")
	}

	return out
}
