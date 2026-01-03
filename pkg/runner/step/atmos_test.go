package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// AtmosHandler registration is tested in command_handlers_test.go.
// AtmosHandler validation is tested in command_handlers_test.go.
// This file tests the helper methods that don't require running actual atmos commands.

func TestAtmosHandler_ResolveStack(t *testing.T) {
	handler, ok := Get("atmos")
	require.True(t, ok)
	atmosHandler := handler.(*AtmosHandler)

	tests := []struct {
		name        string
		step        *schema.WorkflowStep
		vars        *Variables
		expected    string
		expectError bool
	}{
		{
			name: "empty stack returns empty",
			step: &schema.WorkflowStep{
				Name:  "test",
				Stack: "",
			},
			vars:        NewVariables(),
			expected:    "",
			expectError: false,
		},
		{
			name: "static stack value",
			step: &schema.WorkflowStep{
				Name:  "test",
				Stack: "dev-us-east-1",
			},
			vars:        NewVariables(),
			expected:    "dev-us-east-1",
			expectError: false,
		},
		{
			name: "template stack value",
			step: &schema.WorkflowStep{
				Name:  "test",
				Stack: "{{ .steps.env.value }}-us-east-1",
			},
			vars: func() *Variables {
				v := NewVariables()
				v.Set("env", NewStepResult("prod"))
				return v
			}(),
			expected:    "prod-us-east-1",
			expectError: false,
		},
		{
			name: "invalid template",
			step: &schema.WorkflowStep{
				Name:  "test",
				Stack: "{{ .steps.missing.value",
			},
			vars:        NewVariables(),
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := atmosHandler.resolveStack(tt.step, tt.vars)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAtmosHandler_ResolveWorkDir(t *testing.T) {
	handler, ok := Get("atmos")
	require.True(t, ok)
	atmosHandler := handler.(*AtmosHandler)

	tests := []struct {
		name        string
		step        *schema.WorkflowStep
		vars        *Variables
		expected    string
		expectError bool
	}{
		{
			name: "empty workdir returns empty",
			step: &schema.WorkflowStep{
				Name: "test",
			},
			vars:        NewVariables(),
			expected:    "",
			expectError: false,
		},
		{
			name: "static workdir",
			step: &schema.WorkflowStep{
				Name:             "test",
				WorkingDirectory: "/path/to/dir",
			},
			vars:        NewVariables(),
			expected:    "/path/to/dir",
			expectError: false,
		},
		{
			name: "template workdir",
			step: &schema.WorkflowStep{
				Name:             "test",
				WorkingDirectory: "{{ .steps.base.value }}/components",
			},
			vars: func() *Variables {
				v := NewVariables()
				v.Set("base", NewStepResult("/project"))
				return v
			}(),
			expected:    "/project/components",
			expectError: false,
		},
		{
			name: "invalid template",
			step: &schema.WorkflowStep{
				Name:             "test",
				WorkingDirectory: "{{ .steps.invalid.value",
			},
			vars:        NewVariables(),
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := atmosHandler.resolveWorkDir(tt.step, tt.vars)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAtmosHandler_ResolveEnvVars(t *testing.T) {
	handler, ok := Get("atmos")
	require.True(t, ok)
	atmosHandler := handler.(*AtmosHandler)

	tests := []struct {
		name        string
		step        *schema.WorkflowStep
		vars        *Variables
		expectNil   bool
		expectError bool
	}{
		{
			name: "nil env returns nil",
			step: &schema.WorkflowStep{
				Name: "test",
				Env:  nil,
			},
			vars:        NewVariables(),
			expectNil:   true,
			expectError: false,
		},
		{
			name: "empty env returns nil",
			step: &schema.WorkflowStep{
				Name: "test",
				Env:  map[string]string{},
			},
			vars:        NewVariables(),
			expectNil:   true,
			expectError: false,
		},
		{
			name: "static env vars",
			step: &schema.WorkflowStep{
				Name: "test",
				Env: map[string]string{
					"AWS_REGION": "us-east-1",
					"TF_VAR_env": "prod",
				},
			},
			vars:        NewVariables(),
			expectNil:   false,
			expectError: false,
		},
		{
			name: "template env vars",
			step: &schema.WorkflowStep{
				Name: "test",
				Env: map[string]string{
					"TARGET_ENV": "{{ .steps.env.value }}",
				},
			},
			vars: func() *Variables {
				v := NewVariables()
				v.Set("env", NewStepResult("staging"))
				return v
			}(),
			expectNil:   false,
			expectError: false,
		},
		{
			name: "invalid template in env",
			step: &schema.WorkflowStep{
				Name: "test",
				Env: map[string]string{
					"BAD_VAR": "{{ .steps.missing.value",
				},
			},
			vars:        NewVariables(),
			expectNil:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := atmosHandler.resolveEnvVars(tt.step, tt.vars)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectNil {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
				}
			}
		})
	}
}

func TestAtmosHandler_PrepareExecution(t *testing.T) {
	handler, ok := Get("atmos")
	require.True(t, ok)
	atmosHandler := handler.(*AtmosHandler)

	t.Run("basic preparation", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "atmos",
			Command: "terraform plan vpc",
		}
		vars := NewVariables()
		ctx := context.Background()

		opts, err := atmosHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "terraform plan vpc", opts.command)
		assert.Empty(t, opts.stack)
		assert.Empty(t, opts.workDir)
		assert.Nil(t, opts.envVars)
	})

	t.Run("full preparation", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test",
			Type:             "atmos",
			Command:          "terraform plan {{ .steps.component.value }}",
			Stack:            "{{ .steps.env.value }}-us-east-1",
			WorkingDirectory: "/project",
			Env: map[string]string{
				"AWS_REGION": "us-east-1",
			},
		}
		vars := NewVariables()
		vars.Set("component", NewStepResult("vpc"))
		vars.Set("env", NewStepResult("prod"))
		ctx := context.Background()

		opts, err := atmosHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "terraform plan vpc", opts.command)
		assert.Equal(t, "prod-us-east-1", opts.stack)
		assert.Equal(t, "/project", opts.workDir)
		assert.NotNil(t, opts.envVars)
	})

	t.Run("command resolution error", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "atmos",
			Command: "terraform plan {{ .steps.missing.value",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := atmosHandler.prepareExecution(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("stack resolution error", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "atmos",
			Command: "terraform plan vpc",
			Stack:   "{{ .steps.invalid.value",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := atmosHandler.prepareExecution(ctx, step, vars)
		assert.Error(t, err)
	})
}

func TestAtmosHandler_BuildAtmosResult(t *testing.T) {
	handler, ok := Get("atmos")
	require.True(t, ok)
	atmosHandler := handler.(*AtmosHandler)

	t.Run("success result", func(t *testing.T) {
		result := atmosHandler.buildAtmosResult("stdout content", "stderr content", nil)
		assert.Equal(t, "stdout content", result.Value)
		assert.Equal(t, "stdout content", result.Metadata["stdout"])
		assert.Equal(t, "stderr content", result.Metadata["stderr"])
		assert.Equal(t, 0, result.Metadata["exit_code"])
		assert.Empty(t, result.Error)
	})

	t.Run("error result", func(t *testing.T) {
		result := atmosHandler.buildAtmosResult("partial stdout", "error message", assert.AnError)
		assert.Equal(t, "partial stdout", result.Value)
		assert.Equal(t, "partial stdout", result.Metadata["stdout"])
		assert.Equal(t, "error message", result.Metadata["stderr"])
		assert.Equal(t, "error message", result.Error)
		assert.NotEqual(t, 0, result.Metadata["exit_code"])
	})

	t.Run("trims stdout on success", func(t *testing.T) {
		result := atmosHandler.buildAtmosResult("  output with whitespace  \n", "", nil)
		assert.Equal(t, "output with whitespace", result.Value)
		// Raw stdout preserved in metadata.
		assert.Equal(t, "  output with whitespace  \n", result.Metadata["stdout"])
	})
}

// containsStackFlag is already tested in command_handlers_test.go.
