package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPackerDryRun checks that --dry-run previews the Packer invocation without starting Packer
// or writing the variable file, and that explicit or omitted --dry-run still runs it.
func TestPackerDryRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		dryRun bool
	}{
		{name: "dry-run", flags: []string{"--dry-run"}, dryRun: true},
		{name: "dry-run-true", flags: []string{"--dry-run=true"}, dryRun: true},
		{name: "explicit-false", flags: []string{"--dry-run=false"}},
		{name: "execute"},
	} {
		for _, subcommand := range []string{"build", "validate"} {
			t.Run(subcommand+"/"+tc.name, func(t *testing.T) {
				_ = NewTestKit(t)
				root := componentCommandFixture(t, "packer")
				t.Setenv("ATMOS_LOGS_LEVEL", "Error")
				output := captureHelmfilePreview(t)
				binary, err := os.Executable()
				require.NoError(t, err)
				marker := filepath.Join(root, "subprocess-ran")
				t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
				args := []string{"packer", subcommand, "app", "-s", "dev", "--command", binary}
				setComponentCommandArgs(t, append(args, tc.flags...))

				require.NoError(t, Execute())
				varfile := filepath.Join(root, "components", "app", "dev-app.packer.vars.json")
				if !tc.dryRun {
					actual, err := os.ReadFile(marker)
					require.NoError(t, err, "packer must run without --dry-run")
					assert.Contains(t, string(actual), subcommand)
					assert.NotContains(t, string(actual), "--dry-run", "--dry-run is an Atmos flag, not a Packer flag")
					return
				}
				require.NoFileExists(t, marker, "--dry-run must not start packer %s", subcommand)
				require.NoFileExists(t, varfile, "--dry-run must not write the Packer variable file")
				assert.Contains(t, output(), binary, "--dry-run must print the command that would run")
				assert.Contains(t, output(), subcommand)
			})
		}
	}
}
