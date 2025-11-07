package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/global"
)

func TestAuthExecOptions_GetPositionalArgs(t *testing.T) {
	opts := &AuthExecOptions{
		PositionalArgs: []string{"command", "arg1", "arg2"},
	}

	args := opts.GetPositionalArgs()

	assert.Equal(t, []string{"command", "arg1", "arg2"}, args)
}

func TestAuthExecOptions_GetPositionalArgs_Empty(t *testing.T) {
	opts := &AuthExecOptions{
		PositionalArgs: []string{},
	}

	args := opts.GetPositionalArgs()

	assert.Empty(t, args)
}

func TestAuthExecOptions_GetSeparatedArgs(t *testing.T) {
	opts := &AuthExecOptions{
		SeparatedArgs: []string{"--flag", "value"},
	}

	args := opts.GetSeparatedArgs()

	assert.Equal(t, []string{"--flag", "value"}, args)
}

func TestAuthExecOptions_GetSeparatedArgs_Empty(t *testing.T) {
	opts := &AuthExecOptions{
		SeparatedArgs: []string{},
	}

	args := opts.GetSeparatedArgs()

	assert.Empty(t, args)
}

func TestAuthShellOptions_GetPositionalArgs(t *testing.T) {
	opts := &AuthShellOptions{
		PositionalArgs: []string{"shell", "arg1"},
	}

	args := opts.GetPositionalArgs()

	assert.Equal(t, []string{"shell", "arg1"}, args)
}

func TestAuthShellOptions_GetPositionalArgs_Empty(t *testing.T) {
	opts := &AuthShellOptions{
		PositionalArgs: []string{},
	}

	args := opts.GetPositionalArgs()

	assert.Empty(t, args)
}

func TestAuthShellOptions_GetSeparatedArgs(t *testing.T) {
	opts := &AuthShellOptions{
		SeparatedArgs: []string{"--flag", "value"},
	}

	args := opts.GetSeparatedArgs()

	assert.Equal(t, []string{"--flag", "value"}, args)
}

func TestAuthShellOptions_GetSeparatedArgs_Empty(t *testing.T) {
	opts := &AuthShellOptions{
		SeparatedArgs: []string{},
	}

	args := opts.GetSeparatedArgs()

	assert.Empty(t, args)
}

func TestNewIdentityFlag(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{
			name:     "non-empty value",
			value:    "my-identity",
			expected: "my-identity",
		},
		{
			name:     "empty value",
			value:    "",
			expected: "",
		},
		{
			name:     "selector value",
			value:    cfg.IdentityFlagSelectValue,
			expected: cfg.IdentityFlagSelectValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := NewIdentityFlag(tt.value)

			assert.Equal(t, tt.expected, flag.Value())
		})
	}
}

func TestIdentityFlag_Value(t *testing.T) {
	flag := NewIdentityFlag("test-identity")

	value := flag.Value()

	assert.Equal(t, "test-identity", value)
}

func TestIdentityFlag_IsInteractiveSelector(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "selector value",
			value:    cfg.IdentityFlagSelectValue,
			expected: true,
		},
		{
			name:     "regular value",
			value:    "my-identity",
			expected: false,
		},
		{
			name:     "empty value",
			value:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := NewIdentityFlag(tt.value)

			result := flag.IsInteractiveSelector()

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdentityFlag_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "empty value",
			value:    "",
			expected: true,
		},
		{
			name:     "non-empty value",
			value:    "my-identity",
			expected: false,
		},
		{
			name:     "selector value",
			value:    cfg.IdentityFlagSelectValue,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := NewIdentityFlag(tt.value)

			result := flag.IsEmpty()

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAuthExecOptions_EmbeddedGlobalFlags(t *testing.T) {
	opts := &AuthExecOptions{
		Flags: global.Flags{
			BasePath: "/test/path",
			NoColor:  true,
		},
		Identity:       NewIdentityFlag("test-identity"),
		PositionalArgs: []string{"command"},
		SeparatedArgs:  []string{"--flag"},
	}

	// Test that embedded global flags are accessible
	assert.Equal(t, "/test/path", opts.BasePath)
	assert.True(t, opts.NoColor)
	assert.Equal(t, "test-identity", opts.Identity.Value())
}

func TestAuthShellOptions_EmbeddedGlobalFlags(t *testing.T) {
	opts := &AuthShellOptions{
		Flags: global.Flags{
			BasePath: "/test/path",
			NoColor:  true,
		},
		Identity:       NewIdentityFlag("test-identity"),
		Shell:          "/bin/bash",
		PositionalArgs: []string{"-c", "echo hello"},
		SeparatedArgs:  []string{},
	}

	// Test that embedded global flags are accessible
	assert.Equal(t, "/test/path", opts.BasePath)
	assert.True(t, opts.NoColor)
	assert.Equal(t, "test-identity", opts.Identity.Value())
	assert.Equal(t, "/bin/bash", opts.Shell)
}
