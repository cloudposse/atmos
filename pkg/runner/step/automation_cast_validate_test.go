package step

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/castplan"
)

// Compile-time guard for the plan fields these cases construct.
var _ = castplan.Action{Kind: "run", Fields: map[string]any{}, Children: nil}

// castValidationPlan returns a declaration with a valid output and the given actions.
func castValidationPlan(actions ...castplan.Action) *castplan.Plan {
	return &castplan.Plan{
		Configuration: map[string]any{"output": map[string]any{"cast": "demo.cast", "mode": "raw"}},
		Actions:       actions,
	}
}

// TestValidateCastRejectsInvalidDeclarations covers the read-only declaration checks
// that run before any recording starts. Every case must fail without executing anything.
func TestValidateCastRejectsInvalidDeclarations(t *testing.T) {
	tests := []struct {
		name string
		plan *castplan.Plan
		want error
		// contains pins down which check failed when want is the generic automation error.
		contains string
	}{
		{
			name:     "missing output cast",
			plan:     &castplan.Plan{Configuration: map[string]any{}, Actions: []castplan.Action{{Kind: "simulate", Fields: map[string]any{"text": "hi"}}}},
			want:     errUtils.ErrAutomation,
			contains: "requires output.cast",
		},
		{
			name:     "unknown action kind",
			plan:     castValidationPlan(castplan.Action{Kind: "teleport", Fields: map[string]any{}}),
			want:     errUtils.ErrAutomation,
			contains: `unknown action "teleport"`,
		},
		{
			name: "screenshot without path",
			plan: castValidationPlan(castplan.Action{Kind: "screenshot", Fields: map[string]any{}}),
			want: ErrScreenshotActionRequiresPath,
		},
		{
			name:     "nested recording",
			plan:     castValidationPlan(castplan.Action{Kind: "step", Fields: map[string]any{"type": "cast"}}),
			want:     errUtils.ErrAutomation,
			contains: "nested recordings are not supported",
		},
		{
			name: "unknown registered step type",
			plan: castValidationPlan(castplan.Action{Kind: "step", Fields: map[string]any{"type": "no-such-step"}}),
			want: errUtils.ErrUnknownStepType,
		},
		{
			name:     "unknown run field",
			plan:     castValidationPlan(castplan.Action{Kind: "run", Fields: map[string]any{"command": "echo hi", "expct": "hi"}}),
			want:     errUtils.ErrAutomation,
			contains: `unknown shell action field "expct"`,
		},
		{
			name: "terminal without actions",
			plan: castValidationPlan(castplan.Action{Kind: "session", Fields: map[string]any{}}),
			want: ErrCastSessionRequiresActions,
		},
		{
			name: "terminal with unsupported key",
			plan: castValidationPlan(castplan.Action{Kind: "session", Fields: map[string]any{}, Children: []castplan.Action{
				{Kind: "key", Fields: map[string]any{"key": "page-up"}},
			}}),
			want: errUtils.ErrUnsupportedCastKey,
		},
		{
			name: "invalid action inside a hidden group",
			plan: castValidationPlan(castplan.Action{Kind: "hidden", Children: []castplan.Action{
				{Kind: "screenshot", Fields: map[string]any{}},
			}}),
			want: ErrScreenshotActionRequiresPath,
		},
		{
			name: "invalid setup action",
			plan: func() *castplan.Plan {
				plan := castValidationPlan(castplan.Action{Kind: "simulate", Fields: map[string]any{"text": "hi"}})
				plan.Setup = []castplan.Action{{Kind: "step", Fields: map[string]any{"type": "no-such-step"}}}
				return plan
			}(),
			want: errUtils.ErrUnknownStepType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := &api.StepCall{Type: "cast", WorkingDirectory: t.TempDir()}
			err := NewAutomationLibrary(nil, nil).ValidateCast(tt.plan, call)
			require.ErrorIs(t, err, tt.want)
			if tt.contains != "" {
				require.ErrorContains(t, err, tt.contains)
			}
		})
	}
}

func TestValidateCastRequiresDeclarationAndInvocation(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	require.ErrorIs(t, library.ValidateCast(nil, &api.StepCall{}), errUtils.ErrAutomation)
	require.ErrorIs(t, library.ValidateCast(castValidationPlan(), nil), errUtils.ErrAutomation)
}

func TestValidateCastAcceptsValidDeclaration(t *testing.T) {
	plan := castValidationPlan(
		castplan.Action{Kind: "simulate", Fields: map[string]any{"text": "# narration"}},
		castplan.Action{Kind: "run", Fields: map[string]any{"command": "echo hi"}, Expect: []castplan.Matcher{{Text: "hi"}}},
		castplan.Action{Kind: "pause", Fields: map[string]any{"duration": "10ms"}},
		castplan.Action{Kind: "screenshot", Fields: map[string]any{"path": "shot.png"}},
		castplan.Action{Kind: "session", Fields: map[string]any{}, Children: []castplan.Action{
			{Kind: "write", Fields: map[string]any{"text": "echo hi"}},
			{Kind: "key", Fields: map[string]any{"key": "enter"}},
			{Kind: "wait", Fields: map[string]any{"text": "hi"}},
		}},
	)
	call := &api.StepCall{Type: "cast", WorkingDirectory: t.TempDir()}
	require.NoError(t, NewAutomationLibrary(nil, nil).ValidateCast(plan, call))
}
