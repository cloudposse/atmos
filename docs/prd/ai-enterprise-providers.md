# PRD: Enterprise AI Provider Support

## Overview

This document describes the implementation of enterprise-grade AI providers for Atmos AI Assistant, specifically AWS Bedrock and Azure OpenAI, which offer enhanced security, compliance, and data privacy features required by enterprise customers.

## Background

Atmos currently supports several AI providers (Anthropic, OpenAI, Google Gemini, xAI Grok, Ollama), but lacks enterprise-specific options that provide:
- Enhanced data privacy and residency controls
- Enterprise security and compliance certifications
- Integration with existing cloud infrastructure
- Centralized billing and governance

## Objectives

1. Add AWS Bedrock provider support with AWS SDK integration
2. Add Azure OpenAI provider support with Azure SDK compatibility
3. Maintain compatibility with existing multi-provider architecture
4. Ensure seamless provider switching in TUI
5. Document enterprise configuration patterns

## Target Users

- **Enterprise DevOps Teams**: Organizations with strict data governance requirements
- **AWS/Azure-native Organizations**: Companies heavily invested in specific cloud ecosystems
- **Compliance-focused Teams**: Industries requiring SOC2, HIPAA, or other certifications
- **Cost-conscious Organizations**: Teams wanting to leverage existing cloud commitments

## Technical Architecture

### Provider Interface

Both providers implement the standard `ai.Client` interface:

```go
type Client interface {
    SendMessage(ctx context.Context, message string) (string, error)
    GetModel() string
    GetMaxTokens() int
}
```

### AWS Bedrock Implementation

**Location**: `pkg/ai/agent/bedrock/`

**Key Features**:
- Uses AWS SDK v2 for Go (`github.com/aws/aws-sdk-go-v2/service/bedrockruntime`)
- Supports standard AWS authentication methods (IAM roles, profiles, environment credentials)
- Configurable region selection via `base_url` field
- Supports Claude models via Bedrock Runtime API

**Configuration**:
```yaml
settings:
  ai:
    providers:
      bedrock:
        model: "anthropic.claude-sonnet-4-20250514-v2:0"
        max_tokens: 4096
        base_url: "us-east-1"  # AWS region
```

**Authentication**:
- Uses standard AWS SDK credential chain
- Respects `AWS_PROFILE`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
- Supports IAM roles in ECS/EKS/EC2 environments
- No separate API key required

**Supported Models**:
- `anthropic.claude-sonnet-4-20250514-v2:0` (default)
- `anthropic.claude-3-haiku-20240307-v1:0`
- `anthropic.claude-3-opus-20240229-v1:0`
- Any Bedrock-supported Claude model

### Azure OpenAI Implementation

**Location**: `pkg/ai/agent/azureopenai/`

**Key Features**:
- Uses OpenAI SDK with Azure endpoint configuration
- Requires Azure resource endpoint and deployment name
- Supports Azure AD authentication and API keys
- Compatible with Azure OpenAI API versioning

**Configuration**:
```yaml
settings:
  ai:
    providers:
      azureopenai:
        model: "gpt-4o"  # Your deployment name
        api_key_env: "AZURE_OPENAI_API_KEY"
        max_tokens: 4096
        base_url: "https://<resource>.openai.azure.com"
```

**Authentication**:
- API key via environment variable (default: `AZURE_OPENAI_API_KEY`)
- Custom API key environment variable supported
- API version: `2024-02-15-preview` (default)

**Supported Models**:
- `gpt-4o` (default)
- `gpt-4-turbo`
- `gpt-35-turbo`
- Any Azure OpenAI deployment

## Implementation Details

### Factory Integration

Both providers are registered in `pkg/ai/factory.go`:

```go
switch provider {
case "bedrock":
    ctx := context.Background()
    return bedrock.NewClient(ctx, atmosConfig)
case "azureopenai":
    return azureopenai.NewClient(atmosConfig)
// ... other providers
}
```

### Configuration Extraction Pattern

Both providers follow the same configuration extraction pattern:

1. Set defaults (model, max_tokens, etc.)
2. Check if AI is enabled globally
3. Extract provider-specific config from `Providers` map
4. Override defaults with provider-specific values

### TUI Integration

Both providers added to `availableProviders` in `pkg/ai/tui/chat.go`:

```go
{"bedrock", "AWS Bedrock - Enterprise-grade AI with AWS security and compliance"},
{"azureopenai", "Azure OpenAI - Enterprise OpenAI with Microsoft Azure integration"},
```

Users can switch to these providers mid-conversation using `Ctrl+P`.

## Testing Strategy

### Unit Tests

Both providers include comprehensive test coverage:

**Test Cases**:
1. `TestExtractConfig`: Validates configuration extraction with multiple scenarios
   - Default configuration (disabled)
   - Enabled with full configuration
   - Partial configuration (defaults applied)
   - Custom fields (region, endpoint)

2. `TestNewClient_Disabled`: Ensures proper error when AI is disabled

3. `TestClientGetters`: Validates getter methods return correct values

4. `TestDefaultConstants`: Verifies default values

5. `TestConfig_AllFields`: Tests configuration struct fields

**Coverage**: All tests pass with proper isolation (no external API calls)

### Integration Testing

Enterprise providers should be tested with:
- Real AWS/Azure credentials in staging environments
- Various model configurations
- Error handling for invalid credentials
- Network timeout scenarios

## Security Considerations

### AWS Bedrock

**Benefits**:
- Data never leaves AWS infrastructure
- Supports AWS PrivateLink for private connectivity
- Audit logging via AWS CloudTrail
- Encryption at rest and in transit
- IAM-based access control
- VPC isolation possible

**Best Practices**:
- Use IAM roles instead of access keys when possible
- Enable CloudTrail logging for audit compliance
- Configure VPC endpoints for private communication
- Use resource-based policies for fine-grained control

### Azure OpenAI

**Benefits**:
- Data residency controls (data stays in Azure region)
- Azure AD integration for authentication
- Managed identity support
- Compliance certifications (SOC2, HIPAA, ISO)
- Private endpoint support via Azure Private Link
- Customer-managed encryption keys (BYOK)

**Best Practices**:
- Use managed identities instead of API keys when possible
- Configure private endpoints for production
- Enable diagnostic logging for compliance
- Use Azure Key Vault for API key management
- Implement network security groups for access control

## Documentation Updates

### Website Documentation

Updated the following files:

1. **`website/docs/ai/ai.mdx`**:
   - Added providers to Quick Start configuration
   - Updated provider filtering list
   - Added configuration examples

2. **`website/docs/cli/commands/ai/chat.mdx`**:
   - Added providers to configuration section
   - Updated Supported Providers table
   - Added enterprise provider notes

3. **`website/docs/cli/commands/ai/ask.mdx`**:
   - Added environment variable examples
   - Noted AWS Bedrock credential handling

### Configuration Examples

**AWS Bedrock in Production**:
```yaml
settings:
  ai:
    enabled: true
    default_provider: "bedrock"
    providers:
      bedrock:
        model: "anthropic.claude-sonnet-4-20250514-v2:0"
        max_tokens: 4096
        base_url: "us-east-1"
```

**Azure OpenAI in Production**:
```yaml
settings:
  ai:
    enabled: true
    default_provider: "azureopenai"
    providers:
      azureopenai:
        model: "gpt-4o-production"  # Your deployment name
        api_key_env: "AZURE_OPENAI_API_KEY"
        max_tokens: 4096
        base_url: "https://company-prod.openai.azure.com"
```

## Migration Path

### From Anthropic to AWS Bedrock

For teams currently using Anthropic directly:

```yaml
# Before
providers:
  anthropic:
    model: "claude-sonnet-4-20250514"
    api_key_env: "ANTHROPIC_API_KEY"

# After (same model via Bedrock)
providers:
  bedrock:
    model: "anthropic.claude-sonnet-4-20250514-v2:0"
    base_url: "us-east-1"
```

**Benefits**:
- Same Claude models
- No API key management (uses AWS credentials)
- Lower cost with AWS commits
- Enhanced compliance and audit

### From OpenAI to Azure OpenAI

For teams currently using OpenAI directly:

```yaml
# Before
providers:
  openai:
    model: "gpt-4o"
    api_key_env: "OPENAI_API_KEY"

# After
providers:
  azureopenai:
    model: "gpt-4o-deployment"
    api_key_env: "AZURE_OPENAI_API_KEY"
    base_url: "https://company.openai.azure.com"
```

**Benefits**:
- Same GPT models
- Data stays in Azure region
- Azure AD integration
- Better SLA guarantees

## Cost Considerations

### AWS Bedrock Pricing

- **Claude 3.5 Sonnet**: ~$3/1M input tokens, ~$15/1M output tokens
- **Benefits**: Can leverage AWS Enterprise Agreements and Reserved Capacity
- **No minimum commitment**: Pay-per-use pricing
- **Free tier**: Limited free tier available for new accounts

### Azure OpenAI Pricing

- **GPT-4o**: ~$2.50/1M input tokens, ~$10/1M output tokens
- **Benefits**: Can leverage Azure commitments and Enterprise Agreements
- **Provisioned throughput**: Option for reserved capacity at lower cost
- **Regional pricing**: Varies by Azure region

### Cost Optimization

1. **Use appropriate models**: Claude Haiku or GPT-3.5 Turbo for simpler tasks
2. **Configure max_tokens**: Limit response length to control costs
3. **Monitor usage**: Use cloud provider cost management tools
4. **Reserved capacity**: Consider for high-volume production use

## Monitoring and Observability

### AWS Bedrock

**CloudWatch Metrics**:
- `ModelInvocationLatency`
- `ModelInputTokenCount`
- `ModelOutputTokenCount`
- `InvocationThrottles`
- `InvocationErrors`

**CloudTrail Events**:
- `InvokeModel` API calls
- Authentication attempts
- Configuration changes

### Azure OpenAI

**Azure Monitor Metrics**:
- `Total Calls`
- `Successful Calls`
- `Total Tokens`
- `Processing Time`
- `Throttled Calls`

**Diagnostic Logs**:
- Request/response logs
- Token usage analytics
- Error tracking
- Performance metrics

## Future Enhancements

1. **Google Cloud Vertex AI**: Add support for Vertex AI as another enterprise option
2. **Private Endpoints**: Document VPC/PrivateLink configuration patterns
3. **Cost Tracking**: Built-in token usage tracking and reporting
4. **Model Fallback**: Automatic failover between providers for reliability
5. **Regional Optimization**: Automatic region selection for lowest latency
6. **Managed Identity Support**: Enhanced Azure authentication options

## Success Criteria

1. ✅ Both providers successfully implement `ai.Client` interface
2. ✅ Comprehensive unit test coverage (100% for new code)
3. ✅ Integration with existing multi-provider architecture
4. ✅ TUI provider switching works seamlessly
5. ✅ Documentation covers configuration and best practices
6. ✅ Error handling for missing credentials and invalid configs
7. ✅ Compatible with existing session management

## Conclusion

The addition of AWS Bedrock and Azure OpenAI providers enables Atmos to serve enterprise customers with strict security and compliance requirements. Both providers integrate seamlessly with the existing multi-provider architecture while offering enterprise-specific benefits like enhanced data privacy, compliance certifications, and cloud infrastructure integration.

## Related Documents

- [MCP Integration Architecture](./mcp-integration-architecture.md)
- [Command Registry Pattern](./command-registry-pattern.md)
- [Phase 1 Implementation Summary](./phase-1-implementation-summary.md)
