# Research: Atmos Pro Command-Execution Metadata Upload

**Feature**: 002-pro-exec-metadata
**Date**: 2026-08-11

---

## Decision 1: New package for the feature (`pkg/proexec`)

**Decision**: A new package `pkg/proexec` owns the CI+Pro gate check, the execution-record
envelope assembly, resource-metrics collection orchestration, and both the async and sync
delivery paths. It depends on `pkg/pro` (client/DTOs), `pkg/telemetry` (CI detection),
`pkg/metrics/process` (new, resource metrics), and `pkg/git` (repo info).

**Rationale**: `internal/exec/pro.go` already holds Pro-upload glue for `uploadStatus`/
`shouldUploadStatus`, but it is scoped to the terraform plan/apply drift-status upload and
lives inside `internal/exec` (business logic for terraform), not a general-purpose,
reusable capability every command can call. Per CLAUDE.md's package-organization mandate
("new functionality gets its own purpose-built package"; "avoid utils package bloat"),
this feature is genuinely cross-cutting (every command, not just terraform) and belongs in
its own `pkg/` package, following the precedent of `pkg/pro/`, `pkg/git/`, `pkg/telemetry/`.

**Alternatives considered**:
- Extend `internal/exec/pro.go` — rejected: that file is terraform-specific business
  logic; the async default path must be reachable from `cmd/root.go` for *every* command,
  which `internal/exec` is not an appropriate dependency for (it would invert the
  cmd → internal/exec dependency direction for the common case).
- Extend `pkg/telemetry` — rejected: telemetry sends anonymous, opt-out usage data to
  PostHog; this feature sends CI-execution data to Atmos Pro, a distinct sink with
  different content and different consumer. Conflating them would make both harder to
  reason about, and telemetry's opt-out semantics don't apply here.

---

## Decision 2: Hook points — async default vs. synchronous allowlist

**Decision**: Two distinct call sites:
1. **Async default**: `cmd/root.go`, immediately after the existing
  `telemetry.CaptureCmd(cmd, err)` call (line ~1905), add
  `proexec.CaptureAsync(cmd, err)`. This covers every command uniformly, mirroring the
  existing telemetry hook's placement exactly.
2. **Synchronous allowlist**: `terraform plan`, `terraform apply` (both already funnel
  through `internal/exec/terraform.go: ExecuteTerraform`), and `describe affected` call
  `proexec.CaptureSync(...)` directly and explicitly from their own execution path,
   *before* returning, passing whatever command-specific structured data they have
  already computed (e.g. `terraform plan`'s `plugin.OutputResult`).

`proexec.CaptureAsync`/`CaptureSync` both internally re-check the CI+Pro gate and no-op
if it doesn't hold, so call sites don't need to duplicate that check.

**Rationale**: The root-level hook is the only place that sees *every* command uniformly
(exactly where `telemetry.CaptureCmd` already lives), so it is the natural home for the
async default. But by the time `cmd/root.go` regains control, `terraform plan`/`apply`
have already returned — there is no way to block "command completion" from outside the
command's own execution path. So the synchronous allowlist necessarily requires an
explicit call inside each of those three commands' own code, which also matches the
spec's assumption that the sync/async classification is "fixed, code-defined" per command
rather than data-driven.

**Alternatives considered**:
- A `PersistentPostRunE` per sync command — rejected: `ExecuteTerraform` is a shared
  pipeline for all terraform subcommands; the natural point to attach synchronous
  behavior is inside that pipeline right after plan/apply output is parsed, not by adding
  a new Cobra lifecycle hook layer.
- Single unified hook with a "wait if sync-listed" branch in `cmd/root.go` — rejected: by
  the time `internal.Execute(RootCmd)` returns, the sync command has already completed and
  its own success/failure has already been decided; the record-delivery outcome could no
  longer influence FR-008's fail-vs-warn decision for that command.

---

## Decision 3: Resource-usage metrics (`pkg/metrics/process`, new package)

**Decision**: New package `pkg/metrics/process` exposes:
```go
type ProcessMetrics struct {
    WallTime         time.Duration
    UserCPUTime      time.Duration
    SystemCPUTime    time.Duration
    MaxRSSBytes      int64 // 0 on platforms without support
    MinorPageFaults  int64
    MajorPageFaults  int64
    InBlockOps       int64
    OutBlockOps      int64
    VolCtxSwitches   int64
    InvolCtxSwitches int64
}

func Baseline() Snapshot        // captured once, as early as possible in process lifetime
func (s Snapshot) Since() ProcessMetrics // diff against current usage
```
It measures the **current (`atmos`) process itself** — via `syscall.Getrusage
(RUSAGE_SELF, ...)` on Unix (Linux/macOS) and `GetProcessTimes`/`GetProcessMemoryInfo`
via `golang.org/x/sys/windows` on Windows — not a child/subprocess. Unix-only fields
(`MaxRSSBytes`, page faults, block I/O, context switches) are zero-valued (and omitted
from the JSON payload via `omitempty`) on Windows, where only `WallTime` and process CPU
times are available. `Baseline()` is captured once in `cmd/root.go`'s `Execute()` before
`internal.Execute(RootCmd)` runs, so the same baseline underlies both the async default
(diffed at the existing telemetry hook point) and any synchronous command's own call
(diffed at the point that command builds its execution record).

**Rationale**: The feature reports metadata about "every `atmos` command execution" —
i.e. the `atmos` CLI process's own resource consumption, not the terraform/tofu
subprocess it may shell out to (that is a separate, previously-proposed, still-unmerged
idea in PR #2217, which measured *subprocess* resource usage via `os.Process.SysUsage()`
after `cmd.Wait()`). Measuring the parent process directly needs a different technique
(`syscall.Getrusage(RUSAGE_SELF, ...)`, no child process handle involved) but the same
underlying `Rusage`-shaped data on Unix. A single baseline-and-diff pair works uniformly
for both the async and sync paths without needing per-command timer plumbing.

**Alternatives considered**:
- Reuse `pkg/process.Result` (existing `Runner`/`Result` types) — rejected: that package
  models *subprocess* invocations (`Runner.Run` wraps `os/exec`), which is a different
  target than the `atmos` process's own usage; extending it would conflate two distinct
  measurement subjects.
- Only wall time (skip Unix rusage entirely) — rejected: the spec's FR-004 explicitly
  requires CPU time at minimum and additional Unix metrics "whenever the host platform
  makes them available," and PR #2217 already established the field-naming precedent to
  follow.

---

## Decision 4: CI + Pro-configured gate

**Decision**: `proexec.gateOpen(atmosConfig *schema.AtmosConfiguration) bool` returns
`telemetry.IsCI() && proConfigured(atmosConfig)`, where `proConfigured` checks
`atmosConfig.Settings.Pro.Token != ""` OR (`GithubOIDC.RequestURL != ""` AND
`GithubOIDC.RequestToken != ""` AND `WorkspaceID != ""`) — i.e., "an
`AtmosProAPIClient` could plausibly be constructed," without actually constructing one
(and therefore without making a network call) just to decide whether to proceed.

**Rationale**: Reuses `telemetry.IsCI()` verbatim (already exported, already the
project's canonical CI-detection function) per the spec's explicit requirement to reuse
existing detection rather than build a new mechanism. The Pro-configured check mirrors
exactly the branching already in `pro.NewAtmosProAPIClientFromEnv` (static token vs. OIDC
path) without duplicating its side-effecting OIDC exchange call.

**Alternatives considered**:
- Always call `pro.NewAtmosProAPIClientFromEnv` and treat any error as "gate closed" —
  rejected: for the async default path (running on *every* command), this would trigger a
  real OIDC token exchange HTTP call on every single `atmos` invocation in CI, even for
  commands that end up not needing to upload anything — an unacceptable default-path cost.

---

## Decision 5: Command-specific structured data — explicit parameter, no registry

**Decision**: No interface, no registry. `proexec.CaptureSync` and `proexec.CaptureAsync`
accept an `any` parameter (`Data any`) for command-specific structured data; the calling
command's own code passes it directly (or `nil`). For `terraform plan`/`apply`,
`internal/exec/terraform.go` passes the already-computed `*plugin.TerraformOutputData`
(from `pkg/ci/internal/plugin`) it already builds today for Native CI job summaries. All
other commands pass `nil`.

**Rationale**: The spec's Assumptions section explicitly defers this decision to planning
and requires it not need user-facing configuration. With only two commands (`terraform
plan`/`apply`) producing structured data at launch, and the "sync vs. async" classification
itself already hardcoded per CLAUDE.md's Simplicity principle (no premature abstraction
for a hypothetical N-th consumer), a plain parameter is the simplest mechanism that
satisfies FR-005. A registry or new interface would add indirection with no current second
implementer to justify it — YAGNI.

**Alternatives considered**:
- New `CommandProvider`-adjacent interface (e.g. `ExecMetadataProvider`) — rejected:
  `CommandProvider` governs command *registration* (Cobra wiring), not runtime data flow
  during execution; conflating the two registries would blur an already load-bearing
  abstraction. Revisit only if a third or fourth command needs structured data and the
  duplication becomes real (not hypothetical).
- Central registry keyed by command path — rejected for the same YAGNI reason; two known
  producers do not justify a lookup table.

---

## Decision 6: New Atmos Pro API endpoint client method

**Decision**: `pkg/pro/api_client_exec.go` adds
`(c *AtmosProAPIClient) UploadExecMetadata(dto *dtos.ExecUploadRequest) error`,
`POST {BaseURL}/{BaseAPIEndpoint}/atmos/exec`. The base envelope (identity, git info,
`Metrics`, and any small always-present `Data` summary fields) is marshaled and sent via
the same `doWithRetry("UploadExecMetadata", ..., c, defaultRetryConfig())` →
`getAuthenticatedRequest` → `c.HTTPClient.Do` → `handleAPIResponse` shape as
`UploadInstanceStatus` whenever it fits under `MaxPayloadBytes` on its own (the common
case — most commands have no `Data` at all). When a command's structured data includes a
large, chunkable array (`dtos.ExecUploadRequest.DataItems []json.RawMessage` — e.g. the
combined per-resource change list for `terraform plan`/`apply`), `UploadExecMetadata`
instead calls `pro.sendChunked(dto.DataItems, c.MaxPayloadBytes, overhead, sendFn)`
exactly like `UploadAffectedStacks`/`UploadInstances` do: the full envelope (identity,
`Metrics`, small `Data` fields) is repeated on **every** chunk request so each one is
self-contained and independently retryable, and each request carries `BatchInfo`
(`batch_id`/`batch_index`/`batch_total`) for server-side reassembly. Added to
`AtmosProAPIClientInterface` alongside the other five methods.

**Rationale**: Clarification (spec.md, Session 2026-08-11) requires that command-specific
structured data is never truncated or dropped for size — only the (potentially large)
`DataItems` array is split across correlated requests, reusing the existing
chunked-upload/`BatchInfo` mechanism (`pkg/pro/chunked_upload.go`) already proven by
`UploadAffectedStacks` and `UploadInstances`, rather than inventing a second chunking
implementation. The base envelope and resource-usage metrics are small and bounded, so
they are always sent in full and never subject to chunking (FR-011). Reusing `doWithRetry`
per chunk gets 401-refresh-and-retry and 5xx-backoff-retry for free, matching FR-012, for
every chunk independently — a single failed chunk can be retried without re-sending the
whole record from scratch.

**Alternatives considered**:
- Client-side truncation with a `"... truncated"` marker (the original Decision 6) —
  rejected per clarification: silently dropping resource/output/warning data from the
  execution record defeats the purpose of User Story 3 (seeing exactly what changed
  without re-reading raw CI logs); a `terraform plan` touching thousands of resources
  must still report all of them, not a truncated subset.
- Chunking the *entire* `ExecutionRecord` (envelope + metrics + data) the way
  `UploadAffectedStacks` chunks its whole `Stacks` array — rejected: the envelope and
  metrics are a single indivisible identity for the invocation (there is exactly one
  exit code, one wall-time, one git SHA), not a repeatable per-item array; only the
  command-specific structured data can meaningfully be split into independently-postable
  pieces, per clarification.
- One `sendChunked` call per resource-action array (`CreatedResources`,
  `UpdatedResources`, `DeletedResources`, `ReplacedResources` sent as four separate
  batched uploads) — rejected: four independent batch sequences per record quadruples
  request count and `batch_id` bookkeeping for no benefit over one flat, chunkable
  `DataItems` list; `pkg/proexec` maps `TerraformOutputData`'s four/six resource-address
  slices into one `[]json.RawMessage` of `{action, address}` items before calling
  `UploadExecMetadata`.

---

## Decision 7: Synchronous wait timeout configuration surface

**Decision**: New `schema.ProSettings.Exec.SyncTimeoutSeconds int` (nested
`ExecSettings` struct), default `10` when unset/zero, bound via the existing
`viper`-in-`pkg/config/load.go` pattern already used for `settings.pro.token` /
`settings.pro.workspace_id` (env var `ATMOS_PRO_EXEC_SYNC_TIMEOUT_SECONDS`). Only
lengthens the wait — `CaptureSync` clamps any configured value below the 10s default up
to the default rather than allowing it to shorten (per the clarification: "increase in
configs," not decrease).

**Rationale**: Matches the existing `settings.pro.*` config surface exactly (same file,
same binding mechanism, same nesting convention as `GitSTS`/`GithubOIDC`). Keeping it a
pure timeout (not a kill switch) preserves the spec's "no new opt-out" requirement.

**Alternatives considered**:
- A `--pro-exec-timeout` CLI flag — rejected: this is invocation-wide operator policy
  (how long CI is willing to wait), not something that varies command-to-command, so it
  belongs in `atmos.yaml`/env config alongside the rest of `settings.pro`, not a
  per-invocation flag on `terraform plan`/`apply`/`describe affected` individually.

---

## Decision 8: Async best-effort flush mechanism

**Decision**: `proexec.CaptureAsync` launches the upload in a goroutine and blocks the
caller (`cmd/root.go`) for at most a short fixed ceiling (2 seconds) via a
`sync.WaitGroup` + `select`/timer, exactly mirroring the shape of PostHog's client
`Close()`/flush-with-timeout pattern already implicitly relied upon by
`pkg/telemetry/telemetry.go`. If the upload hasn't finished within the ceiling, the
process proceeds to exit without waiting further (the goroutine is abandoned; Go's
runtime permits process exit with in-flight goroutines).

**Rationale**: Directly implements the clarification answer ("short bounded wait ...
best-effort flush with a small timeout"). A fixed, small (not user-configurable) ceiling
keeps the async path's cost predictable and matches SC-004's "no more than a brief,
bounded delay."

**Alternatives considered**:
- `os.Exit` deferral via `context.Context` cancellation propagated through Cobra —
  rejected: adds lifecycle complexity for a two-second best-effort wait; a bare
  goroutine + timeout channel is simpler and matches the existing telemetry precedent's
  spirit (fire, give it a moment, move on).

---

## Decision 9: Pact contract extension (9th interaction)

**Decision**: Extend the existing `pkg/pro/consumer_pact_test.go` /
`pkg/pro/pact_helpers_test.go` (both `//go:build pact`) with a 9th interaction,
`UploadExecMetadata`, `POST /api/v1/atmos/exec`, using `matchers.Like()` for all dynamic
fields (IDs, timestamps, resource-usage numbers, structured-data contents) and exact
literal values only for constant/enum fields, exactly as documented in
`specs/001-pact-consumer-contracts/research.md` Decision 7. Regenerating
`pacts/atmos-AtmosPro.json` via `go test -tags pact ./pkg/pro/...` picks up the new
interaction automatically (pact-go merges all interactions from a test run into one
consumer-provider pact file). No Pact Broker publishing, no new CI job — same as the
existing 8 interactions.

**Rationale**: This directly satisfies the spec's FR-013/SC-006 requirement for a
"verifiable, versioned" contract artifact the Atmos Pro team can implement against
without reading Go source, using infrastructure that already exists and is already the
project's established pattern for exactly this purpose.

**Alternatives considered**: None — the existing suite's conventions are followed
directly; there is no reasonable alternative that wouldn't duplicate that infrastructure.

---

## Decision 10: Dedup fix — single shared sync-allowlist predicate

**Decision**: Move the sync-allowlist check out of `internal/exec/terraform.go`'s private
`isExecMetadataSyncSubcommand` into a new exported `proexec.IsSyncCommand(commandPath
string) bool` in `pkg/proexec/classify.go`. `cmd/root.go`'s `Execute()` calls it before
`proexec.CaptureAsync(cmd, err)` and skips the async call entirely when true;
`internal/exec/terraform.go`'s `captureExecMetadataSync` calls the same function instead
of its own copy.

**Rationale**: Production data showed a single `atmos terraform plan` invocation
producing two execution records — one via `cmd/root.go`'s unconditional
`proexec.CaptureAsync`, one via `ExecuteTerraform`'s `captureExecMetadataSync` →
`proexec.CaptureSync`. The root cause is that the allowlist existed only inside
`internal/exec`, invisible to `cmd/root.go`, so the two call sites could never stay in
sync by construction. A single exported predicate in `pkg/proexec` (the package both call
sites already depend on) removes the possibility of the two diverging again, and matches
FR-007's 2026-08-18 clarification that sync/async delivery is mutually exclusive per
invocation.

**Alternatives considered**:
- Have `cmd/root.go` special-case `terraform plan`/`apply`/`deploy`/`describe affected`
  by string match inline — rejected: reintroduces exactly the two-copies-of-the-same-list
  problem that caused the bug; a shared function in the package both sites already import
  is strictly simpler.
- Have `ExecuteTerraform` suppress `cmd/root.go`'s later `CaptureAsync` via a shared
  mutable flag (e.g. a package-level "already delivered" marker) — rejected: implicit,
  order-dependent state across package boundaries is harder to reason about and test than
  a pure predicate function; the predicate approach requires no coordination at call time.

---

## Decision 11: Multi-component aggregation — one record per graph run, not per node

**Decision**: Move the exec-metadata capture call out of `ExecuteTerraform` (which
`cmd/terraform/utils.go`'s `executeSingleComponent` calls once, but which the
multi-component graph scheduler also calls once *per node*) and into
`cmd/terraform/utils.go`'s graph-run lifecycle, which already tracks
`wasMultiComponentExecution` and already owns a per-run `terraformNodeHooks` value shared
across all nodes. `terraformNodeHooks` accumulates each node's outcome (component name,
exit code, structured data) into a slice as `AfterWithWriters` fires per node, and after
the graph scheduler returns, exactly one `proexec.CaptureSync` call is made with the
accumulated per-component list as `dataItems`. For the single-component path
(`executeSingleComponent`), `ExecuteTerraform`'s existing per-invocation
`captureExecMetadataSync` call is unchanged (there is only ever one node, so "per node"
and "per invocation" already coincide).

**Rationale**: Per FR-006a (2026-08-18 clarification), a multi-component `--affected`/
`--all` run must produce exactly one execution record for the whole CLI invocation, not
one per component — matching how `describe affected` (a related multi-component command)
already reports as a single record. `cmd/terraform/utils.go`'s graph orchestration is the
only place in the codebase that already knows when a multi-component run starts and ends,
making it the natural aggregation point; `ExecuteTerraform` itself has no visibility into
whether it is being called once or N times as part of a larger run.

**Alternatives considered**:
- Keep per-node uploads but tag them with a shared `BatchID` so Atmos Pro can group them
  client-side — rejected: FR-006a's clarification specifically calls for one record with
  per-component data folded into `DataItems`, not N correlated-but-separate records; the
  existing `BatchID`/chunking mechanism (FR-011) is reserved for splitting one oversized
  `DataItems` array, not for aggregating logically-distinct component runs, and reusing it
  for both purposes would conflate two different correlation semantics.
- Buffer per-node results in a package-level slice inside `internal/exec` and flush on
  process exit — rejected: `internal/exec` has no reliable "the graph run just ended"
  signal (that lifecycle lives in `cmd/terraform/utils.go`); piggy-backing on process exit
  would also misbehave for the sync delivery/timeout guarantees FR-008a requires.

---

## Decision 12: US3 unblocked — reuse existing `WithStdoutCapture`, not a new tee

**Decision**: Issue #2924's proposed fix (add a new `MultiWriter`-based stdout tee to
`ExecuteTerraform`'s shared pipeline) is **not adopted**. Instead: `cmd/terraform/plan.go`,
`apply.go`, and (newly) `deploy.go` already construct a `bytes.Buffer` and pass
`e.WithStdoutCapture(&stdoutBuf)`/`e.WithStderrCapture(&stderrBuf)` as `ShellCommandOption`s
into `terraformRunWithOptions` — today gated on `ciMode` (`--ci` flag / `ATMOS_CI`/`CI` env
/ `pkg/ci.IsCI()` auto-detect) and consumed only by `PostRunE`'s Native-CI job-summary
hooks. This plan decouples that capture from the `ciMode` gate: the buffer is now always
captured for `plan`/`apply`/`deploy` (cheap — an in-memory `bytes.Buffer`, no behavior
change to the real stdout stream, which still receives the tee'd copy unchanged), and
after `terraformRunWithOptions` returns, the ANSI-stripped text is passed through the
already-public `terraform.ParsePlanOutput`/`ParseApplyOutput` and forwarded as an
additional `opts`-carried value into `ExecuteTerraform`, which `captureExecMetadataSync`
reads to populate `CaptureSync`'s `data`/`dataItems` arguments (data-model.md's
`TerraformExecData` mapping, unchanged in shape).

**Rationale**: The capture mechanism US3 needs already exists, is already exercised in
production (Native CI job summaries depend on it today), and is already scoped to exactly
the three commands (`plan`/`apply`/`deploy`) that need it — extending its use avoids the
much larger blast radius #2924 originally proposed (a new tee across a shared pipeline
used by every terraform subcommand, requiring re-verification of streaming/TTY/masking
behavior for all of them, not just three). The only real gap was plumbing: the captured
buffer never left the `cmd/terraform/plan.go`/`apply.go` closures. Decoupling the capture
from `ciMode` (rather than reusing that flag as the exec-metadata gate) matters because
the two gates are independently controlled — `ciMode` enables the *separate*,
independently-configured Native CI job-summary feature (`atmosConfig.CI.Enabled`), while
exec-metadata upload is gated by `telemetry.IsCI() && Pro-configured`; a run could have
one true without the other, and coupling them would make exec-metadata's structured data
silently depend on an unrelated feature flag.

**Alternatives considered**:
- #2924's original proposal (new `MultiWriter` tee in `ExecuteTerraform`'s shared
  pipeline, using `WithStdoutOverride`'s prior art from `terraform_plan_diff.go`) —
  rejected: `WithStdoutOverride` *replaces* stdout (used to redirect noisy output to
  stderr), not tee it; `WithStdoutCapture` already tees, so no new option type is needed.
  Scoping the change to the shared pipeline (touched by every terraform subcommand) is
  also strictly riskier than scoping it to the three call sites that already opt into
  capture today.
- Gate the always-on capture behind the exec-metadata gate (`telemetry.IsCI() &&
  Pro-configured`) instead of making it unconditional — considered but not adopted as the
  primary mechanism: the capture itself is cheap (an in-memory buffer append, no I/O), so
  gating it saves negligible cost while adding a second condition to reason about at the
  `cmd/terraform/plan.go` call site; the exec-metadata gate is still checked downstream by
  `proexec.CaptureSync` itself (as it already is today), so no wasted upload occurs either
  way. Revisit only if profiling shows the always-on buffer append is measurably costly.

---

## Resolved NEEDS CLARIFICATION Items

All ambiguities were resolved during the `/speckit-clarify` sessions (2026-08-11,
2026-08-18) before planning began. No NEEDS CLARIFICATION markers remain in this plan or
the spec.
