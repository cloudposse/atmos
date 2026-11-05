package ai

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsInvalidToolMode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "invalid mode", err: errUtils.ErrAIToolsInvalidMode, want: true},
		{name: "wrapped invalid mode", err: fmt.Errorf("failed to set up tool approval: %w", errUtils.ErrAIToolsInvalidMode), want: true},
		{name: "another tool error", err: errUtils.ErrAIToolsDisabled, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isInvalidToolMode(tt.err))
		})
	}
}

// A mistyped ai.tools.mode must surface as the sentinel the commands stop on, not be lost in a warning.
func TestInitializeAIToolsAndExecutor_InvalidModeIsFatalSentinel(t *testing.T) {
	cfg := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	cfg.AI.Tools.Enabled = true
	cfg.AI.Tools.Mode = "alow"
	cfg.AI.DefaultProvider = "claude-code" // A CLI provider, so no MCP servers are started.

	result, err := initializeAIToolsAndExecutor(cfg, nil, "")

	require.Error(t, err)
	assert.True(t, isInvalidToolMode(err))
	assert.Nil(t, result)
}

func TestAppendTurn(t *testing.T) {
	seed := []types.Message{
		{Role: types.RoleUser, Content: "earlier question"},
		{Role: types.RoleAssistant, Content: "earlier answer"},
	}

	t.Run("records the prompt the AI received, so stack context survives", func(t *testing.T) {
		got := appendTurn(seed, "stack context\n\nhow many stacks?", "289")

		require.Len(t, got, 4)
		assert.Equal(t, "earlier question", got[0].Content, "first element is the earliest turn")
		assert.Equal(t, "stack context\n\nhow many stacks?", got[2].Content)
		assert.Equal(t, types.RoleUser, got[2].Role)
		assert.Equal(t, "289", got[3].Content, "last element is the new answer")
		assert.Equal(t, types.RoleAssistant, got[3].Role)
	})

	t.Run("does not write into the caller's slice", func(t *testing.T) {
		spare := make([]types.Message, 2, 8)
		copy(spare, seed)

		got := appendTurn(spare, "q", "a")

		require.Len(t, got, 4)
		assert.Equal(t, types.Message{}, spare[:cap(spare)][2], "spare capacity of the caller's slice is untouched")
		assert.Equal(t, seed, spare)
	})

	t.Run("the result is independent of the input", func(t *testing.T) {
		got := appendTurn(seed, "q", "a")
		got[0].Content = "mutated"

		assert.Equal(t, "earlier question", seed[0].Content)
	})
}
