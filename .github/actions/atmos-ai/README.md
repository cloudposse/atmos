# Atmos AI GitHub Action

Execute Atmos AI commands in your GitHub Actions workflows for automated infrastructure analysis, PR reviews, and CI/CD integration.

## Features

- 🤖 **Automated PR Reviews** - AI-powered reviews of infrastructure changes
- 🔍 **Security Analysis** - Scan for security issues and misconfigurations
- ✅ **Validation** - Validate stack configurations before merge
- 📊 **Cost Analysis** - Analyze infrastructure costs
- 💬 **PR Comments** - Post AI analysis as PR comments
- 🔄 **Multi-Provider Support** - Works with all 7 AI providers (Anthropic, OpenAI, Gemini, Grok, Bedrock, Azure OpenAI, Ollama)
- 📈 **Token Tracking** - Monitor token usage and caching efficiency

## Quick Start

### Basic PR Review

```yaml
name: Atmos AI PR Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: cloudposse/atmos/.github/actions/atmos-ai@main
        with:
          prompt: |
            Review this PR for:
            1. Stack configuration errors
            2. Security issues
            3. Best practices violations
            4. Breaking changes
          provider: anthropic
          api-key: ${{ secrets.ANTHROPIC_API_KEY }}
          post-comment: true
```

### Security Scan

```yaml
name: Security Scan

on:
  pull_request:
    paths:
      - 'stacks/**'
      - 'components/**'

jobs:
  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: cloudposse/atmos/.github/actions/atmos-ai@main
        with:
          prompt: |
            Perform a security audit:
            - Check for exposed secrets or credentials
            - Validate IAM permissions follow least privilege
            - Review network security configurations
            - Flag any compliance issues
          provider: anthropic
          model: claude-sonnet-4-20250514
          api-key: ${{ secrets.ANTHROPIC_API_KEY }}
          format: json
          post-comment: true
          fail-on-error: false
```

### Cost Analysis

```yaml
name: Cost Analysis

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  cost-analysis:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: cloudposse/atmos/.github/actions/atmos-ai@main
        with:
          prompt: |
            Analyze the infrastructure changes in this PR:
            - Estimate cost impact
            - Flag expensive resources
            - Suggest cost optimizations
          provider: openai
          model: gpt-4o
          api-key: ${{ secrets.OPENAI_API_KEY }}
          post-comment: true
          comment-header: '💰 Cost Analysis'
```

### Multi-Turn Analysis (Session-Based)

```yaml
name: Detailed Analysis

on:
  pull_request:
    types: [opened]

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Initial Review
        uses: cloudposse/atmos/.github/actions/atmos-ai@main
        with:
          prompt: "Review all changed stack configurations"
          session: pr-${{ github.event.pull_request.number }}
          api-key: ${{ secrets.ANTHROPIC_API_KEY }}
          post-comment: false

      - name: Security Analysis
        uses: cloudposse/atmos/.github/actions/atmos-ai@main
        with:
          prompt: "Based on the previous review, perform a detailed security analysis"
          session: pr-${{ github.event.pull_request.number }}
          api-key: ${{ secrets.ANTHROPIC_API_KEY }}
          post-comment: true
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `prompt` | The prompt or command to send to the AI | ✅ Yes | - |
| `provider` | AI provider: `anthropic`, `openai`, `gemini`, `grok`, `bedrock`, `azureopenai`, `ollama` | No | From atmos.yaml |
| `model` | AI model to use | No | From atmos.yaml |
| `api-key` | API key for the AI provider (from secrets) | No | - |
| `format` | Output format: `json`, `text`, `markdown` | No | `json` |
| `post-comment` | Post AI response as PR comment | No | `false` |
| `fail-on-error` | Fail workflow if AI execution fails | No | `true` |
| `atmos-version` | Atmos version to install | No | `latest` |
| `working-directory` | Directory where atmos.yaml is located | No | `.` |
| `session` | Session name for multi-turn conversations | No | - |
| `token` | GitHub token for PR comments | No | `${{ github.token }}` |
| `comment-header` | Header text for PR comments | No | `🤖 Atmos AI Analysis` |

## Outputs

| Output | Description |
|--------|-------------|
| `response` | AI response text |
| `success` | Whether execution succeeded (`true`/`false`) |
| `tool-calls` | Number of tool calls executed |
| `tokens-used` | Total tokens used |
| `cached-tokens` | Number of cached tokens used |
| `exit-code` | Exit code (0=success, 1=AI error, 2=tool error) |

## Examples

### Use Outputs in Subsequent Steps

```yaml
- name: Run AI Analysis
  id: ai-analysis
  uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Analyze the VPC configuration"
    api-key: ${{ secrets.ANTHROPIC_API_KEY }}
    format: json

- name: Check Result
  run: |
    echo "Success: ${{ steps.ai-analysis.outputs.success }}"
    echo "Response: ${{ steps.ai-analysis.outputs.response }}"
    echo "Tokens Used: ${{ steps.ai-analysis.outputs.tokens-used }}"
    echo "Cached: ${{ steps.ai-analysis.outputs.cached-tokens }}"
```

### Conditional Workflows

```yaml
- name: AI Review
  id: review
  uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Are there any critical issues in this PR?"
    api-key: ${{ secrets.ANTHROPIC_API_KEY }}
    fail-on-error: false

- name: Block Merge on Critical Issues
  if: contains(steps.review.outputs.response, 'CRITICAL')
  run: |
    echo "::error::Critical issues found, blocking merge"
    exit 1
```

### Multiple Providers

```yaml
jobs:
  analyze:
    strategy:
      matrix:
        provider: [anthropic, openai, gemini]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: cloudposse/atmos/.github/actions/atmos-ai@main
        with:
          prompt: "Review this infrastructure change"
          provider: ${{ matrix.provider }}
          api-key: ${{ secrets[format('{0}_API_KEY', matrix.provider)] }}
          comment-header: '🤖 ${{ matrix.provider }} Analysis'
          post-comment: true
```

### Slack Notification

```yaml
- name: AI Analysis
  id: analysis
  uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Summarize the infrastructure changes"
    api-key: ${{ secrets.ANTHROPIC_API_KEY }}

- name: Notify Slack
  uses: slackapi/slack-github-action@v1
  with:
    payload: |
      {
        "text": "AI Analysis Complete",
        "blocks": [
          {
            "type": "section",
            "text": {
              "type": "mrkdwn",
              "text": "*AI Analysis Result*\n${{ steps.analysis.outputs.response }}"
            }
          }
        ]
      }
  env:
    SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

## Configuration

The action uses your existing `atmos.yaml` configuration. Ensure AI is properly configured:

```yaml
# atmos.yaml
settings:
  ai:
    enabled: true
    default_provider: "anthropic"
    providers:
      anthropic:
        model: "claude-sonnet-4-20250514"
        api_key_env: "ANTHROPIC_API_KEY"
        max_tokens: 4096
```

## API Keys

Store API keys as GitHub secrets:

1. Go to your repository **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret**
3. Add secrets for your providers:
   - `ANTHROPIC_API_KEY`
   - `OPENAI_API_KEY`
   - `GEMINI_API_KEY`
   - `XAI_API_KEY` (for Grok)
   - For Bedrock: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
   - For Azure: `AZURE_OPENAI_API_KEY`

## Exit Codes

- **0** - Success
- **1** - AI execution error (model error, rate limit, etc.)
- **2** - Tool execution error (permission denied, tool failure, etc.)

Use `fail-on-error: false` to continue workflow even on errors.

## Token Caching

The action automatically benefits from Atmos AI's token caching capabilities:

- **Anthropic**: 90% savings on cached tokens
- **OpenAI**: 50% savings on cached tokens
- **Gemini**: Free cached tokens
- **Grok**: 75% savings on cached tokens
- **Bedrock**: 90% savings on cached tokens

Cached tokens are reported in outputs and PR comments.

## Limitations

- Requires Atmos configuration in the repository
- AI API keys must be available as secrets
- PR comments only work on `pull_request` events
- Session-based conversations require persistent storage (sessions stored in temporary runner)

## Troubleshooting

### Action fails with "Atmos not found"

The action automatically installs Atmos. If it fails, check:
- Runner has internet access
- GitHub API rate limits not exceeded

### API key not working

Ensure:
- Secret name matches the environment variable expected by the provider
- Secret value is correct and not expired
- Provider is enabled in `atmos.yaml`

### PR comment not posted

Check:
- `post-comment: true` is set
- `token` input has proper permissions
- Workflow is triggered by `pull_request` event

### Token usage higher than expected

Enable token caching in your `atmos.yaml`:
```yaml
settings:
  ai:
    providers:
      anthropic:
        enable_caching: true  # For Anthropic explicit caching
```

## Advanced Usage

### Custom Atmos Version

```yaml
- uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    atmos-version: "1.95.0"
    prompt: "Review infrastructure"
    api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

### Different Working Directory

```yaml
- uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    working-directory: ./infrastructure
    prompt: "Analyze stacks"
    api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

### Markdown Output

```yaml
- uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Generate infrastructure documentation"
    format: markdown
    post-comment: true
    api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Related Documentation

- [Atmos AI Documentation](https://atmos.tools/ai)
- [Atmos AI Configuration](https://atmos.tools/ai/configuration)
- [Atmos AI Providers](https://atmos.tools/ai/providers)
- [Atmos AI CLI Reference](https://atmos.tools/cli/commands/ai)

## Contributing

Contributions are welcome! Please submit issues and pull requests to the [Atmos repository](https://github.com/cloudposse/atmos).

## License

Apache 2.0 - See [LICENSE](../../../LICENSE) for details.
