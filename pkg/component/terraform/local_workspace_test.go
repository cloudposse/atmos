package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestScopeLocalPlanWorkspace(t *testing.T) {
	t.Setenv("TF_WORKSPACE", "")
	t.Setenv("TF_DATA_DIR", "")
	root := t.TempDir()
	config := &schema.AtmosConfiguration{}
	config.Stacks.IncludedPaths = []string{"*.yaml"}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan", TerraformWorkspace: "blue", ComponentMetadataSection: map[string]any{"component": "network", "terraform_workspace": "blue"}}
	require.NoError(t, scopeLocalPlanWorkspace(config, info, root))
	require.False(t, hasScopedWorkspace(info))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "terraform.tfstate.d", "blue"), 0o755))
	require.NoError(t, scopeLocalPlanWorkspace(config, info, root))
	require.True(t, hasScopedWorkspace(info))
	require.True(t, shouldSkipWorkspaceSetup(info))
	info.ComponentEnvList = []string{"TF_WORKSPACE=other"}
	require.ErrorContains(t, scopeLocalPlanWorkspace(config, info, root), "differs")
	info.ComponentEnvList = nil
	info.SubCommand = "workspace"
	require.NoError(t, scopeLocalPlanWorkspace(config, info, root))
	require.False(t, hasScopedWorkspace(info))
	info.SubCommand = "plan"
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("terraform {\n backend \"s3\" {}\n}"), 0o600))
	require.NoError(t, scopeLocalPlanWorkspace(config, info, root))
	require.False(t, hasScopedWorkspace(info))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("terraform {"), 0o600))
	require.Error(t, scopeLocalPlanWorkspace(config, info, root))
	info.ComponentMetadataSection = nil
	require.NoError(t, scopeLocalPlanWorkspace(config, info, root))
	require.False(t, hasScopedWorkspace(info))
}
