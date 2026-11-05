# Plugin platform configuration, manifests, and on-disk layout

**Status:** Native pilot implemented in the working branch; this document also retains target configuration for later work. The implementation boundary below takes precedence over the broader examples. [Platform workstream](README.md).

This document consolidates the planned configuration surfaces. The [first-release scope](../first-release.md) enables auth and executable commands; fields for other adapters/runtimes are reserved follow-ons, not promises of installed support. Unsupported required exports fail compatibility checks before installation is committed. The product PRDs ([user experience](../user-experience.md), [authoring](../authoring.md), [package composition](../package-composition.md), [toolchain dependencies](../toolchain-dependencies.md), [distribution](../distribution.md), [testing](../testing.md), [extensions](../extensions/README.md)) show fragments in context; this is the complete form.

| Surface | Location | Scope | Committed |
|---|---|---|---|
| `plugins:` section | root `atmos.yaml` | project | yes |
| User plugin config | XDG config `atmos/plugins.yaml` | user | no |
| Machine policy | `/etc/atmos/plugins.yaml`, `%ProgramData%\atmos\plugins.yaml`, or `ATMOS_PLUGINS_POLICY_FILE` | machine | no |
| `AtmosPluginLock` | `.atmos.plugins.lock.yaml` beside root `atmos.yaml`; user scope: XDG config `atmos/plugins.lock.yaml` | project, user | project only |
| `AtmosPluginConfig` | `plugin.yaml` at the package root, copied into the release | package | in the plugin repository |
| `AtmosPluginCatalog` | OCI artifact in a registry; cached under XDG cache | registry | no |
| `AtmosPluginTestConfig` | `tests/contract.yaml` in the plugin repository | package | in the plugin repository |

All four manifest kinds use the existing [manifest envelope](../../../../pkg/manifest/manifest.go) (`apiVersion: atmos/v1`, `kind`, `metadata`, `spec`). The machine policy file is plain YAML state, not enveloped kinds.

## Native pilot configuration boundary

Implemented project configuration lives in `schema.AtmosConfiguration.Plugins`, with declarations, named registry hosts, optional catalog/repository discovery inputs, `discovery.cache_ttl`, `max_concurrency`, `cache_dir`, and signature/offline/development policy. The generated configuration schema includes these fields. Auth and command manifests, release locks, catalogs, and test configuration use the shared manifest loader. Local dev state is separate from release locks.

| Surface | Current implementation |
|---|---|
| Download concurrency | `plugins.max_concurrency`: default four (zero selects default), positive explicit limit; shared `pkg/ui/batch` presentation. |
| Registry auth | `identity` references `auth.identities`; shared OCI resolution authenticates the exact registry host through Atmos auth. Built-in `oci/user` supports username/token credentials, while cloud identities use matching ECR/ACR integrations. Without an identity, omitted `auth` or `auth: docker` retains the shared credential chain. Other named helper selectors below remain rejected. |
| Package scope | Project only; user scope and `--global` remain pending. |
| Signatures | `require_signature` and `trusted_publishers` implemented; requiring signatures also requires trusted publisher constraints. All loaded Atmos config layers retain mandatory constraints. |
| General admission | Protected `Policy` manifests using `on: plugin` in `policy.d/` or `ATMOS_POLICY_DIR`; see [Plugin policies](../policy.md). No new nested CEL/Starlark settings under `plugins.policy`. |
| Reserved policy | Legacy `policy.cache_ttl` remains rejected; use `plugins.discovery.cache_ttl`. A dedicated machine plugin policy file and the additional plugin environment mappings below remain proposed. |
| Locks | Exact source/constraint/version, index and platform digests, runtime, exports, and informational signature identity/issuer. Tool references and contract copies shown in the broader lock example remain proposed. |
| Shared payloads | XDG data `atmos/plugins/releases/<index-digest-hex>/<os>/<arch>/`, containing `oci/` and `payload/`; `cache_dir` overrides the root. Shared across projects. |
| Discovery cache | Catalog snapshots and explicit repository indexes share XDG cache `atmos/plugins/discovery`; default freshness is `15m`, explicit refresh and cached offline search are supported. |

No project policy may weaken mandatory requirements accumulated from loaded configuration files. This does not claim that the separate machine-policy file or every environment variable proposed below is already loaded. For organizational CEL/Starlark enforcement, protect `ATMOS_POLICY_DIR`, the runtime, environment, and lockfile independently of candidate changes.

## atmos.yaml: plugins section

The top-level `plugins:` section is distinct from Helm/Helmfile component plugin settings. The following is the broader target form; use the boundary table above when configuring the native pilot:

```yaml
plugins:
  max_concurrency: 4                      # Bounded parallel downloads, shared toolchain UI.
  cache_dir: ~/.cache/atmos-plugins        # Optional. Replaces the XDG data and cache roots for plugins.
  discovery:
    cache_ttl: 15m
  registries:
    acme:
      catalog: oci://ghcr.io/acme/atmos-plugins/catalog:stable
      identity: registry-reader             # Existing auth.identities entry. Never a secret.
    team:
      host: registry.example.com
      repositories:
        - oci://registry.example.com/team/oidc
        - oci://registry.example.com/team/credentials
  packages:
    corporate:                             # Project alias; chosen at install time with --as.
      source: oci://ghcr.io/acme/atmos-plugins/auth
      version: '~1.2'
      scope: project
      enabled: true
      config:                              # Plugin-owned; validated by spec.config_schema.
        default_audience: atmos
    okta:
      source: oci://ghcr.io/acme/atmos-plugins/okta
      version: '~1.0'
      command: okta                        # Selects one command export of a multi-command package.
  policy:
    require_signature: true
    trusted_publishers:
      - publisher: acme
        identity: https://github.com/acme/*
        issuer: https://token.actions.githubusercontent.com
    offline: false
```

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `plugins.max_concurrency` | integer | no | `4` | Bounds independent package downloads and discovery source requests; `0` selects the default and negative values fail validation. |
| `plugins.cache_dir` | path | no | XDG data `atmos/plugins` (payloads) and XDG cache `atmos/plugins/discovery` (discovery) | Absolute or `~`-prefixed; relative paths are rejected. An override stores release payloads beneath that root and discovery metadata under `<cache_dir>/discovery`. |
| `plugins.discovery.cache_ttl` | duration | no | `15m` | Nonnegative Go duration; `0` revalidates every online search. `search --refresh` bypasses freshness. Payloads are immutable and have no TTL. |
| `plugins.registries.<name>.host` | string | no | inferred from inputs | Exact registry authority, with optional port. All configured discovery inputs must match. A host-only entry supports auth without discovery. |
| `plugins.registries.<name>.catalog` | OCI ref | no | none | Optional published discovery snapshot. Independent of package installation and publication. |
| `plugins.registries.<name>.repositories` | list of OCI refs | no | none | Explicit repositories without tags or digests, indexed locally from their release metadata. No registry-wide enumeration. May coexist with a catalog. |
| `plugins.registries.<name>.identity` | string | no | unset | Existing Atmos auth identity for this exact registry host, whether explicit or inferred from discovery inputs. The same binding covers release reads/writes and signatures. Conflicting identities for one host fail closed. `auth login/logout` owns the session; registry login/logout are conveniences. |
| `plugins.registries.<name>.auth` | string | no | `docker` | Names a credential helper: `docker` (Docker config and `docker-credential-*` chain), `ghcr` (existing [`GHCRAuth`](../../../../pkg/oci/auth.go)), or a helper whose executable is `docker-credential-<name>`. Credentials are never stored in YAML, catalogs, or lockfiles. `registry login` stores them outside project configuration. |
| `plugins.packages.<alias>.source` | OCI ref | yes | none | Repository without tag or digest. `install` records the resolved reference so later catalog edits cannot redirect it. |
| `plugins.packages.<alias>.version` | constraint | yes | none | Semver constraint (`~1.2`, `>=1.2 <2`, `1.2.0`). The lockfile records the selected exact release. |
| `plugins.packages.<alias>.path` | path | no | none | Runs a plugin built in a local directory instead of the release named by `source` and `version`. Relative to the project directory (the directory of the root `atmos.yaml`); absolute paths are allowed. When set it wins: `source` and `version` stay in the merged configuration but are unused, and a package with `path` needs neither. Declare it in a profile so it applies only while that profile is active. See [Path packages](#path-packages). |
| `plugins.packages.<alias>.scope` | `project`, `user` | no | scope of the declaring file | Must equal the declaring file's scope; a mismatch is a validation error. Present so writers and `list` output are explicit. |
| `plugins.packages.<alias>.enabled` | bool | no | `true` | `false` keeps the declaration and lock entry but registers none of the package's exports. |
| `plugins.packages.<alias>.config` | map | no | `{}` | Opaque to Atmos. Validated against the package's `spec.config_schema` at `install`, `validate`, and before first use. Never holds secrets; use `inputs[]`, which the `api.Secrets` service resolves. |
| `plugins.packages.<alias>.command` | string | no | none | Export name for packages with several command trees. A `--command <export-name>` flag on `plugin run` is proposed; today the declaration is the only selector. |
| `plugins.policy.require_signature` | bool | no | `false` | Unsigned or unverifiable releases are rejected. |
| `plugins.policy.trusted_publishers[]` | list of `{publisher, identity, issuer}` | no | none | `publisher` matches the `metadata.name` prefix; `identity` and `issuer` bind it to a signer. Identity permits anchored `*` wildcards; issuer is exact. A nonempty list requires signatures. |
| `plugins.policy.offline` | bool | no | `false` | Forbids registry network access; restores only from supplied OCI layouts or verified cache. |

### Precedence

This is the target model. Today only project `atmos.yaml` keys, `--frozen` on `install`, and `--registry` on `search` exist; the `--offline` and `--global` flags, the `ATMOS_PLUGINS_*` environment mappings, the user layer, and the machine policy file are proposed.

| Order | Layer | Applies to |
|---|---|---|
| 1 | CLI flags (`--frozen`, `--offline`, `--config`, `--global`, `--registry`) | all keys |
| 2 | Environment, prefix `ATMOS_PLUGINS_` | all keys |
| 3 | Project `atmos.yaml` `plugins:` (including imports) | all keys except machine-only paths |
| 4 | User `atmos/plugins.yaml` | all keys |
| 5 | Defaults | all keys |
| policy | Machine policy file | `policy.*` only |

Resolution rules:

- `registries` merge by name; the higher layer replaces the whole entry. `packages` merge by alias with whole-entry replacement, never field-level merging, so a user pin cannot leak into a project declaration. Profiles are different: they are part of the project configuration and deep-merge field by field before this model applies, which is how `path` layers beside `source` and `version`.
- Security keys are **strictest-wins across every layer**, including the machine policy file and CLI flags. No layer can weaken another. `require_signature`: true if any layer is true. `offline`: true if any layer is true. `trusted_publishers`: when more than one layer defines a list, the same signature must satisfy each list (OR within a list, AND across lists), so project configuration can only narrow what user or machine policy trusts.
- Project configuration therefore cannot weaken a machine or CI policy: a project that sets `require_signature: false` under a machine policy of `true` has no effect, and `plugin info` reports the winning layer.
- The machine policy file honors only `policy` keys. CI images set it with `ATMOS_PLUGINS_POLICY_FILE`.
- Flags bind through `pkg/flags`; code never calls `viper.BindEnv` directly.

Environment variables map one-to-one onto keys: `ATMOS_PLUGINS_CACHE_DIR`, `_CACHE_TTL`, `_OFFLINE`, `_REQUIRE_SIGNATURE`, and `_TRUSTED_PUBLISHERS` (comma-separated publisher names, intersected with file-defined identities). `ATMOS_PLUGINS_POLICY_FILE` names the machine policy file, and `ATMOS_PLUGINS_FROZEN` equals `--frozen`.

## User-scope configuration (pending)

XDG config `atmos/plugins.yaml` (resolved with [`GetXDGConfigDir`](../../../../pkg/xdg/xdg.go), honoring `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME`). The keys are the `plugins:` keys without the wrapper; scope is implied.

```yaml
registries:
  acme:
    catalog: oci://ghcr.io/acme/atmos-plugins/catalog:stable
    auth: ghcr
packages:
  stacks:
    source: oci://ghcr.io/acme/atmos-plugins/stacks
    version: '~1.0'
policy:
  require_signature: true
  trusted_publishers: [{publisher: acme, identity: "https://github.com/acme/*", issuer: "https://token.actions.githubusercontent.com"}]
```

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `registries`, `packages`, `policy`, `cache_dir` | as above | no | empty | Same types and validation as the project section. `packages.<alias>.scope` is always `user`. |

- Once user scope is implemented, `plugin install --global` and `plugin registry add --global` will write this file (neither flag exists today); without `--global`, they edit the root `atmos.yaml` (or `--config <file>`). Writers preserve unrelated fields and comments.
- Its exact selections are recorded in the user-scope `AtmosPluginLock` at XDG config `atmos/plugins.lock.yaml`, never in a project lockfile.
- A project declaration of an alias wins over a user declaration of the same alias, wholly. A missing or incompatible project plugin never falls back to the user copy.
- User-scope packages activate **only their `commands` exports**, so `atmos stacks list` works in any directory. Every other export (auth, components, CI, validation, hooks, steps, stores, secrets, backends, MCP tools, skills) stays visible in `list` and `info` but inactive unless the project declares the package. Unrelated user integrations never join a project's registries or MCP server.
- User-scope `registries` supply discovery only; a project may use them for `search` but its `packages` always carry an explicit `source`.

## Lockfile: .atmos.plugins.lock.yaml

Located beside the root `atmos.yaml`; committed. The user-scope lock has the same shape.

```yaml
apiVersion: atmos/v1
kind: AtmosPluginLock
metadata:
  name: plugins
spec:
  lock_version: 1
  generated_at: "2026-10-19T14:02:11Z"
  atmos_version: 1.230.0
  toolchain_lock: toolchain.lock.yaml
  packages:
    corporate:
      plugin: acme/auth
      source: oci://ghcr.io/acme/atmos-plugins/auth
      constraint: '~1.2'
      version: 1.2.0
      index_digest: sha256:4d1f0c7e9a31b8c25e6f0a9d73b4c1e8f2a6d5b09c3e7f1a8b2d4c6e0f9a1b3d
      platforms:
        linux/amd64: sha256:11aa22bb33cc44dd55ee66ff77aa88bb99cc00dd11ee22ff33aa44bb55cc66dd
        darwin/arm64: sha256:33cc44dd55ee66ff77aa88bb99cc00dd11ee22ff33aa44bb55cc66dd77ee88ff
      runtime: {type: native, protocol: 1}
      contracts: {auth.provider: 1, atmos.http: 1, atmos.secrets: 1}
      exports:
        - {capability: auth.provider, name: acme/broker}
        - {capability: auth.identity, name: acme/account}
      tools:
        - tool: hashicorp/terraform
          version: 1.10.0
          source: aqua:hashicorp/terraform
          lock_ref: toolchain.lock.yaml#hashicorp/terraform@1.10.0
      verification:
        signature: verified
        identity: "https://github.com/acme/oidc/.github/workflows/release.yaml@refs/tags/v1.2.0"
        issuer: "https://token.actions.githubusercontent.com"
```

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `spec.lock_version` | int | yes | `1` | Lock format version; independent of `apiVersion`. |
| `spec.generated_at`, `spec.atmos_version` | RFC 3339, string | yes | set on write | Informational; mirror the [toolchain lockfile](../../../../pkg/toolchain/lockfile/lockfile.go) metadata. Not compared in frozen mode. |
| `spec.toolchain_lock` | path | when any package has `tools` | configured `toolchain.lock_file` | Toolchain lockfile that `lock_ref` entries point into. |
| `spec.packages.<alias>` | map | yes | `{}` | One entry per enabled declared package; the alias matches `plugins.packages`. |
| `…plugin` | `publisher/name` | yes | none | Release identity from `metadata.name`. |
| `…source` | OCI ref | yes | none | Must equal the declared `source`. |
| `…constraint` | string | yes | none | Declared `version` at lock time. Must equal the declaration in frozen mode. |
| `…version` | semver | yes | none | Selected release; satisfies `constraint`. |
| `…index_digest` | digest | yes | none | OCI index digest of the release. Moved tags resolve to a different digest and are rejected. |
| `…platforms` | map `os/arch[/variant]` → digest | native only | none | Child manifest digest per platform advertised by the verified index. Lock creation fetches all; install downloads only the running platform. |
| `…runtime` | `{type, protocol\|api}` | yes | none | Copied from the manifest. `native` records `protocol`; `starlark` records `api`; `data` records nothing else. |
| `…contracts` | map service → int | no | `{}` | Resolved contract versions (capability and API service) the release was locked against. |
| `…exports[]` | `{capability, name}` | yes | none | Extension exports validated at install; runtime registration must match. |
| `…tools[]` | `{tool, version, source, lock_ref}` | no | `[]` | Canonical tool identity and exact version. `lock_ref` is `<file>#<owner/repo>@<version>` into the toolchain lockfile, which owns checksums and verification. |
| `…verification.signature` | `verified`, `unverified` | yes | `unverified` | Informational result of installation. Failed verification never replaces the previous lock. Current policy is always checked again before execution. |
| `…verification.identity`, `…verification.issuer` | string | when `verified` | none | Authenticated certificate identity and issuer. Neither field authorizes execution without verification of the cached evidence. |

Rules:

- `install --frozen` requires exact agreement: every enabled declaration has one lock entry with identical `source` and `constraint`; no lock entry lacks a declaration; every `lock_ref` resolves in the toolchain lockfile; the index and platform digests match the registry or offline layout. Any difference is `ErrPluginFrozenDrift`. Frozen mode edits neither lockfile.
- Restoration uses the locked digest rather than re-resolving a mutable version tag. Frozen mode does not contact a tag to select a replacement. Publishing different bytes under an existing version is rejected; changing any platform payload requires a new release version.
- Ordinary install creates missing locks but reuses existing selections. `update` explicitly resolves within the declaration; an exact pin remains pinned unless the user changes it, for example with `update <alias> --version <constraint>`. Plugin locks follow toolchain reproducibility conventions but are a separate format with their own update semantics.
- Recheck current CEL/Starlark admission and mandatory signature policy on reuse. Lockfile identity/issuer fields are informational, not authorization evidence.
- Dev overrides are never written to the lockfile and are rejected in frozen mode.
- Install and update prepare artifacts, tool resolution, and both lockfile changes before committing them together. An interrupted commit is recoverable and never reported as success.
- Rollback is restoring the previous `atmos.yaml`, `.atmos.plugins.lock.yaml`, and toolchain lockfile from version control, then `atmos plugin install --frozen`. It requires the locked artifacts to remain in a registry, the cache, or an offline layout.
- Lock entries never contain credentials, tokens, or registry auth.

## Plugin manifest: AtmosPluginConfig

`plugin.yaml` is read without starting the plugin. This example exports every capability type in one package. A real package declares only what it ships; a package has exactly one `runtime.type`, and data exports (rules, skills) need no process under any runtime.

```yaml
apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/suite
  version: 1.4.0
  description: Acme platform integration
  homepage: https://github.com/acme/atmos-suite
  license: Apache-2.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/atmos-suite
  platforms:
    linux/amd64: {executable: bin/linux-amd64/atmos-suite, sha256: "<hex>"}
    darwin/arm64: {executable: bin/darwin-arm64/atmos-suite, sha256: "<hex>"}
    windows/amd64: {executable: bin/windows-amd64/atmos-suite.exe, sha256: "<hex>"}
  compatibility:
    atmos: ">=1.200.0 <2.0.0"
    protocol: ">=1 <2"
    contracts: {auth.provider: ">=1", atmos.http: ">=1", atmos.secrets: ">=1"}
  config_schema: schemas/config.json
  dependencies:
    tools:
      hashicorp/terraform: "1.10.0"
  capabilities:
    commands:
      suite:
        description: Manage Acme resources
        usage: atmos suite <command> [flags]
        executable: bin/atmos-suite
        args: [command]
        shortcut: true
        aliases: [acme]
        subcommands: {apps: {description: List applications}}
    auth:
      credential_types:
        acme/session/v1: {schema: schemas/session.json, sensitive_keys: [token]}
      providers:
        acme/broker:
          schema: schemas/broker.json
          credentials: [aws/v1, acme/session/v1]
          features: [refresh, logout, prepare_environment, paths]
          inputs: [{field: token_env, type: environment, secret: true}]
      identities:
        acme/account: {schema: schemas/identity.json, accepts: [acme/session/v1], credentials: [aws/v1]}
    components:
      pulumi:
        group: Infrastructure
        schema: schemas/pulumi-component.json
        commands:
          - {name: preview, description: Preview changes, exit_codes: {ok: [0], changes_detected: [2]}}
          - {name: up, description: Apply changes}
        artifacts:
          - {type: preview, role: plan, format: pulumi-preview-json/v1}
        events: [before.pulumi.up, after.pulumi.up]
    ci:
      providers:
        acme/runner:
          detect: {env: {all: [{name: ACME_CI, equals: "true"}]}, callback: true}
          features: [checks, comments, outputs, summaries, log_grouping, labels]
      components:
        pulumi: {events: [after.pulumi.preview, after.pulumi.up]}
    hooks:
      acme/deployment-audit: {schema: schemas/audit.json, events: [after.terraform.apply]}
    steps:
      acme/change-request:
        schema: schemas/change.json
        result_schema: schemas/change-result.json
        category: command
        requires_tty: false
        aliases: [change]
    validation:
      rules:
        acme/required-tags: {category: policy, input: component, engine: opa, path: policies/required-tags.rego, options_schema: schemas/required-tags.json}
      validators:
        acme/naming: {category: lint, input: file, options_schema: schemas/naming.json}
        acme/no-public-ingress: {category: policy, input: artifact, artifacts: [terraform-plan-json/v1], options_schema: schemas/ingress.json}
    mcp:
      tools:
        acme.suite.list_apps:
          description: List applications available to the selected identity
          input_schema: schemas/list-apps-input.json
          output_schema: schemas/list-apps-output.json
          annotations: {read_only: true, destructive: false, idempotent: true, open_world: true}
    skills:
      suite-operations: {name: suite-operations, path: skills/suite-operations/SKILL.md}
    stores:
      acme/kv: {schema: schemas/store.json, operations: [raw, delete, exists, list], secret_capable: true, sensitive_mode: encrypted}
    secrets:
      providers:
        acme/secrets: {schema: schemas/secrets.json, scopes: [instance, stack, global], operations: [get, set, delete, status, raw_get]}
    provisioners:
      backends:
        acme/managed-state: {schema: schemas/backend.json, backend_types: [s3], operations: [ensure, exists, describe, delete]}
```

### Envelope, runtime, and compatibility

Atmos's shared [`Metadata`](../../../../pkg/manifest/manifest.go) carries `name`, `description`, `author`, and `version`. The plugin kinds need `homepage` and `license`, and the catalog needs `revision` and `generated_at`; add these as optional `omitempty` fields rather than a second metadata type.

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `metadata.name` | `publisher/name` | yes | none | Package identity. Lowercase letters, digits, `-`, `_`, `.`. `catalogs` and names starting with `.` are reserved. |
| `metadata.version` | semver | yes | none | Release version. Independent of `apiVersion`, `runtime.protocol`, and Atmos. |
| `metadata.description`, `homepage`, `license` | string | no (`description` recommended) | empty | `license` is an SPDX identifier. Shown in `search` and `info`. |
| `spec.runtime.type` | `native`, `starlark`, `data` | yes | none | One runtime per package. |
| `spec.runtime.protocol` | int | native | none | Native protocol version the executable speaks. |
| `spec.runtime.executable` | path | native | none | Default entrypoint, relative to the platform package. |
| `spec.runtime.environment` | list of names | no | `[]` | Environment variable names forwarded from the Atmos process to the plugin process. Values are copied verbatim; nothing else from Atmos environment is passed. Secrets use `inputs[]` and Atmos secret service instead. |
| `spec.runtime.api` | int | starlark | none | Starlark plugin API version. |
| `spec.runtime.entrypoint` | path | starlark | none | `.star` file evaluated for execution or testing, never for discovery or help. |
| `spec.platforms.<os/arch[/variant]>` | map | native | none | Declared support matrix. `executable` overrides `runtime.executable` for that platform; `sha256` is computed by `pack` and required in a packed release. `pack` rejects a release whose platform payloads disagree on identity, version, contracts, or schemas. |
| `spec.compatibility.atmos` | constraint | no | any | Atmos versions the release supports. |
| `spec.compatibility.protocol` | constraint | no | exactly `runtime.protocol` | Atmos protocol versions accepted (native). Starlark checks `runtime.api` against Atmos's supported APIs instead. |
| `spec.compatibility.contracts` | map service → constraint | no | `{}` | Capability and API-service contract versions required, such as `auth.provider` or `atmos.http`. |
| `spec.config_schema` | path | no | none | JSON Schema for `plugins.packages.<alias>.config`. |
| `spec.dependencies.tools` | map tool → version | no | `{}` | Existing [`dependencies.tools`](../../../../pkg/schema/dependencies.go) vocabulary with canonical `owner/repo` identities. Any export may add its own `dependencies.tools`; effective requirements are the union, and conflicts fail before execution. |

### Capabilities

Every export key is the registered kind, type, tool, or check ID, namespaced `publisher/name` (MCP tools use dots, as in the [MCP PRD](../extensions/mcp-tools.md)). Every `schema`, `path`, and `*_schema` value is a package-relative path that cannot escape the verified package. A registry collision on any key is an installation error.

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `capabilities.commands.<name>.description`, `.usage` | string | description | empty | Static help; the command tree is listed without starting the plugin. |
| `capabilities.commands.<name>.executable` | path | one of `executable` or `function` | `runtime.executable` | Executable dispatched by `atmos <name>` and `plugin run`. |
| `capabilities.commands.<name>.function` | string | one of `executable` or `function` | none | Starlark callable for Starlark packages. |
| `capabilities.commands.<name>.args` | list | no | `[]` | Fixed argv prefix; user argv follows unchanged. |
| `capabilities.commands.<name>.shortcut` | bool | no | `true` | `false` exposes the command only through `plugin run`. A shortcut that collides with a built-in or YAML command is unavailable and diagnosed. |
| `capabilities.commands.<name>.aliases`, `.subcommands.<n>.description` | list, map | no | empty | Static help only; collisions apply to aliases too. |
| `capabilities.auth.credential_types.<type>.schema`, `.sensitive_keys` | path, list | schema | `[]` | Declares a plugin-namespaced credential type (`publisher/name/vN`). `sensitive_keys` drive masking and keyring encryption. |
| `capabilities.auth.providers.<kind>.schema` | path | yes | none | Validates `auth.providers.<name>.spec`. |
| `capabilities.auth.providers.<kind>.credentials` | list of types | yes | none | Credential types the provider can produce. |
| `capabilities.auth.providers.<kind>.features` | list | no | `[]` | Optional handlers implemented: `refresh`, `logout`, `prepare_environment`, `paths`. Undeclared handlers are never called. |
| `capabilities.auth.providers.<kind>.inputs[]` | `{field, type, secret}` | no | `[]` | Schema fields that name a Atmos-supplied input (`type: environment` or `prompt`); the value is read through Atmos secret service, never from YAML. |
| `capabilities.auth.identities.<kind>.schema` | path | yes | none | Validates `principal`. |
| `capabilities.auth.identities.<kind>.accepts`, `.credentials` | lists of types | yes | none | Upstream types consumed and downstream types returned; Atmos validates every chain link statically. |
| `capabilities.components.<type>.group` | string | no | `Other` | Help group, as `GetGroup` today. |
| `capabilities.components.<type>.schema` | path | no | none | Validates the component's engine settings. |
| `capabilities.components.<type>.commands[]` | `{name, description, exit_codes}` | yes | none | `exit_codes.ok` and `exit_codes.changes_detected` give command-specific success semantics; other codes fail. |
| `capabilities.components.<type>.artifacts[]` | `{type, role, format}` | no | `[]` | `role` is `source`, `plan`, or `applied`; keeps a plan distinct from the applied artifact. |
| `capabilities.components.<type>.events[]` | list | no | `[]` | Lifecycle events the engine publishes (for hooks and CI). |
| `capabilities.ci.providers.<id>.detect.env` | `{all[], any[]}` of `{name, equals\|present}` | no | none | Static predicates that narrow candidates without launching plugins. |
| `capabilities.ci.providers.<id>.detect.callback` | bool | no | `false` | Permits one bounded, read-only detection callback after predicates match. |
| `capabilities.ci.providers.<id>.features` | list | no | `[]` | `checks`, `comments`, `outputs`, `summaries`, `base_resolution`, `log_grouping`, `labels`, `commit_comments`, `debug_context`. Undeclared features return unsupported. |
| `capabilities.ci.components.<type>.events` | list | yes | none | Component CI reporting events handled for an engine, built-in or plugin-provided. |
| `capabilities.hooks.<kind>.schema`, `.events` | path, list | schema | any event | `events` restricts the events a configuration may select. Installation never subscribes. |
| `capabilities.steps.<type>.schema`, `.result_schema` | path | schema | none | Supply the `knownFields` equivalent for static validation; `result_schema` validates values before later steps read them. |
| `capabilities.steps.<type>.category`, `.requires_tty`, `.aliases` | string, bool, list | no | `command`, `false`, `[]` | Mirror `StepHandler`; aliases are checked for collisions. |
| `capabilities.validation.rules.<id>` | `{category, input, engine, path, options_schema}` | category, input, engine, path | none | Rule pack for an existing engine (`opa`, `jsonschema`, `tflint`). `category`: `policy`, `lint`, `validation`. `input`: `file`, `component`, `artifact`. |
| `capabilities.validation.validators.<id>` | `{category, input, options_schema, artifacts}` | category, input | none | Go or Starlark validator. `artifacts` lists accepted formats (for example `terraform-plan-json/v1`) and is required when `input: artifact`. |
| `capabilities.mcp.tools.<name>.description` | string | yes | none | Served from metadata without starting the plugin. |
| `capabilities.mcp.tools.<name>.input_schema`, `.output_schema` | path | input | none | Raw JSON Schema passed through to MCP unchanged. |
| `capabilities.mcp.tools.<name>.annotations` | `{read_only, destructive, idempotent, open_world}` | no | MCP defaults | Descriptive only; Atmos permission policy authorizes calls. |
| `capabilities.skills.<key>.path` | path | yes | none | `SKILL.md` inside the package. |
| `capabilities.skills.<key>.name` | string | no | `<key>` | Must equal the `name` in the `SKILL.md` frontmatter ([`SkillMetadata`](../../../../pkg/ai/skills/marketplace/metadata.go)); a mismatch fails validation. Description and tool restrictions stay in the frontmatter. |
| `capabilities.stores.<kind>.schema` | path | yes | none | Validates `stores.<name>.options`. |
| `capabilities.stores.<kind>.operations` | list | no | `[]` | Optional operations: `raw`, `delete`, `exists`, `list`. Get and set are always required. |
| `capabilities.stores.<kind>.secret_capable`, `.sensitive_mode` | bool, `none`, `encrypted`, `delegated` | no | `false`, `none` | Replace the hardcoded store capability lists. Metadata is not proof of encryption; Atmos policy decides sensitive use. |
| `capabilities.secrets.providers.<kind>.schema` | path | yes | none | Validates `secrets.providers.<name>.options`. |
| `capabilities.secrets.providers.<kind>.scopes`, `.operations` | lists | scopes | `[instance]`, `[get, set]` | Scopes: `instance`, `stack`, `global`. Operations: `get`, `set`, `delete`, `status`, `raw_get`. Unsupported scopes or operations fail before any write. |
| `capabilities.provisioners.backends.<kind>.schema` | path | yes | none | Validates `provision.backend.options`. |
| `capabilities.provisioners.backends.<kind>.backend_types`, `.operations` | lists | backend_types | none, `[ensure, exists]` | Backend protocols the provisioner may serve. `ensure` and `exists` are mandatory; `describe`, `update`, `delete`, `list` are explicit. |

### Credential types

Credentials cross the plugin boundary in one versioned envelope, registered per type in the credential-type registry:

```yaml
type: aws/v1
expires_at: "2026-10-19T22:00:00Z"
data: {access_key_id: AKIAEXAMPLE, secret_access_key: "<masked>", session_token: "<masked>", region: us-east-1}
sensitive_keys: [secret_access_key, session_token]
```

Core types are `aws/v1`, `gcp/v1`, `azure/v1`, `oidc/v1`, and `atmos-pro/v1`; each wraps the existing `ICredentials` implementation through an adapter. Plugin types are `publisher/name/vN` and must be declared under `capabilities.auth.credential_types`. Plugin-specific session data stays namespaced to its package and is never fed to another package. The envelope wire encoding is in [protocol](protocol.md); the `oidc/v1` data schema and private refresh-session rules are in the [OIDC pilot](../oidc-pilot.md#credential-and-session-contracts). Each other core codec must define its data schema in P0.2 before advertising that type.

### Static versus runtime

- **Static (no process, no network):** `metadata`, `compatibility`, `platforms`, `dependencies.tools`; command help, `shortcut`, `aliases`; skills; MCP tool metadata and schemas; `validation.rules`; every `schema`/`*_schema` (config, workflow, and hook validation); `auth` accepted and produced types (chain validation before login); `ci.providers.*.detect.env` (CI resolution at startup).
- **Runtime (process or Starlark session):** `runtime.executable`/`entrypoint`; registered handlers, which must match the declared exports exactly; `detect.callback`; auth `features` handlers; validator, tool, store, secret, and backend operations; credential values, prompts, and API-service calls.

Starlark source is evaluated only for execution or explicit testing, never for discovery, help, or listing.

## Catalog: AtmosPluginCatalog

An optional OCI snapshot in the configured registry, referenced by `plugins.registries.<name>.catalog`. It holds discovery metadata only; inclusion does not imply publisher verification, and install validates the referenced release independently.

```yaml
apiVersion: atmos/v1
kind: AtmosPluginCatalog
metadata:
  name: acme
  revision: 14
  generated_at: "2026-10-19T13:55:00Z"
spec:
  entries:
    - name: acme/auth
      description: Authenticate through the Acme credential broker
      publisher: acme
      capabilities: [auth.provider, auth.identity]
      runtime: native
      source: oci://ghcr.io/acme/atmos-plugins/auth
      versions:
        - version: 1.2.0
          digest: sha256:4d1f0c7e9a31b8c25e6f0a9d73b4c1e8f2a6d5b09c3e7f1a8b2d4c6e0f9a1b3d
          platforms: [linux/amd64, linux/arm64, darwin/arm64, windows/amd64]
          compatibility: {atmos: ">=1.200.0 <2.0.0", protocol: ">=1 <2"}
```

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `metadata.name` | string | yes | none | Catalog name. |
| `metadata.revision` | int | yes | `1` | Incremented on every successful write; used for conflict detection. |
| `metadata.generated_at` | RFC 3339 | yes | set on write | Informational; the cache records its own `fetched_at`. |
| `spec.entries[].name` | `publisher/name` | yes | none | Matches the package `metadata.name`. |
| `spec.entries[].description`, `.publisher` | string | description | empty | Search fields. |
| `spec.entries[].capabilities[]` | list | yes | none | Export selectors from the [extension index](../extensions/README.md) for `--capability` filters. |
| `spec.entries[].runtime` | `native`, `starlark`, `data` | yes | none | `--runtime` filter. |
| `spec.entries[].source` | OCI ref | yes | none | Repository holding the releases. |
| `spec.entries[].versions[].version`, `.digest` | semver, digest | yes | none | Release version and immutable index digest. |
| `spec.entries[].versions[].platforms[]` | list | native | `[]` | Platforms advertised by the index. |
| `spec.entries[].versions[].compatibility` | `{atmos, protocol}` | no | any | Used for compatibility filtering; not authoritative over the package manifest. |
| `spec.entries[].versions[].yanked` | `{reason}` | no | absent | Excluded from new resolution and `update`; locked installs still restore. |

Catalog maintainers must externally serialize `catalog publish` and `catalog add|remove`, covering the read, update, and replacement. The writer rechecks the remote revision/digest and reports an observed conflict (`ErrPluginCatalogConflict`), but this is not atomic compare-and-swap and simultaneous writers can still lose an update. Release publication never modifies a catalog. Removing an entry unlists a release and never deletes its artifact. See [Discovery](../discovery.md#independent-publication-and-catalog-ownership).

## Test configuration: AtmosPluginTestConfig

`tests/contract.yaml` in the plugin repository, consumed by `atmos plugin test`.

```yaml
apiVersion: atmos/v1
kind: AtmosPluginTestConfig
metadata:
  name: acme-auth-contracts
spec:
  timeout: 30s
  scenarios:
    - name: expired-session
      suite: contract
      capability: auth.provider
      kind: acme/broker
      fixture: fixtures/expired-session.yaml
      expect:
        credential_type: aws/v1
        broker_requests: 1
        reused_expired_credentials: false
    - name: live-login
      suite: integration
      capability: auth.provider
      kind: acme/broker
      fixture: fixtures/live-login.yaml
      requires:
        inputs: [ACME_BROKER_TEST_TOKEN]
      expect:
        credential_type: aws/v1
```

A fixture is a plain YAML file, not an enveloped kind:

```yaml
clock: "2026-10-19T12:00:00Z"
config: {endpoint: https://broker.test, audience: atmos, token_env: ACME_BROKER_TOKEN}
inputs: {ACME_BROKER_TOKEN: synthetic-token}
stored_credentials: {type: aws/v1, expires_at: "2026-10-19T11:00:00Z"}
mock:
  http:
    - {method: POST, url: https://broker.test/session, status: 200, body: {access_key_id: AKIAEXAMPLE}}
```

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `spec.timeout` | duration | no | `30s` | Per-scenario default. |
| `spec.scenarios[].name` | string | yes | none | Unique in the file; used by `--run`. |
| `spec.scenarios[].suite` | `contract`, `integration` | no | `contract` | `integration` scenarios run only with `--integration` and are reported as not selected otherwise, never as passing. |
| `spec.scenarios[].capability` | export selector | yes | none | From the [extension index](../extensions/README.md). |
| `spec.scenarios[].kind` | string | yes | none | The export under test; must exist in the manifest. |
| `spec.scenarios[].fixture` | path | yes | none | Provider config, frozen clock, stored state, synthetic inputs, mocked API responses. Real credentials are never loaded. |
| `spec.scenarios[].requires.inputs[]` | list | integration | `[]` | Names of required test inputs; `--integration` fails clearly when any is absent. |
| `spec.scenarios[].expect` | map | yes | none | Capability-specific assertions (credential type, request counts, findings, exit status, diagnostics). Unexpected `api.HTTP` or process calls fail the mocked suite. |

## Path packages

`plugins.packages.<alias>.path` selects a plugin directory instead of a release. Profiles deep-merge over the base configuration, so the usual form is a profile of the developer's own choosing (any name) that adds `path` to a package the base configuration declares from a registry:

```yaml
# atmos.yaml
plugins:
  packages:
    corporate:
      source: oci://registry.example.com/plugins/oidc
      version: ^0.1.0
```

```yaml
# profiles/<any-name>/atmos.yaml, activated with --profile or ATMOS_PROFILE
plugins:
  packages:
    corporate:
      path: build/corporate-oidc
```

The merged package has `source`, `version` and `path`. With the profile inactive nothing changes and the package is a normal locked release. With it active, `path` wins:

- The manifest is read from `<path>/plugin.yaml` on every command and the executable comes from it (the entry for the running platform, otherwise `spec.runtime.executable`), so rebuilding is picked up without reinstalling. There is no lock entry, cache, digest or signature evidence.
- `install` and automatic restoration skip the package, print a notice, and never lock it; an existing lock entry for the release is left untouched.
- `install --frozen` fails with `ErrPluginFrozenDrift` while the active configuration sets `path`, naming the package and, when known, the active profiles. `remove <alias>` fails with `ErrPluginPathPackage` because the path comes from configuration.
- Activation prints one warning per alias per invocation: the plugin runs from a local path and lock, signature and version checks are bypassed.
- Like release packages, a path package is activated only when a command needs it, either because the configuration references a kind it exports or because `plugin run` selects it.
- `list` and `info` show state `path` (or `broken` with a hint when `plugin.yaml` cannot be read) and report the name and version from the local manifest; structured output carries `source: path`.
- Policy: `origin: development`, with `input.path` (relative to the project, or absolute when outside it) and `input.profiles`. `plugins.policy.require_signature` governs released packages only, and there is no separate switch for local builds; deny them with a rule. See [Plugin policies](../policy.md).

A path package is trusted code execution from a local path.

## Stack and component configuration additions

New fields other PRDs introduce. Each coexists with the existing field named in its comment.

```yaml
# atmos.yaml
ci:
  enabled: true                       # Existing. provider is new.
  provider: acme/runner               # Exported ci.providers ID; omitted = ordered detection.
validate:
  format: rich                        # Existing, alongside editorconfig.
  checks:                             # New. Project-level file and artifact checks.
    naming:
      validator: acme/naming          # A validation.rules or validation.validators export.
      files: ["stacks/**/*.yaml"]
      options: {pattern: "^[a-z][a-z0-9-]+$"}
stores:
  corporate:
    kind: acme/kv                     # Existing kind, now also a plugin kind.
    options: {endpoint: https://store.acme.example}
auth:
  providers:
    corporate-sso:
      kind: acme/broker               # Plugin provider kind.
      spec: {endpoint: https://broker.acme.example, audience: atmos, token_env: ACME_BROKER_TOKEN}
  identities:
    production:
      kind: acme/account              # Plugin identity kind; aws/assume-role remains valid.
      via: {provider: corporate-sso}
      principal: {account: prod}
ai:
  skills:
    suite-operations:
      source: plugin://suite/suite-operations   # Existing source field; plugin://<alias>/<skill> is new.
      clients: [claude]
```

```yaml
# Stack manifest
components:
  terraform:
    app:
      backend_type: s3                # Existing; kind below is independent of it.
      backend: {bucket: acme-state, key: app/terraform.tfstate, region: us-east-1}
      provision:
        backend:
          enabled: true               # Existing.
          kind: acme/managed-state    # New. Omitted = dispatch by backend_type.
          options: {service_tier: standard}
      settings:
        validation:
          required-tags:
            validator: acme/required-tags   # New. Exclusive with schema_type/schema_path in one entry.
            options: {required: [Owner, CostCenter]}
            timeout: 10
      secrets:
        providers:                    # New.
          corporate:
            kind: acme/secrets
            options: {endpoint: https://secrets.acme.example}
        vars:
          API_TOKEN:
            provider: corporate       # New. Exclusive with store and sops in one declaration.
            scope: instance
      hooks:
        audit-deployment:
          events: [after.terraform.apply]
          kind: acme/deployment-audit # Existing field; plugin kinds are new values.
          on_failure: fail
          with: {endpoint: https://audit.acme.example/deployments}
  pulumi:                             # Plugin component type; sections follow the dynamic component shape.
    network:
      vars: {cidr: 10.0.0.0/16}
```

```yaml
# workflows/release.yaml
workflows:
  prepare:
    steps:
      - name: change
        type: acme/change-request     # Existing type field; plugin step types are new values.
        with: {service: payments, environment: production}
```

| Addition | Coexists with | Rule |
|---|---|---|
| `ci.provider` | `ci.enabled`, reporting settings | Explicit selection wins and must be a declared, compatible export. Multiple detection matches without it are an error; no match keeps the generic fallback. |
| `validate.checks.<name>.{validator, files, options}` | `validate.format`, `validate.editorconfig` | `validator` must resolve to an export whose `input` is `file` or `artifact`; unknown `--check` names are errors. |
| `settings.validation.<name>.{validator, options, timeout}` | `schema_type`, `schema_path`, `disabled` | `validator` is mutually exclusive with `schema_type`/`schema_path`; `options` validate against `options_schema`. |
| `provision.backend.{enabled, kind, options}` | `backend_type`, `backend` | `kind` selects the provisioner; `backend_type` still names the state protocol Terraform understands. |
| `secrets.providers.<name>.{kind, options}`, `secrets.vars.<K>.provider` | `store`, `sops` selectors | `provider` is exclusive with `store` and `sops`; the referenced name must exist under `secrets.providers`. |
| `stores.<name>.kind` (plugin) | `type`, `identity`, `secret`, `options` | Kind resolves to a declared `stores` export; `options` validate against its schema. |
| `auth.providers.<name>.kind`, `auth.identities.<name>.kind` (plugin) | `spec`, `via`, `principal` | Plugin schemas validate `spec` and `principal`; chain types are checked before login. |
| `hooks.<name>.kind` (plugin) | `events`, `when`, `on_failure`, `with` | `with` validates against the hook schema; `kind: step` with a plugin `type` also works. |
| workflow `type:` (plugin) | `with`, `name` | Namespaced types pass static validation through the step schema. |
| `ai.skills.<name>.source: plugin://<alias>/<skill>` | `clients`, `scope` | Resolves at the lock's version; cannot upgrade independently of the package. |

## On-disk layout (target beyond the native pilot)

```text
$XDG_DATA_HOME/atmos/plugins/                       payloads; content-addressed
  acme/auth/1.2.0/sha256-4d1f…b3d/
    plugin.yaml                                     verified manifest
    bin/acme-auth                                   selected platform only
    schemas/  skills/  policies/                    package-relative files
    .install.yaml                                   receipt: index digest, platform digest, verification result
  .staging/<id>/                                    atomic install workspace; removed on success
$XDG_CACHE_HOME/atmos/plugins/discovery/
  <source-and-scope-hash>.json                       entries, digest metadata, fetched time
  ...                                             source reference/kind, auth, realm, host bindings scope
$XDG_CONFIG_HOME/atmos/
  plugins.yaml                                      user-scope config
  plugins.lock.yaml                                 user-scope lock
<project>/
  atmos.yaml                                        plugins: section (committed)
  .atmos.plugins.lock.yaml                          committed
  toolchain.lock.yaml                               existing; referenced by lock_ref (committed)
  .atmos/skills/installations.json                  existing; plugin-owned skill copies add ownership records
```

Paths resolve through [`GetXDGDataDir`, `GetXDGCacheDir`, and `GetXDGConfigDir`](../../../../pkg/xdg/xdg.go), so `ATMOS_XDG_*_HOME` and `XDG_*_HOME` apply. Digest directories use `sha256-<hex>` because `:` is invalid in Windows paths. `plugins.cache_dir` places payloads and discovery metadata under one root.

Rollback restores `atmos.yaml`, `.atmos.plugins.lock.yaml`, and `toolchain.lock.yaml` together.

Cache behavior:

- Payload directories are immutable and keyed by digest. Every use verifies the recorded digest; a mismatch is `ErrPluginDigestMismatch`. Unreferenced payloads are removed only by an explicit prune, never implicitly; removing a package never deletes shared artifacts.
- Discovery caches are fresh until `plugins.discovery.cache_ttl` elapses. `search --refresh` bypasses freshness; direct repository indexing reuses metadata for unchanged immutable digests. Offline search accepts stale metadata but fails for missing/corrupt cache or `--refresh`. Online refresh errors are reported rather than silently using stale data. Current source policy applies to cache hits and result destinations. Cached discovery never replaces installation verification. See [Discovery](../discovery.md).
- A failed or interrupted install leaves `.staging/<id>` and the previous selection intact; retry re-verifies before completing.
- Ordinary commands restore missing declared project packages automatically. Missing lock entries are resolved and recorded once; existing entries restore by digest without selecting upgrades or rewriting the lock. Verified cached payloads require no registry access. Offline policy rejects missing artifacts, and integrity failures are not treated as absent installations. Disabled packages and packages that set `path` skip restoration; `plugin run` restores only its requested alias.

## Schema and code touchpoints

| Config surface | Go struct (to add) | JSON Schema | Validation entry point |
|---|---|---|---|
| `plugins:` in `atmos.yaml` (implemented) | `schema.PluginsConfig` as `AtmosConfiguration.Plugins` (`yaml:"plugins"`), plain data in `pkg/schema` | Generated into `pkg/datafetcher/schema/atmos/config/1.0.json` | `pkg/plugin/config.Load` after `atmos.yaml` and imports, before auth, stores, or stack processing |
| Governing plugin admission (implemented) | `policy.PluginInput` / `guard.PluginInput` | Registered `plugin` input schema and `Policy` manifest `on` enum | Existing `pkg/policy/admission` invocation; collectors in `pkg/plugin/policy` |
| User `plugins.yaml`, machine policy (pending) | Same `schema.PluginsConfig` | Validates against the `plugins` subschema; no separate generator target | `pkg/plugin/config.LoadUser`, `LoadPolicy` |
| `AtmosPluginConfig` | `pkg/plugin/manifest.ConfigSpec` | Generated by `manifest.Register`; committed copy under `pkg/datafetcher/schema/plugin/` | `manifest.Load[ConfigSpec]` plus `plugin validate` path, schema, and collision checks |
| `AtmosPluginLock` | `pkg/plugin/lock.Spec` | Same mechanism | `pkg/plugin/lock.Read`, and frozen comparison in `pkg/plugin/install` |
| `AtmosPluginCatalog` | `pkg/plugin/catalog.Spec` | Same mechanism | `pkg/plugin/catalog.Read`, revision recheck in the catalog writer |
| `AtmosPluginTestConfig` | `pkg/plugin/manifest.TestSpec` | Same mechanism | `plugin test` loader |
| Dev override state | `pkg/plugin/config.DevState` | None (local state) | `pkg/plugin/config.LoadDev` |
| Stack additions (`ci.provider`, `validate.checks`, `settings.validation.*.validator`, `provision.backend.kind`, `secrets.providers`, `secrets.vars.*.provider`) | Fields on the corresponding `pkg/schema` structs | `pkg/datafetcher/schema/stacks/stack-config/1.0.json` and `pkg/datafetcher/schema/atmos/manifest/1.0.json` | Stack processing, then plugin option schemas at use |
| `ai.skills` `plugin://` source | Existing [`AISkillConfig.Source`](../../../../pkg/schema/ai.go) (no new field) | Existing schema | `skills` loader plugin stage |

How the pieces register:

- The four manifest kinds register through [`manifest.Register`](../../../../pkg/manifest/registry.go), which generates the envelope JSON Schema from the spec prototype at startup and compiles it, so a bad spec struct fails early. Registration currently replaces on duplicates; the platform changes it to error (Phase 0 item P0.3). Each spec type needs `jsonschema` tags and doc comments so generated descriptions are useful.
- `atmos.yaml` schema is reflected from `schema.AtmosConfiguration` by [`pkg/config/schema`](../../../../pkg/config/schema/schema.go). Adding `Plugins` to the struct with doc comments on every field adds the section; `go generate ./pkg/config/schema` rewrites `pkg/datafetcher/schema/atmos/config/1.0.json`, and a drift test fails until it is committed. Fields tagged `yaml:"-"` stay out.
- Stack-manifest schemas are hand-maintained and need edits in both stack schema files, plus the section whitelists noted in the [gap analysis](gap-analysis.md).
- Package-supplied schemas (`config_schema`, `spec`, `principal`, `options`, `with`) are fetched from the verified install directory and compiled on demand, not copied into Atmos's schemas.
- Per CLAUDE.md, every new config option also needs Docusaurus pages, and Go docs on each exported field become schema descriptions.
