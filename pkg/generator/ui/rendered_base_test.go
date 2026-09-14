package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

	tempDir, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"}, nil)
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

	_, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"}, nil)
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

	_, _, err := ui.renderPristineBase(cfg, map[string]interface{}{}, nil)

	require.Error(t, err)
}

func TestRenderPristineBase_CleanupRemovesTempDir(t *testing.T) {
	ui := createTestUI(t)

	tempDir, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"}, nil)
	require.NoError(t, err)

	cleanup()

	_, statErr := os.Stat(tempDir)
	assert.True(t, os.IsNotExist(statErr), "cleanup must remove the temp directory")
}

// TestRenderPristineBase_WritesFilesEvenWhenProcessorDryRun proves the
// internal pristine render actually writes oldConfig's files to tempDir even
// when the outer run has enabled --dry-run (ui.processor.DryRun):
// engine.Processor.ProcessFile skips the real write whenever DryRun is set,
// and this render shares ui.processor with the real run, so without
// temporarily disabling it here, SetupRenderedBaseStorage would be pointed
// at an empty tempDir and a --dry-run --update-strategy=rendered preview
// would have nothing to diff its 3-way merge against.
func TestRenderPristineBase_WritesFilesEvenWhenProcessorDryRun(t *testing.T) {
	ui := createTestUI(t)
	ui.SetDryRun(true)

	tempDir, cleanup, err := ui.renderPristineBase(renderedBaseConfig(), map[string]interface{}{"project_name": "old-project"}, nil)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	static, err := os.ReadFile(filepath.Join(tempDir, "static.txt"))
	require.NoError(t, err)
	assert.Equal(t, "static content\n", string(static), "the pristine base render must write real files even under an outer --dry-run")

	assert.True(t, ui.processor.DryRun, "renderPristineBase must restore the outer run's own DryRun setting once its internal render finishes")
}

func TestSetupUpdateBase_TrackedWithEmptyBaseRef_IsNoOp(t *testing.T) {
	ui := createTestUI(t)
	// UpdateStrategyTracked is the zero value; no SetUpdateStrategy call needed.

	cleanup, err := ui.setupUpdateBase(t.TempDir(), "", nil)

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

	cleanup, err := ui.setupUpdateBase(targetDir, "", nil)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	err = ui.processor.ProcessFile(engine.File{Path: "static.txt", Content: "template content\n", Permissions: 0o644}, targetDir, false, true, nil, nil)
	require.NoError(t, err)

	merged, err := os.ReadFile(filepath.Join(targetDir, "static.txt"))
	require.NoError(t, err)
	assert.Equal(t, "template content\n", string(merged))
}

// TestSetupUpdateBase_Rendered_DryRun_UsesRealMergeBase proves the 3-way
// merge base wired up by setupUpdateBase's rendered branch is a real,
// materialized render even when the outer run is a --dry-run preview -- not
// an empty temp directory. Its test file "conflict.txt" is set up so base,
// ours, and theirs all genuinely diverge on the same line: with a real base,
// the resulting change ratio trips the merger's conflict/threshold check and
// ProcessFile returns an error. If renderPristineBase's internal render were
// silently skipped because it shares Processor.DryRun with the outer run
// (the bug this guards against), baseStorage.LoadBase would report "not
// found", determineBaseContent would treat the file as user-added, and the
// merge (and its error) would never happen at all -- ProcessFile would
// report success despite the genuine conflict.
func TestSetupUpdateBase_Rendered_DryRun_UsesRealMergeBase(t *testing.T) {
	ui := createTestUI(t)
	ui.SetDryRun(true)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)

	oldConfig := &templates.Configuration{
		Name: "conflict-base",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: renderedBaseScaffoldYAML, Permissions: 0o644},
			{Path: "conflict.txt", Content: "line1\nbase-line2\nline3\n", Permissions: 0o644},
		},
	}
	ui.SetRenderedBaseSource(oldConfig, map[string]interface{}{"project_name": "base-project"})

	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "conflict.txt"), []byte("line1\nours-line2\nline3\n"), 0o644))

	cleanup, err := ui.setupUpdateBase(targetDir, "", nil)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	err = ui.processor.ProcessFile(engine.File{Path: "conflict.txt", Content: "line1\ntheirs-line2\nline3\n", Permissions: 0o644}, targetDir, false, true, nil, nil)
	require.Error(t, err, "a real merge base must surface the genuine ours/theirs divergence even under --dry-run")
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)

	// --dry-run must never actually write conflict markers to disk.
	onDisk, readErr := os.ReadFile(filepath.Join(targetDir, "conflict.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "line1\nours-line2\nline3\n", string(onDisk))
}

// renderedBaseCustomDelimitersScaffoldYAML declares its own spec.delimiters
// ("<<"/">>") and a body template that only renders correctly under them.
// Since ResolveDelimiters (see ui.go) always prefers a scaffold's own
// declared spec.delimiters over any caller-supplied override, and
// config.LoadScaffoldConfigFromContent validates this file's own
// matrix/template syntax against that same declaration at load time, this
// scaffold's own delimiters -- not renderPristineBase's delimiters argument
// -- are what actually govern its render either way.
const renderedBaseCustomDelimitersScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: rendered-base-custom-delimiters
spec:
  delimiters: ["<<", ">>"]
  fields:
    - name: project_name
      type: input
      required: true
`

// TestRenderPristineBase_HonorsOldScaffoldsOwnDelimiters proves
// renderPristineBase accepts and forwards a delimiters argument through to
// renderPristineBaseFiles's ResolveDelimiters(delimiters, oldScaffoldConfig)
// call without breaking a scaffold that declares its own custom
// spec.delimiters -- exercising the exact plumbing CodeRabbit flagged as
// hardcoded to nil, even though (see the doc comment above) a scaffold's own
// declared delimiters win here regardless of what this call passes. As of
// this fix, renderPristineBase's own callers (ui.setupUpdateBase, called
// from ExecuteWithDelimiters) always pass "{{"/"}}"  today -- so in
// production this parameter is not yet observably different from the
// pre-fix hardcoded nil; the value of threading it through is API
// consistency with the sibling executeWithSetup/ResolveDelimiters call and
// correctness for any future caller that does pass a non-default value.
func TestRenderPristineBase_HonorsOldScaffoldsOwnDelimiters(t *testing.T) {
	ui := createTestUI(t)
	oldConfig := &templates.Configuration{
		Name: "rendered-base-custom-delimiters",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: renderedBaseCustomDelimitersScaffoldYAML, Permissions: 0o644},
			{Path: "README.md", Content: "# << .Config.project_name >>\n", IsTemplate: true, Permissions: 0o644},
		},
	}

	tempDir, cleanup, err := ui.renderPristineBase(oldConfig, map[string]interface{}{"project_name": "old-project"}, []string{"{{", "}}"})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	readme, err := os.ReadFile(filepath.Join(tempDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# old-project\n", string(readme), "the scaffold's own spec.delimiters must render its own template regardless of the delimiters argument passed in")
}

// TestSetupUpdateBase_Rendered_WithoutBaseSourceReturnsErrorNotPanic
// reproduces the field-test crash: a caller can flip updateStrategy to
// Rendered (e.g. via SetUpdateStrategy) without ever calling
// SetRenderedBaseSource first -- notably, the CLI's "confirm update instead"
// retry path used to do exactly this, since it flips update=true only after
// the initial non-update attempt already failed, bypassing the normal
// opts.update-gated ResolveRenderedBase/SetRenderedBaseSource wiring. Before
// the nil check in setupUpdateBase, this panicked with a nil pointer
// dereference inside loadOldScaffoldConfig; it must now return a normal
// error instead.
func TestSetupUpdateBase_Rendered_WithoutBaseSourceReturnsErrorNotPanic(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	// Deliberately never calling ui.SetRenderedBaseSource here.

	require.NotPanics(t, func() {
		_, err := ui.setupUpdateBase(t.TempDir(), "", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrRenderedBaseNotConfigured)
	})
}

func TestSetupUpdateBase_Rendered_PropagatesRenderFailure(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	ui.SetRenderedBaseSource(&templates.Configuration{
		Name:  "no-scaffold-yaml",
		Files: []templates.File{{Path: "README.md", Content: "static\n", Permissions: 0o644}},
	}, map[string]interface{}{})

	_, err := ui.setupUpdateBase(t.TempDir(), "", nil)

	require.Error(t, err)
}
