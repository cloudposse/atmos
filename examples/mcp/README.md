# Atmos MCP Integrations Example

This example demonstrates how to connect Atmos to external MCP (Model Context Protocol) servers from the AWS ecosystem.
Instead of reimplementing cloud provider functionality, Atmos installs and orchestrates existing MCP servers — their
tools become available across the Atmos AI surface:

- **`atmos ai chat`** — interactive conversations with MCP tools
- **`atmos ai ask`** — one-shot questions using MCP tools
- **`atmos ai exec`** — execute AI-driven tasks with MCP tools
- **`atmos mcp tools <name>`** — directly list tools from any MCP server
- **`atmos mcp test <name>`** — verify server connectivity and available tools

## What's Included

The following AWS MCP servers are pre-configured in `atmos.yaml`:

### Cost Analysis & FinOps

| Server               | What It Does                                      | Credentials               |
|----------------------|---------------------------------------------------|---------------------------|
| **aws-billing**      | Billing summaries, payment history, cost tags     | Yes — `ce:*`, `billing:*` |
| **aws-pricing**      | On-demand/reserved pricing, cost comparisons      | Yes — `pricing:*` (free)  |

### Security & Compliance

| Server             | What It Does                                    | Credentials                     |
|--------------------|-------------------------------------------------|---------------------------------|
| **aws-security**   | Well-Architected Security Pillar assessment     | Yes — security services read    |
| **aws-iam**        | IAM role/policy analysis, permission boundaries | Yes — `iam:Get*`, `iam:List*`   |
| **aws-cloudtrail** | CloudTrail event history, API call auditing     | Yes — `cloudtrail:LookupEvents` |

### General

| Server            | What It Does                                 | Credentials |
|-------------------|----------------------------------------------|-------------|
| **aws-api**       | Direct AWS CLI access with security controls | Yes         |
| **aws-docs**      | Search and fetch AWS documentation           | No          |
| **aws-knowledge** | Managed AWS knowledge base (remote service)  | No          |

All configured MCP servers are available across all Atmos AI commands — `atmos ai ask`,
`atmos ai chat`, and `atmos ai exec`.

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
# Start the Atmos MCP server (for IDE/Claude Code integration)
atmos mcp start

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

### Smart Server Routing

When multiple MCP servers are configured, Atmos automatically selects only the servers
relevant to your question using a lightweight routing call to your configured AI provider.
This keeps tool payloads small and responses fast, even with dozens of servers configured:

```text
$ atmos ai ask "List all IAM roles with admin access"
ℹ MCP routing selected 1 of 8 servers: aws-iam
ℹ MCP server "aws-iam" started (29 tools)
ℹ Registered 29 tools from 1 MCP server(s)
ℹ AI tools initialized: 39
```

Use `--mcp` to override and specify servers directly:

```bash
# Specify one server
atmos ai ask --mcp aws-iam "List all admin roles"

# Specify multiple servers (comma-separated)
atmos ai ask --mcp aws-iam,aws-cloudtrail "Who accessed the admin role last week?"

# Specify multiple servers (repeated flag)
atmos ai ask --mcp aws-iam --mcp aws-cloudtrail "Who accessed the admin role?"

# Works with all AI commands
atmos ai chat --mcp aws-billing
atmos ai exec --mcp aws-security,aws-iam "audit our security posture"

# Environment variable
ATMOS_AI_MCP=aws-billing atmos ai ask "What did we spend last month?"
```

Routing is skipped when only one server is configured or when `--mcp` is provided.
In `chat` mode, routing is skipped because the question isn't known upfront — use
`--mcp` to filter servers in chat.

### How to Know MCP Tools Are Active

When you run any AI command, Atmos logs which MCP servers started and how many tools were discovered:

```text
ℹ MCP routing selected 2 of 8 servers: aws-docs, aws-pricing
ℹ MCP server "aws-docs" started (4 tools)
ℹ MCP server "aws-pricing" started (7 tools)
ℹ Registered 11 tools from 2 MCP server(s)
ℹ AI tools initialized: 26 total
```

After the AI responds, a "Tool Executions" section shows which tools were actually called:

```text
---
## Tool Executions (2)
1. ✅ aws-docs → aws.search_documentation (234ms)
2. ✅ aws-pricing → get_pricing (456ms)
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

# Spend breakdown (uses aws-billing)
atmos ai ask "What did we spend on EC2 last month?"

# Billing history (uses aws-billing)
atmos ai ask "Show our billing summary for the past 3 months"

# Security posture (uses aws-security)
atmos ai ask "Is GuardDuty enabled in all regions?"

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
#   NAME           STATUS   TOOLS  DESCRIPTION
#   aws-api        running  2      AWS API — direct AWS CLI access with security controls
#   aws-billing    running  25     AWS Billing — billing summaries and payment history
#   aws-cloudtrail running  5      AWS CloudTrail — event history and API call auditing
#   aws-docs       running  4      AWS Documentation — search and fetch AWS docs
#   aws-iam        running  29     AWS IAM — role/policy analysis and access patterns
#   aws-knowledge  running  6      AWS Knowledge — managed AWS knowledge base (remote)
#   aws-pricing    running  9      AWS Pricing — real-time pricing and cost analysis
#   aws-security   running  6      AWS Security — Well-Architected security posture assessment
```

## Configuration Reference

### atmos.yaml Structure

```yaml
mcp:
  servers:
    <server-name>:
      # Standard MCP fields (compatible with Claude Code / Codex / Gemini CLI)
      command: "uvx"                              # Command to run
      args: [ "package-name@latest" ]             # Arguments
      env: # Environment variables
        AWS_REGION: "us-east-1"

      # Atmos extensions
      description: "Human-readable description"    # Shown in `atmos mcp list`
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

Map `uv` to the aqua registry so the toolchain can resolve it, then install:

```yaml
# atmos.yaml
toolchain:
  aliases:
    uv: astral-sh/uv
```

```bash
atmos toolchain install astral-sh/uv@0.7.12
```

Atmos resolves `uvx` from the toolchain PATH before starting any MCP server.

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

## See It in Action

### List configured servers

```text
$ atmos mcp list
       NAME         STATUS                           DESCRIPTION
─────────────────────────────────────────────────────────────────────────────────────────
 aws-api            stopped  AWS API — direct AWS CLI access with security controls
 aws-billing        stopped  AWS Billing — billing summaries and payment history
 aws-cloudtrail     stopped  AWS CloudTrail — event history and API call auditing
 aws-docs           stopped  AWS Documentation — search and fetch AWS docs
 aws-iam            stopped  AWS IAM — role/policy analysis and access patterns
 aws-knowledge      stopped  AWS Knowledge — managed AWS knowledge base (remote)
 aws-pricing        stopped  AWS Pricing — real-time pricing and cost analysis
 aws-security       stopped  AWS Security — Well-Architected security posture assessment
```

### Explore tools from a server

```text
$ atmos mcp tools aws-api
         TOOL                                      DESCRIPTION
───────────────────────────────────────────────────────────────────────────────────────────
 suggest_aws_commands  Suggest AWS CLI commands based on a natural language query.
 call_aws              Execute AWS CLI commands with validation and proper error handling.
```

```text
$ atmos mcp tools aws-security
           TOOL                                                         DESCRIPTION
──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
 CheckSecurityServices     Verify if selected AWS security services are enabled in the specified region and account.
 GetSecurityFindings       Retrieve security findings from AWS security services.
 GetStoredSecurityContext  Retrieve security services data that was stored in context from a previous CheckSecurityServices call.
 CheckStorageEncryption    Check if AWS storage resources have encryption enabled.
 ListServicesInRegion      List all AWS services being used in a specific region.
 CheckNetworkSecurity      Check if AWS network resources are configured for secure data-in-transit.
```

### Test server connectivity

```text
$ atmos mcp test aws-docs
✓ Server started successfully
✓ Initialization handshake complete
✓ 4 tools available
✓ Server responds to ping
```

```text
$ atmos mcp test aws-security
✓ Server started successfully
✓ Initialization handshake complete
✓ 6 tools available
✓ Server responds to ping
```

### Ask AI with MCP tools

```text
$ atmos ai ask "How do I configure S3 bucket lifecycle rules?"

ℹ MCP routing selected 1 of 8 servers: aws-knowledge
ℹ MCP server "aws-knowledge" started (6 tools)
ℹ Registered 6 tools from 1 MCP server(s)
ℹ AI tools initialized: 16
👽 Thinking...

   Configuring S3 Bucket Lifecycle Rules

   S3 lifecycle rules automate object management by transitioning objects between
   storage classes, archiving, or expiring them. Here's a comprehensive guide:

  --------

  ## 📋 Lifecycle Rule Structure

   Each rule contains 3 key components:

   Component │ Description
  ───────────┼────────────────────────────────────────────────────────
   Metadata  │ Rule ID and Status (Enabled/Disabled)
   Filter    │ Which objects the rule applies to (prefix, tags, size)
   Actions   │ What to do (transition, expire, delete)

  │  A bucket can have up to 1,000 rules per lifecycle configuration.

  --------

  ## 🔧 Available Actions

   Action                           │ Description
  ──────────────────────────────────┼────────────────────────────────────────────────────────
    Transition                      │ Move objects to a different storage class after N days
    Expiration                      │ Permanently delete current object versions
    NoncurrentVersionTransition     │ Transition old versions in versioned buckets
    NoncurrentVersionExpiration     │ Delete noncurrent versions in versioned buckets
    AbortIncompleteMultipartUpload  │ Cancel incomplete uploads after N days
    ExpiredObjectDeleteMarker       │ Remove expired delete markers

  ---

  ## Tool Executions (1)

  1. ✅ aws-knowledge → aws.search_documentation (2874ms)
```

### Ask AI about billing with MCP tools

```text
$ atmos ai ask "Show our billing summary for the past 2 months"
ℹ MCP routing selected 1 of 8 servers: aws-billing
ℹ MCP server "aws-billing" started (25 tools)
ℹ Registered 25 tools from 1 MCP server(s)
ℹ AI tools initialized: 35
👽 Thinking...

   I'll retrieve your billing summary for the past 2 months right away!

   Here's your billing summary for the past 2 months (February–March 2026):

  --------

  ## 📊 AWS Billing Summary — February & March 2026

  ### 💰 Monthly Totals

   Service                                  │ Feb 2026 │ Mar 2026 │ Change
  ──────────────────────────────────────────┼──────────┼───────────┼──────────
   Amazon Virtual Private Cloud             │ $309.53  │ $261.17   │ ▼ $48.36
   Amazon Elastic Container Registry Public │ $103.63  │ $103.66   │ ▲ $0.03
   EC2 - Other                              │ $88.12   │ $123.39   │ ▲ $35.27
   AWS Key Management Service               │ $50.94   │ $47.98    │ ▼ $2.96
   Amazon Redshift                          │ $50.87   │ $0.30     │ ▼ $50.57
   Tax                                      │ $47.80   │ $40.98    │ ▼ $6.82
   Amazon Relational Database Service       │ $43.99   │ $0.11     │ ▼ $43.88
   AWS App Runner                           │ $28.42   │ $27.55    │ ▼ $0.87
   AWS WAF                                  │ $18.00   │ $18.00    │ —
   Amazon EC2 Compute                       │ $5.03    │ $15.30    │ ▲ $10.27
   AmazonCloudWatch                         │ $10.08   │ $10.45    │ ▲ $0.37
   Amazon Simple Storage Service            │ $10.08   │ $9.35     │ ▼ $0.73
   Amazon Route 53                          │ $5.61    │ $5.68     │ ▲ $0.07
   Amazon Elastic File System               │ $3.65    │ —         │ ▼ $3.65
   Amazon EC2 Container Registry (ECR)      │ $2.69    │ $2.75     │ ▲ $0.06
   Amazon EKS                               │ —        │ $1.86     │ ▲ $1.86
   Amazon Registrar                         │ —        │ $14.00    │ ▲ $14.00
   AWS Cost Explorer                        │ $2.07    │ $1.91     │ ▼ $0.16
   Other services                           │ ~$1.65   │ ~$1.41    │ ▼ $0.24

  --------

  ### 📈 Grand Totals

   Month      │ Total Cost
  ────────────┼──────────────────
   Feb 2026   │ ~$782.16
   Mar 2026   │ ~$745.26
   Difference │ ▼ $36.90 (-4.7%)

  --------

  ### 🔍 Key Observations

    1. Amazon VPC is the largest cost at ~$261–$310/month (NAT Gateway data
       transfer is typically the main driver). Costs dropped notably in March.
    2. Redshift & RDS dropped sharply in March ($50.87 → $0.30 and
       $43.99 → $0.11), suggesting those resources were stopped or deleted.
    3. EC2 - Other increased by $35 in March — worth investigating if this
       correlates with EBS, data transfer, or Elastic IP costs.
    4. Amazon EC2 Compute tripled in March ($5.03 → $15.30), possibly linked
       to the appearance of EKS ($1.86), indicating new compute workloads.
    5. Amazon Registrar appeared as a new $14 charge in March (domain renewal).

  │  ⚠️ Note: Credits and refunds are excluded from these figures.

  --------

  ## Tool Executions (1)

  1. ✅ aws-billing → cost-explorer (381ms)
```

### Check security posture with MCP tools

```text
$ atmos ai ask "Is GuardDuty enabled in all regions?"
ℹ MCP routing selected 2 of 8 servers: aws-api, aws-security
ℹ MCP server "aws-api" started (2 tools)
ℹ MCP server "aws-security" started (6 tools)
ℹ Registered 8 tools from 2 MCP server(s)
ℹ AI tools initialized: 18
👽 Thinking...

  ## ❌ GuardDuty is NOT Enabled in All Regions

   GuardDuty is only enabled in 1 out of 34 regions checked.

  ### ✅ Enabled (1 region)

   Region    │ Detector ID
  ───────────┼────────────────────────────────────
   us-east-2 │  c8c419ea4992fa2eb9c9c23aef14a66e

  ### ❌ Not Enabled (33 regions)

   Region         │ Region         │ Region
  ────────────────┼────────────────┼────────────────
   ap-south-1     │ ap-south-2     │ ap-southeast-1
   ap-southeast-2 │ ap-southeast-3 │ ap-southeast-4
   ap-northeast-1 │ ap-northeast-2 │ ap-northeast-3
   ap-east-1      │ ca-central-1   │ ca-west-1
   eu-central-1   │ eu-central-2   │ eu-north-1
   eu-south-1     │ eu-south-2     │ eu-west-1
   eu-west-2      │ eu-west-3      │ me-central-1
   me-south-1     │ il-central-1   │ af-south-1
   sa-east-1      │ us-east-1      │ us-west-1
   us-west-2      │ mx-central-1   │ ...

  --------

  ### 🔒 Recommendations

    1. Enable GuardDuty in all active regions — even regions where you don't
       actively deploy can be targeted by attackers.
    2. Use a GuardDuty delegated administrator (via AWS Organizations) to
       centrally enable and manage across all accounts and regions.
    3. Consider AWS Security Hub to aggregate findings from all regions.
    4. Prioritize us-east-1, us-west-2, eu-west-1 immediately.

  --------

  ## Tool Executions (4)

  1. ✅ aws-api → call_aws (400ms)
  2. ✅ aws-api → call_aws (14ms)
  3. ✅ aws-api → call_aws (7ms)
  4. ✅ aws-api → call_aws (9450ms)
```

### Audit IAM roles with smart routing

```text
$ atmos ai ask "List all IAM roles with admin access"
ℹ MCP routing selected 1 of 8 servers: aws-iam
ℹ MCP server "aws-iam" started (29 tools)
ℹ Registered 29 tools from 1 MCP server(s)
ℹ AI tools initialized: 39
👽 Thinking...

   I'll help you find all IAM roles with admin access. Let me fetch the
   list of IAM roles and policies simultaneously.

  --------

  ## 🔐 IAM Roles with Admin Access

  ### 1. ✅ Direct AdministratorAccess Policy (4 attachments)

   Role Name                                        │ Description                                    │ Trust Principal
  ──────────────────────────────────────────────────┼────────────────────────────────────────────────┼───────────────────────────
    AWSReservedSSO_AdministratorAccess_...          │ Allow Full Administrator access to the account │ AWS SSO (SAML Federation)
    AWSReservedSSO_RootAccess_...                   │ Centralized root access to member accounts     │ AWS SSO (SAML Federation)
    AWSReservedSSO_TerraformApplyAccess_...         │ Full Terraform state and account access        │ AWS SSO (SAML Federation)
    AWSReservedSSO_TerraformApplyAccess-Core_...    │ Full Terraform access (core backend)           │ AWS SSO (SAML Federation)

  --------

  ## 📋 Summary

   Category                                  │ Count
  ───────────────────────────────────────────┼──────────
   Full Admin (AdministratorAccess policy)   │ 4 roles
   Broad Terraform/State access (elevated)   │ 4 roles
   AWS Service-Linked Roles (scoped)         │ 13 roles
   Other application roles (Lambda, etc.)    │ 4 roles

  --------

  ### 🛡️ Security Recommendations

    1. Review SSO assignments for AdministratorAccess and RootAccess roles.
    2. Audit TerraformApplyAccess roles — ensure MFA/session policies are enforced.
    3. Monitor tfstate roles — cross-account trust across 14 accounts.
    4. Enable CloudTrail for AssumeRole calls on high-privilege roles.

  --------

  ## Tool Executions (2)

  1. ✅ aws-iam → list_roles (314ms)
  2. ✅ aws-iam → list_policies (174ms)
```

### Check status of all servers

```text
$ atmos mcp status
      NAME       STATUS   TOOLS                        DESCRIPTION
─────────────────────────────────────────────────────────────────────────────────────────
 aws-api         running  2      AWS API — direct AWS CLI access with security controls
 aws-billing     running  25     AWS Billing — billing summaries and payment history
 aws-cloudtrail  running  5      AWS CloudTrail — event history and API call auditing
 aws-docs        running  4      AWS Documentation — search and fetch AWS docs
 aws-iam         running  29     AWS IAM — role/policy analysis and access patterns
 aws-knowledge   running  6      AWS Knowledge — managed AWS knowledge base (remote)
 aws-pricing     running  9      AWS Pricing — real-time pricing and cost analysis
 aws-security    running  6      AWS Security — Well-Architected security posture assessment
```

## Learn More

- [Atmos MCP Documentation](https://atmos.tools/cli/commands/mcp)
- [AWS MCP Servers](https://github.com/awslabs/mcp)
- [MCP Protocol Specification](https://modelcontextprotocol.io/)
- [Atmos AI Documentation](https://atmos.tools/ai)
- [Atmos Auth Documentation](https://atmos.tools/cli/commands/auth)
