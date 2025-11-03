package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeStacksParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *DescribeStacksOptions
		wantErr  bool
	}{
		{
			name: "default values",
			args: []string{},
			expected: &DescribeStacksOptions{
				Stack:              "",
				Format:             "yaml",
				File:               "",
				ProcessTemplates:   true,
				ProcessFunctions:   true,
				Components:         []string{},
				ComponentTypes:     []string{},
				Sections:           []string{},
				IncludeEmptyStacks: false,
				Skip:               []string{},
				Query:              "",
			},
		},
		{
			name: "with stack flag",
			args: []string{"--stack", "prod"},
			expected: &DescribeStacksOptions{
				Stack:            "prod",
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
			},
		},
		{
			name: "with format flag",
			args: []string{"--format", "json"},
			expected: &DescribeStacksOptions{
				Format:           "json",
				ProcessTemplates: true,
				ProcessFunctions: true,
			},
		},
		{
			name: "with file flag",
			args: []string{"--file", "output.yaml"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				File:             "output.yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
			},
		},
		{
			name: "with process-templates false",
			args: []string{"--process-templates=false"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: false,
				ProcessFunctions: true,
			},
		},
		{
			name: "with process-functions false",
			args: []string{"--process-functions=false"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: false,
			},
		},
		{
			name: "with components filter",
			args: []string{"--components", "vpc,rds"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
				Components:       []string{"vpc", "rds"},
			},
		},
		{
			name: "with component-types filter",
			args: []string{"--component-types", "terraform,helmfile"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
				ComponentTypes:   []string{"terraform", "helmfile"},
			},
		},
		{
			name: "with sections filter",
			args: []string{"--sections", "vars,backend"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
				Sections:         []string{"vars", "backend"},
			},
		},
		{
			name: "with include-empty-stacks",
			args: []string{"--include-empty-stacks"},
			expected: &DescribeStacksOptions{
				Format:             "yaml",
				ProcessTemplates:   true,
				ProcessFunctions:   true,
				IncludeEmptyStacks: true,
			},
		},
		{
			name: "with skip functions",
			args: []string{"--skip", "terraform.output,store.get"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             []string{"terraform.output", "store.get"},
			},
		},
		{
			name: "with query",
			args: []string{"--query", ".components.vpc"},
			expected: &DescribeStacksOptions{
				Format:           "yaml",
				ProcessTemplates: true,
				ProcessFunctions: true,
				Query:            ".components.vpc",
			},
		},
		{
			name: "with multiple flags",
			args: []string{
				"--stack", "dev",
				"--format", "json",
				"--sections", "vars,env",
				"--components", "vpc",
				"--include-empty-stacks",
			},
			expected: &DescribeStacksOptions{
				Stack:              "dev",
				Format:             "json",
				ProcessTemplates:   true,
				ProcessFunctions:   true,
				Sections:           []string{"vars", "env"},
				Components:         []string{"vpc"},
				IncludeEmptyStacks: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test.
			v := viper.New()

			// Create parser and command.
			parser := NewDescribeStacksParser()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)
			require.NoError(t, parser.BindToViper(v))

			// Set args and parse.
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())

			// Parse options.
			opts, err := parser.Parse(context.Background(), tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, opts)

			// Check expected values.
			assert.Equal(t, tt.expected.Stack, opts.Stack)
			assert.Equal(t, tt.expected.Format, opts.Format)
			assert.Equal(t, tt.expected.File, opts.File)
			assert.Equal(t, tt.expected.ProcessTemplates, opts.ProcessTemplates)
			assert.Equal(t, tt.expected.ProcessFunctions, opts.ProcessFunctions)
			assert.Equal(t, tt.expected.IncludeEmptyStacks, opts.IncludeEmptyStacks)
			assert.Equal(t, tt.expected.Query, opts.Query)

			// Check slices (use ElementsMatch for order-independent comparison).
			if len(tt.expected.Components) > 0 {
				assert.ElementsMatch(t, tt.expected.Components, opts.Components)
			}
			if len(tt.expected.ComponentTypes) > 0 {
				assert.ElementsMatch(t, tt.expected.ComponentTypes, opts.ComponentTypes)
			}
			if len(tt.expected.Sections) > 0 {
				assert.ElementsMatch(t, tt.expected.Sections, opts.Sections)
			}
			if len(tt.expected.Skip) > 0 {
				assert.ElementsMatch(t, tt.expected.Skip, opts.Skip)
			}
		})
	}
}

func TestDescribeStacksBuilder_Methods(t *testing.T) {
	t.Run("NewDescribeStacksOptionsBuilder creates empty builder", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder()
		assert.NotNil(t, builder)
		assert.NotNil(t, builder.options)
	})

	t.Run("WithStack adds stack flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithStack()
		assert.NotNil(t, builder)
	})

	t.Run("WithFormat adds format flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithFormat()
		assert.NotNil(t, builder)
	})

	t.Run("WithFile adds file flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithFile()
		assert.NotNil(t, builder)
	})

	t.Run("WithProcessTemplates adds process-templates flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithProcessTemplates()
		assert.NotNil(t, builder)
	})

	t.Run("WithProcessFunctions adds process-functions flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithProcessFunctions()
		assert.NotNil(t, builder)
	})

	t.Run("WithComponents adds components flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithComponents()
		assert.NotNil(t, builder)
	})

	t.Run("WithComponentTypes adds component-types flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithComponentTypes()
		assert.NotNil(t, builder)
	})

	t.Run("WithSections adds sections flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithSections()
		assert.NotNil(t, builder)
	})

	t.Run("WithIncludeEmptyStacks adds include-empty-stacks flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithIncludeEmptyStacks()
		assert.NotNil(t, builder)
	})

	t.Run("WithSkip adds skip flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithSkip()
		assert.NotNil(t, builder)
	})

	t.Run("WithQuery adds query flag", func(t *testing.T) {
		builder := NewDescribeStacksOptionsBuilder().WithQuery()
		assert.NotNil(t, builder)
	})

	t.Run("Build creates parser", func(t *testing.T) {
		parser := NewDescribeStacksOptionsBuilder().
			WithStack().
			WithFormat().
			WithSections().
			Build()
		assert.NotNil(t, parser)
		assert.NotNil(t, parser.parser)
	})
}

func TestDescribeStacksParser_RegisterFlagsAndBindToViper(t *testing.T) {
	parser := NewDescribeStacksParser()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Check flags were registered.
	assert.NotNil(t, cmd.Flags().Lookup("stack"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))
	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("process-templates"))
	assert.NotNil(t, cmd.Flags().Lookup("process-functions"))
	assert.NotNil(t, cmd.Flags().Lookup("components"))
	assert.NotNil(t, cmd.Flags().Lookup("component-types"))
	assert.NotNil(t, cmd.Flags().Lookup("sections"))
	assert.NotNil(t, cmd.Flags().Lookup("include-empty-stacks"))
	assert.NotNil(t, cmd.Flags().Lookup("skip"))
	assert.NotNil(t, cmd.Flags().Lookup("query"))

	// Check binding to viper.
	v := viper.New()
	err := parser.BindToViper(v)
	assert.NoError(t, err)
}
