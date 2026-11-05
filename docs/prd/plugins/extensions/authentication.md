# Authentication extensions

**Status:** Initial release. [All extension types](README.md).

Plugins can export provider kinds, identity kinds, or both. An integration package may also export commands, MCP tools, and skills. Consumers install it once and continue using normal `atmos auth` and component commands.

## Contract

- Providers validate their own schema, acquire typed credentials, report expiry and identity, prepare supported credential outputs, and implement applicable refresh/reuse/logout behavior.
- Identities accept declared upstream credential types and principal configuration, then return typed downstream credentials.
- Atmos owns chain orchestration, realms, credential isolation, persistence policy, environment preparation, masking, and cancellation. Plugin-specific session data stays namespaced to its package.
- Existing credential formats have adapters. Plugin credentials can feed a built-in identity when their contract matches; unsupported combinations fail before an operation.
- Prompts and browser interactions are Atmos-mediated. Noninteractive execution reports required interaction explicitly.
- Complete auth registry adoption without breaking built-in provider kinds or schemas. Do not expose internal managers across the protocol.
- Credential preparation is part of this contract; opening every separate auth-integration registry interface is a follow-on.

The [authoring walkthrough](../authoring.md) defines Go/Starlark scaffolds and the provider exchange. The [consumer walkthrough](../user-experience.md#use-the-provider-through-normal-atmos-auth-commands) shows configuration and login. [Package composition](../package-composition.md) shows commands and identities in one Okta package.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`registry.Registry`](../../../../pkg/auth/registry/registry.go) with `RegisterProvider(kind, ProviderConstructor)` and `RegisterIdentity(kind, IdentityConstructor)`. Built-in and plugin constructors share the process-wide registry; duplicate kinds are rejected. |
| Duplicate policy | Provider and identity registration reject duplicate kinds, including collisions between built-ins and plugins; batch registration is atomic. |
| Context on the interface | Yes. `Authenticate`, `PrepareEnvironment`, and `Logout` take `context.Context`. |
| Reachable from outside the module | Yes. [`types.Provider` and `types.Identity`](../../../../pkg/auth/types/interfaces.go) are public, and exported registration functions add constructors to the process-wide registry. |
| Closed lists to open | `NewProvider` and `NewIdentity` fall through to a closed `switch config.Kind`. Providers: `aws/iam-identity-center`, `aws/saml`, `azure/cli`, `azure/device-code`, `azure/interactive`, `azure/oidc`, `github/oidc`, `atmos/pro`, `mock`, `mock/aws`. Identities: `aws/permission-set`, `aws/assume-role`, `aws/assume-root`, `ambient`, `aws/ambient`, `aws/user`, `aws/credential-process`, `atmos/pro`, `azure/subscription`, `azure/pim-role`, `mock`, `mock/aws`. The [keyring store](../../../../pkg/auth/credentials/keyring_store.go) decodes credentials through a second closed `switch` over the tags `aws`, `gcp`, `oidc`, `atmos-pro`, and `mock`; Azure is absent. |
| Boundary leaks | `Provider.PreAuthenticate(manager AuthManager)` and `PostAuthenticateParams.Manager` hand internal managers to the implementation. `ConfigSetter.SetConfig` passes the whole `*schema.Identity`. |

The `aws/v1` credential type does not exist yet. Credentials are `ICredentials` implementations inside an untyped `{type, data}` envelope, so a versioned credential-type registry (N3 in the gap analysis) is new work. The separate [auth integrations registry](../../../../pkg/auth/integrations/registry.go) (`Register` replaces by kind) is a follow-on.

**Adapter:** `pkg/plugin/adapter/auth` implements `types.Provider` and `types.Identity` and calls the `AuthProvider` and `AuthIdentity` wire services. It consumes the `api.Credentials`, `api.Prompt`, `api.Browser`, `api.HTTP`, and `api.Log` API services.

**Injection points:**
- `registry.NewProvider` and `registry.NewIdentity`: registry-only lookups; bootstrap registers constructors for verified plugin provider/identity kinds through the same public registration API as built-ins. Unknown kinds remain errors.
- Keyring encode/decode in `pkg/auth/credentials`: resolve through the credential-type registry instead of the tag `switch`.
- `schema.Provider` and `schema.Identity` kind validation: read registered plugin metadata.
- Chain validation: check declared accepted and produced credential types.
- `atmos auth list` and `atmos auth whoami`: display plugin kinds.

**Prerequisites:** P0.1 Auth registry completion; P0.2 Credential-type registry and versioned envelope; Phase 3 work package WP3.1 (auth). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `types.AuthManager`: replaced by the `api.Credentials` and `api.Prompt` services.
- `*schema.Identity` and `*schema.Provider`: replaced by validated `spec`/`principal` maps plus identity name and realm.
- `*schema.AuthContext`: replaced by typed credential envelopes resolved through `api.Credentials`.

## Acceptance

The first-release gate is the [Go OIDC pilot](../oidc-pilot.md), including browser login with PKCE, validated tokens, plugin-to-built-in identity chaining, cross-process reuse, refresh, and logout. P0.1 and P0.2 are required foundations.

Run the Go contract fixtures first; add identical applicable Starlark fixtures when that runtime ships. Both cover successful chains, plugin-to-built-in handoff, invalid inputs, expiry, reuse, refresh, logout, separate realms, cancellation, noninteractive failure, and secret redaction. An enterprise publishes privately, installs precompiled artifacts, and uses its Go OIDC provider without changing Atmos or installing a compiler.
