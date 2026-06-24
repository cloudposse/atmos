# Example: MCP Server Integrations

Connect Atmos to external MCP servers from the AWS ecosystem. Their tools become available
in `atmos ai chat`, `atmos ai ask`, and `atmos ai exec` — no custom integration code needed.

Learn more in the [MCP Configuration documentation](https://atmos.tools/cli/configuration/mcp).

## MCP Servers Included

| Server             | Description                           | Credentials |
|--------------------|---------------------------------------|-------------|
| **aws-docs**       | Search and fetch AWS documentation    | No          |
| **aws-knowledge**  | Managed AWS knowledge base (remote)   | No          |
| **aws-billing**    | Billing summaries and payment history | Yes         |
| **aws-pricing**    | Real-time pricing and cost analysis   | Yes         |
| **aws-security**   | Well-Architected security posture     | Yes         |
| **aws-iam**        | IAM role/policy analysis              | Yes         |
| **aws-cloudtrail** | Event history and API auditing        | Yes         |
| **aws-api**        | Direct AWS CLI access (read-only)     | Yes         |

## Prerequisites

1. **Python 3.10+** — `uvx` is auto-installed by the [Atmos Toolchain](https://atmos.tools/cli/configuration/toolchain).

2. **Atmos Auth** — servers that need credentials use `identity: "readonly"`.
   Update the `auth` section in `atmos.yaml` with your SSO start URL, permission set,
   and account ID, then run:
   ```bash
   atmos auth login
   ```
   See the [Atmos Auth documentation](https://atmos.tools/cli/configuration/auth) for setup details.

3. **AI provider** — configure at least one [AI provider](https://atmos.tools/cli/configuration/ai/providers).

## Try It

```shell
cd examples/mcp

# List configured servers
atmos mcp list

# Test a server (no credentials needed)
atmos mcp test aws-docs

# List tools from a server
atmos mcp tools aws-docs

# Ask a question (AI auto-routes to the right server)
atmos ai ask "How do I configure S3 bucket lifecycle rules?"

# Billing query (requires Atmos Auth)
atmos ai ask "What did we spend on EC2 last month?"

# Security audit
atmos ai ask "Is GuardDuty enabled in all regions?"

# Manual server selection (skip auto-routing)
atmos ai ask --mcp aws-iam "List all IAM roles with admin access"
```

## Smart Server Routing

When multiple servers are configured, Atmos automatically selects only the relevant ones:

```text
$ atmos ai ask "List all IAM roles with admin access"
ℹ MCP routing selected 1 of 8 servers: aws-iam
ℹ MCP server "aws-iam" started (29 tools)
```

Use `--mcp` to override routing and specify servers directly.
See the [MCP documentation](https://atmos.tools/cli/configuration/mcp#smart-routing) for details.

## IDE Integration

Generate `.mcp.json` for Claude Code, Cursor, or any MCP-compatible IDE:

```bash
atmos mcp export
atmos mcp export --output .cursor/mcp.json
```

Servers with `identity` are wrapped with `atmos auth exec` for automatic credential injection.

## See It in Action

> Outputs below are from an AWS account. Identifiers have been redacted.

### Documentation Search

```text
$ atmos ai ask "How do I configure S3 bucket lifecycle rules?"

ℹ MCP routing selected 1 of 8 servers: aws-knowledge
ℹ AI tools initialized: 16
👽 Thinking...

   Configuring S3 Bucket Lifecycle Rules

   S3 lifecycle rules automate object management by transitioning objects between
   storage classes, archiving, or expiring them.

   Component │ Description
  ───────────┼────────────────────────────────────────────────────────
   Metadata  │ Rule ID and Status (Enabled/Disabled)
   Filter    │ Which objects the rule applies to (prefix, tags, size)
   Actions   │ What to do (transition, expire, delete)

  │  A bucket can have up to 1,000 rules per lifecycle configuration.

  ---
  ## Tool Executions (1)
  1. ✅ aws-knowledge → aws.search_documentation (2874ms)
```

### Billing Summary

```text
$ atmos ai ask "Show our billing summary for the past 2 months"

ℹ MCP routing selected 1 of 8 servers: aws-billing
ℹ AI tools initialized: 35
👽 Thinking...

  ## 📊 AWS Billing Summary — February & March 2026

  ### 💰 Monthly Totals

   Service                                  │ Feb 2026 │ Mar 2026 │ Change
  ──────────────────────────────────────────┼──────────┼───────────┼──────────
   Amazon Virtual Private Cloud             │ $309.53  │ $261.17   │ ▼ $48.36
   Amazon Elastic Container Registry Public │ $103.63  │ $103.66   │ ▲ $0.03
   EC2 - Other                              │ $88.12   │ $123.39   │ ▲ $35.27
   AWS Key Management Service               │ $50.94   │ $47.98    │ ▼ $2.96
   Amazon Redshift                          │ $50.87   │ $0.30     │ ▼ $50.57
   ...                                      │          │           │

  ### 📈 Grand Totals

   Month      │ Total Cost
  ────────────┼──────────────────
   Feb 2026   │ ~$782.16
   Mar 2026   │ ~$745.26
   Difference │ ▼ $36.90 (-4.7%)

  ---
  ## Tool Executions (1)
  1. ✅ aws-billing → cost-explorer (381ms)
```

### Security Audit

```text
$ atmos ai ask "Is GuardDuty enabled in all regions?"

ℹ MCP routing selected 2 of 8 servers: aws-api, aws-security
ℹ AI tools initialized: 18
👽 Thinking...

  ## ❌ GuardDuty is NOT Enabled in All Regions

   GuardDuty is only enabled in 1 out of 34 regions checked.

  ### ✅ Enabled (1 region)

   Region    │ Detector ID
  ───────────┼────────────────────────────────────
   us-east-2 │  <detector-id-redacted>

  ### ❌ Not Enabled (33 regions)

   ap-south-1, ap-southeast-1, eu-central-1, eu-west-1, us-east-1,
   us-west-2, ... (28 more)

  ### 🔒 Recommendations
    1. Enable GuardDuty in all active regions
    2. Use delegated administrator via AWS Organizations
    3. Consider AWS Security Hub to aggregate findings
    4. Prioritize us-east-1, us-west-2, eu-west-1

  ---
  ## Tool Executions (4)
  1. ✅ aws-api → call_aws (400ms)
  2. ✅ aws-api → call_aws (14ms)
  3. ✅ aws-api → call_aws (7ms)
  4. ✅ aws-api → call_aws (9450ms)
```

### IAM Audit

```text
$ atmos ai ask "List all IAM roles with admin access"

ℹ MCP routing selected 1 of 8 servers: aws-iam
ℹ AI tools initialized: 39
👽 Thinking...

  ## 🔐 IAM Roles with Admin Access

  ### Direct AdministratorAccess Policy (4 attachments)

   Role Name                                        │ Trust Principal
  ──────────────────────────────────────────────────┼───────────────────────────
    AWSReservedSSO_AdministratorAccess_...          │ AWS SSO (SAML Federation)
    AWSReservedSSO_RootAccess_...                   │ AWS SSO (SAML Federation)
    AWSReservedSSO_TerraformApplyAccess_...         │ AWS SSO (SAML Federation)
    AWSReservedSSO_TerraformApplyAccess-Core_...    │ AWS SSO (SAML Federation)

  ### 📋 Summary

   Category                                  │ Count
  ───────────────────────────────────────────┼──────────
   Full Admin (AdministratorAccess policy)   │ 4 roles
   Broad Terraform/State access (elevated)   │ 4 roles
   AWS Service-Linked Roles (scoped)         │ 13 roles

  ---
  ## Tool Executions (2)
  1. ✅ aws-iam → list_roles (314ms)
  2. ✅ aws-iam → list_policies (174ms)
```

## Related Examples

- **[AI with Claude Code CLI](../ai-claude-code/)** — Use your Claude subscription
  instead of API tokens. Claude Code manages MCP servers via pass-through.
- **[AI with API Providers](../ai/)** — Multi-provider AI configuration with sessions and tools.

## Key Files

| File                    | Purpose                                                     |
|-------------------------|-------------------------------------------------------------|
| `atmos.yaml`            | MCP servers, auth, AI provider, and toolchain configuration |
| `stacks/`               | Stack configuration files                                   |
| `components/terraform/` | Mock Terraform components                                   |

## Learn More

- [MCP Configuration](https://atmos.tools/cli/configuration/mcp)
- [AWS MCP Servers](https://github.com/awslabs/mcp)
- [Atmos AI Documentation](https://atmos.tools/ai)
- [Atmos Auth Documentation](https://atmos.tools/cli/configuration/auth)
