# Fix: Route yq's internal diagnostics through the Atmos logger and close the remaining cross-package race

**Date:** 2026-07-28

## Summary

PR #2822 (merged upstream) fixed the crash reported in #2821 — concurrent stack-file processing corrupting
two of yq's process-global variables — but only within `pkg/utils`, and it still wrote yq's diagnostics
straight to yq's own `go-logging` backend (unformatted, unmasked, direct to stderr). This change centralizes
yq's global state in a new `internal/yq` package shared by both callers (`pkg/utils` and `pkg/yaml`), closing
a residual cross-package data race #2822 left in place, and installs a `go-logging` backend adapter that
forwards yq's diagnostics through the Atmos logger — so they inherit Atmos's formatting, configured log
destination, and secret masking, matching the existing `pkg/logger/logrus_adapter.go` precedent for
third-party loggers.

## Context

Issue `#2822` fixed `#2821`'s two process-globals (yq's `go-logging` module level and `yqlib.ExpressionParser`) by
adding a private mutex-guarded `setYqLogLevel` and a `sync.Once`-guarded parser init, but both live entirely
inside `pkg/utils/yq_utils.go`. `pkg/yaml/edit.go` calls the *same* process-global `go-logging` state
(`logging.SetLevel(..., "yq-lib")`) directly and independently, with no shared synchronization. Reproduced
with `go test -race -run TestYqConcurrentEvaluationAcrossCallers ./pkg/utils/...` (32+16×2 goroutines split
between `pkg/utils.EvaluateYqExpression` and `pkg/yaml.Query` running concurrently): the race detector still
flags a concurrent map read/write in `go-logging`'s `moduleLeveled.levels` map, because `SetBackend` always
wraps a plain `Backend` in that map-based type unless the backend already satisfies `Leveled` itself.

Separately, yq processes YAML that can carry secrets, and its diagnostics (via `go-logging`'s default
backend) went straight to stderr, bypassing Atmos's I/O layer entirely — no Atmos formatting, no respect for
`Logs.File`, and no masking of any secret the diagnostic message might echo back. Atmos's existing precedent
for this exact problem is `pkg/logger/logrus_adapter.go`'s `ConfigureLogrusForAtmos`, which redirects
`saml2aws`'s logrus output through the Atmos logger. This change applies the same pattern to yq's
`go-logging` output.

## Changes

- Added `internal/yq/backend.go`: a `yq` package that installs a single process-global `go-logging` backend
  (`goLoggingBackend`) via `logging.SetBackend` in an `init()`, before any goroutine exists. The backend type
  implements `logging.Backend` and `logging.Leveled` directly with an `atomic.Int32` level, so
  `go-logging`'s `AddModuleLevel` (called internally by `SetBackend`) detects it already satisfies
  `LeveledBackend` and installs it as-is — the unsynchronized `moduleLeveled.levels` map is never in the
  picture, for any caller, closing the residual cross-package race. `Log()` masks the record's message with
  `pkg/io.MaskString` (the same global masker Atmos's I/O layer uses) and dispatches by level to the Atmos
  logger (`pkg/logger`) via an injectable `levelLogger` interface, mirroring `logrusAdapter`'s test-injection
  pattern. `SetLevel` and `InitExpressionParser` (wrapping `yqlib.InitExpressionParser` in a `sync.Once`) are
  exported for both callers to share.
- `internal/yq/backend.go` also exports `WithEvaluationLevel(level, fn)`, guarded by a package-level
  `evalMu sync.Mutex`: it sets the level and runs `fn` (the actual yq evaluate call) with that level held
  fixed for the whole call, serialized against every other caller. The atomic level alone prevented a data
  race on the value but not a semantic one: a Trace-configured evaluation and a concurrent Silent-configured
  one (the YAML editor always wants silent, regardless of Atmos's configured log level) could each set their
  own level and then run under whichever level the other caller set last. Every yq evaluation now goes
  through this shared scope instead of calling `SetLevel` and evaluating separately (caught in review on
  #2826 by CodeRabbit).
- `pkg/utils/yq_utils.go`: removed the package-private `yqLogLevelMu` mutex, `yqLogModule`/`yqSilentLevel`
  constants, `init()`, and `initYqExpressionParser`/`setYqLogLevel` that #2822 added. `configureYqLogger` was
  replaced with a pure `yqLoggerLevel(atmosConfig) logging.Level` that only computes the desired level;
  `EvaluateYqExpression` and `EvaluateYqExpressionWithType` call `atmosyq.InitExpressionParser()` once and
  then run their `evaluator.Evaluate(...)` call inside `atmosyq.WithEvaluationLevel(yqLoggerLevel(atmosConfig), ...)`.
- `pkg/yaml/edit.go`: `evaluateWithOptions` runs its `yqlib.NewStringEvaluator().Evaluate(...)` call inside
  `atmosyq.WithEvaluationLevel(atmosyq.SilentLevel, ...)` instead of calling `atmosyq.SetLevel` and evaluating
  as two separate steps.
- `pkg/utils/yq_utils_test.go`: replaced `TestConfigureYqLogger`/`TestConfigureYqLogger_LevelIsReversible`
  (which exercised the removed side-effecting `configureYqLogger`) with a table-driven `TestYqLoggerLevel`
  against the new pure function, and updated `TestEvaluateYqExpression_ConcurrentCallsAreRaceFree` to drop
  the now-nonexistent standalone level-priming call.
- `pkg/utils/yq_concurrency_external_test.go` (new): `TestYqConcurrentEvaluate` and
  `TestYqConcurrentEvaluationAcrossCallers` — the regression coverage that reproduces the residual
  cross-package race described above and proves it's gone.
- `internal/yq/backend_test.go` (new): `TestBackend_Log_RoutesToCorrectLevel` (table-driven, all six
  `go-logging` levels), `TestBackend_Log_SilentLevelBlocksEverything`, `TestBackend_Log_MasksRegisteredSecrets`
  (a registered secret echoed in a yq diagnostic reaches the recording logger already redacted),
  `TestBackend_Log_ReachesConfiguredAtmosDestination` (a yq diagnostic written through `logging.MustGetLogger`
  shows up in the Atmos logger's configured `io.Writer`), `TestInitExpressionParser_Idempotent`, and
  `TestWithEvaluationLevel_ScopesLevelToCall` (a Trace-level and a Silent-level caller running concurrently,
  50 iterations each, asserting neither ever observes the other's level mid-call).

## Validation

- `go build ./...` and `go vet ./...` — clean.
- `gofumpt -l` on all changed/added files — no output (already formatted).
- `go test -race -count=5 ./internal/yq/...` and `go test -race ./pkg/utils/... ./pkg/yaml/... ./pkg/merge/...`
  — all pass, including `TestYqConcurrentEvaluationAcrossCallers` (fails under `-race` against #2822 alone)
  and `TestWithEvaluationLevel_ScopesLevelToCall` (confirmed to fail reliably when `WithEvaluationLevel`'s
  lock is removed, by temporarily reverting it and reapplying the fix).
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues, after addressing `godot`/`gosec`/`importas`/
  `lintroller`/`revive` findings from the first run.
- Not yet run: the full `atmos test --full` suite.

## Follow-ups

None. The `internal/yq/backend_test.go` and `pkg/utils/yq_concurrency_external_test.go` suites cover level
routing, masking, log-destination integration, the cross-package concurrency race, and the level-scoping
guarantee this change closes.
