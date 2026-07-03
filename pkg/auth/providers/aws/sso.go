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
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/browser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/spinner/fps"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	ssoDefaultSessionMinutes = 60
)

// isInteractive checks if we're running in an interactive terminal.
// For SSO device flow, we need stderr to be a TTY so the user can see the authentication URL.
// We check stderr (not stdin) because that's where we output the authentication instructions.
// Respects --force-tty / ATMOS_FORCE_TTY for environments where TTY detection fails.
func isInteractive() bool {
	if viper.GetBool("force-tty") {
		return true
	}
	return isTTY()
}

// ssoProvider implements AWS IAM Identity Center authentication.
type ssoProvider struct {
	name         string
	config       *schema.Provider
	startURL     string
	region       string
	cacheStorage CacheStorage
	ssoClient    ssoClient          // For dependency injection in tests.
	sessionStore *sessionTokenStore // In-process registry that coalesces same-portal providers.
	realm        string             // Credential isolation realm set by auth manager.
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
		name:         name,
		config:       config,
		startURL:     config.StartURL,
		region:       config.Region,
		cacheStorage: &defaultCacheStorage{}, // Use default filesystem operations.
		sessionStore: defaultSessionStore,    // Shared across providers in this process.
	}, nil
}

// sessionKey returns the cache key for this provider's SSO portal session. Two
// providers with identical (start_url, region) tuples produce the same key and
// therefore share a token.
func (p *ssoProvider) sessionKey() string {
	return sessionKey(p.startURL, p.region)
}

// Kind returns the provider kind.
func (p *ssoProvider) Kind() string {
	return "aws/iam-identity-center"
}

// Name returns the configured provider name.
func (p *ssoProvider) Name() string {
	return p.name
}

// SetRealm sets the credential isolation realm for this provider.
func (p *ssoProvider) SetRealm(realm string) {
	p.realm = realm
}

// PreAuthenticate is a no-op for SSO provider.
func (p *ssoProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// Authenticate performs AWS SSO authentication.
//
// The flow is structured as a fast-path / slow-path pipeline:
//
//  1. In-memory session cache hit — instant return, no I/O. Coalesces concurrent
//     callers across providers that share an SSO portal.
//  2. On-disk session cache hit — returns within ms, no network.
//  3. Refresh-token exchange — silent network call to the SSO OIDC service, no
//     browser interaction. Used when the access token expired but the refresh
//     token is still valid (typically up to ~8 hours after initial login).
//  4. Device-authorization flow — full browser interaction. Only runs when all
//     fast paths miss. Single-flighted per session so concurrent callers wait on
//     one flow rather than starting their own.
func (p *ssoProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	key := p.sessionKey()

	// 1. In-memory fast path. Coalesces concurrent providers sharing this session.
	if cached, ok := p.sessionStore.Get(key); ok {
		log.Debug("Using in-memory cached SSO token", "expiresAt", cached.ExpiresAt)
		return ssoCacheToCredentials(cached, p.region), nil
	}

	// Serialize on the per-session mutex so concurrent Authenticate() calls for the
	// same portal don't all kick off device-auth flows.
	mu := p.sessionStore.Acquire(key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring the lock — another goroutine may have just
	// completed authentication for this session while we were blocked.
	if cached, ok := p.sessionStore.Get(key); ok {
		log.Debug("Using in-memory cached SSO token (post-lock)", "expiresAt", cached.ExpiresAt)
		return ssoCacheToCredentials(cached, p.region), nil
	}

	// 2. On-disk cache. Survives process restart; populated by prior atmos sessions.
	if diskCached, ok := p.loadFullCachedToken(); ok {
		log.Debug("Using on-disk cached SSO token", "expiresAt", diskCached.ExpiresAt)
		p.sessionStore.Put(key, diskCached)
		return ssoCacheToCredentials(diskCached, p.region), nil
	}

	// Prepare AWS config once — both the refresh path and the device-auth path need it.
	oidcClient, err := p.newOIDCClient(ctx)
	if err != nil {
		return nil, err
	}

	// 3. Refresh-token exchange. Skipped if the on-disk cache had no refresh token
	//    (e.g., written by an older atmos version, or registration expired).
	if expired, ok := p.loadExpiredCachedTokenForRefresh(); ok {
		if refreshed, rerr := tryRefreshToken(ctx, oidcClient, expired); rerr == nil {
			log.Debug("SSO token refreshed without browser interaction")
			if serr := p.saveFullCachedToken(refreshed); serr != nil {
				log.Debug("Failed to persist refreshed token", "error", serr)
			}
			p.sessionStore.Put(key, refreshed)
			return ssoCacheToCredentials(refreshed, p.region), nil
		} else {
			log.Debug("Refresh-token exchange failed; falling through to device authorization", "error", rerr)
		}
	}

	// 4. Device-authorization flow. Requires an interactive terminal.
	if !isInteractive() {
		return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanation("AWS SSO device flow requires an interactive terminal (TTY) for user authorization").
			WithHint("Use 'aws sso login' to authenticate before running Atmos in headless environments").
			WithHint("For CI/CD pipelines, use AWS environment credentials (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)").
			WithHint("For GitHub Actions, use OIDC authentication with aws/assume-role identity").
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithContext("region", p.region).
			WithExitCode(2).
			Err()
	}

	freshToken, err := p.runDeviceAuthFlow(ctx, oidcClient)
	if err != nil {
		return nil, err
	}

	if err := p.saveFullCachedToken(freshToken); err != nil {
		log.Debug("Failed to cache SSO token", "error", err)
	}
	p.sessionStore.Put(key, freshToken)
	return ssoCacheToCredentials(freshToken, p.region), nil
}

// newOIDCClient constructs an ssooidc client with an isolated AWS config.
// Extracted from the legacy linear Authenticate() body so both refresh and
// device-auth paths share one source of truth for client configuration.
func (p *ssoProvider) newOIDCClient(ctx context.Context) (*ssooidc.Client, error) {
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(p.region),
		// Disable credential providers to avoid hanging on EC2 metadata service or other credential sources.
		// SSO device flow doesn't require existing credentials.
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	}

	if resolverOpt := awsCloud.GetBaseEndpointConfigOption(nil, p.config); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	log.Debug("Loading AWS config for SSO authentication", "region", p.region)
	cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrLoadAWSConfig).
			WithExplanationf("Failed to load AWS configuration for SSO authentication in region '%s'", p.region).
			WithHint("Verify that the AWS region is valid and accessible").
			WithHint("Check your network connectivity and AWS service availability").
			WithContext("provider", p.name).
			WithContext("region", p.region).
			WithContext("start_url", p.startURL).
			WithExitCode(1).
			Err()
	}

	return ssooidc.NewFromConfig(cfg), nil
}

// runDeviceAuthFlow executes the full browser-based device-authorization flow and
// returns a complete token bundle (access token, refresh token, client credentials,
// and registration expiry) suitable for caching.
//
// The OIDC client is registered with the refresh-token grant type and the
// sso:account:access scope so that the resulting CreateToken response includes a
// refresh token — without these, refresh would not work and every expiry would
// force another browser interaction.
func (p *ssoProvider) runDeviceAuthFlow(ctx context.Context, oidcClient *ssooidc.Client) (ssoTokenCache, error) {
	log.Debug("Registering SSO client")
	registerResp, err := oidcClient.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("atmos-auth"),
		ClientType: aws.String("public"),
		// Request a refresh-token-capable registration. Without these fields the SSO
		// OIDC service returns access tokens with no refresh token, forcing a full
		// browser flow on every expiry.
		GrantTypes: []string{
			"urn:ietf:params:oauth:grant-type:device_code",
			ssoRefreshGrantType,
		},
		Scopes: []string{"sso:account:access"},
	})
	if err != nil {
		return ssoTokenCache{}, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanation("Failed to register SSO client with AWS IAM Identity Center").
			WithHint("Verify your AWS SSO configuration in atmos.yaml is correct").
			WithHintf("Ensure the start_url '%s' is valid and accessible", p.startURL).
			WithHint("Check that AWS SSO is enabled in your AWS account").
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithContext("region", p.region).
			WithExitCode(1).
			Err()
	}
	log.Debug("SSO client registered successfully")

	log.Debug("Starting device authorization")
	authResp, err := oidcClient.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     registerResp.ClientId,
		ClientSecret: registerResp.ClientSecret,
		StartUrl:     aws.String(p.startURL),
	})
	if err != nil {
		return ssoTokenCache{}, errUtils.Build(errUtils.ErrSSODeviceAuthFailed).
			WithExplanation("Failed to initiate AWS SSO device authorization flow").
			WithHint("Verify your AWS SSO session is active with 'aws sso login'").
			WithHintf("Check that the SSO start URL '%s' is correct in your atmos.yaml", p.startURL).
			WithHint("Ensure your AWS account has SSO enabled and configured").
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithContext("region", p.region).
			WithExitCode(1).
			Err()
	}
	log.Debug("Device authorization started")

	p.promptDeviceAuth(authResp)

	result, err := p.pollForAccessTokenWithSpinner(ctx, oidcClient, registerResp, authResp)
	if err != nil {
		return ssoTokenCache{}, err
	}

	if result.ExpiresAt.IsZero() {
		result.ExpiresAt = time.Now().Add(time.Duration(p.getSessionDuration()) * time.Minute)
	}
	result.Region = p.region
	result.StartURL = p.startURL
	result.ClientID = aws.ToString(registerResp.ClientId)
	result.ClientSecret = aws.ToString(registerResp.ClientSecret)
	if registerResp.ClientSecretExpiresAt != 0 {
		result.RegistrationExpiresAt = time.Unix(registerResp.ClientSecretExpiresAt, 0)
	}

	log.Debug("Authentication successful", "expiration", result.ExpiresAt)
	return result, nil
}

// ssoCacheToCredentials converts a cached SSO token to the AWSCredentials shape that
// downstream identities (permission-set, assume-role) consume. The cache's
// AccessToken is placed in AccessKeyID because identities use it as the bearer token
// for SSO API calls (sso.GetRoleCredentials).
func ssoCacheToCredentials(token ssoTokenCache, region string) *authTypes.AWSCredentials {
	return &authTypes.AWSCredentials{
		AccessKeyID: token.AccessToken,
		Region:      region,
		Expiration:  token.ExpiresAt.Format(time.RFC3339),
	}
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
		if err := browser.New().Open(*authResp.VerificationUriComplete); err != nil {
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
	utils.PrintfMessageToTUI("%s\n", boxStyle.Render(content.String()))
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
//
//nolint:dupl // SSO and SAML providers have identical path logic but are separate implementations
func (p *ssoProvider) Paths() ([]authTypes.Path, error) {
	basePath := awsCloud.GetFilesBasePath(p.config)

	// Use AWSFileManager to get correct provider-namespaced paths with realm isolation.
	fileManager, err := awsCloud.NewAWSFileManager(basePath, p.realm)
	if err != nil {
		return nil, err
	}

	paths := []authTypes.Path{
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
	}

	// Add AWS cache directory if it can be determined (contains SSO and CLI cache).
	// This directory should be writable so the AWS SDK can update cache.
	awsCacheDir := fileManager.GetCachePath()
	if awsCacheDir != "" {
		paths = append(paths, authTypes.Path{
			Location: awsCacheDir,
			Type:     authTypes.PathTypeDirectory,
			Required: false, // Cache is optional - AWS SDK will create if needed.
			Purpose:  "AWS SDK cache directory (SSO tokens, CLI cache)",
			Metadata: map[string]string{
				"read_only": "false", // Cache must be writable.
			},
		})
	}

	return paths, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For SSO providers, this method is typically not called directly since SSO providers
// authenticate to get identity credentials, which then have their own PrepareEnvironment.
// However, we implement it for interface compliance and inject AWS_REGION.
func (p *ssoProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.ssoProvider.PrepareEnvironment")()

	// Create a copy to avoid modifying the input map.
	result := make(map[string]string, len(environ))
	for k, v := range environ {
		result[k] = v
	}

	// Inject provider-specific environment variables (AWS_REGION).
	// SSO provider doesn't write credential files itself - that's done by identities.
	providerEnv, err := p.Environment()
	if err != nil {
		return nil, err
	}
	for k, v := range providerEnv {
		result[k] = v
	}

	return result, nil
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
//
// Returns an ssoTokenCache populated with AccessToken, RefreshToken, and ExpiresAt
// from the device-auth flow; other fields (StartURL, Region, ClientID, ClientSecret,
// RegistrationExpiresAt) are added by the caller.
func (p *ssoProvider) pollForAccessTokenWithSpinner(ctx context.Context, oidcClient *ssooidc.Client, registerResp *ssooidc.RegisterClientOutput, authResp *ssooidc.StartDeviceAuthorizationOutput) (ssoTokenCache, error) {
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
		token, err := p.pollForAccessToken(ctx, oidcClient, registerResp, authResp)
		resultChan <- pollResult{token: token, err: err}
	}()

	// Create and run the spinner.
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	fps.Apply(&s)

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
		return res.token, res.err
	}

	// Get the result from the final model.
	finalSpinner := finalModel.(spinnerModel)
	if finalSpinner.result == nil {
		return ssoTokenCache{}, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanation("AWS SSO authentication did not complete").
			WithHint("The authentication flow was interrupted unexpectedly").
			WithHint("Try running the authentication again").
			WithContext("provider", p.name).
			WithExitCode(1).
			Err()
	}
	if finalSpinner.result.err != nil {
		return ssoTokenCache{}, finalSpinner.result.err
	}
	return finalSpinner.result.token, nil
}

// pollResult carries a complete (or partial-on-error) SSO token bundle from the
// polling goroutine back to the bubbletea spinner model.
type pollResult struct {
	token ssoTokenCache
	err   error
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

// pollForAccessToken polls the device authorization endpoint until an access token is
// available or times out. The returned ssoTokenCache carries AccessToken, RefreshToken
// (when issued), and ExpiresAt; remaining fields are populated by runDeviceAuthFlow.
func (p *ssoProvider) pollForAccessToken(ctx context.Context, oidcClient *ssooidc.Client, registerResp *ssooidc.RegisterClientOutput, authResp *ssooidc.StartDeviceAuthorizationOutput) (ssoTokenCache, error) {
	var result ssoTokenCache
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
			result.AccessToken = aws.ToString(tokenResp.AccessToken)
			result.RefreshToken = aws.ToString(tokenResp.RefreshToken)
			result.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
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

		return ssoTokenCache{}, errUtils.Build(errUtils.ErrSSOTokenCreationFailed).
			WithExplanation("Failed to create AWS SSO access token").
			WithHint("Ensure you completed the device authorization in your browser").
			WithHint("The verification code may have expired - try authenticating again").
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithExitCode(1).
			Err()
	}

	if result.AccessToken == "" {
		return ssoTokenCache{}, errUtils.Build(errUtils.ErrSSOTokenCreationFailed).
			WithExplanation("AWS SSO authentication timed out waiting for browser confirmation").
			WithHint("Complete the device authorization in your browser within the time limit").
			WithHint("Visit the verification URL and enter the code displayed earlier").
			WithHint("Try running 'aws sso login' to verify your SSO configuration").
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithContext("region", p.region).
			WithExitCode(1).
			Err()
	}
	return result, nil
}

// Logout removes provider-specific credential storage.
func (p *ssoProvider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.ssoProvider.Logout")()

	// Drop the in-memory session entry so a subsequent login can't short-circuit.
	// Note: this affects all providers sharing this portal — matching `aws sso logout`
	// semantics where logout is portal-scoped, not profile-scoped.
	if p.sessionStore != nil {
		p.sessionStore.Forget(p.sessionKey())
	}

	// Delete cached SSO token (non-fatal if fails).
	if err := p.deleteCachedToken(); err != nil {
		log.Debug("Failed to delete cached SSO token", "error", err)
	}

	// Get base_path from provider spec if configured.
	basePath := awsCloud.GetFilesBasePath(p.config)

	// Use realm for credential isolation between different repositories.
	fileManager, err := awsCloud.NewAWSFileManager(basePath, p.realm)
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

	// Use realm for credential isolation between different repositories.
	fileManager, err := awsCloud.NewAWSFileManager(basePath, p.realm)
	if err != nil {
		return "~/.aws/atmos"
	}

	return fileManager.GetDisplayPath()
}
