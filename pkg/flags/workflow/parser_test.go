package workflow

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

func TestNewWorkflowParser(t *testing.T) {
	parser := NewWorkflowParser()
	require.NotNil(t, parser)
	assert.NotNil(t, parser.parser)
}

func TestNewWorkflowParser_WithOptions(t *testing.T) {
	parser := NewWorkflowParser(
		flags.WithStringFlag("file", "f", "", "Workflow file"),
		flags.WithBoolFlag("dry-run", "", false, "Dry run mode"),
	)

	require.NotNil(t, parser)
	assert.NotNil(t, parser.parser)
}

func TestWorkflowParser_RegisterFlags(t *testing.T) {
	parser := NewWorkflowParser(
		flags.WithStringFlag("file", "f", "", "Workflow file"),
		flags.WithBoolFlag("dry-run", "", false, "Dry run mode"),
	)

	cmd := &cobra.Command{
		Use: "workflow",
	}

	parser.RegisterFlags(cmd)

	assert.NotNil(t, parser.cmd)
	assert.Equal(t, cmd, parser.cmd)

	// Verify flags were registered
	fileFlag := cmd.Flags().Lookup("file")
	require.NotNil(t, fileFlag)
	assert.Equal(t, "file", fileFlag.Name)

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "dry-run", dryRunFlag.Name)
}

func TestWorkflowParser_BindToViper(t *testing.T) {
	parser := NewWorkflowParser(
		flags.WithStringFlag("file", "f", "", "Workflow file"),
	)

	cmd := &cobra.Command{
		Use: "workflow",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)

	assert.NoError(t, err)
	assert.NotNil(t, parser.viper)
}

func TestWorkflowParser_BindFlagsToViper(t *testing.T) {
	parser := NewWorkflowParser(
		flags.WithStringFlag("file", "f", "", "Workflow file"),
	)

	cmd := &cobra.Command{
		Use: "workflow",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindFlagsToViper(cmd, v)

	assert.NoError(t, err)
}

func TestWorkflowParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		setup    func(*cobra.Command)
		expected *WorkflowOptions
		wantErr  bool
	}{
		{
			name: "default values",
			args: []string{},
			expected: &WorkflowOptions{
				WorkflowName: "",
				FromStep:     "",
			},
		},
		{
			name: "with workflow name",
			args: []string{"deploy"},
			expected: &WorkflowOptions{
				WorkflowName: "deploy",
				FromStep:     "",
			},
		},
		{
			name: "with file flag",
			args: []string{"--file", "workflows.yaml", "deploy"},
			expected: &WorkflowOptions{
				WorkflowName: "deploy",
				FromStep:     "",
			},
		},
		{
			name: "with dry-run flag",
			args: []string{"--dry-run", "deploy"},
			expected: &WorkflowOptions{
				WorkflowName: "deploy",
				FromStep:     "",
			},
		},
		{
			name: "with from-step flag",
			args: []string{"--from-step", "step2", "deploy"},
			expected: &WorkflowOptions{
				WorkflowName: "deploy",
				FromStep:     "step2",
			},
		},
		{
			name: "with stack flag",
			args: []string{"--stack", "prod", "deploy"},
			expected: &WorkflowOptions{
				WorkflowName: "deploy",
				FromStep:     "",
			},
		},
		{
			name: "with multiple flags",
			args: []string{"--file", "workflows.yaml", "--dry-run", "--from-step", "step2", "--stack", "prod", "deploy"},
			expected: &WorkflowOptions{
				WorkflowName: "deploy",
				FromStep:     "step2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewWorkflowParser(
				flags.WithStringFlag("file", "f", "", "Workflow file"),
				flags.WithBoolFlag("dry-run", "", false, "Dry run mode"),
				flags.WithStringFlag("from-step", "", "", "Resume from step"),
				flags.WithStringFlag("stack", "s", "", "Stack"),
			)

			cmd := &cobra.Command{
				Use: "workflow",
			}
			parser.RegisterFlags(cmd)

			v := viper.New()
			err := parser.BindToViper(v)
			require.NoError(t, err)

			if tt.setup != nil {
				tt.setup(cmd)
			}

			ctx := context.Background()
			opts, err := parser.Parse(ctx, tt.args)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, opts)
			assert.Equal(t, tt.expected.WorkflowName, opts.WorkflowName)
			assert.Equal(t, tt.expected.FromStep, opts.FromStep)
		})
	}
}

func TestWorkflowParser_Parse_ExtractsPositionalArgs(t *testing.T) {
	parser := NewWorkflowParser()

	cmd := &cobra.Command{
		Use: "workflow",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()
	opts, err := parser.Parse(ctx, []string{"deploy", "extra1", "extra2"})

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "deploy", opts.WorkflowName)
	assert.Len(t, opts.GetPositionalArgs(), 3)
	assert.Equal(t, "deploy", opts.GetPositionalArgs()[0])
	assert.Equal(t, "extra1", opts.GetPositionalArgs()[1])
	assert.Equal(t, "extra2", opts.GetPositionalArgs()[2])
}

func TestWorkflowParser_Parse_EmptyWorkflowName(t *testing.T) {
	parser := NewWorkflowParser()

	cmd := &cobra.Command{
		Use: "workflow",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()
	opts, err := parser.Parse(ctx, []string{})

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "", opts.WorkflowName)
}

func TestWorkflowParser_IntegrationWithBuilder(t *testing.T) {
	// Test that builder-created parser works correctly
	builder := NewWorkflowOptionsBuilder().
		WithFile(false).
		WithDryRun().
		WithFromStep().
		WithStack(false)

	parser := builder.Build()

	cmd := &cobra.Command{
		Use: "workflow",
	}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()
	opts, err := parser.Parse(ctx, []string{"--file", "test.yaml", "--dry-run", "--from-step", "step1", "deploy"})

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "deploy", opts.WorkflowName)
	assert.Equal(t, "step1", opts.FromStep)
	assert.True(t, opts.DryRun)
	assert.Equal(t, "test.yaml", opts.File)
}
