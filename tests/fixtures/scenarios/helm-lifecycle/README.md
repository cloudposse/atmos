# Native Helm lifecycle scenario

This fixture exercises native Helm lifecycle behavior against the Kubernetes
emulator. It intentionally contains failure cases, delayed readiness, hooks,
Jobs, CRDs, dependency ordering, rollback, cleanup, dry-run, and timeout
coverage.

It also installs pinned ingress-nginx chart `4.15.1` from the official
repository. That real-chart gate covers pre/post install and upgrade hook Jobs,
hook cleanup, repository acquisition, stack-level `!secret` values, multiline
masking, GitHub job-summary masking, upgrade convergence, and visible
apply/delete status output.

The user-facing happy-path demo remains in `examples/helm`.
