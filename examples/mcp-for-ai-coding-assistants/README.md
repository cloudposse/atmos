# Example: MCP for AI Coding Assistants

Configure MCP servers (the [Atmos MCP server](/ai/mcp-server) plus the
[AWS MCP server suite](https://github.com/awslabs/mcp)) **once** in `atmos.yaml`, then use
the same set of tools — with the same AWS credentials managed by Atmos Auth — from
[Claude Code](https://www.anthropic.com/claude-code),
[OpenAI Codex CLI](https://github.com/openai/codex), and
[Google Gemini CLI](https://github.com/google-gemini/gemini-cli).

Learn more in the [MCP configuration documentation](/cli/configuration/mcp).

---

### Your AI knows your stacks and components. And your cloud. And your history.

In one prompt, your AI coding assistant answers:

- What's **configured** in your infrastructure
- What's **deployed** in your cloud accounts
- What **changed** — when, why, how, and by whom

Centralized auth. Centralized security and permissions. One `atmos.yaml`.
[Jump to the setup →](#one-time-setup)

---

## What This Example Demonstrates

- **One source of truth** — every MCP server is defined in `atmos.yaml`
  (versioned with your infrastructure code) instead of three separate
  per-CLI config files.
- **Security — every credential, in one place** —
  [Atmos Auth](/cli/configuration/auth) is the only place
  AWS credentials live. The exported `.mcp.json` references IAM role names
  but holds **no static secrets** — safe to check into the repo. Each
  external MCP server is spawned by `atmos auth exec`, which resolves
  credentials at runtime and writes them only into that subprocess's env.
  No `~/.aws/credentials`, no scattered `AWS_PROFILE`, no per-server
  authentication, no token files in each CLI's config directory.
- **Convenience — one login, every account auto-routed** — configure all
  the accounts you care about in `auth.identities`, run `atmos auth login`
  once, and Atmos handles the rest. Each MCP server is pinned to the
  account where its API actually works (billing → payer, CloudTrail → audit,
  IAM analysis → root, workload introspection → dev). When the AI calls a
  tool, Atmos automatically picks the right account for that tool's server.
  No identity juggling between prompts, no `AWS_PROFILE` swapping, no
  re-logins to ask a billing question after asking a VPC question.
- **Toolchain managed by Atmos** — `uvx` is installed and resolved via the
  [Atmos toolchain](/cli/configuration/toolchain) so every
  CLI uses the same binary version. No "works on my machine" drift.
- **Atmos's own AI tools exposed** — your AI assistant can call
  `describe_component`, `list_stacks`, `validate_stacks`, `read_stack_file`,
  `execute_atmos_command`, etc. alongside the AWS MCP tools.

## MCP Servers Configured

| Server             | Purpose                                                  | Transport | Auth                       |
|--------------------|----------------------------------------------------------|-----------|----------------------------|
| **atmos**          | Atmos AI tools (describe/list/validate, search)          | stdio     | Atmos Auth                 |
| **atmos-pro**      | Atmos Pro — drift, deployments, workflow runs, audit log | HTTP      | Browser OAuth (GitHub)     |
| **aws-docs**       | Search and fetch AWS documentation                       | stdio     | None (public docs)         |
| **aws-knowledge**  | Managed AWS knowledge base (remote)                      | stdio     | None (public)              |
| **aws-pricing**    | Real-time pricing and cost analysis                      | stdio     | AWS (via Atmos Auth)       |
| **aws-billing**    | Billing summaries and payment history                    | stdio     | AWS (via Atmos Auth)       |
| **aws-iam**        | IAM role/policy analysis (read-only)                     | stdio     | AWS (via Atmos Auth)       |
| **aws-cloudtrail** | Event history and API auditing                           | stdio     | AWS (via Atmos Auth)       |
| **aws-security**   | Well-Architected security posture assessment             | stdio     | AWS (via Atmos Auth)       |
| **aws-api**        | Direct AWS CLI access (read-only by default)             | stdio     | AWS (via Atmos Auth)       |

Atmos Auth injects AWS credentials into the AWS servers (`identity: readonly` in
`atmos.yaml` wraps them with `atmos auth exec -i readonly --`).

The **atmos-pro** server is HTTP-transport. `atmos.yaml`'s `mcp.servers`
currently only supports `stdio` servers, so `atmos-pro` is registered directly with each
AI CLI (see [Atmos Pro section](#atmos-pro-server-http-transport) below) rather
than through `atmos mcp export`. This may change in a future release.

## What the `atmos` MCP Server Does

The first entry in the table above — **atmos** — is *not* an `awslabs` server.
It's the [Atmos MCP server](/ai/mcp-server) running inside the
`atmos` binary itself, started by `atmos mcp start`. Including it in your AI
coding assistant's config gives the assistant direct programmatic access to
**your Atmos project** — your stacks, components, manifests, validation logic,
and the `atmos` CLI as a whole — alongside the AWS introspection tools.

The `atmos` server exposes more than 20 tools:
- Stack & component introspection (read-only, no credentials)
- File operations (read + write)
- Execution (`atmos` and `bash` commands)
- Security / compliance (introspection of finding data)

### Why have this in addition to the AWS MCP servers?

The AWS MCP servers tell the assistant what's **deployed** — IAM roles in
the account, EC2 spend last month, GuardDuty status. The Atmos MCP server
tells it what's **defined** — which stacks exist, what their declared
config is, what the dependency graph looks like, what would change if you
modified a variable.

### What requires permission

Tools marked **(requires permission)** above respect Atmos's
[tool permission model](/cli/configuration/ai/tools#permissions).
By default, the MCP server runs in YOLO mode (no interactive prompts —
the client handles approvals), so the AI coding assistant's own
permission UI (Claude Code's per-tool approval, Codex's confirm prompt,
Gemini's tool consent dialog) gates execution.

## What the `atmos-pro` MCP Server Does

[Atmos Pro](https://atmos-pro.com/) is the fastest way to deploy your apps
on AWS with Terraform and GitHub Actions. The
[Atmos Pro MCP server](https://atmos-pro.com/mcp/install) lets your AI
coding assistant query everything Atmos Pro knows about your workspace —
drift, deployments, workflow runs, audit log — **without leaving the
editor**. No dashboard switching, no copy-pasting URLs from GitHub
Actions logs.

Unlike the local servers above, the Atmos Pro MCP server runs on
`https://atmos-pro.com/mcp` over **HTTP transport**. Authentication is a
one-time browser-based OAuth flow (GitHub login); short-lived tokens land in
your OS keychain. No API keys, no static credentials, revocable from the
Atmos Pro UI under *Settings → MCP Clients*.

### Capabilities

- **Workflow & deployment analysis** — inspect workflow runs with approval
  states and job summaries; view deployment history linked to commits and
  pull requests; analyze logs from failed steps and understand failure
  patterns over time.
- **Infrastructure triage & diagnostics** — list/inspect drifted or errored
  instances, stacks, and components; access repair history and
  recommendations; retrieve structured failure explanations with suggested
  actions.
- **Historical context & trends** — identify when failures began and detect
  flapping patterns; compare workspace stability against previous periods;
  access the complete audit log of all infrastructure changes.
- **Security & access control** — audit every agent tool call with actor
  type, client name, and arguments; review permissions and permission
  errors; track audit history for compliance.

### Where it fits in the picture

The three layers complement each other:

> The **AWS servers** tell the assistant what is **deployed**.
> The **atmos** server tells it what is **defined**.
> The **atmos-pro** server tells it what is **happening over time** — drift,
> who/what changed it, why a run failed, when problems began.

So the assistant can answer questions like *"why did our vpc deployment fail
yesterday, what changed in the stack config, and which AWS resource is now
out of sync?"* in a single prompt — pulling deployment history from
`atmos-pro`, the declared stack config from `atmos` (or your local infra repo), and the live AWS state
from `aws-api`.

## Pair with Atmos Agent Skills

MCP servers give your AI coding assistant **tools** — to inspect your
stacks, query AWS, check Atmos Pro. [Atmos Agent Skills](/ai/agent-skills)
give it **knowledge** — domain-specific skills (stacks, components,
validation, YAML functions, vendoring, toolchain, GitOps, design patterns,
auth, …) that activate automatically based on what you ask.

Without skills, an AI assistant falls back to general training data that
may be outdated or incomplete for Atmos: it generates invalid YAML, misses
features like `!store` / `!terraform.output` / `!terraform.state`, or uses
the wrong CLI flags. With skills, the assistant loads the right Atmos
context just before answering — *"how do I set up cross-stack dependencies
with remote state?"* gets the real Atmos-native pattern, not a generic
Terraform workaround.

Skills are built on two open standards — [AGENTS.md](https://agents.md/)
and [Agent Skills](https://agentskills.io/specification) — so they work
across every AI coding assistant (Claude Code, Codex CLI,
Gemini CLI) plus Cursor, Windsurf, GitHub Copilot, and others.

**Claude Code (one command):**

```bash
/plugin marketplace add cloudposse/atmos
/plugin install atmos@cloudposse
```

**Other tools** (Codex CLI, Gemini CLI, Cursor, Windsurf, GitHub Copilot,
JetBrains Junie, …): see the
[AI Agent Skills announcement](/changelog/ai-agent-skills) for
each tool's install path, or the
[Atmos Agent Skills documentation](/ai/agent-skills) for
the full skill reference.

**MCP + Skills together** are stronger than either alone. MCP answers
*"what does this code do?"* by reading the actual files and querying
live state; skills answer *"what should this code do?"* by teaching the
assistant Atmos's conventions. The same prompt — e.g., *"validate every
stack and explain any errors"* — pulls live data through MCP **and**
applies Atmos-specific reasoning from the relevant skill.

## Prerequisites

1. **An AI coding assistant** installed and authenticated — pick at least one:
   - [Claude Code](https://docs.claude.com/en/docs/claude-code/setup):
     `brew install --cask claude-code` → `claude /login`
   - [OpenAI Codex CLI](https://github.com/openai/codex):
     `brew install codex` *or* `npm install -g @openai/codex` → `codex login`
   - [Google Gemini CLI](https://github.com/google-gemini/gemini-cli):
     `brew install gemini-cli` *or* `npm install -g @google/gemini-cli` → `gemini auth login`

2. **Python 3.10+** — required by the AWS MCP servers. `uvx` is installed by the Atmos
   toolchain.

3. **Atmos Auth** for AWS MCP servers. Edit the `auth` section of `atmos.yaml` with your
   SSO start URL, permission set, and account ID, then:
   ```bash
   atmos auth login
   ```

4. **[Atmos Pro](https://atmos-pro.com/) accounts and workspaces** for the
   `atmos-pro` MCP server. If you're not using Atmos Pro — the rest
   of the example works without it. If you are, the first time any AI CLI starts the
   `atmos-pro` server you'll be redirected to GitHub to authorize Atmos Pro. The token
   binds to a specific Atmos Pro workspace; switch workspaces by re-authorizing.

## One-Time Setup

```bash
cd examples/mcp-for-ai-coding-assistants

# Authenticate against your AWS organization via SSO.
atmos auth login

# Verify the MCP servers can start and report their tool counts.
atmos mcp status

# (Optional) Test one server end-to-end without involving an AI CLI.
atmos mcp test aws-docs
atmos mcp tools aws-docs
```

## Wiring the MCP Servers Into Your AI CLI

Each CLI has its own config file format and location. The cleanest workflow varies slightly:

### Option 1 — Claude Code

Claude Code reads MCP servers from a `.mcp.json` file in the project root. Atmos generates
this file natively:

```bash
# Generates .mcp.json in the current directory.
atmos mcp export
```

Then either start Claude Code from this directory:

```bash
cd examples/mcp-for-ai-coding-assistants
claude
```

Or register the servers globally with the `claude mcp add` command (one per server):

```bash
# Each AWS server is wrapped with `atmos auth exec -i readonly --` so credentials flow.
claude mcp add --transport stdio aws-pricing -- \
  atmos auth exec -i readonly -- uvx awslabs.aws-pricing-mcp-server@latest

# The Atmos MCP server doesn't need an identity.
claude mcp add --transport stdio atmos -- atmos mcp start
```

Manage with `claude mcp list`, `claude mcp get <name>`, `claude mcp remove <name>`.

Once registered, ask away:

```text
List all the IAM roles in this account that have admin access.
What did we spend on EC2 last month?
Audit our security posture against the Well-Architected framework.
```

Claude Code picks which MCP tools to call based on the question — you don't need to specify them.

### Option 2 — OpenAI Codex CLI

Codex CLI reads MCP servers from `~/.codex/config.toml` (TOML, not JSON).

#### Direct integration

Generate `.mcp.json` and translate to TOML in `~/.codex/config.toml`. For each server
in the JSON, add a `[mcp_servers.<name>]` block. Example (matches what this example's
`atmos.yaml` produces):

```toml
# ~/.codex/config.toml

[mcp_servers.atmos]
command = "atmos"
args = ["mcp", "start"]

[mcp_servers.aws-docs]
command = "uvx"
args = ["awslabs.aws-documentation-mcp-server@latest"]

[mcp_servers.aws-docs.env]
FASTMCP_LOG_LEVEL = "ERROR"

[mcp_servers.aws-pricing]
command = "atmos"
args = ["auth", "exec", "-i", "readonly", "--",
        "uvx", "awslabs.aws-pricing-mcp-server@latest"]

[mcp_servers.aws-pricing.env]
AWS_REGION = "us-east-1"
FASTMCP_LOG_LEVEL = "ERROR"

# … repeat for aws-billing, aws-iam, aws-cloudtrail, aws-security, aws-api
```

The `atmos auth exec -i readonly --` wrapper is what gives Codex the credentials — same
pattern Claude Code uses.

Then run `codex` and ask infrastructure questions just like you would in Claude Code.

#### Via `atmos ai exec` (no manual TOML)

If you'd rather skip the format translation, run Codex through Atmos:

```bash
atmos ai exec --provider codex-cli "List unused IAM roles in this account"
```

Atmos writes the TOML config for you (to a temp file or `~/.codex/config.toml`), spawns
Codex with the right args, and tears down afterward.

### Option 3 — Google Gemini CLI

Gemini CLI reads MCP servers from `.gemini/settings.json` (project) or
`~/.gemini/settings.json` (user). The format is structurally identical to `.mcp.json`,
so the cleanest workflow is to export directly there:

```bash
# Per-project (recommended — checked in with your atmos.yaml):
atmos mcp export --output .gemini/settings.json

# Or globally for your user:
atmos mcp export --output ~/.gemini/settings.json
```

Or use the `gemini mcp add` command per server:

```bash
gemini mcp add aws-pricing -- \
  atmos auth exec -i readonly -- uvx awslabs.aws-pricing-mcp-server@latest

gemini mcp add atmos -- atmos mcp start
```

Then start Gemini in this directory:

```bash
cd examples/mcp-for-ai-coding-assistants
gemini
```

:::tip
Gemini's [Trusted Folders feature](https://github.com/google-gemini/gemini-cli/blob/main/docs/trusted-folders.md)
blocks MCP servers in untrusted directories. Trust this folder once via the Gemini UI
or settings before the MCP servers will start.
:::

### Atmos Pro server (HTTP transport)

The [Atmos Pro MCP server](https://atmos-pro.com/mcp/install) runs at
`https://atmos-pro.com/mcp` over HTTP. This server is registered with each AI
CLI directly (not via `atmos mcp export`). The OAuth flow runs the first time
you start the server from any client — log in with GitHub, the token lands in
your OS keychain.

#### Claude Code

```bash
claude mcp add --transport http atmos-pro https://atmos-pro.com/mcp
```

Or add to `.mcp.json` / `~/.claude.json` directly:

```json
{
  "mcpServers": {
    "atmos-pro": {
      "type": "http",
      "url": "https://atmos-pro.com/mcp"
    }
  }
}
```

If you already have a `.mcp.json` from `atmos mcp export`, merge the entry
above into the existing `mcpServers` object.

#### Codex CLI

Append to `~/.codex/config.toml`:

```toml
[mcp_servers.atmos-pro]
type = "http"
url = "https://atmos-pro.com/mcp"
```

#### Gemini CLI

```bash
gemini mcp add --transport http atmos-pro https://atmos-pro.com/mcp
```

Or merge into `.gemini/settings.json` / `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "atmos-pro": {
      "type": "http",
      "url": "https://atmos-pro.com/mcp"
    }
  }
}
```

#### First-run OAuth

The first time any of these CLIs spawns the atmos-pro server, you'll be
redirected to GitHub to authorize Atmos Pro. The token is bound to a specific
workspace; switching workspaces re-prompts. Revoke any time from the Atmos Pro
UI under *Settings → MCP Clients*.

## What the Exported Config Looks Like

`atmos mcp export` produces a `.mcp.json` like this (truncated):

```json
{
  "mcpServers": {
    "atmos": {
      "command": "atmos",
      "args": ["mcp", "start"],
      "env": {
        "PATH": "/Users/you/.atmos/toolchain/...:..."
      }
    },
    "aws-pricing": {
      "command": "atmos",
      "args": [
        "auth", "exec", "-i", "readonly", "--",
        "uvx", "awslabs.aws-pricing-mcp-server@latest"
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
# Documentation lookup (no credentials needed)
"How do I configure S3 bucket lifecycle rules?"

# Cost analysis (uses aws-billing)
"What did we spend on EC2 across all accounts last month?"

# Security audit (uses aws-security + aws-iam + aws-api)
"Is GuardDuty enabled in all regions?"
"List all IAM roles with AdministratorAccess attached."

# Atmos introspection (uses the atmos MCP server)
# These require stacks/ and components/ — this example ships without them
# (so the focus stays on MCP). Run from a real Atmos project to see real
# answers; the prompts below illustrate the shape.
"What stacks are defined in this project?"
"Show me the vpc component config for dev."
"Validate every stack and report any errors."

# Atmos Pro — drift, deployments, history (uses atmos-pro)
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

## Switching Profiles or Identities

Need different credentials for different tasks? Atmos profiles handle this — see the
[profile documentation](/cli/configuration/profiles) for the full
flow. Quick version:

```bash
# Login with a specific profile, then re-export:
atmos --profile billing auth login
atmos --profile billing mcp export

# The AI CLI now uses billing-account credentials for every aws-* server.
```

## Related Examples

- **This example (`examples/mcp-for-ai-coding-assistants/`)** — You drive the AI
  assistant from **outside** Atmos (`claude`, `codex`, `gemini`) and want them
  to use Atmos-managed MCP servers with centralized auth.

- **[Atmos MCP integrations](/examples/mcp)** — You drive the
  AI loop **through Atmos** (`atmos ai ask`, `atmos ai chat`, `atmos ai exec`)
  and want it to call external MCP servers. Atmos hosts the AI conversation;
  the AWS MCP suite is configured the same way as this example, but you stay
  inside the `atmos` CLI.

- **[Atmos AI with Claude Code](/examples/ai-claude-code)** —
  You want to use your Claude Pro/Max subscription as the AI provider for
  `atmos ai ask` (no Anthropic API key needed). Atmos hosts the conversation;
  Claude Code provides the model. MCP servers are passed through to Claude Code
  automatically.

- **[Atmos AI (multi-provider)](/examples/ai)** — You want to
  chat with your infrastructure using API-key providers (Anthropic, OpenAI,
  Ollama, …). Multi-provider Atmos AI setup, no external CLI needed.

## Key Files

| File             | Purpose                                                                                       |
|------------------|-----------------------------------------------------------------------------------------------|
| `atmos.yaml`     | Toolchain, MCP servers, Atmos Auth, and AI configuration                                      |
| `.tool-versions` | Pins `uvx` to a known version so the exported `.mcp.json` carries a consistent toolchain PATH |

## Learn More

- [MCP Configuration](/cli/configuration/mcp)
- [Atmos Auth Documentation](/cli/configuration/auth)
- [Atmos Toolchain](/cli/configuration/toolchain)
- [Atmos MCP Server](/ai/mcp-server)
- [Atmos Agent Skills](/ai/agent-skills) — 21 domain-specific skills that pair with MCP tools to give AI assistants deep Atmos knowledge
- [Atmos Agent Skills announcement](/changelog/ai-agent-skills) — install paths for Claude Code, Codex, Gemini, Cursor, Windsurf, Copilot
- [Atmos Pro](https://atmos-pro.com/) — the fastest way to deploy your apps on AWS with Terraform and GitHub Actions
- [Atmos Pro MCP server install](https://atmos-pro.com/mcp/install)
- [Atmos Pro MCP server announcement](https://atmos-pro.com/changelog/2026-05-09-mcp-server)
- [AWS MCP Servers (awslabs/mcp)](https://github.com/awslabs/mcp)
