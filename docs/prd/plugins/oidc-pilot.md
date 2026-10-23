# Go OIDC authentication pilot

**Status:** Native Go pilot implemented and locally validated; live customer integration remains unverified. Required for the [first release](first-release.md). [Authentication contract](extensions/authentication.md) · [Testing](testing.md).

The pilot proves a customer can publish an independently built Go plugin and authenticate through normal Atmos commands. It must perform browser login with PKCE, validate tokens, exchange the appropriate token for downstream credentials, reuse cached credentials across Atmos processes, refresh them, and log out. A fixture supplied with an existing bearer token does not satisfy this gate.

## Reproducible pilot workflow

Start with `atmos plugin create`, then use `plugin build` and `plugin test` against the generated package. Pack and publish its precompiled release to the OCI emulator, discover it through the catalog, install it from a versioned declaration, and run both its executable command and OIDC conformance through the downloaded binary. The lifecycle cast (`website/static/casts/examples/native-auth-plugins/lifecycle.cast`; added with the CLI release layer) and changelog (`website/blog/2026-10-19-native-auth-plugins.mdx`; added with the CLI release layer) demonstrate that local flow. Regenerate it through `atmos --chdir=demo/casts casts generate examples native-auth-plugins lifecycle`.

[Plugin admission policy](policy.md) applies before author testing, publication, installation, and activation. A company-registry policy requires explicit trusted development/layout exceptions where those workflows are needed. The default local demo is not evidence of production issuer approval or completed live customer sign-off.

## Ownership and implementation

The Go plugin owns issuer discovery, OIDC/OAuth protocol behavior, state and nonce generation/validation, PKCE, ID-token verification, provider-specific exchange, and remote revocation where supported. It uses maintained Go libraries such as [go-oidc](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc) and [oauth2](https://pkg.go.dev/golang.org/x/oauth2). A general built-in OIDC provider or Starlark OIDC service is not a prerequisite.

Atmos owns provider/identity registration, invocation lifetime, browser launch and callback transport, shared HTTP configuration, credential storage/isolation, chain orchestration, masking, and command output. The pilot requires P0.1 (auth constructor registry) and P0.2 (credential-type registry), plus the native runtime, API broker, Go SDK, and contract runner.

Browser authorization uses the external browser and authorization-code flow with S256 PKCE and a loopback redirect. The provider generates fresh state, nonce, and verifier values for each login. ID-token acceptance validates signature/key selection, issuer, audience/authorized party, expiration, and nonce as applicable. These requirements follow [RFC 8252](https://www.rfc-editor.org/rfc/rfc8252.html) and [OIDC Core](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation).

## Required flow

| Stage | Acceptance evidence |
|---|---|
| Author and distribute | Generate the Go starter through existing scaffolding, complete its configuration, build/test its declared platforms, publish privately, and install on a stock Atmos binary without a consumer-side compiler. |
| Login | Resolve the configured issuer; bind the callback listener before opening the authorization URL; exchange the returned code using the original verifier and exact redirect URI; verify the ID token. |
| Downstream exchange | The provider returns `oidc/v1`; a plugin identity accepts it and exchanges the configured token with a mock corporate broker for `aws/v1`. A built-in AWS identity then consumes those credentials against a mock cloud endpoint, proving both registration paths. |
| Reuse | A fresh Atmos process reuses still-valid credentials without browser interaction, token exchange, or broker calls. |
| Refresh | Advance test time to expiry. Refresh the OIDC session, persist any rotated refresh token, renew downstream credentials, and preserve chain behavior without another browser login. |
| Logout | Invoke provider revocation where supported, remove plugin session state and Atmos-managed dependent credentials, and prove a fresh process cannot reuse them. Remote revocation failure is reported while local cleanup still completes. Global IdP browser-session termination is not promised. |

The example documents the exact downstream token contract. ID tokens and access tokens are distinct; an ID token is sent to the broker only when that broker explicitly accepts an ID token for the configured audience. Refresh tokens never enter downstream identity inputs.

## Browser callback contract

`Browser.Listen` takes a callback path and the expected state. Its stream first emits `Ready` with the actual loopback redirect URI, then at most one matching callback containing code/state or the provider error. The SDK returns a listener only after `Ready`, so the plugin can construct the authorization URL before calling `Browser.Open`.

The listener binds only loopback on an OS-assigned port. Wrong paths, wrong/missing state, and unrelated requests do not consume the valid callback. The plugin also validates correlation before exchanging a code. Callback values and token-bearing URLs never enter logs. Closing the SDK listener cancels its stream; completion, cancellation, deadline, or plugin exit releases the socket. The browser receives a generic response without echoed secrets. Complete and flush that response before delivering the callback to the consumer, because the consumer may immediately close the listener; a successful callback must not truncate the browser response. The pilot uses query-mode callbacks; other response modes require an explicit contract addition.

Fresh noninteractive login returns `INTERACTION_REQUIRED` before opening a browser or listener. Reuse and refresh can succeed noninteractively when valid cached state permits them.

## Credential and session contracts

`oidc/v1` uses the common credential envelope with this `data` schema:

| Field | Requirement |
|---|---|
| `issuer`, `subject`, `client_id` | Required verified issuer/subject and configured OAuth client identity. |
| `scopes` | Granted scopes as a list of strings. |
| `access_token`, `token_type`, `access_token_expires_at` | Required for the pilot's broker exchange; expiration is an RFC 3339 timestamp derived from the issuer response. |
| `id_token`, `id_token_expires_at` | Required for the pilot's initial authenticated result; verified raw token and its expiration. A refresh response may omit a new ID token; the plugin must not invent or extend the old token's validity. |

The envelope's `expires_at` is the earliest expiration of tokens required by the configured downstream exchange. An identity requiring an expired ID token cannot consume a still-valid access token as a substitute. A provider without a usable token expiration must define a supported lifetime policy before claiming the pilot contract.

Core token fields are sensitive by schema; omission from `sensitive_keys` cannot make them printable. `whoami` exposes only allowed identity metadata. The provider stores refresh tokens and any additional private state in a plugin-owned versioned envelope through `Credentials.Get/Put/Delete`. Storage remains readable for refresh after an access token expires. Private state is never supplied to another plugin or identity.

Atmos scopes session state by package, provider configuration, and realm, with a configuration fingerprint including issuer, client ID, and requested scopes. The provider separates subjects within that scope. Atmos serializes authentication/refresh/logout for a session across processes, re-reads stored state after acquiring the lock, and atomically persists each replacement. Locks are released on crash/cancellation. A caller waiting behind refresh reuses the new result. Logout prevents an in-flight refresh from resurrecting deleted credentials.

If refresh rotates the token, persist the replacement before publishing renewed downstream credentials. If the response omits a refresh token, retain the previous valid token. On confirmed invalid/revoked refresh credentials, clear the unusable state and require fresh login. Network failures and ambiguous responses do not trigger blind refresh retries: a crash after remote rotation but before local persistence may require reauthentication, and must not be reported as recoverable success.

## Standard Go HTTP integration

The Go SDK supplies an `http.RoundTripper` backed by `api.HTTP`. Both OIDC discovery/JWKS and OAuth token/refresh calls use an injected `http.Client` with that transport. Preserve request context, status codes, headers, form-encoded bodies, response bodies, and errors under the common size limits. Atmos applies proxy, TLS/CA, deadline, and masking policy; it does not inject unrelated credentials.

Each transport call performs one HTTP exchange and returns redirects to `http.Client`. The SDK's client applies bounded redirects, strips sensitive headers when crossing origins, and refuses HTTPS downgrades. Neither layer automatically retries code exchange, rotating refresh, or revocation. Tests cover cancellation, custom CA/proxy configuration, redirects, and unexpected requests. The SDK does not add a second OAuth implementation.

## Conformance and pilot sign-off

The default suite uses a local mock issuer with discovery, signed tokens, JWKS, and refresh/revocation endpoints; a broker; a cloud endpoint; an isolated keyring; and controlled time. A fake browser follows the authorization URL through a real loopback callback to exercise readiness and ordering. Tests invoke the actual native plugin through Atmos, including fresh processes for persistence and concurrent processes for refresh coordination. No developer credentials or live identity are used.

Required negative cases include wrong issuer/audience/nonce/signature, expired tokens, JWKS rotation, wrong state followed by a valid callback, reused codes, PKCE mismatch, user denial, malformed exchange results, revoked refresh tokens, rotated-token persistence, concurrent refresh/logout, failures before and after remote rotation, cancellation, noninteractive login, realm/provider isolation, and redaction of tokens/codes/verifiers in every output channel. Failed validation must prevent downstream exchange.

The customer pilot also runs an explicit integration walkthrough against its configured issuer and broker with dedicated test inputs. Record supported issuer settings and downstream token expectations. Mock conformance and live integration results are reported separately; missing inputs mean the live gate is incomplete. Starlark parity is a follow-on and does not gate this pilot.

## Implementation evidence

The initial implementation is kept in one PR for validation before separating the foundation changes. `cmd/plugin_oidc_test.go` generates the project with `plugin create`, compiles its executable with `plugin build`, runs `plugin test`, runs it from a profile that sets `path`, invokes its command, and drives login/reuse/refresh/logout through fresh CLI processes. Three concurrent CLI logins after expiration share one refresh. All credentials and issuer/broker endpoints in this test are isolated fixtures.

`pkg/plugin/scaffold/oidc_integration_test.go` exercises the generated native provider, downstream broker, and built-in AWS identity with local endpoints, including negative ID-token cases. The browser API, private session cleanup, transport, transaction lock, and credential envelope checks have additional focused tests. Static credential contracts reject incompatible declared plugin chains before authenticating; the built-in AWS assume-role identity exposes its AWS contract, while other legacy built-ins retain their existing validation.

These results establish a working generated example. They do not constitute a live customer-issuer sign-off or completion of every negative case in the broader first-release acceptance matrix above. Additional adapter coverage and production release gates remain in the first-release PRD.
