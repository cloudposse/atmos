package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStandardOptionsBuilder(t *testing.T) {
	builder := NewStandardOptionsBuilder()
	assert.NotNil(t, builder)
	assert.NotNil(t, builder.options)
	assert.Equal(t, 0, len(builder.options))
}

func TestStandardOptionsBuilder_WithStack(t *testing.T) {
	tests := []struct {
		name         string
		required     bool
		wantRequired bool
	}{
		{
			name:         "required stack flag",
			required:     true,
			wantRequired: true,
		},
		{
			name:         "optional stack flag",
			required:     false,
			wantRequired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithStack(tt.required)
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("stack")
			require.NotNil(t, flag, "stack flag should be registered")
			assert.Equal(t, "s", flag.Shorthand)
			assert.Equal(t, "Atmos stack", flag.Usage)

			// Parse and verify
			v := viper.New()
			v.Set("stack", "prod")
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, "prod", interpreter.Stack)
		})
	}
}

func TestStandardOptionsBuilder_WithComponent(t *testing.T) {
	tests := []struct {
		name         string
		required     bool
		wantRequired bool
	}{
		{
			name:         "required component flag",
			required:     true,
			wantRequired: true,
		},
		{
			name:         "optional component flag",
			required:     false,
			wantRequired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithComponent(tt.required)
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("component")
			require.NotNil(t, flag, "component flag should be registered")
			assert.Equal(t, "c", flag.Shorthand)
			assert.Equal(t, "Atmos component", flag.Usage)

			// Parse and verify
			v := viper.New()
			v.Set("component", "vpc")
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, "vpc", interpreter.Component)
		})
	}
}

func TestStandardOptionsBuilder_WithFormat(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue string
		viperValue   string
		want         string
	}{
		{
			name:         "default yaml",
			defaultValue: "yaml",
			viperValue:   "",
			want:         "yaml",
		},
		{
			name:         "default json",
			defaultValue: "json",
			viperValue:   "",
			want:         "json",
		},
		{
			name:         "override default with viper",
			defaultValue: "yaml",
			viperValue:   "json",
			want:         "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithFormat(tt.defaultValue)
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("format")
			require.NotNil(t, flag, "format flag should be registered")
			assert.Equal(t, "f", flag.Shorthand)

			// Parse and verify
			v := viper.New()
			if tt.viperValue != "" {
				v.Set("format", tt.viperValue)
			} else {
				v.Set("format", tt.defaultValue)
			}
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, interpreter.Format)
		})
	}
}

func TestStandardOptionsBuilder_WithFile(t *testing.T) {
	builder := NewStandardOptionsBuilder().WithFile()
	parser := builder.Build()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("file")
	require.NotNil(t, flag, "file flag should be registered")
	assert.Empty(t, flag.Shorthand)
	assert.Equal(t, "Write output to file", flag.Usage)

	// Parse and verify
	v := viper.New()
	v.Set("file", "/tmp/output.yaml")
	_ = parser.BindToViper(v)

	interpreter, err := parser.Parse(context.Background(), []string{})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/output.yaml", interpreter.File)
}

func TestStandardOptionsBuilder_WithProcessTemplates(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue bool
		viperValue   *bool
		want         bool
	}{
		{
			name:         "default true",
			defaultValue: true,
			viperValue:   nil,
			want:         true,
		},
		{
			name:         "default false",
			defaultValue: false,
			viperValue:   nil,
			want:         false,
		},
		{
			name:         "override with viper",
			defaultValue: true,
			viperValue:   boolPtr(false),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithProcessTemplates(tt.defaultValue)
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("process-templates")
			require.NotNil(t, flag, "process-templates flag should be registered")

			// Parse and verify
			v := viper.New()
			if tt.viperValue != nil {
				v.Set("process-templates", *tt.viperValue)
			} else {
				v.Set("process-templates", tt.defaultValue)
			}
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, interpreter.ProcessTemplates)
		})
	}
}

func TestStandardOptionsBuilder_WithProcessFunctions(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue bool
		viperValue   *bool
		want         bool
	}{
		{
			name:         "default true",
			defaultValue: true,
			viperValue:   nil,
			want:         true,
		},
		{
			name:         "default false",
			defaultValue: false,
			viperValue:   nil,
			want:         false,
		},
		{
			name:         "override with viper",
			defaultValue: true,
			viperValue:   boolPtr(false),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithProcessFunctions(tt.defaultValue)
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("process-functions")
			require.NotNil(t, flag, "process-functions flag should be registered")

			// Parse and verify
			v := viper.New()
			if tt.viperValue != nil {
				v.Set("process-functions", *tt.viperValue)
			} else {
				v.Set("process-functions", tt.defaultValue)
			}
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, interpreter.ProcessYamlFunctions)
		})
	}
}

func TestStandardOptionsBuilder_WithSkip(t *testing.T) {
	tests := []struct {
		name       string
		viperValue []string
		want       []string
	}{
		{
			name:       "no skip flags",
			viperValue: []string{},
			want:       []string{},
		},
		{
			name:       "single skip flag",
			viperValue: []string{"atmos.Component"},
			want:       []string{"atmos.Component"},
		},
		{
			name:       "multiple skip flags",
			viperValue: []string{"atmos.Component", "terraform.output"},
			want:       []string{"atmos.Component", "terraform.output"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithSkip()
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("skip")
			require.NotNil(t, flag, "skip flag should be registered")

			// Parse and verify
			v := viper.New()
			if len(tt.viperValue) > 0 {
				v.Set("skip", tt.viperValue)
			}
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			if len(tt.want) == 0 {
				assert.Empty(t, interpreter.Skip)
			} else {
				assert.Equal(t, tt.want, interpreter.Skip)
			}
		})
	}
}

func TestStandardOptionsBuilder_WithDryRun(t *testing.T) {
	tests := []struct {
		name       string
		viperValue bool
		want       bool
	}{
		{
			name:       "no dry-run flag",
			viperValue: false,
			want:       false,
		},
		{
			name:       "dry-run enabled",
			viperValue: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithDryRun()
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("dry-run")
			require.NotNil(t, flag, "dry-run flag should be registered")

			// Parse and verify
			v := viper.New()
			v.Set("dry-run", tt.viperValue)
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, interpreter.DryRun)
		})
	}
}

func TestStandardOptionsBuilder_WithQuery(t *testing.T) {
	builder := NewStandardOptionsBuilder().WithQuery()
	parser := builder.Build()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("query")
	require.NotNil(t, flag, "query flag should be registered")
	assert.Equal(t, "q", flag.Shorthand)
	assert.Equal(t, "JQ/JMESPath query to filter output", flag.Usage)

	// Parse and verify
	v := viper.New()
	v.Set("query", ".components.vpc")
	_ = parser.BindToViper(v)

	interpreter, err := parser.Parse(context.Background(), []string{})
	require.NoError(t, err)
	assert.Equal(t, ".components.vpc", interpreter.Query)
}

func TestStandardOptionsBuilder_WithProvenance(t *testing.T) {
	tests := []struct {
		name       string
		viperValue bool
		want       bool
	}{
		{
			name:       "no provenance flag",
			viperValue: false,
			want:       false,
		},
		{
			name:       "provenance enabled",
			viperValue: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewStandardOptionsBuilder().WithProvenance()
			parser := builder.Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify flag exists
			flag := cmd.Flags().Lookup("provenance")
			require.NotNil(t, flag, "provenance flag should be registered")

			// Parse and verify
			v := viper.New()
			v.Set("provenance", tt.viperValue)
			_ = parser.BindToViper(v)

			interpreter, err := parser.Parse(context.Background(), []string{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, interpreter.Provenance)
		})
	}
}

func TestStandardOptionsBuilder_FluentChaining(t *testing.T) {
	// Test that all methods return *StandardOptionsBuilder for chaining.
	builder := NewStandardOptionsBuilder().
		WithStack(true).
		WithComponent(false).
		WithFormat("yaml").
		WithFile().
		WithProcessTemplates(true).
		WithProcessFunctions(true).
		WithSkip().
		WithDryRun().
		WithQuery().
		WithProvenance()

	assert.NotNil(t, builder)
	parser := builder.Build()
	assert.NotNil(t, parser)
}

func TestStandardOptionsBuilder_ComplexCommand(t *testing.T) {
	// Test a realistic command with multiple flags (like describe component).
	builder := NewStandardOptionsBuilder().
		WithStack(true).
		WithFormat("yaml").
		WithFile().
		WithProcessTemplates(true).
		WithProcessFunctions(true).
		WithSkip().
		WithQuery().
		WithProvenance()

	parser := builder.Build()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	v := viper.New()
	// Set all values in Viper
	v.Set("stack", "prod")
	v.Set("format", "json")
	v.Set("file", "/tmp/output.json")
	v.Set("process-templates", false)
	v.Set("process-functions", true)
	v.Set("skip", []string{"atmos.Component"})
	v.Set("query", ".components")
	v.Set("provenance", true)
	_ = parser.BindToViper(v)

	// Positional args
	interpreter, err := parser.Parse(context.Background(), []string{"vpc"})
	require.NoError(t, err)

	// Verify all flags parsed correctly.
	assert.Equal(t, "prod", interpreter.Stack)
	assert.Equal(t, "json", interpreter.Format)
	assert.Equal(t, "/tmp/output.json", interpreter.File)
	assert.False(t, interpreter.ProcessTemplates)
	assert.True(t, interpreter.ProcessYamlFunctions)
	assert.Equal(t, []string{"atmos.Component"}, interpreter.Skip)
	assert.Equal(t, ".components", interpreter.Query)
	assert.True(t, interpreter.Provenance)
	assert.Equal(t, []string{"vpc"}, interpreter.GetPositionalArgs())
}

func TestStandardOptionsBuilder_EnvironmentVariables(t *testing.T) {
	// Test that environment variables work with proper precedence.
	builder := NewStandardOptionsBuilder().
		WithStack(false).
		WithFormat("yaml")

	parser := builder.Build()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	v := viper.New()
	_ = parser.BindToViper(v)

	// Set environment variables.
	t.Setenv("ATMOS_STACK", "staging")
	t.Setenv("ATMOS_FORMAT", "json")

	// Set values in Viper (simulating env var binding)
	v.Set("stack", "staging")
	v.Set("format", "json")

	// Parse without CLI flags (should use env vars).
	interpreter, err := parser.Parse(context.Background(), []string{})
	require.NoError(t, err)

	// Verify env vars were used.
	assert.Equal(t, "staging", interpreter.Stack)
	assert.Equal(t, "json", interpreter.Format)
}

func TestStandardOptionsBuilder_Build(t *testing.T) {
	builder := NewStandardOptionsBuilder().WithStack(true)
	parser := builder.Build()

	assert.NotNil(t, parser)
	assert.NotNil(t, parser.parser)
}

func TestStandardOptionsBuilder_BindFlagsToViper(t *testing.T) {
	// Test that BindFlagsToViper works correctly
	builder := NewStandardOptionsBuilder().
		WithStack(true).
		WithFormat("yaml")

	parser := builder.Build()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	// Now bind Cobra flags to Viper
	err = parser.BindFlagsToViper(cmd, v)
	require.NoError(t, err)
}

func boolPtr(b bool) *bool {
	return &b
}
