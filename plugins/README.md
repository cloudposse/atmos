# Official Atmos plugins

Official plugins are independently versioned native packages in this monorepo. Consumers install precompiled binaries from `oci://ghcr.io/cloudposse/atmos/plugins/<name>`; they do not need Go or this repository. The first package is [auth-secrets](auth-secrets/README.md).

## Versions and declared inputs

Each package's `plugin.yaml` is authoritative: `metadata.name` is `cloudposse/<name>` and `metadata.version` is a stable `major.minor.patch`. An Atmos release does not advance plugin versions. Release tags in GHCR use the manifest version, without `v`.

`releases.yaml` declares each package directory, OCI repository, and shared build inputs. `atmos plugin release plan` reads this committed inventory and emits package/build matrices. Platform-to-runner mappings are optional for projects that choose distributed native tests; our releases cross-build on Linux. Changing package files or declared SDK/protocol inputs requires increasing the affected package's version. Inventory changes affect only packages whose entries or platform runner mappings changed. Changes elsewhere do not require plugin bumps. The planner rejects downgrades and incomplete runner mappings when mappings are supplied. With `--base`, it checks version increases against the accepted base.

Official package modules use a local SDK replacement. CI records the latest commit affecting the package, declared shared inputs, or inventory, and checks out that exact source. Unrelated commits retain the previous source; reviewed changes within one proposed release can span several commits. This is the official monorepo build convention; external plugin projects should depend on a released SDK version.

## Validation and publication

The Tests workflow calls `.github/workflows/plugin-packages.yaml` and reuses its existing Linux Atmos binary. The workflow has two Linux jobs: `check` runs `atmos plugin release --inventory plugins/releases.yaml --check`; the protected `release` job rebuilds the recorded sources and publishes them with `--sign --yes --verify-public --catalog cloudposse`. Atmos iterates the packages, cross-builds all declared targets with bounded concurrency, runs host tests, and validates complete OCI layouts. There is no package or platform matrix and no layout artifact transfer. Each job uses an isolated plugin Go cache.

Package tests run on Linux. The existing Plugin SDK workflow retains its six-platform native protocol and runtime checks; it does not run auth-secrets package tests on those runners. Cross-built macOS, Windows, and ARM binaries receive static format validation during packaging. Pull requests have no publishing credentials. A manual run on `main` reconciles accepted versions after interrupted publication.

Publishing uses the protected `plugin-release` environment, one global `official-plugin-releases` concurrency group, immutable version tags, and signing. Rebuilding the same recorded source is an idempotent retry when bytes match; differing bytes under an existing version fail instead of overwriting it. Preserve the workflow's Go toolchain pin and build inputs. Changes that alter release bytes need a new package version.

`atmos plugin release` owns the build/test/package/publish lifecycle and temporary staging cleanup. Inventory releases check out each package's recorded source in an isolated worktree. `--check` validates the same release without publication or signing. CI base detection uses Atmos's CI provider; an explicit `--base` takes precedence. Custom commands are optional aliases, and no Python release helpers are needed.

The release job holds its global lock throughout publication and catalog reconciliation. `--catalog cloudposse` fetches the current default branch from `origin` before reading the accepted inventory, verifies published releases, and retains historical versions and listing/yank state. A queued old run cannot overwrite the catalog using an old checkout's inventory. Missing releases are deferred; authentication and integrity failures fail the command. Standalone `plugin catalog sync` remains available for separately serialized reconciliation.

## Required public GHCR setup

**Both `atmos/plugins/auth-secrets` and `atmos/plugins/catalog` must be public GHCR packages before their first release.** GitHub creates new packages private by default, even in a public repository. The release workflow checks package visibility before uploading release bytes and checks anonymous access to the full OCI graph afterward. A private or inaccessible package fails closed.

An organization package administrator must provision the package namespaces, set **Package settings → Change visibility → Public**, and grant this repository Actions write access. If GHCR requires an initial object to create the namespace, use a clearly separate bootstrap tag containing no plugin release, then set public visibility before running this workflow. Do not publish a version tag or catalog `stable` while private. The workflow does not attempt an undocumented REST visibility mutation.

Configure the `plugin-release` environment to allow trusted `main` releases and any required organization approval policy. Ensure its `GITHUB_TOKEN` can read package metadata and write the packages, and that keyless signing has `id-token: write`. Restrict catalog writes to this workflow. Manual dispatch of the **Tests** workflow on `main` reruns the full validation and publication pipeline; it is not a bypass.

Public visibility is necessary for distribution, but does not grant trust. Consumers may additionally require signatures and an explicit trusted publisher policy.
