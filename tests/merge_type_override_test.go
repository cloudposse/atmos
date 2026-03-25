package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMergeTypeOverride_StackComposition exercises the merge-type-override fixture
// through the full stack-composition pipeline, verifying that type overrides
// (list→map, list→scalar, list→null) work end-to-end when loaded and merged
// via the command layer.
func TestMergeTypeOverride_StackComposition(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "merge-type-override"))

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Verify the sandbox EKS stack resolves — it overrides a list with {} (empty map).
	t.Run("list overridden by empty map", func(t *testing.T) {
		section, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "eks-cluster",
				Stack:     "sandbox-use1-sandbox",
			},
		)
		require.NoError(t, err, "eks-cluster in sandbox must resolve despite list→map override")
		require.NotNil(t, section)

		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok)

		// allowed_accounts was a list in defaults, overridden with {} in sandbox.
		accounts, ok := vars["allowed_accounts"].(map[string]any)
		require.True(t, ok, "allowed_accounts should be an empty map after override")
		assert.Empty(t, accounts, "allowed_accounts should be empty")

		// rbac_roles was a list in defaults, overridden with [] in sandbox (same type).
		roles, ok := vars["rbac_roles"].([]any)
		require.True(t, ok, "rbac_roles should be an empty list after override")
		assert.Empty(t, roles)

		// node_groups should be deep-merged.
		nodeGroups, ok := vars["node_groups"].(map[string]any)
		require.True(t, ok)
		main, ok := nodeGroups["main"].(map[string]any)
		require.True(t, ok)
		instanceTypes, ok := main["instance_types"].([]any)
		require.True(t, ok)
		assert.Equal(t, "c6a.xlarge", instanceTypes[0], "instance_types should be overridden")
	})

	// Verify the sandbox VPC stack resolves — it overrides a list with a scalar.
	t.Run("list overridden by scalar", func(t *testing.T) {
		section, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "vpc",
				Stack:     "sandbox-use1-sandbox",
			},
		)
		require.NoError(t, err, "vpc in sandbox must resolve despite list→scalar override")
		require.NotNil(t, section)

		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "10.99.0.0/16", vars["cidr_blocks"],
			"cidr_blocks should be the scalar string from sandbox override")
	})

	// Verify the sandbox S3 stack resolves — it overrides a list with null.
	t.Run("list overridden by null", func(t *testing.T) {
		section, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "s3-bucket",
				Stack:     "sandbox-use1-sandbox",
			},
		)
		require.NoError(t, err, "s3-bucket in sandbox must resolve despite list→null override")
		require.NotNil(t, section)

		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok)
		assert.Nil(t, vars["lifecycle_rules"], "lifecycle_rules should be null after override")
	})

	// Verify the dev EKS stack resolves normally (no type overrides).
	t.Run("no type override baseline", func(t *testing.T) {
		section, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "eks-cluster",
				Stack:     "dev-use1-dev",
			},
		)
		require.NoError(t, err, "eks-cluster in dev must resolve normally")
		require.NotNil(t, section)

		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok)

		// allowed_accounts should be the original list from defaults.
		accounts, ok := vars["allowed_accounts"].([]any)
		require.True(t, ok, "allowed_accounts should be a list in dev (no override)")
		assert.Len(t, accounts, 2, "allowed_accounts should have 2 entries from defaults")
	})
}
