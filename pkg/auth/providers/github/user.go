package github

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// DeviceFlowClient defines the interface for GitHub Device Flow operations.
// This allows us to mock the Device Flow for testing.
type DeviceFlowClient interface {
	// StartDeviceFlow initiates the Device Flow and returns device/user codes.
	StartDeviceFlow(ctx context.Context) (*DeviceFlowResponse, error)
	// PollForToken polls GitHub for the access token after user authorization.
	PollForToken(ctx context.Context, deviceCode string, interval int) (string, error)
}

// DeviceFlowResponse contains the response from GitHub's device authorization endpoint.
type DeviceFlowResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// userProvider implements GitHub User authentication via OAuth Device Flow.
type userProvider struct {
	name          string
	config        *schema.Provider
	clientID      string
	scopes        []string
	tokenLifetime time.Duration
	deviceClient  DeviceFlowClient
}

// NewUserProvider creates a new GitHub User provider.
func NewUserProvider(name string, config *schema.Provider) (types.Provider, error) {
	defer perf.Track(nil, "github.NewUserProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidProviderConfig)
	}

	if config.Kind != KindUser {
		return nil, fmt.Errorf("%w: invalid provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract client_id from spec (optional, defaults to GitHub CLI OAuth App).
	clientID := DefaultClientID
	usingDefaultClientID := true
	if config.Spec != nil {
		if customClientID, ok := config.Spec["client_id"].(string); ok && customClientID != "" {
			clientID = customClientID
			usingDefaultClientID = false
		}
	}

	if usingDefaultClientID {
		log.Debug("Using default GitHub CLI OAuth App client ID", "client_id", clientID, "provider", name)
	} else {
		log.Debug("Using custom OAuth App client ID", "provider", name)
	}

	// Extract scopes.
	var scopes []string
	if config.Spec != nil {
		if scopesRaw, ok := config.Spec["scopes"].([]interface{}); ok {
			for _, scope := range scopesRaw {
				if scopeStr, ok := scope.(string); ok {
					scopes = append(scopes, scopeStr)
				}
			}
		}
	}

	// Extract token lifetime (default: 8 hours).
	tokenLifetime := 8 * time.Hour
	if config.Spec != nil {
		if lifetimeStr, ok := config.Spec["token_lifetime"].(string); ok {
			if d, err := time.ParseDuration(lifetimeStr); err == nil {
				tokenLifetime = d
			}
		}
	}

	return &userProvider{
		name:          name,
		config:        config,
		clientID:      clientID,
		scopes:        scopes,
		tokenLifetime: tokenLifetime,
	}, nil
}

// Kind returns the provider kind.
func (p *userProvider) Kind() string {
	return KindUser
}

// Name returns the provider name.
func (p *userProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for GitHub User provider.
func (p *userProvider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// Authenticate performs GitHub User authentication via OAuth Device Flow.
// Following AWS pattern: just return credentials, auth manager handles storage.
func (p *userProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "github.userProvider.Authenticate")()

	log.Info("Starting GitHub User authentication", "provider", p.name)

	// Validate provider configuration.
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Initialize Device Flow client if not already set (for testing).
	if p.deviceClient == nil {
		p.deviceClient = NewDeviceFlowClient(p.clientID, p.scopes)
		log.Debug("Initialized Device Flow client", "provider", p.name, "client_id", p.clientID)
	}

	// Initiate Device Flow (no caching - auth manager handles that).
	log.Info("Initiating GitHub Device Flow", "provider", p.name)

	deviceResp, err := p.deviceClient.StartDeviceFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to start Device Flow: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Display styled dialog with instructions.
	displayDeviceFlowInstructions(deviceResp.VerificationURI, deviceResp.UserCode)

	// Poll for token with spinner.
	token, err := runSpinnerDuringPoll(ctx, p.deviceClient, deviceResp.DeviceCode, deviceResp.Interval)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to obtain token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("GitHub User authentication successful", "provider", p.name)

	// Return credentials - auth manager will store them via credentialStore.
	return &types.GitHubUserCredentials{
		Token:      token,
		Provider:   p.name,
		Expiration: time.Now().Add(p.tokenLifetime),
	}, nil
}

// Validate validates the provider configuration.
func (p *userProvider) Validate() error {
	defer perf.Track(nil, "github.userProvider.Validate")()

	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}

	// Validate token lifetime is reasonable (1h to 24h).
	if p.tokenLifetime < time.Hour || p.tokenLifetime > 24*time.Hour {
		return fmt.Errorf("%w: token_lifetime must be between 1h and 24h", errUtils.ErrInvalidProviderConfig)
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *userProvider) Environment() (map[string]string, error) {
	defer perf.Track(nil, "github.userProvider.Environment")()

	// GitHub User provider doesn't set environment variables at the provider level.
	// Environment variables are set by the identity after authentication.
	return map[string]string{}, nil
}

// Logout is not supported for GitHub User provider.
// Credentials are managed by the auth manager's credential store.
func (p *userProvider) Logout(ctx context.Context) error {
	return fmt.Errorf("logout not supported for GitHub User provider - use 'atmos auth logout' to remove credentials from credential store")
}

// displayDeviceFlowInstructions shows a styled dialog box with Device Flow instructions.
func displayDeviceFlowInstructions(verificationURI, userCode string) {
	// Define styles.
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 2).
		Width(60)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		MarginBottom(1)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Underline(true)

	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen)).
		Bold(true).
		Background(lipgloss.Color("#2A2A2A")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite))

	// Build the dialog content.
	title := titleStyle.Render("GitHub Authentication Required")
	step1 := labelStyle.Render("1. Visit: ") + urlStyle.Render(verificationURI)
	step2 := labelStyle.Render("2. Enter code: ") + codeStyle.Render(userCode)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		step1,
		step2,
	)

	dialog := dialogStyle.Render(content)

	// Print to stderr.
	fmt.Fprintln(os.Stderr, "\n"+dialog+"\n")
}

// spinnerModel is a simple spinner model for waiting during authentication.
type spinnerModel struct {
	spinner    spinner.Model
	message    string
	quitting   bool
	cancelChan chan struct{} // Signal cancellation to parent.
}

func newSpinnerModel(message string, cancelChan chan struct{}) spinnerModel {
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	s.Spinner = spinner.Dot

	return spinnerModel{
		spinner:    s,
		message:    message,
		cancelChan: cancelChan,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			// Signal cancellation to parent.
			if m.cancelChan != nil {
				close(m.cancelChan)
			}
			return m, tea.Quit
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m spinnerModel) View() string {
	if m.quitting {
		return ""
	}
	return fmt.Sprintf("\r%s %s", m.spinner.View(), m.message)
}

// runSpinnerDuringPoll runs a spinner while polling for the token.
func runSpinnerDuringPoll(ctx context.Context, deviceClient DeviceFlowClient, deviceCode string, interval int) (string, error) {
	// Create context that can be cancelled by Ctrl+C.
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel to signal completion.
	tokenChan := make(chan string, 1)
	errChan := make(chan error, 1)
	userCancelChan := make(chan struct{}, 1)

	// Start polling in a goroutine.
	go func() {
		token, err := deviceClient.PollForToken(pollCtx, deviceCode, interval)
		if err != nil {
			errChan <- err
			return
		}
		tokenChan <- token
	}()

	// Check if we have TTY support for spinner.
	var p *tea.Program
	hasTTY := true
	if fileInfo, err := os.Stderr.Stat(); err != nil || (fileInfo.Mode()&os.ModeCharDevice) == 0 {
		// No TTY - just print a message instead of spinner.
		hasTTY = false
		fmt.Fprintln(os.Stderr, "Waiting for authentication... (Press Ctrl+C to cancel)")
	} else {
		// Run spinner with TTY.
		p = tea.NewProgram(newSpinnerModel("Waiting for authentication... (Press Ctrl+C to cancel)", userCancelChan))
		go func() {
			if _, err := p.Run(); err != nil {
				log.Debug("Failed to run spinner", "error", err)
			}
		}()
	}

	// Wait for either token, error, or user cancellation.
	var token string
	var pollErr error

	select {
	case token = <-tokenChan:
	case pollErr = <-errChan:
	case <-userCancelChan:
		// User pressed Ctrl+C - cancel polling.
		cancel()
		pollErr = fmt.Errorf("authentication cancelled by user")
	case <-ctx.Done():
		pollErr = ctx.Err()
	}

	// Stop spinner if running.
	if hasTTY && p != nil {
		p.Quit()
		fmt.Fprintln(os.Stderr, "") // New line after spinner.
	}

	if pollErr != nil {
		return "", pollErr
	}

	return token, nil
}
