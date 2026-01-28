# Atmos AI GitHub Actions Examples

This directory contains example workflows demonstrating how to use the Atmos AI GitHub Action for various use cases.

## Available Examples

### 1. PR Review (`atmos-ai-pr-review.yml`)

Automatically review pull requests for infrastructure changes.

**Features:**
- Configuration error detection
- Security issue scanning
- Best practices validation
- Breaking change identification
- Cost impact assessment

**Use Case:** Run on every PR to catch issues early

### 2. Security Scan (`atmos-ai-security-scan.yml`)

Comprehensive security audit of infrastructure configurations.

**Features:**
- Authentication & authorization checks
- Network security validation
- Data protection verification
- Compliance requirements
- Secrets management audit

**Use Case:** Daily scheduled scans + PR checks

### 3. Cost Analysis (`atmos-ai-cost-analysis.yml`)

Analyze cost impact of infrastructure changes.

**Features:**
- New resource cost estimation
- Resource scaling impact analysis
- Cost optimization recommendations
- High-cost change alerts
- Approval workflow for expensive changes

**Use Case:** Run on PRs affecting infrastructure resources

## How to Use These Examples

### Step 1: Choose an Example

Pick the workflow(s) that match your needs. You can use one or combine multiple workflows.

### Step 2: Copy to Workflows Directory

```bash
# Copy individual workflow
cp .github/workflows/examples/atmos-ai-pr-review.yml .github/workflows/

# Or copy all examples
cp .github/workflows/examples/*.yml .github/workflows/
```

### Step 3: Configure API Keys

Add your AI provider API key as a GitHub secret:

1. Go to repository **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret**
3. Add the appropriate secret:
   - `ANTHROPIC_API_KEY` (recommended)
   - `OPENAI_API_KEY`
   - `GEMINI_API_KEY`
   - `XAI_API_KEY` (for Grok)
   - For Bedrock: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
   - For Azure: `AZURE_OPENAI_API_KEY`

### Step 4: Customize for Your Needs

Edit the workflow files to:
- Adjust file paths and triggers
- Modify prompts for your requirements
- Change AI providers/models
- Customize comments and notifications
- Add team-specific checks

### Step 5: Test

Create a test PR to verify the workflow runs correctly.

## Customization Tips

### Change AI Provider

```yaml
provider: openai  # or gemini, grok, bedrock, azureopenai
model: gpt-4o
api-key: ${{ secrets.OPENAI_API_KEY }}
```

### Adjust Failure Behavior

```yaml
fail-on-error: false  # Continue even if AI fails
```

### Customize PR Comments

```yaml
comment-header: '🚀 Custom Analysis'
```

### Add Multiple Providers

Use matrix strategy to run analysis with multiple AI providers:

```yaml
strategy:
  matrix:
    provider: [anthropic, openai, gemini]
```

## Common Patterns

### Conditional Execution

Only run on specific file changes:

```yaml
on:
  pull_request:
    paths:
      - 'stacks/**'
      - 'components/terraform/**'
```

### Multi-Step Analysis

Use sessions for multi-turn analysis:

```yaml
- name: Initial Review
  uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Review all configurations"
    session: pr-${{ github.event.pull_request.number }}

- name: Follow-up Analysis
  uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Based on the previous review, perform security analysis"
    session: pr-${{ github.event.pull_request.number }}
```

### Using Outputs

Access AI analysis results in subsequent steps:

```yaml
- name: AI Analysis
  id: analysis
  uses: cloudposse/atmos/.github/actions/atmos-ai@main
  with:
    prompt: "Analyze configuration"

- name: Check Result
  run: |
    echo "Success: ${{ steps.analysis.outputs.success }}"
    echo "Tokens: ${{ steps.analysis.outputs.tokens-used }}"
```

## Best Practices

1. **Use Specific Prompts** - Clear, detailed prompts get better results
2. **Enable Token Caching** - Configure in `atmos.yaml` to reduce costs
3. **Set Appropriate Timeouts** - Some analyses take longer than others
4. **Handle Failures Gracefully** - Use `fail-on-error: false` for non-critical checks
5. **Post Comments Selectively** - Not every analysis needs a PR comment
6. **Use Sessions for Complex Analysis** - Multi-turn conversations maintain context
7. **Monitor Token Usage** - Track costs via outputs and adjust accordingly

## Troubleshooting

### Workflow doesn't trigger

Check:
- Workflow is in `.github/workflows/` (not in `examples/`)
- File paths in triggers match your changes
- Branch protection rules allow workflows

### API key errors

Verify:
- Secret name matches the workflow
- Secret value is correct
- Provider is enabled in `atmos.yaml`

### PR comments not appearing

Ensure:
- `post-comment: true` is set
- Workflow has `pull-requests: write` permission
- Running on `pull_request` event

### High token usage

Optimize by:
- Enabling token caching in `atmos.yaml`
- Using more efficient prompts
- Choosing appropriate models (Haiku for simple tasks)

## Additional Resources

- [Atmos AI Action README](../.github/actions/atmos-ai/README.md)
- [Atmos AI Documentation](https://atmos.tools/ai)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)

## Support

- Issues: [GitHub Issues](https://github.com/cloudposse/atmos/issues)
- Discussions: [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- Slack: [Cloud Posse Slack](https://slack.cloudposse.com)
