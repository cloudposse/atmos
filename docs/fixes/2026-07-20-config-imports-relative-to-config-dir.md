# Fix: atmos.yaml imports resolve relative to the config directory

**Date:** 2026-07-20

## Summary

Top-level `import:` entries in `atmos.yaml` now resolve relative to the
directory containing that `atmos.yaml` instead of the current working directory,
so commands run from a subdirectory can still load imports such as
`import: [".atmos/commands/**/*"]`.

## Context

`mergeImports` computed the import base path with `filepath.Abs(base_path)`.
When `base_path` is empty or dot-relative (the common case, e.g. `.` or `./`),
`filepath.Abs` anchors it to the process CWD. Running a command from a nested
directory (e.g. `/repo/acme/somepath` while `atmos.yaml` is at `/repo/acme`)
therefore resolved `import:` globs against the subdirectory and failed to find
them. Imports should anchor to the atmos root — the directory of the discovered
`atmos.yaml` — matching how `base_path` is resolved for config-file sources.

The fix must preserve the established source-aware base-path convention: runtime
sources (`ATMOS_BASE_PATH`, `--base-path`, the `atmos_base_path` provider
parameter) and the `!cwd` YAML function keep CWD semantics for dot-relative
paths; bare paths keep git-root search; absolute paths pass through unchanged.

## Changes

- `pkg/config/load.go`: `mergeImports` and `ResolveConfigImportBasePath` (the
  file-aware resolver for nested imports) route the import base path through the
  canonical `resolveAbsolutePath` source-aware category model: config-sourced
  empty and dot-relative paths anchor to the config directory, bare paths use
  git-root search, absolute paths pass through, and runtime/`!cwd` dot paths
  anchor to the CWD. A `base_path: !cwd` provenance is detected from the raw
  YAML tag via `importBasePathDeclaration`.
- `pkg/config/load.go`: a runtime base-path override (`ATMOS_BASE_PATH`,
  `--base-path`, or the `atmos_base_path` provider param) is recorded on the
  main Viper instance by `LoadConfig` and honored during import resolution, so
  its dot-relative values anchor imports to the CWD. Previously runtime
  overrides were applied only after imports were processed, so top-level imports
  could not distinguish a runtime dot base path from a config-file
  `base_path: .`.
- `pkg/config/imports.go`: import processing reports the directory of an
  imported file that declares `base_path`, so provenance follows imported
  declarations.
- `pkg/config/load_config_args.go`: `mergeFiles` tracks the declaring config
  directory across multiple `--config` files; an empty base path anchors to each
  importing file rather than the first one.
- `pkg/config/adapters/local_adapter.go`,
  `pkg/config/adapters/gogetter_adapter.go`: nested imports resolve a declared
  relative `base_path` against the importing file's directory via the shared
  `ResolveConfigImportBasePath` helper.
- Tests: `pkg/config/import_base_path_test.go` covers the resolver, the
  CWD-independent `.atmos/commands/**/*`-from-a-subdirectory reproduction,
  bare-path and `!cwd` compatibility, and multi-`--config` provenance;
  `pkg/config/import_base_path_runtime_test.go` covers runtime override
  detection (flag/provider/env precedence) and end-to-end CWD anchoring of
  imports for `ATMOS_BASE_PATH`, contrasted with the config-file case. The
  import-merge tests in `config_import_test.go` register the local adapter and
  assert the real deep-merge behavior; the nested-import assertion in
  `adapters/adapters_test.go` was tightened. Error-path tests exercise the
  resolver, temp-dir, and base-path-parse failure branches.

## Validation

```bash
go build ./...
go test ./pkg/config/... -count=1
go test ./pkg/config ./pkg/config/adapters -shuffle=on -count=10
go test ./internal/exec/... ./cmd/... -short -count=1
```

The two new CWD-independence tests fail on the pre-fix code (the import resolves
against the CWD, yielding a value of `0`) and pass after the fix. `gofumpt`
reports no changes and the custom `golangci-lint` build reports no findings on
the changed lines.

## Follow-ups

None.
