# `atmos describe stacks -s <stack>` Authenticates Every Component in Every Other Stack

**Date:** 2026-06-22
**Severity:** High — on a large multi-stack repo, `describe stacks -s <stack>` (and other auth-enabled describe-stacks callers) authenticate every component in every stack before filtering, re-authenticating each one from scratch; the command runs for minutes or appears to hang
**Issue:** https://github.com/cloudposse/atmos/issues/2639 (originally reported against `atmos secret list`; that command was fixed separately by #2646)
**Reproducer:** `internal/exec/describe_stacks_component_processor_auth_test.go` (`TestProcessComponentEntry_OutOfScopeSkipsAuth`, `TestResolveComponentAuthManager_CachesByAuthSection`)

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bug fix in the shared describe-stacks processor: it scopes per-component
authentication to the components the caller actually requested, and reuses one authenticated manager
per identity within a pass. There is no new command, flag, or feature — only a correction so
auth-enabled `describe stacks` / `list` / `terraform --all` stop authenticating out-of-scope
components and stop re-authenticating the same identity once per component. Per the repo's label
decision tree that makes it a `patch`, which does not require a `website/blog/` post or a roadmap
milestone.

## Symptom

`atmos describe stacks -s <stack>` on a repo with many components — where components declare their
own `default: true` identity — does not return promptly. Debug logging shows it authenticates
components from stacks **other than** the one requested, running a full authentication cycle
(credentials-file rewrite under a file lock + keyring rebuild) for each. On a representative
multi-stack repo the command authenticated 1,000+ components across all stacks and effectively hung.

The same shape applies to every auth-enabled `ExecuteDescribeStacks` caller: `atmos list values` /
`list instances`, `atmos terraform --all` / `--query`, etc. `atmos secret list` was the originally
reported case in #2639; #2646 fixed that command specifically by disabling per-component auth during
secret-list enumeration, but the shared processor was left unchanged.

## Root Cause

In `processComponentEntry` (`internal/exec/describe_stacks_component_processor.go`), per-component
auth was resolved **before** the stack and component filters, and was never memoized:

```go
// Old order
componentAuthManager, err := p.resolveComponentAuthManager(...)  // full auth cycle, every component
propagateAuth(&info, componentAuthManager)
if shouldFilterByStack(p.filterByStack, stackFileName, stackName) { return nil }  // filter, too late
...
if !componentIncluded { return nil }                                              // filter, too late
```

Because `ExecuteDescribeStacks` walks every stack file, this authenticated every component that
declares its own default identity — in every stack — before discarding the out-of-scope ones.
Per-component auth exists only to populate `info.AuthContext` for that component's **later** template
(`atmos.Component(...)`) and YAML-function (`!terraform.state`, `!terraform.output`) processing,
which is skipped for filtered-out components. And because there was no cache, components that resolve
to the same identity each re-ran the full authentication cycle.

## Fix

Two changes in `processComponentEntry` / `resolveComponentAuthManager`:

1. **Scope — filter before auth.** Move the stack and component filters above
   `resolveComponentAuthManager` so only in-scope components authenticate. Authentication still
   precedes every consumer of `info.AuthContext` (`BuildTerraformWorkspace`, template processing,
   YAML-function processing), which all run later in the function.

1. **Reuse — cache per identity.** Add a pass-scoped cache keyed by the parent chain plus a
   deterministic JSON fingerprint of the component auth section. `createComponentAuthManager` derives
   its result solely from the auth section, the constant global config, and the parent manager
   (`componentName`/`stackName` affect only logging), so components sharing an auth section produce an
   equivalent, identity-scoped manager — safe to reuse. Fingerprinting the auth section (rather than a
   bare identity name) keeps reuse provably correct: it never merges components whose sections differ.
   Errors are never cached; sections that can't be serialized fall back to no caching.

No `cmd/secret` change is needed — #2646 already made `secret list` credential-free. The fix lives in
shared code, so `describe stacks -s`, `list values/instances`, and `terraform --all/--query` all
benefit.

## Verification

- `go test ./internal/exec/ -run 'ProcessComponentEntry|ResolveComponentAuthManager'` — the
  out-of-scope tests assert the resolver is invoked **zero** times for components outside the
  requested stack/component set; the cache test asserts two components sharing an auth section resolve
  **once** and a distinct section resolves anew.
- End-to-end on a representative multi-stack repo, running the **identical** command
  `atmos describe stacks -s <stack> --logs-level Debug` under a 45s budget with **only the atmos
  binary varying**:
  - **Latest release (v1.221.1):** did not complete within 45s — authenticated mostly components in
    stacks *other than* the requested one.
  - **Current main (post-#2646):** did not complete within 45s — same out-of-scope per-component
    authentication.
  - **With this fix:** completed in ~18s — 43 authentications, with in-scope processor-path auth
    reduced to **2**; the remaining ~42 are the nested `!terraform.output` / `atmos.Component`
    resolution path.

## Recommendations

- **Further dedup the nested YAML/template auth path.** Components that reference other components via
  `!terraform.output` / `!terraform.state` / `atmos.Component(...)` authenticate through
  `resolveAuthManagerForNestedComponent`, which does not share this processor-level cache. Extending a
  per-identity cache to that path would remove the remaining cross-component auth cycles during
  template/YAML resolution. Tracked separately.
