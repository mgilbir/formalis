package formalis

import "context"

// This file validates the Portuguese CIUS-PT (urn:feap.gov.pt:CIUS-PT) on top of
// the EN 16931 core. CIUS-PT is the AT/eSPap public-sector profile; it makes
// several EN 16931-optional terms mandatory — the parties' VAT identifiers, the
// Seller and Deliver-to postal addresses, the document totals and total VAT
// amount, and a delivery. The same syntax-neutral model feeds it.
//
// The CIUS also defines many conditional structural-completeness rules (BR-CIUS-
// PT-24 … 63: "if this optional UBL group is present, its mandatory child must be
// too") and Portuguese VAT-category rate rules (BR-CIUS-PT-13/15/17/18). Those
// depend on UBL element presence the syntax-neutral model does not carry, and are
// not emitted here; the mandatory-term rules below are the validity-affecting core.
//
// Not vendored: the CIUS-PT Schematron and sample instances (phax/phive-rules)
// are used only as the oracle.

// ValidateCIUSPT validates an invoice XML against the Portuguese CIUS-PT: the
// EN 16931 core plus the CIUS-PT mandatory-term rules. It accepts either syntax.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateCIUSPT(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	p, err := parseEN16931(r, xmlData)
	if err != nil {
		return r.finish(syntaxViolation(err))
	}
	return r.finish(validateCIUSPT(r, p))
}

func validateCIUSPT(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p.inv, ProfileEN16931)
	return append(out, validateCIUSPTRules(p.inv)...)
}

func validateCIUSPTRules(inv *en16931Invoice) []Violation {
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// BR-CIUS-PT-01/03: the Seller (BT-31) and Buyer (BT-48) VAT identifiers are
	// mandatory.
	if !inv.sellerVATID {
		add("BR-CIUS-PT-01", "the Invoice shall contain the Seller VAT identifier (BT-31)")
	}
	if !inv.buyerVATID {
		add("BR-CIUS-PT-03", "the Invoice shall contain the Buyer VAT identifier (BT-48)")
	}

	// BR-CIUS-PT-05/06/07: the Seller postal address is mandatory in full.
	if inv.sellerStreet == "" {
		add("BR-CIUS-PT-05", "the Seller postal address (BG-5) shall contain a Seller address line 1 (BT-35)")
	}
	if inv.sellerCity == "" {
		add("BR-CIUS-PT-06", "the Seller postal address (BG-5) shall contain a Seller city (BT-37)")
	}
	if inv.sellerPostCode == "" {
		add("BR-CIUS-PT-07", "the Seller postal address (BG-5) shall contain a Seller post code (BT-38)")
	}

	// BR-CIUS-PT-10/11: the Document totals (BG-22) and the Total VAT amount
	// (BT-110) are mandatory.
	if !inv.hasTotals {
		add("BR-CIUS-PT-10", "the Invoice shall contain the Document totals (BG-22)")
	}
	if inv.totals.taxTotal == "" {
		add("BR-CIUS-PT-11", "the Invoice shall contain the Total VAT amount (BT-110)")
	}

	// BR-CIUS-PT-66: at least one Deliver-to address (BG-15) is mandatory.
	if !inv.deliverToPresent {
		add("BR-CIUS-PT-66", "the Invoice shall contain at least one Deliver to address (BG-15)")
	}
	// BR-CIUS-PT-64: a delivery must be evidenced — an actual delivery date
	// (BT-72) or a Deliver-to address/location.
	if inv.deliveryDate == "" && !inv.deliverToPresent {
		add("BR-CIUS-PT-64", "the Actual delivery date (BT-72) or a Deliver to address/location shall be present")
	}

	// BR-CIUS-PT-21/22/23: a Deliver-to address, when present, is mandatory in full.
	if inv.deliverToPresent {
		if inv.deliverToStreet == "" {
			add("BR-CIUS-PT-21", "each Deliver to address (BG-15) shall contain an address line 1 (BT-75)")
		}
		if inv.deliverToCity == "" {
			add("BR-CIUS-PT-22", "each Deliver to address (BG-15) shall contain a city (BT-77)")
		}
		if inv.deliverToPostCode == "" {
			add("BR-CIUS-PT-23", "each Deliver to address (BG-15) shall contain a post code (BT-78)")
		}
	}

	return out
}
