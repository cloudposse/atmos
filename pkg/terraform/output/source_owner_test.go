package output

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
)

func TestOutputConfigValidatesSourceOwnerWithoutProvisioning(t *testing.T) {
	for _, owner := range []struct{ component, stack string }{{"instance", "dev"}, {"other", "dev"}, {"instance", "prod"}} {
		t.Run(owner.component+"-"+owner.stack, func(t *testing.T) {
			base := t.TempDir()
			config := testAtmosConfig(base, true, false)
			config.Components.Terraform.AutoProvisionWorkdirForOutputs = false
			target := filepath.Join(base, "custom")
			require.NoError(t, workdir.WriteMetadata(target, &workdir.WorkdirMetadata{Component: owner.component, Stack: owner.stack}))
			sections := map[string]any{
				"atmos_component": "instance", "atmos_stack": "dev", "workspace": "dev",
				"command": "terraform", "component_info": map[string]any{"component_type": "terraform"},
				"source": "./source", "component": "module",
				"metadata":  map[string]any{"working_directory": "custom"},
				"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			}
			got, err := ExtractComponentConfig(config, sections, "instance", "dev")
			if owner.component != "instance" || owner.stack != "dev" {
				require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.False(t, got.AutoProvisionWorkdirForOutputs)
			require.Equal(t, target, got.ComponentPath)
		})
	}
}
