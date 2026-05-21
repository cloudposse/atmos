# Example: MCP for AI Coding Assistants

Configure MCP servers ([Atmos MCP server](https://atmos.tools/ai/mcp-server) plus the
[AWS MCP server suite](https://github.com/awslabs/mcp)) **once** in `atmos.yaml`, then use
the same set of tools — with the same AWS credentials managed by Atmos Auth — from
[Claude Code](https://www.anthropic.com/claude-code),
[OpenAI Codex CLI](https://github.com/openai/codex), and
[Google Gemini CLI](https://github.com/google-gemini/gemini-cli).

Learn more in the [MCP configuration documentation](/cli/configuration/mcp).

---

## Your AI knows your stacks and components. And your cloud. And your history.

Your AI coding assistant can answer:

- What's **configured** in your infrastructure
- What's **deployed** in your cloud accounts
- What **changed** — when, why, how, and by whom

Centralized auth. Centralized security and permissions. One `atmos.yaml`.

---

## What This Example Demonstrates

- **One source of truth** — every MCP server is defined in
  `atmos.yaml` and versioned with your infrastructure code.
- **Security — every credential, in one place** —
  [Atmos Auth](/cli/configuration/auth) is the only place
  AWS credentials live. Each server with an `identity` (everything
  except `aws-docs` and `aws-knowledge`, which talk to public endpoints)
  is spawned by `atmos auth exec`, which resolves credentials at
  runtime and writes them only into that subprocess's env.
- **Convenience — one login, every account auto-routed** — configure all
  the accounts you care about in `auth.identities`, run `atmos auth login`
  once, and Atmos handles the rest. When the AI calls a
  tool, Atmos automatically picks the right account for that tool's server.
  No identity juggling between prompts, no `AWS_PROFILE` swapping, no
  re-logins to ask a billing question after asking a VPC question.
- **Toolchain managed by Atmos** — `uvx` is installed and resolved via the
  [Atmos toolchain](/cli/configuration/toolchain) so every
  CLI uses the same binary version.

## MCP Servers Configured

| Server             | Purpose                                                  | Auth                                         |
|--------------------|----------------------------------------------------------|----------------------------------------------|
| **atmos**          | Atmos AI tools (describe/list/validate, search)          | Atmos Auth                                   |
| **atmos-pro**      | Atmos Pro — drift, deployments, workflow runs, audit log | Browser OAuth (registered separately, HTTP)  |
| **aws-docs**       | Search and fetch AWS documentation                       | None (public docs)                           |
| **aws-knowledge**  | Managed AWS knowledge base (remote)                      | None (public)                                |
| **aws-pricing**    | Real-time pricing and cost analysis                      | AWS (via Atmos Auth)                         |
| **aws-billing**    | Billing summaries and payment history                    | AWS (via Atmos Auth)                         |
| **aws-iam**        | IAM role/policy analysis (read-only)                     | AWS (via Atmos Auth)                         |
| **aws-cloudtrail** | Event history and API auditing                           | AWS (via Atmos Auth)                         |
| **aws-security**   | Well-Architected security posture assessment             | AWS (via Atmos Auth)                         |
| **aws-api**        | Direct AWS CLI access (read-only by default)             | AWS (via Atmos Auth)                         |

### Where it fits in the picture

The layers complement each other:

> The **AWS servers** tell the assistant what is **deployed**.
> The **atmos** server tells it what is **defined**.
> The **atmos-pro** server tells it what is **happening over time** — drift,
> who/what changed it, why a run failed, when problems began.

So the AI can answer questions like *"why did our vpc deployment fail
yesterday, what changed in the stack config, and which AWS resource is now
out of sync?"* in a single prompt — pulling deployment history from
`atmos-pro`, the declared stack config from `atmos` (or your repo's Atmos config), and the live AWS state
from `aws-api`.

## Wiring the MCP Servers Into Your AI CLI

### Claude Code

Claude Code reads MCP servers from a `.mcp.json` file in the project root. Atmos generates
this file natively:

```bash
# Generates .mcp.json in the current directory.
atmos mcp export
claude
```

### OpenAI Codex CLI

Codex CLI reads MCP servers from `~/.codex/config.toml`:

```toml
# ~/.codex/config.toml

[mcp_servers.aws-pricing]
command = "atmos"
args = ["auth", "exec", "-i", "readonly", "--",
        "uvx", "awslabs.aws-pricing-mcp-server@latest"]
```

### Google Gemini CLI

Gemini CLI reads MCP servers from `.gemini/settings.json` (project) or
`~/.gemini/settings.json` (user):

```bash
# Per-project
atmos mcp export --output .gemini/settings.json

# Or globally for your user
atmos mcp export --output ~/.gemini/settings.json
```

## What the Exported Config Looks Like

`atmos mcp export` produces a `.mcp.json` like this (truncated):

```json
{
  "mcpServers": {
    "aws-pricing": {
      "command": "atmos",
      "args": [
        "auth",
        "exec",
        "-i",
        "readonly",
        "--",
        "uvx",
        "awslabs.aws-pricing-mcp-server@latest"
      ],
      "env": {
        "AWS_REGION": "us-east-1",
        "FASTMCP_LOG_LEVEL": "ERROR",
        "PATH": "/Users/you/.atmos/toolchain/...:..."
      }
    }
  }
}
```

Two things to notice:

1. Servers **with** `identity` (`aws-pricing` and the rest) get wrapped in
   `atmos auth exec -i readonly --`. When the AI CLI starts the subprocess, Atmos Auth
   resolves credentials and writes them into the subprocess environment.
2. Every server's `env.PATH` includes the Atmos toolchain directory so `uvx` resolves
   regardless of the user's system `PATH`.

## Example Questions to Ask

```text
# Cost analysis (uses aws-billing)
"What did we spend on EC2 across all accounts last month?"

# Security audit (uses aws-security + aws-iam + aws-api)
"Is GuardDuty enabled in all regions?"
"List all IAM roles with AdministratorAccess attached."

# Atmos Pro — drift, deployments, history
"Which workspaces have drift right now?"
"Why did the last deploy of vpc in prod fail? Show me the failed job."
"Has this stack been flapping over the past week?"
"Show me the audit log for changes to the dev stack this month."

# Combined (AI picks tools across multiple servers)
"Compare our actual EC2 spend last month with what the AWS Pricing
 calculator would have predicted for our current instance count."
"Why did our vpc deploy fail yesterday — what changed in the stack
 config, what does Atmos Pro show for that run, and which AWS resource
 is now out of sync?"
```

## Related Examples

- **[Atmos MCP integrations](/examples/mcp)** — You drive the
  AI loop **through Atmos** (`atmos ai ask`, `atmos ai chat`, `atmos ai exec`)
  and want it to call external MCP servers.

- **[Atmos AI with Claude Code](/examples/ai-claude-code)** —
  You want to use your Claude Pro/Max subscription as the AI provider for
  `atmos ai ask` (no Anthropic API key needed).

- **[Atmos AI (multi-provider)](/examples/ai)** — You want to
  chat with your infrastructure using API-key providers (Anthropic, OpenAI).
  Multi-provider Atmos AI setup, no external CLI needed.
