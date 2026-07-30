// The examples in this file are the ones the README shows. They live here so
// that `go test` compiles and runs them: a README example that has drifted from
// the API is worse than no example, and this package's exported surface has
// changed enough that prose alone could not be trusted to keep up.
//
// They are in package formalis_test, not formalis, so that every call is written
// the way a consumer writes it — qualified, through the exported API only.
package formalis_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/mgilbir/formalis"
)

// exampleUBL is a minimal EN 16931 UBL invoice declaring no CIUS.
const exampleUBL = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2">
<CustomizationID>urn:cen.eu:en16931:2017</CustomizationID>
<ID>INV-1</ID><IssueDate>2024-01-15</IssueDate>
<InvoiceTypeCode>380</InvoiceTypeCode><DocumentCurrencyCode>EUR</DocumentCurrencyCode>
<AccountingSupplierParty><Party>
  <PostalAddress><Country><IdentificationCode>DE</IdentificationCode></Country></PostalAddress>
  <PartyTaxScheme><CompanyID>DE123456789</CompanyID><TaxScheme><ID>VAT</ID></TaxScheme></PartyTaxScheme>
  <PartyLegalEntity><RegistrationName>Seller Ltd</RegistrationName></PartyLegalEntity>
</Party></AccountingSupplierParty>
<AccountingCustomerParty><Party>
  <PostalAddress><Country><IdentificationCode>DE</IdentificationCode></Country></PostalAddress>
  <PartyLegalEntity><RegistrationName>Buyer Ltd</RegistrationName></PartyLegalEntity>
</Party></AccountingCustomerParty>
<TaxTotal><TaxAmount>19.00</TaxAmount>
  <TaxSubtotal><TaxableAmount>100.00</TaxableAmount><TaxAmount>19.00</TaxAmount>
    <TaxCategory><ID>S</ID><Percent>19</Percent></TaxCategory></TaxSubtotal>
</TaxTotal>
<LegalMonetaryTotal><LineExtensionAmount>100.00</LineExtensionAmount>
  <TaxExclusiveAmount>100.00</TaxExclusiveAmount><TaxInclusiveAmount>119.00</TaxInclusiveAmount>
  <PayableAmount>119.00</PayableAmount></LegalMonetaryTotal>
<InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="C62">1</InvoicedQuantity>
  <LineExtensionAmount>100.00</LineExtensionAmount>
  <Item><Name>Widget</Name><ClassifiedTaxCategory><ID>S</ID><Percent>19</Percent></ClassifiedTaxCategory></Item>
  <Price><PriceAmount>100.00</PriceAmount></Price></InvoiceLine>
</Invoice>`

// Example validates a document against whichever rule set it declares, and reads
// the answer the way this package intends: not "were there findings?" but "was
// anything left unexamined?".
//
// The error is a separate question from the findings. It means the input could not
// be read at all — malformed XML, an encoding this package does not implement —
// and nothing about the invoice, which is why it is not a finding.
//
// The four lines it prints are four different facts, and the last two are the ones
// worth reading together. This document declares no CIUS, so it is validated
// against the EN 16931 core: nothing was found; the core has no rule left that CEN
// would reject this invoice over and that this package did not check, so it is
// conformant; and it is complete, because everything left unevaluated is a rule CEN
// published that no validator can evaluate — four bound to the XPath expression
// true(), three unreachable in CEN's own Schematron rule ordering, one whose test a
// correctly masked card number trips.
//
// So "gaps" and "complete" are both true at once, and that is not a contradiction:
// Report.NotEvaluated still names those seven rules, because a caller comparing
// this package against a reference validator deserves to know they exist, while
// Report.Complete passes over them because no reference validator evaluates them
// either. RuleFamily.Unevaluable is the field that separates the two.
//
// A document declaring a CIUS whose fatal rules are only partly implemented prints
// conformant: false and complete: false, with the missing families in
// Report.NotEvaluated.
func Example() {
	report, err := formalis.ValidateCIUS(context.Background(), []byte(exampleUBL))
	if err != nil {
		fmt.Println("could not read it:", err)
		return
	}

	for _, v := range report.Fatal() {
		fmt.Printf("%s %s: %s\n", v.Source, v.Rule, v.Message)
	}

	fmt.Println("nothing found:", len(report.Violations) == 0)
	fmt.Println("conformant:   ", report.Conformant())
	fmt.Println("complete:     ", report.Complete())
	fmt.Println("gaps:         ", len(report.NotEvaluated) > 0)

	// Output:
	// nothing found: true
	// conformant:    true
	// complete:      true
	// gaps:          true
}

// ExampleValidate checks the EN 16931 core against a declared Factur-X profile,
// and shows what a finding looks like.
func ExampleValidate() {
	// An invoice whose seller has no registered name breaks BR-06.
	broken := strings.Replace(exampleUBL, "<RegistrationName>Seller Ltd</RegistrationName>", "", 1)

	report, err := formalis.Validate(context.Background(), []byte(broken), formalis.ProfileEN16931)
	if err != nil {
		fmt.Println("could not read it:", err)
		return
	}
	for _, v := range report.Violations {
		fmt.Printf("%s %s (%s)\n", v.Source, v.Rule, v.Severity)
	}

	// Output:
	// EN 16931 BR-06 (fatal)
}

// ExampleDetect routes a document without a table of the caller's own. The three
// answers Detect can give are kept apart: an error means it could not be read,
// an unrecognised Detection means it was read and is no format this package
// validates, and anything else names the rule set and the entry point.
func ExampleDetect() {
	// A Malaysian Peppol PINT invoice. Its specification identifier contains the
	// substring "peppol", and it is not a Peppol BIS Billing 3.0 document; the
	// arbitration Detect applies is what tells the two apart.
	const pint = `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2">
	<CustomizationID>urn:peppol:pint:billing-1@my-1</CustomizationID>
	</Invoice>`

	det, err := formalis.Detect([]byte(pint))
	switch {
	case err != nil:
		fmt.Println("could not read it:", err)
	case !det.Recognised():
		fmt.Println("read it; no format this package validates")
	default:
		fmt.Println("root:  ", det.Root)
		fmt.Println("source:", det.Source)
		fmt.Println("CIUS:  ", det.CIUS)

		// Detection.Validator is ValidatePINT here, so the findings are PINT's
		// and not Peppol BIS Billing 3.0's.
		report, verr := det.Validator()(context.Background(), []byte(pint))
		if verr != nil {
			fmt.Println("could not read it:", verr)
			return
		}
		fmt.Println("judged by:", report.Violations[0].Source)
	}

	// Output:
	// root:   Invoice
	// source: PINT
	// CIUS:   PINT
	// judged by: PINT
}

// ExampleCoverage answers "what will that validator not look at?" before a call
// is made. It takes no document and cannot fail.
//
// The severity on each family is what a caller acts on: a fatal gap means a rule
// that could have rejected this document was never evaluated, so Report.Conformant
// cannot be true. NLCIUS is the one rule set here whose only gap is advisory, and
// it is two entries rather than one because SimplerInvoicing publishes a UBL
// binding and a CII binding that do not carry the same rules.
func ExampleCoverage() {
	for _, gap := range formalis.Coverage(formalis.SourceNLCIUS) {
		fmt.Printf("not evaluated: %s [%s] — %s\n", gap.Rules, gap.Severity, gap.Reason)
	}

	// Output:
	// not evaluated: BR-NL-19..21, 24..26, 27-1..27-4, 28-1..28-4, 29..31, 32-1..32-3, 33, 35 (UBL) [warning] — SI-UBL 2.0's "not recommended" rules, which do not make an invoice non-conformant
	// not evaluated: BR-NL-19..26, 27-1..27-4, 28-1..28-4, 29..31, 32-and-34, 33, 35 (CII) [warning] — the same advisory tier in NLCIUS-CII-validation.sch, which publishes two rules the UBL one does not
}

// ExampleIsCheckerViolation separates "the invoice is wrong" from "the checker
// did not judge it". A cancelled run reports a RuleLimit violation rather than
// an empty Violations slice, so it can never be read as a clean invoice.
func ExampleIsCheckerViolation() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := formalis.ValidateCIUS(ctx, []byte(exampleUBL))
	if err != nil {
		fmt.Println("could not read it:", err)
		return
	}
	for _, v := range report.Violations {
		if formalis.IsCheckerViolation(v) {
			fmt.Printf("unknown: %s %s\n", v.Source, v.Rule)
		} else {
			fmt.Printf("defect:  %s %s\n", v.Source, v.Rule)
		}
	}
	fmt.Println("conformant:", report.Conformant())

	// Output:
	// unknown: checker limit
	// conformant: false
}

// ExampleViolation shows why findings must be aggregated on the pair
// (Source, Rule). Two authorities may mint the same identifier, and Rule alone
// is not an identity.
func ExampleViolation() {
	type key struct {
		Source formalis.Source
		Rule   string
	}

	counts := map[key]int{}
	for _, doc := range [][]byte{[]byte(exampleUBL), []byte(`<Invoice/>`)} {
		report, err := formalis.ValidateCIUS(context.Background(), doc)
		if err != nil {
			fmt.Println("could not read it:", err)
			return
		}
		for _, v := range report.Violations {
			counts[key{v.Source, v.Rule}]++
		}
	}

	// "BR-06" means one thing under EN 16931 and could mean another under any
	// other authority; a suppression list keyed on the string alone would
	// suppress both.
	fmt.Println("EN 16931 BR-06:", counts[key{formalis.SourceEN16931, "BR-06"}])
	fmt.Println("Peppol BR-06:  ", counts[key{formalis.SourcePeppol, "BR-06"}])

	// Output:
	// EN 16931 BR-06: 1
	// Peppol BR-06:   0
}

// ExampleIsFacturae shows the three answers every Is* predicate gives. They are
// independent tests rather than a partition — more than one can be true of the
// same bytes — so use Detect to route and these to ask about one format.
func ExampleIsFacturae() {
	for _, doc := range []string{
		`<Facturae><FileHeader/></Facturae>`,
		exampleUBL,
		`<Facturae>`, // truncated: not well-formed
	} {
		ok, err := formalis.IsFacturae([]byte(doc))
		switch {
		case err != nil:
			fmt.Println("could not tell; do not dispatch on this")
		case ok:
			fmt.Println("Facturae")
		default:
			fmt.Println("read it; some other format")
		}
	}

	// Output:
	// Facturae
	// read it; some other format
	// could not tell; do not dispatch on this
}
