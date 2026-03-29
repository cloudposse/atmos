# Atmos MCP Servers — External MCP Server Management

**Version:** 1.0
**Last Updated:** 2026-03-28

---

## Executive Summary

Atmos MCP Servers extends the `atmos mcp` command to support connecting to and consuming
**external MCP servers** — bringing the same MCP client capability found in Claude Code,
Gemini CLI, and AI IDEs directly into the Atmos CLI.

Instead of reimplementing cloud provider functionality (AWS APIs, GCP APIs, Azure APIs),
Atmos installs and orchestrates existing MCP servers from the ecosystem — like the
20+ AWS MCP servers from `awslabs/mcp` — and exposes their tools alongside native Atmos
tools in a unified interface.

**Key Insight:** The Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`) that
Atmos already depends on has full client support (`mcp.NewClient`, `CommandTransport`,
`ClientSession`). No new dependencies are needed.

### Why This Matters

1. **Leverage the ecosystem** — 100+ MCP servers exist for AWS, GCP, Azure, databases,
   monitoring, CI/CD. Reimplementing this is wasted effort.
2. **Parity with AI tools** — Claude Code, Cursor, Windsurf all manage MCP servers.
   Atmos should too.
3. **Speed** — Installing an AWS MCP server takes seconds. Building equivalent
   functionality takes weeks.
4. **Composability** — Users can mix native Atmos tools (describe stacks, validate) with
   external tools (AWS CloudFormation, EKS, S3) in the same AI conversation.

---

## Two Approaches

Atmos supports two complementary approaches for using external MCP servers:

| | Atmos AI Integration | IDE/Claude Code Integration |
|---|---|---|
| **Target user** | Uses `atmos ai ask/chat/exec` | Uses Claude Code / Cursor / IDE |
| **Config location** | `atmos.yaml` under `mcp.servers` | `.mcp.json` + custom commands in `.atmos.d/` |
| **Server lifecycle** | Atmos manages (spawn, bridge, call) | IDE manages (via `.mcp.json`) |
| **Auth** | `identity` field on server config | `atmos auth exec -i <identity> --` wraps subprocess |
| **Tool invocation** | AI executor → BridgedTool → `Session.CallTool` | IDE → stdio → MCP server directly |
| **Discovery** | `atmos mcp list/tools/test/status` | Manual (IDE-driven via generated .mcp.json and `atmos mcp test <name>`) |

Both approaches coexist. `atmos.yaml` is the single source of truth:
`atmos mcp generate-config` emits a `.mcp.json` from the configured servers,
wrapping each with `atmos auth exec` for credential injection.

### Unified Experience

From a single `atmos.yaml` config:

```yaml
mcp:
  servers:
    aws-pricing:
      command: uvx
      args: ["awslabs.aws-pricing-mcp-server@latest"]
      env:
        AWS_REGION: "us-east-1"
      identity: "readonly"   # Atmos Auth identity (from the auth section)
      description: "AWS Pricing"
```

Users get:

- `atmos ai ask/chat/exec` — server tools appear alongside native Atmos tools
- `atmos mcp test aws-pricing` — tests connectivity and authentication
- `atmos mcp generate-config` — emits `.mcp.json` for Claude Code / IDE
- `atmos mcp list` — shows all configured servers with status

The `.mcp.json` generation wraps servers with `atmos auth exec`:

```json
{
  "mcpServers": {
    "aws-pricing": {
      "command": "atmos",
      "args": ["auth", "exec", "-i", "readonly", "--",
               "uvx", "awslabs.aws-pricing-mcp-server@latest"],
      "env": { "AWS_REGION": "us-east-1" }
    }
  }
}
```

---

## AWS MCP Servers — Primary Use Case

The `awslabs/mcp` repository provides 20+ MCP servers covering the AWS ecosystem:

| Server            | Package                                | Purpose                                |
|-------------------|----------------------------------------|----------------------------------------|
| AWS MCP Server    | `awslabs.aws-mcp-server`               | Comprehensive AWS API access (preview) |
| Amazon EKS        | `awslabs.amazon-eks-mcp-server`        | EKS cluster management                 |
| Amazon ECS        | `awslabs.amazon-ecs-mcp-server`        | ECS service management                 |
| AWS IaC           | `awslabs.aws-iac-mcp-server`           | CloudFormation/CDK operations          |
| Amazon S3         | `awslabs.s3-mcp-server`                | S3 bucket operations                   |
| DynamoDB          | `awslabs.dynamodb-mcp-server`          | DynamoDB table operations              |
| AWS Serverless    | `awslabs.aws-serverless-mcp-server`    | SAM CLI operations                     |
| Lambda Tool       | `awslabs.lambda-tool-mcp-server`       | Lambda function management             |
| AWS Support       | `awslabs.aws-support-mcp-server`       | AWS Support cases                      |
| AWS Documentation | `awslabs.aws-documentation-mcp-server` | AWS docs search                        |
| Amazon Bedrock    | `awslabs.amazon-bedrock-mcp-server`    | Bedrock model operations               |
| AWS Knowledge     | `awslabs.aws-knowledge-mcp-server`     | AWS knowledge base search              |
| Billing & Cost    | `awslabs.billing-cost-management-mcp-server` | Billing, cost explorer, forecasts |
| AWS Pricing       | `awslabs.aws-pricing-mcp-server`       | On-demand/reserved pricing             |
| CloudWatch        | `awslabs.cloudwatch-mcp-server`        | Metrics, logs, alarms                  |
| CloudTrail        | `awslabs.cloudtrail-mcp-server`        | Event history and auditing             |
| IAM               | `awslabs.iam-mcp-server`              | Role/policy analysis                   |
| Well-Arch Security| `awslabs.well-architected-security-mcp-server` | Security posture assessment  |
| Network           | `awslabs.aws-network-mcp-server`       | VPC/subnet/route analysis              |

**Installation:** All use `uvx` (Python's `uv` package manager): `uvx awslabs.package@latest`

**Transport:** All use **stdio** (subprocess spawned, JSON-RPC over stdin/stdout).

---

## Configuration

### atmos.yaml

```yaml
mcp:
  servers:
    aws-billing:
      description: "AWS Billing — summaries, cost explorer, and forecasts"
      command: "uvx"
      args: ["awslabs.billing-cost-management-mcp-server@latest"]
      env:
        AWS_REGION: "us-east-1"
      identity: "readonly"   # Atmos Auth identity (from the auth section)
      timeout: "30s"

    aws-docs:
      description: "AWS Documentation — search and fetch docs"
      command: "uvx"
      args: ["awslabs.aws-documentation-mcp-server@latest"]
```

### Server Config Fields

**Standard MCP fields** (compatible with Claude Code, Codex CLI, Gemini CLI):

- `command` — Command to run the server (e.g., `uvx`, `npx`, or an absolute path).
- `args` — Command arguments (e.g., `["awslabs.aws-pricing-mcp-server@latest"]`).
- `env` — Environment variables passed to the subprocess. Supports YAML functions
  (`!env`, `!exec`, `!repo-root`, `!cwd`).

**Atmos extensions:**

- `description` — Human-readable description shown in `atmos mcp list` and `atmos mcp status`.
- `identity` — Atmos Auth identity (from the `auth` section) for credential injection.
- `auto_start` — Start the server automatically when Atmos starts.
- `timeout` — Connection timeout as a Go duration string (default: `30s`).

### Toolchain Integration

```yaml
toolchain:
  aliases:
    uv: astral-sh/uv
```

```bash
atmos toolchain install astral-sh/uv@0.7.12
```

This ensures `uvx` is available before any AWS MCP server is started.

---

## CLI Commands

```bash
atmos mcp start              # Start Atmos as an MCP server (server mode)
atmos mcp list               # List configured external MCP servers
atmos mcp status             # Show live status of all servers
atmos mcp tools <name>       # List tools from a server
atmos mcp test <name>        # Test connectivity to a server
atmos mcp restart <name>     # Restart a server
atmos mcp generate-config    # Generate .mcp.json for Claude Code / IDE
```

---

## Architecture

### Package Structure

```text
pkg/mcp/
├── server.go              # Atmos MCP server (exposes Atmos tools to IDEs)
├── adapter.go             # Atmos tool → MCP bridge
├── client/
│   ├── config.go          # Server configuration parsing and validation
│   ├── session.go         # MCP client session (subprocess lifecycle, handshake, tools)
│   ├── manager.go         # Multi-session manager (start/stop/list)
│   ├── bridge.go          # External MCP tool → Atmos tools.Tool bridge (BridgedTool, BridgedToolInfo)
│   └── register.go        # Starts servers and registers tools in AI registry
└── router/
    └── router.go          # Smart server routing (two-pass selection)
```

### Tool Execution Flow

The tool invocation loop lives in `pkg/ai/executor/executor.go`:

1. Atmos sends the prompt + list of available tools (native + MCP) to the AI provider.
   Each tool includes its **name**, **description**, and **full parameter schema**.
2. AI responds with either:
   - `StopReasonEndTurn` — answered directly, no tools needed.
   - `StopReasonToolUse` + `ToolCalls` array — AI explicitly requests specific tools.
3. Atmos executes the requested tools via `executeTools()`.
4. Results are recorded in `result.ToolCalls`.
5. Results sent back to AI for the final answer.
6. The markdown formatter renders the "Tool Executions" section from `result.ToolCalls`.

The AI provider controls which tools are called. Atmos never guesses or infers tool usage.

### MCP Tool Call Routing

When the AI provider decides to use an external MCP tool:

1. AI responds with `tool_use` requesting e.g. `aws-docs__search_documentation` (sanitized name).
2. Tool executor looks up the sanitized name in the registry → finds `BridgedTool`.
3. `BridgedTool.Execute()` calls `session.CallTool()` using the **original tool name**
   (`"search_documentation"`, without the server prefix).
4. `Session.CallTool()` sends JSON-RPC over stdio to the running MCP server subprocess.
5. The MCP server executes the tool and returns the result over stdio.
6. `BridgedTool.Execute()` extracts text content and returns it as `tools.Result`.
7. The executor sends the result back to the AI provider for the final answer.

Each `BridgedTool` holds a reference to the specific `Session` it came from. Even with
multiple MCP servers running, each tool routes to the correct server process. The namespacing
(`aws-docs__search_documentation`) is only for registry lookup — the actual MCP call uses
the original tool name that the server understands. Tool names are sanitized to match
AI provider constraints (`^[a-zA-Z0-9_-]{1,128}$`), and displayed in human-readable
format (`aws-docs → search_documentation`) via the `BridgedToolInfo` interface.

### Auth Integration

AWS MCP servers need credentials. With `identity`, Atmos Auth handles this automatically:

1. Atmos Auth resolves the identity through the provider chain (SSO → role assumption).
2. Credentials are written to isolated files at `~/.aws/atmos/<realm>/`.
3. The MCP server subprocess gets environment variables:
   - `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AWS_PROFILE`
   - `AWS_REGION`, `AWS_SDK_LOAD_CONFIG=1`, `AWS_EC2_METADATA_DISABLED=true`
4. The server's AWS SDK picks up credentials automatically.

No manual `AWS_PROFILE` setup, no SSO login prompts during execution, credentials scoped
per-identity and isolated per-realm.

### User-Facing Feedback

MCP server startup and tool execution are visible at Info log level:

```text
ℹ MCP routing selected 2 of 8 servers: aws-docs, aws-pricing
ℹ MCP server "aws-docs" started (4 tools)
ℹ MCP server "aws-pricing" started (7 tools)
ℹ Registered 11 tools from 2 MCP server(s)
ℹ AI tools initialized: 26 total
```

After the AI responds, tool executions are listed with error details:

```text
---
## Tool Executions (2)
1. ✅ aws-docs → aws.search_documentation (234ms)
2. ❌ aws-pricing → get_pricing (1234ms)
   Error: MCP server returned error for tool: credentials expired
```

---

## Implementation Summary

All phases are implemented and shipped.

### Phase 1: Core Client Infrastructure ✅

- `pkg/mcp/client/session.go` — Session lifecycle: Start (subprocess + MCP handshake + tool list), Stop, CallTool, Ping
- `pkg/mcp/client/manager.go` — Multi-session manager: NewManager, Start/Stop/StopAll, Get/List, Test
- `pkg/mcp/client/config.go` — Config parsing with validation and timeout resolution
- `cmd/mcp/client/list.go` — `atmos mcp list` themed table
- `cmd/mcp/client/tools.go` — `atmos mcp tools <name>` connect, list, disconnect
- `cmd/mcp/client/test_cmd.go` — `atmos mcp test <name>` full lifecycle test

### Phase 2: Tool Bridge + AI Integration ✅

- `pkg/mcp/client/bridge.go` — `BridgedTool` implements `tools.Tool` interface with JSON Schema extraction
- `pkg/mcp/client/register.go` — `RegisterMCPTools` starts servers and registers tools in AI registry
- `cmd/ai/init.go` — MCP tools registered in both interactive (`chat`, `exec`) and non-interactive (`ask`) paths

### Phase 3: Management Commands ✅

- `cmd/mcp/client/status.go` — `atmos mcp status` health table
- `cmd/mcp/client/restart.go` — `atmos mcp restart <name>` stop + start cycle

### Phase 4: Auth + Toolchain ✅

- `identity` field on `MCPServerConfig` with `AuthEnvProvider` interface
- `WithAuthManager` and `WithToolchain` start options for credential and binary resolution
- YAML functions (`!env`, `!exec`, `!repo-root`, `!cwd`) work in env values via `preprocessAtmosYamlFunc`

### Phase 5: Unified Experience ✅

- Auth identity wired in AI commands (creates `AuthEnvProvider` when servers need auth)
- `atmos mcp generate-config` — emits `.mcp.json` for IDE integration
- User-facing feedback via `ui.Info`/`ui.Error` for server startup and tool execution
- Error details displayed for failed tool calls (credential failures, server errors)

### Phase 6: Smart Tool Routing ✅

- `pkg/mcp/router/router.go` — Two-pass server selection using configured AI provider
- `cmd/ai/init.go` — Server filtering via routing or `--mcp` flag before `RegisterMCPTools`
- `--mcp` flag on `ask`, `chat`, `exec` commands with `ATMOS_AI_MCP` env var support
- `pkg/schema/mcp.go` — `MCPRoutingConfig` with `enabled` field (defaults to true)
- Graceful fallback to all servers on routing failure

---

## Security Considerations

1. **Process isolation** — Each MCP server runs as a separate subprocess with its own environment.
2. **Environment scoping** — Environment variables are explicitly configured per server.
3. **Permission model** — External MCP tools use the Atmos AI permission system (Allow/Prompt/YOLO).
4. **Transport security** — stdio transport is local-only (no network exposure).
5. **Supply chain** — MCP servers installed via package managers (`uvx`, `npx`) with version pinning.

---

## Phase 6 — Smart Tool Routing

### Problem

When many MCP servers are configured (e.g., 8 servers, 96 tools), the full tool schema
payload overwhelms the AI context — increasing latency, cost, and reducing response quality.
Most queries only need 1-2 servers.

### Solution

Two-pass routing with manual override:

- **Default:** Before starting MCP servers, send the question + server descriptions to
  the user's configured AI provider. The AI returns which servers are relevant. Only those
  servers are started, keeping the tool payload small.
- **Override:** `--mcp` flag on `ask`, `chat`, `exec` for direct server selection.

### Routing Flow

```
atmos ai ask "List unused IAM roles"

1. If --mcp flag provided → start only specified servers
2. Else if single server → start it directly (no routing)
3. Else if routing disabled → start all servers
4. Else if no question (chat mode) → start all servers
5. Else → routing call to select servers → start only selected
6. On routing error → fall back to all servers
```

### Configuration

```yaml
mcp:
  routing:
    enabled: true   # Default: true. Uses the same AI provider and model from ai.default_provider.
```

### CLI

```bash
# Automatic routing (default)
atmos ai ask "List unused IAM roles"

# Manual override
atmos ai ask --mcp aws-iam,aws-cloudtrail "List unused IAM roles"

# Works with all AI subcommands
atmos ai chat --mcp aws-billing
atmos ai exec --mcp aws-security,aws-iam "audit our security posture"

# Environment variable
ATMOS_AI_MCP=aws-iam,aws-billing atmos ai ask "List admin roles"
```

### Design Decisions

- **Graceful fallback:** Routing failures start all servers. Never blocks the user.
- **Single-server optimization:** Routing skipped when only one server is configured.
- **Same model, reduced tokens:** Uses the user's configured provider/model with
  `max_tokens` reduced to 256. No extra model configuration needed.
- **Upstream filtering:** Server selection happens before `RegisterMCPTools`.
- **Chat mode:** Routing skipped because the question isn't known upfront. Use `--mcp`.

---

## Future Considerations

- Stack-level MCP server overrides (per-stack `settings.mcp.servers` config)
- Composite MCP server (expose external tools via Atmos MCP server to IDEs)
- Connection pooling and health checks with auto-restart on failure
- `tools/list_changed` notification handling for dynamic tool updates
