package formalis

import (
	"context"
	"strings"
)

// This file dispatches an invoice to the right validator based on the CIUS it
// declares in its Specification identifier (BT-24, the UBL CustomizationID or the
// CII GuidelineSpecifiedDocumentContextParameter/ID). Callers that do not know
// which national profile an invoice targets can use ValidateCIUS and let the
// document route itself.

// CIUS identifies a Core Invoice Usage Specification the dispatcher recognises.
type CIUS string

const (
	CIUSNone      CIUS = ""          // plain EN 16931 core (no recognised CIUS)
	CIUSXRechnung CIUS = "XRechnung" // German public-sector CIUS
	CIUSPeppol    CIUS = "Peppol"    // OpenPEPPOL BIS Billing 3.0
	CIUSNLCIUS    CIUS = "NLCIUS"    // Dutch SimplerInvoicing / SI-UBL
	CIUSPortugal  CIUS = "CIUS-PT"   // Portuguese AT/eSPap CIUS-PT
	CIUSRomania   CIUS = "CIUS-RO"   // Romanian ANAF RO e-Factura
	CIUSBelgium   CIUS = "UBL.BE"    // Belgian UBL.BE
	CIUSSerbia    CIUS = "SRBDT"     // Serbian SRBDT
)

// DetectCIUS reports the CIUS that a Specification identifier (BT-24) declares, or
// CIUSNone when it names no recognised national CIUS. XRechnung is checked before
// Peppol because an XRechnung identifier may also reference the Peppol base.
func DetectCIUS(specID string) CIUS {
	id := strings.ToLower(specID)
	switch {
	case strings.Contains(id, "xrechnung"):
		return CIUSXRechnung
	case strings.Contains(id, "nlcius") || strings.Contains(id, "nen.nl"):
		return CIUSNLCIUS
	case strings.Contains(id, "cius-pt") || strings.Contains(id, "feap.gov.pt"):
		return CIUSPortugal
	case strings.Contains(id, "cius-ro") || strings.Contains(id, "mfinante.ro"):
		return CIUSRomania
	case strings.Contains(id, "ubl.be"):
		return CIUSBelgium
	case strings.Contains(id, "srbdt") || strings.Contains(id, "mfin.gov.rs"):
		return CIUSSerbia
	case strings.Contains(id, "peppol"):
		return CIUSPeppol
	}
	return CIUSNone
}

// ValidateCIUS validates an invoice against whichever CIUS its Specification
// identifier (BT-24) declares, falling back to the EN 16931 core when none is
// recognised. It routes both syntaxes (CII and UBL).
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty Report, so it cannot be mistaken for a valid invoice.
//
// The Report's coverage follows the document, not this entry point: it names
// the gaps of the rule set the BT-24 dispatch actually ran, so an XRechnung
// invoice comes back with the XRechnung gaps and a Portuguese one with the
// CIUS-PT gaps. A document that declares no recognised CIUS is validated
// against the EN 16931 core and reports the core's gaps alone.
func ValidateCIUS(ctx context.Context, xmlData []byte) Report {
	r := newRun(ctx)
	p, err := parseEN16931(r, xmlData)
	if err != nil {
		// The document never got as far as declaring a CIUS, so the core — the
		// rule set every branch of the dispatch runs — is the honest claim here.
		return newReport(r.finish(syntaxViolation(err)), SourceEN16931)
	}
	out, sources := validateCIUS(r, p)
	return newReport(r.finish(out), sources...)
}

// validateCIUS routes the document and reports which authorities' rules it ran,
// so the caller's Report can name that rule set's gaps rather than a fixed set
// this entry point guessed at.
func validateCIUS(r *run, p *parsed) ([]Violation, []Source) {
	// The dispatch is to the unexported forms so the whole dispatched call shares
	// this run: one cancellation signal and one set of budgets across the detect
	// and the validate, rather than a fresh (uncancellable) allowance for the
	// second half of the work.
	//
	// It also hands on the document already parsed. Routing on BT-24 needs the
	// model, and the validator it routes to needs the same model: reading the
	// bytes again to rebuild a byte-identical tree would charge the shared
	// element budget twice for one document, so the CIUS entry point would refuse
	// documents the CIUS-specific entry point accepts.
	switch DetectCIUS(p.inv.specID) {
	case CIUSXRechnung:
		return validateXRechnung(r, p), []Source{SourceEN16931, SourceXRechnung}
	case CIUSNLCIUS:
		return validateNLCIUS(r, p), []Source{SourceEN16931, SourceNLCIUS}
	case CIUSPortugal:
		return validateCIUSPT(r, p), []Source{SourceEN16931, SourceCIUSPT}
	case CIUSRomania:
		return validateCIUSRO(r, p), []Source{SourceEN16931, SourceCIUSRO}
	case CIUSBelgium:
		return validateUBLBE(r, p), []Source{SourceEN16931, SourceUBLBE}
	case CIUSSerbia:
		return validateSRBDT(r, p), []Source{SourceEN16931, SourceSRBDT}
	case CIUSPeppol:
		return validatePeppol(r, p), []Source{SourceEN16931, SourcePeppol}
	default:
		return validateEN16931(r, p.inv, ProfileEN16931), []Source{SourceEN16931}
	}
}
