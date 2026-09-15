# Remove unintended source downloads and repeated Helm Diff installs in CI

## Problem

Two CloudFormation PR runs failed on unrelated network requests:

- PR #3001's `TestAutoProvisionSource_InvocationGuard_SetAfterProvisioning`
  seeded `.workdir/terraform/demo-null-label`, but the provisioner looked for
  the current hash-suffixed path. The cache miss downloaded a real GitHub
  repository, making a cache-hit regression test dependent on DNS.
- PR #3000's macOS acceptance shard failed installing Helm Diff when its release
  asset returned HTTP 500 after five download attempts. Each acceptance shard
  and registry-cache job independently repeated the same installation.

## Changes

The invocation-guard test now uses `workdir.BuildPath` and matching metadata.
A loopback HTTP server counts unexpected requests. The test asserts no fetch,
preserved cached contents, the resolved workdir path, the invocation guard, and
absence of the reprovisioning marker.

The existing Linux, macOS ARM, and Windows build jobs prepare Helm Diff once
after installing the pinned toolchain. Installation uses a private temporary
plugin directory and up to three attempts, with 15- and 30-second backoffs.
Failed partial installations are removed before retrying; exhaustion preserves
the final installation error.

The installed version is verified before publishing a tar archive, which
preserves executable permissions and excludes Git metadata. Each downstream
job downloads the platform/version artifact from the same workflow run,
extracts into its own temporary directory, verifies the binary version, and
exports `HELM_PLUGINS`. Restore never invokes installation hooks or falls back
to release downloads. Existing artifact-download retry behavior still applies.

The macOS Intel build has no consumer that uses this plugin and does not
prepare it. Shard counts, job parallelism, and production APIs are unchanged.

## Validation

- `go test -race -count=10 -shuffle=on ./pkg/provisioner/source -run TestAutoProvisionSource_InvocationGuard`
- `go test -race -shuffle=on ./pkg/provisioner/source ./pkg/provisioner/workdir`
- `python3 .github/actions/setup-helm-diff/test_helm_diff.py`
- `shellcheck .github/actions/setup-helm-diff/helm-diff.sh`
- `actionlint .github/workflows/test.yml`

Action tests use a fake Helm executable with real archives to cover successful
restoration, executable permissions, clean retry staging, retry exhaustion,
version mismatches, corrupt archives, invalid modes, and independent concurrent
consumers. The Tests workflow runs these checks in its existing magefiles job.
