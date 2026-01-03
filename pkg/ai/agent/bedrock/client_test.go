package bedrock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedConfig *base.Config
	}{
		{
			name: "Default configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   false,
				Model:     "anthropic.claude-sonnet-4-20250514-v2:0",
				BaseURL:   "us-east-1", // Region stored in BaseURL.
				MaxTokens: 4096,
			},
		},
		{
			name: "Enabled configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								Model:     "anthropic.claude-3-haiku-20240307-v1:0",
								MaxTokens: 8192,
								BaseURL:   "us-west-2",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-3-haiku-20240307-v1:0",
				BaseURL:   "us-west-2",
				MaxTokens: 8192,
			},
		},
		{
			name: "Partial configuration",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								Model: "anthropic.claude-3-opus-20240229-v1:0",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-3-opus-20240229-v1:0",
				BaseURL:   "us-east-1",
				MaxTokens: 4096,
			},
		},
		{
			name: "Custom region only",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"bedrock": {
								BaseURL: "eu-west-1",
							},
						},
					},
				},
			},
			expectedConfig: &base.Config{
				Enabled:   true,
				Model:     "anthropic.claude-sonnet-4-20250514-v2:0",
				BaseURL:   "eu-west-1",
				MaxTokens: 4096,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base.ExtractConfig(tt.atmosConfig, ProviderName, base.ProviderDefaults{
				Model:     DefaultModel,
				MaxTokens: DefaultMaxTokens,
				BaseURL:   DefaultRegion, // Region stored in BaseURL.
			})
			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}

func TestNewClient_Disabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Enabled: false,
			},
		},
	}

	client, err := NewClient(context.TODO(), atmosConfig)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "AI features are disabled")
}

func TestClientGetters(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "anthropic.claude-sonnet-4-20250514-v2:0",
		BaseURL:   "us-east-1",
		MaxTokens: 4096,
	}

	client := &Client{
		client: nil, // We don't need a real client for testing getters.
		config: config,
		region: "us-east-1",
	}

	assert.Equal(t, "anthropic.claude-sonnet-4-20250514-v2:0", client.GetModel())
	assert.Equal(t, 4096, client.GetMaxTokens())
	assert.Equal(t, "us-east-1", client.GetRegion())
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, 4096, DefaultMaxTokens)
	assert.Equal(t, "anthropic.claude-sonnet-4-20250514-v2:0", DefaultModel)
	assert.Equal(t, "us-east-1", DefaultRegion)
}

func TestConfig_AllFields(t *testing.T) {
	config := &base.Config{
		Enabled:   true,
		Model:     "test-model",
		BaseURL:   "ap-southeast-1",
		MaxTokens: 1000,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "test-model", config.Model)
	assert.Equal(t, "ap-southeast-1", config.BaseURL)
	assert.Equal(t, 1000, config.MaxTokens)
}
