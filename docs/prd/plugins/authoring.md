# Authoring Atmos plugins

**Status:** Go OIDC scaffolding and authoring commands implemented in the working branch. Broader SDK excerpts and Starlark examples below remain contract proposals; use the generated starter and [current authoring reference](../../../website/docs/cli/commands/plugin/create.mdx) for executable instructions. [All plugin PRDs](README.md).

## Reuse existing scaffolding

`atmos plugin create` is a plugin-specific entry point to the existing `AtmosScaffoldConfig` system. Reuse [`pkg/generator/templates`](../../../pkg/generator/templates), [`pkg/generator/source`](../../../pkg/generator/source), [`pkg/generator/engine`](../../../pkg/generator/engine), and the shared UI/orchestration used by [`cmd/scaffold`](../../../cmd/scaffold). Extract command-neutral orchestration if needed; do not call the scaffold Cobra command or create another renderer, template catalog, update algorithm, or OCI downloader.

First-release templates support Go auth providers/identities and executable commands. Include a configurable Go OIDC starter, mock issuer/broker fixtures, consumer project, README, and publication workflow. Document the template selector through existing scaffold configuration rather than inventing a second template-source setting. The full [OIDC pilot](oidc-pilot.md) is the runnable reference. Generic unfinished handlers must fail contract tests until implemented; the completed OIDC starter must pass with its configured local test services.

The exchange-only example below remains useful for understanding the handler contract. It does not satisfy the OIDC pilot. Starlark snippets describe follow-on support and are not required to author the first-release Go provider.

## Implemented author workflow

The Go OIDC scaffold is the pilot, built through the existing generator. `plugin build [directory]` reads the executable path from `plugin.yaml`, explicitly sets the Go compiler target, streams compiler output through Atmos I/O, and preserves the previous binary if compilation fails. `--platform OS/ARCH[/VARIANT]` builds a declared target; `--all-platforms` builds every declared target into its own manifest path. These flags are mutually exclusive. Inherited `GOOS`, `GOARCH`, and architecture feature settings must not silently change the selected target. A default build selects a compatible target for the running Atmos process. Cross-compilation prepares artifacts; it does not execute or verify them on the destination platform. The acceptance flow is `plugin create` → `plugin build` → `plugin test` → `plugin pack` → `plugin publish`, followed by discovery, installation, and tests against the downloaded executable. `publish --sign` signs the immutable published index; `plugin sign` adds signing separately.

The generated repository includes `atmos.yaml` custom commands: `atmos build`, `atmos test`, and `atmos push`. Push builds, validates, tests, packs into a new ignored local release directory, and publishes. Arguments pass through native Starlark argument lists rather than shell interpolation. Failed build/test steps prevent publication. A preconfigured anonymous OCI emulator and direct repository discovery make `atmos dev registry up` → `atmos push` work without credentials or copied demo fixtures. `atmos dev registry down` removes the ephemeral registry. Publishing never modifies a catalog. `search --refresh` immediately indexes a newly published release. Authenticated publishing uses the existing `atmos auth login --identity publisher` flow and an explicit registry identity binding; local development does not create a dummy identity.

Manifests use `metadata.name` as their stable ID, with version and optional display name, description, author, and repository URL. Repository metadata is author-supplied provenance, not an authenticated publisher claim. Authors may add `spec.compatibility.atmos` with a semver range; protocol and required contract versions are separate checks. The generated starter currently leaves the optional Atmos range unset.

SDK development stays in this repository under the nested `sdk/` module. `atmos dev generate proto` generates both CLI and SDK bindings using pinned tools; parity tests prevent drift. The main module does not depend on an unpublished SDK tag or a local replacement. An explicitly requested scaffold development workspace can use the local SDK; published consumers use a released module version.

Author testing, local `path` packages, publishing, and signing are also subject to [plugin policies](policy.md). A company policy that denies local development must explicitly permit the author workflow in its trusted development environment.

## Package identity and repository naming

Use `<publisher>/<package>` for `metadata.name`. The publisher maintains the Atmos integration. Use the product name for the package and its default consumer alias. For a standalone source repository, use `atmos-plugin-<package>` to identify its purpose outside the Atmos catalog. The repository prefix is not part of the package ID.

| Purpose | Terragrunt example |
|---|---|
| Package ID (`metadata.name`) | `cloudposse/terragrunt` |
| Default product alias (`plugins.packages.<alias>`) | `terragrunt` |
| Source repository (`metadata.repository`) | `https://github.com/cloudposse/atmos` |
| Source directory | `plugins/terragrunt` |
| Release location | Explicit OCI reference, recorded with its digest in the lockfile |
| Standalone repository, if created | `cloudposse/atmos-plugin-terragrunt` |

The independently versioned package is the identity unit. Several packages can share a source repository. Moving a package to a standalone repository must not require consumers to change its ID or alias. Repository naming is an authoring convention, not a requirement to move existing plugins out of the monorepo.

This follows [Terraform's distinction between provider addresses and local names](https://developer.hashicorp.com/terraform/language/providers/requirements). Terraform's public registry uses a separate [`terraform-provider-<name>` repository convention](https://developer.hashicorp.com/terraform/registry/providers/publishing). [TFLint](https://github.com/terraform-linters/tflint/blob/master/docs/user-guide/plugins.md) also uses a short plugin name, but its installation source directly identifies a repository and its name determines the executable. Atmos uses OCI package releases, so its identity does not require one Git repository per plugin.

Toolchain's `owner/repo` identifies the executable distribution; a plugin's `publisher/package` identifies the Atmos integration. For example, `gruntwork-io/terragrunt` is the toolchain product, while `cloudposse/terragrunt` is Cloud Posse's integration. Both can use `terragrunt` as a short name within their respective configuration sections. Consumers can override the plugin alias with `plugin install --as`; the alias does not rename the package or its exported capabilities.

The installer uses the product name as the default alias for a new declaration and requires an explicit alias on a collision. Plugin management has not appeared in a stable release, so the product alias is the only default and needs no edition journal entry; existing declarations always retain their names.

A registry alias selects a discovery source and is separate from the publisher namespace. The installation selector `<registry-alias>/<package-id>` can therefore contain three segments, such as `team/acme/oidc`. Do not interpret the package ID as a GitHub URL or infer an OCI location from it. The configured source and lockfile identify the release; signature and publisher policy establish trust. Identical package names from different sources must not cause silent source substitution.

## Scaffold a separate plugin repository (broader contract example)

```shell
atmos plugin create acme-auth --capability auth.provider --runtime go --kind acme/broker
cd acme-auth
```

The generated repository contains:

```text
plugin.yaml                 Package identity, version, exports, runtime, compatibility
schemas/broker.json          JSON Schema for the provider's spec
cmd/acme-auth/main.go        Registration and plugin startup via the SDK
provider.go                 Provider handlers to implement
provider_test.go            Unit tests against a mock broker
tests/contract.yaml         Behavioral scenarios consumed by atmos plugin test
tests/fixtures/             Mock HTTP responses and expected results
examples/project/           Atmos configuration for trying the provider
go.mod                      Pins github.com/cloudposse/atmos/sdk at an sdk/vX.Y.Z tag
```

The manifest is read without starting the executable. Reuse the existing [Atmos manifest envelope](../../../pkg/manifest/manifest.go): `apiVersion: atmos/v1`, `kind`, `metadata`, and a kind-specific `spec`. Register the proposed `AtmosPluginConfig` and `AtmosPluginTestConfig` kinds with the shared manifest loader and schema registry. This proposed minimal plugin manifest exports one provider:

```yaml
apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/auth
  version: 1.2.0
  description: Authenticate through the Acme credential broker
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/acme-auth
  capabilities:
    auth:
      providers:
        acme/broker:
          schema: schemas/broker.json
          credentials: [aws/v1]
```

The `aws/v1` credential type is registered; see [credential types](platform/configuration.md#credential-types).

`apiVersion` versions the document format; `metadata.version` identifies the independently released plugin; `spec.runtime.protocol` identifies the native Atmos/plugin protocol. These versions are independent. Releasing plugin `1.2.1` does not change `apiVersion` or require an Atmos release. Plugin catalog and lockfile documents must follow the same envelope convention with their own kinds and specs.

`--runtime go` generates a native implementation. In the Starlark follow-on, `--runtime starlark` generates a `.star` entrypoint and the same schema, example, and contract fixtures. Native release packages carry an OS/architecture descriptor and the matching executable. The example executable path is relative to the selected platform package. Starlark manifests use `spec.runtime.type: starlark`, `spec.runtime.api: 1`, and `spec.runtime.entrypoint: provider.star`; the runtime API version is also separate from the document and release versions.

`schemas/broker.json` requires `endpoint`, `audience`, and `token_env`, with a URI format for `endpoint`. Secrets are declared as Atmos-supplied inputs rather than schema defaults. Validation errors identify the configuration field, for example `auth.providers.corporate-sso.spec.endpoint`.

## Implement the authentication exchange

The proposed Go SDK provides typed credential values, requests, results, and a provider base with explicit defaults. These excerpts show the provider implementation and its registration; generated package declarations and imports are omitted:

```go
// Proposed SDK API; exchangeWithBroker is implemented in this plugin's provider.go.
type Broker struct {
    auth.BaseProvider
}

func (b *Broker) Authenticate(ctx context.Context, req auth.Request, atmos api.Services) (auth.Result, error) {
    credentials, err := exchangeWithBroker(ctx, atmos, req.Spec)
    if err != nil {
        return auth.Result{}, err
    }
    return auth.Result{Credentials: auth.AWS(credentials)}, nil
}

func main() {
    plugin.Main(plugin.Definition{
        AuthProviders: map[string]auth.Provider{
            "acme/broker": &broker.Broker{},
        },
    })
}
```

The Plugin SDK imports are the proposed `github.com/cloudposse/atmos/sdk/plugin`, `/auth`, and `/api` packages (`api.Services` provides the Atmos API bindings), plus Go's `context`; `broker` is the author's own provider package imported by `main`. `exchangeWithBroker` reads the declared token through Atmos secret service, sends the broker request through Atmos HTTP service, and maps the response into access key ID, secret access key, session token, region, and expiration. It receives cancellation from `ctx` and returns typed diagnostics on rejection. It does not write credentials to stdout or manage Atmos's keyring.

The scaffold's `main` imports the root provider package and registers its exported `Broker` type. Its generated `Authenticate` implementation initially returns a not-implemented diagnostic; the authentication contract tests fail until the author supplies the exchange. Scaffolding never manufactures working credentials or reports an unfinished provider as passing.

The same exchange in the proposed Starlark binding is:

```python
def authenticate(request):
    token = atmos.secrets.environment(request.spec["token_env"])
    response = atmos.http.post(
        url = request.spec["endpoint"] + "/session",
        headers = {"Authorization": "Bearer " + token},
        json = {"audience": request.spec["audience"]},
    )
    response.raise_for_status()
    data = response.json()
    return auth.result(credentials = auth.aws(
        access_key_id = data["access_key_id"],
        secret_access_key = data["secret_access_key"],
        session_token = data["session_token"],
        region = data["region"],
        expiration = data["expiration"],
    ))

plugin.register_auth_provider("acme/broker", authenticate = authenticate)
```

`plugin`, `auth`, and `atmos` are versioned modules supplied by Atmos. Registration declares callbacks; it does not authenticate. Starlark source is evaluated only for execution or explicit testing, not discovery or help. API services make the exchange equally testable in Go and Starlark.

| Handler or service | Developer responsibility | Atmos responsibility |
|---|---|---|
| Schema and optional validation | Define valid provider-specific inputs. | Validate configuration and render field diagnostics. |
| Authenticate | Exchange upstream proof for typed credentials and expiration. | Resolve the chain, supply scoped inputs, mask and persist results. |
| Optional refresh | Renew provider-specific session state. | Reuse unexpired credentials; reauthenticate when refresh is unsupported. |
| Optional logout | Revoke or clean up provider-owned sessions. | Clear Atmos-managed local credentials, preserving isolation. |
| Optional prepare-environment/paths | Describe extra environment changes or credential files. | Apply them to the relevant command and manage standard credential formats. |
| Prompts and browser interaction | Request the interaction needed by the provider. | Enforce interactive/noninteractive mode and protect secret inputs. |

Default handlers perform no provider-specific refresh, revocation, or file writes; they do not claim those capabilities succeeded. A plugin needing them implements and declares them. Opaque session data includes a plugin-owned schema version; incompatible cached state triggers reauthentication rather than an unsafe decode.

## Try locally, then publish

```shell
go test ./...
atmos plugin build .
atmos plugin validate .
atmos plugin test .
atmos --chdir examples/project --profile=local-build auth login --identity production
atmos plugin pack . --output dist/acme-auth
atmos plugin publish dist/acme-auth --to oci://ghcr.io/acme/atmos-plugins/auth
```

`pack` creates an offline OCI layout from already-built payloads; it does not invoke an arbitrary build script. The [multi-architecture release example](../../../examples/plugin-multiarch-release/README.md) supplies a workflow to copy into the generated repository. It builds and runs conformance natively on Linux, macOS, and Windows for both amd64 and arm64, collects the tested binaries, and packs once before signing and publishing the complete index. The scaffold itself starts with the local platform; authors explicitly add release targets and unique executable paths. Every binary must embed the same final manifest. The example requires a released Atmos version containing these build commands and native test selection; it does not claim the six-platform release has already run. Explicit variants such as `linux/arm/v7` and `linux/amd64/v3` must map to compiler settings, distinct output paths, and compatible test runners. Baseline amd64/arm64 releases remain the default support matrix; consumers never silently cross architectures or request source builds. See [variant selection](distribution.md#architecture-variants). A Starlark package needs no compiler. `publish` takes the release version from the packaged manifest's `metadata.version`, verifies it, and uploads it independently of discovery. An explicit repository list makes releases searchable after refresh without catalog maintenance. Curated catalogs use a separate externally serialized publisher; see [Discovery](discovery.md).

A consumer runs a local build by setting `path` for the package in a profile of its own choosing (here `examples/project/profiles/local-build/atmos.yaml` sets `plugins.packages.corporate.path`). The profile deep-merges over the base declaration, so the package keeps its `source` and `version` for when the profile is inactive, and `path` wins while it is active: the package is never fetched, locked or signature-checked, and Atmos warns each time it is activated. Changes to Starlark source apply on the next invocation; Go changes require rebuilding the binary. `list` and `info` mark the package with state `path` and read its name and version from the local manifest. Frozen installation refuses while the active configuration sets `path`, and `atmos plugin remove corporate` explains that the path comes from configuration instead of editing it. Normal installation from a local packaged OCI layout is digest-verified and can be used offline.

To release a fix, change `metadata.version`, rebuild, rerun tests, pack, and publish. Consumers run `atmos plugin update corporate`; they keep the same Atmos binary and auth configuration when the contract remains compatible.

```shell
# Unlist a release without removing its artifact.
atmos plugin catalog remove acme acme/auth --version 1.2.0
# Delete the exact remote release digest after separately removing catalog references.
atmos plugin delete oci://ghcr.io/acme/atmos-plugins/auth@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

## Extend other capabilities

Proposed: `create` also accepts the selectors in the [extension index](extensions/README.md), including `auth.identity`, `component`, `ci.provider`, `ci.component`, `command`, `validation.validator`, `validation.rulepack`, `hook`, `step`, `mcp.tool`, `skill`, `store`, `secret.provider`, and `provisioner.backend`; additional capabilities can be declared in the same package. Generated adapters and contract fixtures match the chosen capability. A combined plugin may export an engine, its CI reporting hooks, and its validators. Rule packs and skill-only packages can use `--runtime data`; executable extensions use Go or Starlark. Repeating `--capability` to scaffold [multiple extensions in one package](package-composition.md) is also proposed. Today `create` takes a single `--capability` and accepts only `auth.provider`.

For `auth.identity`, the handler receives typed upstream credentials and the identity's `principal`, then returns the next typed credential result. Go registers it under `AuthIdentities`; Starlark uses `plugin.register_auth_identity`. An author adds `acme/account` to the manifest's `spec.capabilities.auth.identities` map with its schema and accepted/returned credential types. Atmos validates chain compatibility and continues to own orchestration.

| Capability | The author implements | What Atmos passes |
|---|---|---|
| Component | Validation, artifact generation, command execution, command metadata. | Resolved component/stack values, working directory, args/flags, identity environment, cancellation, output streams. |
| CI provider | Detection, context/base resolution, checks, comments, outputs and summaries. | Scoped CI environment and credentials, structured reporting requests, `api.HTTP`/output services. |
| Component CI hooks | Declared lifecycle callbacks and result interpretation. | Event, command, exit status, output and structured results; an `api.CIReporter` service for the selected CI provider. |
| Command | An executable entrypoint or Starlark main function. | Arguments, working directory, standard streams, exit and cancellation handling; declared API services for Starlark. |
| Validation | Rules for an existing engine or a callback returning structured findings. | Selected files, resolved component values, or an explicitly supplied typed artifact; provenance, cancellation and scoped API services. |

An executable command can wrap an existing program without the Go SDK. Its static manifest supplies shortcut/help metadata. Native integrations register typed handlers with the SDK; Starlark integrations register the equivalent callbacks. Neither imports Atmos's internal registry packages.


See the [extension PRDs](extensions/README.md) for each contract, [precompiled distribution](distribution.md) for release matrices, and [toolchain dependencies](toolchain-dependencies.md) for required tools.
