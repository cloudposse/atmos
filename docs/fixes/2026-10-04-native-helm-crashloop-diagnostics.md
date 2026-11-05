# Fix: native Helm surfaces crash-looping pod diagnostics on release failure

**Date:** 2026-10-04

**Issue:** [cloudposse/atmos#3271](https://github.com/cloudposse/atmos/issues/3271)

## Summary

When a native-Helm release fails its readiness wait, the error (added in #2849) named the operation,
release, namespace, wait strategy, timeout, and config source - but not *why* the workload was not
ready. Operators had to leave Atmos and run `kubectl get pods`/`describe`/`logs`, and when
`upgrade.on_failure: rollback` was configured the rollback deleted the failing pods first, destroying
the evidence.

Atmos now enumerates the release's pods on failure and folds their diagnostics - not-ready container
status (`CrashLoopBackOff`/`ImagePullBackOff`/exit code/restart count), and, at debug/trace level, a
tail of the failing container's log plus the pod's recent events - into the same error. Crucially,
this happens **before** the rollback/uninstall deletes the pods.

Example (`-logs-level=Debug`):

```
Error: failed to perform helm release operation
  workload diagnostics:
    pod keda-operator-7d9f  keda-operator CrashLoopBackOff (exit 1, 5 restarts)
      last log (keda-operator):
        panic: failed to load config: invalid duration "5x"
      events:
        BackOff  Back-off restarting failed container
```

## Root cause

Helm v4 performs its `RollbackOnFailure` / uninstall-on-failure *inside* `action.Upgrade.RunWithContext`
/ `action.Install` (see `failRelease` in Helm's `pkg/action/upgrade.go`). By the time Atmos received the
error, the rollback had already replaced the crashing pods, so any diagnostics Atmos collected afterward
would race the pod teardown.

## Changes

- `pkg/component/helm/diagnostics.go` (new): `collectReleaseFailureDiagnostics` lists the release's pods
  by the standard `app.kubernetes.io/instance=<release>` label, summarizes not-ready init/regular
  containers, and - when verbose - tails the failing container's previous-instance log (where a crash
  log lives) and the pod's recent events. It is strictly best-effort: any cluster-access error returns
  an empty string so diagnostics never mask the original failure. Output is bounded (max pods/events,
  log tail lines, message length). The clientset is built behind the `newReleaseClientset` seam for
  testing.
- `pkg/component/helm/client.go`:
  - `configureInstallLifecycle` / `configureUpgradeLifecycle` no longer set Helm's `RollbackOnFailure`;
    Atmos owns recovery so diagnostics can run first.
  - `installRelease` / `upgradeRelease` collect diagnostics on failure (gated behind debug/trace for the
    log tail and events), then perform the rollback (`rollbackFailedUpgrade`, via `action.NewRollback`
    honoring `MaxHistory`/`CleanupOnFailure`, with `enforceReleaseHistoryLimit` as the authoritative
    history trim) or uninstall (`uninstallFailedInstall`, via `action.NewUninstall`).
  - `releaseOperationErrorWithDiagnostics` folds the diagnostics into the #2849 error as an explanation.
- `errors/errors.go`: new `ErrHelmReleaseRollback` sentinel.

## Behavior

- Normal (success) output is unchanged; diagnostics are produced only on failure.
- Container-status summaries are always included on failure; the log tail and events are included only at
  debug/trace level (`-logs-level=Debug`/`Trace`), so default failure output stays concise.
- `on_failure: rollback` (upgrade) and `on_failure: uninstall` (install) still roll back / uninstall as
  before; the only change is that Atmos performs them after collecting diagnostics, preserving the
  history-retention behavior from #2849.
- Dry-run failures do not query the cluster (nothing was applied).

## Validation

- `pkg/component/helm` unit tests: the collector (crash-loop/image-pull/terminated/init-container cases,
  verbose vs. non-verbose, release-label scoping, healthy/empty, clientset/pod-list/event-list errors)
  via a fake clientset; `containerFailureSummary`/`isFailureReason`/`truncate`/`indentLines`; and
  end-to-end that a failed upgrade and a failed install fold the diagnostics into the returned
  `ErrHelmReleaseOperation` before the rollback/uninstall, which still runs.
- Existing lifecycle tests (rollback history retention, dry-run, cancellation) still pass with
  Atmos-owned recovery; `TestConfigureReleaseLifecycleActions` updated to reflect that Helm's
  `RollbackOnFailure` is now left off.
- `go build ./...`, `go test ./pkg/component/helm/...`, and `golangci-lint` (0 issues) pass.
