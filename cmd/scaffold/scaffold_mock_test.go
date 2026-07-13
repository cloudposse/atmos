package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestExecuteTemplateGeneration_NoOfferWhenForceSet(t *testing.T) {
	selectedConfig := &templates.Configuration{Name: "test"}
	opts := &scaffoldGenerateOptions{
		interactive:    true,
		force:          true,
		templateValues: map[string]interface{}{},
	}

	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)

	// With --force already set, a failure must propagate directly -- no
	// ConfirmUpdateInstead call at all.
	mockUI.EXPECT().
		ExecuteWithBaseRef(selectedConfig, "/tmp/target", true, false, false, "", opts.templateValues).
		Return(errUtils.ErrTargetDirectoryNotEmpty)

	err := executeTemplateGeneration(selectedConfig, "/tmp/target", opts, mockUI)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
}
