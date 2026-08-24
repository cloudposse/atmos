package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

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

// TestExecuteTemplateWithoutTargetDir_NoUpdateSkipsTargetResolution verifies
// that without --update, executeTemplateWithoutTargetDir does not pre-resolve
// the target directory (which would require an extra prompt round-trip):
// the base ref is unused for fresh generation, so ResolveTargetPath must not
// be called, and the interactive flow's own prompt (inside
// ExecuteWithInteractiveFlowAndBaseRefResult) is the only one that runs.
func TestExecuteTemplateWithoutTargetDir_NoUpdateSkipsTargetResolution(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	dir := t.TempDir()

	opts := &scaffoldGenerateOptions{
		interactive:    true,
		templateValues: map[string]interface{}{"key": "value"},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().SetSkipHooks(gomock.Any())
	// No ResolveTargetPath expectation: gomock fails the test if it's called.
	mockUI.EXPECT().
		ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", false, false, false, "", opts.templateValues).
		Return(dir, nil)

	targetDir, err := executeTemplateWithoutTargetDir(selectedConfig, opts, mockUI)

	require.NoError(t, err)
	assert.Equal(t, dir, targetDir)
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
