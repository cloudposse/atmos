package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	// Import providers to register them.
	_ "github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/azureopenai"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/bedrock"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/gemini"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/grok"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/ollama"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/openai"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name: "No AI settings",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectError: true,
			errorMsg:    "AI features are disabled in configuration",
		},
		{
			name: "Anthropic provider (explicit)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:         true,
						DefaultProvider: "anthropic",
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Anthropic provider (default)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
					},
				},
			},
			expectError: false,
		},
		{
			name: "Unsupported provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:         true,
						DefaultProvider: "unsupported",
						Providers: map[string]*schema.AIProviderConfig{
							"unsupported": {},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "unsupported AI provider: unsupported",
		},
		{
			name: "OpenAI provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:         true,
						DefaultProvider: "openai",
						Providers: map[string]*schema.AIProviderConfig{
							"openai": {},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Gemini provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:         true,
						DefaultProvider: "gemini",
						Providers: map[string]*schema.AIProviderConfig{
							"gemini": {},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Grok provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:         true,
						DefaultProvider: "grok",
						Providers: map[string]*schema.AIProviderConfig{
							"grok": {},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Ollama provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:         true,
						DefaultProvider: "ollama",
						Providers: map[string]*schema.AIProviderConfig{
							"ollama": {},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Disabled AI",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expectError: true,
			errorMsg:    "AI features are disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.atmosConfig)

			// Handle expected error cases.
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, client)
				return
			}

			// Handle success cases.
			// Note: These tests require API key to be set.
			// We're testing the factory routing logic, not the actual client creation.
			if err == nil {
				assert.NotNil(t, client)
				return
			}

			// Check for expected errors when API key is not set.
			errMsg := err.Error()
			if strings.Contains(errMsg, "AI features are disabled") ||
				strings.Contains(errMsg, "API key not found") ||
				strings.Contains(errMsg, "base URL is required") ||
				strings.Contains(errMsg, "failed to load AWS configuration") {
				t.Skipf("Skipping test: %s (expected for factory test without API key)", errMsg)
				return
			}

			// Unexpected error.
			assert.NoError(t, err)
			assert.NotNil(t, client)
		})
	}
}
