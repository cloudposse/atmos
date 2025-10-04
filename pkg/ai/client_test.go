package ai

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
)

// TestAnthropicClientImplementsInterface verifies that anthropic.SimpleClient implements ai.Client interface.
func TestAnthropicClientImplementsInterface(t *testing.T) {
	var _ Client = (*anthropic.SimpleClient)(nil)
}
