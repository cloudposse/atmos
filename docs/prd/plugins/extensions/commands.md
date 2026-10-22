# Command extensions

**Status:** Initial release for executable commands; Starlark command support follows later. [All extension types](README.md).

A plugin can add a top-level command tree alongside any other extensions. The [Okta package example](../package-composition.md) combines `atmos okta …` with authentication, MCP tools, and a skill under one version.

## Contract

- Static metadata supplies names, help, usage, and discoverable subcommands without starting a plugin.
- A command may dispatch an ordinary executable or Starlark entrypoint. Plain executable dispatch does not require RPC; a Go package may use one binary with separate command and plugin modes.
- Preserve argv boundaries, `--`, stdin, stdout/stderr, working directory, cancellation, and exit status. Do not interpolate arguments into a shell command.
- Commands enter the existing command registry. They cannot replace built-in commands or YAML custom commands. Today no registrant is prevented from doing so; the plugin adapter enforces this for plugins, and the collision policy in [Decision 8](../platform/decisions.md) applies.
- A conflicting shortcut is unavailable and diagnosed; explicit invocation through `atmos plugin run <package-alias> -- <arguments>` remains available. Packages with several command trees additionally select `--command <export-name>`.
- Multiple capabilities share a package version and implementation code. A command needing credentials uses `api.Credentials` rather than a separate credential cache.
- Tool dependencies use Atmos toolchain; normal command dispatch does not install or upgrade packages/tools.

## Authoring and tests

```shell
atmos plugin create acme-tools --capability command --runtime go
atmos plugin create acme-scripts --capability command --runtime starlark
```

Generated tests verify help without execution, literal argument forwarding, streamed output, input, exit propagation, interruption, missing executables, platform selection, and collisions. Test both top-level and qualified invocation of the same implementation. Executable and Starlark examples must be runnable without an Atmos source checkout.

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

**Adapter:** `cmd/plugin` hosts `pkg/plugin/adapter/command`, which builds `CommandProvider` values from static manifest metadata. Executable commands need no wire service; the process inherits stdio. Starlark commands use the `api.Process` and `api.FS` API services.

**Injection points:**
- `cmd/root.go`: insert plugin command adapters between `RegisterAll` and `processCustomCommands`.
- Collision recording: when a plugin shortcut conflicts with a built-in or YAML command, record the collision and leave the shortcut unavailable.
- `atmos plugin run <alias> -- ...`: always dispatches, regardless of collisions.
- Help rendering: reads manifest metadata without starting the plugin.

**Prerequisites:** P0.3 Duplicate policy (collision detection across built-in, YAML, and plugin commands); Phase 3 work package WP3.2 (commands). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `*cobra.Command`: replaced by the command name and literal arguments.
- `*schema.ConfigAndStacksInfo` and `*schema.AtmosConfiguration`: replaced by resolved values and a scoped environment supplied by Atmos.
