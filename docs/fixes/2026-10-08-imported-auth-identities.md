# Fix: Preserve imported auth identities during configuration loading

**Date:** 2026-10-08

## Summary

Identities from explicit imports, `atmos.d` / `.atmos.d` fragments, and provisioned
identity files now survive when the main `atmos.yaml` also declares identities.
Partial overrides retain imported fields, dotted names remain intact, and original
identity and environment key casing is preserved. Fixes
[cloudposse/atmos#3335](https://github.com/cloudposse/atmos/issues/3335).

## Context

The dotted-name fix in [#2129](https://github.com/cloudposse/atmos/pull/2129)
introduced identity reconstruction from tracked YAML files after Viper merges
configuration. The concurrency fix in
[#2980](https://github.com/cloudposse/atmos/pull/2980) subsequently keyed source
tracking by Viper instance, but imports use a temporary instance that was never
associated with the outer load's tracker. Imported paths were silently omitted.
Reconstruction then replaced the correctly merged identity map with only the
main file's identities. This attribution follows inspection of the commit
history; a historical bisect was not run.

Existing tests covered individual stages: dotted identities in one file, imported
identities in intermediate Viper values, and case preservation with manually
registered source paths. Concurrent-load tests also used files without imports.
They did not assert the final `LoadConfig` result with identities split between
the main file and an import.

## Changes

- Associate temporary import Viper instances with the owning load's tracker and
  release their registry entries on both success and error.
- Retain immutable source YAML in effective merge order, including repeated
  merges, so reconstruction still works after temporary imports are deleted;
  keep `LoadedConfigFiles()` deduplicated.
- Record the main configuration after its imports in the directory-loading path
  and use the retained sources for identity reconstruction and case restoration.
- Deep-merge raw identity maps before decoding, preserving imported fields when
  later sources set only `default` or override a nested principal field, including
  explicit `false` values.
- Add final-result regression tests for explicit and default imports, nested
  imports, profiles, multiple configuration files, provisioned identities,
  temporary sources, dotted names, casing, YAML functions, concurrent isolation,
  tracker cleanup, and configurations with no main-file identities.

## Validation

The following checks passed:

```shell
go test ./pkg/config ./pkg/config/adapters -short -count=1
go test -race ./pkg/config -run 'TestLoadConfig_(ImportedIdentities|ConcurrentCalls)|TestIdentityNamesWithDots|TestAuthIdentitiesResolve' -count=1
go test -race ./pkg/config/adapters -run '^TestLoadConfig_ImportedIdentitiesTemporarySource$' -count=1
```

Built the workspace CLI and verified the issue's `main/admin` and `shared/reader`
identities appear in both `describe config` and `auth list`, using default config
discovery and explicit `--config-path`, without an AWS login. Patch-scoped lint
with the repository's custom `golangci-lint` binary reported zero issues. The
initial system linter lacked `lintroller`; building the supported custom binary
resolved that tooling limitation.

## Follow-ups

None.
