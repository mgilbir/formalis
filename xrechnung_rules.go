package formalis

import (
	"regexp"
	"strings"
)

// This file evaluates the XRechnung (KoSIT) rules that are statements about a
// document *tree* rather than about the syntax-neutral model: the payment-means
// group rules (BR-DE-19/20/23/24/25/30/31) and the settlement-discount text
// format (BR-DE-18). xrechnung.go holds the entry point and the mandatory-term
// rules that the shared model already answers.
//
// Fidelity. Every rule below is transcribed from the assertion KoSIT publishes in
// testdata/xrechnung/schematron/src/validation/schematron/ubl/XRechnung-UBL-validation.sch
// and .../cii/XRechnung-CII-validation.sch, with the variables of
// .../common.sch resolved, and each cites its XPath. That is not ceremony: PR 14
// found several CEN rule titles that describe a different rule than their XPath,
// and KoSIT's own titles have the same problem. BR-DE-30's message asks for BT-90,
// the bank assigned creditor identifier, and its XPath is
// "((BT-89 or BT-91) and BT-90) or no BG-19" — so a creditor identifier with
// neither a mandate reference nor a debited account fails the rule that asks for
// it. The XPath is the rule.
//
// Both syntaxes, separately. KoSIT publishes two Schematron files and they are
// not translations of each other. Where an identifier exists in both, its XPath
// can still test a different thing: BG-19 ("DIRECT DEBIT") is one cac:PaymentMandate element in
// UBL and, in CII, the *semantic* group KoSIT reconstructs from "any of BT-89,
// BT-90, BT-91 is present", which is why BR-DE-30 and BR-DE-31 have two bodies
// here rather than one over the model.
//
// Severity is quoted, never chosen: xrechnungFlags in xrechnung.go carries
// KoSIT's flag for every identifier this package evaluates, and xrAdder reads it.

// validateXRechnungTreeRules evaluates the tree-shaped half of the XRechnung
// rule set against the document as parsed, dispatching on the binding the
// invoice was expressed in.
func validateXRechnungTreeRules(r *run, p *parsed) []Violation {
	var out []Violation
	add := xrAdder(&out)
	root := p.root
	if p.inv.syntax == "CII" {
		xrCIIRules(r, root, add)
		return out
	}
	xrUBLRules(r, root, add)
	return out
}

// ---------------------------------------------------------------------------
// The UBL binding
// ---------------------------------------------------------------------------

// xrUBLRules evaluates the rules of the ubl-pattern whose context is a tree
// position rather than the whole document.
func xrUBLRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	// BR-DE-18, context /ubl:Invoice | /cn:CreditNote:
	//   every $line in cac:PaymentTerms/cbc:Note[1]/tokenize(., '(\r?\n)')
	//                  [starts-with(normalize-space(.), '#')]
	//   satisfies matches(normalize-space($line), $XR-SKONTO-REGEX) and
	//             matches(cac:PaymentTerms/cbc:Note[1]/tokenize(., '#.+#')[last()], '^\s*\n')
	xrSkontoRule(xrFirstNotes(root, "PaymentTerms", "Note"), add)

	// BR-DE-30, context /ubl:Invoice | /cn:CreditNote:
	//   not(cac:PaymentMeans/cac:PaymentMandate) or
	//   (cac:AccountingSupplierParty/cac:Party/cac:PartyIdentification/cbc:ID[@schemeID='SEPA']
	//    | cac:PayeeParty/cac:PartyIdentification/cbc:ID[@schemeID='SEPA'])
	//
	// BT-90 is a party identifier with the SEPA scheme, not the mandate's own
	// identifier: an invoice can carry a mandate reference and still be missing
	// the creditor identifier the mandate was issued under.
	mandates := nodesAt(root, "PaymentMeans", "PaymentMandate")
	if len(mandates) > 0 {
		sepa := false
		for _, id := range append(
			nodesAt(root, "AccountingSupplierParty", "Party", "PartyIdentification", "ID"),
			nodesAt(root, "PayeeParty", "PartyIdentification", "ID")...) {
			if id.attr("schemeID") == "SEPA" {
				sepa = true
			}
		}
		if !sepa {
			add("BR-DE-30", "A direct debit (BG-19) shall carry the Bank assigned creditor identifier (BT-90): "+
				"a Seller or Payee party identifier with the SEPA scheme")
		}
		// BR-DE-31: not(cac:PaymentMeans/cac:PaymentMandate) or
		//           (cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID)
		if len(nodesAt(root, "PaymentMeans", "PaymentMandate", "PayerFinancialAccount", "ID")) == 0 {
			add("BR-DE-31", "A direct debit (BG-19) shall carry the Debited account identifier (BT-91)")
		}
	}

	// The three payment-means groups. Each is one Schematron rule whose context is
	// a cac:PaymentMeans selected by its BT-81, so a document with several groups
	// is checked group by group and a group of one kind may not carry another
	// kind's account details.
	for _, pm := range root.all("PaymentMeans") {
		if r.stopped() {
			return
		}
		code := normalizeSpace(pm.str("PaymentMeansCode"))
		credit := pm.child("PayeeFinancialAccount") != nil
		card := pm.child("CardAccount") != nil
		mandate := pm.child("PaymentMandate") != nil
		switch {
		// context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = ('30','58')]
		case code == "30" || code == "58":
			// BR-DE-19: not(code = '58') or the IBAN check on
			// cac:PayeeFinancialAccount/cbc:ID. KoSIT flags it warning, and its own
			// comment records that code 30 was left out of the check on purpose.
			if code == "58" && !validIBAN(pm.str("PayeeFinancialAccount", "ID")) {
				add("BR-DE-19", "The Payment account identifier (BT-84) shall be a valid IBAN for a SEPA credit transfer")
			}
			if !credit {
				add("BR-DE-23-a", "A Payment means type code (BT-81) for a credit transfer (30, 58) requires the "+
					"Credit transfer group (BG-17)")
			}
			// BR-DE-23-b: not(cac:CardAccount) and not(cac:PaymentMandate)
			if card || mandate {
				add("BR-DE-23-b", "A Payment means type code (BT-81) for a credit transfer (30, 58) shall carry "+
					"neither the Payment card group (BG-18) nor the Direct debit group (BG-19)")
			}
		// context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = ('48','54','55')]
		case code == "48" || code == "54" || code == "55":
			if !card {
				add("BR-DE-24-a", "A Payment means type code (BT-81) for a card payment (48, 54, 55) requires the "+
					"Payment card information group (BG-18)")
			}
			if credit || mandate {
				add("BR-DE-24-b", "A Payment means type code (BT-81) for a card payment (48, 54, 55) shall carry "+
					"neither the Credit transfer group (BG-17) nor the Direct debit group (BG-19)")
			}
		// context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = '59']
		case code == "59":
			// BR-DE-20: the IBAN check on cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID.
			if !validIBAN(pm.str("PaymentMandate", "PayerFinancialAccount", "ID")) {
				add("BR-DE-20", "The Debited account identifier (BT-91) shall be a valid IBAN for a SEPA direct debit")
			}
			if !mandate {
				add("BR-DE-25-a", "A Payment means type code (BT-81) for a direct debit (59) requires the "+
					"Direct debit group (BG-19)")
			}
			if credit || card {
				add("BR-DE-25-b", "A Payment means type code (BT-81) for a direct debit (59) shall carry neither the "+
					"Credit transfer group (BG-17) nor the Payment card group (BG-18)")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// The CII binding
// ---------------------------------------------------------------------------

// xrCIIRules evaluates the rules of the cii-pattern whose context is a tree
// position rather than the whole document.
func xrCIIRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	tx := root.child("SupplyChainTradeTransaction").orNil()
	settle := tx.child("ApplicableHeaderTradeSettlement").orNil()

	// BR-DE-18, context /rsm:CrossIndustryInvoice, over
	// ram:SpecifiedTradePaymentTerms/ram:Description[1].
	xrSkontoRule(xrFirstNotes(settle, "SpecifiedTradePaymentTerms", "Description"), add)

	// BR-DE-30 / BR-DE-31, context /rsm:CrossIndustryInvoice. CII has no element
	// for BG-19, so KoSIT reconstructs the group from its three mandatory terms:
	//   BT-89  ram:SpecifiedTradePaymentTerms/ram:DirectDebitMandateID
	//   BT-90  ram:CreditorReferenceID
	//   BT-91  ram:SpecifiedTradeSettlementPaymentMeans/ram:PayerPartyDebtorFinancialAccount/ram:IBANID
	// The group exists when any of the three is present, so BT-90 alone — a
	// creditor identifier with no mandate and no debited account — is a BG-19 that
	// is missing BT-91, which is a case the model's directDebitPresent could not
	// express because it was set from BT-89 and BT-91 only.
	bt89 := len(nodesAt(settle, "SpecifiedTradePaymentTerms", "DirectDebitMandateID")) > 0
	bt90 := len(settle.all("CreditorReferenceID")) > 0
	bt91 := len(nodesAt(settle, "SpecifiedTradeSettlementPaymentMeans", "PayerPartyDebtorFinancialAccount", "IBANID")) > 0
	if bt89 || bt90 || bt91 {
		if !((bt89 || bt91) && bt90) {
			add("BR-DE-30", "A direct debit (BG-19) shall carry the Bank assigned creditor identifier (BT-90)")
		}
		if !((bt89 || bt90) && bt91) {
			add("BR-DE-31", "A direct debit (BG-19) shall carry the Debited account identifier (BT-91)")
		}
	}

	// The three payment-means groups, context ram:SpecifiedTradeSettlementPaymentMeans
	// selected by BT-81. BG-18 is ram:ApplicableTradeSettlementFinancialCard and
	// BG-19 is the reconstructed group above, which is document-scoped: KoSIT's
	// BR-DE-23-b and BR-DE-24-b look at the document's BT-89/BT-90 and at this
	// group's BT-91.
	for _, pm := range settle.all("SpecifiedTradeSettlementPaymentMeans") {
		if r.stopped() {
			return
		}
		code := normalizeSpace(pm.str("TypeCode"))
		credit := pm.child("PayeePartyCreditorFinancialAccount") != nil
		card := pm.child("ApplicableTradeSettlementFinancialCard") != nil
		groupBT91 := len(nodesAt(pm, "PayerPartyDebtorFinancialAccount", "IBANID")) > 0
		directDebit := bt89 || bt90 || groupBT91
		switch {
		case code == "30" || code == "58":
			if code == "58" && !validIBAN(pm.str("PayeePartyCreditorFinancialAccount", "IBANID")) {
				add("BR-DE-19", "The Payment account identifier (BT-84) shall be a valid IBAN for a SEPA credit transfer")
			}
			if !credit {
				add("BR-DE-23-a", "A Payment means type code (BT-81) for a credit transfer (30, 58) requires the "+
					"Credit transfer group (BG-17)")
			}
			if card || directDebit {
				add("BR-DE-23-b", "A Payment means type code (BT-81) for a credit transfer (30, 58) shall carry "+
					"neither the Payment card group (BG-18) nor the Direct debit group (BG-19)")
			}
		case code == "48" || code == "54" || code == "55":
			if !card {
				add("BR-DE-24-a", "A Payment means type code (BT-81) for a card payment (48, 54, 55) requires the "+
					"Payment card information group (BG-18)")
			}
			if credit || directDebit {
				add("BR-DE-24-b", "A Payment means type code (BT-81) for a card payment (48, 54, 55) shall carry "+
					"neither the Credit transfer group (BG-17) nor the Direct debit group (BG-19)")
			}
		case code == "59":
			if !validIBAN(pm.str("PayerPartyDebtorFinancialAccount", "IBANID")) {
				add("BR-DE-20", "The Debited account identifier (BT-91) shall be a valid IBAN for a SEPA direct debit")
			}
			if !directDebit {
				add("BR-DE-25-a", "A Payment means type code (BT-81) for a direct debit (59) requires the "+
					"Direct debit group (BG-19)")
			}
			// BR-DE-25-b names four elements, two more than the UBL binding does:
			// the payee and payer financial institutions (BT-86) as well as BG-17
			// and BG-18.
			if credit || card || pm.child("PayeeSpecifiedCreditorFinancialInstitution") != nil ||
				pm.child("PayerSpecifiedDebtorFinancialInstitution") != nil {
				add("BR-DE-25-b", "A Payment means type code (BT-81) for a direct debit (59) shall carry neither the "+
					"Credit transfer group (BG-17), the Payment card group (BG-18) nor a financial institution (BT-86)")
			}
		}
	}

}

// ---------------------------------------------------------------------------
// Shared rule bodies and code lists
// ---------------------------------------------------------------------------

// xrHasChildValue is XPath's `child = 'v'` over a node set: true when any child
// with that name has that value once normalized.
func xrHasChildValue(n *ciiNode, name, value string) bool {
	for _, c := range n.all(name) {
		if normalizeSpace(c.text) == value {
			return true
		}
	}
	return false
}

// xrFirstNotes is `<container>/<note>[1]` — the first note of *each* container,
// which is a sequence and not one value. BR-DE-18 evaluates over all of them.
func xrFirstNotes(n *ciiNode, container, note string) []string {
	var out []string
	for _, c := range n.all(container) {
		if first := c.child(note); first != nil {
			out = append(out, first.rawText())
		}
	}
	return out
}

// rawText is an element's string value with nothing done to it. Most rules want
// str(), which trims; the two that match a regular expression against an element
// — BR-DE-18 and BR-TMP-2 — must not, because their patterns are anchored and
// their subject is the value a reference validator sees.
func (n *ciiNode) rawText() string {
	if n == nil {
		return ""
	}
	return n.text
}

// The settlement-discount ("Skonto") format of BR-DE-18, from common.sch:
//
//	$XR-SKONTO-REGEX  (^|\r?\n)#(SKONTO)#TAGE=([0-9]+#PROZENT=[0-9]+\.[0-9]{2})(#BASISBETRAG=-?[0-9]+\.[0-9]{2})?#$
//
// The alternation at the front is vestigial: the assertion applies the pattern to
// normalize-space($line) of one tokenized line, which cannot contain a newline,
// so only the ^ arm can ever match.
var (
	xrSkontoRE      = regexp.MustCompile(`^#SKONTO#TAGE=[0-9]+#PROZENT=[0-9]+\.[0-9]{2}(#BASISBETRAG=-?[0-9]+\.[0-9]{2})?#$`)
	xrSkontoLineRE  = regexp.MustCompile(`\r?\n`)
	xrSkontoEntryRE = regexp.MustCompile(`#[^\n]+#`)
	xrSkontoTailRE  = regexp.MustCompile(`^[ \t\r\n]*\n`)
)

// xrSkontoRule is BR-DE-18 over the payment-terms notes of either binding.
//
// The assertion is one `every ... satisfies A and B`, and both halves are inside
// the quantifier, which is what makes it vacuously true for a note with no
// settlement-discount line at all — the common case, and the reason a plain
// "Zahlbar sofort ohne Abzug." payment term does not trip a format rule about a
// syntax it never uses.
//
//	A: each line starting with '#' matches $XR-SKONTO-REGEX once normalized.
//	B: tokenize(note, '#.+#')[last()] matches '^\s*\n' — i.e. what follows the
//	   last discount entry begins with a newline. '.' excludes newline in XPath,
//	   so an entry cannot span lines.
func xrSkontoRule(notes []string, add func(rule, msg string)) {
	var entryTokens []string
	for _, n := range notes {
		entryTokens = append(entryTokens, xrSkontoEntryRE.Split(n, -1)...)
	}
	tailOK := len(entryTokens) > 0 && xrSkontoTailRE.MatchString(entryTokens[len(entryTokens)-1])
	for _, n := range notes {
		for _, line := range xrSkontoLineRE.Split(n, -1) {
			line = normalizeSpace(line)
			if !strings.HasPrefix(line, "#") {
				continue
			}
			if !xrSkontoRE.MatchString(line) || !tailOK {
				add("BR-DE-18", "A settlement-discount line in the Payment terms (BT-20) does not match the required "+
					"format #SKONTO#TAGE=n#PROZENT=n.nn[#BASISBETRAG=n.nn]# followed by a line break")
				return
			}
		}
	}
}
