package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkspaceSetupInitRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, output            string
		skip, never, persistent bool
		wantInit                int
	}{
		{name: "missing provider", output: "Required plugins are not installed", wantInit: 1},
		{name: "skip init", output: "Required plugins are not installed", skip: true},
		{name: "mode never", output: "Required plugins are not installed", never: true},
		{name: "unknown provider error", output: "Failed to load plugin schemas: permission denied"},
		{name: "auth error", output: "AccessDenied: expired credentials"},
		{name: "state lock", output: "Error acquiring the state lock"},
		{name: "bounded persistent failure", output: "Required plugins are not installed", persistent: true, wantInit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exe, err := os.Executable()
			require.NoError(t, err)
			path := t.TempDir()
			argsFile := filepath.Join(path, "args.txt")
			cfg := &schema.AtmosConfiguration{}
			if tc.never {
				cfg.Components.Terraform.Init.Mode = schema.TerraformInitModeNever
			}
			info := &schema.ConfigAndStacksInfo{SubCommand: "plan", Command: exe, TerraformWorkspace: "dev", SkipInit: tc.skip, ComponentEnvList: []string{"_ATMOS_TEST_EXIT_ONE=1", "_ATMOS_TEST_STDOUT=" + tc.output}}
			executor := testExecutor(t)
			calls := 0
			executor.executeTerraformInitForcedFn = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _, _ string, reconfigure, upgrade bool, _ ...component.ExecutionOption) error {
				calls++
				assert.False(t, reconfigure)
				assert.False(t, upgrade)
				if !tc.persistent {
					info.ComponentEnvList = []string{"_ATMOS_TEST_ARGS_FILE=" + argsFile}
				}
				return nil
			}
			err = executor.runWorkspaceSetupPhase(cfg, info, path)
			assert.Equal(t, tc.wantInit, calls)
			if tc.wantInit == 1 && !tc.persistent {
				require.NoError(t, err)
				content, e := os.ReadFile(argsFile)
				require.NoError(t, e)
				assert.Equal(t, "workspace\nselect\n-or-create\ndev", string(content))
			} else {
				require.Error(t, err)
			}
		})
	}
}
