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

Each was checked against every `.xml` under `testdata/` — 1,805 of them, not only
`examples/` — comparing whitespace-normalised content, and again on invoice
identity (BT-1, BT-2, the seller name and the grand total) so that a document
that had been reserialised somewhere could not slip through a byte comparison.
None of the six duplicates anything fetched.

The selection is the reason there are only six. The container corpus these came
from holds 75 PDFs, 74 of which yield an invoice: 5 MINIMUM, 3 BASIC WL, 5 BASIC,
31 EN 16931, 26 EXTENDED and 4 XRechnung. **59 of the 74 are duplicates of
documents already fetched** — the FNFE specification bundle publishes most of
them as bare XML too — and 15 are not. Of those 15, six are lean-tier and are
here; the other nine are eight EN 16931 and one EXTENDED, which are the two tiers
`examples/` already covers 23 and 25 deep. They are worth having one day and are
not worth a second exception to the gitignore today.

## What the tests assert about them

`facturx_lean_tiers_test.go` holds them as an **expected-failure table**. Each
document carries the tier its own BT-24 declares, every fatal finding it draws
with the node that finding fires at, and a written reason naming what in the
document causes it and what FNFE's rule says. That is shape (1) of the three
[#61](https://github.com/mgilbir/formalis/issues/61) offers: it keeps the
documents that disagree without claiming they pass. It needs no corpus, so it
runs in CI's corpus-less job too.

Three tests hold it up:

- `TestFacturXLeanTierSamplesDrawExactlyTheseFindings` compares (rule, node)
  pairs and fails **in either direction** — a finding that appears is as
  interesting as one that vanishes, and the vanishing case is the more dangerous:
  it would mean a rule that is present, reachable and now inert. Comparing nodes
  rather than a rule list is what makes fourteen findings becoming fourteen
  *different* findings red.
- `TestFacturXLeanTierExpectedFailuresHaveTheStatedCause` checks the reasons. Each
  row's stated cause is looked up in the generated data-model table and has to be
  the shape FNFE publishes for that rule, and each is checked against the document
  decoded with `encoding/xml` — independently of the parser that produced the
  finding. It also closes the currency claim over the whole file: every
  `@currencyID` anywhere in a document is either on `ram:TaxTotalAmount` or is
  accounted for by exactly one row.
- `TestFacturXForbidsRestatingTheCurrencyOnEveryAmountButTaxTotalAmount` reads the
  design those reasons appeal to out of all five profile tables.

```
                       tier       fatal  rules
fnfe_BASIC.xml         BASIC         14  FX-DM-BASIC-0018/0107/0108/0182/0183/0184/0185/0189/0224/0259
fnfe_MINIMUM.xml       MINIMUM        5  FX-DM-MINIMUM-0019/0022/0043/0044/0045
fnfe_MINIMUM_UE.xml    MINIMUM        5  FX-DM-MINIMUM-0019/0022/0043/0044/0045
intarsys_MINIMUM.xml   MINIMUM        1  FX-DM-MINIMUM-0019
intarsys_BASIC.xml     BASIC          0
mustang_BASICWL_avoir.xml  BASIC WL   0

MINIMUM: 3 documents, 11 findings · BASIC: 2 documents, 14 · BASIC WL: 1, 0
```

The findings are correct rather than false, and each was read back out of the
vendored profile Schematron:

- **Nineteen** are `report @currencyID` on an amount. The attribute is not
  optional-and-omitted at these tiers: the element table excludes it, because an
  invoice is denominated once as BT-5 `ram:InvoiceCurrencyCode` and a per-amount
  restatement adds nothing while letting one document carry two answers. The one
  exception is `ram:TaxTotalAmount`, where FNFE *constrains* `@currencyID` against
  the currency code list instead of forbidding it — the one place two amounts in
  two currencies legitimately coexist, BT-110 and BT-111. CEN reached the same
  design independently, carving `ram:TaxTotalAmount` out of `CII-DT-031` by name.
  All six documents here carry the attribute on their `ram:TaxTotalAmount` and
  draw nothing for it.
- **Five** are `report true()`: the buyer's `ram:PostalTradeAddress` and
  `ram:SpecifiedTaxRegistration` at MINIMUM, a head-only tier whose element table
  has neither. Presence is the whole of the offence. The evidence for these is the
  artefact alone. It is worth being precise about what is *not* evidence: the two
  fetched MINIMUM documents carry no buyer address, and that corroborates nothing
  — a `report true()` rule fires on presence, so a document without the element
  never reaches the rule. What those two documents *do* corroborate is the
  `@currencyID` claim: each carries the attribute on `ram:TaxTotalAmount` and on
  no other amount, which is exactly what `FX-DM-MINIMUM-0043/0044/0045` require
  and what FNFE's own MINIMUM samples do not do.
- **One**, `FX-DM-BASIC-0018`, is the code-database lookup on BT-24 itself.
  `FACTUR-X_BASIC_codedb.xml` enumerates exactly two values and
  `Avoir_FR_type381_BASIC` declares neither:
  `urn:cen.eu:en16931:2017:compliant:factur-x.eu:1p0:basic`, which writes the
  separators as `:` where both published identifiers write `#` and drops the inner
  `urn:` scheme. It names the profile the code list permits and spells it a way
  the code list does not hold — which is also why routing still reads it as BASIC:
  `facturXProfileFromSpecID` matches the brand stem as a substring, while the
  code-list assertion is an exact lookup. `intarsys_BASIC.xml` is the same tier
  with the identifier written correctly and draws nothing, so the rule
  discriminates rather than accusing.

So the reading is that some published sample invoices depart from the published
data model at the leanest tiers.

### One open question, recorded and not acted on

MINIMUM selects one of its three `ram:TaxTotalAmount` rules on
`@currencyID=../../ram:TaxCurrencyCode` — BT-6, the accounting currency — at a
tier whose purpose is a reduced head-only summary, and it is the only one of the
five profiles that references BT-6 without defining it: the other four give
`ram:TaxCurrencyCode` a rule of its own, MINIMUM gives it none. Whether the tier
genuinely admits a second currency or the predicate is copy-paste from the richer
profiles is unresolved. No document decides it — of the 65 Factur-X documents in
this tree exactly one carries a BT-6, `X07_01_Fremdwaehrung.xml`, and it is
EXTENDED. `TestMinimumKeysARuleOnACurrencyItsOwnTierNeverDefines` pins the
asymmetry so a revision resolving it arrives as a question rather than as silence.

These are **not** in `TestAuthoritySamplesDrawNoFatalFinding`'s population, and
that guard is unchanged. Its Factur-X entry judges the documents FNFE's own
valitool passes, read out of the `*_fx_validation_report.xml` beside each example
— eight of the 59 examples have no report and are already not judged there for
the same reason. These six have no verdict from FNFE either; ZUGFeRD/corpus files
them under `correct/`, but that is a third party's classification of a PDF
container rather than the authority saying the invoice inside passes its business
rules. The argument is written out in `facturx_lean_tiers_test.go`.

### One thing they found

Two of the six declare ZUGFeRD-branded identifiers (`urn:zugferd.de:2p0:minimum`,
`urn:cen.eu:en16931:2017#compliant#urn:zugferd.de:2p0:basic`). Factur-X 1.0 and
ZUGFeRD 2.x are one specification under two brands, and FNFE's own code database
enumerates both for every tier that names itself — but `specIDRules` matched only
`factur-x.eu`, so `intarsys_MINIMUM.xml` was routed to CEN's EN 16931 CII binding
and accused of `BR-16` and `BR-CO-18`: an invoice line and a VAT breakdown, at a
head-only tier that has neither by design. That is C44 in the German half of the
identifier space, and it was live. Both brands are matched now, and
`TestFacturXRoutingAcceptsEveryIdentifierTheAuthorityPublishes` reads the pairs
back out of the code lists so a third cannot be missed the same way.

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
