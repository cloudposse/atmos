# Command extensions

**Status:** Initial release for executable commands; Starlark command support follows later. [All extension types](README.md).

A plugin can add a top-level command tree alongside any other extensions. The [Okta package example](../package-composition.md) combines `atmos okta …` with authentication, MCP tools, and a skill under one version.

## Contract

- Static metadata supplies names, help, usage, and discoverable subcommands without starting a plugin.
- Native commands use the [`Commands.Run`](../platform/protocol.md#commands) server-streaming RPC through go-plugin. The same plugin binary may implement commands and other services. Starlark entrypoints are a proposed follow-on.
- Preserve argument boundaries and `--` without shell interpolation. Stdout and stderr arrive as events. The native process starts in the package directory; command bootstrap currently leaves `Invocation.working_directory` unset. Exactly one terminal exit event followed by EOF determines the Atmos exit code, and cancellation follows the RPC context. The current wire contract has no stdin stream or TTY attachment.
- Commands enter the existing command registry. They cannot replace built-in commands or YAML custom commands. Today no registrant is prevented from doing so; the plugin adapter enforces this for plugins, and the collision policy in [Decision 8](../platform/decisions.md) applies.
- A conflicting shortcut is unavailable and diagnosed; explicit invocation through `atmos plugin run <package-alias> -- <arguments>` remains available. Packages with several command trees additionally select `--command <export-name>`.
- Multiple capabilities share a package version and implementation code. Current command bootstrap grants `api.HTTP`, but not an auth provider's private credential namespace. Credential access for commands requires a separately scoped API contract; it is not implied by sharing a binary.
- Tool dependencies use Atmos toolchain; normal command dispatch does not install or upgrade packages/tools.

## Authoring and tests

```shell
atmos plugin create acme-tools --capability command --runtime go
```

Command contract tests cover help without execution, literal argument forwarding, streamed output, exit propagation, cancellation, missing executables, platform selection, and collisions. Stream tests reject missing or duplicate terminal events and output after exit. Test both top-level and qualified invocation of the same implementation. Go examples must be runnable without an Atmos source checkout; Starlark scaffolding and examples follow the planned runtime.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`internal.Register(provider CommandProvider)`](../../../../cmd/internal/registry.go) and `RegisterAll(root)`. |
| Duplicate policy | Silently overwrites the existing registration ("plugin override functionality"). |
| Context on the interface | Not applicable; `CommandProvider` returns a `*cobra.Command` and builders. |
| Reachable from outside the module | No. `cmd/internal` is Go-internal, so the adapter lives in `cmd/plugin`, the only bridge. |
| Closed lists to open | None. YAML custom commands are processed after `RegisterAll` by `processCustomCommands` in [`cmd/cmd_utils.go`](../../../../cmd/cmd_utils.go) and can extend or override built-ins. |
| Boundary leaks | [`CommandProvider`](../../../../cmd/internal/command.go) (`GetCommand`, `GetFlagsBuilder`, `GetPositionalArgsBuilder`, `GetCompatibilityFlags`, and others) is built on `*cobra.Command`. |

No `atmos-<name>` executable discovery exists. The closest precedent is the top-level `aliases:` configuration, which re-execs Atmos as a subprocess.

**Adapter:** CLI command registration reads static manifest metadata and dispatches through `pkg/plugin/bootstrap.Run`, which starts the native runtime and invokes `Commands.Run`. `pkg/plugin/runtime/native` validates stream chunks and terminal exit status before returning. Output goes to caller-provided writers; callers provide masked writers for terminal output. The process does not inherit an interactive command terminal. Starlark command bindings and their API services remain proposed.

**Injection points:**
- `cmd/root.go`: insert plugin command adapters between `RegisterAll` and `processCustomCommands`.
- Collision recording: when a plugin shortcut conflicts with a built-in or YAML command, record the collision and leave the shortcut unavailable.
- `atmos plugin run <alias> -- ...`: always dispatches, regardless of collisions.
- Help rendering: reads manifest metadata without starting the plugin.

**Prerequisites:** P0.3 Duplicate policy (collision detection across built-in, YAML, and plugin commands); Phase 3 work package WP3.2 (commands). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `*cobra.Command`: replaced by the command name and literal arguments.
- `*schema.ConfigAndStacksInfo` and `*schema.AtmosConfiguration`: replaced by resolved values and a scoped environment supplied by Atmos.
