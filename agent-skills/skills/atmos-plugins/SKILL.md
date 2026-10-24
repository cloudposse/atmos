---
name: atmos-plugins
description: "Native Atmos plugins (experimental): declaring plugins.packages, install/update/remove/list/info and the release lock, registries and discovery, authoring and publishing a plugin (create/build/test/pack/publish/sign), running a local build through a profile that sets plugins.packages.<alias>.path, on: plugin policy, and troubleshooting"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: dev-tooling
---

# Atmos Plugins

Use this skill when a project consumes, publishes, develops, or governs a native Atmos plugin: a compiled Go program
that provides an `auth` provider, an `auth` identity, or a command. The feature is experimental; commands print an
experimental notice. Only plugin authors need Go. Consumers install a compiled release from an OCI registry.

## Consume a plugin

Declare the package in `atmos.yaml`, or let `atmos plugin install` write the declaration:

```yaml
plugins:
  packages:
    corporate:
      source: oci://registry.example.com/team/oidc
      version: "^0.1.0"
      timeout: 5m          # Optional: bounds every call into the plugin.
auth:
  providers:
    corporate:
      kind: acme/oidc      # A kind exported by the plugin.
```

```shell
atmos plugin install oci://registry.example.com/team/oidc --as=corporate
atmos plugin install                  # Restore every declaration from the lock.
atmos plugin install --frozen         # Restore exactly the lock; never writes the lock, declaration, or cache.
atmos plugin update [alias]           # Re-resolve within the constraint; rewrites only the lock.
atmos plugin update corporate --version='^0.2'
atmos plugin list                     # No network. States: installed, missing, corrupt, path, broken, disabled.
atmos plugin info corporate
atmos plugin remove corporate         # Declaration and lock entry; refuses while auth references the plugin.
atmos plugin run corporate --         # Run the plugin's command export.
```

Rules to remember:

- **Lock file.** `.atmos.plugins.lock.yaml` records version, OCI index and platform digests, exports, and verification
  status. Commit it with `atmos.yaml`. Normal execution restores the locked digest and never upgrades.
- **Caret pin default.** `install` without `--version` writes `^<installed>` (`^0.1.0` for 0.x allows only that minor;
  a pre-release is pinned exactly). `--version` is stored verbatim. Re-running `install` keeps the existing constraint.
- **Automatic installation** happens only when a command needs a plugin export, such as an `auth` provider or identity
  kind Atmos does not provide. Commands like `describe` or `list` install nothing. In CI, Atmos refuses to create a new
  lock entry (run `atmos plugin install` locally and commit the lock); restoring an already locked release is allowed.
  A declaration that differs from the lock fails and asks for an explicit `install` or `update`.
- **Corrupt cache.** Before a cached release runs, its files are re-verified. A damaged directory is moved to
  `<release>.corrupt-<timestamp>` and the locked digest is downloaded again, except under `--frozen`, in CI, or with
  `plugins.policy.offline`, where Atmos fails with a hint and changes nothing. `plugin list` shows `corrupt`.
- **Disabled and path packages.** `enabled: false` skips installation and activation. A package with `path` is skipped
  by `install` (see local development below).
- **Downloads** run in parallel (`plugins.max_concurrency`, default 4; `1` is sequential) with toolchain-style progress
  on stderr. A failed or canceled batch leaves the declaration and lock unchanged.
- Native plugins run with the invoking user's privileges. Install only packages you trust.

## Registries, discovery, and credentials

```yaml
auth:
  identities:
    registry-reader:
      kind: oci/user
      spec: {registry: ghcr.io, username: company-bot, password_env: GHCR_TOKEN}
plugins:
  discovery: {cache_ttl: 15m}
  registries:
    team:
      host: ghcr.io
      identity: registry-reader
      repositories: [oci://ghcr.io/company/atmos-plugins/oidc]
    curated:
      catalog: oci://ghcr.io/acme/infrastructure/catalog:stable
```

```shell
atmos plugin search oidc [--registry=team] [--refresh] [--capability=auth.provider]
atmos plugin install team/acme/oidc --as=corporate --version='^1.0'   # <registry>/<package> selector.
atmos plugin registry add|list|remove|login|logout
atmos plugin catalog add|remove|build|publish                          # Optional curated catalogs.
```

Credential order per registry host: an identity binding (exclusive: a failure stops the operation, with no fallback),
then Docker configuration, then GitHub settings for `ghcr.io`, then credentials stored by `plugin registry login`, then
anonymous. Authentication belongs to `atmos auth`; see `atmos-auth`. A plugin cannot download itself using its own
auth provider. Repository discovery lists the repositories you name and never downloads executables.
`localhost` registries use plain HTTP.

## Author and publish

```shell
atmos plugin create ./corporate-oidc --kind=acme/oidc --module=example.com/acme/oidc --defaults
atmos plugin validate ./corporate-oidc   # Static: manifest, schemas, bundle-path rules. Starts nothing.
atmos plugin build ./corporate-oidc      # Needs Go. The executable embeds plugin.yaml, so rebuild after editing it.
atmos plugin test ./corporate-oidc       # Starts the executable against local fixtures; --run, --format=json, --integration.
atmos plugin pack ./corporate-oidc --output=./oidc-release
atmos plugin publish ./oidc-release --to=oci://registry.example.com/team/oidc [--sign --yes]
atmos plugin sign oci://registry.example.com/team/oidc@sha256:<index-digest>
atmos plugin delete registry.example.com/team/oidc@sha256:<digest> --yes
```

- A release contains only `plugin.yaml`, `schemas/**/*.json`, and the declared native executables for each platform
  in `spec.platforms`. Hidden paths, credential or key names, symlinks, and non-native binaries are rejected at pack,
  publish, and extract time. `pack` and `publish` print the per-platform file inventory.
- A version cannot be republished with different bytes. Bump `metadata.version`.
- The scaffold declares only the creating machine's platform; list every shipped OS and architecture before publishing.
- Add `--sdk-path /absolute/path/to/atmos/sdk` to `create` only when working against an unreleased Atmos checkout.
- The generated project defines `atmos build`, `atmos test`, and `atmos push` custom commands.
- Consumers enforce signatures with `plugins.policy.trusted_publishers`; signing does not grant trust by itself.

## Run a local build through a profile

`plugins.packages.<alias>.path` runs a plugin from a directory instead of the release. Set it in a profile the
developer chooses; the profile deep-merges over the base configuration, so `source` and `version` stay declared and
apply again when the profile is inactive. The profile name has no special meaning, so never assume `dev`.

```yaml
# .atmos/profiles/<any-name>/atmos.yaml  (or ~/.config/atmos/profiles/<any-name>/atmos.yaml)
plugins:
  packages:
    corporate:
      path: plugins/corporate-oidc   # Relative to the project directory; absolute is allowed.
```

```shell
atmos --profile=<any-name> plugin run corporate --
ATMOS_PROFILE=<any-name> atmos plugin info corporate
atmos plugin build ./plugins/corporate-oidc   # Rebuild; the next command runs the new executable, no reinstall.
```

A `path` package has no lock entry, cache, signature, or version check, and Atmos reads the manifest and executable
from the directory on every activation. It is activated only when a command needs the plugin, with one warning per
alias: `Plugin <alias> runs from local path ...; lock, signature and version checks are bypassed.` `install` skips it,
`install --frozen` refuses (the hint names the active profiles), `remove` explains that the path comes from
configuration, and `plugin list`/`info` show state `path` (or `broken` when `plugin.yaml` cannot be read). Do not
suggest `plugin install --dev` or `allow_dev_overrides`; both were removed. Tutorial:
`/tutorials/plugin-development` on the Atmos website.

## Policy (`on: plugin`)

Rules load from protected `policy.d/` or `ATMOS_POLICY_DIR` and run before installation, activation, test, publish,
sign, and registry operations. Deny rules use `action: deny`; a false predicate is not applicable, not an approval.

| Input | Meaning |
|---|---|
| `operation` | `install`, `activate`, `test`, `search`, `publish`, `sign`, `registry.read`, `registry.write`, `registry.delete`, `registry.configure`, `registry.login` |
| `phase` | `source` (before registry access) or `package` (after the manifest is inspected) |
| `origin` | `registry`, `cache`, `development` (a `path` package or `plugin test`), `layout` (`--from-layout`) |
| `registry`, `repository`, `source` | Exact authority including port, repository path, normalized `oci://` source; empty for local builds |
| `name`, `version`, `digest`, `runtime`, `capabilities` | Inspected package facts; empty in the source phase |
| `development` | Boolean for local development or testing |
| `path` | Local build directory: project-relative slash path for `origin: development`, absolute when outside the project, empty otherwise |
| `profiles` | Active profile names as typed, in order; empty when none |

```yaml
approved-dev-roots:
  on: plugin
  action: deny
  reason: Local plugin builds must live under an approved directory
  with: {dev_roots: [plugins/]}
  rule: "input.origin == 'development' && !params.dev_roots.exists(r, input.path.startsWith(r))"
```

Other recipes: test-only development is `input.origin == 'development' && input.operation != 'test'`; a registry
allowlist that still permits local builds is `input.origin == 'layout' || (input.origin != 'development' &&
!(input.registry in params.allowed_registries))`. Profile names are chosen by developers, so `input.profiles` is
advisory (audit, naming conventions); gate trust on `origin` and `path`. `plugin build` is not a policy operation.
`plugin test` is (`operation == 'test'`). Full table and more rules: [references/policy.md](references/policy.md).
Policy authoring in general: `atmos-policy`.

## `plugins.policy` settings

| Key | Effect |
|---|---|
| `require_signature` | Admit only releases signed by a trusted publisher. Needs at least one `trusted_publishers` entry in some loaded file; a later file cannot turn it off. Governs released packages only. |
| `trusted_publishers` | List of `publisher` (namespace before `/` in `metadata.name`), `identity` (certificate subject; `*` is the only wildcard), `issuer` (exact). A non-empty list enables enforcement. Each layer's list must be satisfied by the same signature. |
| `offline` | Forbid registry access. Install, restore, and cache repair need `--from-layout`; discovery serves a stale cache. Cannot be turned off by a later file. |

## Troubleshooting

| Symptom | Fix |
|---|---|
| `policy denied execution` naming a plugin rule | Read the rule's `reason`; check `origin`, `path`, `operation`, and `profiles`. `plugin build` is never denied. |
| `plugin declarations or artifacts differ from the release lock` | Declaration and lock disagree, or a profile sets `path` under `--frozen`. Run `plugin install` or `plugin update`, or drop the profile. |
| Unlocked plugin refused in CI | Run `atmos plugin install` locally and commit the lock. |
| `plugin source looks like a filesystem path` | `install` fetches releases only. Declare `path` in a profile. |
| State `corrupt` | Run `atmos plugin install` online (or with `--from-layout`). |
| State `broken` or `no such file or directory` for the executable | The `path` directory lacks `plugin.yaml` or the built binary. Run `atmos plugin build <dir>`. |
| `profile not found` | The error lists available profiles; `atmos profile list` shows locations. |
| `plugin version already exists with different bytes` | Bump `metadata.version` and republish. |
| Pulls fail after `registry login` | A stale Docker config entry for the host wins. Run `docker logout <host>` or bind an identity. |
| Interrupted install | The next lifecycle command finishes `.atmos/plugins/transaction.json`; ordinary commands report what resolves it. |

## Routing

| Need | Load |
|---|---|
| Profile discovery, activation, merge behavior | `atmos-profiles` |
| Policy engine, authoring and testing rules | `atmos-policy` |
| Auth providers, identities, registry identities (`oci/user`), keyring | `atmos-auth` |
| Cosign for signing, tool installs | `atmos-toolchain` |

## Official packages

The default `cloudposse` registry points to `oci://ghcr.io/cloudposse/atmos/plugins/catalog:stable`. Set `plugins.discovery.default_registry: false` to disable the implicit alias; an explicit registry override replaces the whole entry. `registry remove cloudposse` persists this opt-out. Direct OCI installation does not require discovery.

`oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets` independently versions the `secrets/store` provider and `secrets/env` identity together. Provider `spec.credentials.<logical-name>` declares `store` and `key` (optionally both `stack` and `component`). Identity `principal.environment.<ENV_NAME>` selects a logical field via the provider. Stores must be secret-enabled; pinned store identities cannot create authentication cycles. Unpinned stores use ambient credentials. Values go only into the execution environment, are masked, and must never be echoed for debugging. Five-minute credentials permit process-local reuse; fresh authentication rereads stores.

Official package manifests and shared build inputs live under `plugins/`; the protected release workflow publishes complete six-platform indexes only after native tests, public GHCR visibility checks, and anonymous integrity verification. Plugin versions are independent of Atmos releases. Never claim a package has been published just because its workflow or source exists.
