#!/usr/bin/env python3
"""Generate cius_overrides_table.go: the CEN conditions a CIUS re-writes.

Run `make cius-schematron en16931-artefacts` first (both are gitignored). Then
`python3 gen.py` rewrites ../../cius_overrides_table.go.

The problem
-----------
A national CIUS does not always *reference* CEN's Schematron. Five of the ones this
package validates against ship a **copy** of it, and a copy can be edited. Where it
has been, a document validated under that CIUS is judged by its authority's reading
of a CEN identifier and not by CEN's — so this package, which evaluates CEN's
condition and reports it under SourceEN16931, says something the authority's own
validator does not. That is the audit's C40.

Acting on it needs one question answered per identifier, and answered mechanically:

    is the condition in this CIUS's copy something the authority wrote,
    or something CEN wrote and later changed?

Both produce a copy that differs from CEN's current file, and only the first is a
national reading. The second is a *stale vendored copy* — a lag, not a rule — and
honouring it would mean reporting BR-51 with a four-to-six-character card-number
test because SimplerInvoicing has not refreshed a directory since 2018.

The discriminator, and why it is not a judgement call
----------------------------------------------------
testdata/en16931-artefacts is a git clone of CEN/TC 434's own repository. Every
version of every abstract file and every syntax binding CEN ever published is in it.
So the question above has an exact answer:

    resolve each identifier's (context, kind, test) at **every commit** that
    touched those files; a CIUS condition CEN never published at any commit
    is the authority's own, and one CEN did publish is CEN's, whenever it
    was written.

That is the whole classification. There is no normalisation step, no list of
"differences that look cosmetic", and nothing is compared by eye — which matters,
because the differences that *do* look cosmetic are the overwhelming majority and
the audit's own reading of them turned out to be wrong. `exists(cbc:ID)` for BR-02,
which C40 records as AT/eSPap being more permissive than CEN, is CEN's own text
from 2017-03-14 (commit 51988aa); so are the dropped `cac:TaxScheme/cbc:ID='VAT'`
predicates, BR-48's missing `or category='O'` escape, BR-CO-19's missing
DescriptionCode alternative, and UBL-SR-27 counting cbc:InstructionNote.

The axes are compared independently — a context CEN never published, a polarity CEN
never published, or a test CEN never published each make the identifier the
authority's. Requiring the whole triple to be unpublished would flag an identifier
whose *combination* is new because the CIUS pinned an old test against a new
context, which is a vintage artefact and not a reading.

What is emitted
---------------
For each authority that has at least one identifier of its own, the whole of the
pattern that carries it — every <rule>, in the authority's order, with its context —
and assertions for the overridden identifiers only. The rules that carry nothing are
there for their contexts: under ISO Schematron a node goes to the first rule of a
pattern whose context matches it, so dropping them would hand nodes to a rule that
cannot have them. That is the fourth time rule order has decided a rule's meaning in
this repository (PR 14, PR 23, PR 25, PR 26).

Nothing is translated. Each assertion carries the authority's own XPath, polarity,
flag and message text verbatim, so cius_overrides_test.go's drift check is a string
comparison against the artefact.

The per-authority verdict is emitted too, derived rather than asserted: how many
identifiers each copy shares with CEN, how many carry a condition CEN published at
some release, and how many are the authority's own. An authority with none is
recorded with a zero, which is the form in which "CIUS-RO ships an older copy, not a
modified one" becomes a checked fact rather than a claim in a commit message.

What a copy leaves out
----------------------
The same question, asked about absence. A CEN identifier a copy does not carry is
missing for the same two reasons a condition differs — the authority left it out, or
CEN had not written it yet — and telling them apart needs one more fact: *which CEN
release the copy was taken from*.

That is derived from the copy too, by pin_release below. A release that does not
publish an identifier the copy carries cannot be the one it was taken from; among the
releases that survive that exclusion, the one whose assertions the copy reproduces
most closely. Tagged releases rather than commits, because "which release did this
authority vendor" is a question with a nameable answer and the working commits between
two tags are drafts nobody could have downloaded. No version string is read: a CIUS
says "EN 16931" in its title whichever release it copied, and the whole lesson of C40
is that content is the arbiter where content is available.

Two things make the result honest rather than merely computed:

  * the omission set is read off the authority's *whole distribution*, every .sch
    under it, and not off its copy of CEN's files. AT/eSPap moved BR-CO-04, BR-S-01,
    BR-E-01, BR-17, BR-27 and BR-28 out of its copy of CEN's model file and into a
    pattern of its own, and a reading that looked only at the copy would report all
    six as dropped when the rule set evaluates every one of them.
  * whether an authority ships a copy of a CEN file at all is read from its *master*
    Schematron — the manifest its validator is pointed at. CIUS-RO's and both NLCIUS
    masters <include> codelist/EN16931-*-codes.sch, so those authorities do run CEN's
    code-list rules and this repository simply does not vendor their copy; CIUS-PT's
    master includes no code-list file of any name, so AT's rule set does not run one.
    A hand-written list would make those two look alike, and getting them the wrong
    way round would either invent 22 Romanian omissions or hide 19 Portuguese ones.

The result today: CIUS-PT vendored CEN validation-1.1.0 (2018-06-26) and left out 114
identifiers CEN had already published; every other copy left out none. What this
package does with that is in cius_overrides.go — nothing, deliberately, and
cius_omissions_test.go holds the reasoning to the corpus.

Failing loudly
--------------
  * a shallow testdata/en16931-artefacts aborts the run. The classification is a
    statement about CEN's history, and a clone without history would silently
    reclassify every stale condition as the authority's own — turning a lag into
    nine hundred overrides.
  * a <rule context> outside the shape the Go walk describes aborts the run.
  * an assertion or context predicate whose XPath is outside the subset the
    evaluator implements aborts the run naming the rule. The Go side parses the
    emitted table again at load.
  * an authority whose copy this generator cannot locate aborts the run rather than
    being reported as having no overrides. "We found nothing" and "we did not look"
    are the two things this repository has most often confused (C26, C33, C35, C39).
"""
import os
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

SCH = "{http://purl.oclc.org/dsdl/schematron}"
HERE = os.path.dirname(os.path.abspath(__file__))
TD = os.path.abspath(os.path.join(HERE, ".."))
CEN = os.path.join(TD, "en16931-artefacts")
OUT = os.path.abspath(os.path.join(HERE, "..", "..", "cius_overrides_table.go"))

# CEN's two syntax bindings, each an (abstract, binding) pair. The abstract file
# holds the rules, their order and their flags; the binding resolves every
# $parameter in them to XPath. Neither half is a rule set on its own.
CEN_UBL = [
    ("ubl/schematron/abstract/EN16931-model.sch", "ubl/schematron/UBL/EN16931-UBL-model.sch"),
    ("ubl/schematron/abstract/EN16931-syntax.sch", "ubl/schematron/UBL/EN16931-UBL-syntax.sch"),
    # The code-list pattern is concrete rather than abstract — CEN writes BR-CL-*
    # with its XPath in place — so it has no binding half. It is here because UBL.BE
    # copies it too, and an identifier CEN publishes in a file this generator does
    # not read would be classified as "not CEN's" and silently skipped.
    ("ubl/schematron/codelist/EN16931-UBL-codes.sch", None),
]
CEN_CII = [
    ("cii/schematron/abstract/EN16931-CII-model.sch", "cii/schematron/CII/EN16931-CII-model.sch"),
    ("cii/schematron/abstract/EN16931-CII-syntax.sch", "cii/schematron/CII/EN16931-CII-syntax.sch"),
    ("cii/schematron/codelist/EN16931-CII-codes.sch", None),
]

PT = os.path.join(TD, "cius-pt", "schematron", "2.1.1")
RO = os.path.join(TD, "cius-ro", "schematron", "1.0.9")
NL = os.path.join(TD, "nlcius", "schematron")
BE = os.path.join(TD, "cius-be", "schematron", "v1.31", "GLOBALUBL.BE.sch")
RS = os.path.join(TD, "cius-rs", "schematron", "1.0.0")

# Every copy of CEN's files this package's CIUS ship, and how to read it.
#
#   kind "pair"  — an abstract file plus a binding, the shape CEN publishes and the
#                  shape CIUS-PT, CIUS-RO and NLCIUS copy.
#   kind "flat"  — patterns already resolved, which is how UBL.BE ships them.
#   kind "none"  — the authority ships no copy of CEN's files at all. Named here so
#                  that saying so is a checked fact and not an omission.
#
# `source` is the Go Source constant the authority's reading is attributed to.
#
# `apply` is whether this package evaluates the authority's conditions. Every
# authority is *classified* whether or not it is applied, and the verdict is emitted
# either way — an authority whose overrides are not applied is a stated gap with its
# identifiers named, not a silence. `reason` says why, and is emitted with it.
AUTHORITIES = [
    {
        "name": "CIUS-PT 2.1.1 (UBL)", "source": "SourceCIUSPT", "var": "ptConditionOverrides",
        "kind": "pair", "syntax": "UBL", "history": CEN_UBL, "apply": True, "reason": "",
        "copies": {"model": "EN16931-model.sch", "syntax": "EN16931-syntax.sch"},
        "dist": PT, "master": PT + "/urn_feap.gov.pt_CIUS-PT_2.1.1.sch",
        "pairs": [
            ("model",
             PT + "/abstract/urn_feap.gov.pt_CIUS-PT_2.1.1-model.sch",
             PT + "/UBL/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-model.sch"),
            ("syntax",
             PT + "/abstract/urn_feap.gov.pt_CIUS-PT_2.1.1-syntax.sch",
             PT + "/UBL/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-syntax.sch"),
        ],
    },
    {
        "name": "CIUS-RO 1.0.9 (UBL)", "source": "SourceCIUSRO", "var": "roConditionOverrides",
        "kind": "pair", "syntax": "UBL", "history": CEN_UBL, "apply": True, "reason": "",
        "copies": {"model": "EN16931-model.sch", "syntax": "EN16931-syntax.sch"},
        "dist": RO, "master": RO + "/EN16931-CIUS_RO-UBL-validation.sch",
        "pairs": [
            ("model", RO + "/abstract/EN16931-model.sch", RO + "/UBL/EN16931-UBL-model.sch"),
            ("syntax", RO + "/abstract/EN16931-syntax.sch", RO + "/UBL/EN16931-UBL-syntax.sch"),
        ],
    },
    {
        "name": "NLCIUS SI-UBL 2.0.3.2 (UBL)", "source": "SourceNLCIUS", "var": "nlUBLConditionOverrides",
        "kind": "pair", "syntax": "UBL", "history": CEN_UBL, "apply": True, "reason": "",
        "copies": {"model": "EN16931-model.sch", "syntax": "EN16931-syntax.sch"},
        "dist": NL + "/ubl", "master": NL + "/ubl/si-ubl-2.0.3.2.sch",
        "pairs": [
            ("model", NL + "/ubl/cen/EN16931-model.sch", NL + "/ubl/cen/EN16931-UBL-model.sch"),
            ("syntax", NL + "/ubl/cen/EN16931-syntax.sch", NL + "/ubl/cen/EN16931-UBL-syntax.sch"),
        ],
    },
    {
        # The G-account extension replaces CEN's abstract syntax file with one of its
        # own and says so in the name. Read separately: it is a different rule set
        # from SI-UBL's base, not a second copy of it.
        "name": "NLCIUS SI-UBL G-account 1.0.2 (UBL)", "source": "SourceNLCIUS",
        "var": "nlGAccountConditionOverrides",
        "kind": "pair", "syntax": "UBL", "history": CEN_UBL, "apply": True, "reason": "",
        # An overlay: it replaces one of CEN's files and <include>s the whole of
        # SI-UBL 2.0 for the rest (C43). Its distribution is the entry above's, so it
        # gets no omission record of its own — the same set counted twice would read
        # as two authorities dropping the same rules.
        "omitClassified": False,
        "omitReason": "an overlay on NLCIUS SI-UBL 2.0.3.2: it replaces CEN's abstract "
                      "syntax file and <include>s the whole of SI-UBL 2.0 for the rest, "
                      "so what it omits is what that entry omits. Recorded there",
        "pairs": [
            ("syntax", NL + "/ubl/cen/EN16931-syntax-modified.sch", NL + "/ubl/cen/EN16931-UBL-syntax.sch"),
        ],
    },
    {
        "name": "NLCIUS 1.0.3 (CII)", "source": "SourceNLCIUS", "var": "nlCIIConditionOverrides",
        "kind": "pair", "syntax": "CII", "history": CEN_CII, "apply": True, "reason": "",
        "copies": {"model": "EN16931-CII-model.sch", "syntax": "EN16931-CII-syntax.sch"},
        "dist": NL + "/cii", "master": NL + "/cii/nlcius-cii-1.0.3.sch",
        "pairs": [
            ("model", NL + "/cii/cen/abstract/EN16931-CII-model.sch", NL + "/cii/cen/CII/EN16931-CII-model.sch"),
            ("syntax", NL + "/cii/cen/abstract/EN16931-CII-syntax.sch", NL + "/cii/cen/CII/EN16931-CII-syntax.sch"),
        ],
    },
    {
        "name": "UBL.BE v1.31 (UBL)", "source": "SourceUBLBE", "var": "beConditionOverrides",
        "kind": "flat", "syntax": "UBL", "history": CEN_UBL,
        # GLOBALUBL.BE.sch merges CEN's rules, OpenPEPPOL's and five OpenPEPPOL
        # country sets into one file. Only the three patterns that are CEN's copy
        # are read; the rest publish their own identifiers and are other rule sets'
        # business.
        "file": BE, "patterns": ["ubl-model", "UBL-syntax", "Codesmodel"],
        # Not classified for omissions, and the reason is measured rather than
        # asserted: GLOBALUBL.BE.sch re-cases CEN's UBL-CR-* family as ubl-CR-*, so an
        # omission set computed on exact identifiers would report the whole family as
        # dropped when the file carries every one of them. The count is derived below
        # and emitted with the reason, so the day the file stops re-casing them this
        # entry fails rather than staying quietly wrong.
        "omitClassified": False,
        "apply": False,
        "reason": "the five BR-*-08 conditions sum xs:decimal amounts across a node "
                  "population reached through the parent axis, and the two BR-CL-* ones "
                  "sit in <rule> elements whose contexts carry two predicates on one "
                  "step, which neither this generator's context grammar nor "
                  "ptDTCtxStep describes. Applying them would mean extending both and "
                  "re-measuring an arithmetic comparison over the whole corpus; "
                  "reported here rather than applied half-checked",
    },
    {
        "name": "SRBDT 1.0.0 (UBL)", "source": "SourceSRBDT", "var": None,
        "kind": "none", "syntax": "UBL", "apply": True, "reason": "",
        # EN16931-UBL-srbdt-validation.sch says so itself, in a comment: the Serbian
        # rule set is an overlay and the CEN half is expected from elsewhere. The
        # check below is that no file under this directory carries a CEN identifier.
        "dir": RS,
    },
]

# A CEN identifier is one CEN's own artefacts publish. Everything else in a CIUS's
# file — BR-CIUS-PT-*, BR-NL-*, ubl-BE-*, PEPPOL-*, the country sets — belongs to
# another authority and is another rule set's business, so it is not an override of
# anything.
#
# The membership test is "CEN publishes this identifier", read out of CEN's own
# artefacts, and not a prefix pattern. A guard that enumerates published identifiers
# through a pattern only enumerates the ones its author anticipated, which is how an
# entire BR-AA-* family came to be invisible (C39).


def norm(s):
    return " ".join((s or "").split())


# ---------------------------------------------------------------------------
# Reading a Schematron
# ---------------------------------------------------------------------------
MSG_PREFIX = re.compile(r"^\[[^]]*\]\s*-?\s*")


def message(el):
    """The assertion's own text with the leading "[rule-id]-" stripped."""
    return norm(MSG_PREFIX.sub("", norm("".join(el.itertext()))))


def params_of(root):
    out = {}
    for p in root.iter(SCH + "param"):
        out[norm(p.get("name"))] = p.get("value")
    return out


def deref(v, params):
    v = (v or "").strip()
    if v.startswith("$"):
        return params.get(norm(v[1:]), v)
    return v


def read_pair(abstract_bytes, binding_bytes):
    """Resolve one (abstract, binding) pair to a list of rules in document order.

    Each rule is {"ctx", "asserts": [(id, kind, flag, test, message)]}.
    """
    params = params_of(ET.fromstring(binding_bytes)) if binding_bytes is not None else {}
    root = ET.fromstring(abstract_bytes)
    rules = []
    for r in root.iter(SCH + "rule"):
        asserts = []
        for c in r:
            tag = c.tag.replace(SCH, "")
            if tag in ("assert", "report"):
                asserts.append((c.get("id"), tag, c.get("flag"),
                                norm(deref(c.get("test"), params)), message(c)))
        rules.append({"ctx": norm(deref(r.get("context"), params)), "asserts": asserts})
    return rules


def read_flat(path, want_patterns):
    """Read already-resolved patterns out of one file, in document order."""
    root = ET.parse(path).getroot()
    rules = []
    seen = set()
    for pat in root.iter(SCH + "pattern"):
        if pat.get("id") not in want_patterns:
            continue
        seen.add(pat.get("id"))
        for r in pat.findall(SCH + "rule"):
            asserts = []
            for c in r:
                tag = c.tag.replace(SCH, "")
                if tag in ("assert", "report"):
                    asserts.append((c.get("id"), tag, c.get("flag"),
                                    norm(c.get("test")), message(c)))
            rules.append({"ctx": norm(r.get("context")), "asserts": asserts})
    missing = sorted(set(want_patterns) - seen)
    if missing:
        sys.exit("gen.py: %s: pattern(s) %s not in the file" % (path, ", ".join(missing)))
    return rules


def index(rules):
    """identifier -> (rule position, context, kind, flag, test, message)."""
    out = {}
    for i, r in enumerate(rules):
        for rid, kind, flag, test, msg in r["asserts"]:
            out[rid] = (i, r["ctx"], kind, flag, test, msg)
    return out


# ---------------------------------------------------------------------------
# What CEN ever published
# ---------------------------------------------------------------------------
def git(*args):
    r = subprocess.run(["git", "-C", CEN] + list(args), capture_output=True)
    if r.returncode != 0:
        sys.exit("gen.py: git %s failed: %s" % (" ".join(args), r.stderr.decode().strip()))
    return r.stdout


def require_history():
    if os.path.exists(os.path.join(CEN, ".git", "shallow")):
        sys.exit(
            "gen.py: %s is a shallow clone.\n"
            "  This generator classifies a CIUS condition by asking whether CEN ever\n"
            "  published it, which is a question about the repository's history. A shallow\n"
            "  clone answers 'no' to every version CEN has since changed, which would turn\n"
            "  three stale vendored copies into nine hundred national overrides.\n"
            "  Run `make en16931-artefacts` — it clones with full history for this reason."
            % CEN)


def cen_history(pairs):
    """identifier -> {(context, kind, test)} over every commit that touched the files."""
    commits = []
    for a, b in pairs:
        for p in (a, b):
            if p is not None:
                commits += git("log", "--format=%H", "--", p).decode().split()
    commits = list(dict.fromkeys(commits))

    # One `git cat-file --batch` for every blob rather than a process per file: the
    # UBL binding alone has 138 versions and the walk is otherwise most of this
    # generator's runtime.
    want = []
    for c in commits:
        for a, b in pairs:
            want.append("%s:%s" % (c, a))
            if b is not None:
                want.append("%s:%s" % (c, b))
    blobs = cat_file(want)

    hist = {}
    for c in commits:
        for a, b in pairs:
            ab = blobs.get("%s:%s" % (c, a))
            if ab is None:
                continue
            bi = blobs.get("%s:%s" % (c, b)) if b is not None else None
            try:
                for rid, v in index(read_pair(ab, bi)).items():
                    hist.setdefault(rid, set()).add((v[1], v[2], v[4]))
            except ET.ParseError:
                # A commit in which CEN left the file mid-edit is not a release and
                # carries no verdict. Skipping it can only withhold provenance from a
                # condition, which classifies it as the authority's own and is the
                # direction that reports rather than hides.
                continue
    return hist, len(commits)


def cen_releases(pairs):
    """CEN's tagged releases, oldest first, each resolved to a per-file rule index.

    A *tag* rather than a commit, because "which release did this authority vendor"
    is a question with a nameable answer and a commit is not one. CEN tags every
    published validation artefact; the working commits between two tags are drafts
    nobody could have downloaded.
    """
    tags = git("tag").decode().split()
    dated = []
    for t in tags:
        rev = git("rev-list", "-1", t).decode().strip()
        dated.append((git("log", "-1", "--format=%cI", rev).decode().strip()[:10], t, rev))
    dated.sort()

    want = []
    for _, _, rev in dated:
        for a, b in pairs:
            want.append("%s:%s" % (rev, a))
            if b is not None:
                want.append("%s:%s" % (rev, b))
    blobs = cat_file(want)

    out = []
    for date, tag, rev in dated:
        per = {}
        for a, b in pairs:
            ab = blobs.get("%s:%s" % (rev, a))
            if ab is None:
                continue
            bi = blobs.get("%s:%s" % (rev, b)) if b is not None else None
            try:
                per[os.path.basename(a)] = index(read_pair(ab, bi))
            except ET.ParseError:
                sys.exit("gen.py: %s: %s does not parse at release %s" % (a, b, tag))
        out.append((tag, date, per))
    return out


def pin_release(name, cius, releases, copied_files):
    """Which CEN release this authority's copy was taken from, derived from the copy.

    Two facts decide it, in this order:

      * a release the copy carries an identifier of that the release does not
        publish cannot be the one the copy was taken from. That is a hard exclusion,
        not a score, and it is what rules out every release before the identifier
        was minted.
      * among the releases that survive it, the one whose assertions the copy
        reproduces most closely — comparing the whole assertion (context, polarity,
        flag, test, message), because a reworded message is as good a version
        fingerprint as a rewritten test and this comparison is not deciding whether
        a difference is national.

    Version strings are not consulted. A CIUS that says "EN 16931" in its title says
    it whichever release it copied, and the whole lesson of C40 is that the artefact's
    content is the arbiter where the content is available.

    Returns (first_tag, last_tag, date, unpublished, differing, shared). CEN
    republishes a file unchanged across releases often enough that the minimum is
    frequently a *run* of tags rather than one; reporting the run says so instead of
    picking its first member and implying more precision than the evidence carries.
    """
    ever = set()
    for _, _, per in releases:
        for f in copied_files:
            ever |= set(per.get(f, {}))
    shared = {r: v for r, v in cius.items() if r in ever}

    scored = []
    for tag, date, per in releases:
        idx = {}
        for f in copied_files:
            idx.update(per.get(f, {}))
        unpublished = sum(1 for r in shared if r not in idx)
        differing = sum(1 for r, v in shared.items()
                        if r in idx and (v[1], v[2], v[3], v[4], v[5]) !=
                        (idx[r][1], idx[r][2], idx[r][3], idx[r][4], idx[r][5]))
        scored.append((unpublished, differing, tag, date))
    scored.sort()
    best = scored[0]
    if best[0] != 0:
        sys.exit("gen.py: %s: no CEN release publishes every identifier its copy carries "
                 "(best is %s, short by %d). The copy is not a copy of a CEN release, or "
                 "the files it is read from are the wrong ones" % (name, best[2], best[0]))
    tied = sorted((t, d) for u, di, t, d in scored if (u, di) == (best[0], best[1]))
    tied.sort(key=lambda td: td[1])
    return tied[0][0], tied[-1][0], tied[0][1], best[0], best[1], len(shared)


def master_includes(path):
    """The <include href> basenames of an authority's master Schematron.

    The master is the authority's own manifest: the file its validator is pointed at,
    naming every pattern the rule set runs. It is the only thing that can answer
    "does this authority publish a copy of CEN's code-list file at all", and it
    answers it from a vendored artefact rather than from a hand-kept list.

    It matters because the three answers are different and look alike. CIUS-RO's
    master and both NLCIUS masters <include> codelist/EN16931-*-codes.sch, so those
    authorities do run CEN's code-list rules and this repository simply does not
    vendor their copy — nothing can be said about what it contains. CIUS-PT's master
    includes no code-list file of any name, so AT/eSPap's rule set does not run one.
    A hand-written "absent" list would have made those two indistinguishable, and
    getting them the wrong way round would either invent 22 Romanian omissions or
    hide 19 Portuguese ones.
    """
    root = ET.parse(path).getroot()
    return {os.path.basename(i.get("href") or "") for i in root.iter(SCH + "include")}


def omissions(published, releases, pin_tag, copied_files, included, cen_now_per_file):
    """Per CEN file, the identifiers CEN publishes today that this authority does not.

    `published` is every identifier the authority's whole distribution carries, not
    only the ones in its copy of CEN's files. That distinction is the whole accuracy
    of this table: AT/eSPap moved BR-CO-04, BR-S-01, BR-E-01, BR-17, BR-27 and BR-28
    out of its copy of CEN's model file and into a `condition` pattern of its own, and
    a reading that looked only at the copy would report all six as dropped when the
    rule set evaluates every one of them.

    Each CEN file lands in one of three states, and only two of them carry a claim:

      * copied — this repository vendors the authority's copy, so its contents can be
        compared. The identifiers CEN publishes today and the copy does not are split
        into `dropped` (CEN had published it at the release the copy was taken from:
        the authority had the rule in hand and left it out) and `postdates` (CEN had
        not: the copy's age, not a decision). That is the same discriminator the
        condition classification draws between `stale` and `own`, applied to absence.
      * absent — the authority's master includes no copy of this CEN file, so its
        rule set does not run it. Same split, over the whole file.
      * not fetched — the authority's master includes it and this repository does not
        vendor it. No claim either way; recorded so the silence is visible.
    """
    per_pin = dict((t, per) for t, _, per in releases)[pin_tag]
    out = []
    for f in sorted(cen_now_per_file):
        copied = f in copied_files
        if not copied and f in included:
            out.append({"file": f, "copied": False, "fetched": False,
                        "dropped": [], "postdates": []})
            continue
        published_then = set(per_pin.get(f, {}))
        missing = sorted(set(cen_now_per_file[f]) - published)
        out.append({
            "file": f,
            "copied": copied,
            "fetched": True,
            "dropped": [r for r in missing if r in published_then],
            "postdates": [r for r in missing if r not in published_then],
        })
    return out


def cat_file(revs):
    """Read many `commit:path` blobs with one git process. Missing ones are absent."""
    p = subprocess.Popen(["git", "-C", CEN, "cat-file", "--batch"],
                         stdin=subprocess.PIPE, stdout=subprocess.PIPE)
    out, _ = p.communicate(("\n".join(revs) + "\n").encode())
    blobs, i, n = {}, 0, 0
    while i < len(out):
        j = out.index(b"\n", i)
        header = out[i:j].decode()
        i = j + 1
        parts = header.split()
        if len(parts) == 2 and parts[1] == "missing":
            n += 1
            continue
        size = int(parts[2])
        blobs[revs[n]] = out[i:i + size]
        i += size + 1
        n += 1
    return blobs


# ---------------------------------------------------------------------------
# Contexts
#
# Every context this generator emits is a union of `|`-separated branches, and every
# branch is a path of element steps with at most one predicate each. A branch
# beginning //ubl:Invoice or //cn:CreditNote is anchored at that document element;
# one written relative is read from the document element, which is the reading
# cius_pt_datatype.go takes for the same parameters in the same files and is
# narrower than an XSLT match pattern would be.
# ---------------------------------------------------------------------------
STEP = re.compile(r"^(?:[A-Za-z][\w.]*:)?([A-Za-z][\w.]*)(?:\[(.*)\])?$")
ROOTS = {"Invoice": "Invoice", "CreditNote": "CreditNote",
         "CrossIndustryInvoice": "CrossIndustryInvoice"}


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


def parse_context(ctx, what):
    """Return [(root, [(name, pred), ...]), ...] for a <rule context>."""
    out = []
    for branch in split_top(ctx, "|"):
        branch = branch.strip()
        if not branch:
            sys.exit("gen.py: %s: empty union branch in %r" % (what, ctx))
        root = ""
        if branch.startswith("//"):
            branch = branch[2:]
        elif branch.startswith("/*/"):
            branch = branch[3:]
        elif branch.startswith("/"):
            branch = branch[1:]
        steps = []
        for i, raw in enumerate(split_top(branch, "/")):
            m = STEP.match(raw.strip())
            if not m:
                sys.exit(
                    "gen.py: %s: cannot describe the context step %r of %r\n"
                    "  Every context emitted here is a path of element steps anchored at the\n"
                    "  document element. A step outside that shape is a node population the Go\n"
                    "  walk cannot build, and emitting the rule anyway would mis-scope every\n"
                    "  assertion under it. Extend ptDTCtxPath in cius_pt_datatype_eval.go and\n"
                    "  this function together." % (what, raw.strip(), ctx))
            name, pred = m.group(1), norm(m.group(2) or "")
            if i == 0 and name in ROOTS and not pred:
                root = ROOTS[name]
                continue
            if pred:
                check_expr(what, pred)
            steps.append((name, pred))
        out.append((root, steps))
    return out


# ---------------------------------------------------------------------------
# The expression grammar
#
# A validator for the XPath subset the Go evaluator implements, run over every
# assertion and context predicate this generator is about to emit. It computes
# nothing: its only job is to refuse. Keep FUNCTIONS in step with ptDTFunctions in
# cius_pt_datatype_eval.go.
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

FUNCTIONS = {
    "normalize-space": (1, 1), "matches": (2, 3), "not": (1, 1), "starts-with": (2, 2),
    "contains": (2, 2), "concat": (2, 8), "substring-before": (2, 2),
    "substring-after": (2, 2), "substring": (2, 3), "string-length": (1, 1),
    "exists": (1, 1), "count": (1, 1), "sum": (1, 1), "round": (1, 1),
    "distinct-values": (1, 1), "index-of": (2, 2), "xs:decimal": (1, 1),
    "string": (0, 1), "text": (0, 0), "true": (0, 0), "false": (0, 0),
    "upper-case": (1, 1),
}
KEYWORDS = {"and", "or", "div", "every", "in", "satisfies", "castable", "as"}
CASTABLE_TYPES = {"xs:date"}


class Refuse(Exception):
    pass


def lex(s):
    out, i = [], 0
    while i < len(s):
        m = TOKEN.match(s, i)
        if not m:
            raise Refuse("cannot tokenise at %r" % s[i:i + 24])
        i = m.end()
        kind = m.lastgroup
        if kind != "ws":
            out.append((kind, m.group()))
    return out


class P:
    """A recursive-descent acceptor for the subset. It returns nothing."""

    def __init__(self, toks):
        self.t, self.i = toks, 0

    def peek(self):
        return self.t[self.i] if self.i < len(self.t) else (None, None)

    def at(self, text):
        k, v = self.peek()
        return v == text

    def take(self, text=None):
        k, v = self.peek()
        if v is None:
            raise Refuse("unexpected end of expression")
        if text is not None and v != text:
            raise Refuse("expected %r, found %r" % (text, v))
        self.i += 1
        return v

    def expr(self):
        self.and_()
        while self.at("or"):
            self.take()
            self.and_()

    def and_(self):
        self.cmp()
        while self.at("and"):
            self.take()
            self.cmp()

    def cmp(self):
        self.additive()
        while self.peek()[1] in ("=", "!=", "<", "<=", ">", ">="):
            self.take()
            self.additive()

    def additive(self):
        self.mult()
        while self.peek()[1] in ("+", "-"):
            self.take()
            self.mult()

    def mult(self):
        self.unary()
        while self.peek()[1] in ("*", "div"):
            self.take()
            self.unary()

    def unary(self):
        if self.at("-"):
            self.take()
        self.union()
        if self.at("castable"):
            self.take()
            self.take("as")
            k, v = self.peek()
            if v not in CASTABLE_TYPES:
                raise Refuse("castable as %r is outside the subset" % v)
            self.take()

    def union(self):
        self.path()
        while self.at("|"):
            self.take()
            self.path()

    def path(self):
        k, v = self.peek()
        if v == "every":
            self.take()
            k, v = self.peek()
            if k != "var":
                raise Refuse("every must bind a variable")
            self.take()
            self.take("in")
            self.expr()
            self.take("satisfies")
            self.expr()
            return
        if v == "(":
            self.take()
            self.expr()
            self.take(")")
            self.steps()
            return
        if k == "lit" or k == "num" or k == "var":
            self.take()
            return
        if v in ("/", "//"):
            self.take()
            self.relative()
            return
        self.relative()

    def relative(self):
        self.step()
        self.steps()

    def steps(self):
        while self.peek()[1] in ("/", "//"):
            self.take()
            self.step()

    def step(self):
        k, v = self.peek()
        if v == "..":
            self.take()
            return
        if v == ".":
            self.take()
            self.preds()
            return
        if v == "*":
            self.take()
            self.preds()
            return
        if v == "@":
            self.take()
            k, v = self.peek()
            if k != "name" and v != "*":
                raise Refuse("@ must name an attribute")
            self.take()
            return
        if k == "axis":
            self.take()
            k, v = self.peek()
        if v == "(":
            self.take()
            self.expr()
            self.take(")")
            self.preds()
            return
        if k != "name":
            raise Refuse("expected a name, found %r" % (v,))
        name = self.take()
        if self.at("("):
            if name in KEYWORDS:
                raise Refuse("%r is not a function" % name)
            if name not in FUNCTIONS:
                raise Refuse("the function %s() is outside the subset the evaluator implements" % name)
            lo, hi = FUNCTIONS[name]
            self.take("(")
            n = 0
            if not self.at(")"):
                self.expr()
                n = 1
                while self.at(","):
                    self.take()
                    self.expr()
                    n += 1
            self.take(")")
            if not (lo <= n <= hi):
                raise Refuse("%s() takes %d..%d arguments, given %d" % (name, lo, hi, n))
            self.steps()
            return
        self.preds()

    def preds(self):
        while self.at("["):
            self.take()
            self.expr()
            self.take("]")


def check_expr(what, xpath):
    try:
        p = P(lex(xpath))
        p.expr()
        if p.i != len(p.t):
            raise Refuse("trailing %r" % (p.t[p.i][1],))
    except Refuse as e:
        sys.exit(
            "gen.py: %s: %s\n"
            "  in: %s\n"
            "  The Go evaluator (cius_pt_datatype_eval.go) would not read this the way a\n"
            "  reference processor does, so the override is refused rather than emitted and\n"
            "  quietly mis-evaluated. Extend both sides together, or leave the identifier to\n"
            "  CEN's condition and say why." % (what, e, xpath))


# ---------------------------------------------------------------------------
# Classification
# ---------------------------------------------------------------------------
def classify(cius, cen_now, hist):
    """Split a CIUS's CEN identifiers three ways.

    Returns (same, stale, own): identifiers whose condition matches CEN's current
    file; identifiers whose condition differs from it but CEN published at some
    commit; and identifiers carrying an axis CEN never published.
    """
    same, stale, own = [], [], {}
    for rid, v in sorted(cius.items()):
        if rid not in cen_now:
            continue  # not CEN's identifier; another rule set's business
        pos, ctx, kind, flag, test, msg = v
        cn = cen_now[rid]
        if (ctx, kind, test) == (cn[1], cn[2], cn[4]):
            same.append(rid)
            continue
        h = hist.get(rid, set())
        axes = []
        if not any(c == ctx for c, _, _ in h):
            axes.append("context")
        if not any(k == kind for _, k, _ in h):
            axes.append("polarity")
        if not any(t == test for _, _, t in h):
            axes.append("test")
        if axes:
            own[rid] = axes
        else:
            stale.append(rid)
    return same, stale, own


# ---------------------------------------------------------------------------
# Emitting Go
# ---------------------------------------------------------------------------
def golit(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def emit_pattern(var, name, rules, own, source, authority):
    n = sum(1 for r in rules for a in r["asserts"] if a[0] in own)
    out = [
        "// %s is %s's copy of CEN's pattern, carrying the %d" % (var, authority, n),
        "// assertion%s whose condition %s wrote itself. Every <rule> of the pattern is here" % ("" if n == 1 else "s", authority),
        "// in the authority's order, because under ISO Schematron a node goes to the first",
        "// rule whose context matches it and a rule dropped for carrying nothing would hand",
        "// its nodes to a rule below it. The rules that carry no assertion are here for",
        "// their contexts alone.",
        "var %s = ptDTPattern{" % var,
        "\tname: %s," % golit(name),
        "\trules: []ptDTRuleSrc{",
    ]
    for r in rules:
        out.append("\t\t{")
        out.append("\t\t\tcontext: %s," % golit(r["ctx"]))
        parts = []
        for root, steps in r["paths"]:
            steps_src = ", ".join("{%s, %s}" % (golit(nm), golit(pred)) for nm, pred in steps)
            parts.append("{%s, []ptDTCtxStep{%s}}" % (golit(root), steps_src))
        out.append("\t\t\tpaths:   []ptDTCtxPath{" + ", ".join(parts) + "},")
        kept = [a for a in r["asserts"] if a[0] in own]
        if kept:
            out.append("\t\t\tasserts: []ptDTAssertSrc{")
            for rid, kind, flag, test, msg in kept:
                out.append("\t\t\t\t{%s, %s, %s, %s}," % (golit(rid), golit(kind), golit(test), golit(msg)))
            out.append("\t\t\t},")
        out.append("\t\t},")
    out += ["\t},", "}"]
    return "\n".join(out)


def emit_overrides(var, source, syntax, patterns, own, flags):
    out = [
        "var %s = &ciusOverrides{" % var,
        "\tauthority: %s," % source,
        "\tsyntax:    %s," % golit(syntax),
        "\trules: map[string]Severity{",
    ]
    for rid in sorted(own):
        out.append("\t\t%s: %s," % (golit(rid), flags[rid]))
    out += ["\t},", "\tpatterns: []ptDTPattern{" + ", ".join(patterns) + "},", "}"]
    return "\n".join(out)


def emit_verdicts(verdicts):
    out = [
        "// ciusCENCopyVerdicts records, per authority, what its copy of CEN's Schematron",
        "// turned out to be. Derived by testdata/cius-condition-overrides/gen.py from the",
        "// copy and from CEN's own git history, and re-derived by",
        "// TestCIUSCopiesOfCENAreClassifiedFromTheArtefacts on every run.",
        "//",
        "// stale is the count that matters for reading this table: a condition the copy",
        "// carries, CEN's current file does not, and some CEN commit did. It is a measure of",
        "// how old the vendored copy is and not of anything national. own is the count this",
        "// package acts on.",
        "var ciusCENCopyVerdicts = []ciusCENCopyVerdict{",
    ]
    for v in verdicts:
        out.append("\t{")
        out.append("\t\tauthority: %s," % golit(v["name"]))
        out.append("\t\tsource:    %s," % v["source"])
        out.append("\t\tships:     %s," % ("true" if v["ships"] else "false"))
        out.append("\t\tshared:    %d," % v["shared"])
        out.append("\t\tsame:      %d," % v["same"])
        out.append("\t\tstale:     %d," % v["stale"])
        out.append("\t\tapplied:   %s," % ("true" if v["applied"] else "false"))
        if v["reason"]:
            out.append("\t\tnotApplied: %s," % golit(v["reason"]))
        out.append("\t\town:       map[string]string{")
        for rid in sorted(v["own"]):
            out.append("\t\t\t%s: %s," % (golit(rid), golit(", ".join(v["own"][rid]))))
        out.append("\t\t},")
        out.append("\t},")
    out.append("}")
    return "\n".join(out)


def emit_omissions(records):
    out = [
        "// ciusCENCopyOmissions records, per authority that ships a copy of CEN's",
        "// Schematron, which CEN release the copy was taken from and which CEN identifiers",
        "// the copy does not carry. Derived by testdata/cius-condition-overrides/gen.py and",
        "// re-derived by TestCIUSCopyOmissionsAreClassifiedFromTheArtefacts on every run.",
        "//",
        "// It is the absence half of the question ciusCENCopyVerdicts answers for differing",
        "// conditions, and it is split by the same discriminator: CEN's own history. An",
        "// identifier the copy lacks that CEN had published when the copy was taken is one",
        "// the authority dropped; one CEN had not published yet is the copy's age.",
        "var ciusCENCopyOmissions = []ciusCENCopyOmission{",
    ]
    for r in records:
        out.append("\t{")
        out.append("\t\tauthority: %s," % golit(r["name"]))
        out.append("\t\tsource:    %s," % r["source"])
        out.append("\t\tclassified: %s," % ("true" if r["classified"] else "false"))
        if not r["classified"]:
            out.append("\t\tnotClassified: %s," % golit(r["notClassified"]))
            out.append("\t},")
            continue
        out.append("\t\trelease:     %s," % golit(r["release"]))
        out.append("\t\treleaseThrough: %s," % golit(r["through"]))
        out.append("\t\treleaseDate: %s," % golit(r["date"]))
        out.append("\t\tcarried:     %d," % r["shared"])
        out.append("\t\tdiffering:   %d," % r["differing"])
        out.append("\t\toverlay:     %s," % ("true" if r["overlay"] else "false"))
        out.append("\t\tfiles: []ciusCENFileOmission{")
        for f in r["files"]:
            out.append("\t\t\t{")
            out.append("\t\t\t\tcenFile: %s," % golit(f["file"]))
            out.append("\t\t\t\tcopied:  %s," % ("true" if f["copied"] else "false"))
            out.append("\t\t\t\tfetched: %s," % ("true" if f["fetched"] else "false"))
            for key in ("dropped", "postdates"):
                if not f[key]:
                    continue
                out.append("\t\t\t\t%s: []string{" % key)
                line = "\t\t\t\t\t"
                for rid in f[key]:
                    if len(line) + len(rid) + 4 > 96:
                        out.append(line)
                        line = "\t\t\t\t\t"
                    line += "%s, " % golit(rid)
                out.append(line.rstrip())
                out.append("\t\t\t\t},")
            out.append("\t\t\t},")
        out.append("\t\t},")
        out.append("\t},")
    out.append("}")
    return "\n".join(out)


HEADER = '''package formalis

// Code generated from the CIUS Schematrons and CEN/TC 434's own git history by
// testdata/cius-condition-overrides/gen.py; DO NOT EDIT. Regenerate with
// `make cius-condition-overrides`.
//
// Five of the CIUS this package validates against ship a *copy* of CEN's Schematron
// rather than referencing it, and a copy can be edited. This file is the answer to
// "which CEN conditions did each authority write itself", derived by resolving each
// copy and asking CEN's repository whether it ever published what the copy carries.
// A condition CEN published at some release is a stale vendored copy and not a
// national reading; one CEN never published is the authority's own, and only those
// are overridden. The reasoning is in cius_overrides.go and the derivation in the
// generator's docstring.
'''


def main():
    require_history()
    if not os.path.isdir(CEN):
        sys.exit("gen.py: %s is missing; run `make en16931-artefacts`" % CEN)

    cen_now, cen_now_ubl_per_file = {}, {}
    for a, b in CEN_UBL:
        ix = index(read_pair(open(os.path.join(CEN, a), "rb").read(),
                             open(os.path.join(CEN, b), "rb").read() if b else None))
        cen_now.update(ix)
        cen_now_ubl_per_file[os.path.basename(a)] = ix
    cen_cii_now, cen_now_cii_per_file = {}, {}
    for a, b in CEN_CII:
        ix = index(read_pair(open(os.path.join(CEN, a), "rb").read(),
                             open(os.path.join(CEN, b), "rb").read() if b else None))
        cen_cii_now.update(ix)
        cen_now_cii_per_file[os.path.basename(a)] = ix

    hist_ubl, n_ubl = cen_history(CEN_UBL)
    hist_cii, n_cii = cen_history(CEN_CII)
    sys.stderr.write("gen.py: CEN history: %d commits touching the UBL binding, %d the CII binding\n"
                     % (n_ubl, n_cii))

    rel_ubl = cen_releases(CEN_UBL)
    rel_cii = cen_releases(CEN_CII)
    sys.stderr.write("gen.py: CEN releases: %d tagged, %s .. %s\n"
                     % (len(rel_ubl), rel_ubl[0][0], rel_ubl[-1][0]))

    verdicts, blocks, omits = [], [], []
    for auth in AUTHORITIES:
        now = cen_now if auth["syntax"] == "UBL" else cen_cii_now
        hist = hist_ubl if auth["syntax"] == "UBL" else hist_cii
        releases = rel_ubl if auth["syntax"] == "UBL" else rel_cii
        now_per_file = cen_now_ubl_per_file if auth["syntax"] == "UBL" else cen_now_cii_per_file

        if auth["kind"] == "none":
            found = cen_identifiers_under(auth["dir"], now)
            if found:
                sys.exit("gen.py: %s is recorded as shipping no copy of CEN's files, but %s\n"
                         "  carry CEN identifiers: %s" % (auth["name"], auth["dir"], ", ".join(sorted(found)[:8])))
            verdicts.append({"name": auth["name"], "source": auth["source"], "ships": False,
                             "shared": 0, "same": 0, "stale": 0, "own": {},
                             "applied": True, "reason": ""})
            omits.append({"name": auth["name"], "source": auth["source"], "classified": False,
                          "notClassified": "ships no copy of CEN's files, so it omits all of them; "
                                           "EN16931-UBL-srbdt-validation.sch says so itself, in a "
                                           "comment, and ciusCENCopyVerdicts records the same fact "
                                           "as ships: false"})
            sys.stderr.write("gen.py: %-38s ships no copy of CEN's files\n" % auth["name"])
            continue

        if auth["kind"] == "pair":
            groups = []
            for label, a, b in auth["pairs"]:
                if not os.path.exists(a):
                    sys.exit("gen.py: %s: %s is missing; run `make cius-schematron`" % (auth["name"], a))
                groups.append((label, read_pair(open(a, "rb").read(), open(b, "rb").read())))
        else:
            if not os.path.exists(auth["file"]):
                sys.exit("gen.py: %s: %s is missing; run `make cius-schematron`" % (auth["name"], auth["file"]))
            groups = [("cen-copy", read_flat(auth["file"], auth["patterns"]))]

        cius = {}
        for _, rules in groups:
            cius.update(index(rules))
        same, stale, own = classify(cius, now, hist)
        applied = auth["apply"] or not own
        verdicts.append({"name": auth["name"], "source": auth["source"], "ships": True,
                         "shared": len(same) + len(stale) + len(own),
                         "same": len(same), "stale": len(stale), "own": own,
                         "applied": applied, "reason": "" if applied else auth["reason"]})
        sys.stderr.write("gen.py: %-38s shared %4d  same %4d  stale %3d  own %d %s%s\n"
                         % (auth["name"], len(same) + len(stale) + len(own), len(same),
                            len(stale), len(own), sorted(own) if own else "",
                            "" if applied else "  (recorded, not applied)"))

        # The absence half. Classified for every copy that uses CEN's identifiers
        # verbatim; recorded with a derived reason for the one that does not.
        if not auth.get("omitClassified", True):
            reason = auth.get("omitReason")
            if reason is None:
                recased = sorted(r for r in cius
                                 if r not in now and r.lower() in {k.lower() for k in now})
                reason = ("its file re-cases %d of CEN's identifiers (UBL-CR-001 as ubl-CR-001 "
                          "and so on), so an omission set computed on exact identifiers would "
                          "report a family the file carries in full as dropped. Classifying it "
                          "means deciding whether a re-cased identifier is CEN's, which is a "
                          "question about this authority's identifier namespace and not about "
                          "absence" % len(recased))
            omits.append({"name": auth["name"], "source": auth["source"], "classified": False,
                          "notClassified": reason})
            sys.stderr.write("gen.py: %-38s omissions not classified\n" % auth["name"])
        else:
            copied_files = set(auth["copies"].values())
            for f in copied_files:
                if f not in now_per_file:
                    sys.exit("gen.py: %s names %s, which is not one of CEN's files for %s"
                             % (auth["name"], f, auth["syntax"]))
            for label in auth["copies"]:
                if label not in dict(groups):
                    sys.exit("gen.py: %s: copies names the group %r and there is none"
                             % (auth["name"], label))
            included = master_includes(auth["master"])
            first, last, date, unpub, differing, shared = pin_release(
                auth["name"], cius, releases, copied_files)
            # Every CEN identifier the authority's *whole* distribution publishes,
            # read off every .sch under it rather than off its copy of CEN's files:
            # an authority that moved a CEN rule into a pattern of its own still
            # evaluates it, and counting it as dropped would be a false accusation.
            published = set(cen_identifiers_under(auth["dist"], now))
            files = omissions(published, releases, first, copied_files, included, now_per_file)
            omits.append({"name": auth["name"], "source": auth["source"], "classified": True,
                          "release": first, "through": last if last != first else "",
                          "date": date, "shared": shared, "differing": differing,
                          "overlay": bool(auth.get("overlay")), "files": files})
            sys.stderr.write("gen.py: %-38s vendored CEN %s%s (%s): dropped %d, postdates %d\n"
                             % (auth["name"], first, " .. " + last if last != first else "", date,
                                sum(len(f["dropped"]) for f in files),
                                sum(len(f["postdates"]) for f in files)))

        if not own or not applied:
            continue

        flags = {rid: ("SeverityWarning" if cius[rid][3] == "warning" else "SeverityFatal") for rid in own}
        names = []
        for label, rules in groups:
            if not any(a[0] in own for r in rules for a in r["asserts"]):
                continue
            for r in rules:
                r["paths"] = parse_context(r["ctx"], "%s %s" % (auth["name"], r["ctx"]))
                for rid, kind, flag, test, msg in r["asserts"]:
                    if rid in own:
                        check_expr("%s %s" % (auth["name"], rid), test)
            var = "%sOverride%sPattern" % (auth["var"].replace("ConditionOverrides", ""), label.capitalize())
            blocks.append(emit_pattern(var, label, rules, own, auth["source"], auth["name"]))
            names.append(var)
        blocks.append(emit_overrides(auth["var"], auth["source"], auth["syntax"], names, own, flags))

    body = (HEADER + "\n" + emit_verdicts(verdicts) + "\n\n" + emit_omissions(omits)
            + "\n\n" + "\n\n".join(blocks) + "\n")
    open(OUT, "w").write(body)
    sys.stderr.write("gen.py: wrote %s\n" % OUT)


def cen_identifiers_under(directory, cen_now):
    """Every CEN identifier any Schematron under directory publishes."""
    found = set()
    for dirpath, _, files in os.walk(directory):
        for f in files:
            if not f.endswith(".sch"):
                continue
            try:
                root = ET.parse(os.path.join(dirpath, f)).getroot()
            except ET.ParseError:
                continue
            for c in root.iter():
                if c.tag.replace(SCH, "") in ("assert", "report") and c.get("id") in cen_now:
                    found.add(c.get("id"))
    return found


if __name__ == "__main__":
    main()
