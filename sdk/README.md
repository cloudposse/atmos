# Atmos Plugin SDK for Go

This standalone module serves native Go plugins over the versioned Atmos plugin protocol. In Atmos releases that include the `atmos plugin` commands, generate a complete provider with `atmos plugin create` rather than implementing the transport by hand.

`plugin.Main` registers an `Info` service and the supplied auth provider, auth identity, and command implementations. Atmos validates the binary's package name, version, manifest digest, exports, and service versions before connecting its API broker. Stdout belongs to the transport handshake. Commands send stdout/stderr events followed by exactly one exit event (0–255); output chunks are limited to 64 KiB.

`api.Client` supplies invocation-scoped HTTP, browser, secret-read, and opaque private credential APIs. Attach the request's invocation ID with `api.WithInvocation`. The `HTTPClient` is a standard Go `http.Client`; inject it with `oidc.ClientContext` for OIDC/OAuth libraries. Requests retain their cancellation context, redirects are handled by the client, and credentials are stripped on cross-origin redirects. Token exchanges and refreshes are not retried automatically.

Browser listeners return only after Atmos has bound the loopback port. Build the authorization URL using `RedirectURI`, open it, receive one correlated callback, and close the listener. Only interactive invocations can open a browser. Private credentials are scoped by Atmos to the package, auth configuration, and realm. Refresh tokens belong in private storage, not the credential envelope returned to downstream identities.

The SDK does not import the Atmos CLI module. Protocol sources live in `../proto`; run `atmos dev generate proto` from the repository root to regenerate both SDK and CLI bindings. The command installs pinned protoc and gofumpt versions through the toolchain, and builds pinned Go generators in a temporary directory. The root protocol parity test checks that both copies implement the same contract. Do not link the SDK and CLI generated packages into one binary: they intentionally describe the same protobuf names.

The SDK release tag is `sdk/vX.Y.Z` for Atmos `vX.Y.Z`. During unreleased development, generated plugins can use `--sdk-path /absolute/path/to/sdk` to create a Go workspace. Released source should use the matching module version instead.

`ReadSecret(ctx, name)` calls `atmos.api.v1.Secrets.Read` for a logical credential name granted by the host. The `auth-secrets` provider receives grants from its configured `spec.credentials`; identity and command invocations have no secret-read grant. The host resolves store references and masks returned text. This API is separate from private session `Get`/`Put`/`Delete` and cannot enumerate stores or accept arbitrary backend paths. See `../plugins/auth-secrets` for a complete native provider and environment identity.

Official monorepo packages under `../plugins` are an exception to external SDK version pinning: their module replacement selects this SDK at the exact recorded source commit, with shared inputs declared in the release inventory. Consumers download precompiled OCI artifacts and never resolve that local replacement.

## Component configuration contracts (in development)

`plugin.Services` accepts optional `Discovery`, `Validation`, `Preparation`, and `State` handlers. Negotiate these separately as `component.discovery`, `component.validation`, `component.preparation`, and `component.state`, each at version 1. A configuration provider does not need to supply an executor. The host runtime has matching clients; installed component activation and stack integration are not available yet.

Use `component.Encode` and `component.Decode` for infrastructure values. They retain exact cty numbers, types, nulls, unknowns, and nested sensitivity. Translate evaluator-specific sensitivity to `component.Sensitive` before encoding. Other marks are rejected. The payload and its sensitivity paths can contain secrets; never log them. Invalid wire values produce a payload-free `component.ErrInvalidValue`.

Discovery supplies logical coordinates, file/directory/glob provenance, and separate ordering/output dependencies. Preparation supplies fresh inputs and literal file bytes for the selected operation. State resolution returns backend/workspace identity independently. Effect policy defaults to denial and must be enforced by the evaluator and host grants; native code is not sandboxed. See `../docs/prd/plugins/extensions/component-engines.md` for remaining implementation requirements.
