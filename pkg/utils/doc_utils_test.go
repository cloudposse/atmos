package utils

import (
	"os"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
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

// TestDisplayDocs_PagerOutputMasksSecretSplitAcrossWrites proves a registered secret that the
// pager emits across two writes is masked in the output rather than leaked in halves.
func TestDisplayDocs_PagerOutputMasksSecretSplitAcrossWrites(t *testing.T) {
	const secret = "pager-split-secret-value"
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	require.NoError(t, iolib.Initialize())
	iolib.RegisterSecret(secret)

	// The test binary doubles as the pager so the test is cross-platform.
	exe, err := os.Executable()
	require.NoError(t, err)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("pager", exe)
	t.Setenv(splitOutputPagerEnv, "1")

	out, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	origStdout := os.Stdout
	os.Stdout = out
	t.Cleanup(func() { os.Stdout = origStdout })

	runErr := DisplayDocs("ignored", true)
	os.Stdout = origStdout
	require.NoError(t, out.Close())
	require.NoError(t, runErr)

	got, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	assert.Equal(t, "doc="+iolib.MaskReplacement+" done\n", string(got))
}
