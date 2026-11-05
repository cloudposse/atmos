# Plugin platform gap analysis

**Status:** Build inventory for the [plugin platform](../overview.md). Findings describe the code as of this document's date; required changes are proposed. [Platform workstream](README.md) · [Delivery plan](delivery-plan.md).

This document answers one question per surface: what exists today, what is missing, and where Atmos must call the plugin registry. The product PRDs say what each extension must do; this document says what has to be built for that to be true. Line numbers are omitted because they drift; function and type names are stable.

This inventory records the pre-implementation baseline and remaining target surfaces. Auth now uses `pkg/auth/registry`; native auth/command loading, OCI lifecycle, shared UI, signatures, and general plugin policy have working implementations. Consult the [first-release checkpoint](../first-release.md#native-pilot-checkpoint) and [plugin policy contract](../policy.md) before treating a gap below as current.

## Summary

| Surface | Existing registry | Duplicate policy | `ctx` on the interface | Reachable from outside the module | Verdict |
|---|---|---|---|---|---|
| Auth providers/identities | [`registry.Registry`](../../../../pkg/auth/registry/registry.go), **partial** | Replaces | Yes | Yes | Finish registry adoption; delete the switch |
| Credentials | None (closed keyring switch) | n/a | n/a | n/a | **Create** credential-type registry |
| Component engines | [`component.Register`](../../../../pkg/component/registry.go) | Replaces | Yes (`ExecutionContext.Context`) | Yes | Register legacy engines; open four closed lists |
| CI providers | [`ci.Register`](../../../../pkg/ci/registry_provider.go) | Replaces | Yes | Yes (alias) | Deterministic detection; explicit selector |
| Component CI reporting | [`ci.RegisterPlugin`](../../../../pkg/ci/plugin_registry.go) | Errors | Via `HookContext` | **No** (`pkg/ci/internal/plugin`) | Export public aliases |
| Commands | [`internal.Register`](../../../../cmd/internal/registry.go) | Replaces | n/a | **No** (`cmd/internal`) | Collision detection between built-ins, YAML, and plugins |
| Hooks | [`hooks.RegisterKind`](../../../../pkg/hooks/kind.go) | Errors | **No** | Yes | Add context; remove `cobra.Command` from the wire |
| Steps | [`step.Register`](../../../../pkg/runner/step/registry.go) | Replaces | Yes | Yes | Error on external collisions; static validation of namespaced types |
| MCP tools | [`tools.Registry`](../../../../pkg/ai/tools/registry.go) (instance) | Errors | Yes | Yes | Raw JSON-schema path; lazy start |
| Skills | [`skills.Registry`](../../../../pkg/ai/skills/registry.go) (instance) | First loaded wins | n/a | Yes | Plugin loader stage; `plugin://` source |
| Stores | [`store.Register`](../../../../pkg/store/registry.go) | Panics | **No** | Yes | Add context; registered capability metadata |
| Secret providers | [`providers.Register`](../../../../pkg/secrets/providers/registry.go) keyed by **track** | Panics | **No** | Yes | Kind-keyed registry under a generic track |
| Backend provisioning | Four func maps in [`backend`](../../../../pkg/provisioner/backend/backend.go) | Replaces | Yes | Yes | **Create** one interface with existence states |
| Validation | None (closed switch); CI-only [`validate.Register`](../../../../pkg/ci/validate/validate.go) | Panics (CI) | Yes (CI) | Yes | **Create** engine and check registries |
| Manifest kinds | [`manifest.Register`](../../../../pkg/manifest/registry.go) | Replaces | n/a | Yes | Register four plugin kinds; error on duplicates |
| Package lifecycle | None | n/a | n/a | n/a | **Create** `pkg/plugin` |

Dependency status in `go.mod`: `google/go-containerregistry` direct; `google.golang.org/grpc` direct (cloud clients only); `go.starlark.net` direct; `oras.land/oras-go/v2`, `extism/go-sdk`, `tetratelabs/wazero` indirect; `hashicorp/go-plugin` and sigstore absent.

The contract package adapters bind already exists: [`pkg/api`](../../../../pkg/api) (renamed from `pkg/automation`; [Decision 18](decisions.md), P0.12) holds the step library, process execution policy, and filesystem view with no Starlark dependency. It is the Atmos API contract package in the main module.

## First-release prerequisites and reusable foundations

The [release boundary](../first-release.md) requires P0.1 (complete auth registry registration), P0.2 (credential-type registry), P0.3 for manifest/command collisions, and P0.12 (`pkg/api`). Remaining surface hardening is retained below and delivered with its adapter. Auth includes session locking across processes and registration into the same dispatch path used by built-ins; an external-provider fallback beside the existing switches is insufficient.

The platform also needs an extension descriptor registry for static validation, negotiation, scaffolds/tests, and adapter construction. Auth and executable commands are the first consumers. A test-only third descriptor must integrate without modifications to package lifecycle code. Future contracts are not advertised as runnable until their adapter is registered.

[OCI reuse](oci-reuse.md) adds a concrete inventory of shared pulls, auth, digest pinning, version tag listing, extraction, and test harnesses. Extend `pkg/oci` and migrate direct registry calls from the version resolver; keep plugin package/catalog policy above that layer. The future Atmos Artifacts feature is not a dependency.

## Extension surfaces

### Authentication

| Aspect | Finding |
|---|---|
| Registry | `Registry.RegisterProvider(kind, ProviderConstructor)` and `RegisterIdentity(kind, IdentityConstructor)` exist. All built-in and plugin constructors now share the process-wide registry, with exported registration APIs and duplicate rejection. |
| Closed lists | `NewProvider` and `NewIdentity` fall through to `switch config.Kind` covering every AWS, Azure, GitHub, Atmos Pro, and mock kind. The keyring store decodes credentials through a second closed `switch` over the tags `aws`, `gcp`, `oidc`, `atmos-pro`, `mock` (Azure is absent). |
| Duplicate policy | Plain map assignment; a second registration silently replaces the first. |
| Boundary leaks | `Provider.PreAuthenticate(manager AuthManager)` and `PostAuthenticateParams{AuthContext, ConfigAndStacksInfo, Manager}` hand internal managers to the implementation. `ConfigSetter.SetConfig(*schema.Identity)` passes the whole config struct. |
| Credential types | No versioned credential type exists. The product PRDs reference `aws/v1`; the code has `ICredentials` implementations and an untyped `{type, data}` envelope. |
| Interfaces to adapt | [`types.Provider`](../../../../pkg/auth/types/interfaces.go) (11 methods including `Environment`, `Paths`, `PrepareEnvironment`, `Logout`, `SetRealm`), `types.Identity` (12 methods including `CredentialsExist`, `LoadCredentials`, `PostAuthenticate`), optional `Provisioner`, `ConsoleAccessProvider`, `JustificationConsumer`. |

Injection points: bootstrap registers verified plugin provider/identity constructors into the same registry as built-ins; `registry.NewProvider` and `NewIdentity` perform registry-only lookup; keyring encode/decode → credential-type registry; `schema.Provider`/`schema.Identity` kind validation → registered metadata; `atmos auth list`/`whoami` display of plugin kinds.

Prerequisites: P0.1 (registry completion), P0.2 (credential types). Follow-on: [auth integrations registry](../../../../pkg/auth/integrations/registry.go) (`Register` replaces; `Integration{Kind, Execute, Cleanup, Environment}`).

### Component engines

| Aspect | Finding |
|---|---|
| Registry | `component.Register(provider ComponentProvider) error`; `GetProvider`, `MustGetProvider`, `ListTypes`, `ListAllComponents`. Built-ins register in `init()` through blank imports in `cmd/root.go`: ansible, aws/cloudformation, container, emulator, helm, kubernetes, mock. [`custom/provider.go`](../../../../pkg/component/custom/provider.go) registers at runtime. |
| Not registered | **terraform, helmfile, and packer never call `component.Register`.** They run through legacy paths in `internal/exec`. |
| Duplicate policy | "Last registration wins, allowing plugin override." |
| Closed lists | `schema.Components.GetComponentConfig` switches on built-in types; dynamic types land in `Components.Plugins map[string]any` (`mapstructure:",remain"`). `internal/exec/describe_component.go` `autoDetectComponentTypes`; `internal/exec/describe_stacks_component_processor.go` `typeEntries`; `internal/exec/describe_stacks.go` `getComponentBasePath`; three type lists in `pkg/list/extract/components.go`; `pkg/utils/component_reverse_path_utils.go` and `cmd/list/sources.go` hardcode `terraform, helmfile, packer`. Component sections (`vars`, `settings`, `env`, `hooks`, `secrets`, …) are whitelisted in the stack processor (`pkg/config/const.go`, `internal/exec/stack_processor_process_stacks_helpers_extraction.go`, `internal/exec/stack_processor_merge.go`); custom types survive only through the raw-map fallback in `internal/exec/stack_processor_process_stacks.go`. |
| Boundary leaks | `Execute(ctx *ExecutionContext)` and `GenerateArtifacts` take a struct carrying `*schema.AtmosConfiguration`, `*schema.ConfigAndStacksInfo`, and `Flags map[string]any`. |
| Interface to adapt | [`ComponentProvider`](../../../../pkg/component/provider.go): `GetType`, `GetGroup`, `GetBasePath(*schema.AtmosConfiguration)`, `ListComponents(ctx, stack, stackConfig)`, `ValidateComponent(config)`, `Execute(*ExecutionContext)`, `GenerateArtifacts(*ExecutionContext)`, `GetAvailableCommands()`. |

Injection points: `GetProvider` miss → plugin engines; every closed list above reads `component.ListTypes()` plus plugin metadata; stack processing treats a registered external type like a custom type; dependency graphs and `describe affected` resolve types through the registry; the component CI registry (below) accepts the engine's `ci.component` extension.

Prerequisites: P0.3 (duplicate policy), P0.6 (registry completeness).

### CI providers and component CI reporting

| Aspect | Finding |
|---|---|
| Provider registry | `ci.Register(p provider.Provider)` has no validation and replaces by `Name()`. `Detect()` returns the first match while iterating a Go map, so detection order is nondeterministic. `ResolveProvider` falls back to `generic`. |
| Provider interface | [`provider.Provider`](../../../../pkg/ci/internal/provider/types.go), re-exported as [`ci.Provider`](../../../../pkg/ci/provider.go): `Name`, `Detect`, `Context`, `GetStatus`, `CreateCheckRun`, `UpdateCheckRun`, `PostComment`, `OutputWriter`, `ResolveBase`; optional `LabelReader`, `DebugModeDetector`, `LogGrouper`. |
| Component CI registry | `ci.RegisterPlugin(p plugin.Plugin) error` errors on duplicates. `Plugin{GetType, GetHookBindings}`, `HookBinding{Event, Handler HookHandler}`, `HookHandler func(*HookContext) error`. |
| Reachability | `Plugin`, `HookBinding`, and `HookContext` live in `pkg/ci/internal/plugin` with no public alias; only code under `pkg/ci/...` can implement them. Built-ins: terraform, helmfile, helm, kubernetes, cloudformation. |
| Configuration | No `ci.provider` selector exists. |

Injection points: `ResolveProvider` consults explicit `ci.provider`, then an ordered candidate list that includes plugin providers; `GetPluginForEvent` consults plugin `ci.component` extensions; reporting calls route through the `api.CIReporter` service rather than giving plugins the `OutputWriter`.

Prerequisites: P0.3, P0.5 (public aliases), P0.7 (deterministic detection and selector). Artifact ([`artifact.Register`](../../../../pkg/ci/artifact/registry.go)) and cache ([`cache.Register`](../../../../pkg/ci/cache/registry.go)) backends panic on invalid input, replace on duplicate, and remain follow-ons.

### Commands

| Aspect | Finding |
|---|---|
| Registry | `internal.Register(provider CommandProvider)` and `RegisterAll(root)`; duplicate names silently overwrite ("plugin override functionality"). YAML custom commands are processed **after** `RegisterAll` by `processCustomCommands` in [`cmd/cmd_utils.go`](../../../../cmd/cmd_utils.go) and may extend or override built-ins. |
| Reachability | `cmd/internal` is Go-internal. [`CommandProvider`](../../../../cmd/internal/command.go): `GetCommand`, `GetName`, `GetGroup`, `GetFlagsBuilder`, `GetPositionalArgsBuilder`, `GetCompatibilityFlags`, `GetAliases`, `IsExperimental`. |
| Precedent | Top-level `aliases:` re-exec Atmos as a subprocess. No `atmos-<name>` executable discovery exists. |
| Contradiction | [`extensions/commands.md`](../extensions/commands.md) says plugins cannot replace built-ins or YAML commands. Nothing enforces that today for any registrant. |

Injection points: after `RegisterAll` and before `processCustomCommands`, a plugin command adapter adds root commands from static manifest metadata and records collisions; `atmos plugin run <alias> -- …` dispatches regardless; help rendering reads manifest metadata without execution.

Prerequisites: P0.3 (collision policy). Decision: `CommandProvider` stays in `cmd/internal`; the adapter lives in `cmd/plugin`.

### Hooks

| Aspect | Finding |
|---|---|
| Registry | `hooks.RegisterKind(k *Kind) error` errors on duplicate, nil, empty name, or nil engine; `GetKind`, `ListKinds`. Built-ins: command, store, step/steps, and [`kinds/`](../../../../pkg/hooks/kinds) (infracost, kics, git, tfmigrate, checkov, tflint, trivy). |
| Engine | `Engine.Run(ctx *ExecContext) (*Output, error)`; `ResultHandler func(*ExecContext) (*Summary, error)`; `Kind{Name, Command, DefaultArgs, DefaultEnv, CaptureStdout, OnFailure, Engine, ResultHandler}`. |
| Missing context | `ExecContext` has no `context.Context`; it carries `Cmd *cobra.Command`, `AtmosConfig`, and `Info *schema.ConfigAndStacksInfo`. |
| Findings | `hooks.Finding{Path, Line, Severity, RuleID, Message}` is separate from [`validation.Diagnostic`](../../../../pkg/validation/diagnostic.go). [`RegisterFormatHandler`](../../../../pkg/hooks/format.go) errors on duplicates. |
| Step bridge | [`step_engine.go`](../../../../pkg/hooks/step_engine.go) registers `step` and `steps` with `OnFailureWarn`, resolves `hook.Type` through `runnerstep.Get`, and builds a `schema.WorkflowStep` from `type:` and `with:`. |

Injection points: `GetKind` miss → plugin hook kinds; hook option validation reads the plugin schema; `ResultHandler` output maps through a shared finding adapter.

Prerequisites: P0.4 (context plumbing).

### Steps

| Aspect | Finding |
|---|---|
| Registry | `step.Register(handler StepHandler)` silently replaces, aliases included; `Get` falls back to aliases; about 44 handlers register in `init()`. |
| Interface | [`StepHandler`](../../../../pkg/runner/step/registry.go): `GetName`, `GetCategory`, `Execute(ctx, *schema.WorkflowStep, *Variables) (*StepResult, error)`, `Validate(*schema.WorkflowStep) error`, `RequiresTTY`; optional unexported `GetAliases()` and `KnownFields()`. |
| Starlark bridge | [`api.StepLibrary`](../../../../pkg/api/steps.go) `{Names, Fork, Validate(*StepCall), Run(ctx, *StepCall)}` implemented by [`NewAutomationLibrary`](../../../../pkg/runner/step/automation_library.go). `StepCall` carries `Type`, `Configuration`, `WorkingDirectory`, `ProcessEnv`, `Stdout`, `Stderr`, `Parallel`. |
| Static validation | Unknown-field checks depend on `KnownFields()`; a namespaced external type must supply equivalent metadata without a handler process. |

Injection points: `Get` miss → plugin steps; workflow, custom-command, and hook validators consult plugin option schemas; `StepLibrary.Names()` includes plugin types so `steps.run("acme/x")` works.

Prerequisites: P0.3, P0.11.

### MCP tools

| Aspect | Finding |
|---|---|
| Registry | Instance-based `tools.NewRegistry()`; `Register(tool Tool) error` errors on duplicate; `ListByCategory` ignores its argument (TODO). Built-ins register in [`atmos.RegisterTools`](../../../../pkg/ai/tools/atmos/setup.go); external MCP servers through [`client.RegisterMCPTools`](../../../../pkg/mcp/client/register.go). |
| Interface | [`Tool`](../../../../pkg/ai/tools/types.go): `Name`, `Description`, `Parameters() []Parameter`, `Execute(ctx, params map[string]any) (*Result, error)`, `RequiresPermission`, `IsRestricted`. `Parameter{Name, Description, Type, Required, Default}` is flat; [`Server.generateInputSchema`](../../../../pkg/mcp/server.go) derives JSON Schema from it. |
| Execution path | `tools.NewExecutor(registry, *permission.Checker, timeout)` → [`permission.Checker.CheckPermission`](../../../../pkg/ai/tools/permission/checker.go) → `mcp.NewAdapter(registry, executor)` → [`NewServer`](../../../../pkg/mcp/server.go). |

Injection points: `RegisterTools` adds plugin tools from manifest metadata; an optional `JSONSchema()` interface lets `generateInputSchema` pass plugin schemas through unchanged; `Execute` starts the plugin lazily; the permission checker applies unchanged.

Prerequisites: none in Phase 0 beyond the trivial `ListByCategory` fix.

### Skills

| Aspect | Finding |
|---|---|
| Registry | Instance `skills.Registry`; `Register(*Skill) error` errors on duplicate; `LoadSkills` skips duplicates with a warning, so **first loaded wins**. |
| Load order | [`LoadSkills`](../../../../pkg/ai/skills/loader.go): project skills from `.atmos/skills/installations.json` ([`owned_loader.go`](../../../../pkg/ai/skills/marketplace/owned_loader.go)), then `~/.atmos/skills` and the local registry, then inline `ai.skills` entries. |
| Format | `SKILL.md` frontmatter is [`marketplace.SkillMetadata`](../../../../pkg/ai/skills/marketplace/metadata.go) (agentskills.io shape). Client distribution lives in [`marketplace/installer.go`](../../../../pkg/ai/skills/marketplace/installer.go). |

Injection points: a plugin-owned loader stage before user skills; a `plugin://<alias>/<skill>` source adapter; ownership records in `installations.json` tied to package digest.

Prerequisites: none in Phase 0.

### Stores

| Aspect | Finding |
|---|---|
| Registry | `store.Register(kind string, factory StoreFactory)` panics on duplicate; `StoreFactory func(name, StoreConfig) (Store, error)`; legacy `type` maps to `kind`. |
| Closed lists | `secretByDefaultKinds` and `secretIncapableKinds` are hardcoded in [`registry.go`](../../../../pkg/store/registry.go). |
| Interface | [`Store`](../../../../pkg/store/store.go): `Set(stack, component, key, value)`, `Get(stack, component, key)`, `GetKey(key)`, **no context**. Optional `RawStore`, `DeletableStore`, [`IdentityAwareStore`](../../../../pkg/store/identity.go) with a per-cloud `AuthContextResolver`. |

Injection points: `NewStoreRegistry` kind miss → plugin stores; capability metadata comes from the manifest; auth context is passed as a generic scoped identity rather than per-cloud resolvers.

Prerequisites: P0.4 (context).

### Secret providers

| Aspect | Finding |
|---|---|
| Registry | `providers.Register(track string, c Constructor)` panics on duplicate and is keyed by **track**; only `store` and `sops` exist. `Constructor func(*schema.AtmosConfiguration, name, sectionProviders map[string]any) (Provider, error)`. |
| Interface | [`Provider`](../../../../pkg/secrets/providers/provider.go): `Set(Coordinate, value)`, `Get(Coordinate)`, `Delete(Coordinate)`, `Status(Coordinate) (bool, error)`, `Kind`, `SupportsScope(Scope)`, **no context**; optional `RawGetter`, `FileResettable`, `LocalStatus`, keygen. |
| Configuration | No `provider:` selector; declarations choose `store` or `sops`. |

Injection points: a generic external track resolves `kind` through the plugin registry; `provider:` selector in secret declarations; `atmos secret` commands and `!secret` resolution unchanged.

Prerequisites: P0.4, P0.9.

### Backend provisioning

| Aspect | Finding |
|---|---|
| Registry | Four parallel maps: `RegisterBackendCreate`, `RegisterBackendDelete`, `RegisterBackendExists`, `RegisterBackendName`; all replace silently. Only `s3` and `azurerm` register; there is no `gcs` provisioner. |
| Lifecycle registry | [`provisioner.RegisterProvisioner`](../../../../pkg/provisioner/registry.go) appends without deduplication; `ProvisionerFunc` receives `*schema.AtmosConfiguration`, `*schema.AuthContext`, `OutputWriters`, and `*TerraformExecContext`. |
| Existence semantics | A missing exists-checker falls back rather than reporting unknown. |
| Related closed map | [`internal/terraform_backend`](../../../../internal/terraform_backend/terraform_backend_registry.go) state readers (local, s3, gcs, azurerm) are closed but not required by this extension. [Targets](../../../../pkg/provisioner/target/target.go) replace on duplicate and remain a follow-on. |

Injection points: backend operations resolve by new `provision.backend.kind` and fall back to `backend_type`; explicit `atmos terraform backend` commands and pre-init lifecycle use the same path.

Prerequisites: P0.3, P0.10.

### Validation

| Aspect | Finding |
|---|---|
| Registry | **None for engines.** [`validate_component.go`](../../../../internal/exec/validate_component.go) hardcodes `switch schemaType { "jsonschema", "opa" }`; `ValidateWithCue` in [`validate_utils.go`](../../../../internal/exec/validate_utils.go) is unreachable. |
| CI-only registry | [`validate.Register`](../../../../pkg/ci/validate/validate.go) with `Validator{Name, Validate(ctx, Request) (validation.Report, error)}` and `Request{Root, Paths, WorkflowPath, ConfigFile}`; the only registrant is actionlint. |
| Reusable model | [`validation.Diagnostic`](../../../../pkg/validation/diagnostic.go) `{Source, RuleID, Severity, Message, File, Line, Column, EndLine, EndColumn}`, `Report`, [`SARIF`](../../../../pkg/validation/sarif.go), `ToAnnotations`. [`validator.Validator`](../../../../pkg/validator/schema_validator.go) wraps gojsonschema. |
| Configuration | `validate.checks`, `settings.validation.<name>.validator`, and `atmos validate artifact` do not exist. |

Injection points: component validation dispatch reads check descriptors; root `atmos validate` runs configured project checks; a new artifact subcommand; CI validators register through the same check registry.

Prerequisites: P0.8 (engine and check registries).

## Registries to create

| # | Registry | Package | Replaces | Keys | Notes |
|---|---|---|---|---|---|
| N1 | Installed-plugin registry | `pkg/plugin/registry` | nothing | alias → package | Scope precedence (project over user), digest, manifest, runtime handle; cross-namespace collision detection for kinds, types, commands, tools, skills, stores, checks. |
| N2 | Plugin manifest kinds | `pkg/plugin/manifest` via [`manifest.Register`](../../../../pkg/manifest/registry.go) | single `AtmosScaffoldConfig` registrant | kind | `AtmosPluginConfig`, `AtmosPluginLock`, `AtmosPluginCatalog`, `AtmosPluginTestConfig`. `manifest.Register` must stop replacing on duplicates. |
| N3 | Credential types | `pkg/auth/credentials` | keyring type switch | `aws/v1`, `gcp/v1`, `azure/v1`, `oidc/v1`, `atmos-pro/v1`, plugin-namespaced | Versioned envelope with `sensitive_keys`; adapters from existing `ICredentials` implementations. |
| N4 | Validation engines and checks | `pkg/validation/engine`, `pkg/validation/check` | `validate_component.go` switch | engine name; check ID | Engines: jsonschema, opa, cue, tflint adapter. Checks carry category, input type, options schema. |
| N5 | Plugin runtimes | `pkg/plugin/runtime` | nothing | `native` first; `starlark`, `data`, `wasm` follow-on | `Runtime.Start(ctx, Package) (Handle, error)`. |
| N6 | Secret-provider kinds | `pkg/secrets/providers` generic track | track-keyed map | kind | Built-in tracks unchanged. |
| N7 | Backend provisioner interface | `pkg/provisioner/backend` | four func maps | kind, backend type | `Ensure`, `Exists` returning absent/present-compatible/present-incompatible/denied/unknown, `Delete`, `Name`, `Describe`. |
| N8 | Extension descriptors | `pkg/plugin/extension` | no existing shared extension dispatch | capability selector | Static schemas/versions, adapter factory, scaffold/test hooks; auth and executable commands first, with a test-only third descriptor proving reuse. |

## Registries to change

| Registry | Change | Phase 0 item |
|---|---|---|
| `component.Register` | Error on duplicate for external registrants; register terraform, helmfile, packer; replace the closed type and section lists with registry reads | P0.3, P0.6 |
| `internal.Register` (commands) | Collision detection across built-in, YAML, and plugin commands; plugin shortcut unavailable on conflict | P0.3 |
| `hooks.RegisterKind` | Add `Context` to `ExecContext`; keep `Cmd` Atmos-side; finding adapter to `validation.Diagnostic` | P0.4 |
| `step.Register` | Error on external collisions including aliases; `KnownFields` equivalent from manifest metadata | P0.3, P0.11 |
| `ci.Register` | Ordered candidate detection; `ci.provider` selector; error on duplicate | P0.3, P0.7 |
| `ci.RegisterPlugin` | Public aliases for `Plugin`, `HookBinding`, `HookContext` | P0.5 |
| `store.Register` | Context on `Store`; capability metadata from registration instead of two hardcoded lists; generic auth context | P0.4 |
| `providers.Register` (secrets) | Context on `Provider`; kind-keyed generic track; `provider:` selector | P0.4, P0.9 |
| `provisioner.RegisterProvisioner` | Deduplicate by type and event | P0.3 |
| `backend.Register*` | Replaced by N7 | P0.10 |
| `manifest.Register` | Error on duplicate kind | P0.3 |
| `registry.Register*` (auth) | Export registration; error on duplicate; all built-ins registered; switch removed | P0.1 |
| `tools.Registry` | Optional `JSONSchema()`; fix `ListByCategory` | none |
| `skills.LoadSkills` | Plugin-owned loader stage; `plugin://` source | none |

## Missing injection points

Grouped by subsystem. Each is a place where Atmos code must consult the installed-plugin registry or its metadata.

**Bootstrap** ([`pkg/config`](../../../../pkg/config))
- Load `plugins:` and `.atmos.plugins.lock.yaml` after `atmos.yaml` and its imports, before auth, stores, or stack processing.
- Resolve the installed-plugin registry from metadata only: no network, no subprocess, no Starlark evaluation.
- Register static extension metadata (kinds, types, commands, tools, skills, checks) so validation and help work without execution.

**Commands** ([`cmd/root.go`](../../../../cmd/root.go), [`cmd/cmd_utils.go`](../../../../cmd/cmd_utils.go))
- Insert plugin command adapters between `RegisterAll` and `processCustomCommands`.
- Record shortcut collisions; always expose `atmos plugin run`.

**Auth** ([`pkg/auth/registry`](../../../../pkg/auth/registry), [`pkg/auth/credentials`](../../../../pkg/auth/credentials))
- Register verified plugin provider/identity constructors during bootstrap; `NewProvider`/`NewIdentity` use the same registry as built-ins, without a separate plugin fallback.
- Keyring encode/decode → credential-type registry.
- Chain validation checks declared accepted and produced credential types.

**Components** ([`pkg/component`](../../../../pkg/component), [`internal/exec`](../../../../internal/exec), [`pkg/list/extract`](../../../../pkg/list/extract))
- `GetProvider` miss → plugin engine.
- `GetComponentConfig`, `autoDetectComponentTypes`, `typeEntries`, `getComponentBasePath`, and the list extractors read registered types.
- Stack processor treats registered external types through the custom-type path, including `metadata.inherits`.
- Dependency graphs and `describe affected` resolve types through the registry.

**CI** ([`pkg/ci`](../../../../pkg/ci))
- `ResolveProvider`: explicit selector, then ordered candidates including plugins.
- `GetPluginForEvent`: plugin `ci.component` extensions.
- An `api.CIReporter` service carries reporting requests to the selected provider.

**Automation** ([`pkg/hooks`](../../../../pkg/hooks), [`pkg/runner/step`](../../../../pkg/runner/step), [`pkg/api`](../../../../pkg/api))
- `GetKind` miss and `Get` miss → plugin kinds and types.
- Workflow, custom-command, and hook validation read plugin option schemas.
- `StepLibrary.Names()` includes plugin types.

**Validation** ([`internal/exec/validate_component.go`](../../../../internal/exec/validate_component.go), [`pkg/validation`](../../../../pkg/validation))
- Component validation dispatch through check descriptors.
- Root `atmos validate` runs `validate.checks`; new `atmos validate artifact`.

**Data access** ([`pkg/store`](../../../../pkg/store), [`pkg/secrets/providers`](../../../../pkg/secrets/providers))
- `NewStoreRegistry` kind miss → plugin store.
- `providers.New` generic track → plugin kind.

**Provisioning** ([`pkg/provisioner/backend`](../../../../pkg/provisioner/backend))
- Lookup by `provision.backend.kind`, falling back to `backend_type`.

**AI** ([`pkg/ai/tools/atmos/setup.go`](../../../../pkg/ai/tools/atmos/setup.go), [`pkg/mcp`](../../../../pkg/mcp), [`pkg/ai/skills`](../../../../pkg/ai/skills))
- `RegisterTools` adds plugin tools; adapter passes raw schemas; lazy start on `Execute`.
- `LoadSkills` plugin stage; `plugin://` source.

**Toolchain** ([`pkg/dependencies`](../../../../pkg/dependencies), [`pkg/toolchain/lockfile`](../../../../pkg/toolchain/lockfile))
- Plugin `dependencies.tools` restore through `EnsureTools`; scoped PATH per invocation; plugin lock references exact `toolchain.lock.yaml` entries.

**Starlark** ([`pkg/script/starlark/engine.go`](../../../../pkg/script/starlark/engine.go))
- `predeclared()` gains `plugin`, `auth`, `validation`, and `hooks` modules for plugin sessions only and extends `atmos` with the API services.
- One evaluation session per package per process; isolated thread per invocation; existing `AfterFunc` cancellation.

**OCI** ([`pkg/oci`](../../../../pkg/oci))
- Artifact (non-image) push, index creation, tag listing, manifest deletion, and referrers on top of go-containerregistry; existing `ResolveImage`, `PinDigest`, `GHCRAuth` reused.

**Schemas** ([`pkg/datafetcher/schema`](../../../../pkg/datafetcher/schema), [`pkg/config/schema`](../../../../pkg/config/schema))
- `plugins:` in the generated `atmos.yaml` schema; stack schema for `provision.backend.kind`, `secrets.providers`, `settings.validation.<name>.validator`, `ci.provider`, `validate.checks`.

## Internal types that must not cross the boundary

| Type | Where it leaks today | Replacement on the wire |
|---|---|---|
| `types.AuthManager` | `Provider.PreAuthenticate`, `PostAuthenticateParams.Manager` | `api.Credentials` and `api.Prompt` |
| `*schema.Identity`, `*schema.Provider` | `ConfigSetter.SetConfig`, switch constructors | Validated `spec`/`principal` maps plus identity name and realm |
| `*schema.ConfigAndStacksInfo`, `*schema.AtmosConfiguration` | `component.ExecutionContext`, `hooks.ExecContext`, `ProvisionerFunc`, secrets `Constructor` | Resolved component values, stack coordinates, working directory, scoped environment |
| `*cobra.Command` | `hooks.ExecContext.Cmd` | Command name and arguments |
| `OutputWriters`, `OutputWriter`, `io.Writer` | provisioners, CI providers, steps | Streamed output chunks and `api.Log` and `api.CIReporter` |
| `AuthContextResolver` (per-cloud) | `IdentityAwareStore` | Generic scoped identity reference resolved by Atmos |
| `HookHandler func(*HookContext)` | component CI plugins | `CIComponent` service with structured outcome and reporting requests |

## Follow-on surfaces

One row each; none is an initial-release commitment. Columns match the summary.

| Surface | Existing registry | Duplicate policy | Gap to a public contract |
|---|---|---|---|
| Generators | [`generator`](../../../../pkg/generator/registry.go) | Replace | Ordering and file ownership between generators |
| YAML functions | [`function`](../../../../pkg/function/registry.go) | Replace | Sensitive/unknown values, caching, evaluation phases, cycles |
| Import sources | [`import_adapter_registry`](../../../../pkg/config/import_adapter_registry.go) | Replace | Bootstrap must locate plugins before imports need them |
| Artifact-delivery targets | [`target`](../../../../pkg/provisioner/target/target.go) | Replace | `Deliver`/`Fetch` scoping and credentials |
| Tool registries and version resolvers | [`toolchain/registry`](../../../../pkg/toolchain/registry/registry.go), [`version/resolver`](../../../../pkg/version/resolver/resolver.go) | Factory setters | Trust and checksum provenance for external sources |
| Git and PR providers | [`git`](../../../../pkg/git/registry.go), [`pull_request.go`](../../../../pkg/git/pull_request.go) | Replace | Separate from CI selection |
| CI artifacts and caches | [`artifact`](../../../../pkg/ci/artifact/registry.go), [`cache`](../../../../pkg/ci/cache/registry.go) | Panic/replace | Blob retention and restore semantics |
| Script engines and emulators | [`script`](../../../../pkg/script/registry.go), [`emulator`](../../../../pkg/emulator/driver.go) | Replace (engine) | Language value isolation; emulator lifecycle |
| AI model providers | [`ai/registry`](../../../../pkg/ai/registry/registry.go) | Replace | Streaming and accounting |
| Auth integration adapters | [`auth/integrations`](../../../../pkg/auth/integrations/registry.go) | Replace | Credential consumers beyond chains |
| Format handlers | [`hooks/format.go`](../../../../pkg/hooks/format.go) | Error | Shared rendering first |
| Terraform state readers | [`terraform_backend`](../../../../internal/terraform_backend/terraform_backend_registry.go) | Closed map | Only needed if an engine reads foreign state |
| Wasm runtime | none (`extism`, `wazero` indirect) | n/a | Extism host functions mirroring the API services |

## Contract testing

The plugin boundary is not a Pact candidate. Atmos consumes plugin services from an open set of third-party providers, and consumer-driven contracts cannot be verified against providers the project does not control. The contract is protobuf, so schema compatibility is a `buf breaking` gate, and behavioral conformance is the Atmos-shipped suite in [testing](../testing.md) plus pinned fixture plugins for old protocol versions. The existing pact-go consumer tests for the Atmos Pro HTTP API ([`pkg/pro`](../../../../pkg/pro), `pact` build tag, `pacts/atmos-AtmosPro.json`) are unaffected. See [Decision 15](decisions.md).
