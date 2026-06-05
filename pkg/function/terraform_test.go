package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewTerraformOutputFunction(t *testing.T) {
	fn := NewTerraformOutputFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagTerraformOutput, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestNewTerraformStateFunction(t *testing.T) {
	fn := NewTerraformStateFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagTerraformState, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestParseTerraformArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          string
		contextStack  string
		wantComponent string
		wantStack     string
		wantOutput    string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "three parts - component stack output",
			args:          "vpc tenant1-ue2-dev vpc_id",
			contextStack:  "default",
			wantComponent: "vpc",
			wantStack:     "tenant1-ue2-dev",
			wantOutput:    "vpc_id",
		},
		{
			name:          "two parts - component output uses context stack",
			args:          "vpc vpc_id",
			contextStack:  "tenant1-ue2-prod",
			wantComponent: "vpc",
			wantStack:     "tenant1-ue2-prod",
			wantOutput:    "vpc_id",
		},
		{
			name:          "with extra whitespace",
			args:          "  vpc   tenant1-ue2-dev   vpc_id  ",
			contextStack:  "default",
			wantComponent: "vpc",
			wantStack:     "tenant1-ue2-dev",
			wantOutput:    "vpc_id",
		},
		{
			name:         "too few arguments",
			args:         "vpc",
			contextStack: "default",
			wantErr:      true,
			errContains:  "requires 2 or 3 arguments",
		},
		{
			name:         "too many arguments",
			args:         "vpc stack output extra",
			contextStack: "default",
			wantErr:      true,
			errContains:  "requires 2 or 3 arguments",
		},
		{
			name:         "empty args",
			args:         "",
			contextStack: "default",
			wantErr:      true,
			errContains:  "", // Empty args return EOF error from parser.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCtx := &ExecutionContext{
				Stack: tt.contextStack,
			}

			parsed, err := parseTerraformArgs(tt.args, execCtx)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, parsed)
			assert.Equal(t, tt.wantComponent, parsed.component)
			assert.Equal(t, tt.wantStack, parsed.stack)
			assert.Equal(t, tt.wantOutput, parsed.output)
		})
	}
}

func TestTerraformOutputFunction_Execute_NilContext(t *testing.T) {
	fn := NewTerraformOutputFunction()

	// Test with nil execution context.
	_, err := fn.Execute(context.Background(), "vpc vpc_id", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
	assert.Contains(t, err.Error(), "requires AtmosConfig")

	// Test with nil AtmosConfig.
	execCtx := &ExecutionContext{AtmosConfig: nil}
	_, err = fn.Execute(context.Background(), "vpc vpc_id", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
}

func TestTerraformOutputFunction_Execute_EmptyArgs(t *testing.T) {
	fn := NewTerraformOutputFunction()
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "test-stack",
	}

	// Test with empty args.
	_, err := fn.Execute(context.Background(), "", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArguments)
	assert.Contains(t, err.Error(), "requires arguments")
}

func TestTerraformOutputFunction_Execute_InvalidArgs(t *testing.T) {
	fn := NewTerraformOutputFunction()
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "test-stack",
	}

	// Test with single argument.
	_, err := fn.Execute(context.Background(), "vpc", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArguments)
}

func TestTerraformOutputFunction_Execute_NotMigrated(t *testing.T) {
	// This tests the current placeholder implementation.
	fn := NewTerraformOutputFunction()
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "tenant1-ue2-dev",
	}

	// Execute with valid args - should return "not yet migrated" error.
	_, err := fn.Execute(context.Background(), "vpc tenant1-ue2-dev vpc_id", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
	assert.Contains(t, err.Error(), "not yet fully migrated")
	assert.Contains(t, err.Error(), "component=vpc")
	assert.Contains(t, err.Error(), "stack=tenant1-ue2-dev")
	assert.Contains(t, err.Error(), "output=vpc_id")
}

func TestTerraformStateFunction_Execute_NilContext(t *testing.T) {
	fn := NewTerraformStateFunction()

	// Test with nil execution context.
	_, err := fn.Execute(context.Background(), "vpc vpc_id", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
	assert.Contains(t, err.Error(), "requires AtmosConfig")

	// Test with nil AtmosConfig.
	execCtx := &ExecutionContext{AtmosConfig: nil}
	_, err = fn.Execute(context.Background(), "vpc vpc_id", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
}

func TestTerraformStateFunction_Execute_EmptyArgs(t *testing.T) {
	fn := NewTerraformStateFunction()
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "test-stack",
	}

	// Test with empty args.
	_, err := fn.Execute(context.Background(), "", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArguments)
	assert.Contains(t, err.Error(), "requires arguments")
}

func TestTerraformStateFunction_Execute_InvalidArgs(t *testing.T) {
	fn := NewTerraformStateFunction()
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "test-stack",
	}

	// Test with single argument.
	_, err := fn.Execute(context.Background(), "vpc", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArguments)
}

func TestTerraformStateFunction_Execute_NotMigrated(t *testing.T) {
	// This tests the current placeholder implementation.
	fn := NewTerraformStateFunction()
	execCtx := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "tenant1-ue2-prod",
	}

	// Execute with valid args - should return "not yet migrated" error.
	_, err := fn.Execute(context.Background(), "eks cluster_name", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
	assert.Contains(t, err.Error(), "not yet fully migrated")
	assert.Contains(t, err.Error(), "component=eks")
	assert.Contains(t, err.Error(), "stack=tenant1-ue2-prod")
	assert.Contains(t, err.Error(), "output=cluster_name")
}

func TestTerraformArgs_Struct(t *testing.T) {
	args := &terraformArgs{
		component: "vpc",
		stack:     "tenant1-ue2-dev",
		output:    "vpc_id",
	}

	assert.Equal(t, "vpc", args.component)
	assert.Equal(t, "tenant1-ue2-dev", args.stack)
	assert.Equal(t, "vpc_id", args.output)
}
