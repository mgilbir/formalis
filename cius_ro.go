package formalis

import (
	"context"
	"regexp"
)

// This file validates the Romanian CIUS-RO (RO_CIUS / RO e-Factura, the ANAF
// national profile) on top of the EN 16931 core. CIUS-RO makes several EN 16931-
// optional terms mandatory (the parties' and delivery/tax-representative address
// lines and cities), restricts the invoice type code and the VAT accounting
// currency, and requires a numeric character in the invoice number. The same
// syntax-neutral model feeds it.
//
// It also validates the Romanian country subdivision (BT-39/54/79): a Romanian
// address must carry a subdivision, which for tax representatives and deliveries
// must be one of the 42 ISO 3166-2:RO county codes, and where it is Bucharest
// (RO-B) the city must be a sector code. The county and sector code lists are
// taken from the CIUS-RO Schematron.
//
// The BR-RO-L* rules are per-field maximum-length limits; they are low value and
// not emitted. The BR-DEC-RO decimal rules and the allowance/charge-conditional
// VAT-identifier rules (BR-RO-065/120, which overlap the EN 16931 core) are also
// not emitted.
//
// Not vendored: the CIUS-RO Schematron and sample instances (phax/phive-rules)
// are used only as the oracle.

// roInvoiceTypeCodes is the invoice type code set CIUS-RO permits (BR-RO-020_1).
var roInvoiceTypeCodes = map[string]bool{"380": true, "384": true, "389": true, "751": true}

// roCountyCodes is the ISO 3166-2:RO country-subdivision code set CIUS-RO
// requires for Romanian addresses (BR-RO-170/212): the 41 counties (județe) plus
// Bucharest (RO-B). Taken verbatim from the CIUS-RO Schematron's ISO-3166-RO-CODES.
var roCountyCodes = map[string]bool{
	"RO-AB": true, "RO-AG": true, "RO-AR": true, "RO-B": true, "RO-BC": true, "RO-BH": true,
	"RO-BN": true, "RO-BR": true, "RO-BT": true, "RO-BV": true, "RO-BZ": true, "RO-CJ": true,
	"RO-CL": true, "RO-CS": true, "RO-CT": true, "RO-CV": true, "RO-DB": true, "RO-DJ": true,
	"RO-GJ": true, "RO-GL": true, "RO-GR": true, "RO-HD": true, "RO-HR": true, "RO-IF": true,
	"RO-IL": true, "RO-IS": true, "RO-MH": true, "RO-MM": true, "RO-MS": true, "RO-NT": true,
	"RO-OT": true, "RO-PH": true, "RO-SB": true, "RO-SJ": true, "RO-SM": true, "RO-SV": true,
	"RO-TL": true, "RO-TM": true, "RO-TR": true, "RO-VL": true, "RO-VN": true, "RO-VS": true,
}

// roBucharestSectors is the set of Bucharest sector codes a city must use when
// the subdivision is RO-B (BR-RO-100/101/160/202).
var roBucharestSectors = map[string]bool{
	"SECTOR1": true, "SECTOR2": true, "SECTOR3": true, "SECTOR4": true, "SECTOR5": true, "SECTOR6": true,
}

var roDigit = regexp.MustCompile(`[0-9]`)

// roSector matches a Bucharest sector written in the country-subdivision field.
// Real Romanian invoices encode Bucharest as its sector here (e.g. "Sector 1")
// rather than the Schematron's abstract RO-B/SECTOR1 pair, so the subdivision
// validity check accepts either an ISO 3166-2:RO county code or a sector.
var roSector = regexp.MustCompile(`(?i)^sector\s*[1-6]$`)

// roValidSubdivision reports whether a country-subdivision value is an ISO
// 3166-2:RO county code or a Bucharest sector.
func roValidSubdivision(s string) bool {
	return roCountyCodes[s] || roSector.MatchString(s)
}

// ValidateCIUSRO validates an invoice XML against the Romanian CIUS-RO: the
// EN 16931 core plus the CIUS-RO mandatory-term rules. It accepts either syntax.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateCIUSRO(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	p, err := parseEN16931(r, xmlData)
	if err != nil {
		return r.finish(syntaxViolation(err))
	}
	return r.finish(validateCIUSRO(r, p))
}

func validateCIUSRO(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p.inv, ProfileEN16931)
	return append(out, validateCIUSRORules(p.inv)...)
}

func validateCIUSRORules(inv *en16931Invoice) []Violation {
	var out []Violation
	add := adder(&out, SourceCIUSRO)

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

	// Country-subdivision (county) rules. For a Romanian address the country
	// subdivision (BT-39/54/79) is mandatory; a tax representative's and a
	// delivery subdivision must additionally be a valid ISO 3166-2:RO code; and
	// where the subdivision is Bucharest (RO-B) the city must be a sector code.
	// BR-RO-110/111: Seller (BT-39) and Buyer (BT-54) subdivision present when in RO.
	if inv.sellerCountry == "RO" && inv.sellerSubentity == "" {
		add("BR-RO-110", "the Seller country subdivision (BT-39) shall be provided for a Romanian Seller")
	}
	if inv.buyerCountry == "RO" && inv.buyerSubentity == "" {
		add("BR-RO-111", "the Buyer country subdivision (BT-54) shall be provided for a Romanian Buyer")
	}
	// BR-RO-100/101: Bucharest (RO-B) requires the city to be a sector code.
	if inv.sellerCountry == "RO" && inv.sellerSubentity == "RO-B" && !roBucharestSectors[inv.sellerCity] {
		add("BR-RO-100", "a Seller in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-37)")
	}
	if inv.buyerCountry == "RO" && inv.buyerSubentity == "RO-B" && !roBucharestSectors[inv.buyerCity] {
		add("BR-RO-101", "a Buyer in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-52)")
	}
	// BR-RO-170/160: tax representative subdivision valid, and RO-B sector city.
	if inv.taxRepAddressPresent && inv.taxRepCountry == "RO" {
		if !roValidSubdivision(inv.taxRepSubentity) {
			add("BR-RO-170", "the Seller tax representative country subdivision shall be a valid ISO 3166-2:RO code")
		}
		if inv.taxRepSubentity == "RO-B" && !roBucharestSectors[inv.taxRepCity] {
			add("BR-RO-160", "a tax representative in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city")
		}
	}
	// BR-RO-211/212/202: delivery subdivision present, valid, and RO-B sector city.
	if inv.deliverToPresent {
		if inv.deliverToSubentity == "" {
			add("BR-RO-211", "the Deliver to country subdivision (BT-79) shall be provided")
		}
		if inv.deliverToCountry == "RO" && !roValidSubdivision(inv.deliverToSubentity) {
			add("BR-RO-212", "the Deliver to country subdivision (BT-79) shall be a valid ISO 3166-2:RO code")
		}
		if inv.deliverToCountry == "RO" && inv.deliverToSubentity == "RO-B" && !roBucharestSectors[inv.deliverToCity] {
			add("BR-RO-202", "a delivery in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-77)")
		}
	}

	return out
}
