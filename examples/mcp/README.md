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

Five AWS MCP servers are pre-configured in `atmos.yaml`:

| Server            | What It Does                                 | Credentials Required |
|-------------------|----------------------------------------------|----------------------|
| **aws-api**       | Direct AWS CLI access with security controls | Yes                  |
| **aws-docs**      | Search and fetch AWS documentation           | No                   |
| **aws-knowledge** | Managed AWS knowledge base (remote service)  | No                   |
| **aws-pricing**   | Real-time AWS pricing and cost analysis      | Yes (free API)       |
| **aws-security**  | Well-Architected security posture assessment | Yes (read-only)      |

## Prerequisites

1. **Python 3.10+** with the `uv` package manager:
   ```bash
   # macOS
   brew install uv

   # Or via pip
   pip install uv
   ```

2. **AWS credentials** (for servers that need them):
   ```bash
   # Option A: AWS CLI profile
   aws configure

   # Option B: Environment variables
   export AWS_ACCESS_KEY_ID=...
   export AWS_SECRET_ACCESS_KEY=...
   export AWS_DEFAULT_REGION=us-east-1
   ```

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

# Add a new server
atmos mcp add my-server \
  --command uvx \
  --args "awslabs.some-mcp-server@latest" \
  --description "My custom server" \
  --env AWS_REGION=us-east-1

# Remove a server
atmos mcp remove my-server
```

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
# Pricing questions
atmos ai ask "What's the on-demand price for an m7i.xlarge in us-east-1?"

# Documentation lookups
atmos ai ask "How do I configure S3 bucket versioning?"

# Security checks
atmos ai ask "Is GuardDuty enabled in us-east-1?"

# AWS knowledge
atmos ai ask "Which AWS regions support Amazon Bedrock?"

# Cost comparison
atmos ai ask "Compare the pricing of t3.medium vs t3.large in us-west-2"
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
#   NAME            STATUS    TOOLS   DESCRIPTION
#   aws-api         running   3       AWS API — direct AWS CLI access
#   aws-docs        running   4       AWS Documentation — search and fetch
#   aws-knowledge   running   2       AWS Knowledge — managed knowledge base
#   aws-pricing     running   7       AWS Pricing — real-time pricing
#   aws-security    running   6       AWS Security — posture assessment
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

For production use, configure Atmos Auth to automatically inject AWS credentials into MCP server subprocesses. This
eliminates manual credential management:

```yaml
# atmos.yaml
auth:
  enabled: true
  providers:
    aws-sso:
      type: aws-sso
      spec:
        start_url: "https://your-org.awsapps.com/start"
        region: "us-east-1"
  identities:
    security-audit:
      provider: aws-sso
      spec:
        role_name: "SecurityAuditRole"
        account_id: "123456789012"
    pricing-reader:
      provider: aws-sso
      spec:
        role_name: "PricingReadOnly"
        account_id: "123456789012"

mcp:
  servers:
    aws-security:
      command: uvx
      args: [ "awslabs.well-architected-security-mcp-server@latest" ]
      auth_identity: "security-audit"   # ← Credentials injected automatically

    aws-pricing:
      command: uvx
      args: [ "awslabs.aws-pricing-mcp-server@latest" ]
      auth_identity: "pricing-reader"   # ← Credentials injected automatically
```

When `auth_identity` is set, Atmos:

1. Authenticates through the identity chain (SSO → role assumption)
2. Writes isolated credential files to `~/.aws/atmos/<realm>/`
3. Sets `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AWS_PROFILE` on the subprocess
4. The MCP server's AWS SDK picks up credentials automatically

## Atmos Toolchain Integration

To auto-install the `uv` package manager (which provides `uvx`), add it to your toolchain:

```yaml
# atmos.yaml
toolchain:
  tools:
    uv:
      version: "0.7.x"
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

**Configuration:** No `env` variables needed. Optionally set `AWS_DOCUMENTATION_PARTITION`
to `"aws-cn"` for China partition docs.

### aws-knowledge — Managed Knowledge Base

Remote MCP server operated by AWS. Provides documentation, code samples, and regional availability information. No
credentials or local installation needed.

## Learn More

- [Atmos MCP Documentation](https://atmos.tools/cli/commands/mcp)
- [AWS MCP Servers](https://github.com/awslabs/mcp)
- [MCP Protocol Specification](https://spec.modelcontextprotocol.io/)
- [Atmos AI Documentation](https://atmos.tools/ai)
- [Atmos Auth Documentation](https://atmos.tools/cli/commands/auth)
