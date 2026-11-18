package base

import (
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// UsageFromAnthropic converts token counts in the Anthropic Messages API format into types.Usage.
//
// The Anthropic API, and Claude on Amazon Bedrock, report prompt cache reads and writes outside
// input_tokens: the full prompt is input_tokens + cache_read_input_tokens + cache_creation_input_tokens.
// The OpenAI-compatible and Gemini APIs already count cached tokens in their prompt total, so this adds
// the cache counts to InputTokens. InputTokens is then the full prompt size for every provider, and
// CacheReadTokens and CacheCreationTokens are the parts of it read from and written to the cache.
//
// Returns nil when the response reports no tokens.
func UsageFromAnthropic(inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64) *types.Usage {
	if inputTokens <= 0 && outputTokens <= 0 && cacheReadTokens <= 0 && cacheCreationTokens <= 0 {
		return nil
	}

	promptTokens := inputTokens + cacheReadTokens + cacheCreationTokens

	return &types.Usage{
		InputTokens:         promptTokens,
		OutputTokens:        outputTokens,
		TotalTokens:         promptTokens + outputTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
	}
}
