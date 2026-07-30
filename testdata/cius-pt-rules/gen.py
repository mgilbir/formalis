#!/usr/bin/env python3
"""Generate cius_pt_datatype_table.go from AT/eSPap's CIUS-PT Schematron.

Run `make cius-schematron` first (it vendors the gitignored
testdata/cius-pt/schematron/). Then `python3 gen.py` rewrites
../../cius_pt_datatype_table.go with the 291 fatal DT-CIUS-PT-* assertions the
Portuguese profile publishes, in the two patterns that carry them:

  * datatype/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-datatype.sch, a concrete pattern of
    152 rules and 269 assertions — the attribute-level constraints AT places on
    every typed element; and
  * abstract/…-condition.sch bound through UBL/…-UBL-condition.sch, of which the
    22 assertions under this prefix are AT's arithmetic tier (DT-CIUS-PT-157..177).

cius_pt_datatype_test.go re-derives both tables from the Schematron and asserts the
committed file still matches, so the tables cannot drift from the source of truth.

What is emitted, and what is not
--------------------------------
Every DT-CIUS-PT-* assertion of both patterns, with its polarity, its context and
AT's own XPath, whitespace-normalised and otherwise verbatim. Nothing is
translated, so a rule cannot change meaning on the way into the table and the drift
test is a string comparison.

Every <rule> of the datatype pattern is emitted whether or not it carries an
assertion this generator can express, because a rule with none can still *shadow*
one that does: under ISO Schematron a node is processed by the first rule in a
pattern whose context matches it. Modelling the ordering means a rule that no
processor reaches falls out of the table rather than having to be remembered — the
lesson CEN's CII-DT-010/011/012 taught, and the one BR-CIUS-PT-13/15/17 taught
again in PR 23.

The BR-* assertions of the condition pattern are *not* emitted. They are the
business rules cius_pt_rules.go evaluates by hand, and they live in the same <rule>
elements as the DT-CIUS-PT-* ones, so neither shadows the other.

Failing loudly
--------------
Three things could be silently dropped here, and every one of them aborts the run:

  * a <rule context> outside the shape the Go walk can describe. Every context in
    both patterns is a union of paths anchored at the document element, and
    parse_context below refuses anything else. A context this generator cannot
    describe is one whose node population the evaluator would get wrong, and
    emitting the rule without it would silently either suppress or over-report
    every assertion under it.
  * an assertion, a <let> or a context predicate whose XPath is outside the subset
    the evaluator implements. check_expr parses each one with the same grammar the
    Go side parses, and an expression it cannot parse aborts the run naming the
    rule. The Go side parses the emitted table again at load and its own test
    asserts every row parses, so the gate is closed from both ends.
  * a published DT-CIUS-PT-* identifier that ends up in neither pattern's output.
    The last check in main() counts what the artefact publishes and what was
    written and refuses to write a table that is short of it. A rule quietly
    skipped by a generator is how two fatal UBL-CR-* rules came to sit inside a
    coverage entry describing their family as advisory (C27), and how an entire
    BR-AA-* family came to be invisible to the guard built to find missing rules
    (C39).
"""
import os
import re
import sys
import xml.etree.ElementTree as ET

SCH = "{http://purl.oclc.org/dsdl/schematron}"
HERE = os.path.dirname(os.path.abspath(__file__))
ART = os.path.abspath(os.path.join(HERE, "..", "cius-pt", "schematron"))
OUT = os.path.abspath(os.path.join(HERE, "..", "..", "cius_pt_datatype_table.go"))

# The version this package evaluates. 2.0.0 is vendored beside it and publishes 287
# of these 291; TestCIUSPTDatatypeVersionsDiffer pins the difference.
VERSION = "2.1.1"

# The identifier family this generator owns. The BR-* half of the condition pattern
# is cius_pt_rules.go's.
OWN = re.compile(r"^DT-CIUS-PT-")


def norm(s):
    """Collapse whitespace, the way the Go drift test does."""
    return " ".join((s or "").split())


# ---------------------------------------------------------------------------
# Contexts
# ---------------------------------------------------------------------------
#
# Every <rule context> in both patterns is a union of `|`-separated branches, and
# every branch is a path of element steps with at most one predicate each. A branch
# beginning //ubl:Invoice or //cn:CreditNote is anchored at that document element;
# one written relative (cac:InvoiceLine, cac:TaxTotal/cac:TaxSubtotal) is read from
# the document element and applies to both.
#
# That last reading is narrower than an XSLT match pattern, which would match such
# an element anywhere in the document — a UBL invoice line has a cac:TaxTotal of its
# own, so `cac:TaxTotal/cac:TaxSubtotal` as a pattern also selects a line-level
# subtotal. It is the reading PR 23 took for the same parameters in the same file,
# it is the reading AT's prose describes, and where the two differ this one selects
# fewer nodes and therefore reports fewer findings. The divergence is recorded in
# cius_pt_datatype.go rather than left implicit.
STEP = re.compile(r"^(?:[A-Za-z][\w.]*:)?([A-Za-z][\w.]*)(?:\[(.*)\])?$")

ROOTS = {"Invoice": "Invoice", "CreditNote": "CreditNote"}


def parse_context(ctx, what):
    """Return [(root, [(name, pred), ...]), ...] for a <rule context>."""
    out = []
    for branch in split_top(ctx, "|"):
        branch = branch.strip()
        if not branch:
            sys.exit(f"gen.py: {what}: empty union branch in {ctx!r}")
        root = ""
        if branch.startswith("//"):
            branch = branch[2:]
        steps = []
        for i, raw in enumerate(split_top(branch, "/")):
            m = STEP.match(raw.strip())
            if not m:
                sys.exit(
                    f"gen.py: {what}: cannot describe the context step {raw.strip()!r} of {ctx!r}\n"
                    f"  Every context in these two patterns is a path of element steps anchored at the\n"
                    f"  document element. A step outside that shape is a node population the Go walk\n"
                    f"  cannot build, and emitting the rule anyway would mis-scope every assertion under\n"
                    f"  it. Extend ptDTCtxPath in cius_pt_datatype_eval.go and this function together."
                )
            name, pred = m.group(1), norm(m.group(2) or "")
            if i == 0 and name in ROOTS and not pred:
                root = ROOTS[name]
                continue
            if pred:
                check_expr(what, pred)
            steps.append((name, pred))
        out.append((root, steps))
    return out


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
# The expression grammar
#
# A validator for the XPath subset the Go evaluator implements, run over every
# assertion, <let> and context predicate this generator is about to emit. It
# computes nothing: its only job is to refuse. The Go parser is the one that
# evaluates, and its own test parses the whole committed table, so an expression
# that slips past this one is caught there rather than mis-evaluated.
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

# The whole function library the Go evaluator implements, with arities. Keep in step
# with ptDTFunctions in cius_pt_datatype_eval.go.
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
}

# Go's regexp refuses a repetition bound above this; ptDTCompilePattern compiles the
# anchored-length family `^(.{1,N})$` by shape instead, which is the only pattern in
# this rule set that exceeds it (DT-CIUS-PT-111.1's 6,826,666-character ceiling).
GO_MAX_REPEAT = 1000
LENGTH_RE = re.compile(r"^\^\(\.\{\d+,\d+\}\)\$$")
REPEAT_RE = re.compile(r"\{\d+,(\d+)\}")


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


def check_regexes(rule, xpath):
    """Refuse a matches() pattern the Go side would not compile the same way."""
    for m in re.finditer(r"matches\(", xpath):
        # The pattern is the second argument and is a string literal in every one of
        # these 192 calls; the Go side requires that too, so that every regular
        # expression is compiled once at load.
        rest = xpath[m.end():]
        depth, quote, args, cur = 1, None, [], []
        for ch in rest:
            if quote:
                cur.append(ch)
                if ch == quote:
                    quote = None
                continue
            if ch in "'\"":
                quote = ch
                cur.append(ch)
                continue
            if ch == "(":
                depth += 1
            elif ch == ")":
                depth -= 1
                if depth == 0:
                    args.append("".join(cur))
                    break
            if ch == "," and depth == 1:
                args.append("".join(cur))
                cur = []
                continue
            cur.append(ch)
        if len(args) < 2:
            raise Refuse("matches() takes at least two arguments")
        pat = args[1].strip()
        if not (pat.startswith("'") and pat.endswith("'")):
            raise Refuse("matches()'s pattern must be a string literal")
        pat = pat[1:-1]
        if len(args) > 2:
            flags = args[2].strip()
            if not (flags.startswith("'") and flags.endswith("'")):
                raise Refuse("matches()'s flags must be a string literal")
            for f in flags[1:-1]:
                if f != "s":
                    raise Refuse(f"unsupported matches() flag {f!r}")
        if LENGTH_RE.match(pat):
            continue
        for r in REPEAT_RE.finditer(pat):
            if int(r.group(1)) > GO_MAX_REPEAT:
                raise Refuse(
                    f"the repetition bound {r.group(0)} in {pat!r} is above Go's regexp limit of "
                    f"{GO_MAX_REPEAT} and is not the anchored-length shape ptDTLengthMatcher compiles"
                )
        try:
            re.compile(pat)
        except re.error as e:
            raise Refuse(f"the regular expression {pat!r} does not compile: {e}")


def check_expr(rule, xpath):
    try:
        check_regexes(rule, xpath)
        p = P(lex(xpath))
        p.expr()
        if p.peek()[0] != "eof":
            raise Refuse(f"trailing input at {p.peek()[1]!r}")
    except Refuse as e:
        sys.exit(
            f"gen.py: {rule}: cannot express {xpath!r}: {e}\n"
            f"  This is a rule the evaluator would silently not check. Extend the grammar in\n"
            f"  gen.py and in cius_pt_datatype.go, or record the rule in Coverage(SourceCIUSPT)\n"
            f"  as an unevaluated gap. Do not drop it."
        )


# ---------------------------------------------------------------------------
# Reading the Schematron
# ---------------------------------------------------------------------------
MSG_PREFIX = re.compile(r"^\[[^]]*\]\s*-?\s*")


def message(el, rid):
    msg = MSG_PREFIX.sub("", norm("".join(el.itertext())))
    if not msg:
        sys.exit(f"gen.py: {rid} has no assertion text to report")
    return msg


def read_datatype(path):
    """The concrete UBL-datatype pattern: rules, lets and assertions in order."""
    root = ET.parse(path).getroot()
    pattern_lets, rules = [], []
    for el in root:
        if el.tag == SCH + "let":
            value = norm(el.get("value"))
            check_expr(f"the pattern-level let ${el.get('name')}", value)
            pattern_lets.append((el.get("name"), value))
            continue
        if el.tag != SCH + "rule":
            sys.exit(f"gen.py: unexpected element {el.tag} at the top of {os.path.basename(path)}")
        ctx = norm(el.get("context"))
        paths = parse_context(ctx, f"the rule context {ctx!r}")
        lets, asserts = [], []
        for a in el:
            if a.tag == SCH + "let":
                value = norm(a.get("value"))
                check_expr(f"the let ${a.get('name')} of {ctx!r}", value)
                lets.append((a.get("name"), value))
            elif a.tag in (SCH + "assert", SCH + "report"):
                rid, flag = a.get("id"), a.get("flag")
                if not OWN.match(rid or ""):
                    sys.exit(f"gen.py: {rid} is in the datatype pattern and is not a DT-CIUS-PT-* identifier")
                if flag != "fatal":
                    sys.exit(f"gen.py: {rid} is flagged {flag!r}; every assertion of this pattern is fatal")
                test = norm(a.get("test"))
                check_expr(rid, test)
                asserts.append((rid, a.tag[len(SCH):], test, message(a, rid)))
            else:
                sys.exit(f"gen.py: unexpected element {a.tag} inside the rule {ctx!r}")
        rules.append((ctx, paths, lets, asserts))
    return pattern_lets, rules


def read_condition(abstract, binding):
    """The abstract condition pattern, resolved through the UBL binding's params.

    Only the DT-CIUS-PT-* assertions are emitted: the BR-* ones in the same <rule>
    elements are cius_pt_rules.go's, and they are hand-written because they need
    judgement rather than transcription.
    """
    params = {}
    for e in ET.parse(binding).getroot().iter(SCH + "param"):
        params[e.get("name")] = e.get("value")
    names = sorted(params, key=len, reverse=True)

    def resolve(expr, what):
        expr = expr or ""
        for n in names:
            expr = expr.replace("$" + n, params[n])
            expr = expr.replace("$" + n.strip(), params[n])
        if "$" in re.sub(r"'[^']*'", "", expr):
            leftover = re.findall(r"\$[\w.]+", re.sub(r"'[^']*'", "", expr))
            # `every $v in …` binds its own variable; anything else is a parameter
            # the binding does not define.
            bound = set(re.findall(r"every\s+\$([\w.]+)\s+in", expr))
            unbound = [v for v in leftover if v[1:] not in bound]
            if unbound:
                sys.exit(f"gen.py: {what} still refers to {sorted(set(unbound))} after resolution")
        return norm(expr)

    rules = []
    for r in ET.parse(abstract).getroot().findall(SCH + "rule"):
        ctx_raw = r.get("context")
        ctx = resolve(ctx_raw, f"the context {ctx_raw!r}")
        asserts = []
        for a in r.findall(SCH + "assert") + r.findall(SCH + "report"):
            rid = a.get("id")
            if not OWN.match(rid or ""):
                continue
            if a.get("flag") != "fatal":
                sys.exit(f"gen.py: {rid} is flagged {a.get('flag')!r}; every DT-CIUS-PT-* assertion is fatal")
            test = resolve(a.get("test"), f"the assertion {rid}")
            check_expr(rid, test)
            asserts.append((rid, a.tag[len(SCH):], test, message(a, rid)))
        if not asserts:
            # A rule of this pattern carrying only BR-* assertions still claims its
            # nodes away from the rules below it, so it is emitted with no assertion
            # for exactly the reason CEN's empty rules are.
            rules.append((ctx, parse_context(ctx, f"the rule context {ctx!r}"), [], []))
            continue
        rules.append((ctx, parse_context(ctx, f"the rule context {ctx!r}"), [], asserts))
    return [], rules


# ---------------------------------------------------------------------------
# Emitting Go
# ---------------------------------------------------------------------------
def golit(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def emit_pattern(varname, schname, lets, rules, blurb):
    n = sum(len(a) for _, _, _, a in rules)
    out = [
        f"// {varname} is {blurb}",
        f"// {len(rules)} rules carrying {n} fatal DT-CIUS-PT-* assertions, in pattern order. A rule",
        "// with no assertion is here for its context alone, which under ISO Schematron claims its",
        "// nodes away from every rule below it.",
        f"var {varname} = ptDTPattern{{",
        f"\tname: {golit(schname)},",
    ]
    if lets:
        out.append("\tlets: []ptDTLetSrc{")
        for name, value in lets:
            out.append(f"\t\t{{{golit(name)}, {golit(value)}}},")
        out.append("\t},")
    out.append("\trules: []ptDTRuleSrc{")
    for ctx, paths, rlets, asserts in rules:
        out.append("\t\t{")
        out.append(f"\t\t\tcontext: {golit(ctx)},")
        parts = []
        for root, steps in paths:
            steps_src = ", ".join(f"{{{golit(nm)}, {golit(pred)}}}" for nm, pred in steps)
            parts.append(f"{{{golit(root)}, []ptDTCtxStep{{{steps_src}}}}}")
        out.append("\t\t\tpaths:   []ptDTCtxPath{" + ", ".join(parts) + "},")
        if rlets:
            out.append("\t\t\tlets: []ptDTLetSrc{")
            for name, value in rlets:
                out.append(f"\t\t\t\t{{{golit(name)}, {golit(value)}}},")
            out.append("\t\t\t},")
        if asserts:
            out.append("\t\t\tasserts: []ptDTAssertSrc{")
            for rid, kind, test, msg in asserts:
                out.append(f"\t\t\t\t{{{golit(rid)}, {golit(kind)}, {golit(test)}, {golit(msg)}}},")
            out.append("\t\t\t},")
        out.append("\t\t},")
    out.append("\t},")
    out.append("}")
    return "\n".join(out)


def published_ids(base):
    """Every DT-CIUS-PT-* identifier the vendored version publishes, decoded."""
    ids = set()
    for dirpath, _, names in os.walk(base):
        for fn in sorted(names):
            if not fn.endswith(".sch"):
                continue
            for el in ET.parse(os.path.join(dirpath, fn)).getroot().iter():
                if el.tag in (SCH + "assert", SCH + "report") and OWN.match(el.get("id") or ""):
                    ids.add(el.get("id"))
    return ids


def main():
    base = os.path.join(ART, VERSION)
    if not os.path.isdir(base):
        sys.exit(f"gen.py: {base} is not present; run `make cius-schematron` first")

    dt_lets, dt_rules = read_datatype(
        os.path.join(base, "datatype", f"urn_feap.gov.pt_CIUS-PT_{VERSION}-UBL-datatype.sch"))
    _, cond_rules = read_condition(
        os.path.join(base, "abstract", f"urn_feap.gov.pt_CIUS-PT_{VERSION}-condition.sch"),
        os.path.join(base, "UBL", f"urn_feap.gov.pt_CIUS-PT_{VERSION}-UBL-condition.sch"))

    written = set()
    for _, rules in (("dt", dt_rules), ("cond", cond_rules)):
        for _, _, _, asserts in rules:
            for rid, _, _, _ in asserts:
                written.add(rid)
    missing = sorted(published_ids(base) - written)
    if missing:
        sys.exit(f"gen.py: the artefact publishes {missing} and neither pattern emitted them; a rule\n"
                 f"  a generator drops is a rule nothing checks. Find the pattern it lives in and read it.")

    blocks = [
        emit_pattern("ptDatatypePattern", "UBL-datatype", dt_lets, dt_rules,
                     "AT/eSPap's concrete UBL-datatype pattern:"),
        emit_pattern("ptConditionPattern", "UBL-condition", [], cond_rules,
                     "the DT-CIUS-PT-* half of AT/eSPap's condition pattern, resolved through its UBL binding:"),
    ]
    hdr = f"""package formalis

// Code generated from AT/eSPap's CIUS-PT {VERSION} Schematron by
// testdata/cius-pt-rules/gen.py; DO NOT EDIT. Regenerate with `make cius-pt-rules`.
//
// These are the 291 fatal DT-CIUS-PT-* assertions the Portuguese profile publishes:
// 269 in datatype/urn_feap.gov.pt_CIUS-PT_{VERSION}-UBL-datatype.sch and 22 in the
// abstract condition pattern, resolved through its UBL binding the way a Schematron
// processor resolves it. Each assertion's test is AT's own XPath,
// whitespace-normalised and otherwise verbatim, so this file can be read against the
// Schematron line by line; cius_pt_datatype.go is the parser,
// cius_pt_datatype_eval.go the evaluator, and cius_pt_datatype_test.go re-derives
// these tables from the Schematron and fails if the committed ones have drifted.
//
// The BR-* assertions that share the condition pattern's <rule> elements are not
// here: they need judgement rather than transcription and are written by hand in
// cius_pt_rules.go.
"""
    with open(OUT, "w") as f:
        f.write(hdr + "\n" + "\n\n".join(blocks) + "\n")
    print(f"wrote {OUT}")
    print(f"  ptDatatypePattern:  {len(dt_rules)} rules, "
          f"{sum(len(a) for _, _, _, a in dt_rules)} assertions")
    print(f"  ptConditionPattern: {len(cond_rules)} rules, "
          f"{sum(len(a) for _, _, _, a in cond_rules)} assertions")
    print(f"  {len(written)} distinct DT-CIUS-PT-* identifiers")


if __name__ == "__main__":
    main()
