# Using Atmos plugins

**Status:** Native project-scoped pilot implemented; broader UX below remains a target. Remote `info`, user-wide installs, and command shortcuts are not yet implemented. See the [first-release checkpoint](first-release.md#native-pilot-checkpoint). [All plugin PRDs](README.md).

## Discover and install an enterprise auth plugin

A platform team publishes `acme/auth` to its private registry. An infrastructure developer runs:

```shell
# Register a discovery source in this project's atmos.yaml.
atmos plugin registry add acme oci://ghcr.io/acme/atmos-plugins/catalog:stable --identity=registry-reader
atmos plugin registry login acme
atmos plugin search --registry acme --capability auth.provider
atmos plugin install acme/auth --as corporate --version '~1.2'
atmos plugin list
atmos plugin info corporate
atmos plugin list --format json
```

Registry auth belongs to `atmos auth login/logout`. Configure `plugins.registries.<alias>.identity` or use `registry add --identity` to reference a named identity; registry login/logout are convenience entry points for the same session. Built-in `oci/user` supports username/token access, and cloud identities use matching ECR/ACR integrations. Unbound registries retain the shared Docker credential chain and explicit `--username` / `--password-stdin` fallback. `registry add/remove` manages registry hosts and optional discovery sources independently. Catalogs are optional: a small private collection can list OCI repositories, while public or product catalogs provide curated snapshots. Both feed the same cached search interface; see [Discovery](discovery.md). Credentials never enter plugin manifests or project locks. A registry must be accessible before downloading its auth plugin; authentication cannot bootstrap through the plugin being fetched.

Example search and installation output:

```text
PLUGIN      VERSION  CAPABILITIES              RUNTIME  SOURCE
acme/auth   1.2.0    auth.provider             native   ghcr.io/acme/atmos-plugins/auth

✓ corporate (1.2.0)
```

Installation writes the declaration to `atmos.yaml` (a new package without `--version` is pinned as `^1.2.0`) and the selected release to `.atmos.plugins.lock.yaml`. The text above is illustrative; `plugin list` and `plugin info` show the exported kinds.

`registry add` and `install <source>` edit the project's root `atmos.yaml`, preserving unrelated fields and comments. They do not rewrite imported files; an incompatible inherited declaration produces a diagnostic identifying its source. The global `--config` flag selects a different root configuration. User-scope (`--global`) plugin configuration is a target and is not implemented: `install` and `registry add` have no `--global` flag, and package declarations accept only `scope: project`. These management commands also work in a new directory before any integration plugin is available.

The resulting configuration is ordinary, reviewable YAML:

```yaml
plugins:
  registries:
    acme:
      catalog: oci://ghcr.io/acme/atmos-plugins/catalog:stable
  packages:
    corporate:
      source: oci://ghcr.io/acme/atmos-plugins/auth
      version: '~1.2'
```

The first-release authentication walkthrough must additionally exercise the full [Go OIDC pilot](oidc-pilot.md); the token-to-broker example here is only its exchange step.

`acme/auth` is the package name returned by discovery; `corporate` is this project's dependency alias; `acme/broker` is the provider kind exported by the package. Installation records the resolved OCI source, so later catalog edits cannot redirect an already-declared dependency. The lockfile records the selected version, manifest digest, and platform artifact digests. Commit both configuration and lockfile; downloaded payloads live in Atmos's user cache.

## Use the provider through normal Atmos auth commands

The installed plugin adds a provider kind, not a separate login command. Add this alongside the plugin declaration:

```yaml
auth:
  providers:
    corporate-sso:
      kind: acme/broker
      spec:
        endpoint: https://broker.acme.example
        audience: atmos
        token_env: ACME_BROKER_TOKEN
  identities:
    production:
      kind: aws/assume-role
      via:
        provider: corporate-sso
      principal:
        assume_role: arn:aws:iam::111111111111:role/PlatformAdmin
```

The example broker exchanges a token supplied through `ACME_BROKER_TOKEN` for temporary AWS credentials. The plugin declares this input and reads it through Atmos's secret service; the token value is never written into the manifest, project configuration, or lockfile. A browser-based provider uses the same plugin mechanism with `api.Browser` and prompt services instead.

```shell
atmos auth login --identity production
atmos auth whoami --identity production
atmos terraform plan vpc -s production --identity production
atmos auth logout --identity production
```

On login, Atmos resolves `corporate-sso` to the installed `acme/broker` implementation, validates `spec` using its packaged schema, and calls its authenticate handler. The returned `aws/v1` (a registered [credential type](platform/configuration.md#credential-types)) credentials become input to the existing `aws/assume-role` identity. Atmos retains chain orchestration, credential storage, expiration handling, environment preparation, and masking.

A provider yielding an opaque corporate session instead of AWS credentials cannot feed `aws/assume-role` directly. Its package can also export an identity kind, such as `acme/account`, which exchanges that session for cloud credentials. The user then sets `kind: acme/account`, keeps `via.provider: corporate-sso`, and supplies the account selector under `principal`. No Atmos source edit or blank import is needed for either kind.

```mermaid
flowchart LR
    Config["kind: acme/broker"] --> Registry["Atmos auth registry"]
    Registry --> Adapter["Native RPC or Starlark adapter"]
    Adapter --> Broker["Corporate broker"]
    Broker --> Credentials["aws/v1 credentials"]
    Credentials --> Identity["Built-in aws/assume-role"]
    Identity --> Execution["Atmos command environment"]
```

## Restore, update, and remove

```shell
# A teammate or CI runner restores the committed selection.
atmos plugin install --frozen
# Select a newer release within the declared constraint.
atmos plugin update corporate
# Deliberately change the constraint and selected release.
atmos plugin update corporate --version '~2.0'
# Restore previously committed configuration and lockfile, then reinstall.
atmos plugin install --frozen
# Remove the local declaration and lock entry; remote releases remain available.
atmos plugin remove corporate
```

`install` with no source restores declared dependencies; it creates missing locks only outside frozen mode. `--frozen` requires a matching lockfile and never resolves a newer version. `update` reports the old and new version/digest and edits the lockfile only after installation succeeds. Removal reports remaining configuration references and requires those references to be removed or changed; it does not silently delete identities. Restoring a prior lockfile is the rollback operation.

Missing declared plugins install automatically before their exports are registered, including before authentication for a normal `plan`. First use resolves and locks a declaration without an existing entry; subsequent installations restore its immutable digest. Cached use does not contact registries. Offline policy, integrity failures, and declaration/lock drift fail explicitly; ordinary execution never upgrades a locked selection. Disabled packages and `path` packages skip release downloads. `plugin run` restores only its selected alias, while inspection commands remain read-only. `atmos plugin info corporate` shows the effective source, version, digest, runtime, exported kinds, schemas, compatibility, and the local `path` when one is set.

## Consistent output and company controls

`search`, `list`, `info`, `registry list`, `validate`, `pack`, `publish`, and `sign` use the common list renderer. `--format` accepts `table` (default), `json`, `yaml`, `csv`, and `tsv`; `ATMOS_PLUGIN_FORMAT` supplies an environment default. Authoring commands confirm their completed operation and show the relevant exports, layout, or immutable release metadata. JSON is explicit. Interactive installed entries have a green dot in an unlabeled indicator column and a separate readable status; piped output has no ANSI decorations. Downloads use the same bounded parallel batch UI as toolchain.

`plugin build` uses the same shared four-line, two-column-padded output viewport as `atmos build`, with a compilation status and an executable path for each completed target. `--platform` names one declared `os/architecture[/variant]`; `--all-platforms` reports every declared target and its output path. Compiler failures identify the target and stop the remaining builds. Redirected output streams diagnostics on stderr. A failed or canceled compilation preserves the previous executable. No plugin-specific TUI or generic `internal/exec` implementation is introduced.

`plugin test` streams Markdown-aware descriptions of completed checks and a final result count. Its default text output explains contract agreement, the local OIDC lifecycle, and command fixture expectations; stable check IDs remain in explicit JSON output and `--run` filtering. `plugin run` identifies the selected command and description before forwarding its output. The starter's `about` command states that authentication was not performed and directs users to `atmos auth login`.

The authoring entry point is `plugin create → build → validate → test` in an ordinary working directory. Copying repository demo fixtures is not a user prerequisite. An unreleased checkout needs only `--sdk-path` for the unpublished SDK; the disposable project and OCI emulator in the recorded lifecycle are recording infrastructure.

Companies restrict sources with protected CEL or Starlark `on: plugin` rules. The [company-registry example](policy.md#company-only-registry-example) permits only the configured company authority, rejects development/layout bypasses, and optionally limits actual manifest names. Automatic restoration and cached activation re-evaluate those rules. This policy is independent of future plugin-provided validators.

Exact version declarations remain exact during `update`. Use `update <alias> --version <new-constraint>` to deliberately change the permitted range or pin. Plugin locks follow the reproducible declaration/lock/restore model used by toolchain, but are a separate format and do not claim identical update semantics.

## Component engines, CI providers, and command extensions (follow-on UX)

The other categories follow the same install-and-configure model:

```shell
atmos plugin install acme/pulumi --as pulumi --version '~1.0'
atmos pulumi preview network -s development
atmos pulumi up network -s development
atmos plugin install acme/buildkite --as buildkite --version '~1.0'
atmos terraform plan vpc -s development --ci
atmos plugin install acme/stacks --as stacks --version '~1.0'
atmos stacks list
atmos plugin run stacks -- list
```

The Pulumi plugin exports the `pulumi` component type and its `preview` and `up` commands. Configure `components.pulumi.base_path: components/pulumi` in `atmos.yaml` and define `components.pulumi.network.vars` in a stack. Atmos resolves imports, inheritance, stack selection, identity, and tool pins before calling the plugin. Plugin engine settings use the existing dynamic component configuration shape.

The CI example adds the proposed explicit selector `ci.provider: acme/buildkite` alongside `ci.enabled: true` and existing reporting settings. Its exported provider ID is `acme/buildkite`. Omitting the selector permits detection among declared providers; multiple matches are an error. The user continues running component commands, while the selected provider handles reporting. Providers cannot bypass Atmos's existing fork-trust and reporting-enable gates.

The `stacks` command extension exposes a root command shortcut. `atmos plugin run <alias> -- <arguments>` always provides a qualified entrypoint, including when the shortcut conflicts with a built-in or YAML custom command. Arguments after `--` are passed unchanged. Should user-wide installation be added later, project declarations win over a user-scope installation of the same alias, and unrelated user-scope integration providers do not join a project's registries.

## Policies, linting, and validation

Install a standards package with `atmos plugin install acme/standards --as standards --version '~1.0'`. Enable its project checks under `validate.checks` or its component checks under `settings.validation`, then run `atmos validate`, `atmos validate component vpc -s production`, or the proposed focused selector `atmos validate --check naming`. Installing a package does not activate every rule.

This is a follow-on extension type; it does not gate the native auth/command release. Packages can distribute OPA policies, JSON Schemas, and lint configuration for existing engines, or implement new validators in Go/Starlark. Both return shared findings for terminal, JSON, SARIF, and CI output. The [policy, linting, and validation design](extensions/validation.md) specifies configuration, authoring, plan checks, enforcement, and tests.


## Combined integrations and additional extensions

A single [Okta package](package-composition.md) can add commands, providers, identities, MCP tools, and skills. [Hook and step extensions](automation-contracts.md) participate in existing automation; [stores](extensions/stores.md) and [secret providers](extensions/secret-providers.md) extend normal data access. Each has a focused PRD in the [extension index](extensions/README.md).
