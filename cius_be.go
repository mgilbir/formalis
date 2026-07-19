package formalis

import (
	"strconv"
	"strings"
	"time"
)

// This file validates the Belgian UBL.BE CIUS on top of the EN 16931 core. Unlike
// the other CIUS in this package, UBL.BE's own rules (ubl-BE-*) are UBL-structural
// rather than semantic — they check UBL-specific elements (the UBL.BE profile
// markers, delivery terms, settlement discounts, per-line tax totals, tax-category
// names) and Belgian code lists — so they are evaluated against the raw parsed XML
// tree rather than the syntax-neutral model.
//
// The free-text description code lists (BELMText for ubl-BE-06, BVERCText for
// ubl-BE-12) are bilingual exact-match lists that add little validation value and
// are not enforced.
//
// Not vendored: the UBL.BE Schematron and sample instances (phax/phive-rules) are
// used only as the oracle.

// beDeliveryTerms is the BELM delivery-terms code list (ubl-BE-05).
var beDeliveryTerms = map[string]bool{
	"BELM-001": true, "BELM-002": true, "BELM-003": true, "BELM-004": true,
	"BELM-005": true, "BELM-006": true, "BELM-007": true, "BELM-008": true,
}

// beTaxCategoryNames is the BTCC Belgian tax-category code list (ubl-BE-10).
var beTaxCategoryNames = map[string]bool{
	"00": true, "01": true, "02": true, "03": true, "45": true, "NA": true, "FD": true,
	"SC": true, "00/44": true, "03/SE": true, "MA": true, "46/GO": true, "47/TO": true,
	"47/AS": true, "47/DI": true, "47/SE": true, "44": true, "46/TR": true, "47/EX": true,
	"47/EI": true, "47/EE": true, "NS": true, "OSS-S": true, "OSS-G": true, "OSS-I": true,
}

// beExemptionReasonCodes is the BVERC Belgian VAT-exemption reason code list (ubl-BE-11).
var beExemptionReasonCodes = map[string]bool{
	"BETE-45": true, "BETE-EX": true, "BETE-FD": true, "BETE-SC": true, "BETE-00/44": true,
	"BETE-03/SE": true, "BETE-MA": true, "BETE-46/GO": true, "BETE-47/TO": true, "BETE-47/AS": true,
	"BETE-47/DI": true, "BETE-47/SE": true, "BETE-44": true, "BETE-46/TR": true, "BETE-47/EX": true,
	"BETE-47/EI": true, "BETE-47/EE": true, "BETE-NS": true,
}

// ValidateUBLBE validates an invoice XML against the Belgian UBL.BE CIUS: the
// EN 16931 core plus the UBL.BE-specific rules. UBL.BE is a UBL profile; the
// ubl-BE rules are evaluated against the raw XML tree.
func ValidateUBLBE(xmlData []byte) []Violation {
	inv, err := parseEN16931(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	out := validateEN16931(inv, ProfileEN16931)
	if root, err := parseCII(xmlData); err == nil {
		out = append(out, validateUBLBERules(root)...)
	}
	return out
}

func validateUBLBERules(root *ciiNode) []Violation {
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// ubl-BE-02: exactly one AdditionalDocumentReference/DocumentDescription with
	// the value "CommercialInvoice" or "CreditNote".
	docType := 0
	for _, d := range root.findAll("DocumentDescription") {
		if t := strings.TrimSpace(d.text); t == "CommercialInvoice" || t == "CreditNote" {
			docType++
		}
	}
	if docType != 1 {
		add("ubl-BE-02", "exactly one document type (DocumentDescription 'CommercialInvoice' or 'CreditNote') shall be specified")
	}

	// ubl-BE-03: exactly one element cbc:ID with the value "UBL.BE".
	marker := 0
	for _, x := range root.findAll("ID") {
		if strings.TrimSpace(x.text) == "UBL.BE" {
			marker++
		}
	}
	if marker != 1 {
		add("ubl-BE-03", "exactly one cbc:ID with the value 'UBL.BE' shall be present")
	}

	// ubl-BE-05: each Delivery terms identifier shall be in the BELM list.
	for _, dt := range root.findAll("DeliveryTerms") {
		if id := strings.TrimSpace(dt.str("ID")); id != "" && !beDeliveryTerms[id] {
			add("ubl-BE-05", "the Delivery terms ID ("+id+") shall be in the BELM list")
		}
	}

	// ubl-BE-10: each VAT breakdown tax-category name shall be in the BTCC list.
	for _, ts := range root.findAll("TaxSubtotal") {
		if nm := strings.TrimSpace(ts.str("TaxCategory", "Name")); nm != "" && !beTaxCategoryNames[nm] {
			add("ubl-BE-10", "the VAT category name ("+nm+") shall be in the BTCC list")
		}
	}

	// ubl-BE-11: each tax-exemption reason code, when present, shall be in the BVERC list.
	for _, c := range root.findAll("TaxExemptionReasonCode") {
		if v := strings.TrimSpace(c.text); v != "" && !beExemptionReasonCodes[v] {
			add("ubl-BE-11", "the VAT exemption reason code ("+v+") shall be in the BVERC list")
		}
	}

	// ubl-BE-07/08/09: a settlement discount (PaymentTerms/SettlementDiscountPercent)
	// shall be a percentage in (0,100) or -1, and requires an amount and a valid
	// due date (YYYY-MM-DD).
	for _, pt := range root.findAll("PaymentTerms") {
		sdp := strings.TrimSpace(pt.str("SettlementDiscountPercent"))
		if sdp == "" {
			continue
		}
		if v, err := strconv.ParseFloat(sdp, 64); err != nil || !((v > 0 && v < 100) || v == -1) {
			add("ubl-BE-07", "the settlement discount percent shall be numeric and between 0 and 100")
		}
		if strings.TrimSpace(pt.str("Amount")) == "" {
			add("ubl-BE-08", "a settlement discount shall have an Amount")
		}
		due := strings.TrimSpace(pt.str("PaymentDueDate"))
		if _, err := time.Parse("2006-01-02", due); len(due) != 10 || err != nil {
			add("ubl-BE-09", "a settlement discount shall have a PaymentDueDate formatted YYYY-MM-DD")
		}
	}

	// ubl-BE-14/13: each invoice/credit-note line shall have exactly one TaxTotal,
	// carrying a numeric TaxAmount.
	lines := append(root.findAll("InvoiceLine"), root.findAll("CreditNoteLine")...)
	for _, ln := range lines {
		tts := ln.all("TaxTotal")
		if len(tts) != 1 {
			add("ubl-BE-14", "each invoice line shall have exactly one TaxTotal")
			continue
		}
		ta := strings.TrimSpace(tts[0].str("TaxAmount"))
		if _, err := strconv.ParseFloat(ta, 64); ta == "" || err != nil {
			add("ubl-BE-13", "each invoice line TaxTotal shall have a numeric TaxAmount")
		}
	}

	// ubl-BE-15: each classified tax category (line level) shall have a Name.
	for _, ctc := range root.findAll("ClassifiedTaxCategory") {
		if strings.TrimSpace(ctc.str("Name")) == "" {
			add("ubl-BE-15", "each ClassifiedTaxCategory shall have a Name")
		}
	}

	return out
}
