# Mage targets

Atmos keeps repository automation in Go-based [Mage](https://magefile.org/)
targets. Run targets from the repository root with the repository-pinned Mage
version:

```console
go tool mage -l
go tool mage <target> [arguments]
```

The target list and per-target usage are generated from the Go declarations:

```console
go tool mage -h <target>
```

## Package layout

`magefile.go` is a thin `//mage:import` entrypoint. Targets and their tests live
in topic packages: `acceptance/` (also coverage), `bench/`, `build/`, `ci/`,
`lint/`, `notice/`, `qualitygate/`, `release/`, `s3/`, and `test/`. Shared
repository/process helpers, GitHub data types, and test fixtures live under
`internal/`. Command names and the `lint:changed` default are unchanged.
Imported target implementations do not carry a `mage` build tag: Mage compiles
them through normal Go imports. Their tests retain the tag so the dedicated
Mage CI job owns their coverage and race instrumentation. Run their full tests with `go test -tags=mage ./magefiles/...`.

## Target catalog

| Target | Arguments | Purpose |
| --- | --- | --- |
| `acceptance:precompile` | `<targetValue> [-outputDir=<string>]` | Precompile the acceptance-test binaries for one target. The output directory defaults to `build`. |
| `acceptance:run` | `<modeValue> <targetValue>` | Run one acceptance-test shard selected by `ATMOS_TEST_SHARD` and `ATMOS_TEST_SHARD_COUNT`. |
| `acceptance:verify` | `<targetValue> <shardCount> [-binaryDir=<string>]` | Verify that the acceptance workflow matrix and package assignments are complete and unique. The binary directory defaults to `build`. |
| `build:binary` | `<target> <version>` | Build Atmos for `default`, `linux`, `windows`, `macos`, or `macos-intel` and stamp the version and Git commit. Empty values default to `default` and `test`. |
| `ci:checkshardresults` | `<repo> <runID> <runAttempt> <check>` | Wait for all acceptance shards behind a required check and fail unless all succeed. |
| `ci:classifyinfrafailures` | `<repo> <runID> <runAttempt>` | Classify unsuccessful GitHub Actions jobs as infrastructure failures or real failures. |
| `ci:reruninfrafailures` | `<repo> <runID> <event> <headSHA> <prNumbers>` | Rerun failed or cancelled jobs after verifying that a pull request run is still current. |
| `ci:vulncheck` | `<output>` | Scan every package with the installed `govulncheck` and write a SARIF report to `output`. Exact duplicate stacks, which GitHub code scanning rejects, are removed; a failed scan or malformed output fails the target and leaves an existing report untouched. |
| `coverage:collect` | none | Run the packages in `TEST` with native subprocess coverage and merge the counters. |
| `coverage:merge` | `<inputs>` | Merge a comma-separated list of native Go coverage directories. |
| `coverage:mergeshards` | none | Merge all downloaded Linux coverage shards. |
| `lint:changed` | none | Run the complete changed-files lint chain: module validation, custom golangci-lint build, and pre-commit lint. This is the default target. |
| `lint:customgcl` | none | Build the custom golangci-lint binary when it is missing or stale. |
| `lint:gomodcheck` | none | Reject root-module `replace` and `exclude` directives that would break consumer installs. |
| `lint:lintroller` | none | Build the Atmos lintroller when stale and analyze all non-testdata packages. |
| `lint:precommit` | none | Run the prebuilt custom golangci-lint binary against files changed from `origin/main`. |
| `notice:generate` | none | Regenerate `NOTICE` from Go dependency licenses. |
| `release:summarizenotes` | `<repo> <releaseID>` | Condense a draft GitHub release's pull-request notes, optionally with OpenAI summaries. |
| `s3:deploy` | `<localDir> <s3URI>` | Deploy a static site with explicit content metadata and content-hash incrementality. Unchanged deployments make no S3 write requests. |
| `test:race` | none | Run all non-acceptance packages with the race detector and shuffled test order. |
| `test:racematrix` | none | Emit the GitHub Actions matrix that partitions race-test packages across shards. |

## Environment variables

Only variables that change target behavior are listed here. Standard Go and
GitHub Actions variables continue to work normally.

| Variable | Targets | Behavior |
| --- | --- | --- |
| `ATMOS_TEST_SHARD`, `ATMOS_TEST_SHARD_COUNT` | `acceptance:run` | Select the one-based acceptance shard and total shard count. `TEST_SHARD_COUNT` is the count fallback. |
| `TEST`, `TESTARGS` | `coverage:collect`, `test:race` | Override the package list and additional `go test` arguments. |
| `COVERAGE_DIR`, `COVERAGE_DATA_OUT`, `COVERAGE_OUT`, `COVERAGE_SHARDS_DIR`, `GO_TEST_TIMEOUT` | `coverage:*` | Override coverage input, intermediate, output, and timeout paths. |
| `TEST_SHARD_COUNT` | `coverage:mergeshards`, `ci:checkshardresults` | Set the expected acceptance-shard count. |
| `ATMOS_BUILD_NO_CACHE` | `build:binary` | Set to `true` to force package recompilation without deleting the shared Go cache. |
| `GITHUB_OUTPUT`, `GITHUB_STEP_SUMMARY` | `ci:*`, `test:racematrix` | Write structured workflow outputs and summaries. |
| `ATMOS_CI_STUCK_GAP` | `ci:classifyinfrafailures` | Override the infrastructure-zombie detection interval with a positive Go duration such as `300s`. |
| `GITHUB_TOKEN` | `ci:*`, `release:summarizenotes` | Authenticate GitHub API requests. |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `RELEASE_NOTES_DRY_RUN` | `release:summarizenotes` | Enable model summaries, select a model, and preview without updating the release. |
| `PROTECTED_PATTERNS` | `s3:deploy` | Newline-separated glob patterns that must not be deleted from the destination. |
| `RACE_SHARD_COUNT`, `ATMOS_TEST_RACE_SHARD_SEED` | `test:racematrix` | Set the number of race shards and deterministic shuffle seed. |
| `ATMOS_TEST_RACE_PARALLEL` | `test:race` | Override per-package test parallelism; defaults to `4`. |

## S3 deployment behavior

`s3:deploy` stores `.cloudposse-deploy-manifest-v1.json` in the destination.
The manifest records each managed object's SHA-256 digest, size, and content
type. On later runs, the target uploads only added or content/metadata-changed
files and deletes only removed managed files. Files matched by
`PROTECTED_PATTERNS` are never deleted. Retained protected objects remain in the
manifest, including objects discovered during bootstrap, so removing a pattern
later makes those objects eligible for cleanup again.

The first deployment uploads every managed file once with explicit metadata,
then lists the destination and deletes stale, unprotected remote objects.
Browser-significant types are selected by extension; the existing `mimetype`
magic-number library provides the fallback for unknown extensions. Textual
types receive explicit UTF-8 metadata, such as `text/html; charset=utf-8`. The
manifest is uploaded only after the deployment succeeds, so a failed run cannot
record an incomplete state as current.

`aws s3 sync` cannot append `charset=utf-8` selectively while preserving each
file's distinct MIME type: `--content-type` supplies one value for the entire
invocation. The Mage target computes and sends the exact header for each object,
so correct browser metadata does not require a second S3-to-S3 copy pass.

All S3 operations use the repository's existing AWS SDK for Go v2 dependency.
The target does not shell out to the AWS CLI or create temporary request files.
SDK calls preserve typed request metadata, built-in retries, context
cancellation, and per-object `DeleteObjects` error handling.

Callers must serialize deployments that target the same bucket and prefix. The
checked-in workflows enforce that contract with one production concurrency
group and one concurrency group per preview PR, with in-flight cancellation
disabled.

```console
PROTECTED_PATTERNS=$'previews/**\nrobots.txt' \
  go tool mage s3:deploy ./dist s3://example-docs/site
```

## Quality Gate and performance snapshots

Benchmark collection, snapshot retrieval, comparisons, and field tests live in
`magefiles/bench/`. Readiness checks and the composite publisher live separately
in `magefiles/qualitygate/`. The latter contributes `bench:report` to the
benchmark namespace so it can include performance results in the shared comment.
Benchmark code never imports or evaluates quality gates.

The **Quality Gate** maintains one bot-owned PR comment, matching job summaries,
and mutually exclusive `quality:ready` / `quality:not-ready` labels. Label
names, colors, and descriptions live in `.github/settings.yml`, alongside the
repository's existing labels and labels referenced by its automation.

A PR is ready when all requirements pass:

- The PR is marked ready for review. A draft PR is never ready, and the checklist
  shows this as its own **Ready for review** requirement.
- GitHub reports no merge conflicts with `main` (**No merge conflicts**). GitHub
  computes mergeability lazily after a push or base update; until it does, the
  requirement is reported as unavailable evidence rather than a failure, and the
  previous report is kept and retried. Other mergeable states such as blocked,
  behind, or unstable are not conflicts.
- All current PR workflows, check runs, and commit statuses have finished
  successfully (neutral and skipped CI results are allowed). Failures,
  cancellations, timeouts, pending results, and API errors block readiness.
  The latest run of each workflow counts, including pending reruns. The
  reporter's own workflows (`quality-report.yml`, `quality-reconcile.yml`) and
  `release-milestones.yml` background runs are not evidence: a queued reporter
  run replaced in its concurrency group is cancelled, and the report it would
  have written is the one being evaluated. Checks on
  the current synthetic merge commit are included as well as head checks.
  Duplicate app/name checks across suites and pages are selected by start time,
  with check IDs breaking ties. If a group has missing start times, IDs order
  that whole group; response order never determines which result wins.
- Codecov reports success for the **current head SHA**, using its existing
  configuration and thresholds. Missing, pending, or neutral coverage results
  do not pass. Results from previous commits cannot satisfy this requirement.
- CodeRabbit has an active approval on the current head SHA.
- Every PR commit has a GitHub-verified signature.
- Every review thread is resolved or belongs to a dismissed review. Outdated
  unresolved threads still count. General conversation comments have no GitHub
  resolution state and are not included.
- Fewer than 150 files changed (149 passes; 150 fails).
- No benchmark report processing error is recorded for the current head.

Benchmark report recovery resolves PRs whose head commit exists only in a fork
through the fork's head branch, because GitHub's commit association endpoint
lists nothing for them. If a completed benchmark run still has no PR to publish
for, the gate is published with performance pending and a workflow warning is
printed, instead of silently doing nothing.

Performance slowdowns are advisory and do not block this gate. Benchmark
execution failures still count as CI failures. Malformed or incomplete benchmark
reports refresh the other gates, show “comparison unavailable,” and block readiness.
Reconciliation retries failed reports and continues through other PRs before
returning aggregated errors. Missing historical baselines do not block readiness.
This automation does not modify branch protection or merge PRs.

### Adding a quality gate

Add one `gateDefinition` to `defaultQualityRegistry` in
[`qualitygate/registry.go`](qualitygate/registry.go), plus the check and its tests.
The definition provides a stable `ID`, display `Name`, `Check`, PR-relative
`EvidencePath`, and trusted `Remediation` instructions. Set `Advisory: true`
only for a check that should warn without blocking readiness. For example:

```go
gateDefinition{
    ID: "example", Name: "Example requirement", Section: "Measures",
    Check: (*qualityClient).example, EvidencePath: "/checks",
    Remediation: "Inspect the evidence and fix the unmet requirement.",
}
```

The registry runs checks in registration order and supplies the report links,
job summary, label verdict, and dynamic agent instructions. Duplicate IDs or
incomplete definitions fail validation; check errors fail closed. Do not copy
third-party check text into remediation instructions. Gates are compiled from
trusted repository code; artifacts cannot register executable plugins.

Checks should return a concrete `Problem` and state-specific `NextAction` for
unmet gates. Pending CI calls for waiting; a failed job calls for inspecting its
logs; absent Codecov evidence must not be described as failed coverage. `Signals`
name the actual blocking results and are quoted as evidence, never instructions.
`Remediation` is the fallback for simple gates. Keep raw check/review text out of
`Problem` and `NextAction`; construct those from trusted policy and parsed state.

| Target | Behavior |
| --- | --- |
| `bench:run <outputDir>` | Build Atmos, prepare offline fixtures, collect raw measurements and a provenance manifest. Requires a fresh output directory. |
| `bench:fetch <repo> <ref> <outputDir>` | Fetch up to 10 compatible, successful main snapshots reachable from the immutable 40-character main SHA. Candidate defaults to the sibling `candidate` directory. |
| `bench:compare <snapshotsDir> <candidateDir> <outputDir>` | Compare the same candidate independently with each snapshot; write `report.md`, `report.json`, and separate benchstat output for each pair. |
| `bench:report <repo> <runID>` | Validate a completed PR benchmark run, download data, regenerate comparisons with trusted code, and update the composite comment, summary, and labels. |
| `quality:report <repo>` | Reconcile quality gates only for PRs associated with the Actions event (including fork and stacked PRs): a release label or milestone change, or CI completion for their head. Skips repository-wide events and never falls back to every open PR. Recover missing benchmark reports. |
| `quality:reconcile <repo>` | True up quality gates after a repository-wide event. Scheduled runs select open PRs with a head commit from the last 24 hours or no final report for their current head and base, plus `quality:ready` PRs whose head now has a failing commit status (for example `Release milestone`). Merges, milestone and release changes, and manual runs reconcile all open PRs targeting this repository, regardless of base branch. Refuses per-PR events: release label and milestone changes on an open PR, and CI completion. |

Run these from the repository root with `go tool mage`, for example:

```console
go tool mage bench:run build/benchmarks/candidate
go tool mage bench:fetch cloudposse/atmos <main-sha> build/benchmarks/history
go tool mage bench:compare build/benchmarks/history build/benchmarks/candidate build/benchmarks/report
```

`GITHUB_TOKEN` authenticates fetch/report operations. `ATMOS_BENCH_CANDIDATE_DIR`
overrides the candidate location for fetch. `ATMOS_BENCH_SAMPLES` sets 10–100
samples (default 10); `ATMOS_BENCH_THRESHOLD` sets the positive regression
percentage (default 10). `ATMOS_BENCH_BASE_SHA` records the event's immutable main
base. `GITHUB_STEP_SUMMARY` receives the same Markdown as the sticky comment.

The expandable agent prompt contains instructions for the gates currently
failing, with advisory performance investigations listed separately. It uses
trusted remediation text rather than copying third-party check output into
instructions.

### Measurement and comparison contract

The suite includes merging, deep copying, templates, YAML resolution, affected
detection, instance listing, repeated locals, and 2,200 deferred store lookups
over 11 keys, asserting exactly 11 backend reads. CLI fixtures exercise shared
imports, inheritance, locals, templates, and offline YAML functions.
`TemplateProcessingComplex` measures date, random, arithmetic, and base64 templates
without auth paths, including the overhead of credential-source protection.
Auth-containing source shapes have separate guard and renderer correctness tests;
this workload does not measure credential authentication or backend latency.
`describe affected` covers no changes, shared catalog changes, and component
source changes at 100, 1,000, and 10,000 instances. `describe stacks` and
`list instances` cover 1,000 instances. Expected output is checked outside the
timed region. Fixtures use local Git worktrees and `--repo-path` without fetching.
An additional 1,000-instance catalog-change workload uses `--include-dependents`
with roots, branches, and leaves across ten stacks. It validates the entire
nested dependency tree and exercises the resolution path used by Pro uploads
without uploading anything.

The `FunctionsRepeated/instances=200` affected workload adds 600 references per
pass to one static target through `!terraform.output`, `!terraform.state`, and
`atmos.Component`. Consumers use one lookup kind each, so a stale value in one
path cannot be hidden by another path detecting the change. Scenarios cover
unchanged repositories, a changed referenced value, and the same change with
`--include-dependents`. Checks outside timing assert resolved values in HEAD and
BASE, exact affected instance names, and all 200 dependents. Masking stays enabled
at its default; static backends need neither cloud access nor Terraform. This
covers inspection caching and repeated deferred target descriptions. The
non-static output/auth-disabled wrapper path and actual OIDC authentication need
separate unit tests; static backends return before that wrapper is created.

The `StoreCallerStacks/stacks=20/keys=11` workload spreads 200 consumers across
20 caller stacks, changes their shared component, and enables dependents with
`--process-functions=false --skip=store.get --skip=store`. Unlike the default
CLI harness, it leaves identity selection implicit and uses fake ambient AWS
credentials against a loopback SSM server. It verifies resolved store values,
all 200 affected identities, and exactly 22 reads (11 per HEAD/BASE root).
This detects caller-stack cache fragmentation for stores without configured
auth identities; it does not model a configured default identity or real AWS latency.

Builds and fixture setup are excluded from measurements. Packages run
sequentially with `GOMAXPROCS=2`, CGO disabled, no race/coverage instrumentation,
and a one-second calibration for in-process benchmarks. Every CLI sample uses
one fresh process with isolated home/configuration/application caches, no cloud
credentials, and authentication, telemetry, version checks, and Pro disabled
except for the ambient-store scenario described above.
CLI results report elapsed time only; harness allocations are not CLI memory.
In-process results also include bytes and allocations per operation.

The manifest records source SHA, run/attempt, timestamp, Go/build settings,
runner image family, CPU, benchmark inventory, and source/fixture fingerprints.
Changes to a package's benchmark definitions invalidate that package's
comparisons. Different toolchains, CPUs, runner environments, or build settings
are incomparable. Added, removed, changed, and missing metrics are reported
explicitly. Raw data must contain the declared sample count for every metric;
malformed or incomplete results fail the reporter.

Each baseline-to-PR pair is analyzed independently with pinned `golang.org/x/perf`
benchstat/benchmath. A median increase of at least 10% with `p < 0.05` is flagged;
a statistically significant increase from zero is also flagged. The newest
compatible snapshot determines the headline, while older warnings remain
visible individually. These comparisons reuse the same PR samples and are
correlated; cross-run timing variability can produce noise. Inspect repeated
measurements and full reports before attributing a change to code.

### Workflows, trust, and rollout

`Benchmarks` measures the event's immutable PR merge revision, main pushes, and manual dispatches
on `ubuntu-24.04` using repository-pinned Go and existing cache restoration.
For PRs, `SHA` identifies the measured merge revision, `PRHeadSHA` separately
identifies the PR head, and `BaseSHA` records the merge revision's first parent
(the main commit actually measured; `github.event.pull_request.base.sha` can lag it).
The trusted reporter verifies the merge's two parents against the recorded base
and workflow's PR head, then selects snapshots reachable from that base.
Older snapshots can also reflect intervening main changes. Stale-head and fork
association checks continue to use the PR head, not the measured merge SHA.
Superseded PR runs cancel; main runs finish. Measurement artifacts use unique
run/attempt names and 90-day retention; partial measurements remain available
for diagnostics but never qualify as baselines.

Storage uses GitHub Actions artifacts and the built-in `GITHUB_TOKEN` only;
no dedicated credentials or Git object archive are required. Reporters retain
read-only repository-content permissions. Expired history is unavailable, not
silently rebuilt; use the explicit historical field-test target when needed.

Snapshot selection is frozen at the candidate workflow's creation time: only
successful main runs completed before that cutoff and reachable from the
recorded main base SHA qualify. Later runs, PR artifacts, expired artifacts,
and duplicate source SHAs are excluded. Selection searches up to 1,000 runs,
newest first, for ten compatible snapshots. Empty history reports
“comparison unavailable”; history bootstraps naturally after merge without
rebuilding old commits.

`CI Timing Summary` is the single automatic completion coordinator: it checks
that the latest PR workflows and their jobs have finished, posts its timing
comment using the existing TypeScript stopwatch action, then calls the reusable
`Quality Gate` workflow (`quality:report`) whenever the timing coordinator succeeds, even if
stopwatch defers publication because CI is still running.
The Go reporter publishes its own separate quality/benchmark comment. Only the
workflow scheduling is shared; no timing content is passed into Go. Failed or
canceled completed runs still produce reports. The standalone `Benchmark Report` workflow
is manual-only for explicit recovery by run ID.

All reporters execute the default branch's trusted code with scoped API
permissions. Neither uses `pull_request_target`. Downloaded PR artifacts are
bounded, validated data files, never code or executables. PR association includes
forks. The publisher owns only its marked bot comment, rechecks the head before
writing, rejects older benchmark runs/attempts, and preserves benchmark results
only for the current head. Stopwatch continues to manage its own timing comment.

The read-only `Quality Gate Signal` forwards PR/review activity to the same
completion coordinator. Quality Gate no longer independently subscribes to
workflow completions, checks, statuses, or comments. The separate
`Quality Gate Reconcile` workflow (`quality:reconcile`) runs a 15-minute best-effort
schedule that recovers missed events and late external results, including Codecov and
review-thread resolution. It selects open PRs targeting this repository, including
stacked PRs, whose head commit
(the later of author and committer date) is within the last 24 hours (inclusive),
plus any PR with no final gate report for its current head and base, a pending
CI snapshot, an active current-head workflow, or a gate whose evidence was
unavailable, at any age. Active workflows are checked before either age cutoff
so a missed rerun event cannot preserve stale readiness. Each report
records the evaluated base branch and SHA, so advancing a prerequisite or
retargeting the PR triggers reconciliation even when its head is unchanged. GitHub's
`updated_at` is not used as an activity signal because the gate's own labels and
comments, Mergify, CodeRabbit and Codecov advance it for nearly every PR; it only
skips the commit lookup for PRs not updated since the cutoff. PRs without a
usable report are reconciled first, rotated by schedule slot so a PR that hangs
or fails every run cannot keep the PRs behind it from being visited.
Event-associated reports are not restricted by age.
Commits that exist only in a fork are resolved through the fork's head branch
(`workflow_run` payloads) or a commit SHA search, and a PR matches only while its
head is exactly that commit. Manual runs select all open PRs targeting this
repository, regardless of base branch.
While Actions workflows are pending, reconciliation publishes one initial
snapshot for the current head/base revision and clears stale readiness. Later
completions retain that snapshot until CI finishes, then publish the full report
regardless of success or failure. A same-head rerun starts another pending cycle.
Benchmark recovery also waits for CI completion. Completion of Actions workflows
does not imply that external checks have finished or passed.

Each PR is reconciled under its own deadline, so one slow or failing PR cannot
starve the PRs behind it. Every failure is logged as a workflow warning
annotation; the run exits non-zero only when every attempted PR failed (a
systemic problem such as credentials or an outage). Isolated failures are retried
by the next scheduled run. When the run budget ends, unvisited PRs are left for
the next run. If a gate's evidence cannot be read (for example an API error),
the existing report for that head and base, its comment and its label, is kept and the
run reports the failure, instead of publishing "not ready" and flipping the
label back when the next run succeeds. A report for a different base cannot
retain readiness: the new report records unavailable evidence until it can be
evaluated. Known incomplete CI also clears stale readiness even when another
gate's evidence is unavailable.

Unchanged readiness labels and report comments are not rewritten. A different
reporter-run summary URL alone does not count as a change: the comment keeps its
previous summary link while each publishing pass still writes a job summary.
Reports group checks into Readiness, Reviews, and Measures. Each scheduled
run logs its UTC cutoff and selected PR count. Per-PR reports serialize in a
job-level group per PR, so skipped label events never enter it; the true-up has
its own workflow and group, so a long reconcile never cancels a queued PR report.
Publication freshness
checks protect overlapping writers. Labels and comments are eventually consistent
with GitHub events and are not a replacement for required checks. The schedule
bounds activity by head-commit recency, not a fixed request budget, and the
completion coordinator still wakes on relevant workflow completions.

These trusted reporters become active after their workflows and Mage code reach
the default branch. No compatible main snapshots exist until collection has run
there. Benchmark and signal workflow completions participate in the CI timing
coordinator.

### Historical field tests

When no stored snapshots exist, explicitly rebuild known-good and known-bad
versions with the **same current CLI harness**, rather than comparing whichever
benchmarks happened to exist in each old checkout:

```console
go tool mage bench:fieldTest v1.229.0,v1.230.0,v1.230.1,v1.231.0,origin/main .context/field-store
```

The first comma-separated ref is the reference for every subsequent version.
Refs resolve to immutable SHAs before building. All builds finish before
measurements begin, using the same local Go toolchain (`GOTOOLCHAIN=local`),
CGO disabled, and `GOMAXPROCS=2`. The current harness drives the historical
binaries; no old test files need to exist or compile. Build worktrees are
removed afterward. Binaries, build logs, raw measurements, manifests, and
per-version Markdown/JSON/benchstat reports remain under the output directory.
The directory must be empty. Use a local toolchain new enough for all refs.

The default field test reproduces #3353: 200 component instances each reference
11 store keys (2,200 template references per processing pass). A loopback SSM
endpoint returns fixed values with 1 ms latency per request. This exercises the
real CLI, template evaluation, deferred authentication path, SDK client setup,
and cache, with synthetic credentials and no AWS access. Both `describe affected`
and `describe stacks` report elapsed time and **backend reads**, never harness
memory. The fixed CLI may read each key in multiple processing contexts; normal
CI bounds those reads at the current main budgets of 44 and 22 respectively.
Comparisons still flag increases against older snapshots with fewer reads;
the assertion budget is not a claim that current main is optimal. The isolated in-process
benchmark separately asserts exactly 11 reads for 2,200 calls in one context.
Field tests observe excessive reads instead of aborting, so the known-bad build
can produce statistics and demonstrate detection.

`ATMOS_BENCH_FIELD_FILTER` selects another CLI benchmark expression; for example,
`^BenchmarkCLI$/^DescribeAffected$/^instances=1000$` replays the three 1,000-instance
scenarios, and `^BenchmarkCLI$` runs the entire CLI suite. For repeated function
lookups use `^BenchmarkCLI$/^DescribeAffected$/^FunctionsRepeated$`. Append
`/^instances=200$/^change=none$` to isolate timing on historical releases that
fail the changed-value correctness checks; those failures must remain visible
and must not be interpreted as a passing performance result. Sample count and
regression threshold use the ordinary benchmark environment variables. Historical
field-test manifests record the measured revision, source ref, harness revision,
and workload fingerprints, and are deliberately incompatible with CI snapshots.
This is manual validation, not automatic baseline rebuilding or artifact seeding.

The October 10, 2026 local field test detected the known regression in every bad
release. Ten fresh invocations per command/version used Go 1.27.1, macOS on an
Apple M4 Max, `GOMAXPROCS=2`, and the fixed 1 ms loopback delay:

| Revision | Affected median | Affected backend reads | Stacks median | Stacks backend reads |
| --- | ---: | ---: | ---: | ---: |
| v1.229.0 (`4ffd8804d350`) | 0.544 s | 11 | 0.363 s | 11 |
| v1.230.0 (`e4269500304c`) | 9.206 s | 4,488 | 4.669 s | 2,244 |
| v1.230.1 (`67c41025906d`) | 9.395 s | 4,488 | 4.639 s | 2,244 |
| v1.231.0 (`8722c8ec8359`) | 9.952 s | 4,488 | 3.948 s | 2,244 |
| main (`6b85a0074f9c`) | 0.601 s | 44 | 0.359 s | 22 |

All three bad releases triggered the advisory timing rule against v1.229.0
for both commands (`p < 0.00002`). Main removed most of the slowdown, but
`describe affected` still triggered a **10.7% increase, p=0.0015** against the
good release. Its backend reads increased fourfold; stacks reads doubled even
though stacks timing showed no significant increase (`p=0.97`). These remaining
warnings are intentionally preserved. This synthetic local experiment proves
detection of #3353; it does not estimate production latency or establish Linux
runner noise. A separate unchanged-build check (10 versus 10 CLI samples)
showed about 2% timing variation with `p=0.91` on this machine.

The remaining read counts are consistent with the fix's deliberate cache key:
caller stack and configuration paths are included to preserve authentication
isolation. Two stacks explain 22 reads for stacks; two repository contexts
explain 44 for affected. The measured increase warrants an advisory, but it does
not by itself establish another cache bug or justify removing that isolation.

A second ten-sample field test compared the same revisions without store reads,
using `ATMOS_BENCH_FIELD_FILTER='^BenchmarkCLI$/^DescribeAffected$/^(instances=1000|Dependents)$'`.
It found no timing regressions in the additional 1,000-instance paths:

| Affected workload | v1.229.0 median | Main median | Change | p-value |
| --- | ---: | ---: | ---: | ---: |
| No changes | 1.721 s | 1.641 s | -4.62% | 0.0115 |
| Shared catalog change | 1.689 s | 1.671 s | -1.08% | 0.7394 |
| Component source change | 1.732 s | 1.653 s | -4.54% | 0.00049 |
| Catalog change with nested dependents | 2.489 s | 2.403 s | -3.48% | 0.0232 |

These results narrow the remaining warning to the store-heavy fixture among the
paths measured here; they do not prove every affected-detection path is faster.

### Coverage checklist from `docs/fixes`

A workload being present does not prove it detects every regression. Validate a
known-good/bad pair before claiming historical detection, and keep provider or
runner-specific gaps explicit.

| Documented problem | Workload / current coverage |
| --- | --- |
| [Inspection and deferred component function caches](../docs/fixes/2026-10-13-describe-affected-function-lookup-cache.md) | 200 consumers / 600 references per pass across output, state, and Component, static backend with default masking; unchanged, changed referenced values, and dependents. HEAD/BASE and complete result assertions guard cache correctness. No cloud/OIDC or non-static output executor latency. |
| [Deferred store lookups, #3353](../docs/fixes/2026-10-09-deferred-store-template-lookup-cache.md) | Real CLI against loopback SSM, 2,200 references / 11 keys, plus in-process backend-read assertion. Field-test release sequence above verifies the regression and recovery. |
| [Shared inheritance and cache contention](../docs/fixes/2026-05-23-describe-affected-component-inheritance-perf.md) | Shared imports and two inheritance levels at 100/1,000/10,000 instances; repeated locals extraction; template and YAML-resolution microbenchmarks. |
| [Native merge and deep copies](../docs/fixes/2026-03-19-deep-merge-native-fixes.md) | Curated deep-copy, two/five-input merge, and production-scale merge benchmarks; inherited CLI fixtures. |
| [Repeated stack-name resolution](../docs/fixes/2026-03-20-executedescribestacks-refactor.md) | `name_template` on every generated stack, with many components per stack; `describe stacks` and `list instances`. |
| [Dependent discovery explosion](../docs/fixes/2026-03-24-describe-affected-upload-timeout.md) | `--include-dependents` on a 1,000-instance catalog change, with validated two-level dependency trees and repeated component names across stacks. Pro upload packing/network remains outside this workload. |
| [Per-component auth scoping](../docs/fixes/2026-06-22-describe-stacks-scope-and-cache-per-component-auth.md) | Explicit gap: ordinary CLI fixtures disable cloud authentication; this requires a separate counting auth-provider fixture. |
| [SSO account-name pagination/cache](../docs/fixes/2026-10-09-sso-account-name-pagination-cache.md) | Explicit gap: real SSO login and persistent credential-cache behavior are outside the isolated-cache CLI suite; existing provider regression tests cover correctness. |
| [Windows runner caches](../docs/fixes/2026-09-04-windows-go-caches-on-fast-disk.md), [Defender](../docs/fixes/2026-09-04-windows-defender-exclusions-are-a-noop.md), [worktree fsmonitor](../docs/fixes/2026-08-24-fsmonitor-worktree-saturation.md) | Infrastructure performance, outside Linux CLI runtime comparisons. Build and fixture preparation are intentionally excluded from benchmark timing. |
