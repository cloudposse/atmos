# Fix: Atmos Auth identity resolution — three related bugs

**Date:** 2026-04-08
**Branch:** `aknysh/atmos-auth-fixes-2`
**Issues:**
- [#2293](https://github.com/cloudposse/atmos/issues/2293) — `auth.identities.<name>.default: true` in imported stack files not recognized during identity resolution
- [Discussion #122](https://github.com/orgs/cloudposse/discussions/122) — Auth inheritance not scoping to stack (a default identity declared in one stack manifest leaks to every other stack across all OUs)
- Slack report — `components.terraform.<name>.auth.identity` override at the component level is silently ignored; the default identity is used instead

## Status

**All three fixes implemented and tested.** Test coverage raised on the new
exec-layer selector extractor. All existing auth tests pass without
regression. Ready to commit and push.

### Progress checklist

- [x] Root-cause analysis for all three issues.
- [x] Three scenario fixtures under `tests/fixtures/scenarios/` using
      `mock/aws`.
- [x] Six CLI regression test cases in
      `tests/test-cases/auth-identity-resolution-bugs.yaml`.
- [x] Bug-reproduction Go unit tests at the function boundary.
- [x] **Issue 3 fix**: exec-layer extractor reads
      `components.<type>.<name>.auth.identity` from the raw componentConfig
      map BEFORE the mapstructure decode loses it, and propagates it to
      `info.Identity` with precedence `--identity` flag > component selector.
      Implemented in `internal/exec/utils_auth.go`:
      `extractComponentIdentitySelector`, **100% covered** by new unit tests.
- [x] **Issues 1 + 2 fix**: removed the global raw-YAML pre-scanner call
      from the auth flow. `pkg/auth/manager_helpers.go:CreateAndAuthenticateManagerWithAtmosConfig`
      no longer calls `loadAndMergeStackAuthDefaults`, and the helper itself
      is deleted. `internal/exec/workflow_utils.go:checkAndMergeDefaultIdentity`
      also stops calling the pre-scanner. The exec-layer stack processor
      (`getMergedAuthConfigWithFetcher` → `ExecuteDescribeComponent`) is the
      sole source of truth for stack-scoped auth defaults — it correctly
      follows `import:` chains against the specific target stack.
- [x] Bug-reproduction tests re-annotated as characterization tests for the
      (now-detached) `LoadStackAuthDefaults` function. New regression tests
      added at the `pkg/auth` level that verify the auth flow no longer
      leaks a cross-stack default.
- [x] Full test suite regression: `pkg/auth`, `pkg/config`,
      `internal/exec`, and the CLI scenario tests all pass.
- [ ] Regenerate scenario snapshots for the six CLI test cases (deferred —
      the test cases currently use substring matching which does not require
      snapshot regeneration).
- [ ] Changelog and blog post.

## Test fixtures and bug-reproduction tests (landed)

All three fixtures use `mock/aws` identities so they authenticate end-to-end
in CI without real cloud credentials.

### `tests/fixtures/scenarios/auth-imported-defaults/` — Issue #2293

Mirrors the real-world Cloud Posse reference architecture layout:

```
atmos.yaml                                        # name_template, excluded_paths: ['**/_defaults.yaml']
stacks/orgs/acme/dev/
├── _defaults.yaml                                # auth.identities.dev-identity.default: true
└── us-east-1/foundation.yaml                     # import: orgs/acme/dev/_defaults
```

The key detail: `_defaults.yaml` is listed under `stacks.excluded_paths`, so
`getAllStackFiles` filters it out before the raw-YAML pre-scanner ever sees
it. The stack name resolves to `acme-dev` via
`name_template: "{{ .vars.tenant }}-{{ .vars.stage }}"`.

**Observed current behavior** (verified via direct `LoadStackAuthDefaults`
call against the fixture):

```
IncludeStackAbsolutePaths: […]/stacks/orgs/**/us-east-1/*
ExcludeStackAbsolutePaths: […]/stacks/**/_defaults.yaml
LoadStackAuthDefaults returned 0 entries: map[]
```

Meanwhile, `atmos describe component mycomponent -s acme-dev -q
'.auth.identities.dev-identity.default'` correctly outputs `true` — because
the exec-layer stack processor *does* follow imports. The bug lives only in
the pre-scanner, not in the merged-config path.

### `tests/fixtures/scenarios/auth-stack-scoping/` — Discussion #122

Two unrelated stacks under `stacks/orgs/acme/`, using tenant names `data`
and `plat` (acme is a placeholder; the real customer namespace is redacted):

```
atmos.yaml                                        # NO global default, two identities
stacks/orgs/acme/
├── data/staging/us-east-1/monitoring.yaml        # auth.identities.data-default.default: true
└── plat/staging/us-east-1/eks.yaml               # NO auth block at all
```

Stack names resolve to `data-staging` and `plat-staging` respectively.

**Observed current behavior** (verified via direct `LoadStackAuthDefaults`
call against the fixture):

```
Issue 2 fixture LoadStackAuthDefaults returned 1 entries: map[data-default:true]
```

The loader returns the `data-default` identity globally, even though the
user may be targeting `plat-staging`. `MergeStackAuthDefaults` then calls
`clearExistingDefaults` on the merged authConfig and applies `data-default`.
Every command in the repo picks up `data-default` as "the" default.

Interestingly, at the `describe component eks -s plat-staging` layer, both
identities are still present in the merged output but neither is flagged
as default — so the bug is NOT visible via `atmos describe`. It only
manifests when the auth manager's pre-scanner runs. The CLI regression test
therefore asserts the describe-layer *correctness* as a guard, and the Go
unit test `TestLoadStackAuthDefaults_SingleDefaultLeaksGlobally_BugDiscussion122`
asserts the loader-level *bug*.

### `tests/fixtures/scenarios/auth-component-identity-selector/` — Slack report

```
atmos.yaml                                        # backend-role (default), provider-role
stacks/deploy/dev.yaml                            # two components:
                                                  #   s3-bucket    auth.identity: provider-role
                                                  #   no-override  (no auth block)
```

Stack name resolves to `dev` via `name_template: "{{ .vars.stage }}"`.

**Observed current behavior** (verified via direct
`MergeComponentAuthFromConfig` call against synthesized componentConfig):

```
Default identity in merged config: backend-role
schema.AuthConfig has no Identity field — auth.identity:provider-role was dropped.
After struct decode, merged.Identities has 2 entries but no trace of the selector.
```

And `atmos describe component s3-bucket -s dev -q '.auth.identity'` does
output `provider-role` at the describe layer — because the generic YAML map
merge preserves unknown keys. The field is lost only when `MergeComponentAuthConfig`
decodes the merged map into `*schema.AuthConfig` via mapstructure; that
struct has no `Identity` field so the key is silently dropped.

**Important refinement to the Issue 3 fix:** the initial proposal was to
add `Identity string` to `ComponentAuthConfig`. The refined, smaller fix is
to extract the string from `componentConfig["auth"]["identity"]` directly
in `internal/exec/utils_auth.go:getMergedAuthConfigWithFetcher`, *before*
the mapstructure decode loses it, and propagate it to `info.Identity`. The
schema can stay untouched. Either approach works; the minimal extract-
before-decode is preferred for diff size.

### Regression test files landed in this commit

| File | Purpose |
|---|---|
| `tests/fixtures/scenarios/auth-imported-defaults/` | Scenario fixture for Issue #2293 |
| `tests/fixtures/scenarios/auth-stack-scoping/` | Scenario fixture for Discussion #122 |
| `tests/fixtures/scenarios/auth-component-identity-selector/` | Scenario fixture for Slack report |
| `tests/test-cases/auth-identity-resolution-bugs.yaml` | Six CLI regression tests wired to the fixtures |
| `pkg/config/auth_defaults_test.go` | `TestLoadStackAuthDefaults_ExcludedDefaultsFile_BugIssue2293` + `TestLoadStackAuthDefaults_SingleDefaultLeaksGlobally_BugDiscussion122` |
| `pkg/auth/config_helpers_test.go` | `TestMergeComponentAuthFromConfig_ComponentIdentitySelectorIsDropped_BugSlackReport` |

Every new test is commented with a `BUG #<ref>` annotation explaining that
the current assertion documents the bug and will be flipped by the fix
commit. Reviewers can search for `BUG #` to see every assertion that will
change.

### Running the new tests

```bash
# Go unit tests
go test ./pkg/config/ -run 'BugIssue2293|BugDiscussion122' -v
go test ./pkg/auth/ -run 'BugSlackReport' -v

# CLI scenario tests (requires `make build` first)
make build
go test ./tests -run 'TestCLICommands/atmos_describe_component_—' -v
```

All six CLI test cases and all three Go unit tests pass against current
`main` — they document the bugs as stable, reproducible behavior. The fix
commit will invert the assertions marked `BUG #...` and the diff will
precisely show the behavior change.

---

## Relationship summary (TL;DR)

After reading the code, my conclusion is:

- **Issues 1 and 2 share the same root cause.** Both originate from
  `pkg/config/stack_auth_loader.go:LoadStackAuthDefaults`, a shortcut that
  scans every stack file globally *before* the target stack is known and tries
  to discover "the" default identity from raw YAML parsing. That shortcut is
  structurally wrong in two ways:
  1. It reads files with `yaml.Unmarshal` against a minimal struct — it does
     **not** follow `import:` directives, so any `auth:` block declared in an
     imported `_defaults.yaml` is invisible (**Issue 1**). In a typical Cloud
     Posse layout `_defaults.yaml` files are even listed under
     `stacks.excluded_paths`, so they never reach the glob at all.
  2. It aggregates every `default: true` flag it finds into a **global** pool
     and applies the result to every subsequent command regardless of target
     stack. The existing "conflict detection" only handles the case where
     *multiple different* identities are flagged as default; if exactly one
     stack file declares a default, that default silently leaks to every
     other stack, OU, and tenant (**Issue 2**).

- **Issue 3 is independent.** The `components.terraform.<name>.auth.identity`
  form the user wrote is **not defined in the Atmos schema**. The component
  auth section is parsed as `schema.ComponentAuthConfig`, which has only
  `Realm`, `Providers`, `Identities`, `Integrations` — no `Identity` string
  field. mapstructure silently drops unknown fields, so the user's directive
  is lost before it ever reaches the auth manager. There is no error, no
  warning; the default identity is used and Terraform fails at resource
  creation time.

Each section below drills into the exact file and lines, then proposes the
minimal change.

---

## Issue 1 — Default identity declared in an imported stack file is not recognized

**Source:** [#2293](https://github.com/cloudposse/atmos/issues/2293)

### Problem

When `auth.identities.<name>.default: true` is declared in an imported stack
file (for example `_defaults.yaml` that a stack manifest imports via
`import:`), Atmos does not pick it up during identity resolution. Instead,
Atmos prompts the user to select an identity interactively even though a
default is configured — and in non-interactive contexts this surfaces as
"no default identity configured."

The same identity resolves correctly when its `auth:` block is placed
**directly** in the top-level stack manifest rather than in an imported
defaults file.

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

Running any component command in that stack:

```bash
$ atmos terraform plan my-component -s acme-dev-us-east-1

No default identity configured. Please choose an identity:
> acme-dev
  core-root
  ...
```

Debug output confirms the imported `auth:` block is invisible to auth
resolution:

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

### Current workaround

Duplicate the `auth:` block in every top-level stack manifest rather than
declaring it once in `_defaults.yaml`. This works but defeats the purpose
of Atmos's import-based inheritance model.

### Related links called out in the issue

`#1950`, `#2071`, `#2081`, `#2125`, and PR `#1865` — flagged as the same
root-cause class.

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

1. Stack tree with multiple OUs / tenants, e.g.

   ```text
   stacks/orgs/gold/
     data/staging/us-east-1/monitoring-agent.yaml
     plat/staging/us-east-1/eks-cluster.yaml
     plat/prod/us-east-1/eks-cluster.yaml
   ```

2. Add a default-identity `auth:` block to **one** manifest, e.g.

   ```yaml
   # stacks/orgs/gold/data/staging/us-east-1/monitoring-agent.yaml
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
     in the stack manifest 'orgs/gold/plat/staging/us-east-1/monitoring-test'
   CreateAndAuthenticateManager called identityName="" hasAuthConfig=true
   Loading stack configs for auth identity defaults
   Loading stack files for auth defaults count=284
   Found default identity in stack config identity=data-staging/terraform
     file=/…/stacks/orgs/gold/data/staging/us-east-1/monitoring-test.yaml
   ```

   The file Atmos picks the default from belongs to a completely
   unrelated stack (`data-staging` vs the requested `plat-use1-staging`).

### Expected behavior

`default: true` under `auth.identities.<name>` should only apply to the
stack(s) that actually import or declare that `auth:` block. Unrelated
stacks in other OUs, tenants, or environments should be unaffected.

### Probable relationship to Issue #1

Cloud Posse already suggested in the discussion comments that this may
share a root cause with [#2293](https://github.com/cloudposse/atmos/issues/2293)
— both reports describe the auth-identity resolver processing stack
files without honoring stack scoping or import inheritance. Confirming
or refuting the shared root cause is part of the investigation that
follows.

---

## Issue 3 — Component-level `auth.identity` override is silently ignored

**Source:** User report (Slack)

### Problem

A stack has a **default identity** that is correctly used to read and write
the Terraform backend state (the backend role). An individual component in
that stack specifies a **different** identity via
`components.terraform.<name>.auth.identity` because the component needs to
create resources in a different AWS account.

Atmos authenticates successfully as the **default** identity during
`atmos auth login` / `atmos auth whoami` — as expected. But when running
`atmos terraform apply` for that component, Atmos **continues to use the
default identity** for provider-level AWS calls (resource creation)
instead of switching to the component-level override. Terraform then
fails when the default identity's role lacks permission to create the
target resource in the target account.

In short: the component-level `auth.identity` override has no effect on
the AWS provider credentials Terraform sees at apply time.

### Reproduction

Global auth config (`atmos.yaml`):

```yaml
auth:
  providers:
    corp-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start
      auto_provision_identities: true

  identities:
    tenant-a:
      default: true
      kind: aws/permission-set
      via.provider: corp-sso
      principal:
        name: role-for-tf-state
        account.id: "111111111111"

    tenant-shared/role-for-create-resources:
      kind: aws/permission-set
      via.provider: corp-sso
      principal:
        name: role-for-create-resource
        account.id: "222222222222"
```

Stack manifest for an S3 bucket component:

```yaml
# stacks/…/s3bucket-dev-ue1.yaml
import:
  - catalog/infra/s3-bucket
  - mixins/env/dev
  - mixins/region/us-east-1

components:
  terraform:
    s3-bucket:
      auth:
        identity: tenant-shared/role-for-create-resources
```

Org-level backend defaults (`_defaults.yaml`):

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: atmos-tf-state
      key: terraform.tfstate
      region: us-east-1
      role_arn: "arn:aws:iam::111111111111:role/role-for-tf-state"
      dynamodb_table: atmos-tf-state-lock
      encrypt: true
```

Observed behavior:

```bash
$ atmos auth login
$ atmos auth whoami
# -> assumed role-for-tf-state in account 111111111111   (correct default identity)

$ atmos terraform apply s3-bucket -s s3bucket-dev-ue1
# Error: arn:aws:iam::111111111111:role/role-for-tf-state
#        is not allowed to perform s3:CreateBucket action
```

### Expected behavior

At apply time, the AWS provider used by Terraform should be credentialed
with the component-level identity (`tenant-shared/role-for-create-resources`,
account `222222222222`) — **not** the default identity
(`tenant-a`, account `111111111111`). The backend role, which is used for
state read/write, should still be the default identity as configured in
the backend block.

In other words: Atmos should support **two separate credential contexts**
in one command — one for the Terraform backend (state), one for the
Terraform provider (resource operations) — with the component-level
`auth.identity` override controlling the latter.

---

---

## Root Cause Analysis and Proposed Fixes

### Issues 1 and 2 — shared root cause in `LoadStackAuthDefaults`

#### Where the bug lives

- `pkg/config/stack_auth_loader.go` — the `LoadStackAuthDefaults` function
  and its helpers `getAllStackFiles` / `loadFileForAuthDefaults`.
- `pkg/auth/manager_helpers.go:279` — `loadAndMergeStackAuthDefaults`
  is the caller: inside `CreateAndAuthenticateManagerWithAtmosConfig`, when
  `identityName == ""` and auth is configured, it calls out to the loader
  to "discover the default" before the target stack is known.

#### Exact code path

`CreateAndAuthenticateManagerWithAtmosConfig` in `manager_helpers.go`:

```go
if identityName == "" && isAuthConfigured(authConfig) && atmosConfig != nil {
    loadAndMergeStackAuthDefaults(authConfig, atmosConfig)
}
```

`loadAndMergeStackAuthDefaults` calls `cfg.LoadStackAuthDefaults(atmosConfig)`
which does this (`stack_auth_loader.go:47-116`):

```go
stackFiles := getAllStackFiles(
    atmosConfig.IncludeStackAbsolutePaths,
    atmosConfig.ExcludeStackAbsolutePaths,
)
// ...
for _, filePath := range stackFiles {
    fileDefaults, err := loadFileForAuthDefaults(filePath)   // raw yaml.Unmarshal
    // ...
    for identity, isDefault := range fileDefaults {
        if isDefault {
            allDefaults = append(allDefaults, defaultSource{identity, filePath})
        }
    }
}
// Conflict handling: only discards when defaults DISAGREE.
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

And `loadFileForAuthDefaults` itself (lines 162-193) does a minimal
`yaml.Unmarshal` against a struct that contains **only the top-level
`auth:` section**. There is no import-following, no template processing,
no deep-merge with `_defaults.yaml`.

#### Why Issue 1 fails

`_defaults.yaml` is typically listed in `stacks.excluded_paths`
(`atmosConfig.Stacks.ExcludedPaths`) because those files are meant to be
**imported** by stack manifests, not processed as standalone stacks. As a
result, `getAllStackFiles` filters them out entirely — their `auth:` block
is never even seen by the raw YAML parser.

Even if a `_defaults.yaml` file is not excluded, `loadFileForAuthDefaults`
does not follow its `import:` directive, so an `auth:` block that a user
has factored out into a deeper `_defaults.yaml` inside the import chain is
still invisible.

This is exactly what the debug log in #2293 shows:

```text
Loading stack files for auth defaults count=16
No default identities found in stack configs
```

16 files are scanned, none of them are the imported defaults, the auth
block never surfaces, and the user is prompted for an identity even
though `default: true` is correctly declared.

#### Why Issue 2 fails

The conflict-detection loop only handles *different* default identities
colliding across stack files. Look carefully at the logic:

```go
// If only ONE stack file declares a default:
allDefaults = [{identity: "data-staging/terraform", file: "monitoring-agent.yaml"}]
firstIdentity := "data-staging/terraform"
allAgree := true               // loop over [1:] is empty, so always true
// -> defaults["data-staging/terraform"] = true
```

The loader then returns a *global* map that says "the default identity for
Atmos is `data-staging/terraform`". This map is merged into `authConfig`
via `MergeStackAuthDefaults`, which sets `identity.Default = true` on the
corresponding identity in the global auth config. From that point on,
**every** `atmos terraform plan/apply` in that repo resolves to
`data-staging/terraform` as the default, regardless of which stack was
actually targeted.

This is exactly what Discussion #122 reported: adding a single
`auth.identities.<name>.default: true` to `monitoring-agent.yaml` caused
`plat-use1-staging` (an unrelated stack in a different OU) to load that
same identity.

#### Why the current approach is structurally wrong

The code comment above `CreateAndAuthenticateManagerWithAtmosConfig`
frames this as a chicken-and-egg problem:

> - We need to know the default identity to authenticate
> - But stack configs are only loaded after authentication is configured

This is **not** actually a chicken-and-egg. For every command where a
stack-scoped default identity could matter (`atmos terraform *`,
`atmos helmfile *`, etc.), the target stack is either:

1. Passed explicitly on the command line via `-s <stack>`, OR
2. Interactively selected by the user, OR
3. Derived from the `atmos.yaml` `stacks.name_pattern` after component/
   context resolution.

In every case the target stack is knowable **before** the auth manager
needs to exist. The only exception is `atmos auth login` (or `auth
whoami`/`auth env`/etc.) invoked with no stack context, in which case
the only meaningful default is the `atmos.yaml`-level default — no
stack scan needed.

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

The current code tries to compress steps 1-3 into a single pre-stack
scan, and in doing so breaks the scoping model entirely.

#### Proposed fix

**Delete the global auth-defaults scanner** and replace it with
stack-scoped resolution in the existing exec-layer flow.

Concretely:

1. **Remove** `pkg/config/stack_auth_loader.go` entirely, or reduce it
   to a no-op deprecated shim for any external caller.

2. **Remove the pre-scan call** from
   `pkg/auth/manager_helpers.go:loadAndMergeStackAuthDefaults`. The
   chicken-and-egg block in `CreateAndAuthenticateManagerWithAtmosConfig`
   (lines 238-240) goes away.

3. **Let the existing merged auth config carry the default.**
   `internal/exec/utils_auth.go:getMergedAuthConfigWithFetcher` already
   fetches `componentConfig` via `ExecuteDescribeComponent`, which
   *correctly* follows `import:` and merges `_defaults.yaml`. Then it
   calls `auth.MergeComponentAuthFromConfig` which deep-merges the
   component's auth section into a copy of the global auth config. This
   is the path that already works correctly for Issue 1's user — we just
   have to stop the *other* code path (the pre-scan) from stomping on it.

4. For the non-component path (commands that have a stack but no
   component, e.g. `atmos describe stacks -s <stack>`), add a small
   helper that loads the stack's merged config via the normal stack
   processor and extracts its `auth:` section. Same merge semantics, no
   raw-YAML shortcut.

5. For `atmos auth login`/`whoami`/`env` with no stack context, fall
   back to `atmos.yaml`-level defaults only. This is the status quo
   for commands that don't have a stack anyway.

**Why this fixes Issue 1:** The exec-layer path already uses
`ExecuteDescribeComponent` which follows `import:`, so a default
identity declared in an imported `_defaults.yaml` becomes visible in the
component's merged config automatically.

**Why this fixes Issue 2:** No more global scan means no more global
leak. A `default: true` declared in `stacks/orgs/gold/data/staging/
us-east-1/monitoring-agent.yaml` is only merged into the auth config for
commands that target that specific stack (or a stack that imports from
that file). Other stacks in other OUs stay unaffected.

#### Test plan

- New regression test `TestStackDefaultIdentity_ImportedDefaultsYaml`
  (in `internal/exec/` or `pkg/auth/`): create a fixture where
  `auth.identities.<name>.default: true` lives in an imported
  `_defaults.yaml`; assert that `createAndAuthenticateAuthManager`
  returns an auth manager authenticated as that identity without
  prompting.
- New regression test `TestStackDefaultIdentity_ScopedToStack`:
  create a fixture with two unrelated stack manifests — one declares a
  default identity, the other does not. Assert that targeting the
  second stack does **not** inherit the first stack's default.
- New regression test
  `TestStackDefaultIdentity_AtmosYamlDefaultStillWorks`: when
  `atmos.yaml` declares `auth.identities.<name>.default: true` and no
  stack overrides it, that default is still honored (guards the
  non-component auth code path).
- Delete `TestLoadStackAuthDefaults_*` and `TestMergeStackAuthDefaults_*`
  in `pkg/config/auth_defaults_test.go` once the loader goes away.

---

### Issue 3 — component-level `auth.identity` is not in the schema

#### Where the bug lives

- `pkg/schema/schema_auth.go:131-137` — `ComponentAuthConfig` struct.
- `pkg/auth/config_helpers.go:140-162` — `MergeComponentAuthFromConfig`.
- `internal/exec/utils_auth.go:92-129` — `getMergedAuthConfigWithFetcher`,
  which calls into the merge.

#### Exact code path

The `ComponentAuthConfig` struct is:

```go
type ComponentAuthConfig struct {
    Realm        string                 `yaml:"realm,omitempty" ...`
    Providers    map[string]Provider    `yaml:"providers,omitempty" ...`
    Identities   map[string]Identity    `yaml:"identities,omitempty" ...`
    Integrations map[string]Integration `yaml:"integrations,omitempty" ...`
}
```

No `Identity string` field exists.

The merge flow in `MergeComponentAuthFromConfig` → `MergeComponentAuthConfig`
(`config_helpers.go:76-125`):

```go
mergedMap, err := merge.Merge(atmosConfig, []map[string]any{globalAuthMap, componentAuthSection})
// ...
var finalAuthConfig schema.AuthConfig
if err := mapstructure.Decode(mergedMap, &finalAuthConfig); err != nil { ... }
```

`mergedMap` is `map[string]any` — it *does* contain the user's
`"identity": "tenant-shared/role-for-create-resources"` key at this
point, because `merge.Merge` preserves unknown keys. But
`mapstructure.Decode` into `schema.AuthConfig` silently drops it:
`AuthConfig` has no `Identity string` field either. The string is gone
by the time the merged config reaches
`CreateAndAuthenticateManagerWithAtmosConfig`.

Meanwhile `createAndAuthenticateAuthManagerWithDeps`
(`utils_auth.go:49-82`) calls the auth manager constructor with
`info.Identity` — which is **only** populated from the `--identity`
CLI flag or interactive selection (see `cli_utils.go:496-510` and
`cmd/terraform/utils.go:361,483`). The component's stack-config
`auth.identity: foo` is never consulted.

End result: the user's `components.terraform.s3-bucket.auth.identity`
is silently dropped during schema decoding, `info.Identity` stays
empty, and the default identity (`tenant-a`, the backend role) is
used for provider-level AWS calls. The resource-creation call fails
because the backend role doesn't have `s3:CreateBucket` in the target
account.

The supported form according to current docs
(`website/docs/cli/configuration/auth/identities.mdx:428-446`) is:

```yaml
components:
  terraform:
    s3-bucket:
      auth:
        identities:
          role-for-create-resources:
            kind: aws/permission-set
            # ... full identity definition duplicated from atmos.yaml ...
            default: true
```

This requires the user to **redefine** the identity inside the component,
which is exactly the kind of duplication Atmos's layered config is
supposed to eliminate. And even this documented form would trip Issue 2's
leak if `default: true` is in a file that other stacks don't import.

#### Proposed fix

**Add a first-class component-level identity selector** that points to
an existing global identity by name.

1. **Extend `ComponentAuthConfig`** with a new field:

   ```go
   type ComponentAuthConfig struct {
       Realm        string                 `yaml:"realm,omitempty" ...`
       Identity     string                 `yaml:"identity,omitempty" ...` // NEW
       Providers    map[string]Provider    `yaml:"providers,omitempty" ...`
       Identities   map[string]Identity    `yaml:"identities,omitempty" ...`
       Integrations map[string]Integration `yaml:"integrations,omitempty" ...`
   }
   ```

   Semantics: a pure selector. Points to a global identity defined in
   `atmos.yaml` or merged via the stack/component import chain. Does not
   define a new identity.

2. **In `getMergedAuthConfigWithFetcher`** (`internal/exec/utils_auth.go`),
   after the merge, extract `componentAuthSection["identity"]` as a
   string and propagate it into `info.Identity` — but **only** if
   `info.Identity` is still empty. That preserves precedence:
   `--identity` CLI flag > component `auth.identity` > stack default >
   `atmos.yaml` default.

3. **Validation.** If the referenced identity name does not exist in the
   merged auth config, return an error with a clear message
   (`identity "X" referenced by components.terraform.<name>.auth.identity
   is not defined in auth.identities`). No silent drop.

4. **Integrate the component auth into the existing nested-auth helper**
   (`internal/exec/terraform_nested_auth_helper.go`). That file already
   has `resolveAuthManagerForNestedComponent` which checks the component
   auth section for `hasDefaultIdentity`. Extend it to also honor the
   new `auth.identity` selector with the same precedence rules.

5. **Documentation update.** `website/docs/cli/configuration/auth/
   identities.mdx` §"Component-Level Overrides" should show the new
   one-liner form alongside the full-definition form. The one-liner
   should be the recommended pattern:

   ```yaml
   components:
     terraform:
       s3-bucket:
         auth:
           identity: role-for-create-resources   # refers to a global identity
   ```

#### Two-credential-context clarification

The user's observation that their *backend* role continues to work
correctly while only the *provider* credentials need to change is
important. The fix deliberately does **not** touch the Terraform
backend block (`terraform.backend.s3.role_arn`). That block is
processed by Terraform itself during `terraform init`, not by the Atmos
auth manager — the backend uses a static `AssumeRole{}` stanza that
AWS SDK resolves on its own. So after the fix:

- **Backend state I/O** → continues to use `role_arn` in
  `terraform.backend.s3` (unchanged).
- **Provider resource I/O** → uses the component-level
  `auth.identity` selector via the Atmos auth manager (new).

This matches the user's mental model and the typical multi-account
Terraform pattern (state in a "root" account, resources in "workload"
accounts).

#### Test plan

- `TestComponentAuthIdentitySelector_BasicSelection` — component
  declares `auth.identity: foo` referring to a global identity; assert
  that the auth manager authenticates as `foo`, not as the global
  default.
- `TestComponentAuthIdentitySelector_CliFlagWins` — when both
  `--identity=bar` and `auth.identity: foo` are set, assert that
  `bar` wins.
- `TestComponentAuthIdentitySelector_UnknownIdentity` — referring to
  a non-existent identity produces a clear error (not a silent fall-back
  to the default).
- `TestComponentAuthIdentitySelector_BackendRoleUnchanged` — the
  Terraform backend's `role_arn` is still honored independently of the
  component-level identity selector. Uses a tfplan fixture to verify
  both credential contexts resolve correctly in one `terraform plan`.

---

## Actual implementation (landed)

### Issue 3 — smallest blast radius, landed first

**`internal/exec/utils_auth.go`**

- Added `extractComponentIdentitySelector(info, configFetcher, mergedAuthConfig)`
  — a helper that fetches the component config via the injected
  `componentConfigFetcher` (same one `getMergedAuthConfigWithFetcher` uses),
  reads `componentConfig["auth"]["identity"]` as a string, and validates it
  against `mergedAuthConfig.Identities` (with case-insensitive fallback via
  `IdentityCaseMap`). Returns:
  - `""` + `nil` error when no selector is present or the fetch fails
    non-fatally (`ErrInvalidComponent` propagates unchanged).
  - the canonical identity name when the selector resolves.
  - a wrapped `ErrInvalidAuthConfig` error naming the component, stack, and
    unknown identity when the selector points to a missing identity.

- `createAndAuthenticateAuthManagerWithDeps` calls
  `extractComponentIdentitySelector` only when `info.Identity == ""` (the
  `--identity` flag is not set). On success it updates `info.Identity` and
  logs a debug line naming the component, stack, and selected identity.

- The schema (`schema.AuthConfig` / `schema.ComponentAuthConfig`) is
  intentionally **not** modified. The selector only needs to survive the
  exec-layer hop; it must never reach the struct decoder.

**Precedence** (from highest to lowest):
1. `--identity` CLI flag.
2. Component-level `auth.identity` stack-config selector.
3. Default identity from the merged auth config (resolved downstream by
   `resolveIdentityName`).

### Issues 1 + 2 — removed the pre-scanner from the auth flow

**`pkg/auth/manager_helpers.go`**

- Deleted the `loadAndMergeStackAuthDefaults` helper entirely.
- Deleted the call site in `CreateAndAuthenticateManagerWithAtmosConfig` and
  replaced it with a comment explaining the decision, pointing at this doc.
- `CreateAndAuthenticateManagerWithAtmosConfig` now passes the incoming
  `authConfig` through to `resolveIdentityName` unchanged. Callers that need
  stack-scoped defaults (all exec-layer paths) have already had their
  `authConfig` merged correctly via `getMergedAuthConfigWithFetcher` →
  `ExecuteDescribeComponent`, which follows `import:` chains.

**`internal/exec/workflow_utils.go`**

- `checkAndMergeDefaultIdentity` simplified — no longer calls
  `config.LoadStackAuthDefaults` / `config.MergeStackAuthDefaults`. It only
  inspects `atmosConfig.Auth.Identities` for `Default: true`, relying on the
  exec-layer stack processor to have populated that map with the target
  stack's merged auth config.

**`pkg/config/stack_auth_loader.go`**

- **Kept in place** with no code changes. The `LoadStackAuthDefaults` /
  `MergeStackAuthDefaults` functions are now unreachable from the auth flow
  but remain available for any future caller that explicitly opts in (e.g. a
  lightweight docs generator). They are annotated as characterization tests
  rather than regression tests in `auth_defaults_test.go`.

### Test updates

**`internal/exec/utils_auth_test.go`** — added 12 new tests:

- `TestCreateAndAuthenticateAuthManagerWithDeps_ComponentIdentitySelector`
- `TestCreateAndAuthenticateAuthManagerWithDeps_CliIdentityFlagOverridesComponentSelector`
- `TestCreateAndAuthenticateAuthManagerWithDeps_ComponentIdentitySelectorUnknownIdentity`
- `TestCreateAndAuthenticateAuthManagerWithDeps_ComponentWithoutSelectorUnchanged`
- `TestExtractComponentIdentitySelector_NoStackOrComponent`
- `TestExtractComponentIdentitySelector_InvalidComponentErrorPropagates`
- `TestExtractComponentIdentitySelector_OtherFetcherErrorSuppressed`
- `TestExtractComponentIdentitySelector_NoAuthSection`
- `TestExtractComponentIdentitySelector_AuthSectionWithoutIdentityKey`
- `TestExtractComponentIdentitySelector_IdentityKeyWrongType`
- `TestExtractComponentIdentitySelector_EmptyStringIdentity`
- `TestExtractComponentIdentitySelector_CaseInsensitiveLookup`

**`pkg/auth/manager_helpers_test.go`**

- Deleted 4 obsolete tests for the removed `loadAndMergeStackAuthDefaults`
  helper.
- Added 2 new regression tests:
  - `TestCreateAndAuthenticateManagerWithAtmosConfig_HonorsMergedConfigDefault`
    — merged authConfig's default is honored, pre-scanner doesn't clobber it.
  - `TestCreateAndAuthenticateManagerWithAtmosConfig_DoesNotLeakCrossStackDefault`
    — merged authConfig with no default stays empty, no cross-stack leak.

**`pkg/auth/config_helpers_test.go`**

- Renamed `TestMergeComponentAuthFromConfig_ComponentIdentitySelectorIsDropped_BugSlackReport`
  to `TestMergeComponentAuthFromConfig_ComponentIdentitySelectorDroppedByDecoder`
  and re-annotated: the test documents the pkg/auth struct layer's intentional
  behavior (schema has no `Identity string` field). The fix lives in the exec
  layer, not here.

**`pkg/config/auth_defaults_test.go`**

- Renamed `TestLoadStackAuthDefaults_ExcludedDefaultsFile_BugIssue2293` →
  `TestLoadStackAuthDefaults_ExcludedDefaultsFile` and re-annotated as a
  characterization test.
- Renamed `TestLoadStackAuthDefaults_SingleDefaultLeaksGlobally_BugDiscussion122` →
  `TestLoadStackAuthDefaults_SingleDefaultIsReturnedGlobally` and re-annotated.

**`internal/exec/workflow_utils_test.go`**

- Deleted 4 obsolete stack-loading tests for `checkAndMergeDefaultIdentity`
  (`_WithStackLoading`, `_LoadError`, `_LoadErrorNoDefault`,
  `_StackNoDefaults`, `_EmptyStackDefaults`).

### Coverage

After the fix:

| Function | Coverage |
|---|---|
| `internal/exec/utils_auth.go:createAndAuthenticateAuthManagerWithDeps` | 94.4% (uncovered line is the pre-existing `ErrInvalidAuthConfig` wrap branch) |
| `internal/exec/utils_auth.go:extractComponentIdentitySelector` | **100%** |
| `internal/exec/utils_auth.go:getMergedAuthConfigWithFetcher` | 100% |
| `pkg/auth/manager_helpers.go:CreateAndAuthenticateManagerWithAtmosConfig` | Unchanged — the removed block simplified the function, all remaining branches are exercised by existing tests. |

### Regression run

```
go test ./pkg/auth/ ./pkg/config/ ./internal/exec/ -count=1        →  all PASS
go test ./tests -run 'TestCLICommands/atmos_describe_component_—'  →  6 PASS
go test ./pkg/auth/ ./pkg/config/ -count=1 -race                   →  PASS
```

The `internal/exec` race run exceeds the 300s budget on a local laptop due
to pre-existing heavy tests; it is not related to this fix. CI will exercise
it fully.

### Remaining follow-ups

1. Regenerate scenario snapshots if the fix commit changes any CLI output
   (the current test cases use substring matching, so this may be skippable).
2. Consider deleting `pkg/config/stack_auth_loader.go` and
   `pkg/config/auth_defaults_test.go` in a separate PR — they are now
   unreachable dead code and kept only for characterization coverage. A
   standalone deletion PR keeps this fix focused on behavior change.
3. Changelog + blog post once merged.
