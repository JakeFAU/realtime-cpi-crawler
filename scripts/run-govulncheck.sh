#!/usr/bin/env bash

set -euo pipefail

OUT="$(mktemp)"
ERR="$(mktemp)"
cleanup() {
  rm -f "$OUT" "$ERR"
}
trap cleanup EXIT

if govulncheck ./... >"$OUT" 2>"$ERR"; then
  cat "$OUT"
  exit 0
fi

if grep -q "github.com/go-json-experiment/json/jsontext.Value" "$ERR"; then
  cat "$OUT"
  echo "govulncheck: ignoring known upstream panic caused by github.com/go-json-experiment/json/jsontext.Value" >&2
  echo "govulncheck: please rerun once the upstream tool is fixed." >&2
  exit 0
fi

cat "$ERR" >&2
exit 1
