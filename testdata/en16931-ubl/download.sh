#!/bin/bash
# Downloads the EN 16931 UBL example-invoice oracle listed in sources.tsv
# (CEN TC 434 eInvoicing-EN16931; OpenPEPPOL peppol-bis-invoice-3) into this directory.
# The files are a local validation oracle; they are gitignored, not vendored.
#
# A download that fails is fatal. This corpus is an FP=0 oracle, and a run that
# fetched half of it and exited 0 would leave the oracle reporting a clean
# verdict over whatever arrived — which is a weaker claim wearing the same words.
set -euo pipefail
dir="$(cd "$(dirname "$0")" && pwd)"
failed=0
while IFS=$'\t' read -r url name; do
  [ -z "$url" ] && continue
  if curl -fsSL --retry 3 --retry-delay 2 "$url" -o "$dir/$name"; then
    echo "  $name"
  else
    echo "  FAIL $name" >&2
    rm -f "$dir/$name"
    failed=$((failed + 1))
  fi
done < "$dir/sources.tsv"
if [ "$failed" -ne 0 ]; then
  echo "$failed of the EN 16931 UBL examples could not be fetched; the corpus is incomplete" >&2
  exit 1
fi
