package terraform_backend_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSourceWorkdirStateUsesOverrideAndNestedModule(t *testing.T) {
	for _, explicitBackend := range []bool{false, true} {
		t.Run(map[bool]string{false: "workspace", true: "backend-path"}[explicitBackend], func(t *testing.T) {
			base := t.TempDir()
			config := &schema.AtmosConfiguration{BasePath: base}
			sections := map[string]any{
				"atmos_component": "instance", "atmos_stack": "dev", "workspace": "dev",
				"source": "./source", "component": "module",
				"metadata":  map[string]any{"working_directory": "custom", "component": "modules/child"},
				"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			}
			relativeState := filepath.Join("terraform.tfstate.d", "dev", "terraform.tfstate")
			if explicitBackend {
				relativeState = "saved-state.json"
				sections["backend"] = map[string]any{"path": relativeState}
			}
			statePath := filepath.Join(base, "custom", "modules", "child", relativeState)
			require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))
			state := []byte(`{"version":4,"serial":7,"lineage":"preserved","outputs":{}}`)
			require.NoError(t, os.WriteFile(statePath, state, 0o600))
			require.NoError(t, workdir.WriteMetadata(filepath.Join(base, "custom"), &workdir.WorkdirMetadata{Component: "instance", Stack: "dev"}))
			got, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
			require.NoError(t, err)
			require.Equal(t, state, got)

			// An occupied legacy target must produce a migration error, never a
			// missing-state fallback or a read from a different instance.
			legacy, err := workdir.BuildPath(base, "terraform", "instance", "dev", sections)
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(legacy, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(legacy, "terraform.tfstate"), []byte("legacy state"), 0o600))
			_, err = tb.ReadTerraformBackendLocal(config, &sections, nil)
			require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
			after, err := os.ReadFile(statePath)
			require.NoError(t, err)
			require.Equal(t, state, after)
		})
	}
}

func TestSourceWorkdirStateRejectsOtherOwner(t *testing.T) {
	for _, owner := range []struct{ component, stack string }{{"other", "dev"}, {"instance", "prod"}} {
		t.Run(owner.component+"-"+owner.stack, func(t *testing.T) {
			base := t.TempDir()
			config := &schema.AtmosConfiguration{BasePath: base}
			sections := map[string]any{
				"atmos_component": "instance", "atmos_stack": "dev", "workspace": "default",
				"source": "./source", "component": "module",
				"metadata":  map[string]any{"working_directory": "custom"},
				"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			}
			target := filepath.Join(base, "custom")
			require.NoError(t, workdir.WriteMetadata(target, &workdir.WorkdirMetadata{Component: owner.component, Stack: owner.stack}))
			state := []byte(`{"version":4,"outputs":{"private":{"value":"other-instance"}}}`)
			statePath := filepath.Join(target, "terraform.tfstate")
			require.NoError(t, os.WriteFile(statePath, state, 0o600))
			got, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
			require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
			require.Nil(t, got)
			after, err := os.ReadFile(statePath)
			require.NoError(t, err)
			require.Equal(t, state, after)
		})
	}
}
