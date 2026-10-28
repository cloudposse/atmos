package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelmfileExecutionFlags(t *testing.T) {
	for _, mode := range []string{"dry-run", "redirect-stderr", "global-options"} {
		t.Run(mode, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			binary, err := os.Executable()
			require.NoError(t, err)
			marker := filepath.Join(root, "subprocess-ran")
			stderrFile := filepath.Join(root, "stderr.log")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			t.Setenv("_ATMOS_TEST_COMPONENT_STDERR", "helmfile stderr control\n")
			args := []string{"helmfile", "apply", "app", "-s", "dev", "--command", binary}
			switch mode {
			case "global-options":
				args = append(args, "--global-options=--no-color --namespace=test")
			case "dry-run":
				args = append(args, "--dry-run")
			default:
				args = append(args, "--redirect-stderr", stderrFile)
			}
			setComponentCommandArgs(t, args)
			require.NoError(t, Execute())
			switch mode {
			case "global-options":
				actual, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Contains(t, string(actual), "--no-color\n--namespace=test\napply")
			case "dry-run":
				require.NoFileExists(t, marker, "dry-run must not start the component executable")
			default:
				require.FileExists(t, marker)
				actual, err := os.ReadFile(stderrFile)
				require.NoError(t, err)
				require.Equal(t, "helmfile stderr control\n", string(actual))
			}
		})
	}
}

func TestHelmfileVersionDryRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		dryRun bool
	}{
		{name: "execute"},
		{name: "dry-run", flags: []string{"--dry-run"}, dryRun: true},
		{name: "explicit-false", flags: []string{"--dry-run=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			binary, err := os.Executable()
			require.NoError(t, err)
			marker := filepath.Join(root, "subprocess-ran")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			args := []string{"helmfile", "version", "--command", binary}
			setComponentCommandArgs(t, append(args, tc.flags...))

			require.NoError(t, Execute())
			if tc.dryRun {
				require.NoFileExists(t, marker, "dry-run must not start the version executable")
				return
			}
			actual, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "version", string(actual))
		})
	}
}
