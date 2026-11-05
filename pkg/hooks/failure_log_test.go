package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFailureLogFields(t *testing.T) {
	t.Run("message and hints without the error chain", func(t *testing.T) {
		err := fmt.Errorf("starlark execution failed: %w",
			errUtils.Build(errUtils.ErrInvalidConfig).
				WithHint("Check the `working_directory` setting.").
				WithHint("TITLE:ignored").
				Err())

		fields := failureLogFields(err)
		require.Len(t, fields, 4)
		assert.Equal(t, "error", fields[0])
		assert.Contains(t, fields[1], "starlark execution failed")
		assert.Equal(t, "hint", fields[2])
		assert.Equal(t, "Check the `working_directory` setting.", fields[3])
	})

	t.Run("no hint key without hints", func(t *testing.T) {
		fields := failureLogFields(errors.New("boom"))
		assert.Equal(t, []any{"error", "boom"}, fields)
	})
}

// TestApplyOnFailure_LogsMessageNotErrorChain verifies the warn path logs the
// failure as a message plus its hint rather than dumping the raw cockroachdb
// error chain ("forced error mark", "Error types: ...").
func TestApplyOnFailure_LogsMessageNotErrorChain(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	previous := log.GetLevel()
	log.SetLevel(log.WarnLevel)
	t.Cleanup(func() { log.SetLevel(previous) })

	runErr := fmt.Errorf("starlark execution failed: %w",
		errUtils.Build(errUtils.ErrInvalidConfig).WithHint("Check the `working_directory` setting.").Err())
	ctx := &ExecContext{Hook: &Hook{Kind: "step"}, Info: &schema.ConfigAndStacksInfo{}}

	require.NoError(t, applyOnFailure(ctx, runErr))

	out := buf.String()
	assert.Contains(t, out, "starlark execution failed")
	assert.Contains(t, out, "Check the `working_directory` setting.")
	assert.NotContains(t, out, "Error types:")
	assert.NotContains(t, out, "forced error mark")
	assert.NotContains(t, out, "Wraps:")
}

// TestRunAllResolvesComponentDirectoryFromRootModule verifies that a hook's
// default working directory is the Terraform root module (metadata.component),
// not the component instance name. Callers that never resolved the component
// (the multi-component dispatcher passes only the instance name) leave
// FinalComponent empty, so RunAll backfills it from the component sections it
// already described.
func TestRunAllResolvesComponentDirectoryFromRootModule(t *testing.T) {
	const (
		instance   = "mycomponent"
		rootModule = "mock"
	)
	base := t.TempDir()
	config := &schema.AtmosConfiguration{TerraformDirAbsolutePath: base}

	run := func(t *testing.T, sections map[string]any, info *schema.ConfigAndStacksInfo) *ExecContext {
		t.Helper()
		engine := &depthRecorder{}
		h := depthHooks(registerDepthKind(t, engine))
		h.sections = sections
		h.config = config
		require.NoError(t, h.RunAll(AfterTerraformPlan, config, info, nil, nil))
		require.Len(t, engine.seen, 1)
		return engine.seen[0]
	}
	newInfo := func() *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{ComponentFromArg: instance, Stack: "prod", ComponentType: "terraform"}
	}

	t.Run("unresolved info uses the root module", func(t *testing.T) {
		info := newInfo()
		ctx := run(t, map[string]any{"component": rootModule}, info)
		assert.Equal(t, filepath.Join(base, rootModule), ComponentPath(ctx))
		assert.Empty(t, info.FinalComponent, "the caller's info must not be mutated")
	})

	t.Run("root module with a folder", func(t *testing.T) {
		ctx := run(t, map[string]any{"component": "shared/mock"}, newInfo())
		assert.Equal(t, filepath.Join(base, "shared", "mock"), ComponentPath(ctx))
	})

	t.Run("resolved info wins", func(t *testing.T) {
		info := newInfo()
		info.FinalComponent = "already-resolved"
		ctx := run(t, map[string]any{"component": rootModule}, info)
		assert.Equal(t, filepath.Join(base, "already-resolved"), ComponentPath(ctx))
	})

	t.Run("no component section falls back to the instance name", func(t *testing.T) {
		ctx := run(t, map[string]any{}, newInfo())
		assert.Equal(t, filepath.Join(base, instance), ComponentPath(ctx))
	})
}
