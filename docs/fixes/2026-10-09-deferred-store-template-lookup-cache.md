# Fix: Restore cached `atmos.Store` template lookups with deferred auth

**Date:** 2026-10-09

## Summary

Repeated `atmos.Store` calls in templates now reuse successful values within a
deferred-auth invocation. Concurrent calls for the same value share one backend
read. The backend's credential binding and lock remain unchanged on cache misses.

## Context

[Issue #3353](https://github.com/cloudposse/atmos/issues/3353) reports that
`atmos describe affected` grew from about 13 seconds on v1.229.0 to about nine
minutes on v1.231.0 in a repository with roughly 2,200 template calls over 11
distinct store keys. The first slow release was v1.230.0.

[PR #3212](https://github.com/cloudposse/atmos/pull/3212) made list and describe
authenticate only when a requested value needs credentials. This was needed
because inventory commands previously authenticated a default identity before
checking whether their output used it, so an unavailable provider could block
credential-free output. The change routed `AtmosFuncs.Store` through
`storedeferred.LookupStore` whenever auth was deferred. That resolver deliberately
leaves value memoization to its caller, but the new template caller did not add
a cache. The eager `storeFunc` path still cached non-nil results, so repeated
template references changed from 11 backend reads to roughly 2,200.

The deferred store path also resets auth context before each lookup and holds a
per-backend lock around binding and reading. Those safeguards were added because
store clients are shared across components: the same identity name can have
different credentials or endpoints in different effective auth configurations,
and concurrent rebinding must not alter a read in progress. With no value cache,
every repeated template call paid for a client reset and serialized remote read.

## Changes

- Add a separate store-value map and singleflight group to the existing
  invocation-scoped deferred evaluation context. Cache only successful,
  non-nil template results; later calls retry nil results and errors.
- Build the cache key from the backend instance, requested store/stack/component/key,
  effective merged auth configuration, selected store identity, auth-disabled
  state, and relevant config paths. This allows components with the same auth to
  share reads without sharing values across credentials or invocations. If a safe
  key cannot be built, the lookup runs uncached.
- Route only deferred `atmos.Store` template calls through the cache. YAML
  `!store` reads and scoped secret-store operations keep their existing behavior.
  Cache hits avoid backend access; misses still use the existing auth reset and
  per-backend lock, preserving the cross-component credential isolation.

## Validation

- A regression test made 2,200 template calls across components and 11 keys;
  mock expectations confirmed exactly 11 backend reads.
- Tests covered separate credentials, configured identities, disabled auth,
  backend instances, and invocations; retry of nil results and errors;
  concurrent same-key calls; and avoidance of repeated auth binding on hits.
- Focused store/template tests and the existing concurrent credential-isolation
  test passed. `go test -race ./internal/exec ./pkg/store/deferred
  ./pkg/stack/deferred -run 'Test(DeferredStoreTemplate|ConcurrentStoreReadsKeepTheirComponentCredentials|DeferredStoreSuccessfulAuthIsNotRepeated|ValueCache)' -count=1`
  passed. `git diff --check` passed.
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh
  docs/fixes/2026-10-09-deferred-store-template-lookup-cache.md` passed.
- The reporter's AWS-backed repository was unavailable, so its wall-clock
  improvement was not measured locally.

## Follow-ups

None.
