package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

const renderedBaseScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: rendered-base
spec:
  fields:
    - name: project_name
      type: input
      required: true
`

func renderedBaseConfig() *templates.Configuration {
	return &templates.Configuration{
		Name: "rendered-base",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: renderedBaseScaffoldYAML, Permissions: 0o644},
			{Path: "README.md", Content: "# {{ .Config.project_name }}\n", IsTemplate: true, Permissions: 0o644},
			{Path: "static.txt", Content: "static content\n", Permissions: 0o644},
		},
	}
}

func TestRenderPristineBase_RendersOldConfigWithOldValues(t *testing.T) {
	ui := createTestUI(t)

	tempDir, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"})
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	readme, err := os.ReadFile(filepath.Join(tempDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# old-project\n", string(readme))

	static, err := os.ReadFile(filepath.Join(tempDir, "static.txt"))
	require.NoError(t, err)
	assert.Equal(t, "static content\n", string(static))

	// The pristine render itself must never be recorded as a real generation.
	_, err = os.Stat(filepath.Join(tempDir, ".atmos", "scaffold.yaml"))
	assert.True(t, os.IsNotExist(err), "renderPristineBase must not write a project record")
	_, err = os.Stat(filepath.Join(tempDir, "scaffold.yaml"))
	assert.True(t, os.IsNotExist(err), "scaffold.yaml itself is schema-only and must not be written to the render")
}

func TestRenderPristineBase_DoesNotLeakOutputIntoRealBuffer(t *testing.T) {
	ui := createTestUI(t)
	ui.writeOutput("existing output\n")

	_, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	assert.Equal(t, "existing output\n", ui.output.String(), "the pristine render's own progress lines must not interleave with the real run's output")
}

func TestRenderPristineBase_MissingScaffoldConfigErrors(t *testing.T) {
	ui := createTestUI(t)
	cfg := &templates.Configuration{
		Name:  "no-scaffold-yaml",
		Files: []templates.File{{Path: "README.md", Content: "static\n", Permissions: 0o644}},
	}

	_, _, err := ui.renderPristineBase(cfg, map[string]interface{}{})

	require.Error(t, err)
}

func TestRenderPristineBase_CleanupRemovesTempDir(t *testing.T) {
	ui := createTestUI(t)

	tempDir, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"})
	require.NoError(t, err)

	cleanup()

	_, statErr := os.Stat(tempDir)
	assert.True(t, os.IsNotExist(statErr), "cleanup must remove the temp directory")
}

func TestSetupUpdateBase_TrackedWithEmptyBaseRef_IsNoOp(t *testing.T) {
	ui := createTestUI(t)
	// UpdateStrategyTracked is the zero value; no SetUpdateStrategy call needed.

	cleanup, err := ui.setupUpdateBase(t.TempDir(), "")

	require.NoError(t, err)
	require.NotNil(t, cleanup)
	cleanup() // Must not panic even though tracked mode never set anything up.
}

// TestSetupUpdateBase_Rendered_WiresRenderedBaseIntoProcessor proves
// setupUpdateBase's rendered branch really does point the Processor's 3-way
// merge base at the pristine old-ref render, rather than just checking that
// it returns no error.
//
// The test file static.txt's on-disk content below is left identical to the
// pristine render's own static.txt (the "base"), so the merge is a clean
// base==ours case that only succeeds -- taking the template's new content --
// if the base storage was actually wired up; without it, update-mode
// ProcessFile fails outright with ErrThreeWayMerge before ever reaching the
// merge itself (see Processor.baseStorage == nil in handleExistingFile).
func TestSetupUpdateBase_Rendered_WiresRenderedBaseIntoProcessor(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	ui.SetRenderedBaseSource(renderedBaseConfig(), map[string]interface{}{"project_name": "base-project"})

	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "static.txt"), []byte("static content\n"), 0o644))

	cleanup, err := ui.setupUpdateBase(targetDir, "")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	err = ui.processor.ProcessFile(engine.File{Path: "static.txt", Content: "template content\n", Permissions: 0o644}, targetDir, false, true, nil, nil)
	require.NoError(t, err)

	merged, err := os.ReadFile(filepath.Join(targetDir, "static.txt"))
	require.NoError(t, err)
	assert.Equal(t, "template content\n", string(merged))
}

func TestSetupUpdateBase_Rendered_PropagatesRenderFailure(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	ui.SetRenderedBaseSource(&templates.Configuration{
		Name:  "no-scaffold-yaml",
		Files: []templates.File{{Path: "README.md", Content: "static\n", Permissions: 0o644}},
	}, map[string]interface{}{})

	_, err := ui.setupUpdateBase(t.TempDir(), "")

	require.Error(t, err)
}
