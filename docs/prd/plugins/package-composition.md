# One plugin, multiple extensions

**Status:** Full composition is a follow-on. The first release proves auth and executable commands in one package; MCP and skills follow their adapters. All new plugin APIs below are proposed. [All plugin PRDs](README.md).

A plugin is the installable unit: one identity, release version, OCI artifact/index, and lockfile entry. Extensions are the functionality it exports. Users must not install separate packages merely to get an integration's command, identity, MCP tools, or skills.

## Example: an Okta integration

```shell
atmos plugin install acme/okta --as okta --version '~1.0'
atmos plugin info okta
atmos okta apps list
atmos auth login --identity corporate
atmos mcp start
atmos ai skill list
```

This is an illustrative customer-authored Okta package, not a claim about an existing official implementation or specific Okta authentication flow. Its configured `corporate` identity uses the package's registered provider/identity kinds. The same package makes an `okta` command tree, MCP tool, and skill available through their respective Atmos registries. Configuration still selects identities and Atmos policy controls tool exposure.

```text
acme/okta 1.0.0
  command:       atmos okta …
  auth provider: acme/okta
  identity kind: acme/okta-account
  MCP tool:      acme.okta.list_apps
  skill:         okta-operations
```

## Author experience

```shell
atmos plugin create acme-okta --runtime go \
  --capability command --capability auth.provider --capability auth.identity \
  --capability mcp.tool --capability skill
```

Repeated capability flags scaffold one repository, shared implementation/services, extension adapters, schemas, tests, and a release workflow. The author fills in export names and schemas. Each generated executable extension has positive/negative tests; unfinished handlers return not-implemented errors rather than succeeding with empty data.

A proposed manifest fragment:

```yaml
apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/okta
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/atmos-okta
  capabilities:
    commands:
      okta:
        description: Manage corporate Okta access
        executable: bin/atmos-okta
        args: [command]
    auth:
      providers:
        acme/okta:
          schema: schemas/provider.json
          credentials: [acme/okta-session/v1]
      identities:
        acme/okta-account:
          schema: schemas/identity.json
          accepts: [acme/okta-session/v1]
          credentials: [acme/okta-session/v1]
    mcp:
      tools:
        acme.okta.list_apps:
          description: List applications available to the selected identity
          input_schema: schemas/list-apps-input.json
          output_schema: schemas/list-apps-output.json
    skills:
      okta-operations:
        path: skills/okta-operations/SKILL.md
```

The default binary entrypoint serves the native protocol. The same binary's `command` mode handles executable dispatch: Atmos prepends the manifest's fixed `args`, then forwards user argv unchanged. Authors may choose separate executables inside one package; users still install one artifact. Platform mappings select the executable for each OS/architecture.

Provider, identity, command, and tool handlers can call shared package code. Auth-sensitive operations use `api.Credentials` so interfaces do not create independent session caches. Plugin-specific session credentials remain namespaced; feeding a built-in identity requires a declared compatible credential adapter.

## Native services, configuration, and functions together

The proposed [package configuration contract](package-configuration.md) allows the same Go SDK package to export a consumer `atmos.yaml`, custom commands, Git hook definitions, and supporting Starlark scripts. Consumers declare the OCI package once; Atmos loads its explicitly exported configuration automatically, without manual includes or package-cache paths. Loading configuration does not run scripts or install Git shims.

The proposed [function contract](extensions/functions.md) additionally exposes `alias.function(...)` to Starlark callers. Functions may be native handlers or packaged Starlark implementations, including both within a native package. A service runtime does not restrict the other resource types its package can carry.

These capabilities share the native services' version, platform selection, trust, lock entry, and atomic update/rollback. They do not require a second package or a consumer Go compiler. Both contracts are follow-on work, not implemented behavior of the native pilot.

## Selection, atomicity, and collisions

The lock records the package once. Installation validates all declared extensions before committing the package selection; a broken required extension cannot leave half the package activated. Runtime registration must match static metadata. Partial process startup/registration failure is reported without leaking registrations.

Command shortcut collisions are handled separately from invalid registrations: `atmos okta` may be unavailable while `atmos plugin run okta -- apps list` still works. A package with multiple command exports selects one with `--command <export-name>`. Provider, identity, tool, skill, hook, and step name collisions are explicit errors; no extension silently replaces another implementation.

Update/rollback selects one release for every extension. Long-lived MCP/plugin sessions retain their selection until restarted; they cannot mix schemas or skills from one release with handlers from another. User-installed packages cannot fill missing extensions from a project-selected package.

## Acceptance

A single Go package and a Starlark equivalent each provide a command, auth provider/identity, MCP tool, and skill. Tests verify one installation and lock entry, shared credentials without leakage, static inspection without execution, command collision fallback, atomic registration failure, and unified update/rollback. Native consumers receive compiled platform artifacts and never rebuild Atmos.
