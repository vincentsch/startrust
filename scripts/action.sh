#!/usr/bin/env bash
# Runs inside the StarTrust GitHub Action. Installs a release binary (unless
# STARTRUST_BIN points at one), checks the repo, writes the Markdown report to
# the job summary, sets the step outputs and writes the badge.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
work=${RUNNER_TEMP:-$(mktemp -d)}/startrust
mkdir -p "$work"

bin=${STARTRUST_BIN:-}
if [ -z "$bin" ]; then
    version=${INPUT_VERSION:-latest}
    if [ "$version" != latest ]; then export STARTRUST_VERSION=$version; fi
    INSTALL_DIR="$work/bin" sh "$here/../install.sh"
    bin="$work/bin/startrust"
    if [ "${RUNNER_OS:-}" = Windows ]; then bin="$bin.exe"; fi
fi

report="$work/report.json"
args=(check "$INPUT_REPOSITORY" --json --details --out "$report" --no-prompt)
if [ -n "${INPUT_FAIL_ON:-}" ]; then args+=(--fail-on "$INPUT_FAIL_ON"); fi
# INPUT_ARGS holds plain flags; splitting on spaces is intended.
# shellcheck disable=SC2206
extra=(${INPUT_ARGS:-})

# status holds the check's exit code, so --fail-on still fails the step
# after the summary, the outputs and the badge are written.
status=0
"$bin" "${args[@]}" ${extra[@]+"${extra[@]}"} || status=$?
if [ ! -s "$report" ]; then
    echo "StarTrust could not check $INPUT_REPOSITORY (exit $status)." >&2
    exit "$status"
fi

"$bin" report "$report" --markdown >> "${GITHUB_STEP_SUMMARY:-/dev/stdout}"

risk=$(jq -r .risk "$report")
verdict=$(jq -r .verdict "$report")
{
    echo "risk=$risk"
    echo "verdict=$verdict"
    echo "report=$report"
} >> "${GITHUB_OUTPUT:-/dev/null}"
echo "$INPUT_REPOSITORY: $verdict"
if [ "$risk" = unknown ]; then
    # --fail-on never matches an unknown risk, so a warning makes it visible.
    echo "::warning title=StarTrust could not judge $INPUT_REPOSITORY::$verdict"
fi

if [ -n "${INPUT_BADGE:-}" ]; then
    "$bin" badge "$report" --out "$INPUT_BADGE" || { [ "$status" -ne 0 ] || status=$?; }
fi
exit "$status"
