# Fix: Windows Go cache restores took 4-7 minutes; put the caches on the fast disk

**Date:** 2026-09-04

## Summary

On `windows-latest` runners, restoring the `actions/setup-go` cache (our ~1.7 GB
`GOMODCACHE` + `GOCACHE` archive) took 4-7 minutes, and saving it 7-10 minutes, on every
Windows job of `test.yml`. Downloading the archive takes ~16 s; the rest is tar writing ~200k
small files onto the runner's C: drive.

Root cause: Go's default cache locations (`%LOCALAPPDATA%\go-build`, `%GOPATH%\pkg\mod`) are on
C:, a ~540 IOPS disk. The runner's work disk D: (where `RUNNER_TEMP` and the workspace live) is a
separate disk with ~8.8x the small-file write rate (4,545 vs 515 files/s for 20,000 x 1 KiB
writes, measured on the image). `actions/setup-go` restores to wherever `go env` points, so the
archive was landing on the slow disk.

## Fix

**`.github/actions/setup-go-cache`** (new local composite action): before `actions/setup-go`
runs, on Windows only, set `GOCACHE` and `GOMODCACHE` (via `GITHUB_ENV`, so the whole job and
setup-go's post-step save see them) to directories under `RUNNER_TEMP` - but only when
`RUNNER_TEMP`'s drive differs from `LOCALAPPDATA`'s. No-op on Linux/macOS and on any Windows image
without the split. Then the pinned `actions/setup-go@v5.6.0` with pass-through inputs
(`go-version-file`, `cache`, `cache-dependency-path`) and outputs (`cache-hit`, `go-version`).

Adopted at every `setup-go` call site that runs on Windows or was on v5.6.0: `build`,
`terraform-registry-cache`, `test`, `magefiles`, `floci-go`, `kubernetes-e2e`, `container-step`
in `test.yml`, plus `setup-go-cache-warmup.yml`. `terraform-registry-cache` moves from setup-go v6
to the action's v5.6.0. The Linux-only `coverage` job and `setup-atmos-build` stay on v6. The
Linux-only `race` job also stays on a direct v5.6.0 pin rather than the shared action -- it has no
Windows disk-relocation work to do, so there's nothing the action would add.

The cache *version* hashes the restore paths, so the first run after this lands is a cold start
(miss + save) for the Windows jobs; from then on they restore from D:.

## Measurements

All from a throwaway lab workflow that lived on PR #3049's branch while the numbers were
gathered (`cache-perf.yml` + a `cache-perf-lab` composite, last at commit `8fe6ed6384`; removed
before merge, resurrectable from git if ever needed). It seeded a real go-build+mod archive per
cache layout under the branch's own cache scope, then restored it under each tar/disk
combination and timed the `Set up Go` step. Same ~1.7 GB archive, `windows-latest`:

| Restore              | C: (Go defaults) | D: (work disk) |
|----------------------|------------------|----------------|
| GNU tar (Git's MSYS) | 264 s / 276 s    | **94 s**       |
| bsdtar (System32)    | 353 s            | 117 s          |

And from the lab's seed jobs (both GNU tar): cache **save** 590 s on C: vs **175 s** on D:;
`go mod download && go build ./...` 599 s vs 455 s.

`bsdtar` loses on both disks even though the research literature (actions/cache#752,
actions/toolkit#2379) reports it ~4x faster at extraction: `@actions/cache`'s BSD-tar path is two
passes - `zstd -d -o cache.tar` writes the full uncompressed tar to disk, then `tar -xf cache.tar`
reads it back - and that extra multi-GB write costs more than the MSYS per-file overhead it avoids.
So GNU tar stays - now codified once as **`.github/actions/windows-gnu-tar`** (idempotent, no-op off
Windows), which `setup-go-cache` calls as its first step; the published `actions/cache` action (behind
every "Cache Atmos toolchain" step) carries the same step inline, since a published action cannot
reference a local one. Every cache-using action now provisions its own tar, and the four hand-copied
"Add GNU tar to PATH" steps and their 2024 comment (#877, never measured) are gone.

## Temp directories too

A test log path (`C:\Users\RUNNER~1\AppData\Local\Temp\TestParse...`) showed the acceptance tests'
`t.TempDir()` scratch on the same slow disk, and Go's build scratch (`GOTMPDIR`) with it. The action
now also points `GOTMPDIR`, `TEMP` and `TMP` at the work disk. Measured on the Windows acceptance
shards' "Acceptance tests" step (same commit, temp on C: vs D:):

| shard | C: | D: |
|-------|-----|-----|
| 1 | 350 s | 231 s |
| 2 | 239 s | 180 s |
| 4 | 409 s | 278 s |
| 5 | 390 s | 249 s |
| 6 | 403 s | 280 s |
| 7 | 247 s | 230 s |
| 8 | 252 s | 228 s |
| 9 | 251 s | 235 s |
| 10 | 285 s | 260 s |
| **sum** | **2,826 s** | **2,171 s** |

23% less test time, most of it on the shards that copy the most fixtures.

The temp dir lives *beside* `RUNNER_TEMP` (`D:\a\tmp`), not under it: `RUNNER_TEMP` is `D:\a\_temp`,
and with `TEMP` under it `TestCleanToolsAndCaches` failed because atmos printed
`D:\a_temp\...` for `D:\a\_temp\...`. That is a real bug in the output layer - messages such as
`ui.Successf("Deleted **%d** files from %s", ...)` contain Markdown, so the whole line goes through
the Markdown renderer, where `\_` in the interpolated path is an escape sequence. Any Windows path
with a backslash before an underscore prints wrong in every `ui.*` message that mixes Markdown with
interpolated text; it needs fixing in `pkg/ui` (escape interpolated arguments before rendering),
separately from this change.

## Things learned the hard way

- **Main's Go cache never survives.** The repo sits at the 10 GB cache quota (9.9 GB across 5
  entries, ~1.8 GB per Windows setup-go archive), so every save evicts the least recently used
  entries and the only `setup-go-Windows` entries that exist are PR-scoped. Main's Windows jobs
  therefore pay a cold miss plus the full save on every run - which is why the *save* speedup
  matters as much as the restore.
- **A step-level `PROGRAMFILES` env override does nothing on Windows runners** (the process
  keeps its own `ProgramFiles`), so the first "bsdtar" measurements silently used GNU tar; only
  the tar command line in the log tells the truth. The lab selected bsdtar by renaming Git's
  `tar.exe` aside for the job, and printed which tar `@actions/cache` would pick.
- **Windows Defender was not a factor**: real-time monitoring is off at the image level (see
  `2026-09-04-windows-defender-exclusions-are-a-noop.md`).
- **The jobs API does not list nested composite steps**, so the lab timed setup-go with its own
  clock.
- An unquoted `C:`/`D:` inside a YAML step name is a mapping-key parse error; quote such names.

## Validation

- `actionlint` on `test.yml` and `setup-go-cache-warmup.yml`; pre-commit hooks clean.
- The lab's final run (run 4 on the branch) exercised the production action itself as its D:
  layout, so the 94 s / 117 s D: restores measured exactly what production now runs.
- First production run (the PR's own `Tests` run 33898160232, after every earlier run on the branch
  had been cancelled by the next push): `Build (windows)` logged the relocation, took the expected
  cold miss, and **saved 1.84 GB in 156 s** (main's last Windows build spent 416 s on that save from
  C:); all ten Windows acceptance shards then **restored it in 90-116 s** (`Cache hit`, 1,840 MB,
  post step 0 s) where the same restore took 4-7 minutes before.

## Follow-ups

- The "Cache Atmos toolchain" archive was checked and deliberately left on C:: it is ~361 MB of a
  few large binaries, restores in 6-16 s and saves in 34 s on the Windows shards, so the small-file
  IOPS penalty that makes D: matter for the Go caches barely applies, and relocating it would mean
  moving the Atmos cache root (`ATMOS_XDG_CACHE_HOME`) job-wide, which the acceptance tests forbid.
  For consumers of the published `actions/cache` action, whose cached trees may well be small-file
  heavy (provider mirrors, plugin caches), the pattern is codified as its opt-in `cache-home` input.
- The 10 GB quota saturation is a separate problem (every Tests run writes ~5 GB); smaller
  archives or fewer cached jobs would let main's entry survive between runs.
