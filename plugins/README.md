# Official Atmos plugins

Official plugins are independently versioned native packages in this monorepo. Consumers install precompiled binaries from `oci://ghcr.io/cloudposse/atmos/plugins/<name>`; they do not need Go or this repository. The first package is [auth-secrets](auth-secrets/README.md).

## Versions and declared inputs

Each package's `plugin.yaml` is authoritative: `metadata.name` is `cloudposse/<name>` and `metadata.version` is a stable `major.minor.patch`. An Atmos release does not advance plugin versions. Release tags in GHCR use the manifest version, without `v`.

`releases.yaml` declares each package directory, OCI repository, shared build inputs, and platform-to-runner mappings. `atmos plugin release plan` reads this committed inventory and emits package/build matrices. Changing package files or declared SDK/protocol inputs requires increasing the affected package's version. Inventory changes affect only packages whose entries or platform runner mappings changed. Changes elsewhere do not require plugin bumps. The planner rejects downgrades and missing runner mappings. With `--base`, it checks version increases against the accepted base.

Official package modules use a local SDK replacement. CI records the latest commit affecting the package, declared shared inputs, or inventory, and checks out that exact source. Unrelated commits retain the previous source; reviewed changes within one proposed release can span several commits. This is the official monorepo build convention; external plugin projects should depend on a released SDK version.

## Validation and publication

The Tests workflow calls `.github/workflows/plugin-packages.yaml` after building Atmos, reusing the four existing host artifacts and building only the two additional ARM64 hosts. Plugin compilation uses a separate Go build cache. Ready pull requests, merge-queue entries, and trusted pushes validate packages. It tests Linux, macOS, and Windows on both amd64 and arm64 runners, runs native protocol conformance, then validates one complete OCI layout. Only a successful complete matrix can publish. Pull requests have no publishing credentials. A manual run on `main` reconciles accepted versions after interrupted publication.

Publishing uses the protected `plugin-release` environment, package-specific concurrency, immutable version tags, and signing. Rebuilding the same recorded source is an idempotent retry when bytes match; differing bytes under an existing version fail instead of overwriting it. Preserve the workflow's Go toolchain pin and build inputs. Changes that alter release bytes need a new package version.

`atmos plugin release` owns the local build/test/package/publish lifecycle, including temporary layout cleanup. Distributed CI uses the individual commands to transfer artifacts between native runners. `plugin publish --verify-public` owns visibility preflight and anonymous verification; `plugin catalog sync --inventory plugins/releases.yaml` reads committed package manifests and owns catalog reconciliation without requiring a release plan or CI runner mappings. Custom commands are optional aliases, and no Python release helpers are needed.

The separate catalog job holds `official-plugin-catalog` concurrency across fetching current `main`, reading accepted manifests, verifying public releases, and updating `catalog:stable`. It retains previously listed versions. It never publishes a snapshot prepared before acquiring the lock. A missing release is deferred to a later successful publisher or manual reconciliation; authentication and integrity failures fail the job. A catalog failure does not invalidate a published immutable package.

## Required public GHCR setup

**Both `atmos/plugins/auth-secrets` and `atmos/plugins/catalog` must be public GHCR packages before their first release.** GitHub creates new packages private by default, even in a public repository. The release workflow checks package visibility before uploading release bytes and checks anonymous access to the full OCI graph afterward. A private or inaccessible package fails closed.

An organization package administrator must provision the package namespaces, set **Package settings → Change visibility → Public**, and grant this repository Actions write access. If GHCR requires an initial object to create the namespace, use a clearly separate bootstrap tag containing no plugin release, then set public visibility before running this workflow. Do not publish a version tag or catalog `stable` while private. The workflow does not attempt an undocumented REST visibility mutation.

Configure the `plugin-release` environment to allow trusted `main` releases and any required organization approval policy. Ensure its `GITHUB_TOKEN` can read package metadata and write the packages, and that keyless signing has `id-token: write`. Restrict catalog writes to this workflow. Manual dispatch of the **Tests** workflow on `main` reruns the full validation and publication pipeline; it is not a bypass.

Public visibility is necessary for distribution, but does not grant trust. Consumers may additionally require signatures and an explicit trusted publisher policy.
