# Atmos AI - Complete Product Requirements Document

**Status:** Production Ready
**Version:** 2.0
**Last Updated:** 2025-10-31

---

## Executive Summary

Atmos AI is an intelligent assistant integrated directly into Atmos CLI, designed specifically for infrastructure-as-code management. Unlike general-purpose AI coding assistants, Atmos AI provides deep understanding of Atmos stacks, components, inheritance patterns, and infrastructure workflows.

**Key Achievement:** Atmos AI successfully combines the productivity patterns found in industry-leading AI systems with domain-specific intelligence for infrastructure management, while maintaining privacy-first architecture and enterprise-grade security.

### Current Status

**✅ Production Ready** - All core features implemented and tested.

- **7 AI Providers** - Anthropic, OpenAI, Google Gemini, xAI Grok, Ollama, AWS Bedrock, Azure OpenAI
- **Session Management** - SQLite-backed persistence with full CRUD operations
- **Project Memory** - ATMOS.md for persistent context across sessions
- **Tool Execution** - 19 tools with granular permission system
- **Agent System** - 5 built-in specialized agents + marketplace (production ready)
- **MCP Integration** - stdio/HTTP transports for external clients
- **LSP Integration** - YAML/Terraform validation with real-time diagnostics
- **Enhanced TUI** - Markdown rendering, syntax highlighting, session management
- **Token Caching** - Save up to 90% on API costs with prompt caching (6/7 providers)

---

## Vision & Strategic Goals

### Vision Statement

**"The AI assistant that truly understands your infrastructure"**

Atmos AI aims to be the intelligent partner for infrastructure engineers, providing context-aware assistance for Atmos stack management while respecting security, privacy, and enterprise requirements.

### Strategic Goals

1. **Domain Expertise** - Deep understanding of Atmos concepts (stacks, components, inheritance)
2. **Productivity** - Reduce time spent on repetitive tasks and documentation lookup
3. **Safety** - Prevent accidental destructive operations through permission controls
4. **Privacy** - Support on-premises and air-gapped deployments
5. **Extensibility** - Enable community contributions through agent marketplace
6. **Enterprise Ready** - Meet compliance and security requirements for large organizations

### Competitive Differentiation

Compared to industry-leading AI systems:

| Feature | Industry-Leading Systems | Atmos AI |
|---------|-------------------------|----------|
| **Domain Knowledge** | General software development | ✅ **Atmos infrastructure-specific** |
| **Stack Context** | N/A | ✅ **Deep stack analysis** |
| **Session Persistence** | ✅ SQLite-backed | ✅ **SQLite-backed** |
| **Tool Execution** | ✅ Bash, file operations | ✅ **Atmos-specific + file ops** |
| **Project Memory** | ✅ Markdown-based | ✅ **ATMOS.md** |
| **MCP Support** | ✅ stdio/HTTP | ✅ **stdio/HTTP** |
| **LSP Integration** | ✅ Multi-language | ✅ **YAML/Terraform** |
| **On-Premises** | ❌ Cloud-only | ✅ **Ollama support** |
| **Enterprise Providers** | ❌ Limited | ✅ **Bedrock, Azure OpenAI** |

**Key Advantage:** Atmos AI has all the productivity features of industry-leading systems PLUS infrastructure-specific intelligence.

---

## Architecture Overview

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                      Atmos AI System                         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         User Interfaces                               │  │
│  ├──────────────────────────────────────────────────────┤  │
│  │ • TUI (Bubble Tea)      • CLI (atmos ai ask/chat)    │  │
│  │ • MCP Server (stdio/HTTP)                            │  │
│  └────────────────────┬─────────────────────────────────┘  │
│                       │                                      │
│  ┌────────────────────▼─────────────────────────────────┐  │
│  │         Core AI Engine                                │  │
│  ├──────────────────────────────────────────────────────┤  │
│  │ • Multi-Provider Factory                             │  │
│  │ • Session Manager                                    │  │
│  │ • Agent Registry                                     │  │
│  │ • Tool Executor                                      │  │
│  │ • Permission System                                  │  │
│  └────────────────────┬─────────────────────────────────┘  │
│                       │                                      │
│  ┌────────────────────▼─────────────────────────────────┐  │
│  │         Storage & Context                             │  │
│  ├──────────────────────────────────────────────────────┤  │
│  │ • SQLite (sessions)  • ATMOS.md (memory)             │  │
│  │ • Local Registry     • Cache                         │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Package Structure

```
pkg/ai/
├── agent/                  # AI provider implementations
│   ├── anthropic/         # Claude via Anthropic API
│   ├── openai/            # GPT via OpenAI API
│   ├── gemini/            # Gemini via Google AI
│   ├── grok/              # Grok via xAI API
│   ├── ollama/            # Local LLMs via Ollama
│   ├── bedrock/           # Claude via AWS Bedrock
│   └── azureopenai/       # GPT via Azure OpenAI
├── agents/                # Agent system
│   ├── agent.go           # Agent interface
│   ├── registry.go        # Agent registry
│   ├── builtin.go         # Built-in agents
│   ├── prompts/           # Embedded prompts
│   └── marketplace/       # Agent marketplace
├── session/               # Session management
│   ├── manager.go         # Session lifecycle
│   ├── storage/           # SQLite storage
│   └── compactor.go       # Auto-compact
├── memory/                # Project memory
│   ├── manager.go         # ATMOS.md management
│   └── parser.go          # Markdown parser
├── tools/                 # Tool execution
│   ├── interface.go       # Tool interface
│   ├── executor.go        # Tool executor
│   ├── registry.go        # Tool registry
│   ├── permission/        # Permission system
│   └── atmos/             # Atmos-specific tools
├── tui/                   # Terminal UI
│   ├── chat.go            # Chat interface
│   ├── sessions.go        # Session management UI
│   └── create_session.go  # Session creation
├── mcp/                   # Model Context Protocol
│   ├── server.go          # MCP server
│   ├── stdio_transport.go # stdio transport
│   └── http_transport.go  # HTTP transport
├── lsp/                   # Language Server Protocol
│   ├── client.go          # LSP client
│   ├── manager.go         # Multi-server management
│   └── diagnostics.go     # Diagnostic formatting
└── factory.go             # Provider factory

cmd/
├── ai_chat.go             # Interactive chat command
├── ai_ask.go              # Single-shot query command
├── ai_memory.go           # Memory management commands
├── ai_sessions.go         # Session management commands
└── mcp_server.go          # MCP server command
```

---

## Core Features

### 1. Session Management

**Status:** ✅ Production Ready

#### Overview

Persistent conversation sessions with SQLite backend enable multi-day infrastructure workflows without losing context.

#### Key Features

- **SQLite Storage** - Reliable, ACID-compliant local storage
- **Full CRUD Operations** - Create, Read, Update, Delete sessions
- **Provider-Aware** - Each session remembers its AI provider and model
- **Message History** - Complete conversation persistence
- **Auto-Compact** - AI-powered history summarization for extended conversations
- **Auto-Cleanup** - Configurable retention (auto-delete after N days)
- **TUI Integration** - Visual session management with keyboard shortcuts

#### User Experience

```bash
# CLI: Create named session
atmos ai chat --session vpc-migration

# CLI: Resume existing session
atmos ai chat --session vpc-migration

# CLI: List all sessions
atmos ai sessions list

# TUI: Interactive session management
Ctrl+N  - Create new session
Ctrl+L  - Switch session
d       - Delete session
r       - Rename session
f       - Filter by provider
```

#### Configuration

```yaml
settings:
  ai:
    sessions:
      enabled: true
      storage: sqlite
      path: .atmos/sessions
      max_sessions: 10
      auto_save: true
      retention_days: 30
      max_history_messages: 50

      auto_compact:
        enabled: false  # Opt-in
        trigger_threshold: 0.75
        compact_ratio: 0.4
        preserve_recent: 10
        use_ai_summary: true
        summary_provider: anthropic
        summary_model: claude-3-5-haiku-20241022
```

#### Auto-Compact Feature

**Status:** ✅ Production Ready

Intelligent conversation history compaction enables extended multi-day conversations by AI-powered summarization of older messages.

**How It Works:**
1. Monitors message count and token usage
2. Triggers compaction when 75% of max messages reached
3. Uses AI to summarize oldest 40% of conversation
4. Replaces original messages with concise summary
5. Preserves recent messages for context continuity

**Configuration Options:**
- `enabled` - Enable/disable auto-compact (default: false, opt-in)
- `trigger_threshold` - Percentage threshold to trigger (default: 0.75)
- `compact_ratio` - Ratio of old messages to compact (default: 0.4)
- `preserve_recent` - Number of recent messages to always keep (default: 10)
- `use_ai_summary` - Use AI for summarization vs simple truncation (default: true)
- `summary_provider` - AI provider for summaries (default: anthropic)
- `summary_model` - Model for summaries (default: claude-3-5-haiku-20241022)

**Benefits:**
- Extended conversations without context loss
- Reduced token usage and API costs
- Automatic rate limit management
- Semantic meaning preservation

#### Database Schema

```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_path TEXT NOT NULL,
    model TEXT NOT NULL,
    provider TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    metadata TEXT
);

CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

#### Technical Implementation

- **Location**: `pkg/ai/session/`
- **Storage**: `pkg/ai/session/sqlite.go`
- **Compactor**: `pkg/ai/session/compactor.go`
- **TUI**: `pkg/ai/tui/sessions.go`
- **Tests**: 100% coverage, all passing

---

### 2. Project Memory (ATMOS.md)

**Status:** ✅ Production Ready

#### Overview

ATMOS.md provides persistent project knowledge that the AI can reference across all sessions, reducing repetitive context loading.

#### Key Features

- **Markdown Format** - Human-readable, version-control friendly
- **Structured Sections** - Project context, common commands, stack patterns, etc.
- **Auto-Creation** - Generated from template if missing
- **Manual Updates** - Users can edit directly
- **Context Injection** - Automatically included in AI prompts
- **Preserves Edits** - User changes respected during updates

#### Example ATMOS.md

```markdown
# Atmos Project Memory

## Project Context

**Organization:** acme-corp
**Environments:** dev, staging, prod
**Primary Regions:** us-east-1, us-west-2

**Stack Naming:** {org}-{env}-{region}-{stage}

## Common Commands

### Deploy VPC
```bash
atmos terraform plan vpc -s acme-prod-use1-network
atmos terraform apply vpc -s acme-prod-use1-network
```

## Stack Patterns

### Network Stack Structure
All network stacks inherit from:
- catalog/stacks/network/baseline
- catalog/stacks/network/security-groups

### CIDR Blocks
- dev: 10.0.0.0/16
- staging: 10.1.0.0/16
- prod: 10.2.0.0/16

## Frequent Issues & Solutions

### Q: Stack not found error
**Problem:** `Error: stack 'acme-dev-use1' not found`
**Solution:** Check stack naming matches pattern and verify config exists
```

#### Configuration

```yaml
settings:
  ai:
    memory:
      enabled: true
      file_path: ATMOS.md
      auto_update: false
      create_if_missing: true
      sections:
        - project_context
        - common_commands
        - stack_patterns
        - frequent_issues
        - infrastructure_patterns
```

#### Technical Implementation

- **Location**: `pkg/ai/memory/`
- **Parser**: `pkg/ai/memory/parser.go`
- **Template**: `templates/atmos_md.tmpl`
- **CLI Commands**: `atmos ai memory init/view/edit/update`

---

### 3. Tool Execution System

**Status:** ✅ Production Ready

#### Overview

AI can execute Atmos commands and file operations with granular permission controls, enabling autonomous infrastructure management.

#### Tool Categories

**Atmos-Specific Tools:**
- `atmos_describe_component` - Describe component configuration
- `atmos_describe_stacks` - Describe all stacks
- `atmos_list_stacks` - List available stacks
- `atmos_list_components` - List available components
- `atmos_validate_stacks` - Validate stack configurations
- `atmos_validate_component` - Validate specific component

**File Operations:**
- `file_read` - Read file contents
- `file_write` - Write file contents
- `file_search` - Search files/content
- `read_component_file` - Read component files (Terraform/Helmfile/Packer)
- `read_stack_file` - Read stack YAML files
- `write_component_file` - Write component files (with permission)
- `write_stack_file` - Write stack files (with permission)

**LSP Tools:**
- `validate_file_lsp` - Real-time YAML/Terraform validation

**Search & Analysis:**
- `web_search` - Search the web (DuckDuckGo/Google Custom Search)

#### Permission System

**Permission Categories:**
1. **Allowed Tools** - Execute without prompting
2. **Restricted Tools** - Always require confirmation
3. **Blocked Tools** - Never execute
4. **YOLO Mode** - Bypass all confirmations (dangerous!)

**Configuration:**

```yaml
settings:
  ai:
    tools:
      enabled: true
      require_confirmation: true

      allowed_tools:
        - atmos_describe_*
        - atmos_list_*
        - file_read

      restricted_tools:
        - file_write
        - atmos_terraform_plan

      blocked_tools:
        - atmos_terraform_apply
        - atmos_terraform_destroy

      yolo_mode: false

      audit:
        enabled: true
        path: .atmos/ai-audit.log
```

#### Permission Prompt Example

```
┌─────────────────────────────────────────┐
│ Tool Execution Permission               │
├─────────────────────────────────────────┤
│ Tool: atmos_describe_component          │
│ Component: vpc                          │
│ Stack: prod-use1                        │
│                                         │
│ This will execute:                      │
│ $ atmos describe component vpc \        │
│     -s prod-use1                        │
│                                         │
│ [Allow Once] [Allow Always] [Deny]      │
└─────────────────────────────────────────┘
```

#### Technical Implementation

- **Location**: `pkg/ai/tools/`
- **Interface**: `pkg/ai/tools/interface.go`
- **Atmos Tools**: `pkg/ai/tools/atmos/`
- **Permissions**: `pkg/ai/tools/permission/`
- **Executor**: `pkg/ai/tools/executor.go`

---

### 4. Agent System

**Status:** ✅ Production Ready (Built-in Agents & Marketplace)

#### Overview

Specialized AI agents provide task-specific expertise and focused tool access, improving response quality for specific operations.

#### Built-in Agents

1. **General (Default)**
   - Purpose: General-purpose assistant
   - Tools: All tools
   - Use case: Everyday infrastructure questions

2. **Stack Analyzer**
   - Purpose: Analyze stack configurations and dependencies
   - Tools: `describe_*`, `list_*`, `read_stack_file`
   - Use case: Architecture reviews, stack analysis

3. **Component Refactor**
   - Purpose: Refactor Terraform/Helmfile components
   - Tools: `read_*`, `write_*`, `search_files`
   - Use case: Code improvements, modernization

4. **Security Auditor**
   - Purpose: Security review of infrastructure
   - Tools: `describe_*`, `read_*`, `validate_*`
   - Use case: Security audits, compliance checks

5. **Config Validator**
   - Purpose: Validate Atmos configurations
   - Tools: `validate_*`, `read_stack_file`, `validate_file_lsp`
   - Use case: Configuration troubleshooting

#### User Experience

**TUI Agent Selection:**
```
Press Ctrl+A to select agent:

1. General (default)
2. Stack Analyzer
3. Component Refactor
4. Security Auditor
5. Config Validator

Select agent (1-5): _
```

**CLI Agent Selection:**
```bash
atmos ai ask --agent stack-analyzer "Analyze all prod stacks"
```

#### Agent Architecture

**File-Based Prompts:**
- Each agent has a dedicated Markdown file in `pkg/ai/agents/prompts/`
- Prompts embedded in binary at compile time using `go:embed`
- ~6KB per agent, loaded only when active
- Easy to maintain and version control

**Agent Structure:**
```go
type Agent struct {
    Name            string
    DisplayName     string
    Description     string
    SystemPrompt    string      // Loaded from prompt file
    SystemPromptPath string     // Path to embedded prompt
    AllowedTools    []string    // Tools this agent can use
    RestrictedTools []string    // Tools requiring extra confirmation
    Category        string
    IsBuiltIn       bool
}
```

#### Agent Marketplace

**Status:** ✅ Production Ready

**Agent Distribution:**
- Agents distributed via GitHub repositories
- Install with: `atmos ai agent install github.com/user/agent-name`
- Versioned using Git tags (semantic versioning)
- Stored in `~/.atmos/agents/`

**Agent Format:**
```
agent-repo/
├── .agent.yaml        # Agent metadata
├── prompt.md          # System prompt
└── README.md          # Documentation
```

**Example `.agent.yaml`:**
```yaml
name: cost-analyzer
display_name: Cost Analyzer
version: 1.2.3
author: username
description: Analyzes infrastructure costs
category: optimization

atmos:
  min_version: 1.50.0

prompt:
  file: prompt.md

tools:
  allowed:
    - describe_stacks
    - describe_component
  restricted:
    - terraform_apply

repository: https://github.com/username/agent-repo
```

**Commands:**
```bash
# Install agent
atmos ai agent install github.com/user/agent-name
atmos ai agent install github.com/user/agent-name@v1.2.3

# Manage agents
atmos ai agent list
atmos ai agent update <name>
atmos ai agent uninstall <name>
atmos ai agent info <name>

# Search agents
atmos ai agent search "cost"
```

#### Technical Implementation

- **Location**: `pkg/ai/agents/`
- **Registry**: `pkg/ai/agents/registry.go`
- **Prompts**: `pkg/ai/agents/prompts/` (embedded)
- **TUI**: `pkg/ai/tui/chat.go` (agent switcher)
- **Marketplace**: `pkg/ai/agents/marketplace/`
- **CLI**: `cmd/ai/agent/` (install, list, uninstall commands)

---

### 5. Model Context Protocol (MCP)

**Status:** ✅ Production Ready

#### Overview

MCP integration enables Atmos tools to be accessed from any MCP-compatible client (Claude Desktop, VSCode, etc.), standardizing AI-to-tool communication.

#### Supported Transports

1. **stdio (Default)** - For desktop clients
   - Claude Desktop
   - VSCode/Cursor
   - Local development

2. **HTTP + SSE** - For remote/cloud clients (SDK-based)
   - Cloud Desktop
   - Remote environments
   - Containerized deployments
   - Note: Implementation uses official MCP SDK

#### Usage

**Start MCP Server:**
```bash
# stdio (default)
atmos mcp-server

# HTTP
atmos mcp-server --transport http --port 3000
```

**Claude Desktop Configuration:**
```json
{
  "mcpServers": {
    "atmos": {
      "command": "atmos",
      "args": ["mcp-server"],
      "env": {
        "ATMOS_CLI_CONFIG_PATH": "/path/to/atmos.yaml"
      }
    }
  }
}
```

#### Architecture

```
┌────────────────────────────────────────────┐
│         MCP Clients                        │
│  (Claude Desktop, VSCode, etc.)            │
└────────────────┬───────────────────────────┘
                 │
        stdio or HTTP/SSE
                 │
┌────────────────▼───────────────────────────┐
│         Atmos MCP Server                   │
│         (atmos mcp-server)                 │
├────────────────────────────────────────────┤
│ • JSON-RPC 2.0 Protocol Handler            │
│ • Transport Layer (stdio/HTTP)             │
│ • Tool/Resource Adapters                   │
└────────────────┬───────────────────────────┘
                 │
        ┌────────┴────────┐
        ▼                 ▼
  ┌──────────┐      ┌──────────┐
  │ MCP Tools│      │ Resources│
  ├──────────┤      ├──────────┤
  │ list_*   │      │ configs  │
  │ describe_│      │ schemas  │
  │ validate_│      └──────────┘
  └──────────┘
        │
        ▼
┌────────────────────────────────────────────┐
│         Atmos Core Engine                  │
│  (shared with 'atmos ai chat')             │
└────────────────────────────────────────────┘
```

#### Benefits

- **Universal Access** - Use Atmos tools from any MCP client
- **Standardized Protocol** - No custom integrations needed
- **Multiple Clients** - HTTP mode supports concurrent clients
- **Tool Reuse** - Same tools as embedded AI

#### Configuration

```yaml
settings:
  ai:
    mcp:
      enabled: true
      transport: stdio  # or http

      http:
        port: 3000
        host: localhost
```

#### Technical Implementation

- **Location**: `pkg/mcp/`
- **Server**: `pkg/mcp/server.go` (uses official MCP SDK)
- **Adapter**: `pkg/mcp/adapter.go` (converts Atmos tools to MCP format)
- **Command**: `cmd/mcp-server/`
- **Documentation**: `website/docs/ai/mcp-server.mdx`

---

### 6. Language Server Protocol (LSP) Integration

**Status:** ✅ Production Ready

#### Overview

LSP integration provides real-time validation of YAML and Terraform files using industry-standard language servers.

#### Supported LSP Servers

1. **yaml-language-server** - YAML/YML validation
   - JSON Schema support
   - Real-time syntax checking
   - Atmos stack file validation

2. **terraform-ls** - Terraform/HCL validation
   - Terraform syntax validation
   - Module validation
   - Provider schema checking

#### Features

- **Real-time Diagnostics** - Precise line/column error locations
- **Multi-server Management** - Manage multiple LSP servers simultaneously
- **Diagnostic Formatting** - AI-optimized, detailed, and compact formats
- **AI Tool Integration** - `validate_file_lsp` tool for AI validation
- **File Type Routing** - Automatic server selection based on file type

#### Configuration

```yaml
settings:
  lsp:
    enabled: true
    servers:
      yaml-ls:
        command: "yaml-language-server"
        args: ["--stdio"]
        filetypes: ["yaml", "yml"]
        root_patterns: ["atmos.yaml", ".git"]
        initialization_options:
          yaml:
            schemas:
              https://json.schemastore.org/github-workflow.json: ".github/workflows/*.{yml,yaml}"
            format:
              enable: true
            validation: true

      terraform-ls:
        command: "terraform-ls"
        args: ["serve"]
        filetypes: ["tf", "tfvars", "hcl"]
        root_patterns: [".terraform", ".git"]
        initialization_options:
          experimentalFeatures:
            validateOnSave: true
```

#### AI Integration

**Tool: `validate_file_lsp`**

Allows AI to validate files using LSP servers:

```
User: "Validate stacks/prod/vpc.yaml"

AI: *Uses validate_file_lsp tool*

AI: "Found 2 issue(s) in /project/stacks/prod/vpc.yaml:

ERRORS (1):
1. Line 15, Col 5: Unknown property 'vpc_ciddr' (did you mean 'vpc_cidr'?)

WARNINGS (1):
1. Line 23, Col 3: Property 'availability_zones' is deprecated, use 'azs'

Would you like me to help fix these issues?"
```

#### Technical Implementation

- **Location**: `pkg/lsp/`
- **Client**: `pkg/lsp/client.go` (JSON-RPC 2.0)
- **Manager**: `pkg/lsp/manager.go` (multi-server)
- **Diagnostics**: `pkg/lsp/diagnostics.go` (formatting)
- **Tool**: `pkg/ai/tools/atmos/validate_file_lsp.go`
- **Tests**: 100% coverage with comprehensive test suite

---

## Multi-Provider Support

### Supported AI Providers

Atmos AI supports 7 AI providers, covering cloud, on-premises, and enterprise use cases:

#### 1. Anthropic (Claude)

**Models:**
- `claude-sonnet-4-20250514` (default) - Most capable
- `claude-3-5-haiku-20241022` - Fast and cost-effective
- `claude-3-opus-20240229` - Maximum intelligence

**Configuration:**
```yaml
settings:
  ai:
    provider: anthropic
    model: claude-sonnet-4-20250514
    api_key_env: ANTHROPIC_API_KEY
```

**Best For:** General infrastructure tasks, complex analysis

#### 2. OpenAI (GPT)

**Models:**
- `gpt-4o` (default) - Latest multimodal model
- `gpt-4-turbo` - Fast GPT-4
- `gpt-3.5-turbo` - Cost-effective

**Configuration:**
```yaml
settings:
  ai:
    provider: openai
    model: gpt-4o
    api_key_env: OPENAI_API_KEY
```

**Best For:** Code generation, refactoring

#### 3. Google Gemini

**Models:**
- `gemini-1.5-pro` (default) - Most capable
- `gemini-1.5-flash` - Fast and efficient

**Configuration:**
```yaml
settings:
  ai:
    provider: gemini
    model: gemini-1.5-pro
    api_key_env: GOOGLE_API_KEY
```

**Best For:** Large context windows, document analysis

#### 4. xAI Grok

**Models:**
- `grok-2-latest` (default)
- `grok-vision-beta`

**Configuration:**
```yaml
settings:
  ai:
    provider: grok
    model: grok-2-latest
    api_key_env: XAI_API_KEY
```

**Best For:** Alternative to OpenAI, competitive pricing

#### 5. Ollama (Local LLMs)

**Models:**
- `llama3.3:70b` (default) - Production quality
- `llama3.1:8b` - Fast and lightweight
- `codellama` - Code-focused

**Configuration:**
```yaml
settings:
  ai:
    provider: ollama
    model: llama3.3:70b
    base_url: http://localhost:11434/v1
```

**Setup:**
```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# Download model
ollama pull llama3.3:70b

# Use with Atmos
atmos ai chat
```

**Best For:**
- **Privacy** - Data never leaves premises
- **Offline** - Air-gapped environments
- **Compliance** - Data residency requirements
- **Cost** - Zero API costs

#### 6. AWS Bedrock (Enterprise)

**Models:**
- `anthropic.claude-sonnet-4-20250514-v2:0` (default)
- `anthropic.claude-3-haiku-20240307-v1:0`
- `anthropic.claude-3-opus-20240229-v1:0`

**Configuration:**
```yaml
settings:
  ai:
    provider: bedrock
    model: anthropic.claude-sonnet-4-20250514-v2:0
    base_url: us-east-1  # AWS region
```

**Authentication:**
- Uses standard AWS SDK credential chain
- Respects `AWS_PROFILE`, `AWS_ACCESS_KEY_ID`, etc.
- Supports IAM roles (ECS/EKS/EC2)

**Best For:**
- **AWS-native organizations** - Existing AWS infrastructure
- **Compliance** - Data stays in AWS
- **Audit** - CloudTrail logging
- **Cost** - Leverage AWS commits

#### 7. Azure OpenAI (Enterprise)

**Models:**
- `gpt-4o` (deployment name)
- `gpt-4-turbo`
- `gpt-35-turbo`

**Configuration:**
```yaml
settings:
  ai:
    provider: azureopenai
    model: gpt-4o-deployment  # Your deployment name
    api_key_env: AZURE_OPENAI_API_KEY
    base_url: https://<resource>.openai.azure.com
```

**Best For:**
- **Azure-native organizations** - Existing Azure infrastructure
- **Data residency** - Data stays in Azure region
- **Compliance** - Azure certifications (SOC2, HIPAA, ISO)
- **SLA** - Enterprise SLA guarantees

### Provider Comparison

| Feature | Anthropic | OpenAI | Gemini | Grok | Ollama | Bedrock | Azure OpenAI |
|---------|-----------|--------|--------|------|--------|---------|--------------|
| **Context Window** | 200K | 128K | 2M | 128K | Varies | 200K | 128K |
| **Tool Use** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Streaming** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Cost** | Medium | Medium | Low | Medium | Free | Medium | Medium |
| **Privacy** | Cloud | Cloud | Cloud | Cloud | **Local** | AWS | Azure |
| **Compliance** | SOC2 | SOC2 | SOC2 | SOC2 | N/A | **Enterprise** | **Enterprise** |
| **Offline** | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ |

### Multi-Provider Configuration

**Full Configuration Example:**

```yaml
settings:
  ai:
    enabled: true
    default_provider: anthropic

    providers:
      anthropic:
        model: claude-sonnet-4-20250514
        api_key_env: ANTHROPIC_API_KEY
        max_tokens: 4096

      openai:
        model: gpt-4o
        api_key_env: OPENAI_API_KEY
        max_tokens: 4096

      gemini:
        model: gemini-1.5-pro
        api_key_env: GOOGLE_API_KEY
        max_tokens: 4096

      grok:
        model: grok-2-latest
        api_key_env: XAI_API_KEY
        max_tokens: 4096

      ollama:
        model: llama3.3:70b
        base_url: http://localhost:11434/v1

      bedrock:
        model: anthropic.claude-sonnet-4-20250514-v2:0
        base_url: us-east-1

      azureopenai:
        model: gpt-4o-deployment
        api_key_env: AZURE_OPENAI_API_KEY
        base_url: https://company.openai.azure.com
```

**Provider Switching:**

Users can switch providers:
- **CLI**: `atmos ai chat --provider ollama`
- **TUI**: Press `Ctrl+P` to select provider
- **Per-Session**: Each session remembers its provider

---

## Advanced Features

All advanced features listed below are already implemented and production-ready. This section describes future enhancements planned beyond the current release.

---

## Security & Privacy

### Security Model

#### Tool Permissions

**Three-Tier Permission System:**

1. **Allowed Tools** - Execute without prompting
   - Read-only operations
   - Safe analysis commands
   - Example: `atmos_describe_component`

2. **Restricted Tools** - Require confirmation
   - File modifications
   - Potentially risky operations
   - Example: `file_write`

3. **Blocked Tools** - Never execute
   - Destructive operations
   - Example: `atmos_terraform_destroy`

**YOLO Mode:**
- Bypass all confirmations (use with extreme caution)
- Recommended only for trusted environments
- Disabled by default

**Audit Logging:**
```yaml
settings:
  ai:
    tools:
      audit:
        enabled: true
        path: .atmos/ai-audit.log
        retention_days: 90
```

#### Data Privacy

**Local Storage:**
- All session data stored locally in SQLite
- ATMOS.md stored in project directory
- No data sent to external services (except AI API)

**Privacy-First Options:**

1. **Ollama** - Complete on-premises deployment
   - All data stays local
   - No internet required
   - HIPAA/GDPR compliant

2. **Enterprise Providers** - Data residency controls
   - **AWS Bedrock** - Data stays in AWS
   - **Azure OpenAI** - Data stays in Azure region

3. **Context Control** - User controls what AI sees
   - Explicit permission for file access
   - `.atmosignore` for sensitive files
   - Configurable context limits

### Compliance & Enterprise

#### Enterprise Features

**AWS Bedrock:**
- Data never leaves AWS infrastructure
- CloudTrail audit logging
- IAM-based access control
- VPC isolation possible
- Encryption at rest and in transit

**Azure OpenAI:**
- Azure AD integration
- Managed identity support
- Private endpoint support
- Customer-managed encryption keys (BYOK)
- Compliance certifications (SOC2, HIPAA, ISO)

**Audit Trail:**
- All tool executions logged
- Session activity tracked
- Agent selection recorded
- Timestamps and user context

#### Security Best Practices

**Configuration:**
```yaml
settings:
  ai:
    # Restrict tool access
    tools:
      allowed_tools:
        - atmos_describe_*
        - atmos_list_*
      blocked_tools:
        - atmos_terraform_destroy
        - atmos_terraform_apply

    # Enable audit logging
    tools:
      audit:
        enabled: true
        path: .atmos/ai-audit.log

    # Use enterprise provider
    provider: bedrock  # or azureopenai

    # Limit context exposure
    context:
      send_by_default: false
      prompt_on_send: true
```

**Recommendations:**
1. Use enterprise providers (Bedrock/Azure) for production
2. Enable audit logging
3. Configure tool restrictions
4. Use YOLO mode only in dev environments
5. Review `.atmosignore` for sensitive files

---

## User Experience

### Terminal UI (TUI)

**Built with Bubble Tea** - Modern, responsive terminal interface

#### Key Features

- **Session Management** - Visual session list with CRUD operations
- **Provider Switching** - Switch AI provider mid-conversation (Ctrl+P)
- **Agent Selection** - Choose specialized agents (Ctrl+A)
- **Markdown Rendering** - Rich formatting with Glamour
- **Syntax Highlighting** - Code blocks with Chroma
- **History Navigation** - ↑/↓ arrows for previous messages
- **Multi-line Input** - Shift+Enter for new lines
- **Status Line** - Current session, provider, agent displayed

#### Keyboard Shortcuts

```
Ctrl+N      Create new session
Ctrl+L      Switch session
Ctrl+P      Switch provider
Ctrl+A      Switch agent
Ctrl+C/Q    Quit
↑/↓         Navigate history
Shift+Enter Multi-line input
d           Delete session (in session list)
r           Rename session (in session list)
f           Filter by provider (in session list)
```

#### Session List UI

```
┌─ Atmos AI Sessions ────────────────────────────────────┐
│ Filter: All | Claude | GPT | Gemini | Grok | Ollama    │
├────────────────────────────────────────────────────────┤
│ [Claude] vpc-migration                                  │
│   Created: 2025-10-28  Messages: 45                    │
│                                                          │
│ [GPT] security-audit                                    │
│   Created: 2025-10-27  Messages: 23                    │
│                                                          │
│ [Ollama] cost-analysis                                  │
│   Created: 2025-10-26  Messages: 12                    │
├────────────────────────────────────────────────────────┤
│ Ctrl+N: New | Ctrl+L: Switch | d: Delete | r: Rename   │
│ f: Filter | Esc: Back                                  │
└────────────────────────────────────────────────────────┘
```

#### Chat Interface

```
┌─ Atmos AI Chat ────────────────────────────────────────┐
│ Session: vpc-migration | Provider: Claude | Agent: General│
├────────────────────────────────────────────────────────┤
│                                                          │
│ You: What's the VPC CIDR for prod?                      │
│                                                          │
│ AI: The production VPC uses 10.2.0.0/16...              │
│                                                          │
│ You: ▊                                                   │
│                                                          │
├────────────────────────────────────────────────────────┤
│ Ctrl+P: Provider | Ctrl+A: Agent | Ctrl+N: New Session │
│ Ctrl+C: Quit                                            │
└────────────────────────────────────────────────────────┘
```

### CLI Commands

#### Interactive Chat

```bash
# Start new session
atmos ai chat

# Resume named session
atmos ai chat --session vpc-migration

# Use specific provider
atmos ai chat --provider ollama

# Use specific agent
atmos ai chat --agent stack-analyzer
```

#### Single-Shot Queries

```bash
# Quick question
atmos ai ask "What is Atmos?"

# With specific provider
atmos ai ask --provider bedrock "List all stacks"

# With specific agent
atmos ai ask --agent security-auditor "Audit prod VPC security"
```

#### Session Management

```bash
# List all sessions
atmos ai sessions list

# Delete session
atmos ai sessions delete vpc-migration

# Clean old sessions
atmos ai sessions clean --older-than 30d
```

#### Memory Management

```bash
# Initialize project memory
atmos ai memory init

# View current memory
atmos ai memory view

# Edit memory in $EDITOR
atmos ai memory edit

# Update specific section
atmos ai memory update --section project_context
```

#### MCP Server

```bash
# Start MCP server (stdio)
atmos mcp-server

# Start HTTP server
atmos mcp-server --transport http --port 3000
```

---

## Configuration Reference

### Complete Configuration Example

```yaml
# atmos.yaml
settings:
  ai:
    # Core settings
    enabled: true
    default_provider: anthropic

    # Provider configurations
    providers:
      anthropic:
        model: claude-sonnet-4-20250514
        api_key_env: ANTHROPIC_API_KEY
        max_tokens: 4096

      openai:
        model: gpt-4o
        api_key_env: OPENAI_API_KEY
        max_tokens: 4096

      gemini:
        model: gemini-1.5-pro
        api_key_env: GOOGLE_API_KEY
        max_tokens: 4096

      grok:
        model: grok-2-latest
        api_key_env: XAI_API_KEY
        max_tokens: 4096

      ollama:
        model: llama3.3:70b
        base_url: http://localhost:11434/v1

      bedrock:
        model: anthropic.claude-sonnet-4-20250514-v2:0
        base_url: us-east-1

      azureopenai:
        model: gpt-4o-deployment
        api_key_env: AZURE_OPENAI_API_KEY
        base_url: https://company.openai.azure.com

    # Session management
    sessions:
      enabled: true
      storage: sqlite
      path: .atmos/sessions
      max_sessions: 10
      auto_save: true
      retention_days: 30
      max_history_messages: 50

    # Project memory
    memory:
      enabled: true
      file_path: ATMOS.md
      auto_update: false
      create_if_missing: true
      sections:
        - project_context
        - common_commands
        - stack_patterns
        - frequent_issues

    # Tool execution
    tools:
      enabled: true
      require_confirmation: true

      allowed_tools:
        - atmos_describe_*
        - atmos_list_*
        - file_read

      restricted_tools:
        - file_write
        - atmos_terraform_plan

      blocked_tools:
        - atmos_terraform_apply
        - atmos_terraform_destroy

      yolo_mode: false

      audit:
        enabled: true
        path: .atmos/ai-audit.log
        retention_days: 90

    # Agents (custom agents can be added)
    default_agent: general

    # MCP server
    mcp:
      enabled: true
      transport: stdio  # or http
      http:
        port: 3000
        host: localhost

  # LSP integration (independent from AI)
  lsp:
    enabled: true
    servers:
      yaml-ls:
        command: yaml-language-server
        args: ["--stdio"]
        filetypes: ["yaml", "yml"]
        root_patterns: ["atmos.yaml", ".git"]

      terraform-ls:
        command: terraform-ls
        args: ["serve"]
        filetypes: ["tf", "tfvars", "hcl"]
        root_patterns: [".terraform", ".git"]
```

---

## Roadmap

### Recently Completed Features

**Token Caching (Prompt Caching)** - *Completed: October 2025*
- **Provider Support:** 6 of 7 providers support token caching (all except Ollama)
- **Cost Savings:** Up to 90% reduction on cached tokens (Anthropic: 90%, OpenAI/Azure: 50%, Gemini: free, Grok: 75%, Bedrock: 90%)
- **Implementation:** Automatic for most providers, explicit configuration for Anthropic
- **Cache Metrics:** Token usage includes `cached` and `cache_creation` fields in JSON output
- **Documentation:** Complete in website/docs/ai/providers.mdx
- **Impact:** Reduces API costs by 50-74% in typical usage with system prompts and ATMOS.md

**Technical Details:**
- Added `SendMessageWithSystemPromptAndTools()` to Client interface
- Implemented for all 7 providers (Anthropic, OpenAI, Gemini, Grok, Bedrock, Azure, Ollama)
- Updated chat.go and executor.go to use cached method
- Cache metrics tracked and returned in ExecutionResult
- Comprehensive test coverage with updated mock clients

---

## Future Roadmap

### Phase 3: Future Enhancements (6+ months)

**Advanced Agent Features:**
- Agent analytics and metrics
- Agent dependency resolution
- Agent composition and workflows
- Private agent registries

**Enhanced Analytics:**
- Token usage tracking
- Cost analysis per session
- Agent performance metrics
- Tool usage statistics

**Advanced LSP:**
- More language servers (HCL, JSON Schema)
- Custom diagnostic rules
- Auto-fix suggestions
- Integration with IDE plugins

**Multi-Agent Workflows:**
- Agent delegation
- Parallel agent execution
- Agent collaboration
- Task decomposition

### Phase 4: Enterprise Features (12+ months)

**Private Agent Registries:**
- Company-specific agents
- Access control and permissions
- Internal agent marketplace

**Team Collaboration:**
- Shared sessions across team
- Session templates
- Collaborative editing

**Advanced Security:**
- Agent sandboxing
- Runtime permission requests
- Security scanning
- GPG signature verification

**Centralized Management:**
- Organization-wide policies
- Centralized audit logging
- Usage quotas and limits
- Cost allocation

---

## Technical Architecture

### Design Principles

1. **Interface-Driven Design** - All major components use interfaces for testability
2. **Registry Pattern** - Extensible registration for commands, tools, agents, providers
3. **Options Pattern** - Avoid parameter drilling with functional options
4. **Context Usage** - Context only for cancellation, deadlines, request-scoped values
5. **Package Organization** - Purpose-built packages, avoid utils bloat
6. **Comment Preservation** - Never delete helpful comments
7. **Testing Strategy** - Unit tests with mocks over integration tests

### Code Quality Standards

- **Test Coverage:** >80% (enforced by CodeCov)
- **Linting:** golangci-lint with godot, gofmt, gofumpt
- **Code Review:** All changes require PR review
- **Documentation:** Comprehensive godoc, website docs, PRDs
- **Performance:** Session operations <10ms, tool execution varies

### Dependencies

**Core Dependencies:**
- `github.com/charmbracelet/bubbletea` - TUI framework
- `modernc.org/sqlite` - Pure Go SQLite (no CGO)
- `github.com/anthropic-ai/anthropic-sdk-go` - Anthropic API
- `github.com/openai/openai-go` - OpenAI API
- `github.com/google-gemini/generative-ai-go` - Gemini API
- `github.com/aws/aws-sdk-go-v2` - AWS Bedrock
- `github.com/go-yaml/yaml` - YAML parsing
- `github.com/spf13/cobra` - CLI framework

---

## Success Metrics

### Adoption Metrics

- **Active Users** - Monthly active users of AI features
- **Session Count** - Number of AI sessions created
- **Tool Executions** - Number of tool invocations
- **Agent Usage** - Distribution of agent usage

### Quality Metrics

- **Response Accuracy** - Qualitative user feedback
- **Context Preservation** - Session continuity success rate
- **Tool Success Rate** - Percentage of successful tool executions
- **Error Rate** - AI and tool error rates

### Performance Metrics

- **Response Latency** - Time to first response token
- **Session Load Time** - Time to load session history
- **Tool Execution Time** - Average tool execution duration
- **Memory Usage** - Peak memory during operations

### Cost Metrics

- **Token Usage** - Average tokens per session
- **API Costs** - Cost per user/session
- **Provider Distribution** - Usage across providers
- **Ollama Adoption** - On-premises deployment rate

---

## Documentation

### User Documentation

**Website Documentation** (`website/docs/ai/`):
- `ai.mdx` - Main AI documentation and quick start
- `configuration.mdx` - Complete configuration guide
- `providers.mdx` - AI provider comparison and setup
- `tools.mdx` - Tool system and permissions
- `agents.mdx` - Agent system and marketplace
- `memory.mdx` - Project memory (ATMOS.md)
- `sessions.mdx` - Session management
- `mcp-server.mdx` - MCP integration guide
- `claude-code-integration.mdx` - IDE integration
- `troubleshooting.mdx` - Troubleshooting guide

**CLI Documentation:**
- `atmos ai --help` - Main AI help
- `atmos ai chat --help` - Interactive chat help
- `atmos ai ask --help` - Single-shot query help
- `atmos ai sessions --help` - Session management help
- `atmos ai memory --help` - Memory management help

### Developer Documentation

**PRD Documents:**
- This document - Complete Atmos AI PRD

**Code Documentation:**
- Comprehensive godoc comments
- Architecture diagrams
- Design decision records

**Contributing:**
- `docs/developing-atmos-commands.md` - Command development
- `CLAUDE.md` - Development guidelines
- Agent development guide (future)

---

## Acknowledgments

Atmos AI builds upon patterns and ideas from industry-leading AI systems while maintaining its unique focus on infrastructure-as-code management. The project benefits from:

- **Open Source Community** - Contributions and feedback
- **AI Research** - Advances in LLM capabilities
- **Industry Standards** - MCP and LSP protocols
- **CloudPosse Team** - Vision, architecture, and implementation

---

## Appendix

### Glossary

- **Agent** - Specialized AI assistant with focused expertise and tool access
- **ATMOS.md** - Project memory file for persistent context
- **Auto-Compact** - Intelligent conversation history summarization
- **LSP** - Language Server Protocol for real-time validation
- **MCP** - Model Context Protocol for standardized AI-tool communication
- **Session** - Persistent conversation with message history
- **Tool** - Executable operation the AI can perform (e.g., validate, describe)
- **TUI** - Terminal User Interface built with Bubble Tea

### Version History

- **v1.0** (2025-10-20) - Initial release with sessions, tools, memory
- **v1.5** (2025-10-25) - Added MCP integration, LSP support
- **v2.0** (2025-10-30) - Agent system, enterprise providers, production ready

### References

- [Atmos Documentation](https://atmos.tools)
- [Model Context Protocol](https://modelcontextprotocol.io)
- [Language Server Protocol](https://microsoft.github.io/language-server-protocol/)
- [Anthropic Documentation](https://docs.anthropic.com)
- [OpenAI Documentation](https://platform.openai.com/docs)
- [Google Gemini API](https://ai.google.dev/gemini-api/docs)
- [Ollama](https://ollama.com)

---

**Document Status:** Production Ready
**Maintenance:** Living document, updated with each release
**Contact:** https://github.com/cloudposse/atmos/issues
