package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkflowOptionsBuilder(t *testing.T) {
	builder := NewWorkflowOptionsBuilder()
	require.NotNil(t, builder)
	assert.NotNil(t, builder.options)
}

func TestWorkflowOptionsBuilder_WithFile(t *testing.T) {
	tests := []struct {
		name     string
		required bool
	}{
		{
			name:     "optional file flag",
			required: false,
		},
		{
			name:     "required file flag",
			required: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewWorkflowOptionsBuilder().
				WithFile(tt.required)

			require.NotNil(t, builder)
			assert.NotEmpty(t, builder.options)
		})
	}
}

func TestWorkflowOptionsBuilder_WithDryRun(t *testing.T) {
	builder := NewWorkflowOptionsBuilder().
		WithDryRun()

	require.NotNil(t, builder)
	assert.NotEmpty(t, builder.options)
}

func TestWorkflowOptionsBuilder_WithFromStep(t *testing.T) {
	builder := NewWorkflowOptionsBuilder().
		WithFromStep()

	require.NotNil(t, builder)
	assert.NotEmpty(t, builder.options)
}

func TestWorkflowOptionsBuilder_WithStack(t *testing.T) {
	tests := []struct {
		name     string
		required bool
	}{
		{
			name:     "optional stack flag",
			required: false,
		},
		{
			name:     "required stack flag",
			required: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewWorkflowOptionsBuilder().
				WithStack(tt.required)

			require.NotNil(t, builder)
			assert.NotEmpty(t, builder.options)
		})
	}
}

func TestWorkflowOptionsBuilder_WithIdentity(t *testing.T) {
	builder := NewWorkflowOptionsBuilder().
		WithIdentity()

	require.NotNil(t, builder)
	assert.NotEmpty(t, builder.options)
}

func TestWorkflowOptionsBuilder_Build(t *testing.T) {
	builder := NewWorkflowOptionsBuilder().
		WithFile(false).
		WithDryRun().
		WithFromStep().
		WithStack(false).
		WithIdentity()

	parser := builder.Build()

	require.NotNil(t, parser)
	assert.NotNil(t, parser.parser)
}

func TestWorkflowOptionsBuilder_FluentInterface(t *testing.T) {
	// Test that all builder methods return the builder for chaining
	builder := NewWorkflowOptionsBuilder().
		WithFile(true).
		WithDryRun().
		WithFromStep().
		WithStack(true).
		WithIdentity()

	require.NotNil(t, builder)

	parser := builder.Build()
	require.NotNil(t, parser)
}

func TestWorkflowOptionsBuilder_MinimalConfiguration(t *testing.T) {
	// Test building with no additional configuration
	builder := NewWorkflowOptionsBuilder()
	parser := builder.Build()

	require.NotNil(t, parser)
	assert.NotNil(t, parser.parser)
}

func TestWorkflowOptionsBuilder_PartialConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*WorkflowOptionsBuilder) *WorkflowOptionsBuilder
		wantNil bool
	}{
		{
			name: "only file flag",
			setup: func(b *WorkflowOptionsBuilder) *WorkflowOptionsBuilder {
				return b.WithFile(false)
			},
			wantNil: false,
		},
		{
			name: "only dry-run flag",
			setup: func(b *WorkflowOptionsBuilder) *WorkflowOptionsBuilder {
				return b.WithDryRun()
			},
			wantNil: false,
		},
		{
			name: "only from-step flag",
			setup: func(b *WorkflowOptionsBuilder) *WorkflowOptionsBuilder {
				return b.WithFromStep()
			},
			wantNil: false,
		},
		{
			name: "only stack flag",
			setup: func(b *WorkflowOptionsBuilder) *WorkflowOptionsBuilder {
				return b.WithStack(false)
			},
			wantNil: false,
		},
		{
			name: "only identity flag",
			setup: func(b *WorkflowOptionsBuilder) *WorkflowOptionsBuilder {
				return b.WithIdentity()
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewWorkflowOptionsBuilder()
			builder = tt.setup(builder)
			parser := builder.Build()

			if tt.wantNil {
				assert.Nil(t, parser)
			} else {
				require.NotNil(t, parser)
			}
		})
	}
}
