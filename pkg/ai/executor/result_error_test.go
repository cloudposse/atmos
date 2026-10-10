package executor

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
)

func TestResultError(t *testing.T) {
	denied := errUtils.Build(errUtils.ErrCLIProviderToolDenied).WithHint("Run in a terminal").Err()

	tests := []struct {
		name       string
		result     *formatter.ExecutionResult
		wantIs     []error
		wantExact  error
		wantText   string
		wantNotNil bool
	}{
		{
			name:   "successful result with an answer",
			result: &formatter.ExecutionResult{Success: true, Response: "You have 42 stacks."},
		},
		{
			name:       "nil result",
			result:     nil,
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrAIExecutionFailed},
		},
		{
			name:       "empty answer is an error",
			result:     &formatter.ExecutionResult{Success: true, Response: ""},
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrAIEmptyResponse},
		},
		{
			name:       "whitespace-only answer is an error",
			result:     &formatter.ExecutionResult{Success: true, Response: " \n\t"},
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrAIEmptyResponse},
		},
		{
			name: "empty answer with tool activity is fine",
			result: &formatter.ExecutionResult{
				Success:   true,
				ToolCalls: []formatter.ToolCallResult{{Tool: "atmos_list_stacks"}},
			},
		},
		{
			name:       "failure without details",
			result:     &formatter.ExecutionResult{Success: false},
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrAIExecutionFailed},
		},
		{
			name: "plain failure is wrapped and keeps its message",
			result: &formatter.ExecutionResult{
				Success: false,
				Error:   &formatter.ErrorInfo{Message: "boom", Err: errors.New("boom")},
			},
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrAIExecutionFailed},
			wantText:   "AI execution failed: boom",
		},
		{
			name: "failure without original error still reports the message",
			result: &formatter.ExecutionResult{
				Success: false,
				Error:   &formatter.ErrorInfo{Message: "boom"},
			},
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrAIExecutionFailed},
			wantText:   "AI execution failed: boom",
		},
		{
			name: "user abort is returned unchanged",
			result: &formatter.ExecutionResult{
				Success: false,
				Error:   &formatter.ErrorInfo{Message: "user aborted", Err: fmt.Errorf("claude: %w", errUtils.ErrUserAborted)},
			},
			wantNotNil: true,
			wantIs:     []error{errUtils.ErrUserAborted},
		},
		{
			name: "provider error with hints is returned unchanged",
			result: &formatter.ExecutionResult{
				Success: false,
				Error:   &formatter.ErrorInfo{Message: denied.Error(), Err: denied},
			},
			wantNotNil: true,
			wantExact:  denied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ResultError(tt.result)

			if !tt.wantNotNil {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			for _, target := range tt.wantIs {
				assert.ErrorIs(t, err, target)
			}
			if tt.wantExact != nil {
				assert.Same(t, tt.wantExact, err)
			}
			if tt.wantText != "" {
				assert.EqualError(t, err, tt.wantText)
			}
		})
	}
}

func TestResultError_PlainFailureDoesNotLeakSelfExplainingSentinels(t *testing.T) {
	result := &formatter.ExecutionResult{
		Success: false,
		Error:   &formatter.ErrorInfo{Message: "boom", Err: errors.New("boom")},
	}

	err := ResultError(result)

	assert.NotErrorIs(t, err, errUtils.ErrUserAborted)
}
