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

const (
	// MaxTokenLifetime is the maximum allowed token lifetime (24 hours).
	MaxTokenLifetime = 24 * time.Hour

	// DefaultTokenLifetime is the default token lifetime (8 hours).
	DefaultTokenLifetime = 8 * time.Hour

	// DefaultPollInterval is the default polling interval in seconds if not specified by GitHub.
	DefaultPollInterval = 10

	// DeviceFlowPromptWidth is the display width for device flow prompts.
	DeviceFlowPromptWidth = 60

	// WebFlowPromptWidth is the display width for web flow prompts.
	WebFlowPromptWidth = 70

	// LogKeyProvider is the log key for provider name.
	LogKeyProvider = "provider"
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

// userProvider implements GitHub User authentication via OAuth Device Flow or Web Application Flow.
type userProvider struct {
	name          string
	config        *schema.Provider
	clientID      string
	clientSecret  string
	scopes        []string
	tokenLifetime time.Duration
	flowType      string // "device" or "web"
	deviceClient  DeviceFlowClient
	webClient     WebFlowClient
}

// extractStringFromSpec extracts a string value from provider spec with a default.
func extractStringFromSpec(spec map[string]interface{}, key, defaultValue string) string {
	if spec == nil {
		return defaultValue
	}

	if value, ok := spec[key].(string); ok && value != "" {
		return value
	}

	return defaultValue
}

// extractScopesFromSpec extracts OAuth scopes from provider spec.
func extractScopesFromSpec(spec map[string]interface{}) []string {
	var scopes []string
	if spec == nil {
		return scopes
	}

	scopesRaw, ok := spec["scopes"].([]interface{})
	if !ok {
		return scopes
	}

	for _, scope := range scopesRaw {
		if scopeStr, ok := scope.(string); ok {
			scopes = append(scopes, scopeStr)
		}
	}

	return scopes
}

// extractTokenLifetimeFromSpec extracts token lifetime from provider spec with a default.
func extractTokenLifetimeFromSpec(spec map[string]interface{}, defaultLifetime time.Duration) time.Duration {
	if spec == nil {
		return defaultLifetime
	}

	lifetimeStr, ok := spec["token_lifetime"].(string)
	if !ok {
		return defaultLifetime
	}

	duration, err := time.ParseDuration(lifetimeStr)
	if err != nil {
		return defaultLifetime
	}

	return duration
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

	clientID := extractStringFromSpec(config.Spec, "client_id", DefaultClientID)
	if clientID == DefaultClientID {
		log.Debug("Using default GitHub CLI OAuth App client ID", "client_id", clientID, LogKeyProvider, name)
	} else {
		log.Debug("Using custom OAuth App client ID", LogKeyProvider, name)
	}

	clientSecret := extractStringFromSpec(config.Spec, "client_secret", DefaultClientSecret)
	flowType := extractStringFromSpec(config.Spec, "flow_type", "device")
	scopes := extractScopesFromSpec(config.Spec)
	tokenLifetime := extractTokenLifetimeFromSpec(config.Spec, DefaultTokenLifetime)

	return &userProvider{
		name:          name,
		config:        config,
		clientID:      clientID,
		clientSecret:  clientSecret,
		scopes:        scopes,
		tokenLifetime: tokenLifetime,
		flowType:      flowType,
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

// Authenticate performs GitHub User authentication via OAuth Device Flow or Web Application Flow.
// Following AWS pattern: just return credentials, auth manager handles storage.
func (p *userProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "github.userProvider.Authenticate")()

	log.Info("Starting GitHub User authentication", LogKeyProvider, p.name, "flow", p.flowType)

	// Validate provider configuration.
	if err := p.Validate(); err != nil {
		return nil, err
	}

	var token string
	var err error

	switch p.flowType {
	case "device":
		token, err = p.authenticateDeviceFlow(ctx)
	case "web":
		token, err = p.authenticateWebFlow(ctx)
	default:
		return nil, fmt.Errorf("%w: invalid flow_type: %s (must be 'device' or 'web')", errUtils.ErrInvalidProviderConfig, p.flowType)
	}

	if err != nil {
		return nil, err
	}

	log.Info("GitHub User authentication successful", LogKeyProvider, p.name)

	// Return credentials - auth manager will store them via credentialStore.
	return &types.GitHubUserCredentials{
		Token:      token,
		Provider:   p.name,
		Expiration: time.Now().Add(p.tokenLifetime),
	}, nil
}

// authenticateDeviceFlow performs OAuth Device Flow authentication.
func (p *userProvider) authenticateDeviceFlow(ctx context.Context) (string, error) {
	defer perf.Track(nil, "github.userProvider.authenticateDeviceFlow")()

	// Initialize Device Flow client if not already set (for testing).
	if p.deviceClient == nil {
		p.deviceClient = NewDeviceFlowClient(p.clientID, p.scopes)
		log.Debug("Initialized Device Flow client", LogKeyProvider, p.name, "client_id", p.clientID)
	}

	// Initiate Device Flow (no caching - auth manager handles that).
	log.Info("Initiating GitHub Device Flow", LogKeyProvider, p.name)

	deviceResp, err := p.deviceClient.StartDeviceFlow(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: failed to start Device Flow: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Display styled dialog with instructions.
	displayDeviceFlowInstructions(deviceResp.VerificationURI, deviceResp.UserCode)

	// Poll for token with spinner.
	token, err := runSpinnerDuringPoll(ctx, p.deviceClient, deviceResp.DeviceCode, deviceResp.Interval)
	if err != nil {
		return "", fmt.Errorf("%w: failed to obtain token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	return token, nil
}

// authenticateWebFlow performs OAuth Web Application Flow authentication.
func (p *userProvider) authenticateWebFlow(ctx context.Context) (string, error) {
	defer perf.Track(nil, "github.userProvider.authenticateWebFlow")()

	// Initialize Web Flow client if not already set (for testing).
	if p.webClient == nil {
		p.webClient = NewWebFlowClient(p.clientID, p.clientSecret, p.scopes)
		log.Debug("Initialized Web Flow client", LogKeyProvider, p.name)
	}

	// Start web flow and get authorization URL.
	log.Info("Initiating GitHub Web Application Flow", LogKeyProvider, p.name)

	authURL, state, err := p.webClient.StartWebFlow(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: failed to start Web Flow: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Display instructions and open browser.
	displayWebFlowInstructions(authURL)

	// Try to open browser automatically.
	if err := OpenBrowser(authURL); err != nil {
		log.Debug("Failed to open browser automatically", "error", err)
		fmt.Fprintf(os.Stderr, "Please open the URL manually if the browser doesn't open automatically.\n\n")
	}

	// Wait for callback with spinner.
	token, err := runSpinnerDuringWebFlow(ctx, p.webClient, state)
	if err != nil {
		return "", fmt.Errorf("%w: failed to obtain token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	return token, nil
}

// Validate validates the provider configuration.
func (p *userProvider) Validate() error {
	defer perf.Track(nil, "github.userProvider.Validate")()

	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}

	// Validate token lifetime is reasonable (1h to 24h).
	if p.tokenLifetime < time.Hour || p.tokenLifetime > MaxTokenLifetime {
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
	return fmt.Errorf("%w: use 'atmos auth logout' instead", errUtils.ErrAuthenticationFailed)
}

// displayDeviceFlowInstructions shows a styled dialog box with Device Flow instructions.
func displayDeviceFlowInstructions(verificationURI, userCode string) {
	// Define styles.
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 2).
		Width(DeviceFlowPromptWidth)

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

func newSpinnerModel(message string, cancelChan chan struct{}) *spinnerModel {
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	s.Spinner = spinner.Dot

	return &spinnerModel{
		spinner:    s,
		message:    message,
		cancelChan: cancelChan,
	}
}

func (m *spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m *spinnerModel) View() string {
	if m.quitting {
		return ""
	}
	return fmt.Sprintf("\r%s %s", m.spinner.View(), m.message)
}

// spinnerUI manages TTY spinner display.
type spinnerUI struct {
	hasTTY  bool
	program *tea.Program
}

// startSpinner initializes and starts the spinner if TTY is available.
func startSpinner(message string, cancelChan chan struct{}) *spinnerUI {
	ui := &spinnerUI{}

	fileInfo, err := os.Stderr.Stat()
	if err != nil || (fileInfo.Mode()&os.ModeCharDevice) == 0 {
		// No TTY - just print a message.
		fmt.Fprintln(os.Stderr, message)
		return ui
	}

	// Run spinner with TTY.
	ui.hasTTY = true
	ui.program = tea.NewProgram(newSpinnerModel(message, cancelChan))
	go func() {
		if _, err := ui.program.Run(); err != nil {
			log.Debug("Failed to run spinner", "error", err)
		}
	}()

	return ui
}

// stop stops the spinner if it's running.
func (ui *spinnerUI) stop() {
	if ui.hasTTY && ui.program != nil {
		ui.program.Quit()
		fmt.Fprintln(os.Stderr, "")
	}
}

// runSpinnerDuringPoll runs a spinner while polling for the token.
func runSpinnerDuringPoll(ctx context.Context, deviceClient DeviceFlowClient, deviceCode string, interval int) (string, error) {
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	tokenChan := make(chan string, 1)
	errChan := make(chan error, 1)
	userCancelChan := make(chan struct{}, 1)

	go func() {
		token, err := deviceClient.PollForToken(pollCtx, deviceCode, interval)
		if err != nil {
			errChan <- err
			return
		}
		tokenChan <- token
	}()

	spinner := startSpinner("Waiting for authentication... (Press Ctrl+C to cancel)", userCancelChan)
	defer spinner.stop()

	select {
	case token := <-tokenChan:
		return token, nil
	case err := <-errChan:
		return "", err
	case <-userCancelChan:
		cancel()
		return "", errUtils.ErrAuthenticationCancelled
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// displayWebFlowInstructions shows a styled dialog box with Web Flow instructions.
func displayWebFlowInstructions(authURL string) {
	// Define styles.
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 2).
		Width(WebFlowPromptWidth)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		MarginBottom(1)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Underline(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite))

	// Build the dialog content.
	title := titleStyle.Render("GitHub Authentication Required")
	instruction := labelStyle.Render("Opening browser for authentication...")
	urlLine := labelStyle.Render("URL: ") + urlStyle.Render(authURL)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		instruction,
		urlLine,
	)

	dialog := dialogStyle.Render(content)

	// Print to stderr.
	fmt.Fprintln(os.Stderr, "\n"+dialog+"\n")
}

// runSpinnerDuringWebFlow runs a spinner while waiting for the web flow callback.
func runSpinnerDuringWebFlow(ctx context.Context, webClient WebFlowClient, state string) (string, error) {
	callbackCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	tokenChan := make(chan string, 1)
	errChan := make(chan error, 1)
	userCancelChan := make(chan struct{}, 1)

	go func() {
		token, err := webClient.WaitForCallback(callbackCtx, state)
		if err != nil {
			errChan <- err
			return
		}
		tokenChan <- token
	}()

	spinner := startSpinner("Waiting for browser authorization... (Press Ctrl+C to cancel)", userCancelChan)
	defer spinner.stop()

	select {
	case token := <-tokenChan:
		return token, nil
	case err := <-errChan:
		return "", err
	case <-userCancelChan:
		cancel()
		return "", errUtils.ErrAuthenticationCancelled
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
