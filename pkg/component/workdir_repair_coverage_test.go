package component

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRebuiltSourceWorkdirUsesExplicitRootAndNestedModule(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "explicit")
	nested := filepath.Join(explicit, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, workdir.WriteMetadata(explicit, &workdir.WorkdirMetadata{Component: "alias", Stack: "dev"}))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "terraform.tfstate"), []byte("existing state"), 0o600))
	info := &schema.ConfigAndStacksInfo{FinalComponent: "alias", Stack: "dev", BaseComponentPath: "nested", ComponentSection: map[string]any{
		"component": "module", "atmos_component": "alias", "atmos_stack": "dev",
		"source": map[string]any{"uri": "./not-fetched"}, "settings": map[string]any{"working_directory": explicit},
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
	}}
	path, exists, err := BuildAndResolveWorkdirPath(&schema.AtmosConfiguration{BasePath: root}, info, config.TerraformComponentType)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, nested, path, "rebuilt metadata must find the same explicit source module")
	content, err := os.ReadFile(filepath.Join(path, "terraform.tfstate"))
	require.NoError(t, err)
	require.Equal(t, "existing state", string(content))
	require.NoDirExists(t, filepath.Join(root, ".workdir"))
}

func TestSourcePathValidationRemainsActiveDuringDryRunAndRebuild(t *testing.T) {
	for _, operation := range []string{"dry-run source", "dry-run subpath", "rebuild source"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			section := map[string]any{"component": "module", "atmos_component": "alias", "atmos_stack": "dev", "source": map[string]any{"uri": "./missing"}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}}
			info := &schema.ConfigAndStacksInfo{DryRun: true, FinalComponent: "alias", Stack: "dev", ComponentSection: section}
			if operation == "dry-run subpath" {
				info.BaseComponentPath = filepath.Join(root, "outside")
			} else {
				section["component"] = "../outside"
			}
			cfg := &schema.AtmosConfiguration{BasePath: root}
			var err error
			if operation == "rebuild source" {
				_, _, err = BuildAndResolveWorkdirPath(cfg, info, config.TerraformComponentType)
			} else {
				_, _, err = ProvisionAndResolveComponentPath(context.Background(), provisioner.OutputWriters{}, cfg, info, config.TerraformComponentType, root)
			}
			if operation == "dry-run subpath" {
				require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
			} else {
				require.ErrorIs(t, err, errUtils.ErrSourceComponentNameInvalid)
			}
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries, "invalid paths must fail before provisioning anything")
		})
	}
}
