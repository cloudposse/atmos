# Fix --identity Flag Not Accepting Space-Separated Values

**Date:** 2026-02-17

**Related Issue:** `--identity` flag causes "Too many command line arguments" error from OpenTofu/Terraform
when the value is provided with a space separator instead of `=`.

**Affected Atmos Version:** v1.160.0+ (introduced with Atmos Auth)

**Severity:** High — users cannot use the standard CLI convention `--identity <value>` (space-separated),
forcing them to use either `--identity=<value>` or `ATMOS_IDENTITY=<value>`.

## Background

The `--identity` flag was introduced to specify which identity to authenticate with before running
Terraform commands. It supports three usage patterns:

1. `--identity=<name>` — use the specified identity (works correctly).
2. `--identity` (no value) — interactively select an identity (works correctly).
3. `--identity <name>` (space-separated) — use the specified identity (**broken**).

To support pattern #2 (no value → interactive selection), the flag was registered with
`NoOptDefVal: "__SELECT__"`. In pflag, `NoOptDefVal` ("no option default value") causes the flag to
behave like a boolean: when the flag appears without `=`, pflag uses `NoOptDefVal` instead of consuming
the next argument as the value.

## Symptoms

```text
$ atmos terraform plan tfstate-backend -s core-usw2-root --identity my-identity

Error: Too many command line arguments

Expected at most 1 positional argument(s), got 2
```

The workaround was to use `=` syntax or the environment variable:
```shell
# Workaround 1: Use = syntax
$ atmos terraform plan tfstate-backend -s core-usw2-root --identity=my-identity

# Workaround 2: Use environment variable
$ ATMOS_IDENTITY=my-identity atmos terraform plan tfstate-backend -s core-usw2-root
```

## Root Cause

The flag registration in `cmd/terraform/flags.go`:

```go
registry.Register(&flags.StringFlag{
    Name:        "identity",
    NoOptDefVal: "__SELECT__",  // This is the problem
    // ...
})
```

When pflag encounters `--identity my-identity` (space-separated):

1. pflag sees `--identity` without `=`.
2. Because `NoOptDefVal` is set, pflag uses `"__SELECT__"` as the identity value.
3. `my-identity` is left as an orphaned positional argument.
4. The orphaned arg passes through to Terraform/OpenTofu as `terraform plan my-identity`.
5. Terraform reports "Too many command line arguments".

The manual identity extraction in `processArgsAndFlags` (line 743) correctly handles `--identity <value>`,
but it never fires because Cobra/pflag already consumed `--identity` before the manual parsing runs.

## Fix

### Approach

Normalize `--identity <value>` to `--identity=<value>` **before** Cobra/pflag parses the arguments.
This happens in `preprocessCompatibilityFlags()` which already runs before Cobra parsing.

The normalization detects known flags that use `NoOptDefVal` and joins them with the following
non-flag argument using `=`, converting the space-separated form to the `=`-separated form that
pflag handles correctly.

### Implementation

Uses the existing `FlagRegistry.PreprocessNoOptDefValArgs` method (`pkg/flags/registry.go`)
which is the same normalization used by `AtmosFlagParser.Parse()` for commands that have adopted
the new `StandardParser` pattern.

#### 1. Register flag registry in command registry (`cmd/internal/registry.go`)

Added `RegisterCommandFlagRegistry` / `GetCommandFlagRegistry` functions that allow commands
to register their `FlagRegistry` for use in `preprocessCompatibilityFlags`. This follows the
same pattern as `RegisterCommandCompatFlags` / `GetCompatFlagsForCommand`.

#### 2. Register terraform flag registry (`cmd/terraform/terraform.go`)

Call `internal.RegisterCommandFlagRegistry("terraform", terraformParser.Registry())` during init,
making the terraform parser's flag registry (which includes `--identity` with `NoOptDefVal`)
available to `preprocessCompatibilityFlags`.

#### 3. Use registry in `preprocessCompatibilityFlags` (`cmd/root.go`)

Instead of a hardcoded flag map, `preprocessCompatibilityFlags` now calls
`flagRegistry.PreprocessNoOptDefValArgs(osArgs)` using the command's registered flag registry.
This is the same method used by `AtmosFlagParser.Parse()` (Step 2.6 in `pkg/flags/flag_parser.go`).

### Files changed

| File                         | Change                                                                |
|------------------------------|-----------------------------------------------------------------------|
| `cmd/root.go`                | Use `FlagRegistry.PreprocessNoOptDefValArgs` in `preprocessCompatibilityFlags` |
| `cmd/internal/registry.go`   | Add `RegisterCommandFlagRegistry` / `GetCommandFlagRegistry`           |
| `cmd/terraform/terraform.go` | Register terraform flag registry during init                           |

### Tests

Normalization is tested by the existing `TestFlagRegistry_PreprocessNoOptDefValArgs` tests
in `pkg/flags/registry_preprocess_test.go` which cover all scenarios including:

| Test case                                            | What it verifies                                                    |
|------------------------------------------------------|---------------------------------------------------------------------|
| `identity flag with space syntax`                    | `--identity value` becomes `--identity=value`                        |
| `identity flag with equals syntax`                   | `--identity=value` is unchanged                                     |
| `identity at end of args`                            | `--identity` alone is unchanged (interactive selection)              |
| `identity flag followed by another flag`             | `--identity --dry-run` leaves identity without value                 |
| `double dash prefix`                                 | `--identity -- plan` is not normalized (next arg is `--`)            |
| `empty registry`                                     | Args without NoOptDefVal flags are unchanged                         |
| `multiple NoOptDefVal flags`                         | Multiple flags are each normalized correctly                         |
