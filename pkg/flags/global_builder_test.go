package flags

import (
	"testing"

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

		// Verify key flags are registered as persistent flags (inherited by subcommands).
		assert.NotNil(t, cmd.PersistentFlags().Lookup("logs-level"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("logs-file"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("chdir"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("config"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("config-path"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("force-color"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("force-tty"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("mask"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("pager"))
		assert.NotNil(t, cmd.PersistentFlags().Lookup("identity"))
	})

	t.Run("uses defaults from NewGlobalFlags", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		defaults := NewGlobalFlags()

		// Verify defaults match NewGlobalFlags().
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

		defaults := NewGlobalFlags()
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
