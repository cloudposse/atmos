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

```yaml
settings:
  ai:
    enabled: true                    # Enable AI features (default: false)
    provider: "anthropic"            # AI provider (currently only "anthropic" supported)
    model: "claude-3-5-sonnet-20241022"  # Model to use (default: claude-3-5-sonnet-20241022)
    api_key_env: "ANTHROPIC_API_KEY" # Environment variable for API key (default: ANTHROPIC_API_KEY)
    max_tokens: 4096                 # Maximum tokens per response (default: 4096)
```

### Environment Setup

1. **Get an Anthropic API Key**: Sign up at [https://console.anthropic.com/](https://console.anthropic.com/) and create an API key.

2. **Set the Environment Variable**:
   ```bash
   export ANTHROPIC_API_KEY="your-api-key-here"
   ```

3. **Verify Configuration**:
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

## Security and Privacy

- **API Key Security**: Store your Anthropic API key securely and never commit it to version control
- **Configuration Privacy**: The AI assistant does not store or transmit your configuration data beyond the current session
- **Local Processing**: All processing is done through the Anthropic API; no data is stored locally by the AI features

## Limitations

- **AI Knowledge Cutoff**: The AI's knowledge of Atmos is current as of its training date
- **API Dependencies**: Requires internet connection and valid Anthropic API key
- **Configuration Context**: The AI works with your current Atmos configuration but cannot make direct changes
- **Rate Limits**: Subject to Anthropic API rate limits and usage policies

## Troubleshooting

### AI Features Not Working

1. **Check Configuration**:
   ```bash
   # Verify AI is enabled in atmos.yaml
   grep -A 5 "ai:" atmos.yaml
   ```

2. **Verify API Key**:
   ```bash
   echo $ANTHROPIC_API_KEY
   ```

3. **Test Connectivity**:
   ```bash
   atmos ai ask "test"
   ```

### Common Issues

- **"AI features are not enabled"**: Add `ai.enabled: true` to your `atmos.yaml`
- **"API key not found"**: Set the `ANTHROPIC_API_KEY` environment variable
- **"Failed to create AI client"**: Check your API key is valid and has sufficient credits
- **Rate limiting errors**: Wait and retry, or check your Anthropic account usage

## Contributing

The AI assistant is extensible and can be enhanced with additional capabilities. To contribute:

1. Review the AI package structure in `pkg/ai/`
2. Add new tools or capabilities in `pkg/ai/agent/`
3. Update documentation and examples
4. Test with various Atmos configurations

For issues or feature requests, please open an issue in the [Atmos repository](https://github.com/cloudposse/atmos/issues).