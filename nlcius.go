package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Dutch NLCIUS (SimplerInvoicing / SI-UBL) Core Invoice
// Usage Specification on top of the EN 16931 core. NLCIUS is the one CIUS in this
// package whose authority publishes **both** syntax bindings —
// si-ubl-2.0/si-ubl-2.0-nlcius.sch and nlcius-cii/NLCIUS-CII-validation.sch — and
// the two are not the same rule set.
//
// The rules below are transcribed from those two files rather than from prose,
// which is what they used to be. Reading the artefacts changed four things:
//
//   - The gate is not one condition, it is two, and the file names them $si and $s.
//     $si is "this document declares the NLCIUS customization identifier"; $s is
//     "$si *and* the supplier is in the Netherlands". This package gated every
//     BR-NL rule on the supplier being Dutch and nothing else, which is wrong in
//     both directions: BR-NL-13 is bound to $si, so it applies to a foreign
//     supplier's NLCIUS invoice and was not evaluated for one; and no rule applied
//     to a document that is not an NLCIUS invoice at all, which this package
//     checked nowhere, so a Peppol BIS invoice from a Dutch seller collected
//     findings a reference NLCIUS validator does not report. PR 21 found the same
//     shape in OpenPEPPOL's Dutch rules, where five different gates were being read
//     as one.
//   - BR-NL-10 accepts a KVK (scheme 0106) *or an OIN* (scheme 0190) for the
//     customer, exactly as BR-NL-1 does for the supplier. This package accepted only
//     the KVK, so a Dutch public body identified by its OIN — the identifier the
//     Dutch government issues for precisely this purpose — was reported as
//     non-conformant.
//   - BR-NL-11 carries no exemption for credit notes. This package excused type
//     code 381 from needing a means of payment, which the artefact does not.
//   - BR-NL-11's two bindings do not test the same thing. The UBL binding asserts
//     "xs:decimal(cbc:PayableAmount) <= 0.0 or (//cac:PaymentMeans)"; the CII one
//     asserts "xs:decimal(ram:DuePayableAmount) >= 0.0 or (…PaymentMeans/TypeCode)",
//     whose first disjunct is true for every non-negative amount. So in CII the rule
//     only fires on a *negative* amount with no payment means, and this package
//     fired it on a positive one — a finding no NLCIUS-CII validator produces.
//
// BR-NL-8 is published in the UBL binding only (it asserts that the type code
// agrees with the UBL document element, a question CII does not have), and
// BR-NL-22/23 in the CII binding only; this package evaluates neither of the
// latter. cius_artefacts_test.go pins the difference between the two bindings, so a
// rule cannot quietly start firing in a syntax that does not publish it.
//
// Severity needed no correction: the twelve fatal identifiers this package
// evaluates are flagged fatal in both bindings, and the 22 advisory ones it does not
// are flagged warning, which is what Coverage(SourceNLCIUS) already said.
//
// Not evaluated: the advisory "not recommended" rules. See Coverage(SourceNLCIUS).

// nlciusPaymentMeans is the payment means code set BR-NL-12 permits: 30 credit
// transfer, 48 bank card, 49 direct debit, 57 standing agreement, 58 SEPA credit
// transfer, 59 SEPA direct debit.
var nlciusPaymentMeans = map[string]bool{"30": true, "48": true, "49": true, "57": true, "58": true, "59": true}

// nlciusTypeCodes is the invoice type code set BR-NL-7 permits.
var nlciusTypeCodes = map[string]bool{"380": true, "381": true, "384": true, "389": true}

// nlciusCustomization is the substring both bindings test the specification
// identifier for. The UBL binding writes
// contains($customizationID, '#compliant#urn:fdc:nen.nl:nlcius:v1.0') and the CII
// one compares for equality against that identifier and against its
// "#conformant#urn:fdc:nen.nl:gaccount:v1.0" extension; the containment test covers
// both, and is the one the authority itself uses in the syntax where the extension
// is expressed as a suffix.
const nlciusCustomization = "#compliant#urn:fdc:nen.nl:nlcius:v1.0"

// nlciusApplies is $si: this document declares itself an NLCIUS invoice.
//
// It is a real condition and not a formality. Both bindings put it on every rule,
// including the ones that are not otherwise about the Netherlands, so a document
// that does not declare the customization identifier collects no BR-NL finding from
// a reference validator — it collects SI-V20-INV-R000 instead, which is the
// wrapper's way of saying it was handed the wrong document. ValidateNLCIUS reports
// nothing rather than that, because a caller who asked for NLCIUS validation has
// said what they think the document is and the core rules still ran.
func nlciusApplies(inv *en16931Invoice) bool {
	return strings.Contains(inv.specID, nlciusCustomization)
}

// ValidateNLCIUS validates an invoice XML against the Dutch NLCIUS (SimplerInvoicing)
// CIUS: the EN 16931 core plus the NLCIUS-specific rules. It accepts either syntax,
// and it is the one CIUS in this package of which that is true because its
// authority publishes both bindings rather than because this package assumed so.
//
// The BR-NL rules apply to a document that declares the NLCIUS customization
// identifier, and all but BR-NL-13 additionally require the supplier to be in the
// Netherlands. Those are the artefact's own two conditions, $si and $s.
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
// The Report names the rule families neither rule set evaluates. NLCIUS is the
// one CIUS here whose fatal rules are implemented in full; its entry in the
// coverage table is the advisory half alone. The EN 16931 core it runs on is
// another matter — see Coverage(SourceEN16931).
func ValidateNLCIUS(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceNLCIUS}, validateNLCIUS)
}

func validateNLCIUS(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931)
	return append(out, validateNLCIUSRules(p.inv)...)
}

// validateNLCIUSRules applies the fatal NLCIUS rules, in the binding that publishes
// each and behind the gate that publishes it.
func validateNLCIUSRules(inv *en16931Invoice) []Violation {
	// $si. Everything below is inside it.
	if !nlciusApplies(inv) {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceNLCIUS)

	// BR-NL-13 is gated on $si alone, in both bindings: an order line reference
	// (BT-132) requires a document-level order reference (BT-13) whatever country
	// the supplier is in. It sits before the $s guard for that reason.
	if inv.hasOrderLineRef && inv.orderRef == "" {
		add("BR-NL-13", "an order line reference (BT-132) requires a document-level Purchase order reference (BT-13)")
	}

	// BR-NL-7 is gated on $si in the CII binding — context
	// /*/rsm:ExchangedDocument/ram:TypeCode[$si] — and on $s in the UBL one, where
	// the context is cbc:InvoiceTypeCode[$s]|cbc:CreditNoteTypeCode[$s]. The same
	// identifier, two gates.
	if inv.syntax == "CII" && !nlciusTypeCodes[inv.typeCode] {
		add("BR-NL-7", fmt.Sprintf("the invoice type code (BT-3=%q) must be one of 380, 381, 384, 389", inv.typeCode))
	}

	// $s: everything below additionally requires the supplier to be in the
	// Netherlands.
	if inv.sellerCountry != "NL" {
		return out
	}

	// BR-NL-1: the supplier must identify its legal entity with a KVK (scheme
	// 0106) or OIN (scheme 0190) number.
	if !((inv.sellerLegalScheme == "0106" || inv.sellerLegalScheme == "0190") && inv.sellerLegalReg != "") {
		add("BR-NL-1", "the Seller legal registration identifier (BT-30) must be a KVK (scheme 0106) or OIN (scheme 0190) number")
	}

	// BR-NL-2: the invoice must carry a buyer reference (BT-10) or a purchase
	// order reference (BT-13).
	if inv.buyerReference == "" && inv.orderRef == "" {
		add("BR-NL-2", "the invoice must contain a Buyer reference (BT-10) or a Purchase order reference (BT-13)")
	}

	// BR-NL-3: the Seller postal address must contain a street (BT-35), city
	// (BT-37) and post code (BT-38).
	if inv.sellerStreet == "" || inv.sellerCity == "" || inv.sellerPostCode == "" {
		add("BR-NL-3", "the Seller postal address must contain a street (BT-35), city (BT-37) and post code (BT-38)")
	}

	// BR-NL-4: a Dutch Buyer's postal address must contain a street, city and post code.
	if inv.buyerCountry == "NL" && (inv.buyerStreet == "" || inv.buyerCity == "" || inv.buyerPostCode == "") {
		add("BR-NL-4", "a Dutch Buyer postal address must contain a street (BT-50), city (BT-52) and post code (BT-53)")
	}

	// BR-NL-5: a Dutch tax representative's postal address must contain a street,
	// city and post code.
	if inv.taxRepCountry == "NL" && (inv.taxRepStreet == "" || inv.taxRepCity == "" || inv.taxRepPostCode == "") {
		add("BR-NL-5", "a Dutch tax representative postal address must contain a street, city and post code")
	}

	// BR-NL-7, UBL binding: gated on $s, unlike the CII one above.
	if inv.syntax != "CII" && !nlciusTypeCodes[inv.typeCode] {
		add("BR-NL-7", fmt.Sprintf("the invoice type code (BT-3=%q) must be one of 380, 381, 384, 389", inv.typeCode))
	}

	// BR-NL-8: the type code must match the UBL document element — 381 (credit
	// note) belongs to a CreditNote, every other permitted code to an Invoice.
	// Published in the UBL binding only.
	if inv.syntax == "UBL" {
		if inv.isCreditNote && inv.typeCode != "381" {
			add("BR-NL-8", fmt.Sprintf("a CreditNote document must use type code 381, not %q", inv.typeCode))
		} else if !inv.isCreditNote && inv.typeCode == "381" {
			add("BR-NL-8", "type code 381 (credit note) must not be used in an Invoice document")
		}
	}

	// BR-NL-9: a corrective invoice (type 384) must reference the preceding
	// invoice it corrects (BT-25).
	if inv.typeCode == "384" && !(inv.hasBillingRef && !inv.billingRefNoID) {
		add("BR-NL-9", "a corrective invoice (type 384) must contain a Preceding Invoice reference (BT-25)")
	}

	// BR-NL-10: a Dutch Buyer must identify its legal entity with a KVK (0106) or
	// an OIN (0190). The OIN half was missing, and an OIN is what every Dutch
	// public body uses.
	if inv.buyerCountry == "NL" && !((inv.buyerLegalScheme == "0106" || inv.buyerLegalScheme == "0190") && inv.buyerLegalReg) {
		add("BR-NL-10", "a Dutch Buyer legal registration identifier (BT-47) must be a KVK (scheme 0106) or OIN (scheme 0190) number")
	}

	// BR-NL-11: the invoice must state a means of payment. The two bindings differ,
	// and the difference is not a nuance:
	//
	//   UBL  xs:decimal(cbc:PayableAmount) <= 0.0 or (//cac:PaymentMeans)
	//   CII  xs:decimal(ram:DuePayableAmount) >= 0.0 or (…PaymentMeans/ram:TypeCode)
	//
	// so in UBL the rule applies to a positive amount due and in CII to a negative
	// one. That reads like an upstream slip, and it is not this package's to
	// correct: reporting a rule in a syntax where the authority's own validator
	// cannot report it is the defect C32 records eight times over. There is no
	// exemption for credit notes in either binding, and this package used to grant
	// one.
	due, dueKnown := parseAmount(inv.totals.duePayable)
	if inv.syntax == "CII" {
		if dueKnown && due < 0 && len(inv.paymentMeans) == 0 {
			add("BR-NL-11", "the invoice must provide a means of payment (BG-16)")
		}
	} else if !(dueKnown && due <= 0) && len(inv.paymentMeans) == 0 {
		add("BR-NL-11", "the invoice must provide a means of payment (BG-16)")
	}

	// BR-NL-12: each payment means code must be one NLCIUS permits.
	for _, code := range inv.paymentMeans {
		if !nlciusPaymentMeans[code] {
			add("BR-NL-12", fmt.Sprintf("the payment means code (BT-81=%q) must be one of 30, 48, 49, 57, 58, 59", code))
			break
		}
	}

	return out
}
