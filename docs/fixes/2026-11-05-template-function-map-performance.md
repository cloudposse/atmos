# Fix: Stack processing slowed by per-render Gomplate setup and edition journal sorting

**Date:** 2026-11-05

## Summary

`describe stacks`, `describe affected`, and `list instances` were 37–72% slower in v1.234.0-rc.1 than in
v1.233.2 with identical output and identical store backend reads. Two per-render costs caused most of it.
Building a template function map now reuses Gomplate's stateless namespaces, and behavior-edition lookups use a
cached index instead of copying and sorting the journal on every call.

## Context

The v1.233.2 → v1.234.0-rc.1 regression test (`go tool mage bench:fieldTest v1.233.2,v1.234.0-rc.1`) reported
`describe affected` +70% (0.665 → 1.131 s) and `describe stacks` +54% (0.449 → 0.693 s) on the 2,200-reference
store workload. The 1,000-instance workloads without stores showed `describe stacks` +37% and `list instances`
+55%, so the slowdown was not store-specific.

CPU profiles of the published binaries showed two causes:

- #3457 moved template rendering from Gomplate v3 to v4. Atmos builds a function map for every template it
  renders, including every component's stack name template. Gomplate v4's `CreateFuncs` creates its `file`
  namespace with `datafs.WrapWdFS`, which calls `os.Getwd` on each call. That system call was about 70% of the
  additional CPU time. It remained significant with `PWD` set (+33–43%).
- `edition.BehaviorChanged` runs for every evaluated YAML value through `utils.ExplicitYAMLTags` (new with
  explicit YAML tags). It called `Journal()`, which copies and sorts the entire journal each time.

## Changes

- `pkg/template/gomplate/funcs.go`: Gomplate's function map is created once per process for each experimental
  setting and cloned for each render. Namespaces that capture process state when created stay per render and
  are created lazily on first use, as with Gomplate v3: `file` binds the working directory, and `aws`, `gcp`, and
  the `ec2*` functions read client options from the environment and cache clients. `SetExperimental` records the
  experimental setting for the cache because Gomplate's own flag is not readable outside its module.
- `pkg/template/gomplate/data.go`: `CreateFuncs`, `JSON`, `CSV`, and `TOML` use the shared map.
- `internal/exec/template_utils.go`: Both callers use the package's `SetExperimental`.
- `pkg/edition/behavior.go`: `BehaviorChanged` uses a per-journal index of the first behavior transition per key,
  rebuilt when the journal slice changes (tests swap it).

## Validation

- New tests in `pkg/template/gomplate/funcs_test.go`: function names and signatures match Gomplate's for both
  experimental settings; maps are isolated from each other and from the shared map; `file.Read` resolves against
  the working directory at each render; cloud namespaces are per map and reused within a map; experimental
  functions are gated and do not leak. `BenchmarkCreateFuncs`: about 1 µs and 23 allocations per map.
- New test in `pkg/edition/behavior_test.go`: replacing the journal refreshes the index, and the oldest transition
  for a key decides.
- `go test` passed for `./pkg/template/...`, `./pkg/edition/`, `./pkg/utils/`, `./pkg/generator/...`, and the
  template, Gomplate, and datasource tests in `./internal/exec/`. `gofumpt -l` reported no changes.
  `golangci-lint` was not run (the command was not permitted in that session).
- Same-host CLI benchmarks (Apple M4 Max, 8 interleaved samples; published binaries; locally built fix):

  | Workload | v1.233.2 | rc.1 | fix |
  | --- | ---: | ---: | ---: |
  | describe stacks, 1,000 instances | 1.355 s | 1.570 s | 1.254 s (no significant difference) |
  | list instances, 1,000 instances | 0.682 s | 1.045 s | 0.774 s (+13%) |
  | describe affected, store, 2,200 references | 0.683 s | 1.174 s | 0.975 s (+43%) |
  | describe stacks, store, 2,200 references | 0.464 s | 0.720 s | 0.621 s (+34%) |

  Output was byte-identical to v1.233.2 for the profiled `describe stacks` and `list instances` runs.

## Follow-ups

The remaining slowdown on the store and `list instances` workloads is spread across other per-render costs:
`yamlsource.Decode` parsing base64 JSON metadata for tagged values, the YAML encode and parse round trip in
`processComponentSectionTemplates`, registering about 300 functions with `text/template.Funcs` per render, and
template evaluation in `naming.Resolver.workspaceContextPrefix`. These are part of the same unreleased
v1.234.0 work and should be addressed before the final release.
