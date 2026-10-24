# Official Atmos plugins

Official plugins are independently versioned native packages in this monorepo. Consumers install precompiled binaries from `oci://ghcr.io/cloudposse/atmos/plugins/<name>`; they do not need Go or this repository. The first package is [auth-secrets](auth-secrets/README.md).

## Versions and declared inputs

Each package's `plugin.yaml` is authoritative: `metadata.name` is `cloudposse/<name>` and `metadata.version` is a stable `major.minor.patch`. An Atmos release does not advance plugin versions. Release tags in GHCR use the manifest version, without `v`.

`releases.yaml` declares each package directory and shared build inputs. Changing package files or declared SDK/protocol inputs requires increasing the affected package's version. Inventory changes conservatively require all existing package versions to increase. Changes elsewhere do not require plugin bumps. The planner rejects a downgrade, incomplete platform set, or input changes after the commit accepting the manifest.

Official package modules use a local SDK replacement. CI records and checks out the exact manifest commit, so the binary includes the SDK from that commit. This is the official monorepo build convention; external plugin projects should depend on a released SDK version.

## Validation and publication

`.github/workflows/plugin-packages.yaml` validates pull requests and trusted pushes to `main`. It tests Linux, macOS, and Windows on both amd64 and arm64 runners, runs native protocol conformance, then validates one complete OCI layout. Only a successful complete matrix can publish. Pull requests have no publishing credentials. A manual run on `main` reconciles accepted versions after interrupted publication.

Publishing uses the protected `plugin-release` environment, package-specific concurrency, immutable version tags, and signing. Rebuilding the same recorded source is an idempotent retry when bytes match; differing bytes under an existing version fail instead of overwriting it. Preserve the workflow's Go toolchain pin and build inputs. Changes that alter release bytes need a new package version.

The separate catalog job holds `official-plugin-catalog` concurrency across fetching current `main`, reading accepted manifests, verifying public releases, and updating `catalog:stable`. It retains previously listed versions. It never publishes a snapshot prepared before acquiring the lock. A missing release is deferred to a later successful publisher or manual reconciliation; authentication and integrity failures fail the job. A catalog failure does not invalidate a published immutable package.

## Required public GHCR setup

**Both `atmos/plugins/auth-secrets` and `atmos/plugins/catalog` must be public GHCR packages before their first release.** GitHub creates new packages private by default, even in a public repository. The release workflow checks package visibility before uploading release bytes and checks anonymous access to the full OCI graph afterward. A private or inaccessible package fails closed.

An organization package administrator must provision the package namespaces, set **Package settings → Change visibility → Public**, and grant this repository Actions write access. If GHCR requires an initial object to create the namespace, use a clearly separate bootstrap tag containing no plugin release, then set public visibility before running this workflow. Do not publish a version tag or catalog `stable` while private. The workflow does not attempt an undocumented REST visibility mutation.

Configure the `plugin-release` environment to allow trusted `main` releases and any required organization approval policy. Ensure its `GITHUB_TOKEN` can read package metadata and write the packages, and that keyless signing has `id-token: write`. Restrict catalog writes to this workflow. `workflow_dispatch` reruns the full validation and publication pipeline; it is not a bypass.

Public visibility is necessary for distribution, but does not grant trust. Consumers may additionally require signatures and an explicit trusted publisher policy.
