# formalis

A dependency-free (Go standard library only) validator for electronic invoices.

It checks **EN 16931** and the **Core Invoice Usage Specifications (CIUS)**
layered on it, and it also checks the national invoice formats that are not
EN 16931 profiles at all — Italian FatturaPA, Spanish Facturae, Austrian
ebInterface, Polish KSeF, Finnish Finvoice and TEAPPS, Danish OIOUBL, Swedish
Svefaktura, Hungarian NAV Online Számla, Turkish UBL-TR, Saudi ZATCA, and Peppol
PINT for AE / AUNZ / EU / JP / MY / OM / SG. The Franco-German Order-X order
document is validated too. Twenty-one rule sets in all; the table below names
each one, the entry point that runs it, and the `Source` that scopes its
findings.

One syntax-neutral rule engine validates both EN 16931 syntaxes — UN/CEFACT
Cross Industry Invoice (CII, used by Factur-X/ZUGFeRD) and OASIS UBL (Peppol
BIS, XRechnung, NLCIUS) — and each CIUS adds its own rule layer on top of it.
The formats that are not EN 16931 profiles are checked against their own
mandatory structure and code lists instead.

Every exported function is safe to call from any number of goroutines at once:
the package holds no mutable global state, and `TestValidatorsAreSafeForConcurrentUse`
pins that under `-race`.

## Usage

```go
report, err := formalis.ValidateCIUS(ctx, xml)
if err != nil {
    // The input could not be read at all: malformed XML, or an encoding this
    // package does not implement. Nothing was judged.
    return err
}

for _, v := range report.Fatal() {
    fmt.Printf("%s %s: %s\n", v.Source, v.Rule, v.Message)
}

fmt.Println("nothing found:", len(report.Violations) == 0)
fmt.Println("conformant:   ", report.Conformant())
```

`ValidateCIUS` routes the document against whichever rule set it declares. Every
exported validator has this shape — `func(context.Context, []byte) (Report, error)`
— and `Report` is the whole answer:

```go
type Report struct {
    Violations   []Violation  // what was found
    NotEvaluated []RuleFamily // the rule families that were not evaluated
}

func (r Report) Conformant() bool  // no fatal finding, no fatal gap, run not cut short
func (r Report) Complete() bool    // nothing unevaluated at all, advisory included
func (r Report) Fatal() []Violation
func (r Report) Warnings() []Violation
```

The zero `Report` is deliberately neither `Conformant` nor `Complete`, so a value
nobody filled in — including the one returned beside an error — cannot pass for a
clean invoice.

## Four answers, and the difference between them

The distinctions this package exists for are all in how a call can end:

| Outcome | Means | Read it with |
|---|---|---|
| `error`, zero `Report` | the input could not be read at all — malformed XML, an unimplemented encoding. A statement about the *file* | `errors.Is(err, formalis.ErrMalformedXML)`, `ErrUnsupportedEncoding` |
| a `RuleLimit` finding | the run stopped before it had seen everything — cancelled context, tripped budget. A statement about the *run* | `formalis.IsCheckerViolation(v)` |
| findings | the document departs from rules that were evaluated | `report.Fatal()`, `report.Warnings()` |
| no findings | everything that *was* evaluated passed | `report.NotEvaluated`, `report.Conformant()` |

A run that stopped stays a finding rather than becoming an error, deliberately:
the checks that did complete are still true, and an error would discard them.
A well-formed document that is simply not an invoice is a finding too — `RuleRoot`
for the EN 16931 entry points, `FPA-root`, `ZA-root`, `ORDER-root` … for the
national ones — because the document *was* read.

## Severity

Every `Violation` carries the severity its authority gave the rule: CEN's
`flag="fatal"` or `flag="warning"` in the Schematron, and the national
authorities' equivalents. `SeverityFatal` is the zero value, so an unstamped
finding reads as blocking rather than as advisory — the fail-safe direction.

Today every rule this package implements is one its authority flags fatal, and
that is checked rather than assumed: one test sweeps the whole corpus for a
finding at any other severity, and another reads the flag off each of the 108
EN 16931 rules the corpus exercises straight from the vendored CEN Schematron.

`Coverage`'s families carry a severity too, and that is what makes
`Conformant()` answerable at all: an advisory gap leaves the verdict intact and
only makes the report less informative than a reference validator's, while a
fatal gap means a rule that could have rejected this document was never run.

## What a clean report does and does not mean

**`Conformant()` returns false for every document today.** That is not a bug and
it is the first thing to understand about this package.

`len(report.Violations) == 0` means only *the checks that ran found nothing*. It
is equally true of a run that checked everything, a run that was cancelled or hit
a resource budget, and a run whose rule set does not implement every rule its
authority publishes. No rule set here is in that last sense complete — each
evaluates a documented subset — so `Complete()` is false everywhere. And every
validator's report names at least one gap its authority flags **fatal**, so
`Conformant()` is false too. (NLCIUS is the one rule set whose *own* gap is purely
advisory; every CIUS validator also runs the EN 16931 core, whose two
unimplemented fatal `UBL-CR-*` rules are a fatal gap.)

Rather than hide that behind a number in this file that would drift as rules
land, the package makes it machine-readable:

- **`Coverage(src Source) []RuleFamily`** names the rule families `src` publishes
  and this package does not evaluate, with the severity of each. It takes no
  document, parses nothing and cannot fail, so you can ask *before* deciding to
  trust a validator.
- **`Report.NotEvaluated`** is the same information for the run that just
  happened — the union across every authority that call applied. A validator that
  layers a CIUS on the core reports both sets.
- **`Report.Complete()`** is false when either kind of gap is present: a rule set
  with holes of any severity, or a run that stopped early. **`Conformant()`**
  passes over the advisory holes.

```go
for _, gap := range formalis.Coverage(formalis.SourceNLCIUS) {
    fmt.Printf("not evaluated: %s [%s] — %s\n", gap.Rules, gap.Severity, gap.Reason)
}
// not evaluated: BR-NL-19..35 [warning] — NLCIUS's "not recommended" rules,
//                which do not make an invoice non-conformant
```

Use `Conformant()` when you need the strong claim, and `len(r.Fatal()) == 0` when
the weaker one will do — with `r.NotEvaluated` beside it saying exactly what it
omits.

## Coverage

| Format | Entry point | `Is*` predicate | `Source` |
|---|---|---|---|
| EN 16931 core (CII + UBL, incl. Factur-X/ZUGFeRD) | `Validate` (with a `Profile`), `ValidateCIUS` | — | `SourceEN16931` |
| XRechnung (DE) | `ValidateXRechnung` | — | `SourceXRechnung` |
| Peppol BIS Billing 3.0 | `ValidatePeppol` | — | `SourcePeppol` |
| NLCIUS / SimplerInvoicing (NL) | `ValidateNLCIUS` | — | `SourceNLCIUS` |
| CIUS-PT (PT) | `ValidateCIUSPT` | — | `SourceCIUSPT` |
| CIUS-RO / RO e-Factura (RO) | `ValidateCIUSRO` | — | `SourceCIUSRO` |
| UBL.BE (BE) | `ValidateUBLBE` | — | `SourceUBLBE` |
| SRBDT (RS) | `ValidateSRBDT` | — | `SourceSRBDT` |
| Peppol PINT (AE/AUNZ/EU/JP/MY/OM/SG) | `ValidatePINT` | `IsPINT` | `SourcePINT` |
| FatturaPA / FatturaElettronica (IT) | `ValidateFatturaPA` | `IsFatturaPA` | `SourceFatturaPA` |
| Facturae (ES) | `ValidateFacturae` | `IsFacturae` | `SourceFacturae` |
| ebInterface (AT) | `ValidateEbInterface` | `IsEbInterface` | `SourceEbInterface` |
| KSeF FA (PL) | `ValidateKSeF` | `IsKSeF` | `SourceKSeF` |
| Finvoice (FI) | `ValidateFinvoice` | `IsFinvoice` | `SourceFinvoice` |
| TEAPPSXML (FI) | `ValidateTEAPPS` | `IsTEAPPS` | `SourceTEAPPS` |
| OIOUBL (DK) | `ValidateOIOUBL` | `IsOIOUBL` | `SourceOIOUBL` |
| Svefaktura (SE) | `ValidateSvefaktura` | `IsSvefaktura` | `SourceSvefaktura` |
| ZATCA (SA) | `ValidateZATCA` | `IsZATCA` | `SourceZATCA` |
| NAV Online Számla / OSA (HU) | `ValidateOSA` | `IsOSA` | `SourceOSA` |
| UBL-TR e-Fatura (TR) | `ValidateTurkishInvoice` | `IsTurkishInvoice` | `SourceUBLTR` |
| Order-X (order, not invoice) | `ValidateOrderXML` | — | `SourceOrderX` |

Eight of these are `CIUS` constants — XRechnung, Peppol, NLCIUS, CIUS-PT,
CIUS-RO, UBL.BE, SRBDT and PINT — and `ValidateCIUS` routes to them on the
document's **Specification identifier** (BT-24, what `DetectCIUS` reads). It
routes to OIOUBL and UBL-TR on that same identifier though neither is a CIUS, to
ZATCA on a profile identifier or a document reference, to ebInterface and
Svefaktura on a distinguishing child element, to the remaining formats on their
root element, and otherwise to the EN 16931 core. There is no `Is*` predicate for the EN 16931 core, for the
seven CIUS layered on it, or for Order-X, and none is needed: the CIUS are told
apart by that identifier (`DetectCIUS`, or `Detection.CIUS`), and Order-X by a
root element no other format uses.

`SourceChecker` is a `Source` but not a format: it carries this package's
statements about its own run, or about the file it was handed — `RuleLimit`,
`RuleProfile` and `RuleRoot` — and
`Coverage(SourceChecker)` is nil because it publishes no rules. `SourceNone` is
the zero value, which `Detect` reports for a document it read and recognised as
no format here, and which no `Violation` ever carries.

## Routing

`Detect` is the routing entry point. It reads the document once, without building
a tree, and arbitrates between the formats in an order that is part of its
documented contract:

```go
det, err := formalis.Detect(xml)
switch {
case err != nil:
    // Could not read it. Do not dispatch on this.
case !det.Recognised():
    // Read it; no format this package validates.
default:
    report, err := det.Validator()(ctx, xml)
}
```

`Detection` also carries `SpecID` (BT-24 as the document wrote it, trimmed),
`CIUS` and `Root`, and `Coverage(det.Source)` answers what the validator it named
will not check — before the call.

The twelve `Is*` predicates remain, and each answers about **one** format. They
are independent tests, **not a partition**: more than one can be true of the same
bytes (an `<Invoice>` with both a `Biller` and a `SellerParty` is `IsEbInterface`
*and* `IsSvefaktura`). That is why `Detect` exists — it is the arbitration,
written down once. Each predicate answers three ways:

```go
ok, err := formalis.IsFacturae(xml)
switch {
case err != nil:
    // Malformed XML, an unsupported encoding, or a tripped guard.
    // Not the same thing as "not a Facturae invoice".
case ok:
    report, err := formalis.ValidateFacturae(ctx, xml)
}
```

`Profile` is a Factur-X/ZUGFeRD **data-richness** tier (`MINIMUM`, `BASIC WL`,
`BASIC`, `EN 16931`, `EXTENDED`) and nothing else — it never selects a national
rule set. `ProfileFor` maps a PDF's XMP `ConformanceLevel` to one; `CIUSFor` maps
the levels that name a CIUS instead (today, `"XRECHNUNG"`). A `Profile` this
package does not implement is refused with a `RuleProfile` violation rather than
silently read as EN 16931.

## Rule identity is `(Source, Rule)`

`Source` names the authority that defines a rule; two authorities may mint the
same string. Aggregate and suppress on the pair, never on `Rule` alone:

```go
type key struct {
    Source formalis.Source
    Rule   string
}
counts := map[key]int{}
for _, v := range report.Violations {
    counts[key{v.Source, v.Rule}]++
}
```

Most national formats publish no rule identifier this package could quote, so the
identifiers under those `Source`s — `FPA-*`, `FE-*`, `ZA-*`, `ORDER-*`, … — were
minted here. The `Source` is still the format the document was judged against,
which is what a caller routing or suppressing by format needs; it is not a claim
that the format's own documentation uses these names. The `Source`s whose
identifiers *are* quoted from a published rule set are EN 16931, XRechnung,
Peppol, NLCIUS, CIUS-PT, CIUS-RO, UBL.BE and SRBDT.

## Bounded work

Validation honours `ctx` and is bounded by the package's own limits on document
depth and element count, so the cost of a call is not set by a hostile document.
A run that stops early — a cancelled context, a tripped budget — reports a
`RuleLimit` violation rather than an empty `Violations` slice, so it can never be
read as a clean invoice:

```go
for _, v := range report.Violations {
    if formalis.IsCheckerViolation(v) {
        // The checker did not judge this document.
        // Neither conformant nor non-conformant: unknown.
    }
}
```

`RuleRoot` is deliberately not a checker violation: "this document is not an
EN 16931 invoice" is a definite answer about a document that *was* read. Neither
is the error an unreadable document produces, which does not arrive as a finding
at all.

## Examples

Runnable versions of everything above live in
[`example_test.go`](example_test.go) as Go `Example` functions, written against
the exported API from outside the package. `go test` compiles them and checks
their output, so an example that has drifted from the API fails the build rather
than misleading a reader. `go doc` shows them alongside the symbols they
document.

## Tests

`make test` runs the suite. Without the reference corpora it exercises the rule
engine against hand-written fixtures and skips the oracle-backed tests; that is
the mode a clean checkout and the fast CI job run in, and it is green.

The oracles need corpora this repository does not vendor (`testdata/` is
gitignored). Fetching them needs **`git`**, **`bash`**, **`curl`**, **`python3`**
— several Romanian and Portuguese sample filenames are non-ASCII and are
URL-encoded with it — and **`gh` authenticated against GitHub**, because the
fetch makes about fifteen `gh api` calls and the unauthenticated rate limit of 60
requests an hour is not enough for them. `make check-deps` verifies all of this
up front and names what is missing, rather than failing two hundred lines into a
download.

```
make check-deps                    # what the fetch targets need
make cius-oracles                  # ~600 documents: XRechnung, Peppol, NLCIUS,
                                   # the CIUS and the national-format samples
make en16931-artefacts             # the CEN/TC 434 per-rule unit-test suite
make en16931-ubl                   # the EN 16931 UBL example invoices
make en16931-genericode            # the official code lists (needs unzip)
make test
```

Each target is stamped, so re-running it is a no-op rather than an error; the
matching `clean-*` target removes the stamp and the data, and is how you force a
re-fetch. `make en16931-codelists` goes one step further and *regenerates* the
committed code-list tables from the genericode with `gen.py` — a deliberate act,
not part of running the tests, which is why fetching the oracle is its own
target.

With the corpora present the suite runs with no skips, and each oracle ratchets
the number of documents it saw — the constants are collected in
[`corpus_test.go`](corpus_test.go). A corpus that arrives truncated therefore
fails the build rather than reporting a clean verdict over whatever landed, and
the fetch itself fails on the first download it cannot complete.
