#!/usr/bin/env bash
# Run the Brick verification ladder.
#
#   verify.sh          quick gate: build, unit tests, vet, format, self-host parity
#   verify.sh deep     everything, including e2e, bootstrap and the interpreter diff
#   verify.sh bench    rebuild and run the four-language benchmark
#
# Stops at the first failure and prints which rule in SKILL.md applies.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT" || exit 1

MODE="${1:-quick}"
FAILED=0

step() { printf '\n== %s\n' "$1"; }
fail() {
    printf '\nFAILED: %s\n' "$1" >&2
    printf 'See .agents/skills/brick-lang/SKILL.md, "Debugging playbook".\n' >&2
    exit 1
}

step "build"
make build || fail "go build (stale ./bin/brick causes phantom 'undefined function')"

step "go test"
go test ./... || fail "unit tests (checker/codegen/interp expectations)"

step "go vet"
go vet ./... || fail "go vet"

step "format check"
make fmt-check || fail "examples not canonically formatted (run ./bin/brick fmt <file>)"

step "selfhost parity"
make selfhost || fail "Go and Brick implementations diverged (the target names the component)"

if [ "$MODE" = "deep" ] || [ "$MODE" = "bench" ]; then
    step "differential testing"
    make diff || fail "interpreter and compiled binary disagree"
fi

if [ "$MODE" = "deep" ]; then
    step "selfhost e2e"
    make selfhost-e2e || fail "self-hosted compiler emits C that does not build or run"

    step "bootstrap fixpoint"
    make bootstrap || fail "stage2 and stage3 C differ: a runtime helper is ungated or differs by a byte"
fi

if [ "$MODE" = "bench" ]; then
    step "benchmark"
    python3 bench/run.py || fail "checksum mismatch: a runtime change altered behaviour"
fi

printf '\nok: %s gate passed\n' "$MODE"
