#!/usr/bin/env python3
"""Generate cius_ro_rules_table.go from ANAF's CIUS-RO Schematron.

Run `make cius-schematron` first (it vendors the gitignored
testdata/cius-ro/schematron/). Then `python3 gen.py` rewrites
../../cius_ro_rules_table.go with the mechanical half of RO16931-rules.sch:

  * BR-RO-L*     — 64 per-field maximum-length limits;
  * BR-DEC-RO-*  — 21 maximum-decimal-place limits;
  * BR-RO-DT*    —  7 date-format rules, published for the first time in 1.0.9;
  * BR-RO-A*     —  4 maximum-occurrence limits.

cius_ro_rules_test.go re-derives the table from the Schematron and asserts the
committed file still matches, so it cannot drift from the source of truth.

The 25 BR-RO-NNN business rules of the same file are *not* emitted. They need
judgement rather than transcription — the two VAT-identifier rules are a
four-way disjunction over allowance, charge and line categories — and they are
written by hand in cius_ro.go, whose conditions cius_ro_artefact_test.go compares
against this same artefact. BR-27 is not emitted either: ANAF re-publishes one CEN
identifier verbatim in its own file, and an identifier CEN minted stays CEN's and is
reported under SourceEN16931 with CEN's condition (the audit's C40).

What this generator refuses, and why each refusal is load-bearing
----------------------------------------------------------------
Every <rule> is emitted whether or not it carries an assertion this generator
writes, because a rule with none still *shadows* one that does: under ISO
Schematron a node is processed by the first rule in a pattern whose context matches
it, and the whole of RO16931-rules.sch is one pattern. Three of ANAF's rules turn
out to be shadowed by an earlier one and their five assertions can never be
reported by any conforming processor; they are detected here rather than
remembered, listed in ro_unevaluable, and named in Coverage(SourceCIUSRO).

  * a <rule context> outside the shape the Go walk can describe aborts the run.
  * an assertion or a context predicate whose XPath is outside the subset the
    evaluator implements aborts the run naming the rule. The Go side parses the
    emitted table again at load and its own test asserts every row parses.
  * an identifier in none of the four generated families, the hand-written list or
    the one CEN identifier aborts the run. An entire published family was once
    invisible because a guard filtered identifiers through a pattern its author had
    anticipated (C39); this one refuses what it does not recognise instead.
  * an assertion that ends up in neither the table nor ro_unevaluable aborts the
    run. A rule a generator drops is a rule nothing checks (C27).
"""
import os
import re
import sys
import xml.etree.ElementTree as ET

SCH = "{http://purl.oclc.org/dsdl/schematron}"
HERE = os.path.dirname(os.path.abspath(__file__))
ART = os.path.abspath(os.path.join(HERE, "..", "cius-ro", "schematron"))
OUT = os.path.abspath(os.path.join(HERE, "..", "..", "cius_ro_rules_table.go"))

# The version this package evaluates. 1.0.3, 1.0.4 and 1.0.8 are vendored beside it;
# TestCIUSROVersionsDiffer pins what each publishes and what ANAF withdrew.
VERSION = "1.0.9"

RULES = os.path.join(ART, VERSION, "cius-ro", "RO16931-rules.sch")
# RO16931-rules.sch is an included pattern and declares no prefix of its own: the
# <sch:ns> elements that bind ubl, cn, cac and cbc for every XPath in it are in the
# file that includes it. Reading the prefixes from the pattern file would bind them
# to nothing, and a context anchored at a prefix that resolves to nothing is exactly
# the case this generator has to tell apart from a live one.
WRAPPER = os.path.join(ART, VERSION, "EN16931-CIUS_RO-UBL-validation.sch")

# The four families this generator owns.
GENERATED = re.compile(r"^(?:BR-RO-L|BR-RO-DT|BR-RO-A|BR-DEC-RO-)")

# The business rules cius_ro.go writes by hand, named one by one rather than by a
# prefix so that an identifier ANAF adds to the family is a failed build here and
# not a rule that quietly goes unevaluated.
HAND_WRITTEN = {
    "BR-RO-001", "BR-RO-010", "BR-RO-020_1", "BR-RO-020_2", "BR-RO-030",
    "BR-RO-040", "BR-RO-065", "BR-RO-081", "BR-RO-082", "BR-RO-091",
    "BR-RO-092", "BR-RO-100", "BR-RO-101", "BR-RO-110", "BR-RO-111",
    "BR-RO-120", "BR-RO-140", "BR-RO-150", "BR-RO-160", "BR-RO-170",
    "BR-RO-180", "BR-RO-201", "BR-RO-202", "BR-RO-211", "BR-RO-212",
}

# CEN's, re-published in ANAF's file. Reported under SourceEN16931 with CEN's own
# condition; see C40.
CEN_OWNED = {"BR-27"}

UBL_INVOICE_NS = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
UBL_CREDITNOTE_NS = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"

FLOATING = "//"


def norm(s):
    """Collapse whitespace, the way the Go drift test does."""
    return " ".join((s or "").split())


# ---------------------------------------------------------------------------
# Contexts
# ---------------------------------------------------------------------------
#
# ANAF writes its contexts as XSLT match patterns, which is what a Schematron
# <rule context> is: `cbc:IssueDate` selects every issue date in the document,
# `cac:TaxTotal/cac:TaxSubtotal` every VAT breakdown including the ones inside an
# invoice line, and `//cac:PaymentMeans` the same nodes as `cac:PaymentMeans`.
# CIUS-PT's generator refuses a branch that is not anchored at the document element
# and reads the rest as paths from the root, which is narrower; that reading cannot
# be used here without changing which nodes two thirds of these rules claim.
#
# A branch beginning with a single `/` is anchored: its first step must name a
# document element, and it must name it in the right namespace.
STEP = re.compile(r"^(?:([A-Za-z][\w.]*):)?([A-Za-z][\w.]*)(?:\[(.*)\])?$")


class Dead(Exception):
    """A context branch that cannot select a node in any UBL document."""


def parse_context(ctx, what, nsmap):
    """Return ([(root, [(name, pred), ...]), ...], [dead branch, ...])."""
    out, dead = [], []
    for branch in split_top(ctx, "|"):
        try:
            out.append(parse_branch(branch, ctx, what, nsmap))
        except Dead as e:
            dead.append((norm(branch), str(e)))
    if not out and not dead:
        sys.exit(f"gen.py: {what}: {ctx!r} has no branch at all")
    return out, dead


def parse_branch(branch, ctx, what, nsmap):
    branch = branch.strip()
    if not branch:
        sys.exit(f"gen.py: {what}: empty union branch in {ctx!r}")
    root = FLOATING
    anchored = False
    if branch.startswith("//"):
        branch = branch[2:]
    elif branch.startswith("/"):
        branch = branch[1:]
        anchored = True
    steps = []
    for i, raw in enumerate(split_top(branch, "/")):
        m = STEP.match(raw.strip())
        if not m:
            sys.exit(
                f"gen.py: {what}: cannot describe the context step {raw.strip()!r} of {ctx!r}\n"
                f"  A step outside the shape ptDTCtxPath can hold is a node population the Go\n"
                f"  walk cannot build, and emitting the rule anyway would mis-scope every\n"
                f"  assertion under it. Extend ptDTCtxPath in cius_pt_datatype_eval.go and this\n"
                f"  function together."
            )
        prefix, name, pred = m.group(1), m.group(2), norm(m.group(3) or "")
        uri = nsmap.get(prefix or "", "")
        if i == 0:
            # A name that can only be a document element is checked against its
            # namespace, in both directions. ANAF writes `/ubl:CreditNote/...` in
            # six contexts and `cac:CreditNote` in one, and in that file `ubl` is
            # the Invoice-2 namespace and `cac` is CommonAggregateComponents-2: a
            # credit note's root is `cn:CreditNote`, so neither can ever match. The
            # branch is dropped rather than read as "CreditNote", because the Go
            # walk matches on local names and would otherwise apply the rule to
            # every credit note — the C37 defect this same file already taught.
            if name == "Invoice" and uri == UBL_INVOICE_NS:
                root, anchored = "Invoice", False
                continue
            if name == "CreditNote" and uri == UBL_CREDITNOTE_NS:
                root, anchored = "CreditNote", False
                continue
            if name in ("Invoice", "CreditNote"):
                raise Dead(
                    f"{prefix}:{name} is {uri or 'in no declared namespace'}, and a UBL document "
                    f"element named {name} is in "
                    f"{UBL_INVOICE_NS if name == 'Invoice' else UBL_CREDITNOTE_NS}"
                )
            if anchored:
                sys.exit(f"gen.py: {what}: {ctx!r} is anchored at {prefix}:{name}, which is not a "
                         f"UBL document element")
        if pred:
            check_expr(what, pred)
        steps.append((name, pred))
    if anchored:
        sys.exit(f"gen.py: {what}: {ctx!r} begins with '/' and names no document element")
    if root == FLOATING and not steps:
        sys.exit(f"gen.py: {what}: {ctx!r} has a floating branch with no step")
    return (root, steps)


def split_top(s, sep):
    """Split on sep at bracket depth zero and outside string literals."""
    parts, depth, quote, cur = [], 0, None, []
    for ch in s:
        if quote:
            cur.append(ch)
            if ch == quote:
                quote = None
            continue
        if ch in "'\"":
            quote = ch
            cur.append(ch)
            continue
        if ch in "([":
            depth += 1
        elif ch in ")]":
            depth -= 1
        if ch == sep and depth == 0:
            parts.append("".join(cur))
            cur = []
            continue
        cur.append(ch)
    parts.append("".join(cur))
    return parts


# ---------------------------------------------------------------------------
# Shadowing
# ---------------------------------------------------------------------------
#
# One branch subsumes another when every node the second selects is also selected by
# the first. Two shapes cover every case in this pattern, and both are exact rather
# than approximate — an approximation here would either invent an unreachable rule
# or hide one.
def subsumes(outer, inner):
    oroot, osteps = outer
    iroot, isteps = inner
    if oroot == FLOATING:
        # A match pattern of n steps selects an element whose own name and whose
        # ancestors' end with those steps, at any depth. Anything selected by a
        # branch whose last n steps are the same — anchored or floating — is
        # therefore selected by it too.
        return len(osteps) <= len(isteps) and osteps == isteps[len(isteps) - len(osteps):]
    return oroot == iroot and osteps == isteps


# ---------------------------------------------------------------------------
# Tautologies
# ---------------------------------------------------------------------------
#
# `count(.)` counts the context node, which is one node. Two of ANAF's four
# occurrence limits are written that way and are therefore true for every document —
# the ubl-BE-13 shape, and D10's definition of a rule the authority published that no
# validator can evaluate. Recognised by shape rather than by identifier so that the
# claim is re-derived from whatever is vendored.
COUNT_SELF = re.compile(r"^count\(\s*\.\s*\)\s*<=\s*\d+$")


# ---------------------------------------------------------------------------
# The expression grammar
#
# A validator for the XPath subset the Go evaluator implements, run over every
# assertion and context predicate this generator is about to emit. It computes
# nothing: its only job is to refuse.
# ---------------------------------------------------------------------------
TOKEN = re.compile(
    r"""
      (?P<ws>\s+)
    | (?P<lit>'[^']*'|"[^"]*")
    | (?P<num>[0-9]+(?:\.[0-9]+)*|\.[0-9]+)
    | (?P<var>\$[A-Za-z_][\w.]*)
    | (?P<axis>ancestor::|child::)
    | (?P<name>[A-Za-z_](?:[\w.]|-(?=[A-Za-z_]))*(?::[A-Za-z_](?:[\w.]|-(?=[A-Za-z_]))*)?)
    | (?P<op><=|>=|!=|//|\.\.|[=<>/()\[\]@|,.+*-])
""",
    re.X,
)

# Keep in step with ptDTFunctions in cius_pt_datatype_eval.go.
FUNCTIONS = {
    "normalize-space": (1, 1),
    "matches": (2, 3),
    "not": (1, 1),
    "starts-with": (2, 2),
    "contains": (2, 2),
    "concat": (2, 8),
    "substring-before": (2, 2),
    "substring-after": (2, 2),
    "substring": (2, 3),
    "string-length": (1, 1),
    "exists": (1, 1),
    "count": (1, 1),
    "sum": (1, 1),
    "round": (1, 1),
    "distinct-values": (1, 1),
    "index-of": (2, 2),
    "xs:decimal": (1, 1),
    "string": (0, 1),
    "text": (0, 0),
    "true": (0, 0),
    "false": (0, 0),
}

# The only type `castable as` may name; ptDTCheck refuses any other.
CASTABLE_TYPES = {"xs:date"}


class Refuse(Exception):
    pass


def lex(s):
    out, i = [], 0
    while i < len(s):
        m = TOKEN.match(s, i)
        if not m:
            raise Refuse(f"cannot tokenise at offset {i}: {s[i:i + 24]!r}")
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

    def expr(self):
        if self.at("every") and self.peek(1)[0] == "var":
            self.next()
            self.next()
            if not self.at("in"):
                raise Refuse("expected 'in' after `every $v`")
            self.next()
            self.or_expr()
            if not self.at("satisfies"):
                raise Refuse("expected 'satisfies'")
            self.next()
            self.expr()
            return
        self.or_expr()

    def or_expr(self):
        self.and_expr()
        while self.at("or"):
            self.next()
            self.and_expr()

    def and_expr(self):
        self.cmp_expr()
        while self.at("and"):
            self.next()
            self.cmp_expr()

    def cmp_expr(self):
        self.additive()
        if self.peek()[0] == "op" and self.peek()[1] in ("=", "!=", "<", "<=", ">", ">="):
            self.next()
            self.additive()

    def additive(self):
        self.multiplicative()
        while self.at("+") or self.at("-"):
            self.next()
            self.multiplicative()

    def multiplicative(self):
        self.unary()
        while self.at("*") or self.at("div"):
            self.next()
            self.unary()

    def unary(self):
        if self.at("-"):
            self.next()
            self.unary()
            return
        self.union()
        while self.at("castable"):
            self.next()
            if not self.at("as"):
                raise Refuse("expected 'as' after 'castable'")
            self.next()
            kind, val = self.peek()
            if kind != "name" or val not in CASTABLE_TYPES:
                raise Refuse(f"`castable as {val}`: the only type the evaluator implements is xs:date")
            self.next()

    def union(self):
        self.value()
        while self.at("|"):
            self.next()
            self.value()

    def value(self):
        head = True
        if self.at("//"):
            self.next()
            head = False
        elif self.at("/"):
            self.next()
            head = False
        while self.at(".."):
            self.next()
            head = False
            if not self.at("/"):
                return
            self.eat("/")
        while True:
            self.step(head)
            head = False
            if not self.at("/"):
                return
            self.next()

    def step(self, head):
        kind, val = self.peek()
        if val == "@":
            self.next()
            if self.peek()[0] != "name":
                raise Refuse("'@' must be followed by an attribute name")
            self.next()
        elif val == ".":
            self.next()
        elif kind == "axis":
            self.next()
            if self.peek()[0] != "name":
                raise Refuse(f"the {val} axis must be followed by a name")
            self.next()
        elif kind == "name" and self.peek(1)[1] == "(":
            name = val
            if name not in FUNCTIONS:
                raise Refuse(f"unsupported function {name}()")
            self.next()
            self.eat("(")
            args = 0
            if not self.at(")"):
                while True:
                    args += 1
                    self.expr()
                    if not self.at(","):
                        break
                    self.next()
            self.eat(")")
            lo, hi = FUNCTIONS[name]
            if not lo <= args <= hi:
                raise Refuse(f"{name}() takes {lo}..{hi} arguments, given {args}")
        elif kind == "name":
            self.next()
        elif kind in ("lit", "num", "var"):
            if not head and kind != "var":
                raise Refuse(f"a {kind} cannot be a path step")
            self.next()
        elif val == "(":
            self.next()
            while True:
                self.expr()
                if not self.at(","):
                    break
                self.next()
            self.eat(")")
        else:
            raise Refuse(f"expected a step, found {val!r}")
        while self.at("["):
            self.next()
            self.expr()
            self.eat("]")


def check_expr(rule, xpath):
    # No <let> is emitted: the six ANAF declares are read only by the hand-written
    # business rules, and three of their names carry hyphens the Go lexer does not
    # admit in a variable. An emitted expression that referred to one would compile
    # to an undeclared-variable error at load, so it is refused here instead.
    if re.search(r"\$[A-Za-z_]", re.sub(r"'[^']*'", "", xpath)):
        sys.exit(f"gen.py: {rule}: {xpath!r} reads a <let> variable, and this table declares none.\n"
                 f"  Emit the <let> it needs (and check the Go lexer admits its name), or move the\n"
                 f"  rule to the hand-written half in cius_ro.go.")
    try:
        p = P(lex(xpath))
        p.expr()
        if p.peek()[0] != "eof":
            raise Refuse(f"trailing input at {p.peek()[1]!r}")
    except Refuse as e:
        sys.exit(
            f"gen.py: {rule}: cannot express {xpath!r}: {e}\n"
            f"  This is a rule the evaluator would silently not check. Extend the grammar in\n"
            f"  gen.py and in cius_pt_datatype.go, or record the rule in Coverage(SourceCIUSRO)\n"
            f"  as an unevaluated gap. Do not drop it."
        )


# ---------------------------------------------------------------------------
# Reading the Schematron
# ---------------------------------------------------------------------------
MSG_PREFIX = re.compile(r"^\[[^]]*\]\s*-?\s*")


def message(el, rid):
    """ANAF's own text, Romanian half discarded.

    Every assertion's text is `[ID]-<Romanian> #<English>`, and the English half is
    what this package reports. A rule whose text has no '#' keeps the whole string:
    dropping it would be inventing a message, and quoting the authority is the point.
    """
    text = MSG_PREFIX.sub("", norm("".join(el.itertext())))
    if "#" in text:
        text = text.split("#", 1)[1].strip()
    # The value-of placeholders ANAF interpolates leave "( = '')" behind once the
    # decoder drops the <name/> and <value-of/> children; the reported message is
    # about the rule and not about the value, so the empty parenthesis goes.
    text = norm(re.sub(r"\(\s*=\s*''\s*\)\s*$", "", text))
    if not text:
        sys.exit(f"gen.py: {rid} has no assertion text to report")
    return text


def read_namespaces(wrapper):
    """The <sch:ns> bindings every XPath in the included pattern is read against."""
    ns = {}
    for el in ET.parse(wrapper).getroot().iter(SCH + "ns"):
        ns[el.get("prefix")] = el.get("uri")
    for want in ("ubl", "cn", "cac", "cbc"):
        if want not in ns:
            sys.exit(f"gen.py: {os.path.basename(wrapper)} declares no prefix {want!r}; "
                     f"every context in the pattern is read against these bindings")
    return ns


def read_rules(path, wrapper):
    """The ROmodel pattern: its rules and assertions, in document order."""
    nsmap = read_namespaces(wrapper)
    root = ET.parse(path).getroot()
    if root.tag != SCH + "pattern" or root.get("id") != "ROmodel":
        sys.exit(f"gen.py: {os.path.basename(path)} is not the ROmodel pattern")
    rules, seen = [], set()
    for el in root:
        if el.tag == SCH + "let":
            continue
        if el.tag != SCH + "rule":
            sys.exit(f"gen.py: unexpected element {el.tag} at the top of {os.path.basename(path)}")
        ctx = norm(el.get("context"))
        paths, dead = parse_context(ctx, f"the rule context {ctx!r}", nsmap)
        asserts = []
        for a in el:
            if a.tag not in (SCH + "assert", SCH + "report"):
                sys.exit(f"gen.py: unexpected element {a.tag} inside the rule {ctx!r}")
            rid, flag = a.get("id"), a.get("flag")
            if rid in seen:
                sys.exit(f"gen.py: {rid} is published twice")
            seen.add(rid)
            if flag != "fatal":
                sys.exit(f"gen.py: {rid} is flagged {flag!r}; every assertion of this pattern is fatal")
            if rid in CEN_OWNED or rid in HAND_WRITTEN:
                continue
            if not GENERATED.match(rid or ""):
                sys.exit(
                    f"gen.py: {rid!r} is in neither the four generated families, the hand-written\n"
                    f"  list nor the one CEN identifier ANAF re-publishes. An identifier a survey\n"
                    f"  does not recognise is an identifier nothing checks: decide which half it\n"
                    f"  belongs to and say so here."
                )
            test = norm(a.get("test"))
            check_expr(rid, test)
            asserts.append((rid, a.tag[len(SCH):], test, message(a, rid)))
        rules.append({"ctx": ctx, "paths": paths, "dead": dead, "asserts": asserts})
    return rules, seen


def mark_unevaluable(rules):
    """Drop the assertions no conforming processor can report, with the reason."""
    out = {}
    for i, r in enumerate(rules):
        if not r["paths"]:
            for rid, _, _, _ in r["asserts"]:
                out[rid] = (f"the rule context {r['ctx']} selects nothing in any UBL document: "
                            + "; ".join(why for _, why in r["dead"]))
            r["asserts"] = []
            continue
        shadow = None
        for b in r["paths"]:
            hit = None
            for j in range(i):
                for c in rules[j]["paths"]:
                    if subsumes(c, b):
                        hit = j
                        break
                if hit is not None:
                    break
            if hit is None:
                shadow = None
                break
            shadow = hit if shadow is None else max(shadow, hit)
        if shadow is not None:
            for rid, _, _, _ in r["asserts"]:
                out[rid] = (f"unreachable: every node the rule context {r['ctx']} selects is claimed by "
                            f"the earlier rule {rules[shadow]['ctx']}, and under ISO Schematron a node "
                            f"goes to the first matching rule only")
            r["asserts"] = []
            continue
        kept = []
        for rid, kind, test, msg in r["asserts"]:
            if kind == "assert" and COUNT_SELF.match(test):
                out[rid] = (f"count(.) counts the context node, so {test} is true for every document; "
                            f"the rule was written for a document-wide count and cannot fail as bound")
                continue
            kept.append((rid, kind, test, msg))
        r["asserts"] = kept
    return out


# ---------------------------------------------------------------------------
# Emitting Go
# ---------------------------------------------------------------------------
def golit(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def emit_pattern(rules):
    n = sum(len(r["asserts"]) for r in rules)
    out = [
        "// roRulesPattern is ANAF's ROmodel pattern, mechanical half:",
        f"// {len(rules)} rules carrying {n} fatal assertions, in pattern order. A rule with no",
        "// assertion is here for its context alone, which under ISO Schematron claims its nodes",
        "// away from every rule below it — and three of these rules are here for no other reason.",
        "var roRulesPattern = ptDTPattern{",
        "\tname: \"ROmodel\",",
        "\trules: []ptDTRuleSrc{",
    ]
    for r in rules:
        out.append("\t\t{")
        out.append(f"\t\t\tcontext: {golit(r['ctx'])},")
        parts = []
        for root, steps in r["paths"]:
            steps_src = ", ".join(f"{{{golit(nm)}, {golit(pred)}}}" for nm, pred in steps)
            parts.append(f"{{{golit(root)}, []ptDTCtxStep{{{steps_src}}}}}")
        out.append("\t\t\tpaths:   []ptDTCtxPath{" + ", ".join(parts) + "},")
        if r["asserts"]:
            out.append("\t\t\tasserts: []ptDTAssertSrc{")
            for rid, kind, test, msg in r["asserts"]:
                out.append(f"\t\t\t\t{{{golit(rid)}, {golit(kind)}, {golit(test)}, {golit(msg)}}},")
            out.append("\t\t\t},")
        out.append("\t\t},")
    out.append("\t},")
    out.append("}")
    return "\n".join(out)


def emit_unevaluable(unev):
    out = [
        "// roUnevaluableAsserts are the assertions ANAF publishes that no conforming",
        "// Schematron processor can report, with the reason derived from the artefact rather",
        "// than asserted. They are RuleFamily.Unevaluable entries in Coverage(SourceCIUSRO),",
        "// and cius_ro_rules_test.go checks the two agree.",
        "var roUnevaluableAsserts = map[string]string{",
    ]
    for rid in sorted(unev):
        out.append(f"\t{golit(rid)}: {golit(unev[rid])},")
    out.append("}")
    return "\n".join(out)


def main():
    for f in (RULES, WRAPPER):
        if not os.path.isfile(f):
            sys.exit(f"gen.py: {f} is not present; run `make cius-schematron` first")

    rules, published = read_rules(RULES, WRAPPER)
    unev = mark_unevaluable(rules)

    written = {rid for r in rules for rid, _, _, _ in r["asserts"]}
    accounted = written | set(unev) | HAND_WRITTEN | CEN_OWNED
    missing = sorted(published - accounted)
    if missing:
        sys.exit(f"gen.py: the artefact publishes {missing} and this run emitted them nowhere; a rule\n"
                 f"  a generator drops is a rule nothing checks. Find the pattern it lives in and read it.")
    stale = sorted(HAND_WRITTEN - published)
    if stale:
        sys.exit(f"gen.py: cius_ro.go is listed as evaluating {stale}, which {VERSION} does not publish")

    hdr = f"""package formalis

// Code generated from ANAF's CIUS-RO {VERSION} Schematron by
// testdata/cius-ro-rules/gen.py; DO NOT EDIT. Regenerate with `make cius-ro-rules`.
//
// These are the {len(written)} fatal assertions of the mechanical half of
// cius-ro/RO16931-rules.sch: the BR-RO-L* length limits, the BR-DEC-RO-* decimal
// limits, the BR-RO-DT* date formats and the BR-RO-A* occurrence limits. Each
// assertion's test is ANAF's own XPath, whitespace-normalised and otherwise
// verbatim, so this file reads against the Schematron line by line;
// cius_pt_datatype.go is the parser it is written in, cius_pt_datatype_eval.go the
// evaluator, and cius_ro_rules_test.go re-derives these tables from the Schematron
// and fails if the committed ones have drifted.
//
// The 25 BR-RO-NNN business rules that share these <rule> elements are not here:
// they need judgement rather than transcription and are written by hand in
// cius_ro.go. Neither shadows the other — they are assertions of the same rules.
"""
    with open(OUT, "w") as f:
        f.write(hdr + "\n" + emit_pattern(rules) + "\n\n" + emit_unevaluable(unev) + "\n")
    print(f"wrote {OUT}")
    print(f"  roRulesPattern: {len(rules)} rules, {len(written)} assertions")
    print(f"  {len(published)} published identifiers: {len(written)} generated, "
          f"{len(HAND_WRITTEN)} hand-written, {len(unev)} unevaluable, {len(CEN_OWNED)} CEN's")
    for rid in sorted(unev):
        print(f"    unevaluable {rid}: {unev[rid]}")
    for r in rules:
        for branch, why in r["dead"]:
            print(f"    dead branch in {r['ctx']!r}: {branch!r} — {why}")


if __name__ == "__main__":
    main()
