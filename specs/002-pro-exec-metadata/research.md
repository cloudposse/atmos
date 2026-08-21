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

## Decision 13: `Command`/`Args`/`Flags` shape — correlatable with `uploadStatus` without merging it

**Decision**: `ExecUploadRequest.Command` is changed from `cmd.CommandPath()` (e.g.
`"atmos terraform plan"`) to the subcommand path with the leading `atmos` root segment
stripped (`"terraform plan"`). `ExecUploadRequest.Args` — currently always sent empty via
`maskArgs(nil)` in `pkg/proexec/envelope.go:55`, a live bug — is populated with only the
invocation's positional arguments (e.g. `["cdn"]`). A **new** `ExecUploadRequest.Flags`
field is added to carry the CLI flags actually passed (e.g.
`["-s", "plat-use2-dev", "--upload-status"]`), masked through the same existing
secret-masking path already used for `Args`. Positional args and flags are kept in
separate fields, never combined into one array. The older, independently-gated
`uploadStatus`/`--upload-status` mechanism (`internal/exec/pro.go`, `PATCH .../instances`)
is explicitly left unmodified — its `Command` field (a bare subcommand string, e.g.
`"plan"`, sourced from `info.SubCommand`) and separate `Component`/`Stack` fields have no
literal field-for-field equivalent on the `ExecUploadRequest` side, and this decision does
not attempt to force one; it only ensures both mechanisms' output is *content-correlatable*
for a human or the Atmos Pro backend reading both records for the same invocation.

**Rationale**: A production bug report showed a single `atmos terraform plan` invocation
producing two Atmos-Pro-side rows with different shapes — one from `uploadStatus` (has
`component`/`args`-like fields, no resource metrics) and one from this feature's
`POST /v1/atmos/exec` (has resource metrics, but `args` always empty and no `component`
field at all). Investigation (see `plan.md`'s Note) showed these are two structurally
independent, independently-gated upload mechanisms with no shared record ID — not (only)
the already-fixed `CaptureSync`/`CaptureAsync` race. Rather than introduce cross-mechanism
coordination (skip logic, a shared ID, or a merged payload — all rejected during
`/speckit-clarify` as unjustified scope expansion into a different, pre-existing endpoint
this feature does not own), the fix scoped to this feature alone: make its own `Command`/
`Args`/`Flags` correct and complete, so that whatever correlation Atmos Pro's backend
performs across the two record types has real data to work with instead of an always-empty
`Args` array and an `atmos`-prefixed `Command` that don't match `uploadStatus`'s shape at
all.

**Alternatives considered**:
- Make `ExecUploadRequest.Command`/`Args` byte-for-byte identical to `uploadStatus`'s
  `Command`/`Component`/`Stack` — rejected: the two DTOs have genuinely different shapes
  (`uploadStatus.Command` is a bare subcommand with no flags at all, `Component`/`Stack`
  are separate fields with no `ExecUploadRequest` counterpart); forcing byte-identical
  equality would mean either dropping information (`ExecUploadRequest.Flags`, which
  `uploadStatus` doesn't carry) or fabricating fields `uploadStatus` doesn't have. Content
  correlation (subcommand-name equivalence, e.g. `"plan"` is a suffix of `"terraform
  plan"`) is achievable without shape equality.
- Extend `uploadStatus`'s DTO/call site to also send `Flags`, achieving true field-level
  equality across both mechanisms (clarification session's "Option B") — rejected: this
  touches a second, older, pre-existing Atmos Pro endpoint (`PATCH .../instances`) that
  this feature does not own and has no other reason to modify; the smaller, one-sided fix
  (fixing only `ExecUploadRequest`, which already has a live bug to fix regardless) fully
  resolves the observed symptom without that added blast radius.
- Combine `Args` and `Flags` into one array (`["cdn", "-s", "plat-use2-dev",
  "--upload-status"]`) — rejected during clarification: the user explicitly requested
  positional arguments and flags be kept in distinct fields, matching how the two are
  independently useful to a downstream consumer (e.g. filtering/searching by flag without
  string-parsing a combined array).

---

## Decision 14: `Flags` source of truth and bare-token wire shape

**Decision**: `captureExecMetadataSync` (`internal/exec/terraform.go`) MUST NOT source
`Flags` from `info.AdditionalArgsAndFlags`. That field holds only pass-through args left
over *after* Cobra has already parsed and consumed recognized atmos flags (per
`cli_utils.go`, `info.AdditionalArgsAndFlags = args[componentArgIndex+1:]`) — `-s`/
`--stack` is never in it, and `buildPlanSubcommandArgs` additionally strips
`--upload-status` out of it before capture runs (`terraform_execute_helpers_args.go:36`).
The correct source of truth is the invoking `*cobra.Command`'s own record of explicitly-set
flags (`cmd.Flags().Visit`, `Changed == true`) — exactly the pattern the async path's
`commandArgsAndFlags` (`pkg/proexec/async.go:109-126`) already uses, so both paths must
converge on one shared helper (`proexec.FlagsFromCommand`, added this session). `Flags`
MUST serialize each flag by its canonical long name (`--stack`, never the shorthand `-s`,
since `pflag.Flag` never records which alias was typed) — a boolean/valueless flag like
`--upload-status` appears alone with no synthesized value; this required
`commandArgsAndFlags`'s prior implementation (which appended `"--"+f.Name, f.Value.String()`
unconditionally, producing `["--upload-status", "true"]` for a bool flag) to change to skip
the value for bool-typed flags (`pflag.Flag.Value.Type() == "bool"`) — implemented and
tested this session (`TestCommandArgsAndFlags_BoolFlagOnly`).
**Superseded (2026-08-19, later same day)**: an earlier version of this decision also
required literal "bare tokens exactly as typed" — preserving shorthand-vs-long form and
invocation order. Real-world testing after implementation showed `cmd.Flags().Visit()`
structurally cannot deliver either: `pflag.Flag` has no record of which alias was typed,
and `Visit()` iterates in flag-name lexicographic order, not invocation order. A follow-up
`/speckit-clarify` explicitly relaxed this — canonical long-form names and
Cobra's-own-iteration-order are now the accepted, final shape; true as-typed reconstruction
(which would require parsing raw pre-parse command-line tokens instead of the parsed flag
set) was evaluated and rejected as unnecessary scope. See spec.md's third 2026-08-19 Q&A
and FR-003b's current text, which is authoritative over this paragraph's now-superseded
"bare tokens exactly as typed" framing below.

**Rationale**: A production report (2026-08-19) showed an `atmos terraform plan cdn -s
plat-use2-dev --upload-status` invocation's sync-path execution record with a completely
empty `flags` column. Investigation (this session) traced it to the wrong data source, not
a flag-stripping ordering bug — `info.AdditionalArgsAndFlags` was never capable of holding
`-s` in the first place. A regression test
(`internal/exec/terraform_exec_metadata_flags_test.go::TestCaptureExecMetadataSync_FlagsReflectRealInvocation`)
reproduces this exact shape and fails on current HEAD. `/speckit-clarify` (2026-08-19) then
resolved a second, adjacent ambiguity the fix surfaced: whether `--upload-status` itself
belongs in the reported `Flags` (yes — no exclusions, matching FR-003b's own worked
example) and what wire shape `Flags` should use (bare tokens as typed, not `--name value`
pairs for every flag — see spec.md Session 2026-08-19).

**Alternatives considered**:
- Fix only the stripping-order bug (capture flags before `buildPlanSubcommandArgs` mutates
  `info.AdditionalArgsAndFlags`) — rejected: this would still leave `-s`/`--stack` and every
  other atmos-recognized flag permanently absent from `Flags`, since they were never in that
  slice to begin with; it treats a symptom, not the actual wrong-data-source defect.
  Superseded task: `tasks.md` T006/T009.
  - Keep the async path's `--name value`-pair-for-every-flag serialization and change the
    spec's worked example to match instead — rejected during `/speckit-clarify`: bare tokens
    are what the user actually typed and what `uploadStatus`'s content-correlation guarantee
    (FR-003a) implicitly assumes; pairs would silently diverge two "flags actually passed"
    representations across the two upload mechanisms for the same invocation.

**Open, not yet resolved by this decision**: A second production symptom — two `atexec_*`
rows for what should be one invocation, with different `workflow_job` ids — was
investigated this session but not conclusively root-caused. Client-side inspection of
`pkg/proexec/classify.go`/`async.go` and `internal/exec/terraform.go:199-242` found the
sync/async mutual-exclusion gate (`IsSyncCommand`) structurally sound for a plain
`terraform plan`; the sample rows' shape is also consistent with one row predating the
`2fe4fabe0` DTO-shape fix, though their timestamps (2026-08-19, ~14h after that commit
landed) argue against a simple stale-build explanation. This needs a real end-to-end
regression test (driving the actual `cmd.Execute()` top-level entrypoint against a fake Pro
server) to settle one way or the other before further action — tracked as `tasks.md` T026,
not yet implemented.

---

## Decision 15: `ExecutionID` — fresh UUID v4 per invocation, generated in `buildRecord`

**Decision**: `buildRecord` (`pkg/proexec/envelope.go`) generates `ExecutionID :=
uuid.New().String()` once per call (i.e. once per invocation, since `buildRecord` is called
exactly once per `CaptureSync`/`CaptureAsync` invocation) using `github.com/google/uuid` —
already a direct dependency, already used identically for `BatchID` generation in
`pkg/pro/chunked_upload.go`. It is set on `dtos.ExecUploadRequest.ExecutionID` alongside the
existing `AtmosProRunID`, and is *not* derived from or cached against any other identifier
(git SHA, run ID, process PID) — it exists solely to uniquely name this one execution record
and, when `Data` requires out-of-band delivery (Decision 16), to key the correlated
`/exec/data` upload.

**Rationale**: Directly implements the 2026-08-19 clarification: the existing
`AtmosProRunID` correlates records *across* a CI run (many commands, one ID), while
`ExecutionID` uniquely identifies *this one* record — a distinct correlation axis. Reusing
`google/uuid` (already imported, already the project's UUID-generation library) avoids a new
dependency; reusing the exact `uuid.New().String()` call shape `chunked_upload.go` already
uses for `BatchID` keeps UUID generation idiomatically consistent across the Pro client
layer.

**Alternatives considered**:
- Derive `ExecutionID` deterministically from existing fields (e.g. hash of
  command+timestamp+PID) — rejected: a random UUID is simpler, has no collision-avoidance
  bookkeeping to reason about, and matches the "opaque per-execution identifier" framing the
  clarification settled on (UUID v4, not v7/time-ordered).
- Generate `ExecutionID` at the `CaptureSync`/`CaptureAsync` call site instead of inside
  `buildRecord` — rejected: `buildRecord` is the single place both delivery paths already
  converge to assemble the envelope; generating it there (rather than in two separate call
  sites) removes any chance of the two paths diverging on *how* the ID is produced, mirroring
  the rationale behind Decision 10's single shared `IsSyncCommand` predicate.

---

## Decision 16: `Data` delivery redesign — inline-or-blob-URL, retiring multi-chunk `DataItems`

**Decision**: The previously-shipped multi-chunk model (`dtos.ExecUploadRequest.DataItems
[]json.RawMessage` + `BatchID`/`BatchIndex`/`BatchTotal`, `pkg/pro/api_client_exec.go`'s
`sendChunked` call) is removed entirely and replaced with a binary choice on the single
`Data json.RawMessage` field:

- `UploadExecMetadata` (`pkg/pro/api_client_exec.go`) marshals the full record (envelope +
  `Metrics` + `Data` inline) once and compares its byte length against the existing
  `c.MaxPayloadBytes` (falls back to `DefaultMaxPayloadBytes = 4 * 1024 * 1024`,
  `pkg/pro/chunked_upload.go` — already exactly 4 MB, so no new constant is introduced).
- If under the threshold: send the marshaled record as-is to `POST /v1/atmos/exec`, `Data`
  inline as a JSON structure — this is the common case (most commands have no `Data` at
  all, or a small one).
- If at/over the threshold: first call a new `UploadExecData(dto
  *dtos.ExecDataUploadRequest) (*dtos.ExecDataUploadResponse, error)` — `POST {BaseURL}/
  {BaseAPIEndpoint}/atmos/exec/data`, body `{execution_id, data}` — in a single request
  (never chunked; the blob store handles arbitrarily large single uploads). On success, its
  response's `url` field replaces `Data` (re-marshaled as a JSON string, e.g.
  `"https://blob.vercel-storage.com/..."`) on the record, which is then sent to
  `POST /v1/atmos/exec` as normal.
- `dtos.ExecUploadRequest.DataItems`, `BatchID`, `BatchIndex`, `BatchTotal` are deleted.
  `pkg/pro/chunked_upload.go` itself (`sendChunked`, `BatchInfo`, `splitSlice`) is
  **unchanged** — it remains in active use by `UploadAffectedStacks`/`UploadInstances`; only
  its use for `ExecUploadRequest` specifically is retired.
- `pkg/proexec.buildRecord`, `CaptureSync`, `CaptureAsync` drop their `dataItems []any`
  parameter; command-specific bulk data (e.g. per-resource `terraform plan`/`apply` change
  lists, and the multi-component per-component breakdown from Decision 11) is now folded
  into the single `data any` argument as a JSON array/object, exactly as `Data` already
  represents "small" structured summaries today — the inline-vs-blob choice at the HTTP layer
  makes the client-side size distinction between "small structured data" and "potentially
  large structured data" unnecessary; `UploadExecMetadata` handles both uniformly by size.

**Rationale**: Directly implements the 2026-08-19 "redo batch uploading" clarification: the
client estimates the whole payload's size and only reaches for the out-of-band path when
necessary, using a dedicated single-request blob-upload endpoint (Vercel Blob-backed, per the
clarification) rather than splitting one logical `Data` value across multiple correlated
`/exec` requests. This is a strict simplification over the multi-chunk model — one binary
branch and, at most, one extra request — versus the prior model's variable-length chunk
sequence, `BatchInfo` bookkeeping on every chunk, and the requirement that the full envelope
be repeated on every chunk request. It also removes the awkward asymmetry where `Data`
(small, never chunked) and `DataItems` (potentially large, chunked) were two different
fields with different size-handling rules for what is conceptually one "command-specific
structured data" concept — after this decision there is exactly one `Data` field, and its
size-handling is uniform.

**Alternatives considered**:
- Keep `DataItems`/multi-chunk for the "already over 4 MB even as inline JSON" case and add
  the blob-URL path only for larger cases still — rejected: this reintroduces two competing
  size-handling mechanisms for the same conceptual field, which is exactly what this decision
  set out to eliminate; the user's explicit instruction was to "redo" batch uploading, not
  layer a third mechanism on top of the second.
- Have `UploadExecMetadata` chunk the blob upload itself for extremely large `Data` — rejected
  per the clarification: the blob store handles arbitrarily large single uploads, so
  client-side chunking of `/exec/data` would be unused complexity with no scenario that
  requires it.
- Add an explicit boolean/enum field (e.g. `data_is_url`) to disambiguate `Data`'s two shapes
  server-side instead of relying on JSON-type inspection (string vs. object/array) —
  considered, but not required by the clarification, which described `Data` as "type of two -
  json struct or string that would be url" without requesting a separate discriminator field;
  Atmos Pro's provider implementation can distinguish the two shapes by JSON type alone (a
  JSON string vs. a JSON object/array), matching how `json.RawMessage` naturally represents
  both without a wrapper. Revisit only if Atmos Pro's implementation finds type-sniffing
  insufficient in practice — not asserted here since this repo does not own that side.

---

## Decision 17: Multi-component aggregation folds into `Data`, not a separate list

**Decision**: `cmd/terraform/utils.go`'s per-node accumulator (Decision 11 — collects each
component's identity/outcome/structured-data as the multi-component graph run proceeds) now
builds its accumulated result as the single `data` value passed to `CaptureSync`, rather than
a separate `dataItems []any` list (which no longer exists per Decision 16). The shape is
otherwise unchanged: one entry per component (`{"component": ..., "stack": ..., "exitCode":
..., "action": ..., "address": ...}`-shaped items), now nested under `data`'s per-component
array/section instead of being `DataItems`' top-level array.

**Rationale**: Decision 11's aggregation point (the graph-run lifecycle in
`cmd/terraform/utils.go`) is unaffected by Decision 16 — it still owns collecting per-node
results into one record per invocation (FR-006a). What changes is only which single-argument
field the accumulated result becomes: `Data`, sized by `UploadExecMetadata`'s new
inline-vs-blob logic (Decision 16) rather than `DataItems`'s old chunking logic. A
multi-component `--affected`/`--all` run touching thousands of resources is exactly the
scenario most likely to exceed 4 MB and take the blob-upload path — this is expected and
correctly handled by Decision 16's threshold check without any special-casing for the
multi-component case.

**Alternatives considered**:
- Keep a separate "bulk" vs. "summary" split within `data` itself (e.g. `data.summary` always
  inline, `data.items` conditionally blob-uploaded) — rejected: Decision 16 already
  establishes that the *whole* record's size (not a sub-field) decides inline-vs-blob; adding
  a second, nested size decision inside `data` would reintroduce the two-mechanisms problem
  Decision 16 exists to eliminate.

---

## Decision 18: Single-component structured data — caller-supplied parser closure, not a package restructure

**Decision**: `internal/exec/shell_utils.go` gains a new `ShellCommandOption`,
`WithExecMetadataParser(fn func(subCommand string) any) ShellCommandOption`, storing `fn` on
`shellCommandConfig.execMetadataParser`, plus an `execMetadataParserFromOpts(opts
...ShellCommandOption) func(subCommand string) any` extractor — both exactly mirroring the
already-shipped `WithInvokingCommand`/`invokingCommandFromOpts` pair (same file, same
pattern, same rationale: thread caller-only-available data through the options list without
adding a parameter to every call in between). `internal/exec/terraform.go`'s
`captureExecMetadataSync` extracts this closure via `execMetadataParserFromOpts(opts...)`
and, only when it is non-nil AND `proexec.IsSyncCommand` is true AND `info.NodeHooks == nil`
(the existing multi-component skip, Decision 16/17's session), calls `parser(subCommand)`
once to obtain `data any` for `proexec.CaptureSync` — never more than once per invocation,
never for async/non-allowlisted commands.

`cmd/terraform/plan.go`/`apply.go`/`deploy.go` supply this closure. Each already constructs
`stdoutBuf`/`stderrBuf` `bytes.Buffer`s and passes them to `WithStdoutCapture`/
`WithStderrCapture` — this construction moves outside the existing `ciMode` conditional (see
below) so the buffers are always populated by the time `ExecuteTerraform`'s
`captureExecMetadataSync` call (at the very end of the pipeline) invokes the closure. The
closure itself is a thin call to a new `cmd/terraform/utils.go` helper,
`buildTerraformExecData(subCommand string, output string) any`, which calls
`citerraform.ParseOutput(output, mappedCommand)` (safe from `cmd/terraform`, per Decision
17's finding) and decodes the result via a shared `parseTerraformOutputMirror` helper
(refactored out of the already-shipped `parseTerraformResourceChanges`, so both call sites
decode through one code path) into the single-component combined-object shape
(`resource_counts`/`outputs`/`warnings`/`changes`) that `data-model.md`'s `TerraformExecData`
already specifies — no new wire shape, this only makes `internal/exec` able to populate a
shape the wire contract already defined.

**`ciMode` decoupling**: `WithStdoutCapture`/`WithStderrCapture` construction in `plan.go`/
`apply.go`/`deploy.go` moves outside the `if ciMode` block entirely — capture always happens
now. The *separate*, `ciMode`-gated post-processing (`capturedPlanOutput` assignment, used
only by the independently-configured Native CI job-summary feature) is untouched and stays
gated exactly as before; only the underlying buffer construction/option-wiring is now
unconditional, since both consumers (CI job summaries and, now, exec-metadata) need the same
tee'd buffer and there is no reason to build it twice or gate one consumer on the other's
flag.

**Rationale**: Directly resolves the architecture question the previous implementation pass
left open (tasks.md's Phase 5 "Status" note, this session). Two options were on the table
going in: (a) restructure `pkg/ci`'s package boundaries so `internal/exec` could safely
import the parser directly, or (b) invert control so the one package that already can call
the parser (`cmd/terraform` — proven safe this session, Decision 17) supplies the result to
the one package that needs it (`internal/exec`) via a caller-injected closure, never
importing the parser itself. (b) is chosen: it requires no changes to `pkg/ci`'s existing
import graph (zero risk of breaking that package's own tests/behavior), and it reuses a
pattern (`WithInvokingCommand`) already proven in this exact file for the exact same kind of
problem (Cobra-command-only-available-to-the-caller data needed deep inside
`ExecuteTerraform`). Restructuring `pkg/ci`'s packages was rejected as strictly higher-risk
for no additional benefit — nothing about the wire contract or spec requires the parser to
live somewhere `internal/exec` can import; the constraint is purely "no cycle," which a
closure satisfies trivially.

Decoupling stdout/stderr capture from `ciMode` was already argued for by Decision 12
("capture itself is cheap... gating it saves negligible cost while adding a second condition
to reason about") but not implemented in that session, since nothing yet consumed the buffer
outside the `ciMode` path. This decision is what finally requires it: the exec-metadata
parser closure needs the buffer populated regardless of whether Native CI job summaries are
also enabled, since the two features are independently gated (exec-metadata by
`telemetry.IsCI() && Pro-configured`, Native CI by `atmosConfig.CI.Enabled`) and a run can
have one true without the other.

**Alternatives considered**:
- Restructure `pkg/ci/plugins/terraform`/`pkg/ci/internal/plugin` so the parser (or a
  narrower "just extract resource changes" function) lives in a package neither
  `internal/exec` nor `pkg/ci/plugins/terraform` needs to avoid importing — rejected: a much
  larger, riskier change (touches a package this feature does not otherwise own) for no
  benefit over the closure approach, which fully resolves the cycle with a one-file addition.
- Give `internal/exec` its own, independent, duplicate parsing logic (re-implement resource
  extraction without calling `pkg/ci/plugins/terraform` at all) — rejected: would create two
  independently-maintained terraform-output parsers that could silently drift apart; the
  whole point of Decision 17's finding was that the *existing* parser is safely callable
  (from the right package), so duplicating it defeats that finding's purpose.
- A new `ExecMetadataDataProvider` interface instead of a plain closure — rejected per YAGNI
  (constitution's Simplicity principle): there is exactly one implementer
  (`cmd/terraform`), and the existing `WithInvokingCommand` precedent in the same file already
  establishes "plain closure/value via functional option" as this codebase's answer to
  exactly this kind of problem; introducing a second, interface-based pattern alongside it
  for no functional gain would be inconsistent, not more flexible.

---

## Decision 19: Dual independent output masking — Terraform `sensitive` flag, then Gitleaks

**Decision**: `cmd/terraform/utils.go` gains a new pure helper,
`maskSensitiveOutputs(outputs map[string]json.RawMessage) map[string]any`, called from
`buildTerraformExecData` in place of the current `"outputs": data.Outputs` pass-through. For
each entry, it decodes the `json.RawMessage` into the existing `{Value, Type, Sensitive}`
shape (mirroring `plugin.TerraformOutput`'s fields, same as the rest of
`terraformOutputDataMirror`'s decoding), and returns a `map[string]any` where a `Sensitive:
true` entry's `Value` is replaced with `pkg/io.MaskReplacement` (`"<MASKED>"`) and every other
field/entry passes through unchanged. This is layer 1. Layer 2 — `pkg/proexec/envelope.go`'s
already-shipped `maskedDataJSON`, which runs Gitleaks-pattern masking (`io.MaskString`) over
the whole marshaled `Data` blob — is untouched and continues to run afterward, unconditionally,
over the already-layer-1-masked structure. Both layers always execute; neither is
conditional on the other having (or not having) already redacted the same value.

**Rationale**: The 2026-08-20 `/speckit-clarify` session confirmed a real gap: layer 2's
Gitleaks-style masking is pattern-based (it matches known secret *shapes* — API keys, tokens,
credentials), while Terraform's own `sensitive` output flag is a *metadata* marker with no
required relationship to content shape — an output can be marked sensitive (e.g. an internal
database ID, a customer name) without its value ever matching a Gitleaks pattern, and would
therefore ship in plaintext under layer 2 alone. Reading the actual current implementation
(`cmd/terraform/utils.go:697`, `"outputs": data.Outputs`) confirmed this is not hypothetical:
today, `Sensitive: true` outputs are forwarded completely unmasked into the structured `Data`
payload; only the outer Gitleaks pass has any chance of catching them, and only if the value
happens to match a pattern. This is the exact scenario the PR referenced in the clarify
session's context (`extractOutputValues`, "Sensitive outputs are replaced with `<MASKED>` to
prevent secret leakage") was pointing at.

The referenced PR's own `extractOutputValues` flattens each output down to a bare
`map[string]any{key: value_or_MASKED}`, discarding `type`/`sensitive` from the wire payload
entirely. This plan deliberately does **not** copy that flattening: `contracts/
interactions.md` already documents (and the shipped Pact contract already encodes) the
per-output shape as `{value, type, sensitive}` — an object, not a bare scalar — and Atmos Pro
may already rely on being able to tell "this value is a masked placeholder because `sensitive`
is true" apart from "this value is genuinely the literal string `<MASKED>`" via the
still-present `sensitive` field. Changing the wire shape now would be a breaking contract
change with no functional benefit over keeping the field and only substituting its `value`.
Reusing `pkg/io.MaskReplacement` (rather than a locally-defined `"<MASKED>"` string literal,
which is what the referenced PR uses) keeps both masking layers visibly producing the same
placeholder if they ever redact the same value, and keeps the literal defined in exactly one
place (`pkg/io/masker.go`) — consistent with the existing `settings.terminal.mask.replacement`
override already respected by `pkg/io`'s general masking path (`newMasker`), though this
narrow helper does not currently thread that override through (see Alternatives).

**Fail-safe default for malformed entries**: if a given output's `json.RawMessage` fails to
decode into the expected shape, `maskSensitiveOutputs` treats it as sensitive (masks it)
rather than forwarding the raw, undecoded bytes — consistent with FR-010's existing
"exclude/mask on doubt" posture; an undecodable entry is exactly the kind of ambiguous case
that policy already covers.

**Alternatives considered**:
- Flatten `outputs` to match the referenced PR's `map[string]any{key: value}` shape exactly —
  rejected: breaking change to an already-shipped, already-contracted wire shape for no
  functional gain (see Rationale above).
- Rely on layer 2 (Gitleaks) alone, treating the referenced PR's `extractOutputValues` pattern
  as unnecessary since "the existing masking already covers secrets" — rejected: this is
  precisely the assumption the 2026-08-20 clarification session tested and rejected (`answer:
  "Both, applied independently"`); Gitleaks masking is pattern-based and provably does not
  cover a sensitive-flagged value with no recognizable secret shape.
- Thread `Settings.Terminal.Mask.Replacement` (the user-configurable override already
  respected by `pkg/io`'s general masker) into `maskSensitiveOutputs` instead of using the
  `MaskReplacement` constant directly — deferred, not rejected outright: doing so would
  require plumbing `*schema.AtmosConfiguration` into `buildTerraformExecData`, which currently
  takes only `(subCommand, output string)` and is called from a closure
  (`terraformExecMetadataParserFunc`) with no config in scope at that point in the call chain.
  Out of scope for this narrow fix; the constant default is correct today since FR-010a's own
  clarification only specifies "a fixed placeholder," not a configurable one, and threading
  config through is a larger, separate change if ever needed.

---

## Decision 20: `has_changes`/`has_errors`/`errors` — decode the discarded top-level `OutputResult` fields

**Decision**: `terraformOutputResultMirror` (`cmd/terraform/utils.go`) is extended with three
new fields — `HasChanges bool`, `HasErrors bool`, `Errors []string` — decoded via the same
JSON round-trip already used for `Data`, since `citerraform.ParseOutput`'s actual return type
(`plugin.OutputResult`) already carries all three at the top level (alongside `Data any`).
`parseTerraformOutputMirror` returns the fuller struct; `buildTerraformExecData` surfaces
`has_changes`/`has_errors`/`errors` as new top-level keys in its returned map, alongside the
existing `resource_counts`/`outputs`/`warnings`/`changes`.

**Rationale**: FR-006 already required "warnings/errors" in the structured payload before
this session — the 2026-08-20 clarification confirmed the gap was real, not just missing
spec text: `terraformOutputResultMirror`'s JSON tags (`json:"Data"` only) mean
`json.Unmarshal` silently drops `HasChanges`/`HasErrors`/`Errors` from the marshaled
`OutputResult`, even though the parser itself already computes them correctly (per
`pkg/ci/plugins/terraform/parser.go`'s `result.HasErrors`/`result.Errors` assignments,
confirmed present for both the plan and apply/deploy parse paths). This is a pure decode-gap
fix — no new parsing logic, no new call to the CI plugin parser (already called once per
invocation, per Decision 18's "at most once" constraint, which this delta does not relax).

**Alternatives considered**:
- Compute `has_changes`/`has_errors` on the Atmos side by inspecting `changes`/`warnings`
  after the fact (e.g. `has_changes = len(changes) > 0`) instead of decoding the parser's own
  flags — rejected: `HasErrors`/`Errors` are not fully derivable this way (e.g. a
  `terraform plan` that errors before producing any resource-change data would show
  `len(changes) == 0` either way, indistinguishable from "no changes" without the parser's own
  error signal); reusing the parser's already-correct computation is strictly more accurate
  and avoids a second, potentially-drifting derivation of the same fact.

## Decision 21: `component`/`stack` — threaded from `cmd/terraform`'s already-resolved call-site data

**Decision**: `buildTerraformExecData` gains two new string parameters, `component` and
`stack`, threaded through `terraformExecMetadataParserFunc` and `terraformCaptureShellOpts`
from `cmd/terraform/plan.go`/`apply.go`/`deploy.go`'s `RunE`, where `args[0]` (the positional
component identifier) and the already-parsed `--stack`/`-s` flag value are already available
before `terraformCaptureShellOpts` is called. When non-empty, `buildTerraformExecData` adds
`component`/`stack` as new top-level keys in its returned map; when either is empty (should
not occur on the single-component-only path this delta covers), the corresponding key is
omitted entirely rather than emitted as an empty string.

**Rationale**: Today a single-component invocation's structured `Data` carries no identity of
its own — a consumer must infer `component`/`stack` from the base envelope's `Args[0]`/the
`--stack` value buried in `Flags` (itself only reliable since the FR-003b fix landed). Adding
first-class `component`/`stack` fields directly to the structured payload removes that
inference requirement entirely and mirrors what the multi-component aggregation path already
does per-node (`execNodeResult.Component`/`.Stack`, `cmd/terraform/utils.go:535-536`) — this
delta brings the single-component path to parity with an identity pattern the multi-component
path already established, rather than inventing a new one.

Sourcing `component`/`stack` from `cmd/terraform`'s call site (not `internal/exec`'s later-
resolved `schema.ConfigAndStacksInfo`) was the only option considered that doesn't reopen
Decision 18's import-cycle problem: `internal/exec` still never imports the CI plugin parser
or gains new coupling to `cmd/terraform`'s internals — it only receives already-computed
strings via the existing closure mechanism, exactly as `subCommand` already is.

**Alternatives considered**:
- Resolve `component`/`stack` inside `internal/exec/terraform.go`'s `captureExecMetadataSync`
  from `info.Component`/`info.Stack` (the `schema.ConfigAndStacksInfo` already available
  there) and pass them into the parser closure as arguments, rather than threading them from
  `cmd/terraform` — rejected: `info.Component`/`info.Stack` are shaped by `internal/exec`'s
  own field naming/lifecycle, and passing that context into a `cmd/terraform`-defined closure
  argument would create an implicit, easy-to-break coupling between the two packages' data
  models where a plain, pre-resolved string pair (matching `subCommand`'s existing style) is
  simpler and requires no shared type.
- Emit `component`/`stack` as empty strings rather than omitting the keys when unknown —
  rejected: an empty string is ambiguous with "the component/stack genuinely has an empty
  name" (impossible in practice, but inconsistent with this feature's existing `omitempty`
  convention for "field legitimately absent," e.g. `ResourceUsageMetrics`'s platform-specific
  fields); omission is the clearer signal.

---

## Decision 22: `describe affected` structured data — return the already-computed `[]schema.Affected`, don't recompute

**Decision**: `internal/exec/describe_affected.go`'s `executeInner(a *DescribeAffectedCmdArgs)
error` becomes `executeInner(a *DescribeAffectedCmdArgs) ([]schema.Affected, error)`, returning
the same `affected []schema.Affected` slice it already computes internally for every
invocation (rendering, and — inside the existing `if args.Upload` branch — the
`UploadAffectedStacksRequest.Stacks` field, `describe_affected.go:595-602`). `Execute`
(`describe_affected.go:364-385`), which currently hardcodes `Data` to nothing on its
`ExecRecordInput` (its own doc comment: "Data is passed as nil — describe affected has no
defined structured-data extension"), captures this new return value and passes
`Data: proexec.VersionedData(1, "stacks", affected)` instead.

**Rationale**: FR-006b (2026-08-20 clarification) requires `describe affected`'s structured
data to reuse the same per-stack data already sent to `POST /api/v1/affected-stacks` — and
reading the actual code confirms `affected` is already computed unconditionally (it's the
list the command renders to the user), not only when `--upload` fires. Returning it from
`executeInner` costs nothing beyond widening one function's return type; the only alternative
that would avoid a signature change (recomputing `affected` a second time inside `Execute`)
was rejected outright as both wasteful and risky — a second resolution pass could theoretically
produce a different answer under non-deterministic conditions (e.g. a concurrent git state
change between calls), silently making the execution record's `Data` disagree with what was
actually shown to the user or uploaded to `/affected-stacks`.

**No `--upload` gating**: unlike `list instances` (Decision 23), `describe affected`'s `Data`
is attached regardless of whether `--upload` was passed, because the underlying list is always
computed — there is no "extra cost avoided by gating" the way there is for `list instances`'
`[]UploadInstance` list, which genuinely is not built unless `--upload` triggers it. This
asymmetry between the two commands is intentional and follows directly from each command's own
existing compute-cost profile, not an inconsistency introduced by this feature.

**Alternatives considered**:
- Store `affected` on the `describeAffectedExec` receiver (`d.lastAffected` or similar, mirroring
  `cmd/terraform/plan.go`'s package-level `capturedPlanOutput` pattern) instead of widening
  `executeInner`'s return signature — rejected: a receiver/package-level field introduces
  cross-call mutable state for a value that has an obvious, direct call-path relationship
  (`executeInner` computes it, `Execute` calls `executeInner` once and immediately needs the
  result) — a return value is strictly simpler and requires no "reset before next test" concern,
  unlike the package-level globals this feature already carries for other reasons (`async.go`'s
  `currentAtmosConfig`, `plan.go`'s `capturedPlanOutput`), which exist only because their
  producer and consumer are NOT in a direct call relationship.
- Gate `describe affected`'s `Data` on `--upload`, mirroring `list instances` — rejected: would
  needlessly withhold data from Atmos Pro that costs nothing extra to include, and FR-006b's
  own wording ("reusing the same per-stack data already reported...") doesn't condition it on
  `--upload` the way FR-006c explicitly does for `list instances`.

## Decision 23: `list instances` structured data — `SetPendingAsyncData`, a minimal hand-off to the generic async hook

**Decision**: `pkg/proexec/async.go` gains a new unexported package-level var,
`pendingAsyncData any`, plus an exported setter, `SetPendingAsyncData(data any)`, mirroring the
file's existing `SetAtmosConfig(atmosConfig *schema.AtmosConfiguration)` /
`currentAtmosConfig` pair exactly (same file, same "hook called at a `(cmd, err)`-only call
site needs data only available earlier, in a different package" rationale).
`CaptureAsync(cmd, err)` (`async.go:63`), when building its `ExecRecordInput`, reads
`pendingAsyncData` and immediately clears it (`data := pendingAsyncData; pendingAsyncData =
nil`) rather than leaving it for a caller to clear — so a value set by one invocation can never
leak into a later one within the same process. `pkg/list/list_instances.go`'s
`ExecuteListInstancesCmd`, inside its existing `if opts.Upload { ... }` branch (confirmed via
`list_instances.go`'s `--upload` gating around line 774+, `InstancesUploadRequest`/`req.Instances`
construction around line 544), calls
`proexec.SetPendingAsyncData(proexec.VersionedData(1, "instances", req.Instances))` once the
list is already built for `UploadInstances` — never outside that branch, so a plain
`atmos list instances` (no `--upload`) triggers zero extra computation.

**Rationale**: `list instances` has no exec-metadata wiring at all today (confirmed: no
`proexec` import in `pkg/list/list_instances.go`) — it relies entirely on the generic async
default path, `cmd/root.go`'s `proexec.CaptureAsync(cmd, err)`, which is deliberately
command-agnostic (it only ever sees a `*cobra.Command` and an `error`, per
`commandArgsAndFlags`) and so always builds `Data: nil`. FR-006c requires `list instances` to
attach its own `Data`, gated on `--upload` — but there is no existing mechanism for a
non-sync-allowlisted command to hand data to that generic hook. `SetPendingAsyncData` is the
smallest addition that closes this gap: it reuses an already-proven pattern in the exact same
file (`currentAtmosConfig`) rather than introducing a new one (e.g. a `context.Context` value
threaded through Cobra's command tree, or a `cmd.Annotations` string-only side-channel that
can't hold a typed `any` payload cleanly).

**Alternatives considered**:
- Have `list_instances.go` call a `proexec` capture function directly, bypassing the generic
  `CaptureAsync(cmd, err)` hook entirely for this command (the same shape as `describe
  affected`'s direct `CaptureSync` call) — rejected: `list instances` is NOT on the synchronous
  allowlist (FR-007) and must still go through the async 2-second bounded-flush timing rule
  (FR-009) that `CaptureAsync` already implements; duplicating that timing logic at a second
  call site (or extracting it into a shared helper just for two callers) is more machinery than
  a single package-level hand-off, and risks the two async call sites' timing behavior drifting
  apart the same way `IsSyncCommand` was introduced (Decision 10) specifically to prevent for
  sync/async classification.
- A `context.Context` value passed from `list_instances.go` down to `cmd/root.go`'s
  `CaptureAsync` call — rejected: `CaptureAsync(cmd, err)`'s signature only has the `*cobra
  .Command`, not a request-scoped `context.Context`; wiring one through would touch more of the
  call chain than the package-level var, and CLAUDE.md's Context Usage rules reserve
  `context.Context` for cancellation/deadlines/request-scoped tracing IDs, not for passing this
  kind of producer-to-consumer payload (that's what the Options-pattern/DI guidance is for,
  and neither fits a `(cmd, err)`-shaped hook cleanly — a plain setter does).
- Read `req.Instances` back out of `cmd`'s flags/state inside `CaptureAsync` itself (parsing
  `--upload`'s effect after the fact) — rejected: `CaptureAsync` has no access to the
  already-built `[]UploadInstance` list (it isn't stored anywhere reachable from `cmd`), and
  reconstructing it a second time would duplicate `list_instances.go`'s own list-building logic
  for no benefit over the producer just handing over what it already built.

## Decision 24: `version` field — `VersionedData` helper for the two matching shapes, direct literal for the one that doesn't

**Decision**: `pkg/proexec` gains `VersionedData(version int, key string, payload any)
map[string]any`, returning `map[string]any{"version": version, key: payload}`. `describe
affected` (Decision 22) and `list instances` (Decision 23) both use it —
`VersionedData(1, "stacks", affected)` and `VersionedData(1, "instances", req.Instances)`
respectively — since both shapes are genuinely identical in structure: one `version` field
plus exactly one array wrapped under one key. `cmd/terraform/utils.go`'s
`buildTerraformExecData` does **not** use `VersionedData`: its shape has multiple top-level
keys (`resource_counts`, `outputs`, `warnings`, `changes`, `has_changes`, `has_errors`,
`errors`, and conditionally `component`/`stack` — research.md Decisions 19-21), not one
wrapped payload, so it simply adds `"version": 1` as one more key in its own existing
`map[string]any{...}` literal.

**Rationale**: FR-005a requires every one of the three structured-`Data` shapes to carry its
own `version` field, starting at `1`, independent per shape. Three hand-written
`map[string]any{"version": 1, ...}` literals would work, but two of the three
(`describe affected`, `list instances`) are byte-for-byte the same shape modulo the wrapped
key name — exactly the kind of duplication the constitution's "three similar lines is
acceptable; a premature abstraction is not" guidance is calibrated against, tipping toward "one
small helper" once there are two *genuinely identical* call sites, not merely superficially
similar ones. Forcing `TerraformExecData`'s structurally different shape through the same
helper (e.g. by giving `VersionedData` a variadic/merge-style signature so it could inject
`version` into an arbitrary pre-built map) was considered and rejected — see Alternatives.

**Alternatives considered**:
- A more general `VersionedData(version int, extra map[string]any) map[string]any` that merges
  `version` into an arbitrary pre-built map, usable by all three call sites uniformly —
  rejected: `buildTerraformExecData` already assembles its full map in one literal; forcing it
  to build the map first, then call a merge function, adds a layer of indirection for zero
  benefit over adding one more key to the literal directly, and a merge-style signature is
  strictly more general (and more error-prone — e.g. a case-sensitive key collision) than this
  feature actually needs for its one non-conforming caller.
- Three independent literals, no shared helper at all — rejected only for the two genuinely
  identical call sites (`describe affected`/`list instances`); accepted for
  `buildTerraformExecData`, where it's the correct choice (see Decision text above) rather than
  a rejected alternative.
- Naming the field `schema_version` or `data_version` instead of `version` — rejected: the
  2026-08-20 clarification's own answer used the bare term `version`, and no other field in any
  of this feature's shapes uses a compound `_version` suffix that would create a naming
  precedent to follow instead.

## Decision 25: Pact coverage — 6 interactions (3 shapes × 2 delivery modes), constructed directly

**Decision**: The Pact consumer suite (`pkg/pro/consumer_pact_test.go`) gains dedicated
inline-mode and blob-URL-mode interaction pairs for each of the two new structured-`Data`
shapes (`describe affected`'s `{version, stacks}`, `list instances`' `{version, instances}`),
alongside the existing terraform pair (interactions 9/10) and the shared `/exec/data` blob
interaction (11) — six interactions total covering `Data` delivery, up from the existing
three. Each new pair is constructed directly: one interaction with `data` inline (a small,
representative `stacks`/`instances` array), one with `data` as a URL string paired with its own
`/exec/data` blob-upload interaction reusing the same request/response shape as interaction 11
— neither pair is produced by lowering `Settings.Pro.MaxPayloadBytes` and routing a fixture
through the real size-threshold decision code (`pkg/proexec`'s inline-vs-blob-URL check);
that logic continues to be covered by non-Pact unit tests (`pkg/proexec/*_test.go`), per the
2026-08-20 clarification's explicit answer.

**Rationale**: FR-013/the Assumptions section (as amended 2026-08-20) requires per-shape
coverage, not one representative example standing in for all three — each shape has distinct
fields (`stacks[*]` is a `schema.Affected`, `instances[*]` is a `dtos.UploadInstance`, neither
resembling `TerraformExecData`'s fields), so a consumer contract that only demonstrated the
terraform shape would leave Atmos Pro's provider implementation unable to validate/parse the
other two shapes from the contract artifact alone (violating SC-006's "without reading Atmos's
Go source code" bar for the two new shapes). Constructing the interactions directly (not via
the real threshold logic) keeps the new Pact cases fast and deterministic — a Pact interaction
asserts a request/response *shape*, not the code path that decided to produce that shape;
that decision is already unit-tested with a real threshold (the seventh re-plan's Testing
section already listed `pkg/proexec`-level threshold tests, unaffected by this decision).

**Alternatives considered**:
- One representative shape (terraform's) demonstrating both delivery modes, with the two new
  shapes covered only in inline mode (on the theory that "the blob-URL wire shape is identical
  regardless of which command's data is inside it") — rejected: explicitly rejected by the
  2026-08-20 clarification's own wording ("not only for one representative command"); the
  wire-shape argument is true for the *envelope* (`data` as a URL string looks the same
  regardless of payload), but the contract's job is to let Atmos Pro validate each shape's own
  fields, which only the inline-mode interaction actually exercises per shape — so skipping
  blob-URL mode for two of three shapes would still leave a real (if narrow) coverage gap for
  "does the blob-URL variant's `execution_id` pairing behave identically for this shape," even
  if unlikely to differ in practice.
- Route each new shape's blob-URL case through the real threshold check with an artificially
  small `MaxPayloadBytes`, matching quickstart.md's manual-verification pattern — rejected by
  the clarification answer itself; a Pact consumer test's job is contract-shape verification,
  and coupling it to the actual threshold-decision code path would make these tests brittle
  against unrelated changes to that logic and slower to write/maintain for a benefit (exercising
  the real decision) already delivered by the separate unit tests.

---

## Decision 26: List-typed `Data` fields normalize to `[]`, never `null`

**Decision**: Every list-typed field in every command-specific structured `Data` shape
(`TerraformExecData`'s `changes`/`warnings`/`errors` today; any future shape's list fields)
MUST be initialized as a non-nil, zero-length slice before marshaling, so an empty case
serializes as JSON `[]`, never `null`.

**Rationale**: A real CI payload (`atmos-pro-qa-3` run 32412509172, surfaced in the
2026-08-20 clarify session) showed `errors: null` and `changes: null` for a run with none of
either — a plain Go nil-slice-marshals-to-`null` artifact, since `buildTerraformExecData`
(`cmd/terraform/utils.go:740`) passes `result.Errors`/`terraformResourceChanges(...)`'s
return value straight into the map literal without a nil check. This contradicts
data-model.md's own worked example (`"errors": []`) and forces every consumer to treat
`null` and `[]` as equivalent, which is exactly the kind of wire-shape ambiguity FR-013's
"verifiable, versioned description" exists to eliminate.

**Alternatives considered**:
- Leave `null` as acceptable and just fix the doc's example to match — rejected: pushes the
  null-vs-`[]` normalization burden onto every consumer (Atmos Pro and any future integrator)
  instead of the one producer; Go's `omitempty` doesn't apply here either, since these fields
  are always present, just sometimes empty.

## Decision 27: `exit_code` — the authoritative pass/fail/parse-completeness signal

**Decision**: `TerraformExecData` gains an `exit_code` field: the terraform/tofu subprocess's
own process exit code, populated from the same `errUtils.GetExitCode(execErr)` call already
used elsewhere in `cmd/terraform` (e.g. `execNodeResult.ExitCode`, `outcome.ExitCode`) — not
a new exit-code-detection mechanism. This is distinct from the base `ExecutionRecord`'s own
`ExitCode` field (FR-003), which is the `atmos` process's own exit code and can differ from
the terraform subprocess's (e.g. a multi-component run where `atmos` exits non-zero because
one of several components failed, while a given component's own subprocess exited 0).
Consumers MUST treat `TerraformExecData.exit_code` — not the presence/absence/zero-ness of
`resource_counts`/`outputs`/`changes` — as the authoritative signal for whether a given
plan/apply/deploy actually failed or was fully parsed.

**Rationale**: The same real payload showed `has_changes: true` next to all-zero
`resource_counts` and an empty `outputs` object — the top-level status flags
(`has_changes`/`has_errors`, Decision 20) and the itemized data (`resource_counts`/`outputs`/
`changes`) are populated from related but distinct parts of `citerraform.ParseOutput`'s
result and can silently diverge when the parser detects a change occurred but fails to
extract itemized detail for this run's specific output format. Rather than inventing a new
`partial_parse`-style indicator (considered and rejected — see Alternatives), reusing
`exit_code` gives consumers a signal they can already reason about without learning a new
concept, and one that is orthogonal to parsing quality entirely: a non-zero exit code means
the subprocess itself reported failure, independent of how much of its output Atmos
successfully parsed afterward.

**Alternatives considered**:
- A new `partial_parse: bool` (or similar) indicator flagging "has_changes/has_errors were
  determined but itemized fields could not be" — rejected by explicit user direction in the
  2026-08-20 clarify session: `exit_code` was chosen instead as a single field that already
  exists conceptually (every subprocess has one) rather than a new bespoke concept a consumer
  would need documentation to interpret correctly.

## Decision 28: `exit_code` scoping — per-component for multi-component runs, never one aggregate value

**Decision**: For a single-component invocation, `exit_code` is a top-level field in
`TerraformExecData` alongside `component`/`stack` (Decision 21). For a multi-component
`--affected`/`--all` invocation (FR-006a), `exit_code` is instead reported per-component, on
each entry in the folded aggregate breakdown — there is no separate top-level aggregate
`exit_code` field. Concretely, this reuses the exit code multi-component aggregation already
captures per node: `execNodeResult.ExitCode` (`cmd/terraform/utils.go:538`,
`recordExecResult`) already threads each node's own exit code into its per-node entry in the
aggregate `Data`; this decision requires that field to be treated as the multi-component
counterpart of the new single-component `exit_code` field, not a separate/parallel concept
requiring new plumbing.

**Rationale**: Each component in a multi-component run executes its own terraform/tofu
subprocess with its own exit code; collapsing those into one aggregate value would either
mask a per-component failure (if aggregated as "0 unless all failed") or falsely flag every
component as failed (if aggregated as "non-zero if any failed"), either way losing the
per-component granularity `execNodeResult` already preserves today. Reusing the existing
field rather than adding a new one keeps this decision a pure documentation/contract fix, not
a new implementation surface.

## Decision 29: Minimal `Data` is still attached when itemized parsing fails entirely

**Decision**: When a component's terraform output cannot be parsed at all (e.g. the
subprocess crashed or produced no recognizable terraform-shaped output, so
`parseTerraformOutputMirror` would otherwise return `(nil, false)`), `buildTerraformExecData`
MUST still return a non-nil `TerraformExecData` populated with `version`, `exit_code`, and
`component`/`stack` (when known) — with every field that could not be parsed defaulted to its
empty/zero/false equivalent (`resource_counts` all `0`, `outputs: {}`, `changes`/`warnings`/
`errors`: `[]` per Decision 26, `has_changes`/`has_errors`: `false`) — rather than the current
behavior of omitting `Data` entirely for that component.

**Rationale**: `exit_code` (Decision 27) is specifically meant to be available as a
disambiguating signal in exactly the case where the rest of the payload is empty or
inconsistent; if `Data` is simply absent whenever parsing fails completely, the one case
where `exit_code` is most needed — a run whose output Atmos couldn't parse at all, which is
precisely when a consumer most needs to know whether the underlying command actually failed —
is the one case where it's unavailable. This changes `buildTerraformExecData`'s return
contract: it goes from "return `nil` unless `parseTerraformOutputMirror` succeeds" to "return
`nil` only when there is no exit code and no output at all to report" (i.e., effectively never
for a `terraform plan`/`apply` invocation that actually ran a subprocess, since an exit code
is always known by the time this function is called).

**Alternatives considered**:
- Keep the current all-or-nothing behavior and rely solely on the base `ExecutionRecord`'s own
  `exit_code` for this edge case — rejected: for a multi-component run, the base record's exit
  code is the aggregate `atmos` process's own code, which cannot distinguish which specific
  component failed to parse versus which failed for another reason; per-component `exit_code`
  (Decision 28) only has value if it's actually present even when parsing otherwise fails.

## Decision 30 (SUPERSEDED — see Decision 30r): `terraform deploy` — two full `TerraformExecData` objects, not one collapsed-as-apply view

**Original decision** (retracted, kept here for the record): `deploy`'s structured `Data`
would be redefined as `{"version": 1, "component": ..., "stack": ..., "plan": {...}, "apply":
{...}}`, on the premise that "deploy internally runs plan then apply as two separate
terraform/tofu subprocess invocations." **That premise is factually wrong** — see Decision
30r.

## Decision 30r: `terraform deploy` keeps the single collapsed-as-apply `TerraformExecData` shape

**Decision**: `deploy`'s structured `Data` is the same `TerraformExecData` shape as
`plan`/`apply` — no `plan`/`apply` sub-object split. `deploy` continues to be parsed with
apply semantics (`parseTerraformOutputMirror`, `cmd/terraform/utils.go`: `if parseCommand ==
"deploy" { parseCommand = "apply" }`, unchanged), now also carrying Decisions 26/27/29's
`exit_code`/null-safe-lists/minimal-Data amendments identically to `plan`/`apply`.

**Rationale**: Decision 30's premise — "deploy internally runs plan then apply as two
separate terraform/tofu subprocess invocations" — was discovered false during implementation.
`internal/exec/terraform.go`'s `handleDeploySubcommand` rewrites `info.SubCommand` from
`"deploy"` to `"apply"` **in place, before any subprocess runs** ("downstream terraform
invocation logic can treat them uniformly"). `atmos terraform deploy` therefore executes
exactly one `terraform apply -auto-approve`-equivalent subprocess call — one captured
stdout/stderr stream, one exit code. Terraform's own `apply` computes an implicit plan
internally as part of that single process and prints combined plan-then-apply text output,
but this is one OS-level subprocess invocation, not two Atmos-orchestrated ones — there is no
independent "plan phase" exit code or itemized output for Atmos to capture and report
separately from the apply phase's, because Atmos never runs a plan phase as a separate step
for `deploy`. A `{plan, apply}` wire shape built by parsing the *same single captured output*
twice would produce two identical (or arbitrarily/incorrectly split) sub-objects with no
independent basis for either — worse than the single collapsed view it was meant to improve
on, since it would imply two real subprocess executions to any consumer reading the shape.

**Alternatives considered** (post-correction):
- Change `deploy`'s actual execution to run two real subprocesses (a genuine `plan`, then a
  genuine `apply`) so the two-phase shape has real, independent data to report — rejected:
  this changes `deploy`'s real-world runtime behavior (timing, auto-approve semantics, the
  meaning of `--from-plan`/`--planfile`), a materially larger and riskier change than a
  structured-data reporting fix, and out of scope for a feature whose stated purpose is
  reporting on executions, not changing how they run. Flagged to the user during
  implementation; explicitly declined in favor of reverting to Decision 30r.
- Keep Decision 30's shape but populate `plan`/`apply` identically (both parsed from the same
  single captured output) — rejected: actively misleading, since it would present one
  subprocess's outcome as if it were two independently-meaningful ones, the opposite of what
  Decision 30 was trying to achieve (losslessly preserving genuinely distinct phase data that,
  it turns out, doesn't exist to preserve).

---

## Resolved NEEDS CLARIFICATION Items

All ambiguities were resolved during the `/speckit-clarify` sessions (2026-08-11,
2026-08-18, 2026-08-19, 2026-08-20 — four separate sessions) before planning began. No NEEDS
CLARIFICATION markers remain in this plan or the spec.
