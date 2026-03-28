# Atmos MCP Integrations Example

This example demonstrates how to connect Atmos to external MCP (Model Context Protocol) servers from the AWS ecosystem.
Instead of reimplementing cloud provider functionality, Atmos installs and orchestrates existing MCP servers — their
tools become available across the Atmos AI surface:

- **`atmos ai chat`** — interactive conversations with MCP tools
- **`atmos ai ask`** — one-shot questions using MCP tools
- **`atmos ai exec`** — execute AI-driven tasks with MCP tools
- **`atmos terraform plan --ai`** — AI analysis of command output with MCP context
- **`atmos mcp tools <name>`** — directly list tools from any MCP server
- **`atmos mcp test <name>`** — verify server connectivity and available tools

## What's Included

The following AWS MCP servers are pre-configured in `atmos.yaml`:

### Cost Analysis & FinOps

| Server               | What It Does                                      | Credentials               |
|----------------------|---------------------------------------------------|---------------------------|
| **aws-billing**      | Billing summaries, payment history, cost tags     | Yes — `ce:*`, `billing:*` |
| **aws-cost-explorer**| Spend breakdowns by service/account/tag, forecasts| Yes — `ce:*`              |
| **aws-pricing**      | On-demand/reserved pricing, cost comparisons      | Yes — `pricing:*` (free)  |

### Security & Compliance

| Server              | What It Does                                       | Credentials                      |
|---------------------|----------------------------------------------------|---------------------------------|
| **aws-security**    | Well-Architected Security Pillar assessment        | Yes — security services read    |
| **aws-iam**         | IAM role/policy analysis, permission boundaries    | Yes — `iam:Get*`, `iam:List*`  |
| **aws-cloudtrail**  | CloudTrail event history, API call auditing        | Yes — `cloudtrail:LookupEvents`|

### General

| Server            | What It Does                                 | Credentials |
|-------------------|----------------------------------------------|-------------|
| **aws-api**       | Direct AWS CLI access with security controls | Yes         |
| **aws-docs**      | Search and fetch AWS documentation           | No          |
| **aws-knowledge** | Managed AWS knowledge base (remote service)  | No          |

All configured MCP servers are available across all Atmos AI commands — `atmos ai ask`,
`atmos ai chat`, `atmos ai exec`, and `--ai` flag.

## Prerequisites

1. **Python 3.10+** — the `uv` package manager is auto-installed by the Atmos toolchain
   (configured in `atmos.yaml`), so you don't need to install it manually.

2. **Atmos Auth** — all servers that need AWS credentials use `auth_identity: "readonly"`.
   Update the auth section in `atmos.yaml` with your SSO start URL, permission set, and
   account ID, then run:
   ```bash
   atmos auth login
   ```
   That's it — Atmos authenticates once and injects credentials into every MCP server
   automatically. No `aws configure`, no environment variables, no credential files.

## Quick Start

```bash
# Navigate to this example
cd examples/mcp

# List all configured MCP servers
atmos mcp list

# Test connectivity to a server (starts it, checks tools, pings)
atmos mcp test aws-docs

# List tools from a specific server
atmos mcp tools aws-docs

# Use MCP tools in AI chat (requires AI provider configured)
atmos ai chat
# Then ask: "Search AWS docs for EKS best practices"
```

## CLI Commands

### Managing Servers

```bash
# List all configured servers
atmos mcp list

# Show live status (starts each server and checks health)
atmos mcp status

# Test a specific server
atmos mcp test aws-pricing

# List tools from a server
atmos mcp tools aws-security

# Restart a server
atmos mcp restart aws-api

# Generate .mcp.json for Claude Code / Cursor / IDE
atmos mcp generate-config
```

### How to Know MCP Tools Are Active

When you run any AI command, Atmos logs which MCP servers started and how many tools were discovered:

```text
ℹ MCP server "aws-docs" started (4 tools)
ℹ MCP server "aws-pricing" started (7 tools)
ℹ Registered 11 tools from 2 MCP server(s)
ℹ AI tools initialized: 26 total
```

After the AI responds, a "Tool Executions" section shows which tools were actually called:

```text
---
## Tool Executions (2)
1. ✅ **aws-docs.search_documentation** (234ms)
2. ✅ **aws-pricing.get_pricing** (456ms)
```

Tool usage is not inferred — the AI provider explicitly declares which tools it wants to call
via the API protocol (`tool_use` stop reason with a `tool_calls` array). Atmos executes the
requested tools, sends results back to the AI for the final answer, and records every call.
If no "Tool Executions" section appears, the AI genuinely chose not to use any tools for
that question.

### Using MCP Tools in AI Chat

Interactive chat sessions with full access to all MCP server tools:

```bash
# Start an AI chat session — MCP tools are automatically available
atmos ai chat

# Example prompts:
# "What's the current pricing for m7i.xlarge instances in us-east-1?"
# "Check the security posture of our production account"
# "Search AWS docs for how to set up VPC peering"
# "List all EC2 instances in us-west-2"
# "What AWS services are available in the af-south-1 region?"
```

### Using MCP Tools with `atmos ai ask`

One-shot questions — get an answer and exit. No interactive session:

```bash
# Cost analysis (uses aws-pricing)
atmos ai ask "What's the on-demand price for m7i.xlarge in us-east-1?"

# Spend breakdown (uses aws-cost-explorer)
atmos ai ask "What did we spend on EC2 last month?"

# Billing history (uses aws-billing)
atmos ai ask "Show our billing summary for the past 3 months"

# Security posture (uses aws-security)
atmos ai ask "Is GuardDuty enabled in us-east-1?"

# IAM analysis (uses aws-iam)
atmos ai ask "List all IAM roles with admin access"

# Audit trail (uses aws-cloudtrail)
atmos ai ask "Show recent API calls from the root account"

# Documentation (uses aws-docs, no credentials needed)
atmos ai ask "How do I configure S3 bucket lifecycle rules?"

# AWS knowledge (uses aws-knowledge, no credentials needed)
atmos ai ask "Which AWS regions support Amazon Bedrock?"
```

### Using MCP Tools with `atmos ai exec`

Execute multi-step AI tasks with tool access:

```bash
# Generate a security report
atmos ai exec "Check security services, storage encryption, and network \
  security in us-east-1, then summarize the findings as a markdown report"

# Research pricing for a migration
atmos ai exec "Look up pricing for m7i.xlarge, r7i.2xlarge, and c7i.xlarge \
  in us-east-1 and us-west-2, then create a comparison table"

# Documentation research
atmos ai exec "Find AWS best practices for EKS cluster security, \
  then list the top 5 recommendations with links"
```

### Using MCP Tools with the `--ai` Flag

When you add `--ai` to any Atmos command, the output is sent to AI for analysis.
MCP tools provide additional context for the analysis:

```bash
# Analyze a terraform plan with pricing context
atmos terraform plan vpc -s prod --ai
# AI can use aws-pricing to estimate cost impact of the planned changes

# Analyze terraform output with security context
atmos terraform plan eks -s prod --ai
# AI can use aws-security to check if the EKS config follows best practices

# Analyze stack configuration with documentation context
atmos describe stacks -s prod --ai
# AI can use aws-docs to reference relevant AWS documentation

# Combine with skills for deeper analysis
atmos terraform plan vpc -s prod --ai --skill atmos-terraform
```

### Directly Exploring MCP Servers

Use `atmos mcp` commands to explore what each server offers without AI:

```bash
# List all tools from the AWS API server
atmos mcp tools aws-api

# List tools from the security server
atmos mcp tools aws-security
# Example output:
#   TOOL                      DESCRIPTION
#   CheckSecurityServices     Verify security services are enabled
#   GetSecurityFindings       Retrieve security findings with severity filtering
#   CheckStorageEncryption    Check encryption on S3, EBS, RDS, DynamoDB, EFS
#   CheckNetworkSecurity      Check TLS/HTTPS on ELB, VPC, API Gateway
#   ListServicesInRegion      List active AWS services in a region

# List tools from the pricing server
atmos mcp tools aws-pricing

# Test all servers at once
atmos mcp status
# Example output:
#   NAME              STATUS    TOOLS   DESCRIPTION
#   aws-api           running   3       AWS API — direct AWS CLI access
#   aws-billing       running   5       AWS Billing — summaries and payment history
#   aws-cloudtrail    running   3       AWS CloudTrail — event history and auditing
#   aws-cost-explorer running   7       AWS Cost Explorer — spend breakdowns
#   aws-docs          running   4       AWS Documentation — search and fetch
#   aws-iam           running   4       AWS IAM — role/policy analysis
#   aws-knowledge     running   2       AWS Knowledge — managed knowledge base
#   aws-pricing       running   7       AWS Pricing — real-time pricing
#   aws-security      running   6       AWS Security — posture assessment
```

## Configuration Reference

### atmos.yaml Structure

```yaml
mcp:
  servers:
    <server-name>:
      # Standard MCP fields (compatible with Claude Code / Codex / Gemini CLI)
      command: "uvx"                              # Command to run
      args: [ "package-name@latest" ]               # Arguments
      env: # Environment variables
        AWS_REGION: "us-east-1"

      # Atmos extensions
      description: "Human-readable description"   # Shown in `atmos mcp list`
      auth_identity: "my-identity"                 # Atmos Auth credential injection
      auto_start: false                            # Start automatically
      timeout: "30s"                               # Connection timeout
```

### YAML Functions in Environment Variables

Atmos YAML functions work in `env` values:

```yaml
mcp:
  servers:
    my-server:
      command: uvx
      args: [ "my-mcp-server@latest" ]
      env:
        # Read from OS environment
        AWS_REGION: !env AWS_DEFAULT_REGION
        AWS_PROFILE: !env AWS_PROFILE

        # Execute a command (e.g., read a secret)
        API_KEY: !exec "vault kv get -field=key secret/mcp"

        # Git repository root
        PROJECT_ROOT: !repo-root

        # Current working directory
        WORK_DIR: !cwd
```

## Atmos Auth Integration

This example uses Atmos Auth to automatically inject AWS credentials into every MCP server
that needs them. Instead of manually running `aws configure` or exporting environment
variables for each server, you configure auth once and every server with `auth_identity`
gets credentials automatically.

### Setup

1. Edit the `auth` section in `atmos.yaml` — update `start_url`, `permission_set`, and
   `account.id` to match your AWS organization
2. Run `atmos auth login` to authenticate
3. All MCP servers with `auth_identity: "readonly"` will get credentials injected

### How it works

When `auth_identity` is set on a server, Atmos:

1. Authenticates through the identity chain (SSO → role assumption)
2. Writes isolated credential files to `~/.aws/atmos/<realm>/`
3. Sets `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AWS_PROFILE` on the subprocess
4. The MCP server's AWS SDK picks up credentials automatically

No credential files to manage, no environment variables to set, no expiration headaches.
Run `atmos auth login` once and all servers work.

## Atmos Toolchain Integration

To auto-install the `uv` package manager (which provides `uvx`), add it to your toolchain:

```yaml
# atmos.yaml
toolchain:
  tools:
    uv:
      version: ">=0.7"
```

Atmos will automatically install `uv` (and make `uvx` available) before starting any MCP server. This ensures the
correct version is used across all team members and CI/CD.

## Server Details

### aws-api — Direct AWS CLI Access

The most powerful server — enables AI to run any AWS CLI command. Use with caution.

**Safety controls:**

- `READ_OPERATIONS_ONLY=true` — Only allow read operations (default in this example)
- `REQUIRE_MUTATION_CONSENT=true` — Require explicit approval before mutations

**IAM:** `ReadOnlyAccess` for read-only mode, `AdministratorAccess` for full access.

### aws-security — Security Posture Assessment

Checks your AWS environment against the Well-Architected Security Pillar:

- Security services enabled (GuardDuty, Inspector, SecurityHub, Access Analyzer)
- Storage encryption (S3, EBS, RDS, DynamoDB, EFS)
- Network security (ELB HTTPS, API Gateway WAF, CloudFront TLS)

**IAM:** Read-only access to security services + storage/network resource metadata.

### aws-pricing — Cost Analysis

Real-time pricing lookups and cost comparisons. All Pricing API calls are free.

**IAM:** `pricing:*` (all calls are free of charge).

### aws-docs — Documentation Search

Searches and fetches AWS documentation in markdown format. **No credentials needed** — this is
the easiest server to try first since it accesses public AWS documentation endpoints.

**Try it now:**

```bash
# Verify the server works
atmos mcp test aws-docs

# See what tools are available
atmos mcp tools aws-docs

# Ask a documentation question
atmos ai ask "How do I configure S3 bucket lifecycle rules?"

# Research a topic
atmos ai ask "What are the VPC quotas and limits?"

# Interactive session for deeper research
atmos ai chat
# Then: "Search AWS docs for EKS pod identity best practices"
# Then: "What's the difference between IRSA and EKS Pod Identity?"
```

The AI calls the MCP server's tools (like `search_documentation`, `get_documentation`)
behind the scenes and renders the markdown response directly in the terminal. You can also
see the raw tool list without AI:

```bash
atmos mcp tools aws-docs
```

**Configuration:** No `env` variables needed. Optionally set `AWS_DOCUMENTATION_PARTITION`
to `"aws-cn"` for China partition docs.

### aws-billing — Billing & Cost Management

Access to billing summaries, payment history, and cost allocation tags.

**IAM:** `ce:*`, `billing:*`

### aws-cost-explorer — Spend Breakdowns & Forecasts

Break down spend by service, account, tag, and time period. Provides cost trends and forecasts.

**IAM:** `ce:*`

### aws-iam — IAM Analysis

Analyze IAM roles, policies, permission boundaries, and access patterns. Read-only — no changes to IAM.

**IAM:** `iam:Get*`, `iam:List*`

### aws-cloudtrail — Event History & Auditing

Query CloudTrail event history for API call auditing, security investigations, and compliance reporting.

**IAM:** `cloudtrail:LookupEvents`

### aws-knowledge — Managed Knowledge Base

Remote MCP server operated by AWS. Provides documentation, code samples, and regional availability information. No
credentials or local installation needed.

## IDE Integration (Claude Code / Cursor)

Generate a `.mcp.json` file from your `atmos.yaml` configuration for use with Claude Code,
Cursor, or any MCP-compatible IDE:

```bash
atmos mcp generate-config
```

This creates a `.mcp.json` file where:

- Servers **without** `auth_identity` use their command directly
- Servers **with** `auth_identity` are wrapped with `atmos auth exec -i <identity> --` for
  automatic credential injection

Example generated output:

```json
{
  "mcpServers": {
    "aws-docs": {
      "command": "uvx",
      "args": ["awslabs.aws-documentation-mcp-server@latest"],
      "env": { "FASTMCP_LOG_LEVEL": "ERROR" }
    },
    "aws-security": {
      "command": "atmos",
      "args": ["auth", "exec", "-i", "readonly", "--",
               "uvx", "awslabs.well-architected-security-mcp-server@latest"],
      "env": { "AWS_REGION": "us-east-1" }
    }
  }
}
```

Use `--output` to specify a different file path:

```bash
atmos mcp generate-config --output .cursor/mcp.json
```

## Learn More

- [Atmos MCP Documentation](https://atmos.tools/cli/commands/mcp)
- [AWS MCP Servers](https://github.com/awslabs/mcp)
- [MCP Protocol Specification](https://spec.modelcontextprotocol.io/)
- [Atmos AI Documentation](https://atmos.tools/ai)
- [Atmos Auth Documentation](https://atmos.tools/cli/commands/auth)
