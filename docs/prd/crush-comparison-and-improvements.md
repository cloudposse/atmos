# Crush vs Atmos AI: Feature Comparison & Improvement Recommendations

**Document Version:** 1.0
**Date:** 2025-10-23
**Status:** Draft
**Author:** AI Analysis based on https://github.com/charmbracelet/crush

---

## Executive Summary

Crush is a **full-featured AI coding agent** focused on general-purpose software development, while Atmos AI is a **domain-specific AI assistant** focused on infrastructure-as-code management. Crush has significantly more features but serves a different purpose.

**Key Finding:** Atmos AI should adopt Crush's productivity patterns (sessions, tools, memory) while maintaining its domain-specific intelligence advantage.

---

## Feature Comparison Matrix

| Feature Category | Crush | Atmos AI | Priority for Atmos |
|-----------------|-------|----------|-------------------|
| **Session Management** | ✅ SQLite-backed persistent sessions | ❌ Stateless (new context each time) | 🔴 HIGH |
| **LSP Integration** | ✅ Multi-language LSP support | ❌ No LSP | 🟡 MEDIUM |
| **MCP Support** | ✅ stdio/HTTP/SSE transport | ❌ No MCP | 🟡 MEDIUM |
| **Tool Execution** | ✅ Bash, file edit, search | ❌ Read-only (no execution) | 🔴 HIGH |
| **Permission System** | ✅ Granular + YOLO mode | ⚠️ Basic (prompt for context) | 🟡 MEDIUM |
| **Model Switching** | ✅ Mid-session (preserves context) | ❌ Restart required | 🟢 LOW |
| **File Ignore** | ✅ .gitignore + .crushignore | ⚠️ Glob patterns only | 🟢 LOW |
| **Context Persistence** | ✅ CRUSH.md project memory | ❌ No persistent memory | 🔴 HIGH |
| **Provider Management** | ✅ Auto-update from Catwalk | ⚠️ Manual config | 🟢 LOW |
| **Git Attribution** | ✅ Co-authored-by + generated-with | ❌ No git integration | 🟢 LOW |
| **Interactive Chat** | ✅ Full TUI with history | ✅ Basic TUI (Bubble Tea) | 🟡 MEDIUM |
| **Domain Context** | ❌ No domain-specific knowledge | ✅ Atmos-specific knowledge | ✅ Core |
| **Stack Context** | ❌ N/A | ✅ Stack file analysis | ✅ Core |
| **AI Providers** | ✅ 10+ providers | ✅ 4 providers | 🟢 LOW |

---

## Missing Features in Atmos AI (by Priority)

### 🔴 HIGH PRIORITY - Critical for Productivity

#### 1. Session Management & Context Persistence

**What Crush Has:**
- SQLite-backed session storage
- Maintains conversation history across restarts
- Multiple concurrent sessions per project
- Automatic state persistence
- Session isolation with independent contexts

**Why It Matters for Atmos:**
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

#### 2. Tool Execution System

**What Crush Has:**
- Bash command execution with security controls
- File read/write/edit operations
- Search and grep capabilities
- Granular permission system with allowlists
- Tool result streaming to AI

**Why It Matters for Atmos:**
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

#### 3. Project Memory (ATMOS.md)

**What Crush Has:**
- `CRUSH.md` file for persistent project knowledge
- Automatically updated by AI during sessions
- Stores frequently used commands
- Remembers coding style preferences
- Maintains codebase structure notes
- Avoids re-discovering same information

**Why It Matters for Atmos:**
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

### 🟡 MEDIUM PRIORITY - Quality of Life Improvements

#### 4. LSP Integration

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

#### 5. Enhanced Interactive Chat UI

**Current State:**
- Basic Bubble Tea TUI
- No history navigation
- No multi-line input
- No syntax highlighting
- No session management UI
- Basic message display

**Improvements Needed:**

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
- `pkg/ai/tui/chat.go` - Enhanced chat model
- `pkg/ai/tui/input.go` - Multi-line input handler
- `pkg/ai/tui/renderer.go` - Markdown/syntax highlighting
- `pkg/ai/tui/history.go` - History navigation
- `pkg/ai/tui/sessions.go` - Session selector UI

---

#### 6. MCP (Model Context Protocol) Support

**What It Is:**
- Anthropic's standardized protocol for AI context providers
- Enables external tools and data sources
- Three transport types: stdio, HTTP, SSE
- Dynamic tool registration

**What It Enables for Atmos:**

**A. External Integrations**
```yaml
settings:
  ai:
    mcp:
      enabled: true
      servers:
        # GitHub integration
        - name: github
          transport: http
          url: https://api.github.com
          env:
            GITHUB_TOKEN: $(echo $GITHUB_TOKEN)

        # Spacelift integration
        - name: spacelift
          transport: http
          url: https://acme.app.spacelift.io
          env:
            SPACELIFT_API_TOKEN: $(echo $SPACELIFT_TOKEN)

        # AWS resource lookup
        - name: aws-resources
          transport: stdio
          command: aws-mcp-server
          args: ["--region", "us-east-1"]
```

**B. Use Cases**

**GitHub PR Context:**
```bash
User: "What components are affected by PR #123?"
AI: *Uses GitHub MCP to fetch PR diff*
AI: *Runs atmos describe affected with PR changes*
AI: "PR #123 affects:
- vpc component in prod-use1
- security-groups component in prod-use1
- Changes: CIDR block expansion"
```

**Spacelift Stack Status:**
```bash
User: "What's the status of the VPC deployment?"
AI: *Queries Spacelift MCP server*
AI: "VPC stack (prod-use1):
- Last run: 2 hours ago
- Status: Applied successfully
- Resources: 45 created, 0 changed, 0 destroyed"
```

**AWS Resource Lookup:**
```bash
User: "What RDS instances exist in prod?"
AI: *Queries AWS MCP server*
AI: "Found 3 RDS instances in us-east-1:
1. acme-prod-postgres (db.r6g.xlarge) - Running
2. acme-prod-analytics (db.r6g.2xlarge) - Running
3. acme-prod-reports (db.r6g.large) - Stopped"
```

**Implementation:**
- `pkg/ai/mcp/client.go` - MCP client implementation
- `pkg/ai/mcp/registry.go` - MCP server registry
- `pkg/ai/mcp/tools.go` - MCP tool adapter

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

#### 8. Auto-Updating Provider Database

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

---

## Changelog

- **2025-10-23:** Initial draft based on Crush analysis
- **Future:** Update as features are implemented

---

*This document is a living PRD. Update it as implementation progresses and requirements evolve.*
