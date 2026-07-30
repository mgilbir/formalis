package formalis

import (
	"context"
	"regexp"
	"strings"
)

// This file validates the Romanian CIUS-RO (RO_CIUS / RO e-Factura, the ANAF
// national profile) on top of the EN 16931 core.
//
// The rules below are transcribed from cius-ro/RO16931-rules.sch rather than from
// the specification's prose, which is what they used to be. Reading the artefact
// changed five things:
//
//   - ANAF publishes CIUS-RO for **UBL only**. EN16931-CIUS_RO-UBL-validation.sch
//     binds the abstract model to UBL and there is no CII binding; every context in
//     RO16931-rules.sch is a UBL path. This package evaluated all twenty rules on
//     the shared syntax-neutral model, so every Factur-X/ZUGFeRD invoice was liable
//     to be accused of a Romanian rule that does not exist for its syntax — C32's
//     eight-rule defect, again.
//   - There is no BR-RO-020. ANAF publishes BR-RO-020_1, bound to
//     cbc:InvoiceTypeCode, and BR-RO-020_2, bound to cbc:CreditNoteTypeCode. This
//     package reported "BR-RO-020", an identifier no artefact carries — the same
//     class of defect as C34's two Peppol phantoms, in the emission site rather
//     than in the coverage table.
//   - BR-RO-110 and BR-RO-111 are not presence tests. Both are <report> elements
//     that fire when the party is Romanian and its country subdivision is *not* one
//     of the 42 ISO 3166-2:RO codes, which includes but is not limited to its being
//     absent. This package tested only for absence, so a Romanian address carrying
//     an invented subdivision passed.
//   - BR-RO-140/150/160/170 and BR-RO-180/201/202/211/212 are bound to
//     "/ubl:Invoice/… | /ubl:CreditNote/…", and in that file the prefix ubl is the
//     Invoice-2 namespace while a credit note's root is cn:CreditNote. The second
//     alternative therefore matches nothing, and no conforming validator applies
//     these nine rules to a credit note. That is upstream's slip and not this
//     package's to correct: a rule the authority cannot report is a rule this
//     package must not report either (D10's reasoning, applied one rule at a time).
//   - BR-RO-010 and BR-RO-030 fire on an absent term too, because their contexts are
//     the document element and normalize-space() of nothing is the empty string.
//
// All 125 published identifiers are flagged fatal, so the plain adder is right and
// the coverage table's fail-safe fatal turned out to be ANAF's own flag.
// cius_artefacts_test.go checks both directions.
//
// Not evaluated: the per-field length limits (BR-RO-L*), the decimal limits
// (BR-DEC-RO-*), the aggregate rules (BR-RO-A*), the datatype rules (BR-RO-DT*) and
// the rest of the BR-RO set. See Coverage(SourceCIUSRO).

// roInvoiceTypeCodes is the invoice type code set BR-RO-020_1 permits, quoted from
// its test: contains(' 380 384 389 751 ', concat(' ', normalize-space(.), ' ')).
var roInvoiceTypeCodes = map[string]bool{"380": true, "384": true, "389": true, "751": true}

// roCountyCodes is the ISO 3166-2:RO country-subdivision code set CIUS-RO requires
// for Romanian addresses: the 41 counties (județe) plus Bucharest (RO-B). Taken
// verbatim from RO16931-rules.sch's $ISO-3166-RO-CODES.
var roCountyCodes = map[string]bool{
	"RO-AB": true, "RO-AG": true, "RO-AR": true, "RO-B": true, "RO-BC": true, "RO-BH": true,
	"RO-BN": true, "RO-BR": true, "RO-BT": true, "RO-BV": true, "RO-BZ": true, "RO-CJ": true,
	"RO-CL": true, "RO-CS": true, "RO-CT": true, "RO-CV": true, "RO-DB": true, "RO-DJ": true,
	"RO-GJ": true, "RO-GL": true, "RO-GR": true, "RO-HD": true, "RO-HR": true, "RO-IF": true,
	"RO-IL": true, "RO-IS": true, "RO-MH": true, "RO-MM": true, "RO-MS": true, "RO-NT": true,
	"RO-OT": true, "RO-PH": true, "RO-SB": true, "RO-SJ": true, "RO-SM": true, "RO-SV": true,
	"RO-TL": true, "RO-TM": true, "RO-TR": true, "RO-VL": true, "RO-VN": true, "RO-VS": true,
}

// roBucharestSectors is $SECTOR-RO-CODES: the city values a Bucharest (RO-B)
// address must use.
var roBucharestSectors = map[string]bool{
	"SECTOR1": true, "SECTOR2": true, "SECTOR3": true, "SECTOR4": true, "SECTOR5": true, "SECTOR6": true,
}

var roDigit = regexp.MustCompile(`[0-9]`)

// ValidateCIUSRO validates an invoice XML against the Romanian CIUS-RO: the
// EN 16931 core plus the CIUS-RO mandatory-term rules.
//
// The EN 16931 core accepts either syntax. The CIUS-RO rules are evaluated for a
// UBL document only, because that is the only binding ANAF publishes: a CII invoice
// is validated against the core and reported as carrying no CIUS-RO finding, which
// is what a reference CIUS-RO validator says about it too.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice or credit note.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. It is a
// statement about the file rather than about the document, and the Report
// returned with it is the zero Report, so a caller who ignores the error cannot
// read the value as clean. See ErrMalformedXML.
//
// The Report names the rule families neither rule set evaluates — the union of
// Coverage(SourceEN16931) and Coverage(SourceCIUSRO).
func ValidateCIUSRO(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceCIUSRO}, validateCIUSRO)
}

func validateCIUSRO(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931)
	return append(out, validateCIUSRORules(p.inv, p.root)...)
}

func validateCIUSRORules(inv *en16931Invoice, root *ciiNode) []Violation {
	if inv.syntax != "UBL" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceCIUSRO)

	// BR-RO-010, context /ubl:Invoice | /cn:CreditNote:
	//   matches(normalize-space(cbc:ID), '([0-9])')
	// An absent invoice number normalizes to "" and has no digit, so the rule fires
	// for it too; BR-1 in the core reports the absence separately.
	if !roDigit.MatchString(inv.number) {
		add("BR-RO-010", "the Invoice number (BT-1) shall contain at least one numeric character")
	}

	// BR-RO-020_1, context cbc:InvoiceTypeCode, and BR-RO-020_2, context
	// cbc:CreditNoteTypeCode. Two identifiers, one per document element, each
	// requiring a non-empty code from its own set: 380/384/389/751 for an invoice
	// and 381 for a credit note. The leading "." in each test is what makes an
	// empty element fail.
	if inv.isCreditNote {
		if root.child("CreditNoteTypeCode") != nil && inv.typeCode != "381" {
			add("BR-RO-020_2", "a Credit note type code (BT-3) shall be 381")
		}
	} else if root.child("InvoiceTypeCode") != nil && !roInvoiceTypeCodes[inv.typeCode] {
		add("BR-RO-020_1", "the Invoice type code (BT-3) shall be one of 380, 384, 389, 751")
	}

	// BR-RO-030, context /ubl:Invoice | /cn:CreditNote. The published test is a
	// four-way disjunction that reduces to "the VAT accounting currency (BT-6) is
	// RON, or the invoice currency (BT-5) is RON" — so an invoice with no
	// DocumentCurrencyCode at all fails it, which is why there is no non-empty
	// guard on inv.currency here.
	if inv.currency != "RON" && inv.taxCurrency != "RON" {
		add("BR-RO-030", "when the Invoice currency (BT-5) is not RON, the VAT accounting currency (BT-6) shall be RON")
	}

	// BR-RO-081/091/082/092, context /ubl:Invoice | /cn:CreditNote, each asserting
	// the address element exists *and* normalizes to something non-empty:
	//   cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:StreetName[boolean(normalize-space(.))]
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

	// BR-RO-110/111, context /ubl:Invoice | /cn:CreditNote, both <report>:
	//   IdentificationCode = 'RO' and not(normalize-space(CountrySubentity) = $ISO-3166-RO-CODES)
	// A validity test, not a presence test. An absent subdivision normalizes to ""
	// and is not in the list, so absence is one of the cases it covers.
	if inv.sellerCountry == "RO" && !roCountyCodes[inv.sellerSubentity] {
		add("BR-RO-110", "the Seller country subdivision (BT-39) shall be an ISO 3166-2:RO code for a Romanian Seller")
	}
	if inv.buyerCountry == "RO" && !roCountyCodes[inv.buyerSubentity] {
		add("BR-RO-111", "the Buyer country subdivision (BT-54) shall be an ISO 3166-2:RO code for a Romanian Buyer")
	}
	// BR-RO-100/101, same context, both <report>: Bucharest (RO-B) requires the city
	// to be one of $SECTOR-RO-CODES.
	if inv.sellerCountry == "RO" && inv.sellerSubentity == "RO-B" && !roBucharestSectors[inv.sellerCity] {
		add("BR-RO-100", "a Seller in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-37)")
	}
	if inv.buyerCountry == "RO" && inv.buyerSubentity == "RO-B" && !roBucharestSectors[inv.buyerCity] {
		add("BR-RO-101", "a Buyer in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-52)")
	}

	// The nine rules below are bound to "/ubl:Invoice/… | /ubl:CreditNote/…", and
	// ubl is the Invoice-2 namespace in RO16931-rules.sch, so the credit-note
	// alternative matches nothing and ANAF's own validator does not apply them to a
	// credit note. Reporting them there would be a finding no reference validator
	// produces.
	if inv.isCreditNote {
		return out
	}

	// BR-RO-140/150, context
	// /ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress:
	//   cbc:StreetName[boolean(normalize-space(.))] / cbc:CityName[…]
	if inv.taxRepAddressPresent {
		if inv.taxRepStreet == "" {
			add("BR-RO-140", "the Seller tax representative address line 1 (BT-64) shall be provided")
		}
		if inv.taxRepCity == "" {
			add("BR-RO-150", "the Seller tax representative city (BT-66) shall be provided")
		}
		// BR-RO-170/160, same context, both <report>.
		if inv.taxRepCountry == "RO" && !roCountyCodes[inv.taxRepSubentity] {
			add("BR-RO-170", "the Seller tax representative country subdivision (BT-68) shall be an ISO 3166-2:RO code")
		}
		if inv.taxRepCountry == "RO" && inv.taxRepSubentity == "RO-B" && !roBucharestSectors[inv.taxRepCity] {
			add("BR-RO-160", "a tax representative in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city")
		}
	}

	// BR-RO-180/201/211, context
	// /ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address, each a non-empty
	// assertion; BR-RO-212/202 are <report> elements in the same context.
	if inv.deliverToPresent {
		if inv.deliverToStreet == "" {
			add("BR-RO-180", "the Deliver to address line 1 (BT-75) shall be provided")
		}
		if inv.deliverToCity == "" {
			add("BR-RO-201", "the Deliver to city (BT-77) shall be provided")
		}
		if inv.deliverToSubentity == "" {
			add("BR-RO-211", "the Deliver to country subdivision (BT-79) shall be provided")
		}
		if inv.deliverToCountry == "RO" && !roCountyCodes[inv.deliverToSubentity] {
			add("BR-RO-212", "the Deliver to country subdivision (BT-79) shall be an ISO 3166-2:RO code")
		}
		if inv.deliverToCountry == "RO" && inv.deliverToSubentity == "RO-B" && !roBucharestSectors[inv.deliverToCity] {
			add("BR-RO-202", "a delivery in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-77)")
		}
	}

	return out
}

// roSectorSubdivision reports whether s is a Bucharest sector written where an ISO
// 3166-2:RO subdivision belongs — "Sector 5" rather than "RO-B".
//
// It is not part of any rule. It used to be: roValidSubdivision accepted a sector
// as a valid subdivision, which made BR-RO-170 and BR-RO-212 more permissive than
// ANAF publishes them, and the comment justifying it said real Romanian invoices
// write Bucharest that way. They do — seven of ANAF's own eleven sample invoices
// write "Sector 5" in the buyer's BT-54 — but ANAF's Schematron reports BR-RO-111
// for exactly that, so accepting it made this package quieter than the authority
// rather than more accurate. The predicate survives in the test that accounts for
// those seven documents, and nowhere else.
func roSectorSubdivision(s string) bool {
	return roSector.MatchString(strings.TrimSpace(s))
}

var roSector = regexp.MustCompile(`(?i)^sector\s*[1-6]$`)
