# Fix: Atmos Auth stack-level default identity resolution

**Date:** 2026-04-08

**Issues:**
- [#2293](https://github.com/cloudposse/atmos/issues/2293) — `auth.identities.<name>.default: true` in imported stack files not recognized during identity resolution
- [Discussion #122](https://github.com/orgs/cloudposse/discussions/122) — Auth inheritance not scoping to stack (a default identity declared in one stack manifest leaks to every other stack across all OUs)

## Status

**Two related bugs fixed.** Both originate from the same global raw-YAML
pre-scanner in `pkg/config/stack_auth_loader.go`. The fix is to remove the
pre-scanner call from the auth flow; the exec-layer stack processor already
merges auth defaults correctly against the target stack.

### Progress checklist

- [x] Root-cause analysis.
- [x] Two scenario fixtures under `tests/fixtures/scenarios/` using
      `mock/aws`.
- [x] CLI regression test cases in
      `tests/test-cases/auth-identity-resolution-bugs.yaml`.
- [x] Go unit tests at the function boundary and the auth-manager boundary.
- [x] Fix landed: removed the global raw-YAML pre-scanner call from the
      auth flow.
- [x] Full regression suite passes.

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

## Fix

### 1. Remove the pre-scanner call from the auth flow

**`pkg/auth/manager_helpers.go`**

- Deleted the `loadAndMergeStackAuthDefaults` helper entirely.
- Deleted the call site in `CreateAndAuthenticateManagerWithAtmosConfig`
  and replaced it with a comment explaining the decision and pointing at
  this doc.
- `CreateAndAuthenticateManagerWithAtmosConfig` now passes the incoming
  `authConfig` through to `resolveIdentityName` unchanged. Callers that
  need stack-scoped defaults (all exec-layer paths) have already had their
  `authConfig` merged correctly via
  `internal/exec/utils_auth.go:getMergedAuthConfigWithFetcher` →
  `ExecuteDescribeComponent`, which correctly follows `import:` chains
  against the specific target stack.

**`internal/exec/workflow_utils.go`**

- `checkAndMergeDefaultIdentity` simplified — no longer calls
  `config.LoadStackAuthDefaults` / `config.MergeStackAuthDefaults`. It only
  inspects `atmosConfig.Auth.Identities` for `Default: true`, relying on
  the exec-layer stack processor to have populated that map with the
  target stack's merged auth config.

### 2. Keep the loader functions in place (dead-code tolerant)

**`pkg/config/stack_auth_loader.go`**

- **Kept unchanged.** The `LoadStackAuthDefaults` and
  `MergeStackAuthDefaults` functions are now unreachable from the auth
  flow but remain available for any future caller that explicitly opts in
  (e.g. a lightweight docs generator or a tooling command that wants the
  global map without a target-stack context). Their tests in
  `pkg/config/auth_defaults_test.go` are re-annotated as **characterization
  tests** that document the function's standalone behavior.

### Fixed flow

For a command like `atmos terraform plan my-component -s acme-dev`:

```text
1. ExecuteTerraform → createAndAuthenticateAuthManager(atmosConfig, info)
2. getMergedAuthConfigWithFetcher(atmosConfig, info, …)
   → ExecuteDescribeComponent (follows import: chains correctly)
   → mergedAuthConfig carries `dev-identity.default: true` from the
     imported _defaults.yaml — correctly scoped to this stack only.
3. CreateAndAuthenticateManagerWithAtmosConfig(
     identityName="", mergedAuthConfig, …)
   → no more pre-scanner clobbering the merged config.
   → resolveIdentityName finds the default from mergedAuthConfig.
4. Auth manager constructed with dev-identity. No leak into other stacks.
```

For a command against an unrelated stack like
`atmos terraform plan eks -s plat-staging`:

```text
1. getMergedAuthConfigWithFetcher returns mergedAuthConfig with NO default
   (plat-staging's stack has no auth block, global atmos.yaml has no
   default).
2. resolveIdentityName returns the empty string → no auth.
   → no more cross-stack leak.
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
into three describe-component assertions that guard the exec-layer merge's
stack scoping — the exact behavior that the pre-scanner removal relies on.

### Go unit tests

**`pkg/config/auth_defaults_test.go`**

- `TestLoadStackAuthDefaults_ExcludedDefaultsFile` — characterizes the
  pre-scanner's inability to see auth blocks inside excluded
  `_defaults.yaml` files. Documents the function's standalone behavior;
  the auth flow no longer consumes this result.
- `TestLoadStackAuthDefaults_SingleDefaultIsReturnedGlobally` —
  characterizes the pre-scanner's global flattening of a single stack's
  default.

**`pkg/auth/manager_helpers_test.go`** (new regression tests)

- `TestCreateAndAuthenticateManagerWithAtmosConfig_HonorsMergedConfigDefault`
  — when the merged `authConfig` already carries a default (as produced by
  the exec-layer stack processor for a target stack that imports
  `_defaults.yaml`), that default is resolved correctly and the
  pre-scanner no longer runs to clobber it.
- `TestCreateAndAuthenticateManagerWithAtmosConfig_DoesNotLeakCrossStackDefault`
  — when the merged `authConfig` has NO default (simulating a target stack
  like `plat-staging` with no auth block), the auth flow no longer
  inherits a default from an unrelated stack file elsewhere in the repo.

**`pkg/auth/manager_helpers_test.go`** (tests deleted)

- 4 obsolete tests for the removed `loadAndMergeStackAuthDefaults` helper
  (`TestLoadAndMergeStackAuthDefaults_*`).

**`internal/exec/workflow_utils_test.go`** (tests deleted)

- 4 obsolete stack-loading tests for `checkAndMergeDefaultIdentity`
  (`_WithStackLoading`, `_LoadError`, `_LoadErrorNoDefault`,
  `_StackNoDefaults`, `_EmptyStackDefaults`) — the function no longer
  performs stack loading.

### Regression run

```text
go test ./pkg/auth/ ./pkg/config/ ./internal/exec/ -count=1        → all PASS
go test ./tests -run 'TestCLICommands/atmos_describe_component_—'  → all PASS
go test ./pkg/auth/ ./pkg/config/ -count=1 -race                   → PASS
```

The `internal/exec` race run exceeds the 300s budget on a local laptop due
to pre-existing heavy tests; it is not related to this fix. CI will
exercise it fully.

---

## Related

- `docs/fixes/2026-04-06-mcp-server-env-not-applied-to-auth-setup.md` —
  another auth-context propagation fix in a different code path.
- `docs/fixes/2026-03-25-describe-affected-auth-identity-not-used.md` —
  previous auth-identity propagation fix.
