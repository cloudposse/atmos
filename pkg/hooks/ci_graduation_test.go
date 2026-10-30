package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestStableCIHooks verifies real output through the Terraform plugin and generic
// provider, including the local/disabled cases where no hook should execute.
func TestStableCIHooks(t *testing.T) {
	for _, mode := range []string{"disable", "error", "warn", "warn-daily", "silence"} {
		for _, scenario := range []struct {
			name    string
			enabled bool
			forced  bool
		}{
			{name: "enabled and forced", enabled: true, forced: true},
			{name: "disabled config", enabled: false, forced: true},
			{name: "local invocation", enabled: true, forced: false},
		} {
			t.Run(mode+"/"+scenario.name, func(t *testing.T) {
				t.Cleanup(ci.SwapRegistryForTest())
				ci.Register(generic.NewProvider())
				t.Setenv("CI", "")
				t.Setenv("GITHUB_ACTIONS", "")
				outputPath := filepath.Join(t.TempDir(), "outputs")
				t.Setenv("ATMOS_CI_OUTPUT", outputPath)
				t.Setenv("ATMOS_CI_SUMMARY", filepath.Join(t.TempDir(), "summary"))
				config := &schema.AtmosConfiguration{
					CI:       schema.CIConfig{Enabled: scenario.enabled},
					Settings: schema.AtmosSettings{Experimental: mode},
				}
				err := RunCIHooks(&RunCIHooksOptions{
					Event: AfterTerraformApply, AtmosConfig: config,
					Info:        &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
					Output:      "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.",
					ForceCIMode: scenario.forced,
				})
				require.NoError(t, err)
				if !scenario.enabled || !scenario.forced {
					assert.NoFileExists(t, outputPath)
					return
				}
				output, err := os.ReadFile(outputPath)
				require.NoError(t, err)
				assert.Contains(t, string(output), "component=vpc")
				assert.Contains(t, string(output), "stack=dev")
				assert.Contains(t, string(output), "success=true")
			})
		}
	}
}

func TestLifecycleWorkerModeControlsCIHooks(t *testing.T) {
	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(generic.NewProvider())
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "")
	output := filepath.Join(t.TempDir(), "output")
	summary := filepath.Join(t.TempDir(), "summary")
	t.Setenv("ATMOS_CI_OUTPUT", output)
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	config := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	opts := &RunCIHooksOptions{Event: AfterTerraformApply, AtmosConfig: config, Info: &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}, Output: "Apply complete! Resources: 1 added, 0 changed, 0 destroyed."}
	t.Setenv(ci.WorkerModeEnv, "false")
	require.NoError(t, RunCIHooks(opts))
	assert.NoFileExists(t, output)
	assert.NoFileExists(t, summary)
	assert.Equal(t, "true", os.Getenv("CI"))
	t.Setenv(ci.WorkerModeEnv, "true")
	require.NoError(t, RunCIHooks(opts))
	content, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Contains(t, string(content), "component=vpc")
	assert.Contains(t, string(content), "success=true")
}
