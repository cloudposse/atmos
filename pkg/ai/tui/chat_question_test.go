package tui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/approval"
)

type questionApproverFunc func(context.Context, approval.Request) (approval.Decision, error)

func (f questionApproverFunc) Approve(ctx context.Context, req approval.Request) (approval.Decision, error) {
	return f(ctx, req)
}

// TestChatQuestionTerminalHandoff covers the new question path alongside ordinary
// tools, whose permission prompter remains responsible for terminal ownership.
func TestChatQuestionTerminalHandoff(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		tool                              string
		promptErr, releaseErr, restoreErr error
		calls, released, restored         int
	}{
		{name: "question", tool: approval.AskUserQuestionTool, calls: 1, released: 1, restored: 1},
		{name: "tool uses existing handoff", tool: "Bash", calls: 1},
		{name: "abort restores chat", tool: approval.AskUserQuestionTool, promptErr: context.Canceled, calls: 1, released: 1, restored: 1},
		{name: "release fails", tool: approval.AskUserQuestionTool, releaseErr: context.Canceled, released: 1},
		{name: "restore fails", tool: approval.AskUserQuestionTool, restoreErr: context.Canceled, calls: 1, released: 1, restored: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terminal := &approvalTerminal{releaseErr: tc.releaseErr, restoreErr: tc.restoreErr}
			calls := 0
			answers := map[string]any{"answers": map[string]string{"Continue?": "Yes"}}
			inner := questionApproverFunc(func(_ context.Context, req approval.Request) (approval.Decision, error) {
				calls++
				assert.Equal(t, tc.tool, req.ToolName)
				assert.Equal(t, tc.released, terminal.released, "release precedes the prompt")
				assert.Zero(t, terminal.restored, "restore follows the prompt")
				return approval.Decision{Allow: tc.promptErr == nil, UpdatedInput: answers}, tc.promptErr
			})
			approver := &chatQuestionApprover{inner: inner, terminal: func() chatTerminal { return terminal }}
			decision, err := approver.Approve(t.Context(), approval.Request{ToolName: tc.tool})
			if tc.promptErr != nil || tc.releaseErr != nil || tc.restoreErr != nil {
				require.ErrorIs(t, err, context.Canceled)
				assert.False(t, decision.Allow)
			} else {
				require.NoError(t, err)
				assert.True(t, decision.Allow)
				assert.Equal(t, answers, decision.UpdatedInput)
			}
			assert.Equal(t, tc.calls, calls)
			assert.Equal(t, tc.released, terminal.released)
			assert.Equal(t, tc.restored, terminal.restored)
		})
	}
}

// TestChatQuestionWithoutTerminal cannot start a question before chat is attached.
func TestChatQuestionWithoutTerminal(t *testing.T) {
	approver := &chatQuestionApprover{terminal: func() chatTerminal { return nil }}
	decision, err := approver.Approve(t.Context(), approval.Request{ToolName: approval.AskUserQuestionTool})
	require.ErrorIs(t, err, errUtils.ErrInteractiveNotAvailable)
	assert.False(t, decision.Allow)
}
