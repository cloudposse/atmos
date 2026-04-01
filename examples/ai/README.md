# Example: Atmos AI

Configure and use the Atmos AI Assistant with mock components — no cloud credentials required.

Learn more in the [Atmos AI documentation](https://atmos.tools/ai).

## What You'll See

- [Multi-provider AI configuration](https://atmos.tools/cli/configuration/ai/providers) (OpenAI, Anthropic, Bedrock, Azure OpenAI, Gemini, Grok, Ollama)
- [Interactive chat](https://atmos.tools/cli/commands/ai/chat) and single-question modes
- [Session management](https://atmos.tools/cli/configuration/ai/sessions) with persistent conversation history
- [Tool execution](https://atmos.tools/ai) — AI inspects stacks, components, and dependencies
- [Project instructions](https://atmos.tools/ai) via `ATMOS.md` for context-aware responses
- [Global `--ai` flag](https://atmos.tools/cli/global-flags) — AI-powered analysis of any command output
- [`--skill` flag](https://atmos.tools/cli/global-flags) — Domain-specific AI analysis with skills

## Try It

```shell
cd examples/ai

# Set up at least one provider API key
export ANTHROPIC_API_KEY="your-api-key"

# Interactive chat
atmos ai chat

# Ask a single question
atmos ai ask "What stacks and components do we have?"

# Structured output for CI/CD
atmos ai exec "validate stacks" --format json

# AI-powered analysis of any command
atmos terraform plan vpc -s ue1-network --ai

# With domain-specific skill
atmos terraform plan vpc -s ue1-prod --ai --skill atmos-terraform

# Multiple skills (comma-separated)
atmos terraform plan vpc -s ue1-prod --ai --skill atmos-terraform,atmos-stacks

# Multiple skills (repeated flag)
atmos terraform plan vpc -s ue1-prod --ai --skill atmos-terraform --skill atmos-stacks

# Via environment variables
ATMOS_AI=true ATMOS_SKILL=atmos-terraform,atmos-stacks atmos terraform plan vpc -s ue1-prod
```

## Related Examples

- **[AI with Claude Code CLI Provider](../ai-claude-code/)** — Use your Claude Pro/Max
  subscription instead of API tokens. Includes MCP server pass-through for AWS tools.
- **[MCP Server Integrations](../mcp/)** — Connect Atmos to external AWS MCP servers
  for billing, security, IAM, and documentation queries.

## Key Files

| File                     | Purpose                                          |
|--------------------------|--------------------------------------------------|
| `atmos.yaml`             | Atmos configuration with AI provider settings    |
| `ATMOS.md`               | Project instructions the AI reads automatically  |
| `stacks/deploy/`         | Environment-specific stack files                 |
| `stacks/mixins/`         | Shared region and stage configuration            |
| `components/terraform/`  | Mock Terraform components (VPC, Transit Gateway) |
| `workflows/ai-demo.yaml` | Workflow demonstrating AI usage                  |
