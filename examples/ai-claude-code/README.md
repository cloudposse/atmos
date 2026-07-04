# Example: AI with Claude Code CLI

Use your Claude Pro/Max subscription instead of API tokens. Claude Code manages the AI
conversation — Atmos provides MCP server orchestration with automatic AWS credential injection.

Learn more in the [Atmos AI documentation](https://atmos.tools/ai).

## What You'll See

- [Claude Code as an AI provider](https://atmos.tools/cli/configuration/ai/providers#cli-providers) — no API key needed
- [MCP pass-through](https://atmos.tools/cli/configuration/mcp) — AWS MCP servers passed to Claude Code automatically
- [Atmos Auth](https://atmos.tools/cli/configuration/auth) — SSO credential injection for MCP servers

## Prerequisites

1. **Claude Code** installed and authenticated:
   ```bash
   brew install --cask claude-code
   claude auth login
   ```

2. **Atmos Auth** configured for AWS MCP servers that need credentials.
   Update the `auth` section in `atmos.yaml` with your SSO start URL, permission set,
   and account ID, then run:
   ```bash
   atmos auth login
   ```
   See the [Atmos Auth documentation](https://atmos.tools/cli/configuration/auth) for setup details.

## Try It

```shell
cd examples/ai-claude-code

# Simple question (no MCP needed)
atmos ai ask "What stacks do we have?"

# Uses aws-docs MCP server (no credentials needed)
atmos ai ask "Search AWS docs for VPC peering"

# Uses aws-billing MCP server (requires Atmos Auth)
atmos ai ask "What did we spend on EC2 last month?"
```

## Related Examples

- **[AI with API Providers](../ai/)** — Use API tokens (Anthropic, OpenAI, etc.)
  instead of a CLI subscription.

## Key Files

| File                        | Purpose                                          |
|-----------------------------|--------------------------------------------------|
| `atmos.yaml`                | AI provider, MCP servers, and auth configuration |
| `stacks/`                   | Minimal stack configuration                      |
| `components/terraform/vpc/` | Mock VPC component                               |
