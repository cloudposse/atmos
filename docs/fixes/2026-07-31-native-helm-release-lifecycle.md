# Native Helm release lifecycle

**Date:** 2026-07-31

Native Helm cluster operations now expose Helm 4 wait, timeout, recovery,
history, hook, and CRD controls through stack configuration and explicit command
flags. Apply and delete dry runs now reach the Helm SDK without persisting release
state. Caller cancellation propagates through direct and dependency-ordered
execution into install and upgrade actions and into delete wait and hook phases;
Helm 4 does not expose a context-aware uninstall request.

Atmos reports the selected action and effective release policy before the Helm
action begins, including any `hookOnly` to `watcher` promotion required by
failure recovery. Bulk delete uses reverse dependency order so dependents are
removed before the releases they consume.

## Migration notes

- An omitted `release.timeout` remains `0s` (unbounded) for one minor release and emits a
  warning. The omitted default becomes `5m` in the following minor. Configure
  `release.timeout: 0s` explicitly to keep unbounded behavior without the warning.
- An omitted `release.history.max` retains ten upgrade revisions, matching the
  Helm CLI. Configure `release.history.max: 0` to retain unlimited history.
- Failure recovery is operation-specific: use `release.install.on_failure: uninstall`
  for failed first installs and `release.upgrade.on_failure: rollback` for failed
  upgrades. Upgrade cleanup is controlled independently by
  `release.upgrade.cleanup_on_failure`.
- Boolean `--wait=true` and `--wait=false` remain accepted temporarily; use
  `--wait=watcher` and `--wait=hookOnly`.
- Explicit lifecycle flags cannot be combined with a non-Kubernetes provision
  target. Stored lifecycle configuration is intentionally bypassed for external
  delivery and is identified as such in the execution summary.
- Chart-loading commands do not fetch missing dependencies unless
  `--dependency-update` is explicitly supplied. The opt-in follows Helm's
  dependency-update behavior and may access repositories and mutate the chart's
  `charts/` directory and lock file.
