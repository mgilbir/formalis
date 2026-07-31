# Factur-X invoices extracted from their PDF containers

Six invoice XMLs, committed rather than fetched. Everything else under
`testdata/` is downloaded by a `make` target and gitignored; these are the
exception, for one reason:

**They exist only inside PDF/A-3 files, and this package cannot open a PDF.**

A Factur-X document is a PDF with the invoice XML attached to it. For most of
the corpus that does not matter, because the publisher also ships the bare XML —
`make facturx-examples` takes those. These six are published only as complete
Factur-X PDFs, so a fetch target could download them and still have nothing this
package can read. Extracting them needs a PDF parser, which is deliberately not
a dependency here.

## Why these six

They are the leanest tiers, which the fetched corpus barely covers. Counting the
specification identifier each document declares, `examples/` holds 25 EXTENDED
and 23 EN 16931 against 2 MINIMUM and 2 BASIC WL.

That gap matters more for the Factur-X rule set than it would for CEN's, because
`FX-DM-*` is *per tier*: `FX-DM-MINIMUM-0019` and `FX-DM-BASIC-0107` are
different rules with different contexts, and no document of another tier can
reach either. These add three MINIMUM, two BASIC and one BASIC WL.

| file | tier | source |
|---|---|---|
| `fnfe_MINIMUM.xml` | MINIMUM | ZUGFeRD/corpus `ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_FR_MINIMUM.pdf` |
| `fnfe_MINIMUM_UE.xml` | MINIMUM | ZUGFeRD/corpus `ZUGFeRDv2/correct/FNFE-factur-x-examples/Facture_UE_MINIMUM.pdf` |
| `intarsys_MINIMUM.xml` | MINIMUM | ZUGFeRD/corpus `ZUGFeRDv2/correct/intarsys/MINIMUM/zugferd_2p0_MINIMUM.pdf` |
| `fnfe_BASIC.xml` | BASIC | ZUGFeRD/corpus `ZUGFeRDv2/correct/FNFE-factur-x-examples/Avoir_FR_type381_BASIC.pdf` |
| `intarsys_BASIC.xml` | BASIC | ZUGFeRD/corpus `ZUGFeRDv2/correct/intarsys/BASIC/zugferd_2p0_BASIC_Einfach.pdf` |
| `mustang_BASICWL_avoir.xml` | BASIC WL | ZUGFeRD/mustangproject `validator/src/test/resources/validAvoir_FR_type380_BASICWL.pdf` |

Both repositories are Apache-2.0. The XML is byte-identical to the stream inside
the PDF — extraction copies the attachment out, it does not reserialise.

Each was checked against every document already in `examples/`, comparing
whitespace-normalised content, so none of these duplicates one that is fetched.
Eleven further documents were extracted from the same PDFs and dropped for
exactly that reason.

## They are not wired into any test

Nothing globs this directory. That is deliberate: **four of the six draw fatal
findings**, and deciding what the corpus should assert about them is
[#61](https://github.com/mgilbir/formalis/issues/61), not something this
directory settles.

```
fnfe_BASIC.xml         BASIC     FX-DM-BASIC-0018/0107/0108/0182/0183/0184/0185/0189/0224/0259
fnfe_MINIMUM.xml       MINIMUM   FX-DM-MINIMUM-0019/0022/0043/0044/0045
fnfe_MINIMUM_UE.xml    MINIMUM   FX-DM-MINIMUM-0019/0022/0043/0044/0045
intarsys_MINIMUM.xml   MINIMUM   FX-DM-MINIMUM-0019
```

The findings look correct rather than false, on two independent grounds. The
`@currencyID` ones are what CEN's `CII-DT-031` reported before v0.3.0 narrowed
the binding — two authorities reaching the same conclusion by different routes.
And `FX-DM-MINIMUM-0019` says a buyer `PostalTradeAddress` is unused at MINIMUM,
which is what both fetched MINIMUM documents do: neither carries one.

So the reading is that some published sample invoices depart from the published
data model at the leanest tiers. Adding these to `examples/` would turn
`TestAuthoritySamplesDrawNoFatalFinding` red on the day it landed, which is the
call #61 exists to make — take them with a per-tier expected-findings ratchet,
take only the two that are clean, or leave the corpus alone and record the
divergence in the coverage tables.

## Reproducing the extraction

The containers come from the same two Apache-2.0 repositories the fetched corpus
uses. Any PDF library that can read an embedded file will do; these were taken
with `github.com/mgilbir/pdf0`, whose `ValidateFacturX` returns the attachment it
found:

```go
data, _ := os.ReadFile("fnfe_MINIMUM.pdf")
doc, _ := pdf0.Read(bytes.NewReader(data), int64(len(data)))
os.WriteFile("fnfe_MINIMUM.xml", pdf0.ValidateFacturX(doc, data).XML, 0o644)
```
