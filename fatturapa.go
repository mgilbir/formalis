package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Italian FatturaPA / FatturaElettronica format. Unlike
// the CIUS in this package, FatturaPA is not an EN 16931 profile — it is Italy's
// own national e-invoice XML (the format exchanged through the Sistema di
// Interscambio, SdI). It has no EN 16931 business-rule Schematron; the SdI
// validates it against an XSD plus a set of consistency checks. This validator
// therefore checks the mandatory structure and the Italian code lists directly
// against the parsed XML tree (parseCII is namespace-agnostic, so it reads the
// FatturaElettronica tree by local element name).
//
// Rule identifiers are FPA-* (this package's own), since FatturaPA has no public
// rule-id scheme; the messages reference the SdI terms.
//
// Not vendored: the FatturaPA sample instances (phax/phive-rules) are used only
// as the oracle.

// fpaFormats is the set of FormatoTrasmissione values (and the matching versione
// attribute): FPA12 for public administration, FPR12 for private, and the
// superseded SDI10/SDI11.
var fpaFormats = map[string]bool{"FPA12": true, "FPR12": true, "SDI11": true, "SDI10": true}

// fpaTipoDocumento is the TD** document-type code set (TD01 … TD28).
var fpaTipoDocumento = buildRange("TD", 1, 28)

// fpaRegimeFiscale is the RF** seller tax-regime code set (RF01 … RF19).
var fpaRegimeFiscale = buildRange("RF", 1, 19)

func buildRange(prefix string, lo, hi int) map[string]bool {
	m := make(map[string]bool, hi-lo+1)
	for i := lo; i <= hi; i++ {
		m[fmt.Sprintf("%s%02d", prefix, i)] = true
	}
	return m
}

// IsFatturaPA reports whether the XML is a FatturaElettronica document.
func IsFatturaPA(xmlData []byte) bool {
	r := newRun(nil)
	root, err := parseCII(r, xmlData)
	return err == nil && root.name == "FatturaElettronica"
}

// ValidateFatturaPA validates an Italian FatturaPA / FatturaElettronica document
// against its mandatory structure and Italian code lists.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation and never
// an empty slice, so it cannot be mistaken for a valid invoice.
func ValidateFatturaPA(ctx context.Context, xmlData []byte) []Violation {
	r := newRun(ctx)
	return r.finish(validateFatturaPA(r, xmlData))
}

func validateFatturaPA(r *run, xmlData []byte) []Violation {
	root, err := parseCII(r, xmlData)
	if err != nil {
		return syntaxViolation(err)
	}
	if root.name != "FatturaElettronica" {
		return []Violation{{Rule: "FPA-root", Message: "the document root shall be FatturaElettronica"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// FPA-format: FormatoTrasmissione shall be a valid value and match the
	// document's versione attribute.
	hdr := root.child("FatturaElettronicaHeader").orNil()
	tx := hdr.child("DatiTrasmissione").orNil()
	format := strings.TrimSpace(tx.str("FormatoTrasmissione"))
	if !fpaFormats[format] {
		add("FPA-format", fmt.Sprintf("the FormatoTrasmissione (%q) shall be one of FPA12, FPR12", format))
	}
	if v := strings.TrimSpace(root.attr("versione")); v != "" && format != "" && v != format {
		add("FPA-format", fmt.Sprintf("the versione attribute (%q) shall match the FormatoTrasmissione (%q)", v, format))
	}

	// FPA-transmitter: the transmitter identity (IdPaese + IdCodice) is mandatory.
	if tx.str("IdTrasmittente", "IdPaese") == "" || tx.str("IdTrasmittente", "IdCodice") == "" {
		add("FPA-transmitter", "the transmitter identifier (IdTrasmittente/IdPaese and IdCodice) shall be present")
	}
	// FPA-destination: the recipient code (CodiceDestinatario) is mandatory.
	if strings.TrimSpace(tx.str("CodiceDestinatario")) == "" {
		add("FPA-destination", "the recipient code (CodiceDestinatario) shall be present")
	}

	// Seller (CedentePrestatore) and Buyer (CessionarioCommittente).
	validateFPAParty(hdr.child("CedentePrestatore").orNil(), "seller", "CedentePrestatore", true, add)
	validateFPAParty(hdr.child("CessionarioCommittente").orNil(), "buyer", "CessionarioCommittente", false, add)

	// Each FatturaElettronicaBody: general data, lines and VAT summary.
	bodies := root.all("FatturaElettronicaBody")
	if len(bodies) == 0 {
		add("FPA-body", "the invoice shall contain at least one FatturaElettronicaBody")
	}
	for _, b := range bodies {
		dg := b.child("DatiGenerali", "DatiGeneraliDocumento").orNil()
		if td := strings.TrimSpace(dg.str("TipoDocumento")); !fpaTipoDocumento[td] {
			add("FPA-doctype", fmt.Sprintf("the document type (TipoDocumento=%q) shall be a valid TD** code (TD01-TD28)", td))
		}
		if cur := strings.TrimSpace(dg.str("Divisa")); !en16931Currencies[cur] {
			add("FPA-currency", fmt.Sprintf("the currency (Divisa=%q) shall be a valid ISO 4217 code", cur))
		}
		if strings.TrimSpace(dg.str("Data")) == "" {
			add("FPA-date", "the document date (Data) shall be present")
		}
		if strings.TrimSpace(dg.str("Numero")) == "" {
			add("FPA-number", "the document number (Numero) shall be present")
		}
		bs := b.child("DatiBeniServizi").orNil()
		if len(bs.all("DettaglioLinee")) == 0 {
			add("FPA-lines", "the invoice body shall contain at least one line (DettaglioLinee)")
		}
		if len(bs.all("DatiRiepilogo")) == 0 {
			add("FPA-summary", "the invoice body shall contain at least one VAT summary (DatiRiepilogo)")
		}
	}

	return out
}

// validateFPAParty checks a FatturaPA party's tax identity, name and address.
// seller parties additionally require a tax regime (RegimeFiscale).
func validateFPAParty(p *ciiNode, who, elem string, seller bool, add func(rule, msg string)) {
	da := p.child("DatiAnagrafici").orNil()
	// A tax identity is required: an IdFiscaleIVA (VAT number) or a CodiceFiscale.
	hasVAT := da.str("IdFiscaleIVA", "IdPaese") != "" && da.str("IdFiscaleIVA", "IdCodice") != ""
	if !hasVAT && strings.TrimSpace(da.str("CodiceFiscale")) == "" {
		add("FPA-"+who+"-id", fmt.Sprintf("the %s (%s) shall have an IdFiscaleIVA or a CodiceFiscale", who, elem))
	}
	// A name: Denominazione, or Nome and Cognome.
	an := da.child("Anagrafica").orNil()
	if strings.TrimSpace(an.str("Denominazione")) == "" &&
		(strings.TrimSpace(an.str("Nome")) == "" || strings.TrimSpace(an.str("Cognome")) == "") {
		add("FPA-"+who+"-name", fmt.Sprintf("the %s (%s) shall have a Denominazione or a Nome and Cognome", who, elem))
	}
	if seller {
		if rf := strings.TrimSpace(da.str("RegimeFiscale")); !fpaRegimeFiscale[rf] {
			add("FPA-seller-regime", fmt.Sprintf("the seller tax regime (RegimeFiscale=%q) shall be a valid RF** code (RF01-RF19)", rf))
		}
	}
	// A postal address (Sede): street, post code, town and country.
	sede := p.child("Sede").orNil()
	if strings.TrimSpace(sede.str("Indirizzo")) == "" || strings.TrimSpace(sede.str("CAP")) == "" ||
		strings.TrimSpace(sede.str("Comune")) == "" || strings.TrimSpace(sede.str("Nazione")) == "" {
		add("FPA-"+who+"-address", fmt.Sprintf("the %s address (%s/Sede) shall contain Indirizzo, CAP, Comune and Nazione", who, elem))
	}
}
