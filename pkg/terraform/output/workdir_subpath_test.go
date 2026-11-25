package output

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Output reads must resolve the same module directory as command execution, including
// when provisioning has already completed and the next read reuses its workdir.
func TestOutputProvisioningResolvesModuleSubpath(t *testing.T) {
	for _, tc := range []struct {
		name       string
		subpath    string
		createFile bool
		wantError  bool
	}{
		{name: "nested module", subpath: "modules/network"},
		{name: "inheritance identity falls back to root", subpath: "abstract-base"},
		{name: "file cannot be module directory", subpath: "invalid", createFile: true, wantError: true},
		{name: "absolute subpath rejected", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ResetWorkdirProvisionCache()
			t.Cleanup(ResetWorkdirProvisionCache)
			root := t.TempDir()
			module := filepath.Join(root, "modules", "network")
			require.NoError(t, os.MkdirAll(module, 0o755))
			if tc.createFile {
				require.NoError(t, os.WriteFile(filepath.Join(root, tc.subpath), []byte("not a module"), 0o600))
			}
			subpath := tc.subpath
			if subpath == "" {
				subpath = module
			}
			sections := jitSections()
			sections["metadata"] = map[string]any{"component": subpath}
			provisioner := NewMockWorkdirProvisioner(gomock.NewController(t))
			provisioner.EXPECT().Provision(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
			executor := NewExecutor(nil, WithWorkdirProvisioner(provisioner))
			for range 2 {
				// A freshly described component starts with the package root on every lookup.
				config := &ComponentConfig{ComponentPath: root, AutoProvisionWorkdirForOutputs: true}
				err := executor.ensureWorkdirProvisioned(context.Background(), &schema.AtmosConfiguration{BasePath: root}, sections, nil, "network", "dev", config)
				if tc.wantError {
					require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
					require.Equal(t, root, config.ComponentPath, "failed resolution must not publish an invalid execution path")
					continue
				}
				require.NoError(t, err)
				want := root
				if tc.subpath == "modules/network" {
					want = module
				}
				require.Equal(t, want, config.ComponentPath)
				require.DirExists(t, config.ComponentPath)
			}
		})
	}
}
