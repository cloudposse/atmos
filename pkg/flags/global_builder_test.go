package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/flags/preprocess"
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
		assert.NotNil(t, cmd.PersistentFlags().Lookup("cast"), "cast flag should be registered")
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

		// AI integration flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("ai"), "ai flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("skill"), "skill flag should be registered")

		// System flags.
		assert.NotNil(t, cmd.PersistentFlags().Lookup("redirect-stderr"), "redirect-stderr flag should be registered")
		assert.NotNil(t, cmd.PersistentFlags().Lookup("use-version"), "use-version flag should be registered")
		// Note: --version is NOT a global persistent flag - it's a local flag on RootCmd only.
		// See global_builder.go registerSystemFlags() for explanation.
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

	t.Run("handles NoOptDefVal for cast", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		cast := cmd.PersistentFlags().Lookup("cast")
		assert.NotNil(t, cast)
		assert.Equal(t, "__AUTO__", cast.NoOptDefVal)
	})

	t.Run("handles NoOptDefVal for profile", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)

		profile := cmd.PersistentFlags().Lookup("profile")
		require.NotNil(t, profile)
		assert.Equal(t, cfg.ProfileFlagSelectValue, profile.NoOptDefVal, "profile should support the bare-flag sentinel like identity")
		assert.Equal(t, "stringSlice", profile.Value.Type(), "profile must remain a StringSliceFlag")
	})
}

// preprocessProfileArgs replicates cmd/root.go's preprocessNoOptDefValFlags step
// (which uses GlobalFlagsRegistry(), not GlobalOptionsBuilder) so these tests can
// exercise the exact preprocessing pipeline the real RootCmd runs before Cobra ever
// sees the args. Without this rewrite, a NoOptDefVal flag turns "--profile name"
// into an ambiguous bare-flag + stray-positional-arg pair.
func preprocessProfileArgs(args []string) []string {
	registry := GlobalFlagsRegistry()
	allFlags := registry.All()
	flagInfos := make([]preprocess.FlagInfo, len(allFlags))
	for i, f := range allFlags {
		flagInfos[i] = f
	}
	pipeline := preprocess.NewPipeline(preprocess.NewNoOptDefValPreprocessor(flagInfos))
	return pipeline.Run(args)
}

// TestGlobalOptionsBuilder_ProfileNoOptDefVal is an end-to-end regression test for the
// --profile bare-flag pattern: it exercises the real root-level preprocessing step
// (preprocessProfileArgs) together with the actual Cobra flag registered by
// GlobalOptionsBuilder, verifying the exact bug this feature fixes -- previously,
// "atmos auth login --profile" failed with "flag needs an argument" because
// StringSliceFlag had no NoOptDefVal support at all.
func TestGlobalOptionsBuilder_ProfileNoOptDefVal(t *testing.T) {
	newCmdAndViper := func() (*cobra.Command, *viper.Viper) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)
		require.NoError(t, parser.BindToViper(v))
		return cmd, v
	}

	bindAndGetProfile := func(cmd *cobra.Command, v *viper.Viper) []string {
		require.NoError(t, v.BindPFlag("profile", cmd.PersistentFlags().Lookup("profile")))
		return v.GetStringSlice("profile")
	}

	t.Run("--profile=a,b equals syntax parses as explicit values, unaffected", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--profile=a,b"})
		require.NoError(t, cmd.ParseFlags(args))
		assert.Equal(t, []string{"a", "b"}, bindAndGetProfile(cmd, v))
	})

	t.Run("--profile a,b space syntax parses as explicit value, unaffected", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--profile", "a,b"})
		require.NoError(t, cmd.ParseFlags(args))
		assert.Equal(t, []string{"a", "b"}, bindAndGetProfile(cmd, v))
	})

	t.Run("--profile foo --profile bar repeated flag still accumulates normally", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--profile", "foo", "--profile", "bar"})
		require.NoError(t, cmd.ParseFlags(args))
		result := bindAndGetProfile(cmd, v)
		require.Len(t, result, 2)
		assert.Equal(t, "foo", result[0])
		assert.Equal(t, "bar", result[1])
	})

	t.Run("bare --profile at end resolves to sentinel with no parse error", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--profile"})
		require.NoError(t, cmd.ParseFlags(args), "bare --profile must not error (this is the bug being fixed)")
		assert.Equal(t, []string{cfg.ProfileFlagSelectValue}, bindAndGetProfile(cmd, v))
	})

	t.Run("bare --profile followed by another flag resolves to sentinel, does not swallow the next flag", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--profile", "--identity=x"})
		require.NoError(t, cmd.ParseFlags(args))
		assert.Equal(t, []string{cfg.ProfileFlagSelectValue}, bindAndGetProfile(cmd, v))
		require.NoError(t, v.BindPFlag("identity", cmd.PersistentFlags().Lookup("identity")))
		assert.Equal(t, "x", v.GetString("identity"), "--identity=x must not be swallowed as --profile's value")
	})
}

// TestGlobalOptionsBuilder_IdentityPagerCastNoOptDefValUnchanged is a regression guard
// ensuring the --profile NoOptDefVal addition does not alter existing --identity,
// --pager, or --cast bare-flag behavior.
func TestGlobalOptionsBuilder_IdentityPagerCastNoOptDefValUnchanged(t *testing.T) {
	newCmdAndViper := func() (*cobra.Command, *viper.Viper) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := NewGlobalOptionsBuilder().Build()
		parser.RegisterPersistentFlags(cmd)
		require.NoError(t, parser.BindToViper(v))
		return cmd, v
	}

	t.Run("bare --identity resolves to sentinel", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--identity"})
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, v.BindPFlag("identity", cmd.PersistentFlags().Lookup("identity")))
		assert.Equal(t, "__SELECT__", v.GetString("identity"))
	})

	t.Run("--identity prod-admin space syntax still resolves explicit value", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--identity", "prod-admin"})
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, v.BindPFlag("identity", cmd.PersistentFlags().Lookup("identity")))
		assert.Equal(t, "prod-admin", v.GetString("identity"))
	})

	t.Run("bare --pager resolves to true", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--pager"})
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, v.BindPFlag("pager", cmd.PersistentFlags().Lookup("pager")))
		assert.Equal(t, "true", v.GetString("pager"))
	})

	t.Run("bare --cast does not consume the next positional arg", func(t *testing.T) {
		cmd, v := newCmdAndViper()
		args := preprocessProfileArgs([]string{"--cast", "terraform"})
		require.NoError(t, cmd.ParseFlags(args))
		require.NoError(t, v.BindPFlag("cast", cmd.PersistentFlags().Lookup("cast")))
		assert.Equal(t, "__AUTO__", v.GetString("cast"))
		assert.Equal(t, []string{"terraform"}, cmd.Flags().Args())
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
