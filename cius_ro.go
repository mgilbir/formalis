package formalis

import "regexp"

// This file validates the Romanian CIUS-RO (RO_CIUS / RO e-Factura, the ANAF
// national profile) on top of the EN 16931 core. CIUS-RO makes several EN 16931-
// optional terms mandatory (the parties' and delivery/tax-representative address
// lines and cities), restricts the invoice type code and the VAT accounting
// currency, and requires a numeric character in the invoice number. The same
// syntax-neutral model feeds it.
//
// CIUS-RO also carries a large set of Romanian county (județ) address-code rules
// (BR-RO-L*, ~64 rules over the SIRUTA/ISO 3166-2:RO subdivision code lists) and
// the related country-subdivision presence rules (BT-39/54/79). Those need a
// county code table and a country-subdivision term the syntax-neutral model does
// not carry, and are not emitted here; the mandatory-term rules below are the
// validity-affecting core.
//
// Not vendored: the CIUS-RO Schematron and sample instances (phax/phive-rules)
// are used only as the oracle.

// roInvoiceTypeCodes is the invoice type code set CIUS-RO permits (BR-RO-020_1).
var roInvoiceTypeCodes = map[string]bool{"380": true, "384": true, "389": true, "751": true}

var roDigit = regexp.MustCompile(`[0-9]`)

// ValidateCIUSRO validates an invoice XML against the Romanian CIUS-RO: the
// EN 16931 core plus the CIUS-RO mandatory-term rules. It accepts either syntax.
func ValidateCIUSRO(xmlData []byte) []Violation {
	inv, err := parseEN16931(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	out := validateEN16931(inv, ProfileEN16931)
	out = append(out, validateCIUSRORules(inv)...)
	return out
}

func validateCIUSRORules(inv *en16931Invoice) []Violation {
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// BR-RO-010: the invoice number (BT-1) shall contain at least one digit.
	if inv.number != "" && !roDigit.MatchString(inv.number) {
		add("BR-RO-010", "the Invoice number (BT-1) shall contain at least one numeric character")
	}

	// BR-RO-020: the type code (BT-3) is restricted — 381 for a credit note,
	// otherwise one of 380, 384, 389, 751.
	if inv.isCreditNote {
		if inv.typeCode != "" && inv.typeCode != "381" {
			add("BR-RO-020", "a Credit note type code (BT-3) shall be 381")
		}
	} else if inv.typeCode != "" && !roInvoiceTypeCodes[inv.typeCode] {
		add("BR-RO-020", "the Invoice type code (BT-3) shall be one of 380, 384, 389, 751")
	}

	// BR-RO-030: when the invoice currency (BT-5) is not RON, the VAT accounting
	// currency (BT-6) shall be RON.
	if inv.currency != "" && inv.currency != "RON" && inv.taxCurrency != "RON" {
		add("BR-RO-030", "when the Invoice currency (BT-5) is not RON, the VAT accounting currency (BT-6) shall be RON")
	}

	// BR-RO-081/091/082/092: the Seller and Buyer postal addresses shall carry an
	// address line 1 and a city.
	if inv.sellerStreet == "" {
		add("BR-RO-081", "the Seller address line 1 (BT-35) shall be provided")
	}
	if inv.sellerCity == "" {
		add("BR-RO-091", "the Seller city (BT-37) shall be provided")
	}
	if inv.buyerStreet == "" {
		add("BR-RO-082", "the Buyer address line 1 (BT-50) shall be provided")
	}
	if inv.buyerCity == "" {
		add("BR-RO-092", "the Buyer city (BT-52) shall be provided")
	}

	// BR-RO-140/150: a Seller tax representative address, when present, shall
	// carry an address line 1 and a city.
	if inv.taxRepAddressPresent {
		if inv.taxRepStreet == "" {
			add("BR-RO-140", "the Seller tax representative address line 1 (BT-64) shall be provided")
		}
		if inv.taxRepCity == "" {
			add("BR-RO-150", "the Seller tax representative city (BT-66) shall be provided")
		}
	}

	// BR-RO-180/201: a Deliver-to address, when present, shall carry an address
	// line 1 and a city.
	if inv.deliverToPresent {
		if inv.deliverToStreet == "" {
			add("BR-RO-180", "the Deliver to address line 1 (BT-75) shall be provided")
		}
		if inv.deliverToCity == "" {
			add("BR-RO-201", "the Deliver to city (BT-77) shall be provided")
		}
	}

	return out
}
