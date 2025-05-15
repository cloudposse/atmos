package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteListDeploymentsCmd tests the ExecuteListDeploymentsCmd function
func TestExecuteListDeploymentsCmd(t *testing.T) {
	// Create a new command for testing
	cmd := &cobra.Command{}
	cmd.Flags().Bool("drift-enabled", false, "Filter deployments with drift detection enabled")
	cmd.Flags().Bool("upload", false, "Upload deployments to pro API")

	// Test with default flags
	t.Run("default flags", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{}
		err := ExecuteListDeploymentsCmd(info, cmd, []string{})
		require.NoError(t, err)
	})

	// Test with drift detection enabled
	t.Run("drift detection enabled", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{}
		cmd.Flags().Set("drift-enabled", "true")
		err := ExecuteListDeploymentsCmd(info, cmd, []string{})
		require.NoError(t, err)
	})

	// Test with upload enabled
	t.Run("upload enabled", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{}
		cmd.Flags().Set("upload", "true")
		err := ExecuteListDeploymentsCmd(info, cmd, []string{})
		// Note: This might fail if ATMOS_PRO_TOKEN is not set, which is expected
		if err != nil {
			assert.Contains(t, err.Error(), "ATMOS_PRO_TOKEN is not set")
		}
	})

	// Test with both flags enabled
	t.Run("both flags enabled", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{}
		cmd.Flags().Set("drift-enabled", "true")
		cmd.Flags().Set("upload", "true")
		err := ExecuteListDeploymentsCmd(info, cmd, []string{})
		// Note: This might fail if ATMOS_PRO_TOKEN is not set, which is expected
		if err != nil {
			assert.Contains(t, err.Error(), "ATMOS_PRO_TOKEN is not set")
		}
	})
}
