package initcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/project/config"
)

const initRetryTestScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: retry-rendered
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// writeLocalInitRenderedRetryTemplate creates a minimal on-disk scaffold
// template (a local directory source, so source.ResolveRenderedBase's
// Hydrate call resolves it without needing git or network access) and
// returns its directory.
func writeLocalInitRenderedRetryTemplate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(initRetryTestScaffoldYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o600))
	return dir
}

// These tests exercise the retry-as-update confirmation flow in
// runInitTargetedFlow/runInitInteractiveFlow, and resolveInteractiveInitBaseRef's
// --update branch, using a mocked InitUI. That flow needs a real TTY and a
// pre-populated non-empty target directory to reach via integration tests, so
// it was previously only covered indirectly (or not at all for the "user
// declines"/error branches). Mocking InitUI lets each branch be asserted
// deterministically. Mirrors cmd/scaffold/scaffold_mock_test.go, which solves
// the same problem for the sibling command.

func TestRunInitTargetedFlow_OffersUpdateAndRetriesOnConfirm(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		targetDir:    "/tmp/target",
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, "/tmp/target", false, false, false, "", opts.templateVars).
			Return(errUtils.ErrTargetDirectoryNotEmpty),
		mockUI.EXPECT().
			ConfirmUpdateInstead("/tmp/target").
			Return(true, nil),
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, "/tmp/target", false, true, false, "HEAD", opts.templateVars).
			Return(nil),
	)

	targetDir, err := runInitTargetedFlow(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, "/tmp/target", targetDir)
}

// TestRunInitTargetedFlow_RenderedStrategyRetryWiresBaseSource mirrors
// cmd/scaffold's TestExecuteTemplateGeneration_RenderedStrategyRetryWiresBaseSource:
// under --update-strategy=rendered, the initial (non-update) attempt fails
// with ErrTargetDirectoryNotEmpty before executeInit's own opts.update-gated
// ResolveRenderedBase setup ever ran (that setup requires opts.update to
// already be true). Confirming the "update instead" offer used to retry with
// update=true directly, reaching setupUpdateBase's rendered branch with no
// base source ever configured -- a nil pointer panic. This asserts the retry
// now resolves and wires SetRenderedBaseSource before the retry
// ExecuteWithBaseRef call, using a real target dir with a real recorded
// project record and a real (local-directory) template source.
func TestRunInitTargetedFlow_RenderedStrategyRetryWiresBaseSource(t *testing.T) {
	templateDir := writeLocalInitRenderedRetryTemplate(t)
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-rendered"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: templateDir, RenderedRef: "irrelevant-for-local-source"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		targetDir:      targetDir,
		interactive:    true,
		updateStrategy: "rendered",
		templateVars:   map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, targetDir, false, false, false, "", opts.templateVars).
			Return(errUtils.ErrTargetDirectoryNotEmpty),
		mockUI.EXPECT().
			ConfirmUpdateInstead(targetDir).
			Return(true, nil),
		mockUI.EXPECT().
			SetRenderedBaseSource(gomock.Any(), gomock.Any()).
			Do(func(cfg *templates.Configuration, values map[string]interface{}) {
				require.NotNil(t, cfg)
				assert.NotEmpty(t, cfg.Files, "the old ref's template must be fully hydrated before the retry")
			}),
		// The retry base ref is "" under rendered mode: shouldOfferUpdate's
		// tracked-only defaultBaseRef resolution is skipped, since a non-empty
		// value here would otherwise flow unchanged into executeWithSetup's
		// spec.baseRef write regardless of strategy.
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, targetDir, false, true, false, "", opts.templateVars).
			Return(nil),
	)

	resultDir, err := runInitTargetedFlow(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, targetDir, resultDir)
}

// TestRunInitTargetedFlow_TrackedStrategyRetryRejectsSwitchFromRendered
// covers a target last generated under --update-strategy=rendered
// (spec.renderedRef set, spec.baseRef empty) whose initial (non-update)
// attempt fails with ErrTargetDirectoryNotEmpty, offering the same
// "confirm update instead" retry as
// TestRunInitTargetedFlow_RenderedStrategyRetryWiresBaseSource -- but this
// time the retry itself defaults to --update-strategy=tracked. Before this
// fix, prepareRenderedRetryBase returned immediately for a non-rendered
// strategy without ever calling source.CheckNotSwitchedFromRendered, so the
// retry's ExecuteWithBaseRef call would have gone on to attempt a tracked
// 3-way merge against a target that was deliberately generated with no
// git-history dependency. It must instead fail loudly here, before that
// retry ExecuteWithBaseRef call ever happens.
func TestRunInitTargetedFlow_TrackedStrategyRetryRejectsSwitchFromRendered(t *testing.T) {
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-tracked"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: "embedded", RenderedRef: "abc123"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		targetDir:    targetDir,
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	// The retry's own ExecuteWithBaseRef call must never happen: the
	// strategy-switch check must reject the retry first.
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, targetDir, false, false, false, "", opts.templateVars).
		Return(errUtils.ErrTargetDirectoryNotEmpty).
		Times(1)
	mockUI.EXPECT().
		ConfirmUpdateInstead(targetDir).
		Return(true, nil)

	_, err := runInitTargetedFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUpdateStrategySwitchedToTracked)
}

func TestRunInitTargetedFlow_DeclinesUpdateOffer(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		targetDir:    "/tmp/target",
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	// ExecuteWithBaseRef must be called exactly once: declining the offer
	// must not trigger a retry.
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, "/tmp/target", false, false, false, "", opts.templateVars).
		Return(errUtils.ErrTargetDirectoryNotEmpty).
		Times(1)
	mockUI.EXPECT().
		ConfirmUpdateInstead("/tmp/target").
		Return(false, nil)

	targetDir, err := runInitTargetedFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
	assert.Equal(t, "/tmp/target", targetDir)
}

func TestRunInitInteractiveFlow_OffersUpdateAndRetriesOnConfirm(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", false, false, false, "", opts.templateVars).
			Return("/tmp/picked", errUtils.ErrTargetDirectoryNotEmpty),
		mockUI.EXPECT().
			ConfirmUpdateInstead("/tmp/picked").
			Return(true, nil),
		mockUI.EXPECT().
			ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "/tmp/picked", false, true, false, "HEAD", opts.templateVars).
			Return("/tmp/picked", nil),
	)

	targetDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, "/tmp/picked", targetDir)
}

func TestRunInitInteractiveFlow_DeclinesUpdateOffer(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", false, false, false, "", opts.templateVars).
		Return("/tmp/picked", errUtils.ErrTargetDirectoryNotEmpty).
		Times(1)
	mockUI.EXPECT().
		ConfirmUpdateInstead("/tmp/picked").
		Return(false, nil)

	targetDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
	assert.Equal(t, "/tmp/picked", targetDir)
}

// TestRunInitInteractiveFlow_ResolveInteractiveInitBaseRefError verifies that
// when resolveInteractiveInitBaseRef fails (--update with an unreadable
// metadata pin), runInitInteractiveFlow returns the error without ever
// calling ExecuteWithInteractiveFlowAndBaseRefResult.
func TestRunInitInteractiveFlow_ResolveInteractiveInitBaseRefError(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.InitMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &initOptions{
		interactive:  true,
		update:       true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(dir, opts.templateVars, false, nil)
	// No ExecuteWithInteractiveFlowAndBaseRefResult expectation: gomock fails
	// the test if it's called.

	targetDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.Equal(t, dir, targetDir)
}

// TestRunInitTargetedFlow_ShouldOfferUpdateErrorPropagates verifies that when
// shouldOfferUpdate itself fails (a corrupt metadata pin at the target
// directory, surfaced while resolving the retry base ref), that error is
// returned directly instead of ConfirmUpdateInstead ever being called.
func TestRunInitTargetedFlow_ShouldOfferUpdateErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.InitMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &initOptions{
		targetDir:    dir,
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, dir, false, false, false, "", opts.templateVars).
		Return(errUtils.ErrTargetDirectoryNotEmpty)
	// No ConfirmUpdateInstead expectation: gomock fails the test if it's called.

	targetDir, err := runInitTargetedFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
	assert.Equal(t, dir, targetDir)
}

// TestRunInitInteractiveFlow_ShouldOfferUpdateErrorPropagates is the
// interactive-flow counterpart of
// TestRunInitTargetedFlow_ShouldOfferUpdateErrorPropagates.
func TestRunInitInteractiveFlow_ShouldOfferUpdateErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.InitMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &initOptions{
		interactive:  true,
		templateVars: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", false, false, false, "", opts.templateVars).
		Return(dir, errUtils.ErrTargetDirectoryNotEmpty)
	// No ConfirmUpdateInstead expectation: gomock fails the test if it's called.

	targetDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
	assert.Equal(t, dir, targetDir)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_ResolvesTargetAndBaseRef
// reproduces the bug reported against `atmos init --update` with no
// positional target: the base ref used to default to "HEAD" because it was
// resolved (via defaultBaseRef) against the empty target passed to the RunE
// handler *before* the interactive flow prompted for and picked the real
// directory, so any pin at that real directory
// (.atmos/init/metadata.yaml, written by gen.PinInitialBaseRefForInit) was
// silently ignored. This asserts the base ref returned is resolved against
// the actual directory ResolveTargetPath returns, and picks up its pin.
func TestResolveInteractiveInitBaseRef_UpdateTrue_ResolvesTargetAndBaseRef(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadata := storage.NewInitMetadata("test", "1.0.0", "embedded", "pinned-after-prompt", nil)
	require.NoError(t, storage.NewMetadataStorage(storage.InitMetadataPath(dir)).Save(metadata))

	opts := &initOptions{
		update:       true,
		interactive:  true,
		templateVars: map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	// ResolveTargetPath stands in for the interactive prompt picking `dir`.
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(dir, opts.templateVars, true, nil)

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, dir, resolved.targetDir)
	// The regression: baseRef must be the pin resolved against `dir` (the
	// real, resolved target), not "HEAD" -- which is what a premature
	// defaultBaseRef("", "") call against the empty positional target would
	// have produced.
	assert.Equal(t, "pinned-after-prompt", resolved.baseRef)
	assert.True(t, resolved.useDefaults)
	assert.Equal(t, opts.templateVars, resolved.templateValues)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_ResolveTargetPathError verifies
// that a ResolveTargetPath failure is propagated without calling
// defaultBaseRef.
func TestResolveInteractiveInitBaseRef_UpdateTrue_ResolveTargetPathError(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		update:      true,
		interactive: true,
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return("/tmp/picked", opts.templateVars, false, errUtils.ErrInitialization)

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInitialization)
	assert.Equal(t, "/tmp/picked", resolved.targetDir)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_DefaultBaseRefError verifies a
// corrupt/unreadable metadata file at the resolved target surfaces as an
// error rather than silently resolving to "HEAD".
func TestResolveInteractiveInitBaseRef_UpdateTrue_DefaultBaseRefError(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.InitMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &initOptions{
		update:      true,
		interactive: true,
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(dir, opts.templateVars, false, nil)

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.Equal(t, dir, resolved.targetDir)
	assert.Empty(t, resolved.baseRef)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_InvalidUpdateStrategyPropagatesError
// covers resolveInteractiveInitBaseRef's own engine.ParseUpdateStrategy error
// branch: a bogus --update-strategy value must surface directly, without
// ever reaching source.ResolveRenderedBase or source.CheckNotSwitchedFromRendered.
func TestResolveInteractiveInitBaseRef_UpdateTrue_InvalidUpdateStrategyPropagatesError(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	opts := &initOptions{
		update:         true,
		interactive:    true,
		updateStrategy: "bogus",
		templateVars:   map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(dir, opts.templateVars, false, nil)

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.Equal(t, dir, resolved.targetDir)
	assert.Empty(t, resolved.baseRef)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_RenderedStrategyResolveFailurePropagatesError
// covers resolveInteractiveInitBaseRef's own source.ResolveRenderedBase error
// branch for a *valid* --update-strategy=rendered (unlike
// TestResolveInteractiveInitBaseRef_UpdateTrue_InvalidUpdateStrategyPropagatesError
// above, which covers the parse failure): the resolved target has no
// recorded project state at all, so resolution itself fails and must
// propagate directly, without ever reaching SetRenderedBaseSource (which
// would panic against a nil mock expectation).
func TestResolveInteractiveInitBaseRef_UpdateTrue_RenderedStrategyResolveFailurePropagatesError(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	opts := &initOptions{
		update:         true,
		interactive:    true,
		updateStrategy: "rendered",
		templateVars:   map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(dir, opts.templateVars, false, nil)
	// No SetRenderedBaseSource expectation: gomock fails the test if it's
	// reached despite the resolution failure.

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.Equal(t, dir, resolved.targetDir)
	assert.Empty(t, resolved.baseRef)
	assert.Nil(t, resolved.cleanup)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_RenderedStrategyWiresBaseSource
// covers resolveInteractiveInitBaseRef's --update-strategy=rendered branch:
// once ResolveTargetPath (standing in for the interactive prompt) picks the
// real target directory, this must resolve the rendered old-ref base against
// it, call SetRenderedBaseSource, and return a non-nil cleanup -- while
// skipping the tracked-only CheckNotSwitchedFromRendered/defaultBaseRef calls
// entirely (no baseRef is produced for rendered).
func TestResolveInteractiveInitBaseRef_UpdateTrue_RenderedStrategyWiresBaseSource(t *testing.T) {
	templateDir := writeLocalInitRenderedRetryTemplate(t)
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-rendered"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: templateDir, RenderedRef: "irrelevant-for-local-source"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		update:         true,
		interactive:    true,
		updateStrategy: "rendered",
		templateVars:   map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(targetDir, opts.templateVars, false, nil)
	mockUI.EXPECT().
		SetRenderedBaseSource(gomock.Any(), gomock.Any()).
		Do(func(cfg *templates.Configuration, values map[string]interface{}) {
			require.NotNil(t, cfg)
			assert.NotEmpty(t, cfg.Files, "the old ref's template must be fully hydrated")
		})

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, targetDir, resolved.targetDir)
	assert.Empty(t, resolved.baseRef, "rendered mode never produces a tracked-style base ref")
	assert.NotNil(t, resolved.cleanup)
}

// TestResolveInteractiveInitBaseRef_UpdateTrue_TrackedStrategyRejectsSwitchFromRendered
// covers resolveInteractiveInitBaseRef's tracked-strategy
// CheckNotSwitchedFromRendered error branch: a target last generated under
// --update-strategy=rendered (spec.renderedRef set, spec.baseRef empty) must
// reject a plain (tracked-defaulting) --update instead of silently resolving
// a base ref against git history the target was never meant to have.
func TestResolveInteractiveInitBaseRef_UpdateTrue_TrackedStrategyRejectsSwitchFromRendered(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-tracked"}}
	require.NoError(t, config.SaveProjectRecord(dir, sampleConfig,
		config.ProjectRecordProvenance{Source: "embedded", RenderedRef: "abc123"}, nil))

	opts := &initOptions{
		update:       true,
		interactive:  true,
		templateVars: map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(dir, opts.templateVars, false, nil)

	resolved, err := resolveInteractiveInitBaseRef(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUpdateStrategySwitchedToTracked)
	assert.Equal(t, dir, resolved.targetDir)
}

// TestRunInitInteractiveFlow_RenderedStrategy_DefersResolvedCleanup covers
// runInitInteractiveFlow's own "if resolved.cleanup != nil { defer
// resolved.cleanup() }" branch: with --update-strategy=rendered,
// resolveInteractiveInitBaseRef returns a non-nil cleanup (see
// TestResolveInteractiveInitBaseRef_UpdateTrue_RenderedStrategyWiresBaseSource
// above), which runInitInteractiveFlow itself must defer -- exercised here
// through the real entry point rather than calling
// resolveInteractiveInitBaseRef directly.
func TestRunInitInteractiveFlow_RenderedStrategy_DefersResolvedCleanup(t *testing.T) {
	templateDir := writeLocalInitRenderedRetryTemplate(t)
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-rendered"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: templateDir, RenderedRef: "irrelevant-for-local-source"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		update:         true,
		interactive:    true,
		updateStrategy: "rendered",
		templateVars:   map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, false, opts.templateVars).
		Return(targetDir, opts.templateVars, false, nil)
	mockUI.EXPECT().SetRenderedBaseSource(gomock.Any(), gomock.Any())
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, targetDir, false, true, false, "", opts.templateVars).
		Return(targetDir, nil)

	resultDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, targetDir, resultDir)
}

// TestRunInitInteractiveFlow_RetryConfirmed_RenderedStrategyQueuesCleanup
// covers runInitInteractiveFlow's "if renderedCleanup != nil { defer
// renderedCleanup() }" branch inside the confirmed-retry path: the initial
// (non-update) attempt fails with ErrTargetDirectoryNotEmpty, the user
// confirms the "update instead" offer, and --update-strategy=rendered makes
// prepareRenderedRetryBase resolve and return a non-nil cleanup that this
// function must defer before the retry call.
func TestRunInitInteractiveFlow_RetryConfirmed_RenderedStrategyQueuesCleanup(t *testing.T) {
	templateDir := writeLocalInitRenderedRetryTemplate(t)
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-rendered"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: templateDir, RenderedRef: "irrelevant-for-local-source"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &initOptions{
		interactive:    true,
		updateStrategy: "rendered",
		templateVars:   map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", false, false, false, "", opts.templateVars).
			Return(targetDir, errUtils.ErrTargetDirectoryNotEmpty),
		mockUI.EXPECT().
			ConfirmUpdateInstead(targetDir).
			Return(true, nil),
		mockUI.EXPECT().
			SetRenderedBaseSource(gomock.Any(), gomock.Any()).
			Do(func(cfg *templates.Configuration, values map[string]interface{}) {
				require.NotNil(t, cfg)
				assert.NotEmpty(t, cfg.Files, "the old ref's template must be fully hydrated before the retry")
			}),
		mockUI.EXPECT().
			ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, targetDir, false, true, false, "", opts.templateVars).
			Return(targetDir, nil),
	)

	resultDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.NoError(t, err)
	assert.Equal(t, targetDir, resultDir)
}

// TestRunInitInteractiveFlow_RetryConfirmed_PrepareRenderedRetryBaseErrorPropagates
// covers runInitInteractiveFlow's "if prepErr != nil { return finalTargetDir,
// prepErr }" branch: --update-strategy=rendered is a valid strategy (so
// shouldOfferUpdate itself offers the retry unconditionally, without needing
// any project record -- see shouldOfferUpdate's rendered short-circuit), but
// prepareRenderedRetryBase's source.ResolveRenderedBase call fails because
// targetDir has no recorded project state to resolve the old ref/answers
// from. That failure must return directly, without ever attempting the
// retry's second ExecuteWithInteractiveFlowAndBaseRefResult call.
func TestRunInitInteractiveFlow_RetryConfirmed_PrepareRenderedRetryBaseErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	targetDir := t.TempDir()
	opts := &initOptions{
		interactive:    true,
		updateStrategy: "rendered",
		templateVars:   map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", false, false, false, "", opts.templateVars).
			Return(targetDir, errUtils.ErrTargetDirectoryNotEmpty),
		mockUI.EXPECT().
			ConfirmUpdateInstead(targetDir).
			Return(true, nil),
	)
	// No second ExecuteWithInteractiveFlowAndBaseRefResult expectation: gomock
	// fails the test if the retry is attempted despite prepErr.

	resultDir, err := runInitInteractiveFlow(mockUI, selectedConfig, opts)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
	assert.Equal(t, targetDir, resultDir)
}

// TestConfigureInitMergeSettings_InvalidMergeStrategyPropagatesError covers
// configureInitMergeSettings's own merge.ResolveConflictStrategy error
// branch. RunE's WithValidValues registration for --merge-strategy already
// rejects a bogus value before executeInit is ever reached (see
// TestInitCmd_RunE_MergeStrategyInvalidValueRejected), so this exercises
// configureInitMergeSettings directly to prove it still fails safely --
// returning the error and a nil cleanup, and never reaching
// SetConflictStrategy/SetMergeDriver/SetSkipHooks/SetUpdateStrategy -- for
// any other caller that skips that upfront validation.
func TestConfigureInitMergeSettings_InvalidMergeStrategyPropagatesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	mockUI.EXPECT().SetMaxChanges(42)
	// No SetConflictStrategy/SetMergeDriver/SetSkipHooks/SetUpdateStrategy
	// expectations: gomock fails the test if any of them are called after the
	// error.

	opts := &initOptions{maxChanges: 42, mergeStrategy: "bogus"}

	cleanup, err := configureInitMergeSettings(mockUI, opts)

	require.Error(t, err)
	assert.Nil(t, cleanup)
}

// TestConfigureInitMergeSettings_InvalidUpdateStrategyPropagatesError covers
// configureInitMergeSettings's engine.ParseUpdateStrategy error branch,
// mirroring TestConfigureInitMergeSettings_InvalidMergeStrategyPropagatesError
// above for the strategy parsed last (after SetConflictStrategy/
// SetMergeDriver/SetSkipHooks have already run).
func TestConfigureInitMergeSettings_InvalidUpdateStrategyPropagatesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUI := NewMockInitUI(ctrl)
	gomock.InOrder(
		mockUI.EXPECT().SetMaxChanges(0),
		mockUI.EXPECT().SetConflictStrategy(gomock.Any()),
		mockUI.EXPECT().SetMergeDriver(gomock.Any()),
		mockUI.EXPECT().SetSkipHooks(gomock.Any()),
	)
	// No SetUpdateStrategy expectation: gomock fails the test if it's called.

	opts := &initOptions{updateStrategy: "bogus"}

	cleanup, err := configureInitMergeSettings(mockUI, opts)

	require.Error(t, err)
	assert.Nil(t, cleanup)
}
