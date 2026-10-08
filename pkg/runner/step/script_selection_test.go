package step

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A nested Atmos command started by a script step runs with the profile and identity chosen on the
// parent's command line, the same as one started by a standalone script or a `type: atmos` step.
// Those selections exist only in the parent's memory until they are forwarded through the
// environment.
func TestScriptHandler_ForwardsProfileAndIdentity(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get("script")
	require.True(t, ok)
	exe, err := os.Executable()
	require.NoError(t, err)

	run := func(t *testing.T, env map[string]string) string {
		t.Helper()
		env["_ATMOS_STEP_FAKE"] = "echo-selection"
		env["WP5_BIN"] = exe
		step := &schema.WorkflowStep{
			Name: "nested", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: string(OutputModeNone), Env: env,
			Script: `output = exec.run([env["WP5_BIN"]], output="capture").stdout`,
		}
		result, err := handler.Execute(context.Background(), step, NewVariables())
		require.NoError(t, err)
		return result.Value
	}

	t.Setenv("ATMOS_PROFILE", "")
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
	t.Setenv("ATMOS_IDENTITY", "")
	require.NoError(t, os.Unsetenv("ATMOS_IDENTITY"))
	t.Cleanup(func() {
		cfg.GlobalViper().Set("profile", nil)
		cfg.GlobalViper().Set("identity", nil)
	})

	t.Run("nothing selected forwards nothing", func(t *testing.T) {
		assert.Equal(t, "|", run(t, map[string]string{}))
	})
	t.Run("profiles from the command line", func(t *testing.T) {
		cfg.GlobalViper().Set("profile", []string{"base", "dev"})
		defer cfg.GlobalViper().Set("profile", nil)
		assert.Equal(t, "base,dev|", run(t, map[string]string{}))
	})
	t.Run("identity from the command line", func(t *testing.T) {
		cfg.GlobalViper().Set("identity", "admin")
		defer cfg.GlobalViper().Set("identity", nil)
		assert.Equal(t, "|admin", run(t, map[string]string{}))
	})
	t.Run("the step's own env wins", func(t *testing.T) {
		cfg.GlobalViper().Set("profile", []string{"dev"})
		defer cfg.GlobalViper().Set("profile", nil)
		assert.Equal(t, "prod|", run(t, map[string]string{"ATMOS_PROFILE": "prod"}))
	})
}
