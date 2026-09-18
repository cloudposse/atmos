# Distribute toolchain race tests and reduce repeated downloads

The race suite previously placed the entire `pkg/toolchain` package on one of
four workers. A measured run reported 689.409 seconds in that package, with
450 passing top-level tests and a slowest individual test of 32.88 seconds.
The previous run reported 980.219 seconds. Package shuffling could move this
bottleneck between workers but could not divide its execution.

The planner now excludes only the exact toolchain root package. Every worker
runs a deterministic, duration-balanced subset of its discovered tests, then its
ordinary packages in a separate invocation. Toolchain subpackages retain their
existing package assignment. Failures in either invocation fail the worker;
ordinary packages still run if toolchain discovery or execution fails.

Discovery happens on workers with the same race build configuration, avoiding a
compilation barrier in the central planner. Selection is anchored at top-level
test names, retains all subtests, includes runnable examples and fuzz seeds,
and automatically includes new tests. A missing timing manifest uses equal
weights; an empty selection never becomes an unfiltered run. Tests prove
complete non-overlapping assignment and exercise selection against real Go tests.

Both the legacy and split workflows keep four workers and their existing check
names. Each worker uploads its inventory, selection, raw JSON events, and timing
summaries. Local unsharded `atmos test race` behavior is unchanged. To reproduce a
worker, set `ATMOS_TEST_RACE_SHARD`, `RACE_SHARD_COUNT`, and `TEST` to its ordinary
package list. `ATMOS_TEST_RACE_REPORT_DIR` optionally preserves timing output.

Seven repeated installation tests use local HTTP ZIP fixtures through the
existing configured-registry interfaces. They still execute real download,
extraction, alias resolution, default selection, batch dispatch, and version-file
updates. Assertions check installed bytes and resulting state. Install/skip/
reinstall cases use independent installer instances and parallel subtests; serial
public-adapter cases restore global configuration and isolate their cache paths.
The old install/skip test accepted download failures without asserting them; every
fixture case now requires success and verifies whether the existing binary was
preserved or replaced.

Live registry/latest-version tests remain, as do installer checksum/signature
verification and concurrent batch tests. No production toolchain APIs changed.
Sharding does not replace tests that exercise shared resources concurrently
inside one process. The remaining global adapters are deliberately serial.

The new package distribution and local fixtures need hosted timing validation;
local fixture timings are not a prediction of end-to-end CI time. Cold race
compilation, runner/cache setup, other large packages, and macOS/Colima can still
dominate the pipeline. See `.github/test-timings/README.md` for artifact details
and the timing refresh procedure.
