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

#2822 fixed #2821's two process-globals (yq's `go-logging` module level and `yqlib.ExpressionParser`) by
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
- `pkg/utils/yq_utils.go`: removed the package-private `yqLogLevelMu` mutex, `yqLogModule`/`yqSilentLevel`
  constants, `init()`, and `initYqExpressionParser`/`setYqLogLevel` that #2822 added; `configureYqLogger` now
  computes the desired level and calls `atmosyq.SetLevel(...)`, and both `EvaluateYqExpression` and
  `EvaluateYqExpressionWithType` call `atmosyq.InitExpressionParser()`.
- `pkg/yaml/edit.go`: `evaluateWithOptions` now calls `atmosyq.SetLevel(atmosyq.SilentLevel)` and
  `atmosyq.InitExpressionParser()` instead of calling `logging.SetLevel` directly, so it shares the same
  atomic-backed level state and one-time parser init as `pkg/utils`.
- `pkg/utils/yq_utils_test.go`: updated the two tests that referenced the removed `pkg/utils`-local
  symbols (`TestConfigureYqLogger_LevelIsReversible`, `TestEvaluateYqExpression_ConcurrentCallsAreRaceFree`)
  to assert against `atmosyq.SilentLevel` and the public `logging.GetLevel("yq-lib")` instead.
- `pkg/utils/yq_concurrency_external_test.go` (new): `TestYqConcurrentEvaluate` and
  `TestYqConcurrentEvaluationAcrossCallers` — the regression coverage that reproduces the residual
  cross-package race described above and proves it's gone.
- `internal/yq/backend_test.go` (new): `TestBackend_Log_RoutesToCorrectLevel` (table-driven, all six
  `go-logging` levels), `TestBackend_Log_SilentLevelBlocksEverything`, `TestBackend_Log_MasksRegisteredSecrets`
  (a registered secret echoed in a yq diagnostic reaches the recording logger already redacted),
  `TestBackend_Log_ReachesConfiguredAtmosDestination` (a yq diagnostic written through `logging.MustGetLogger`
  shows up in the Atmos logger's configured `io.Writer`), and `TestInitExpressionParser_Idempotent`.

## Validation

- `go build ./...` and `go vet ./...` — clean.
- `gofumpt -l` on all changed/added files — no output (already formatted).
- `go test -race ./pkg/utils/... ./pkg/yaml/... ./internal/yq/...` — all pass, including
  `TestYqConcurrentEvaluationAcrossCallers`, which fails under `-race` against #2822 alone (confirmed before
  this change) and passes after centralizing the backend in `internal/yq`.
- Not yet run: `atmos lint --changed` / `./custom-gcl run --new-from-rev=origin/main` (no prebuilt
  `custom-gcl` binary available in this shell) and the full `atmos test --full` suite. These are outstanding
  before this change is PR-ready.

## Follow-ups

None. The new `internal/yq/backend_test.go` and `pkg/utils/yq_concurrency_external_test.go` cover level
routing, masking, log-destination integration, and the cross-package concurrency race this change closes.
