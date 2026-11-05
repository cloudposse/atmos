# Plugin admission policies

**Status:** Implemented for the native pilot in the working branch; not yet released. [First release](first-release.md) · [General policy PRD](../policies.md) · [User reference](../../../website/docs/policies/plugins.mdx).

## Outcome and scope

A company can allow plugins only from its own OCI registry and optionally approve individual packages, versions, runtimes, or capabilities. The same decision applies to direct OCI installation, catalog-discovered and repository-indexed packages, automatic restoration, and cached activation. Discovery re-evaluates source policy even when all metadata comes from the local cache. Cached metadata is a discovery hint, never installation trust: source/package policy, digest integrity, compatibility, and signature verification still run during installation. An approved catalog cannot redirect a dependency to a prohibited registry.

Reuse `pkg/policy` and its protected governing definitions, CEL and restricted Starlark evaluators, parameters, input schemas, denial diagnostics, and offline policy tests. Register `on: plugin`; do not create a second expression engine or plugin-specific policy directory. These controls govern plugin admission. Plugin-provided validation engines and rule packs are separate, follow-on extension types.

## Company-only registry example

Place this manifest in the governing `policy.d/` directory, or an absolute trusted directory selected by `ATMOS_POLICY_DIR`:

```yaml
apiVersion: atmos/v1
kind: Policy
metadata:
  name: company-plugins
  description: Permit approved packages from the company OCI registry.
spec:
  policies:
    approved-registries:
      on: plugin
      action: deny
      reason: Plugins and catalogs must use the company registry
      with:
        allowed_registries:
          - registry.example.com
      rule: >-
        input.origin in ['development', 'layout'] || !(input.registry in params.allowed_registries)
    approved-packages:
      on: plugin
      action: deny
      reason: Only approved plugin packages may be installed or activated
      with:
        allowed_plugins:
          - acme/oidc
          - acme/platform
      rule: "input.phase == 'package' && !(input.name in params.allowed_plugins)"
```

Registry entries match the exact authority, including a port when present. They contain neither `oci://` nor repository paths. A shared registry such as `ghcr.io` also needs a repository restriction if only one organization is allowed. Package entries match inspected `metadata.name`, not a project alias or catalog label. The package rule is optional when registry membership alone is sufficient.

For Starlark, set `engine: starlark` and use these respective predicates:

```python
return ctx.input["origin"] in ["development", "layout"] or ctx.input["registry"] not in ctx.params["allowed_registries"]
```

```python
return ctx.input["phase"] == "package" and ctx.input["name"] not in ctx.params["allowed_plugins"]
```

The [allowed registries](../../../policy-library/allowed-plugin-registries/README.md) and [allowed packages](../../../policy-library/allowed-plugin-packages/README.md) library entries ship both implementations with shared allowing and denying fixtures.

## Input contract

All fields are present. Unavailable strings are empty and unavailable capabilities are an empty list. The input contains no credentials, tokens, or plugin configuration. The only filesystem path is `path`, which is relative to the project directory whenever the plugin lives inside it.

| Field | Meaning |
|---|---|
| `operation` | Guarded action; see the operation table below. |
| `phase` | `source` before access; `package` after static inspection. |
| `origin` | `registry`, `cache`, `development`, or `layout`. |
| `source` | Normalized `oci://registry/repository`, without tag or digest; login uses `oci://registry`; development uses an empty string. |
| `registry` | Registry authority including port; empty for development. |
| `repository` | Repository path; empty for login and development. |
| `name`, `version` | Inspected manifest identity and version; empty in the source phase. |
| `digest` | Release index digest when available; empty otherwise. |
| `runtime` | Inspected runtime, such as `native`; empty in the source phase. |
| `capabilities` | Sorted, unique IDs discovered through the extension registry, such as `auth.identity`, `auth.provider`, and `command`. |
| `development` | True for local `path` packages and local conformance testing. |
| `path` | Plugin directory of a local build (`origin: development`). Inside the project it is a clean forward-slash path relative to the project directory with symlinks resolved, such as `build/corporate-oidc`; outside the project it is absolute and slash-normalized. It never contains `..`, so a relative allow-root cannot match a directory that escapes the project. Empty for every other origin. |
| `profiles` | Active profile names exactly as typed, without duplicates, in activation order (from `--profile`, `ATMOS_PROFILE`, or `profiles.default`); an empty list when none is active. |

Profile names are chosen by the developer, so a rule keyed on a name can be avoided by renaming the profile. Rules that gate trust rely on `origin` and `path`; use `profiles` for organization-specific conventions.

### Approved roots for local builds

A `path` package (`plugins.packages.<alias>.path`, usually set in a profile) and `plugin test <dir>` have `origin: development`. Allow local builds only below approved project roots:

```yaml
spec:
  policies:
    approved-dev-roots:
      on: plugin
      action: deny
      reason: Local plugin builds must live under an approved directory
      with:
        dev_roots:
          - build/
      rule: "input.origin == 'development' && !params.dev_roots.exists(r, input.path.startsWith(r))"
```

`exists` and `startsWith` are standard CEL and are covered by `TestAdmissionPathPackageAllowRoots` in `pkg/plugin/bootstrap`. A plugin outside the project has an absolute `path`, so it never matches a relative root such as `build/`.

Capability IDs are contracts, not raw manifest grouping keys such as `auth` or `commands`. Adding a registered extension descriptor supplies its policy metadata through the same discovery mechanism; it does not require edits to the policy collector. Unsupported required capabilities still fail compatibility validation.

Source rules cannot depend on metadata not yet fetched. Name, runtime, and capability predicates select `phase == 'package'`. Registry rules apply at both phases. Rules do not accept component-instance `match` selectors because plugin admission has no component instance.

## Enforcement points

| Operation | Required gate |
|---|---|
| `install` | Source before download, cached reuse, or local import; inspected package before cache promotion and declaration/lock commit. Includes automatic installation. |
| `activate` | Current policy against verified cached bytes or a `path` package manifest before export activation and execution. |
| `test` | Local source and inspected manifest before the conformance runner starts executable code. |
| `search` | Each configured discovery source, including results served from the local cache. |
| `publish` | Destination and inspected release before publication. |
| `sign` | Source and inspected immutable release before signing and attachment. |
| `registry.read` | Actual catalog/package repository before metadata, tags, descriptors, or referrers are read. |
| `registry.write` | Actual repository before catalog, release, or signature writes. |
| `registry.delete` | Actual repository before remote deletion. |
| `registry.configure` | Catalog reference before registry configuration changes. |
| `registry.login` | Registry authority before reading/storing credentials. |

`test` acts on a local development directory (exposed as `input.path`), so it is governed only by `on: plugin` rules (for example `input.operation == 'test'`, optionally with `input.development`). It is deliberately not subject to `plugins.policy.require_signature`: an author must be able to test an unsigned local plugin before there is anything to sign. Signature requirements apply to released packages when they are installed or activated, not while a plugin is authored. `plugin build` compiles local source, admits nothing and runs no plugin code, so it is not a policy operation: a broad `on: plugin` rule (such as the company-registry example, which rejects `origin: development`) does not block it.

`pkg/plugin/policy` collects inputs and wraps the shared OCI registry client. Domain entry points in install, bootstrap, conformance, publish, sign, and management supply inspected-package checks. Transport gates run before credential resolution and network access. No plugin business logic belongs in `internal/exec`.

Read-only local inspection and local removal/logout remain usable for diagnosis and cleanup. Downloaded bytes may remain in shared storage after a failed batch, but no partial declaration/lock transaction may report success.

## Cache, development, and offline imports

Cached artifacts are marked `origin: cache`, selected through a protected project lockfile, and verified against their index/platform digests. Re-read actual manifest metadata; do not authorize from a copied lockfile name, an old policy decision, or an informational signature label. Tightening policy must block an already-installed package on its next activation.

Local development and conformance testing have `origin: development`, an empty registry, and no authenticated release provenance. The company-only example rejects them. A governing policy may explicitly permit development, for example below approved roots (see above); package-name rules still apply. A `path` package has no signature, so `plugins.policy.require_signature` governs released packages only and policy rules over `origin` and `path` are the only gate for local builds. There is no separate switch that disables local builds: deny `origin: development` with a rule.

An explicit `--from-layout` import has `origin: layout`. Its declared source can name an approved repository without contacting it, so the company-only policy rejects layouts independently of the claimed registry. Supporting offline imports requires an explicit policy exception and the same digest/signature verification as registry installation. General offline cache reuse is a separate path and remains possible when all required bytes and trust evidence are available.

## Composition and trust boundary

A denial or evaluation error fails closed. A passing rule cannot override another denial, digest checks, compatibility validation, or `plugins.policy` signature/trusted-publisher requirements. There is no policy result cache that authorizes future executions.

Registry restrictions constrain the repository contacted; package metadata is not cryptographic publisher identity. Pair them with authenticated Sigstore identity/issuer constraints when publisher provenance is required. Mandatory signer groups from loaded configuration layers compose as OR within a layer and AND across layers against the same signature. Offline verification requires cached, unexpired trust metadata and valid evidence. Native plugins remain executable code with the user's privileges; admission does not sandbox subsequent plugin network or filesystem activity.

Organizational enforcement requires trusted control of the Atmos binary, policy directory/revision, environment, and project lockfile. CI must protect those inputs independently of candidate changes. A user who can replace the runtime or governing policy can bypass local checks. This feature does not imply remote organizational policy distribution, a new machine-policy file, or a process sandbox.

## Acceptance

- CEL and Starlark share fixtures for approved/unapproved authorities, explicit ports, lookalike hosts, package names, source/package phases, and development/layout exceptions.
- Direct OCI references and catalog entries cannot bypass the allowlist. Catalog and package destinations are checked separately.
- Denied operations perform no registry request, credential lookup/storage, executable launch, or configuration/lock mutation at the applicable gate.
- Fresh install, automatic restoration, offline cached activation, local testing, publish/sign, and registry configuration/login use the same governing policy.
- Cache integrity is checked before admitting actual metadata; forged lock metadata is rejected.
- Normalized capability checks work for auth, executable commands, and a test-only future descriptor without a new collector switch.
- A passing general policy cannot bypass a required signature or trusted publisher of a released package.
- `input.path` is relative and `..`-free for plugins inside the project (including through symlinks) and absolute otherwise; `input.profiles` is empty when no profile is active.
- Input/schema registration, `policy check`, diagnostics, and policy-library tests include the new kind. Credentials never appear in inputs or denial output.
