package azure

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSpinnerModel(t *testing.T) {
	t.Run("Init returns Tick command", func(t *testing.T) {
		model := newSpinnerModel()
		cmd := model.Init()
		assert.NotNil(t, cmd)
	})

	t.Run("Update handles authCompleteMsg success", func(t *testing.T) {
		model := newSpinnerModel()
		now := time.Now().UTC()

		msg := authCompleteMsg{
			token:     "test-token",
			expiresOn: now,
			err:       nil,
		}

		updatedModel, cmd := model.Update(msg)
		assert.NotNil(t, cmd)

		m := updatedModel.(*spinnerModel)
		assert.True(t, m.quitting)
		assert.Equal(t, "test-token", m.token)
		assert.Equal(t, now, m.expiresOn)
		assert.NoError(t, m.authErr)
	})

	t.Run("Update handles authCompleteMsg error", func(t *testing.T) {
		model := newSpinnerModel()
		testErr := errUtils.ErrAuthenticationFailed

		msg := authCompleteMsg{
			token:     "",
			expiresOn: time.Time{},
			err:       testErr,
		}

		updatedModel, cmd := model.Update(msg)
		assert.NotNil(t, cmd)

		m := updatedModel.(*spinnerModel)
		assert.True(t, m.quitting)
		assert.Empty(t, m.token)
		assert.ErrorIs(t, m.authErr, testErr)
	})

	t.Run("Update handles Ctrl+C", func(t *testing.T) {
		model := newSpinnerModel()

		msg := tea.KeyMsg{Type: tea.KeyCtrlC}

		updatedModel, cmd := model.Update(msg)
		assert.NotNil(t, cmd)

		m := updatedModel.(*spinnerModel)
		assert.True(t, m.quitting)
		assert.Error(t, m.authErr)
		assert.ErrorIs(t, m.authErr, errUtils.ErrAuthenticationFailed)
	})

	t.Run("View shows spinner when not quitting", func(t *testing.T) {
		model := newSpinnerModel()
		view := model.View()
		assert.Contains(t, view, "Waiting for authentication...")
	})

	t.Run("View shows success when quitting without error", func(t *testing.T) {
		model := newSpinnerModel()
		model.quitting = true
		model.authErr = nil
		view := model.View()
		assert.Contains(t, view, "✓")
		assert.Contains(t, view, "Authentication successful!")
	})

	t.Run("View shows empty string when quitting with error", func(t *testing.T) {
		model := newSpinnerModel()
		model.quitting = true
		model.authErr = errUtils.ErrAuthenticationFailed
		view := model.View()
		assert.Empty(t, view)
	})
}
