# Fix: bump `shared-go-auto-release.yml` pin so cosign can mint a GitHub OIDC token

**Date:** 2026-09-03

## Summary

Every push to `main` and every nightly build has failed in the `release / goreleaser` job since
2026-08-31. Cosign's keyless signing could not obtain a GitHub OIDC token, fell back to the Sigstore
device flow, and timed out after 300 s with `error obtaining token: expired_token`. The reusable
workflow `cloudposse/.github/.github/workflows/shared-go-auto-release.yml` at the SHA Atmos pinned
(`3911c663309ecdda30d8b8fcbec7bde19d1d6ddb`) declared `permissions: {}`, which stripped the caller's
`id-token: write`. This fix bumps the pin to `da6de3218cc965251e38d84f6addd37e7ddde2e6`, which
includes the upstream fix that declares `id-token: write` in the called workflow.

## Context

- A called reusable workflow can only reduce the permissions its caller grants; it cannot widen
  them. With `permissions: {}` at the top of `shared-go-auto-release.yml`, the caller-side
  `id-token: write` in `test.yml`, `nightlybuilds.yml`, and `feature-release.yml` was discarded, so
  the runner never exported `ACTIONS_ID_TOKEN_REQUEST_URL` and cosign had no OIDC endpoint to use.
- The `##[group]GITHUB_TOKEN Permissions` block of a failing goreleaser job (for example job
  `100482554969` in nightly run `33701764071`) shows only `Metadata: read`. Later in the same log,
  cosign prints `Non-interactive mode detected, using device flow.` followed by
  `Error: signing dist/atmos_1.228.0-rc.5_SHA256SUMS: getting keypair and token: retrieving ID token:
  authenticating caller: error obtaining token: expired_token`.
- The first failing `main` run is `33356404722`, created `2026-08-31T04:14:51Z`, two seconds after
  Atmos PR #2958 merged (`2026-08-31T04:14:49Z`). That PR added the cosign `signs:` block to the
  GoReleaser config, which is what first exercised the OIDC endpoint from this workflow.
- Nightly builds have failed since 2026-09-01: runs `33457547849`, `33577453449`, and `33701764071`.
- cloudposse/.github fixed the reusable workflow on 2026-09-02 in commit
  `af506fd87a68098a975b7c20bd5eb38e95ea91d4` ("Add permissions for id-token and contents in
  workflow"). Current `main` there is `da6de3218cc965251e38d84f6addd37e7ddde2e6`; the only other
  file that differs from the old pin is `SECURITY.md`.

## Changes

- `.github/workflows/test.yml`: bumped the `release` job's
  `uses: cloudposse/.github/.github/workflows/shared-go-auto-release.yml@...` pin from
  `3911c663309ecdda30d8b8fcbec7bde19d1d6ddb` to `da6de3218cc965251e38d84f6addd37e7ddde2e6`, and
  amended the comment above it: the OIDC endpoint is gated by this job's `id-token` permission and by
  the called workflow's own `permissions` block, which is why the pin must include the upstream fix.
  Also added `contents: read` to that job's `permissions:` block (see "Job-level permissions replace,
  not merge" below).
- `.github/workflows/nightlybuilds.yml`: same pin bump. No `permissions:` change: its `release` job
  has no job-level block, so it inherits the workflow-level default (`contents: write`,
  `id-token: write`, ...), which already satisfies what the called workflow requires.
- `.github/workflows/feature-release.yml`: same pin bump, plus a comment update because the old
  comment stated that the reusable workflow "declares `permissions: {}` itself", which is no longer
  true at the new pin. Also added `contents: read` to that job's `permissions:` block, for the same
  reason as `test.yml` (see below).
- `build.yml` is untouched: it pins `shared-release-branches.yml`, which did not change.

### Job-level permissions replace, not merge

A job-level `permissions:` block replaces the workflow-level default entirely for that job rather
than merging with it, so every scope the called workflow needs must be listed explicitly. The
upstream fix at the new pin declares both `id-token: write` **and** `contents: read` at the top of
`shared-go-auto-release.yml`. Any caller whose `release` job overrides permissions with only
`id-token: write` therefore grants `contents: none`, and GitHub rejects the whole workflow file at
parse time — before any job starts, and regardless of the `release` job's `if:` condition — with
`requesting 'contents: read', but is only allowed 'contents: none'`.

Both `test.yml` and `feature-release.yml` have such a job-level block, so both needed
`contents: read` added:

```yaml
    permissions:
      id-token: write
      contents: read
```

`feature-release.yml` had only `id-token: write` when the pin was first bumped, which produced a
`startup_failure` on every `pull_request` run of the branch (the `release` job is skipped without the
`release/feature` label, but the parse-time rejection fires anyway). `nightlybuilds.yml` is
unaffected because it has no job-level override.

## Validation

- Verified the upstream state with the GitHub API: `da6de32...` is the current head of
  `cloudposse/.github` `main`; the compare between the old and new pins lists only
  `.github/workflows/shared-go-auto-release.yml` and `SECURITY.md`; the workflow at the new pin
  declares `id-token: write` and `contents: read`, while the old pin declares `permissions: {}`.
- Confirmed no open Atmos PR already bumps this pin.
- `actionlint` passes on `test.yml`, `nightlybuilds.yml`, and `feature-release.yml`; all three parse
  as YAML. Note that `actionlint` does **not** validate cross-workflow reusable-workflow permission
  requirements, so it did not catch the missing `contents: read` — that surfaced only as a runtime
  `startup_failure`. The parse-time rejection was confirmed empirically: every "Feature release" run
  on the branch reported `startup_failure` (~1 s, zero jobs) after the pin bump, versus a clean
  `skipped` on `main`, until `contents: read` was added.
- **Not yet verified end to end.** The fix cannot be confirmed from a pull request: the `release`
  job in `test.yml` only runs on `push` to `main`, and the nightly workflow runs on its schedule.
  The fix remains unverified until the first post-merge `test.yml` push run and the next Nightly
  Builds run both succeed. See Follow-ups for the pending checks.

## Follow-ups

- Pending post-merge verification (no separate issue; tracked here until both checks succeed):
  - The first `test.yml` run on `push` to `main` after this lands must reach `success` in the
    `release / goreleaser` job. Check with `gh run list -R cloudposse/atmos -w test.yml --event push -L 3`.
  - The next Nightly Builds run must succeed. Check with
    `gh run list -R cloudposse/atmos -w nightlybuilds.yml -L 1`.
