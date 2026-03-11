# Example: Atmos AI

Configure and use the Atmos AI Assistant with mock components — no cloud credentials required.

Learn more in the [Atmos AI documentation](https://atmos.tools/ai).

## What You'll See

- [Multi-provider AI configuration](https://atmos.tools/cli/configuration/ai/providers) (Anthropic, OpenAI, Gemini, Ollama)
- [Interactive chat](https://atmos.tools/cli/commands/ai/chat) and single-question modes
- [Session management](https://atmos.tools/cli/configuration/ai/sessions) with persistent conversation history
- [Tool execution](https://atmos.tools/ai) — AI inspects stacks, components, and dependencies
- [Project instructions](https://atmos.tools/ai) via `ATMOS.md` for context-aware responses

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
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Atmos configuration with AI provider settings |
| `ATMOS.md` | Project instructions the AI reads automatically |
| `stacks/deploy/` | Environment-specific stack files |
| `stacks/mixins/` | Shared region and stage configuration |
| `components/terraform/` | Mock Terraform components (VPC, Transit Gateway) |
| `workflows/ai-demo.yaml` | Workflow demonstrating AI usage |
