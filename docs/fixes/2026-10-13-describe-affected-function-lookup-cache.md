# Fix: Restore inspection caches for Terraform and component references

**Date:** 2026-10-13

## Summary

Repeated `!terraform.output`, `!terraform.state`, and `atmos.Component` references
reuse inspection results while keeping them separate from execution results.
Deferred authentication also reuses the raw target describe and a stable
auth-disabled manager within the invocation.

## Context

A customer repository (~200 stacks, ~690 instances) reported that
`atmos describe affected --upload --process-functions=true` increased from
roughly 3 minutes to 15–48 minutes after upgrading to v1.230.1, and still took
18–24 minutes on v1.232.0. Supplied logs showed hundreds of repeated output
fetches in the earlier slow release. The repository and its cloud identities
were not available for local reproduction; this evidence supports the diagnosis
but does not attribute every minute of the reported runtime to these caches.

The secret-inspection protection introduced in #3174 bypassed all three lookup
caches when masking without resolving secrets. Deferred auth introduced in
#3212 also described the raw target before every cache lookup, and created a new
auth-disabled wrapper on each reference. The output cache retains manager
identity for credential isolation, so those fresh wrappers prevented cache hits.
Nested target describes evaluate their own functions, independently of the
outer command's skip list.

This is related to #3353/#3354, which restored the missing deferred
`atmos.Store` template cache. The existing store benchmark exercises that path;
it did not cover these three Terraform/component lookup paths.

## Changes

- Partition inspection and execution values instead of bypassing caches during
  inspection. Include configuration paths in global keys so HEAD and BASE
  cannot exchange static outputs or component sections.
- Preserve effective-auth configuration partitions for deferred values and
  retain the supplied manager/context isolation in output keys. Reuse disabled
  wrappers only within their invocation and effective target configuration.
- Cache successful raw describes per invocation and target/configuration,
  including auth configuration. Deep-copy stored and returned sections before
  callers inherit authentication. Failed, nil, and computed describes remain retryable.
  Initialize the shared evaluation context before describe passes copy configuration.
- Preserve the existing cycle detector without wrapping describes or evaluated
  lookups in singleflight. Even raw component describes may evaluate stack locals
  during loading; concurrent recursive lookups must not wait on each other.
  Concurrent cold misses may duplicate work; completed values are reused.
- Invalidate all output/state partitions for an updated target. Preserve
  computed-value retries and cold/warm static missing/null output semantics.
- Add an offline CLI workload with 200 consumers and 600 references per pass,
  split across all three functions. Cover unchanged repositories, changed
  referenced values, and dependents without requiring Terraform or cloud access.

## Validation

- Regression tests reproduced repeated inspection fetches and unstable disabled
  manager identity before the fix. Raw-describe cache tests reproduced repeated
  work and aliasing with an uncached implementation.
- Focused tests verify both inspection/execution orders, no secret reads in
  inspection, configuration/auth/invocation isolation, retries, invalidation,
  and concurrent raw-describe access with independent maps.
- `go test -race ./pkg/terraform/output -count=1` and
  `go test -race ./pkg/deferred ./pkg/stack/deferred -count=1` passed.
- `go build ./...` passed. The new CLI fixture passed all three scenarios,
  including exact affected identities and resolved HEAD/BASE values.
- `go test -tags=mage ./magefiles/bench -count=1` passed.
- The final focused race selection passed across output, state/component,
  deferred value/auth caches, secret inspection, and dependency cycle tests.
- The static workload does not model Terraform process latency, remote backend
  latency, or OIDC login time.

## Follow-ups

None.
