# Fix: Scorecard flags same-repository `$/` actions as unpinned

**Date:** 2026-10-23

## Summary

Upgrade `ossf/scorecard-action` from v2.4.4 to v2.5.0. That release bundles
Scorecard v5.6.0, which recognizes GitHub's self-repository action syntax.

## Context

The workflows reference local actions as `uses: $/.github/actions/...`. GitHub
resolves these references to the exact workflow commit, so they cannot drift.
Scorecard v5.5.0 and earlier did not recognize this syntax. Pinned-Dependencies
reported 67 code-scanning alerts against the `harden-runner` references.

Upstream fixed this in [ossf/scorecard#5191](https://github.com/ossf/scorecard/pull/5191),
which closed [ossf/scorecard#5232](https://github.com/ossf/scorecard/issues/5232).
The fix shipped in Scorecard v5.6.0 and scorecard-action v2.5.0. The v5.6.0
release notes also tell Scorecard Action users to upgrade to v2.5.0 after the
service infrastructure migration.

## Changes

- Pin `ossf/scorecard-action` to v2.5.0 by full commit SHA, `fa7117db83c52d787ae5a89f350999701505d78e`.
  This is the annotated tag `v2.5.0` peeled to its commit.

## Validation

- `go.mod` at `v2.5.0` requires `github.com/ossf/scorecard/v5 v5.6.0`.
- Both v2.4.4 and v2.5.0 pull their image from `ghcr.io`, so the harden-runner
  allowlist needs no change.
- The 67 existing alerts were dismissed as false positives through the
  code-scanning API.
- The Scorecard workflow runs only on `main`, so this PR cannot exercise it.
  The first push to `main` after merge validates it.

## Follow-ups

None.
