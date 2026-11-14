package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSourceWorkingDirectoryPrecedenceEditions(t *testing.T) {
	for _, field := range []string{"metadata", "settings"} {
		for _, pin := range []string{"2026-10-27", "2026-10-28", "2026-11", "", "invalid"} {
			t.Run(field+"/"+pin, func(t *testing.T) {
				base := t.TempDir()
				config := &schema.AtmosConfiguration{BasePath: base, Edition: pin}
				section := map[string]any{
					"atmos_component": "nested/instance", "atmos_stack": "dev",
					"component": "module", "source": "./source",
					"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
					field:       map[string]any{"working_directory": "custom/instance"},
				}
				legacy, err := workdir.BuildPath(base, "terraform", "module", "dev", section)
				require.NoError(t, err)
				target, err := ResolveTarget(config, "terraform", "module", section)
				if pin == "invalid" {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.True(t, target.IsWorkdir)
				if pin == "2026-10-27" {
					assert.Equal(t, legacy, target.Dir)
				} else {
					assert.Equal(t, filepath.Join(base, "custom", "instance"), target.Dir)
				}
				require.NoError(t, os.MkdirAll(legacy, 0o755))
				state := filepath.Join(legacy, "custom-state")
				require.NoError(t, os.WriteFile(state, []byte("state must survive"), 0o600))
				_, err = ResolveTarget(config, "terraform", "module", section)
				if pin == "2026-10-27" {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
				}
				content, err := os.ReadFile(state)
				require.NoError(t, err)
				assert.Equal(t, "state must survive", string(content))
			})
		}
	}
}

func TestSourceWorkingDirectoryRetainsExplicitLegacyPath(t *testing.T) {
	base := t.TempDir()
	section := map[string]any{
		"component": "module", "atmos_component": "instance", "atmos_stack": "dev",
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
	}
	legacy, err := workdir.BuildPath(base, "terraform", "module", "dev", section)
	require.NoError(t, err)
	section["settings"] = map[string]any{"working_directory": legacy}
	require.NoError(t, workdir.WriteMetadata(legacy, &workdir.WorkdirMetadata{Component: "instance", Stack: "dev"}))
	refreshWrite(t, legacy, "terraform.tfstate", "preserved")
	target, err := ResolveTarget(&schema.AtmosConfiguration{BasePath: base}, "terraform", "module", section)
	require.NoError(t, err)
	require.Equal(t, legacy, target.Dir, "an explicit existing path is not a migration")
	contents, err := os.ReadFile(filepath.Join(legacy, "terraform.tfstate"))
	require.NoError(t, err)
	require.Equal(t, "preserved", string(contents))
}

func TestSourceWorkingDirectoryRefusesUninspectableLegacyPath(t *testing.T) {
	base := t.TempDir()
	section := map[string]any{
		"component": "module", "atmos_component": "instance", "atmos_stack": "dev",
		"settings":  map[string]any{"working_directory": "custom"},
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
	}
	legacy, err := workdir.BuildPath(base, "terraform", "module", "dev", section)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	require.NoError(t, os.WriteFile(legacy, []byte("unexpected caller data"), 0o600))
	_, err = ResolveTarget(&schema.AtmosConfiguration{BasePath: base}, "terraform", "module", section)
	require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
	require.ErrorContains(t, err, "inspect existing source workdir")
	require.NoDirExists(t, filepath.Join(base, "custom"))
	contents, err := os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, "unexpected caller data", string(contents))
}
