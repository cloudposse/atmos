package exec

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsWorkflowStepFunction(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"env with args", "!env STACK_ACCOUNT dev", true},
		{"env without args", "!env", true},
		{"exec", "!exec echo hi", true},
		{"plain string", "dev", false},
		{"go template", "{{ .env.STACK_ACCOUNT }}", false},
		{"stack-dependent function is not gated", "!terraform.output vpc id", false},
		{"env-like prefix but different tag", "!environment FOO", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isWorkflowStepFunction(tt.value))
		})
	}
}

func TestResolveStepFunctionString_Env(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("env var set is returned", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_STEP_ACCOUNT", "prod")
		got, err := resolveStepFunctionString(atmosConfig, "!env ATMOS_TEST_STEP_ACCOUNT dev")
		require.NoError(t, err)
		assert.Equal(t, "prod", got)
	})

	t.Run("env var unset falls back to default", func(t *testing.T) {
		got, err := resolveStepFunctionString(atmosConfig, "!env ATMOS_TEST_STEP_UNSET dev")
		require.NoError(t, err)
		assert.Equal(t, "dev", got)
	})

	t.Run("plain value passes through unchanged", func(t *testing.T) {
		got, err := resolveStepFunctionString(atmosConfig, "dev")
		require.NoError(t, err)
		assert.Equal(t, "dev", got)
	})

	t.Run("go template passes through unchanged", func(t *testing.T) {
		got, err := resolveStepFunctionString(atmosConfig, "{{ .env.STACK_ACCOUNT }}")
		require.NoError(t, err)
		assert.Equal(t, "{{ .env.STACK_ACCOUNT }}", got)
	})

	t.Run("stack-dependent function passes through unchanged", func(t *testing.T) {
		got, err := resolveStepFunctionString(atmosConfig, "!terraform.output vpc id")
		require.NoError(t, err)
		assert.Equal(t, "!terraform.output vpc id", got)
	})

	t.Run("malformed env function returns an error", func(t *testing.T) {
		_, err := resolveStepFunctionString(atmosConfig, "!env")
		require.Error(t, err)
	})
}

func TestResolveStepFunctionString_Exec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell output handling differs on Windows")
	}
	got, err := resolveStepFunctionString(&schema.AtmosConfiguration{}, "!exec echo ci-value")
	require.NoError(t, err)
	assert.Equal(t, "ci-value", got)
}

func TestResolveWorkflowStepFunctions(t *testing.T) {
	t.Setenv("ATMOS_TEST_WF_ACCOUNT", "prod")
	t.Setenv("ATMOS_TEST_WF_PROMPT", "Pick account")

	def := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:    "account",
				Type:    "choose",
				Prompt:  "!env ATMOS_TEST_WF_PROMPT Account",
				Options: []string{"!env ATMOS_TEST_WF_ACCOUNT dev", "staging"},
				Default: "!env ATMOS_TEST_WF_ACCOUNT dev",
			},
			{
				Name:        "tag",
				Type:        "input",
				Prompt:      "Tag",
				Placeholder: "!env ATMOS_TEST_WF_UNSET latest",
				Default:     "!env ATMOS_TEST_WF_UNSET latest",
			},
		},
	}

	err := resolveWorkflowStepFunctions(&schema.AtmosConfiguration{}, def)
	require.NoError(t, err)

	assert.Equal(t, "prod", def.Steps[0].Default)
	assert.Equal(t, "Pick account", def.Steps[0].Prompt)
	assert.Equal(t, []string{"prod", "staging"}, def.Steps[0].Options)

	assert.Equal(t, "latest", def.Steps[1].Default)
	assert.Equal(t, "latest", def.Steps[1].Placeholder)
}

func TestResolveWorkflowStepFunctions_NestedSteps(t *testing.T) {
	t.Setenv("ATMOS_TEST_WF_NESTED", "resolved")

	def := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name: "group",
				Type: "parallel",
				Steps: []schema.WorkflowStep{
					{
						Name:    "inner",
						Type:    "input",
						Prompt:  "Inner",
						Default: "!env ATMOS_TEST_WF_NESTED fallback",
					},
				},
			},
		},
	}

	err := resolveWorkflowStepFunctions(&schema.AtmosConfiguration{}, def)
	require.NoError(t, err)
	assert.Equal(t, "resolved", def.Steps[0].Steps[0].Default)
}

func TestResolveWorkflowStepFunctions_NilDefinition(t *testing.T) {
	err := resolveWorkflowStepFunctions(&schema.AtmosConfiguration{}, nil)
	require.NoError(t, err)
}
