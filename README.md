# formalis

A dependency-free (Go standard library only) validator for European electronic
invoices against **EN 16931** and its national **Core Invoice Usage
Specifications (CIUS)**.

One syntax-neutral rule engine validates both XML syntaxes — UN/CEFACT Cross
Industry Invoice (CII, used by Factur-X/ZUGFeRD) and OASIS UBL (Peppol BIS,
XRechnung, NLCIUS) — and each CIUS adds its own rule layer.

## Usage

```go
import "github.com/mgilbir/formalis"

// Validate against a specific profile:
v := formalis.Validate(ctx, xml, formalis.ProfileEN16931)

// Or let the invoice route itself by its CustomizationID (BT-24):
v := formalis.ValidateCIUS(ctx, xml)

for _, x := range v {
    fmt.Printf("%s: %s\n", x.Rule, x.Message)
}
```

Validation is bounded and honours `ctx`. A run that stops early — a cancelled
context, or a guard tripped by a hostile document — reports a `formalis.RuleLimit`
violation rather than returning what it happened to collect, so an empty result
always means "read in full, nothing to report":

```go
for _, x := range v {
    if formalis.IsCheckerViolation(x) {
        // The checker stopped. The invoice is neither valid nor invalid.
    }
}
```

The `Is*` predicates identify a document's format. They answer three ways, not
two: `(true, nil)` yes, `(false, nil)` read it and it is some other format, and a
non-nil error for input that could not be read at all — malformed XML, an
encoding this package does not implement, or a tripped guard. A truncated
Facturae invoice is not the same thing as "not a Facturae invoice", and routing
on the difference is the point:

```go
ok, err := formalis.IsFacturae(xml)
switch {
case err != nil:
    // Could not tell. Do not dispatch on this.
case ok:
    v = formalis.ValidateFacturae(ctx, xml)
}
```

## CIUS coverage

- EN 16931 core (198/198 rules, FP=0 against the CEN unit-test suite)
- Factur-X / ZUGFeRD profiles (MINIMUM … EXTENDED)
- XRechnung (German public-sector CIUS)
- Peppol BIS Billing 3.0
- NLCIUS (Dutch SimplerInvoicing / SI-UBL)

## Tests

`make test` runs the suite; oracle-backed tests skip until their (gitignored)
reference data is fetched — see the `en16931-*` and `cius-oracles` Makefile
targets.
