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
					AI: nil,
				},
			},
			expected: false,
		},
		{
			name: "AI explicitly disabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: map[string]interface{}{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name: "AI enabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "AI enabled with other settings",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: map[string]interface{}{
						"enabled":    true,
						"model":      "claude-3-5-sonnet-20241022",
						"max_tokens": 4096,
					},
				},
			},
			expected: true,
		},
		{
			name: "AI with invalid enabled value",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: map[string]interface{}{
						"enabled": "true", // string instead of bool
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
