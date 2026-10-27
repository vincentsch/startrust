#!/usr/bin/env bash
# Scores the CLI against the rungrad spec. Uses only offline commands, so it
# needs no network: report, badge and cache read a check saved from a fixture.
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
CGO_ENABLED=0 go build -o "$work/startrust" ./cmd/startrust
cp internal/cli/testdata/check-saved.json "$work/check.json"
go run github.com/vincentsch/rungrad/cmd/rungrad@v0.3.3 score "$work/startrust" \
  --manifest auto --strict \
  --read "report $work/check.json" \
  --destructive "cache prune --cache-dir $work/cache" \
  --not-found "report $work/missing.json"
