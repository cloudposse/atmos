# Native CI Integration — CI Debug Mode → Atmos Log Level Auto-Promotion

> Related: [CI Detection](./ci-detection.md) | [Interfaces](./interfaces.md) | [Provider — GitHub Actions](../providers/github/provider.md)

## Context

When a workflow misbehaves, debugging Atmos is just as important as debugging the workflow around it. The whole point of this feature is to make that easy — users should be able to pull on one obvious lever and get verbose output from every tool in the run, including Atmos.

GitHub Actions provides exactly that lever: a built-in ["Re-run with debug logging"](https://github.blog/changelog/2022-05-24-github-actions-re-run-jobs-with-debug-logging/) button. Clicking it re-launches the workflow with two env vars set on every job by the runner itself:

- `ACTIONS_RUNNER_DEBUG=true` — runner diagnostic logging.
- `ACTIONS_STEP_DEBUG=true` — step debug logging.

(See [Enable debug logging](https://docs.github.com/en/actions/how-tos/monitor-workflows/enable-debug-logging) for the full mechanism.)

Today Atmos ignores those env vars. Even with `ci.enabled: true` in `atmos.yaml`, re-running with debug logging gets the user a noisier *runner* but the same quiet *Atmos* output — which is usually the part they actually want to inspect. The fix is to detect the signal and promote Atmos's own log level for the run, so the debug button does what a user expects: everything gets louder.

This is a common gap. Other GitHub-published tools have hit the same friction; see for example [pypa/gh-action-pypi-publish#322](https://github.com/pypa/gh-action-pypi-publish/issues/322), which asks for the same env-var-driven debug promotion in a different ecosystem.

## Goal

When (a) `ci.enabled: true` in `atmos.yaml` AND (b) the detected CI provider reports its run-level debug mode is active, automatically promote `atmos.Logs.Level` to `Debug` before the logger is initialized, and announce the promotion in the logs.

## Non-goals

- Promoting to `Trace`. The CI-side toggle maps to "verbose," not "everything."
- Listening to the generic `CI` environment variable. That signal is too coarse — every CI run sets it, and it would silently make every Atmos invocation in CI emit debug logs even when the user did not opt in.
- Mapping `RUNNER_DEBUG=1` (which the runner sets *itself* when `ACTIONS_RUNNER_DEBUG` is on). The user-facing toggles are the documented, stable contract.
- Demoting the log level when the user explicitly set something verbose. The CI-side toggle is an explicit "be verbose" signal; we don't fight it.

## Functional requirements

### FR-1: Provider-discovered capability

The startup path **MUST NOT** know about any specific CI provider's environment variables. The runner-debug capability is exposed by the provider through an optional interface in `pkg/ci/internal/provider`:

```go
// DebugModeDetector is an optional capability for providers that expose a
// "debug mode" signal set at the runner / step / job level.
type DebugModeDetector interface {
    IsDebugMode() bool
}
```

Providers that have no analogous toggle simply do not implement the interface; `DetectDebugMode` returns `Active=false` for them.

### FR-2: Provider-agnostic registry helper

`pkg/ci` exposes one helper that does the type assertion and returns a value type the call site can render generically:

```go
type DebugModeInfo struct {
    Active   bool   // detected provider reports debug mode active
    Provider string // detected provider name, empty when no CI detected
}

func DetectDebugMode() DebugModeInfo
```

The call site reads only the public `pkg/ci` package — it does not import the GitHub provider directly and it does not name the internal interface.

### FR-3: Promotion semantics

`cmd.maybePromoteLogLevelForDebugMode(cfg, configLoaded)` promotes `cfg.Logs.Level` to `"Debug"` when **all** of the following are true:

1. `configLoaded` is true (we have a valid `atmos.yaml`).
2. `cfg.CI.Enabled` is true.
3. `ci.DetectDebugMode().Active` is true.

The promotion is **unconditional with respect to the prior level**. It overrides:

- `--logs-level` from the CLI.
- `ATMOS_LOGS_LEVEL` from the environment.
- `logs.level` from `atmos.yaml`.
- The startup default.

This is intentional. The repo/workflow-level CI debug toggles are a high-priority, explicit user signal: when set, the user wants *every* tool in the run to be verbose. Atmos respecting them outranks any per-invocation `--logs-level` the workflow happens to pass.

`Trace` is also overridden (promoted *down* to `Debug`). The reasoning is symmetry: the user opted into "verbose" at the CI level; Atmos honors that exact request rather than second-guessing them with `Trace`.

### FR-4: Announce the auto-detection

After `SetupLogger`, the call site emits a single Info-level log line so the user understands why their output got louder:

```text
INFO CI provider debug mode detected — using Debug log level for this run provider=github-actions from=Info
```

The message names only the detected CI provider — no GHA-specific env var names. The user already knows which toggles are in play on their CI side; we just acknowledge the signal arrived.

### FR-5: No effect outside the gate

If any of the three conditions in FR-3 are false, `maybePromoteLogLevelForDebugMode` returns a zero `debugModePromotion` and leaves `cfg.Logs.Level` untouched. Specifically:

- `ci.enabled: false` (or unset) → no promotion, even with `ACTIONS_STEP_DEBUG=true`. `ci.enabled` is the hard kill switch ([CI Detection](./ci-detection.md)).
- No CI provider detected (e.g., local shell) → no promotion.
- Detected provider does not implement `DebugModeDetector` → no promotion.

## Provider implementation: GitHub Actions

`pkg/ci/providers/github/debug.go` implements the optional interface as a method on the existing `*Provider`:

```go
func (p *Provider) IsDebugMode() bool {
    if os.Getenv("GITHUB_ACTIONS") != "true" {
        return false
    }
    return os.Getenv("ACTIONS_RUNNER_DEBUG") == "true" ||
        os.Getenv("ACTIONS_STEP_DEBUG") == "true"
}
```

`GITHUB_ACTIONS == "true"` is technically redundant in production because the GHA provider only registers itself when `GITHUB_ACTIONS=true` (see `provider.go:Detect`). It is kept so the method behaves correctly when called directly in tests outside a real GHA environment.

Env var values must equal the literal string `"true"` — `"1"`, `"TRUE"`, `"yes"` are all rejected. This matches the strictness GitHub's own runner uses and prevents accidental triggers.

## Pipeline placement

The promotion happens between config init and `SetupLogger` in `cmd/root.go`:

```go
// after InitCliConfig has populated atmosConfig, before SetupLogger:

debugPromote := maybePromoteLogLevelForDebugMode(&atmosConfig, initErr == nil)

SetupLogger(&atmosConfig)

if debugPromote.Promoted {
    log.Info("CI provider debug mode detected — using Debug log level for this run",
        "provider", debugPromote.Provider,
        "from", debugPromote.From,
    )
}
```

`SetupLogger` reads `atmosConfig.Logs.Level` directly, so the mutation must precede the call. The announcement is emitted *after* `SetupLogger` so the global logger is ready to format it; the announcement uses `log.Info`, which is now visible because the level was just promoted to `Debug` (Info ≤ Debug).

`ExecuteVersion` (called for `atmos --version` before the full config-init flow) intentionally skips this path. Without a loaded `atmos.yaml` we have no `CI.Enabled` to read, and `--version` is meant to be cheap and side-effect-free.

## Precedence summary

The CI-debug auto-promote interacts with the existing log-level precedence ([Configuration](../../../cli/configuration/logs.mdx)) as follows:

| Source                                    | Beats |
|-------------------------------------------|-------|
| CI provider debug-mode (this PRD)         | everything below, when `ci.enabled: true` |
| `--logs-level` CLI flag                   | env, config, default |
| `ATMOS_LOGS_LEVEL` env var                | config, default |
| `logs.level` in `atmos.yaml`              | default |
| Default (`Info`)                          | — |

## Testing

Unit tests (no fixtures needed):

- `pkg/ci/providers/github` — `(&Provider{}).IsDebugMode()` matrix across `GITHUB_ACTIONS`, `ACTIONS_RUNNER_DEBUG`, `ACTIONS_STEP_DEBUG`. Includes negative cases for `"1"`, `"TRUE"`, and explicit `"false"`.
- `pkg/ci` — `DetectDebugMode()` with (a) empty registry, (b) detected provider without the capability, (c) detected provider with capability returning `false`, (d) detected provider with capability returning `true`.
- `cmd` — `maybePromoteLogLevelForDebugMode` table covering each gate (config-not-loaded, `ci.enabled=false`, no provider, no debug env), each positive case (`ACTIONS_RUNNER_DEBUG`, `ACTIONS_STEP_DEBUG`), and the override semantics (promotes from `Info`, `Warning`, `Trace`, `Off`).

End-to-end smoke (run locally against a fixture with `ci.enabled: true`):

```bash
# Promotion fires and Debug logs appear despite --logs-level Warning.
env GITHUB_ACTIONS=true ACTIONS_STEP_DEBUG=true \
    ./build/atmos --logs-level Warning ci status

# Negative: ci.enabled=false fixture → no promotion.
env GITHUB_ACTIONS=true ACTIONS_RUNNER_DEBUG=true \
    ./build/atmos --logs-level Info version
```

## Extending to other providers

To add the capability to a new provider (e.g., GitLab):

1. In the provider package, add a method on the provider struct:
   ```go
   func (p *Provider) IsDebugMode() bool {
       return os.Getenv("CI_DEBUG_TRACE") == "true"
   }
   ```
2. That's it. `provider.DebugModeDetector` is an optional interface; satisfying it is enough — no registration changes, no edits to `pkg/ci` or `cmd/root.go`.

No further changes are needed because:
- `ci.DetectDebugMode` performs the type assertion against whichever provider `Detect()` returns.
- The startup announcement uses `provider.Name()`, so the new provider's name appears in the log line automatically.

## Open questions / future work

- **Should a provider implement `DebugModeDetector` with `IsDebugMode() bool` returning `false` even when their CI has no analogous toggle?** No — the optional-interface pattern means "I don't have one" maps to "don't implement it." `DetectDebugMode` cleanly returns `Active=false` via the type-assertion miss.
- **Should we also surface the provider's debug-mode flag to plugins (e.g., to pass `TF_LOG=DEBUG` through to Terraform automatically)?** Out of scope for this PRD. The Atmos log level is the immediate target; downstream tool log levels can be wired later through the same `DebugModeInfo` value.
