# Atmos AI with Claude Code CLI Provider

Atmos AI supports two ways to connect to AI models:

1. **API providers** — Direct API calls using purchased tokens (Anthropic, OpenAI, Gemini,
   Grok, Ollama, Bedrock, Azure OpenAI). See the [`examples/ai/`](../ai/) example.
2. **CLI providers** — Invoke locally installed AI tools as subprocesses, reusing your
   existing subscription. This example demonstrates this approach with Claude Code.

This example shows how to use your **Claude Pro/Max subscription** instead of API tokens.
Claude Code manages the AI conversation and tool execution — Atmos provides infrastructure
context, MCP server orchestration, and automatic AWS credential injection.

## Two Ways to Use Atmos AI

### API Providers (Token-Based)

The traditional approach requires purchasing API tokens from a provider:

```yaml
# examples/ai/atmos.yaml
ai:
  default_provider: "anthropic"
  providers:
    anthropic:
      model: "claude-sonnet-4-6"
      api_key: !env "ANTHROPIC_API_KEY"    # ← Requires API key
```

**Setup:** Create API account → Generate key → Set env var → Pay per token.
Atmos sends prompts directly to the provider's API and manages tool execution in-process.
See the [`examples/ai/`](../ai/) example for this approach.

### CLI Providers (Subscription-Based) — This Example

CLI providers invoke your locally installed AI tool (`claude`, `codex`, `gemini`) as a
subprocess. No API keys needed — the CLI tool uses your existing subscription:

```yaml
# examples/ai-claude-code/atmos.yaml
ai:
  default_provider: "claude-code"
  providers:
    claude-code:
      max_turns: 10                        # ← No API key needed
```

**Setup:** `brew install claude` + `claude auth login` → Done.
Atmos passes the prompt to `claude -p`, which handles the AI conversation using your
Claude Pro/Max subscription. MCP servers are passed via `--mcp-config` so Claude Code
can use AWS tools directly.

### Comparison

| Feature      | API Providers (examples/ai/)         | CLI Providers (this example)          |
|--------------|--------------------------------------|---------------------------------------|
| **Auth**     | API key in env var                   | Claude Pro/Max subscription (OAuth)   |
| **Cost**     | Per-token ($3-15/M tokens)           | Included in subscription              |
| **Setup**    | Generate API key + configure env var | `brew install claude` (already done)  |
| **Tools**    | Atmos tools executed in-process      | Claude Code tools + MCP pass-through  |
| **MCP**      | Atmos manages MCP servers            | Claude Code manages MCP servers       |
| **CI/CD**    | Works everywhere (env var auth)      | Requires CLI + interactive login      |
| **Best for** | Automation, CI/CD, enterprise        | Interactive development, cost savings |

### Mixing Providers

You can configure both approaches in the same `atmos.yaml` and switch between them:

```yaml
ai:
  default_provider: "claude-code"   # Default: use subscription
  providers:
    claude-code:
      max_turns: 10
    anthropic:                      # Fallback: use API tokens
      model: "claude-sonnet-4-6"
      api_key: !env "ANTHROPIC_API_KEY"
```

```bash
# Uses claude-code (default)
atmos ai ask "What stacks do we have?"

# Override to use API provider
atmos ai ask --provider anthropic "What stacks do we have?"
```

### Available CLI Providers

| Provider     | Binary   | Subscription                         | Config Key    |
|--------------|----------|--------------------------------------|---------------|
| Claude Code  | `claude` | Claude Pro/Max ($20-200/mo)          | `claude-code` |
| OpenAI Codex | `codex`  | ChatGPT Plus/Pro ($20-200/mo)        | `codex-cli`   |
| Gemini CLI   | `gemini` | Google account (free tier available) | `gemini-cli`  |

## Prerequisites

1. **Claude Code** installed and authenticated:
   ```bash
   brew install claude
   claude auth login
   ```

2. **Python 3.10+** — for AWS MCP servers (installed via `uvx`)

3. **Atmos Auth** (for AWS MCP servers) — update the auth section in `atmos.yaml`
   with your SSO start URL, permission set, and account ID, then:
   ```bash
   atmos auth login
   ```

## Quick Start

```bash
# Navigate to this example
cd examples/ai-claude-code

# Simple questions (no MCP needed)
atmos ai ask "What stacks do we have?"
atmos ai ask "Describe the vpc component"

# Questions that use MCP servers (auto-routed)
atmos ai ask "What did we spend on EC2 last month?"
atmos ai ask "Is GuardDuty enabled in all regions?"
atmos ai ask "List all IAM roles with admin access"

# Specify MCP servers directly
atmos ai ask --mcp aws-billing "Show our billing summary"
atmos ai ask --mcp aws-iam,aws-cloudtrail "Who accessed the admin role?"

# Interactive chat with MCP tools
atmos ai chat --mcp aws-billing
```

## How It Works

```text
User: atmos ai ask "What did we spend on EC2 last month?"

1. Atmos selects provider: claude-code
2. Atmos checks: mcp.servers configured? Yes (8 servers)
3. Smart routing selects relevant server: aws-billing
4. Atmos generates temp .mcp.json:
   {
     "mcpServers": {
       "aws-billing": {
         "command": "atmos",
         "args": ["auth", "exec", "-i", "readonly", "--",
                  "uvx", "awslabs.billing-cost-management-mcp-server@latest"],
         "env": { "AWS_REGION": "us-east-1", "PATH": "/toolchain/bin:..." }
       }
     }
   }
5. Atmos invokes: claude -p --output-format json --mcp-config /tmp/atmos-mcp.json
6. Claude Code starts the MCP server (with auth credentials)
7. Claude Code calls aws-billing tools to get cost data
8. Atmos parses the JSON response and displays it
```

## Configuration

The `atmos.yaml` in this example configures:

- **Claude Code as the AI provider** — no API key needed
- **8 AWS MCP servers** — billing, pricing, security, IAM, CloudTrail, API, docs, knowledge
- **Atmos Auth** — automatic SSO credential injection for MCP servers
- **Toolchain** — `uv` mapped to aqua registry for `uvx` resolution

### Key Settings

```yaml
ai:
  enabled: true
  default_provider: "claude-code"
  providers:
    claude-code:
      max_turns: 10          # Max agentic turns per invocation
      # max_budget_usd: 1.00 # Optional budget cap
```

### MCP Server Pass-Through

When `mcp.servers` is configured, Atmos automatically:
1. Generates a temp `.mcp.json` with auth wrapping
2. Injects toolchain PATH so `uvx` is available
3. Passes `--mcp-config` to Claude Code
4. Cleans up the temp file after the command

## Available MCP Servers

| Server             | Description                           | Credentials |
|--------------------|---------------------------------------|-------------|
| **aws-docs**       | Search and fetch AWS docs             | No          |
| **aws-knowledge**  | Managed AWS knowledge base            | No          |
| **aws-pricing**    | Real-time pricing and cost analysis   | Yes         |
| **aws-api**        | Direct AWS CLI access (read-only)     | Yes         |
| **aws-security**   | Well-Architected security posture     | Yes         |
| **aws-billing**    | Billing summaries and payment history | Yes         |
| **aws-iam**        | IAM role/policy analysis              | Yes         |
| **aws-cloudtrail** | Event history and API call auditing   | Yes         |

## See It in Action

> All outputs below are from a real AWS account. Identifiers have been redacted.

```text
$ atmos ai ask "What is our security posture in us-east-2 region?"

ℹ MCP servers configured: 8 (config: /tmp/atmos-mcp-config.json)
ℹ AI tools initialized: 16 total
ℹ AI provider: claude-code
👽 Thinking...

  ## Security Posture Summary for us-east-2

  ### ✅ Enabled Security Services (2/6)

   GuardDuty - Fully operational

    • Status: ENABLED
    • Finding frequency: Every 6 hours
    • Active data sources: CloudTrail, DNS Logs, VPC Flow Logs,
      S3 Logs, EKS Audit Logs, RDS Login Events
    • Current findings: None (clean)

   Inspector - Partially enabled

    • Status: ENABLED but no scan types active
    • EC2 scanning: DISABLED
    • ECR scanning: DISABLED
    • Lambda scanning: DISABLED

  ### ❌ Disabled Security Services (4/6)

    1. IAM Access Analyzer - Not configured
    2. Security Hub - Not enabled
    3. Trusted Advisor - Error checking status
    4. Macie - Not enabled

  ### Key Recommendations

   High Priority:

    1. Enable Security Hub for centralized security findings aggregation
    2. Enable IAM Access Analyzer to identify unintended resource access
    3. Enable at least one Inspector scan type for vulnerability management

   Medium Priority:

    4. Enable GuardDuty Runtime Monitoring for deeper threat detection
    5. Enable EBS Malware Protection in GuardDuty
    6. Enable Macie if you have sensitive data in S3 buckets

   Your security posture is moderate — you have basic threat detection
   enabled but are missing several important security services for
   comprehensive protection.
```

## Learn More

- [Atmos AI Documentation](https://atmos.tools/ai)
- [MCP Configuration](https://atmos.tools/cli/configuration/mcp)
- [Claude Code CLI Reference](https://docs.anthropic.com/en/docs/claude-code/cli-reference)
- [AWS MCP Servers](https://github.com/awslabs/mcp)
