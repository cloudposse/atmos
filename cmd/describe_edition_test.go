package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditionPinSource(t *testing.T) {
	newCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "test"}
		c.Flags().String("edition", "", "")
		return c
	}

	t.Run("no pin has no source", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "")
		require.NoError(t, os.Unsetenv("ATMOS_EDITION"))
		assert.Empty(t, editionPinSource(newCmd(), ""))
	})

	t.Run("flag wins", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "2026")
		c := newCmd()
		require.NoError(t, c.Flags().Set("edition", "2025"))
		assert.Equal(t, "flag", editionPinSource(c, "2025"))
	})

	t.Run("env when flag unset", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "2026")
		assert.Equal(t, "env", editionPinSource(newCmd(), "2026"))
	})

	t.Run("config when neither flag nor env", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "")
		require.NoError(t, os.Unsetenv("ATMOS_EDITION"))
		assert.Equal(t, "config", editionPinSource(newCmd(), "2026"))
	})
}

func TestDescribeEditionCmdOutputFormats(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("ATMOS_EDITION", "")
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("edition: \"2025-09\"\n"), 0o600))

	for _, tt := range []struct {
		name   string
		format string
		want   string
	}{
		{name: "yaml", format: "yaml", want: "pinned: true"},
		{name: "json", format: "json", want: "\"pinned\": true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testCmd := &cobra.Command{Use: "edition"}
			testCmd.Flags().String("format", tt.format, "")
			stdout, _ := captureStdoutStderr(t, func() {
				require.NoError(t, describeEditionCmd.RunE(testCmd, nil))
			})
			assert.Contains(t, stdout, tt.want)
		})
	}
}
