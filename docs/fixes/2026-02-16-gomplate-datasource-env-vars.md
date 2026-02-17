# Fix Gomplate Datasource Environment Variable Propagation

**Date:** 2026-02-16

**Related Issue:** [#2083](https://github.com/cloudposse/atmos/issues/2083) — Gomplate datasources can't access AWS
credentials from `settings.templates.settings.env`

**Affected Atmos Version:** v1.160.0+ (env propagation never worked due to `mapstructure:"-"` tag)

**Severity:** High — blocks gomplate datasources that require AWS credentials (S3, SSM, Secrets Manager)

## Background

Atmos supports configuring environment variables for template processing via `templates.settings.env` in
`atmos.yaml` and `settings.templates.settings.env` in stack manifests. These environment variables should be
set in the OS environment before gomplate datasources are evaluated, allowing datasources like S3, SSM, and
Secrets Manager to use AWS credentials.

Example configuration in `atmos.yaml`:

```yaml
templates:
  settings:
    enabled: true
    gomplate:
      enabled: true
    env:
      AWS_PROFILE: "my-profile"
      AWS_REGION: "us-east-1"
```

Or in stack manifests:

```yaml
settings:
  templates:
    settings:
      env:
        AWS_PROFILE: "production"
        AWS_REGION: "us-west-2"
```

## Root Cause

Three interconnected bugs prevent `templates.settings.env` from propagating to gomplate datasources:

### Bug 1: `mapstructure:"-"` drops Env field during merge pipeline

`TemplatesSettings.Env` at `pkg/schema/schema.go:416` uses `mapstructure:"-"` tag to avoid a type collision
with `Command.Env` (which is `[]CommandEnv`, a different type). This causes `mapstructure.Decode` to silently
drop the `Env` field during both struct-to-map and map-to-struct conversions.

In `ProcessTmplWithDatasources` (`internal/exec/template_utils.go`), the merge pipeline works as:

1. `mapstructure.Decode(atmosConfig.Templates.Settings, &cliConfigMap)` — Env dropped (struct → map)
2. `mapstructure.Decode(settingsSection.Templates.Settings, &stackManifestMap)` — Env dropped (struct → map)
3. `merge.Merge(cliConfigMap, stackManifestMap)` — merged map has no env
4. `mapstructure.Decode(merged, &templateSettings)` — Env still nil

The `os.Setenv` loop at lines 190-195 never executes because `templateSettings.Env` is always nil.

### Bug 2: Viper lowercases env var keys

Viper lowercases all YAML map keys, so `AWS_PROFILE` becomes `aws_profile`. The `caseSensitivePaths`
mechanism restores case for specific paths but didn't include `templates.settings.env`.

### Bug 3: Stack manifest env vars dropped at caller sites

When stack manifests define `settings.templates.settings.env`, it gets decoded via
`mapstructure.Decode(settingsSection, &settingsSectionStruct)` in callers (`utils.go`, `describe_stacks.go`).
The same `mapstructure:"-"` tag drops the env field at these decode sites too.

## Fix

### Approach

Rather than changing the `mapstructure:"-"` tag (which would reintroduce the type collision with
`Command.Env`), we extract the `env` map directly from raw map sources before `mapstructure.Decode`
drops it, then inject it back after the merge.

### Implementation

#### 1. Helper function `extractEnvFromRawMap` (template_utils.go)

Extracts env vars from raw `map[string]any` sources, navigating the nested path
`templates` → `settings` → `env`. Handles both `map[string]any` and `map[string]string` value types.

```go
func extractEnvFromRawMap(settingsMap map[string]any) map[string]string
```

#### 2. Helper function `setEnvVarsWithRestore` (template_utils.go)

Sets environment variables and returns a cleanup function that restores original values.
Prevents env pollution across components by saving and restoring the original state.

```go
func setEnvVarsWithRestore(envVars map[string]string) (func(), error)
```

#### 3. Fix `ProcessTmplWithDatasources` (template_utils.go)

- Before the mapstructure encode/decode/merge pipeline, extracts `Env` directly from both
  `atmosConfig.Templates.Settings.Env` and `settingsSection.Templates.Settings.Env`
- After the merge/decode pipeline, injects the merged env back into `templateSettings.Env`
  (stack manifest env overrides CLI config env)
- Uses `setEnvVarsWithRestore` for deferred cleanup instead of raw `os.Setenv`
- Also extracts env from the re-decoded template settings within the evaluation loop.

#### 4. Fix stack manifest env extraction (utils.go, describe_stacks.go)

After `mapstructure.Decode(settingsSection, &settingsSectionStruct)`, calls `extractEnvFromRawMap`
on the raw map and injects the result into `settingsSectionStruct.Templates.Settings.Env`.
Applied at 4 call sites:

- `internal/exec/utils.go` — 1 site (terraform component processing)
- `internal/exec/describe_stacks.go` — 3 sites (terraform, helmfile, packer sections)

#### 5. Case sensitivity fix (load.go, schema.go)

- Added `"templates.settings.env"` to `caseSensitivePaths` in `pkg/config/load.go`
- Extended `GetCaseSensitiveMap` in `pkg/schema/schema.go` to handle `"templates.settings.env"` path
- After `preserveCaseSensitiveMaps` runs, applies case restoration to `atmosConfig.Templates.Settings.Env`

### Files changed

| File                                       | Change                                                                                    |
|--------------------------------------------|-------------------------------------------------------------------------------------------|
| `internal/exec/template_utils.go`          | Added `extractEnvFromRawMap`, `setEnvVarsWithRestore`; fixed `ProcessTmplWithDatasources` |
| `internal/exec/utils.go`                   | Added env extraction after mapstructure.Decode (1 site)                                   |
| `internal/exec/describe_stacks.go`         | Added env extraction after mapstructure.Decode (3 sites)                                  |
| `pkg/config/load.go`                       | Added `"templates.settings.env"` to `caseSensitivePaths`; apply case map after extraction |
| `pkg/schema/schema.go`                     | Extended `GetCaseSensitiveMap` to support `"templates.settings.env"`                      |
| `internal/exec/template_utils_env_test.go` | New: 6 test functions with comprehensive coverage                                         |

### Tests

`internal/exec/template_utils_env_test.go` contains:

| Test                                                      | What it verifies                                                                         |
|-----------------------------------------------------------|------------------------------------------------------------------------------------------|
| `TestExtractEnvFromRawMap`                                | 8 subtests: nil/empty/missing keys, map[string]any, map[string]string, non-string values |
| `TestSetEnvVarsWithRestore`                               | 2 subtests: set+restore cycle, empty map no-op                                           |
| `TestProcessTmplWithDatasources_EnvVarsFromConfig`        | Env vars from `atmosConfig.Templates.Settings.Env` are available in templates            |
| `TestProcessTmplWithDatasources_EnvVarsFromStackManifest` | Stack manifest env overrides CLI config env                                              |
| `TestProcessTmplWithDatasources_EnvVarsCleanedUp`         | Original env values are restored after template processing                               |
| `TestProcessTmplWithDatasources_EnvVarsCaseSensitive`     | Uppercase env var keys (e.g., `AWS_TEST_CASE_KEY`) work correctly                        |

All tests pass:

```text
ok  github.com/cloudposse/atmos/internal/exec  1.207s
ok  github.com/cloudposse/atmos/pkg/config     3.984s
ok  github.com/cloudposse/atmos/pkg/schema     0.273s
```
