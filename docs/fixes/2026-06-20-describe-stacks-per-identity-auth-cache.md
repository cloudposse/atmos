# Describe-Stacks Re-Authenticates Per Component That Shares an Auth Section

**Date:** 2026-06-20 **Severity:** Medium — functional output is correct, but
every component with its own default identity triggers a fresh authentication
cycle (credential-file rewrite + file lock + keyring rebuild), so a stack with
many same-identity components does N redundant auth cycles instead of one
**Follow-up to:**
docs/fixes/2026-06-20-secret-list-stack-scope-per-component-auth.md
**Reproducer:** `internal/exec/describe_stacks_component_processor_auth_test.go`
(`TestResolveComponentAuthManager_CachesByAuthSection`)

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` performance fix in the shared `describe stacks` processor: it
memoizes the per-component AuthManager within a single pass so components that
share an auth section authenticate once instead of N times. There is no new
command, flag, or feature — only fewer redundant credential and keyring writes.
Per the repo's label decision tree that makes it a `patch`, which does not
require a `website/blog/` post or a roadmap milestone.

## Symptom

After scoping per-component auth to the requested stack (the linked stack-scope
fix), running `atmos secret list -s <stack>` on a stack with many components
still authenticated once **per component**. On a repo whose components all
import the same default identity from a shared `_defaults.yaml`, debug logs
showed ~one `Created component-specific AuthManager` per component for the
requested stack — e.g. 74 cycles for 74 components that all resolve to the same
identity. Each cycle rewrites the SSO credentials/config files (under a file
lock) and rebuilds keyring entries, which is slow and entirely redundant when
the identity is identical.

## Root Cause

`describeStacksProcessor.resolveComponentAuthManager`
(`internal/exec/describe_stacks_component_processor.go`) called the
per-component resolver (`createComponentAuthManager`) once for every component
that declares its own `auth:` section with a `default: true` identity, with no
memoization across the pass. `createComponentAuthManager` derives its result
solely from the component's auth section, the (constant) global auth config, and
the parent manager — `componentName` and `stackName` are used only for logging —
so components that share an auth section produce an equivalent authenticated
manager, yet each one paid the full authentication cost.

## Fix

Add a pass-scoped cache (`authManagerCache`) on `describeStacksProcessor`, keyed
by the parent manager's chain plus a deterministic JSON fingerprint of the
component's auth section. `createComponentAuthManager` derives its result solely
from the auth section, the (constant) global config, and the parent manager
(`componentName`/`stackName` affect only logging), so two components with the
same auth section produce an equivalent authenticated manager. Because
identities are defined globally and only referenced by components, "same auth
section" is a safe, provable proxy for "same identity": the first component
authenticates, and subsequent components with the same auth section reuse the
cached manager (its `AuthContext` is identity-scoped, so the credentials match).

- The cache is consulted only after the existing guards (auth enabled,
  templates/YAML will run, the component declares its own default identity), so
  behavior is unchanged for components that never resolved per-component auth.
- Resolver **errors are never cached** — they remain fatal per call.
- Fingerprinting the auth section (rather than bare identity name) keeps reuse
  provably correct: it never merges two components whose sections differ, so a
  component that overrides an identity inline can't accidentally borrow
  another's credentials.
- When the auth section cannot be serialized deterministically (e.g. non-string
  map keys), the key is marked non-cacheable and the component resolves without
  caching — correct, just not deduplicated.
- The describe-stacks pass is single-threaded (`executeDescribeStacks` iterates
  stacks sequentially), so the map needs no synchronization.

## Verification

- `go test ./internal/exec/ -run 'ResolveComponentAuthManager|ProcessComponentEntry'`
  — `TestResolveComponentAuthManager_CachesByAuthSection` asserts the resolver
  runs once for two components that share an auth section and again for a
  distinct one; existing per-component auth and out-of-scope tests are
  unchanged.
- End-to-end: `atmos secret list -s <stack>` on a multi-stack repo whose
  components share one default identity now emits a single
  `Created component-specific AuthManager` for that shared identity instead of
  one per component, and returns faster.

## Recommendations

- **Further dedup the nested YAML/template auth path.** Components that
  reference other components via `!terraform.output` / `!terraform.state` /
  `atmos.Component(...)` authenticate through
  `resolveAuthManagerForNestedComponent`, which does not share this
  processor-level cache. Extending a per-identity cache to that path would
  remove the remaining cross-component auth cycles during template/YAML
  resolution. Tracked separately.
