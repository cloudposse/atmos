package flags

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGlobalOptionsBuilder(t *testing.T) {
	t.Run("builds parser with all global flags", func(t *testing.T) {
		parser := NewGlobalOptionsBuilder().Build()
		assert.NotNil(t, parser)
	})

	t.Run("registers all global flags on command as persistent flags", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		// Working directory and path configuration flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("chdir"), "chdir flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("base-path"), "base-path flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("config"), "config flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("config-path"), "config-path flag should be registered")

		// Logging configuration flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("logs-level"), "logs-level flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("logs-file"), "logs-file flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("no-color"), "no-color flag should be registered")

		// Terminal and I/O configuration flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("force-color"), "force-color flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("force-tty"), "force-tty flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("mask"), "mask flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("pager"), "pager flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("interactive"), "interactive flag should be registered")

		// Authentication flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("identity"), "identity flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("profile"), "profile flag should be registered")

		// Profiling flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("profiler-enabled"), "profiler-enabled flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("profiler-port"), "profiler-port flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("profiler-host"), "profiler-host flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("profile-file"), "profile-file flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("profile-type"), "profile-type flag should be registered")

		// Performance heatmap flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("heatmap"), "heatmap flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("heatmap-mode"), "heatmap-mode flag should be registered")

		// System flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("redirect-stderr"), "redirect-stderr flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("version"), "version flag should be registered")
	})

	t.Run("uses defaults from global.NewFlags", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		defaults := global.NewFlags()

		// Verify defaults match global.NewFlags().
		logsLevel := cmd.PersistentFlags().Lookup("logs-level")
		assert.Equal(t, defaults.LogsLevel, logsLevel.DefValue)

		logsFile := cmd.PersistentFlags().Lookup("logs-file")
		assert.Equal(t, defaults.LogsFile, logsFile.DefValue)

		mask := cmd.PersistentFlags().Lookup("mask")
		assert.Equal(t, "true", mask.DefValue) // defaults.Mask is true
	})

	t.Run("binds to viper successfully", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		err := parser.BindToViper(v)
		assert.NoError(t, err)
	})

	t.Run("handles chdir shorthand flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		chdir := cmd.PersistentFlags().Lookup("chdir")
		assert.NotNil(t, chdir)
		assert.Equal(t, "C", chdir.Shorthand)
	})

	t.Run("handles NoOptDefVal for pager", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		pager := cmd.PersistentFlags().Lookup("pager")
		assert.NotNil(t, pager)
		assert.Equal(t, "true", pager.NoOptDefVal)
	})

	t.Run("handles NoOptDefVal for identity", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		identity := cmd.PersistentFlags().Lookup("identity")
		assert.NotNil(t, identity)
		assert.Equal(t, "__SELECT__", identity.NoOptDefVal)
	})
}

func TestGlobalOptionsBuilder_FlagPrecedence(t *testing.T) {
	t.Run("CLI flag overrides default", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterFlags(cmd)
		_ = parser.BindToViper(v)

		// Set CLI flag value.
		v.Set("logs-level", "Debug")

		flags := ParseGlobalFlags(cmd, v)
		assert.Equal(t, "Debug", flags.LogsLevel)
	})

	t.Run("uses default when nothing set", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterFlags(cmd)
		_ = parser.BindToViper(v)

		defaults := global.NewFlags()
		flags := ParseGlobalFlags(cmd, v)
		assert.Equal(t, defaults.LogsLevel, flags.LogsLevel)
	})

	t.Run("environment variable overrides default", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterFlags(cmd)
		_ = parser.BindToViper(v)

		// Simulate environment variable.
		t.Setenv("ATMOS_LOGS_LEVEL", "Trace")
		_ = v.BindEnv("logs-level", "ATMOS_LOGS_LEVEL")

		flags := ParseGlobalFlags(cmd, v)
		assert.Equal(t, "Trace", flags.LogsLevel)
	})
}
