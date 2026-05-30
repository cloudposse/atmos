# Example: Atmos AI

Use AI to chat with your infrastructure, inspect stacks and components, and analyze command output.

Learn more in the [Atmos AI documentation](https://atmos.tools/ai).

> This example uses mock Terraform components — no cloud credentials required.

## What You'll See

- [Multi-provider AI configuration](https://atmos.tools/cli/configuration/ai/providers) (Anthropic, OpenAI, Gemini, Ollama)
- [Interactive chat](https://atmos.tools/cli/commands/ai/chat) and single-question modes
- [Tool execution](https://atmos.tools/ai) — AI inspects stacks and components
- [Global `--ai` flag](https://atmos.tools/cli/global-flags) — AI analysis of any command output

## Try It

```shell
cd examples/ai

# Set up at least one provider API key
export ANTHROPIC_API_KEY="your-api-key"

# Ask a single question
atmos ai ask "What stacks and components do we have?"

# Interactive chat
atmos ai chat

# AI-powered analysis of any command
atmos terraform plan vpc -s dev --ai
```

## Related Examples

- **[AI with Claude Code CLI](../ai-claude-code/)** — Use your Claude Pro/Max subscription
  instead of API tokens, with MCP server pass-through for AWS tools.

## Key Files

| File                    | Purpose                                          |
|-------------------------|--------------------------------------------------|
| `atmos.yaml`            | Atmos configuration with AI provider settings    |
| `ATMOS.md`              | Project instructions the AI reads automatically  |
| `stacks/`               | Stack configuration files                        |
| `components/terraform/` | Mock Terraform components (VPC, Transit Gateway) |
