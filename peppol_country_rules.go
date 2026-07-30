package formalis

import "strings"

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
	// Italy — 4 fatal, both bindings.
	"IT-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-004": {peppolUBL | peppolCII, SeverityFatal},
	// Norway — 1 fatal, 1 advisory, both bindings.
	"NO-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"NO-R-002": {peppolUBL | peppolCII, SeverityWarning},
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
