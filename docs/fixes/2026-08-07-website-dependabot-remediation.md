# Fix: remediate 7 open Dependabot alerts in website dependencies

**Date:** 2026-08-07

## Summary

Bumped `pnpm.overrides` in `website/package.json` for two transitive `website/` dependencies to
their patched versions, closing all 7 open Dependabot alerts (2 high, 4 medium, 1 low) with no
major-version bumps.

## Context

`git push` on branch `osterman/field-test-ai-commands` triggered the `security-remediate-trigger`
PostToolUse hook after GitHub reported 7 open Dependabot alerts on the default branch. Per the
`security-remediate` skill, alerts are fixed directly on the branch already in flight rather than
in a separate PR.

Live query confirmed: 7 open Dependabot alerts, 0 open CodeQL alerts. All 7 were `npm`, transitive,
in `website/pnpm-lock.yaml`:
- `js-yaml` 3.15.0 → patched 3.15.1, and 4.3.0 → patched 4.3.1 (`GHSA-5p4m-2wfm-xmqj`, quadratic CPU
  consumption in `!!omap` resolution, high severity, alerts #268/#269).
- `mermaid` 11.16.0 → patched 11.16.1 (`GHSA-rhh3-jpg6-66xh`, `GHSA-c4c3-pg64-4m4v`,
  `GHSA-6x64-9x62-f2gx`, `GHSA-3rrr-jr9j-h3q3`, `GHSA-2v8p-3f2j-5mp7`, alerts #263-#267).

Both are within-major-version patches, so `.github/dependabot.yml`'s major-bump ignore policy
doesn't block them.

## Changes

- `website/package.json`: bumped the `js-yaml@^3`, `js-yaml@^4`, and `mermaid@^11` entries in
  `pnpm.overrides` to `^3.15.1`, `^4.3.1`, and `^11.16.1` respectively.
- `website/pnpm-lock.yaml`: regenerated via `pnpm install --no-frozen-lockfile`, confirming both
  `js-yaml@3.15.1`/`js-yaml@4.3.1` and `mermaid@11.16.1` now appear in the lockfile.
- `NOTICE`: regenerated via `scripts/generate-notice.sh` — unchanged (no license-set change from
  these bumps).

## Validation

- `grep` confirmed the patched versions landed in `pnpm-lock.yaml`.
- `cd website && pnpm run build` — succeeds; no new broken links (the one pre-existing broken-anchor
  warning is unrelated to this change).
- No Go files touched, so no `go build`/`go test`/lint run was needed for this fix.

## Follow-ups

None.
