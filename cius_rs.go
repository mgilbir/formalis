package formalis

import (
	"context"
	"regexp"
)

// This file validates the Serbian CIUS (SRBDT, urn:mfin.gov.rs:srbdt, the
// Ministry of Finance e-invoice profile) on top of the EN 16931 core. SRBDT makes
// the parties' tax and registration identifiers, electronic addresses and cities
// mandatory and constrains their formats (the Serbian PIB is "RS" + 9 or 13
// digits; electronic addresses use scheme 9948), restricts the invoice type code,
// and forbids the tax point date (BT-7) in favour of the tax point date code.
//
// The Serbian VAT-category rules (RSK-X-*) and the finer identifier/endpoint
// cross-checks (BT-8 code values, endpoint-contains-PIB, buyer registration
// format) depend on terms or relationships the syntax-neutral model does not
// carry, and are not emitted; the mandatory-term and format rules below are the
// validity-affecting core.
//
// Not vendored: the SRBDT Schematron and sample instances (phax/phive-rules) are
// used only as the oracle.

// rsTypeCodes is the invoice type code set SRBDT permits (RSR-03).
var rsTypeCodes = map[string]bool{"380": true, "381": true, "383": true, "386": true}

// rsPIB matches a Serbian tax identifier (PIB): "RS" followed by 9 or 13 digits
// (RSR-11/21).
var rsPIB = regexp.MustCompile(`^RS(\d{9}|\d{13})$`)

// ValidateSRBDT validates an invoice XML against the Serbian CIUS (SRBDT): the
// EN 16931 core plus the SRBDT mandatory-term and format rules.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty Report, so it cannot be mistaken for a valid invoice.
//
// The Report names the rule families neither rule set evaluates — the union of
// Coverage(SourceEN16931) and Coverage(SourceSRBDT).
func ValidateSRBDT(ctx context.Context, xmlData []byte) Report {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceSRBDT}, validateSRBDT)
}

func validateSRBDT(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p.inv, ProfileEN16931)
	return append(out, validateSRBDTRules(p.inv)...)
}

func validateSRBDTRules(inv *en16931Invoice) []Violation {
	var out []Violation
	add := adder(&out, SourceSRBDT)

	// RSR-03: the invoice type code (BT-3) shall be 380, 381, 383 or 386.
	if inv.typeCode != "" && !rsTypeCodes[inv.typeCode] {
		add("RSR-03", "the Invoice type code (BT-3) shall be one of 380, 381, 383, 386")
	}

	// RSR-04: the invoice shall not carry a tax point date (BT-7); the tax point
	// date code (BT-8) is used instead.
	if inv.vatPointDate != "" {
		add("RSR-04", "the invoice shall not contain a tax point date (BT-7); use the tax point date code (BT-8)")
	}

	// Seller identifiers. RSR-09/10: the Seller VAT (BT-31) and tax registration
	// (BT-32) identifiers are mandatory. RSR-11: the Seller PIB is "RS"+9/13 digits.
	if !inv.sellerVATID {
		add("RSR-09", "the invoice shall contain the Seller VAT identifier / PIB (BT-31)")
	} else if !rsPIB.MatchString(inv.sellerVATIDValue) {
		add("RSR-11", "the Seller PIB (BT-31) shall be 'RS' followed by 9 or 13 digits")
	}
	if !inv.sellerTaxReg {
		add("RSR-10", "the invoice shall contain the Seller tax registration identifier (BT-32)")
	}
	// RSR-13/14: the Seller electronic address (BT-34) is mandatory with scheme 9948.
	if !inv.sellerEndpointPresent {
		add("RSR-13", "the invoice shall contain the Seller electronic address (BT-34)")
	} else if inv.sellerEndpointScheme != "9948" {
		add("RSR-14", "the Seller electronic address (BT-34) shall use scheme identifier '9948'")
	}
	// RSR-16: the Seller city (BT-37) is mandatory.
	if inv.sellerCity == "" {
		add("RSR-16", "the invoice shall contain the Seller city (BT-37)")
	}

	// Buyer identifiers. RSR-17: the Buyer registration identifier (BT-47) is
	// mandatory. RSR-20/21: the Buyer VAT (BT-48) is mandatory and "RS"+9/13 digits.
	if !inv.buyerLegalReg {
		add("RSR-17", "the invoice shall contain the Buyer registration identifier (BT-47)")
	}
	if !inv.buyerVATID {
		add("RSR-20", "the invoice shall contain the Buyer VAT identifier / PIB (BT-48)")
	} else if !rsPIB.MatchString(inv.buyerVATIDValue) {
		add("RSR-21", "the Buyer PIB (BT-48) shall be 'RS' followed by 9 or 13 digits")
	}
	// RSR-22/23: the Buyer electronic address (BT-49) is mandatory with scheme 9948.
	if !inv.buyerEndpointPresent {
		add("RSR-22", "the invoice shall contain the Buyer electronic address (BT-49)")
	} else if inv.buyerEndpointScheme != "9948" {
		add("RSR-23", "the Buyer electronic address (BT-49) shall use scheme identifier '9948'")
	}
	// RSR-25: the Buyer city (BT-52) is mandatory.
	if inv.buyerCity == "" {
		add("RSR-25", "the invoice shall contain the Buyer city (BT-52)")
	}

	return out
}
