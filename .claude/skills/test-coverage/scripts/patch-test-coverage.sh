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

dirs="$(
	echo "$touched_files" | while IFS= read -r file; do
		dirname "$file"
	done | sort -u
)"

# Some touched directories belong to a *separate* Go module nested under the
# repo root (e.g. tools/lintroller, tools/gomodcheck each carry their own
# go.mod so they can be built by `go install`/`golangci-lint custom` without
# pulling the whole atmos dependency graph in). `go test ./tools/lintroller`
# from the root module fails immediately with "main module ... does not
# contain package ..." -- that's a false positive in this check, not a real
# test failure, so route those packages through their own nested `go test`
# invocation instead of the root module's package list.
root_packages=""
nested_module_roots=""
while IFS= read -r dir; do
	walk="$dir"
	nested_root=""
	while [[ "$walk" != "." && "$walk" != "/" ]]; do
		if [[ "$walk" != "." && -f "$walk/go.mod" ]]; then
			nested_root="$walk"
			break
		fi
		walk="$(dirname "$walk")"
	done
	if [[ -n "$nested_root" ]]; then
		nested_module_roots="${nested_module_roots}${nested_root}"$'\n'
	elif [[ "$dir" == "." ]]; then
		root_packages="${root_packages}./"$'\n'
	else
		root_packages="${root_packages}./${dir}"$'\n'
	fi
done <<<"$dirs"
packages="$(printf '%s' "$root_packages" | sort -u)"
nested_module_roots="$(printf '%s' "$nested_module_roots" | sort -u | sed '/^$/d')"

tmp_profile="$(mktemp "${TMPDIR:-/tmp}/atmos-patch-coverage.XXXXXX")"
tmp_output="$(mktemp "${TMPDIR:-/tmp}/atmos-patch-test-output.XXXXXX")"
cleanup() {
	rm -f "$tmp_profile" "$tmp_output"
}
trap cleanup EXIT

set +e
test_exit=0
if [[ -n "$packages" ]]; then
	# Go's default per-binary timeout is 10m, tuned for a single package on a
	# dedicated runner. This branch's scoped package set includes slow
	# acceptance suites (internal/exec, tests) that finish well under 10m on
	# CI's dedicated runners, but on a shared dev machine running several
	# worktree sessions at once (multiple `go test`/dev-server processes
	# competing for the same CPUs -- confirmed via `uptime` load averages well
	# above core count) the same suites can take several times longer, panicking
	# the whole run with no actual test failure. -timeout 55m gives headroom up
	# to just under CI's own Acceptance Tests job-level ceiling
	# (.github/workflows/test.yml's `timeout-minutes: 60`) without masking a
	# genuine deadlock outright.
	#
	# -tags mage lets magefiles/*.go (gated behind `//go:build mage`, see
	# CLAUDE.md's Compilation section) build under this check; every other
	# package in the repo is unaffected since they declare no such
	# constraint. Without it, a touched magefiles/*.go reports a
	# "build constraints exclude all Go files" [setup failed] false positive.
	# shellcheck disable=SC2086
	go test -tags mage -v -timeout 55m -coverprofile="$tmp_profile" -covermode=set $packages >"$tmp_output" 2>&1
	test_exit=$?
fi

nested_output=""
if [[ -n "$nested_module_roots" ]]; then
	while IFS= read -r nested_root; do
		nested_output+=$'\n'"# nested module: ${nested_root}"$'\n'
		module_result="$(cd "$nested_root" && go test -v -timeout 10m ./... 2>&1)"
		module_exit=$?
		nested_output+="$module_result"$'\n'
		if [[ $module_exit -ne 0 ]]; then
			test_exit=1
		fi
	done <<<"$nested_module_roots"
fi
set -e

echo "== Touched packages (vs ${BASE_REF}) =="
echo "$packages"
if [[ -n "$nested_module_roots" ]]; then
	echo "Nested modules (own go.mod, tested separately, not part of the coverage profile below):"
	echo "$nested_module_roots"
fi
echo

if [[ $test_exit -ne 0 ]]; then
	echo "STATUS: TESTS_FAILING"
	echo
	echo "== Raw test output =="
	cat "$tmp_output"
	echo "$nested_output"
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
