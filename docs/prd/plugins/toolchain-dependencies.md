# Plugin toolchain dependencies and lockfiles

**Status:** Plugin release locking is implemented; tool dependency references and coordinated two-lock transactions below remain release requirements, not completed pilot behavior. [All plugin PRDs](README.md).

## Implemented plugin lock semantics

The project declares an exact version or semver constraint in `plugins.packages`. `.atmos.plugins.lock.yaml` records the selected exact release, source/constraint, index/platform digests, runtime, exports, and informational signature results. Ordinary install and automatic restoration reuse that selection; missing locks are created only outside frozen mode. `--frozen` requires exact declaration/lock agreement, refuses while the active configuration sets `path` for a package, and writes no lock. Restore a previous declaration/lock pair to roll back.

`plugin update` resolves within the current constraint. An exact pin stays exact until explicitly changed, for example with `plugin update <alias> --version <constraint>`. This follows toolchain's reproducibility model but does not duplicate its lock schema or imply identical exact-pin update behavior. Current admission and signature policy apply even to a previously installed, locked package.

Payloads use shared XDG data storage; independent downloads use the existing toolchain batch UI and a configurable concurrency limit. This reuse does not mean tool dependency references or two-lock commits are already implemented. The sections below specify that remaining integration.

## Declaring tools

Plugins may require toolchain-managed programs. Reuse the existing [dependencies.tools vocabulary](../../../pkg/schema/dependencies.go) under the manifest's `spec`. Publisher install scripts and a separate tool installer are unnecessary.

```yaml
apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/infrastructure
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/acme-infrastructure
  dependencies:
    tools:
      hashicorp/terraform: "1.10.0"
      kubernetes/kubectl: "1.32.0"
```

This fragment illustrates exact requirements; supported constraints follow the toolchain resolver. Real packages also declare extensions and platform artifacts. Extension descriptors may add their own `dependencies.tools`; effective requirements combine with package-level dependencies. Canonical tool identities prevent ambiguous short names.

```shell
atmos plugin install acme/infrastructure --as infrastructure --version '~1.0'
atmos plugin install --frozen
atmos plugin update infrastructure
```

Explicit installation restores required tools through the existing toolchain manager as well as the plugin payload. `plugin info` shows requirements without installation or execution. Ordinary plugin execution checks that locked tools are present; missing tools produce restore instructions, never an implicit upgrade/download.

## Ownership and locking

The existing [toolchain lockfile](../../../pkg/toolchain/lockfile/lockfile.go) supports multiple versions and platform checksums. Reuse its resolver, verification, cache, and frozen-policy mechanisms. Its existing schema remains valid; plugin manifests do not force a toolchain lockfile format migration.

- `.atmos.plugins.lock.yaml` records plugin source/constraint, release/index digest, platform manifests, contracts, and exact tool dependency references.
- The configured toolchain lockfile owns tool source, version, platform checksums, and verification data. Plugin references select exact canonical identity/version/source entries.
- Frozen restoration requires consistent entries in both files and edits neither. Project pins satisfy requirements only when compatible; conflicts name both requirements and their dependency path.
- Different plugins may use different tool versions through scoped executable paths. Incompatible requirements for one executable in a single invocation fail before execution.
- Install/update prepares artifacts, resolution, and both lock changes before committing consistent records. Interrupted commits must be recoverable and never reported as a successful mixed selection.
- User-scoped dependency records live outside the project. Removing a plugin removes its ownership references, not shared artifacts or unrelated tool declarations.

One package owns all its extension versions. Updating a command cannot leave that package's identity or skill on a different release. Rollback restores declarations and both lockfiles, then performs frozen restoration. Tool versions and plugin versions remain independent.

## Invocation and verification

Atmos process services supply verified executable paths and a scoped PATH. An unrelated ambient executable cannot satisfy a locked dependency. Existing built-in tool auto-install behavior remains unchanged. Automatic restoration of a declared plugin package does not resolve the plugin's separate tool dependencies; those dependencies retain this document's explicit provisioning contract.

Metadata inspection needs no installed tool or running plugin. Offline restoration requires plugin and tool payloads. Installation credentials must exist before dependency retrieval; authentication cycles through the plugin being installed fail with their dependency path.

Tests cover exact/constrained requirements, platform checksums, isolated versions, conflicting project overrides, missing offline tools, interrupted updates, rollback, and cancellation. A fixture places a different binary on ambient PATH and verifies that the locked tool is invoked.
