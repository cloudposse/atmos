package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	ssoDefaultSessionMinutes = 60
)

// isInteractive checks if we're running in an interactive terminal.
// For SSO device flow, we need stderr to be a TTY so the user can see the authentication URL.
// We check stderr (not stdin) because that's where we output the authentication instructions.
func isInteractive() bool {
	return term.IsTTYSupportForStderr()
}

// ssoProvider implements AWS IAM Identity Center authentication.
type ssoProvider struct {
	name     string
	config   *schema.Provider
	startURL string
	region   string
}

// NewSSOProvider creates a new AWS SSO provider.
func NewSSOProvider(name string, config *schema.Provider) (*ssoProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "aws/iam-identity-center" {
		return nil, fmt.Errorf("%w: invalid provider kind for SSO provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	if config.StartURL == "" {
		return nil, fmt.Errorf("%w: start_url is required for AWS SSO provider", errUtils.ErrInvalidProviderConfig)
	}

	if config.Region == "" {
		return nil, fmt.Errorf("%w: region is required for AWS SSO provider", errUtils.ErrInvalidProviderConfig)
	}

	return &ssoProvider{
		name:     name,
		config:   config,
		startURL: config.StartURL,
		region:   config.Region,
	}, nil
}

// Kind returns the provider kind.
func (p *ssoProvider) Kind() string {
	return "aws/iam-identity-center"
}

// Name returns the configured provider name.
func (p *ssoProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for SSO provider.
func (p *ssoProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// Authenticate performs AWS SSO authentication.
func (p *ssoProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	// Note: SSO provider no longer caches credentials directly.
	// Caching is handled at the manager level to prevent duplicates.

	// Check if we're in a headless environment - SSO device flow requires user interaction.
	if !isInteractive() {
		return nil, fmt.Errorf("%w: SSO device flow requires an interactive terminal (no TTY detected). Use environment credentials or service account authentication in headless environments", errUtils.ErrAuthenticationFailed)
	}

	// Build config options.
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(p.region),
		// Disable credential providers to avoid hanging on EC2 metadata service or other credential sources.
		// SSO device flow doesn't require existing credentials.
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	}

	// Add custom endpoint resolver if configured.
	if resolverOpt := awsCloud.GetResolverConfigOption(nil, p.config); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	log.Debug("Loading AWS config for SSO authentication", "region", p.region)
	// Initialize AWS config for the SSO region with isolated environment
	// to avoid conflicts with external AWS env vars.
	cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %w", errUtils.ErrAuthenticationFailed, err)
	}
	log.Debug("AWS config loaded successfully")

	// Create OIDC client for device authorization.
	oidcClient := ssooidc.NewFromConfig(cfg)

	log.Debug("Registering SSO client")
	// Register the client.
	registerResp, err := oidcClient.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("atmos-auth"),
		ClientType: aws.String("public"),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to register SSO client: %w", errUtils.ErrAuthenticationFailed, err)
	}
	log.Debug("SSO client registered successfully")

	log.Debug("Starting device authorization")
	// Start device authorization.
	authResp, err := oidcClient.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     registerResp.ClientId,
		ClientSecret: registerResp.ClientSecret,
		StartUrl:     aws.String(p.startURL),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to start device authorization: %w", errUtils.ErrAuthenticationFailed, err)
	}
	log.Debug("Device authorization started")

	p.promptDeviceAuth(authResp)

	// Poll for token with a spinner (if TTY).
	accessToken, tokenExpiresAt, err := p.pollForAccessTokenWithSpinner(ctx, oidcClient, registerResp, authResp)
	if err != nil {
		return nil, err
	}

	// Calculate expiration time.
	// Use token expiration (fallback to session duration if unavailable).
	expiration := tokenExpiresAt
	if expiration.IsZero() {
		expiration = time.Now().Add(time.Duration(p.getSessionDuration()) * time.Minute)
	}
	log.Debug("Authentication successful", "expiration", expiration)

	return &authTypes.AWSCredentials{
		AccessKeyID: accessToken, // Used by identities to get actual credentials
		Region:      p.region,
		Expiration:  expiration.Format(time.RFC3339),
	}, nil
}

// promptDeviceAuth displays user code and verification URI.
// Shows the prompt unless we're in a non-interactive environment (real CI without TTY).
func (p *ssoProvider) promptDeviceAuth(authResp *ssooidc.StartDeviceAuthorizationOutput) {
	code := ""
	if authResp.UserCode != nil {
		code = *authResp.UserCode
	}

	verificationURL := ""
	if authResp.VerificationUriComplete != nil && *authResp.VerificationUriComplete != "" {
		verificationURL = *authResp.VerificationUriComplete
	} else if authResp.VerificationUri != nil {
		verificationURL = *authResp.VerificationUri
	}

	// Always display the prompt - even if CI env vars are set, the user might be running make locally.
	log.Debug("Displaying authentication prompt", "url", verificationURL, "code", code, "isCI", telemetry.IsCI())

	// Check if we have a TTY for fancy output.
	if isTTY() && !telemetry.IsCI() {
		displayVerificationDialog(code, verificationURL)
	} else {
		// Fallback to simple text output for non-TTY or CI environments.
		displayVerificationPlainText(code, verificationURL)
	}

	// Open browser if not in CI. The browser open will work if there's a display available.
	if !telemetry.IsCI() && authResp.VerificationUriComplete != nil && *authResp.VerificationUriComplete != "" {
		if err := utils.OpenUrl(*authResp.VerificationUriComplete); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		} else {
			log.Debug("Browser opened successfully")
		}
	}
	log.Debug("Finished promptDeviceAuth, starting polling")
}

// isTTY checks if stderr is a terminal.
func isTTY() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// displayVerificationDialog shows a styled dialog with the verification code.
func displayVerificationDialog(code, url string) {
	// Styles using Atmos theme colors.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		PaddingLeft(1).
		PaddingRight(1)

	codeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorGreen)).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(0, 1)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorBorder)).
		Italic(true)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorDarkGray))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	// Build the content.
	var content strings.Builder
	content.WriteString(titleStyle.Render("🔐 AWS SSO Authentication Required"))
	content.WriteString("\n\n")
	content.WriteString("Verification Code: ")
	content.WriteString(codeStyle.Render(code))
	content.WriteString("\n\n")

	if url != "" {
		content.WriteString(urlStyle.Render(url))
		content.WriteString("\n\n")
		content.WriteString(instructionStyle.Render("Opening browser... If it doesn't open, visit the URL above."))
	}

	// Render the box and display it.
	fmt.Fprintf(os.Stderr, "%s\n", boxStyle.Render(content.String()))
}

// displayVerificationPlainText shows verification code in plain text (for non-TTY/CI).
func displayVerificationPlainText(code, url string) {
	utils.PrintfMessageToTUI("🔐 **AWS SSO Authentication Required**\n")
	utils.PrintfMessageToTUI("Verification Code: **%s**\n", code)

	if url != "" {
		if !telemetry.IsCI() {
			utils.PrintfMessageToTUI("Opening browser to: %s\n", url)
			utils.PrintfMessageToTUI("If the browser does not open, visit the URL above and enter the code.\n")
		} else {
			utils.PrintfMessageToTUI("Verification URL: %s\n", url)
		}
	}

	utils.PrintfMessageToTUI("Waiting for authentication...\n")
}

// Validate validates the provider configuration.
func (p *ssoProvider) Validate() error {
	defer perf.Track(nil, "aws.ssoProvider.Validate")()

	if p.startURL == "" {
		return fmt.Errorf("%w: start_url is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.region == "" {
		return fmt.Errorf("%w: region is required", errUtils.ErrInvalidProviderConfig)
	}

	// Validate spec.files.base_path if provided.
	if err := awsCloud.ValidateFilesBasePath(p.config); err != nil {
		return err
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *ssoProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	env["AWS_REGION"] = p.region
	return env, nil
}

// Paths returns credential files/directories used by this provider.
func (p *ssoProvider) Paths() ([]authTypes.Path, error) {
	basePath := awsCloud.GetFilesBasePath(p.config)

	// Use AWSFileManager to get correct provider-namespaced paths.
	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return nil, err
	}

	return []authTypes.Path{
		{
			Location: fileManager.GetCredentialsPath(p.name),
			Type:     authTypes.PathTypeFile,
			Required: true,
			Purpose:  fmt.Sprintf("AWS credentials file for provider %s", p.name),
			Metadata: map[string]string{
				"read_only": "true",
			},
		},
		{
			Location: fileManager.GetConfigPath(p.name),
			Type:     authTypes.PathTypeFile,
			Required: false, // Config file is optional.
			Purpose:  fmt.Sprintf("AWS config file for provider %s", p.name),
			Metadata: map[string]string{
				"read_only": "true",
			},
		},
	}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For SSO providers, this method is typically not called directly since SSO providers
// authenticate to get identity credentials, which then have their own PrepareEnvironment.
// However, we implement it for interface compliance.
func (p *ssoProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.ssoProvider.PrepareEnvironment")()

	// SSO provider doesn't write credential files itself - that's done by identities.
	// Just return the environment unchanged.
	return environ, nil
}

// Note: SSO caching is now handled at the manager level to prevent duplicate entries.

// getSessionDuration returns the session duration in minutes.
func (p *ssoProvider) getSessionDuration() int {
	if p.config.Session != nil && p.config.Session.Duration != "" {
		// Parse duration (e.g., "15m", "1h").
		if duration, err := time.ParseDuration(p.config.Session.Duration); err == nil {
			return int(duration.Minutes())
		}
	}
	return ssoDefaultSessionMinutes // Default to 60 minutes
}

// pollForAccessTokenWithSpinner wraps pollForAccessToken with a spinner for TTY environments.
func (p *ssoProvider) pollForAccessTokenWithSpinner(ctx context.Context, oidcClient *ssooidc.Client, registerResp *ssooidc.RegisterClientOutput, authResp *ssooidc.StartDeviceAuthorizationOutput) (string, time.Time, error) {
	// If not a TTY or in CI, use simple polling without spinner.
	if !isTTY() || telemetry.IsCI() {
		return p.pollForAccessToken(ctx, oidcClient, registerResp, authResp)
	}

	// Derive a cancellable context and a channel to receive the result.
	ctx, cancel := context.WithCancel(ctx)
	resultChan := make(chan pollResult, 1)

	// Start polling in a goroutine.
	go func() {
		defer close(resultChan)
		token, expiresAt, err := p.pollForAccessToken(ctx, oidcClient, registerResp, authResp)
		resultChan <- pollResult{token, expiresAt, err}
	}()

	// Create and run the spinner.
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	model := spinnerModel{
		spinner:    s,
		message:    "Waiting for authentication",
		done:       false,
		resultChan: resultChan,
		cancel:     cancel,
	}

	// Run the spinner until authentication completes.
	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	finalModel, err := prog.Run()
	if err != nil {
		// Cancel poll on UI failure, then read best-effort result (if any).
		cancel()
		res := <-resultChan
		return res.token, res.expiresAt, res.err
	}

	// Get the result from the final model.
	finalSpinner := finalModel.(spinnerModel)
	if finalSpinner.result == nil {
		return "", time.Time{}, fmt.Errorf("%w: no result received", errUtils.ErrAuthenticationFailed)
	}
	if finalSpinner.result.err != nil {
		return "", time.Time{}, finalSpinner.result.err
	}
	return finalSpinner.result.token, finalSpinner.result.expiresAt, nil
}

// pollResult holds the result of polling for an access token.
type pollResult struct {
	token     string
	expiresAt time.Time
	err       error
}

// spinnerModel is a bubbletea model for the authentication spinner.
// Note: The struct is large (~656 bytes) due to spinner.Model from bubbletea.
// We must use value receivers (not pointers) as required by the bubbletea framework.
// The performance impact is minimal since this is only used during UI updates.
type spinnerModel struct {
	spinner    spinner.Model
	message    string
	done       bool
	resultChan chan pollResult
	cancel     context.CancelFunc
	result     *pollResult // Store result pointer to keep struct small.
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.checkResult())
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			m.result = &pollResult{err: errUtils.ErrUserAborted}
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case pollResult:
		m.done = true
		m.result = &msg
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	return m, nil
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m spinnerModel) View() string {
	if m.done {
		if m.result != nil && m.result.err != nil {
			return fmt.Sprintf("%s Authentication failed\n", theme.Styles.XMark)
		}
		// Success - don't print message here, auth login will display detailed table.
		return ""
	}
	return fmt.Sprintf("%s %s...", m.spinner.View(), m.message)
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m spinnerModel) checkResult() tea.Cmd {
	return func() tea.Msg {
		select {
		case res := <-m.resultChan:
			return res
		case <-time.After(100 * time.Millisecond):
			// Check again after a short delay.
			return m.checkResult()()
		}
	}
}

// pollForAccessToken polls the device authorization endpoint until an access token is available or times out.
func (p *ssoProvider) pollForAccessToken(ctx context.Context, oidcClient *ssooidc.Client, registerResp *ssooidc.RegisterClientOutput, authResp *ssooidc.StartDeviceAuthorizationOutput) (string, time.Time, error) {
	var accessToken string
	var tokenExpiresAt time.Time
	expiresIn := authResp.ExpiresIn
	interval := authResp.Interval
	// Normalize to a sane minimum to avoid divide-by-zero and busy-waiting.
	if interval <= 0 {
		interval = 1
	}

	intervalDur := time.Duration(interval) * time.Second

	// Initial delay before first poll.
	time.Sleep(intervalDur)
	for i := 0; i < int(expiresIn/interval); i++ {
		tokenResp, err := oidcClient.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     registerResp.ClientId,
			ClientSecret: registerResp.ClientSecret,
			DeviceCode:   authResp.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})
		if err == nil {
			accessToken = aws.ToString(tokenResp.AccessToken)
			tokenExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
			break
		}

		var authPendingErr *types.AuthorizationPendingException
		var slowDownErr *types.SlowDownException

		if errors.As(err, &authPendingErr) {
			time.Sleep(intervalDur)
			continue
		} else if errors.As(err, &slowDownErr) {
			// Slow down: double the interval as requested by the server.
			intervalDur = time.Duration(interval*2) * time.Second
			time.Sleep(intervalDur)
			continue
		}

		return "", time.Time{}, fmt.Errorf("%w: failed to create token: %w", errUtils.ErrAuthenticationFailed, err)
	}

	if accessToken == "" {
		return "", time.Time{}, fmt.Errorf("%w: authentication timed out", errUtils.ErrAuthenticationFailed)
	}
	return accessToken, tokenExpiresAt, nil
}

// Logout removes provider-specific credential storage.
func (p *ssoProvider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.ssoProvider.Logout")()

	// Get base_path from provider spec if configured.
	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrProviderLogout, errUtils.ErrLogoutFailed, err)
	}

	if err := fileManager.Cleanup(p.name); err != nil {
		log.Debug("Failed to cleanup AWS files for SSO provider", "provider", p.name, "error", err)
		return errors.Join(errUtils.ErrProviderLogout, errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Cleaned up AWS files for SSO provider", "provider", p.name)
	return nil
}

// GetFilesDisplayPath returns the display path for AWS credential files.
func (p *ssoProvider) GetFilesDisplayPath() string {
	defer perf.Track(nil, "aws.ssoProvider.GetFilesDisplayPath")()

	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return "~/.aws/atmos"
	}

	return fileManager.GetDisplayPath()
}
