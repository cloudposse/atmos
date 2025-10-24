package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsAIEnabled(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expected    bool
	}{
		{
			name: "AI not configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expected: false,
		},
		{
			name: "AI explicitly disabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expected: false,
		},
		{
			name: "AI enabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
					},
				},
			},
			expected: true,
		},
		{
			name: "AI enabled with provider configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model:     "claude-3-5-sonnet-20241022",
								MaxTokens: 4096,
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "AI with invalid enabled value (defaults to false)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAIEnabled(tt.atmosConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}
