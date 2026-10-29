# Fix: Terraform output provisioning cache isolation across checkouts

**Date:** 2026-10-28

## Summary

Output provisioning caches no longer reuse a HEAD workdir result for a BASE checkout with the
same stack/component names. Nested module output lookup resolves the module subdirectory.

## Context

Evaluated affected discovery failed writing a generated backend file into a missing temporary
BASE workdir. The provisioning cache was keyed only by stack and component, unlike the existing
reference-value caches that include the configuration root.

## Changes

The cache and concurrent-work key includes checkout, effective path, source configuration and
hashed authentication context. Missing target directories trigger provisioning again. CLI
invocation boundaries clear reuse records. Source subdirectories resolve after provisioning.

## Validation

Unit tests assert separate calls across HEAD/BASE, version changes and deleted targets while warm
repeats reuse a valid target. The complete output package short suite passes. A reduced source
producer/template consumer fails twice on each engine with main and reconciled binaries; fixed
Terraform 1.15.6 and OpenTofu 1.12.5 runs succeed twice and select the source-changed component.
No-evaluation discovery remains successful.

Cold and cached preparation tests also verify nested module selection, inheritance fallback,
and classified errors for invalid file or absolute module paths. Reconstructed explicit source
workdirs resolve the existing nested module and state without creating a second workdir.

## Follow-ups

Evaluated affected discovery still permits output lookup to refresh HEAD source files and generate
execution artifacts. This repair isolates HEAD/BASE caches; it does not make evaluated discovery
read-only. Strict no-write staging and general relative-path-preserving workdirs are deferred:
the user prefers preserving today's workdir design over introducing the proposed execution-tree
architecture in this PR. No new layout or migration is implemented. The deferred design is retained
in `.context/plans/terraform-workdir-context-plan.md`; the reusable
`tests/fixtures/scenarios/terraform-registry-field-test/relative-context/` fixture now reproduces
the inherited nested-module failure on main and the repaired binary, twice per engine. Those checks
establish the problem, not a fix. All field commands ran against disposable directories, never
production state or the user's source checkout.
