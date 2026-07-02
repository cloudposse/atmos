package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
