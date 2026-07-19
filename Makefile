.PHONY: test en16931-artefacts en16931-codelists en16931-ubl cius-oracles \
	clean-en16931-artefacts clean-en16931-codelists clean-en16931-ubl clean-cius-oracles

# Run the tests. Oracle-backed tests skip when their (gitignored) data is absent;
# fetch it with the targets below.
test:
	go test ./...

UBL_DIR := testdata/en16931-ubl
EN16931_DIR := testdata/en16931-artefacts
CODELISTS_DIR := testdata/en16931-codelists

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
	git clone --depth 1 https://github.com/ConnectingEurope/eInvoicing-EN16931 $(EN16931_DIR)
	touch $@
clean-en16931-artefacts:
	rm -rf $(EN16931_DIR)

# Official code lists (genericode + EAS/VATEX). gen.py regenerates
# en16931_codelists.go; the fidelity test checks the committed tables.
en16931-codelists:
	bash $(CODELISTS_DIR)/download.sh
	python3 $(CODELISTS_DIR)/gen.py
clean-en16931-codelists:
	rm -rf $(CODELISTS_DIR)/genericode $(CODELISTS_DIR)/*.zip $(CODELISTS_DIR)/*.xlsx

# National CIUS oracles: KoSIT XRechnung, OpenPEPPOL BIS 3, and the Dutch NLCIUS
# (SimplerInvoicing SI-UBL) instance test suite.
cius-oracles:
	git clone --depth 1 https://github.com/itplr-kosit/xrechnung-schematron testdata/xrechnung/schematron
	git clone --depth 1 https://github.com/itplr-kosit/xrechnung-testsuite testdata/xrechnung/testsuite
	git clone --depth 1 https://github.com/OpenPEPPOL/peppol-bis-invoice-3 testdata/peppol/repo
	mkdir -p testdata/nlcius/testsuite
	gh api repos/phax/phive-rules/contents/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/SI-UBL-2.0.3.2 --jq '.[].name' \
		| grep '\.xml$$' \
		| while read f; do curl -sSL "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/SI-UBL-2.0.3.2/$$f" -o "testdata/nlcius/testsuite/$$f"; done
	@# CIUS-PT (Portuguese AT/eSPap) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-pt/testsuite
	for ver in 2.0.0 2.1.1; do \
		gh api "repos/phax/phive-rules/contents/phive-rules-cius-pt/src/test/resources/external/test-files/$$ver" --jq '.[] | select(.name|endswith(".xml")) | .name' \
		| while read -r name; do \
			enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
			curl -sSL "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-cius-pt/src/test/resources/external/test-files/$$ver/$$enc" -o "testdata/cius-pt/testsuite/$${ver}_$$name"; \
		done; \
	done
	@# CIUS-RO (Romanian ANAF RO e-Factura) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-ro/testsuite
	for ver in 1.0.3 1.0.4 1.0.8 1.0.9; do \
		gh api "repos/phax/phive-rules/contents/phive-rules-cius-ro/src/test/resources/external/test-files/$$ver" --jq '.[] | select(.name|endswith(".xml")) | .name' \
		| while read -r name; do \
			enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
			curl -sSL "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-cius-ro/src/test/resources/external/test-files/$$ver/$$enc" -o "testdata/cius-ro/testsuite/$${ver}_$$name"; \
		done; \
	done
	@# UBL.BE (Belgian) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-be/testsuite
	gh api "repos/phax/phive-rules/contents/phive-rules-ublbe/src/test/resources/external/test-files/en16931/v1.31" --jq '.[] | select(.name|endswith(".xml")) | .name' \
	| while read -r name; do \
		enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
		curl -sSL "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-ublbe/src/test/resources/external/test-files/en16931/v1.31/$$enc" -o "testdata/cius-be/testsuite/$$name"; \
	done
	@# SRBDT (Serbian) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-rs/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-serbia/src/test/resources/external/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$(basename "$$p")"); \
		curl -sSL "https://raw.githubusercontent.com/phax/phive-rules/master/$$(dirname "$$p")/$$enc" -o "testdata/cius-rs/testsuite/$$(basename "$$p")"; \
	done
clean-cius-oracles:
	rm -rf testdata/xrechnung testdata/peppol testdata/nlcius testdata/cius-pt testdata/cius-ro testdata/cius-be testdata/cius-rs
