package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
			errorMsg:    "AI settings not configured",
		},
		{
			name: "Anthropic provider (explicit)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled:  true,
						Provider: "anthropic",
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
						Enabled:  true,
						Provider: "unsupported",
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
						Enabled:  true,
						Provider: "openai",
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
						Enabled:  true,
						Provider: "gemini",
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
						Enabled:  true,
						Provider: "grok",
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

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, client)
			} else {
				// Note: These tests require API key to be set.
				// We're testing the factory routing logic, not the actual client creation.
				if err != nil {
					// Expected errors when API key is not set.
					if err.Error() == "AI features are disabled in configuration" ||
						err.Error() == "API key not found in environment variable: ANTHROPIC_API_KEY" ||
						err.Error() == "API key not found in environment variable: OPENAI_API_KEY" ||
						err.Error() == "API key not found in environment variable: GEMINI_API_KEY" ||
						err.Error() == "API key not found in environment variable: XAI_API_KEY" {
						t.Skipf("Skipping test: %s (expected for factory test without API key)", err.Error())
					}
					// Unexpected error.
					assert.NoError(t, err)
				}
				assert.NotNil(t, client)
			}
		})
	}
}
