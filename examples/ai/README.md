# Atmos AI Assistant Example

This example demonstrates how to configure and use the Atmos AI Assistant with a realistic multi-region AWS
infrastructure project.

- **Multi-provider configuration** — Configure multiple AI providers (Anthropic, OpenAI, Gemini, Ollama)
- **Multi-region infrastructure** — Hub-spoke Transit Gateway topology across us-east-1 and us-west-2
- **Session management** — Persistent conversation history with auto-compact
- **Tool execution** — AI-powered infrastructure inspection with component dependencies
- **Custom skills** — Specialized AI skills for cost optimization and security review
- **Project instructions** — Persistent context via ATMOS.md

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
   atmos ai ask "What stacks and components do we have in the infra?"
   ```

## See It in Action

Ask a question about your infrastructure, and Atmos AI automatically inspects your stacks, components, and configuration:

```text
$ atmos ai ask "what stacks and components do we have in the infra?"

👽 Thinking...

  I'll list all the available stacks and components in your infrastructure.

  Based on the infrastructure configuration, here are the stacks and components:

  ## Stacks (4 total)

  1. **ue1-network** - Network stack in us-east-1
  2. **ue1-prod** - Production stack in us-east-1
  3. **uw2-network** - Network stack in us-west-2
  4. **uw2-prod** - Production stack in us-west-2

  ## Components by Stack

  ### ue1-network

  • tgw/attachment - Transit Gateway attachment
  • tgw/hub - Transit Gateway hub
  • vpc - Virtual Private Cloud

  ### ue1-prod

  • tgw/attachment - Transit Gateway attachment
  • vpc - Virtual Private Cloud

  ### uw2-network

  • tgw/attachment - Transit Gateway attachment
  • tgw/cross-region-hub-connector - Cross-region Transit Gateway connector
  • vpc - Virtual Private Cloud

  ### uw2-prod

  • tgw/attachment - Transit Gateway attachment
  • vpc - Virtual Private Cloud

  ## Summary

  Your infrastructure is organized across two AWS regions (us-east-1 and us-west-2),
  with dedicated network and production stacks in each region. The network stacks
  contain Transit Gateway hubs for network connectivity, while the production stacks
  use Transit Gateway attachments to connect to the network infrastructure. All stacks
  include VPC components for network isolation.
```

### Validate Stacks

Ask the AI to validate all stacks and present the results in a table:

```text
$ atmos ai ask "validate all stacks, show issues in a table"

👽 Thinking...

  I'll validate all Atmos stacks right away!

  Here are the validation results for all stacks:

   # | Check                                | Status    | Details
  ---|--------------------------------------|-----------|---------------------------------------
   1 | Stack Schema Validation (jsonschema) | ✅ Passed | All stacks conform to the JSON schema

  --------

  **Summary:** All stacks passed validation with **0 issues found**. Your Atmos stack
  configurations are valid and well-formed. 🎉

  If you'd like deeper validation, I can also:

  • 🔍 **Validate with OPA policies** (opa schema type) for policy-as-code checks
  • 📋 **List all stacks and components** to review configurations manually
  • 🔄 **Check affected components** based on recent git changes
```

### Automation with JSON Output

Use `atmos ai exec` for scripting and CI/CD pipelines. The `--format json` flag returns structured output
with tool call details, token usage, and metadata:

```text
$ atmos ai exec "validate stacks" --format json
{
  "success": true,
  "response": "I'll validate your Atmos stack configurations right away!\n\n✅ **Stack Validation Passed!**\n\nAll Atmos stack configurations are valid. The stacks were validated against the **JSON Schema** (`jsonschema`) and no issues were found.\n\nYour stack configurations are well-formed and ready to use",
  "tool_calls": [
    {
      "tool": "atmos_validate_stacks",
      "duration_ms": 15,
      "success": true,
      "result": {
        "schema_type": "jsonschema"
      }
    }
  ],
  "tokens": {
    "prompt": 7077,
    "completion": 188,
    "total": 7265
  },
  "metadata": {
    "model": "claude-sonnet-4-6",
    "provider": "anthropic",
    "duration_ms": 5852,
    "timestamp": "2026-03-07T22:36:24.167201-05:00",
    "tools_enabled": true,
    "stop_reason": "end_turn"
  }
}
```

## Features Demonstrated

### Multi-Region Hub-Spoke Architecture

This example models a real-world multi-region AWS networking setup:

- **us-east-1 (hub)** — Transit Gateway hub with VPC and attachments
- **us-west-2 (spoke)** — Cross-region connector peering back to the hub
- **Component dependencies** — Attachments depend on VPCs and hubs across stacks
- **Mixins** — Region and stage configuration reused via imports

### Multi-Provider Configuration

Four AI providers are configured in `atmos.yaml`:

```yaml
settings:
  ai:
    enabled: true
    default_provider: "anthropic"
    providers:
      anthropic:
        model: "claude-sonnet-4-6"
      openai:
        model: "gpt-5.4"
      gemini:
        model: "gemini-2.5-flash"
      ollama:
        model: "llama4"
```

Switch between providers during a chat session by pressing `Ctrl+P`.

### Tool Execution

The AI can inspect your infrastructure using built-in tools:

- `atmos_describe_component` — Describe component configuration
- `atmos_list_stacks` — List available stacks
- `atmos_validate_stacks` — Validate stack configurations

Example conversation:

```text
You: Describe the VPC component in the ue1-network stack
AI: [Uses atmos_describe_component tool]
    The VPC in ue1-network uses CIDR 10.1.0.0/16 with 3 availability zones
    (us-east-1a, us-east-1b, us-east-1c) and NAT Gateways enabled...
```

### Custom Skills

This example includes custom "cost-optimizer" and "security-reviewer" skills:

```bash
# Press Ctrl+A during chat to switch skills
# Or set default skill in atmos.yaml
```

Skills provide specialized behavior and restricted tool access for specific tasks.

### Project Instructions (ATMOS.md)

The `ATMOS.md` file provides persistent project instructions to the AI:

- Architecture overview (hub-spoke topology)
- Stack naming conventions (`ue1-network`, `uw2-prod`, etc.)
- Component descriptions and dependencies
- Common operations

The AI reads this file automatically to provide context-aware responses.

## Directory Structure

```text
examples/ai/
├── README.md                                     # This file
├── atmos.yaml                                    # Atmos configuration with AI settings
├── ATMOS.md                                      # Project instructions for AI context
├── stacks/
│   ├── deploy/
│   │   ├── network/
│   │   │   ├── us-east-1.yaml                    # Network stack (hub region)
│   │   │   └── us-west-2.yaml                    # Network stack (spoke region)
│   │   └── prod/
│   │       ├── us-east-1.yaml                    # Production stack
│   │       └── us-west-2.yaml                    # Production stack
│   └── mixins/
│       ├── region/
│       │   ├── us-east-1.yaml                    # Region: ue1
│       │   └── us-west-2.yaml                    # Region: uw2
│       └── stage/
│           ├── network.yaml                      # Stage: network
│           └── prod.yaml                         # Stage: prod
├── components/
│   └── terraform/
│       ├── vpc/                                  # VPC component
│       │   ├── main.tf
│       │   ├── variables.tf
│       │   └── outputs.tf
│       └── tgw/
│           ├── hub/                              # Transit Gateway hub
│           │   ├── main.tf
│           │   ├── variables.tf
│           │   └── outputs.tf
│           ├── attachment/                       # Transit Gateway attachment
│           │   ├── main.tf
│           │   ├── variables.tf
│           │   └── outputs.tf
│           └── cross-region-hub-connector/       # Cross-region peering
│               ├── main.tf
│               ├── variables.tf
│               └── outputs.tf
└── workflows/
    └── ai-demo.yaml                             # Workflow demonstrating AI usage
```

## Example Commands

```bash
# Interactive chat
atmos ai chat

# Single question
atmos ai ask "List all stacks"

# Describe a component
atmos ai ask "Describe the VPC in ue1-network"

# Analyze dependencies
atmos ai ask "What are the component dependencies in ue1-network?"

# Named session
atmos ai chat --session infrastructure-review

# List sessions
atmos ai sessions list

# Run workflow
atmos workflow ai-demo
```

## Provider-Specific Notes

### Anthropic (Claude)

- Best for complex reasoning and infrastructure analysis
- Requires `ANTHROPIC_API_KEY`
- Token caching enabled by default for cost savings

### OpenAI (GPT)

- Strong general-purpose capabilities
- Requires `OPENAI_API_KEY`
- Automatic prompt caching

### Ollama (Local)

- 100% local processing, no data leaves your machine
- Requires Ollama installed and running: `ollama serve`
- Pull a model first: `ollama pull llama4`

## Keyboard Shortcuts (in chat)

| Key      | Action             |
|----------|--------------------|
| `Ctrl+P` | Switch AI provider |
| `Ctrl+A` | Switch AI skill    |
| `Ctrl+N` | Create new session |
| `Ctrl+L` | List sessions      |
| `Ctrl+C` | Exit chat          |
| `Enter`  | Send message       |
| `Ctrl+J` | New line           |

## Learn More

- [AI Assistant Documentation](https://atmos.tools/ai/)
- [AI Configuration](https://atmos.tools/ai/configuration)
- [AI Providers](https://atmos.tools/ai/providers)
- [AI Skills](https://atmos.tools/ai/skills)
- [Session Management](https://atmos.tools/ai/sessions)
