# Fix: Template rendering no longer rediscovers the policy directory on every render

**Date:** 2026-11-05

## Summary

Policy discovery now remembers the project roots for the current working
directory, arguments, and environment. Template renders outside a pinned CLI
invocation no longer resolve the Git root and parse configuration flags twice
per render. The `policy.d` inside those roots is still checked on every call,
and `ATMOS_POLICY_DIR` is still read on every call.

## Context

Per-render template cost rose about 75% in time and 97% in allocations from
v1.232.0 (83 µs, 222 allocations) to v1.233.0 (145 µs, 437 allocations)
(`BenchmarkTemplateProcessing`, same machine). A bisect over the 95 commits
between those releases, keyed on allocations, found two steps: #3365 (policy
admission, 223 → 269) and #3387 (project-root discovery, 269 → 437). v1.200.0
measured the same as v1.232.0, so per-call template cost had been flat until
then.

Every render calls `guard.Enabled` for function and source rules. Each call
reaches `admission.LoadBundle`. During a CLI run, `admission.BeginInvocation`
in `cmd/root.go` pins the bundle, so `LoadBundle` returns it without disk work.
Without a pinned invocation, as when Atmos is embedded as a Go library (for
example by the Terraform `utils` provider), `LoadBundle` ran
`policy.DirectoryFromEnvironment` each time. That built a new Viper instance and
ran `projectroot.SearchDirs`, which parses `os.Args`, resolves the Git root, and
looks for `atmos.yaml` in parent directories. It ran twice per render.

The pinned CLI path is not affected by this fix. It still measures about 117 µs
and 267 allocations, about 40% slower than v1.232.0 with 20% more allocations.
That cost is spread across the template engine's per-render function-map
assembly and registration, and is not caused by policy discovery.

## Changes

- `pkg/policy/plan_directory.go`: cache the `projectroot.SearchDirs` result,
  keyed on the working directory, `os.Args`, and the full environment. Any
  change to those inputs recomputes the roots. Only the roots are cached:
  `policy.d` is stat-ed on every call, so creating or removing it takes effect
  immediately. Policy files are loaded as before.
- Bind `ATMOS_POLICY_DIR` once instead of creating a Viper instance per call.
  Viper reads a bound variable when it is requested, so changes are still seen;
  a mutex guards reads because Viper is not safe for concurrent use.
- `pkg/policy/discovery_cache_test.go`: reuse for unchanged inputs, live
  `policy.d` creation and removal over cached roots, recomputation when the
  working directory, arguments, a discovery environment variable, or an
  unrelated environment variable changes, concurrent discovery, and key
  separation.

## Validation

- `BenchmarkTemplateProcessing` (unpinned, the library path), relative to
  v1.232.0: +90% time and +112% allocations before, +58% time and +36%
  allocations after (158 → 131 µs, 471 → 303 allocations; 17% less time and
  36% fewer allocations per render). The pinned CLI path is unchanged at
  +41–51% time and +20% allocations.
- `TestPolicyDirectoryReusesRootsForUnchangedInputs` failed with the cache
  disabled and passed with it enabled.
- `go test -race -count=1 ./pkg/policy/...` passed. The new discovery tests
  also passed three times under the race detector.
- `go test -short -count=1 ./internal/exec -run 'Template|Policy|Tmpl'` passed.
- `atmos lint --changed` reported 0 issues; `gofumpt` is clean.
- The existing discovery tests, which create `policy.d` after a first lookup in
  the same directory and set `ATMOS_POLICY_DIR` mid-test, pass unchanged.

## Follow-ups

- The roots are not recomputed if `atmos.yaml` or the Git root changes while
  the working directory, arguments, and environment stay the same. A CLI run
  already pins the whole bundle at start, so this matches CLI behavior.
- The pinned CLI path remains about 40% slower per render than v1.232.0. That
  overhead predates this PR and is awaiting a decision on whether to address it
  here.
