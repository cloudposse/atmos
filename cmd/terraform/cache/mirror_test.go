package cache

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tfmirror "github.com/cloudposse/atmos/pkg/terraform/mirror"
)

func TestMirrorCmdArgs(t *testing.T) {
	// The command accepts zero (multi-component selectors) or one (single component)
	// positional argument, never more.
	assert.NoError(t, mirrorCmd.Args(mirrorCmd, []string{}))
	assert.NoError(t, mirrorCmd.Args(mirrorCmd, []string{"vpc"}))
	assert.Error(t, mirrorCmd.Args(mirrorCmd, []string{"vpc", "extra"}))
}

func TestMirrorCmdRunSingle(t *testing.T) {
	orig := mirrorRun
	t.Cleanup(func() { mirrorRun = orig })

	var got tfmirror.Options
	mirrorRun = func(o tfmirror.Options) error {
		got = o
		return nil
	}

	// `stack` is the inherited persistent terraform flag, read from Viper.
	viper.Set("stack", "plat-ue2-prod")
	t.Cleanup(func() { viper.Set("stack", "") })

	// Execute through the cache group (the package root) so Cobra routes to the
	// mirror subcommand and parses its flags.
	cacheCmd.SetArgs([]string{"mirror", "vpc", "--platform=linux_amd64", "--platform=darwin_arm64"})
	require.NoError(t, cacheCmd.Execute())

	assert.Equal(t, "vpc", got.Component)
	assert.Equal(t, "plat-ue2-prod", got.Stack)
	assert.Equal(t, []string{"linux_amd64", "darwin_arm64"}, got.PlatformsFlag)
	assert.False(t, got.All)
}

func TestMirrorCmdRunAll(t *testing.T) {
	orig := mirrorRun
	t.Cleanup(func() { mirrorRun = orig })

	// mirrorCmd is a package-level singleton, and Options.All is read via
	// v.GetBool("all") (viper's flag binding), which honors the flag's
	// Changed state rather than its value alone. Passing --all here sets
	// Changed=true; without restoring it, a later test that never passes
	// --all (e.g. TestMirrorCmdRunSingle) would still observe All=true
	// under -shuffle=on, since a flag omitted from a later Execute() call
	// keeps whatever value/Changed state the previous parse left it in.
	allFlag := mirrorCmd.Flags().Lookup("all")
	origAllValue := allFlag.Value.String()
	origAllChanged := allFlag.Changed
	t.Cleanup(func() {
		_ = mirrorCmd.Flags().Set("all", origAllValue)
		allFlag.Changed = origAllChanged
	})

	var got tfmirror.Options
	mirrorRun = func(o tfmirror.Options) error {
		got = o
		return nil
	}

	cacheCmd.SetArgs([]string{"mirror", "--all"})
	require.NoError(t, cacheCmd.Execute())

	assert.True(t, got.All)
	assert.Empty(t, got.Component)
}
