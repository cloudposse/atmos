# Atmos MCP Servers — External MCP Server Management

**Status:** Complete — All 4 Phases Shipped
**Version:** 5.0
**Last Updated:** 2026-03-22

---

## Executive Summary

Atmos MCP Servers extends the existing `atmos mcp` command to support installing,
managing, and consuming **external MCP servers** — bringing the same MCP client capability
found in Claude Code, Gemini CLI, and AI IDEs directly into the Atmos CLI.

Instead of reimplementing cloud provider functionality (AWS APIs, GCP APIs, Azure APIs),
Atmos can install and orchestrate existing MCP servers from the ecosystem — like the
20+ AWS MCP servers from `awslabs/mcp` — and expose their tools alongside native Atmos
tools in a unified interface.

**Key Insight:** The Go MCP SDK (`github.com/modelcontextprotocol/go-sdk v1.4.1`) that
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

## Current State

### What Atmos Has Today

Atmos implements an MCP **server** (`atmos mcp start`) that exposes native Atmos tools to
external clients:

```
External AI Tool  ──MCP──>  Atmos MCP Server  ──>  Atmos Tools
(Claude Code)               (pkg/mcp/)              (describe_component, etc.)
```

### What This PRD Adds

An MCP **client** that connects to external MCP servers and makes their tools available
within Atmos:

```
Atmos CLI  ──MCP──>  AWS MCP Server     ──>  AWS APIs
           ──MCP──>  GCP MCP Server     ──>  GCP APIs
           ──MCP──>  Custom MCP Server  ──>  Custom APIs
```

### Combined Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                         Atmos CLI                                   │
├────────────────────────────────────────────────────────────────────┤
│  Unified Tool Registry                                              │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │ Native Atmos     │  │ AWS MCP Server   │  │ GCP MCP Server   │  │
│  │ Tools (15+)      │  │ Tools            │  │ Tools            │  │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘  │
├────────────────────────────────────────────────────────────────────┤
│  MCP Client Layer (pkg/mcp/client/)                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │
│  │ Process      │  │ Connection   │  │ Tool         │             │
│  │ Manager      │  │ Pool         │  │ Bridge       │             │
│  └──────────────┘  └──────────────┘  └──────────────┘             │
├────────────────────────────────────────────────────────────────────┤
│  MCP Server Layer (existing pkg/mcp/server.go)                      │
│  ┌──────────────┐  ┌──────────────┐                               │
│  │ stdio        │  │ HTTP/SSE     │                               │
│  │ transport    │  │ transport    │                               │
│  └──────────────┘  └──────────────┘                               │
├────────────────────────────────────────────────────────────────────┤
│  Toolchain Layer (existing pkg/dependencies/)                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │
│  │ Installer    │  │ Version      │  │ Aqua         │             │
│  │ Engine       │  │ Manager      │  │ Registry     │             │
│  └──────────────┘  └──────────────┘  └──────────────┘             │
└────────────────────────────────────────────────────────────────────┘
```

---

## AWS MCP Servers — Primary Use Case

The `awslabs/mcp` repository provides 20+ MCP servers covering the AWS ecosystem:

| Server | Package | Purpose |
|--------|---------|---------|
| AWS MCP Server | `awslabs.aws-mcp-server` | Comprehensive AWS API access (preview) |
| Amazon EKS | `awslabs.amazon-eks-mcp-server` | EKS cluster management |
| Amazon ECS | `awslabs.amazon-ecs-mcp-server` | ECS service management |
| AWS IaC | `awslabs.aws-iac-mcp-server` | CloudFormation/CDK operations |
| Amazon S3 | `awslabs.s3-mcp-server` | S3 bucket operations |
| DynamoDB | `awslabs.dynamodb-mcp-server` | DynamoDB table operations |
| AWS Serverless | `awslabs.aws-serverless-mcp-server` | SAM CLI operations |
| Lambda Tool | `awslabs.lambda-tool-mcp-server` | Lambda function management |
| AWS Support | `awslabs.aws-support-mcp-server` | AWS Support cases |
| AWS Documentation | `awslabs.aws-documentation-mcp-server` | AWS docs search |
| Amazon Bedrock | `awslabs.amazon-bedrock-mcp-server` | Bedrock model operations |
| AWS Knowledge | `awslabs.aws-knowledge-mcp-server` | AWS knowledge base search |
| Aurora DSQL | `awslabs.aurora-dsql-mcp-server` | Aurora DSQL queries |
| AWS Glue | `awslabs.glue-mcp-server` | Glue ETL operations |
| Finch | `awslabs.finch-mcp-server` | Container image builds |
| Nova Canvas | `awslabs.nova-canvas-mcp-server` | Image generation |

**Installation:** All use `uvx` (Python's `uv` package manager): `uvx awslabs.package@latest`

**Transport:** All use **stdio** (subprocess spawned, JSON-RPC over stdin/stdout).

**Configuration pattern:**
```json
{
  "command": "uvx",
  "args": ["awslabs.amazon-eks-mcp-server@latest"],
  "env": {
    "AWS_PROFILE": "production",
    "AWS_REGION": "us-east-1"
  }
}
```

---

## Configuration Design

### atmos.yaml Configuration

```yaml
mcp:
  # Existing Atmos MCP server configuration.
  enabled: true

  # External MCP server integrations.
  servers:
    # AWS EKS MCP server.
    aws-eks:
        description: "Amazon EKS cluster management"
        command: "uvx"
        args:
          - "awslabs.amazon-eks-mcp-server@latest"
        env:
          AWS_PROFILE: "{{ .vars.aws_profile }}"
          AWS_REGION: "{{ .vars.region }}"
        # Optional: only start when these tools are needed.
        auto_start: true
        # Optional: connection timeout.
        timeout: 30s

      # AWS IaC MCP server for CloudFormation.
      aws-iac:
        description: "AWS Infrastructure as Code (CloudFormation/CDK)"
        command: "uvx"
        args:
          - "awslabs.aws-iac-mcp-server@latest"
        env:
          AWS_PROFILE: "{{ .vars.aws_profile }}"
          AWS_REGION: "{{ .vars.region }}"

      # AWS S3 MCP server.
      aws-s3:
        description: "Amazon S3 bucket operations"
        command: "uvx"
        args:
          - "awslabs.s3-mcp-server@latest"
        env:
          AWS_PROFILE: "{{ .vars.aws_profile }}"

      # Custom MCP server (any stdio-based server).
      custom-db:
        description: "Internal database query server"
        command: "/usr/local/bin/db-mcp-server"
        args:
          - "--config"
          - "/etc/db-mcp.yaml"
        env:
          DB_HOST: "localhost"
```

### Toolchain Integration for Prerequisites

External MCP servers often require runtime dependencies (Python's `uvx`, Node.js `npx`,
etc.). The existing Atmos toolchain can manage these prerequisites:

```yaml
# atmos.yaml
toolchain:
  tools:
    uv:
      # Python package manager (provides uvx for AWS MCP servers).
      version: "0.7.x"
      aliases:
        - uvx
```

This ensures `uvx` is available before any AWS MCP server is started, with version pinning
and automatic installation via the existing Aqua registry integration.

---

## CLI Commands

### Subcommand Structure

Extend the existing `atmos mcp` command:

```
atmos mcp start          # (existing) Start Atmos as an MCP server
atmos mcp list           # List configured external MCP servers and their status
atmos mcp status         # Show status of all running MCP server processes
atmos mcp restart <name> # Restart an MCP server process
atmos mcp tools [name]   # List tools exposed by an MCP server
atmos mcp test <name>    # Test connectivity to an MCP server
```

### Examples

```bash
# List all configured MCP servers.
$ atmos mcp list
NAME       STATUS    TOOLS  DESCRIPTION
aws-eks    running   12     Amazon EKS cluster management
aws-iac    stopped    8     AWS Infrastructure as Code
aws-s3     running    6     Amazon S3 bucket operations
custom-db  error      0     Internal database query server

# List tools from a specific server.
$ atmos mcp tools aws-eks
TOOL                          DESCRIPTION
eks_list_clusters              List EKS clusters in the account
eks_describe_cluster           Get details of an EKS cluster
eks_list_nodegroups            List node groups for a cluster
eks_create_cluster             Create a new EKS cluster
...

# Test connectivity.
$ atmos mcp test aws-eks
✓ Server started successfully
✓ Initialization handshake complete
✓ 12 tools available
✓ Server responds to ping
```

---

## Implementation Architecture

### Package Structure

```
pkg/mcp/
├── server.go              # (existing) Atmos MCP server
├── adapter.go             # (existing) Atmos tool → MCP bridge
├── client/
│   ├── manager.go         # MCP client process manager (lifecycle)
│   ├── session.go         # MCP client session wrapper
│   ├── pool.go            # Connection pool for multiple servers
│   ├── bridge.go          # External MCP tool → Atmos tool bridge
│   └── config.go          # Integration configuration types
└── client_test.go         # Tests
```

### Core Types

```go
// IntegrationConfig represents an external MCP server configuration
// from atmos.yaml mcp.servers.
type IntegrationConfig struct {
    Description string            `yaml:"description" mapstructure:"description"`
    Command     string            `yaml:"command" mapstructure:"command"`
    Args        []string          `yaml:"args" mapstructure:"args"`
    Env         map[string]string `yaml:"env" mapstructure:"env"`
    AutoStart   bool              `yaml:"auto_start" mapstructure:"auto_start"`
    Timeout     time.Duration     `yaml:"timeout" mapstructure:"timeout"`
}

// Manager manages the lifecycle of external MCP server processes.
type Manager struct {
    sessions map[string]*ClientSession
    configs  map[string]IntegrationConfig
    mu       sync.RWMutex
}

// ClientSession wraps an MCP client connection to an external server.
type ClientSession struct {
    name    string
    client  *mcp.ClientSession
    process *os.Process
    tools   []mcp.Tool
    status  SessionStatus
}
```

### Process Lifecycle

```
1. Configuration Load
   atmos.yaml → IntegrationConfig → Manager.Register()

2. Server Start (on-demand or auto_start)
   Manager.Start(name) →
     a. Resolve command via toolchain (ensure uvx/npx available)
     b. Expand env var templates ({{ .vars.region }})
     c. Spawn subprocess via CommandTransport
     d. MCP initialize handshake
     e. List tools → cache tool definitions
     f. Register external tools in unified registry

3. Tool Execution
   AI chat or --ai flag →
     Unified tool registry →
       Native tool? → Execute directly
       External tool? → Manager.CallTool(serverName, toolName, args)
                          → ClientSession.CallTool()
                            → JSON-RPC over stdio
                              → External MCP server executes
                                → Result returned

4. Server Shutdown
   Manager.Stop(name) or Manager.StopAll() →
     a. Close stdin (signal graceful shutdown)
     b. Wait with timeout
     c. SIGTERM if needed
     d. SIGKILL as last resort
```

### Tool Bridge

External MCP tools are bridged into the Atmos tool registry so they appear alongside
native tools:

```go
// BridgedTool wraps an external MCP tool as an Atmos AI tool.
type BridgedTool struct {
    serverName string
    mcpTool    mcp.Tool
    manager    *Manager
}

func (t *BridgedTool) Name() string {
    // Namespace: "aws-eks.list_clusters" to avoid conflicts.
    return t.serverName + "." + t.mcpTool.Name
}

func (t *BridgedTool) Execute(ctx context.Context, params map[string]any) (string, error) {
    result, err := t.manager.CallTool(ctx, t.serverName, t.mcpTool.Name, params)
    if err != nil {
        return "", err
    }
    return extractTextContent(result), nil
}
```

---

## Integration with Existing Atmos Systems

### AI Chat Integration

External MCP tools are available in `atmos ai chat`:

```
$ atmos ai chat
You: List my EKS clusters in production
AI: I'll check your EKS clusters using the AWS EKS integration.

[Calling aws-eks.eks_list_clusters with region=us-east-1]

You have 3 EKS clusters in us-east-1:
1. prod-platform (v1.29, ACTIVE)
2. prod-data (v1.28, ACTIVE)
3. prod-staging (v1.29, UPDATING)
```

### --ai Flag Integration

External tools are available when using `--ai`:

```bash
$ atmos terraform plan vpc -s prod --ai --skill atmos-terraform
# AI analysis can reference AWS MCP tools for context:
# "I see this VPC component. Let me check the current EKS clusters
#  that depend on this VPC using the aws-eks integration..."
```

### Atmos MCP Server (Composite)

When Atmos runs as an MCP server itself, it can expose both native AND external tools to
the upstream client:

```
Claude Code  ──MCP──>  Atmos MCP Server  ──>  Native Atmos tools
                                          ──MCP──>  AWS EKS tools
                                          ──MCP──>  AWS S3 tools
```

This makes Atmos a **composite MCP server** — a single connection point that aggregates
tools from multiple sources.

### Auth Integration — Seamless AWS Credential Flow

This is the key differentiator. AWS MCP servers need credentials. Without Atmos Auth, users
must manually configure `AWS_PROFILE`, manage SSO sessions, and handle role assumption.
With Atmos Auth, credentials flow automatically from the identity chain to the MCP server
subprocess — zero manual credential management.

#### How It Works

Atmos Auth already manages AWS credentials for Terraform/Helmfile subprocesses by:

1. Authenticating through a provider-identity chain (SSO → role assumption → target role).
2. Writing isolated credential files to `~/.aws/atmos/<realm>/<provider>/`.
3. Setting environment variables on the subprocess:
   - `AWS_SHARED_CREDENTIALS_FILE` → Atmos-managed credentials file
   - `AWS_CONFIG_FILE` → Atmos-managed config file
   - `AWS_PROFILE` → Identity name (profile in the credential files)
   - `AWS_REGION` / `AWS_DEFAULT_REGION` → Region from identity or component
   - `AWS_SDK_LOAD_CONFIG=1` → Force SDK to read shared config
   - `AWS_EC2_METADATA_DISABLED=true` → Prevent IMDS fallback
4. Clearing conflicting variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
   `AWS_SESSION_TOKEN`, etc.) to prevent profile-based auth from being overridden.

The **same mechanism** works for MCP servers — they are spawned as subprocesses with these
environment variables, and their boto3/AWS SDK picks up the credentials automatically.

#### Configuration

```yaml
mcp:
  servers:
      # Cost analysis across all accounts.
      aws-cost-explorer:
        description: "AWS Cost Explorer — cost analysis and forecasting"
        command: "uvx"
        args: ["awslabs.cost-explorer-mcp-server@latest"]
        env:
          FASTMCP_LOG_LEVEL: "ERROR"
        # Use Atmos auth identity — credentials injected automatically.
        auth_identity: "billing-readonly"

      # Security posture in production.
      aws-security:
        description: "AWS Well-Architected Security — security findings and posture"
        command: "uvx"
        args:
          - "--from"
          - "awslabs.well-architected-security-mcp-server"
          - "well-architected-security-mcp-server"
        env:
          FASTMCP_LOG_LEVEL: "ERROR"
        auth_identity: "security-audit"
```

#### Credential Flow Diagram

```
User runs: atmos ai chat

1. Atmos Auth resolves identity "billing-readonly"
   ┌──────────────────────────────────────────────────┐
   │ Provider: aws-sso (IAM Identity Center)          │
   │   → Identity: billing-account                    │
   │     → Identity: billing-readonly (assume role)   │
   └──────────────────────────────────────────────────┘

2. Credentials written to ~/.aws/atmos/<realm>/billing-readonly/
   ├── credentials  (access key, secret key, session token)
   └── config       (region, output format)

3. MCP server spawned with env vars:
   ┌──────────────────────────────────────────────────┐
   │ AWS_SHARED_CREDENTIALS_FILE=~/.aws/atmos/.../creds │
   │ AWS_CONFIG_FILE=~/.aws/atmos/.../config          │
   │ AWS_PROFILE=billing-readonly                     │
   │ AWS_REGION=us-east-1                             │
   │ AWS_SDK_LOAD_CONFIG=1                            │
   │ AWS_EC2_METADATA_DISABLED=true                   │
   │ FASTMCP_LOG_LEVEL=ERROR                          │
   └──────────────────────────────────────────────────┘

4. MCP server's boto3 picks up credentials automatically
   → No manual AWS_PROFILE setup needed
   → No SSO login prompts during MCP server execution
   → Credentials are scoped to the specific identity
   → Credentials are isolated per-realm (no cross-project leaking)
```

#### Credential Caching

Atmos Auth caches credentials in the system keyring with 15-minute validity buffer.
When an MCP server is started, the credentials are already cached from the initial
authentication. If the MCP server runs for longer than the credential TTL, Atmos can
refresh credentials and restart the server process transparently.

---

## Concrete Examples — Calling MCP Servers from Atmos CLI

### Example 1: Cost Analysis with AWS Cost Explorer

**Configuration:**
```yaml
mcp:
  servers:
      aws-cost-explorer:
        command: "uvx"
        args: ["awslabs.cost-explorer-mcp-server@latest"]
        env:
          FASTMCP_LOG_LEVEL: "ERROR"
        auth_identity: "billing-readonly"
```

**Tools exposed (7):**

| Tool | Description |
|------|-------------|
| `get_cost_and_usage` | Retrieve cost/usage data with filtering and grouping |
| `get_cost_and_usage_comparisons` | Compare costs between two time periods |
| `get_cost_comparison_drivers` | Analyze top 10 cost change drivers |
| `get_cost_forecast` | Generate cost forecasts from historical data |
| `get_dimension_values` | List available dimensions (SERVICE, REGION, etc.) |
| `get_tag_values` | List available tag values for filtering |
| `get_today_date` | Get current date for contextual queries |

**Usage in `atmos ai chat`:**
```
You: What are our top 5 most expensive AWS services this month?

AI: I'll check your AWS costs using the Cost Explorer integration.

[Calling aws-cost-explorer.get_today_date]
[Calling aws-cost-explorer.get_cost_and_usage with
  start="2026-03-01", end="2026-03-21", granularity="MONTHLY",
  group_by=[{"Type":"DIMENSION","Key":"SERVICE"}]]

Your top 5 AWS services by cost this month:

1. Amazon EC2          $12,450.23  (↑ 8% from last month)
2. Amazon RDS          $8,230.15   (↓ 3%)
3. Amazon S3           $3,120.89   (↑ 12%)
4. AWS Lambda          $1,890.45   (↑ 25%)
5. Amazon CloudFront   $1,200.00   (flat)

Total spend: $31,450.72 (22 days into the month)
Projected monthly total: ~$43,100

Would you like me to analyze what's driving the Lambda cost increase?
```

**Usage with `--ai` flag:**
```bash
# After deploying a VPC change, analyze cost impact.
$ atmos terraform apply vpc -s prod --ai --skill atmos-terraform

AI: The VPC change added 3 NAT gateways. Let me check the cost impact.

[Calling aws-cost-explorer.get_cost_forecast with service="Amazon VPC"]

Projected NAT gateway cost: ~$450/month additional.
Consider using VPC endpoints for S3/DynamoDB to reduce NAT traffic costs.
```

**Note:** Each Cost Explorer API call costs $0.01. Atmos should display this in verbose
mode to avoid surprise charges.

### Example 2: Security Posture with Well-Architected Security

**Configuration:**
```yaml
mcp:
  servers:
      aws-security:
        command: "uvx"
        args:
          - "--from"
          - "awslabs.well-architected-security-mcp-server"
          - "well-architected-security-mcp-server"
        env:
          FASTMCP_LOG_LEVEL: "ERROR"
        auth_identity: "security-audit"
```

**Tools exposed (6):**

| Tool | Description |
|------|-------------|
| `CheckSecurityServices` | Verify security services are enabled (GuardDuty, Inspector, etc.) |
| `GetSecurityFindings` | Retrieve findings with severity filtering |
| `CheckStorageEncryption` | Check encryption on S3, EBS, RDS, DynamoDB, EFS |
| `CheckNetworkSecurity` | Check TLS/HTTPS on ELB, VPC, API Gateway, CloudFront |
| `ListServicesInRegion` | List active AWS services in a region |
| `GetStoredSecurityContext` | Retrieve historical security data |

**Usage in `atmos ai chat`:**
```
You: Check security posture for our production account in us-east-1

AI: I'll run a comprehensive security check using the Well-Architected
Security integration.

[Calling aws-security.CheckSecurityServices with
  region="us-east-1",
  services=["guardduty","inspector","securityhub","accessanalyzer"]]

[Calling aws-security.CheckStorageEncryption with
  region="us-east-1",
  services=["s3","ebs","rds"],
  include_unencrypted_only=true]

[Calling aws-security.CheckNetworkSecurity with
  region="us-east-1",
  services=["elb","apigateway"],
  include_non_compliant_only=true]

## Security Posture Report — us-east-1

### Security Services
✅ GuardDuty: Enabled
✅ Security Hub: Enabled
✅ IAM Access Analyzer: Enabled
⚠️ Inspector: Not enabled — recommend enabling for vulnerability scanning

### Storage Encryption
✅ All S3 buckets encrypted (47/47)
✅ All EBS volumes encrypted (128/128)
⚠️ 2 RDS instances without encryption:
   - db-legacy-reports (db.m5.large)
   - db-temp-analytics (db.t3.medium)

### Network Security
✅ All ALBs use HTTPS listeners
⚠️ 1 API Gateway without WAF:
   - api-internal-v2 (REST API)

Would you like me to check the security findings from GuardDuty?
```

### Example 3: Combined Atmos + AWS MCP Workflow

The real power is combining native Atmos tools with external MCP tools:

```
You: We're planning to decommission the staging VPC. What's the impact?

AI: Let me analyze this from multiple angles.

[Calling describe_component with component="vpc", stack="staging"]
# Native Atmos tool — understands stack configuration

[Calling aws-security.ListServicesInRegion with region="us-west-2"]
# External MCP tool — finds active AWS services

[Calling aws-cost-explorer.get_cost_and_usage with
  filter={"Tags":{"Key":"Stage","Values":["staging"]}},
  group_by=[{"Type":"DIMENSION","Key":"SERVICE"}]]
# External MCP tool — calculates cost savings

## Decommission Impact Analysis

### Components Using staging VPC
- vpc (Terraform component, 3 subnets, 2 NAT gateways)
- eks/cluster (depends on vpc via remote-state)
- rds/primary (depends on vpc subnet group)
- elasticache/redis (depends on vpc security group)

### Active AWS Services in us-west-2 staging
- 12 EC2 instances, 3 RDS instances, 1 EKS cluster
- 2 ElastiCache clusters, 5 Lambda functions

### Cost Savings
Estimated monthly savings: $8,200
- EC2: $4,500, NAT gateways: $900, RDS: $1,800, EKS: $1,000

### Recommended Decommission Order
1. Lambda functions (no dependencies)
2. ElastiCache (depends on VPC only)
3. EKS cluster (depends on VPC)
4. RDS instances (depends on VPC, needs snapshot)
5. VPC (last — all dependencies removed)

Shall I generate an atmos workflow for this decommission sequence?
```

---

## Phased Implementation

### Phase 1: Core Client Infrastructure (MVP) ✅ SHIPPED

**Implemented files:**

| File | Lines | Purpose |
|---|---|---|
| `pkg/schema/mcp.go` | 20 | `MCPServerConfig` type + `Integrations` map on `MCPSettings` |
| `pkg/mcp/client/config.go` | 50 | Config parsing with validation and timeout resolution |
| `pkg/mcp/client/session.go` | 180 | Session lifecycle: Start (subprocess + MCP handshake + tool list), Stop, CallTool, Ping |
| `pkg/mcp/client/manager.go` | 120 | Multi-session manager: NewManager, Start/Stop/StopAll, Get/List, Test |
| `pkg/mcp/client/bridge.go` | 90 | BridgedTool wrapping external MCP tools with namespaced names |
| `cmd/mcp/list.go` | 50 | `atmos mcp list` — tabular output of configured integrations |
| `cmd/mcp/tools.go` | 70 | `atmos mcp tools <name>` — connect, list tools, disconnect |
| `cmd/mcp/test_cmd.go` | 70 | `atmos mcp test <name>` — start, handshake, list tools, ping |
| `errors/errors.go` | +5 | `ErrMCPServerNotFound/NotRunning/StartFailed/CommandEmpty/InvalidTimeout` |

**Tests:** 34 unit tests across 4 test files at 73% coverage.

**Configuration path:** `mcp.servers` in `atmos.yaml` (sibling to existing `mcp.enabled`).

**Key design decisions:**
- Uses Go MCP SDK v1.4.1 `Client` + `CommandTransport` + `ClientSession` — no new dependencies.
- Subprocess environment inherits `os.Environ()` + configured `env` map — external tools get PATH, HOME, etc.
- Session status tracking: `stopped` → `starting` → `running` / `error`.
- Tool bridge uses `server.tool_name` namespacing to avoid conflicts between servers.
- Manager.Test performs full lifecycle: start → handshake → list tools → ping → report.

### Phase 2: Tool Bridge + AI Integration ✅ SHIPPED

**Implemented files:**

| File | Lines | Purpose |
|---|---|---|
| `pkg/mcp/client/register.go` | 60 | `RegisterMCPTools` — starts integrations, bridges tools into AI registry |
| `pkg/mcp/client/register_test.go` | 45 | Tests: no integrations, invalid config, failed start continues |
| `pkg/mcp/client/bridge.go` | 170 | `BridgedTool` implements `tools.Tool` interface with JSON Schema parameter extraction |
| `pkg/mcp/client/bridge_test.go` | 170 | Tests: interface compliance, parameters, schema types, content extraction |
| `cmd/ai/init.go` | modified | `aiToolsResult` struct, calls `RegisterMCPTools` after native tools |
| `cmd/ai/chat.go` | modified | `defer MCPMgr.StopAll()` for subprocess cleanup |
| `cmd/ai/exec.go` | modified | `defer MCPMgr.StopAll()` for subprocess cleanup |

**What shipped:**
- ~~Tool bridge: external MCP tools → Atmos AI tool registry~~ ✅
- ~~Namespaced tool names (`server.tool_name`)~~ ✅
- ~~`atmos ai chat` integration~~ ✅ — MCP tools registered in AI executor
- ~~`atmos ai exec` integration~~ ✅ — MCP tools available in non-interactive mode
- ~~Graceful shutdown on CLI exit~~ ✅ — `defer MCPMgr.StopAll()` in chat and exec

**Key design decisions:**
- `BridgedTool` implements `tools.Tool` interface (compile-time `var _ tools.Tool = (*BridgedTool)(nil)` check).
- `Execute()` returns `*tools.Result` with `Success`, `Output`, and `Error` fields.
- `Parameters()` extracts from MCP `InputSchema` JSON Schema — maps `string`, `integer`,
  `number`, `boolean`, `array`, `object` to Atmos `ParamType`.
- `RegisterMCPTools` is best-effort: failed servers log warnings but don't block other tools.
- `aiToolsResult` struct avoids 4-return-value lint error (`function-result-limit: max 3`).
- `--ai` flag analysis (output capture) does not use tools — MCP tools are available only
  in `atmos ai chat` and `atmos ai exec`.

**Remaining for future:**
- Lazy initialization (auto-start on first tool call instead of upfront)
- `--ai` flag tool integration (requires executor in the analysis path)

### Phase 3: Management Commands ✅ SHIPPED

**Implemented files:**

| File | Lines | Purpose |
|---|---|---|
| `cmd/mcp/status.go` | 80 | `atmos mcp status` — start all, display table (name, status, tools, description) |
| `cmd/mcp/restart.go` | 60 | `atmos mcp restart <name>` — stop and restart integration |
| `cmd/mcp/add_test.go` | 120 | Tests: add new section, add to existing, remove, findAtmosYAML |
| `errors/errors.go` | +1 | `ErrMCPServerAlreadyExists` sentinel |

**What shipped:**
- ~~`atmos mcp restart`~~ ✅ — stop + start cycle
- ~~`atmos mcp status`~~ ✅ — health table with running/degraded/error status

**Full command tree:**
```
atmos mcp start          # Start Atmos as MCP server
atmos mcp list           # List configured integrations
atmos mcp tools <name>   # List tools from a server
atmos mcp test <name>    # Test connectivity
atmos mcp status         # Show all integration statuses
atmos mcp restart <name> # Restart a server
```

### Phase 4: Advanced Features ✅ SHIPPED

**Shipped:**
- ~~Atmos Auth integration for credential injection~~ ✅ — `auth_identity` field on
  `MCPServerConfig`, `AuthEnvProvider` interface, `WithAuthManager` start option,
  `PrepareShellEnvironment` integration. 8 tests.
- ~~Toolchain integration for prerequisite management~~ ✅ — `WithToolchain` start option,
  `ToolchainResolver` interface, command binary resolution via `ForComponent`, toolchain
  PATH prepended to subprocess environment. 3 tests.
- ~~YAML functions in env vars~~ ✅ — `!env`, `!exec`, `!repo-root`, `!cwd`, `!random`
  work out of the box in `atmos.yaml` via `preprocessAtmosYamlFunc`. No code needed.

### Future Considerations

- Stack-level MCP server overrides (per-stack `settings.mcp.servers` config)
- Composite MCP server (expose external tools via Atmos MCP server)
- Connection pooling and health checks with auto-restart on failure
- `tools/list_changed` notification handling for dynamic tool updates
- MCP server registry (curated list of known servers with default configs)

---

## Security Considerations

1. **Process isolation** — Each MCP server runs as a separate subprocess with its own
   environment. No shared memory or file handles.
2. **Environment scoping** — Environment variables are explicitly configured per server.
   No implicit credential leaking between servers.
3. **Permission model** — External MCP tools inherit the Atmos AI permission system
   (Allow/Prompt/YOLO). Users can control which external tools are auto-approved.
4. **Transport security** — stdio transport is local-only (no network exposure). HTTP
   transport for remote servers uses TLS.
5. **Supply chain** — MCP servers are installed via package managers (`uvx`, `npx`) with
   version pinning support. Atmos toolchain can enforce specific versions.

---

## Comparison with Claude Code MCP

| Feature | Claude Code | Atmos (Proposed) |
|---------|-------------|------------------|
| List servers | `claude mcp list` | `atmos mcp list` |
| Config location | `.mcp.json` / `~/.claude.json` | `atmos.yaml` |
| Config scopes | local / project / user | global (atmos.yaml) |
| Transport | stdio / HTTP / SSE | stdio |
| Tool namespacing | Flat (server-level) | `server.tool_name` |
| Auth integration | None | Atmos Auth identities |
| Prerequisite mgmt | Manual | Atmos Toolchain (automatic) |
| Stack context | N/A | Env vars from stack vars |
| Version pinning | Manual | Toolchain + Aqua registry |

---

## Success Metrics

1. **Time to first external tool call** — User can configure and call an AWS MCP tool
   in under 5 minutes.
2. **Tool parity** — All 20+ AWS MCP servers work out of the box with `uvx`.
3. **Zero reimplementation** — No AWS API code in Atmos — all delegated to MCP servers.
4. **Composability** — Native and external tools work together in the same AI conversation.
5. **Reliability** — MCP server processes are managed with proper lifecycle (start, health
   check, restart, graceful shutdown).

---

## References

- [AWS MCP Servers](https://github.com/awslabs/mcp) — 20+ AWS MCP servers
- [MCP Specification](https://spec.modelcontextprotocol.io/) — Protocol version 2025-03-26
- [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) — v1.4.1 (already in Atmos)
- [Claude Code MCP](https://docs.anthropic.com/en/docs/claude-code/mcp) — Reference implementation
- [Atmos Toolchain PRD](./toolchain-implementation.md) — Prerequisite management
- [Atmos AI PRD](./atmos-ai.md) — AI architecture
- [Atmos MCP Server PRD](./atmos-ai.md#mcp-integration) — Existing server implementation
