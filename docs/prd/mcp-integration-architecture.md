# PRD: Model Context Protocol (MCP) Integration Architecture

**Status**: Draft
**Created**: 2025-10-23
**Author**: AI Development Team
**Related**: `ai-implementation-plan.md`

## Executive Summary

This document defines the architecture and implementation strategy for exposing Atmos AI capabilities through the Model Context Protocol (MCP), enabling standardized integration with any MCP-compatible client (Claude Desktop, Claude Code, VSCode, etc.).

## Background

### What is MCP?

The **Model Context Protocol (MCP)** is an open standard created by Anthropic (announced November 2024) that standardizes how AI systems connect to external data sources and tools.

**Think of it as "USB-C for AI"** - a universal way for any AI assistant to connect to any data source/tool without requiring custom integrations for each combination.

### MCP Core Primitives

1. **Resources** (Application-controlled): Data sources that LLMs can access (like GET endpoints) - read-only, no side effects
2. **Tools** (Model-controlled): Functions the LLM can call to perform actions (like function calling)
3. **Prompts** (User-controlled): Pre-defined templates for optimal tool/resource usage

### Protocol Foundation

- **Transport**: JSON-RPC 2.0 over stdio or HTTP
- **Communication**: Bidirectional client-server architecture
- **Discovery**: Capability negotiation via initialization handshake
- **Inspired by**: Language Server Protocol (LSP) architecture

## Current State vs MCP

### Current Atmos AI Architecture

```
Atmos AI (Embedded)
├── Tool Registry (pkg/ai/tools/registry.go)
├── Tool Executor (pkg/ai/tools/executor.go)
├── Individual Tools
│   ├── list_components
│   ├── describe_component
│   ├── validate_stack
│   ├── terraform_plan
│   └── file_access_tools
├── Permission System (pkg/ai/tools/permission/)
└── TUI Chat Interface (pkg/ai/tui/)
```

### MCP-Equivalent Architecture

```
MCP Server
├── MCP Protocol Handler (JSON-RPC 2.0)
├── Transport Layer (stdio/HTTP)
├── Tool/Resource Adapters
├── Same Tool Registry (reused)
└── Same Permission System (reused)
```

**Key Insight**: We already have 90% of the implementation. MCP integration requires wrapping existing tools with the MCP protocol layer.

## Relationship to Atmos AI

Current Atmos AI tools are conceptually equivalent to MCP tools but accessed differently:

| Aspect | Current Atmos AI | MCP Integration |
|--------|-----------------|-----------------|
| **Tools** | Direct Go function calls | JSON-RPC 2.0 messages |
| **Access** | Embedded in `atmos ai chat` | Any MCP client |
| **Discovery** | Hardcoded registry | Dynamic capability negotiation |
| **Transport** | In-process | stdio/HTTP |
| **Clients** | Built-in TUI only | Claude Desktop, VSCode, Claude Code, etc. |

## Architecture Options

### Option A: Embedded/Subprocess (stdio transport) ✅ **Recommended**

**Command:**
```bash
atmos mcp-server
```

**Communication:** stdin/stdout (JSON-RPC 2.0)

**Client Configuration (Claude Desktop):**
```json
{
  "mcpServers": {
    "atmos": {
      "command": "atmos",
      "args": ["mcp-server"]
    }
  }
}
```

**Benefits:**
- Simple deployment - single binary
- No network configuration needed
- Secure - local process only
- Perfect for CLI tools like Atmos
- Standard for desktop MCP integrations

**Use Cases:**
- Claude Desktop integration
- VSCode extension integration
- Local development workflows

### Option B: Standalone Server (HTTP transport with SSE) ✅ **Implemented**

**Command:**
```bash
atmos mcp-server --transport http --port 8080
```

**Communication:** HTTP with Server-Sent Events (SSE) for streaming

**Endpoints:**
- `GET /sse` - Server-Sent Events endpoint for server→client messages
- `POST /message` - JSON-RPC message endpoint for client→server requests
- `GET /health` - Health check endpoint

**Benefits:**
- Multiple concurrent clients
- Network-accessible from remote/cloud environments
- Better for team/shared environments
- Can run as a service or in containers
- Real-time streaming via SSE

**Tradeoffs:**
- More complex deployment
- Requires port management
- Security considerations (currently no auth/TLS in v1)
- Higher operational overhead

**Use Cases:**
- Team shared resources
- CI/CD integration
- Remote/cloud environments
- Containerized deployments
- Cloud Desktop integration

### Recommended Approach: Both Transports Supported ✅

Both transports are implemented with stdio as the default:

```bash
# Default: stdio for desktop clients (Claude Desktop, VSCode)
atmos mcp-server

# HTTP transport for remote/cloud access
atmos mcp-server --transport http --port 8080

# HTTP with custom host and port
atmos mcp-server --transport http --host 0.0.0.0 --port 3000
```

**Future Enhancements (not in v1):**
- Authentication (API keys, JWT tokens)
- TLS/HTTPS support
- Rate limiting
- Connection pooling

## Complete Architecture Diagram

```
┌────────────────────────────────────────────────────────────────────┐
│                        Client Environments                          │
├────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────┐        ┌──────────────────┐                  │
│  │  Claude Desktop │        │  Cloud Desktop   │                  │
│  │  Claude Code    │        │  Web Clients     │                  │
│  │  VSCode/IDEs    │        │  Remote Clients  │                  │
│  └────────┬────────┘        └────────┬─────────┘                  │
│           │                           │                             │
│           │ stdio                     │ HTTP/SSE                   │
│           │ (subprocess)              │ (network)                  │
│           │                           │                             │
│           └───────────┬───────────────┘                             │
│                       ↓                                             │
│           ┌───────────────────────────┐                            │
│           │   Atmos MCP Server        │                            │
│           │   (atmos mcp-server)      │                            │
│           ├───────────────────────────┤                            │
│           │ Transport Layer:          │                            │
│           │  • stdio (default)        │                            │
│           │  • HTTP + SSE             │                            │
│           ├───────────────────────────┤                            │
│           │ Protocol Handler:         │                            │
│           │  • JSON-RPC 2.0           │                            │
│           │  • MCP Spec 2025-03-26    │                            │
│           └───────────┬───────────────┘                            │
│                       │                                             │
│         ┌─────────────┴─────────────┐                              │
│         ↓                           ↓                               │
│  ┌─────────────┐          ┌─────────────┐                         │
│  │ MCP Tools   │          │ MCP Resources│                         │
│  ├─────────────┤          ├─────────────┤                         │
│  │ list_*      │          │ stack_config│                         │
│  │ describe_*  │          │ component_  │                         │
│  │ validate_*  │          │   _schema   │                         │
│  │ terraform_* │          └─────────────┘                         │
│  │ file_access │                                                   │
│  └─────────────┘                                                   │
│         │                                                           │
│         ↓                                                           │
│  ┌──────────────────────────────────┐                             │
│  │    Atmos Core Engine             │                             │
│  │  (shared with 'atmos ai chat')   │                             │
│  ├──────────────────────────────────┤                             │
│  │ • Component Loader               │                             │
│  │ • Stack Processor                │                             │
│  │ • Terraform Integration          │                             │
│  │ • Validation Engine              │                             │
│  │ • Tool Registry (reused)         │                             │
│  │ • Permission System (reused)     │                             │
│  └──────────────────────────────────┘                             │
│                                                                      │
└────────────────────────────────────────────────────────────────────┘
```

## Complete Atmos Ecosystem

```
atmos binary
│
├── atmos [commands]              # Existing CLI commands
│   ├── describe stacks
│   ├── terraform plan
│   ├── validate component
│   └── ...
│
├── atmos ai chat                 # Phase 1-2: Embedded AI (current)
│   ├── Interactive TUI
│   ├── Direct tool calls (in-process)
│   ├── Session management
│   └── Uses tool registry directly
│
├── atmos mcp-server              # Phase 3: MCP Server (NEW)
│   ├── --stdio (default)         # For desktop clients
│   ├── --http                    # For remote access
│   ├── Exposes tools via MCP protocol
│   └── Uses same tool registry (shared)
│
└── atmos lsp-server (Future)     # Optional: LSP for IDE integration
    ├── Language features (autocomplete, diagnostics)
    ├── Can be exposed via MCP bridge
    └── Separate protocol from MCP
```

### Key Architectural Principles

1. **Code Reuse**: MCP server and AI chat share the same tool implementations
2. **Single Binary**: All modes packaged in one `atmos` executable
3. **No Duplication**: Core engine used by both embedded AI and MCP server
4. **Standard Protocols**: MCP for AI, LSP for editor features (future)

## Claude Code Subagents

### What are Subagents?

**Subagents** are specialized AI assistants in Claude Code that provide task-specific expertise. They are a **higher-level abstraction** built on top of MCP tools.

### Subagent Architecture

```
Claude Code
├── Main Agent (general purpose)
│   └── Can use any MCP tool
│
└── Specialized Subagents
    ├── "Atmos Infrastructure Expert"
    │   ├── System Prompt: Domain expertise
    │   ├── Isolated Context
    │   ├── Allowed Tools: atmos_* only
    │   └── Team-shared configuration
    │
    └── "Terraform Plan Reviewer"
        ├── System Prompt: Plan analysis expertise
        ├── Allowed Tools: terraform_plan, describe_*
        └── Permission: read-only
```

### Relationship to MCP

- **Subagents USE MCP tools** but are not MCP tools themselves
- **Subagents ADD**: Specialized personalities, isolated contexts, tool restrictions
- **MCP provides**: Standardized tool access layer
- **Subagents provide**: Orchestration and domain expertise

### Example Atmos Subagent Configuration

```json
{
  "name": "atmos-infrastructure-expert",
  "description": "Specialist for Atmos infrastructure management and stack configuration",
  "systemPrompt": "You are an expert in Atmos infrastructure as code. You help users:\n- Understand stack configurations\n- Debug component issues\n- Validate Terraform plans\n- Follow Atmos best practices\n\nAlways explain Atmos concepts clearly and reference official documentation.",
  "mcpServers": {
    "atmos": {
      "command": "atmos",
      "args": ["mcp-server"]
    }
  },
  "allowedTools": [
    "atmos_list_components",
    "atmos_list_stacks",
    "atmos_describe_component",
    "atmos_describe_stacks",
    "atmos_validate_component",
    "atmos_terraform_plan"
  ],
  "blockedTools": [
    "atmos_write_*",
    "atmos_terraform_apply"
  ]
}
```

## Language Server Protocol (LSP)

### LSP vs MCP

| Aspect | LSP | MCP |
|--------|-----|-----|
| **Purpose** | Editor language features | AI tool/data access |
| **Features** | Autocomplete, diagnostics, definitions | Function calls, resource access |
| **Clients** | IDEs (VSCode, IntelliJ, etc.) | AI assistants (Claude, etc.) |
| **Protocol** | JSON-RPC 2.0 | JSON-RPC 2.0 |
| **Transport** | stdio, sockets | stdio, HTTP |
| **Similarity** | Solves M×N editor×language problem | Solves M×N client×resource problem |

### Potential Atmos LSP Features (Future)

```
Atmos Language Server (Future Phase)
├── Autocomplete for stack YAML files
├── Go-to-definition for components
├── Diagnostics/validation
├── Hover documentation
├── Symbol navigation
└── Workspace-wide search
```

### MCP-LSP Bridge

LSP features can be exposed as MCP tools:

```
MCP Tool: "get_definition"
├── Input: { "file": "stack.yaml", "position": { line, col } }
├── Calls: Atmos LSP server
└── Output: Definition location and content
```

This enables AI assistants to leverage LSP features through MCP.

## Implementation Strategy

### Phase 3: MCP Server Implementation

#### 3.1 Core MCP Server

**Package Structure:**
```
pkg/mcp/
├── server.go           // MCP server implementation
├── protocol/
│   ├── types.go        // MCP protocol types
│   ├── handler.go      // JSON-RPC handler
│   └── messages.go     // Request/response messages
├── transport/
│   ├── stdio.go        // stdin/stdout transport
│   └── http.go         // HTTP transport
├── adapter.go          // Adapts existing tools to MCP format
└── resources.go        // MCP resources implementation
```

**CLI Command:**
```go
cmd/mcp-server/
├── command.go          // Cobra command definition
└── server.go           // Server startup logic
```

#### 3.2 Tool Adaptation

**Existing Tool → MCP Tool:**

```go
// Existing tool (pkg/ai/tools/atmos/list_components.go)
type ListComponentsTool struct { ... }

// MCP adapter (pkg/mcp/adapter.go)
func (a *Adapter) AdaptTool(tool tools.Tool) *protocol.Tool {
    return &protocol.Tool{
        Name:        tool.Name(),
        Description: tool.Description(),
        InputSchema: tool.Schema(),
    }
}

// MCP tool execution
func (a *Adapter) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (*protocol.ToolResult, error) {
    // Reuse existing executor
    result, err := a.executor.Execute(ctx, name, params)
    // Convert to MCP format
    return adaptResult(result), err
}
```

**Key Point**: No tool implementation changes needed - only protocol wrapping.

#### 3.3 Resource Implementation

**MCP Resources** (new capability):

```go
// pkg/mcp/resources.go
type Resource struct {
    URI         string
    Name        string
    Description string
    MimeType    string
}

// Example resources:
// - atmos://stacks/{stack-name}/config
// - atmos://components/{component-name}/schema
// - atmos://config/atmos.yaml
```

Resources provide **read-only** access to Atmos data without executing commands.

#### 3.4 Transport Layer

**stdio Transport (default):**
```go
// pkg/mcp/transport/stdio.go
type StdioTransport struct {
    stdin  io.Reader
    stdout io.Writer
}

func (t *StdioTransport) Serve(ctx context.Context, handler protocol.Handler) error {
    scanner := bufio.NewScanner(t.stdin)
    for scanner.Scan() {
        // Parse JSON-RPC request
        // Call handler
        // Write JSON-RPC response to stdout
    }
}
```

**HTTP Transport:**
```go
// pkg/mcp/transport/http.go
type HTTPTransport struct {
    addr   string
    server *http.Server
}

func (t *HTTPTransport) Serve(ctx context.Context, handler protocol.Handler) error {
    // Implement Streamable HTTP transport
    // POST for requests
    // GET with SSE for responses (or WebSocket future)
}
```

#### 3.5 Integration Points

**Shared Components:**

```
Tool Registry (pkg/ai/tools/registry.go)
    ↓ used by
    ├─→ AI Chat (pkg/ai/tui/)
    └─→ MCP Server (pkg/mcp/)

Tool Executor (pkg/ai/tools/executor.go)
    ↓ used by
    ├─→ AI Chat
    └─→ MCP Server

Permission System (pkg/ai/tools/permission/)
    ↓ used by
    ├─→ AI Chat
    └─→ MCP Server
```

**No duplication** - all use the same implementations.

### Phase 4 (Future): LSP Server

#### 4.1 Language Server Implementation

```
pkg/lsp/
├── server.go           // LSP server
├── features/
│   ├── completion.go   // Autocomplete
│   ├── definition.go   // Go-to-definition
│   ├── diagnostics.go  // Validation
│   └── hover.go        // Hover docs
└── parser/
    ├── stack.go        // Stack YAML parser
    └── component.go    // Component parser
```

#### 4.2 MCP-LSP Bridge (Optional)

Expose LSP features as MCP tools:

```
MCP Tool: "lsp_get_definition"
MCP Tool: "lsp_get_diagnostics"
MCP Tool: "lsp_get_completions"
```

Enables AI to use IDE features through MCP.

## Security Considerations

### MCP Server Security

1. **stdio Transport:**
   - Local process only
   - Inherits user permissions
   - No network exposure
   - **Recommendation**: Default and safest option

2. **HTTP Transport:**
   - Requires authentication (token-based)
   - TLS/HTTPS for remote access
   - Rate limiting
   - IP allowlist
   - **Recommendation**: Only for trusted networks

3. **Tool Permissions:**
   - Reuse existing permission system
   - User confirmation for destructive operations
   - Read-only by default for resources
   - Configurable tool allowlists/blocklists

### Subagent Security

- Subagents can have restricted tool access
- Team admins control subagent configurations
- Isolated contexts prevent information leakage
- Audit logging for tool execution

## Success Metrics

1. **Integration Success:**
   - Atmos tools accessible from Claude Desktop
   - Atmos tools accessible from Claude Code
   - Atmos tools accessible from VSCode with MCP plugin

2. **Adoption:**
   - Number of active MCP client installations
   - Number of tool invocations via MCP
   - User feedback on MCP vs embedded AI

3. **Performance:**
   - Tool execution latency (target: <500ms)
   - Server startup time (target: <100ms)
   - Memory overhead (target: <50MB)

4. **Reliability:**
   - Error rate <1%
   - Graceful degradation on errors
   - Proper error messages to clients

## Open Questions

1. **Resource Design**: What read-only resources are most valuable?
   - Stack configurations?
   - Component schemas?
   - Validation results?
   - Terraform state (read-only)?

2. **HTTP Authentication**: What auth mechanism for HTTP transport?
   - Bearer tokens?
   - API keys?
   - OAuth?
   - mTLS?

3. **Subagent Templates**: Should we provide official Atmos subagent configurations?
   - Infrastructure Expert
   - Terraform Plan Reviewer
   - Stack Configuration Validator

4. **LSP Timeline**: When to implement LSP server?
   - After MCP server proven?
   - Combined with MCP implementation?
   - Separate future phase?

5. **WebSocket Support**: Should we implement WebSocket transport?
   - Better for bidirectional communication
   - Not in MCP spec yet (proposed)
   - Wait for standardization?

## Dependencies

### External Libraries

- **JSON-RPC 2.0**: Consider `github.com/sourcegraph/jsonrpc2`
- **HTTP Server**: stdlib `net/http`
- **stdio**: stdlib `os.Stdin`, `os.Stdout`

### Internal Dependencies

- Existing tool registry (`pkg/ai/tools/`)
- Existing executor (`pkg/ai/tools/executor.go`)
- Existing permission system (`pkg/ai/tools/permission/`)
- Atmos config system (`pkg/config/`)

## Timeline Estimate

### Phase 3: MCP Server (4-6 weeks)

- **Week 1-2**: Core MCP protocol implementation
  - JSON-RPC handler
  - stdio transport
  - Tool adapter

- **Week 3**: HTTP transport
  - Server implementation
  - Authentication
  - Testing

- **Week 4**: Resource implementation
  - Define resource URIs
  - Implement resource handlers
  - Documentation

- **Week 5**: Integration & Testing
  - Claude Desktop integration
  - Claude Code testing
  - Documentation

- **Week 6**: Polish & Release
  - Error handling improvements
  - Performance optimization
  - Release blog post

### Phase 4: LSP Server (6-8 weeks, future)

- TBD after MCP server success

## References

- [Model Context Protocol Specification](https://modelcontextprotocol.io)
- [MCP Documentation (Anthropic)](https://docs.claude.com/en/docs/mcp)
- [Language Server Protocol](https://microsoft.github.io/language-server-protocol/)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [Awesome Claude Code Subagents](https://github.com/VoltAgent/awesome-claude-code-subagents)

## Appendix A: MCP Protocol Example

### Tool Discovery (Initialization)

**Client → Server:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "tools": {}
    },
    "clientInfo": {
      "name": "Claude Desktop",
      "version": "1.0.0"
    }
  }
}
```

**Server → Client:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "tools": {
        "listChanged": true
      },
      "resources": {
        "subscribe": true
      }
    },
    "serverInfo": {
      "name": "atmos-mcp-server",
      "version": "1.23.0"
    }
  }
}
```

### Tool Execution

**Client → Server:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "atmos_list_components",
    "arguments": {
      "type": "terraform"
    }
  }
}
```

**Server → Client:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Components:\n- vpc\n- eks\n- rds\n..."
      }
    ]
  }
}
```

### Resource Access

**Client → Server:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "resources/read",
  "params": {
    "uri": "atmos://stacks/prod-us-east-1/config"
  }
}
```

**Server → Client:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "contents": [
      {
        "uri": "atmos://stacks/prod-us-east-1/config",
        "mimeType": "application/yaml",
        "text": "vars:\n  namespace: prod\n  environment: production\n..."
      }
    ]
  }
}
```

## Appendix B: Claude Desktop Configuration

### User Configuration (~/.config/claude/claude_desktop_config.json)

```json
{
  "mcpServers": {
    "atmos": {
      "command": "atmos",
      "args": ["mcp-server"],
      "env": {
        "ATMOS_CLI_CONFIG_PATH": "/path/to/atmos.yaml",
        "ATMOS_BASE_PATH": "/path/to/infrastructure"
      }
    }
  }
}
```

### Multiple Server Configuration

```json
{
  "mcpServers": {
    "atmos-local": {
      "command": "atmos",
      "args": ["mcp-server"]
    },
    "atmos-remote": {
      "transport": "http",
      "url": "https://atmos-mcp.company.com",
      "headers": {
        "Authorization": "Bearer ${ATMOS_MCP_TOKEN}"
      }
    }
  }
}
```

## Appendix C: Comparison with Current Implementation

### Current: Embedded AI Chat

**Pros:**
- Simple user experience
- Fast (in-process)
- No configuration needed
- Full context in conversation

**Cons:**
- Only accessible via `atmos ai chat`
- Not available to other AI tools
- Limited to TUI interface
- No team sharing

### Future: MCP Server

**Pros:**
- Works with any MCP client
- Available in Claude Desktop, VSCode, etc.
- Standardized protocol
- Team can share configurations
- Multiple concurrent clients (HTTP mode)

**Cons:**
- Requires client configuration
- Slightly higher latency (IPC overhead)
- More complex architecture
- Additional maintenance

### Recommendation: Support Both

Keep `atmos ai chat` for standalone use and add `atmos mcp-server` for MCP clients. Users can choose based on their workflow.
