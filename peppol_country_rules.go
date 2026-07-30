package formalis

import (
	"math"
	"strings"
)

// The country-specific half of the OpenPEPPOL rule set: the 101 identifiers
// PEPPOL-EN16931-UBL.sch and -CII.sch publish under the comment "National rules",
// alongside the 59 PEPPOL-* rules peppol_rules.go evaluates.
//
// # Why they are evaluated here at all
//
// The question is whether a plain ValidatePeppol call should run them, and the
// artefacts answer it three times over:
//
//   - Neither binding file declares a single <phase>. ISO Schematron activates
//     every pattern when no phase is selected, so there is no configuration in
//     which a reference validator runs the PEPPOL-* patterns and skips these. The
//     one <pattern> that carries an id at all — id="german-rules" — is named for a
//     reader, not selected by anything: no <phase>/<active> element refers to it.
//   - buildconfig.xml's peppolbis-en16931-base-3.0-ubl and -cii configurations are
//     `<file path="sch/PEPPOL-EN16931-UBL.sch"/>` with no phase attribute, and the
//     profile-01 configurations inherit them whole. The base configuration is the
//     file.
//   - Every rule is gated inside itself, on the supplier's country and — for the
//     domestic-only ones — the customer's too. That is what makes running them
//     unconditionally correct rather than reckless: the gate is OpenPEPPOL's, and
//     a French invoice matches no country rule's context.
//
// So these are not an opt-in national profile layered on top of Peppol BIS. They
// are part of the base rule set, and this package's not evaluating them was the
// gap Coverage(SourcePeppol) recorded.
//
// # What "gated inside itself" means, precisely
//
// It means five different things, and getting them interchangeable would be the
// same defect as evaluating a UBL rule on a CII document:
//
//	DE  $supplierCountryIsDE and $customerCountryIsDE — both postal addresses (BT-40/BT-55)
//	DK  $DKSupplierCountry / $DKCustomerCountry — both postal addresses, some rules domestic-only
//	GR  $isGreekSender / $isGreekSenderandReceiver — $supplierCountry, which prefers the VAT prefix
//	IS  $SupplierCountry / $CustomerCountry — both postal addresses
//	IT  $supplierCountry = 'IT' — the VAT prefix, then the tax representative's, then the address
//	NL  $supplierCountryIsNL / $customerCountryIsNL / $taxRepresentativeCountryIsNL — postal addresses
//	NO  $supplierCountry = 'NO' — the VAT prefix first, as IT
//	SE  no variable at all: each rule's context carries the country test inline
//
// The difference is load-bearing. A Norwegian seller whose postal address says SE
// but whose VAT identifier begins NO answers to the Norwegian rules and not the
// Swedish ones, because $supplierCountry reads the VAT prefix before the address
// and the Swedish rules read the address. peppolUBLSupplierCountry and
// peppolUBLPostalCountry are therefore two functions and not one.
//
// # Bindings
//
// The UBL file publishes all 101; the CII file publishes 41 of them — DK (13 of
// 14: not DK-R-017), IT, NL, NO and SE — with the same flags and different XPaths.
// DE, GR, IS and DK-R-017 are UBL-only, so they must not fire on a CII document:
// peppolCountryRules records the bindings and peppolEval.has is the gate, the same
// way C32's eight P0104..P0111 rules were stopped from firing on CII.
//
// KoSIT imports none of them — rule-list.xml whitelists 21 PEPPOL-EN16931-*
// identifiers and no country rule — so peppolXRImports keeps the whole family off
// the XRechnung path, and validatePeppolRuleSet does not even walk for them there.
// That matters for the German family in particular: OpenPEPPOL's DE-R-NNN is a
// re-publication of KoSIT's BR-DE-NNN, so an XRechnung validation reporting both
// would report every German defect twice under two authorities' identifiers.

// peppolCountryRules is every country-specific identifier the vendored OpenPEPPOL
// Schematron publishes, with the bindings that publish it and the flag it carries.
//
// It is peppolRules' counterpart for the other rule set in the same two files, and
// the same guards read it: TestPeppolCountryRuleTableMatchesTheSchematron compares
// it against an XML decoder's reading of both files in both directions, and
// TestEveryPublishedPeppolRuleHasBothVerdicts requires a document that trips each
// entry and one that does not.
var peppolCountryRules = map[string]peppolRule{

	// Denmark — 12 fatal, 2 advisory. DK-R-017 is the one the CII file omits.
	"DK-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-003": {peppolUBL | peppolCII, SeverityWarning},
	"DK-R-004": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-005": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-006": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-007": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-008": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-009": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-010": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-011": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-013": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-014": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-016": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-017": {peppolUBL, SeverityWarning},

	// The Netherlands — 9 fatal, both bindings. Distinct from the BR-NL-* of
	// SourceNLCIUS: this is OpenPEPPOL's Dutch rule set, not NLCIUS's.
	"NL-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-004": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-005": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-006": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-007": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-008": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-009": {peppolUBL | peppolCII, SeverityFatal},

	// Italy — 4 fatal, both bindings.
	"IT-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-004": {peppolUBL | peppolCII, SeverityFatal},

	// Norway — 1 fatal, 1 advisory, both bindings.
	"NO-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"NO-R-002": {peppolUBL | peppolCII, SeverityWarning},

	// Sweden — 7 fatal, 6 advisory, both bindings.
	"SE-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-004": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-005": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-006": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-007": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-008": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-009": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-010": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-011": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-012": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-013": {peppolUBL | peppolCII, SeverityFatal},
}

// ---------------------------------------------------------------------------
// The country variables both bindings declare
// ---------------------------------------------------------------------------

// peppolSubstring is XPath's fn:substring over codepoints, with the one-based
// positions and the "keep every position p where start <= p < start+length"
// semantics that make substring($v, 0, 4) the first three characters and not the
// first four. DK-R-008..011 in the CII binding are written that way.
//
// length is variadic so the two-argument form — substring($v, 3), used by NO-R-001
// and GR-R-003 — is the same function.
func peppolSubstring(s string, start int, length ...int) string {
	r := []rune(s)
	lo, hi := start, len(r)+1
	if len(length) > 0 {
		hi = start + length[0]
	}
	if lo < 1 {
		lo = 1
	}
	if hi > len(r)+1 {
		hi = len(r) + 1
	}
	if lo >= hi {
		return ""
	}
	return string(r[lo-1 : hi-1])
}

// peppolCountryOf is the `if (A) then upper-case(normalize-space(A)) else if (B)
// then ... else 'XX'` chain $supplierCountry and $customerCountry share.
//
// The condition is the raw expression's effective boolean value and the value is
// its normalized form, which are not the same test: a candidate that is one space
// selects the branch and yields "". Passing the raw strings and normalizing here
// keeps that distinction rather than collapsing it.
func peppolCountryOf(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return strings.ToUpper(normalizeSpace(c))
		}
	}
	return "XX"
}

// peppolUBLVATPrefix is `cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/substring(cbc:CompanyID, 1, 2)`
// under a UBL party.
//
// The predicate compares the element's string value with no normalization, which
// is how the artefact writes it, so a cbc:ID padded with whitespace does not
// select the group.
func peppolUBLVATPrefix(party *ciiNode) string {
	return peppolSubstring(peppolUBLVATCompanyID(party), 1, 2)
}

// peppolUBLVATCompanyID is the cbc:CompanyID of the same group, untrimmed.
func peppolUBLVATCompanyID(party *ciiNode) string {
	for _, ts := range party.orNil().all("PartyTaxScheme") {
		if ts.child("TaxScheme", "ID").rawText() == "VAT" {
			return ts.child("CompanyID").rawText()
		}
	}
	return ""
}

// peppolUBLPostalCountry is `cac:<party>/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode`
// from the document element — the term BT-40/BT-55, untrimmed.
func peppolUBLPostalCountry(root *ciiNode, party string) string {
	return root.child(party, "Party", "PostalAddress", "Country", "IdentificationCode").rawText()
}

// peppolUBLSupplierCountry is the UBL binding's $supplierCountry: the seller's VAT
// prefix, then the tax representative's, then the seller's postal country, then
// 'XX'. The Italian, Norwegian and Greek rules are gated on it.
func peppolUBLSupplierCountry(root *ciiNode) string {
	return peppolCountryOf(
		peppolUBLVATPrefix(root.child("AccountingSupplierParty", "Party")),
		peppolUBLVATPrefix(root.child("TaxRepresentativeParty")),
		peppolUBLPostalCountry(root, "AccountingSupplierParty"),
	)
}

// peppolUBLCustomerCountry is $customerCountry, which has no tax-representative
// step.
func peppolUBLCustomerCountry(root *ciiNode) string {
	return peppolCountryOf(
		peppolUBLVATPrefix(root.child("AccountingCustomerParty", "Party")),
		peppolUBLPostalCountry(root, "AccountingCustomerParty"),
	)
}

// peppolCIIVATPrefix is `ram:SpecifiedTaxRegistration[ram:ID/@schemeID = 'VAT']/substring(ram:ID, 1, 2)`
// under a CII trade party.
func peppolCIIVATPrefix(party *ciiNode) string {
	return peppolSubstring(peppolCIIVATID(party), 1, 2)
}

// peppolCIIVATID is the ram:ID of the same group, untrimmed.
func peppolCIIVATID(party *ciiNode) string {
	for _, reg := range party.orNil().all("SpecifiedTaxRegistration") {
		for _, id := range reg.all("ID") {
			if id.attr("schemeID") == "VAT" {
				return id.rawText()
			}
		}
	}
	return ""
}

// peppolCIIParty is one of the header trade parties, by element name.
func peppolCIIParty(root *ciiNode, name string) *ciiNode {
	return root.child("SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement", name)
}

// peppolCIIPostalCountry is a trade party's ram:PostalTradeAddress/ram:CountryID,
// untrimmed.
func peppolCIIPostalCountry(root *ciiNode, name string) string {
	return peppolCIIParty(root, name).child("PostalTradeAddress", "CountryID").rawText()
}

// peppolCIISupplierCountry is the CII binding's $supplierCountry.
func peppolCIISupplierCountry(root *ciiNode) string {
	return peppolCountryOf(
		peppolCIIVATPrefix(peppolCIIParty(root, "SellerTradeParty")),
		peppolCIIVATPrefix(peppolCIIParty(root, "SellerTaxRepresentativeTradeParty")),
		peppolCIIPostalCountry(root, "SellerTradeParty"),
	)
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

// peppolCountryUBLRules evaluates the country-specific rules of
// PEPPOL-EN16931-UBL.sch. It is a no-op for any other root.
func peppolCountryUBLRules(e *peppolEval, r *run, root *ciiNode) {
	if root == nil || (root.name != "Invoice" && root.name != "CreditNote") {
		return
	}
	peppolNorwegianUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolItalianUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolDutchUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolDanishUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolSwedishUBLRules(e, root)
}

// peppolCountryCIIRules evaluates the country-specific rules of
// PEPPOL-EN16931-CII.sch — the 41 identifiers that file publishes.
func peppolCountryCIIRules(e *peppolEval, r *run, root *ciiNode) {
	if root == nil || root.name != "CrossIndustryInvoice" {
		return
	}
	peppolNorwegianCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolItalianCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolDutchCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolDanishCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolSwedishCIIRules(e, root)
}

// ---------------------------------------------------------------------------
// Norway
// ---------------------------------------------------------------------------

// peppolNorwegianUBLRules is the Norwegian pattern of the UBL binding, context
// `cac:AccountingSupplierParty/cac:Party[$supplierCountry = 'NO']`.
func peppolNorwegianUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLSupplierCountry(root) != "NO" {
		return
	}
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		// NO-R-002: normalize-space(cac:PartyTaxScheme[normalize-space(cac:TaxScheme/
		// cbc:ID) = 'TAX']/cbc:CompanyID) = 'Foretaksregisteret'
		//
		// This predicate normalizes and the VAT one two lines down does not; both are
		// transcribed as written.
		found := false
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) == "TAX" &&
				normalizeSpace(ts.child("CompanyID").rawText()) == "Foretaksregisteret" {
				found = true
			}
		}
		if !found {
			e.add("NO-R-002", "For Norwegian suppliers, the word \"Foretaksregisteret\" MUST be appended to the "+
				"invoice as a TAX-scheme company identifier")
		}
		// NO-R-001: <prefix is NO> and matches(substring(id,3), '^[0-9]{9}MVA$') and
		// u:mod11(substring(id, 3, 9)) or not(<prefix is NO>)
		//
		// The `or not(...)` arm is the whole conditionality of the rule: a Norwegian
		// seller whose VAT identifier is another country's is not reported.
		id := peppolUBLVATCompanyID(party)
		if peppolNorwegianVATBad(id) {
			e.addf("NO-R-001", "The Norwegian VAT identifier (BT-31=%q) MUST be NO, nine digits passing the mod-11 "+
				"check, and the letters MVA", normalizeSpace(id))
		}
	}
}

// peppolNorwegianCIIRules is the same pattern in the CII binding, context
// `ram:SellerTradeParty[$supplierCountry = 'NO']`.
func peppolNorwegianCIIRules(e *peppolEval, root *ciiNode) {
	if peppolCIISupplierCountry(root) != "NO" {
		return
	}
	for _, party := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement", "SellerTradeParty") {
		// NO-R-002: ram:SpecifiedTaxRegistration/ram:ID[@schemeID = 'FC']
		//           [normalize-space(text()) = 'Foretaksregisteret']
		//
		// An existence test on a predicated node, not a string comparison — the two
		// bindings differ here, and the CII one is satisfied by any such element.
		found := false
		for _, reg := range party.all("SpecifiedTaxRegistration") {
			for _, id := range reg.all("ID") {
				if id.attr("schemeID") == "FC" && normalizeSpace(id.rawText()) == "Foretaksregisteret" {
					found = true
				}
			}
		}
		if !found {
			e.add("NO-R-002", "For Norwegian suppliers, the word \"Foretaksregisteret\" MUST be appended to the "+
				"invoice as an FC-scheme tax registration")
		}
		if id := peppolCIIVATID(party); peppolNorwegianVATBad(id) {
			e.addf("NO-R-001", "The Norwegian VAT identifier (BT-31=%q) MUST be NO, nine digits passing the mod-11 "+
				"check, and the letters MVA", normalizeSpace(id))
		}
	}
}

// peppolNorwegianVATBad is NO-R-001's assertion, negated: both bindings write the
// same test over a different element.
//
//	substring(id, 1, 2) = 'NO' and matches(substring(id, 3), '^[0-9]{9}MVA$')
//	  and u:mod11(substring(id, 3, 9))
//	or not(substring(id, 1, 2) = 'NO')
func peppolNorwegianVATBad(id string) bool {
	if peppolSubstring(id, 1, 2) != "NO" {
		return false
	}
	body := peppolSubstring(id, 3)
	if len(body) != 12 || !strings.HasSuffix(body, "MVA") || !peppolAllDigits(body[:9]) {
		return true
	}
	return !peppolMod11(peppolSubstring(id, 3, 9))
}

// ---------------------------------------------------------------------------
// Italy
// ---------------------------------------------------------------------------

// peppolItalianUBLRules is the Italian pattern of the UBL binding: two rules,
// both gated `cac:AccountingSupplierParty/cac:Party[$supplierCountry = 'IT']`.
func peppolItalianUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLSupplierCountry(root) != "IT" {
		return
	}
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		// IT-R-001, context .../cac:PartyTaxScheme[normalize-space(cac:TaxScheme/
		// cbc:ID) != 'VAT']: matches(normalize-space(cbc:CompanyID),'^[A-Z0-9]{11,16}$')
		//
		// A cac:PartyTaxScheme with no cac:TaxScheme at all selects, because
		// normalize-space(()) is '' and '' != 'VAT'.
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) == "VAT" {
				continue
			}
			id := normalizeSpace(ts.child("CompanyID").rawText())
			if !peppolItalianTaxRegOK(id) {
				e.addf("IT-R-001", "For Italian suppliers the Seller tax registration identifier (BT-32=%q) MUST be "+
					"11 to 16 upper-case alphanumerics", id)
			}
		}
		// IT-R-002/003/004 are existence tests on the postal address, so an element
		// written empty satisfies them — the shape C32 records for R001/R003/R010/R020.
		addr := party.child("PostalAddress")
		if addr.child("StreetName") == nil {
			e.add("IT-R-002", "Italian suppliers MUST provide the Seller address line 1 (BT-35)")
		}
		if addr.child("CityName") == nil {
			e.add("IT-R-003", "Italian suppliers MUST provide the Seller city (BT-37)")
		}
		if addr.child("PostalZone") == nil {
			e.add("IT-R-004", "Italian suppliers MUST provide the Seller post code (BT-38)")
		}
	}
}

// peppolItalianCIIRules is the Italian pattern of the CII binding. IT-R-001 is
// written differently there — the context is every ram:SpecifiedTaxRegistration and
// the test excuses any registration that is not scheme FC — while IT-R-002..004 are
// the same existence tests over the CII address element names.
func peppolItalianCIIRules(e *peppolEval, root *ciiNode) {
	if peppolCIISupplierCountry(root) != "IT" {
		return
	}
	for _, party := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement", "SellerTradeParty") {
		for _, reg := range party.all("SpecifiedTaxRegistration") {
			// IT-R-001: ram:ID[normalize-space(@schemeID) != 'FC'] or
			//           matches(normalize-space(ram:ID[normalize-space(@schemeID) = 'FC']), '^[A-Z0-9]{11,16}$')
			//
			// The first arm is an existence test, so a registration carrying any
			// non-FC identifier passes whatever its FC one looks like.
			nonFC, fc := false, ""
			for _, id := range reg.all("ID") {
				if normalizeSpace(id.attr("schemeID")) != "FC" {
					nonFC = true
				} else if fc == "" {
					fc = normalizeSpace(id.rawText())
				}
			}
			if nonFC || peppolItalianTaxRegOK(fc) {
				continue
			}
			e.addf("IT-R-001", "For Italian suppliers the Seller tax registration identifier (BT-32=%q) MUST be "+
				"11 to 16 upper-case alphanumerics", fc)
		}
		addr := party.child("PostalTradeAddress")
		if addr.child("LineOne") == nil {
			e.add("IT-R-002", "Italian suppliers MUST provide the Seller address line 1 (BT-35)")
		}
		if addr.child("CityName") == nil {
			e.add("IT-R-003", "Italian suppliers MUST provide the Seller city (BT-37)")
		}
		if addr.child("PostcodeCode") == nil {
			e.add("IT-R-004", "Italian suppliers MUST provide the Seller post code (BT-38)")
		}
	}
}

// peppolItalianTaxRegOK is IT-R-001's pattern '^[A-Z0-9]{11,16}$'.
func peppolItalianTaxRegOK(s string) bool {
	if len(s) < 11 || len(s) > 16 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// The Netherlands
// ---------------------------------------------------------------------------

// peppolDutchUBLRules is OpenPEPPOL's Dutch pattern in the UBL binding.
//
// This is not the NLCIUS rule set of nlcius.go. That one is BR-NL-* under
// SourceNLCIUS and belongs to a different specification identifier; these nine
// identifiers are OpenPEPPOL's, published in the Peppol BIS Billing files, and
// several say something NLCIUS does not — NL-R-003/005 restrict the legal
// registration scheme to KVK or OIN, and NL-R-008 restricts the payment means
// code, neither of which BR-NL-* states.
//
// The gates are the three <let> variables of the pattern, and all three read a
// postal address rather than a VAT prefix:
//
//	$supplierCountryIsNL          upper-case(normalize-space(/*/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode)) = 'NL'
//	$customerCountryIsNL          the same over cac:AccountingCustomerParty
//	$taxRepresentativeCountryIsNL the same over /*/cac:TaxRepresentativeParty/cac:PostalAddress
func peppolDutchUBLRules(e *peppolEval, root *ciiNode) {
	isNL := func(s string) bool { return strings.EqualFold(normalizeSpace(s), "NL") }
	supplier := isNL(peppolUBLPostalCountry(root, "AccountingSupplierParty"))
	if !supplier {
		return
	}
	customer := isNL(peppolUBLPostalCountry(root, "AccountingCustomerParty"))
	taxRep := isNL(root.child("TaxRepresentativeParty", "PostalAddress", "Country", "IdentificationCode").rawText())

	// NL-R-001, context cbc:CreditNoteTypeCode[$supplierCountryIsNL]:
	//   /*/cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID
	//
	// The context is the credit-note type code, so an invoice is out of scope
	// without the rule needing to say so.
	for range root.all("CreditNoteTypeCode") {
		if len(nodesAt(root, "BillingReference", "InvoiceDocumentReference", "ID")) == 0 {
			e.add("NL-R-001", "For suppliers in the Netherlands, a credit note MUST contain a Preceding Invoice "+
				"reference (BT-25)")
		}
	}
	// NL-R-002 / NL-R-004 / NL-R-006: cbc:StreetName and cbc:CityName and
	// cbc:PostalZone, over the seller's, the buyer's and the tax representative's
	// postal address. Existence tests, so an empty element satisfies them.
	address := func(rule string, addr *ciiNode, who string) {
		if addr == nil {
			return
		}
		if addr.child("StreetName") == nil || addr.child("CityName") == nil || addr.child("PostalZone") == nil {
			e.addf(rule, "For suppliers in the Netherlands, the %s address MUST contain a street name, a city and a "+
				"post code", who)
		}
	}
	for _, addr := range nodesAt(root, "AccountingSupplierParty", "Party", "PostalAddress") {
		address("NL-R-002", addr, "supplier's")
	}
	// NL-R-003 / NL-R-005: the legal registration identifier's scheme (BT-30/BT-47)
	//   (contains(concat(' ', string-join(@schemeID, ' '), ' '), ' 0106 ') or
	//    contains(..., ' 0190 ')) and (normalize-space(.) != '')
	//
	// Both halves matter: the second is the one existence-test reading would miss,
	// and unlike NL-R-002 this rule does reject an empty element.
	legalID := func(rule string, party *ciiNode, who string) {
		for _, le := range party.orNil().all("PartyLegalEntity") {
			for _, id := range le.all("CompanyID") {
				if peppolDutchLegalScheme(id.attr("schemeID")) && normalizeSpace(id.rawText()) != "" {
					continue
				}
				e.addf(rule, "For suppliers in the Netherlands, the %s legal registration identifier MUST be a KVK or "+
					"OIN number (scheme 0106 or 0190) with a value", who)
			}
		}
	}
	legalID("NL-R-003", root.child("AccountingSupplierParty", "Party"), "supplier's")
	if customer {
		for _, addr := range nodesAt(root, "AccountingCustomerParty", "Party", "PostalAddress") {
			address("NL-R-004", addr, "customer's")
		}
		legalID("NL-R-005", root.child("AccountingCustomerParty", "Party"), "customer's")
	}
	if taxRep {
		for _, addr := range nodesAt(root, "TaxRepresentativeParty", "PostalAddress") {
			address("NL-R-006", addr, "fiscal representative's")
		}
	}
	// NL-R-007, context cac:LegalMonetaryTotal[$supplierCountryIsNL]:
	//   (/ubl:Invoice and xs:decimal(cbc:PayableAmount) <= 0.0) or
	//   (/cn:CreditNote and xs:decimal(cbc:PayableAmount) >= 0.0) or (//cac:PaymentMeans)
	//
	// "A means of payment is required if the payment runs from customer to
	// supplier", expressed as: either nothing is owed, or payment instructions are
	// present.
	hasPaymentMeans := len(root.findAll("PaymentMeans")) > 0
	isCredit := root.name == "CreditNote"
	for _, total := range root.all("LegalMonetaryTotal") {
		if hasPaymentMeans {
			continue
		}
		amt := total.child("PayableAmount")
		if amt == nil {
			// xs:decimal(()) is the empty sequence, and both comparisons against it are
			// false, so the assertion rests on the payment means alone.
			e.add("NL-R-007", "For suppliers in the Netherlands, Payment instructions (BG-16) MUST be provided when "+
				"the payment is from the customer to the supplier")
			continue
		}
		due, ok := parseAmount(amt.text)
		if !ok {
			// XPath raises a dynamic error rather than reporting the assertion; that is
			// peppolDecimalOr's argument for R120 and it holds here.
			continue
		}
		if (!isCredit && due <= 0) || (isCredit && due >= 0) {
			continue
		}
		e.add("NL-R-007", "For suppliers in the Netherlands, Payment instructions (BG-16) MUST be provided when the "+
			"payment is from the customer to the supplier")
	}
	// NL-R-008, context cac:PaymentMeans[$supplierCountryIsNL and $customerCountryIsNL].
	if customer {
		for _, pm := range root.findAll("PaymentMeans") {
			code := normalizeSpace(pm.child("PaymentMeansCode").rawText())
			if !peppolDutchPaymentMeans[code] {
				e.addf("NL-R-008", "For suppliers in the Netherlands invoicing a Dutch customer, the Payment means "+
					"type code (BT-81=%q) MUST be one of 30, 48, 49, 57, 58 or 59", code)
			}
		}
	}
	// NL-R-009, context cac:OrderLineReference/cbc:LineID[$supplierCountryIsNL]:
	//   exists(/*/cac:OrderReference/cbc:ID)
	if len(nodesAt(root, "OrderReference", "ID")) == 0 {
		for _, olr := range root.findAll("OrderLineReference") {
			for range olr.all("LineID") {
				e.add("NL-R-009", "For suppliers in the Netherlands, an Invoice line order line reference (BT-132) "+
					"requires a Purchase order reference (BT-13) on document level")
			}
		}
	}
}

// peppolDutchCIIRules is the same pattern in the CII binding.
//
// Two rules are written differently there and the difference is not cosmetic:
// NL-R-001's context is a document type code drawn from a five-code list rather
// than a credit-note root, because CII expresses both document kinds with one
// root; and NL-R-007 discriminates the two by BT-3 = '381' for the same reason.
func peppolDutchCIIRules(e *peppolEval, root *ciiNode) {
	isNL := func(s string) bool { return strings.EqualFold(normalizeSpace(s), "NL") }
	if !isNL(peppolCIIPostalCountry(root, "SellerTradeParty")) {
		return
	}
	customer := isNL(peppolCIIPostalCountry(root, "BuyerTradeParty"))
	taxRep := isNL(peppolCIIPostalCountry(root, "SellerTaxRepresentativeTradeParty"))
	seller := peppolCIIParty(root, "SellerTradeParty")
	buyer := peppolCIIParty(root, "BuyerTradeParty")
	settlements := nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement")

	// NL-R-001, context rsm:ExchangedDocument[some $code in tokenize('81 83 381
	// 396 532', '\s') satisfies normalize-space(ram:TypeCode) = $code]:
	//   //ram:ApplicableHeaderTradeSettlement/ram:InvoiceReferencedDocument/ram:IssuerAssignedID
	for _, doc := range root.all("ExchangedDocument") {
		credit := false
		for _, tc := range doc.all("TypeCode") {
			if peppolDutchCreditNoteTypes[normalizeSpace(tc.rawText())] {
				credit = true
			}
		}
		if !credit {
			continue
		}
		found := false
		for _, st := range root.findAll("ApplicableHeaderTradeSettlement") {
			if len(nodesAt(st, "InvoiceReferencedDocument", "IssuerAssignedID")) > 0 {
				found = true
			}
		}
		if !found {
			e.add("NL-R-001", "For suppliers in the Netherlands, a credit note MUST contain a Preceding Invoice "+
				"reference (BT-25)")
		}
	}
	address := func(rule string, party *ciiNode, who string) {
		for _, addr := range party.orNil().all("PostalTradeAddress") {
			if addr.child("LineOne") == nil || addr.child("CityName") == nil || addr.child("PostcodeCode") == nil {
				e.addf(rule, "For suppliers in the Netherlands, the %s address MUST contain a street name, a city "+
					"and a post code", who)
			}
		}
	}
	legalID := func(rule string, party *ciiNode, who string) {
		for _, org := range party.orNil().all("SpecifiedLegalOrganization") {
			for _, id := range org.all("ID") {
				if peppolDutchLegalScheme(id.attr("schemeID")) && normalizeSpace(id.rawText()) != "" {
					continue
				}
				e.addf(rule, "For suppliers in the Netherlands, the %s legal registration identifier MUST be a KVK "+
					"or OIN number (scheme 0106 or 0190) with a value", who)
			}
		}
	}
	address("NL-R-002", seller, "supplier's")
	legalID("NL-R-003", seller, "supplier's")
	if customer {
		address("NL-R-004", buyer, "customer's")
		legalID("NL-R-005", buyer, "customer's")
	}
	if taxRep {
		address("NL-R-006", peppolCIIParty(root, "SellerTaxRepresentativeTradeParty"), "fiscal representative's")
	}
	// NL-R-007, context ram:SpecifiedTradeSettlementHeaderMonetarySummation.
	isCredit := false
	for _, doc := range root.all("ExchangedDocument") {
		if normalizeSpace(doc.child("TypeCode").rawText()) == "381" {
			isCredit = true
		}
	}
	hasPaymentMeans := false
	for _, st := range settlements {
		if len(st.all("SpecifiedTradeSettlementPaymentMeans")) > 0 {
			hasPaymentMeans = true
		}
	}
	for _, st := range settlements {
		for _, sum := range st.all("SpecifiedTradeSettlementHeaderMonetarySummation") {
			if hasPaymentMeans {
				continue
			}
			amt := sum.child("DuePayableAmount")
			if amt != nil {
				due, ok := parseAmount(amt.text)
				if !ok {
					continue
				}
				if (!isCredit && due <= 0) || (isCredit && due >= 0) {
					continue
				}
			}
			e.add("NL-R-007", "For suppliers in the Netherlands, Payment instructions (BG-16) MUST be provided when "+
				"the payment is from the customer to the supplier")
		}
	}
	// NL-R-008, context ram:SpecifiedTradeSettlementPaymentMeans.
	if customer {
		for _, st := range settlements {
			for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
				code := normalizeSpace(pm.child("TypeCode").rawText())
				if !peppolDutchPaymentMeans[code] {
					e.addf("NL-R-008", "For suppliers in the Netherlands invoicing a Dutch customer, the Payment means "+
						"type code (BT-81=%q) MUST be one of 30, 48, 49, 57, 58 or 59", code)
				}
			}
		}
	}
	// NL-R-009, context ram:IncludedSupplyChainTradeLineItem/
	// ram:SpecifiedLineTradeAgreement/ram:BuyerOrderReferencedDocument/ram:LineID.
	if len(nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement",
		"BuyerOrderReferencedDocument", "IssuerAssignedID")) > 0 {
		return
	}
	for _, li := range nodesAt(root, "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem") {
		for range nodesAt(li, "SpecifiedLineTradeAgreement", "BuyerOrderReferencedDocument", "LineID") {
			e.add("NL-R-009", "For suppliers in the Netherlands, an Invoice line order line reference (BT-132) "+
				"requires a Purchase order reference (BT-13) on document level")
		}
	}
}

// peppolDutchLegalScheme is NL-R-003/005's scheme test:
//
//	contains(concat(' ', string-join(@schemeID, ' '), ' '), ' 0106 ') or
//	contains(concat(' ', string-join(@schemeID, ' '), ' '), ' 0190 ')
//
// KVK (the Dutch chamber-of-commerce register) or OIN (the government
// organisation identifier). An absent attribute joins to "" and matches neither.
func peppolDutchLegalScheme(scheme string) bool {
	padded := " " + scheme + " "
	return strings.Contains(padded, " 0106 ") || strings.Contains(padded, " 0190 ")
}

// peppolDutchPaymentMeans is NL-R-008's permitted set of BT-81 values.
var peppolDutchPaymentMeans = map[string]bool{
	"30": true, "48": true, "49": true, "57": true, "58": true, "59": true,
}

// peppolDutchCreditNoteTypes is the code list NL-R-001's CII context draws on,
// `tokenize('81 83 381 396 532', '\s')` — the UNTDID 1001 codes that make a CII
// document a credit note. The UBL binding needs no such list because it has a
// credit-note root element.
var peppolDutchCreditNoteTypes = map[string]bool{
	"81": true, "83": true, "381": true, "396": true, "532": true,
}

// ---------------------------------------------------------------------------
// Denmark
// ---------------------------------------------------------------------------

// peppolDanishPaymentMeans is DK-R-005's permitted set of BT-81 values, from
// `contains(' 1 10 31 42 48 49 50 58 59 93 97 ', concat(' ', <code>, ' '))`.
var peppolDanishPaymentMeans = map[string]bool{
	"1": true, "10": true, "31": true, "42": true, "48": true, "49": true,
	"50": true, "58": true, "59": true, "93": true, "97": true,
}

// peppolDanishUNSPSC is DK-R-003's permitted UNSPSC versions.
var peppolDanishUNSPSC = map[string]bool{
	"19.05.01": true, "19.0501": true, "26.08.01": true, "26.0801": true,
}

// peppolDanishUBLRules is the Danish pattern of the UBL binding: 14 rules across
// six contexts, gated on two pattern variables.
//
//	$DKSupplierCountry = concat(cn:CreditNote/…/cac:PostalAddress/cac:Country/cbc:IdentificationCode,
//	                            ubl:Invoice/…/cac:PostalAddress/cac:Country/cbc:IdentificationCode)
//	$DKCustomerCountry   the same over cac:AccountingCustomerParty
//
// The concat is how OpenPEPPOL writes "whichever root this document has"; only one
// arm can be non-empty. The comparison is `= 'DK'` against the untrimmed string
// value, so a padded country code does not select the family — transcribed as
// written rather than normalized, because the German and Dutch patterns *do*
// normalize and the difference is the artefact's.
//
// Nine of the fourteen are domestic-only (both parties Danish); DK-R-002, DK-R-014
// and DK-R-016 need only a Danish seller. And the payment-means rules are bound to
// `ubl:Invoice[…]/cac:PaymentMeans` with no credit-note arm, so a Danish credit
// note's payment means answers to none of DK-R-005..011. That asymmetry is
// OpenPEPPOL's; reading the family as "all Danish documents" would report seven
// rules against credit notes the authority does not check.
func peppolDanishUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLPostalCountry(root, "AccountingSupplierParty") != "DK" {
		return
	}
	domestic := peppolUBLPostalCountry(root, "AccountingCustomerParty") == "DK"
	seller := root.child("AccountingSupplierParty", "Party")
	customer := root.child("AccountingCustomerParty", "Party")

	// DK-R-002: normalize-space(…/cac:PartyLegalEntity/cbc:CompanyID/text()) != ''
	sellerLegal := seller.child("PartyLegalEntity", "CompanyID")
	if normalizeSpace(sellerLegal.rawText()) == "" {
		e.add("DK-R-002", "Danish suppliers MUST provide a legal registration identifier (BT-30, the CVR number)")
	}
	// DK-R-014: not(boolean(<id>) and normalize-space(<id>/@schemeID) != '0184')
	if sellerLegal != nil && normalizeSpace(sellerLegal.attr("schemeID")) != "0184" {
		e.add("DK-R-014", "For Danish suppliers the Seller legal registration identifier (BT-30) MUST declare "+
			"scheme 0184 (DK CVR)")
	}
	// DK-R-016: not((boolean(/cn:CreditNote) and $DKCustomerCountry = 'DK') and
	//               (number(cac:LegalMonetaryTotal/cbc:PayableAmount/text()) < 0))
	if root.name == "CreditNote" && domestic {
		if due, ok := parseAmount(root.child("LegalMonetaryTotal", "PayableAmount").rawText()); ok && due < 0 {
			e.addf("DK-R-016", "For Danish suppliers a credit note's Amount due for payment (BT-115=%s) MUST NOT be "+
				"negative", normalizeSpace(root.child("LegalMonetaryTotal", "PayableAmount").rawText()))
		}
	}
	if !domestic {
		return
	}
	// DK-R-013, context …/cac:Party/cac:PartyIdentification on either party:
	//   not(boolean(cbc:ID) and normalize-space(cbc:ID/@schemeID) = '')
	//
	// An identifier with no @schemeID at all normalizes to '' and is reported, which
	// is the case the rule exists for.
	for _, party := range []*ciiNode{seller, customer} {
		for _, pi := range party.orNil().all("PartyIdentification") {
			if id := pi.child("ID"); id != nil && normalizeSpace(id.attr("schemeID")) == "" {
				e.add("DK-R-013", "For Danish suppliers a Party identification identifier (BT-29/BT-46) MUST declare "+
					"a scheme identifier")
			}
		}
	}
	// DK-R-017, context …/cac:AccountingCustomerParty/cac:Party.
	if customerLegal := customer.child("PartyLegalEntity", "CompanyID"); customerLegal != nil &&
		normalizeSpace(customerLegal.attr("schemeID")) != "0184" {
		e.add("DK-R-017", "For Danish customers the Buyer legal registration identifier (BT-47) MUST declare "+
			"scheme 0184 (DK CVR)")
	}
	// DK-R-003, context …/cac:InvoiceLine | …/cac:CreditNoteLine.
	lineName := "InvoiceLine"
	if root.name == "CreditNote" {
		lineName = "CreditNoteLine"
	}
	for _, li := range root.all(lineName) {
		peppolDanishClassification(e, nodesAt(li, "Item", "CommodityClassification", "ItemClassificationCode"))
	}
	// DK-R-004, context cac:AllowanceCharge[$DKSupplierCountry = 'DK' and
	// $DKCustomerCountry = 'DK'] — every allowance or charge in the document, at any
	// level, unlike the CII binding where the whole rule is one document-level test.
	for _, ac := range root.findAll("AllowanceCharge") {
		if !peppolAnyChildValue(ac, "AllowanceChargeReasonCode", "ZZZ") {
			continue
		}
		if peppolDanishTaxReasonOK(ac.child("AllowanceChargeReason").rawText()) {
			continue
		}
		e.add("DK-R-004", "For Danish domestic invoices a non-VAT tax (AllowanceChargeReasonCode ZZZ) MUST state "+
			"the four-digit tax category or a value containing an internal '#' in the reason (BT-97/BT-104)")
	}
	// DK-R-005..011, context ubl:Invoice[both]/cac:PaymentMeans.
	if root.name != "Invoice" {
		return
	}
	for _, pm := range root.all("PaymentMeans") {
		code := pm.child("PaymentMeansCode").rawText()
		if !peppolDanishPaymentMeans[code] {
			e.addf("DK-R-005", "For Danish suppliers the Payment means type code (BT-81=%q) MUST be one of "+
				"1, 10, 31, 42, 48, 49, 50, 58, 59, 93 or 97", normalizeSpace(code))
		}
		account := pm.child("PayeeFinancialAccount")
		paymentID := pm.child("PaymentID").rawText()
		switch code {
		case "31", "42":
			// DK-R-006: both the account (BT-84) and the branch identifier (BT-85).
			if normalizeSpace(account.child("ID").rawText()) == "" ||
				normalizeSpace(account.child("FinancialInstitutionBranch", "ID").rawText()) == "" {
				e.add("DK-R-006", "For Danish suppliers a payment means of 31 or 42 requires both the Payment account "+
					"identifier (BT-84) and the Payment service provider identifier (BT-85)")
			}
		case "49":
			// DK-R-007: the mandate reference (BT-89) and the debited account (BT-91).
			mandate := pm.child("PaymentMandate")
			if normalizeSpace(mandate.child("ID").rawText()) == "" ||
				normalizeSpace(mandate.child("PayerFinancialAccount", "ID").rawText()) == "" {
				e.add("DK-R-007", "For Danish suppliers a payment means of 49 requires both the Mandate reference "+
					"identifier (BT-89) and the Debited account identifier (BT-91)")
			}
		case "50":
			// DK-R-008: a Giro card code prefix and a 7-or-8-digit Giro account.
			if !peppolDanishCardCode(paymentID, "01#", "04#", "15#") ||
				!peppolDanishGiroAccount(account.child("ID").rawText(), 7, 8) {
				e.add("DK-R-008", "For Danish suppliers a payment means of 50 (Giro) requires a Payment reference "+
					"(BT-83) beginning 01#, 04# or 15# and a 7 or 8 digit Giro account (BT-84)")
			}
			// DK-R-009: those two prefixes carry a 16-digit instruction identifier.
			if peppolDanishCardCode(paymentID, "04#", "15#") && len([]rune(paymentID)) != 19 {
				e.add("DK-R-009", "For Danish suppliers a Payment reference (BT-83) prefixed 04# or 15# MUST be "+
					"nineteen characters — the prefix and a sixteen digit instruction identifier")
			}
		case "93":
			// DK-R-010: an FIK card code prefix and an 8-character account.
			if !peppolDanishCardCode(paymentID, "71#", "73#", "75#") ||
				len([]rune(account.child("ID").rawText())) != 8 {
				e.add("DK-R-010", "For Danish suppliers a payment means of 93 (FIK) requires a Payment reference "+
					"(BT-83) beginning 71#, 73# or 75# and an eight character account (BT-84)")
			}
			// DK-R-011: two of those prefixes carry a 15-or-16-digit identifier.
			if peppolDanishCardCode(paymentID, "71#", "75#") {
				if n := len([]rune(paymentID)); n != 18 && n != 19 {
					e.add("DK-R-011", "For Danish suppliers a Payment reference (BT-83) prefixed 71# or 75# MUST be "+
						"eighteen or nineteen characters")
				}
			}
		}
	}
}

// peppolDanishCIIRules is the Danish pattern of the CII binding — 13 rules, all of
// DK-R-017 absent.
//
// Two differences from the UBL binding are structural rather than incidental:
// DK-R-004 and DK-R-013 are single document-level tests here, where UBL binds them
// to each allowance/charge and each party identification; and DK-R-016 tells a
// credit note from an invoice by BT-3 = '381', CII having one root for both.
//
// A third is worth naming because it looks like a transcription error and is not.
// DK-R-007's XPath reads `ram:SpecifiedTradePaymentTerms/ram:DirectDebitMandateID`
// as a *child* of the payment means, where the CII schema puts payment terms beside
// it under the settlement. OpenPEPPOL's own nine fixtures for the rule are written
// the same way — the passing one nests SpecifiedTradePaymentTerms inside
// SpecifiedTradeSettlementPaymentMeans — so this is the artefact's reading of its
// own rule, and the XPath is transcribed rather than corrected. The consequence for
// a schema-valid Danish CII invoice using payment means 49 is that DK-R-007 fires,
// and a reference Peppol validation reports it too.
func peppolDanishCIIRules(e *peppolEval, root *ciiNode) {
	if peppolCIIPostalCountry(root, "SellerTradeParty") != "DK" {
		return
	}
	domestic := peppolCIIPostalCountry(root, "BuyerTradeParty") == "DK"
	seller := peppolCIIParty(root, "SellerTradeParty")
	buyer := peppolCIIParty(root, "BuyerTradeParty")
	settlements := nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement")

	// DK-R-002: normalize-space(…/ram:SpecifiedLegalOrganization/ram:ID/text()) != ''
	sellerLegal := seller.child("SpecifiedLegalOrganization", "ID")
	if normalizeSpace(sellerLegal.rawText()) == "" {
		e.add("DK-R-002", "Danish suppliers MUST provide a legal registration identifier (BT-30)")
	}
	// DK-R-014.
	if sellerLegal != nil && normalizeSpace(sellerLegal.attr("schemeID")) != "0184" {
		e.add("DK-R-014", "For Danish suppliers the Seller legal registration identifier (BT-30) MUST declare "+
			"scheme 0184 (DK CVR)")
	}
	if domestic {
		// DK-R-013: either party's ram:GlobalID present with an empty @schemeID. It is
		// one test over two named elements here, not a per-node rule.
		for _, party := range []*ciiNode{seller, buyer} {
			if id := party.orNil().child("GlobalID"); id != nil && normalizeSpace(id.attr("schemeID")) == "" {
				e.add("DK-R-013", "For Danish suppliers a Party identification identifier (BT-29/BT-46) MUST declare "+
					"a scheme identifier")
				break
			}
		}
		// DK-R-016: BT-3 = '381' and a negative BT-115.
		credit := false
		for _, doc := range root.all("ExchangedDocument") {
			if normalizeSpace(doc.child("TypeCode").rawText()) == "381" {
				credit = true
			}
		}
		if credit {
			for _, st := range settlements {
				amt := st.child("SpecifiedTradeSettlementHeaderMonetarySummation", "DuePayableAmount")
				if due, ok := parseAmount(amt.rawText()); ok && due < 0 {
					e.addf("DK-R-016", "For Danish suppliers a credit note's Amount due for payment (BT-115=%s) MUST "+
						"NOT be negative", normalizeSpace(amt.rawText()))
				}
			}
		}
		// DK-R-004: one document-level test over the header allowances and charges.
		for _, st := range settlements {
			charges := st.all("SpecifiedTradeAllowanceCharge")
			zzz := false
			reason := ""
			for _, ac := range charges {
				if peppolAnyChildValue(ac, "ReasonCode", "ZZZ") {
					zzz = true
				}
				if reason == "" {
					reason = ac.child("Reason").rawText()
				}
			}
			if zzz && !peppolDanishTaxReasonOK(reason) {
				e.add("DK-R-004", "For Danish domestic invoices a non-VAT tax (ReasonCode ZZZ) MUST state the "+
					"four-digit tax category or a value containing an internal '#' in the reason (BT-97/BT-104)")
			}
		}
		// DK-R-003, context ram:IncludedSupplyChainTradeLineItem.
		for _, li := range nodesAt(root, "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem") {
			peppolDanishClassification(e, nodesAt(li, "SpecifiedTradeProduct", "DesignatedProductClassification", "ClassCode"))
		}
		// DK-R-005..011, context ram:SpecifiedTradeSettlementPaymentMeans.
		for _, st := range settlements {
			// `../ram:PaymentReference` and `../ram:CreditorReferenceID` are the
			// settlement's, which is the payment means' parent.
			paymentRef := st.child("PaymentReference").rawText()
			creditorRef := st.child("CreditorReferenceID").rawText()
			for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
				code := pm.child("TypeCode").rawText()
				if !peppolDanishPaymentMeans[code] {
					e.addf("DK-R-005", "For Danish suppliers the Payment means type code (BT-81=%q) MUST be one of "+
						"1, 10, 31, 42, 48, 49, 50, 58, 59, 93 or 97", normalizeSpace(code))
				}
				iban := pm.child("PayeePartyCreditorFinancialAccount", "IBANID").rawText()
				switch code {
				case "31", "42":
					if normalizeSpace(iban) == "" ||
						normalizeSpace(pm.child("PayeeSpecifiedCreditorFinancialInstitution", "BICID").rawText()) == "" {
						e.add("DK-R-006", "For Danish suppliers a payment means of 31 or 42 requires both the Payment "+
							"account identifier (BT-84) and the Payment service provider identifier (BT-86)")
					}
				case "49":
					if normalizeSpace(creditorRef) == "" ||
						normalizeSpace(pm.child("SpecifiedTradePaymentTerms", "DirectDebitMandateID").rawText()) == "" {
						e.add("DK-R-007", "For Danish suppliers a payment means of 49 requires both the Mandate "+
							"reference identifier (BT-89) and the Bank assigned creditor identifier (BT-90)")
					}
				case "50":
					// substring(../ram:PaymentReference, 0, 4) is the first three characters:
					// XPath keeps the positions p where 0 <= p < 4, and there is no position 0.
					if !peppolDanishCardCode(paymentRef, "01#", "04#", "15#") || len([]rune(iban)) != 7 {
						e.add("DK-R-008", "For Danish suppliers a payment means of 50 (Giro) requires a Payment "+
							"reference (BT-83) beginning 01#, 04# or 15# and a seven character Giro account (BT-84)")
					}
					if peppolDanishCardCode(paymentRef, "04#", "15#") && len([]rune(paymentRef)) != 19 {
						e.add("DK-R-009", "For Danish suppliers a Payment reference (BT-83) prefixed 04# or 15# MUST "+
							"be nineteen characters — the prefix and a sixteen digit instruction identifier")
					}
				case "93":
					if !peppolDanishCardCode(paymentRef, "71#", "73#", "75#") || len([]rune(iban)) != 8 {
						e.add("DK-R-010", "For Danish suppliers a payment means of 93 (FIK) requires a Payment "+
							"reference (BT-83) beginning 71#, 73# or 75# and an eight character account (BT-84)")
					}
					if peppolDanishCardCode(paymentRef, "71#", "75#") {
						if n := len([]rune(paymentRef)); n != 18 && n != 19 {
							e.add("DK-R-011", "For Danish suppliers a Payment reference (BT-83) prefixed 71# or 75# "+
								"MUST be eighteen or nineteen characters")
						}
					}
				}
			}
		}
	}
}

// peppolDanishClassification is DK-R-003 over one line's item classification codes:
//
//	not((…/@listID = 'TST') and not((…/@listVersionID = '19.05.01') or … ))
//
// Both halves are node-set comparisons, so the rule passes when *any* code carries
// a permitted version, whichever code carried the TST list identifier.
func peppolDanishClassification(e *peppolEval, codes []*ciiNode) {
	tst, version := false, false
	for _, c := range codes {
		if c.attr("listID") == "TST" {
			tst = true
		}
		if peppolDanishUNSPSC[c.attr("listVersionID")] {
			version = true
		}
	}
	if tst && !version {
		e.add("DK-R-003", "If a Danish supplier provides an Item classification identifier (BT-158) it should use "+
			"UNSPSC version 19.05.01 or 26.08.01")
	}
}

// peppolDanishTaxReasonOK is DK-R-004's inner disjunction over the
// allowance/charge reason (BT-97/BT-104):
//
//	(string-length(normalize-space(<reason>/text())) = 4 and number(<reason>) >= 0
//	   and number(<reason>) <= 9999)
//	or (<reason> and contains(<reason>, '#') and not(starts-with(<reason>, '#'))
//	   and not(ends-with(<reason>, '#')))
//
// Either a four-character Danish tax category in 0000..9999, or a value carrying a
// '#' that is neither its first nor its last character.
//
// The CII wording of the first arm is `number(<reason> <= 9999)` — a comparison
// wrapped in number(), so it yields 1 or 0 and its effective boolean value is the
// comparison's. The two bindings therefore agree despite the typo, and one function
// serves both.
func peppolDanishTaxReasonOK(reason string) bool {
	trimmed := normalizeSpace(reason)
	if len([]rune(trimmed)) == 4 {
		if v, ok := parseAmount(trimmed); ok && v >= 0 && v <= 9999 {
			return true
		}
	}
	return reason != "" && strings.Contains(reason, "#") &&
		!strings.HasPrefix(reason, "#") && !strings.HasSuffix(reason, "#")
}

// peppolDanishCardCode is `substring(<ref>, 1, 3) = '01#' or …` — the Danish
// "kortartkode" prefix test DK-R-008..011 share. The CII binding writes it
// substring(<ref>, 0, 4), which selects the same three characters.
func peppolDanishCardCode(ref string, codes ...string) bool {
	prefix := peppolSubstring(ref, 1, 3)
	for _, c := range codes {
		if prefix == c {
			return true
		}
	}
	return false
}

// peppolDanishGiroAccount is DK-R-008's `matches(<account>, '^[0-9]{7,8}$')`.
func peppolDanishGiroAccount(account string, lo, hi int) bool {
	n := len(account)
	return n >= lo && n <= hi && peppolAllDigits(account)
}

// peppolAnyChildValue is a node-set comparison against a literal:
// `<parent>/<name> = <value>`, which is true when any such child's string value
// matches. The value is untrimmed, as the artefact's comparisons are.
func peppolAnyChildValue(parent *ciiNode, name, value string) bool {
	for _, c := range parent.orNil().all(name) {
		if c.rawText() == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Sweden
// ---------------------------------------------------------------------------

// peppolSwedishVATRates is SE-R-006's permitted set: Sweden's three standard VAT
// rates.
var peppolSwedishVATRates = []float64{25, 12, 6}

// peppolSwedishUBLRules is the Swedish pattern of the UBL binding: 13 rules over
// eight contexts.
//
// Sweden is the one family with no <let> variable at all. Every rule's context
// carries its own country test inline, and they are not the same test:
//
//	SE-R-001/002/006  the seller's postal country is SE *and* its VAT identifier
//	                  begins SE
//	SE-R-003/004/013  the seller's postal country is SE and a legal registration
//	                  identifier is present — the VAT prefix is not consulted
//	SE-R-005          the seller's postal country is SE and a legal registration
//	                  identifier is present
//	SE-R-007..011     the seller's postal country is SE
//	SE-R-012          both parties' postal country is SE
//
// So a Swedish company invoicing under a foreign VAT registration answers to the
// organisation-number rules and not to the VAT-number ones. Collapsing the five
// gates into one would have widened three of them.
func peppolSwedishUBLRules(e *peppolEval, root *ciiNode) {
	sellerAddrSE, sellerVATSE := false, false
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		if peppolAnyChildValue(party.child("PostalAddress", "Country"), "IdentificationCode", "SE") {
			sellerAddrSE = true
			if peppolSubstring(peppolUBLVATCompanyID(party), 1, 2) == "SE" {
				sellerVATSE = true
			}
		}
	}
	if !sellerAddrSE {
		return
	}
	buyerAddrSE := false
	for _, party := range nodesAt(root, "AccountingCustomerParty", "Party") {
		if peppolAnyChildValue(party.child("PostalAddress", "Country"), "IdentificationCode", "SE") {
			buyerAddrSE = true
		}
	}
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		if !peppolAnyChildValue(party.child("PostalAddress", "Country"), "IdentificationCode", "SE") {
			continue
		}
		vatID := peppolUBLVATCompanyID(party)
		if peppolSubstring(vatID, 1, 2) == "SE" {
			// SE-R-001: string-length(normalize-space(<vat id>)) = 14
			if len([]rune(normalizeSpace(vatID))) != 14 {
				e.addf("SE-R-001", "A Swedish Seller VAT identifier (BT-31=%q) MUST be fourteen characters",
					normalizeSpace(vatID))
			}
			// SE-R-002: string(number(substring(<vat id>, 3, 12))) != 'NaN'
			if !peppolIsXPathNumber(peppolSubstring(vatID, 3, 12)) {
				e.addf("SE-R-002", "A Swedish Seller VAT identifier (BT-31=%q) MUST carry twelve numeric characters "+
					"after the country prefix", normalizeSpace(vatID))
			}
		}
		// SE-R-003/004/013, context cac:PartyLegalEntity[<seller postal SE> and
		// cbc:CompanyID]. The VAT prefix is not part of this gate.
		for _, le := range party.all("PartyLegalEntity") {
			id := le.child("CompanyID")
			if id == nil {
				continue
			}
			raw, trimmed := id.rawText(), normalizeSpace(id.rawText())
			if !peppolIsXPathNumber(raw) {
				e.addf("SE-R-003", "A Swedish organisation number (BT-30=%q) MUST be numeric", trimmed)
			}
			if len([]rune(trimmed)) != 10 {
				e.addf("SE-R-004", "A Swedish organisation number (BT-30=%q) MUST be ten characters", trimmed)
			}
			if !peppolSwedishOrgNr(trimmed) {
				e.addf("SE-R-013", "The last digit of a Swedish organisation number (BT-30=%q) MUST satisfy the Luhn "+
					"check", trimmed)
			}
		}
		// SE-R-005, context cac:PartyTaxScheme[normalize-space(upper-case(
		// cac:TaxScheme/cbc:ID)) != 'VAT']/cbc:CompanyID, gated on the party having a
		// legal registration identifier. The scheme test is upper-cased here and not in
		// the Italian rule that reads the same element.
		if len(nodesAt(party, "PartyLegalEntity", "CompanyID")) == 0 {
			continue
		}
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(strings.ToUpper(ts.child("TaxScheme", "ID").rawText())) == "VAT" {
				continue
			}
			for _, id := range ts.all("CompanyID") {
				if normalizeSpace(strings.ToUpper(id.rawText())) != "GODKÄND FÖR F-SKATT" {
					e.addf("SE-R-005", "For Swedish suppliers the Seller tax registration identifier (BT-32=%q) MUST "+
						"read \"Godkänd för F-skatt\"", normalizeSpace(id.rawText()))
				}
			}
		}
	}
	// SE-R-006, context //cac:TaxCategory[<seller SE and VAT SE> and cbc:ID = 'S'] |
	// //cac:ClassifiedTaxCategory[<same>].
	//
	// The `and cbc:ID = 'S'` predicate is the UBL binding's alone — the CII wording
	// has no category-code test, so the two are not the same rule and are written
	// separately below.
	if sellerVATSE {
		for _, name := range []string{"TaxCategory", "ClassifiedTaxCategory"} {
			for _, tc := range root.findAll(name) {
				if !peppolAnyChildValue(tc, "ID", "S") {
					continue
				}
				peppolSwedishRate(e, tc.child("Percent"))
			}
		}
	}
	// SE-R-007..012, context //cac:PaymentMeans[…].
	for _, pm := range root.findAll("PaymentMeans") {
		code := pm.child("PaymentMeansCode")
		normalized := normalizeSpace(code.rawText())
		account := pm.child("PayeeFinancialAccount")
		branch := normalizeSpace(account.child("FinancialInstitutionBranch", "ID").rawText())
		if normalized == "30" {
			// The account identifier is the context of SE-R-007..010, so a payment means
			// with no cac:PayeeFinancialAccount/cbc:ID at all is out of scope rather than
			// reported.
			for _, id := range account.orNil().all("ID") {
				value := normalizeSpace(id.rawText())
				switch branch {
				case "SE:PLUSGIRO":
					if !peppolIsXPathNumber(value) {
						e.addf("SE-R-007", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be numeric", value)
					}
					if n := len([]rune(value)); n < 2 || n > 8 {
						e.addf("SE-R-010", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be two to eight "+
							"characters", value)
					}
				case "SE:BANKGIRO":
					if !peppolIsXPathNumber(value) {
						e.addf("SE-R-008", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be numeric", value)
					}
					if n := len([]rune(value)); n != 7 && n != 8 {
						e.addf("SE-R-009", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be seven or eight "+
							"characters", value)
					}
				}
			}
		}
		// SE-R-011 / SE-R-012 assert false(), so their context is the whole rule. The
		// code comparison is against the untrimmed string value here — `cbc:PaymentMeansCode
		// = normalize-space('50')` normalizes the literal, not the element — while
		// SE-R-007..010 above normalize the element. Both are transcribed as written.
		raw := code.rawText()
		if raw == "50" || raw == "56" {
			e.add("SE-R-011", "For Swedish suppliers, Bankgiro and Plusgiro are indicated with Payment means type "+
				"code 30 and a financial institution branch of SE:BANKGIRO or SE:PLUSGIRO, not with 50 or 56")
		}
		if buyerAddrSE && raw == "31" {
			e.add("SE-R-012", "For domestic Swedish transactions, a credit transfer should be indicated with Payment "+
				"means type code 30")
		}
	}
}

// peppolSwedishCIIRules is the Swedish pattern of the CII binding.
//
// SE-R-006 is the one rule whose two bindings are not the same rule. The UBL
// context is `//cac:TaxCategory[… and cbc:ID = 'S']`, restricted to the standard-
// rate category; the CII context is `//ram:ApplicableTradeTax[…] |
// //ram:CategoryTradeTax[…]` with no category predicate at all, so it applies to
// every tax group of a Swedish seller whatever its category. That is why the two
// are written out separately rather than sharing a body: a zero-rated CII line
// trips it and the UBL equivalent does not.
func peppolSwedishCIIRules(e *peppolEval, root *ciiNode) {
	seller := peppolCIIParty(root, "SellerTradeParty")
	if !peppolAnyChildValue(seller.child("PostalTradeAddress"), "CountryID", "SE") {
		return
	}
	buyerSE := peppolAnyChildValue(peppolCIIParty(root, "BuyerTradeParty").child("PostalTradeAddress"), "CountryID", "SE")
	vatID := peppolCIIVATID(seller)
	if peppolSubstring(vatID, 1, 2) == "SE" {
		if len([]rune(normalizeSpace(vatID))) != 14 {
			e.addf("SE-R-001", "A Swedish Seller VAT identifier (BT-31=%q) MUST be fourteen characters",
				normalizeSpace(vatID))
		}
		if !peppolIsXPathNumber(peppolSubstring(vatID, 3, 12)) {
			e.addf("SE-R-002", "A Swedish Seller VAT identifier (BT-31=%q) MUST carry twelve numeric characters "+
				"after the country prefix", normalizeSpace(vatID))
		}
	}
	// SE-R-003/004/013, context ram:SpecifiedLegalOrganization[… and ram:ID].
	for _, org := range seller.orNil().all("SpecifiedLegalOrganization") {
		id := org.child("ID")
		if id == nil {
			continue
		}
		raw, trimmed := id.rawText(), normalizeSpace(id.rawText())
		if !peppolIsXPathNumber(raw) {
			e.addf("SE-R-003", "A Swedish organisation number (BT-30=%q) MUST be numeric", trimmed)
		}
		if len([]rune(trimmed)) != 10 {
			e.addf("SE-R-004", "A Swedish organisation number (BT-30=%q) MUST be ten characters", trimmed)
		}
		if !peppolSwedishOrgNr(trimmed) {
			e.addf("SE-R-013", "The last digit of a Swedish organisation number (BT-30=%q) MUST satisfy the Luhn "+
				"check", trimmed)
		}
	}
	// SE-R-005, context ram:SpecifiedTaxRegistration/ram:ID[@schemeID = 'FC'], gated
	// on a legal registration identifier being present. The CII binding selects the
	// registration by scheme where UBL selects it by "not VAT".
	if len(nodesAt(seller, "SpecifiedLegalOrganization", "ID")) > 0 {
		for _, reg := range seller.orNil().all("SpecifiedTaxRegistration") {
			for _, id := range reg.all("ID") {
				if id.attr("schemeID") != "FC" {
					continue
				}
				if normalizeSpace(strings.ToUpper(id.rawText())) != "GODKÄND FÖR F-SKATT" {
					e.addf("SE-R-005", "For Swedish suppliers the Seller tax registration identifier (BT-32=%q) MUST "+
						"read \"Godkänd för F-skatt\"", normalizeSpace(id.rawText()))
				}
			}
		}
	}
	// SE-R-006 — every tax group, no category predicate.
	if peppolSubstring(vatID, 1, 2) == "SE" {
		for _, name := range []string{"ApplicableTradeTax", "CategoryTradeTax"} {
			for _, tax := range root.findAll(name) {
				peppolSwedishRate(e, tax.child("RateApplicablePercent"))
			}
		}
	}
	// SE-R-007..012, context ram:SpecifiedTradeSettlementPaymentMeans[…]. The CII
	// binding normalizes the type code in all four contexts; the UBL binding
	// normalizes it in two of them and not in the other two.
	for _, st := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement") {
		for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
			code := normalizeSpace(pm.child("TypeCode").rawText())
			bic := normalizeSpace(pm.child("PayeeSpecifiedCreditorFinancialInstitution", "BICID").rawText())
			if code == "30" {
				for _, id := range pm.child("PayeePartyCreditorFinancialAccount").orNil().all("ProprietaryID") {
					value := normalizeSpace(id.rawText())
					switch bic {
					case "SE:PLUSGIRO":
						if !peppolIsXPathNumber(value) {
							e.addf("SE-R-007", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be numeric", value)
						}
						if n := len([]rune(value)); n < 2 || n > 8 {
							e.addf("SE-R-010", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be two to eight "+
								"characters", value)
						}
					case "SE:BANKGIRO":
						if !peppolIsXPathNumber(value) {
							e.addf("SE-R-008", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be numeric", value)
						}
						if n := len([]rune(value)); n != 7 && n != 8 {
							e.addf("SE-R-009", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be seven or "+
								"eight characters", value)
						}
					}
				}
			}
			if code == "50" || code == "56" {
				e.add("SE-R-011", "For Swedish suppliers, Bankgiro and Plusgiro are indicated with Payment means type "+
					"code 30 and a creditor financial institution of SE:BANKGIRO or SE:PLUSGIRO, not with 50 or 56")
			}
			if buyerSE && code == "31" {
				e.add("SE-R-012", "For domestic Swedish transactions, a credit transfer should be indicated with "+
					"Payment means type code 30")
			}
		}
	}
}

// peppolSwedishRate is SE-R-006's assertion:
//
//	number(<rate>) = 25 or number(<rate>) = 12 or number(<rate>) = 6
//
// An absent rate makes number() NaN, which equals nothing, so the rule reports it.
// That is the artefact's arithmetic and not a reading of intent.
func peppolSwedishRate(e *peppolEval, rate *ciiNode) {
	value, ok := parseAmount(rate.rawText())
	if ok {
		for _, allowed := range peppolSwedishVATRates {
			if value == allowed {
				return
			}
		}
	}
	e.addf("SE-R-006", "For Swedish suppliers the VAT category rate (BT-119=%q) MUST be 6, 12 or 25",
		normalizeSpace(rate.rawText()))
}

// peppolIsXPathNumber is `string(number($v)) != 'NaN'`: whether XPath's number()
// yields a number for this string. fn:number is xs:double's cast, so it takes
// surrounding whitespace, a sign and an exponent, and it maps the literal "NaN" to
// NaN — which the test then rejects.
func peppolIsXPathNumber(v string) bool {
	value, ok := parseAmount(v)
	return ok && !math.IsNaN(value)
}
