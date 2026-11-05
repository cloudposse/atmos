# MCP tool extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

A plugin may expose tools through the existing Atmos MCP server, alongside commands, identities, and other extensions. It does not require another separately installed MCP server.

## Consumer and developer experience

```shell
atmos plugin install acme/okta --as okta --version '~1.0'
atmos plugin info okta
atmos mcp start
```

The server builds its tool catalog from effective project plugins and Atmos exposure policy. Tools have stable names such as `acme.okta.list_apps`; descriptions and schemas come from verified package metadata, without starting plugin processes. One package may also ship a [skill](skills.md) explaining when and how to use these tools.

```shell
atmos plugin create acme-okta --capability command --capability auth.identity --capability mcp.tool --capability skill --runtime go
```

Repeated capability flags scaffold one repository and manifest. The tool scaffold includes input/output schemas, a callback, mocks, a protocol client test, and example consumer configuration.

## Contract and integration

Adapt the [Atmos tool registry](../../../../pkg/ai/tools/registry.go), [MCP adapter](../../../../pkg/mcp/adapter.go), and [server](../../../../pkg/mcp/server.go). Tool calls use the existing executor and permissions path. Plugins implement a versioned tool request/result contract through Go or Starlark; Atmos owns MCP transport and negotiation.

MCP tools have names, descriptions, JSON input schemas, optional output schemas, and structured/text results. Names must be unique and use the negotiated protocol's supported syntax. Side-effect annotations are descriptive metadata, not authorization. [MCP tools specification](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)

- Validate inputs before execution and outputs before returning success. Preserve structured data through the adapter; do not stringify it into an untyped report.
- Resolve credentials through Atmos; never include tokens in descriptions, schemas, skill text, or ordinary tool results.
- Installed tools are available for Atmos policy to expose. Unrelated global integrations cannot appear in a project's server. Effective allow/deny/confirmation policy applies at invocation, including native handlers.
- Tool names cannot replace built-ins or other plugins. Resolve collisions before exposing a tool list.
- Propagate cancellation, deadlines, progress where supported, and error classification. Keep diagnostic output off MCP protocol streams.
- Start plugins lazily on tool invocation. Listing tools neither executes plugins nor installs dependencies.
- A server session pins its effective package selection. Updating a package requires a server restart in the first MCP adapter release; retain artifacts used by active sessions. Do not mix old schemas with new handlers.

MCP resources/prompts and arbitrary external-server packaging are follow-ons. Skills are their own content extension; shipping a skill does not implicitly create MCP prompts or authorize tool calls.

## Integration

| Aspect | Today |
|---|---|
| Registry | Instance-based [`tools.Registry`](../../../../pkg/ai/tools/registry.go) from `tools.NewRegistry()`. Built-ins register in [`atmos.RegisterTools`](../../../../pkg/ai/tools/atmos/setup.go); external MCP servers through [`client.RegisterMCPTools`](../../../../pkg/mcp/client/register.go). |
| Duplicate policy | `Register(tool Tool) error` errors on duplicates. |
| Context on the interface | Yes. `Execute(ctx, params)` takes `ctx`. |
| Reachable from outside the module | Yes. |
| Closed lists to open | `ListByCategory` ignores its category argument (a TODO in the code); it is a trivial fix, not a Phase 0 item. |
| Boundary leaks | None of the internal types listed in the gap analysis. `Parameter{Name, Description, Type, Required, Default}` is flat, so the [MCP server](../../../../pkg/mcp/server.go) derives JSON Schema from it in `generateInputSchema`; nested or constrained plugin schemas cannot be expressed. |

The execution path is `tools.NewExecutor(registry, *permission.Checker, timeout)`, then [`permission.Checker.CheckPermission`](../../../../pkg/ai/tools/permission/checker.go), then `mcp.NewAdapter(registry, executor)` and `mcp.NewServer`. The [tool interface](../../../../pkg/ai/tools/types.go) is `Name`, `Description`, `Parameters`, `Execute`, `RequiresPermission`, and `IsRestricted`.

**Adapter:** `pkg/plugin/adapter/mcptool` implements `tools.Tool` and calls the `MCPTool` wire service. An optional `JSONSchema()` interface lets the server pass plugin input schemas through unchanged instead of deriving them from `Parameters()`.

**Injection points:**
- `atmos.RegisterTools`: adds plugin tools from verified manifest metadata, so listing never starts a plugin.
- `Server.generateInputSchema`: prefers `JSONSchema()` when the tool implements it.
- `Tool.Execute`: starts the plugin lazily on first invocation.
- The permission checker applies unchanged to plugin tools.

**Prerequisites:** none in Phase 0; the `ListByCategory` fix accompanies the follow-on MCP adapter and does not block Phase 0; Phase 3 work package WP3.8 (MCP and skills). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `[]tools.Parameter`: replaced by a raw JSON Schema for inputs and an optional output schema.
- `*permission.Checker`: stays Atmos-side; plugins never see permission state or approval prompts.

## Acceptance

An MCP test client lists a package's static tools, calls Go/Starlark fixture handlers, validates structured results, and observes cancellation/errors. Test policy denial, conflicting names, malformed schemas/results, sensitive-value masking, session version pinning, update/restart, and no process launches during listing. The same plugin release supplies commands and identities without a second installation.
