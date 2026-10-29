# Plugin platform errors

**Status:** Proposed; part of the [plugin platform](../overview.md). [Platform workstream](README.md).

This document defines how plugin failures reach the user: the implemented sentinels in [`errors/plugin.go`](../../../../errors/plugin.go) and proposed additions, the wire `ErrorClass` that plugins return, how output is masked, and the truthful-failure rules each extension already states. It follows the [error handling guide](../../../errors.md), the [exit code strategy](../../exit-codes.md), and the [sentinel conventions](../../error-types-and-sentinels.md). No new exit codes are introduced.

## Sentinel errors

Implemented plugin sentinels are static `errors.New` values in `errors/plugin.go`; the remaining rows describe proposed additions. Sentinel errors are wrapped with `fmt.Errorf("%w: ...")` or the error builder, and tested with `errors.Is`. The Message column matches the declared text for implemented sentinels; proposed rows specify their intended text. Each hint is one actionable line because every `WithHint()` renders its own lightbulb.

Exit codes reuse the existing conventions: **1** system, external-service, or integrity failure; **2** usage, configuration, or policy violation; **126** cannot execute; **127** not found; **130** SIGINT. `atmos plugin run` propagates a command's exit status as the Atmos exit code. RPC transport or protocol failures use Atmos error mapping instead of the plugin process's exit status. Component engines use their declared status mapping, which can treat particular nonzero statuses as success.

| Sentinel | Message | Hint | Exit | Raised by |
|---|---|---|---|---|
| `ErrPluginNotInstalled` | plugin is not installed; run atmos plugin install | Run `atmos plugin install --frozen` to restore it from the lockfile. | 2 | `pkg/plugin/registry` |
| `ErrPluginNotFound` | plugin not found | Check the source and registry, or run `atmos plugin search <name>`. | 2 (127 from `atmos plugin run <alias>`) | `pkg/plugin/catalog`, `cmd/plugin` |
| `ErrPluginIncompatible` | plugin is not compatible with this Atmos | Install a release whose `compatibility.atmos` and `contracts` include this Atmos version, or upgrade Atmos. | 2 | `pkg/plugin/install` |
| `ErrPluginUnsupportedPlatform` | plugin release does not support this platform | Run `atmos plugin info <name>` to list the published platforms. | 126 | `pkg/plugin/install` |
| `ErrPluginDigestMismatch` | plugin artifact digest does not match | Re-run the install; if it persists, the registry content changed and the release must be re-verified. | 1 | `pkg/plugin/verify` |
| `ErrPluginSignatureInvalid` | plugin signature does not satisfy publisher policy | Install from a trusted publisher or review `plugins.policy.require_signature`. | 2 | `pkg/plugin/verify` |
| `ErrPluginCollision` | plugin registration collides with an existing one | Rename the alias or remove the conflicting package; run `atmos plugin list` to see both. | 2 | `pkg/plugin/registry` |
| `ErrPluginLockMismatch` | plugin lockfile does not match configuration | Run `atmos plugin install` to refresh the lock, then commit `.atmos.plugins.lock.yaml`. | 2 | `pkg/plugin/lock` |
| `ErrPluginFrozenDrift` | plugin declarations or artifacts differ from the release lock | Run `atmos plugin install` locally, commit the updated lockfile, and retry with `--frozen`. | 2 | `pkg/plugin/install` |
| `ErrPluginPathPackage` | plugin package runs from a local path set in configuration | Edit the profile or `atmos.yaml` that sets `plugins.packages.<alias>.path`. A frozen install reports this condition as `ErrPluginFrozenDrift`. | 2 | `pkg/plugin/manage` |
| `ErrPluginProtocol` | plugin protocol error | Update the plugin or run it with `--debug` and report the invocation id to the publisher. | 1 | `pkg/plugin/runtime` |
| `ErrPluginCrashed` | plugin crashed or failed internally | Check the plugin log under the Atmos cache directory or re-run with `--debug`. | 1 | `pkg/plugin/runtime` |
| `ErrPluginTimeout` | plugin call exceeded its deadline | Run the command again; raise its timeout only if the plugin legitimately needs more time. | 1 | `pkg/plugin/runtime/native` |
| `ErrPluginUnsupportedOperation` | plugin does not support this operation | Choose a plugin or configuration that declares the operation in its `capabilities`. | 2 | `pkg/plugin/bridge` |
| `ErrPluginInteractionRequired` | plugin requires interactive authentication | Run in a terminal, or supply the value through configuration or environment. | 2 | `pkg/plugin/bridge` |
| `ErrPluginAuthCycle` | plugin identity chain contains a cycle | Remove the circular `via` reference between the listed identities. | 2 | `pkg/plugin/bridge` |
| `ErrPluginRegistryAuth` | registry authentication failed | Run `atmos auth login` or `docker login <registry>` and retry. | 1 | `pkg/plugin/oci` |
| `ErrPluginImmutableRelease` | plugin version already exists with different bytes; publish a new version | Publish under a new version; existing versions cannot be changed. | 1 | `pkg/plugin/catalog` |
| `ErrPluginCatalogConflict` | plugin catalog changed; reload it before publishing | Hold the catalog publisher lock, reload the current catalog, and retry the explicit catalog operation. | 1 | `pkg/plugin/catalog` |
| `ErrPluginPartialOperation` | plugin operation partially completed | Inspect the reported completed and failed items before retrying; the operation is not rolled back. | 1 | `pkg/plugin/bridge` |
| `ErrPluginManifestInvalid` | plugin manifest is invalid | Run `atmos plugin validate .` and fix the reported fields. | 2 | `pkg/plugin/manifest` |
| `ErrPluginSchemaViolation` | plugin configuration violates its schema | Fix the reported path against the schema from `atmos plugin info <name>`. | 2 | `pkg/plugin/bridge` |

Package names are proposed under `pkg/plugin` and follow the [build inventory](gap-analysis.md#registries-to-create). Cancellation reuses `context.Canceled`; it exits 130 when a signal caused it and 1 otherwise.

The target diagnostic context for a failure from a running plugin invocation contains `plugin`, `version`, `extension`, `operation`, and `invocation_id`, so support can correlate it with the plugin's own log. Current runtime paths do not yet attach these fields uniformly. Pre-invocation failures, including install, lock, manifest, registry, and policy errors, include plugin and version context where known; extension, operation, and invocation ID are optional when no invocation exists. The following builder example illustrates the runtime diagnostic contract:

```go
return errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrPluginCrashed, cause)).
    WithExplanation("The plugin process exited before returning a result").
    WithHint("Check the plugin log under the Atmos cache directory or re-run with `--debug`").
    WithContext("plugin", pkg.Alias).
    WithContext("version", pkg.Version).
    WithContext("extension", "auth.provider").
    WithContext("operation", "Authenticate").
    WithContext("invocation_id", inv.ID).
    WithExitCode(1).
    Err()
```

## Wire error classes

The table below is the proposed `ErrorClass` detail contract; the current protobuf does not define this enum. Current auth adapters translate gRPC statuses directly, and the native runtime rejects malformed protocol events with `ErrPluginProtocol`. Under the proposed contract, a plugin returns an `ErrorClass` in its error detail; Atmos owns mapping it to a sentinel and a user-facing behavior. The class describes the condition, never the exit code. Where the class is domain-specific (an authentication failure, a missing store key), Atmos wraps the existing domain sentinel rather than inventing a plugin one, and adds the plugin context fields.

| `ErrorClass` | gRPC status | Atmos sentinel | User-facing behavior |
|---|---|---|---|
| `ERROR_CLASS_UNSPECIFIED` | `UNKNOWN` | `ErrPluginProtocol` | Fail the command. A plugin that omits the class is non-conforming. |
| `INVALID_CONFIG` | `INVALID_ARGUMENT` | `ErrPluginSchemaViolation` | Fail the command; hint names the configuration path from the detail. |
| `UNSUPPORTED` | `UNIMPLEMENTED` | `ErrPluginUnsupportedOperation` | Fail when the operation is required; warn and skip only when the extension PRD marks it optional. |
| `UNAUTHORIZED` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Existing domain sentinel (for example `ErrAuthenticationFailed`) | Fail the command; never retry with other credentials. |
| `NOT_FOUND` | `NOT_FOUND` | Existing domain sentinel | Fail, or return the extension's explicit "missing" result where the PRD defines one. |
| `UNAVAILABLE` | `UNAVAILABLE` | Existing domain sentinel | Fail with retry guidance. Read-only operations may be retried by the caller; state-changing operations are not. |
| `CONFLICT` | `ABORTED`, `ALREADY_EXISTS` | Existing domain sentinel, or `ErrPluginCatalogConflict` for catalogs | Fail; hint explains how to reload and retry. |
| `CANCELLED` | `CANCELLED` | `context.Canceled` | Stop and report cancelled. |
| `INTERACTION_REQUIRED` | `FAILED_PRECONDITION` | `ErrPluginInteractionRequired` | Prompt through `api.Prompt` when a TTY is available; otherwise fail with the missing input named. |
| `INTERNAL` | `INTERNAL` | `ErrPluginCrashed` | Fail the command; include the plugin's message after masking. |

Atmos-detected conditions during RPC extension invocations map without a plugin-supplied class:

- Process exit or transport loss maps to the proposed `ErrPluginCrashed`.
- A deadline exceeded maps to `ErrPluginTimeout`, as implemented by the native runtime.
- Malformed messages, unknown enum values, or an unsupported protocol version map to `ErrPluginProtocol`.
- A missing `OperationOutcome` maps to `ErrPluginProtocol` only when the extension's response contract requires it.

[Native commands](../extensions/commands.md) use the [`Commands.Run`](protocol.md#commands) RPC to stream stdout, stderr, and exactly one terminal `CommandExit`, followed by EOF. `atmos plugin run` propagates the reported exit code as the Atmos exit code; commands do not return `OperationOutcome`. A valid terminal exit, including a nonzero code, is not a protocol violation. Process or transport termination without that event is a failure, not successful command completion. The current native command receiver reports malformed or incomplete streams as `ErrPluginProtocol`.

Rules:

- A crash, timeout, or protocol violation is never reported as success, an empty result, or "nothing to do".
- Cancellation is never success. Work already completed before cancellation is reported through `ErrPluginPartialOperation` when the operation is state-changing.
- Atmos does not retry state-changing operations automatically. Retry is an explicit user, workflow, or extension-declared policy; read-only operations declared idempotent in the manifest may be retried by the caller.
- Under the proposed detail contract, an unknown `ErrorClass` from a newer plugin maps to `ErrPluginProtocol`, consistently with other unknown enum values. Fail explicitly and advise upgrading Atmos or selecting a compatible plugin; do not reinterpret an unknown condition as `INTERNAL` or success.

## OIDC pilot failures

The [OIDC pilot](../oidc-pilot.md) rejects bad token validation before any downstream exchange. Fresh login without interaction support returns `INTERACTION_REQUIRED`. Invalid or revoked refresh state requires reauthentication; transport failure does not authorize blind retries of a rotating refresh. If rotation succeeds remotely but persistence fails locally, report the failure and require login when recovery is impossible. Logout reports remote revocation failures while still clearing local private and dependent credentials. Cancellation closes callback listeners and releases session locks.

OIDC token fields are sensitive by schema even if omitted from `sensitive_keys`. Never print authorization codes, verifiers, refresh tokens, raw callback URLs, or raw token endpoint responses.

## Diagnostics and masking

The following are target requirements. Current plugin command output uses caller-provided writers, so masking depends on the caller. Credential envelopes preserve `sensitive_keys`, but the boundary does not yet register those values with the masker.

- Plugin stderr and `api.Log` records MUST pass through the existing secret masker in [`pkg/io`](../../../../pkg/io) before reaching a terminal, a log file, a CI summary, or diagnostics events. Masking MUST apply to the plugin's own error message and details, not only to Atmos-authored text.
- Credential envelopes MUST declare `sensitive_keys`; values under those keys MUST be registered with the masker when the envelope crosses the boundary and MUST NOT appear in `message`, hints, `details`, or `OperationOutcome` fields.
- Plugins cannot disable masking. `--mask=false` applies to Atmos process and is not forwarded.
- Runtime invocation diagnostics follow the context contract above (see the builder snippet). Pre-invocation diagnostics include only context available at the failing stage; they do not invent an invocation ID.
- A hint never embeds a secret or a token-bearing URL; registry references are printed without credentials.

## Per-extension failure semantics

Each rule restates a requirement in the extension PRD; this table is the Atmos-side error mapping, not new behavior.

| Extension | Truthful-failure rule | Surfaces as |
|---|---|---|
| [Authentication](../extensions/authentication.md) | Unsupported credential combinations fail before an operation; non-interactive prompts fail instead of hanging. | `ErrPluginUnsupportedOperation`, `ErrPluginInteractionRequired`, `ErrPluginAuthCycle` |
| [Component engines](../extensions/component-engines.md) | The engine declares which statuses are success (for example "changes detected"); Atmos does not assume every nonzero exit is failure. | `OperationOutcome` status, else `ErrPluginCrashed` |
| [CI providers](../extensions/ci-providers.md) | A required reporting operation that is unsupported fails; a reporting failure never retries or alters the infrastructure outcome. | `ErrPluginUnsupportedOperation`; original outcome kept |
| [Commands](../extensions/commands.md) | A shortcut that collides is unavailable and diagnosed; `atmos plugin run` still works. | `ErrPluginCollision` |
| [Hooks](../extensions/hooks.md) | A required before-hook failure blocks execution; an after-hook failure keeps the original operation outcome in diagnostics. | Hook failure policy; `ErrPluginCrashed` |
| [Steps](../extensions/steps.md) | A step failure follows the step's `on_failure`; an unknown namespaced type fails static validation. | `ErrPluginManifestInvalid`, `ErrPluginSchemaViolation` |
| [Validation](../extensions/validation.md) | A missing validator, crash, timeout, or malformed result is an execution error, never an empty report. | `ErrPluginCrashed`, `ErrPluginTimeout`, `ErrPluginProtocol` |
| [Stores](../extensions/stores.md) | An unsupported operation is not an empty value; null and missing are distinct. | `ErrPluginUnsupportedOperation`; `NOT_FOUND` |
| [Secret providers](../extensions/secret-providers.md) | Missing, denied, unavailable, and unsupported are distinct; a write timeout is never silently retried or claimed rolled back. | `NOT_FOUND`, `UNAUTHORIZED`, `UNAVAILABLE`, `ErrPluginUnsupportedOperation`, `ErrPluginPartialOperation` |
| [Backend provisioning](../extensions/backend-provisioning.md) | Unknown existence is not absence; failure before init blocks dependent initialization. | `UNAVAILABLE`; `ErrPluginCrashed` |
| [MCP tools](../extensions/mcp-tools.md) | Policy denial is `UNAUTHORIZED` and applies at invocation; malformed schemas or results are rejected. | `UNAUTHORIZED`, `ErrPluginProtocol` |
| [Skills](../extensions/skills.md) | Data-only; no execution, so failures are manifest or digest errors at install or load. | `ErrPluginManifestInvalid`, `ErrPluginDigestMismatch` |

## Rendered example

A plugin that exits mid-call produces one error with its context and a single hint. The text below is illustrative; layout follows the existing error renderer.

```text
Error: plugin crashed or failed internally: process exited with status 2

The plugin process exited before returning a result.

hint: Check the plugin log under the Atmos cache directory or re-run with `--debug`.

plugin:        corporate
version:       1.2.0
extension:     auth.provider
operation:     Authenticate
invocation_id: 01J9Z3K7Q2
```

A command that was partly applied adds the completed and failed items to the context and uses `ErrPluginPartialOperation`; it never reports success.

## Testing requirements

- Every sentinel has a test that triggers it and asserts `errors.Is`, never string matching.
- Every wire `ErrorClass` has a table-driven Atmos-side test that asserts the sentinel, exit code, and whether the command fails or warns.
- Negative paths: a successful call with empty output does not raise `ErrPluginCrashed`; a read-only `UNAVAILABLE` failure is retried only where the manifest declares idempotence, and a state-changing one never is.
- Masking tests inject a known secret into plugin stderr, `api.Log`, an error message, and a credential `sensitive_keys` entry, and assert it is absent from terminal output, diagnostics events, and CI summaries.
- Conformance fixtures include a plugin that crashes, hangs past its deadline, returns an unspecified class, and returns a malformed outcome; each must fail the command ([testing](../testing.md)).
