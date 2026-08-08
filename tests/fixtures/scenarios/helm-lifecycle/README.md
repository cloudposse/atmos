# Native Helm lifecycle scenario

This fixture exercises native Helm lifecycle behavior against the Kubernetes
emulator. It intentionally contains failure cases, delayed readiness, hooks,
Jobs, CRDs, dependency ordering, rollback, cleanup, dry-run, and timeout
coverage.

The user-facing happy-path demo remains in `examples/helm`.
