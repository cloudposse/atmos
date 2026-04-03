# Example: AWS Security & Compliance

Analyze AWS security findings, map them to Atmos components and stacks, and get
AI-powered remediation guidance.

Learn more in the [AWS Security documentation](https://atmos.tools/cli/commands/aws/security).

> This example requires AWS credentials with Security Hub access.
> Update the `auth` section in `atmos.yaml` with your SSO settings.

## What You'll See

- [Security finding analysis](https://atmos.tools/cli/commands/aws/security) mapped to Atmos components
- [Compliance reporting](https://atmos.tools/cli/commands/aws/compliance) against CIS, PCI-DSS, SOC2
- [Atmos Auth](https://atmos.tools/cli/configuration/auth) for automatic AWS credential injection
- Optional AI remediation via `--ai` flag (root cause, code changes, deploy commands)

## Prerequisites

1. **Atmos Auth** — update `auth` in `atmos.yaml` with your SSO start URL, permission set,
   and security account ID, then authenticate:
   ```bash
   export ATMOS_PROFILE=devops  # or your profile name
   atmos auth login
   ```

2. **AI provider** (optional, for `--ai`):
   ```bash
   export ANTHROPIC_API_KEY="your-api-key"
   ```

## Try It

```shell
cd examples/aws-security-compliance

# Analyze security findings
atmos aws security analyze

# Filter by severity or source
atmos aws security analyze --severity critical,high --source guardduty

# Compliance report
atmos aws compliance report --framework cis-aws

# AI-powered remediation
atmos aws security analyze --ai

# JSON output for automation
atmos aws security analyze --format json --file findings.json
```

## Related Examples

- **[AI with API Providers](../ai/)** — Multi-provider AI configuration with sessions and tools.
- **[AI with Claude Code CLI](../ai-claude-code/)** — Use your Claude subscription with MCP server pass-through.
- **[MCP Server Integrations](../mcp/)** — Connect to AWS MCP servers for billing, IAM, and documentation.

## Key Files

| File         | Purpose                                         |
|--------------|-------------------------------------------------|
| `atmos.yaml` | Security config, auth, AI provider, tag mapping |
