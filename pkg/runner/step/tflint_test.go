package step

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/scanners"
	tflintscanner "github.com/cloudposse/atmos/pkg/scanners/tflint"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTFLintHandlerIsRegistered(t *testing.T) {
	handler, ok := Get(tflintStepType)
	require.True(t, ok)
	assert.Equal(t, tflintStepType, handler.GetName())
	assert.Equal(t, CategoryCommand, handler.GetCategory())
}

func TestTFLintHandlerValidateRequiresComponent(t *testing.T) {
	h := &TFLintHandler{BaseHandler: NewBaseHandler(tflintStepType, CategoryCommand, false)}
	err := h.Validate(&schema.WorkflowStep{Name: "lint", Type: tflintStepType})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrStepFieldRequired))
}

func TestTFLintHandlerExecuteResolvesInputsAndBuildsResult(t *testing.T) {
	original := runTFLint
	t.Cleanup(func() { runTFLint = original })

	vars := NewVariables()
	vars.SetFlag("stack", "dev")
	vars.SetTemplateData(map[string]any{"component": "vpc"})
	vars.SetComponentInfoResolver(func(_ context.Context, component, stack, componentType string) (*schema.ConfigAndStacksInfo, error) {
		assert.Equal(t, "vpc", component)
		assert.Equal(t, "dev", stack)
		assert.Equal(t, "terraform", componentType)
		return &schema.ConfigAndStacksInfo{Component: component, ComponentFromArg: component, Stack: stack}, nil
	})

	runTFLint = func(_ context.Context, options *tflintscanner.Options) (*scanners.Output, *scanners.Context, error) {
		assert.Equal(t, []string{"--chdir=$ATMOS_COMPONENT_PATH", "--format=sarif", "--minimum-failure-severity=warning"}, options.Args)
		assert.Equal(t, map[string]string{"TFLINT_LOG": "info"}, options.Env)
		assert.Equal(t, scanners.OnFailureFail, options.OnFailure)
		return &scanners.Output{Summary: &scanners.Summary{Status: scanners.StatusWarning, Title: "1 finding", Findings: []scanners.Finding{{RuleID: "terraform_unused"}}}}, nil, nil
	}

	h := &TFLintHandler{BaseHandler: NewBaseHandler(tflintStepType, CategoryCommand, false)}
	result, err := h.Execute(context.Background(), &schema.WorkflowStep{
		Name:      "lint",
		Type:      tflintStepType,
		Component: "{{ .component }}",
		Args:      []string{"--minimum-failure-severity=warning"},
		Env:       map[string]string{"TFLINT_LOG": "info"},
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "1 finding", result.Value)
	assert.Equal(t, "vpc", result.Metadata["component"])
	assert.Equal(t, "dev", result.Metadata["stack"])
	assert.Equal(t, 1, result.Metadata["findings"])
}

func TestTFLintHandlerExecuteReturnsScannerErrorWithResult(t *testing.T) {
	original := runTFLint
	t.Cleanup(func() { runTFLint = original })

	scannerErr := errors.New("tflint failed")
	runTFLint = func(_ context.Context, _ *tflintscanner.Options) (*scanners.Output, *scanners.Context, error) {
		return &scanners.Output{Summary: &scanners.Summary{Title: "lint failed"}}, nil, scannerErr
	}

	vars := NewVariables()
	vars.SetFlag("stack", "dev")
	h := &TFLintHandler{BaseHandler: NewBaseHandler(tflintStepType, CategoryCommand, false)}
	result, err := h.Execute(context.Background(), &schema.WorkflowStep{Name: "lint", Type: tflintStepType, Component: "vpc"}, vars)

	require.ErrorIs(t, err, scannerErr)
	require.NotNil(t, result)
	assert.Equal(t, "lint failed", result.Value)
	assert.Equal(t, scannerErr.Error(), result.Error)
}

func TestTFLintHandlerExecuteReturnsResolutionErrors(t *testing.T) {
	h := &TFLintHandler{BaseHandler: NewBaseHandler(tflintStepType, CategoryCommand, false)}

	t.Run("missing stack", func(t *testing.T) {
		_, err := h.Execute(context.Background(), &schema.WorkflowStep{Name: "lint", Type: tflintStepType, Component: "vpc"}, NewVariables())
		require.ErrorIs(t, err, errUtils.ErrStepFieldRequired)
	})

	t.Run("component template", func(t *testing.T) {
		vars := NewVariables()
		vars.SetFlag("stack", "dev")
		vars.SetTemplateRenderer(func(_ string, _ string, _ any) (string, error) { return "", errors.New("template failed") })
		_, err := h.Execute(context.Background(), &schema.WorkflowStep{Name: "lint", Type: tflintStepType, Component: "{{ .component }}"}, vars)
		require.ErrorContains(t, err, "failed to resolve component")
	})
}

func TestResolveTFLintArgsAppendsUserArgsToDefaults(t *testing.T) {
	vars := NewVariables()
	args, err := resolveTFLintArgs(&schema.WorkflowStep{
		Name: "lint",
		Args: []string{"--minimum-failure-severity={{ .flags.severity }}"},
	}, tflintStepConfig{Args: []string{"--minimum-failure-severity={{ .flags.severity }}"}}, vars)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"--chdir=$ATMOS_COMPONENT_PATH",
		"--format=sarif",
		"--minimum-failure-severity=<no value>",
	}, args)
}

func TestTFLintConfigReadsWithPayload(t *testing.T) {
	step := &schema.WorkflowStep{
		Type: tflintStepType,
		With: map[string]any{
			"component": "vpc",
			"stack":     "plat-ue2-dev",
			"args":      []any{"--minimum-failure-severity=error"},
			"env": map[string]any{
				"TFLINT_LOG": "info",
			},
		},
	}

	cfg := tflintConfig(step)
	assert.Equal(t, "vpc", cfg.Component)
	assert.Equal(t, "plat-ue2-dev", cfg.Stack)
	assert.Equal(t, []string{"--minimum-failure-severity=error"}, cfg.Args)
	assert.Equal(t, map[string]string{"TFLINT_LOG": "info"}, cfg.Env)
}

func TestTFLintConfigExplicitFieldsOverrideWithPayload(t *testing.T) {
	step := &schema.WorkflowStep{
		Type:      tflintStepType,
		Component: "explicit-component",
		Stack:     "explicit-stack",
		Args:      []string{"--only-explicit"},
		Env:       map[string]string{"LOG": "debug"},
		With: map[string]any{
			"component": "with-component",
			"stack":     "with-stack",
			"args":      []string{"--with"},
			"env":       map[string]string{"LOG": "info"},
		},
	}
	cfg := tflintConfig(step)
	assert.Equal(t, "explicit-component", cfg.Component)
	assert.Equal(t, "explicit-stack", cfg.Stack)
	assert.Equal(t, []string{"--only-explicit"}, cfg.Args)
	assert.Equal(t, map[string]string{"LOG": "debug"}, cfg.Env)
}

func TestResolveTFLintStackUsesStepThenFlagsThenEnvironment(t *testing.T) {
	vars := NewVariables()
	vars.SetFlag("stack", "flag-stack")
	vars.SetEnv("ATMOS_STACK", "env-stack")
	step := &schema.WorkflowStep{Name: "lint"}

	got, err := resolveTFLintStack(step, tflintStepConfig{Stack: "step-stack"}, vars)
	require.NoError(t, err)
	assert.Equal(t, "step-stack", got)

	got, err = resolveTFLintStack(step, tflintStepConfig{}, vars)
	require.NoError(t, err)
	assert.Equal(t, "flag-stack", got)

	vars.SetFlag("stack", "")
	got, err = resolveTFLintStack(step, tflintStepConfig{}, vars)
	require.NoError(t, err)
	assert.Equal(t, "env-stack", got)

	vars.SetEnv("ATMOS_STACK", "")
	_, err = resolveTFLintStack(step, tflintStepConfig{}, vars)
	require.ErrorIs(t, err, errUtils.ErrStepFieldRequired)
}

func TestTFLintStepResultIncludesSummaryMetadata(t *testing.T) {
	result := tflintStepResult("vpc", "prod", &scanners.Output{Summary: &scanners.Summary{
		Status:   scanners.StatusWarning,
		Title:    "2 findings",
		Counts:   map[string]int{"warning": 2},
		Findings: []scanners.Finding{{RuleID: "terraform_unused"}, {RuleID: "terraform_required_version"}},
	}})
	require.NotNil(t, result)
	assert.Equal(t, "2 findings", result.Value)
	assert.Equal(t, "vpc", result.Metadata["component"])
	assert.Equal(t, "prod", result.Metadata["stack"])
	assert.Equal(t, "warning", result.Metadata["status"])
	assert.Equal(t, 2, result.Metadata["findings"])

	empty := tflintStepResult("vpc", "prod", nil)
	assert.Equal(t, "tflint completed", empty.Value)
}
