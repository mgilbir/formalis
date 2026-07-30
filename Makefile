# Every recipe runs under bash with `set -e -u -o pipefail`. This is load-bearing
# rather than tidiness: the oracle fetches below are ~600 downloads driven by
# `gh api | while read`, and under /bin/sh's defaults a failed `gh api` produces
# an empty list, the loop body never runs, and the target still succeeds. A
# corpus that half-arrived would then be indistinguishable from a corpus that
# arrived, and the FP=0 oracles would report a green verdict backed by whatever
# happened to land (C8). With pipefail, a failed `gh api` fails the recipe line.
SHELL := /bin/bash
.SHELLFLAGS := -e -u -o pipefail -c

# -f is the other half of the same problem: without it curl writes the server's
# error body to the output file and exits 0, so a 404 or a rate-limit response
# becomes an .xml file containing "404: Not Found". --retry rides out the
# transient 5xx that a few hundred sequential requests will otherwise hit.
CURL := curl -fsSL --retry 3 --retry-delay 2 --retry-connrefused

# Percent-encodes a repository path one segment at a time, so it can be pasted
# into a raw.githubusercontent URL. Several sample filenames carry spaces or
# non-ASCII characters — "Finvoice 2.01 example.xml", the Portuguese and
# Romanian samples — and curl refuses a URL containing them outright ("URL
# rejected: Malformed input to a URL function"). Under the old unchecked `curl
# -sSL` that refusal was invisible: the file was simply never fetched, the loop
# carried on, and the corpus was quietly one document short. Every loop that
# builds a URL from a path goes through this.
URLENC := python3 -c "import urllib.parse,sys;print('/'.join(urllib.parse.quote(x) for x in sys.argv[1].split('/')))"

.PHONY: test check-deps en16931-artefacts en16931-codelists en16931-genericode \
	en16931-syntax-rules en16931-ubl cius-oracles cius-schematron \
	clean-en16931-artefacts clean-en16931-codelists clean-en16931-ubl clean-cius-oracles

# Run the tests. Oracle-backed tests skip when their (gitignored) data is absent;
# fetch it with the targets below. When it is present, each oracle also ratchets
# the number of documents it saw — see corpus_test.go — so a partial fetch is a
# red build rather than a quiet one.
test:
	go test ./... -count=1

# Prerequisites for the fetch targets, checked up front rather than 200 lines
# into a download. `gh` must be authenticated: the fetches make ~15 `gh api`
# calls, and the unauthenticated rate limit is 60/hour, which is not enough.
# python3 URL-encodes filenames — several Romanian and Portuguese samples carry
# non-ASCII characters. `gh --jq` uses gh's embedded gojq, so a separate jq is
# not needed.
REQUIRED_TOOLS := git bash curl gh python3
check-deps:
	@missing=""; \
	for t in $(REQUIRED_TOOLS); do command -v "$$t" >/dev/null 2>&1 || missing="$$missing $$t"; done; \
	if [ -n "$$missing" ]; then \
		echo "make: missing required tool(s):$$missing" >&2; \
		echo "  the oracle fetches need: $(REQUIRED_TOOLS)" >&2; \
		exit 1; \
	fi; \
	if ! gh auth status >/dev/null 2>&1; then \
		echo "make: gh is installed but not authenticated" >&2; \
		echo "  run \`gh auth login\`, or set GH_TOKEN — the fetches make ~15 gh api calls" >&2; \
		echo "  and the unauthenticated rate limit (60/hour) is not enough for them" >&2; \
		exit 1; \
	fi

UBL_DIR := testdata/en16931-ubl
EN16931_DIR := testdata/en16931-artefacts
CODELISTS_DIR := testdata/en16931-codelists
CIUS_STAMP := testdata/.cius-oracles.ok
CIUS_SCH_STAMP := testdata/.cius-schematron.ok

# git-sync clones $(1) into $(2), or refreshes it if it is already there, so the
# fetch targets can be re-run without `git clone` failing on an existing
# directory (C21). One shell line: each recipe line gets its own shell.
git-sync = if [ -d "$(2)/.git" ]; then git -C "$(2)" fetch --depth 1 origin HEAD && git -C "$(2)" reset --hard --quiet FETCH_HEAD; else rm -rf "$(2)" && git clone --quiet --depth 1 "$(1)" "$(2)"; fi

# EN 16931 UBL example invoices (CEN TC 434 + OpenPEPPOL) — the UBL FP=0 oracle.
en16931-ubl: $(UBL_DIR)/.ok
$(UBL_DIR)/.ok: $(UBL_DIR)/sources.tsv $(UBL_DIR)/download.sh
	bash $(UBL_DIR)/download.sh
	touch $@
clean-en16931-ubl:
	rm -f $(UBL_DIR)/*.xml $(UBL_DIR)/.ok

# Official CEN/TC 434 EN 16931 artefacts (Schematron + per-rule unit-test suite);
# differential oracle for the rule engine.
en16931-artefacts: $(EN16931_DIR)/.ok
$(EN16931_DIR)/.ok:
	$(call git-sync,https://github.com/ConnectingEurope/eInvoicing-EN16931,$(EN16931_DIR))
	touch $@
clean-en16931-artefacts:
	rm -rf $(EN16931_DIR)

# Official code lists (genericode + EAS/VATEX), in two halves. The bundle is the
# oracle en16931_codelists_test.go checks the committed tables against, and
# fetching it needs nothing but curl and unzip; gen.py *regenerates* those
# tables, which is a deliberate act and not something a test run should do. CI
# wants the first half only.
en16931-genericode: $(CODELISTS_DIR)/genericode/.ok
$(CODELISTS_DIR)/genericode/.ok: $(CODELISTS_DIR)/download.sh
	bash $(CODELISTS_DIR)/download.sh
	touch $@
en16931-codelists: en16931-genericode
	python3 $(CODELISTS_DIR)/gen.py
clean-en16931-codelists:
	rm -rf $(CODELISTS_DIR)/genericode $(CODELISTS_DIR)/*.zip $(CODELISTS_DIR)/*.xlsx

# The advisory halves of CEN's two syntax bindings, generated from the vendored
# Schematron into en16931_syntax_advisory_table.go. Same arrangement as the code
# lists above: the generator is a deliberate act, and the test that re-derives the
# same table from the Schematron runs on every `make test`.
#
# The test run at the end is not belt-and-braces. gen.py refuses an assertion
# whose XPath is outside the subset it recognises, and the package refuses to
# compile a table its own parser cannot read; those are two independent gates on
# the same property, and the second one is only reached by running the tests. A
# regeneration that wrote a file the package cannot read should fail here rather
# than in someone's build.
SYNTAX_RULES_DIR := testdata/en16931-syntax-rules
en16931-syntax-rules: en16931-artefacts
	python3 $(SYNTAX_RULES_DIR)/gen.py
	go test -count=1 -run 'TestAdvisory' .

# National CIUS oracles: KoSIT XRechnung, OpenPEPPOL BIS 3, the Dutch NLCIUS
# (SimplerInvoicing SI-UBL) instance test suite, and the national-format sample
# sets from phax/phive-rules.
#
# Guarded by a stamp file so a second `make cius-oracles` is a no-op instead of
# ~600 repeated downloads; `make clean-cius-oracles` removes it. check-deps is an
# order-only prerequisite: it runs first every time, and never makes the stamp
# look stale.
cius-oracles: $(CIUS_STAMP) $(CIUS_SCH_STAMP)
$(CIUS_STAMP): | check-deps
	$(call git-sync,https://github.com/itplr-kosit/xrechnung-schematron,testdata/xrechnung/schematron)
	$(call git-sync,https://github.com/itplr-kosit/xrechnung-testsuite,testdata/xrechnung/testsuite)
	$(call git-sync,https://github.com/OpenPEPPOL/peppol-bis-invoice-3,testdata/peppol/repo)
	mkdir -p testdata/nlcius/testsuite
	gh api repos/phax/phive-rules/contents/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/SI-UBL-2.0.3.2 --jq '.[].name' \
		| grep '\.xml$$' \
		| while read f; do $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/SI-UBL-2.0.3.2/$$f" -o "testdata/nlcius/testsuite/$$f"; done
	@# CIUS-PT (Portuguese AT/eSPap) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-pt/testsuite
	for ver in 2.0.0 2.1.1; do \
		gh api "repos/phax/phive-rules/contents/phive-rules-cius-pt/src/test/resources/external/test-files/$$ver" --jq '.[] | select(.name|endswith(".xml")) | .name' \
		| while read -r name; do \
			enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
			$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-cius-pt/src/test/resources/external/test-files/$$ver/$$enc" -o "testdata/cius-pt/testsuite/$${ver}_$$name"; \
		done; \
	done
	@# CIUS-RO (Romanian ANAF RO e-Factura) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-ro/testsuite
	for ver in 1.0.3 1.0.4 1.0.8 1.0.9; do \
		gh api "repos/phax/phive-rules/contents/phive-rules-cius-ro/src/test/resources/external/test-files/$$ver" --jq '.[] | select(.name|endswith(".xml")) | .name' \
		| while read -r name; do \
			enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
			$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-cius-ro/src/test/resources/external/test-files/$$ver/$$enc" -o "testdata/cius-ro/testsuite/$${ver}_$$name"; \
		done; \
	done
	@# UBL.BE (Belgian) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-be/testsuite
	gh api "repos/phax/phive-rules/contents/phive-rules-ublbe/src/test/resources/external/test-files/en16931/v1.31" --jq '.[] | select(.name|endswith(".xml")) | .name' \
	| while read -r name; do \
		enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
		$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-ublbe/src/test/resources/external/test-files/en16931/v1.31/$$enc" -o "testdata/cius-be/testsuite/$$name"; \
	done
	@# SRBDT (Serbian) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-rs/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-serbia/src/test/resources/external/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$(basename "$$p")"); \
		$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$(dirname "$$p")/$$enc" -o "testdata/cius-rs/testsuite/$$(basename "$$p")"; \
	done
	@# FatturaPA (Italian FatturaElettronica) sample instances, from phax/phive-rules.
	mkdir -p testdata/fatturapa/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-fatturapa/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/fatturapa/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# Facturae (Spanish) sample instances, from phax/phive-rules.
	mkdir -p testdata/facturae/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-facturae/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/facturae/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# ebInterface (Austrian) sample instances, from phax/phive-rules.
	mkdir -p testdata/ebinterface/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-ebinterface/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/ebinterface/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# KSeF (Polish FA structured invoice) samples — current FA(3) version — from phax/phive-rules.
	mkdir -p testdata/ksef/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-ksef/") and contains("/test-files/fa3/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/ksef/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# Finvoice (Finnish) sample instances, from phax/phive-rules.
	mkdir -p testdata/finvoice/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-finvoice/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/finvoice/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# ZATCA (Saudi Fatoora) UBL sample instances, from phax/phive-rules.
	mkdir -p testdata/zatca/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-zatca/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		i=$$((i+1)); enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/zatca/testsuite/f$$i.xml"; \
	done
	@# Svefaktura (Swedish) and TEAPPS (Finnish) sample instances, from phax/phive-rules.
	for m in svefaktura teapps; do \
		mkdir -p testdata/$$m/testsuite; \
		gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq ".tree[].path | select(contains(\"phive-rules-$$m/\") and contains(\"/test-files/\") and endswith(\".xml\"))" \
		| while read -r p; do \
			enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/$$m/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
		done; \
	done
	@# OIOUBL (Danish) invoices — content-filtered to Invoice-rooted OIOUBL files.
	mkdir -p testdata/oioubl/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-oioubl/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); b=$$($(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc"); \
		if echo "$$b" | grep -qE "<Invoice[ >]" && echo "$$b" | grep -q "CustomizationID>OIOUBL"; then \
			i=$$((i+1)); echo "$$b" > "testdata/oioubl/testsuite/inv$$i.xml"; \
		fi; \
	done
	@# UBL-TR (Turkish) sample instances, from phax/phive-rules.
	mkdir -p testdata/turkey/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-turkey/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		i=$$((i+1)); enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/turkey/testsuite/f$$i.xml"; \
	done
	@# OSA (Hungarian NAV Online Szamla) sample instances, from phax/phive-rules.
	mkdir -p testdata/osa/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-osa/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		i=$$((i+1)); enc=$$($(URLENC) "$$p"); \
		$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/osa/testsuite/f$$i.xml"; \
	done
	@# Peppol PINT (all jurisdictions) sample instances, from phax/phive-rules.
	@# awk rather than `head -8`: head closes the pipe after the eighth line, and
	@# under pipefail the SIGPIPE it delivers upstream would fail the recipe.
	mkdir -p testdata/pint/testsuite
	for j in pint-ae pint-aunz pint-eu pint-jp pint-jp-ntr pint-my pint-om pint-sg; do \
		gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq ".tree[].path | select(contains(\"phive-rules-peppol-pint/\") and contains(\"/test-files/$$j/\") and endswith(\".xml\"))" \
		| grep -iv invalid | awk 'NR<=8' | while read -r p; do \
			enc=$$($(URLENC) "$$p"); \
			$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/pint/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#[/ ]#_#g')"; \
		done; \
	done
	touch $@

# The five national rule sets whose *Schematron* this repository needs and used
# not to fetch. `cius-oracles` above pulls each authority's sample instances out
# of phax/phive-rules under src/test/resources/external/test-files/ and walked
# straight past src/test/resources/external/rule-source/, which is where the same
# repository keeps the authorities' published rule sets. The consequence was not
# cosmetic: with no artefact to quote, every Coverage severity for CIUS-PT,
# CIUS-RO, UBL.BE, SRBDT and NLCIUS was this package's fail-safe guess, no
# implemented rule's XPath had ever been checked against the published one, and
# those five Sources had to be held permanently ineligible for
# RuleFamily.Unevaluable — a claim about a published file cannot be checked for a
# file that is not here (C35). They are .sch only, so this adds no documents to
# the instance corpora.
#
# Which version of each, and why: the one that matches the sample instances
# `cius-oracles` already fetches, so the artefact and the documents validated
# against it are the same release of the same rule set.
#
#   CIUS-PT   2.0.0 and 2.1.1     — test-files/2.0.0 and test-files/2.1.1
#   CIUS-RO   1.0.3/4/8/9         — test-files/1.0.3, 1.0.4, 1.0.8, 1.0.9
#   UBL.BE    en16931/v1.31       — test-files/en16931/v1.31
#   SRBDT     1.0.0               — the only rule-source version published (the
#                                   instances cover 1.0.0 and 1.1.0)
#   NLCIUS    SI-UBL 2.0.3.2      — test-files/simplerinvoicing/SI-UBL-2.0.3.2
#             NLCIUS-CII 1.0.3    — test-files/simplerinvoicing/NLCIUS-CII-1.0.3;
#                                   the CII binding is a separate version series
#                                   and this is the one whose instances are named
#
# What is deliberately *not* fetched: the copies of CEN's own EN16931-*.sch that
# the CIUS-RO and NLCIUS-CII trees vendor as dependencies. Those are already here
# under testdata/en16931-artefacts, and pulling them in beside a national rule set
# is how a survey comes to count CEN's rules as Portuguese or Romanian ones.
# CIUS-PT is the exception that proves the point: its abstract files are a
# *modified* copy of CEN's, in which the UBL-SR-19/20/21 CEN flags fatal are
# flagged warning, so they are fetched and the readers filter on the identifier
# prefix rather than on the file.
#
# Every fetch is one `$(CURL)` (which is -f, so a 404 is a failure and not an .sch
# containing "404: Not Found") on an explicit path under `set -euo pipefail`, so a
# file that moved upstream fails this recipe rather than quietly shrinking the
# oracle — C8's lesson, and C26's. The Go-side ratchets are the second half: each
# reader in cius_artefacts_test.go asserts a floor on the number of identifiers it
# decoded, so a file that arrives truncated or empty is a red build too.
PHIVE := https://raw.githubusercontent.com/phax/phive-rules/master
PT_RULESRC := phive-rules-cius-pt/src/test/resources/external/rule-source
RO_RULESRC := phive-rules-cius-ro/src/test/resources/external/rule-source
BE_RULESRC := phive-rules-ublbe/src/test/resources/external/rule-source
RS_RULESRC := phive-rules-serbia/src/test/resources/external/rule-source
NL_RULESRC := phive-rules-simplerinvoicing/src/test/resources/external/rule-source

cius-schematron: $(CIUS_SCH_STAMP)
$(CIUS_SCH_STAMP): | check-deps
	@# CIUS-PT (Portuguese AT/eSPap). The assertions live in the abstract files and
	@# the UBL files bind them to XPath through <param>, exactly as CEN's do, so
	@# both halves are needed to read a rule.
	for ver in 2.0.0 2.1.1; do \
		for f in "abstract/urn_feap.gov.pt_CIUS-PT_$$ver-model.sch" \
			"abstract/urn_feap.gov.pt_CIUS-PT_$$ver-syntax.sch" \
			"abstract/urn_feap.gov.pt_CIUS-PT_$$ver-condition.sch" \
			"UBL/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-model.sch" \
			"UBL/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-syntax.sch" \
			"UBL/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-condition.sch" \
			"datatype/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-datatype.sch" \
			"urn_feap.gov.pt_CIUS-PT_$$ver.sch"; do \
			mkdir -p "$$(dirname "testdata/cius-pt/schematron/$$ver/$$f")"; \
			$(CURL) "$(PHIVE)/$$($(URLENC) "$(PT_RULESRC)/$$ver/$$f")" -o "testdata/cius-pt/schematron/$$ver/$$f"; \
		done; \
	done
	@# CIUS-RO (Romanian ANAF RO e-Factura). cius-ro/RO16931-rules.sch is the whole
	@# national rule set; the validation file beside it is the wrapper that says
	@# which patterns are active. The UBL/, abstract/ and codelist/ siblings are
	@# CEN's, and are not fetched.
	for ver in 1.0.3 1.0.4 1.0.8 1.0.9; do \
		mkdir -p "testdata/cius-ro/schematron/$$ver/cius-ro"; \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RO_RULESRC)/$$ver/cius-ro/RO16931-rules.sch")" -o "testdata/cius-ro/schematron/$$ver/cius-ro/RO16931-rules.sch"; \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RO_RULESRC)/$$ver/EN16931-CIUS_RO-UBL-validation.sch")" -o "testdata/cius-ro/schematron/$$ver/EN16931-CIUS_RO-UBL-validation.sch"; \
	done
	@# UBL.BE (Belgian). One file, and it is a merged artefact: alongside the 15
	@# ubl-BE-* rules it carries CEN's, OpenPEPPOL's and five of OpenPEPPOL's
	@# country sets. The reader filters on the ubl-BE- prefix.
	mkdir -p testdata/cius-be/schematron/v1.31
	$(CURL) "$(PHIVE)/$$($(URLENC) "$(BE_RULESRC)/en16931/v1.31/GLOBALUBL.BE.sch")" -o testdata/cius-be/schematron/v1.31/GLOBALUBL.BE.sch
	@# SRBDT (Serbian Ministry of Finance). The -pdvcat-* files are the per-VAT-
	@# category halves of one rule set and only -gen carries assertions, but all
	@# five are fetched: which of them is empty is upstream's business and not a
	@# fact this fetch should bake in.
	mkdir -p testdata/cius-rs/schematron/1.0.0
	for f in EN16931-UBL-srbdt.sch EN16931-UBL-srbdt-validation.sch \
		EN16931-UBL-srbdt-pdvcat-gen.sch EN16931-UBL-srbdt-pdvcat-n.sch \
		EN16931-UBL-srbdt-pdvcat-oe.sch EN16931-UBL-srbdt-pdvcat-r.sch \
		EN16931-UBL-srbdt-pdvcat-ss.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RS_RULESRC)/1.0.0/$$f")" -o "testdata/cius-rs/schematron/1.0.0/$$f"; \
	done
	@# NLCIUS (Dutch SimplerInvoicing), both bindings. This is the one CIUS of the
	@# five that genuinely publishes a CII binding as well as a UBL one, and the two
	@# do not publish the same identifiers or gate them the same way, which is the
	@# fact a single-engine implementation is most likely to get wrong.
	mkdir -p testdata/nlcius/schematron/ubl testdata/nlcius/schematron/cii
	for f in si-ubl-2.0.3.2.sch si-ubl-2.0/si-ubl-2.0-nlcius.sch si-ubl-2.0-ext-gaccount-1.0.2.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(NL_RULESRC)/simplerinvoicing/2.0.3.2/$$f")" -o "testdata/nlcius/schematron/ubl/$$(basename "$$f")"; \
	done
	for f in nlcius-cii-1.0.3.sch nlcius-cii/NLCIUS-CII-validation.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(NL_RULESRC)/nlcius/1.0.3/$$f")" -o "testdata/nlcius/schematron/cii/$$(basename "$$f")"; \
	done
	@# The fetch-side ratchet. Every file above is fetched with curl -f under
	@# pipefail, so a missing one has already failed the recipe; this catches the
	@# case a per-file check cannot see — a list that was edited down.
	@n=$$(find testdata/cius-pt/schematron testdata/cius-ro/schematron testdata/cius-be/schematron \
		testdata/cius-rs/schematron testdata/nlcius/schematron -name '*.sch' | wc -l); \
	if [ "$$n" -lt 37 ]; then \
		echo "make: fetched only $$n CIUS Schematron files, expected at least 37" >&2; \
		exit 1; \
	fi; \
	echo "make: vendored $$n CIUS Schematron files"
	touch $@
clean-cius-oracles:
	rm -rf testdata/xrechnung testdata/peppol testdata/nlcius testdata/cius-pt testdata/cius-ro testdata/cius-be testdata/cius-rs testdata/fatturapa testdata/facturae testdata/ebinterface testdata/ksef testdata/finvoice testdata/zatca testdata/svefaktura testdata/teapps testdata/oioubl testdata/turkey testdata/osa testdata/pint $(CIUS_STAMP) $(CIUS_SCH_STAMP)
