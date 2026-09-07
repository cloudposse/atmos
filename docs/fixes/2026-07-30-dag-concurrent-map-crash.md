# Fix: DAG-scheduled bulk terraform commands crash with `concurrent map iteration and map write`

**Date:** 2026-07-30

## Summary

Users running DAG-scheduled bulk terraform commands (`terraform <cmd> --all`, `--affected`, `--query`) with
`--max-concurrency` set above 1 could hit a fatal `concurrent map iteration and map write` crash. The crash
went away at low concurrency, which was the tell: it was a data race, not a logic bug. Fixed by shallow-cloning
the shared, cached component-config map before any downstream code mutates it.

## Context

`FindStacksMap` (`internal/exec/utils.go`) caches fully-processed stack config in a package-global map and,
on a cache hit, returns the cached tree **by reference**. The guarding `RWMutex` protects only the cache
lookup, not the nested maps it hands out.

`ProcessComponentConfig` then extracted a component's section from that shared tree and assigned it (and its
sub-sections) directly into the per-node `ConfigAndStacksInfo`, still aliasing the cache. Two things wrote into
that shared section:

- `ProcessStacks` injecting `atmos_component`, `atmos_stack`, `workspace`, `sources`, `deps`, `deps_all`, etc.
- `mergeGlobalAuthConfig`, which installs the merged `auth` section — and runs once per candidate stack, not
  just the eventual winner.

Meanwhile, `findComponentInStacks` makes every DAG worker evaluate the target component in *every* stack
(not just its own), iterating each candidate's shared section along the way (env-filter loop, `componentConfigsEqual`
deep-compare, template-context range, `ConvertToYAMLPreservingDelimiters` walk). With `--max-concurrency > 1`,
the scheduler (`pkg/scheduler`) runs these workers concurrently, so one worker's write into a shared section
raced with another worker's read/iteration of that same section — the exact crash reported.

The describe-stacks processor (`internal/exec/describe_stacks_component_processor.go`) already had one instance
of the same class of bug fixed via a documented shallow clone; the audit for this fix found two further
un-cloned in-place deletes there that corrupt the cache for later callers in the same process (not part of the
crash itself, since describe runs during the serial planning phase, but a real bug once the cache is polluted).

## Changes

- `internal/exec/utils.go`: `ProcessComponentConfig` now shallow-clones (`maps.Clone`) the extracted component
  section immediately after extraction, before any sub-section derivation, `mergeGlobalAuthConfig`, or later
  mutation. All top-level writers now land on a private clone instead of the cache's own map. Documented the
  sharing contract on `FindStacksMap`'s doc comment (returned maps are shared by reference on a cache hit and
  must be treated as read-only).
- `internal/exec/describe_stacks_component_processor.go`: applied the same shallow-clone-before-mutate fix to
  two adjacent sites that deleted keys (`imports`, `terraform_workspace_pattern`/`terraform_workspace_template`)
  from cache-owned maps in place.
- `internal/exec/process_stacks_shared_cache_test.go` (new): two regression tests —
  `TestProcessStacksDoesNotMutateSharedStacksMapCache` (deterministic snapshot-equality check against the cache,
  no race detector required) and `TestProcessStacksConcurrentSharedCacheAccess` (16 goroutines running
  `ProcessStacks` across two stacks that share a component, reproducing the production interleaving under
  `-race`).

## Validation

- `go test ./internal/exec -run TestProcessStacksDoesNotMutateSharedStacksMapCache -count=1` — confirmed FAIL
  before the fix (cache polluted with `atmos_component`/`workspace`/etc.), PASS after.
- `go test -race ./internal/exec -run TestProcessStacksConcurrentSharedCacheAccess -count=1` — confirmed FAIL
  (data race reported) before the fix, PASS after.
- `go build ./...` — clean.
- `go test ./internal/exec ./pkg/scheduler/... -count=1` — all pass (includes existing describe-stacks golden
  snapshot tests, confirming the two hardening clones don't change output).
- `atmos lint --changed` — 0 issues.
- `atmos test` (full suite): the `tests` package hit an unrelated, pre-existing local-only hang — this sandbox's
  podman auto-boots a vfkit VM during an emulator-preflight CLI test when Docker isn't running, which doesn't
  happen in CI (Docker present there). Verified pre-existing on this machine in a prior session; `pkg/container`
  is untouched by this change.

## Follow-ups

None.
