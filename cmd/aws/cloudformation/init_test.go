package cloudformation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterTemplateBatch(t *testing.T) {
	root := t.TempDir()
	config := schema.AtmosConfiguration{BasePathAbsolute: root, CloudFormationDirAbsolutePath: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), Stacks: schema.Stacks{IncludedPaths: []string{"*.yaml"}}}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	info := schema.ConfigAndStacksInfo{Stack: "dev", DryRun: true}
	path := filepath.Join(root, "templates", "queue.yaml")
	names := []string{"orders-blue", "orders-green"}
	require.NoError(t, registerTemplate(cmd, &config, true, &info, templateSource{path, names}))
	require.NoFileExists(t, filepath.Join(root, "atmos.yaml"))
	info.DryRun = false
	require.NoError(t, registerTemplate(cmd, &config, true, &info, templateSource{path, names}))
	file := filepath.Join(root, "stacks", "dev.yaml")
	original, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(original), "stack_name: orders-blue")
	require.Contains(t, string(original), "stack_name: orders-green")
	require.Contains(t, string(original), "component: templates")
	require.NoError(t, registerTemplate(cmd, &config, false, &info, templateSource{path, names}))
	require.ErrorContains(t, registerTemplate(cmd, &config, false, &info, templateSource{path + ".json", []string{"orders-red", "orders-blue"}}), "already exists")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, original, after)
}

func TestInitManifestConfiguredLayout(t *testing.T) {
	config := schema.AtmosConfiguration{StacksBaseAbsolutePath: t.TempDir(), Stacks: schema.Stacks{IncludedPaths: []string{"deploy/**/*"}}}
	path, err := initManifest(&config, "dev")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(config.StacksBaseAbsolutePath, "deploy", "dev.yaml"), path)
}
