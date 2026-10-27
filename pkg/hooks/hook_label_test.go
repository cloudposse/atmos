package hooks

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepFromHookNamed_LabelsWithHookName(t *testing.T) {
	hook := &Hook{Kind: stepKindName, Type: "shell", With: map[string]any{"command": "true"}}

	ws, err := StepFromHookNamed("before-mark", hook)
	require.NoError(t, err)
	assert.Equal(t, "hook:before-mark", ws.Name)
	assert.Equal(t, "shell", ws.Type)

	// An explicit step name in `with:` wins over the hook name.
	hook.With = map[string]any{"command": "true", "name": "explicit"}
	ws, err = StepFromHookNamed("before-mark", hook)
	require.NoError(t, err)
	assert.Equal(t, "explicit", ws.Name)

	// An empty hook name falls back to the step type, matching StepFromHook.
	hook.With = map[string]any{"command": "true"}
	ws, err = StepFromHookNamed("", hook)
	require.NoError(t, err)
	assert.Equal(t, "hook:shell", ws.Name)
}

func TestHookStepLabel(t *testing.T) {
	assert.Equal(t, "hook:my-hook", hookStepLabel("my-hook", "shell"))
	assert.Equal(t, "hook:shell", hookStepLabel("", "shell"))
}

func TestOutcomeEnv(t *testing.T) {
	tests := []struct {
		name    string
		outcome Outcome
		want    map[string]string
	}{
		{name: "zero outcome yields nothing", outcome: Outcome{}, want: map[string]string{}},
		{
			name:    "success",
			outcome: Outcome{Status: RunSuccess},
			want:    map[string]string{"ATMOS_HOOK_STATUS": "success", "ATMOS_HOOK_EXIT_CODE": "0"},
		},
		{
			name:    "failure with error",
			outcome: Outcome{Status: RunFailure, ExitCode: 3, Err: errors.New("boom")},
			want:    map[string]string{"ATMOS_HOOK_STATUS": "failure", "ATMOS_HOOK_EXIT_CODE": "3", "ATMOS_HOOK_ERROR": "boom"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, OutcomeEnv(tt.outcome))
		})
	}
}
