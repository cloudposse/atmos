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

Atmos's Helm plugin installer now uses clean staging directories and the shared
retry package for transient network failures: three attempts with 15- and
30-second backoffs. Permanent failures stop immediately, cancellation interrupts
backoff, and failed attempts never register partial plugins in the managed path.
A directory-scoped file lock prevents concurrent installs from racing.

For pinned releases of the canonical Helm Diff repository, Atmos downloads and
extracts the complete platform plugin archive with its existing Go dependencies.
This avoids two upstream behaviors observed during validation: Helm 3 can mistake
GitHub's archive content type for a Git repository, and Helm Diff's Windows hook
can silently download latest when `git describe` runs outside its checkout.
Atmos checks both metadata and the actual `helm diff version` output, including
on cache hits. A broken or mismatched cached binary is repaired. Other plugins
continue to install through Helm. Public commands and APIs are unchanged.

The existing Linux, macOS ARM, and Windows build jobs invoke
`atmos helm plugin install diff@<version>` once after installing the pinned
toolchain. The action scopes the managed installation to a private cache path;
shell code only handles artifact transport and environment setup.

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
- `go test -race -shuffle=on -coverprofile=.context/plugin-coverage.out ./pkg/helm/plugin ./cmd/helm`
- `python3 .github/actions/setup-helm-diff/test_helm_diff.py`
- `shellcheck .github/actions/setup-helm-diff/helm-diff.sh`
- `actionlint .github/workflows/test.yml`

Native Go tests cover clean staging, retry recovery and exhaustion, permanent
errors, cancellation, invalid metadata and binaries, cached-binary repair,
concurrent same-directory installs, platform URLs, and real archive downloads
from a loopback server. Action tests use fake Atmos and Helm executables with
real archives to cover delegation, error propagation, executable permissions,
version checks, corrupt archives, and independent concurrent consumers.
