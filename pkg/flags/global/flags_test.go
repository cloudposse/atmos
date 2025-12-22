package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFlags(t *testing.T) {
	flags := NewFlags()

	// Test default values.
	assert.Equal(t, "Warning", flags.LogsLevel, "LogsLevel default")
	assert.Equal(t, "/dev/stderr", flags.LogsFile, "LogsFile default")
	assert.False(t, flags.NoColor, "NoColor default")
	assert.Equal(t, 6060, flags.ProfilerPort, "ProfilerPort default")
	assert.Equal(t, "localhost", flags.ProfilerHost, "ProfilerHost default")
	assert.Equal(t, "cpu", flags.ProfileType, "ProfileType default")
	assert.False(t, flags.Heatmap, "Heatmap default")
	assert.Equal(t, "bar", flags.HeatmapMode, "HeatmapMode default")

	// Test zero values for optional fields.
	assert.Empty(t, flags.Chdir, "Chdir should be empty")
	assert.Empty(t, flags.BasePath, "BasePath should be empty")
	assert.Nil(t, flags.Config, "Config should be nil")
	assert.Nil(t, flags.ConfigPath, "ConfigPath should be nil")
}

func TestFlags_GetGlobalFlags(t *testing.T) {
	flags := Flags{
		LogsLevel: "Debug",
		NoColor:   true,
	}

	got := flags.GetGlobalFlags()
	assert.NotNil(t, got, "GetGlobalFlags should return non-nil")
	assert.Equal(t, "Debug", got.LogsLevel)
	assert.True(t, got.NoColor)
}

func TestFlags_AllFields(t *testing.T) {
	// Test that we can set all fields without errors.
	flags := Flags{
		Chdir:           "/tmp/test",
		BasePath:        "/base",
		Config:          []string{"config1.yaml", "config2.yaml"},
		ConfigPath:      []string{"/path1", "/path2"},
		LogsLevel:       "Trace",
		LogsFile:        "/tmp/logs.txt",
		NoColor:         true,
		Pager:           NewPagerSelector("less", true),
		Identity:        NewIdentitySelector("prod-admin", true),
		ProfilerEnabled: true,
		ProfilerPort:    9090,
		ProfilerHost:    "0.0.0.0",
		ProfileFile:     "/tmp/profile.out",
		ProfileType:     "heap",
		Heatmap:         true,
		HeatmapMode:     "sparkline",
		RedirectStderr:  "/dev/null",
		Version:         true,
	}

	// Verify all fields are set correctly.
	assert.Equal(t, "/tmp/test", flags.Chdir)
	assert.Equal(t, "/base", flags.BasePath)
	assert.Equal(t, []string{"config1.yaml", "config2.yaml"}, flags.Config)
	assert.Equal(t, []string{"/path1", "/path2"}, flags.ConfigPath)
	assert.Equal(t, "Trace", flags.LogsLevel)
	assert.Equal(t, "/tmp/logs.txt", flags.LogsFile)
	assert.True(t, flags.NoColor)
	assert.True(t, flags.Pager.IsEnabled())
	assert.Equal(t, "less", flags.Pager.Pager())
	assert.Equal(t, "prod-admin", flags.Identity.Value())
	assert.True(t, flags.ProfilerEnabled)
	assert.Equal(t, 9090, flags.ProfilerPort)
	assert.Equal(t, "0.0.0.0", flags.ProfilerHost)
	assert.Equal(t, "/tmp/profile.out", flags.ProfileFile)
	assert.Equal(t, "heap", flags.ProfileType)
	assert.True(t, flags.Heatmap)
	assert.Equal(t, "sparkline", flags.HeatmapMode)
	assert.Equal(t, "/dev/null", flags.RedirectStderr)
	assert.True(t, flags.Version)
}

func TestFlags_Embedding(t *testing.T) {
	// Test that Flags can be embedded in other structs.
	type TestInterpreter struct {
		Flags
		Stack string
	}

	interpreter := TestInterpreter{
		Flags: Flags{
			LogsLevel: "Debug",
			NoColor:   true,
		},
		Stack: "prod",
	}

	// Test direct field access (embedding benefit).
	assert.Equal(t, "Debug", interpreter.LogsLevel, "Should access embedded LogsLevel")
	assert.True(t, interpreter.NoColor, "Should access embedded NoColor")
	assert.Equal(t, "prod", interpreter.Stack, "Should access own field")

	// Test GetGlobalFlags method.
	globals := interpreter.GetGlobalFlags()
	assert.Equal(t, "Debug", globals.LogsLevel)
	assert.True(t, globals.NoColor)
}

func TestFlags_ZeroValues(t *testing.T) {
	// Test zero value behavior.
	var flags Flags

	// String fields should be empty.
	assert.Empty(t, flags.Chdir)
	assert.Empty(t, flags.BasePath)
	assert.Empty(t, flags.LogsLevel)
	assert.Empty(t, flags.LogsFile)
	assert.Empty(t, flags.ProfilerHost)
	assert.Empty(t, flags.ProfileFile)
	assert.Empty(t, flags.ProfileType)
	assert.Empty(t, flags.HeatmapMode)
	assert.Empty(t, flags.RedirectStderr)

	// Bool fields should be false.
	assert.False(t, flags.NoColor)
	assert.False(t, flags.ProfilerEnabled)
	assert.False(t, flags.Heatmap)
	assert.False(t, flags.Version)

	// Int fields should be zero.
	assert.Zero(t, flags.ProfilerPort)

	// Slice fields should be nil.
	assert.Nil(t, flags.Config)
	assert.Nil(t, flags.ConfigPath)

	// Special types should be zero values.
	assert.False(t, flags.Identity.IsProvided())
	assert.False(t, flags.Pager.IsProvided())
}

func TestFlags_CommonScenarios(t *testing.T) {
	tests := []struct {
		name        string
		flags       Flags
		description string
	}{
		{
			name: "default configuration",
			flags: Flags{
				LogsLevel:    "Warning",
				LogsFile:     "/dev/stderr",
				ProfilerPort: 6060,
				ProfilerHost: "localhost",
				HeatmapMode:  "bar",
			},
			description: "User didn't override any flags, using all defaults",
		},
		{
			name: "debug mode with no color",
			flags: Flags{
				LogsLevel: "Debug",
				NoColor:   true,
			},
			description: "User wants debug logging without colors (CI environment)",
		},
		{
			name: "profiling enabled",
			flags: Flags{
				ProfilerEnabled: true,
				ProfilerPort:    9090,
				ProfilerHost:    "0.0.0.0",
			},
			description: "User enabled profiling on custom port",
		},
		{
			name: "custom working directory",
			flags: Flags{
				Chdir:    "/tmp/workspace",
				BasePath: "/custom/base",
			},
			description: "User changed working directory and base path",
		},
		{
			name: "identity and pager configured",
			flags: Flags{
				Identity: NewIdentitySelector("prod-admin", true),
				Pager:    NewPagerSelector("less", true),
			},
			description: "User specified identity and pager",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Scenario: %s", tt.description)

			// Verify the flags can be created and accessed.
			assert.NotNil(t, tt.flags)
			assert.NotNil(t, tt.flags.GetGlobalFlags())
		})
	}
}

// TestIdentitySelector_IsInteractiveSelector tests the interactive selector detection.
func TestIdentitySelector_IsInteractiveSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		expected bool
	}{
		{
			name:     "empty value - not interactive",
			selector: NewIdentitySelector("", false),
			expected: false,
		},
		{
			name:     "normal value - not interactive",
			selector: NewIdentitySelector("prod-admin", true),
			expected: false,
		},
		{
			name:     "__SELECT__ value - interactive",
			selector: NewIdentitySelector("__SELECT__", true),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsInteractiveSelector()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestIdentitySelector_IsEmpty tests empty check.
func TestIdentitySelector_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		expected bool
	}{
		{
			name:     "empty value",
			selector: NewIdentitySelector("", false),
			expected: true,
		},
		{
			name:     "non-empty value",
			selector: NewIdentitySelector("prod-admin", true),
			expected: false,
		},
		{
			name:     "interactive selector value",
			selector: NewIdentitySelector("__SELECT__", true),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsEmpty()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestIdentitySelector_IsDisabled tests disabled check.
func TestIdentitySelector_IsDisabled(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		expected bool
	}{
		{
			name:     "empty value - not disabled",
			selector: NewIdentitySelector("", false),
			expected: false,
		},
		{
			name:     "normal identity - not disabled",
			selector: NewIdentitySelector("prod-admin", true),
			expected: false,
		},
		{
			name:     "interactive selector - not disabled",
			selector: NewIdentitySelector("__SELECT__", true),
			expected: false,
		},
		{
			name:     "disabled sentinel value - disabled",
			selector: NewIdentitySelector("__DISABLED__", true),
			expected: true,
		},
		{
			name:     "disabled sentinel but not provided - not disabled",
			selector: NewIdentitySelector("__DISABLED__", false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsDisabled()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestPagerSelector_Value tests value retrieval.
func TestPagerSelector_Value(t *testing.T) {
	tests := []struct {
		name     string
		selector PagerSelector
		expected string
	}{
		{
			name:     "empty value",
			selector: NewPagerSelector("", false),
			expected: "",
		},
		{
			name:     "less pager",
			selector: NewPagerSelector("less", true),
			expected: "less",
		},
		{
			name:     "more pager",
			selector: NewPagerSelector("more", true),
			expected: "more",
		},
		{
			name:     "true value",
			selector: NewPagerSelector("true", true),
			expected: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.Value()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestPagerSelector_IsEnabled tests pager enabled check with different values.
func TestPagerSelector_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		selector PagerSelector
		expected bool
	}{
		{
			name:     "not provided - disabled",
			selector: NewPagerSelector("", false),
			expected: false,
		},
		{
			name:     "provided with empty value - enabled",
			selector: NewPagerSelector("", true),
			expected: true,
		},
		{
			name:     "provided with false - disabled",
			selector: NewPagerSelector("false", true),
			expected: false,
		},
		{
			name:     "provided with true - enabled",
			selector: NewPagerSelector("true", true),
			expected: true,
		},
		{
			name:     "provided with pager name - enabled",
			selector: NewPagerSelector("less", true),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsEnabled()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestPagerSelector_Pager tests pager name retrieval with different scenarios.
func TestPagerSelector_Pager(t *testing.T) {
	tests := []struct {
		name        string
		selector    PagerSelector
		expected    string
		description string
	}{
		{
			name:        "not provided - empty (use config default)",
			selector:    NewPagerSelector("", false),
			expected:    "",
			description: "Returns empty to signal 'use config/env default'",
		},
		{
			name:        "provided with empty value - empty (use default pager)",
			selector:    NewPagerSelector("", true),
			expected:    "",
			description: "Returns empty to signal 'use default pager'",
		},
		{
			name:        "provided with true - empty (use default pager)",
			selector:    NewPagerSelector("true", true),
			expected:    "",
			description: "Returns empty to signal 'use default pager'",
		},
		{
			name:        "provided with false - empty (disabled)",
			selector:    NewPagerSelector("false", true),
			expected:    "",
			description: "Returns empty because pager is disabled",
		},
		{
			name:        "provided with custom pager",
			selector:    NewPagerSelector("more", true),
			expected:    "more",
			description: "Returns specific pager name",
		},
		{
			name:        "provided with bat pager",
			selector:    NewPagerSelector("bat", true),
			expected:    "bat",
			description: "Returns specific pager name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.Pager()
			assert.Equal(t, tt.expected, got, tt.description)
		})
	}
}
