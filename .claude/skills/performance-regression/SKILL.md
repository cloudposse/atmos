---
name: performance-regression
description: "Investigate Atmos CLI slowdowns, interpret benchmark warnings, and add workloads that detect documented performance regressions, especially describe affected. Use for performance investigations and benchmark coverage; ordinary CI failures belong to fix-all."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Performance Regression

Establish whether the reported workload regressed, which operation multiplied,
and whether the benchmark can distinguish the known-good and known-bad builds.
Preserve the user's requested scope: an investigation does not authorize
production fixes, publishing reports, or cloud operations.

## Evidence and workload selection

Search [docs/fixes](../../../docs/fixes/) for the affected command, subsystem,
and symptoms. Use the relevant fix logs as the source for incident history,
reproduction conditions, and known-good/bad revisions; verify those details
against the implementation and current behavior.

Record the command, flags, scale, configuration shape, and external operations
that trigger the slowdown. Reproduce the reported command itself: a faster
neighboring command does not establish that the affected path improved.

Use the suite coverage and field-test results in
[magefiles/README.md](../../../magefiles/README.md) to select or extend a
workload. Preserve the conditions that reach the expensive path, verify expected
outputs outside timing, and count repeated operations where useful. State which
parts of the original workload a local fixture cannot reproduce.

## Measure through Mage

Keep automation in `magefiles`, benchmark workloads in Go, and scratch results
under `.context/` or `build/benchmarks/`. The maintained command contract is in
the Mage README; do not add parallel shell orchestration to the skill.

Collect a new candidate with `go tool mage bench:run <candidateDir>`.
Builds and setup stay outside measurements. Do not run compilation, tests, or
other heavy work concurrently with timed samples. CLI samples use fresh processes
and isolated caches; harness allocations are not CLI memory measurements.

## Retrieve historical benchmarks

Stored measurements are GitHub Actions artifacts from the
[Benchmarks workflow](https://github.com/cloudposse/atmos/actions/workflows/benchmarks.yml).
For an existing PR comparison, follow the sticky comment's measurements link
and download that run's `benchmarks-<runID>-<attempt>` artifact into
`build/benchmarks/candidate/`, preserving `bench.txt` and `manifest.json` together.
Use its recorded `BaseSHA` as `<main-base-sha>` below.

With `GITHUB_TOKEN` available, retrieve history and compare through Mage:

```console
go tool mage bench:fetch cloudposse/atmos <main-base-sha> build/benchmarks/history
go tool mage bench:compare build/benchmarks/history build/benchmarks/candidate build/benchmarks/report
```

Fetch expects the sibling `candidate/` directory; set
`ATMOS_BENCH_CANDIDATE_DIR` when using another location. The history directory
must be empty. Fetch selects up to ten compatible, successful main snapshots
reachable from that immutable SHA and preceding the candidate's cutoff. PR
artifacts never qualify as main baselines. A local candidate needs a compatible
runner, CPU, toolchain, build settings, and workload; a Mac run cannot compare
to Linux CI snapshots.

Inspect `report.md`, `report.json`, and each `benchstat-*.txt` in the report
directory. Downloaded snapshots retain their measurements and provenance;
`history/unavailable.json` explains skipped or unavailable history. Preserve
compatibility checks and sample counts. Every snapshot is compared independently
to the same candidate measurements.

Artifacts expire after 90 days, and history only starts accumulating after the
workflow is deployed. Missing or expired results mean comparison unavailable;
malformed or incomplete measurements are failures. When stored history does not
exist, explicitly rebuild selected revisions as a field test:

```console
go tool mage bench:fieldTest <good-ref>,<bad-ref>,<fixed-ref> .context/performance-field-test
```

The field-test target uses the same current CLI harness for each build. Select
the relevant workload with `ATMOS_BENCH_FIELD_FILTER`; see the Mage README for
supported settings. Use fresh output directories. Label rebuilt results as
field-test evidence; do not present them as historical CI snapshots.

## Interpret and close the loop

Report medians, percentage changes, sample counts, significance, backend-read
counts when relevant, and fixture/environment limitations. The advisory rule is
at least a 10% median increase with `p < 0.05`; read the actual configured threshold.
The newest compatible snapshot drives the headline; warnings against older
snapshots still matter and are correlated, not independent confirmations.

If a known slowdown is absent, inspect workload coverage before concluding it
was fixed. Demonstrate that the benchmark distinguishes the known-good and
known-bad builds. Preserve residual warnings after a large improvement, and do
not relax a budget merely to hide a regression.

When fixes are in scope, retain output/caching correctness checks, then remeasure
the same workload. Add the verified reproducer to the curated suite and its
required inventory; record incident-specific findings in `docs/fixes` and update
the suite coverage documentation. Keep that history out of this skill.
Performance warnings are advisory; do not manually set Quality Gate labels or
rewrite the reporter's sticky comment to make an investigation appear complete.
