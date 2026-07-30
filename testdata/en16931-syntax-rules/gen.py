#!/usr/bin/env python3
"""Generate en16931_syntax_advisory_table.go from the CEN/TC 434 Schematron.

Run `make en16931-artefacts` first (it clones the gitignored
testdata/en16931-artefacts/). Then `python3 gen.py` rewrites
../../en16931_syntax_advisory_table.go with CEN's abstract *syntax* pattern for
each binding: every <rule> in pattern order, and under each rule the assertions
CEN flags warning. en16931_syntax_advisory_test.go re-derives the same tables
from the Schematron and asserts the committed file still matches, so the tables
cannot drift from the source of truth.

What is emitted, and what is not
--------------------------------
The four families this covers — UBL-CR-*, UBL-DT-*, CII-SR-*, CII-DT-* — are
CEN's syntax bindings, and each family is split by flag: the fatal half is
transcribed by hand in en16931_ubl_rules.go and en16931_cii_rules.go, where the
rules that need judgement live, and the advisory half is what this generates.
Only flag="warning" assertions are emitted.

Every <rule> is emitted whether or not it carries an advisory assertion,
because a rule with none can still *shadow* one that does. Under ISO Schematron
a node is processed by the first rule in a pattern whose context matches it, so
an entry with no assertions is not dead weight: it is what stops the evaluator
reporting a rule CEN's own processor never reaches. That is how
CII-DT-010/011/012 come to be unreportable — //ram:TypeCode precedes
/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:TypeCode — and modelling the
ordering means that fact falls out of the table rather than having to be
remembered.

Failing loudly
--------------
Two things could be silently dropped here, and both abort the run instead:

  * a <rule context> this generator has no matcher for. CONTEXTS below maps
    every context CEN's two syntax patterns use onto the matcher fields the Go
    evaluator understands. A context that is not in it is a context whose node
    population this generator cannot describe, and emitting the rule without it
    would silently either suppress or over-report every assertion under it.

  * an assertion whose XPath is outside the subset the evaluator implements.
    check_expr below parses each one with the same grammar the Go side parses,
    and an expression it cannot parse aborts the run naming the rule. The Go
    side parses the emitted table again at load and its own test asserts every
    row parses, so the gate is closed from both ends: this generator refuses to
    write a rule it cannot describe, and the package refuses to ship a table it
    cannot read. A rule quietly skipped by a generator is how two fatal
    UBL-CR-* rules came to sit inside a coverage entry describing their family
    as advisory.
"""
import os
import re
import sys
import xml.etree.ElementTree as ET

SCH = "{http://purl.oclc.org/dsdl/schematron}"
HERE = os.path.dirname(os.path.abspath(__file__))
ART = os.path.abspath(os.path.join(HERE, "..", "en16931-artefacts"))
OUT = os.path.abspath(os.path.join(HERE, "..", "..", "en16931_syntax_advisory_table.go"))

# The two bindings: Go table name, abstract pattern, binding pattern, and the
# families whose advisory half is emitted from it.
BINDINGS = [
    ("advisoryUBLPattern", "UBL",
     "ubl/schematron/abstract/EN16931-syntax.sch",
     "ubl/schematron/UBL/EN16931-UBL-syntax.sch",
     ("UBL-CR-", "UBL-DT-")),
    ("advisoryCIIPattern", "CII",
     "cii/schematron/abstract/EN16931-CII-syntax.sch",
     "cii/schematron/CII/EN16931-CII-syntax.sch",
     ("CII-SR-", "CII-DT-")),
]


def norm(s):
    """Collapse whitespace, the way the Go fidelity test does.

    CEN's own strings carry incidental whitespace — a trailing space on
    `//cac:PostalAddress | //cac:Address `, a run of spaces inside
    `not(ram:TaxPointDate)  or (...)`, newlines and indentation inside the
    four-way union $IDTypeNoAttributes. Normalising both sides means the table
    holds one canonical form and the fidelity test compares like with like,
    without the comparison turning on an editor's reflow.
    """
    return " ".join(s.split())


# ---------------------------------------------------------------------------
# Contexts
#
# Every <rule context> in CEN's two syntax patterns, keyed by its resolved and
# whitespace-normalised XPath, mapped onto the fields of the Go advisoryMatch.
# Read the Go type's comment for what each field means. A context missing from
# here aborts the run.
#
# These are XSLT match patterns rather than paths: `cac:Delivery` matches a
# cac:Delivery anywhere in the document, not one under the document element, and
# the matchers follow that. Only the ones CEN writes as an absolute path from the
# document element are anchored, through "paths".
#
# parseCII keys on local names and discards namespaces, so no prefix survives
# into a matcher. That loses the `ram:*` restriction on five CII contexts
# (`//ram:*[ends-with(name(), 'ID')]` and its siblings); over the whole
# conformance corpus every element in a CrossIndustryInvoice whose local name
# ends in ID, Amount, Quantity, TradeTax or ReferencedDocument is in the ram
# namespace, so the restriction selects nothing the local-name test does not.
# ---------------------------------------------------------------------------
CONTEXTS = {
    # --- UBL ---------------------------------------------------------------
    "//cac:PostalAddress | //cac:Address": {"names": ["PostalAddress", "Address"]},
    "cac:AccountingSupplierParty/cac:Party": {"names": ["Party"], "parent": "AccountingSupplierParty"},
    "cac:AdditionalDocumentReference": {"names": ["AdditionalDocumentReference"]},
    "//*[ends-with(name(), 'Amount') and not(ends-with(name(),'PriceAmount')) and not(ancestor::cac:Price/cac:AllowanceCharge)]":
        {"suffix": "Amount", "notSuffix": "PriceAmount", "notAncestorChild": ["Price", "AllowanceCharge"]},
    "//*[ends-with(name(), 'BinaryObject')]": {"suffix": "BinaryObject"},
    "cac:Delivery": {"names": ["Delivery"]},
    "cac:AllowanceCharge[cbc:ChargeIndicator = false()]": {"names": ["AllowanceCharge"], "pred": "predChargeIndicatorFalse"},
    "cac:AllowanceCharge[cbc:ChargeIndicator = true()]": {"names": ["AllowanceCharge"], "pred": "predChargeIndicatorTrue"},
    "cac:PartyTaxScheme": {"names": ["PartyTaxScheme"]},
    "/ubl:Invoice | /cn:CreditNote": {"names": ["Invoice", "CreditNote"], "documentElement": True},
    "cac:InvoiceLine | cac:CreditNoteLine": {"names": ["InvoiceLine", "CreditNoteLine"]},
    "cac:PayeeParty": {"names": ["PayeeParty"]},
    "cac:PaymentMeans": {"names": ["PaymentMeans"]},
    "cac:BillingReference": {"names": ["BillingReference"]},
    "cac:TaxRepresentativeParty": {"names": ["TaxRepresentativeParty"]},
    "cac:TaxSubtotal": {"names": ["TaxSubtotal"]},

    # --- CII ---------------------------------------------------------------
    "//ram:SpecifiedTradeSettlementPaymentMeans": {"names": ["SpecifiedTradeSettlementPaymentMeans"]},
    "/rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext":
        {"paths": [["CrossIndustryInvoice", "ExchangedDocumentContext"]]},
    "/rsm:CrossIndustryInvoice/rsm:ExchangedDocument":
        {"paths": [["CrossIndustryInvoice", "ExchangedDocument"]]},
    "/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:IncludedNote":
        {"paths": [["CrossIndustryInvoice", "ExchangedDocument", "IncludedNote"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:AssociatedDocumentLineDocument":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "AssociatedDocumentLineDocument"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedTradeProduct":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "SpecifiedTradeProduct"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedTradeProduct/ram:ApplicableProductCharacteristic":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "SpecifiedTradeProduct", "ApplicableProductCharacteristic"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeAgreement":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "SpecifiedLineTradeAgreement"]]},
    "//ram:SpecifiedTradeAllowanceCharge": {"names": ["SpecifiedTradeAllowanceCharge"]},
    "//ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge":
        {"names": ["AppliedTradeAllowanceCharge"], "parent": "GrossPriceProductTradePrice"},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeDelivery":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "SpecifiedLineTradeDelivery"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeSettlement":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "SpecifiedLineTradeSettlement"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeAgreement":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeDelivery":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "ApplicableHeaderTradeDelivery"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeSettlementHeaderMonetarySummation":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement",
                    "SpecifiedTradeSettlementHeaderMonetarySummation"]]},
    "/rsm:CrossIndustryInvoice": {"names": ["CrossIndustryInvoice"], "documentElement": True},
    "//*[ends-with(name(), 'DocumentContextParameter')]": {"suffix": "DocumentContextParameter"},
    "/rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext/ram:GuidelineSpecifiedDocumentContextParameter/ram:ID | "
    "/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:ID | "
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:AssociatedDocumentLineDocument/ram:LineID | "
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedTradeProduct/ram:SellerAssignedID":
        {"paths": [
            ["CrossIndustryInvoice", "ExchangedDocumentContext", "GuidelineSpecifiedDocumentContextParameter", "ID"],
            ["CrossIndustryInvoice", "ExchangedDocument", "ID"],
            ["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
             "AssociatedDocumentLineDocument", "LineID"],
            ["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
             "SpecifiedTradeProduct", "SellerAssignedID"]]},
    "//ram:*[ends-with(name(), 'ID')]": {"suffix": "ID"},
    "//ram:TypeCode": {"names": ["TypeCode"]},
    "/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:TypeCode":
        {"paths": [["CrossIndustryInvoice", "ExchangedDocument", "TypeCode"]]},
    "/rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/ram:SpecifiedLineTradeSettlement/ram:ApplicableTradeTax/ram:CategoryCode":
        {"paths": [["CrossIndustryInvoice", "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem",
                    "SpecifiedLineTradeSettlement", "ApplicableTradeTax", "CategoryCode"]]},
    "//ram:*[ends-with(name(), 'ReferencedDocument')]": {"suffix": "ReferencedDocument"},
    "//ram:*[ends-with(name(), 'Amount') and not (self::ram:TaxTotalAmount)]":
        {"suffix": "Amount", "notNames": ["TaxTotalAmount"]},
    "//ram:*[ends-with(name(), 'Quantity')]": {"suffix": "Quantity"},
    "//ram:*[ends-with(name(), 'TradeTax')]": {"suffix": "TradeTax"},
    "//ram:BillingSpecifiedPeriod": {"names": ["BillingSpecifiedPeriod"]},
    "//ram:PostalTradeAddress": {"names": ["PostalTradeAddress"]},
    "//udt:DateTimeString[@format = '102']": {"names": ["DateTimeString"], "pred": "predFormat102"},
}

# The order advisoryMatch's fields are emitted in, so a regenerated table diffs
# cleanly against the last one.
MATCH_FIELDS = ["names", "notNames", "suffix", "notSuffix", "parent", "paths",
                "documentElement", "pred", "notAncestorChild"]


# ---------------------------------------------------------------------------
# The expression grammar
#
# A validator for the XPath subset the Go evaluator implements, run over every
# assertion this generator is about to emit. It computes nothing: its only job is
# to refuse. The Go parser is the one that evaluates, and its own test parses the
# whole committed table, so an expression that slips past this one is caught
# there rather than mis-evaluated.
# ---------------------------------------------------------------------------
TOKEN = re.compile(r"""
      (?P<ws>\s+)
    | (?P<lit>'[^']*')
    | (?P<num>[0-9]+(?:\.[0-9]+)?)
    | (?P<axis>self::|ancestor::)
    | (?P<name>[A-Za-z_][\w.-]*(?::[A-Za-z_*][\w.-]*)?)
    | (?P<op><=|>=|!=|=|<|>|\.\.|//|/|\(|\)|\[|\]|@|\||-)
""", re.X)

RELOPS = {"<=", ">=", "<", ">"}


class Refuse(Exception):
    pass


def lex(s):
    out, i = [], 0
    while i < len(s):
        m = TOKEN.match(s, i)
        if not m:
            raise Refuse(f"cannot tokenise at offset {i}: {s[i:i + 20]!r}")
        i = m.end()
        if m.lastgroup == "ws":
            continue
        out.append((m.lastgroup, m.group()))
    out.append(("eof", ""))
    return out


class P:
    def __init__(self, toks):
        self.t, self.i = toks, 0

    def peek(self, n=0):
        return self.t[min(self.i + n, len(self.t) - 1)]

    def next(self):
        tok = self.t[self.i]
        self.i += 1
        return tok

    def eat(self, val):
        if self.peek()[1] != val:
            raise Refuse(f"expected {val!r}, found {self.peek()[1]!r}")
        self.next()

    def at(self, val):
        return self.peek()[1] == val

    # Expr -> OrExpr
    def expr(self):
        self.and_expr()
        while self.at("or"):
            self.next()
            self.and_expr()

    def and_expr(self):
        self.cmp_expr()
        while self.at("and"):
            self.next()
            self.cmp_expr()

    # CmpExpr -> Additive [ ('='|'!='|RelOp) Additive ]
    def cmp_expr(self):
        self.additive()
        if self.peek()[0] == "op" and self.peek()[1] in ({"=", "!="} | RELOPS):
            self.next()
            self.additive()

    def additive(self):
        self.unary()
        while self.at("-"):
            self.next()
            self.unary()

    def unary(self):
        kind, val = self.peek()
        if val == "not" and self.peek(1)[1] == "(":
            self.next()
            self.eat("(")
            self.expr()
            self.eat(")")
            return
        if val in ("count", "normalize-space") and self.peek(1)[1] == "(":
            self.next()
            self.eat("(")
            self.path()
            self.eat(")")
            return
        if val in ("true", "false") and self.peek(1)[1] == "(":
            self.next()
            self.eat("(")
            self.eat(")")
            return
        if kind in ("lit", "num"):
            self.next()
            return
        if val == "(" and not self.union_ahead():
            self.next()
            self.expr()
            self.eat(")")
            return
        self.path()

    def union_ahead(self):
        """A '(' that opens a union of name alternatives rather than a subexpression."""
        return self.peek(1)[0] == "name" and self.peek(2)[1] == "|"

    # PathExpr -> [ '//' ] [ '..' '/' ] ( Axis | Union | Step ) ( '/' Step )*
    def path(self):
        if self.at("//"):
            self.next()
        elif self.at(".."):
            self.next()
            self.eat("/")
        if self.peek()[0] == "axis":
            self.next()
            if self.peek()[0] != "name":
                raise Refuse(f"an axis must be followed by a name, found {self.peek()[1]!r}")
            self.next()
            return
        if self.at("(") and self.union_ahead():
            self.next()
            self.next()
            while self.at("|"):
                self.next()
                if self.peek()[0] != "name":
                    raise Refuse("a union alternative must be a name")
                self.next()
            self.eat(")")
        else:
            self.step()
        while self.at("/"):
            self.next()
            self.step()

    def step(self):
        if self.at("@"):
            self.next()
            if self.peek()[0] != "name":
                raise Refuse("@ must be followed by an attribute name")
            self.next()
            return
        if self.peek()[0] != "name":
            raise Refuse(f"expected an element name, found {self.peek()[1]!r}")
        self.next()
        if self.at("["):
            self.next()
            self.expr()
            self.eat("]")


def check_expr(rule, xpath):
    try:
        p = P(lex(xpath))
        p.expr()
        if p.peek()[0] != "eof":
            raise Refuse(f"trailing input at {p.peek()[1]!r}")
    except Refuse as e:
        sys.exit(f"gen.py: {rule}: cannot express {xpath!r}: {e}\n"
                 f"  This is a rule the evaluator would silently not check. Extend the grammar in\n"
                 f"  gen.py and in en16931_syntax_advisory.go, or record the rule in\n"
                 f"  Coverage(SourceEN16931) as an unevaluated gap. Do not drop it.")


# ---------------------------------------------------------------------------
# Reading the Schematron
# ---------------------------------------------------------------------------
MSG_PREFIX = re.compile(r"^\[[^]]*\]\s*-?\s*")


def read_pattern(abstract, binding, families):
    """Return CEN's abstract syntax pattern with its params resolved.

    The abstract file holds the rules, their contexts and the flags; the binding
    file holds the XPath each context and each assertion resolves to. Both halves
    are needed, and the pair is what the preprocessed Schematron a reference
    validator runs is built from.
    """
    params = {e.get("name"): e.get("value")
              for e in ET.parse(binding).getroot().iter(SCH + "param")}

    def resolve(v, what):
        v = v.strip()
        if not v.startswith("$"):
            return v
        key = v[1:]
        if key not in params:
            sys.exit(f"gen.py: {what} refers to the parameter ${key}, which "
                     f"{os.path.basename(binding)} does not define")
        return params[key]

    out = []
    for r in ET.parse(abstract).getroot().findall(SCH + "rule"):
        ctx_raw = r.get("context")
        ctx = norm(resolve(ctx_raw, f"the context {ctx_raw!r}"))
        if ctx not in CONTEXTS:
            sys.exit(f"gen.py: no matcher for the rule context {ctx!r}\n"
                     f"  Every context in CEN's syntax pattern has to be described, including one\n"
                     f"  carrying no advisory assertion: under ISO Schematron it can still claim a\n"
                     f"  node away from a later rule that does. Add it to CONTEXTS in gen.py and to\n"
                     f"  the advisoryMatch documentation in en16931_syntax_advisory.go.")
        held, adv = [], []
        for a in r.findall(SCH + "assert"):
            rid, flag = a.get("id"), a.get("flag")
            held.append(f"{rid} ({flag})")
            if flag != "warning":
                continue
            if not any(rid.startswith(f) for f in families):
                sys.exit(f"gen.py: {rid} is flagged warning in {os.path.basename(abstract)} but is "
                         f"in none of the families {families}; the split by family is what keeps "
                         f"the emitted set the one Coverage(SourceEN16931) describes")
            xp = norm(resolve(a.get("test"), f"the assertion {rid}"))
            check_expr(rid, xp)
            msg = MSG_PREFIX.sub("", norm(a.text or ""))
            if not msg:
                sys.exit(f"gen.py: {rid} has no assertion text to report")
            adv.append((rid, xp, msg))
        out.append((ctx, adv, held))
    return out


# ---------------------------------------------------------------------------
# Emitting Go
# ---------------------------------------------------------------------------
def golit(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def strslice(vals):
    return "[]string{" + ", ".join(golit(v) for v in vals) + "}"


def emit_match(ctx):
    m = CONTEXTS[ctx]
    unknown = set(m) - set(MATCH_FIELDS)
    if unknown:
        sys.exit(f"gen.py: the matcher for {ctx!r} sets the unknown field(s) {sorted(unknown)}")
    parts = []
    for f in MATCH_FIELDS:
        if f not in m:
            continue
        v = m[f]
        if f in ("names", "notNames"):
            parts.append(f"{f}: {strslice(v)}")
        elif f in ("suffix", "notSuffix", "parent"):
            parts.append(f"{f}: {golit(v)}")
        elif f == "paths":
            parts.append(f"paths: [][]string{{" + ", ".join(strslice(p) for p in v) + "}")
        elif f == "documentElement":
            parts.append("documentElement: true")
        elif f == "pred":
            parts.append(f"pred: {v}")
        elif f == "notAncestorChild":
            parts.append(f'notAncestorChild: [2]string{{{golit(v[0])}, {golit(v[1])}}}')
    return "advisoryMatch{" + ", ".join(parts) + "}"


def emit_pattern(name, syntax, rules):
    adv = sum(len(a) for _, a, _ in rules)
    lines = [
        f"// {name} is CEN's abstract syntax pattern as bound to {syntax}, in pattern order:",
        f"// {len(rules)} rules carrying the {adv} assertions CEN flags warning. A rule with no",
        "// assertion is here for its context alone, which under ISO Schematron claims its nodes",
        "// away from every rule below it.",
        f"var {name} = []advisorySyntaxRule{{",
    ]
    for ctx, adv_asserts, held in rules:
        lines.append("\t{")
        lines.append(f"\t\tcontext: {golit(ctx)},")
        lines.append(f"\t\tmatch:   {emit_match(ctx)},")
        if adv_asserts:
            lines.append("\t\tasserts: []advisorySyntaxAssert{")
            for rid, xp, msg in adv_asserts:
                lines.append(f"\t\t\t{{{golit(rid)}, {golit(xp)}, {golit(msg)}}},")
            lines.append("\t\t},")
        else:
            lines.append("\t\t// No advisory assertion: " + ", ".join(held) + ".")
        lines.append("\t},")
    lines.append("}")
    return "\n".join(lines)


def main():
    if not os.path.isdir(ART):
        sys.exit(f"gen.py: {ART} is not present; run `make en16931-artefacts` first")
    blocks, counts = [], []
    for name, syntax, abstract, binding, families in BINDINGS:
        rules = read_pattern(os.path.join(ART, abstract), os.path.join(ART, binding), families)
        blocks.append(emit_pattern(name, syntax, rules))
        counts.append((name, len(rules), sum(len(a) for _, a, _ in rules)))
    hdr = """package formalis

// Code generated from the CEN/TC 434 EN 16931 Schematron by
// testdata/en16931-syntax-rules/gen.py; DO NOT EDIT. Regenerate with
// `make en16931-syntax-rules`.
//
// These are the advisory halves of CEN's two syntax bindings — the assertions
// flagged warning in ubl/schematron/abstract/EN16931-syntax.sch and
// cii/schematron/abstract/EN16931-CII-syntax.sch, resolved through the
// per-binding parameter files. The fatal halves are transcribed by hand in
// en16931_ubl_rules.go and en16931_cii_rules.go. Each assertion's test is CEN's
// own XPath, whitespace-normalised and otherwise verbatim, so this file can be
// read against the Schematron line by line; en16931_syntax_advisory.go is the
// evaluator, and en16931_syntax_advisory_test.go re-derives these tables from
// the Schematron and fails if the committed ones have drifted.
"""
    with open(OUT, "w") as f:
        f.write(hdr + "\n" + "\n\n".join(blocks) + "\n")
    print(f"wrote {OUT}")
    for name, nr, na in counts:
        print(f"  {name}: {nr} rules, {na} advisory assertions")


if __name__ == "__main__":
    main()
