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

The lifecycle assertions run in embedded Starlark and load reusable functions
from `scripts/lifecycle.star`. Kubernetes responses are decoded as JSON; absence
checks use `kubectl --ignore-not-found` so authentication, connection, and API
errors still fail. Expected Helm failures use `exec.run(check=False)` and must
match the lifecycle error as well as the specific timeout or hook failure.
Rollback compares structured deployment state before and after the operation.

The command supplies `ATMOS_CLI_PATH` and `RUNNER_OS` explicitly. Child output is
captured with `output="capture"`, including secret-bearing deployment JSON, so
assertions can inspect it without printing it. Existing retry limits and the
macOS-only upgrade timeout fallback are preserved. These dependent lifecycle
operations remain sequential.

The image-import pipeline and temporary-file checks for template rendering and
CI-summary masking still use shell. They exercise binary streaming or scoped
artifact cleanup, which the Starlark API does not yet provide.
