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
)

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
