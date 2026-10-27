package step

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCastTapeRequirementsUseEffectiveEnvironment(t *testing.T) {
	dir := t.TempDir()
	tool := "cast-review-tool"
	if runtime.GOOS == "windows" {
		tool += ".exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, tool), []byte("executable placeholder"), 0o700))
	for _, mode := range []string{"steps", "session"} {
		t.Run(mode, func(t *testing.T) {
			vars := NewVariables()
			originalPath := t.TempDir()
			vars.SetEnv("PATH", originalPath)
			vars.SetEnv("CAST_TOOL_DIR", dir)
			step := &schema.WorkflowStep{
				Name: "demo", Mode: mode, Tools: []string{tool},
				Env: map[string]string{"PATH": "{{ .Env.CAST_TOOL_DIR }}"},
			}
			require.NoError(t, runCastTapeRequirements(context.Background(), step, vars))
			assert.Equal(t, originalPath, vars.Env["PATH"], "preflight must not persist cast env")
			assert.NotContains(t, vars.Steps, "demo_require", "preflight results stay isolated")

			step.Env["PATH"] = t.TempDir()
			require.ErrorIs(t, runCastTapeRequirements(context.Background(), step, vars), errUtils.ErrRequirementsNotMet)
			assert.Equal(t, originalPath, vars.Env["PATH"])

			step.Env["PATH"] = "{{"
			err := runCastTapeRequirements(context.Background(), step, vars)
			require.ErrorContains(t, err, "step 'demo'")
			assert.Equal(t, originalPath, vars.Env["PATH"])

			step.Env = nil
			vars.SetEnv("PATH", dir)
			require.NoError(t, runCastTapeRequirements(context.Background(), step, vars))
		})
	}
}

func TestCastSessionActionsDispatchVisibilityNatively(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Mode: "session",
		Tape: "Hide\nType \"secret command\" Enter\nShow\nType \"visible command\" Enter",
	}
	require.NoError(t, expandCastTape(step))
	actions := castSessionActions(context.Background(), step, NewVariables(), nil)
	assert.Equal(t, []asciicast.SessionAction{
		{Type: "hide"},
		{Type: "write", Text: "secret command"},
		{Type: "key", Key: "enter"},
		{Type: "show"},
		{Type: "write", Text: "visible command"},
		{Type: "key", Key: "enter"},
	}, actions)
}

func TestCastTapeStepsRejectSetupOnlyHideBlock(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Mode: "steps",
		Tape: "Hide\nType \"export DEMO=value\" Enter\nType \"clear\" Enter\nShow",
	}
	require.ErrorIs(t, expandCastTape(step), errUtils.ErrTapeDirectiveRequiresSessionMode)
}
