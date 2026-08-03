package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestToolchainUseLockFileDefault locks in the contract that toolchain.use_lock_file
// defaults to true (reproducible builds are the default behavior). A regression that
// flipped the default to false would silently degrade every install to "re-resolve
// from the registry every run with no checksum verification" — exactly what the
// lockfile feature exists to prevent.
func TestToolchainUseLockFileDefault(t *testing.T) {
	v := viper.New()
	setDefaultConfiguration(v)
	assert.True(t, v.GetBool("toolchain.use_lock_file"),
		"toolchain.use_lock_file must default to true so reproducible builds are the default")
}

// TestToolchainUseLockFileExplicitFalse verifies that an explicit `false` from config
// wins over the default — users can opt out via `toolchain.use_lock_file: false` in
// atmos.yaml or `ATMOS_TOOLCHAIN_USE_LOCK_FILE=false` if they want the old behavior.
func TestToolchainUseLockFileExplicitFalse(t *testing.T) {
	v := viper.New()
	setDefaultConfiguration(v)
	v.Set("toolchain.use_lock_file", false)
	assert.False(t, v.GetBool("toolchain.use_lock_file"),
		"an explicit false from config must override the default true")
}
