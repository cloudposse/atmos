# Atmos AI Assistant

The Atmos AI Assistant is an AI-powered terminal agent that helps with Atmos infrastructure management. It provides intelligent assistance with understanding Atmos concepts, analyzing configurations, troubleshooting issues, and learning best practices.

## Features

- **Interactive Chat Interface**: Terminal-based chat session for extended conversations
- **Command-line Q&A**: Ask questions directly from the command line
- **Context-aware Help**: Get help on specific Atmos topics
- **Configuration Analysis**: The AI has knowledge of your Atmos setup
- **Best Practice Guidance**: Receive recommendations for optimal configurations

## Configuration

To enable AI features, add the following to your `atmos.yaml` configuration:

### Anthropic (Claude)

```yaml
settings:
  ai:
    enabled: true                          # Enable AI features (default: false)
    provider: "anthropic"                  # AI provider (default: "anthropic")
    model: "claude-3-5-sonnet-20241022"    # Model to use (default: claude-3-5-sonnet-20241022)
    api_key_env: "ANTHROPIC_API_KEY"       # Environment variable for API key (default: ANTHROPIC_API_KEY)
    max_tokens: 4096                       # Maximum tokens per response (default: 4096)
```

### OpenAI (GPT)

```yaml
settings:
  ai:
    enabled: true                # Enable AI features (default: false)
    provider: "openai"           # AI provider
    model: "gpt-4o"              # Model to use (default: gpt-4o)
    api_key_env: "OPENAI_API_KEY" # Environment variable for API key (default: OPENAI_API_KEY)
    max_tokens: 4096             # Maximum tokens per response (default: 4096)
```

### Google (Gemini)

```yaml
settings:
  ai:
    enabled: true                    # Enable AI features (default: false)
    provider: "gemini"               # AI provider
    model: "gemini-2.0-flash-exp"    # Model to use (default: gemini-2.0-flash-exp)
    api_key_env: "GEMINI_API_KEY"    # Environment variable for API key (default: GEMINI_API_KEY)
    max_tokens: 8192                 # Maximum tokens per response (default: 8192)
```

### xAI (Grok)

```yaml
settings:
  ai:
    enabled: true                # Enable AI features (default: false)
    provider: "grok"             # AI provider
    model: "grok-beta"           # Model to use (default: grok-beta)
    api_key_env: "XAI_API_KEY"   # Environment variable for API key (default: XAI_API_KEY)
    max_tokens: 4096             # Maximum tokens per response (default: 4096)
    base_url: "https://api.x.ai/v1" # API endpoint (default: https://api.x.ai/v1)
```

### Environment Setup

**For Anthropic (Claude):**

1. **Get an Anthropic API Key**: Sign up at [https://console.anthropic.com/](https://console.anthropic.com/) and create an API key.

2. **Set the Environment Variable**:
   ```bash
   export ANTHROPIC_API_KEY="your-api-key-here"
   ```

**For OpenAI (GPT):**

1. **Get an OpenAI API Key**: Sign up at [https://platform.openai.com/](https://platform.openai.com/) and create an API key.

2. **Set the Environment Variable**:
   ```bash
   export OPENAI_API_KEY="your-api-key-here"
   ```

**For Google (Gemini):**

1. **Get a Google AI API Key**: Sign up at [https://aistudio.google.com/](https://aistudio.google.com/) and create an API key.

2. **Set the Environment Variable**:
   ```bash
   export GEMINI_API_KEY="your-api-key-here"
   ```

**For xAI (Grok):**

1. **Get an xAI API Key**: Sign up at [https://x.ai/api](https://x.ai/api) and create an API key.

2. **Set the Environment Variable**:
   ```bash
   export XAI_API_KEY="your-api-key-here"
   ```

**Verify Configuration:**
```bash
atmos ai ask "Hello, are you working?"
```

## Usage

### Interactive Chat

Start an interactive chat session:

```bash
atmos ai chat
```

This opens a terminal-based chat interface where you can:
- Ask questions about your Atmos configuration
- Get help with troubleshooting
- Learn about Atmos concepts
- Receive step-by-step guidance

**Example conversation:**
```
You: What components are available in my configuration?
Atmos AI: Based on your configuration, I can help you list components...

You: How do I validate my stack configuration?
Atmos AI: You can validate stack configurations using the `atmos validate stacks` command...
```

### Direct Questions

Ask questions directly from the command line:

```bash
# General questions
atmos ai ask "What are Atmos stacks?"
atmos ai ask "How do I organize my components?"

# Configuration-specific questions
atmos ai ask "List all available components"
atmos ai ask "What's the difference between dev and prod stacks?"
atmos ai ask "How do I troubleshoot component validation errors?"
```

### Topic-specific Help

Get detailed help on specific Atmos topics:

```bash
# Core concepts
atmos ai help stacks      # Learn about stack configuration
atmos ai help components  # Understand components
atmos ai help templating  # Go templating in Atmos
atmos ai help workflows   # Workflow orchestration

# Advanced topics
atmos ai help validation  # Configuration validation
atmos ai help vendoring   # Component vendoring
atmos ai help "terraform integration"  # Custom topics
```

## What the AI Can Help With

### Understanding Concepts
- Atmos architecture and core concepts
- Stack vs component relationships
- Template functions and usage
- Workflow orchestration patterns

### Configuration Analysis
- Reviewing component configurations
- Understanding stack inheritance
- Debugging configuration issues
- Optimizing performance

### Best Practices
- Stack organization strategies
- Component reusability patterns
- Template usage guidelines
- Validation and testing approaches

### Troubleshooting
- Error message interpretation
- Common configuration issues
- Debugging workflows
- Performance optimization

## Example Use Cases

### New User Onboarding
```bash
atmos ai ask "I'm new to Atmos. What should I know?"
atmos ai help stacks
atmos ai ask "How do I create my first component?"
```

### Configuration Review
```bash
atmos ai ask "Review my vpc component configuration"
atmos ai ask "What are potential issues with my stack structure?"
atmos ai ask "How can I optimize my template usage?"
```

### Troubleshooting
```bash
atmos ai ask "I'm getting a validation error for component X. How do I fix it?"
atmos ai ask "My template rendering is failing. What could be wrong?"
atmos ai ask "How do I debug workflow execution issues?"
```

### Learning Advanced Features
```bash
atmos ai help workflows
atmos ai ask "How do I use Gomplate functions in my templates?"
atmos ai ask "What's the best way to handle secrets in Atmos?"
```

## Supported Providers

Atmos AI Assistant supports multiple AI providers:

| Provider | Default Model | API Key Environment Variable | Notes |
|----------|---------------|------------------------------|-------|
| **Anthropic (Claude)** | `claude-3-5-sonnet-20241022` | `ANTHROPIC_API_KEY` | Default provider, advanced reasoning |
| **OpenAI (GPT)** | `gpt-4o` | `OPENAI_API_KEY` | Widely available, strong general capabilities |
| **Google (Gemini)** | `gemini-2.0-flash-exp` | `GEMINI_API_KEY` | Fast responses, larger context window |
| **xAI (Grok)** | `grok-beta` | `XAI_API_KEY` | OpenAI-compatible, real-time knowledge |

You can switch providers by changing the `provider` field in your configuration.

## Security and Privacy

- **API Key Security**: Store your API keys securely and never commit them to version control
- **Configuration Privacy**: The AI assistant does not store or transmit your configuration data beyond the current session
- **Local Processing**: All processing is done through the provider's API; no data is stored locally by the AI features
- **Provider Terms**: Your usage is subject to the terms of service of your chosen provider (Anthropic, OpenAI, Google, or xAI)

## Limitations

- **AI Knowledge Cutoff**: The AI's knowledge of Atmos is current as of its training date
- **API Dependencies**: Requires internet connection and valid API key for your chosen provider
- **Configuration Context**: The AI works with your current Atmos configuration but cannot make direct changes
- **Rate Limits**: Subject to your provider's API rate limits and usage policies

## Troubleshooting

### AI Features Not Working

1. **Check Configuration**:
   ```bash
   # Verify AI is enabled in atmos.yaml
   grep -A 5 "ai:" atmos.yaml
   ```

2. **Verify API Key** (for your chosen provider):
   ```bash
   # For Anthropic
   echo $ANTHROPIC_API_KEY

   # For OpenAI
   echo $OPENAI_API_KEY

   # For Google Gemini
   echo $GEMINI_API_KEY

   # For xAI Grok
   echo $XAI_API_KEY
   ```

3. **Test Connectivity**:
   ```bash
   atmos ai ask "test"
   ```

### Common Issues

- **"AI features are not enabled"**: Add `ai.enabled: true` to your `atmos.yaml`
- **"API key not found"**: Set the appropriate environment variable for your provider
  - Anthropic (Claude): `ANTHROPIC_API_KEY`
  - OpenAI (GPT): `OPENAI_API_KEY`
  - Google (Gemini): `GEMINI_API_KEY`
  - xAI (Grok): `XAI_API_KEY`
- **"Failed to create AI client"**: Check your API key is valid and has sufficient credits
- **"Unsupported AI provider"**: Verify the `provider` field is set to `anthropic`, `openai`, `gemini`, or `grok`
- **Rate limiting errors**: Wait and retry, or check your provider account usage

## Contributing

The AI assistant is extensible and can be enhanced with additional capabilities. To contribute:

1. Review the AI package structure in `pkg/ai/`
2. Add new tools or capabilities in `pkg/ai/agent/`
3. Update documentation and examples
4. Test with various Atmos configurations

For issues or feature requests, please open an issue in the [Atmos repository](https://github.com/cloudposse/atmos/issues).