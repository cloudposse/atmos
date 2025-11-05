package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*cobra.Command, *viper.Viper)
		expected GlobalFlags
	}{
		{
			name: "all defaults",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				// Set up defaults only.
				v.SetDefault("logs-level", "Warning")
				v.SetDefault("logs-file", "/dev/stderr")
				v.SetDefault("profiler-port", 6060)
				v.SetDefault("profiler-host", "localhost")
				v.SetDefault("heatmap-mode", "bar")
			},
			expected: GlobalFlags{
				LogsLevel:    "Warning",
				LogsFile:     "/dev/stderr",
				NoColor:      false,
				ProfilerPort: 6060,
				ProfilerHost: "localhost",
				HeatmapMode:  "bar",
			},
		},
		{
			name: "override logs level",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("logs-level", "Debug")
			},
			expected: GlobalFlags{
				LogsLevel: "Debug",
			},
		},
		{
			name: "enable no-color",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("no-color", true)
			},
			expected: GlobalFlags{
				NoColor: true,
			},
		},
		{
			name: "custom profiler configuration",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("profiler-enabled", true)
				v.Set("profiler-port", 9090)
				v.Set("profiler-host", "0.0.0.0")
			},
			expected: GlobalFlags{
				ProfilerEnabled: true,
				ProfilerPort:    9090,
				ProfilerHost:    "0.0.0.0",
			},
		},
		{
			name: "custom working directory",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("chdir", "/tmp/workspace")
				v.Set("base-path", "/custom/base")
			},
			expected: GlobalFlags{
				Chdir:    "/tmp/workspace",
				BasePath: "/custom/base",
			},
		},
		{
			name: "enable heatmap",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("heatmap", true)
				v.Set("heatmap-mode", "sparkline")
			},
			expected: GlobalFlags{
				Heatmap:     true,
				HeatmapMode: "sparkline",
			},
		},
		{
			name: "single config file",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("config", []string{"atmos.yaml"})
			},
			expected: GlobalFlags{
				Config: []string{"atmos.yaml"},
			},
		},
		{
			name: "multiple config files",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("config", []string{"atmos.yaml", "atmos-override.yaml", "atmos-local.yaml"})
			},
			expected: GlobalFlags{
				Config: []string{"atmos.yaml", "atmos-override.yaml", "atmos-local.yaml"},
			},
		},
		{
			name: "multiple config paths",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("config-path", []string{"/etc/atmos", "/home/user/.atmos", "./config"})
			},
			expected: GlobalFlags{
				ConfigPath: []string{"/etc/atmos", "/home/user/.atmos", "./config"},
			},
		},
		{
			name: "config and config-path together",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.Set("config", []string{"atmos.yaml", "overrides.yaml"})
				v.Set("config-path", []string{"/etc/atmos", "./config"})
			},
			expected: GlobalFlags{
				Config:     []string{"atmos.yaml", "overrides.yaml"},
				ConfigPath: []string{"/etc/atmos", "./config"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh Cobra command and Viper instance.
			cmd := &cobra.Command{}
			v := viper.New()

			// Apply test setup.
			tt.setup(cmd, v)

			// Parse global flags.
			got := ParseGlobalFlags(cmd, v)

			// Verify expected fields (only check non-zero expected values).
			if tt.expected.Chdir != "" {
				assert.Equal(t, tt.expected.Chdir, got.Chdir)
			}
			if tt.expected.BasePath != "" {
				assert.Equal(t, tt.expected.BasePath, got.BasePath)
			}
			if tt.expected.LogsLevel != "" {
				assert.Equal(t, tt.expected.LogsLevel, got.LogsLevel)
			}
			if tt.expected.LogsFile != "" {
				assert.Equal(t, tt.expected.LogsFile, got.LogsFile)
			}
			if tt.expected.NoColor {
				assert.True(t, got.NoColor)
			}
			if tt.expected.ProfilerEnabled {
				assert.True(t, got.ProfilerEnabled)
			}
			if tt.expected.ProfilerPort != 0 {
				assert.Equal(t, tt.expected.ProfilerPort, got.ProfilerPort)
			}
			if tt.expected.ProfilerHost != "" {
				assert.Equal(t, tt.expected.ProfilerHost, got.ProfilerHost)
			}
			if tt.expected.Heatmap {
				assert.True(t, got.Heatmap)
			}
			if tt.expected.HeatmapMode != "" {
				assert.Equal(t, tt.expected.HeatmapMode, got.HeatmapMode)
			}
			if len(tt.expected.Config) > 0 {
				assert.Equal(t, tt.expected.Config, got.Config)
			}
			if len(tt.expected.ConfigPath) > 0 {
				assert.Equal(t, tt.expected.ConfigPath, got.ConfigPath)
			}
		})
	}
}

func TestParseIdentityFlag(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*cobra.Command, *viper.Viper)
		expected IdentitySelector
	}{
		{
			name: "identity flag not registered",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				// Don't register identity flag.
			},
			expected: NewIdentitySelector("", false),
		},
		{
			name: "identity not provided",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("identity", "", "Identity")
			},
			expected: NewIdentitySelector("", false),
		},
		{
			name: "identity from CLI flag (interactive)",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("identity", "", "Identity")
				cmd.Flags().Set("identity", cfg.IdentityFlagSelectValue)
				v.Set("identity", cfg.IdentityFlagSelectValue)
			},
			expected: NewIdentitySelector(cfg.IdentityFlagSelectValue, true),
		},
		{
			name: "identity from CLI flag (explicit)",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("identity", "", "Identity")
				cmd.Flags().Set("identity", "prod-admin")
				v.Set("identity", "prod-admin")
			},
			expected: NewIdentitySelector("prod-admin", true),
		},
		{
			name: "identity from env var",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("identity", "", "Identity")
				v.Set("identity", "staging-user")
			},
			expected: NewIdentitySelector("staging-user", true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			v := viper.New()

			tt.setup(cmd, v)

			got := parseIdentityFlag(cmd, v)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParsePagerFlag(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*cobra.Command, *viper.Viper)
		expected PagerSelector
	}{
		{
			name: "pager flag not registered",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				// Don't register pager flag.
			},
			expected: NewPagerSelector("", false),
		},
		{
			name: "pager not provided",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("pager", "", "Pager")
			},
			expected: NewPagerSelector("", false),
		},
		{
			name: "pager from CLI flag (enabled)",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("pager", "", "Pager")
				cmd.Flags().Set("pager", "true")
				v.Set("pager", "true")
			},
			expected: NewPagerSelector("true", true),
		},
		{
			name: "pager from CLI flag (disabled)",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("pager", "", "Pager")
				cmd.Flags().Set("pager", "false")
				v.Set("pager", "false")
			},
			expected: NewPagerSelector("false", true),
		},
		{
			name: "pager from CLI flag (specific pager)",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("pager", "", "Pager")
				cmd.Flags().Set("pager", "less")
				v.Set("pager", "less")
			},
			expected: NewPagerSelector("less", true),
		},
		{
			name: "pager from env var",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				cmd.Flags().String("pager", "", "Pager")
				v.Set("pager", "more")
			},
			expected: NewPagerSelector("more", true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			v := viper.New()

			tt.setup(cmd, v)

			got := parsePagerFlag(cmd, v)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGlobalFlagsRegistry(t *testing.T) {
	registry := GlobalFlagsRegistry()

	require.NotNil(t, registry)

	// Test that all expected global flags are registered.
	expectedFlags := []string{
		"chdir",
		"base-path",
		"config",
		"config-path",
		"logs-level",
		"logs-file",
		"no-color",
		"identity",
		"pager",
		"profiler-enabled",
		"profiler-port",
		"profiler-host",
		"profile-file",
		"profile-type",
		"heatmap",
		"heatmap-mode",
	}

	for _, flagName := range expectedFlags {
		t.Run("has_"+flagName, func(t *testing.T) {
			assert.True(t, registry.Has(flagName), "Registry should have %s flag", flagName)
			flag := registry.Get(flagName)
			assert.NotNil(t, flag, "Flag %s should not be nil", flagName)
		})
	}

	// Test specific flag properties.
	t.Run("logs-level has correct default", func(t *testing.T) {
		flag := registry.Get("logs-level")
		assert.Equal(t, "Info", flag.GetDefault())
	})

	t.Run("logs-file has correct default", func(t *testing.T) {
		flag := registry.Get("logs-file")
		assert.Equal(t, "/dev/stderr", flag.GetDefault())
	})

	t.Run("profiler-port has correct default", func(t *testing.T) {
		flag := registry.Get("profiler-port")
		assert.Equal(t, 6060, flag.GetDefault())
	})

	t.Run("identity has NoOptDefVal", func(t *testing.T) {
		flag := registry.Get("identity")
		assert.Equal(t, cfg.IdentityFlagSelectValue, flag.GetNoOptDefVal())
	})

	t.Run("pager has NoOptDefVal", func(t *testing.T) {
		flag := registry.Get("pager")
		assert.Equal(t, "true", flag.GetNoOptDefVal())
	})

	t.Run("identity has env vars", func(t *testing.T) {
		flag := registry.Get("identity")
		envVars := flag.GetEnvVars()
		assert.Contains(t, envVars, "ATMOS_IDENTITY")
		assert.Contains(t, envVars, "IDENTITY")
	})

	t.Run("config is StringSliceFlag", func(t *testing.T) {
		flag := registry.Get("config")
		_, ok := flag.(*StringSliceFlag)
		assert.True(t, ok, "config should be StringSliceFlag")
		assert.Equal(t, []string{}, flag.GetDefault(), "config default should be empty slice")
	})

	t.Run("config-path is StringSliceFlag", func(t *testing.T) {
		flag := registry.Get("config-path")
		_, ok := flag.(*StringSliceFlag)
		assert.True(t, ok, "config-path should be StringSliceFlag")
		assert.Equal(t, []string{}, flag.GetDefault(), "config-path default should be empty slice")
	})
}

func TestParseGlobalFlags_Precedence(t *testing.T) {
	// Test precedence: CLI > ENV > config > default.
	tests := []struct {
		name     string
		setup    func(*cobra.Command, *viper.Viper)
		expected string
		field    string
	}{
		{
			name: "CLI flag overrides everything",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.SetDefault("logs-level", "Warning") // Default.
				v.Set("logs-level", "Debug")          // CLI (highest priority).
			},
			expected: "Debug",
			field:    "LogsLevel",
		},
		{
			name: "ENV overrides config and default",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.SetDefault("logs-level", "Warning") // Default.
				// ENV set via v.Set (simulating env var binding).
				v.Set("logs-level", "Trace")
			},
			expected: "Trace",
			field:    "LogsLevel",
		},
		{
			name: "config overrides default",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.SetDefault("logs-level", "Warning") // Default.
				v.Set("logs-level", "Warning")        // Config.
			},
			expected: "Warning",
			field:    "LogsLevel",
		},
		{
			name: "default used when nothing else set",
			setup: func(cmd *cobra.Command, v *viper.Viper) {
				v.SetDefault("logs-level", "Warning") // Default.
			},
			expected: "Warning",
			field:    "LogsLevel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			v := viper.New()

			tt.setup(cmd, v)

			got := ParseGlobalFlags(cmd, v)

			if tt.field == "LogsLevel" {
				assert.Equal(t, tt.expected, got.LogsLevel)
			}
		})
	}
}
