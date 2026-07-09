---
name: atmos-ai
description: "Atmos AI and MCP integrations: connect external AI assistants to Atmos through agent skills, atmos mcp start, multi-CLI MCP export, Atmos Pro MCP, and AWS MCP servers; run AI from Atmos through atmos ai ask/chat/exec, --ai command analysis, API providers, CLI providers, external MCP routing/pass-through, toolchain-aware export, auth-wrapped tools, and MCP+skills pairing"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos AI and MCP

## Purpose

Use this skill when the work is about AI and Atmos together. There are two integration patterns:

- **AI uses Atmos**: external AI assistants use Atmos Agent Skills for knowledge and Atmos MCP
  servers for tools. This includes `atmos mcp start`, `atmos mcp export`, the Atmos MCP server,
  Atmos Pro MCP, AWS MCP servers, and MCP+skills setup for Claude Code, Codex, Gemini, Cursor,
  Windsurf, GitHub Copilot, and similar clients.
- **Atmos uses AI**: Atmos calls AI providers directly or through local CLI providers. This
  includes `atmos ai ask`, `atmos ai chat`, `atmos ai exec`, `--ai` command analysis, API
  providers, CLI providers, external MCP server routing, and CLI provider MCP pass-through.

This skill is the coordination layer for AI providers, agent skills, MCP configuration, MCP
export, and the "AI inside AI" setup where an external assistant calls Atmos, and Atmos can also
call AI.

## Routing

| Work | Use |
|------|-----|
| AI provider setup, `atmos ai`, `--ai`, MCP server/client config, MCP export, agent-skill pairing | Stay in `atmos-ai` |
| Project discovery, resolved stacks/components, provenance, query filters, affected analysis | Load `atmos-introspection` |
| Terraform plan/apply/deploy/destroy, `--affected`, `--all`, `--query`, CI execution matrices | Load `atmos-terraform` or `atmos-ci` |
| Cloud credentials, identities, SSO/OIDC, auth-wrapped MCP servers | Load `atmos-auth` |
| Tool binaries for MCP servers, `uvx`/`npx` resolution, Aqua aliases, PATH injection | Load `atmos-toolchain` |

## Atmos Uses AI: Commands and Providers

```bash
atmos ai chat
atmos ai ask "What stacks do we have?"
atmos ai exec "validate stacks" --format json
atmos ai sessions list
atmos ai skill list

# Analyze any Atmos command output with AI
atmos terraform plan vpc -s prod --ai
atmos terraform plan vpc -s prod --ai --skill atmos-terraform
atmos terraform plan vpc -s prod --ai --skill atmos-terraform,atmos-stacks
```

Use API providers for CI/CD and non-interactive automation. Use CLI providers when the user
wants to reuse an existing local subscription such as Claude Code, Codex CLI, or Gemini CLI.

```yaml
ai:
  enabled: true
  default_provider: claude-code
  providers:
    claude-code:
      max_turns: 10
  tools:
    enabled: true
```

## AI Uses Atmos: MCP and Skills

Atmos MCP has two separate capabilities:

- **Atmos MCP server**: `atmos mcp start` exposes Atmos AI tools to external clients.
- **External MCP connections**: `mcp.servers` lets Atmos or an AI CLI use AWS, cloud, database,
  or custom MCP servers.

The Atmos MCP server is disabled by default. Enabling AI does not enable MCP.

```yaml
mcp:
  enabled: true

ai:
  enabled: true
  tools:
    enabled: true
```

Use Atmos Agent Skills with MCP. MCP provides live tools; skills provide Atmos-native knowledge
and conventions.

| Layer  | What it provides                                | Example                                          |
|--------|-------------------------------------------------|--------------------------------------------------|
| MCP    | **Tools** -- live data and execution capability | "What stacks exist?" -> atmos MCP tool call      |
| Skills | **Knowledge** -- domain patterns and conventions | "How should I structure cross-stack deps?" -> skill |

Without skills, an AI assistant falls back to general training data that may generate invalid
YAML, miss features like `!store` / `!terraform.output`, or use wrong CLI flags. With skills,
the assistant loads the right Atmos context just before answering.

The same prompt -- *"set up cross-stack dependencies with remote state"* -- pulls live data
through MCP **and** applies Atmos-native patterns (`!terraform.state`, abstract components,
inheritance, [remote-state-bridge](../atmos-migration/references/remote-state-bridge.md))
from the relevant skill.

Install the Atmos skills plugin into Claude Code:

```bash
/plugin marketplace add cloudposse/atmos
/plugin install atmos@cloudposse
```

For Codex, Gemini, Cursor, Windsurf, GitHub Copilot, JetBrains Junie, and Amazon Q, see the
[AI Agent Skills announcement](https://atmos.tools/changelog/ai-agent-skills) for tool-specific
install paths.

## The Three Layers

A complete AI assistant setup typically uses three layers of MCP servers, each answering a
different question:

| Layer            | Server(s)                         | Answers                                 |
|------------------|-----------------------------------|-----------------------------------------|
| **Defined**      | `atmos` (Atmos MCP server)        | What's in the stacks, components, repo  |
| **Deployed**     | AWS MCP server suite (awslabs/*)  | What's live in the cloud right now      |
| **Over time**    | `atmos-pro` (Atmos Pro MCP)       | What changed, when, why, who, drift     |

Use this framing when helping users decide which servers to enable. Pure stack questions need
only the `atmos` server. Live-cloud questions need AWS servers. History/drift/deployment
questions need Atmos Pro.

## External MCP Server Configuration

Configure servers once in `atmos.yaml`. `atmos mcp export` writes them to per-CLI config files.

```yaml
toolchain:
  aliases:
    uv: astral-sh/uv                                      # Pin uvx via the Atmos toolchain

mcp:
  enabled: true
  servers:
    # Atmos's own MCP server — exposes describe/list/validate as tools
    atmos:
      command: atmos
      args: ["mcp", "start"]
      description: "Atmos AI tools — stacks, components, validation"

    # AWS MCP server suite — credentials injected via Atmos Auth
    aws-docs:
      command: uvx
      args: ["awslabs.aws-documentation-mcp-server@latest"]
      description: "AWS docs (public, no auth)"

    aws-billing:
      command: uvx
      args: ["awslabs.billing-cost-management-mcp-server@latest"]
      identity: readonly
      env: { AWS_REGION: us-east-1 }
      description: "AWS billing summaries"

    aws-iam:
      command: uvx
      args: ["awslabs.iam-mcp-server@latest"]
      identity: readonly
      description: "IAM role/policy analysis"
```

The canonical AWS server set (use `identity: readonly` via Atmos Auth for servers that
need AWS credentials; `aws-docs` is commonly no-auth):

| Server         | Purpose                                |
|----------------|----------------------------------------|
| aws-docs       | Search AWS documentation (no auth)     |
| aws-knowledge  | Managed AWS knowledge base (remote)    |
| aws-pricing    | Real-time pricing and cost analysis    |
| aws-billing    | Billing summaries and payment history  |
| aws-iam        | IAM role/policy analysis               |
| aws-cloudtrail | Event history and API call auditing    |
| aws-security   | Well-Architected security assessment   |
| aws-api        | Direct AWS CLI (read-only by default)  |

See `examples/mcp-for-ai-coding-assistants/atmos.yaml` for a working full configuration.

## Atmos Pro MCP Server (HTTP transport)

The Atmos Pro MCP server is **HTTP transport** (not stdio), runs at
`https://atmos-pro.com/mcp`, and is registered **separately** from `atmos mcp export`. Auth
is a one-time browser OAuth (GitHub); short-lived tokens land in the OS keychain.

Capabilities: drift detection, deployment history, workflow runs, failed-step logs, audit
log, repair recommendations, flapping detection.

Register it directly with each AI CLI:

```bash
# Claude Code
claude mcp add --transport http atmos-pro https://atmos-pro.com/mcp

# Gemini CLI
gemini mcp add --transport http atmos-pro https://atmos-pro.com/mcp
```

For Codex CLI, append to `~/.codex/config.toml`:

```toml
[mcp_servers.atmos-pro]
type = "http"
url = "https://atmos-pro.com/mcp"
```

## Auth and Profiles

For MCP servers that need cloud credentials, prefer Atmos Auth identities over ambient
`AWS_PROFILE` switching. When `identity` is set on an MCP server, Atmos injects isolated
credential files and profile env vars into that subprocess.

A common pattern is a single `readonly` identity (with `default: true`) used by every
AWS-querying MCP server, plus per-domain identities (e.g., `billing-auditor`,
`security-audit`) for servers that need different account access:

```yaml
auth:
  providers:
    sso:
      kind: aws/iam-identity-center
      start_url: "https://your-org.awsapps.com/start"
      region: us-east-1
  identities:
    readonly:
      kind: aws/permission-set
      default: true
      via: { provider: sso }
      principal:
        name: ReadOnlyAccess
        account: { id: "123456789012" }
```

For local use, authenticate once with the relevant identity:

```bash
atmos auth login readonly
atmos mcp test aws-security
```

For CLI providers, export `ATMOS_PROFILE` when the active profile defines the auth identities
or MCP servers:

```bash
export ATMOS_PROFILE=managers
atmos ai ask "What did we spend on EC2 last month?"
```

Do not add an `atmos auth login` step to non-interactive GitHub OIDC CI unless a specific
integration requires it. In CI, set `ATMOS_PROFILE` and let Atmos exchange the OIDC token when
the command runs.

## Multi-CLI MCP Export

`atmos mcp export` generates MCP client config from `mcp.servers`. The format adapts to the
client based on the output path or `--format` flag.

| Client       | Native config path                  | Format | Export command                                       |
|--------------|-------------------------------------|--------|------------------------------------------------------|
| Claude Code  | `.mcp.json` (project root)          | JSON   | `atmos mcp export`                                   |
| Gemini CLI   | `.gemini/settings.json`             | JSON   | `atmos mcp export --output .gemini/settings.json`    |
| Cursor       | `.cursor/mcp.json`                  | JSON   | `atmos mcp export --output .cursor/mcp.json`         |
| Codex CLI    | `~/.codex/config.toml`              | TOML   | `atmos mcp export --output ~/.codex/config.toml`     |

Claude Code and Gemini share the same JSON schema (`mcpServers` object). Codex uses TOML with
`[mcp_servers.<name>]` tables.

Exported configs preserve two critical behaviors:

- Servers with `identity` are wrapped as `atmos auth exec -i <identity> -- <command> ...`.
- The exported server `env.PATH` includes the Atmos toolchain PATH so tools like `uvx` and
  `npx` resolve even when the AI client does not inherit the user's shell environment.

Inspect and test exported configurations:

```bash
atmos mcp list
atmos mcp status
atmos mcp tools aws-docs
atmos mcp test aws-security
atmos mcp export
```

`atmos mcp restart <name>` validates that the server can stop and start during the command; do
not describe it as creating a long-running background service for stdio servers.

## Gemini Trusted Folders Gotcha

Gemini's Trusted Folders feature blocks MCP servers in untrusted directories. After
`atmos mcp export --output .gemini/settings.json`, the user must trust the folder once via
the Gemini UI/settings before the MCP servers will start. Symptom: servers configured
correctly but no tools available in Gemini.

## Related Examples

- `examples/mcp-for-ai-coding-assistants/` -- canonical full setup: Atmos MCP server + AWS
  server suite + Atmos Pro, exported to Claude Code / Codex / Gemini, AWS credentials via
  Atmos Auth.
- `examples/mcp/` -- external MCP server config when Atmos itself drives the AI loop
  (`atmos ai ask`) instead of an external CLI.
- `examples/ai-claude-code/` -- use a Claude Pro/Max subscription as Atmos's AI provider (no
  Anthropic API key). Atmos hosts the conversation; Claude Code provides the model.
- `examples/ai/` -- multi-provider Atmos AI setup (Anthropic API, OpenAI API, Ollama). No
  external CLI; chat with infrastructure from `atmos ai ask`.

## Guardrails

- Keep MCP server configuration in `atmos.yaml` so agents, IDEs, and CI share one source of truth.
- Pin MCP package versions when repeatability matters; avoid unreviewed `@latest` in production workflows.
- Use `atmos toolchain` for binaries that MCP servers need, then rely on export/toolchain PATH injection.
- Use `--identity=false`, `off`, `0`, or `no` only when deliberately disabling Atmos Auth for a command.
- The exported `.mcp.json` is safe to commit -- it contains no secrets (worst case: IAM role
  names). Credentials resolve at runtime via `atmos auth exec`.
- For `awslabs/*` MCP servers, prefer a single `readonly` identity by default and switch only
  for servers that genuinely need elevated access.

## Related Reading

- [MCP Configuration in Atmos](https://atmos.tools/cli/configuration/mcp)
- [Atmos Auth](https://atmos.tools/cli/configuration/auth)
- [Atmos Toolchain](https://atmos.tools/cli/configuration/toolchain)
- [Atmos MCP Server](https://atmos.tools/ai/mcp-server)
- [Atmos Agent Skills](https://atmos.tools/ai/agent-skills)
- [Atmos Pro MCP server install](https://atmos-pro.com/mcp/install)
- [AWS MCP servers (awslabs/mcp)](https://github.com/awslabs/mcp)
- Blog: [Configure MCPs once in Atmos, use it from Claude Code, Codex, and Gemini](https://atmos.tools/blog/mcp-for-ai-coding-assistants)
