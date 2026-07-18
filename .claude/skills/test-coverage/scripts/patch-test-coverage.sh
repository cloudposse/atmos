#!/usr/bin/env bash
# Run tests scoped to only the Go packages touched by the current diff vs a
# base ref (default origin/main), and print a raw bundle for an agent to
# reason over: pass/fail status, test output, the coverage profile, and the
# diff itself.
#
# Deliberately does NOT compute "which added lines are uncovered" or classify
# failing tests as in-scope vs pre-existing — that cross-referencing is left
# to the consuming agent (test-coverage-fix), which reads this raw output the
# same way coderabbit-review reads raw CodeRabbit comment text. Keeping this
# script dumb keeps it small, and the full test suite (349 packages) takes
# 45-75 minutes per CI timeouts, so scoping to touched packages is required
# for this to run every hour at all.
#
# Usage:
#   patch-test-coverage.sh [base-ref]
#
# base-ref defaults to origin/main.

set -euo pipefail

BASE_REF="${1:-origin/main}"

# Same ignore globs as codecov.yml's coverage.ignore, so our patch-coverage
# view stays consistent with what Codecov itself considers coverable.
IGNORE_PATTERN='(^|/)mock_[^/]*\.go$|(^|/)mock/[^/]*\.go$|_test_helpers\.go$|(^|/)testhelpers/'

touched_files="$(git diff --name-only --diff-filter=ACMR "${BASE_REF}...HEAD" -- '*.go' | grep -Ev "$IGNORE_PATTERN" || true)"

if [[ -z "$touched_files" ]]; then
    echo "STATUS: NO_GO_CHANGES"
    echo "No touched .go files vs ${BASE_REF} (after excluding mocks/testhelpers). Nothing to test."
    exit 0
fi

packages="$(
    echo "$touched_files" | while IFS= read -r file; do
        dir="$(dirname "$file")"
        if [[ "$dir" == "." ]]; then
            printf './\n'
        else
            printf './%s\n' "$dir"
        fi
    done | sort -u
)"

tmp_profile="$(mktemp "${TMPDIR:-/tmp}/atmos-patch-coverage.XXXXXX")"
tmp_output="$(mktemp "${TMPDIR:-/tmp}/atmos-patch-test-output.XXXXXX")"
cleanup() {
    rm -f "$tmp_profile" "$tmp_output"
}
trap cleanup EXIT

set +e
# -timeout 40m matches .atmos.d/test.yaml's full-suite convention. go test's own
# default is 10m for the whole binary, not per test — when the diff touches
# ./tests, this scopes in the entire acceptance-test package (hundreds of tests,
# one binary; Go can't compile/run a subset of a package), and that package's
# genuine runtime regularly exceeds 10m under concurrent-session load or
# unauthenticated GitHub API calls. Confirmed for real: three consecutive hourly
# cycles each panicked with "test timed out after 10m0s" on a different,
# patch-unrelated subtest (whichever happened to still be running when the
# clock ran out), while every one of them passed cleanly in isolation given more
# time — a false positive from the timeout, not a real failure.
# shellcheck disable=SC2086
go test -v -timeout 40m -coverprofile="$tmp_profile" -covermode=set $packages >"$tmp_output" 2>&1
test_exit=$?
set -e

echo "== Touched packages (vs ${BASE_REF}) =="
echo "$packages"
echo

if [[ $test_exit -ne 0 ]]; then
    echo "STATUS: TESTS_FAILING"
    echo
    echo "== Raw test output =="
    cat "$tmp_output"
    echo
    echo "== Touched .go files (for in-scope vs pre-existing classification) =="
    echo "$touched_files"
    echo
    echo "DISCLAIMER: scoped to touched packages' own tests only, not -coverpkg=./... breadth. A fast approximation for this patch, not a source of truth — CI's full-suite Codecov upload remains authoritative."
    exit 1
fi

echo "STATUS: OK"
echo
echo "== Coverage profile (touched packages only) =="
cat "$tmp_profile"
echo
echo "== Diff vs ${BASE_REF} (touched files) =="
# shellcheck disable=SC2086
git diff --unified=0 "${BASE_REF}...HEAD" -- $touched_files
echo
echo "DISCLAIMER: scoped to touched packages' own tests only, not -coverpkg=./... breadth. A fast approximation for this patch, not a source of truth — CI's full-suite Codecov upload remains authoritative."
