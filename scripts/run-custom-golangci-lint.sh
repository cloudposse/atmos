#!/usr/bin/env bash
# Run custom golangci-lint binary with lintroller plugin.
# The binary must be built first using: make custom-gcl

set -e

if [[ ! -x ./custom-gcl ]]; then
    echo "Error: custom-gcl binary not found." >&2
    echo "Please build it first by running: make custom-gcl" >&2
    exit 1
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
