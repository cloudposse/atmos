# `processArgsAndFlags` Refactor Audit — Findings and Fixes

**Date:** 2026-03-20

**Related PR:** #2225 — `refactor(cli_utils): DRY processArgsAndFlags with table-driven flag parsing, 100% unit test coverage`

**Severity:** Low–Medium — correctness improvements and missing test coverage

---

## What PR #2225 Did

PR #2225 refactored `processArgsAndFlags` — the highest-cyclomatic-complexity function in Atmos
(~67 → ~15) — from 25+ copy-paste if/else chains into a table-driven design. It also fixed five
real bugs:

1. **Boolean flags silently dropping adjacent Terraform pass-through args** — `--dry-run`,
   `--skip-init`, `--affected`, `--all`, `--process-templates`, `--process-functions` all
   unconditionally stripped `args[i+1]`, silently dropping flags like `--refresh=false`.

2. **`strings.Split(arg, "=")` rejecting values containing `=`** — e.g.,
   `--query=.tags[?env==prod]` errored. Fixed with `strings.SplitN(arg, "=", 2)`.

3. **`strings.HasPrefix(arg+"=", flag)` prefix collision** — logically inverted; e.g.,
   `--terraform-command-extra` would match `--terraform-command`. Fixed to
   `strings.HasPrefix(arg, flag+"=")`.

4. **`--from-plan` and `--identity` unconditionally consuming `args[i+1]`** — even when the
   next arg was another flag. Fixed to only consume when next arg doesn't start with `-`.

5. **`--settings-list-merge-strategy` leaking into Terraform pass-through args** — added to
   `commonFlags`.

### New Architecture

- **`stringFlagDefs`** — 26-entry table mapping flags to `ArgsAndFlagsInfo` field setters.
- **`parseFlagValue`** — single helper for both `--flag value` and `--flag=value` forms.
- **`parseIdentityFlag`** / **`parseFromPlanFlag`** — dedicated helpers for optional-value
  flag semantics.
- **`valueTakingCommonFlags`** — set distinguishing value-taking from boolean flags for
  correct stripping.

---

## Audit Findings

### Finding 1: `valueTakingCommonFlags` stripping doesn't bounds-check `i+1`

**Severity:** Low (harmless at runtime, inconsistent with other checks)

The optional-value flags `--from-plan` and `--identity` bounds-check before stripping `i+1`:

```go
if len(inputArgsAndFlags) > i+1 && !strings.HasPrefix(inputArgsAndFlags[i+1], "-") {
    indexesToRemove = append(indexesToRemove, i+1)
}
```

But the `valueTakingCommonFlags` branch does not:

```go
} else if valueTakingCommonFlags[f] {
    indexesToRemove = append(indexesToRemove, i+1)  // No bounds check
}
```

For flags in `stringFlagDefs`, `parseFlagValue` returns an error before stripping runs, so
the OOB index is never reached. But flags only in `valueTakingCommonFlags` (e.g., `--stack`,
`-s`, `--global-options`, `--kubeconfig-path`, profiler flags, `--skip`) silently add an OOB
index. The OOB index is harmless (never matches during the second pass), but the inconsistency
should be fixed.

**Fix:** Add `len(inputArgsAndFlags) > i+1` guard to the `valueTakingCommonFlags` branch.

### Finding 2: No test for `--global-options` stripping from `AdditionalArgsAndFlags`

**Severity:** Medium (coverage gap)

`TestProcessArgsAndFlags_GlobalOptions` verifies `GlobalOptions` is populated but doesn't
assert that `--global-options` and its value are stripped from pass-through args.

**Fix:** Add assertions for `AdditionalArgsAndFlags` in the global options test.

### Finding 3: No test for equals-form boolean flag stripping

**Severity:** Medium (coverage gap)

Boolean flags like `--process-templates` can appear in equals form (`--process-templates=false`).
The stripping logic handles this via `HasPrefix(arg, f+"=")`, but no test exercises the path.

**Fix:** Add test cases for equals-form boolean flag stripping.

### Finding 4: No test for flag prefix collision safety

**Severity:** Medium (regression prevention)

Real prefix collisions exist between flags:

| Prefix flag    | Collides with                                                        |
|----------------|----------------------------------------------------------------------|
| `--heatmap`    | `--heatmap-mode`                                                     |
| `--profile`    | `--profiler-enabled`, `--profiler-host/port/file/type`               |
| `--skip`       | `--skip-init`, `--skip-planfile`                                     |

The new `HasPrefix(arg, f+"=")` pattern is safe (e.g., `--heatmap-mode=foo` doesn't start
with `--heatmap=`), but no test proves this. A future change could regress.

**Fix:** Add explicit prefix collision safety tests.

### Finding 5: Three parallel flag lists with no compile-time sync

**Severity:** Medium (maintainability)

Three lists must stay in sync: `commonFlags` (49 entries), `stringFlagDefs` (26 entries),
`valueTakingCommonFlags` (36 entries). A flag added to one but missing from another causes
silent misbehavior (parsed but not stripped, or stripped incorrectly).

**Fix:** Add `TestFlagListConsistency` that validates:
- Every `stringFlagDefs` flag is in `commonFlags`.
- Every `stringFlagDefs` flag is in `valueTakingCommonFlags`.
- Every `valueTakingCommonFlags` entry is in `commonFlags`.

### Finding 6: Space-separated values that look like flags are double-processed

**Severity:** Low (pre-existing behavior, not a regression)

When a value-taking flag's value happens to look like another flag, both parsing paths fire:

```sh
atmos terraform plan vpc --logs-level --logs-file /tmp/out
```

At `i=2`: `parseFlagValue("--logs-level", ..., 2)` matches, reads `args[3]` = `"--logs-file"`.
Sets `LogsLevel = "--logs-file"`.

At `i=3`: `parseFlagValue("--logs-file", ..., 3)` matches, reads `args[4]` = `"/tmp/out"`.
Sets `LogsFile = "/tmp/out"`.

So `--logs-file` is consumed as the VALUE of `--logs-level` AND as a flag name. `LogsLevel`
ends up as `"--logs-file"`, `LogsFile` as `"/tmp/out"`. This is the same pre-existing behavior
from the old code. The `--from-plan` and `--identity` handlers correctly check if the next arg
starts with `-`, but the generic `parseFlagValue` does not.

**Status:** Pre-existing behavior preserved by the refactor. Not fixed — would require a
breaking change to `parseFlagValue` semantics. Documented for awareness.

### Finding 7: `parseFlagValue` match doesn't short-circuit remaining checks

**Severity:** Info (style, no functional impact)

When `parseFlagValue` matches a flag in the `stringFlagDefs` inner loop, it `break`s the inner
loop but the outer `for i, arg` loop continues. The matched arg still runs through
`parseIdentityFlag`, `parseFromPlanFlag`, the boolean switch, and `commonFlags` stripping.
These don't match (the arg is a string flag, not identity/from-plan/boolean), so it's just
unnecessary work — not a bug. Acceptable for the typical 10–20 arg invocations.

### Finding 8: `--help` alone triggers single-arg early return

**Severity:** Low (pre-existing, Cobra handles upstream)

```go
inputArgsAndFlags: []string{"--help"}
// wantSubCommand: "--help"
// wantNeedHelp:   false
```

The single-arg early return (line 559) sets `SubCommand` to the literal `"--help"` and doesn't
set `NeedHelp`. This is surprising but intentional — Cobra processes `--help` as a flag before
this code runs. The behavior is documented in the existing test but worth noting for future
maintainers.

---

## Changes Made

### `internal/exec/cli_utils.go`

- Add bounds check (`len(inputArgsAndFlags) > i+1`) to the `valueTakingCommonFlags` stripping
  branch, matching the pattern used by `--from-plan` / `--identity`.

### `internal/exec/cli_utils_helpers_test.go`

- Add `TestFlagListConsistency` — validates all three flag lists are in sync.
- Add `TestProcessArgsAndFlags_PrefixCollisionSafety` — verifies flags with shared prefixes
  don't interfere during stripping or parsing.
- Add `TestProcessArgsAndFlags_BooleanFlagEqualsFormStripping` — verifies equals-form boolean
  flags are stripped without affecting adjacent args.
- Add `TestProcessArgsAndFlags_GlobalOptionsStripping` — verifies `--global-options` and its
  value are stripped from pass-through args.

---

## References

- PR #2225: `refactor(cli_utils): DRY processArgsAndFlags with table-driven flag parsing`
- `internal/exec/cli_utils.go` — main implementation
- `internal/exec/cli_utils_helpers_test.go` — helper unit tests
- `pkg/config/const.go` — flag constant definitions
