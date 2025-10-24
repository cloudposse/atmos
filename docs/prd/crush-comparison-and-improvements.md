# Crush vs Atmos AI: Feature Comparison & Improvement Recommendations

**Document Version:** 2.0
**Date:** 2025-10-23 (Updated: 2025-10-23)
**Status:** Active - Tracking Implementation Progress
**Author:** AI Analysis based on https://github.com/charmbracelet/crush

---

## Executive Summary

Crush is a **full-featured AI coding agent** focused on general-purpose software development, while Atmos AI is a **domain-specific AI assistant** focused on infrastructure-as-code management.

**Implementation Status (as of 2025-10-23):**
- ✅ **Session Management** - COMPLETE (SQLite-backed, TUI with create/delete/rename/filter)
- ✅ **Tool Execution** - COMPLETE (Atmos-specific tools with permission system)
- ✅ **Project Memory** - COMPLETE (ATMOS.md with auto-update)
- ✅ **MCP Support** - COMPLETE (stdio/HTTP transports for Claude Desktop/VSCode)
- ⚠️ **Enhanced TUI** - PARTIAL (sessions complete, syntax highlighting pending)
- ❌ **LSP Integration** - PENDING (medium priority)

**Key Achievement:** Atmos AI has successfully adopted Crush's productivity patterns (sessions, tools, memory) while maintaining its domain-specific intelligence advantage.

---

## Feature Comparison Matrix

| Feature Category | Crush | Atmos AI (Original) | Atmos AI (Current) | Status |
|-----------------|-------|---------------------|--------------------| -------|
| **Session Management** | ✅ SQLite-backed | ❌ Stateless | ✅ **SQLite-backed** | ✅ COMPLETE |
| **Session TUI** | ✅ Create/Switch | ❌ None | ✅ **Create/Delete/Rename/Filter** | ✅ COMPLETE |
| **Provider-Aware Sessions** | ❌ No | ❌ No | ✅ **Yes** | ✅ COMPLETE |
| **Tool Execution** | ✅ Bash, file edit | ❌ Read-only | ✅ **Atmos tools + file ops** | ✅ COMPLETE |
| **Permission System** | ✅ Granular + YOLO | ⚠️ Basic | ✅ **Granular allowlists** | ✅ COMPLETE |
| **Project Memory** | ✅ CRUSH.md | ❌ None | ✅ **ATMOS.md** | ✅ COMPLETE |
| **MCP Support** | ✅ stdio/HTTP/SSE | ❌ None | ✅ **stdio/HTTP** | ✅ COMPLETE |
| **LSP Integration** | ✅ Multi-language | ❌ None | ❌ **Pending** | 🟡 TODO |
| **Interactive Chat** | ✅ Full TUI | ✅ Basic | ✅ **Enhanced TUI** | ⚠️ PARTIAL |
| **Syntax Highlighting** | ✅ Yes | ❌ No | ❌ **Pending** | 🟡 TODO |
| **Model Switching** | ✅ Mid-session | ❌ Restart required | ❌ **Pending** | 🟢 LOW |
| **Git Attribution** | ✅ Co-authored-by | ❌ None | ❌ **Pending** | 🟢 LOW |
| **Domain Context** | ❌ No | ✅ Atmos-specific | ✅ **Atmos-specific** | ✅ Core |
| **Stack Context** | ❌ N/A | ✅ Stack analysis | ✅ **Stack analysis** | ✅ Core |
| **AI Providers** | ✅ 10+ | ✅ 4 | ✅ **4 (+ Ollama pending)** | ✅ Core |

---

## Implementation Status by Priority

### ✅ COMPLETED - HIGH PRIORITY Features

#### 1. ✅ Session Management & Context Persistence - **COMPLETE**

**✅ Implemented in Atmos AI:**
- ✅ SQLite-backed session storage (`pkg/ai/session/storage/sqlite.go`)
- ✅ Full CRUD operations (Create, Read, Update, Delete sessions)
- ✅ Message history persistence across CLI invocations
- ✅ Provider-aware sessions (each session remembers its AI provider and model)
- ✅ TUI session management (Ctrl+N: create, Ctrl+L: switch, d: delete, r: rename)
- ✅ Session filtering by provider (All/Claude/GPT/Gemini/Grok)
- ✅ Enhanced session display with provider badges, creation date, message count
- ✅ Configurable retention (auto-cleanup after N days)

**Implementation Files:**
- `pkg/ai/session/manager.go` - Session lifecycle management
- `pkg/ai/session/storage/sqlite.go` - SQLite storage implementation
- `pkg/ai/tui/sessions.go` - Session list UI with CRUD operations
- `pkg/ai/tui/create_session.go` - Session creation form with provider selection

**Original Requirements (from Crush):**
- SQLite-backed session storage
- Maintains conversation history across restarts
- Multiple concurrent sessions per project
- Automatic state persistence
- Session isolation with independent contexts

**Why It Mattered for Atmos:**
- Users work on long-running infrastructure changes
- Need to maintain context across multiple CLI invocations
- Should remember previous questions about specific stacks/components
- Enable context-aware follow-up questions
- Reduce repetitive context loading

**Current Limitation:**
```bash
# Every invocation is stateless
atmos ai ask "What's in the VPC component?"
# AI responds...

atmos ai ask "What are the security groups?"
# AI has no memory of previous VPC question
```

**Proposed Solution:**
```yaml
# atmos.yaml
settings:
  ai:
    enabled: true
    sessions:
      enabled: true
      storage: sqlite  # or file-based JSON
      path: .atmos/sessions
      max_sessions: 10
      auto_save: true
      retention_days: 30
```

**Implementation Details:**

**Database Schema:**
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    name TEXT,
    project_path TEXT,
    model TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    metadata JSON
);

CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT,
    role TEXT,  -- user, assistant, system
    content TEXT,
    created_at TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE session_context (
    session_id TEXT,
    context_type TEXT,  -- stack_file, component, setting
    context_key TEXT,
    context_value TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
```

**New Commands:**
```bash
# List sessions
atmos ai sessions

# Resume session
atmos ai chat --session vpc-refactor

# Create named session
atmos ai chat --new-session "prod-migration"

# Clear old sessions
atmos ai sessions clean --older-than 30d
```

**Files to Create:**
- `pkg/ai/session/session.go` - Session management interface
- `pkg/ai/session/storage.go` - SQLite storage implementation
- `pkg/ai/session/manager.go` - Session lifecycle management
- `cmd/ai_sessions.go` - Session management commands

---

#### 2. ✅ Tool Execution System - **COMPLETE**

**✅ Implemented in Atmos AI:**
- ✅ Atmos-specific tools (`atmos_describe_component`, `atmos_list_stacks`, `atmos_validate_stacks`, etc.)
- ✅ File operations (`file_read`, `file_write`, `file_search`)
- ✅ Granular permission system with allowlist/restricted/blocked categories
- ✅ Interactive permission prompts with tool details
- ✅ Tool result streaming to AI for analysis
- ✅ YOLO mode for bypassing confirmations (configurable)
- ✅ Audit logging for tool executions

**Implementation Files:**
- `pkg/ai/tools/interface.go` - Tool interface definition
- `pkg/ai/tools/atmos/tools.go` - Atmos-specific tool implementations
- `pkg/ai/tools/permission/manager.go` - Permission checking logic
- `pkg/ai/tools/executor.go` - Tool execution engine
- `pkg/ai/agent/*/client.go` - Tool use integration (Anthropic, OpenAI, Gemini, Grok)

**Supported Tools:**
- `atmos_describe_component` - Describe component configuration in stack
- `atmos_describe_stacks` - Describe all stacks
- `atmos_list_stacks` - List available stacks
- `atmos_list_components` - List available components
- `atmos_validate_stacks` - Validate stack configurations
- `atmos_validate_component` - Validate specific component
- `file_read` - Read file contents
- `file_write` - Write file contents
- `file_search` - Search for files/content

**Original Requirements (from Crush):**
- Bash command execution with security controls
- File read/write/edit operations
- Search and grep capabilities
- Granular permission system with allowlists
- Tool result streaming to AI

**Why It Mattered for Atmos:**
- AI could validate configurations automatically
- Execute `atmos describe component` to gather detailed context
- Run `atmos validate stacks` to check for issues
- Edit stack files based on user requests
- Analyze terraform plans
- Execute workflows

**Current Limitation:**
```bash
User: "Can you validate my VPC configuration?"
AI: "You should run: atmos validate component vpc -s dev-use1"
User: *has to manually run the command*
```

**Proposed Solution:**
```bash
User: "Can you validate my VPC configuration?"
AI: "I'll validate that for you..."
AI: *executes: atmos validate component vpc -s dev-use1*
AI: "Found 2 validation errors: [details]"
```

**Implementation:**

**Tool Interface:**
```go
package tools

// Tool represents an executable operation the AI can perform.
type Tool interface {
    Name() string
    Description() string
    Parameters() []Parameter
    Execute(ctx context.Context, params map[string]interface{}) (*Result, error)
    RequiresPermission() bool
}

// Parameter defines a tool parameter.
type Parameter struct {
    Name        string
    Description string
    Type        string // string, int, bool, array
    Required    bool
}

// Result contains tool execution results.
type Result struct {
    Success bool
    Output  string
    Error   error
    Data    map[string]interface{}
}
```

**Built-in Atmos Tools:**
```go
// AtmosDescribeComponentTool - Describe a component in a stack
type AtmosDescribeComponentTool struct{}

func (t *AtmosDescribeComponentTool) Name() string {
    return "atmos_describe_component"
}

func (t *AtmosDescribeComponentTool) Description() string {
    return "Describe an Atmos component configuration in a specific stack"
}

func (t *AtmosDescribeComponentTool) Execute(ctx context.Context, params map[string]interface{}) (*Result, error) {
    component := params["component"].(string)
    stack := params["stack"].(string)

    // Execute: atmos describe component <component> -s <stack>
    output, err := exec.DescribeComponent(component, stack, atmosConfig)

    return &Result{
        Success: err == nil,
        Output:  output,
        Error:   err,
    }, nil
}
```

**Other Atmos-Specific Tools:**
- `atmos_describe_stacks` - Describe stacks (replaces current context gathering)
- `atmos_list_stacks` - List available stacks
- `atmos_list_components` - List available components
- `atmos_validate_stacks` - Validate stack configurations
- `atmos_validate_component` - Validate specific component
- `atmos_terraform_plan` - Generate terraform plan (read-only)
- `atmos_workflow_execute` - Execute Atmos workflows
- `file_read` - Read file contents
- `file_edit` - Edit file with diff preview
- `file_search` - Search for files/content

**Permission System Configuration:**
```yaml
settings:
  ai:
    tools:
      enabled: true
      require_confirmation: true  # Prompt before executing

      # Whitelist: These tools can run without prompting
      allowed_tools:
        - atmos_describe_component
        - atmos_describe_stacks
        - atmos_list_stacks
        - atmos_list_components
        - file_read

      # Restricted: Always require confirmation
      restricted_tools:
        - file_edit
        - atmos_terraform_apply
        - atmos_workflow_execute

      # YOLO mode - execute all tools without confirmation (DANGEROUS!)
      yolo_mode: false
```

**TUI Permission Prompt:**
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

**Files to Create:**
- `pkg/ai/tools/interface.go` - Tool interface definition
- `pkg/ai/tools/atmos_tools.go` - Atmos-specific tool implementations
- `pkg/ai/tools/file_tools.go` - File operation tools
- `pkg/ai/tools/permission.go` - Permission checking logic
- `pkg/ai/tools/executor.go` - Tool execution engine

---

#### 3. ✅ Project Memory (ATMOS.md) - **COMPLETE**

**✅ Implemented in Atmos AI:**
- ✅ `ATMOS.md` file for persistent project knowledge
- ✅ Automatically created from template if missing
- ✅ Context injection to AI prompts
- ✅ Configurable sections (project_context, common_commands, stack_patterns, etc.)
- ✅ Manual and auto-update modes
- ✅ Preserves user edits while allowing AI updates
- ✅ Reduces repetitive context loading

**Implementation Files:**
- `pkg/ai/memory/manager.go` - Project memory management
- `pkg/ai/memory/parser.go` - Markdown section parser
- `templates/atmos_md.tmpl` - Default ATMOS.md template

**Configuration:**
```yaml
settings:
  ai:
    memory:
      enabled: true
      file_path: ATMOS.md
      create_if_missing: true
      auto_update: false  # Manual updates recommended
```

**Original Requirements (from Crush):**
- `CRUSH.md` file for persistent project knowledge
- Automatically updated by AI during sessions
- Stores frequently used commands
- Remembers coding style preferences
- Maintains codebase structure notes
- Avoids re-discovering same information

**Why It Mattered for Atmos:**
- Store common stack patterns for the project
- Remember organization-specific naming conventions
- Cache frequently asked questions and answers
- Document project-specific Atmos configurations
- Maintain knowledge of infrastructure patterns
- Remember team conventions and standards

**Example ATMOS.md:**
```markdown
# Atmos Project Memory

> This file is automatically maintained by Atmos AI to remember project-specific
> context, patterns, and conventions. Edit freely - AI will preserve manual changes.

## Project Context

**Organization:** acme-corp
**Atmos Version:** 1.89.0
**Primary Regions:** us-east-1, us-west-2, eu-west-1
**Environments:** dev, staging, prod

**Stack Naming Pattern:**
```
{org}-{tenant}-{environment}-{region}-{stage}
Example: acme-core-prod-use1-network
```

**Common Tags:**
- Team (required)
- CostCenter (required)
- Environment (required)
- ManagedBy: "atmos"

## Common Commands

### Build and Plan
```bash
# Plan VPC in dev
atmos terraform plan vpc -s acme-dev-use1-dev

# Plan all components in prod
atmos describe stacks -s acme-prod-use1-prod

# Validate all stacks
atmos validate stacks
```

### Workflows
```bash
# Deploy full environment
atmos workflow deploy-env -s dev

# Rotate secrets
atmos workflow rotate-secrets -s prod
```

## Stack Patterns

### Network Stack Structure
```yaml
# All network stacks inherit from:
import:
  - catalog/stacks/network/baseline
  - catalog/stacks/network/security-groups

# VPC CIDR blocks:
# dev: 10.0.0.0/16
# staging: 10.1.0.0/16
# prod: 10.2.0.0/16
```

### Component Dependencies
```
vpc → subnets → security-groups → rds → eks
```

### Naming Conventions
- Resources: lowercase with hyphens (my-resource-name)
- Terraform locals: snake_case (my_local_var)
- Component names: lowercase (vpc, rds, eks)

## Frequent Issues & Solutions

### Q: Stack not found error
**Problem:** `Error: stack 'acme-dev-use1' not found`
**Solution:** Check stack naming matches pattern and verify `stacks/orgs/acme/` config exists

### Q: YAML validation fails
**Problem:** `invalid YAML: mapping values are not allowed`
**Solution:** Check indentation - YAML requires 2-space indents, not tabs

### Q: Component inheritance not working
**Problem:** Variables from catalog not appearing in component
**Solution:** Verify import path and check for override in stack config

## Infrastructure Patterns

### Multi-Region VPC
All production workloads use multi-region active-passive pattern:
- Primary: us-east-1
- DR: us-west-2
- Cross-region VPC peering enabled
- Route53 health checks for failover

### RDS Configuration
- Production: Multi-AZ, automated backups, encryption at rest
- Non-prod: Single-AZ, daily backups
- All use db.r6g instance family

### EKS Clusters
- Version: 1.28
- Node groups: managed, spot instances for non-prod
- Networking: AWS CNI with prefix delegation
- Security: Pod security standards, OPA Gatekeeper

## Component Catalog Structure

```
components/
  terraform/
    vpc/           # VPC and networking
    rds/           # RDS databases
    eks/           # EKS clusters
    s3/            # S3 buckets
    iam/           # IAM roles and policies
```

## Team Conventions

- All infrastructure changes require PR review
- Use `atmos validate` before committing
- Tag all resources with Team/CostCenter
- Document component changes in CHANGELOG.md
- Run `atmos describe affected` in CI/CD

## Recent Learnings

*AI adds notes here as it learns about the project*

- 2025-10-23: VPC component uses custom DHCP options for Route53 private zones
- 2025-10-23: Production RDS requires manual snapshot before schema changes
- 2025-10-23: EKS addon versions pinned in catalog/eks/addons.yaml
```

**Implementation:**

**File Management:**
```go
// pkg/ai/memory/memory.go
package memory

type ProjectMemory struct {
    FilePath string
    Content  string
    Sections map[string]string
}

func LoadProjectMemory(basePath string) (*ProjectMemory, error) {
    // Look for ATMOS.md in project root
    // Parse sections
    // Return structured memory
}

func (m *ProjectMemory) Update(section, content string) error {
    // Update specific section
    // Preserve user edits
    // Write back to file
}

func (m *ProjectMemory) GetContext() string {
    // Return relevant sections as context for AI
}
```

**AI Integration:**
```go
// When starting chat/ask session:
memory, err := memory.LoadProjectMemory(atmosConfig.BasePath)
if err == nil {
    systemPrompt := fmt.Sprintf(`You are an Atmos AI assistant.

Project Memory:
%s

Use this memory to provide context-aware responses.
Update memory when you learn new patterns or conventions.
`, memory.GetContext())
}
```

**Configuration:**
```yaml
settings:
  ai:
    memory:
      enabled: true
      file: ATMOS.md  # or custom path
      auto_update: true
      sections:
        - project_context
        - common_commands
        - stack_patterns
        - frequent_issues
        - infrastructure_patterns
```

**Files to Create:**
- `pkg/ai/memory/memory.go` - Project memory management
- `pkg/ai/memory/parser.go` - Markdown section parser
- `ATMOS.md.template` - Default template for new projects

---

---

### ⚠️ PARTIAL - MEDIUM PRIORITY Features

#### 4. ✅ MCP (Model Context Protocol) Support - **COMPLETE**

**✅ Implemented in Atmos AI:**
- ✅ stdio transport for local MCP servers (Claude Desktop, VSCode)
- ✅ HTTP transport for remote MCP servers
- ✅ `atmos mcp-server` command to start MCP server
- ✅ Exposes all Atmos tools via MCP protocol
- ✅ Compatible with Claude Desktop, VSCode, and any MCP client
- ✅ Configurable in `atmos.yaml`

**Implementation Files:**
- `pkg/ai/mcp/server.go` - MCP server implementation
- `pkg/ai/mcp/stdio_transport.go` - stdio transport
- `pkg/ai/mcp/http_transport.go` - HTTP transport
- `cmd/mcp_server.go` - MCP server command

**Usage:**
```bash
# Start MCP server (stdio for Claude Desktop)
atmos mcp-server

# Start HTTP server
atmos mcp-server --transport http --port 3000
```

**Configuration:**
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

See documentation: `website/docs/ai/mcp-server.mdx`

---

#### 5. ❌ LSP Integration - **PENDING**

**What Crush Has:**
- Spawns LSP servers (gopls, rust-analyzer, pyright, typescript-language-server)
- JSON-RPC communication over stdin/stdout
- Real-time diagnostics and code intelligence
- Symbol tables and documentation lookups
- Workspace management and file change notifications

**Why It Matters for Atmos:**
- Integrate with `yaml-language-server` for stack file validation
- Real-time syntax checking for Go templates in configs
- HCL/Terraform LSP (`terraform-ls`) for component analysis
- JSON Schema validation for Atmos configuration
- Better error detection and suggestions

**Use Cases:**
```bash
User: "Check my VPC stack configuration for errors"
AI: *Uses yaml-language-server to validate stacks/vpc-dev.yaml*
AI: "Found 3 issues:
1. Line 23: Unknown property 'vpc_cidr' (did you mean 'cidr_block'?)
2. Line 45: Invalid CIDR format
3. Line 67: Required property 'availability_zones' missing"
```

**Configuration:**
```yaml
settings:
  ai:
    lsp:
      enabled: true
      servers:
        yaml:
          command: yaml-language-server
          args: ["--stdio"]
          filetypes: ["yaml", "yml"]
          initialization_options:
            schemas:
              - uri: "file:///path/to/atmos-schema.json"
                fileMatch: ["stacks/**/*.yaml"]

        terraform:
          command: terraform-ls
          args: ["serve"]
          filetypes: ["tf", "tfvars"]
          root_patterns: [".terraform", "main.tf"]
```

**Implementation:**
- `pkg/ai/lsp/client.go` - LSP client wrapper
- `pkg/ai/lsp/manager.go` - Multi-server management
- `pkg/ai/lsp/diagnostics.go` - Diagnostic processing

---

#### 6. ⚠️ Enhanced Interactive Chat UI - **PARTIAL**

**✅ Completed Features:**
- ✅ Full session management TUI (Ctrl+N: create, Ctrl+L: switch, d: delete, r: rename)
- ✅ Provider selection during session creation
- ✅ Session filtering by provider (f key: All/Claude/GPT/Gemini/Grok)
- ✅ Color-coded provider badges ([Claude], [GPT], [Gemini], [Grok])
- ✅ Rich session metadata (name, provider, creation date, message count)
- ✅ Multi-line input with Shift+Enter
- ✅ Message history persistence
- ✅ Enhanced TUI with Bubble Tea components

**❌ Still Pending:**
- ❌ Syntax highlighting for code blocks in AI responses
- ❌ History navigation (↑/↓ arrows for previous messages)
- ❌ Markdown rendering (bold, italic, lists, tables)
- ❌ Interactive code block buttons (Copy, Save, Apply)
- ❌ History search (Ctrl+R)

**Implementation Files:**
- `pkg/ai/tui/chat.go` - Main chat model
- `pkg/ai/tui/sessions.go` - Session list and management UI
- `pkg/ai/tui/create_session.go` - Session creation form
- `pkg/ai/tui/keys.go` - Keyboard navigation handlers

**Improvements Still Needed:**

**A. History Navigation**
```
▲/▼ arrows - Navigate message history
Ctrl+R     - Search history
Ctrl+P/N   - Previous/next message
```

**B. Multi-line Input**
```
Enter      - Send message (single line)
Shift+Enter - New line (multi-line mode)
Ctrl+Enter - Send multi-line message
```

**C. Syntax Highlighting**
```yaml
# Rendered with syntax colors
components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"
```

**D. Session Management UI**
```
┌─ Atmos AI Chat ──────────────────────────┐
│ Session: vpc-refactor    Model: claude   │
├──────────────────────────────────────────┤
│ > You: What's the VPC CIDR?              │
│ < AI: The VPC uses 10.0.0.0/16...        │
│                                          │
│ > You: ▊                                 │
├──────────────────────────────────────────┤
│ Ctrl+S: Sessions | Ctrl+M: Models | Esc  │
└──────────────────────────────────────────┘
```

**E. Markdown Rendering**
- **Bold**, *italic*, `code`
- Bulleted and numbered lists
- Code blocks with syntax highlighting
- Tables
- Links (clickable in supported terminals)

**F. Code Block Handling**
```
┌─────────────────────────────┐
│ components:                 │
│   terraform:                │
│     vpc:                    │
│       vars:                 │
│         cidr: 10.0.0.0/16   │
├─────────────────────────────┤
│ [Copy] [Save] [Apply]       │
└─────────────────────────────┘
```

**Files to Update:**
- `pkg/ai/tui/renderer.go` - Markdown/syntax highlighting (NEW)
- `pkg/ai/tui/history.go` - History navigation (NEW)

---

### 🟢 LOW PRIORITY - Nice to Have

#### 7. Dynamic Model Switching

**Feature:**
- Switch models mid-session while preserving context
- Compare responses from different models
- Auto-select best model for query type

**Configuration:**
```yaml
settings:
  ai:
    multi_model:
      enabled: true
      models:
        - name: claude-sonnet
          provider: anthropic
          model: claude-3-5-sonnet-20241022
          use_for: [general, complex]

        - name: claude-haiku
          provider: anthropic
          model: claude-3-5-haiku-20241022
          use_for: [quick, simple]

        - name: gpt-4
          provider: openai
          model: gpt-4-turbo
          use_for: [code_generation]

      auto_select: true  # AI chooses best model
```

**Usage:**
```bash
# Switch model in chat
/model claude-haiku

# Compare responses
/compare "Explain Atmos stacks"
# Shows responses from multiple models side-by-side
```

---

#### 8. ❌ Ollama/LLAMA Support - **PENDING (Recommended)**

**What is LLAMA/Ollama:**
- **LLAMA** - Meta's open-source LLM family (Llama 3.1, 3.2, 3.3)
- **Ollama** - Popular local LLM runtime (https://ollama.com)
- Runs models locally on your hardware
- OpenAI-compatible API (easy integration)

**Model Quality Assessment:**

| Model | Size | Quality | Best For |
|-------|------|---------|----------|
| Llama 3.3 70B | ~40GB | ⭐⭐⭐⭐⭐ Excellent | Production (rivals GPT-4) |
| Llama 3.1 8B | ~5GB | ⭐⭐⭐ Good | Quick queries, cost-sensitive |
| Llama 3.2 3B | ~2GB | ⭐⭐ Fair | Simple tasks only |

**Pros for Atmos AI:**
- ✅ **Privacy** - Infrastructure configs never leave premises (critical for enterprises)
- ✅ **Cost** - Zero API costs after initial setup
- ✅ **Offline** - Works in air-gapped environments
- ✅ **Compliance** - Meets data residency requirements
- ✅ **Control** - Fine-tunable for Atmos-specific use cases

**Cons:**
- ❌ **Setup complexity** - Users must install Ollama + download models (5-40GB)
- ❌ **Performance** - Requires GPU for acceptable speeds with larger models
- ❌ **Quality variance** - Smaller models (3B-8B) inferior to cloud APIs
- ❌ **Maintenance** - Users responsible for updates

**Implementation Effort: LOW** (OpenAI-compatible API, 90% code reuse)

**Configuration:**
```yaml
# atmos.yaml
settings:
  ai:
    enabled: true
    provider: "ollama"  # New provider
    model: "llama3.3:70b"
    api_key_env: ""  # Not needed for Ollama
    base_url: "http://localhost:11434/v1"  # Ollama default
```

**User Setup:**
```bash
# 1. Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# 2. Download model (one-time, ~40GB for 70B)
ollama pull llama3.3:70b

# 3. Configure Atmos (see above)

# 4. Use normally
atmos ai chat
```

**Target Use Cases:**
- Government/defense contractors (air-gapped environments)
- Financial institutions (data sovereignty requirements)
- Healthcare (HIPAA compliance)
- Cost-sensitive high-volume users
- Teams with GPU infrastructure

**Implementation Files to Create:**
- `pkg/ai/agent/ollama/client.go` - Ollama client (wraps OpenAI client)

**Priority Recommendation:** 🟢 LOW (after LSP and TUI polish, but before Git integration)

**Strategic Value:** HIGH - Differentiates Atmos from cloud-only AI tools, addresses enterprise privacy requirements.

---

#### 9. Auto-Updating Provider Database

**Feature:** Similar to Crush's Catwalk system
- Automatically discover new AI models
- Update model capabilities from central registry
- Download provider configurations

**Configuration:**
```yaml
settings:
  ai:
    providers:
      auto_update: true
      registry_url: https://ai-models.atmos.tools/providers.json
      update_interval: 24h  # Check daily
```

**Registry Format:**
```json
{
  "providers": [
    {
      "name": "anthropic",
      "models": [
        {
          "id": "claude-3-5-sonnet-20241022",
          "name": "Claude 3.5 Sonnet",
          "context_window": 200000,
          "capabilities": ["tool_use", "vision"],
          "cost_per_1k_tokens": 0.003
        }
      ]
    }
  ],
  "last_updated": "2025-10-23T00:00:00Z"
}
```

---

#### 9. Advanced Permission System

**Features:**
- Per-tool permission levels
- User/group-based permissions
- Audit logging
- YOLO mode for CI/CD

**Configuration:**
```yaml
settings:
  ai:
    tools:
      permission_mode: prompt  # prompt, allow, deny, yolo

      policies:
        # Safe tools - always allow
        - tools: [atmos_list_*, atmos_describe_*, file_read]
          permission: allow

        # Restricted tools - always prompt
        - tools: [file_edit, atmos_terraform_apply]
          permission: prompt

        # Dangerous tools - deny in production
        - tools: [atmos_terraform_destroy]
          permission: deny
          environments: [prod]

      audit:
        enabled: true
        log_file: .atmos/ai-audit.log
```

**Audit Log:**
```json
{
  "timestamp": "2025-10-23T10:30:00Z",
  "user": "john.doe",
  "session_id": "abc123",
  "tool": "atmos_describe_component",
  "params": {"component": "vpc", "stack": "prod-use1"},
  "permission": "allowed",
  "result": "success"
}
```

---

#### 10. Git Integration

**Features:**
- Co-authored-by attribution
- Auto-generate PR descriptions
- Commit message assistance
- Change impact analysis

**Configuration:**
```yaml
settings:
  ai:
    git:
      enabled: true
      attribution:
        co_authored_by: true
        generated_with: true

      commit_messages:
        auto_generate: true
        template: |
          {summary}

          {details}

          Generated with Atmos AI
          Co-Authored-By: Atmos AI <ai@atmos.tools>
```

**Usage:**
```bash
# Generate commit message
atmos ai commit
# AI analyzes: git diff, git status
# AI suggests: "feat(vpc): expand CIDR block to /16..."

# Generate PR description
atmos ai pr --base main
# AI analyzes affected components
# AI generates description with impact analysis
```

---

## Atmos AI Strengths (Things Crush Doesn't Have)

### ✅ Domain-Specific Intelligence

**What Makes Atmos AI Special:**

1. **Deep Atmos Knowledge**
   - Understands stacks, components, inheritance hierarchy
   - Knows catalog structure and import patterns
   - Familiar with Terraform/Helmfile integration patterns
   - Understands Atmos-specific YAML syntax

2. **Context-Aware Help**
   - Analyzes your actual Atmos configuration
   - Provides project-specific recommendations
   - Understands your stack naming conventions
   - Recognizes organization patterns

3. **Stack Analysis**
   - Reads and parses stack YAML files
   - Understands component dependencies
   - Analyzes inheritance chains
   - Validates configuration correctness

4. **Intelligent Context Sending**
   - Detects when stack context is needed
   - Prompts for user consent before sending
   - Respects privacy with ATMOS_AI_SEND_CONTEXT env var
   - Configurable context limits

5. **Atmos-Specific Commands**
   - `atmos ai help [topic]` with curated topics
   - Pre-built prompts for common scenarios
   - Domain-optimized question routing

**These should be preserved and enhanced, not replaced by general-purpose features.**

---

## Implementation Roadmap

### Phase 1: Foundation (1-2 weeks)
**Goal:** Core infrastructure for sessions and tools

**Tasks:**
- [ ] Design session storage schema (SQLite)
- [ ] Implement session manager with CRUD operations
- [ ] Create tool interface and base executor
- [ ] Build permission checking framework
- [ ] Add session commands (`atmos ai sessions`)

**Deliverables:**
- `pkg/ai/session/` - Session management package
- `pkg/ai/tools/` - Tool execution framework
- Database migrations for session storage

---

### Phase 2: Core Features (2-3 weeks)
**Goal:** Session persistence, project memory, basic tools

**Tasks:**
- [ ] Implement ATMOS.md project memory system
- [ ] Create Atmos-specific tools (describe, validate, list)
- [ ] Enhance chat UI with session support
- [ ] Add history navigation in TUI
- [ ] Implement tool execution in chat

**Deliverables:**
- `pkg/ai/memory/` - Project memory package
- `pkg/ai/tools/atmos_tools.go` - Atmos tool implementations
- Enhanced `pkg/ai/tui/` with sessions
- ATMOS.md template

---

### Phase 3: Advanced Features (3-4 weeks)
**Goal:** LSP, MCP, advanced UI

**Tasks:**
- [ ] Integrate yaml-language-server for stack validation
- [ ] Add terraform-ls for component analysis
- [ ] Implement MCP protocol support
- [ ] Build session selector UI
- [ ] Add model switching capability
- [ ] Implement syntax highlighting for responses

**Deliverables:**
- `pkg/ai/lsp/` - LSP integration
- `pkg/ai/mcp/` - MCP support
- Advanced TUI components
- Multi-model support

---

### Phase 4: Polish & Documentation (1-2 weeks)
**Goal:** Production-ready, documented, tested

**Tasks:**
- [ ] Write comprehensive documentation
- [ ] Create example ATMOS.md templates
- [ ] Build tutorial videos/guides
- [ ] Performance optimization
- [ ] Security audit of tool execution
- [ ] Integration testing
- [ ] User acceptance testing

**Deliverables:**
- Documentation in `website/docs/ai/`
- Example configurations
- Tutorial content
- Test coverage >80%
- Security review report

---

## Recommended Configuration Structure

### Full Example: atmos.yaml

```yaml
# atmos.yaml
settings:
  ai:
    # Core settings
    enabled: true
    provider: anthropic  # anthropic, openai, gemini, grok
    model: claude-3-5-sonnet-20241022
    api_key_env: ANTHROPIC_API_KEY
    max_tokens: 4096
    base_url: ""  # For custom endpoints

    # Session management
    sessions:
      enabled: true
      storage: sqlite  # sqlite or json
      path: .atmos/sessions
      max_sessions: 10
      auto_save: true
      retention_days: 30
      default_name_pattern: "session-{timestamp}"

    # Project memory
    memory:
      enabled: true
      file: ATMOS.md
      auto_update: true
      update_frequency: session_end  # session_end, real_time, manual
      sections:
        - project_context
        - common_commands
        - stack_patterns
        - frequent_issues
        - infrastructure_patterns
        - recent_learnings

    # Tool execution
    tools:
      enabled: true
      require_confirmation: true

      # Whitelist: Execute without prompting
      allowed_tools:
        - atmos_describe_component
        - atmos_describe_stacks
        - atmos_list_stacks
        - atmos_list_components
        - file_read

      # Restricted: Always require confirmation
      restricted_tools:
        - file_edit
        - file_write
        - atmos_terraform_plan
        - atmos_workflow_execute

      # Blocked: Never execute
      blocked_tools:
        - atmos_terraform_apply
        - atmos_terraform_destroy

      # YOLO mode - bypass all confirmations (DANGEROUS!)
      yolo_mode: false

      # Audit logging
      audit:
        enabled: true
        path: .atmos/ai-audit.log
        retention_days: 90

    # LSP integration
    lsp:
      enabled: true
      servers:
        yaml:
          command: yaml-language-server
          args: ["--stdio"]
          filetypes: ["yaml", "yml"]
          initialization_options:
            schemas:
              atmos_stack:
                uri: "https://atmos.tools/schemas/stack.json"
                fileMatch: ["stacks/**/*.yaml"]

        terraform:
          command: terraform-ls
          args: ["serve"]
          filetypes: ["tf", "tfvars"]
          root_patterns: [".terraform", "main.tf"]
          initialization_options:
            experimentalFeatures:
              validateOnSave: true

    # MCP (Model Context Protocol) servers
    mcp:
      enabled: true
      servers:
        # GitHub integration
        - name: github
          transport: http
          url: https://api.github.com
          env:
            GITHUB_TOKEN: $(echo $GITHUB_TOKEN)
          tools:
            - name: get_pr_diff
              description: Get diff for a pull request
            - name: list_prs
              description: List open pull requests

        # Spacelift integration
        - name: spacelift
          transport: http
          url: https://acme.app.spacelift.io
          env:
            SPACELIFT_API_TOKEN: $(echo $SPACELIFT_TOKEN)
          tools:
            - name: get_stack_status
              description: Get Spacelift stack status
            - name: list_runs
              description: List recent Spacelift runs

        # AWS resource lookup
        - name: aws
          transport: stdio
          command: aws-mcp-server
          args: ["--region", "us-east-1"]
          env:
            AWS_PROFILE: $(echo $AWS_PROFILE)
          tools:
            - name: describe_instances
              description: List EC2 instances
            - name: describe_rds
              description: List RDS databases

    # Context management
    context:
      send_by_default: false
      prompt_on_send: true
      max_files: 10
      max_lines_per_file: 500

      # File patterns to ignore (like .gitignore)
      ignore_patterns:
        - "*.tfstate"
        - "*.tfstate.backup"
        - ".terraform/"
        - ".atmos/cache/"
        - "**/*.log"
        - "**/node_modules/"

      # Additional ignore file (like .crushignore)
      ignore_file: .atmosignore

    # Performance tuning
    timeout_seconds: 60
    retry_attempts: 3
    retry_delay_seconds: 2

    # Multi-model support
    multi_model:
      enabled: true
      models:
        - name: claude-sonnet
          provider: anthropic
          model: claude-3-5-sonnet-20241022
          use_for: [general, complex, analysis]

        - name: claude-haiku
          provider: anthropic
          model: claude-3-5-haiku-20241022
          use_for: [quick, simple, lookup]

        - name: gpt-4
          provider: openai
          model: gpt-4-turbo
          use_for: [code_generation, refactoring]

      # Let AI choose best model for task
      auto_select: true

    # Git integration
    git:
      enabled: true
      attribution:
        co_authored_by: true
        generated_with: true
        author_name: Atmos AI
        author_email: ai@atmos.tools

      commit_messages:
        auto_generate: true
        template: |
          {type}({scope}): {summary}

          {details}

          Generated with Atmos AI
          Co-Authored-By: Atmos AI <ai@atmos.tools>

    # Provider auto-update (like Catwalk)
    providers:
      auto_update: false  # Disabled by default
      registry_url: https://ai-models.atmos.tools/providers.json
      update_interval: 24h
      update_on_startup: false
```

---

## Key Takeaways

### Strategic Principles

1. **Don't Copy Everything**
   - Crush is general-purpose software development
   - Atmos AI is domain-specific infrastructure management
   - Copy patterns, not features wholesale

2. **Focus on Session Persistence**
   - This is the #1 missing feature
   - Enables conversational infrastructure management
   - Critical for long-running tasks

3. **Add Tool Execution Carefully**
   - Start with read-only tools
   - Add write operations with strong permissions
   - Audit everything for security

4. **Implement Project Memory**
   - ATMOS.md for persistent context
   - Reduces repetitive questions
   - Learns project patterns over time

5. **Enhance the TUI Thoughtfully**
   - Better chat experience with history
   - Session management UI
   - Syntax highlighting for responses
   - Don't sacrifice simplicity

6. **Keep Domain Expertise**
   - This is Atmos AI's unique value proposition
   - Enhance, don't replace, Atmos-specific knowledge
   - Maintain focus on infrastructure workflows

### Success Metrics

**Before Improvements:**
- ❌ Every question starts fresh (no memory)
- ❌ AI can only answer, not execute
- ❌ No way to maintain conversation context
- ❌ Limited to single-shot Q&A

**After Improvements:**
- ✅ Maintains conversation across sessions
- ✅ Can validate, describe, and analyze automatically
- ✅ Remembers project patterns and conventions
- ✅ Full conversational infrastructure assistant

### Vision Statement

**Goal:** Create "Crush-level polish with Atmos-specific intelligence"

Not a direct clone of Crush, but an infrastructure-focused AI assistant that:
- Understands Atmos deeply (like it does now)
- Maintains context persistently (like Crush)
- Can execute operations safely (like Crush)
- Learns project patterns (like Crush)
- Provides exceptional UX (like Crush)

**Tagline:** *"The AI assistant that truly understands your infrastructure"*

---

## References

- **Crush Repository:** https://github.com/charmbracelet/crush
- **Crush Documentation:** https://charm.sh/crush (implied)
- **Model Context Protocol:** https://modelcontextprotocol.io
- **Atmos Documentation:** https://atmos.tools
- **Bubble Tea TUI:** https://github.com/charmbracelet/bubbletea
- **Ollama (Local LLM Runtime):** https://ollama.com
- **Meta LLAMA Models:** https://llama.meta.com

---

## Changelog

- **2025-10-23:** Initial draft based on Crush analysis
- **2025-10-23 (v2.0):** Major update reflecting implementation progress
  - ✅ Marked Session Management as COMPLETE
  - ✅ Marked Tool Execution System as COMPLETE
  - ✅ Marked Project Memory (ATMOS.md) as COMPLETE
  - ✅ Marked MCP Support as COMPLETE
  - ⚠️ Marked Enhanced TUI as PARTIAL (sessions complete, syntax highlighting pending)
  - ❌ Marked LSP Integration as PENDING
  - 📝 Added Ollama/LLAMA analysis and recommendation
  - 📝 Updated feature comparison matrix with 3-column view (Original → Current → Status)
  - 📝 Added implementation file references for completed features
  - 📝 Reorganized sections by completion status (COMPLETED / PARTIAL / PENDING)

---

*This document is a living PRD. Updated as of 2025-10-23 to reflect current implementation status.*
