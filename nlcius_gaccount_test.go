package formalis

import (
	"context"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// gaccountSch is the vendored G-account Schematron, and gaccountDir the ten
// instances SimplerInvoicing ships for it.
const (
	gaccountSch = "testdata/nlcius/schematron/ubl/si-ubl-2.0-ext-gaccount-1.0.2.sch"
	gaccountDir = "testdata/nlcius/gaccount"
)

// gaccountPublished is the whole of si-ubl-2.0-ext-gaccount-1.0.2.sch's own pattern,
// written out so the transcription in nlcius_gaccount.go is a string comparison
// against the artefact rather than a claim about a reading.
//
// It is the rule order, each rule's context, and each assertion's identifier, flag
// and XPath verbatim. C37 is why the XPath is here and the prose is not: fifteen
// national rules in this package were transcribed from an authority's documentation
// and said something the authority's Schematron does not.
var gaccountPublished = []struct {
	context string
	asserts []struct{ id, flag, test string }
}{
	{
		context: "//cbc:CustomizationID",
		asserts: []struct{ id, flag, test string }{
			{"BR-GA-0", "fatal", "normalize-space(.) = 'urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0'"},
		},
	},
	{
		context: "/ubl:Invoice",
		asserts: []struct{ id, flag, test string }{
			{"BR-GA-1", "fatal", "count(cac:PaymentTerms) = 2"},
			{"BR-GA-2", "fatal", "count(cac:PaymentMeans) = 2"},
			{"BR-GA-3", "fatal", "cac:LegalMonetaryTotal/xs:decimal(cbc:PayableAmount) = sum(cac:PaymentTerms/xs:decimal(cbc:Amount))"},
			{"BR-GA-7", "fatal", "count(cac:PaymentMeans/cbc:ID[text()='GACCOUNT']) = 1"},
		},
	},
	{
		context: "/ubl:Invoice/cac:PaymentTerms",
		asserts: []struct{ id, flag, test string }{
			{"BR-GA-4", "fatal", "count(cbc:PaymentMeansID) = 1"},
		},
	},
	{
		context: "/ubl:Invoice/cac:PaymentMeans",
		asserts: []struct{ id, flag, test string }{
			{"BR-GA-5", "fatal", "count(cbc:ID) = 1"},
		},
	},
	{
		// The one assertion in every artefact this repository vendors that carries no
		// flag attribute. "none" is assertFlags's marker for that; severityOfFlag is
		// where it is folded, and TestGAccountSeveritiesAreThePublishedFlags is where
		// the fold is argued.
		context: "/ubl:Invoice/cac:PaymentTerms/cbc:PaymentMeansID",
		asserts: []struct{ id, flag, test string }{
			{"BR-GA-6", "none", ". = $payment-means-ids"},
		},
	},
}

// gaccountRule reads si-ubl-2.0-ext-gaccount-1.0.2.sch's own pattern with an XML
// decoder, in document order.
//
// It decodes the file rather than the merged rule set the extension resolves to:
// the file <include>s si-ubl-2.0-nlcius.sch and four CEN files, and a decoder walking
// the bytes sees the <include> elements and not their contents, which is exactly the
// scope wanted here — the pattern the extension itself publishes.
type gaccountRule struct {
	context string
	asserts []struct{ id, flag, test string }
}

func readGAccountPattern(t *testing.T) []gaccountRule {
	t.Helper()
	data, err := os.ReadFile(gaccountSch)
	if err != nil {
		t.Skipf("G-account Schematron not present; run `make cius-schematron` (%v)", err)
	}
	var out []gaccountRule
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		at := func(n string) string {
			for _, a := range se.Attr {
				if a.Name.Local == n {
					return a.Value
				}
			}
			return ""
		}
		switch se.Name.Local {
		case "rule":
			out = append(out, gaccountRule{context: normSpace(at("context"))})
		case "assert", "report":
			if len(out) == 0 {
				t.Fatalf("%s carries an assertion outside any rule", gaccountSch)
			}
			flag := at("flag")
			if flag == "" {
				flag = "none"
			}
			out[len(out)-1].asserts = append(out[len(out)-1].asserts,
				struct{ id, flag, test string }{at("id"), flag, normSpace(at("test"))})
		}
	}
	return out
}

// TestGAccountAssertionsAreTranscribedFromTheArtefact holds the eight rules in
// nlcius_gaccount.go to the file that publishes them: same rules, same order, same
// contexts, same identifiers, same flags, same XPath.
//
// The XPath comparison is the substantive half. A rule transcribed from the
// specification PDF the file's header links to would be a rule nobody can check
// against anything — C37 is fifteen instances of exactly that, worth 1,954 findings.
func TestGAccountAssertionsAreTranscribedFromTheArtefact(t *testing.T) {
	got := readGAccountPattern(t)
	if len(got) != len(gaccountPublished) {
		t.Fatalf("%s publishes %d rules and this package's transcription has %d",
			gaccountSch, len(got), len(gaccountPublished))
	}
	for i, want := range gaccountPublished {
		if got[i].context != want.context {
			t.Errorf("rule %d: the artefact binds it to %q and this package records %q", i, got[i].context, want.context)
		}
		if len(got[i].asserts) != len(want.asserts) {
			t.Errorf("rule %d (%s): the artefact carries %d assertions and this package records %d",
				i, want.context, len(got[i].asserts), len(want.asserts))
			continue
		}
		for j, wa := range want.asserts {
			ga := got[i].asserts[j]
			if ga.id != wa.id || ga.flag != wa.flag || ga.test != wa.test {
				t.Errorf("rule %d assertion %d: the artefact publishes {%s %s %q} and this package records "+
					"{%s %s %q}", i, j, ga.id, ga.flag, ga.test, wa.id, wa.flag, wa.test)
			}
		}
	}
	// And the identifiers this package claims to evaluate are exactly the identifiers
	// the file publishes, in both directions. ciusEvaluated is checked against the
	// artefact by TestCIUSSeveritiesQuoteTheirAuthority; this narrows it to the
	// extension, so a BR-GA rule added upstream cannot be absorbed by the wider check.
	published, evaluated := map[string]bool{}, map[string]bool{}
	for _, r := range got {
		for _, a := range r.asserts {
			published[a.id] = true
		}
	}
	for id := range ciusEvaluated[SourceNLCIUS] {
		if strings.HasPrefix(id, "BR-GA-") {
			evaluated[id] = true
		}
	}
	for id := range published {
		if !evaluated[id] {
			t.Errorf("%s publishes %s and this package does not evaluate it", filepath.Base(gaccountSch), id)
		}
	}
	for id := range evaluated {
		if !published[id] {
			t.Errorf("this package evaluates %s and %s does not publish it", id, filepath.Base(gaccountSch))
		}
	}
	t.Logf("G-account extension: %d rules, %d assertions, transcribed from %s",
		len(got), len(published), filepath.Base(gaccountSch))
}

// TestGAccountRuleContextsAreNotShadowed is the rule-order verdict, taken rather
// than assumed.
//
// ISO Schematron gives a node to the first rule of a pattern whose context matches
// it. That has decided a rule's meaning five times in this repository — three CEN
// CII-DT-*, three BR-CIUS-PT-*, six CIUS-RO, fifteen SRBDT and four NLCIUS
// assertions — and in two of those cases this package was reporting findings the
// authority's own validator cannot produce (C42). So a new pattern gets the same
// question asked of it before anything is implemented, not after.
//
// The answer for this one is that nothing is shadowed: its five contexts are
// //cbc:CustomizationID and four absolute paths under /ubl:Invoice, and no absolute
// path covers another. All eight assertions are reachable.
func TestGAccountRuleContextsAreNotShadowed(t *testing.T) {
	if _, err := os.Stat(gaccountSch); err != nil {
		t.Skip("G-account Schematron not present; run `make cius-schematron`")
	}
	shadowed := schShadowedRules(t, []string{gaccountSch})
	for id, by := range shadowed {
		t.Errorf("%s's rule for %s is claimed by the earlier context %q, so no processor reaches it and this "+
			"package must not report it. It belongs in Coverage(SourceNLCIUS) with Unevaluable set, the way the "+
			"four NLCIUS assertions in that position do", filepath.Base(gaccountSch), id, by)
	}
	if len(shadowed) == 0 {
		t.Logf("%s: no rule of the g-account-extension pattern repeats or covers an earlier one, so all eight "+
			"assertions are reachable", filepath.Base(gaccountSch))
	}
}

// TestGAccountSeveritiesAreThePublishedFlags pins the one severity in this package
// that is not a literal quotation of a flag attribute, because there is no flag
// attribute to quote.
//
// Seven of the eight are flag="fatal". BR-GA-6 carries none. An assertion with no
// flag is not an assertion with no severity: phive runs ph-schematron, and
// DefaultSVRLErrorLevelDeterminator sends a flag it cannot recognise — null
// included — to DEFAULT_ERROR_LEVEL, which the class declares as EErrorLevel.ERROR.
// So a reference validation reports BR-GA-6 as an error and this package reports it
// fatal.
//
// Both halves are asserted, and the second is the one that matters: if
// SimplerInvoicing ever writes a flag on that assertion, whichever flag it writes,
// this fails and the reading has to be re-derived rather than inherited.
func TestGAccountSeveritiesAreThePublishedFlags(t *testing.T) {
	rules := readGAccountPattern(t)
	flagged := map[string]string{}
	for _, r := range rules {
		for _, a := range r.asserts {
			flagged[a.id] = a.flag
		}
	}
	if got := flagged["BR-GA-6"]; got != "none" {
		t.Errorf("%s now flags BR-GA-6 %q where it carried no flag attribute at all; this package reports it "+
			"fatal on ph-schematron's default-error-level reading, and a published flag supersedes that reading",
			filepath.Base(gaccountSch), got)
	}
	for id, flag := range flagged {
		want, known := severityOfFlag(flag)
		if !known {
			t.Errorf("%s carries the flag %q, which this package cannot fold onto a Severity", id, flag)
			continue
		}
		if got := ciusEvaluated[SourceNLCIUS][id]; got != want {
			t.Errorf("this package reports %s as %s and its authority flags it %q", id, got, flag)
		}
	}
}

// TestGAccountSyntaxCopyRemovesExactlyThreeAdvisoryRules derives
// nlciusGAccountRemovedCEN from the two files rather than believing the header
// comment that names them.
//
// The comparison is between EN16931-syntax-modified.sch and the EN16931-syntax.sch
// **in the same SI-UBL 2.0.3.2 tree**, which is what makes it a measurement of
// SimplerInvoicing's edit rather than of the gap between two release dates. That
// distinction is C40's correction: every "authority X modified CEN's rule" claim in
// this repository's audit rested on diffing a vendored copy against CEN's HEAD, and
// almost all of them turned out to be CEN's own version drift.
//
// Here the two files are the same release of the same copy and differ in exactly
// three assertions, each commented out in place with its original text intact.
func TestGAccountSyntaxCopyRemovesExactlyThreeAdvisoryRules(t *testing.T) {
	const dir = "testdata/nlcius/schematron/ubl/cen"
	plain, err := os.ReadFile(filepath.Join(dir, "EN16931-syntax.sch"))
	if err != nil {
		t.Skip("NLCIUS's copy of CEN's Schematron not present; run `make cius-schematron`")
	}
	modified, err := os.ReadFile(filepath.Join(dir, "EN16931-syntax-modified.sch"))
	if err != nil {
		t.Skip("the G-account syntax copy is not present; run `make cius-schematron`")
	}
	before := assertFlags(t, "EN16931-syntax.sch", plain)
	after := assertFlags(t, "EN16931-syntax-modified.sch", modified)
	if len(before) < 600 {
		t.Fatalf("read only %d identifiers from NLCIUS's copy of CEN's abstract syntax file; the harness is not "+
			"reading the artefact", len(before))
	}
	var removed, added []string
	for id := range before {
		if _, ok := after[id]; !ok {
			removed = append(removed, id)
		}
	}
	for id := range after {
		if _, ok := before[id]; !ok {
			added = append(added, id)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	if len(added) != 0 {
		t.Errorf("the G-account copy of CEN's abstract syntax file adds %v; this package assumes it only removes, "+
			"and an added assertion would be a rule nobody here evaluates", added)
	}
	var want []string
	for id := range nlciusGAccountRemovedCEN {
		want = append(want, id)
	}
	sort.Strings(want)
	if strings.Join(removed, " ") != strings.Join(want, " ") {
		t.Errorf("the G-account copy of CEN's abstract syntax file removes %v and nlciusGAccountRemovedCEN "+
			"suppresses %v. The two must be the same set: a rule the authority still publishes must be reported, "+
			"and one it removed must not be", removed, want)
	}
	// Each removed identifier must be one CEN flags advisory. A fatal one would mean
	// the extension excuses a document from a rule that rejects it, which is a much
	// larger claim than "these three elements are permitted here".
	for _, id := range removed {
		if sev, known := severityOfFlag(pickFlag(before[id])); !known || sev != SeverityWarning {
			t.Errorf("the G-account copy removes %s, which CEN flags %v; this package suppresses it on the "+
				"reading that the extension permits three elements CEN merely discourages", id, keysOf(before[id]))
		}
	}
	t.Logf("the G-account extension's copy of CEN's abstract syntax file removes %v, and nothing else", removed)
}

// nlciusGAccountDoc is a conforming SI-UBL 2.0 G-account invoice: 121.00 due, split
// 91.00 to the beneficiary's account and 30.00 into the blocked one, each payment
// term naming the instruction that pays it.
//
// It exists for the reason nlciusUBLDiscouragedDoc does — the SimplerInvoicing
// instances are the stronger oracle where they apply and they skip on a checkout
// with no testdata/, which is what a clean clone and CI both are.
const nlciusGAccountDoc = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
<cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0</cbc:CustomizationID>
<cbc:ID>INV-GA1</cbc:ID><cbc:IssueDate>2024-01-15</cbc:IssueDate>
<cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cbc:BuyerReference>NLBUYERREF</cbc:BuyerReference>
<cac:AccountingSupplierParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>SellerStreet</cbc:StreetName><cbc:CityName>SellerCity</cbc:CityName><cbc:PostalZone>1011AA</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyTaxScheme><cbc:CompanyID>NL123456789B01</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  <cac:PartyLegalEntity><cbc:RegistrationName>Seller BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">11223344</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingSupplierParty>
<cac:AccountingCustomerParty><cac:Party>
  <cac:PostalAddress><cbc:StreetName>BuyerStreet</cbc:StreetName><cbc:CityName>BuyerCity</cbc:CityName><cbc:PostalZone>2022BB</cbc:PostalZone><cac:Country><cbc:IdentificationCode>NL</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
  <cac:PartyLegalEntity><cbc:RegistrationName>Buyer BV</cbc:RegistrationName><cbc:CompanyID schemeID="0106">55667788</cbc:CompanyID></cac:PartyLegalEntity>
</cac:Party></cac:AccountingCustomerParty>
<cac:PaymentMeans><cbc:ID>BENEFICIARY</cbc:ID><cbc:PaymentMeansCode>58</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>
<cac:PaymentMeans><cbc:ID>GACCOUNT</cbc:ID><cbc:PaymentMeansCode>58</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL91ABNA0417164300</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>
<cac:PaymentTerms><cbc:PaymentMeansID>BENEFICIARY</cbc:PaymentMeansID><cbc:Amount currencyID="EUR">91.00</cbc:Amount></cac:PaymentTerms>
<cac:PaymentTerms><cbc:PaymentMeansID>GACCOUNT</cbc:PaymentMeansID><cbc:Amount currencyID="EUR">30.00</cbc:Amount></cac:PaymentTerms>
<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">21.00</cbc:TaxAmount><cac:TaxSubtotal><cbc:TaxableAmount>100.00</cbc:TaxableAmount><cbc:TaxAmount>21.00</cbc:TaxAmount><cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
<cac:LegalMonetaryTotal><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount>100.00</cbc:TaxExclusiveAmount><cbc:TaxInclusiveAmount>121.00</cbc:TaxInclusiveAmount><cbc:PayableAmount>121.00</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity><cbc:LineExtensionAmount>100.00</cbc:LineExtensionAmount><cac:Item><cbc:Name>Widget</cbc:Name><cac:ClassifiedTaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item><cac:Price><cbc:PriceAmount>100.00</cbc:PriceAmount></cac:Price></cac:InvoiceLine>
</Invoice>`

// gaBreak is one substitution on the G-account baseline. It panics rather than
// returning an error, because a fixture whose mutation does not apply is a fixture
// that silently tests the baseline — which is the "present, reachable and inert"
// state C41 is about.
func gaBreak(from, to string) string {
	out := strings.Replace(nlciusGAccountDoc, from, to, 1)
	if out == nlciusGAccountDoc {
		panic("G-account fixture: mutation string not found: " + from)
	}
	return out
}

// nlciusGAccountExtras is one document per BR-GA identifier, each of which must make
// exactly that rule report. TestEveryEvaluatedCIUSRuleFires reads them.
//
// The BR-GA-0 case is the one that had to be designed rather than written. Its
// artefact test is "the specification identifier is the extension's", and the
// obvious gate for the extension is the same test — which would make the rule
// unfalsifiable, present and inert, and would pass every other guard in this suite
// (C41). It fires here because nlciusGAccountApplies has a second arm: an invoice
// that carries the GACCOUNT payment instruction is inside the extension whether it
// declares it or not, which is the case the rule exists for.
var nlciusGAccountExtras = []ciusDoc{
	{"G-account split without the extension's specification identifier (GA-0)",
		gaBreak("#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0",
			"#compliant#urn:fdc:nen.nl:nlcius:v1.0"), "BR-GA-0"},
	{"one payment term rather than two (GA-1)",
		gaBreak(`<cac:PaymentTerms><cbc:PaymentMeansID>GACCOUNT</cbc:PaymentMeansID><cbc:Amount currencyID="EUR">30.00</cbc:Amount></cac:PaymentTerms>`, ""), "BR-GA-1"},
	{"one payment instruction rather than two (GA-2)",
		gaBreak(`<cac:PaymentMeans><cbc:ID>BENEFICIARY</cbc:ID><cbc:PaymentMeansCode>58</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>NL02ABNA0123456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`, ""), "BR-GA-2"},
	{"the payment amounts do not sum to the amount due (GA-3)",
		gaBreak(`<cbc:Amount currencyID="EUR">30.00</cbc:Amount>`, `<cbc:Amount currencyID="EUR">40.00</cbc:Amount>`), "BR-GA-3"},
	{"a payment term with no payment means reference (GA-4)",
		gaBreak(`<cbc:PaymentMeansID>GACCOUNT</cbc:PaymentMeansID>`, ""), "BR-GA-4"},
	{"a payment instruction with no payment means identifier (GA-5)",
		gaBreak(`<cbc:ID>BENEFICIARY</cbc:ID>`, ""), "BR-GA-5"},
	{"a payment means reference naming no instruction (GA-6)",
		gaBreak(`<cbc:PaymentMeansID>BENEFICIARY</cbc:PaymentMeansID>`, `<cbc:PaymentMeansID>ELSEWHERE</cbc:PaymentMeansID>`), "BR-GA-6"},
	{"no instruction marked as the blocked account (GA-7)",
		gaBreak(`<cbc:ID>GACCOUNT</cbc:ID>`, `<cbc:ID>OTHER</cbc:ID>`), "BR-GA-7"},
}

// TestNLCIUSGAccountConformingInvoiceIsClean is the FP=0 half of this rule set,
// stated on the document that matters most: one that uses the extension correctly.
//
// It asserts nothing at all is reported, not merely nothing fatal, because the three
// advisory CEN rules the extension removes are exactly the ones a conforming
// G-account invoice trips — UBL-CR-411, -453 and -459 forbid the PaymentMeans ID,
// the PaymentTerms PaymentMeansID and the PaymentTerms Amount, which are NL-GA-04,
// NL-GA-02 and NL-GA-03. Before they were suppressed this package reported all three
// on every one of SimplerInvoicing's ten G-account instances, its conforming sample
// included.
func TestNLCIUSGAccountConformingInvoiceIsClean(t *testing.T) {
	r := mustReport(t, context.Background(), ValidateNLCIUS, []byte(nlciusGAccountDoc))
	if len(r.Violations) != 0 {
		t.Errorf("a conforming G-account invoice reports %v", r.Violations)
	}
	// And the suppression is scoped to a G-account document. The same invoice without
	// the extension is judged by CEN's unmodified file and must report all three, or
	// the filter would be silently excusing every Dutch invoice.
	plain := strings.Replace(nlciusGAccountDoc,
		"#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0",
		"#compliant#urn:fdc:nen.nl:nlcius:v1.0", 1)
	plain = strings.ReplaceAll(plain, "<cbc:ID>GACCOUNT</cbc:ID>", "<cbc:ID>SECOND</cbc:ID>")
	plain = strings.ReplaceAll(plain, "<cbc:PaymentMeansID>GACCOUNT</cbc:PaymentMeansID>", "<cbc:PaymentMeansID>SECOND</cbc:PaymentMeansID>")
	got := map[string]bool{}
	for _, v := range mustReport(t, context.Background(), ValidateNLCIUS, []byte(plain)).Violations {
		got[v.Rule] = true
	}
	for id := range nlciusGAccountRemovedCEN {
		if !got[id] {
			t.Errorf("the same invoice outside the G-account extension does not report %s; the three advisory "+
				"rules the extension removes are suppressed for every document rather than for its own", id)
		}
	}
	for id := range got {
		if strings.HasPrefix(id, "BR-GA-") {
			t.Errorf("an invoice that declares neither the extension nor a GACCOUNT payment instruction reports "+
				"%s; the extension is applying to documents outside it", id)
		}
	}
}

// gaccountFixtureRule is the rule each of SimplerInvoicing's nine broken G-account
// instances is broken against, derived by reading each instance against the artefact
// rather than from its file name — the names say which *term* is wrong, not which
// identifier reports.
//
// Two names are worth reading twice. "error_no_identifier" removes a
// cac:PaymentMeans/cbc:ID, so the rule that reports is BR-GA-5 ("each Payment
// Instruction MUST include a Payment Means identifier") and not BR-GA-7, which counts
// the GACCOUNT-marked one and still finds exactly one. "error_no_gaccount" is the
// mirror image: both identifiers are present, and it is the marker that is gone.
var gaccountFixtureRule = map[string]string{
	"si-ubl-2.0-ext-gaccount_error_no_gaccount.xml":                  "BR-GA-7",
	"si-ubl-2.0-ext-gaccount_error_no_identifier.xml":                "BR-GA-5",
	"si-ubl-2.0-ext-gaccount_error_no_reference.xml":                 "BR-GA-4",
	"si-ubl-2.0-ext-gaccount_error_one-paymentmeans.xml":             "BR-GA-2",
	"si-ubl-2.0-ext-gaccount_error_one-paymentterm.xml":              "BR-GA-1",
	"si-ubl-2.0-ext-gaccount_error_paymentterms_wrong_reference.xml": "BR-GA-6",
	"si-ubl-2.0-ext-gaccount_error_sum_amount.xml":                   "BR-GA-3",
	"si-ubl-2.0-ext-gaccount_error_three-paymentmeans.xml":           "BR-GA-2",
	"si-ubl-2.0-ext-gaccount_error_three-paymentterms.xml":           "BR-GA-1",
	"si-ubl-2.0-ext-gaccount_ok_sample.xml":                          "",
}

// TestNLCIUSGAccountFixtures is the extension's oracle, and it is SimplerInvoicing's
// own rather than this repository's.
//
// It answers the question PR 22 asked of the 95 SI-UBL instances and could not ask
// here, because the G-account instances are a separate upstream directory that the
// SI-UBL-2.0.3.2 fetch never touched: **none of those 95 exercises the extension**,
// and these ten are the only documents any authority ships for it.
//
// Three assertions per instance, which is what makes it stronger than "some rule
// fires": the named rule must report, the conforming sample must report nothing at
// all, and no instance may collect a *fatal* finding from any other Source — the
// documents are SI-UBL 2.0 invoices that break one G-account rule each, so anything
// else this package rejects them for is a false positive against the whole stack
// under them.
func TestNLCIUSGAccountFixtures(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(gaccountDir, "*.xml"))
	if len(files) == 0 {
		t.Skip("the SimplerInvoicing G-account instances are not present (make cius-oracles)")
	}
	fired := map[string]bool{}
	for _, f := range files {
		base := filepath.Base(f)
		want, known := gaccountFixtureRule[base]
		if !known {
			t.Errorf("%s is a G-account instance this test has no verdict for; read it against "+
				"si-ubl-2.0-ext-gaccount-1.0.2.sch and add the rule it breaks", base)
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r := mustReport(t, context.Background(), ValidateNLCIUS, data)
		var ga, others []string
		for _, v := range r.Violations {
			switch {
			case strings.HasPrefix(v.Rule, "BR-GA-"):
				ga = append(ga, v.Rule)
			case v.Severity == SeverityFatal:
				others = append(others, string(v.Source)+"/"+v.Rule)
			}
		}
		sort.Strings(ga)
		if len(others) != 0 {
			t.Errorf("%s: SimplerInvoicing ships this as an SI-UBL 2.0 invoice that breaks a G-account rule, and "+
				"this package additionally rejects it for %v", base, others)
		}
		if want == "" {
			if len(r.Violations) != 0 {
				t.Errorf("%s: SimplerInvoicing ships this as a conforming G-account invoice and this package "+
					"reports %v", base, r.Violations)
			}
			continue
		}
		if !containsString(ga, want) {
			t.Errorf("%s: this instance breaks %s and this package reports %v", base, want, ga)
			continue
		}
		fired[want] = true
	}
	atLeast(t, "SimplerInvoicing G-account instances", len(files), minNLCIUSGAccountInstances)
	atLeast(t, "G-account rules a SimplerInvoicing instance fires", len(fired), minNLCIUSGAccountRulesFired)
	t.Logf("NLCIUS G-account: %d instances read, %d of the 8 published rules made to fire by the instance that "+
		"breaks them", len(files), len(fired))
}

// TestGAccountAppliesToNothingElseInTheCorpus is the false-positive sweep for the
// gate, and it is the assertion that a rule set added behind a document-content
// gate needs most: the gate decides which documents are judged at all, so a gate
// that is too wide is a rule set firing on invoices no authority means it for.
//
// Two arms, so two ways to be wrong. Neither has an instance in the corpus outside
// the ten the extension ships: no document declares the conformant identifier, and
// none carries a payment instruction whose identifier is the literal GACCOUNT.
func TestGAccountAppliesToNothingElseInTheCorpus(t *testing.T) {
	files, inside := 0, 0
	var strays []string
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		r := newRun(context.Background())
		parsed, perr := parseEN16931(r, data)
		if perr != nil {
			return nil
		}
		if !nlciusGAccountApplies(parsed) {
			return nil
		}
		inside++
		if filepath.Dir(p) != filepath.Clean(gaccountDir) {
			strays = append(strays, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Skip("corpus not present (make cius-oracles)")
	}
	atLeast(t, "G-account gate sweep corpus", files, minCorpusDocuments)
	sort.Strings(strays)
	for _, p := range strays {
		t.Errorf("%s is judged by the G-account extension and is not one of SimplerInvoicing's G-account "+
			"instances. Either it really does carry a G-account split — in which case say so here — or "+
			"nlciusGAccountApplies is too wide and the eight rules are being asked of a document no authority "+
			"means them for", p)
	}
	atLeast(t, "documents inside the G-account extension", inside, minNLCIUSGAccountInstances)
	t.Logf("the G-account extension applies to %d of %d corpus documents, all of them SimplerInvoicing's own "+
		"instances", inside, files)
}
