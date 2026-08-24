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
2. Category A callers (`atmos terraform *`, `atmos helmfile *`,
   `atmos describe component`, nested-auth flows) are untouched —
   they already pass `processYamlFunctions=true`, so they always
   took the old path and continue to take the new one.
3. Category B callers that set `processYamlFunctions=true`
   (`describe affected`, `describe stacks`, `describe dependents`,
   `list affected`, `aws security`, `aws compliance`, workflows)
   are unchanged in observable behavior — the guard condition was
   already true for them.
4. Category B callers that set `processTemplates=true,
   processYamlFunctions=false` (`list instances`, the
   provenance-aware tree branch) start running per-component auth.
   This is the intended behavior change.
5. Users get `--process-templates` and `--process-functions` on
   every `atmos list` subcommand that processes stack manifests,
   matching the flag surface of the `describe` command family.

## Implementation

### Where the fix lands

Single file in `internal/exec/describe_stacks_component_processor.go`:

1. **Widen the guard** — `if p.processYamlFunctions` becomes
   `if p.processYamlFunctions || p.processTemplates`.
2. **Extract the decision** into a named helper,
   `shouldResolvePerComponentAuth(processTemplates, processYamlFunctions)`,
   so the widened condition is self-documenting and directly
   unit-testable.
3. **Extract the per-component resolution** out of the inline
   `processStackFile` body into a method on the processor,
   `resolveComponentAuthManager(componentSection, componentName,
   stackName) auth.AuthManager`. The method owns the full decision
   tree (guard check → auth-section presence check → default-identity
   check → resolver call → nil-and-error fallback) in one place.
4. **Add an injectable resolver field** to `describeStacksProcessor`:

   ```go
   // componentAuthManagerResolver mirrors the signature of
   // createComponentAuthManager so tests can supply a spy.
   type componentAuthManagerResolver func(
       atmosConfig *schema.AtmosConfiguration,
       componentConfig map[string]any,
       component string,
       stack string,
       parentAuthManager auth.AuthManager,
   ) (auth.AuthManager, error)
   ```

   Defaults to the real `createComponentAuthManager` in
   `newDescribeStacksProcessor`. Tests override the field directly
   on the struct to avoid running actual OIDC/STS.
5. **Comment refresh** above the guard names both consumers
   (YAML functions *and* the `atmos.Component(...)` template path)
   and cross-links this fix doc so future readers understand why the
   predicate is intentionally looser than "YAML functions only".

`createComponentAuthManager`, `propagateAuth`, and the downstream
`atmos.Component(...)` → `ExecuteWithSections` path are unchanged.
The only behavior change is that more components have their auth
resolved up front when the processor is invoked with
`processTemplates=true, processYamlFunctions=false`.

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

### Regression tests

`internal/exec/describe_stacks_component_processor_auth_test.go`
covers the fix in three layers. All tests use a spy resolver so they
run in any environment without cloud credentials:

1. `TestShouldResolvePerComponentAuth` — four-quadrant truth table
   for the new `shouldResolvePerComponentAuth(processTemplates,
   processYamlFunctions)` helper. Names the
   `(templates=true, yaml=false)` case as the regression subject so
   a future refactor that reintroduces the `yaml`-only guard fails
   loudly.
2. `TestResolveComponentAuthManager` — six-row table that exercises
   the full `describeStacksProcessor.resolveComponentAuthManager`
   method with a spy `componentAuthResolver`:
   - All four `(templates, yaml)` quadrants against a component
     whose `auth:` subsection declares a default identity. Asserts
     the spy was called exactly once when expected and never when
     disabled, and that the returned manager is the component-
     specific one (when the spy runs) or the parent manager (when
     it does not).
   - Two "component did not opt in" rows:
     `(templates=true, yaml=false)` with **no** `auth:` section on
     the component, and `(templates=true, yaml=false)` with an
     `auth:` section that has no `default: true` identity. Both
     must skip the resolver — guarding against accidentally running
     per-component auth for components that never opted in.
3. `TestResolveComponentAuthManager_ResolverErrorFallsBackToParent`
   — when the component-auth resolver returns an error, the method
   must silently fall back to the parent manager. This preserves
   the original swallow-on-error behavior of the inline code that
   was refactored.

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
makes, which is the actual fix point.

## Progress checklist

- [x] Fix doc (this file).
- [x] Widen guard in
  `internal/exec/describe_stacks_component_processor.go`.
- [x] Update the comment above the guard to mention template
  consumers (`atmos.Component(...)`) in addition to YAML functions.
- [x] Regression test
  `TestShouldResolvePerComponentAuth` +
  `TestResolveComponentAuthManager` (six-quadrant table with spy
  resolver) +
  `TestResolveComponentAuthManager_ResolverErrorFallsBackToParent`.
- [x] Existing tests continue to pass:
  `TestAuthManagerPropagationToDescribeStacks`,
  `TestAuthManagerPropagationToDescribeComponent`,
  `describe_affected_authmanager_test.go` suite.
- [x] Clarify the `processYamlFunctions=false` contract in
  `pkg/list/list_instances.go` — the old comment conflated the
  YAML-function flag with Go-template functions like
  `atmos.Component(...)`. Replaced with an accurate description.
- [x] Add `--process-templates` / `--process-functions` flags
  (plus `ATMOS_PROCESS_TEMPLATES` / `ATMOS_PROCESS_FUNCTIONS` env
  var bindings) to every `atmos list` subcommand that processes
  stack configurations, matching the flag surface of
  `atmos describe affected` / `atmos describe stacks` /
  `atmos describe component`. Commands updated:
  - `atmos list instances`
  - `atmos list components`
  - `atmos list metadata`
  - `atmos list sources`
  - `atmos list stacks`

  Already exposed these flags: `list affected`, `list settings`,
  `list values` (and its `list vars` alias). Skipped: `list aliases`,
  `list themes`, `list vendor`, `list workflows` — these either do
  not call `ExecuteDescribeStacks` at all or call it only for
  internal enumeration where the flags would be no-ops.
- [x] Thread the two flags through `InstancesCommandOptions` and
  `MetadataOptions` in `pkg/list/`, and through the matrix-format
  and tree-format branches of `list_instances.go`, so every output
  path of the same invocation honors the same flag values.
- [x] Update the `list instances` documentation page with the two
  new flags.

## Flag surface

The fix ships alongside `--process-templates` and
`--process-functions` (and their `ATMOS_PROCESS_TEMPLATES` /
`ATMOS_PROCESS_FUNCTIONS` env var bindings) on every `atmos list`
subcommand that processes stack manifests: `list instances`,
`list components`, `list metadata`, `list sources`, `list stacks`.
Defaults are `true`, matching `describe affected` / `describe stacks`
/ `describe component`. The auth-guard widening above is what makes
defaulting both to `true` safe; without it, the `list instances`
path would fail in CI with the `No valid credential sources found`
error described in the Issue section.

1. **Default behavior matches the describe command family.** Both
   flags default to `true`. Commands that previously ran with
   `processTemplates=true, processYamlFunctions=false` hardcoded
   (the `list instances` / `list metadata` shape) now default to
   `processTemplates=true, processYamlFunctions=true`.
2. **Escape hatch for environments without `tofu` / `terraform` on
   `$PATH`.** Users who want the old hardcoded behavior can pass
   `--process-functions=false` or set
   `ATMOS_PROCESS_FUNCTIONS=false`. This is the documented fallback
   for the original PR #2170 rationale ("don't require `tofu` in
   `$PATH` for `list instances`").
3. **Flag descriptions disambiguate the two axes.** Go template
   functions like `atmos.Component(...)` are controlled by
   `--process-templates`; YAML functions like `!terraform.state`,
   `!terraform.output`, `!store`, `!aws.*` are controlled by
   `--process-functions`.

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
