---
name: atmos-ai
description: "Atmos AI and MCP: AI providers, AI command analysis, MCP server/client configuration, CLI provider pass-through, toolchain-aware MCP export, auth-wrapped tools"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos AI and MCP

## Purpose

Use this skill when configuring or troubleshooting Atmos AI, AI-powered command analysis,
AI providers, or MCP integrations. Atmos AI can call configured providers directly, use
local CLI providers, expose Atmos tools through MCP, and connect to external MCP servers.

For project discovery, resolved stack/component configuration, provenance, query filters,
or affected-instance analysis, use the `atmos-introspection` skill. For Terraform multi-instance
execution with `--affected`, `--all`, or `--query`, use the `atmos-terraform` or `atmos-ci` skill.

## AI Commands

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

## MCP Modes

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

For external MCP servers, configure the server once in `atmos.yaml`:

```yaml
toolchain:
  aliases:
    uv: astral-sh/uv

mcp:
  servers:
    aws-docs:
      command: uvx
      args: ["awslabs.aws-documentation-mcp-server@latest"]
      description: "AWS documentation search"

    aws-security:
      command: uvx
      args: ["awslabs.well-architected-security-mcp-server@latest"]
      description: "AWS security posture"
      identity: security-audit
      env:
        AWS_REGION: us-east-1
```

## Auth and Profiles

For MCP servers that need cloud credentials, prefer Atmos Auth identities over ambient
`AWS_PROFILE` switching. When `identity` is set on an MCP server, Atmos injects isolated
credential files and profile env vars into that subprocess.

For local use, authenticate once with the relevant identity:

```bash
atmos auth login security-audit
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

## Toolchain and MCP Export

Use `atmos mcp export` to generate MCP client config from `mcp.servers`:

```bash
atmos mcp export
atmos mcp export --output .cursor/mcp.json
atmos mcp export --output .gemini/settings.json
```

Exported MCP configs should preserve two critical behaviors:

- Servers with `identity` are wrapped as `atmos auth exec -i <identity> -- <command> ...`.
- The exported server `env.PATH` includes the Atmos toolchain PATH so tools like `uvx` and
  `npx` resolve even when the AI client does not inherit the user's shell environment.

Use these commands to inspect external MCP server configuration and available tools:

```bash
atmos mcp list
atmos mcp status
atmos mcp tools aws-docs
atmos mcp test aws-security
atmos mcp export
```

`atmos mcp restart <name>` validates that the server can stop and start during the command; do
not describe it as creating a long-running background service for stdio servers.

## Guardrails

- Keep MCP server configuration in `atmos.yaml` so agents, IDEs, and CI share one source of truth.
- Pin MCP package versions when repeatability matters; avoid unreviewed `@latest` in production workflows.
- Use `atmos toolchain` for binaries that MCP servers need, then rely on export/toolchain PATH injection.
- Use `--identity=false`, `off`, `0`, or `no` only when deliberately disabling Atmos Auth for a command.
