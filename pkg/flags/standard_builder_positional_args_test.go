package flags_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/describe"
	"github.com/cloudposse/atmos/pkg/flags/list"
	"github.com/cloudposse/atmos/pkg/flags/validate"
)

func TestStandardOptionsBuilder_WithPositionalArgs_Build(t *testing.T) {
	// Test that WithPositionalArgs properly configures the builder and Build() applies it.
	specs, validator, usage := list.NewListSettingsPositionalArgsBuilder().
		WithComponent(false).
		Build()

	parser := flags.NewStandardOptionsBuilder().
		WithProcessTemplates(true).
		WithPositionalArgs(specs, validator, usage).
		Build()

	// Verify parser was created.
	assert.NotNil(t, parser)

	// Verify positional args work after Build().
	opts, err := parser.Parse(context.Background(), []string{"vpc"})
	require.NoError(t, err)
	assert.Equal(t, "vpc", opts.Component)
	assert.Equal(t, []string{"vpc"}, opts.GetPositionalArgs())
}

func TestStandardOptionsBuilder_WithPositionalArgs_RequiredComponent(t *testing.T) {
	// Test required component positional arg through builder.
	specs, validator, usage := describe.NewComponentPositionalArgsBuilder().
		WithComponent(true).
		Build()

	parser := flags.NewStandardOptionsBuilder().
		WithPositionalArgs(specs, validator, usage).
		Build()

	// Should succeed with component.
	opts, err := parser.Parse(context.Background(), []string{"vpc"})
	require.NoError(t, err)
	assert.Equal(t, "vpc", opts.Component)

	// Should fail without component.
	_, err = parser.Parse(context.Background(), []string{})
	assert.Error(t, err)
}

func TestStandardOptionsBuilder_WithPositionalArgs_OptionalSchemaType(t *testing.T) {
	// Test optional schemaType positional arg through builder.
	specs, validator, usage := validate.NewValidateSchemaPositionalArgsBuilder().
		WithSchemaType(false).
		Build()

	parser := flags.NewStandardOptionsBuilder().
		WithPositionalArgs(specs, validator, usage).
		Build()

	// Should succeed with schemaType.
	opts, err := parser.Parse(context.Background(), []string{"jsonschema"})
	require.NoError(t, err)
	assert.Equal(t, "jsonschema", opts.SchemaType)

	// Should succeed without schemaType.
	opts, err = parser.Parse(context.Background(), []string{})
	require.NoError(t, err)
	assert.Equal(t, "", opts.SchemaType)
}

func TestStandardOptionsBuilder_WithPositionalArgs_Key(t *testing.T) {
	// Test key positional arg through builder.
	specs, validator, usage := list.NewListComponentsPositionalArgsBuilder().
		WithKey(true).
		Build()

	parser := flags.NewStandardOptionsBuilder().
		WithPositionalArgs(specs, validator, usage).
		Build()

	// Should succeed with key.
	opts, err := parser.Parse(context.Background(), []string{"region"})
	require.NoError(t, err)
	assert.Equal(t, "region", opts.Key)

	// Should fail without key.
	_, err = parser.Parse(context.Background(), []string{})
	assert.Error(t, err)
}

func TestStandardOptionsBuilder_WithoutPositionalArgs(t *testing.T) {
	// Test that Build() works without WithPositionalArgs.
	parser := flags.NewStandardOptionsBuilder().
		WithProcessTemplates(true).
		Build()

	assert.NotNil(t, parser)

	// Should parse successfully without positional args.
	opts, err := parser.Parse(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, opts.GetPositionalArgs())
}

func TestStandardOptionsBuilder_Build_ExercisesSetPositionalArgs(t *testing.T) {
	// This test specifically exercises the SetPositionalArgs() code path
	// that was showing 0% coverage.
	specs, validator, usage := describe.NewDependentsPositionalArgsBuilder().
		WithComponent(true).
		Build()

	builder := flags.NewStandardOptionsBuilder().
		WithPositionalArgs(specs, validator, usage)

	// Build() should call SetPositionalArgs() internally.
	parser := builder.Build()

	// Verify positional args configuration was applied.
	opts, err := parser.Parse(context.Background(), []string{"vpc"})
	require.NoError(t, err)
	assert.Equal(t, "vpc", opts.Component)
}

func TestStandardOptionsBuilder_WithPositionalArgs_ValidationErrors(t *testing.T) {
	// Test that validation errors from positional args are properly returned.
	specs, validator, usage := describe.NewComponentPositionalArgsBuilder().
		WithComponent(true).
		Build()

	parser := flags.NewStandardOptionsBuilder().
		WithPositionalArgs(specs, validator, usage).
		Build()

	// Too many args should error.
	_, err := parser.Parse(context.Background(), []string{"vpc", "ecs"})
	assert.Error(t, err)

	// Missing required arg should error.
	_, err = parser.Parse(context.Background(), []string{})
	assert.Error(t, err)
}

func TestStandardOptionsBuilder_WithPositionalArgs_AllBuilders(t *testing.T) {
	// Test all 6 positional args builders through the Build() path.
	tests := []struct {
		name      string
		specs     []*flags.PositionalArgSpec
		validator func(*testing.T, *flags.StandardOptions)
		args      []string
	}{
		{
			name: "ListSettings",
			specs: func() []*flags.PositionalArgSpec {
				s, _, _ := list.NewListSettingsPositionalArgsBuilder().WithComponent(false).Build()
				return s
			}(),
			validator: func(t *testing.T, opts *flags.StandardOptions) {
				assert.Equal(t, "vpc", opts.Component)
			},
			args: []string{"vpc"},
		},
		{
			name: "DescribeComponent",
			specs: func() []*flags.PositionalArgSpec {
				s, _, _ := describe.NewComponentPositionalArgsBuilder().WithComponent(true).Build()
				return s
			}(),
			validator: func(t *testing.T, opts *flags.StandardOptions) {
				assert.Equal(t, "vpc", opts.Component)
			},
			args: []string{"vpc"},
		},
		{
			name: "DescribeDependents",
			specs: func() []*flags.PositionalArgSpec {
				s, _, _ := describe.NewDependentsPositionalArgsBuilder().WithComponent(true).Build()
				return s
			}(),
			validator: func(t *testing.T, opts *flags.StandardOptions) {
				assert.Equal(t, "vpc", opts.Component)
			},
			args: []string{"vpc"},
		},
		{
			name: "ValidateSchema",
			specs: func() []*flags.PositionalArgSpec {
				s, _, _ := validate.NewValidateSchemaPositionalArgsBuilder().WithSchemaType(false).Build()
				return s
			}(),
			validator: func(t *testing.T, opts *flags.StandardOptions) {
				assert.Equal(t, "jsonschema", opts.SchemaType)
			},
			args: []string{"jsonschema"},
		},
		{
			name: "ListKeys",
			specs: func() []*flags.PositionalArgSpec {
				s, _, _ := list.NewListKeysPositionalArgsBuilder().WithComponent(true).Build()
				return s
			}(),
			validator: func(t *testing.T, opts *flags.StandardOptions) {
				assert.Equal(t, "vpc", opts.Component)
			},
			args: []string{"vpc"},
		},
		{
			name: "ListComponents",
			specs: func() []*flags.PositionalArgSpec {
				s, _, _ := list.NewListComponentsPositionalArgsBuilder().WithKey(true).Build()
				return s
			}(),
			validator: func(t *testing.T, opts *flags.StandardOptions) {
				assert.Equal(t, "region", opts.Key)
			},
			args: []string{"region"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build validator from specs.
			builder := flags.NewPositionalArgsBuilder()
			for _, spec := range tt.specs {
				builder.AddArg(spec)
			}
			_, validator, usage := builder.Build()

			// Create parser through builder (exercises SetPositionalArgs).
			parser := flags.NewStandardOptionsBuilder().
				WithPositionalArgs(tt.specs, validator, usage).
				Build()

			// Parse and validate.
			opts, err := parser.Parse(context.Background(), tt.args)
			require.NoError(t, err)
			tt.validator(t, opts)
		})
	}
}
