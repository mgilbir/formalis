// Package einvoice validates electronic invoices against the EN 16931 semantic
// model and the national Core Invoice Usage Specifications (CIUS) layered on top
// of it. One syntax-neutral rule engine (fed by parseEN16931) serves both XML
// syntaxes — UN/CEFACT Cross Industry Invoice (CII, used by Factur-X/ZUGFeRD) and
// OASIS UBL (Peppol BIS, XRechnung UBL, NLCIUS) — and each CIUS adds its own rule
// layer in its own file (xrechnung.go, peppol.go, nlcius.go). The package is free
// of any PDF dependency; the pdf0 package wraps it for the Factur-X container.
package formalis

import (
	"fmt"
	"strings"
)

// Profile is an EN 16931 conformance profile, in increasing data richness. The
// first five are the Factur-X/ZUGFeRD profiles; XRechnung is the German CIUS.
type Profile string

const (
	ProfileMinimum   Profile = "MINIMUM"
	ProfileBasicWL   Profile = "BASIC WL"
	ProfileBasic     Profile = "BASIC"
	ProfileEN16931   Profile = "EN 16931"
	ProfileExtended  Profile = "EXTENDED"
	ProfileXRechnung Profile = "XRECHNUNG" // ZUGFeRD 2.x German XRechnung CIUS of EN 16931
)

// ProfileFor maps an XMP ConformanceLevel string to a profile. The value is
// matched case- and space-insensitively, since producers write both "EN 16931"
// and "EN16931", and "BASIC WL" and "BASICWL".
func ProfileFor(level string) (Profile, bool) {
	switch strings.ToUpper(strings.ReplaceAll(level, " ", "")) {
	case "MINIMUM":
		return ProfileMinimum, true
	case "BASICWL":
		return ProfileBasicWL, true
	case "BASIC":
		return ProfileBasic, true
	case "EN16931":
		return ProfileEN16931, true
	case "EXTENDED":
		return ProfileExtended, true
	case "XRECHNUNG":
		return ProfileXRechnung, true
	}
	return "", false
}

// Source identifies the authority that defines a rule, so that a rule identifier
// is unique within it. Two authorities may mint the same string; (Source, Rule)
// is what identifies a rule, and Rule alone is not an identity.
//
// The distinction is not decorative. This package reports identifiers minted by
// CEN, by seven national bodies, and by itself, in one flat string field, and
// they have already collided: the Order-X validator once emitted "BR-O-01" for
// "an Order shall have an order number" while the EN 16931 rule engine emitted
// "BR-O-01" for the "Not subject to VAT" category family. A caller aggregating
// findings across a mailbox keyed by Rule merged two unrelated defects, and a
// suppression list ("we accept BR-O-03 from this supplier") suppressed the wrong
// thing in the other document type. Scoping every finding by its author makes
// that class of mistake impossible to express.
//
// Three judgement calls are recorded here because they are not obvious:
//
//   - The UBL and CII syntax-binding rules (UBL-DT-*, UBL-SR-*, CII-SR-*) carry
//     SourceEN16931, not a Source of their own. Source names the *authority*, and
//     CEN publishes the bindings (EN 16931-3-2, EN 16931-3-3) as normative parts
//     of the same standard as the semantic model (EN 16931-1); they arrive in the
//     same conformance artefacts this package is tested against, and the FP=0
//     oracle counts them among the rules it must catch. Splitting them would also
//     make one finding's Source depend on the invoice's syntax, since the rule
//     engine picks UBL-SR-44 or CII-SR-469 for the same defect from the same
//     model — a distinction the caller did not ask for. A caller that does want
//     "syntax binding" separately has the prefix, which is already disjoint from
//     the core BR-* space.
//
//   - RuleLimit and RuleSyntax carry SourceChecker. They are statements by this
//     checker — "I stopped early", "I could not read this file" — rather than by
//     any rule authority, so attributing them to CEN would be a lie, and leaving
//     them unattributed would make Source unusable as a filter. See
//     IsCheckerViolation for why that predicate still tests Rule alone.
//
//   - Most national formats below (FatturaPA, Facturae, ebInterface, KSeF,
//     Finvoice, TEAPPS, OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT,
//     Order-X) do not publish a rule identifier this package could quote, so the
//     identifiers under those Sources — "FPA-number", "ZA-uuid", "ORDER-01" —
//     were invented here. The Source is still the format they judge the document
//     against, which is what a caller routing or suppressing by format needs; it
//     is not a claim that the format's own documentation uses these names. The
//     Sources whose identifiers *are* quoted from a published rule set are
//     EN 16931, XRechnung, Peppol, NLCIUS, CIUS-PT, CIUS-RO, UBL.BE and SRBDT.
type Source string

const (
	// SourceEN16931 is CEN's EN 16931 — the semantic model's core business rules
	// (BR-*, BR-CO-*, BR-CL-*, BR-DEC-*, BR-IC-*, and the VAT category families)
	// together with the UBL and CII syntax bindings (UBL-DT-*, UBL-SR-*, CII-SR-*).
	SourceEN16931 Source = "EN 16931"
	// SourceXRechnung is the German KoSIT XRechnung CIUS (BR-DE-*).
	SourceXRechnung Source = "XRechnung"
	// SourcePeppol is OpenPEPPOL BIS Billing 3.0 (PEPPOL-EN16931-R*).
	SourcePeppol Source = "Peppol"
	// SourceNLCIUS is the Dutch SimplerInvoicing NLCIUS (BR-NL-*).
	SourceNLCIUS Source = "NLCIUS"
	// SourceCIUSPT is the Portuguese AT/eSPap CIUS-PT (BR-CIUS-PT-*).
	SourceCIUSPT Source = "CIUS-PT"
	// SourceCIUSRO is the Romanian ANAF RO e-Factura CIUS (BR-RO-*).
	SourceCIUSRO Source = "CIUS-RO"
	// SourceUBLBE is the Belgian UBL.BE CIUS (ubl-BE-*).
	SourceUBLBE Source = "UBL.BE"
	// SourceSRBDT is the Serbian SRBDT CIUS (RSR-*).
	SourceSRBDT Source = "SRBDT"
	// SourceFatturaPA is the Italian SdI FatturaPA format (FPA-*).
	SourceFatturaPA Source = "FatturaPA"
	// SourceFacturae is the Spanish Facturae format (FE-*).
	SourceFacturae Source = "Facturae"
	// SourceEbInterface is the Austrian ebInterface format (EB-*).
	SourceEbInterface Source = "ebInterface"
	// SourceKSeF is the Polish KSeF FA(2) format (KS-*).
	SourceKSeF Source = "KSeF"
	// SourceFinvoice is the Finnish Finvoice format (FI-*).
	SourceFinvoice Source = "Finvoice"
	// SourceTEAPPS is the Finnish Tieto TEAPPSXML format (TP-*).
	SourceTEAPPS Source = "TEAPPS"
	// SourceOIOUBL is the Danish OIOUBL profile (OIO-*).
	SourceOIOUBL Source = "OIOUBL"
	// SourceSvefaktura is the Swedish Svefaktura format (SV-*).
	SourceSvefaktura Source = "Svefaktura"
	// SourceZATCA is the Saudi ZATCA e-invoicing profile (ZA-*).
	SourceZATCA Source = "ZATCA"
	// SourceOSA is the Hungarian NAV Online Számla format (HU-*).
	SourceOSA Source = "NAV OSA"
	// SourceUBLTR is the Turkish UBL-TR e-Fatura profile (TR-*).
	SourceUBLTR Source = "UBL-TR"
	// SourcePINT is the Peppol International (PINT) billing model (PINT-*).
	SourcePINT Source = "PINT"
	// SourceOrderX is the Franco-German Order-X order document (ORDER-*).
	SourceOrderX Source = "Order-X"
	// SourceChecker is this package speaking about its own run rather than about
	// any rule: RuleLimit and RuleSyntax.
	SourceChecker Source = "checker"
)

// Violation reports one way in which a document departs from a rule set. Source
// names the authority that defines the rule and Rule is that authority's
// identifier for it (e.g. SourceEN16931/"BR-CO-15", SourceNLCIUS/"BR-NL-1");
// neither is an identity on its own.
type Violation struct {
	Source  Source
	Rule    string
	Message string
	Object  int
}

func (v Violation) Error() string {
	if v.Object != 0 {
		return fmt.Sprintf("%s %s: %s (object %d)", v.Source, v.Rule, v.Message, v.Object)
	}
	return fmt.Sprintf("%s %s: %s", v.Source, v.Rule, v.Message)
}

// adder returns the emission helper a rule set uses to append its findings,
// stamping src on each one.
//
// Every rule set builds its own, so the Source is fixed at the point of
// emission rather than painted on afterwards. That matters because several
// validators emit under more than one authority in a single call — the
// XRechnung validator reports EN 16931 core findings alongside BR-DE-*, as do
// the CIUS-PT, CIUS-RO, NLCIUS, Peppol, SRBDT and UBL.BE validators — so
// stamping one Source over a returned slice would misattribute the core half.
func adder(out *[]Violation, src Source) func(rule, msg string) {
	return func(rule, msg string) {
		*out = append(*out, Violation{Source: src, Rule: rule, Message: msg})
	}
}
