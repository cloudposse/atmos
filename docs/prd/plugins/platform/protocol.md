# Plugin wire protocol

**Status:** Proposed; part of the [plugin platform](../overview.md). Protocol, messages, and bindings below are proposed APIs. [Platform workstream](README.md).

This document defines how Atmos talks to native plugins, how Starlark plugins see the same contracts, and what a Wasm runtime would have to preserve. The product behavior of each extension lives in the [extension PRDs](../extensions/README.md); the Go interfaces being adapted are inventoried in the [gap analysis](gap-analysis.md). Internal Go structs, managers, and callbacks never cross the boundary (see [types that must not cross](gap-analysis.md#internal-types-that-must-not-cross-the-boundary)).

| Decision | Choice |
|---|---|
| Transport | `hashicorp/go-plugin`, gRPC mode only; no `net/rpc`. |
| Ownership | Atmos owns the `.proto`: packages `atmos.plugin.v1` and `atmos.api.v1`, files under `proto/atmos/plugin/v1/` and `proto/atmos/api/v1/`, generated Go in both `pkg/plugin/proto` (CLI) and `sdk/proto` (SDK). The Go SDK wraps its own copy; neither module imports the other. |
| Commands | Plain executable dispatch; no RPC. |
| Starlark | Follow-on runtime: in-process, same contracts and field names. |
| Wasm | Extism runtime is a declared follow-on, not initial. |
| Contract testing | `buf breaking` plus the Atmos-shipped conformance suite; not Pact. See [Decision 15](decisions.md). |

## Transport

### Client configuration

Atmos builds one `plugin.Client` per package, in `pkg/plugin/runtime`.

| `ClientConfig` field | Value | Reason |
|---|---|---|
| `HandshakeConfig` | `{ProtocolVersion: 1, MagicCookieKey: "ATMOS_PLUGIN_MAGIC_COOKIE", MagicCookieValue: <fixed constant>}` | Stops a plugin binary from being run by hand. The cookie is not a secret. |
| `VersionedPlugins` | `map[int]plugin.PluginSet`, keyed by protocol major | Lets one Atmos build speak several majors; see [Versioning](#versioning). |
| `AllowedProtocols` | `[]plugin.Protocol{plugin.ProtocolGRPC}` | A plugin that announces `netrpc` fails with `ErrPluginProtocol`. |
| `AutoMTLS` | `true` | Ephemeral certificates per process, even on loopback. |
| `Managed` | `true` | Registers the client for `plugin.CleanupClients`, so Atmos exit kills every plugin. |
| `SkipHostEnv` | `true` | The plugin gets only the environment in [Process environment](#process-environment). |
| `SyncStdout`, `SyncStderr` | Masking writers | Plugin stdout is not a data channel. Both streams pass through Atmos masker, then `ui` on stderr. |
| `Logger` | `hclog` adapter into Atmos logging | Structured plugin logs keep their level; secrets are masked before emission. |
| `StartTimeout` | 30 seconds, configurable | A plugin that never prints the handshake line fails with `ErrPluginTimeout` and `ErrPluginProtocol`. |
| `GRPCDialOptions` | 16 MiB send and receive limit; keepalive pings | Larger payloads use `OutputChunk` streams or `ArtifactRef` files, never a single message. |

The plugin side calls `plugin.Serve` with the same `VersionedPlugins`; the SDK hides it.

### Process lifecycle

| Stage | Behavior |
|---|---|
| Start | Lazy, on the first RPC for the package. Discovery, help, `plugin validate`, `plugin list`, MCP tool listing, and configuration validation from manifest schemas never start a process. A project with no plugin extension in play launches nothing. |
| Initialize | `Info.Describe`, then `Info.Configure`. Atmos checks the echoed exports against the verified manifest. A mismatch or undeclared export is `ErrPluginProtocol`. |
| Reuse | One process per package per Atmos process. Concurrent invocations multiplex over one connection, so plugins must be safe for concurrent RPCs. No state survives between invocations unless the plugin keeps it in memory; Atmos never relies on that. |
| Health | go-plugin registers `grpc.health.v1`. Atmos pings before reuse after an idle period and after any transport error. A failed ping marks the process dead; the next invocation starts a fresh one. In-flight invocations on the dead process fail with `ErrPluginCrashed`, never a silent retry. |
| Cancel | Cancelling an invocation cancels its gRPC context. The plugin has a grace period (default 5 seconds) to return `CANCELLED`; then Atmos kills the process. |
| Shutdown | `Managed` cleanup on exit. go-plugin plugins ignore `SIGINT`, so Atmos alone owns shutdown. Processes started through `api.Process` are children of Atmos, so cancellation and exit kill them deterministically. |

Native code is trusted code. Process separation gives crash isolation and a narrow API, not an operating-system sandbox. go-plugin uses Unix domain sockets where available and TCP on `127.0.0.1` with an ephemeral port on Windows, protected by `AutoMTLS`; named pipes are an [open question](decisions.md#open-questions).

### Process environment

Atmos constructs `cmd.Env` from scratch:

| Included | Excluded |
|---|---|
| Minimal base: `PATH` (scoped by [`api.Toolchain`](#apitoolchain) when the manifest declares `dependencies.tools`), temp directory, `SYSTEMROOT` on Windows, locale. | Everything else inherited from the Atmos process, including `ATMOS_*`, cloud credentials, and CI tokens. |
| The go-plugin variables and `ATMOS_PLUGIN_MAGIC_COOKIE`. | Identity environments, which arrive per invocation in `Invocation.environment` and are scoped to that call. |
| Names the manifest declares under `spec.runtime.environment`, resolved from the Atmos environment ([configuration](configuration.md#plugin-manifest-atmospluginconfig)). | Secret values, which arrive through [`api.Secrets`](#apisecrets) on demand. |

Values Atmos resolves are registered with the masker before they reach the plugin.
### API services over the broker

API services run on go-plugin's `GRPCBroker`. After the handshake Atmos serves each `atmos.api.v1` service on a broker stream and sends the IDs in `Info.Configure.broker_ids` (service name to broker ID). The plugin dials them with `broker.Dial`. Every API RPC carries the invocation ID in gRPC metadata (`atmos-invocation-id`, set by the SDK). Atmos rejects calls whose invocation is not in flight and applies that invocation's grants, so a late goroutine in a plugin cannot reuse an earlier invocation's access.

## Versioning

Four versions are independent:

| Version | Meaning | Where it lives |
|---|---|---|
| Protocol major | The `.proto` package (`atmos.plugin.v1`) and the go-plugin `ProtocolVersion`. | `VersionedPlugins` key; manifest `spec.runtime.protocol`. |
| Service contract | Integer per plugin or API service, advancing when a service gains a capability. | `Info.Describe` and manifest `spec.compatibility.contracts`, keyed by capability (`auth.provider`) or API service (`atmos.http`). |
| Manifest `apiVersion` | Document format. | `plugin.yaml` ([configuration](configuration.md)). |
| Plugin release | Plugin semver. | `metadata.version`. |

### Compatibility rules

- Within a protocol major, changes are additive only: new messages, new fields with new numbers, new RPCs, new enum values, new feature flags. Never remove, renumber, retype, or reuse a field or enum number; removed names are `reserved`.
- Receivers ignore unknown fields and treat unknown enum values as `UNSPECIFIED`. Behavior that needs a new field is gated by a feature flag or a higher service contract, not assumed.
- An incompatible change is a new package, `atmos.plugin.v2`, with a new go-plugin major. Atmos registers both in `VersionedPlugins` for as long as the deprecation matrix lists the old one.

### Negotiation

1. Static: at install and lock time, Atmos compares manifest `spec.runtime.protocol` and `spec.compatibility.contracts` with its own supported set. No process starts.
2. Process: go-plugin picks the highest protocol major both sides list.
3. Service: `Describe` returns `{service, min, max}` per exported service. Atmos selects the highest version in the intersection of the plugin's range and its own.
4. An empty intersection at any step fails with `ErrPluginIncompatible`, listing both sets:

```text
acme/auth 1.2.0 is incompatible with this Atmos binary
  plugin supports:  protocol 2;  auth.provider 3..4
  atmos supports:   protocol 1;  auth.provider 1..2
```

Starlark packages negotiate `spec.runtime.api` against Atmos's supported Starlark API set the same way; there is no process step.

### Deprecation matrix

Each Atmos release publishes a table like this one (values illustrative). The `buf breaking` gate in [Conformance](#conformance) enforces the additive rule.

| Atmos | Protocol majors | `auth.provider` | `step` | Status |
|---|---|---|---|---|
| Release N | 1 | 1 | 1 | Current |
| Release N+k | 1, 2 | 1..2, 3 | 1..2 | Major 1 deprecated; removal not before two minor releases |

## Shared messages

Defined in `common.proto`. All other files import it.

```proto
syntax = "proto3";
package atmos.plugin.v1;
import "google/protobuf/struct.proto";
import "google/protobuf/timestamp.proto";

message Invocation {
  string id = 1;                              // Stable logical ID; identical across explicit retries.
  uint32 attempt = 2;                         // 1 for the first run; Atmos increments on explicit retry.
  google.protobuf.Timestamp deadline = 3;     // Also set as the gRPC deadline.
  string working_directory = 4;
  map<string, string> environment = 5;        // Scoped to this call; masked in Atmos output.
  string stack = 6;  string component = 7;  string component_type = 8;
}

message Credentials {
  string type = 1;                            // "aws/v1", "gcp/v1", "azure/v1", "oidc/v1", "atmos-pro/v1", "acme/okta-session/v1".
  google.protobuf.Timestamp expires_at = 2;
  google.protobuf.Struct data = 3;
  repeated string sensitive_keys = 4;         // Keys in data to mask and never log.
}

enum Severity { SEVERITY_UNSPECIFIED = 0; SEVERITY_ERROR = 1; SEVERITY_WARNING = 2; SEVERITY_NOTICE = 3; }
message Diagnostic {                          // Mirrors pkg/validation.Diagnostic, plus field.
  string source = 1;  string rule_id = 2;  Severity severity = 3;  string message = 4;
  string file = 5;  int32 line = 6;  int32 column = 7;  int32 end_line = 8;  int32 end_column = 9;
  string field = 10;                          // Logical path such as vars.tags.Owner; no invented line numbers.
}
message Finding { string path = 1; int32 line = 2; string severity = 3; string rule_id = 4; string message = 5; }  // Mirrors pkg/hooks.Finding.

enum OperationStatus {                        // Short names: SUCCESS, CHANGES, NO_CHANGES, FAILED, CANCELLED, UNKNOWN.
  OPERATION_STATUS_UNSPECIFIED = 0;  OPERATION_STATUS_SUCCESS = 1;  OPERATION_STATUS_CHANGES = 2;  // CHANGES: for example plan exit code 2.
  OPERATION_STATUS_NO_CHANGES = 3;  OPERATION_STATUS_FAILED = 4;  OPERATION_STATUS_CANCELLED = 5;
  OPERATION_STATUS_UNKNOWN = 6;               // Outcome cannot be determined; never reported as success.
}
message OperationOutcome { OperationStatus status = 1; int32 exit_code = 2; repeated ArtifactRef artifacts = 3; }
message ArtifactRef { string kind = 1; string path = 2; string digest = 3; string media_type = 4; }  // Path under the working or package directory.
message OutputChunk { enum Stream { STREAM_UNSPECIFIED = 0; STREAM_STDOUT = 1; STREAM_STDERR = 2; } Stream stream = 1; bytes bytes = 2; }
message Unsupported { string operation = 1; string reason = 2; }
message Progress { string message = 1; float fraction = 2; }

enum ErrorClass {                             // Short names: INVALID_CONFIG, UNSUPPORTED, and so on.
  ERROR_CLASS_UNSPECIFIED = 0;  ERROR_CLASS_INVALID_CONFIG = 1;  ERROR_CLASS_UNSUPPORTED = 2;  ERROR_CLASS_UNAUTHORIZED = 3;
  ERROR_CLASS_NOT_FOUND = 4;  ERROR_CLASS_UNAVAILABLE = 5;  ERROR_CLASS_CONFLICT = 6;  ERROR_CLASS_CANCELLED = 7;
  ERROR_CLASS_INTERACTION_REQUIRED = 8;  ERROR_CLASS_INTERNAL = 9;
}
message Error { ErrorClass class = 1; string message = 2; string hint = 3; google.protobuf.Struct details = 4; }
message ValidateResponse { repeated Diagnostic diagnostics = 1; }   // Shared by every Validate RPC.
```

Rules:

- **Secrets never appear in `message`, `hint`, `details`, or `Diagnostic` text.** Atmos masks what it knows, but plugins must not rely on that. `Credentials.sensitive_keys` drives masking and keyring handling; keys not listed are treated as non-sensitive and may appear in `atmos auth whoami`.
- **Credential types are a registry.** `aws/v1`, `gcp/v1`, `azure/v1`, `oidc/v1`, and `atmos-pro/v1` have Atmos adapters. Plugin-namespaced types such as `acme/okta-session/v1` are opaque to Atmos, stored by Atmos keyring, and decoded only by the owning plugin. The version suffix is the plugin-owned schema version; a cached type the plugin no longer accepts triggers reauthentication, not an unsafe decode. A chain that needs a type nobody produces fails with `UNSUPPORTED`.
- **Errors travel as gRPC status with an `Error` detail**, using the code in [Error model](#error-model). Findings that are results travel in normal responses. A crash, timeout, or malformed response is never an empty successful result.
- **Optional operations return `Unsupported`** in a `oneof` beside the success payload. A plugin that does not implement the RPC at all returns gRPC `UNIMPLEMENTED`; Atmos treats both as `ErrPluginUnsupportedOperation`.

## Plugin services

Common conventions: every request carries `Invocation invocation = 1` unless noted. A server-streaming RPC ends with exactly one terminal message. Messages below are abbreviated: field numbers after the listed ones are assigned in the `.proto`, and `Struct` fields use the schema declared in the manifest.

### Info

```proto
service Info { rpc Describe(DescribeRequest) returns (DescribeResponse); rpc Configure(ConfigureRequest) returns (ConfigureResponse); }
message ServiceVersionRange { string service = 1; uint32 min = 2; uint32 max = 3; }
message Export { string service = 1; string kind = 2; }                 // For example {"AuthProvider", "acme/broker"}.
message DescribeRequest  { string atmos_version = 1; repeated ServiceVersionRange host_supported = 2; }
message DescribeResponse { string plugin_name = 1; string plugin_version = 2; string manifest_digest = 3;
                            repeated ServiceVersionRange services = 4; repeated Export exports = 5; repeated string features = 6; }
message ConfigureRequest { map<string, uint32> broker_ids = 1; google.protobuf.Struct settings = 2; string package_dir = 3; }
```

`Describe` echoes the manifest so Atmos can reject drift. `features` are optional behaviors such as `auth.refresh` or `store.list`. `Configure` hands over broker IDs, the package's `config` settings, and the package directory that [`api.FS`](#apifs) may read. This service is new; no Go interface is replaced.

### AuthProvider

The first-release provider is the [Go OIDC pilot](../oidc-pilot.md). Its callback, token/session schemas, refresh coordination, and HTTP-library integration are required parts of this protocol. Remaining extension services and Starlark bindings below are follow-on contracts unless enabled by the [first-release scope](../first-release.md).

```proto
service AuthProvider {
  rpc Validate(ProviderRequest) returns (ValidateResponse);   rpc Authenticate(ProviderRequest) returns (stream AuthenticateEvent);
  rpc Refresh(RefreshRequest) returns (RefreshResponse);      rpc Logout(ProviderRequest) returns (LogoutResponse);
  rpc Environment(ProviderRequest) returns (EnvironmentResponse);  rpc Paths(ProviderRequest) returns (PathsResponse);
  rpc PrepareEnvironment(PrepareEnvironmentRequest) returns (EnvironmentResponse);
}
message ProviderRequest { Invocation invocation = 1; string name = 2; string kind = 3; google.protobuf.Struct spec = 4; string realm = 5; }
message AuthenticateEvent { oneof event { Progress progress = 1; Interaction interaction = 2; Credentials result = 3; } }
message Interaction { string kind = 1; string title = 2; map<string, string> fields = 3; }   // Display only: device code, URL.
message RefreshRequest { ProviderRequest provider = 1; Credentials current = 2; }
message RefreshResponse { oneof result { Credentials credentials = 1; Unsupported unsupported = 2; } }
message PrepareEnvironmentRequest { ProviderRequest provider = 1; map<string, string> environment = 2; }
message PathsResponse { repeated CredentialPath paths = 1; }   // CredentialPath mirrors types.Path: location, type, required, purpose.
```

`Authenticate` streams display events (a device code, a progress line) and ends with `result`. Blocking input goes through [`api.Prompt`](#apiprompt) and [`api.Browser`](#apibrowser); the plugin calls them while the stream is open. `Refresh` is new: `Unsupported` makes Atmos reauthenticate.

| RPC | Replaces on `types.Provider` |
|---|---|
| `Validate`, `Authenticate`, `Logout`, `Environment`, `Paths`, `PrepareEnvironment` | Method of the same name. |
| `Refresh` | None; optional per [authoring](../authoring.md). |
| (none) | `Kind`, `Name`: manifest and request. `SetRealm`: `realm` field. `GetFilesDisplayPath`: `CredentialPath.purpose` and `location`. `PreAuthenticate(AuthManager)`: removed; replaced by `Configure` and `api.Credentials`. |

### AuthIdentity

```proto
service AuthIdentity {
  rpc Validate(IdentityRequest) returns (ValidateResponse);  rpc Authenticate(IdentityAuthenticateRequest) returns (stream AuthenticateEvent);
  rpc Environment(IdentityRequest) returns (EnvironmentResponse);  rpc Paths(IdentityRequest) returns (PathsResponse);
  rpc PrepareEnvironment(IdentityPrepareRequest) returns (EnvironmentResponse);  rpc Logout(IdentityRequest) returns (LogoutResponse);
}
message IdentityRequest { Invocation invocation = 1; string name = 2; string kind = 3; google.protobuf.Struct spec = 4;
                          google.protobuf.Struct principal = 5; string realm = 6; }
message IdentityAuthenticateRequest { IdentityRequest identity = 1; Credentials upstream = 2; }
```

| RPC | Replaces on `types.Identity` |
|---|---|
| `Validate`, `Authenticate(upstream)`, `Environment`, `Paths`, `PrepareEnvironment`, `Logout` | Method of the same name; `Authenticate` takes `baseCreds` as `upstream`. |
| (none) | `PostAuthenticate(PostAuthenticateParams)`: Atmos writes files from `Paths` and applies `PrepareEnvironment`; `Manager` never crosses. `CredentialsExist`, `LoadCredentials`: Atmos keyring. `GetProviderName`: manifest `via`. `SetRealm`: field. |

### Component

```proto
service Component {
  rpc Describe(ComponentDescribeRequest) returns (ComponentDescription);  rpc ListComponents(ListComponentsRequest) returns (ListComponentsResponse);
  rpc Validate(ComponentRequest) returns (ValidateResponse);  rpc GenerateArtifacts(ComponentRequest) returns (GenerateArtifactsResponse);
  rpc Execute(ComponentRequest) returns (stream ExecuteEvent);  rpc Events(ComponentRequest) returns (EventsResponse);
}
message ComponentDescription { string type = 1; string group = 2; string base_path = 3; repeated CommandInfo commands = 4; string schema = 5; }
message ComponentRequest { Invocation invocation = 1; string command = 2; string subcommand = 3; google.protobuf.Struct component_config = 4;
                            repeated string args = 5; google.protobuf.Struct flags = 6; }
message ListComponentsRequest { Invocation invocation = 1; string stack = 2; google.protobuf.Struct stack_config = 3; }
message ExecuteEvent { oneof event { OutputChunk chunk = 1; LifecycleEvent lifecycle = 2; OperationOutcome outcome = 3; } }
```

`Execute` streams `OutputChunk` and optional `LifecycleEvent` messages, then exactly one `OperationOutcome`. The engine declares which statuses mean success, so a nonzero tool exit can be `CHANGES`. `Events` returns the lifecycle events a command emits so Atmos can validate hook bindings. `GenerateArtifacts` returns `ArtifactRef` entries.

| RPC | Replaces on `component.ComponentProvider` |
|---|---|
| `Describe` | `GetType`, `GetGroup`, `GetAvailableCommands`, `GetBasePath` (relative to the Atmos base path). |
| `ListComponents`, `Validate`, `GenerateArtifacts`, `Execute` | `ListComponents`, `ValidateComponent`, `GenerateArtifacts`, `Execute`. |

`ExecutionContext.AtmosConfig` and `ConfigAndStacksInfo` become `Invocation` plus `ComponentRequest`: resolved values, stack coordinates, working directory, scoped environment.

### CIProvider

```proto
service CIProvider {
  rpc Detect(DetectRequest) returns (DetectResponse);  rpc Context(CIRequest) returns (CIContext);
  rpc ResolveBase(CIRequest) returns (BaseResolution);  rpc GetStatus(GetStatusRequest) returns (StatusResponse);
  rpc CreateCheckRun(CheckRunRequest) returns (CheckRun);  rpc UpdateCheckRun(CheckRunRequest) returns (CheckRun);
  rpc PostComment(PostCommentRequest) returns (Comment);  rpc Capabilities(CIRequest) returns (CICapabilities);
  rpc WriteOutput(WriteOutputRequest) returns (WriteResponse);  rpc WriteSummary(WriteSummaryRequest) returns (WriteResponse);
}
message DetectResponse { bool matched = 1; string reason = 2; }
message CICapabilities { repeated string features = 1; }   // For example "labels", "commit_comments", "log_grouping".
```

`CIContext`, `BaseResolution`, `CheckRun`, and `Comment` mirror `provider.Context`, `provider.BaseResolution`, `provider.CheckRun`, and `provider.Comment` field for field, with `PRInfo.fork` and `elevated_event` preserved because Atmos's fork-trust gate reads them. Static `detect.env` predicates narrow candidates in Atmos; `Detect` runs only when the manifest sets `detect.callback`, may read only the declared runner environment, and must not write externally. `Capabilities` echoes the manifest `features`; optional interfaces (`LabelReader`, commit comments) become RPCs additively, and an undeclared one is `Unsupported`.

| RPC | Replaces on `provider.Provider` |
|---|---|
| `Detect`, `Context`, `ResolveBase`, `GetStatus`, `CreateCheckRun`, `UpdateCheckRun`, `PostComment` | Method of the same name. |
| `WriteOutput`, `WriteSummary` | `OutputWriter()`; Atmos calls them for the selected provider when a `CIComponent` reports. |
| `Capabilities` | The optional-interface type assertions. |

### CIComponent

```proto
service CIComponent {
  rpc HandleEvent(HandleEventRequest) returns (HandleEventResponse);
}
message HandleEventRequest { Invocation invocation = 1; string event = 2; string command = 3; OperationOutcome outcome = 4;
                              string output = 5; google.protobuf.Struct result = 6; CIContext ci = 7; }
message HandleEventResponse { bool handled = 1; repeated Diagnostic diagnostics = 2; }
```

Handled events come from the manifest (`capabilities.ci.components.<type>.events`). The plugin interprets `outcome` and `output`, then sends reporting requests through [`api.CIReporter`](#apicireporter); it does not hold a GitHub or GitLab client. A reporting failure never changes the infrastructure outcome.

| RPC | Replaces on `plugin.Plugin` |
|---|---|
| `HandleEvent` | `HookHandler func(*HookContext) error`. `HookContext.Provider`, `TemplateLoader`, and `CreatePlanfileStore` stay Atmos-side. |
| (manifest) | `GetType`, `GetHookBindings`. |

### Hook

```proto
service Hook { rpc Validate(HookValidateRequest) returns (ValidateResponse); rpc Run(HookRunRequest) returns (HookResult); }
message HookRunRequest { Invocation invocation = 1; string kind = 2; string name = 3; string event = 4; google.protobuf.Struct options = 5;
                          OperationOutcome original = 6; }
message HookResult { OperationStatus status = 1; repeated HookArtifact artifacts = 2; HookSummary summary = 3; repeated Finding findings = 4; }
message HookArtifact { string name = 1; string format = 2; bytes body = 3; ArtifactRef ref = 4; map<string, string> metadata = 5; }
message HookSummary { string title = 1; string status = 2; map<string, int32> counts = 3; string body = 4; bytes sarif = 5; }
```

`original` is the triggering operation's outcome, kept separate from the hook's own `status`, so an audit hook can tell a failed apply from its own reporting failure. Small artifact bodies are inline; large ones are `ref`s. Atmos adapts `Finding` into `Diagnostic` for rendering and CI.

| RPC | Replaces on `hooks.Engine` and `hooks.Kind` |
|---|---|
| `Run` | `Engine.Run(*ExecContext) (*Output, error)` and `ResultHandler`; `Output{Artifact, Summary}` maps to `HookResult`. |
| `Validate` | Kind option validation. |
| (none) | `ExecContext.Cmd` (`*cobra.Command`), `AtmosConfig`, `Info`, `Stdout`, `Stderr`; `Kind.Command`, `DefaultArgs`, `OnFailure`, which stay Atmos-side. |

### Step

```proto
service Step { rpc Validate(StepRequest) returns (ValidateResponse); rpc Execute(StepRequest) returns (StepResult); }
message StepRequest { Invocation invocation = 1; string type = 2; string name = 3; google.protobuf.Struct with = 4; }
message StepResult { string value = 1; repeated string values = 2; google.protobuf.Struct metadata = 3; map<string, string> outputs = 4; bool skipped = 5; }
```

Atmos renders templates and conditions before the call; `with` arrives resolved and schema-validated. Result values are validated against the manifest `result_schema` before later steps see them.

| RPC | Replaces on `step.StepHandler` |
|---|---|
| `Validate`, `Execute` | `Validate(*WorkflowStep)`, `Execute(ctx, *WorkflowStep, *Variables) (*StepResult, error)`. `StepResult.Error` becomes an `Error` status. |
| (manifest) | `GetName`, `GetCategory`, `RequiresTTY`, aliases, `KnownFields`. |

### Validator

```proto
service Validator { rpc Describe(ValidatorDescribeRequest) returns (ValidatorDescription); rpc Validate(ValidatorRequest) returns (ValidatorReport); }
message CheckDescriptor { string id = 1; string category = 2; repeated string inputs = 3; string options_schema = 4; }   // ValidatorDescription lists them.
message ValidatorRequest { Invocation invocation = 1; string check = 2; google.protobuf.Struct options = 3; google.protobuf.Struct provenance = 4;
                            oneof input { FilesInput files = 5; google.protobuf.Struct component = 6; ArtifactInput artifact = 7; } }
message FilesInput { string root = 1; repeated string paths = 2; }
message ArtifactInput { string type = 1; ArtifactRef ref = 2; }
message ValidatorReport { repeated Diagnostic diagnostics = 1; int32 files_checked = 2; string target = 3; bool skipped = 4; string skip_reason = 5; }
```

Findings and execution errors are separate: a violation is a `Diagnostic` in the report; a missing tool, crash, malformed input, or timeout is an `Error` status, which fails validation even with zero diagnostics. `skipped` marks a genuinely non-applicable check, reported as skipped and never as passed. Atmos owns sorting, masking, rendering, exit status, and CI annotations.

| RPC | Replaces |
|---|---|
| `Describe` | The proposed check registry descriptors ([validation](../extensions/validation.md)); checked against the manifest. |
| `Validate` | `validate.Validator.Validate(ctx, Request{Root, Paths, WorkflowPath, ConfigFile}) (validation.Report, error)`; `FilesInput` carries `Root` and `Paths`. Component and artifact inputs are new. |

### Store

```proto
service Store {
  rpc Set(StoreSetRequest) returns (StoreWriteResponse);  rpc Get(StoreGetRequest) returns (StoreGetResponse);
  rpc GetKey(StoreKeyRequest) returns (StoreGetResponse);  rpc GetRaw(StoreGetRequest) returns (StoreRawResponse);
  rpc Delete(StoreGetRequest) returns (StoreWriteResponse);  rpc Exists(StoreGetRequest) returns (StoreExistsResponse);
  rpc List(StoreListRequest) returns (StoreListResponse);
}
message StoreInstance { string name = 1; string kind = 2; google.protobuf.Struct options = 3; string identity = 4; bool secret = 5; }
message StoreKey { string stack = 1; string component = 2; string key = 3; }
message StoreGetRequest { Invocation invocation = 1; StoreInstance store = 2; StoreKey key = 3; }
message StoreGetResponse { bool found = 1; google.protobuf.Value value = 2; }   // found=false is missing; a null Value is a stored null.
message StoreWriteResponse { oneof result { bool done = 1; Unsupported unsupported = 2; } }
```

Every request names its `StoreInstance`; Atmos resolves `identity` through `api.Credentials`, replacing per-cloud `AuthContextResolver`. `GetRaw`, `Delete`, `Exists`, and `List` return `Unsupported` when the backend lacks them, never an empty success. `secret` is the configured `secret: true` flag; Atmos enforces the secret-only access path.

| RPC | Replaces |
|---|---|
| `Set`, `Get`, `GetKey` | `Store.Set(stack, component, key, value)`, `Get`, `GetKey`. |
| `GetRaw`, `Delete`, `Exists`, `List` | `RawStore.GetRaw`, `DeletableStore.Delete`, `StatusStore.Has`, `ListableStore.Keys`. |
| (none) | `LocalStore.IsLocal`, `SecretAwareStore.SetSecret`, `ValueListingSupported`: `Info.features` and `StoreInstance`. |

### SecretProvider

```proto
service SecretProvider {
  rpc Set(SecretSetRequest) returns (SecretWriteResponse);  rpc Get(SecretGetRequest) returns (SecretGetResponse);
  rpc GetRaw(SecretGetRequest) returns (SecretRawResponse);  rpc Delete(SecretGetRequest) returns (SecretWriteResponse);
  rpc Status(SecretGetRequest) returns (SecretStatusResponse);  rpc SupportsScope(SupportsScopeRequest) returns (SupportsScopeResponse);
}
enum Scope { SCOPE_UNSPECIFIED = 0; SCOPE_INSTANCE = 1; SCOPE_STACK = 2; SCOPE_GLOBAL = 3; }
message Coordinate { string stack = 1; string component = 2; string key = 3; Scope scope = 4; }
```

`Status` must not retrieve or decrypt the value. Atmos registers every returned value with the masker before using it, and `Get` results are never logged. A write that times out is an `UNAVAILABLE` error with unknown effect; Atmos does not retry or claim rollback. Unsupported scopes fail before any write.

| RPC | Replaces on `providers.Provider` |
|---|---|
| `Set`, `Get`, `Delete`, `Status`, `SupportsScope` | Method of the same name. |
| `GetRaw` | `RawGetter.GetRaw`. |
| (none) | `Kind`: manifest. `FileResettable`, `LocalStatus`, `FilePathProvider`: not carried; separate follow-on. |

### BackendProvisioner

```proto
service BackendProvisioner {
  rpc Describe(BackendDescribeRequest) returns (BackendDescription);  rpc Ensure(BackendRequest) returns (EnsureResponse);
  rpc Exists(BackendRequest) returns (ExistsResponse);  rpc Delete(BackendDeleteRequest) returns (DeleteResponse);
  rpc Name(BackendRequest) returns (NameResponse);
}
enum BackendState { BACKEND_STATE_UNSPECIFIED = 0; BACKEND_STATE_ABSENT = 1; BACKEND_STATE_PRESENT_COMPATIBLE = 2;
                    BACKEND_STATE_PRESENT_INCOMPATIBLE = 3; BACKEND_STATE_DENIED = 4; BACKEND_STATE_UNKNOWN = 5; }
message BackendRequest { Invocation invocation = 1; string kind = 2; string backend_type = 3; google.protobuf.Struct backend = 4;
                          google.protobuf.Struct options = 5; string identity = 6; }
message ExistsResponse { BackendState state = 1; string detail = 2; }
message EnsureResponse { repeated string resource_ids = 1; repeated string warnings = 2; OperationOutcome outcome = 3; }
```

`UNKNOWN` is not `ABSENT`: an unavailable check must not trigger creation. Deletion carries `force` and is explicit; removing a plugin never deletes provisioned infrastructure.

| RPC | Replaces |
|---|---|
| `Ensure` | `BackendCreateFunc`; `ProvisionResult.Warnings` becomes `warnings`. |
| `Exists` | `BackendExistsFunc` (`bool, error`); the enum adds the states the gap analysis requires. |
| `Delete`, `Name` | `BackendDeleteFunc(..., force)`, `BackendNameFunc`. |
| `Describe` | New: supported backend types and operations. |

### MCPTool

```proto
service MCPTool { rpc CallTool(CallToolRequest) returns (stream CallToolEvent); }
message CallToolRequest { Invocation invocation = 1; string name = 2; google.protobuf.Struct arguments = 3; }
message CallToolEvent { oneof event { Progress progress = 1; CallToolResult result = 2; } }
message CallToolResult { bool is_error = 1; string text = 2; google.protobuf.Struct structured = 3; }
```

Tool listing is not an RPC: names, descriptions, input and output JSON Schemas, and `requires_permission` and `restricted` flags come from the verified manifest, so `atmos mcp start` lists tools without starting a process. The plugin starts on the first `CallTool`. Permission checks and the executor stay in Atmos. `is_error` is a tool-level failure the model should see; protocol failures are `Error` statuses.

| RPC | Replaces on `tools.Tool` |
|---|---|
| `CallTool` | `Execute(ctx, params) (*Result, error)`; `Result{Success, Output, Data}` maps to `is_error`, `text`, `structured`. |
| (manifest) | `Name`, `Description`, `Parameters`, `RequiresPermission`, `IsRestricted`. The flat `Parameter` list is replaced by raw JSON Schema. |

## API services

API services are the plugin's sanctioned route to I/O Atmos controls. Atmos applies masking, proxies, deadlines, and policy. Native plugins can still bypass them with their own code, so these services bound what is supported and testable, not what is possible. Each call needs an in-flight invocation (see [broker](#api-services-over-the-broker)). Defined in `proto/atmos/api/v1/api.proto`:

```proto
package atmos.api.v1;

service HTTP { rpc Do(HTTPRequest) returns (HTTPResponse); }
service Secrets { rpc Resolve(ResolveRequest) returns (ResolveResponse); }
service Prompt {
  rpc Input(InputRequest) returns (InputResponse);  rpc Confirm(ConfirmRequest) returns (ConfirmResponse);
  rpc Select(SelectRequest) returns (SelectResponse);
}
service Browser { rpc Open(OpenRequest) returns (OpenResponse); rpc Listen(ListenRequest) returns (stream CallbackEvent); }
message ListenRequest { string path = 1; string expected_state = 2; }
message CallbackReady { string redirect_uri = 1; }
message Callback { string code = 1; string state = 2; string error = 3; }
message CallbackEvent { oneof event { CallbackReady ready = 1; Callback callback = 2; } }
service Process { rpc Run(RunRequest) returns (stream RunEvent); }   // RunEvent: OutputChunk, then exit status.
service FS {
  rpc Read(ReadRequest) returns (ReadResponse);  rpc Write(WriteRequest) returns (WriteResponse);
  rpc List(FSListRequest) returns (FSListResponse);
}
service Credentials {
  rpc Get(GetRequest) returns (Credentials);  rpc Put(PutRequest) returns (PutResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Resolve(IdentityRef) returns (Credentials);
}
service Diagnostics { rpc Emit(EmitRequest) returns (EmitResponse); }   // Diagnostic and Finding batches.
service CIReporter {
  rpc Output(CIOutputRequest) returns (CIReportResponse);  rpc Summary(CISummaryRequest) returns (CIReportResponse);
  rpc Check(CICheckRequest) returns (CIReportResponse);  rpc Comment(CICommentRequest) returns (CIReportResponse);
}
service Log { rpc Log(LogRequest) returns (LogResponse); }   // level, message, attributes.
service Toolchain { rpc ResolveTool(ResolveToolRequest) returns (ResolveToolResponse); }   // name, version -> path.
```

### api.HTTP

`Do` carries method, URL, headers, body, and timeout, and returns status, headers, and body. Each call performs one exchange; it returns redirects without following them. The SDK HTTP client applies bounded redirects, sensitive-header stripping across origins, and downgrade refusal. Atmos applies proxy settings, TLS roots, timeouts, and masking. It adds no credentials; the plugin supplies its own `Authorization` header, usually from `api.Secrets`. Bodies above the message limit fail with `INVALID_CONFIG`. The SDK `http.RoundTripper` preserves request context and form bodies for standard Go OIDC/OAuth clients. Code exchange, refresh, and revocation are not automatically retried.

### api.Secrets

`Resolve` returns a sensitive value for a name. It resolves only input names the package declares (manifest `inputs[]` entries such as the `token_env` field); anything else is `UNAUTHORIZED`. Resolved values are masked immediately.

### api.Prompt

`Input`, `Confirm`, and `Select` render through Atmos UI on stderr; `Input` with `secret = true` masks the echo. A non-interactive invocation fails with `INTERACTION_REQUIRED`, which Atmos maps to `ErrPluginInteractionRequired`.

### api.Browser

`Open` launches a URL, or prints it when no browser is available. `Listen` binds `127.0.0.1` on an ephemeral port and first emits `ready.redirect_uri`; the plugin uses that exact URI before `Open`. A subsequent matching path/state yields one callback. Wrong-state requests do not consume it. SDK `Close` cancels the stream, releasing the listener; invocation completion, deadline, cancellation, and plugin exit also clean it up. Noninteractive fresh login fails before binding. See [callback lifecycle](../oidc-pilot.md#browser-callback-contract).

### api.Process

`Run` resolves the executable through the package's toolchain-scoped `PATH`, runs it as Atmos's child in the invocation's working directory, and streams masked output. Cancellation and deadlines kill the process tree. A nonzero exit is data in the final event; failing to start is `NOT_FOUND` or `UNAVAILABLE`.

### api.FS

`Read`, `Write`, and `List` resolve paths after symlink evaluation and require them to lie under the invocation's working directory or the package directory. The package directory is read-only. Anything else is `UNAUTHORIZED`.

### api.Credentials

`Get`, `Put`, and idempotent `Delete` address only the calling plugin's namespace and carry a plugin-owned schema version; Atmos keyring stores the opaque `Credentials`. `Resolve` turns the generic `identity` reference on a store or backend request into credentials Atmos already authenticated, within the invocation's realm. A plugin cannot read another plugin's session data. Session state additionally scopes by provider configuration and realm, remains readable after access-token expiry for refresh, and follows the [atomic replacement and cross-process locking rules](../oidc-pilot.md#credential-and-session-contracts). `Delete` removes local private session state during logout even if remote revocation fails.

### api.Diagnostics

`Emit` lets a long-running check publish diagnostics before it returns. Atmos sorts, masks, and renders them with the final report.

### api.CIReporter

`Output`, `Summary`, `Check`, and `Comment` are structured requests Atmos routes to the selected CI provider, built-in or plugin. Atmos enforces CI enablement, fork-trust gates, and target validation. Unsupported reports return `Unsupported`; a skipped write is never reported as done.

### api.Log

`Log` is masked and attributed to plugin name, version, and invocation. Log output is never part of a result.

### api.Toolchain

`ResolveTool` returns the executable path pinned by the package's entries in `toolchain.lock.yaml`. A tool the manifest did not declare is `NOT_FOUND`; nothing is installed during an invocation.

## Error model

Atmos maps the wire class; [errors](errors.md) owns the full sentinel list, hints, and exit codes. Classes describe conditions, never exit codes.

| `ErrorClass` | gRPC code | Atmos sentinel | User-facing behavior |
|---|---|---|---|
| `UNSPECIFIED` | `UNKNOWN` | `ErrPluginProtocol` | Non-conforming plugin; fails. |
| `INVALID_CONFIG` | `INVALID_ARGUMENT` | `ErrPluginSchemaViolation` | Hint names the configuration path; fails before side effects where possible. |
| `UNSUPPORTED` | `UNIMPLEMENTED` | `ErrPluginUnsupportedOperation` | Required operations fail; optional ones fall back. |
| `UNAUTHORIZED` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Domain sentinel, for example `ErrAuthenticationFailed` | Fails; no retry with other credentials. |
| `NOT_FOUND` | `NOT_FOUND` | Domain sentinel | Fails, or the extension's explicit "missing" result (stores use `found = false`). |
| `UNAVAILABLE` | `UNAVAILABLE` | Domain sentinel | Fails with guidance; the effect of a write is unknown. |
| `CONFLICT` | `ABORTED`, `ALREADY_EXISTS` | Domain sentinel | Fails with a reload hint. |
| `CANCELLED` | `CANCELLED` | `context.Canceled` | Reported as cancelled. |
| `INTERACTION_REQUIRED` | `FAILED_PRECONDITION` | `ErrPluginInteractionRequired` | Names the missing input. |
| `INTERNAL` | `INTERNAL` | `ErrPluginCrashed` | Plugin bug; message shown after masking. |

Atmos-detected conditions: process exit or transport loss is the proposed `ErrPluginCrashed`; an expired deadline is `ErrPluginTimeout`; malformed messages, unknown enum values, or a missing `OperationOutcome` required by the extension response contract is `ErrPluginProtocol`; an empty version intersection is `ErrPluginIncompatible` (see [Negotiation](#negotiation)).

- **Crash, timeout, cancel.** None is success, skipped, or an empty result. A cancel stops plugin work and owned subprocesses; completed work of a state-changing operation is reported as partial.
- **No automatic retry.** Atmos never reruns an operation after a crash, timeout, or `UNAVAILABLE` write. `Invocation.id` stays stable and `attempt` increments only when workflow or hook retry configuration reruns the operation, so plugins can implement idempotency keys. Exactly-once external execution is not promised.
- **Failure context.** Every surfaced error names plugin, version, service, and kind. Plugin `message` and `hint` are masked before display.

## Starlark parity

Starlark plugins run in process on the existing [Starlark engine](../../../../pkg/script/starlark/engine.go) and implement the same contracts. Field names match the proto; messages appear as frozen dicts and structs, with `None` for unset fields. `spec.runtime.api: 1` versions the Starlark surface alongside the service contracts.

### Service bindings

| Plugin service | Registration | Handlers |
|---|---|---|
| `Info` | Implicit; Atmos synthesizes `Describe` from registrations and the manifest. | n/a |
| `AuthProvider` | `plugin.register_auth_provider(kind, ...)` | `validate`, `authenticate`, `refresh`, `logout`, `environment`, `paths`, `prepare_environment` |
| `AuthIdentity` | `plugin.register_auth_identity(kind, ...)` | `validate`, `authenticate`, `environment`, `paths`, `prepare_environment`, `logout` |
| `Component` | `plugin.register_component(type, ...)` | `describe`, `list_components`, `validate`, `generate_artifacts`, `execute`, `events` |
| `CIProvider` | `plugin.register_ci_provider(kind, ...)` | `detect`, `context`, `resolve_base`, `get_status`, `create_check_run`, `update_check_run`, `post_comment`, `write_output`, `write_summary`, `capabilities` |
| `CIComponent` | `plugin.register_ci_component(type, ...)` | `handle_event` |
| `Hook` | `plugin.register_hook(kind, ...)` | `validate`, `run` |
| `Step` | `plugin.register_step(type, ...)` | `validate`, `execute` |
| `Validator` | `plugin.register_validator(id, ...)` | `describe`, `validate` |
| `Store` | `plugin.register_store(kind, ...)` | `set`, `get`, `get_key`, `get_raw`, `delete`, `exists`, `list` |
| `SecretProvider` | `plugin.register_secret_provider(kind, ...)` | `set`, `get`, `get_raw`, `delete`, `status`, `supports_scope` |
| `BackendProvisioner` | `plugin.register_backend_provisioner(kind, ...)` | `describe`, `ensure`, `exists`, `delete`, `name` |
| `MCPTool` | `plugin.register_mcp_tool(name, ...)` | `call_tool` |

A handler takes one `request` and returns a dict, struct, or the helper constructors in `auth`, `validation`, and `hooks` (`auth.result`, `validation.finding`, `hooks.finding`). An omitted optional handler is `Unsupported`. A handler that streams in Go (for example `execute`) calls `atmos.emit(chunk)` and returns the outcome. `validation.finding` builds a wire `Diagnostic`; a list of findings is shorthand for a `ValidatorReport`.

### API bindings

| API service | Module | Notes |
|---|---|---|
| `api.HTTP` | `atmos.http` | `request`, `get`, `post`, and so on; response has `status`, `headers`, `raise_for_status()`, `json()`. |
| `api.Secrets` | `atmos.secrets` | `environment(name)` resolves a declared input. |
| `api.Prompt` | `atmos.prompt` | `input`, `confirm`, `select`. |
| `api.Browser` | `atmos.browser` | `open`, `listen`. |
| `api.Process` | `atmos.process` | `run`; replaces `exec` in plugin sessions. |
| `api.FS` | `atmos.fs` | `read`, `write`, `list`; replaces `fs`. |
| `api.Credentials` | `atmos.credentials` | `get`, `put`, `delete`, `resolve`. |
| `api.Diagnostics` | `atmos.diagnostics` | `emit`. |
| `api.CIReporter` | `atmos.ci_reporter` | `output`, `summary`, `check`, `comment`. |
| `api.Log` | `atmos.log` | `debug`, `info`, `warn`, `error`. |
| `api.Toolchain` | `atmos.toolchain` | `resolve_tool`. |

### Execution model

- **One session per package per process.** The entrypoint is evaluated once, on first use; `plugin.register_*` calls are valid only during that evaluation, after which globals are frozen. Discovery and help evaluate nothing.
- **Isolated thread per invocation.** Each call runs on a new interpreter thread created like `session.thread` in the engine; frozen globals prevent shared mutable state.
- **Cancellation** reuses the engine's `context.AfterFunc(ctx, t.Cancel)` hook in `session.thread`. Blocked `atmos.*` calls observe the thread's context and return `CANCELLED`.
- **Predeclared modules.** `predeclared()` gains `plugin`, `auth`, `validation`, and `hooks` for plugin sessions only and extends `atmos` with the API services. Plugin sessions keep the pure modules (`json`, `regex`, `digest`, `errors`). They do not predeclare the ambient `exec`, `fs`, `env`, `components`, `steps`, `ci`, or `ui` modules; their scoped equivalents live under `atmos.*` (`atmos.process`, `atmos.fs`, `atmos.steps`, `atmos.ci`), backed by the API services.
- **`load()` is package-local.** The engine's current `load` resolves absolute paths as given. Plugin sessions must reject absolute and `..`-escaping paths and cannot fetch remote code.

### Examples

An auth provider, consistent with [authoring](../authoring.md). `request` is `ProviderRequest`:

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

plugin.register_auth_provider("acme/broker", authenticate = authenticate)   # Omitted refresh is Unsupported.
```

A validator, consistent with [validation](../extensions/validation.md). `request` is `ValidatorRequest` with `component` set:

```python
def validate(request):
    tags = request.component.get("vars", {}).get("tags", {})
    if tags.get("Owner"):
        return []
    return [validation.finding(
        rule_id = "acme/owner-required",
        severity = "error",
        message = "Set vars.tags.Owner to the responsible team",
        field = "vars.tags.Owner",
    )]

plugin.register_validator("acme/owner-required", validate = validate)
```

## Data runtime

Skills and rule packs have no process, no protocol, and no API-service calls. The manifest declares entrypoints (skill directories, rule `path`, `engine`, `options_schema`). In-process engines (`opa`, `jsonschema`, and `cue` once the engine registry exists) evaluate rule files from the verified package directory and produce `Diagnostic` values directly; the skill loader reads `SKILL.md` through the `plugin://` source. A mixed package uses this path for its data exports and the native or Starlark runtime for the rest.

## Wasm follow-on

An [Extism](https://extism.org)-based runtime would be a fourth runtime, selected by `spec.runtime.type: wasm`. It is not part of the initial release: v1 hosts reject the type and no initial contract depends on it. `extism/go-sdk` and `wazero` are already indirect dependencies. It would need to preserve:

- **Same messages.** Plugin exports are functions named `atmos_<service>_<rpc>` taking and returning protobuf-encoded messages. Contract versions and `Describe` work unchanged.
- **Extism host functions are the API surface.** Each unary API RPC becomes an Extism host function (`atmos_api_http_do`, `atmos_api_secrets_resolve`, and so on) with the same scoping, masking, and invocation checks.
- **No process, no ambient access.** `api.Process`, `api.Browser`, and ambient filesystem and network access are off by default. Network is limited to `api.HTTP` plus an allowlist; memory and time limits come from the Extism manifest. Atmos policy may refuse any broader declared need.
- **Streaming and cancellation.** Server-streaming RPCs need a defined shape (Atmos-polled chunks or buffered results), the main open design question. Cancellation maps to Extism timeouts and context checks at host-function boundaries.

## Conformance

- **Schema gate.** CI runs `buf lint`, `buf breaking --against` the last released tag of both `proto/atmos/plugin/v1/` and `proto/atmos/api/v1/` with the `FILE` category, and verifies that generated code in `pkg/plugin/proto` and `sdk/proto` is current and equivalent. Regenerate through `atmos dev generate proto`. A breaking change requires a new protocol major, not an override.
- **Behavioral gate.** `atmos plugin test` loads `tests/contract.yaml` and drives scenarios through the same Atmos adapters that production uses: native RPC through go-plugin, or Starlark callbacks through the engine. Go and Starlark implementations of one extension pass the same applicable scenarios. The suite ships with Atmos, so a plugin author never needs the Atmos source tree. See [testing](../testing.md).
- **Old protocol versions.** The `sdk/` module and Atmos CI retain a fixture plugin for each supported protocol major and contract range and run them against every Atmos build.
- **Not Pact.** Consumer-driven contracts cannot be verified against an open set of third-party providers; the reasoning is in [Decision 15](decisions.md).
