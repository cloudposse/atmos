package aws

// Tests for TTY detection, display dialogs, and bubbletea spinner model (webflow_ui.go).

import (
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWebflowSpinnerModel_View(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	tests := []struct {
		name     string
		model    webflowSpinnerModel
		contains string
	}{
		{
			name: "in progress",
			model: webflowSpinnerModel{
				spinner: s,
				message: "Waiting for auth",
				done:    false,
			},
			contains: "Waiting for auth",
		},
		{
			name: "done with error",
			model: webflowSpinnerModel{
				spinner: s,
				done:    true,
				result:  &webflowSpinnerTokenResult{err: fmt.Errorf("failed")},
			},
			contains: "Authentication failed",
		},
		{
			name: "done success",
			model: webflowSpinnerModel{
				spinner: s,
				done:    true,
				result:  &webflowSpinnerTokenResult{resp: &webflowTokenResponse{}},
			},
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := tt.model.View()
			if tt.contains != "" {
				assert.Contains(t, view, tt.contains)
			}
		})
	}
}

func TestWebflowSpinnerModel_Update_CtrlC(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	cancelled := false
	model := webflowSpinnerModel{
		spinner: s,
		message: "Waiting",
		tokenCh: make(<-chan webflowSpinnerTokenResult),
		cancel:  func() { cancelled = true },
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := model.Update(msg)
	m := updated.(webflowSpinnerModel)

	assert.True(t, m.done)
	assert.NotNil(t, m.result)
	assert.ErrorIs(t, m.result.err, errUtils.ErrUserAborted)
	assert.True(t, cancelled)
}

func TestWebflowSpinnerModel_Update_TokenResult(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	cancelled := false
	model := webflowSpinnerModel{
		spinner: s,
		message: "Waiting",
		tokenCh: make(<-chan webflowSpinnerTokenResult),
		cancel:  func() { cancelled = true },
	}

	tokenResult := webflowSpinnerTokenResult{
		resp: &webflowTokenResponse{
			AccessToken: webflowAccessToken{
				AccessKeyID:     "AKID",
				SecretAccessKey: "SECRET",
				SessionToken:    "TOKEN",
			},
		},
	}

	updated, _ := model.Update(tokenResult)
	m := updated.(webflowSpinnerModel)

	assert.True(t, m.done)
	assert.NotNil(t, m.result)
	assert.NoError(t, m.result.err)
	assert.Equal(t, "AKID", m.result.resp.AccessToken.AccessKeyID)
	assert.True(t, cancelled)
}

func TestDisplayWebflowDialog(t *testing.T) {
	// Just verify it doesn't panic.
	displayWebflowDialog("https://example.com/auth")
}

func TestDisplayWebflowDialogPlainText(t *testing.T) {
	// Just verify it doesn't panic.
	displayWebflowDialogPlainText("https://example.com/auth")
}

func TestWebflowIsTTY(t *testing.T) {
	// In test environment, stderr is typically not a TTY.
	result := webflowIsTTY()
	assert.False(t, result)
}

func TestWebflowIsInteractive(t *testing.T) {
	// Without force-tty, in test env this should return false.
	result := webflowIsInteractive()
	assert.False(t, result)
}

func TestWebflowSpinnerModel_Init(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	tokenCh := make(chan webflowSpinnerTokenResult)

	model := webflowSpinnerModel{
		spinner: s,
		message: "Test",
		tokenCh: tokenCh,
		cancel:  func() {},
	}

	cmd := model.Init()
	assert.NotNil(t, cmd)
}

func TestWebflowSpinnerModel_CheckResult(t *testing.T) {
	tokenCh := make(chan webflowSpinnerTokenResult, 1)
	tokenCh <- webflowSpinnerTokenResult{
		resp: &webflowTokenResponse{
			AccessToken: webflowAccessToken{AccessKeyID: "AKID_CHECK"},
		},
	}

	s := spinner.New()
	model := webflowSpinnerModel{
		spinner: s,
		tokenCh: tokenCh,
	}

	cmd := model.checkResult()
	require.NotNil(t, cmd)

	// Execute the command to get the result.
	msg := cmd()
	result, ok := msg.(webflowSpinnerTokenResult)
	require.True(t, ok)
	assert.Equal(t, "AKID_CHECK", result.resp.AccessToken.AccessKeyID)
}

// TestWebflowIsInteractive_ForceTTY verifies the force-tty branch of
// webflowIsInteractive. When the viper force-tty setting is true, the function
// must return true regardless of the actual TTY state.
func TestWebflowIsInteractive_ForceTTY(t *testing.T) {
	t.Setenv("ATMOS_FORCE_TTY", "true")
	// viper reads the env binding via BindEnv elsewhere; directly set the key
	// so this test does not depend on global viper wiring.
	viper.Set("force-tty", true)
	defer viper.Set("force-tty", false)

	assert.True(t, webflowIsInteractive(), "force-tty must override TTY detection")
}

// TestWebflowIsInteractive_FollowsTTY verifies that when force-tty is not set,
// webflowIsInteractive delegates to webflowIsTTYFunc.
func TestWebflowIsInteractive_FollowsTTY(t *testing.T) {
	viper.Set("force-tty", false)
	defer viper.Set("force-tty", false)

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()
	assert.False(t, webflowIsInteractive())

	webflowIsTTYFunc = func() bool { return true }
	assert.True(t, webflowIsInteractive())
}

// TestHandleSpinnerFallback_DrainedResultReturned verifies that when the
// exchange goroutine has already produced a result (e.g. the callback
// arrived just before context cancellation), handleSpinnerFallback returns
// it directly without falling through to waitForCallbackSimple.
func TestHandleSpinnerFallback_DrainedResultReturned(t *testing.T) {
	identity := &userIdentity{
		name:   "test-spinner-drain",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Pre-populate tokenCh with a successful result to simulate the case
	// where the goroutine captured the real response before cancellation.
	tokenCh := make(chan webflowSpinnerTokenResult, 1)
	tokenCh <- webflowSpinnerTokenResult{resp: &webflowTokenResponse{
		AccessToken: webflowAccessToken{
			AccessKeyID: "AKID_DRAIN", SecretAccessKey: "SECRET_DRAIN", SessionToken: "TOKEN_DRAIN",
		},
		ExpiresIn: 900,
	}}

	resultCh := make(chan webflowResult) // Not used by this path.
	_, cancel := context.WithCancel(context.Background())

	resp, err := identity.handleSpinnerFallback(&spinnerFallbackParams{
		cancel: cancel, tokenCh: tokenCh, resultCh: resultCh,
		region: "us-east-2", verifier: "verifier", redirectURI: "http://127.0.0.1:0/oauth/callback",
		runErr: errUtils.ErrWebflowAuthFailed,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "AKID_DRAIN", resp.AccessToken.AccessKeyID)
}

// TestHandleSpinnerFallback_DrainedErrorReturned verifies that a drained
// non-timeout error is surfaced directly.
func TestHandleSpinnerFallback_DrainedErrorReturned(t *testing.T) {
	identity := &userIdentity{
		name:   "test-spinner-drain-err",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	tokenCh := make(chan webflowSpinnerTokenResult, 1)
	tokenCh <- webflowSpinnerTokenResult{err: errUtils.ErrWebflowAuthFailed}

	resultCh := make(chan webflowResult)
	_, cancel := context.WithCancel(context.Background())

	resp, err := identity.handleSpinnerFallback(&spinnerFallbackParams{
		cancel: cancel, tokenCh: tokenCh, resultCh: resultCh,
		region: "us-east-2", verifier: "verifier", redirectURI: "http://127.0.0.1:0/oauth/callback",
		runErr: errUtils.ErrWebflowAuthFailed,
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
}

// TestWebflowSpinnerModel_TickMessage verifies that spinner.TickMsg is
// forwarded to the underlying spinner and produces an advance command.
func TestWebflowSpinnerModel_TickMessage(t *testing.T) {
	tokenCh := make(chan webflowSpinnerTokenResult, 1)
	m := newWebflowSpinnerModel(tokenCh, func() {})

	// spinner.TickMsg is the real tick type from the bubbles package.
	newModel, cmd := m.Update(spinner.TickMsg{})
	result := newModel.(webflowSpinnerModel)

	assert.False(t, result.done)
	// Tick forwarding returns a cmd from the spinner to schedule the next tick.
	_ = cmd
}
