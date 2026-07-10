#!/usr/bin/env bash
# Run custom golangci-lint binary with lintroller plugin.
# The binary must be built first using: atmos lint custom-gcl

set -e

if [[ ! -x ./custom-gcl ]]; then
    echo "Error: custom-gcl binary not found." >&2
    echo "Please build it first by running: atmos lint custom-gcl" >&2
    exit 1
fi

# --- Per-worktree isolation so parallel worktree lints (e.g. rebase storms) don't
# --- serialize on golangci-lint's machine-global single-instance lock.
#
# golangci-lint's runner lock is a FIXED path: `os.TempDir()/golangci-lint.lock`
# (e.g. /var/folders/.../T/golangci-lint.lock on macOS, /tmp/golangci-lint.lock on
# Linux). It is NOT inside GOLANGCI_LINT_CACHE, so isolating only the cache does not
# isolate the lock: every worktree still blocks on one machine-global lock, and a
# stuck run (or one orphaned by a tool timeout) freezes every other worktree.
#
# os.TempDir() honors $TMPDIR, so pointing TMPDIR at a repo-local dir moves the lock
# into this worktree -> lints in different worktrees run in parallel. GOLANGCI_LINT_CACHE
# is isolated alongside it so those parallel runs never write the same cache (which
# is exactly what allow-serial-runners protected against within a shared cache).
# Within one worktree, concurrent lints (pre-commit + pre-push + retries) still
# serialize on the worktree-local lock, so the cache stays consistent and an orphan
# can only ever block its own worktree.
#
# GOCACHE is deliberately left shared/global so the expensive compile+typecheck
# export data stays warm across all worktrees.
#
# Set ATMOS_LINT_SHARED_CACHE=1 to opt back into the old machine-global shared cache
# and serialized lock (e.g. if per-worktree disk usage is a concern).
if [[ "${ATMOS_LINT_SHARED_CACHE:-}" != "1" ]]; then
    export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-$PWD/.golangci-cache}"
    export TMPDIR="$PWD/.golangci-tmp"
    mkdir -p "$TMPDIR"
fi

args=(
    run
    --config=.golangci.yml
)

if [[ -n "${GOLANGCI_CONCURRENCY:-}" ]]; then
    args+=(--concurrency="${GOLANGCI_CONCURRENCY}")
fi

staged_patch=""
cleanup() {
    if [[ -n "${staged_patch}" ]]; then
        rm -f "${staged_patch}"
    fi
}
trap cleanup EXIT

if git diff --cached --quiet -- '*.go'; then
    if [[ "${GOLANGCI_ALLOW_PARALLEL:-0}" != "0" ]]; then
        args+=(--allow-parallel-runners)
    fi
    ./custom-gcl "${args[@]}" --new-from-rev="${GOLANGCI_NEW_FROM_REV:-origin/main}"
else
    if [[ "${GOLANGCI_ALLOW_PARALLEL:-0}" != "0" ]]; then
        args+=(--allow-parallel-runners)
    fi
    staged_patch="$(mktemp "${TMPDIR:-/tmp}/atmos-golangci-staged.XXXXXX")"
    git diff --cached --binary -- '*.go' > "${staged_patch}"
    package_list="$(
        git diff --cached --name-only --diff-filter=ACMR -- '*.go' |
            while IFS= read -r file; do
                dir="$(dirname "${file}")"
                if [[ "${dir}" == "." ]]; then
                    printf '.\n'
                else
                    printf './%s\n' "${dir}"
                fi
            done |
            sort -u
    )"
    packages=()
    while IFS= read -r package; do
        if [[ -n "${package}" ]]; then
            packages+=("${package}")
        fi
    done <<< "${package_list}"
    ./custom-gcl "${args[@]}" --new-from-patch="${staged_patch}" "${packages[@]}"
fi
