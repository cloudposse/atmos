#!/usr/bin/env bash
# Verify that every symbolic link tracked by Git resolves in this checkout.

set -euo pipefail

errors=0
links=0

while IFS= read -r -d '' entry; do
    metadata=${entry%%$'\t'*}
    path=${entry#*$'\t'}
    mode=${metadata%% *}

    if [[ "$mode" != "120000" ]]; then
        continue
    fi

    links=$((links + 1))
    if [[ -e "$path" ]]; then
        continue
    fi

    target=$(readlink "$path" 2>/dev/null || printf '<unreadable>')
    message="Broken tracked symlink: $path -> $target"
    printf '::error file=%s::%s\n' "$path" "$message"
    printf '%s\n' "$message" >&2
    errors=$((errors + 1))
done < <(git ls-files -s -z)

if ((errors > 0)); then
    printf 'Found %d broken tracked symlink(s).\n' "$errors" >&2
    exit 1
fi

printf 'All %d tracked symlink(s) resolve.\n' "$links"
