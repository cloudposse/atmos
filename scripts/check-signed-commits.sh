#!/usr/bin/env bash
# Reject pushes that contain unsigned or invalidly signed commits.
#
# Git commit hooks cannot reliably inspect the final commit object, so this
# belongs at pre-push time where the exact commits leaving the machine are known.

set -euo pipefail

zero_ref="0000000000000000000000000000000000000000"

check_range() {
    local range="$1"
    local failed=0

    while read -r commit status subject; do
        case "$status" in
            G|U)
                # G = good valid signature. U = good signature with unknown local trust.
                # Both are signed; server-side branch protection remains authoritative.
                ;;
            *)
                printf 'Unsigned or invalidly signed commit: %s [%s] %s\n' "$commit" "$status" "$subject" >&2
                failed=1
                ;;
        esac
    done < <(git log --format='%H %G? %s' "$range")

    return "$failed"
}

main() {
    local failed=0
    local saw_stdin=0

    if [ ! -t 0 ]; then
        while read -r local_ref local_sha remote_ref remote_sha; do
            saw_stdin=1

            # Deletions do not introduce commits.
            if [ "$local_sha" = "$zero_ref" ]; then
                continue
            fi

            if [ "$remote_sha" = "$zero_ref" ]; then
                check_range "$local_sha" || failed=1
            else
                check_range "${remote_sha}..${local_sha}" || failed=1
            fi
        done
    fi

    # pre-commit's pre-push runner may not pass Git's stdin through to local hooks.
    # In that case, check commits ahead of the upstream branch.
    if [ "$saw_stdin" -eq 0 ]; then
        local upstream
        upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null || true)"
        if [ -n "$upstream" ]; then
            check_range "${upstream}..HEAD" || failed=1
        fi
    fi

    if [ "$failed" -ne 0 ]; then
        cat >&2 <<'EOF'

Push rejected: all commits must be signed.

Do not use --no-gpg-sign. To fix the latest commit:
  git commit --amend --no-edit

Then verify:
  git log -1 --show-signature
EOF
    fi

    return "$failed"
}

main "$@"
