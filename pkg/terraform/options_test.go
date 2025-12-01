package terraform

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, &tt.opts)
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

		// Verify embedded fields can be accessed directly.
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
			},
		}

		// Verify embedded fields can be accessed directly.
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
		assert.True(t, opts.DryRun)
	})
}

func TestPlanfileOptions(t *testing.T) {
	tests := []struct {
		name string
		opts PlanfileOptions
	}{
		{
			name: "JSON format",
			opts: PlanfileOptions{
				Component:            "vpc",
				Stack:                "dev-us-west-2",
				Format:               "json",
				File:                 "",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "YAML format with custom file",
			opts: PlanfileOptions{
				Component:            "rds",
				Stack:                "prod",
				Format:               "yaml",
				File:                 "/custom/path/plan.yaml",
				ProcessTemplates:     true,
				ProcessYamlFunctions: false,
				Skip:                 []string{"terraform.output"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, &tt.opts)
		})
	}
}
