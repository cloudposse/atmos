# Atmos AI Assistant Example

This example demonstrates how to configure and use the Atmos AI Assistant features:

- **Multi-provider configuration** - Configure multiple AI providers (Anthropic, OpenAI, Ollama)
- **Session management** - Persistent conversation history with auto-compact
- **Tool execution** - AI-powered infrastructure inspection
- **Custom skills** - Specialized AI skills for different tasks
- **Project memory** - Persistent context via ATMOS.md

## Prerequisites

Set up at least one AI provider API key:

```bash
# Choose one or more providers
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export OPENAI_API_KEY="your-openai-api-key"
# Ollama requires no API key (runs locally)
```

## Quick Start

1. Navigate to this example directory:

   ```bash
   cd examples/ai
   ```

2. Start an interactive AI chat:

   ```bash
   atmos ai chat
   ```

3. Or ask a single question:

   ```bash
   atmos ai ask "What stacks are available in this project?"
   ```

## Features Demonstrated

### Multi-Provider Configuration

This example configures three AI providers in `atmos.yaml`:

```yaml
settings:
  ai:
    enabled: true
    default_provider: "anthropic"
    providers:
      anthropic:
        model: "claude-sonnet-4-5-20250929"
      openai:
        model: "gpt-4o"
      ollama:
        model: "llama3.3:70b"
```

Switch between providers during a chat session by pressing `Ctrl+P`.

### Session Management

Sessions persist conversation history across CLI invocations:

```bash
# Start a named session
atmos ai chat --session vpc-refactor

# Resume the same session later
atmos ai chat --session vpc-refactor

# List all sessions
atmos ai sessions
```

Auto-compact is enabled to intelligently summarize old messages, preserving context while managing history size.

### Tool Execution

The AI can inspect your infrastructure using read-only tools:

- `atmos_describe_component` - Describe component configuration
- `atmos_list_stacks` - List available stacks
- `atmos_validate_stacks` - Validate stack configurations

Example conversation:

```
You: Describe the myapp component in the dev stack
AI: [Uses atmos_describe_component tool]
    The myapp component in dev has the following configuration:
    - Environment: development
    - Instance type: t3.small
    - Replica count: 1
    ...
```

### Custom Skills

This example includes a custom "cost-optimizer" skill:

```bash
# Press Ctrl+A during chat to switch skills
# Or set default skill in atmos.yaml
```

Skills provide specialized behavior and restricted tool access for specific tasks.

### Project Memory (ATMOS.md)

The `ATMOS.md` file provides persistent project context to the AI:

- Stack naming conventions
- Component patterns
- Team preferences
- Common operations

The AI reads this file automatically to provide context-aware responses.

## Directory Structure

```
examples/ai/
├── README.md              # This file
├── atmos.yaml             # Atmos configuration with AI settings
├── ATMOS.md               # Project memory for AI context
├── stacks/
│   └── deploy/
│       ├── dev.yaml       # Development stack
│       └── prod.yaml      # Production stack
├── components/
│   └── terraform/
│       └── myapp/
│           ├── main.tf    # Sample Terraform component
│           ├── variables.tf
│           └── outputs.tf
└── workflows/
    └── ai-demo.yaml       # Workflow demonstrating AI usage
```

## Example Commands

```bash
# Interactive chat
atmos ai chat

# Single question
atmos ai ask "List all stacks"

# Question with context
atmos ai ask "What components are defined in the dev stack?"

# Named session
atmos ai chat --session infrastructure-review

# List sessions
atmos ai sessions

# Run workflow
atmos workflow ai-demo
```

## Provider-Specific Notes

### Anthropic (Claude)

- Best for complex reasoning and code generation
- Requires `ANTHROPIC_API_KEY`
- Token caching enabled by default for cost savings

### OpenAI (GPT)

- Strong general-purpose capabilities
- Requires `OPENAI_API_KEY`
- Automatic prompt caching

### Ollama (Local)

- 100% local processing, no data leaves your machine
- Requires Ollama installed and running: `ollama serve`
- Pull a model first: `ollama pull llama3.3:70b`

## Keyboard Shortcuts (in chat)

| Key           | Action             |
|---------------|--------------------|
| `Ctrl+P`      | Switch AI provider |
| `Ctrl+A`      | Switch AI skill    |
| `Ctrl+N`      | Create new session |
| `Ctrl+L`      | List sessions      |
| `Ctrl+C`      | Exit chat          |
| `Enter`       | Send message       |
| `Shift+Enter` | New line           |

## Learn More

- [AI Assistant Documentation](https://atmos.tools/ai/)
- [AI Configuration](https://atmos.tools/ai/configuration)
- [AI Providers](https://atmos.tools/ai/providers)
- [AI Skills](https://atmos.tools/ai/skills)
- [Session Management](https://atmos.tools/ai/sessions)
