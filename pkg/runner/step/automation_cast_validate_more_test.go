package step

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/castplan"
)

func TestValidateCastReportsActionPositionInDeclaration(t *testing.T) {
	plan := castValidationPlan(
		castplan.Action{Kind: "simulate", Fields: map[string]any{"text": "ok"}},
		castplan.Action{Kind: "pause", Fields: map[string]any{"duration": "soon"}},
	)
	call := &automation.StepCall{Type: "cast", WorkingDirectory: t.TempDir()}
	err := NewAutomationLibrary(nil, nil).ValidateCast(plan, call)
	require.ErrorContains(t, err, "cast action 2 (pause)")
}

func TestValidateCastRejectsMalformedActionFields(t *testing.T) {
	tests := []struct {
		name     string
		plan     *castplan.Plan
		want     error
		contains string
	}{
		{
			name:     "recorded command with a mistyped prompt",
			plan:     castValidationPlan(castplan.Action{Kind: "run", Fields: map[string]any{"command": "echo hi", "prompt": []any{1}}}),
			contains: "cannot unmarshal",
		},
		{
			name:     "terminal action with an unknown field",
			plan:     castValidationPlan(castplan.Action{Kind: "session", Children: []castplan.Action{{Kind: "write", Fields: map[string]any{"txt": "x"}}}}),
			want:     errUtils.ErrAutomation,
			contains: `unknown write action field "txt"`,
		},
		{
			name:     "terminal wait without text or regex",
			plan:     castValidationPlan(castplan.Action{Kind: "session", Children: []castplan.Action{{Kind: "wait"}}}),
			want:     ErrWaitActionRequiresTextOrRegex,
			contains: "text or regex",
		},
		{
			name:     "terminal wait with an invalid regex",
			plan:     castValidationPlan(castplan.Action{Kind: "session", Children: []castplan.Action{{Kind: "wait", Fields: map[string]any{"regex": "("}}}}),
			contains: "wait regex",
		},
		{
			name:     "terminal pause without a duration",
			plan:     castValidationPlan(castplan.Action{Kind: "session", Children: []castplan.Action{{Kind: "pause"}}}),
			want:     ErrPauseActionRequiresDuration,
			contains: "duration",
		},
		{
			name:     "terminal write without text",
			plan:     castValidationPlan(castplan.Action{Kind: "session", Children: []castplan.Action{{Kind: "write"}}}),
			want:     ErrWriteActionRequiresText,
			contains: "text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &automation.StepCall{Type: "cast", WorkingDirectory: t.TempDir()}
			err := NewAutomationLibrary(nil, nil).ValidateCast(tt.plan, call)
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
			require.ErrorContains(t, err, tt.contains)
		})
	}
}

func TestValidateCastAcceptsRegisteredStepAndFieldlessTerminalActions(t *testing.T) {
	plan := castValidationPlan(
		castplan.Action{Kind: "step", Fields: map[string]any{"type": "sleep", "duration": "1ms"}},
		castplan.Action{Kind: "session", Children: []castplan.Action{
			{Kind: "hide"},
			{Kind: "wait", Fields: map[string]any{"regex": "ready-[0-9]+", "timeout": "5s"}},
			{Kind: "show"},
		}},
	)
	call := &automation.StepCall{Type: "cast", WorkingDirectory: t.TempDir()}
	require.NoError(t, NewAutomationLibrary(nil, nil).ValidateCast(plan, call))
}

func TestValidateCastWithoutConfigurationRequiresOutput(t *testing.T) {
	call := &automation.StepCall{Type: "cast", WorkingDirectory: t.TempDir()}
	err := NewAutomationLibrary(nil, nil).ValidateCast(&castplan.Plan{}, call)
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	require.ErrorContains(t, err, "requires output.cast")
}
