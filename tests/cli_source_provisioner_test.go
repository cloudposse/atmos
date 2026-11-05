package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
)

// resetSourceCommandState isolates the shared Terraform source command flags
// and Viper before and after a test. Viper.Reset alone leaves Cobra's parsed
// persistent --dry-run and --stack values, and source --force values, intact.
func resetSourceCommandState(t *testing.T) {
	t.Helper()
	var commandFlags []*pflag.Flag
	for _, path := range [][]string{
		{"terraform"},
		{"terraform", "source", "describe"},
		{"terraform", "source", "list"},
		{"terraform", "source", "pull"},
		{"terraform", "source", "delete"},
	} {
		command, _, err := cmd.RootCmd.Find(path)
		require.NoError(t, err)
		for _, name := range []string{"dry-run", "stack", "force"} {
			if flag := command.Flag(name); flag != nil {
				commandFlags = append(commandFlags, flag)
			}
		}
	}
	reset := func() {
		viper.Reset()
		cmd.RootCmd.SetArgs(nil)
		for _, flag := range commandFlags {
			require.NoError(t, flag.Value.Set(flag.DefValue))
			flag.Changed = false
		}
	}
	reset()
	t.Cleanup(reset)
}

// sourceFixture isolates writable acceptance fixtures from the checked-in tree.
func sourceFixture(t *testing.T, name string) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join("fixtures", "scenarios", name))
	require.NoError(t, err)
	sandbox := t.TempDir()
	require.NoError(t, os.CopyFS(sandbox, os.DirFS(fixture)))
	t.Chdir(sandbox)
}

// TestSourceProvisionerDescribe_Success tests the `atmos terraform source describe` command.
func TestSourceProvisionerDescribe_Success(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-map", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceProvisionerDescribe_URIWithRef tests source with version in URI.
func TestSourceProvisionerDescribe_URIWithRef(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-inline-ref", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceProvisionerDescribe_WithRetry tests source with retry configuration.
func TestSourceProvisionerDescribe_WithRetry(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-retry", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceProvisionerDescribe_NoSource tests error when component has no source.
func TestSourceProvisionerDescribe_NoSource(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-no-source", "--stack", "dev"})

	err := cmd.Execute()
	// Should return error because component has no source configured.
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "source") || strings.Contains(err.Error(), "metadata"),
		"Expected error about missing source")
}

// TestSourceProvisionerDescribe_MissingStack tests error when --stack is not provided.
// Note: This test may be affected by state leakage from previous tests in the same package.
// The Viper state may retain the --stack value from previous tests.
func TestSourceProvisionerDescribe_MissingStack(t *testing.T) {
	t.Skip("Skipping due to Viper state leakage between tests - stack flag persists from previous tests")
}

// TestSourceProvisionerList tests the `atmos terraform source list` command.
func TestSourceProvisionerList(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "list", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceProvisionerDelete_MissingForce tests that delete requires --force flag.
func TestSourceProvisionerDelete_MissingForce(t *testing.T) {
	resetSourceCommandState(t)
	t.Setenv("ATMOS_DRY_RUN", "false")
	t.Setenv("ATMOS_INTERACTIVE", "false")
	sourceFixture(t, "source-provisioner")

	// Create the target directory so delete has something to operate on.
	targetDir := "components/terraform/vpc-map"
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "delete", "vpc-map", "--stack", "dev"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "force") || strings.Contains(err.Error(), "--force") ||
		strings.Contains(err.Error(), "interactive"),
		"Expected error about missing --force flag or non-interactive mode")
}
