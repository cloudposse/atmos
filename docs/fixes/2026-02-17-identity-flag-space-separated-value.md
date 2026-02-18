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

```
$ atmos terraform plan tfstate-backend -s core-usw2-root --identity my-identity

Error: Too many command line arguments

Expected at most 1 positional argument(s), got 2
```

The workaround was to use `=` syntax or the environment variable:
```
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

#### 1. Add `normalizeOptionalValueFlags` function (`cmd/root.go`)

New function that normalizes `--flag value` to `--flag=value` for flags registered with `NoOptDefVal`.
Currently applies to `--identity`.

#### 2. Call normalization in `preprocessCompatibilityFlags` (`cmd/root.go`)

The normalization runs before the compat flag translator, ensuring the args are correct for both
Cobra parsing and compat flag separation.

### Files changed

| File                         | Change                                                                |
|------------------------------|-----------------------------------------------------------------------|
| `cmd/root.go`                | Add `normalizeOptionalValueFlags`; call from `preprocessCompatibilityFlags` |
| `cmd/root_test.go`           | Tests for normalization function                                       |

### Tests

| Test                                                              | What it verifies                                                    |
|-------------------------------------------------------------------|---------------------------------------------------------------------|
| `TestNormalizeOptionalValueFlags_IdentityWithValue`                | `--identity value` becomes `--identity=value`                        |
| `TestNormalizeOptionalValueFlags_IdentityWithEquals`               | `--identity=value` is unchanged                                     |
| `TestNormalizeOptionalValueFlags_IdentityWithoutValue`             | `--identity` alone is unchanged (interactive selection)              |
| `TestNormalizeOptionalValueFlags_IdentityFollowedByFlag`           | `--identity --stack` leaves identity without value                   |
| `TestNormalizeOptionalValueFlags_IdentityAfterEndOfOptions`        | `-- --identity value` is not normalized (after --)                   |
| `TestNormalizeOptionalValueFlags_NoOptionalValueFlags`             | Args without optional-value flags are unchanged                      |
| `TestNormalizeOptionalValueFlags_MultipleOptionalValueFlags`       | Multiple flags are each normalized correctly                         |
