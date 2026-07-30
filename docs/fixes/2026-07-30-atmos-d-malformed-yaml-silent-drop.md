# Fix: `atmos.d`/`.atmos.d` malformed YAML no longer silently dropped

**Date:** 2026-07-30

## Summary

A YAML parse error in an `atmos.d/` or `.atmos.d/` config file was logged at `log.Debug`
only and discarded. Atmos exited `0` with no visible error, and because the per-file merge
loop bails on the first bad file, every file sorted after the broken one silently never
loaded either. This hard-fails `LoadConfig` instead, matching how a malformed root
`atmos.yaml` and malformed profile configs (#2825) already behave.

## Context

[Issue #2836](https://github.com/cloudposse/atmos/issues/2836): a flag description in a
custom command contained an unquoted `default:`, a YAML syntax error. The command silently
never registered — `atmos <name>` reported an unknown command as though the file had never
been written, `atmos --help` exited `0`, and nothing pointed at YAML. Only
`ATMOS_LOGS_LEVEL=debug` revealed the swallowed error. Whether a given `atmos.d`/`.atmos.d`
file's settings applied depended on its filename's sort position relative to an unrelated
broken file elsewhere in the same directory.

`pkg/config/load.go`'s `loadAtmosConfigsFromDirectoryWithMerge` already produced a
well-formed error (`errUtils.ErrParseFile` + source + file path + the underlying
`yaml: line N: ...` detail) — the bug was purely that two call sites threw it away with
`log.Debug`/`log.Trace` instead of returning it.

## Changes

- `pkg/config/load.go`:
  - `loadAtmosDFromDirectory` and `loadAtmosDFromGitRoot` now return `error` instead of
    discarding it; both `atmos.d` and `.atmos.d` are still checked independently, with
    their errors combined via `errors.Join`.
  - `mergeDefaultImports` joins the git-root and config-dir errors instead of always
    returning `nil`.
  - Both call sites (`processConfigImportsAndReapply`, and `LoadConfig`'s
    no-`atmos.yaml`-found default-config fallback) now hard-fail
    (`return ..., err`) unless the error is `errUtils.ErrAtmosDirConfigNotFound` (the
    directory legitimately doesn't exist), which remains non-fatal.
  - Per explicit decision, this applies uniformly to both call sites, including the
    zero-config fallback path — the only carve-outs are the pre-existing ones in
    `cmd/root.go`'s `handleConfigInitErrorWithArgs` (`atmos version`/`--version`,
    `--help`, `atmos config validate`/`atmos validate config`/`atmos validate schema
    config`, and CI git-clone bootstrap), which key off command args, not error content,
    so they required no changes.
  - Stat/permission errors on the `atmos.d`/`.atmos.d` directories themselves (a
    different failure class from a YAML parse error) remain non-fatal and are only
    logged, unchanged from before.
  - Reused the existing `errUtils.ErrParseFile` and `errUtils.ErrAtmosDirConfigNotFound`
    sentinels; no new sentinel errors were added.
- `pkg/config/load_error_paths_test.go`: updated the 5 existing
  `loadAtmosDFromDirectory` call sites to capture and assert the new `error` return
  (valid-YAML cases assert `nil`); added `TestLoadAtmosDFromDirectory_MalformedYAML_AtmosD`,
  `_DotAtmosD`, and `_SortOrderStillSurfacesError` (the issue's exact repro shape: a good
  file, a broken file, and another good file sorting after it).
- `pkg/config/load_error_paths_unix_test.go`: updated
  `TestLoadAtmosDFromDirectory_StatPermissionError`'s call site and stale comment for the
  new signature; behavior (non-fatal for stat errors) is unchanged.
- `pkg/config/load_test.go`: added `TestLoadConfig_AtmosDMalformedYAMLHardFails`
  (atmos.yaml present, co-located `.atmos.d` broken) and
  `TestLoadConfig_DefaultConfigWithGitRootAtmosDMalformedYAMLHardFails` (no atmos.yaml,
  broken `.atmos.d` at the git root) as full `LoadConfig` integration tests. All new
  malformed-YAML tests assert the error message contains both the broken file's path and
  a `"line "` substring, locking in that file/line detail survives the `errors.Join`/`%w`
  propagation chain to the terminal.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/config/...` — all pass, including new and updated tests.
- `go test ./cmd/...` — all pass (confirms `version`/`--version`/`config validate`/
  `validate schema config`/help/CI-bootstrap carve-outs are unaffected).
- Manual repro of the issue's exact shell script against a locally built binary: before
  the fix, `atmos --help` exited `0` and `atmos zulu` reported an unknown command; after
  the fix, `atmos alpha`/`atmos zulu` exit `1` with
  `**Error:** failed to parse file: failed to load configuration file from .atmos.d:
  <path>/.atmos.d/m-broken.yaml: yaml: line 3: mapping values are not allowed in this
  context`. Confirmed `atmos version`, `atmos --version`, `atmos config validate`, and
  `atmos validate schema config` still run against the same broken fixture (the latter
  two correctly report the broken file too, since `atmos validate schema config`
  independently re-globs and validates `atmos.d`/`.atmos.d` fragments regardless of
  `LoadConfig`).
- `gofumpt -l` on all changed files — clean, no output.
- `atmos fix lint` (patch-scoped `--new-from-rev=origin/main`) — initially flagged two
  `nilerr` findings in `loadAtmosDFromGitRoot`'s guard clauses (git-root-detection and
  path-resolution failures deliberately return `nil`, not the git-root-detection error
  itself, since those failures only skip an optional check rather than being fatal).
  Fixed with justified `//nolint:nilerr` annotations; re-ran clean (`0 issues.`).

## Follow-ups

None.
