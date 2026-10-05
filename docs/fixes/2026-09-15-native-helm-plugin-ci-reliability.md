# Harden native Helm plugin installation and source-cache tests

## Problem

The invocation-guard regression seeded an obsolete workdir path and unexpectedly
cloned GitHub, failing on DNS in PR #3001. Separately, PR #3000 failed before its
tests when a repeated Helm Diff download returned HTTP 500.

Atmos already supports Helm plugins through component `plugins` declarations.
The installer delegated to Helm, but did not retry transient failures, clean
partial installations, or distinguish successful installs from metadata left
behind by failed hooks.

## Changes

The source regression uses `workdir.BuildPath` and a loopback source. It asserts
zero download requests, preserved fixture contents, the resolved workdir, and
the expected invocation guard.

The existing generic plugin installer still delegates repository and version
selection to Helm. Hooks run in their final installation directory. Transient
failures get three attempts with 15- and 30-second backoffs; failed attempts
remove only newly created entries. A directory-scoped lock prevents concurrent
Atmos installs from racing. Completed installations receive a source/version
receipt after hooks and metadata validation succeed. Older installations without
a receipt are reinstalled once, repairing caches left incomplete by failed hooks.
Failed replacements restore a backup of the previous plugin; an unsuccessful
rollback retains the backup and reports its location.

Helm Diff is declared in a CI stack, using the existing plugin schema. Build jobs
install that declaration with a direct Atmos command. The existing `ci.cache`
includes the managed plugin directory and shares it with downstream jobs, where
the same command verifies the completed installation. Cache keys include the
plugin declaration so pin changes invalidate immutable entries. Each runner has
its own restored copy; shard counts and parallelism are unchanged.

There is no custom setup action, shell wrapper, artifact transport, or
plugin-specific downloader. Native Helm's diff operation uses the Helm Diff Go
library; Helmfile uses the CLI plugin declared by its component configuration.

## Validation

- `go test -race -count=10 -shuffle=on ./pkg/provisioner/source -run TestAutoProvisionSource_InvocationGuard`
- `go test -race -shuffle=on ./pkg/provisioner/source ./pkg/provisioner/workdir`
- `go test -race -shuffle=on -coverprofile=.context/plugin-coverage.out ./pkg/helm/plugin ./cmd/helm`
- `actionlint .github/workflows/test.yml`

Native tests cover transient recovery, exhaustion, permanent failures, cleanup,
cancellation, metadata validation, completed-install receipts, concurrent
installation, custom plugin names, and rollback. CI exercises the declarative
installation and cache on Linux, macOS, and Windows.
