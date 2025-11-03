package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBaseOptions(t *testing.T) {
	globalFlags := GlobalFlags{
		LogsLevel: "Debug",
		NoColor:   true,
	}
	positionalArgs := []string{"plan", "vpc"}
	passThroughArgs := []string{"-out=plan.tfplan"}

	interpreter := NewBaseOptions(globalFlags, positionalArgs, passThroughArgs)

	// Test global flags.
	assert.Equal(t, "Debug", interpreter.LogsLevel)
	assert.True(t, interpreter.NoColor)

	// Test positional args.
	assert.Equal(t, positionalArgs, interpreter.GetPositionalArgs())

	// Test pass-through args.
	assert.Equal(t, passThroughArgs, interpreter.GetPassThroughArgs())

	// Test GetGlobalFlags.
	globals := interpreter.GetGlobalFlags()
	assert.NotNil(t, globals)
	assert.Equal(t, "Debug", globals.LogsLevel)
}

func TestBaseOptions_GetPositionalArgs(t *testing.T) {
	tests := []struct {
		name           string
		positionalArgs []string
		want           []string
	}{
		{
			name:           "no positional args",
			positionalArgs: []string{},
			want:           []string{},
		},
		{
			name:           "one positional arg",
			positionalArgs: []string{"vpc"},
			want:           []string{"vpc"},
		},
		{
			name:           "two positional args",
			positionalArgs: []string{"plan", "vpc"},
			want:           []string{"plan", "vpc"},
		},
		{
			name:           "multiple positional args",
			positionalArgs: []string{"terraform", "plan", "vpc", "-s", "prod"},
			want:           []string{"terraform", "plan", "vpc", "-s", "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interpreter := NewBaseOptions(GlobalFlags{}, tt.positionalArgs, nil)
			got := interpreter.GetPositionalArgs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBaseOptions_GetPassThroughArgs(t *testing.T) {
	tests := []struct {
		name            string
		passThroughArgs []string
		want            []string
	}{
		{
			name:            "no pass-through args",
			passThroughArgs: []string{},
			want:            []string{},
		},
		{
			name:            "single pass-through arg",
			passThroughArgs: []string{"-out=plan.tfplan"},
			want:            []string{"-out=plan.tfplan"},
		},
		{
			name:            "multiple pass-through args",
			passThroughArgs: []string{"-out=plan.tfplan", "-target=aws_instance.web"},
			want:            []string{"-out=plan.tfplan", "-target=aws_instance.web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interpreter := NewBaseOptions(GlobalFlags{}, nil, tt.passThroughArgs)
			got := interpreter.GetPassThroughArgs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBaseOptions_GetGlobalFlags(t *testing.T) {
	globalFlags := GlobalFlags{
		LogsLevel:    "Trace",
		LogsFile:     "/tmp/logs.txt",
		NoColor:      true,
		ProfilerPort: 9090,
	}

	interpreter := NewBaseOptions(globalFlags, nil, nil)
	got := interpreter.GetGlobalFlags()

	assert.NotNil(t, got)
	assert.Equal(t, "Trace", got.LogsLevel)
	assert.Equal(t, "/tmp/logs.txt", got.LogsFile)
	assert.True(t, got.NoColor)
	assert.Equal(t, 9090, got.ProfilerPort)
}

func TestBaseOptions_Interface(t *testing.T) {
	// Test that BaseOptions implements CommandOptions interface.
	var _ CommandOptions = &BaseOptions{}

	interpreter := NewBaseOptions(GlobalFlags{}, []string{"vpc"}, []string{"-out=plan.tfplan"})

	// Test interface methods.
	assert.NotNil(t, interpreter.GetGlobalFlags())
	assert.Equal(t, []string{"vpc"}, interpreter.GetPositionalArgs())
	assert.Equal(t, []string{"-out=plan.tfplan"}, interpreter.GetPassThroughArgs())
}

func TestBaseOptions_Embedding(t *testing.T) {
	// Test that BaseOptions can be embedded in command-specific interpreters.
	type TerraformOptions struct {
		BaseOptions
		Stack  string
		DryRun bool
	}

	interpreter := TerraformOptions{
		BaseOptions: NewBaseOptions(
			GlobalFlags{LogsLevel: "Debug"},
			[]string{"plan", "vpc"},
			[]string{"-out=plan.tfplan"},
		),
		Stack:  "prod",
		DryRun: true,
	}

	// Test embedded fields (global flags).
	assert.Equal(t, "Debug", interpreter.LogsLevel)

	// Test embedded methods.
	assert.Equal(t, []string{"plan", "vpc"}, interpreter.GetPositionalArgs())
	assert.Equal(t, []string{"-out=plan.tfplan"}, interpreter.GetPassThroughArgs())

	// Test own fields.
	assert.Equal(t, "prod", interpreter.Stack)
	assert.True(t, interpreter.DryRun)

	// Test interface implementation.
	var _ CommandOptions = &interpreter
}

func TestBaseOptions_EmptyState(t *testing.T) {
	// Test zero value behavior.
	var interpreter BaseOptions

	// Should not panic.
	globals := interpreter.GetGlobalFlags()
	assert.NotNil(t, globals)

	positional := interpreter.GetPositionalArgs()
	assert.Nil(t, positional)

	passThrough := interpreter.GetPassThroughArgs()
	assert.Nil(t, passThrough)
}

func TestCommandOptionsInterface(t *testing.T) {
	// Test that the interface can be used with different implementations.
	tests := []struct {
		name        string
		interpreter CommandOptions
		description string
	}{
		{
			name: "BaseOptions",
			interpreter: &BaseOptions{
				GlobalFlags:     GlobalFlags{LogsLevel: "Info"},
				positionalArgs:  []string{"vpc"},
				passThroughArgs: []string{"-out=plan.tfplan"},
			},
			description: "Direct BaseOptions usage",
		},
		{
			name: "Embedded in custom struct",
			interpreter: &struct {
				BaseOptions
			}{
				BaseOptions: NewBaseOptions(
					GlobalFlags{LogsLevel: "Debug"},
					[]string{"plan"},
					nil,
				),
			},
			description: "BaseOptions embedded in anonymous struct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			// Test interface methods work (don't check for nil, just that they don't panic).
			assert.NotPanics(t, func() {
				tt.interpreter.GetGlobalFlags()
				tt.interpreter.GetPositionalArgs()
				tt.interpreter.GetPassThroughArgs()
			})
		})
	}
}
