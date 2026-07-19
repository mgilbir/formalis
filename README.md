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
v := formalis.Validate(xml, formalis.ProfileEN16931)

// Or let the invoice route itself by its CustomizationID (BT-24):
v := formalis.ValidateCIUS(xml)

for _, x := range v {
    fmt.Printf("%s: %s\n", x.Rule, x.Message)
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
