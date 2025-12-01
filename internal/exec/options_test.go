package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessingOptions(t *testing.T) {
	tests := []struct {
		name string
		opts ProcessingOptions
	}{
		{
			name: "All defaults",
			opts: ProcessingOptions{},
		},
		{
			name: "All enabled",
			opts: ProcessingOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             []string{},
			},
		},
		{
			name: "All disabled",
			opts: ProcessingOptions{
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
			},
		},
		{
			name: "With skip functions",
			opts: ProcessingOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             []string{"terraform.output", "terraform.state"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct fields are accessible.
			assert.NotNil(t, &tt.opts)
		})
	}
}

func TestCleanOptions(t *testing.T) {
	tests := []struct {
		name string
		opts CleanOptions
	}{
		{
			name: "Clean all components",
			opts: CleanOptions{
				Component:    "",
				Stack:        "",
				Force:        true,
				Everything:   true,
				SkipLockFile: false,
				DryRun:       false,
			},
		},
		{
			name: "Clean specific component",
			opts: CleanOptions{
				Component:    "vpc",
				Stack:        "dev-us-west-2",
				Force:        false,
				Everything:   false,
				SkipLockFile: false,
				DryRun:       false,
			},
		},
		{
			name: "Dry run mode",
			opts: CleanOptions{
				Component:    "vpc",
				Stack:        "dev",
				Force:        true,
				Everything:   false,
				SkipLockFile: true,
				DryRun:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, &tt.opts)
		})
	}
}

func TestGenerateBackendOptions(t *testing.T) {
	tests := []struct {
		name string
		opts GenerateBackendOptions
	}{
		{
			name: "Basic backend options",
			opts: GenerateBackendOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
				},
			},
		},
		{
			name: "With skip functions",
			opts: GenerateBackendOptions{
				Component: "rds",
				Stack:     "prod-us-east-1",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: false,
					Skip:             []string{"terraform.output"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify embedded ProcessingOptions fields are accessible directly.
			assert.Equal(t, tt.opts.Component, tt.opts.Component)
			// Access embedded field directly (not through ProcessingOptions).
			_ = tt.opts.ProcessTemplates
		})
	}
}

func TestVarfileOptions(t *testing.T) {
	tests := []struct {
		name string
		opts VarfileOptions
	}{
		{
			name: "Default varfile path",
			opts: VarfileOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
				File:      "",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
				},
			},
		},
		{
			name: "Custom varfile path",
			opts: VarfileOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
				File:      "/custom/path/vars.tfvars.json",
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify embedded ProcessingOptions fields are accessible directly.
			// Access embedded fields directly (not through ProcessingOptions).
			_ = tt.opts.ProcessTemplates
			_ = tt.opts.ProcessFunctions
		})
	}
}

func TestShellOptions(t *testing.T) {
	tests := []struct {
		name string
		opts ShellOptions
	}{
		{
			name: "Normal shell",
			opts: ShellOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
				DryRun:    false,
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
				},
			},
		},
		{
			name: "Dry run shell",
			opts: ShellOptions{
				Component: "rds",
				Stack:     "prod",
				DryRun:    true,
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
					Skip:             []string{"terraform.output"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify all fields are accessible.
			assert.Equal(t, tt.opts.DryRun, tt.opts.DryRun)
			// Access embedded field directly (not through ProcessingOptions).
			_ = tt.opts.ProcessTemplates
		})
	}
}

func TestOptionsEmbedding(t *testing.T) {
	t.Run("ProcessingOptions embedded fields accessible directly", func(t *testing.T) {
		opts := VarfileOptions{
			Component: "test",
			Stack:     "test-stack",
			ProcessingOptions: ProcessingOptions{
				ProcessTemplates: true,
				ProcessFunctions: false,
				Skip:             []string{"func1", "func2"},
			},
		}

		// Verify embedded fields can be accessed directly through the struct.
		assert.True(t, opts.ProcessTemplates)
		assert.False(t, opts.ProcessFunctions)
		assert.Len(t, opts.Skip, 2)
	})

	t.Run("GenerateBackendOptions embedded fields accessible directly", func(t *testing.T) {
		opts := GenerateBackendOptions{
			Component: "backend-test",
			Stack:     "backend-stack",
			ProcessingOptions: ProcessingOptions{
				ProcessTemplates: false,
				ProcessFunctions: true,
				Skip:             []string{"skip1"},
			},
		}

		// Verify embedded fields can be accessed directly.
		assert.False(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
		assert.Contains(t, opts.Skip, "skip1")
	})

	t.Run("ShellOptions embedded fields accessible directly", func(t *testing.T) {
		opts := ShellOptions{
			Component: "shell-test",
			Stack:     "shell-stack",
			DryRun:    true,
			ProcessingOptions: ProcessingOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             nil,
			},
		}

		// Verify embedded fields can be accessed directly.
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
		assert.Nil(t, opts.Skip)
		assert.True(t, opts.DryRun)
	})
}
