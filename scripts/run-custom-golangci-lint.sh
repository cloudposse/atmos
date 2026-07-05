#!/usr/bin/env bash
# Run custom golangci-lint binary with lintroller plugin.
# The binary must be built first using: atmos lint custom-gcl

set -e

if [[ ! -x ./custom-gcl ]]; then
    echo "Error: custom-gcl binary not found." >&2
    echo "Please build it first by running: atmos lint custom-gcl" >&2
    exit 1
fi

# Make the lint cache checkout-aware.
#
# golangci-lint defaults to a single machine-global cache directory
# (~/.cache/golangci-lint) guarded by a file lock, so concurrent runs from
# different checkouts of this repo fail fast with "parallel golangci-lint is
# running". This bites anyone with more than one checkout on the machine:
# multiple clones, or git worktrees (e.g. Conductor sessions).
#
# Keying the cache by the checkout's root path gives each checkout its own cache
# directory (and therefore its own lock), so different checkouts lint in parallel
# with no contention. This needs no opt-in and is universal:
#   - single clone  -> one stable cache dir (identical behavior to before);
#   - many clones   -> one cache dir each (also fixes their contention);
#   - worktrees     -> one cache dir each.
#
# The cache lives OUTSIDE the checkout (under the user cache dir), so a commit
# never writes into the worktree -- avoiding the class of worktree corruption
# that motivated keeping builds out of the pre-commit hook in the first place.
#
# An explicitly-set GOLANGCI_LINT_CACHE (e.g. in CI) is always respected.
if [[ -z "${GOLANGCI_LINT_CACHE:-}" ]]; then
    checkout_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
    if command -v shasum >/dev/null 2>&1; then
        checkout_key="$(printf '%s' "$checkout_root" | shasum | cut -c1-16)"
    elif command -v sha1sum >/dev/null 2>&1; then
        checkout_key="$(printf '%s' "$checkout_root" | sha1sum | cut -c1-16)"
    else
        checkout_key="$(printf '%s' "$checkout_root" | cksum | tr -d ' ')"
    fi
    export GOLANGCI_LINT_CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/golangci-lint/checkouts/${checkout_key}"
fi

# --allow-serial-runners: within a single checkout, queue concurrent runs around
# the cache lock (wait) instead of failing fast. Cross-checkout runs already use
# separate caches and never contend, so this only smooths the same-checkout case
# (e.g. a manual `make lint` racing a pre-commit). The lock is kept, so the cache
# is never written by two runners at once.
./custom-gcl run --allow-serial-runners --new-from-rev=origin/main --config=.golangci.yml
