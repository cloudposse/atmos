package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
)

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

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-string", "--stack", "dev"})

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

	// Currently returns "not implemented" error.
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

// TestSourceProvisionerDelete_MissingForce tests that delete requires --force flag.
func TestSourceProvisionerDelete_MissingForce(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "delete", "vpc-map", "--stack", "dev"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "force") || strings.Contains(err.Error(), "--force"),
		"Expected error about missing --force flag")
}
