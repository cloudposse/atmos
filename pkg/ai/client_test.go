package ai

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	"github.com/cloudposse/atmos/pkg/ai/agent/openai"
)

// TestAnthropicClientImplementsInterface verifies that anthropic.SimpleClient implements ai.Client interface.
func TestAnthropicClientImplementsInterface(t *testing.T) {
	var _ Client = (*anthropic.SimpleClient)(nil)
}

// TestOpenAIClientImplementsInterface verifies that openai.Client implements ai.Client interface.
func TestOpenAIClientImplementsInterface(t *testing.T) {
	var _ Client = (*openai.Client)(nil)
}
