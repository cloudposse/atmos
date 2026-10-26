package scaffold

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

const retryTestScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: retry-rendered
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// writeLocalRenderedRetryTemplate creates a minimal on-disk scaffold template
// (a local directory source, so source.ResolveRenderedBase's Hydrate call
// resolves it without needing git or network access) and returns its
// directory.
func writeLocalRenderedRetryTemplate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(retryTestScaffoldYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o600))
	return dir
}

// These tests exercise the retry-as-update confirmation flow in
// executeTemplateGeneration using a mocked ScaffoldUI. That flow needs a real
// TTY and a pre-populated non-empty target directory to reach via
// integration tests, so it was previously only covered indirectly (or not at
// all for the "user declines" branch). Mocking ScaffoldUI lets both branches
// be asserted deterministically.

func TestExecuteTemplateGeneration_OffersUpdateAndRetriesOnConfirm(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, "/tmp/target", false, false, false, "", opts.templateValues).
			Return(errUtils.ErrTargetDirectoryNotEmpty),
		mockUI.EXPECT().
			ConfirmUpdateInstead("/tmp/target").
			Return(true, nil),
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, "/tmp/target", false, true, false, "HEAD", opts.templateValues).
			Return(nil),
	)

	err := executeTemplateGeneration(selectedConfig, "/tmp/target", opts, mockUI)
	require.NoError(t, err)
}

// TestExecuteTemplateGeneration_RenderedStrategyRetryWiresBaseSource
// reproduces the field-test crash: under --update-strategy=rendered, the
// initial (non-update) attempt fails with ErrTargetDirectoryNotEmpty before
// executeScaffoldGenerate's own opts.update-gated ResolveRenderedBase setup
// ever ran (that setup requires opts.update to already be true). Confirming
// the "update instead" offer used to retry with update=true directly,
// reaching setupUpdateBase's rendered branch with no base source ever
// configured -- a nil pointer panic. This asserts the retry now resolves and
// wires SetRenderedBaseSource before the retry ExecuteWithBaseRef call, using
// a real target dir with a real recorded project record and a real
// (local-directory) template source so the resolution actually exercises
// source.ResolveRenderedBase end to end, not just a mocked pass-through.
func TestExecuteTemplateGeneration_RenderedStrategyRetryWiresBaseSource(t *testing.T) {
	templateDir := writeLocalRenderedRetryTemplate(t)
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-rendered"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: templateDir, RenderedRef: "irrelevant-for-local-source"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		updateStrategy: "rendered",
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())

	gomock.InOrder(
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, targetDir, false, false, false, "", opts.templateValues).
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
		// The retry base ref is "" under rendered mode: shouldOfferScaffoldUpdate's
		// tracked-only defaultBaseRef resolution is skipped, since a non-empty
		// value here would otherwise flow unchanged into executeWithSetup's
		// spec.baseRef write regardless of strategy.
		mockUI.EXPECT().
			ExecuteWithBaseRef(selectedConfig, targetDir, false, true, false, "", opts.templateValues).
			Return(nil),
	)

	err := executeTemplateGeneration(selectedConfig, targetDir, opts, mockUI)
	require.NoError(t, err)
}

// TestExecuteTemplateGeneration_TrackedStrategyRetryRejectsSwitchFromRendered
// covers a target last generated under --update-strategy=rendered
// (spec.renderedRef set, spec.baseRef empty) whose initial (non-update)
// attempt fails with ErrTargetDirectoryNotEmpty, offering the same "confirm
// update instead" retry as
// TestExecuteTemplateGeneration_RenderedStrategyRetryWiresBaseSource -- but
// this time the retry itself defaults to --update-strategy=tracked. Before
// this fix, prepareRenderedRetryBase returned immediately for a non-rendered
// strategy without ever calling source.CheckNotSwitchedFromRendered, so the
// retry's ExecuteWithBaseRef call would have gone on to attempt a tracked
// 3-way merge against a target that was deliberately generated with no
// git-history dependency. It must instead fail loudly here, before that
// retry ExecuteWithBaseRef call ever happens.
func TestExecuteTemplateGeneration_TrackedStrategyRetryRejectsSwitchFromRendered(t *testing.T) {
	targetDir := t.TempDir()
	sampleConfig := &config.ScaffoldConfig{Metadata: manifest.Metadata{Name: "retry-tracked"}}
	require.NoError(t, config.SaveProjectRecord(targetDir, sampleConfig,
		config.ProjectRecordProvenance{Source: "embedded", RenderedRef: "abc123"}, nil))

	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())

	// The retry's own ExecuteWithBaseRef call must never happen: the
	// strategy-switch check must reject the retry first.
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, targetDir, false, false, false, "", opts.templateValues).
		Return(errUtils.ErrTargetDirectoryNotEmpty).
		Times(1)
	mockUI.EXPECT().
		ConfirmUpdateInstead(targetDir).
		Return(true, nil)

	err := executeTemplateGeneration(selectedConfig, targetDir, opts, mockUI)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUpdateStrategySwitchedToTracked)
}

func TestExecuteTemplateGeneration_DeclinesUpdateOffer(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())

	// ExecuteWithBaseRef must be called exactly once: declining the offer
	// must not trigger a retry.
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, "/tmp/target", false, false, false, "", opts.templateValues).
		Return(errUtils.ErrTargetDirectoryNotEmpty).
		Times(1)
	mockUI.EXPECT().
		ConfirmUpdateInstead("/tmp/target").
		Return(false, nil)

	err := executeTemplateGeneration(selectedConfig, "/tmp/target", opts, mockUI)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
}

// TestExecuteTemplateWithoutTargetDir_UpdateResolvesBaseRefAfterInteractiveTarget
// reproduces the bug reported against `atmos scaffold generate --update` with
// no positional target: the base ref used to default to "HEAD" because it
// was resolved (via defaultBaseRef) against the empty target passed to the
// RunE handler *before* the interactive flow prompted for and picked the
// real directory, so any pin at that real directory
// (.atmos/scaffold/metadata.yaml, written by gen.PinInitialBaseRef) was
// silently ignored. This asserts the base ref passed to
// ExecuteWithInteractiveFlowAndBaseRefResult is resolved against the actual
// directory ResolveTargetPath returns, and picks up its pin.
func TestExecuteTemplateWithoutTargetDir_UpdateResolvesBaseRefAfterInteractiveTarget(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadata := storage.NewScaffoldMetadata("test", "1.0.0", "embedded", "pinned-after-prompt", nil)
	require.NoError(t, storage.NewMetadataStorage(storage.ScaffoldMetadataPath(dir)).Save(metadata))

	opts := &scaffoldGenerateOptions{
		interactive:    true,
		update:         true,
		useDefaults:    true,
		templateValues: map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	// ResolveTargetPath stands in for the interactive prompt picking `dir`.
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, true, opts.templateValues).
		Return(dir, opts.templateValues, true, nil)
	// The regression: baseRef must be the pin resolved against `dir` (the
	// real, resolved target), not "HEAD" -- which is what a premature
	// defaultBaseRef("", "") call against the empty positional target would
	// have produced.
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, dir, false, true, true, "pinned-after-prompt", opts.templateValues).
		Return(dir, nil)

	targetDir, err := executeTemplateWithoutTargetDir(selectedConfig, opts, mockUI)

	require.NoError(t, err)
	assert.Equal(t, dir, targetDir)
}

// TestExecuteTemplateWithoutTargetDir_NoUpdateStillResolvesTargetEarly
// verifies that even without --update, executeTemplateWithoutTargetDir now
// pre-resolves the target directory via ResolveTargetPath (needed so a
// local-source template's own previously generated output can be re-excluded
// from selectedConfig.Files -- see reloadLocalTemplateFiles -- before
// ExecuteWithInteractiveFlowAndBaseRefResult runs). ResolveTargetPath is a
// no-op passthrough once it returns a directory, so the interactive flow's
// own prompt does not run a second time: the resolved directory is passed
// straight through as ExecuteWithInteractiveFlowAndBaseRefResult's
// targetPath, and the base ref stays empty since it's unused without
// --update.
func TestExecuteTemplateWithoutTargetDir_NoUpdateStillResolvesTargetEarly(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()

	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", false, false, opts.templateValues).
		Return(dir, opts.templateValues, false, nil)
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, dir, false, false, false, "", opts.templateValues).
		Return(dir, nil)

	targetDir, err := executeTemplateWithoutTargetDir(selectedConfig, opts, mockUI)

	require.NoError(t, err)
	assert.Equal(t, dir, targetDir)
}

// TestExecuteTemplateGeneration_OfferErrorPropagates reproduces a corrupt
// .atmos/scaffold/metadata.yaml at targetDir: shouldOfferScaffoldUpdate's own
// defaultBaseRef lookup fails while deciding whether to offer a retry, and
// executeTemplateGeneration must propagate that resolution error directly
// instead of silently treating it as "don't offer" and returning the
// original ErrTargetDirectoryNotEmpty.
func TestExecuteTemplateGeneration_OfferErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, dir, false, false, false, "", opts.templateValues).
		Return(errUtils.ErrTargetDirectoryNotEmpty)
	// ConfirmUpdateInstead must never be reached: the offer decision itself
	// fails first.
	mockUI.EXPECT().ConfirmUpdateInstead(gomock.Any()).Times(0)

	err := executeTemplateGeneration(selectedConfig, dir, opts, mockUI)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty, "the base-ref resolution error must win, not the original merge-offer trigger")
}

func TestExecuteTemplateGeneration_NoOfferWhenForceSet(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		force:          true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())

	// With --force already set, a failure must propagate directly -- no
	// ConfirmUpdateInstead call at all.
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, "/tmp/target", true, false, false, "", opts.templateValues).
		Return(errUtils.ErrTargetDirectoryNotEmpty)

	err := executeTemplateGeneration(selectedConfig, "/tmp/target", opts, mockUI)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
}

// TestResolveInteractiveBaseRef_ResolveTargetPathErrorPropagates reproduces
// ResolveTargetPath itself failing (e.g. the interactive setup form
// erroring) for the --update no-positional-target flow: resolveInteractiveBaseRef
// must return that error directly rather than going on to call
// defaultBaseRef against a bogus/empty targetDir.
func TestResolveInteractiveBaseRef_ResolveTargetPathErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		update:         true,
		useDefaults:    true,
		templateValues: map[string]interface{}{"key": "value"},
	}
	wantErr := errUtils.ErrInitialization

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, true, opts.templateValues).
		Return("", nil, false, wantErr)

	_, baseRef, templateValues, useDefaults, _, err := resolveInteractiveBaseRef(selectedConfig, opts, mockUI)

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Empty(t, baseRef)
	assert.Nil(t, templateValues)
	assert.False(t, useDefaults)
}

// TestResolveInteractiveBaseRef_DefaultBaseRefErrorPropagates reproduces a
// corrupt metadata.yaml at the directory ResolveTargetPath resolved: once the
// real target directory is known, resolveInteractiveBaseRef's own
// defaultBaseRef lookup must surface a Load failure instead of silently
// falling back to "HEAD".
func TestResolveInteractiveBaseRef_DefaultBaseRefErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &scaffoldGenerateOptions{
		update:         true,
		useDefaults:    true,
		templateValues: map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, true, opts.templateValues).
		Return(dir, opts.templateValues, true, nil)

	targetDir, baseRef, templateValues, useDefaults, _, err := resolveInteractiveBaseRef(selectedConfig, opts, mockUI)

	require.Error(t, err)
	assert.Equal(t, dir, targetDir, "the resolved target dir must still be returned so the caller can report it")
	assert.Empty(t, baseRef)
	assert.Nil(t, templateValues)
	assert.False(t, useDefaults)
}

// TestExecuteTemplateWithoutTargetDir_ResolveInteractiveBaseRefErrorPropagates
// covers executeTemplateWithoutTargetDir's own propagation of a
// resolveInteractiveBaseRef failure: ExecuteWithInteractiveFlowAndBaseRefResult
// must never be called once resolving the base ref itself has failed.
func TestExecuteTemplateWithoutTargetDir_ResolveInteractiveBaseRefErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		update:         true,
		useDefaults:    true,
		templateValues: map[string]interface{}{"key": "value"},
	}
	wantErr := errUtils.ErrInitialization

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", true, true, opts.templateValues).
		Return("", nil, false, wantErr)
	// No ExecuteWithInteractiveFlowAndBaseRefResult expectation: gomock fails
	// the test if it's called after base-ref resolution already failed.

	_, err := executeTemplateWithoutTargetDir(selectedConfig, opts, mockUI)

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestExecuteTemplateWithoutTargetDir_OfferErrorPropagates reproduces a
// corrupt metadata.yaml at the directory the interactive flow resolved:
// shouldOfferScaffoldUpdate's defaultBaseRef lookup fails while deciding
// whether to offer a retry, and executeTemplateWithoutTargetDir must
// propagate that resolution error directly instead of returning the original
// ErrTargetDirectoryNotEmpty.
func TestExecuteTemplateWithoutTargetDir_OfferErrorPropagates(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	// opts.update is false, so resolveInteractiveBaseRef's defaultBaseRef
	// lookup is skipped, but it still resolves the real target directory via
	// ResolveTargetPath before generation runs.
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", false, false, opts.templateValues).
		Return(dir, opts.templateValues, false, nil)
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, dir, false, false, false, "", opts.templateValues).
		Return(dir, errUtils.ErrTargetDirectoryNotEmpty)
	mockUI.EXPECT().ConfirmUpdateInstead(gomock.Any()).Times(0)

	finalTargetDir, err := executeTemplateWithoutTargetDir(selectedConfig, opts, mockUI)

	require.Error(t, err)
	assert.Equal(t, dir, finalTargetDir)
	assert.NotErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty, "the base-ref resolution error must win, not the original merge-offer trigger")
}

// TestExecuteTemplateWithoutTargetDir_ReExcludesLocalSourceOutputAfterInteractiveTarget
// covers the gap CodeRabbit flagged in the fix for a local-source scaffold
// template (`source: "."`) re-ingesting its own previously generated output
// as template content: that fix only threaded the resolved target directory
// through when the user supplies <target> on the CLI. With an omitted
// target, the initial loadScaffoldTemplates call in executeScaffoldGenerate
// runs before the target exists (absTargetDir is "" for the no-positional-
// target flow), so selectedConfig.Files was loaded with no exclusion at all.
// Only once the interactive flow resolves a real target -- here, a nested
// local target that lands inside the template's own prior output -- can that
// output be excluded. This exercises both halves of CodeRabbit's own
// suggestion: an omitted target and a nested local target.
func TestExecuteTemplateWithoutTargetDir_ReExcludesLocalSourceOutputAfterInteractiveTarget(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "template.txt"), []byte("hello"), 0o600))

	// A prior run's output, sitting inside the template's own source tree --
	// exactly the scenario that compounded into 47,000+ self-nested
	// directories before the fix (see templates.WithExcludePath's doc).
	priorOutputDir := filepath.Join(sourceDir, "generated", "myapp")
	require.NoError(t, os.MkdirAll(priorOutputDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(priorOutputDir, "output.txt"), []byte("stale"), 0o600))

	// Load exactly as the initial, un-excluded loadScaffoldTemplates call
	// would (absTargetDir == "" since no positional target was given).
	selectedConfig, err := templates.LoadConfigurationFromDir("test", sourceDir)
	require.NoError(t, err)

	// File.Path always uses forward slashes (loadConfiguration builds it with
	// path.Join, never filepath.Join -- see pkg/generator/templates/embeds.go)
	// regardless of host OS, so assertions below compare against a literal
	// forward-slash path rather than one built with filepath.Join.
	const stalePath = "generated/myapp/output.txt"

	// Sanity check the fixture: without the fix, the stale output is loaded
	// as template content.
	require.True(t, containsFilePath(selectedConfig.Files, stalePath),
		"fixture must reproduce the stale-output-loaded bug before asserting the fix removes it")

	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	// The interactive prompt (stood in for by ResolveTargetPath) picks a
	// target nested inside the template's own `generated/` output.
	mockUI.EXPECT().
		ResolveTargetPath(selectedConfig, "", false, false, opts.templateValues).
		Return(priorOutputDir, opts.templateValues, false, nil)
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, priorOutputDir, false, false, false, "", opts.templateValues).
		DoAndReturn(func(cfg *templates.Configuration, targetPath string, _, _, _ bool, _ string, _ map[string]interface{}) (string, error) {
			// By the time generation runs, selectedConfig.Files must already
			// have been reloaded with the target's containing directory
			// excluded.
			assert.False(t, containsFilePath(cfg.Files, stalePath),
				"stale prior-run output must be excluded from Files once the real target is known")
			assert.True(t, containsFilePath(cfg.Files, "template.txt"), "unrelated template content must survive the reload")
			return targetPath, nil
		})

	targetDir, err := executeTemplateWithoutTargetDir(selectedConfig, opts, mockUI)

	require.NoError(t, err)
	assert.Equal(t, priorOutputDir, targetDir)
	// Also assert directly against the mutated selectedConfig, not just what
	// the mock observed mid-call.
	assert.False(t, containsFilePath(selectedConfig.Files, stalePath))
	assert.True(t, containsFilePath(selectedConfig.Files, "template.txt"))
}

// containsFilePath reports whether files contains an entry with the given path.
func containsFilePath(files []templates.File, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}
