package formalis

import (
	"fmt"
	"strings"
)

// This file validates the Polish KSeF FA (Faktura ustrukturyzowana) format — the
// structured invoice exchanged through the Krajowy System e-Faktur. Like the
// other national formats it is XSD-validated rather than rule-validated, so this
// checks the mandatory structure and Polish code lists against the parsed tree.
//
// Rule identifiers are KS-* (this package's own). Not vendored: the KSeF sample
// instances (phax/phive-rules) are used only as the oracle.

// ksefRodzajFaktury is the RodzajFaktury invoice-kind code set: VAT (standard),
// KOR (correction), ZAL (advance), ROZ (settlement), UPR (simplified), and the
// correction variants KOR_ZAL / KOR_ROZ.
var ksefRodzajFaktury = map[string]bool{
	"VAT": true, "KOR": true, "ZAL": true, "ROZ": true, "UPR": true, "KOR_ZAL": true, "KOR_ROZ": true,
}

// IsKSeF reports whether the XML is a KSeF Faktura document.
func IsKSeF(xmlData []byte) bool {
	root, err := parseCII(xmlData)
	return err == nil && root.name == "Faktura" && root.child("Naglowek") != nil
}

// ValidateKSeF validates a Polish KSeF FA document against its mandatory
// structure and Polish code lists.
func ValidateKSeF(xmlData []byte) []Violation {
	root, err := parseCII(xmlData)
	if err != nil {
		return []Violation{{Rule: "syntax", Message: err.Error()}}
	}
	if root.name != "Faktura" {
		return []Violation{{Rule: "KS-root", Message: "the document root shall be Faktura"}}
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, Message: msg}) }

	// KS-header: the form code (Naglowek/KodFormularza) is mandatory.
	if strings.TrimSpace(root.str("Naglowek", "KodFormularza")) == "" {
		add("KS-header", "the header (Naglowek) shall contain a KodFormularza")
	}

	// KS-seller (Podmiot1): NIP, name and country are mandatory.
	p1 := root.child("Podmiot1").orNil()
	if strings.TrimSpace(p1.str("DaneIdentyfikacyjne", "NIP")) == "" {
		add("KS-seller-nip", "the Seller (Podmiot1) shall have a NIP")
	}
	if strings.TrimSpace(p1.str("DaneIdentyfikacyjne", "Nazwa")) == "" {
		add("KS-seller-name", "the Seller (Podmiot1) shall have a Nazwa")
	}
	if strings.TrimSpace(p1.str("Adres", "KodKraju")) == "" {
		add("KS-seller-country", "the Seller address (Podmiot1/Adres) shall have a KodKraju")
	}

	// KS-buyer (Podmiot2): present, and — where an address is given (it is
	// omitted for some public-body buyers) — the address carries a country.
	p2 := root.child("Podmiot2").orNil()
	if p2.name == "" {
		add("KS-buyer", "the invoice shall contain a Buyer (Podmiot2)")
	} else if p2.child("Adres") != nil && strings.TrimSpace(p2.str("Adres", "KodKraju")) == "" {
		add("KS-buyer-country", "a Buyer address (Podmiot2/Adres), when present, shall have a KodKraju")
	}

	// KS-invoice (Fa): currency, issue date, number and invoice kind.
	fa := root.child("Fa").orNil()
	if cur := strings.TrimSpace(fa.str("KodWaluty")); !en16931Currencies[cur] {
		add("KS-currency", fmt.Sprintf("the currency (KodWaluty=%q) shall be a valid ISO 4217 code", cur))
	}
	if strings.TrimSpace(fa.str("P_1")) == "" {
		add("KS-date", "the invoice shall contain an issue date (Fa/P_1)")
	}
	if strings.TrimSpace(fa.str("P_2")) == "" {
		add("KS-number", "the invoice shall contain a number (Fa/P_2)")
	}
	if rf := strings.TrimSpace(fa.str("RodzajFaktury")); !ksefRodzajFaktury[rf] {
		add("KS-type", fmt.Sprintf("the invoice kind (RodzajFaktury=%q) shall be a valid code (VAT, KOR, ZAL, ROZ, UPR, KOR_ZAL, KOR_ROZ)", rf))
	}

	return out
}
