package formalis

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// This file validates the Belgian UBL.BE CIUS on top of the EN 16931 core. UBL.BE's
// own rules (ubl-BE-*) are UBL-structural rather than semantic — they check
// UBL-specific elements and Belgian code lists — so they are evaluated against the
// parsed tree rather than the syntax-neutral model.
//
// The rules below are transcribed from the ubl-model-BE pattern of
// GLOBALUBL.BE.sch rather than from prose, which is what they used to be. Reading
// the artefact changed five things:
//
//   - UBL.BE is a UBL profile and every ubl-BE context is a UBL path, so the rules
//     do not apply to a CII invoice. Nothing said so: validateUBLBERules ran on the
//     tree of whatever was parsed, and on a CII document it found no
//     cbc:DocumentDescription and no cbc:ID reading "UBL.BE", so **ubl-BE-02 and
//     ubl-BE-03 fired on every Factur-X and ZUGFeRD invoice put through
//     ValidateUBLBE**. That is the largest single false positive this file had.
//   - ubl-BE-02 and ubl-BE-03 are bound to the context //cac:AdditionalDocumentReference,
//     so they apply only to a document that has one. This package applied them
//     unconditionally, so an invoice with no additional document reference at all —
//     which is most invoices — was reported for both.
//   - ubl-BE-05, ubl-BE-10 and ubl-BE-11 are scoped: //cac:Delivery/cac:DeliveryTerms,
//     //cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory and the exemption reason code
//     inside that same category. This package searched the whole tree, so a
//     line-level cac:ClassifiedTaxCategory/cbc:TaxExemptionReasonCode was checked
//     against a list the rule does not apply to it. It also skipped an empty value
//     where the artefact's "some $code in $BVERC satisfies …" fails for one.
//   - ubl-BE-15 is count(cbc:Name) = 1, not "has a non-empty name": an empty
//     <cbc:Name/> satisfies it and two names fail it, and this package had both
//     backwards.
//   - ubl-BE-13 cannot fail. See ublBE13Reason below.
//
// All 15 published identifiers are flagged fatal, so the plain adder is right and
// the coverage table's fail-safe fatal turned out to be the authority's own flag.
// cius_artefacts_test.go checks both directions.
//
// The bilingual free-text code lists (BELMText for ubl-BE-06, BVERCText for
// ubl-BE-12) are exact-match lists of sentences and are not enforced; ubl-BE-01
// and ubl-BE-04 are not evaluated either. See Coverage(SourceUBLBE).

// beDeliveryTerms is the BELM delivery-terms code list (ubl-BE-05), quoted from
// $BELM.
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

// beExemptionReasonCodes is the BVERC Belgian VAT-exemption reason code list
// (ubl-BE-11).
var beExemptionReasonCodes = map[string]bool{
	"BETE-45": true, "BETE-EX": true, "BETE-FD": true, "BETE-SC": true, "BETE-00/44": true,
	"BETE-03/SE": true, "BETE-MA": true, "BETE-46/GO": true, "BETE-47/TO": true, "BETE-47/AS": true,
	"BETE-47/DI": true, "BETE-47/SE": true, "BETE-44": true, "BETE-46/TR": true, "BETE-47/EX": true,
	"BETE-47/EI": true, "BETE-47/EE": true, "BETE-NS": true,
}

// ublBE13Reason is the evidence for the one unevaluable family outside CEN's, kept
// beside the rule it is about and quoted verbatim into Coverage(SourceUBLBE).
//
// GLOBALUBL.BE.sch declares, in the ubl-model-BE pattern,
//
//	<let name="TaxAmount" value="if (cbc:TaxAmount) then xs:decimal(cbc:TaxAmount) else -1"/>
//
// and binds ubl-BE-13, in the context //cac:InvoiceLine/cac:TaxTotal |
// //cac:CreditNoteLine/cac:TaxTotal, to the test abs($TaxAmount) >= 0. The absolute
// value of a decimal is never negative and the fallback is the decimal -1, so the
// assertion is true for every document: a line tax total with no cbc:TaxAmount gets
// -1, whose absolute value is 1, and one with any amount at all gets that amount's
// magnitude. No conforming validator reports ubl-BE-13, which is D10's definition
// of unevaluable and the same shape as CEN binding BR-CO-05..08 to true().
//
// This package did report it — whenever a line's tax amount was absent or not a
// number — so the correction is a false-positive fix and not a coverage reduction.
// The only input on which a real processor behaves differently is a non-numeric
// amount, where xs:decimal() raises a dynamic error rather than failing the
// assertion, and a dynamic error is not a finding either.
const ublBE13Reason = "GLOBALUBL.BE.sch declares <let name=\"TaxAmount\" value=\"if (cbc:TaxAmount) then " +
	"xs:decimal(cbc:TaxAmount) else -1\"/> in the ubl-model-BE pattern and binds ubl-BE-13 to the test " +
	"abs($TaxAmount) >= 0. The absolute value of a decimal is never negative and the fallback is -1, so the " +
	"assertion holds for every document, including a line tax total with no amount at all: no conforming " +
	"validator reports this rule. This package reported it whenever a line's cbc:TaxAmount was absent or not " +
	"numeric, so recording it here is a false-positive fix. TestUBLBE13IsBoundToATautology reads the binding " +
	"back out of the file"

// ValidateUBLBE validates an invoice XML against the Belgian UBL.BE CIUS: the
// EN 16931 core plus the UBL.BE-specific rules.
//
// The EN 16931 core accepts either syntax. The ubl-BE rules are evaluated for a UBL
// document only, because UBL.BE is a UBL profile and every rule in it is bound to a
// UBL path: a CII invoice is validated against the core and reported as carrying no
// UBL.BE finding, which is what a reference UBL.BE validator says about it too.
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
// Coverage(SourceEN16931) and Coverage(SourceUBLBE).
func ValidateUBLBE(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceUBLBE}, validateUBLBE)
}

func validateUBLBE(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931)
	// The ubl-BE rules read the tree the parse already built. This used to parse
	// the bytes a second time to obtain it, which produced a byte-identical tree
	// at the cost of a second full pass over the shared element budget.
	if p.inv.syntax != "UBL" {
		return out
	}
	return append(out, validateUBLBERules(p.root)...)
}

func validateUBLBERules(root *ciiNode) []Violation {
	var out []Violation
	add := adder(&out, SourceUBLBE)

	// ubl-BE-02/03, context //cac:AdditionalDocumentReference. Both are
	// document-wide counts evaluated in that context, so they say nothing at all
	// about a document that carries no additional document reference — which is
	// why the guard is on the context and not on the count.
	if len(root.findAll("AdditionalDocumentReference")) > 0 {
		docType := 0
		for _, d := range root.findAll("DocumentDescription") {
			if t := d.text; t == "CommercialInvoice" || t == "CreditNote" {
				docType++
			}
		}
		if docType != 1 {
			add("ubl-BE-02", "exactly one document type (DocumentDescription 'CommercialInvoice' or 'CreditNote') shall be specified")
		}
		marker := 0
		for _, x := range root.findAll("ID") {
			if x.text == "UBL.BE" {
				marker++
			}
		}
		if marker != 1 {
			add("ubl-BE-03", "exactly one cbc:ID with the value 'UBL.BE' shall be present")
		}
	}

	// ubl-BE-05, context //cac:Delivery/cac:DeliveryTerms:
	//   some $code in $BELM satisfies normalize-space(data(cbc:ID)) = $code
	// An absent cbc:ID normalizes to "", which is in no list, so it fails too.
	for _, d := range root.findAll("Delivery") {
		for _, dt := range d.all("DeliveryTerms") {
			if id := strings.TrimSpace(dt.str("ID")); !beDeliveryTerms[id] {
				add("ubl-BE-05", "the Delivery terms ID ("+id+") shall be in the BELM list")
			}
		}
	}

	// ubl-BE-10/11, context //cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory. Both
	// are scoped to the VAT breakdown's category — a line's
	// cac:ClassifiedTaxCategory is a different element and is not in this context.
	for _, tt := range root.findAll("TaxTotal") {
		for _, ts := range tt.all("TaxSubtotal") {
			for _, tc := range ts.all("TaxCategory") {
				if nm := strings.TrimSpace(tc.str("Name")); !beTaxCategoryNames[nm] {
					add("ubl-BE-10", "the VAT category name ("+nm+") shall be in the BTCC list")
				}
				// ubl-BE-11 is guarded on the element's presence, not on its value:
				//   if (cbc:TaxExemptionReasonCode) then some $code in $BVERC … else 1
				if c := tc.child("TaxExemptionReasonCode"); c != nil {
					if v := strings.TrimSpace(c.text); !beExemptionReasonCodes[v] {
						add("ubl-BE-11", "the VAT exemption reason code ("+v+") shall be in the BVERC list")
					}
				}
			}
		}
	}

	// ubl-BE-07/08/09, context //cac:PaymentTerms. All three are guarded on
	// cbc:SettlementDiscountPercent being present — $SettlementDiscountPercent
	// falls back to -1, which ubl-BE-07 permits, and the other two test it
	// directly.
	for _, pt := range root.findAll("PaymentTerms") {
		sdp := strings.TrimSpace(pt.str("SettlementDiscountPercent"))
		if sdp == "" {
			continue
		}
		if v, err := strconv.ParseFloat(sdp, 64); err != nil || !((v > 0 && v < 100) || v == -1) {
			add("ubl-BE-07", "the settlement discount percent shall be numeric and between 0 and 100")
		}
		// xs:decimal(cbc:Amount) as an assertion test: an absent amount is the
		// empty sequence and a zero amount is false, so both fail.
		amt := strings.TrimSpace(pt.str("Amount"))
		if v, err := strconv.ParseFloat(amt, 64); amt == "" || err != nil || v == 0 {
			add("ubl-BE-08", "a settlement discount shall have a non-zero Amount")
		}
		due := strings.TrimSpace(pt.str("PaymentDueDate"))
		if _, err := time.Parse("2006-01-02", due); len(due) != 10 || err != nil {
			add("ubl-BE-09", "a settlement discount shall have a PaymentDueDate formatted YYYY-MM-DD")
		}
	}

	// ubl-BE-14, context //cac:InvoiceLine | //cac:CreditNoteLine:
	//   count(cac:TaxTotal) = 1
	// ubl-BE-13, in the context //cac:InvoiceLine/cac:TaxTotal, is not evaluated:
	// it is bound to an expression that cannot be false. See ublBE13Reason.
	lines := append(root.findAll("InvoiceLine"), root.findAll("CreditNoteLine")...)
	for _, ln := range lines {
		if len(ln.all("TaxTotal")) != 1 {
			add("ubl-BE-14", "each invoice line shall have exactly one TaxTotal")
		}
	}

	// ubl-BE-15, context //cac:ClassifiedTaxCategory: count(cbc:Name) = 1. A count,
	// not a non-empty test — an empty <cbc:Name/> satisfies it, and two of them do
	// not.
	for _, ctc := range root.findAll("ClassifiedTaxCategory") {
		if len(ctc.all("Name")) != 1 {
			add("ubl-BE-15", "each ClassifiedTaxCategory shall have exactly one Name")
		}
	}

	return out
}
