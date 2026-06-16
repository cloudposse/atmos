# Honor `ignore_missing_template_values` for stack `name_template` rendering

**Issue:** [#2345](https://github.com/cloudposse/atmos/issues/2345)
**Date:** 2026-06-15
**Status:** Fixed
**Scope:** This is **PR 1 of 3** in the split of the rejected omnibus PR
[#2387](https://github.com/cloudposse/atmos/pull/2387). It is intentionally small and
self-contained (a flag-routing fix) so it can be reviewed and merged independently of the
larger `locals` / stack-name-derivation work. See
[`locals-current-state.md`](./locals-current-state.md) §12–§13 for the full split plan.

---

## Problem

Atmos supports a global template setting:

```yaml
# atmos.yaml
templates:
  settings:
    ignore_missing_template_values: true
```

When `true`, Go template processing must **not** fail if a referenced variable is missing;
the missing reference renders as `<no value>` instead of aborting with an error. This flag was
added in PR #2158.

Users who set this flag still hit hard errors like:

```
template: ...name-template...: executing "..." at <.vars.tenant>: map has no entry for key "tenant"
```

…specifically when the error originated from rendering the **stack `name_template`** (the
`stacks.name_template` used to derive a logical stack name), rather than from rendering a stack
manifest body. Setting the global flag had no effect on these code paths.

## Root cause

`ProcessTmpl` takes an explicit `ignoreMissingTemplateValues bool` as its last parameter
(`internal/exec/template_utils.go`). When `false`, the template option is `missingkey=error`;
when `true`, it is `missingkey=default` (renders `<no value>`).

Every call site that rendered a **name template** passed a hardcoded `false`, ignoring the
user's global `atmosConfig.Templates.Settings.IgnoreMissingTemplateValues`. So the global flag
was silently bypassed for all name-template rendering, regardless of the user's configuration.

## Fix

Replace the hardcoded `false` with `atmosConfig.Templates.Settings.IgnoreMissingTemplateValues`
at every name-template `ProcessTmpl` call site. At each of these sites `atmosConfig` is already
dereferenced in the same expression (e.g. `atmosConfig.Stacks.NameTemplate`), so no additional
nil-guard is required.

This is a behavior-preserving change for existing users: the flag defaults to `false`, so any
configuration that does not opt in renders exactly as before. The fix only changes behavior when
the user has explicitly set `ignore_missing_template_values: true`.

### Affected call sites (11)

| File | Template name |
|---|---|
| `internal/exec/atlantis_generate_repo_config.go` | `atlantis-stack-name-template` |
| `internal/exec/aws_eks_update_kubeconfig.go` | `cluster_name_template` |
| `internal/exec/describe_affected_utils_2.go` | `spacelift-admin-stack-name-template` |
| `internal/exec/describe_affected_utils_2.go` | `spacelift-stack-name-template` |
| `internal/exec/describe_locals.go` | `describe-locals-name-template` |
| `internal/exec/spacelift_utils.go` | `name-template` |
| `internal/exec/stack_utils.go` | `terraform-workspace-stacks-name-template` |
| `internal/exec/terraform_generate_backends.go` | `terraform-generate-backends-template` |
| `internal/exec/terraform_generate_varfiles.go` | `terraform-generate-varfiles-template` |
| `internal/exec/utils.go` | `name-template` |
| `internal/exec/validate_stacks.go` | `validate-stacks-name-template` |

> Note: the `describe-locals-name-template` site lives inside `deriveStackNameFromTemplate`,
> which PR 2 of the split will rework further (to also read `settings`/`env` and to walk imports).
> Routing the flag here now is forward-compatible with that work.

## Tests

`internal/exec/stack_utils_test.go` →
`TestBuildTerraformWorkspace_IgnoreMissingTemplateValues`.

`BuildTerraformWorkspace` is a stable, low-setup caller of a name-template `ProcessTmpl` site,
chosen so the regression test is independent of the larger derivation rework in PR 2. The test
configures a `name_template` that references a key absent from the component section and asserts
both directions:

- **flag disabled** → rendering errors (`missingkey=error`), preserving the strict default.
- **flag enabled** → rendering succeeds; the present key resolves and the missing key renders as
  `<no value>` (result `"acme-<no value>"`).

Verified to **fail before** the fix (the flag-enabled case errored with
`map has no entry for key "missing_key"`) and **pass after**. The full `internal/exec` package
test suite passes.

## Incidental cleanup in `stack_utils.go` (err113)

Touching `stack_utils.go` for the `terraform-workspace-stacks-name-template` site caused
`gofumpt` to reformat two adjacent, pre-existing `fmt.Errorf` calls (in
`BuildDependentStackNameFromDependsOnLegacy` and `BuildDependentStackNameFromDependsOn`).
That reformatting marked those lines "new" relative to `origin/main`, so
`golangci-lint --new-from-rev=origin/main` flagged the pre-existing `err113` debt on them
("do not define dynamic errors").

Rather than suppress it, those two errors were converted to the repository's mandated
static-wrapped-error pattern: two new sentinels in `errors/errors.go`
(`ErrInvalidDependsOn`, `ErrInvalidSettingsDependsOn`), wrapped with `%w`. Messages are
otherwise unchanged. New tests `TestBuildDependentStackNameFromDependsOnLegacy` and
`TestBuildDependentStackNameFromDependsOn` cover both the resolution branches and the
`errors.Is` sentinel behavior on the unresolved path.

## Out of scope (handled by later PRs in the split)

- #2374 / #2343 — stack-name derivation reading imported `vars`/`settings`/`env` and walking
  imports (PR 2).
- #2343 deeper bug — the import-recursion vars clobber gated by `localsContextResult`, and the
  #2344 perf validation (PR 3).
