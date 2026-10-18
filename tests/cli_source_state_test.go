package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
)

func TestSourceCommands_IsolateDryRunState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fixture   string
		component string
		target    string
	}{
		{"component", "source-provisioner", "vpc-map", "components/terraform/vpc-map"},
		{"workdir", "source-provisioner-workdir", "vpc-remote-workdir", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetSourceCommandState(t)
			sourceFixture(t, tc.fixture)
			t.Setenv("ATMOS_INTERACTIVE", "false")
			t.Setenv("ATMOS_DRY_RUN", "false")
			target := tc.target
			if tc.name == "workdir" {
				var err error
				target, err = workdir.BuildPath(".", "terraform", tc.component, "dev", nil)
				require.NoError(t, err)
			}
			marker := filepath.Join(target, "preserved.tf")
			require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
			require.NoError(t, os.WriteFile(marker, []byte("# preserved\n"), 0o600))

			cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", tc.component, "--stack", "dev", "--dry-run"})
			require.NoError(t, cmd.Execute())
			terraform, _, err := cmd.RootCmd.Find([]string{"terraform"})
			require.NoError(t, err)
			dryRun := terraform.PersistentFlags().Lookup("dry-run")
			require.NotNil(t, dryRun)
			require.True(t, dryRun.Changed, "the preceding real CLI call must contaminate persistent command state")
			require.Equal(t, "true", dryRun.Value.String())

			t.Run("isolated invocation", func(t *testing.T) {
				resetSourceCommandState(t)
				require.False(t, dryRun.Changed, "source test setup must clear Cobra state as well as Viper")
				require.Equal(t, "false", dryRun.Value.String())
				cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", tc.component, "--stack", "dev", "--dry-run"})
				require.NoError(t, cmd.Execute())
				require.True(t, dryRun.Changed)
				require.Equal(t, "true", dryRun.Value.String())
			})
			require.False(t, dryRun.Changed, "test cleanup must remove the preceding invocation's flags")
			require.Equal(t, "false", dryRun.Value.String())
			cmd.RootCmd.SetArgs([]string{"terraform", "source", "delete", tc.component, "--stack", "dev"})
			require.ErrorIs(t, cmd.Execute(), errUtils.ErrInteractiveNotAvailable)
			content, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "# preserved\n", string(content))
		})
	}
}
