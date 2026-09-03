# Fix: Windows Defender exclusions on CI Windows legs and restore-only toolchain cache on the acceptance shards

**Date:** 2026-09-03

## Summary

Two CI-only changes from Phase 2 of the CI-stability plan (see
`docs/fixes/2026-09-03-harden-runner-windows-dns-restore-race.md` for Phase 1). First, every Windows leg of
`test.yml` (and the nightly Go cache warmup) now excludes the workspace, the hosted tool cache, the Go
caches, and the temp directories from Windows Defender's real-time scanner before `Set up Go` runs. Second,
the `test` job's ten shards per OS restore the Atmos toolchain cache without ever saving it: the
`actions/cache` composite action gained a `restore-only` input that switches it to `actions/cache/restore`,
and `terraform-registry-cache` stays the single writer of the key per OS. No user-visible behavior changes;
the new action input is opt-in and defaults to the previous behavior.

## Context

Measured on `test.yml` runs from Aug 24 to Sept 3 (80 successful Windows shards, step-level timelines, raw
job logs, and `gh api repos/cloudposse/atmos/actions/cache/usage`).

**Windows shard time.** Median 14.1 min, p90 16.6, max 36. `Set up Go` alone averages 5.2 min (max 10.5)
restoring a 1.9 GB go-build + mod cache. The download is about 20 s at 100 MB/s; the remainder is tar/zstd
extraction with Defender's real-time scanner inspecting every extracted file. The same scanner holding
handles on freshly written files is what surfaces as `testing.TempDir` cleanup failures
(`unlinkat ... being used by another process`) in the acceptance tests. The existing
`transientErrorDetector` in `internal/ci/acceptance/command.go` only covers the Go-toolchain variant of
that error.

**Toolchain cache.** The repository's Actions cache holds 18.9 GB across 17 entries against a 10 GB LRU
quota. Every entry is scoped to a `refs/pull/N/merge` ref and is minutes old (14 `setup-go-*` entries at
1.6 to 1.9 GB, 8 `atmos-toolchain-*` entries at 350 to 470 MB). Each run writes about 5 GB, so nothing
saved from `main` survives and the static key `atmos-toolchain-<os>-<arch>-v2` never hits: every shard
logs `Cache not found for input keys`, then all 10 shards race to save the same key and log
`Unable to reserve cache with key ..., another job may be creating this cache`. `Post Cache Atmos
toolchain` costs 43 s on average and up to 3 min per shard for nothing.

**Why it is safe to land right after the harden-runner block-mode PR.** The Defender step does not touch
harden-runner's egress policy. It only stops the antivirus from re-scanning files the job itself just wrote
on an ephemeral VM that is discarded when the job ends. The step is `continue-on-error: true` so a future
locked-down image cannot fail the job.

## Changes

- `.github/workflows/test.yml`
  - New step `Exclude the workspace and Go caches from Windows Defender` (`shell: pwsh`,
    `continue-on-error: true`, same draft-PR gating as the neighbouring `Add GNU tar to PATH` step) on the
    Windows legs of `build` (after the `Guard DNS against the harden-runner post-step race` step, before
    `Set up Go`), `terraform-registry-cache`, `test`, and `mock` (each immediately after `Add GNU tar to
    PATH`). It runs `Add-MpPreference -ExclusionPath` for `D:\a`, `C:\hostedtoolcache\windows`,
    `$env:USERPROFILE\go`, `$env:LOCALAPPDATA\go-build`, `$env:LOCALAPPDATA\Temp`, and `$env:RUNNER_TEMP` -
    path-scoped exclusions only. An earlier draft also added
    `Add-MpPreference -ExclusionProcess 'go.exe'`, a name-only process exclusion that would have skipped
    real-time scanning for *any* binary named `go.exe` regardless of where it actually lives - including one
    planted by a compromised dependency during `go build`/`go generate`. Dropped it: the path exclusions
    above already cover the actual scanning cost this fix targets (file I/O in the Go caches and workspace),
    so nothing measured is given up.
  - The `test` job's `Cache Atmos toolchain` step (all three OSes) now passes `restore-only: 'true'`, with a
    comment carrying the cache data above and naming `terraform-registry-cache` as the single writer. The
    `terraform-registry-cache` job's cache step is unchanged.
- `.github/workflows/setup-go-cache-warmup.yml`: the same Defender step on the Windows leg, after the DNS
  guard and before `Set up Go`.
- `actions/cache/action.yml`: new optional input `restore-only` (string, default `'false'`). When `'true'`
  the action runs `actions/cache/restore` (same repository and tag as `actions/cache`, so the same pinned
  SHA `27d5ce7f107fe9357f9df03efb73ab90386fccae # v5.0.5`) with identical `key`, `path`, and
  `restore-keys`; otherwise `actions/cache` runs as before. Exactly one of the two steps runs, so the
  `cache-hit` output is `steps.cache.outputs.cache-hit || steps.cache-restore.outputs.cache-hit`; the `key`
  output is unchanged.
- `actions/cache/README.md`: documents the input and the "many parallel consumers, one writer" pattern.

Not changed on purpose (later PRs): `atmos.yaml` `ci.cache.key`, `setup-go` caching, and the Go cache
warmup's writer role.

## Validation

- `python3 -c 'import yaml; yaml.safe_load(open(f))'` on `.github/workflows/test.yml`,
  `.github/workflows/setup-go-cache-warmup.yml`, and `actions/cache/action.yml`: all parse; the action
  parses with `inputs: [restore-only]` and steps `meta`, `validate`, `cache`, `cache-restore`.
- `actionlint .github/workflows/test.yml .github/workflows/setup-go-cache-warmup.yml`: clean (exit 0).
  actionlint does not lint composite actions, so `actions/cache/action.yml` is covered by the YAML parse
  and the Go regression test below only.
- `go test github.com/cloudposse/atmos/cmd -run TestAtmosCacheActionValidatesMetadataBeforeActionsCache`:
  passes (it asserts the metadata validation step still precedes the cache steps).
- `git diff` reviewed for accidental changes: only the four files above.
- Not validated here: the Windows timing effect itself. `Set up Go` extraction time and
  `Post Cache Atmos toolchain` duration on the shards are measured from the PR's own CI runs with the
  "Windows shard step durations" query in the Phase 1 fix log; the expected outcomes are `Set up Go` from
  5 to 10 min down to about 1 min, `Post Cache Atmos toolchain` absent from the 30 shard jobs, and no
  `Unable to reserve cache` lines in shard logs.

## Follow-ups

- Two further Phase 2 changes are separate PRs already in flight and have no GitHub issue numbers yet (the
  repository rule against opening unprompted issues applies; the numbers are added here when the PRs open):
  the single-writer hashed toolchain key (`atmos-toolchain-{{.OS}}-{{.Arch}}-{{ hashFiles ".tool-versions" }}`
  in `atmos.yaml` `ci.cache.key`, once `.tool-versions` lands), and shipping the toolchains inside the
  existing `build-artifacts-<target>` artifact so the shards stop installing them at all.
- Phase 2c (measure the cache-free Windows shard) and the `setup-go` single-writer change are also still
  open; same tracking note.
