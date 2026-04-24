# Fix: Per-component auth for `atmos list instances --upload` and other template consumers

**Date:** 2026-04-24

## Issue

- `atmos list instances --upload` fails in CI (GitHub Actions) with
  `Error: No valid credential sources found` when component sections
  contain Go-template calls to `atmos.Component(x, y)` that fetch
  terraform outputs from a remote S3 backend.
- In the same workflow (same `atmos.yaml`, same profile, same stacks),
  `atmos describe affected --upload --process-functions=true` succeeds.
  Logs show a cluster of `"Starting GitHub OIDC authentication"` /
  `"GitHub OIDC authentication successful"` lines, one per component
  that has its own `auth:` section with a default identity.
- The `list instances --upload` run shows **zero** OIDC log lines.
  The command proceeds all the way to `terraform init` with an empty
  `AuthContext`, and the S3 backend rejects the request because no AWS
  credentials were ever resolved.
- The failure chain is:
  `list instances → describe stacks → template processing →`
  `atmos.Component(...) → ExecuteWithSections(authContext=nil) →`
  `terraform init → "No valid credential sources found"`.

The practical symptom is that `list instances --upload` was never
usable in CI for repos that rely on stack-level default identities
(the common pattern for GitHub-OIDC → AWS sts flows) combined with
`atmos.Component(...)` template calls inside component sections.

## Root cause

`internal/exec/describe_stacks_component_processor.go` resolves a
per-component `AuthManager` during stack processing and propagates the
resulting `AuthContext` onto the `schema.ConfigAndStacksInfo` that
template functions read. That per-component resolution was gated on
`processYamlFunctions`:

```go
// Old code — describe_stacks_component_processor.go
componentAuthManager := p.authManager
if p.processYamlFunctions {
    authSection, hasAuth := componentSection[cfg.AuthSectionName].(map[string]any)
    if hasAuth && hasDefaultIdentity(authSection) {
        resolved, _ := createComponentAuthManager(...)
        if resolved != nil {
            componentAuthManager = resolved
        }
    }
}
propagateAuth(&info, componentAuthManager)
```

The guard was justified by a comment claiming YAML functions are
"the only consumer of auth context". That assumption is wrong.
`atmos.Component(...)` is a **Go template function**
(`internal/exec/template_funcs_component.go`), not a YAML function.
It runs during the `processTemplates` branch of the processor, reads
`configAndStacksInfo.AuthContext`, and then calls
`tfoutput.ExecuteWithSections(atmosConfig, component, stack, sections, authContext)`
which shells out to `terraform init` + `terraform output`. It needs
credentials just as much as `!terraform.state` does.

Callers that set `processTemplates=true, processYamlFunctions=false`
therefore skipped per-component auth entirely. The two Category B
callers matching that shape are:

- `pkg/list/list_instances.go:processInstancesWithDeps` — intentionally
  disables YAML functions (to avoid requiring `tofu` / `terraform` on
  `$PATH` for the common `list instances` case) but keeps templates
  enabled (so templates can create additional stacks and components).
- `pkg/list/list_instances.go:ExecuteListInstancesCmd` tree-format
  branch — same shape for provenance-aware tree rendering.

Both were implicitly broken for any component that relies on
stack-level default identities *and* has an `atmos.Component(...)`
call in its rendered section. The CI repro used by the reporting
user has exactly that shape: an `onepassword-item-retrieval/snowflake`
component whose `settings` section templates in values from
`atmos.Component("aws-ssm-parameter-store/1password-connect",
"core-use1-auto")`.

The top-level `createAuthManagerForList` did return `nil` in this
scenario because auto-detection via `GetDefaultIdentity(false)` yields
no single winning default in non-interactive CI mode when multiple
stacks declare the same-named default through imports. That is the
**expected** behavior — it matches `describe affected`'s top-level
auth path, which also returns `nil` here. `describe affected` still
works because per-component auth fills in later; `list instances`
did not.

## Status

**Fixed.** The guard in
`internal/exec/describe_stacks_component_processor.go` now fires when
**either** templates or YAML functions will run:

```go
// New code
componentAuthManager := p.authManager
if p.processYamlFunctions || p.processTemplates {
    authSection, hasAuth := componentSection[cfg.AuthSectionName].(map[string]any)
    if hasAuth && hasDefaultIdentity(authSection) {
        resolved, _ := createComponentAuthManager(...)
        if resolved != nil {
            componentAuthManager = resolved
        }
    }
}
propagateAuth(&info, componentAuthManager)
```

The comment above the block was updated to name both consumers
(`!terraform.state` / `!terraform.output` YAML functions *and*
`atmos.Component(...)` template calls).

## Goals

1. `atmos list instances --upload` works end-to-end in CI for stacks
   whose component sections call `atmos.Component(...)` inside Go
   templates.
2. No changes to the public CLI surface. No new flag, no change to
   default behavior visible to users who were not affected.
3. Category A callers (`atmos terraform *`, `atmos helmfile *`,
   `atmos describe component`, nested-auth flows) are untouched —
   they already pass `processYamlFunctions=true`, so they always
   took the old path and continue to take the new one.
4. Category B callers that set `processYamlFunctions=true`
   (`describe affected`, `describe stacks`, `describe dependents`,
   `list affected`, `aws security`, `aws compliance`, workflows)
   are unchanged in observable behavior — the guard condition was
   already true for them.
5. Category B callers that set `processTemplates=true,
   processYamlFunctions=false` (`list instances`, the
   provenance-aware tree branch) start running per-component auth.
   This is the intended behavior change.

## Non-goals

- **Making `atmos.Component(...)` errors non-fatal during
  `list instances`.** That was considered as an alternative — skip the
  template call entirely and render a placeholder — but it hides a
  real misconfiguration class and diverges behavior from describe
  affected, which successfully resolves the same template. The
  proper answer is "run auth", not "suppress errors".
- **Adding a `--process-functions` flag to `list instances`.** The
  flag never existed there; a user workflow was relying on a flag
  that had never been shipped. The reporting user removed that flag
  from their workflow before opening the issue. Re-adding it would
  break the "no new flags" goal and would not fix the underlying
  credential propagation.
- **Changing `pkg/list/list_instances.go`.** The disable-YAML-functions
  choice there (to avoid requiring `tofu` on `$PATH`) is correct. The
  bug was strictly inside the describe-stacks processor.

## Implementation

### Where the fix lands

Single file, single guard, plus a comment refresh:

- `internal/exec/describe_stacks_component_processor.go`
  - `if p.processYamlFunctions` → `if p.processYamlFunctions || p.processTemplates`
  - Comment updated to list both consumers.

Nothing else changes. `createComponentAuthManager`, `propagateAuth`,
and the downstream `atmos.Component(...)` → `ExecuteWithSections` path
are unchanged — we are only widening the condition that schedules the
per-component auth.

### Why widening the guard is safe

`createComponentAuthManager` is guarded by two checks **inside** the
branch:

- `componentSection[cfg.AuthSectionName]` must be a map (the component
  must declare an `auth:` section), **and**
- `hasDefaultIdentity(authSection)` must be true (the component must
  declare a default identity, e.g. `default: true`).

Components that do **not** declare their own `auth:` section continue
to fall through to `componentAuthManager := p.authManager` (the parent
manager from the top-level `createAuthManagerForList` / describe
affected path), exactly as before. The widened guard only *enables*
per-component auth for components that already opted into it.

### Why this is not just a `list instances` fix

Any caller of `ExecuteDescribeStacks` that sets
`processTemplates=true, processYamlFunctions=false` and describes
components that (a) declare their own default identity and
(b) contain `atmos.Component(...)` template calls is affected. Today
that set is `list instances` + its tree-format branch, but nothing in
the type signature of `ExecuteDescribeStacks` prevents future callers
from taking the same shape. Fixing the guard keeps those future
callers correct by construction.

### Cost implication

Commands that previously hit the `processYamlFunctions=false,
processTemplates=true` combination now incur one per-component
OIDC/STS roundtrip for each component with its own default identity,
matching what describe affected already does. For `list instances
--upload` runs in CI, this is a small additive cost per component
and is necessary for the command to succeed at all.

## Testing

### Regression test

`internal/exec/describe_stacks_component_processor_auth_test.go` —
`TestPerComponentAuthRunsForTemplatesOnlyPath`. Exercises the
previously broken path in isolation using mocks so it runs in any
environment without cloud credentials:

- Constructs a `describeStacksProcessor` via `newDescribeStacksProcessor`
  with `processTemplates=true, processYamlFunctions=false` (the
  `list instances` shape).
- Synthesizes a component section whose `auth:` subsection declares a
  default identity.
- Calls the per-component branch that contains the widened guard.
- Asserts, via a spy on the component-auth resolver, that the
  component-auth path **was** entered. A companion subtest with
  `processTemplates=false, processYamlFunctions=false` asserts the
  guard stays off — so the change does not widen the condition into
  the "neither templates nor YAML functions" quadrant.
- A second companion subtest verifies that a component **without** an
  `auth:` section never enters the per-component resolver even when
  `processTemplates=true` — guarding against accidentally running auth
  for components that never opted in.

### Existing coverage that continues to pass

- `TestAuthManagerPropagationToDescribeStacks`
  (`internal/exec/describe_stacks_authmanager_propagation_test.go`)
  covers `processTemplates=false, processYamlFunctions=false` +
  pre-authenticated top-level AuthManager. The widened guard is false
  there, so the test's expectation (propagate the top-level manager,
  no per-component resolution) is unchanged.
- `TestAuthManagerPropagationToDescribeComponent` + nested-auth
  propagation tests cover Category A (`processYamlFunctions=true`),
  unaffected by this change.
- `describe_affected_authmanager_test.go` covers the existing
  per-component path that already worked — unchanged.

### Manual verification

Not applicable for this repository — exercising the real bug requires
a GitHub Actions runner with an OIDC identity provider and an AWS
backend. The regression test reproduces the decision the processor
makes, which is the actual fix point. The follow-up PR that wires the
fix into the `atmos-pro` release-workflow fixtures (if any) will carry
the integration-shaped verification.

## Progress checklist

- [x] Fix doc (this file).
- [x] Widen guard in
  `internal/exec/describe_stacks_component_processor.go`.
- [x] Update the comment above the guard to mention template
  consumers (`atmos.Component(...)`) in addition to YAML functions.
- [x] Regression test
  `TestPerComponentAuthRunsForTemplatesOnlyPath` with companion
  subtests for the "off" quadrants.
- [x] Existing tests continue to pass:
  `TestAuthManagerPropagationToDescribeStacks`,
  `TestAuthManagerPropagationToDescribeComponent`,
  `describe_affected_authmanager_test.go` suite.

## Follow-ups

- **Clarify the `processYamlFunctions=false` contract in
  `pkg/list/list_instances.go`.** The comment there says YAML
  functions like `atmos.Component()` are disabled; that wording
  conflates the YAML-function flag with Go-template functions.
  Separate, doc-only cleanup.
- **Consider a `--process-functions` flag for `list instances`** if
  users want the parity-with-describe-affected explicit control. Not
  blocking this fix.

---

## Related

- `docs/fixes/2026-04-08-atmos-auth-identity-resolution-fixes.md` —
  design rationale for `CreateAndAuthenticateManagerWithStackScan`
  (Category B callers) and
  `CreateAndAuthenticateManagerWithAtmosConfig` (Category A callers).
- `internal/exec/describe_stacks_component_processor.go` — the
  per-component auth resolver that this fix widens.
- `internal/exec/template_funcs_component.go` — the
  `atmos.Component(...)` implementation that reads
  `configAndStacksInfo.AuthContext`.
- `pkg/terraform/output/get.go:ExecuteWithSections` — the terraform
  output fetcher that requires a populated `AuthContext` to
  authenticate the `terraform init` subprocess against the S3
  backend.
- `pkg/list/list_instances.go` — the caller that sets
  `processTemplates=true, processYamlFunctions=false` and therefore
  triggered the bug.
