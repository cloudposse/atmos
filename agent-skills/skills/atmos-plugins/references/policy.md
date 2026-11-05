# Plugin Policy Reference

Rules with `on: plugin` run in the shared policy engine (CEL or restricted Starlark). They load from `policy.d/`
next to the selected `atmos.yaml`, or from `ATMOS_POLICY_DIR`. A denial or evaluation error blocks the operation, and
no rule can weaken `plugins.policy` signature or trusted-publisher checks. Protect the policy directory, the Atmos
binary and environment, and the lock file independently of candidate changes.

## Phases

| Phase | Evidence | Use |
|---|---|---|
| `source` | registry, repository, normalized source, operation, origin, development flag, path, profiles | Reject a destination or import origin before registry access or cached reuse |
| `package` | source fields plus the inspected manifest and release digest | Reject a package before installation, activation, testing, publication, or signing |

Name restrictions select `phase == 'package'`. Registry restrictions apply to both phases and must not depend on
package metadata. Registry operations have no inspected package and use the source phase.

## Operations

| Operation | Guarded action |
|---|---|
| `install` | Install a release or restore a declared package |
| `activate` | Load a plugin for use, including a cached package or a `path` package |
| `test` | `atmos plugin test` on a local, unsigned directory |
| `search` | Check configured discovery sources, including cached results |
| `publish`, `sign` | Publish or sign a release |
| `registry.read`, `registry.write`, `registry.delete`, `registry.configure`, `registry.login` | Catalog and registry operations |

`atmos plugin build` compiles local source and runs no plugin code, so it is not an operation.

## Input fields

All fields are always present; unknown strings are empty and unknown lists are empty.

| Field | Type | Notes |
|---|---|---|
| `operation`, `phase` | string | See above |
| `origin` | string | `registry`, `cache`, `development`, `layout` |
| `source` | string | `oci://registry/repository` without tag or digest; registry login uses `oci://registry`; empty for local builds |
| `registry` | string | Authority including explicit port; empty for local builds |
| `repository` | string | Path within the registry |
| `name`, `version`, `digest`, `runtime` | string | Inspected package; empty in the source phase |
| `capabilities` | list of string | For example `auth.identity`, `auth.provider`, `command`; empty in the source phase |
| `development` | bool | Local development package or conformance testing |
| `path` | string | Project-relative clean slash path (no `..`) for `origin: development` inside the project; absolute outside; empty for other origins; symlinks resolved |
| `profiles` | list of string | Active profile names as typed, deduplicated, in activation order; empty when none |

`source` is not an authenticated publisher identity: an offline layout can claim any repository. Pair registry and
package rules with `trusted_publishers`.

## Recipes

Approved registries and packages (denies local builds and layouts):

```yaml
approved-registries:
  on: plugin
  action: deny
  reason: Plugins and catalogs must use the company registry
  with: {allowed_registries: [registry.example.com]}
  rule: "input.origin in ['development', 'layout'] || !(input.registry in params.allowed_registries)"
approved-packages:
  on: plugin
  action: deny
  reason: Only approved plugin packages may be installed or activated
  with: {allowed_plugins: [acme/oidc, acme/platform]}
  rule: "input.phase == 'package' && !(input.name in params.allowed_plugins)"
```

Registries rule that permits local builds but not offline layouts, and a localhost registry:

```yaml
with: {allowed_registries: [registry.example.com, "localhost:5001"]}
rule: "input.origin == 'layout' || (input.origin != 'development' && !(input.registry in params.allowed_registries))"
```

Local builds only below approved roots (a relative root never matches an absolute path outside the project):

```yaml
with: {dev_roots: [plugins/]}
rule: "input.origin == 'development' && !params.dev_roots.exists(r, input.path.startsWith(r))"
```

Development allowed for `plugin test` only:

```yaml
rule: "input.origin == 'development' && input.operation != 'test'"
```

Organization convention on profile names (advisory; limited to `activate` because `plugin test` has no profile):

```yaml
rule: >-
  input.origin == 'development' && input.operation == 'activate' &&
  !input.profiles.exists(p, p.startsWith('local-'))
```

Starlark variants return a strict boolean from `ctx.input[...]`, for example
`return ctx.input["origin"] == "development" and ctx.input["operation"] != "test"`.
