# Fix: Atmos Auth stack-level default identity resolution

**Date:** 2026-04-08

**Issues:**
- [#2293](https://github.com/cloudposse/atmos/issues/2293) — `auth.identities.<name>.default: true` in imported stack files not recognized during identity resolution
- [Discussion #122](https://github.com/orgs/cloudposse/discussions/122) — Auth inheritance not scoping to stack (a default identity declared in one stack manifest leaks to every other stack across all OUs)

## Status

**Two related bugs fixed without breaking any existing auth functionality.**
Both originate from the same global raw-YAML pre-scanner in
`pkg/config/stack_auth_loader.go`. The chosen fix is a **two-part change**:

1. **Split the auth manager entry points** into a "scan" variant (for
   multi-stack commands that legitimately have no target stack) and a
   "no-scan" variant (for commands that have already done an exec-layer
   merge against a specific target stack). This closes the Discussion #122
   leak at the source — the scanner can no longer contaminate merged
   configs.
2. **Teach the pre-scanner to follow `import:` chains** so that
   `auth.identities.<name>.default: true` declared inside an imported
   `_defaults.yaml` is visible everywhere — including for multi-stack
   commands (`describe stacks`, `describe affected`, `list affected`,
   workflows, `aws security`, `aws compliance`). This closes Issue #2293
   across *all* command categories.

The multi-stack "Approach 2" code path (documented in
`stack-level-default-auth-identity.md`) is preserved. No existing auth
functionality is removed.

### Progress checklist

- [x] Root-cause analysis.
- [x] Caller audit — categorized every call site of
      `CreateAndAuthenticateManagerWithAtmosConfig` into Category A
      (target-stack context) vs Category B (multi-stack / no target).
- [x] Identified regression risk: earlier draft (option "c" below) removed
      the pre-scanner entirely and broke the intentional Approach 2 code
      path for `describe stacks` / `describe affected` / `list affected` /
      workflows / `aws security`.
- [x] Two scenario fixtures under `tests/fixtures/scenarios/` using
      `mock/aws`.
- [x] CLI regression test cases in
      `tests/test-cases/auth-identity-resolution-bugs.yaml`.
- [x] Go unit tests at the function boundary and the auth-manager boundary.
- [x] Option (d+) implemented on fresh branch `aknysh/atmos-auth-fixes-3`
      off `main` — split entry points plus import-following scanner.
- [x] `pkg/config/stack_auth_loader.go` — added `loadAuthWithImports`
      (recursive, cycle-protected), `resolveAuthImportPaths`, and
      `extractImportPathString`. `LoadStackAuthDefaults` now follows
      `import:` chains. `allAgree` conflict detection (Issue #2072) preserved.
- [x] `pkg/auth/manager_helpers.go` — `CreateAndAuthenticateManagerWithAtmosConfig`
      is now the NO-SCAN variant; added `CreateAndAuthenticateManagerWithStackScan`
      for Category B. `scanStackFilesForDefaults` operates on a COPY via
      `copyAuthConfigForScan` so Category B invocations cannot leak into
      Category A reuses of the same `atmosConfig.Auth`.
- [x] `cmd/identity_flag.go` — added `CreateAuthManagerFromIdentityWithStackScan`
      wrapper. `CreateAuthManagerFromIdentityWithAtmosConfig` remains NO-SCAN.
- [x] Category B callers routed to the scan variant:
      `cmd/describe_stacks.go`, `cmd/describe_affected.go`,
      `cmd/describe_dependents.go`, `cmd/list/utils.go`,
      `pkg/list/list_affected.go`, and `pkg/auth/manager_env_overrides.go`
      (MCP scoped auth). `cmd/describe_component.go` kept on the NO-SCAN
      wrapper (Category A). `aws security` / `aws compliance` untouched —
      they bail out when `identityName == ""` so the scan is a no-op.
- [x] `internal/exec/workflow_utils.go:checkAndMergeDefaultIdentity` —
      unchanged on this branch. Already calls `config.LoadStackAuthDefaults`
      directly, and now gets the import-following benefit for free.
- [x] Scenario coverage for Issue #2293 via `describe stacks` added to
      `tests/test-cases/auth-identity-resolution-bugs.yaml`. Uses the
      `auth-imported-defaults/` fixture and exercises the new scan variant
      end-to-end.
- [x] Go unit tests: 12 new scanner tests in
      `pkg/config/stack_auth_loader_test.go`, 5 new auth-helper regression
      tests in `pkg/auth/manager_helpers_test.go`, plus isolation tests for
      `copyAuthConfigForScan`.
- [x] Full regression suite passes: `pkg/auth/`, `pkg/config/`,
      `internal/exec/`, `cmd/`, `pkg/list/`, `cmd/list/`, CLI scenarios
      under `tests/`.

---

## Relationship summary (TL;DR)

**Both issues share the same root cause.** They originate from
`pkg/config/stack_auth_loader.go:LoadStackAuthDefaults`, a shortcut that
scans every stack file globally *before* the target stack is known and tries
to discover "the" default identity from raw YAML parsing. That shortcut is
structurally wrong in two ways:

1. It reads files with `yaml.Unmarshal` against a minimal struct — it does
   **not** follow `import:` directives, so any `auth:` block declared in an
   imported `_defaults.yaml` is invisible (**#2293**). In a typical Cloud
   Posse layout `_defaults.yaml` files are even listed under
   `stacks.excluded_paths`, so they never reach the glob at all.

2. It aggregates every `default: true` flag it finds into a **global** pool
   and applies the result to every subsequent command regardless of target
   stack. The existing "conflict detection" only handles the case where
   *multiple different* identities are flagged as default; if exactly one
   stack file declares a default, that default silently leaks to every
   other stack, OU, and tenant (**Discussion #122**).

---

## Issue 1 — Default identity declared in an imported stack file is not recognized

**Source:** [#2293](https://github.com/cloudposse/atmos/issues/2293)

### Problem

When `auth.identities.<name>.default: true` is declared in an imported stack
file (for example `_defaults.yaml` that a stack manifest imports via
`import:`), Atmos does not pick it up during the pre-scanner identity
resolution. Instead, Atmos prompts the user to select an identity
interactively even though a default is configured — and in non-interactive
contexts this surfaces as "no default identity configured."

### Reproduction

Defaults file declaring the default identity:

```yaml
# stacks/orgs/acme/dev/_defaults.yaml
import:
  - ../_defaults

vars:
  stage: dev

auth:
  identities:
    acme-dev:
      default: true
```

Stack manifest that imports it:

```yaml
# stacks/orgs/acme/dev/us-east-1/foundation.yaml
import:
  - ../_defaults
  - mixins/region/us-east-1
```

Running any component command in that stack with the debug log enabled
shows the pre-scanner missing the imported defaults file:

```text
DEBU  Loading stack configs for auth identity defaults
DEBU  Loading stack files for auth defaults count=16
DEBU  No default identities found in stack configs
```

### Expected behavior

Atmos should resolve the default identity from the **merged** stack config,
honoring the same `import:` / `_defaults.yaml` inheritance semantics that
`vars:` and `components:` already obey. Placing `auth:` in a defaults file
should not require duplication in every manifest that imports it.

---

## Issue 2 — Default identity declared in one stack manifest leaks to all stacks across all OUs

**Source:** [Discussion #122 — "Auth inheritance not scoping to stack"](https://github.com/orgs/cloudposse/discussions/122)

### Problem

When a stack manifest declares:

```yaml
auth:
  identities:
    <org>-<tenant>/terraform:
      default: true
```

…that `default: true` assignment is treated as **global** rather than
scoped to the stack that declared it. Every subsequent `atmos terraform
plan/apply` invocation — regardless of which stack the user selects —
loads that same identity as the default, even for stacks in a completely
different OU, tenant, or environment.

The user tested this across Atmos `1.210`, `1.211`, and `1.213`; the
behavior reproduces on all three.

### Reproduction

1. Stack tree with multiple tenants, e.g.

   ```text
   stacks/orgs/acme/
     data/staging/us-east-1/monitoring-agent.yaml
     plat/staging/us-east-1/eks-cluster.yaml
     plat/prod/us-east-1/eks-cluster.yaml
   ```

2. Add a default-identity `auth:` block to **one** manifest, e.g.

   ```yaml
   # stacks/orgs/acme/data/staging/us-east-1/monitoring-agent.yaml
   auth:
     identities:
       data-staging/terraform:
         default: true
   ```

3. Run any terraform command against a **different** stack:

   ```bash
   $ atmos terraform plan eks/test-eks-agent -s plat-use1-staging
   ```

4. Atmos loads the `data-staging/terraform` identity for the
   `plat-use1-staging` stack command. Debug output (trimmed):

   ```text
   Found component 'eks/test-eks-agent' in the stack 'plat-use1-staging'
     in the stack manifest 'orgs/acme/plat/staging/us-east-1/monitoring-test'
   CreateAndAuthenticateManager called identityName="" hasAuthConfig=true
   Loading stack configs for auth identity defaults
   Loading stack files for auth defaults count=284
   Found default identity in stack config identity=data-staging/terraform
     file=/…/stacks/orgs/acme/data/staging/us-east-1/monitoring-test.yaml
   ```

   The file Atmos picks the default from belongs to a completely
   unrelated stack (`data-staging` vs the requested `plat-use1-staging`).

### Expected behavior

`default: true` under `auth.identities.<name>` should only apply to the
stack(s) that actually import or declare that `auth:` block. Unrelated
stacks in other OUs, tenants, or environments should be unaffected.

---

## Root Cause Analysis

### Where the bug lives

- `pkg/config/stack_auth_loader.go` — the `LoadStackAuthDefaults` function
  and its helpers `getAllStackFiles` / `loadFileForAuthDefaults`.
- `pkg/auth/manager_helpers.go` — `loadAndMergeStackAuthDefaults` was the
  caller: inside `CreateAndAuthenticateManagerWithAtmosConfig`, when
  `identityName == ""` and auth was configured, it called out to the loader
  to "discover the default" before the target stack was known.

### Why it fails

`LoadStackAuthDefaults` does a raw `yaml.Unmarshal` against a minimal
struct that contains only the top-level `auth:` section. There is no
import-following, no template processing, no deep-merge with imports.

```go
stackFiles := getAllStackFiles(
    atmosConfig.IncludeStackAbsolutePaths,
    atmosConfig.ExcludeStackAbsolutePaths,
)
for _, filePath := range stackFiles {
    fileDefaults, err := loadFileForAuthDefaults(filePath)   // raw yaml.Unmarshal
    for identity, isDefault := range fileDefaults {
        if isDefault {
            allDefaults = append(allDefaults, defaultSource{identity, filePath})
        }
    }
}
firstIdentity := allDefaults[0].identity
allAgree := true
for _, d := range allDefaults[1:] {
    if d.identity != firstIdentity {
        allAgree = false
        break
    }
}
if allAgree {
    defaults[firstIdentity] = true   // <-- global default applied here
}
```

**Why Issue 1 fails:** `_defaults.yaml` is typically listed in
`stacks.excluded_paths` because those files are meant to be **imported** by
stack manifests, not processed as standalone stacks. As a result,
`getAllStackFiles` filters them out entirely — their `auth:` block is never
even seen by the raw YAML parser. Even if a `_defaults.yaml` file is not
excluded, `loadFileForAuthDefaults` does not follow its `import:`
directive.

**Why Issue 2 fails:** The conflict-detection loop only handles *different*
default identities colliding across stack files. When only one stack
declares a default, the loop over `allDefaults[1:]` is empty, `allAgree`
stays true, and the identity is added to a global map. `MergeStackAuthDefaults`
then clears any existing defaults in the merged auth config and applies
this global one, so every command in the repo resolves to the leaked
identity as the default.

### Why the approach is structurally wrong

The code comment above `CreateAndAuthenticateManagerWithAtmosConfig` framed
this as a chicken-and-egg problem:

> - We need to know the default identity to authenticate
> - But stack configs are only loaded after authentication is configured

This is **not** actually a chicken-and-egg. For every command where a
stack-scoped default identity could matter (`atmos terraform *`,
`atmos helmfile *`, etc.), the target stack is either:

1. Passed explicitly on the command line via `-s <stack>`, OR
2. Interactively selected by the user, OR
3. Derived from the `atmos.yaml` `stacks.name_pattern` /
   `stacks.name_template` after component/context resolution.

In every case the target stack is knowable **before** the auth manager
needs to exist. The only exception is `atmos auth login` (or `auth whoami`
/ `auth env` / etc.) invoked with no stack context, in which case the only
meaningful default is the `atmos.yaml`-level default — no stack scan needed.

So the correct layering is:

```text
1. Parse CLI args → know the target stack (or know there isn't one).
2. If there is a target stack:
      load its merged config through the normal stack processor
      (which correctly follows import: chains and honors stack scope)
   Else:
      use the atmos.yaml auth defaults only.
3. Resolve the default identity from the merged auth config (global ∪ stack ∪ component).
4. Build and authenticate the auth manager.
```

The pre-scanner tried to compress steps 1-3 into a single pre-stack scan
and in doing so broke the scoping model entirely.

---

## Caller audit — who calls the auth manager, and how?

Before picking a fix, we categorized every call site of
`CreateAndAuthenticateManagerWithAtmosConfig` (and the thin
`cmd/identity_flag.go:CreateAuthManagerFromIdentityWithAtmosConfig` wrapper
that sits in front of it). This matters because the right fix depends on
whether the caller already has a specific target stack or not.

### Category A — target-stack callers (exec-layer merge owns identity resolution)

These commands have a specific `(component, stack)` pair known *before*
auth manager creation. They go through
`internal/exec/utils_auth.go:getMergedAuthConfigWithFetcher` →
`ExecuteDescribeComponent`, which correctly follows `import:` chains,
template rendering, and deep merge against that specific stack:

- `atmos terraform *` (all subcommands) — `internal/exec/utils_auth.go`
- `atmos helmfile *`
- `atmos terraform query` — `internal/exec/terraform_query.go`
- Nested component auth — `internal/exec/terraform_nested_auth_helper.go`
- `atmos describe component` — `cmd/describe_component.go`

For Category A callers, the pre-scanner is **actively harmful**: it can
only contaminate an already-correct merged config. This is the mechanism
behind Discussion #122's cross-stack leak.

### Category B — multi-stack callers (no specific target stack)

These commands legitimately operate across many stacks/components and have
no single `(component, stack)` pair upfront. They were *intentionally*
designed to use the pre-scanner as "Approach 2" in
`docs/fixes/stack-level-default-auth-identity.md`:

- `atmos describe stacks` — `cmd/describe_stacks.go:80`
  (gated on `ProcessYamlFunctions || identityExplicit`)
- `atmos describe affected` — `cmd/describe_affected.go:87`
  (same gate; plus 2026-03-25 fix threads the AuthManager through the
  entire affected/graph pipeline)
- `atmos describe dependents` — `cmd/describe_dependents.go`
- `atmos list affected` — `pkg/list/list_affected.go`
  (cmd/list/affected.go reads `--identity`; 2026-03-25 fix Bug 4)
- `atmos list instances`
- `atmos list <various>` — `cmd/list/utils.go:createAuthManagerForList`
  (comment literally says *"it loads stack configs for default identity"*)
- `atmos aws security` — `cmd/aws/security/security.go`
- `atmos aws compliance` — `cmd/aws/compliance/compliance.go`
- Workflow execution — `internal/exec/workflow_utils.go:checkAndMergeDefaultIdentity`

For Category B callers, removing the pre-scanner entirely is a **real
regression**. These commands have no target stack to scope against, so
their only options are (1) scan all stacks for a unanimous default
(today's pre-scanner behavior, already hardened against conflicts by
Issue #2072), or (2) refuse to resolve a default identity for any
non-`atmos.yaml`-level config.

### Category C — no atmosConfig (never used the pre-scanner)

These commands call the simpler `CreateAndAuthenticateManager` variant
without passing `atmosConfig`, so they never ran the pre-scanner to begin
with. Unaffected by every option below:

- `cmd/terraform/backend/backend_helpers.go`
- `cmd/terraform/utils.go`
- `cmd/identity_flag.go` (simpler variant)
- `cmd/list/sources.go`
- `pkg/provisioner/source/cmd/helpers.go`

---

## Options considered

Four options were evaluated against two hard constraints:

1. **Must fix Issue #2293 and Discussion #122** across all commands where
   they were reported.
2. **Must not regress any existing auth functionality.** Atmos has a long
   history of auth fixes (`docs/fixes/stack-level-default-auth-identity.md`,
   `2026-02-12-auth-realm-isolation-issues.md`,
   `2026-03-25-describe-affected-auth-identity-not-used.md`, etc.) — any
   behavior change that breaks existing Category B users would undo that
   work.

### Option (a) — Remove pre-scanner entirely. ❌ Rejected.

The initial draft of this fix deleted all pre-scanner calls from the auth
flow. It cleanly fixes both bugs for Category A but **regresses every
Category B command**. In particular:

- `atmos describe stacks` / `describe affected` / `list affected` /
  `list instances` / `aws security` / `aws compliance` / workflows would
  silently lose the ability to resolve stack-level default identities.
- The 2026-03-25 describe-affected fix that went to great lengths to
  thread an AuthManager through the entire affected/graph pipeline would
  stop working when users relied on stack-level defaults.
- Users with a single-tenant repo who declared `default: true` in a stack
  manifest would start hitting "no default identity" errors on multi-stack
  commands.

**Verdict:** unacceptable per constraint (2). Rejected even though it was
the smallest diff.

### Option (b) — Conditional fallback in a single helper. ❌ Insufficient.

Keep a single `CreateAndAuthenticateManagerWithAtmosConfig` helper, and
run the pre-scanner as a fallback only when both:

1. `identityName == ""`, AND
2. The incoming `authConfig` already has no default identity flagged.

**Why it fails:** the gating signal "authConfig has no default" cannot
distinguish the two cases that need opposite treatment:

- **Category A, target stack with no auth block** (e.g.,
  `atmos terraform plan eks -s plat-staging`): merged authConfig has no
  default — **correctly**, because `plat-staging` truly has no auth. The
  fallback should NOT fire.
- **Category B, no target stack** (e.g., `atmos describe stacks`):
  authConfig has no default because no per-stack merge happened. The
  fallback SHOULD fire.

Both look identical to the helper, so any single-helper fallback either
leaks (Discussion #122 reproduced against `plat-staging`) or regresses
Category B. Rejected.

### Option (c) — Lazy / deferred auth manager creation. ⚠️ Too big for this PR.

Create an "unresolved" auth manager wrapper and defer real resolution
until after the first stack processing pass. For `describe stacks` etc.
this is correct because the auth manager is only actually *used* when
YAML functions are evaluated — which happens after stack processing.

**Why not now:** this requires threading a lazy-AuthManager abstraction
through every callsite that currently expects a ready-to-use manager, plus
careful design of error reporting (what if identity resolution fails
mid-stack-processing?). It is the cleanest long-term architecture but out
of scope for a point fix. Tracked as a follow-up.

### Option (d+) — Split entry points + import-following scanner. ✅ Chosen.

Two changes, neither of which removes any existing behavior:

1. **Split the pkg/auth entry points.** Introduce
   `CreateAndAuthenticateManagerWithStackScan` alongside the existing
   `CreateAndAuthenticateManagerWithAtmosConfig`. The scan variant is a
   thin wrapper that runs the pre-scanner first, then delegates to the
   no-scan variant. Category A callers use the no-scan variant (no more
   contamination of their merged config). Category B callers use the
   scan variant (existing Approach 2 behavior preserved exactly).

2. **Teach the pre-scanner to follow `import:` chains.** Rewrite
   `pkg/config/stack_auth_loader.go` so `LoadStackAuthDefaults`
   recursively reads imported files' `auth:` sections — including
   `_defaults.yaml` files listed in `excluded_paths`, because the
   `excluded_paths` filter only prevents files from being processed as
   standalone stacks, not from being imported. Uses the same
   `allAgree` conflict-detection logic introduced by Issue #2072, so
   conflicting defaults across stacks are still discarded.

The split fully isolates Category A from Discussion #122. The
import-following scanner fully fixes Issue #2293 for Category B (and
reinforces Category A, which was already fixed via the exec-layer merge).

---

## Coverage matrix

| Scenario                                                                                           | Pre-fix main                  | Option (a)         | Option (b)                  | **Option (d+)**                     |
|----------------------------------------------------------------------------------------------------|-------------------------------|--------------------|-----------------------------|-------------------------------------|
| **#2293** imported default, `terraform plan -s acme-dev` (Category A)                              | ❌ broken                      | ✅ exec-layer merge | ✅ exec-layer merge          | ✅ exec-layer merge                  |
| **#2293** imported default, `describe stacks` / `describe affected` (Category B)                   | ❌ broken                      | ❌ broken           | ❌ broken                    | ✅ **scanner follows imports**       |
| **#2293** imported default, `list affected` / workflows / `aws security` (Category B)              | ❌ broken                      | ❌ broken           | ❌ broken                    | ✅ **scanner follows imports**       |
| **#122** single-stack default leaks to `terraform plan -s other` (Category A)                      | ❌ leak                        | ✅ scanner removed  | ❌ leak still (gating fails) | ✅ Category A never runs scanner     |
| **#122** repo-wide consistent default, `describe stacks` (Category B)                              | ✅ works (scanner picks it up) | ❌ regression       | ✅ works                     | ✅ works (scanner still runs)        |
| Existing `describe stacks` / `describe affected` / `list affected` multi-stack identity resolution | ✅ works                       | ❌ **regression**   | ❌ **regression**            | ✅ preserved bit-for-bit             |
| 2026-03-25 describe-affected AuthManager threading (Bugs 1-4)                                      | ✅ works                       | ❌ regression       | ❌ regression                | ✅ preserved                         |
| Issue #2072 conflicting-defaults discard across stacks                                             | ✅ works                       | N/A (scanner gone) | ✅ works                     | ✅ preserved (same `allAgree` logic) |
| Workflow execution stack-level default loading                                                     | ✅ works                       | ❌ regression       | ❌ regression                | ✅ restored                          |

---

## Fix — Option (d+) implementation

### 1. Teach the scanner to follow imports

**`pkg/config/stack_auth_loader.go`** — replace the flat per-file
`loadFileForAuthDefaults` with a recursive `loadAuthWithImports` that:

- Reads each stack file's top-level `import:` list and `auth:` block via
  a minimal `yaml.Unmarshal` (no template rendering, no full stack
  processing).
- For each import entry, resolves it to absolute file paths, handling the
  three Atmos import forms: plain string, glob string, map form with
  `path`. Imports are resolved relative to the importing file first, then
  relative to `atmosConfig.StacksBaseAbsolutePath`. Adds `.yaml` extension
  if missing.
- Recursively loads imported files — crucially, **including files inside
  `excluded_paths`** when referenced via `import:`. The `excluded_paths`
  filter only prevents direct processing as a standalone stack; imports
  must still resolve them.
- Deep-merges imported auth sections with the current file's auth
  section, with the current file winning on conflict (matches Atmos
  import semantics).
- Uses a `visited` map for cycle protection.
- Templated imports whose path cannot be resolved without running Go
  templates fall back to being skipped (same graceful-degrade behavior as
  today). Document the limitation; rare in practice.

`LoadStackAuthDefaults` then aggregates results across all top-level
stack files and keeps the existing `allAgree` conflict-detection logic
from Issue #2072 unchanged.

### 2. Split the auth manager entry points

**`pkg/auth/manager_helpers.go`**

- `CreateAndAuthenticateManagerWithAtmosConfig` (existing name, reworked
  body): **does not run the pre-scanner.** Takes an already-merged
  `authConfig` and resolves an identity from it. This is what Category A
  callers want: their `authConfig` was already merged by
  `ExecuteDescribeComponent` against the correct target stack, and
  running the scanner on top would only reintroduce the Discussion #122
  leak.
- `CreateAndAuthenticateManagerWithStackScan` (**new**): thin wrapper
  that, when `identityName == ""`, calls
  `config.LoadStackAuthDefaults` + `config.MergeStackAuthDefaults` on a
  *copy* of `authConfig`, then delegates to
  `CreateAndAuthenticateManagerWithAtmosConfig`. This preserves the
  Approach 2 pre-scan behavior for Category B callers.

Both helpers share identical signatures; switching between them is a
single-word change at each call site.

### 3. Route callers

- **Category A** — no change. Already uses
  `CreateAndAuthenticateManagerWithAtmosConfig` (no scan).
- **Category B** — switch to `CreateAndAuthenticateManagerWithStackScan`
  (or, for most of them, update the thin wrapper they go through):
  - `cmd/identity_flag.go:CreateAuthManagerFromIdentityWithAtmosConfig`
    → switch its inner call to the scan variant. This single change
    auto-updates `describe stacks`, `describe affected`, and
    `describe dependents` because they all go through the wrapper.
  - `cmd/list/utils.go:createAuthManagerForList` → scan variant.
  - `pkg/list/list_affected.go` → scan variant.
  - `cmd/list/instances.go` → scan variant (if it currently calls the
    auth helper).
  - `cmd/aws/security/security.go` → scan variant.
  - `cmd/aws/compliance/compliance.go` → scan variant.
- **`internal/exec/workflow_utils.go:checkAndMergeDefaultIdentity`** —
  restore the `config.LoadStackAuthDefaults` +
  `config.MergeStackAuthDefaults` calls that the earlier draft
  simplified away. This is equivalent to calling the scan variant and
  matches the pre-fix behavior exactly.

### 4. Fixed flows

**Category A — `atmos terraform plan my-component -s acme-dev` with imported `_defaults.yaml`:**

```text
1. ExecuteTerraform → createAndAuthenticateAuthManager(atmosConfig, info)
2. getMergedAuthConfigWithFetcher(atmosConfig, info, …)
   → ExecuteDescribeComponent (follows import: chains correctly)
   → mergedAuthConfig carries `dev-identity.default: true` from the
     imported _defaults.yaml — correctly scoped to this stack only.
3. CreateAndAuthenticateManagerWithAtmosConfig(
     identityName="", mergedAuthConfig, …)  ← NO-SCAN variant.
   → no pre-scanner clobbering the merged config.
   → resolveIdentityName finds the default from mergedAuthConfig.
4. Auth manager constructed with dev-identity. No leak into other stacks.
```

**Category A — `atmos terraform plan eks -s plat-staging` against unrelated stack:**

```text
1. getMergedAuthConfigWithFetcher returns mergedAuthConfig with NO default
   (plat-staging's stack has no auth block; global atmos.yaml has no
   default).
2. resolveIdentityName returns the empty string → no auth.
   → no cross-stack leak, because the NO-SCAN variant never consults any
     other stack file.
```

**Category B — `atmos describe stacks` with imported `_defaults.yaml`:**

```text
1. cmd/describe_stacks.go → CreateAuthManagerFromIdentityWithAtmosConfig
   → CreateAndAuthenticateManagerWithStackScan(identityName="", …)
2. Scan variant calls LoadStackAuthDefaults:
   → scanner walks each top-level stack file, recursively following
     `import:` entries (including into `_defaults.yaml` files in
     excluded_paths).
   → merged per-file auth section contains `dev-identity.default: true`.
   → allAgree across stacks → applied to merged authConfig.
3. Delegates to no-scan variant with the populated authConfig.
4. Auth manager constructed with dev-identity. Approach 2 behavior
   preserved; Issue #2293 now fixed for this command too.
```

**Category B — `atmos describe stacks` with conflicting defaults across stacks:**

```text
1. Scanner finds two DIFFERENT identities flagged `default: true`.
2. allAgree → false → defaults map returned empty.
3. Approach 2 falls back to `atmos.yaml`-level default only.
4. Issue #2072 behavior preserved exactly.
```

---

## Test fixtures and regression tests

All fixtures use `mock/aws` identities so they authenticate end-to-end in
CI without real cloud credentials.

### `tests/fixtures/scenarios/auth-imported-defaults/` — Issue #2293

Mirrors the real-world Cloud Posse reference architecture layout:

```text
atmos.yaml                                        # name_template,
                                                  # excluded_paths: ['**/_defaults.yaml']
stacks/orgs/acme/dev/
├── _defaults.yaml                                # auth.identities.dev-identity.default: true
└── us-east-1/foundation.yaml                     # import: orgs/acme/dev/_defaults
```

The key detail: `_defaults.yaml` is listed under `stacks.excluded_paths`,
so `getAllStackFiles` filters it out before the raw-YAML pre-scanner ever
sees it. The stack name resolves to `acme-dev` via
`name_template: "{{ .vars.tenant }}-{{ .vars.stage }}"`.

### `tests/fixtures/scenarios/auth-stack-scoping/` — Discussion #122

Two unrelated stacks under `stacks/orgs/acme/`, using tenant names `data`
and `plat` (acme is a placeholder; real customer namespace is redacted):

```text
atmos.yaml                                        # NO global default
stacks/orgs/acme/
├── data/staging/us-east-1/monitoring.yaml        # auth.identities.data-default.default: true
└── plat/staging/us-east-1/eks.yaml               # NO auth block at all
```

Stack names resolve to `data-staging` and `plat-staging` respectively.

### CLI regression tests

`tests/test-cases/auth-identity-resolution-bugs.yaml` wires the fixtures
into assertions that guard both the Category A exec-layer merge path AND
the Category B scan-variant path:

- **`describe component -s acme-dev`** (Category A) asserts the imported
  `_defaults.yaml` default is present in the merged output.
- **`describe stacks -s acme-dev --process-functions=true`** (Category B)
  asserts the scan variant surfaces the imported default — new coverage
  for #2293 against multi-stack commands.
- **`describe component -s data-staging`** asserts data-staging sees its
  own default identity (happy path).
- **`describe component -s plat-staging`** asserts plat-staging does NOT
  inherit data-staging's default (the Category A non-leak — Discussion
  #122).
- **`describe stacks` across the auth-stack-scoping fixture** asserts the
  scan variant still behaves correctly under the `allAgree` conflict
  logic (Issue #2072 preserved).

### Go unit tests

**`pkg/config/stack_auth_loader_test.go`** (new / updated)

- `TestLoadStackAuthDefaults_FollowsImports` — stack file imports a
  `_defaults.yaml` that declares `default: true`; scanner must see it.
- `TestLoadStackAuthDefaults_FollowsImportsFromExcludedPath` — same as
  above but `_defaults.yaml` is listed in `excluded_paths`. Scanner must
  still follow the import (excluded_paths only filters standalone
  processing, not import resolution).
- `TestLoadStackAuthDefaults_ImportCycleProtection` — two files that
  import each other; scanner must terminate and return a sensible result.
- `TestLoadStackAuthDefaults_GlobImports` — `import:` list contains a
  glob; scanner must expand.
- `TestLoadStackAuthDefaults_TemplatedImportSkipped` — map-form import
  with a Go-template path; scanner must skip gracefully (same as today).
- `TestLoadStackAuthDefaults_ConflictingDefaultsDiscarded` — preserves
  Issue #2072 `allAgree` behavior unchanged.
- `TestLoadStackAuthDefaults_CurrentFileWinsOverImport` — when both the
  importing file and the imported file declare defaults, the importing
  file's default takes precedence.

**`pkg/auth/manager_helpers_test.go`** (new regression tests)

Category A (no-scan variant):

- `TestCreateAndAuthenticateManagerWithAtmosConfig_HonorsMergedConfigDefault`
  — when the merged `authConfig` already carries a default (produced by
  the exec-layer stack processor for a target stack that imports
  `_defaults.yaml`), that default is resolved correctly.
- `TestCreateAndAuthenticateManagerWithAtmosConfig_DoesNotLeakCrossStackDefault`
  — when the merged `authConfig` has NO default (simulating a target
  stack like `plat-staging` with no auth block), the no-scan variant
  never consults unrelated stack files; no leak possible.
- `TestCreateAndAuthenticateManagerWithAtmosConfig_IgnoresStackFilesWithLeakingDefault`
  — end-to-end: a real stack file on disk declares `default: true`; the
  no-scan variant must not pick it up even with a full `atmosConfig`.
- `TestCreateAndAuthenticateManagerWithAtmosConfig_ExplicitIdentityNotOverriddenByStackFiles`
  — `--identity` flag passed explicitly wins over any stack file
  defaults.

Category B (scan variant):

- `TestCreateAndAuthenticateManagerWithStackScan_PicksUpImportedDefault`
  — scan variant finds an imported `_defaults.yaml` default that the
  no-scan variant cannot see. Primary Category B #2293 test.
- `TestCreateAndAuthenticateManagerWithStackScan_HonorsExplicitIdentity`
  — explicit `identityName` bypasses the scan.
- `TestCreateAndAuthenticateManagerWithStackScan_DiscardsConflictingDefaults`
  — two stacks declare different defaults; scan returns empty; falls
  back to `atmos.yaml`-level default. Issue #2072 preserved.
- `TestCreateAndAuthenticateManagerWithStackScan_NoMutationOfInputConfig`
  — scan variant must operate on a copy of `authConfig`; caller's
  original must remain untouched.

**`internal/exec/workflow_utils_test.go`** (restored)

- Restore the stack-loading tests that the earlier draft deleted:
  `TestCheckAndMergeDefaultIdentity_WithStackLoading`, `_LoadError`,
  `_LoadErrorNoDefault`, `_StackNoDefaults`, `_EmptyStackDefaults`. The
  function's pre-fix behavior is restored, so these tests become valid
  again.

### Regression run (required before merge)

```text
go test ./pkg/auth/ ./pkg/config/ ./internal/exec/ -count=1        → all PASS
go test ./tests -run 'TestCLICommands/atmos_describe'              → all PASS
go test ./tests -run 'TestCLICommands/atmos_list'                  → all PASS
go test ./pkg/auth/ ./pkg/config/ -count=1 -race                   → PASS
```

Special attention to:

- `TestCLICommands/describe_affected_*` — 2026-03-25 fix's end-to-end
  coverage for AuthManager threading through the affected pipeline.
- `TestCLICommands/list_affected_*` — 2026-03-25 fix Bug 4.
- Workflow tests that exercise `checkAndMergeDefaultIdentity`.
- `pkg/config/auth_realm_issues_test.go` — Issue #2072 conflicting
  defaults coverage.

---

## Related

- `docs/fixes/stack-level-default-auth-identity.md` — the original
  Approach 1 / Approach 2 design that this fix preserves and extends.
  Approach 2 is the multi-stack scanner code path that option (d+) keeps
  alive for Category B commands.
- `docs/fixes/2026-02-12-auth-realm-isolation-issues.md` — Issue #2072
  introduced the `allAgree` conflict-detection logic in the scanner.
  Option (d+) preserves that logic unchanged.
- `docs/fixes/2026-03-25-describe-affected-auth-identity-not-used.md` —
  threaded an AuthManager through the entire describe-affected pipeline.
  Option (d+) preserves that plumbing; Category B callers still get a
  working AuthManager with stack-level defaults resolved via the scan
  variant.
- `docs/fixes/2026-04-06-mcp-server-env-not-applied-to-auth-setup.md` —
  another auth-context propagation fix in a different code path.
