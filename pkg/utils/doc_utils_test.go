package utils

import (
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDisplayDocs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping test on Windows: uses Unix commands (cat, less)")
	}

	// viper.Reset() is called in each subtest

	t.Run("no pager - prints to stdout", func(t *testing.T) {
		// When usePager is false, should print directly and not use pager
		err := DisplayDocs("test documentation content", false)
		assert.NoError(t, err)
	})

	t.Run("empty pager command returns error", func(t *testing.T) {
		viper.Reset()
		// Set empty pager to test the validation
		viper.Set("pager", "")
		// With an empty string that becomes empty after splitting, should use default "less -r"
		// Let's verify that whitespace-only pager fails
		viper.Set("pager", "   ")
		err := DisplayDocs("test docs", true)
		// This should fail since the pager command is just whitespace
		assert.Error(t, err)
	})

	t.Run("invalid pager command", func(t *testing.T) {
		viper.Reset()
		// Set a pager command that doesn't exist
		viper.Set("pager", "nonexistent-pager-command-12345")

		err := DisplayDocs("test docs", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute pager")
	})

	t.Run("pager with arguments", func(t *testing.T) {
		viper.Reset()
		// Use "cat" as a simple pager for testing (works cross-platform)
		viper.Set("pager", "cat")

		err := DisplayDocs("test documentation", true)
		// Should succeed with cat
		assert.NoError(t, err)
	})

	t.Run("default pager fallback", func(t *testing.T) {
		viper.Reset()
		t.Setenv("PAGER", "")
		t.Setenv("ATMOS_PAGER", "")

		// Should fall back to "less -r" when no pager is set
		// This may fail on systems without less, but that's expected
		err := DisplayDocs("test docs", true)
		// We can't assert success/failure here as it depends on system having 'less'
		// Just verify it doesn't panic
		_ = err
	})

	t.Run("ATMOS_PAGER environment variable", func(t *testing.T) {
		viper.Reset()
		t.Setenv("ATMOS_PAGER", "cat")
		// Need to rebind env after setting it
		_ = viper.BindEnv("pager", "ATMOS_PAGER", "PAGER")

		err := DisplayDocs("test content", true)
		assert.NoError(t, err)
	})

	t.Run("PAGER environment variable", func(t *testing.T) {
		viper.Reset()
		t.Setenv("PAGER", "cat")
		t.Setenv("ATMOS_PAGER", "")
		// Need to rebind env after setting it
		_ = viper.BindEnv("pager", "ATMOS_PAGER", "PAGER")

		err := DisplayDocs("test content", true)
		assert.NoError(t, err)
	})

	t.Run("ATMOS_PAGER takes precedence over PAGER", func(t *testing.T) {
		viper.Reset()
		t.Setenv("ATMOS_PAGER", "cat")
		t.Setenv("PAGER", "nonexistent-command")
		// Need to rebind env after setting it
		_ = viper.BindEnv("pager", "ATMOS_PAGER", "PAGER")

		// Should use ATMOS_PAGER (cat) instead of PAGER
		err := DisplayDocs("test content", true)
		assert.NoError(t, err)
	})

	t.Run("empty pager after trimming returns error", func(t *testing.T) {
		viper.Reset()
		// Set pager to only whitespace
		viper.Set("pager", "    ")

		err := DisplayDocs("test docs", true)
		// After splitting "    " by whitespace, we get empty slice
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidPagerCommand)
	})
}
