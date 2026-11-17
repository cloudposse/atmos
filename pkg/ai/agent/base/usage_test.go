package base

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/types"
)

// TestUsageFromAnthropic checks that cache reads and writes are counted in InputTokens and TotalTokens,
// so the prompt count means the same as for the OpenAI-compatible and Gemini providers.
func TestUsageFromAnthropic(t *testing.T) {
	tests := []struct {
		name                                  string
		input, output, cacheRead, cacheCreate int64
		expected                              *types.Usage
	}{
		{
			name:  "no cache use",
			input: 100, output: 50,
			expected: &types.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			name:  "cache read only",
			input: 90, output: 12, cacheRead: 5526,
			expected: &types.Usage{InputTokens: 5616, OutputTokens: 12, TotalTokens: 5628, CacheReadTokens: 5526},
		},
		{
			name:  "cache write only (first request)",
			input: 90, output: 12, cacheCreate: 5526,
			expected: &types.Usage{InputTokens: 5616, OutputTokens: 12, TotalTokens: 5628, CacheCreationTokens: 5526},
		},
		{
			name:  "cache read and write",
			input: 100, output: 50, cacheRead: 30, cacheCreate: 20,
			expected: &types.Usage{InputTokens: 150, OutputTokens: 50, TotalTokens: 200, CacheReadTokens: 30, CacheCreationTokens: 20},
		},
		{
			name:  "whole prompt read from cache",
			input: 0, output: 0, cacheRead: 2048,
			expected: &types.Usage{InputTokens: 2048, TotalTokens: 2048, CacheReadTokens: 2048},
		},
		{
			name:  "output only",
			input: 0, output: 50,
			expected: &types.Usage{OutputTokens: 50, TotalTokens: 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := UsageFromAnthropic(tt.input, tt.output, tt.cacheRead, tt.cacheCreate)

			require.NotNil(t, usage)
			assert.Equal(t, tt.expected, usage)
			assert.LessOrEqual(t, usage.CacheReadTokens+usage.CacheCreationTokens, usage.InputTokens)
		})
	}
}

// TestUsageFromAnthropic_NoTokens checks that a response reporting no tokens yields no usage.
func TestUsageFromAnthropic_NoTokens(t *testing.T) {
	assert.Nil(t, UsageFromAnthropic(0, 0, 0, 0))
}
