package flagparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGlobalFlags(t *testing.T) {
	flags := NewGlobalFlags()

	// Test default values.
	assert.Equal(t, "Info", flags.LogsLevel, "LogsLevel default")
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

func TestGlobalFlags_GetGlobalFlags(t *testing.T) {
	flags := GlobalFlags{
		LogsLevel: "Debug",
		NoColor:   true,
	}

	got := flags.GetGlobalFlags()
	assert.NotNil(t, got, "GetGlobalFlags should return non-nil")
	assert.Equal(t, "Debug", got.LogsLevel)
	assert.True(t, got.NoColor)
}

func TestGlobalFlags_AllFields(t *testing.T) {
	// Test that we can set all fields without errors.
	flags := GlobalFlags{
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

func TestGlobalFlags_Embedding(t *testing.T) {
	// Test that GlobalFlags can be embedded in other structs.
	type TestInterpreter struct {
		GlobalFlags
		Stack string
	}

	interpreter := TestInterpreter{
		GlobalFlags: GlobalFlags{
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

func TestGlobalFlags_ZeroValues(t *testing.T) {
	// Test zero value behavior.
	var flags GlobalFlags

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

func TestGlobalFlags_CommonScenarios(t *testing.T) {
	tests := []struct {
		name        string
		flags       GlobalFlags
		description string
	}{
		{
			name: "default configuration",
			flags: GlobalFlags{
				LogsLevel:    "Info",
				LogsFile:     "/dev/stderr",
				ProfilerPort: 6060,
				ProfilerHost: "localhost",
				HeatmapMode:  "bar",
			},
			description: "User didn't override any flags, using all defaults",
		},
		{
			name: "debug mode with no color",
			flags: GlobalFlags{
				LogsLevel: "Debug",
				NoColor:   true,
			},
			description: "User wants debug logging without colors (CI environment)",
		},
		{
			name: "profiling enabled",
			flags: GlobalFlags{
				ProfilerEnabled: true,
				ProfilerPort:    9090,
				ProfilerHost:    "0.0.0.0",
			},
			description: "User enabled profiling on custom port",
		},
		{
			name: "custom working directory",
			flags: GlobalFlags{
				Chdir:    "/tmp/workspace",
				BasePath: "/custom/base",
			},
			description: "User changed working directory and base path",
		},
		{
			name: "identity and pager configured",
			flags: GlobalFlags{
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
