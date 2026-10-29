package output

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkdirProvisioningIsolatesCheckoutsAndSourceVersions(t *testing.T) {
	ResetWorkdirProvisionCache()
	t.Cleanup(ResetWorkdirProvisionCache)
	ctrl := gomock.NewController(t)
	provisioner := NewMockWorkdirProvisioner(ctrl)
	executor := NewExecutor(nil, WithWorkdirProvisioner(provisioner))
	provisioner.EXPECT().Provision(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(3)
	root := t.TempDir()
	for _, checkout := range []string{"HEAD", "BASE"} {
		base := filepath.Join(root, checkout)
		require.NoError(t, os.MkdirAll(base, 0o755))
		config := &ComponentConfig{ComponentPath: base, AutoProvisionWorkdirForOutputs: true}
		sections := jitSections()
		sections["source"] = map[string]any{"uri": "git::fixture", "version": "v1"}
		for range 2 {
			require.NoError(t, executor.ensureWorkdirProvisioned(context.Background(), &schema.AtmosConfiguration{BasePath: base}, sections, nil, "vpc", "dev", config))
		}
		if checkout == "HEAD" {
			sections["source"] = map[string]any{"uri": "git::fixture", "version": "v2"}
			require.NoError(t, executor.ensureWorkdirProvisioned(context.Background(), &schema.AtmosConfiguration{BasePath: base}, sections, nil, "vpc", "dev", config))
		}
	}
}

func TestWorkdirProvisioningRecoversRemovedTarget(t *testing.T) {
	ResetWorkdirProvisionCache()
	t.Cleanup(ResetWorkdirProvisionCache)
	ctrl := gomock.NewController(t)
	provisioner := NewMockWorkdirProvisioner(ctrl)
	executor := NewExecutor(nil, WithWorkdirProvisioner(provisioner))
	target := filepath.Join(t.TempDir(), "workdir")
	provisioner.EXPECT().Provision(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.AtmosConfiguration, map[string]any, *schema.AuthContext) error {
			return os.MkdirAll(target, 0o755)
		},
	).Times(2)
	config := &ComponentConfig{ComponentPath: target, AutoProvisionWorkdirForOutputs: true}
	for range 2 {
		require.NoError(t, executor.ensureWorkdirProvisioned(context.Background(), &schema.AtmosConfiguration{}, jitSections(), nil, "vpc", "dev", config))
		require.DirExists(t, target)
		require.NoError(t, os.Remove(target))
	}
}

func TestWorkdirProvisioningIsolatesChangedCredentials(t *testing.T) {
	ResetWorkdirProvisionCache()
	t.Cleanup(ResetWorkdirProvisionCache)
	provisioner := NewMockWorkdirProvisioner(gomock.NewController(t))
	executor := NewExecutor(nil, WithWorkdirProvisioner(provisioner))
	provisioner.EXPECT().Provision(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	base := t.TempDir()
	config := &ComponentConfig{ComponentPath: base, AutoProvisionWorkdirForOutputs: true}
	for _, credential := range []string{"initial-scope", "initial-scope", "rotated-scope"} {
		sections := jitSections()
		sections["auth"] = map[string]any{"password": credential}
		require.NoError(t, executor.ensureWorkdirProvisioned(t.Context(), &schema.AtmosConfiguration{BasePath: base}, sections, nil, "vpc", "dev", config))
	}
}
