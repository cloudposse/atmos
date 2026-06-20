# `atmos secret list -s <stack>` Authenticates Every Component in Every Other Stack

**Date:** 2026-06-20 **Severity:** High — on a repo with many components the
command never reaches its output; it runs a full authentication cycle
(credentials file rewrite + file lock + keyring rebuild) for each component
across all stacks, scaling with the whole repo instead of the requested stack
**Issue:** https://github.com/cloudposse/atmos/issues/2639 **Reproducer:**
`internal/exec/describe_stacks_component_processor_auth_test.go`
(`TestProcessComponentEntry_OutOfScopeStackSkipsAuth`,
`TestProcessComponentEntry_OutOfScopeComponentSkipsAuth`)

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bugfix in the shared `describe stacks` processor: it scopes
per-component authentication to the components the caller actually asked for.
There is no new command, flag, or feature to announce — only a correction so
`-s <stack>` (and `--components`) stop authenticating components that are
immediately filtered out. Per the repo's label decision tree that makes it a
`patch`, which does not require a `website/blog/` post or a roadmap milestone.

______________________________________________________________________

## Symptom

`atmos secret list --stack <stack>` without `--component` does not return on a
repo with many components. Debug logging shows it enumerates components from
stacks **other than** the one requested and runs a full authentication cycle for
**each** one:

```text
DEBU Created component-specific AuthManager component=<other-component> stack=<other-stack> identityChain="[sso acme-prod/terraform]"
DEBU CreateAndAuthenticateManager called identityName="" hasAuthConfig=true
DEBU Writing AWS credentials provider=sso identity=acme-prod/terraform credentials_file=...
DEBU Acquired file lock lock_file=.../credentials.lock
DEBU Building keyring key alias=acme-prod/terraform realm=""
```

The per-component auth count grows roughly linearly with the total number of
components in the repo (not the requested stack), so the command takes far
longer than expected and appears to hang. Adding `--component` scopes it to a
single component and returns immediately, because `secret list` takes a separate
single-component fast path that never calls `ExecuteDescribeStacks`.

## Root Cause

`secret list -s <stack>` (no `--component`) enumerates declarations via
`ExecuteDescribeStacks` with `filterByStack=<stack>`, `processTemplates=true`,
and `processYamlFunctions=true` (`cmd/secret/enumerate.go`). The shared
processor walks **every** stack file and **every** component (`FindStacksMap` →
`processStackFile` → `processComponentEntry`).

Inside `processComponentEntry`
(`internal/exec/describe_stacks_component_processor.go`), per-component auth was
resolved **before** the stack and component filters:

```go
// Old order
componentAuthManager, err := p.resolveComponentAuthManager(...)  // full auth cycle
propagateAuth(&info, componentAuthManager)
if shouldFilterByStack(p.filterByStack, stackFileName, stackName) { return nil }  // filter, too late
...
if !componentIncluded { return nil }                                              // filter, too late
```

`resolveComponentAuthManager` runs the full authentication cycle for every
component that declares its own `auth:` section with a `default: true` identity
— regardless of whether that component belongs to the requested stack. Its only
purpose is to populate `info.AuthContext` for the component's **later** template
(`atmos.Component(...)`) and YAML-function (`!terraform.state`,
`!terraform.output`) processing, which is skipped for any component that gets
filtered out. So for out-of-scope components the authentication was pure waste.

## Fix

Move the stack and component filters **above** `resolveComponentAuthManager` so
a component that is out of scope returns before any authentication happens.
Authentication still runs before every consumer of `info.AuthContext`
(`BuildTerraformWorkspace`, template processing, YAML-function processing),
which all execute later in the function.

```go
// New order
if shouldFilterByStack(p.filterByStack, stackFileName, stackName) { return nil }
if stackName == "" { stackName = stackFileName }
if !componentIncluded { return nil }

// Only now, for in-scope components, resolve per-component auth.
componentAuthManager, err := p.resolveComponentAuthManager(...)
propagateAuth(&info, componentAuthManager)
```

No change to `cmd/secret` is needed — it already passes the correct
`filterByStack`; the processor just applied it too late. The fix is in shared
code, so `atmos describe stacks -s <stack>` and the `atmos list *` family
benefit identically.

## Verification

- `go test ./internal/exec/ -run 'ProcessComponentEntry|ShouldResolvePerComponentAuth|ResolveComponentAuthManager'`
  — new out-of-scope tests assert the auth resolver is invoked **zero** times
  for components outside the requested stack / component set; existing
  per-component auth tests are unchanged.
- End-to-end: `atmos secret list -s <stack>` on a multi-stack repo with a SOPS
  `aws-kms` provider and a default identity returns promptly, and any
  `Created component-specific AuthManager` debug lines reference **only** the
  requested stack.

## Recommendations

- **Follow-up (not in this PR): dedupe per-identity auth within a stack.** Even
  scoped to one stack, components that share the same default identity each run
  a separate authentication cycle. Caching the authenticated manager per
  identity for the duration of a `describe stacks` pass would remove the
  remaining redundant credential/keyring writes. Tracked separately.
