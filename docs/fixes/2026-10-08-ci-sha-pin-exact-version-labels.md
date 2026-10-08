# Fix: SHA-pinned actions are labeled with exact versions, not floating major tags

**Date:** 2026-10-08

## Summary

The `verify` job (SHA pin drift verification) started failing on every pull request:
`actions/download-artifact@3e5f45b… # v8` was reported as a SHA mismatch. Upstream released
v8.0.2 and moved the floating `v8` tag to it, while our pin is the v8.0.1 commit. The pinned SHA
was never wrong; the version label next to it was ambiguous. Every pin labeled with a floating
major tag now carries the exact release its SHA corresponds to, so an upstream release can no
longer make the drift check fail.

## Context

The verifier resolves the version label after each pinned SHA (`uses: owner/repo@<sha> # <tag>`)
and fails when the tag points at a different commit. A floating major tag such as `v8` moves with
every upstream patch release, so a pin labeled `# v8` passes until the next release and then fails
with no change on our side. Nine pins in `.github/` used floating labels:
`actions/download-artifact` (v8), `actions/checkout` (v6), `actions/setup-go` (v5),
`actions/upload-artifact` (v4), `bobheadxi/deployments` (v1), `charmbracelet/vhs-action` (v1),
`cloudposse/github-action-major-release-tagger` (v2), `lycheeverse/lychee-action` (v2), and
`stefanzweifel/git-auto-commit-action` (v4).

## Changes

- Relabeled each of those pins with the exact tag its SHA resolves to (found through the GitHub
  tags API), for example `# v8` to `# v8.0.1` and `# v1` to `# v1.5.0`. No SHA changed, so the
  code each workflow runs is unchanged. Files: `vhs.yaml`, `landing-demos.yaml`, `link-check.yml`,
  `release-major-tag.yml`, `verify-sha-pinning.yml`, `website-preview-deploy.yml`, and
  `.github/actions/download-artifact-retry/action.yml`.
- `.github/actions/verify-sha-pinning/test.mjs` keeps its intentional `# v1` fixture unchanged.

## Validation

- `GITHUB_TOKEN=$(gh auth token) node .github/actions/verify-sha-pinning/test.mjs` from the repo
  root: 42 passed, 0 failed, including the scan of the real workflow files.
- `grep -rnE "@[0-9a-f]{40} # v[0-9]+$" .github` returns nothing.
- `actionlint` on the changed workflows reports only shellcheck notes that already exist on
  `main` in unchanged steps.

## Follow-ups

None.
