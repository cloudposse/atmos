package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func flagVars(mode string) *Variables {
	vars := NewVariables()
	vars.SetFlag("mode", mode)
	return vars
}

func TestResolveStepOutputMode(t *testing.T) {
	tests := []struct {
		name    string
		step    schema.WorkflowStep
		vars    *Variables
		want    string
		wantErr bool
	}{
		{name: "unset", step: schema.WorkflowStep{Name: "s"}, vars: flagVars("none"), want: ""},
		{name: "literal mode", step: schema.WorkflowStep{Name: "s", Output: "raw"}, vars: flagVars("none"), want: "raw"},
		{name: "templated mode", step: schema.WorkflowStep{Name: "s", Output: "{{ .Flags.mode }}"}, vars: flagVars("none"), want: "none"},
		{name: "templated unknown mode", step: schema.WorkflowStep{Name: "s", Output: "{{ .Flags.mode }}"}, vars: flagVars("bogus"), wantErr: true},
		{name: "templated empty result", step: schema.WorkflowStep{Name: "s", Output: "{{ .Flags.mode }}"}, vars: flagVars(""), want: ""},
		{name: "template without vars is rejected", step: schema.WorkflowStep{Name: "s", Output: "{{ .Flags.mode }}"}, vars: nil, wantErr: true},
		{name: "literal field keeps braces and is rejected", step: schema.WorkflowStep{Name: "s", Output: "{{ .Flags.mode }}", LiteralFields: []string{"output"}}, vars: flagVars("none"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveStepOutputMode(&tc.step, tc.vars)
			if tc.wantErr {
				require.ErrorIs(t, err, schema.ErrStepInvalidOutputMode)
				assert.Contains(t, err.Error(), "valid modes are raw, log, viewport, none")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestApplyStepOutputModeRendersInPlaceAndKeepsTemplateInHandlerCopies(t *testing.T) {
	step := schema.WorkflowStep{Name: "s", Type: schema.TaskTypeScript, Output: "{{ .Flags.mode }}"}

	resolved, err := resolveOutputStep(&step, flagVars("log"))
	require.NoError(t, err)
	assert.Equal(t, "log", resolved.Output)
	assert.Equal(t, "{{ .Flags.mode }}", step.Output, "the shared step keeps its template for later runs")

	require.NoError(t, ApplyStepOutputMode(&step, flagVars("none")))
	assert.Equal(t, "none", step.Output)
	assert.Equal(t, OutputModeNone, GetOutputMode(&step, nil))
}

func TestResolveOutputStepSkipsTypesWithTheirOwnOutput(t *testing.T) {
	step := schema.WorkflowStep{Name: "s", Type: schema.TaskTypeParallel, Output: "grouped"}
	resolved, err := resolveOutputStep(&step, flagVars("none"))
	require.NoError(t, err)
	assert.Same(t, &step, resolved)
}

func TestSleepRejectsFieldsOfOtherStepTypes(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)

	err := library.Validate(&automation.StepCall{Type: "sleep", Configuration: map[string]any{"duration": "100ms"}})
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Contains(t, err.Error(), `unknown field "duration" for step type "sleep"`)
	assert.Contains(t, err.Error(), "timeout")

	require.NoError(t, library.Validate(&automation.StepCall{Type: "sleep", Configuration: map[string]any{"timeout": "100ms", "name": "pause"}}))

	// Every built-in step declares its fields, so join rejects the field that belongs to sleep.
	err = library.Validate(&automation.StepCall{Type: "join", Configuration: map[string]any{"content": "x", "duration": "1s"}})
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Contains(t, err.Error(), `unknown field "duration" for step type "join"`)
}

// A handler registered without KnownFields keeps the union of all step fields.
func TestHandlerWithoutKnownFieldsKeepsTheUnionOfStepFields(t *testing.T) {
	handler := NewMockStepHandler(gomock.NewController(t))
	const handlerName = "undeclared-fields-test"
	handler.EXPECT().GetName().Return(handlerName).AnyTimes()
	handler.EXPECT().Validate(gomock.Any()).Return(nil).AnyTimes()
	handler.EXPECT().RequiresTTY().Return(false).AnyTimes()
	Register(handler)
	t.Cleanup(func() {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		delete(registry.handlers, handlerName)
	})

	library := NewAutomationLibrary(nil, nil)
	require.NoError(t, library.Validate(&automation.StepCall{Type: handlerName, Configuration: map[string]any{"content": "x", "duration": "1s"}}))
	err := library.Validate(&automation.StepCall{Type: handlerName, Configuration: map[string]any{"not_a_field": "x"}})
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Contains(t, err.Error(), `unknown step field "not_a_field"`)
}

func TestAutomationValidationReportsMisspelledRequiredField(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	err := library.Validate(&automation.StepCall{Type: "script", Configuration: map[string]any{"scritp": "print(1)", "interpreter": "starlark"}})
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Contains(t, err.Error(), `unknown field "scritp" for step type "script"`)
}

func TestSchedulerPolicyErrorNamesTheOffendingFields(t *testing.T) {
	step := &schema.WorkflowStep{Type: "join", Needs: []string{"a"}, Identity: "admin"}
	err := validateAutomationStep(step, false)
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Contains(t, err.Error(), "needs, identity are not supported in a direct step call or a Git hook")
	assert.Contains(t, err.Error(), "supported on steps in workflows and lifecycle hooks")
	assert.NotContains(t, err.Error(), "when, continue, identity, background execution, and freshness policies are")
}

func TestUnsupportedStepTypeErrorListsTypesAndHintsStarlark(t *testing.T) {
	err := UnsupportedStepTypeError("custom command `build`", "compile", "starlark")
	require.ErrorIs(t, err, errUtils.ErrInvalidWorkflowStepType)
	// Assert the visible wording independently of the terminal color profile.
	formatted := atmosansi.Strip(errUtils.Format(err, errUtils.FormatterConfig{}))
	assert.Contains(t, formatted, "Step compile of custom command build")
	assert.Contains(t, formatted, "script")
	assert.Contains(t, formatted, "shell")
	assert.Contains(t, formatted, "Use type: script with interpreter: starlark.")

	other := atmosansi.Strip(errUtils.Format(UnsupportedStepTypeError("workflow `w`", "", "nope"), errUtils.FormatterConfig{}))
	assert.Contains(t, other, "(unnamed)")
	assert.NotContains(t, other, "interpreter: starlark")
}

func TestExecutorReportsLoadErrorWithMissingField(t *testing.T) {
	loadErr := errors.New("step at index 0: unknown field \"scritp\"")
	step := &schema.WorkflowStep{Name: "s", Type: schema.TaskTypeScript, Interpreter: "starlark", LoadError: loadErr}
	_, err := NewStepExecutor().Execute(t.Context(), step)
	require.ErrorIs(t, err, loadErr)
	assert.ErrorIs(t, err, errUtils.ErrStepFieldRequired, "the missing-field error is reported alongside the unknown field")
	require.ErrorIs(t, ValidateStep(step), loadErr)
}
