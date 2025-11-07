package azure

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	// Default client ID for Atmos Azure authentication (Azure CLI public client).
	defaultAzureClientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"

	// Default timeout for device code authentication.
	deviceCodeTimeout = 15 * time.Minute
)

// isInteractive checks if we're running in an interactive terminal.
// For device code flow, we need stderr to be a TTY so the user can see the authentication URL.
func isInteractive() bool {
	return term.IsTTYSupportForStderr()
}

// deviceCodeProvider implements Azure Entra ID device code authentication.
type deviceCodeProvider struct {
	name           string
	config         *schema.Provider
	tenantID       string
	subscriptionID string
	location       string
	clientID       string
	cacheStorage   CacheStorage
}

// NewDeviceCodeProvider creates a new Azure device code provider.
func NewDeviceCodeProvider(name string, config *schema.Provider) (*deviceCodeProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "azure/device-code" {
		return nil, fmt.Errorf("%w: invalid provider kind for Azure device code provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract Azure-specific config from Spec.
	tenantID := ""
	subscriptionID := ""
	location := ""
	clientID := defaultAzureClientID

	if config.Spec != nil {
		if tid, ok := config.Spec["tenant_id"].(string); ok {
			tenantID = tid
		}
		if sid, ok := config.Spec["subscription_id"].(string); ok {
			subscriptionID = sid
		}
		if loc, ok := config.Spec["location"].(string); ok {
			location = loc
		}
		if cid, ok := config.Spec["client_id"].(string); ok && cid != "" {
			clientID = cid
		}
	}

	// Tenant ID is required.
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required in spec for Azure device code provider", errUtils.ErrInvalidProviderConfig)
	}

	return &deviceCodeProvider{
		name:           name,
		config:         config,
		tenantID:       tenantID,
		subscriptionID: subscriptionID,
		location:       location,
		clientID:       clientID,
		cacheStorage:   &defaultCacheStorage{},
	}, nil
}

// Kind returns the provider kind.
func (p *deviceCodeProvider) Kind() string {
	return "azure/device-code"
}

// Name returns the configured provider name.
func (p *deviceCodeProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for device code provider.
func (p *deviceCodeProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// Authenticate performs Azure device code authentication.
func (p *deviceCodeProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.deviceCodeProvider.Authenticate")()

	// Check for cached token first to avoid unnecessary device authorization.
	cachedToken, cachedExpiry, cachedGraphToken, cachedGraphExpiry, err := p.loadCachedToken()
	if err != nil {
		log.Debug("Error loading cached token, will proceed with device authorization", "error", err)
	}
	if cachedToken != "" {
		log.Debug("Using cached Azure device code token", "expiresAt", cachedExpiry)

		// Update Azure CLI files even when using cached credentials.
		// This ensures Terraform providers can always find the credentials.
		// Note: KeyVault tokens are obtained fresh each time, not cached in device code cache.
		if err := p.updateAzureCLICache(cachedToken, cachedExpiry, cachedGraphToken, cachedGraphExpiry, "", time.Time{}); err != nil {
			log.Debug("Failed to update Azure CLI cache from cached credentials", "error", err)
		}

		creds := &authTypes.AzureCredentials{
			AccessToken:    cachedToken,
			TokenType:      "Bearer",
			Expiration:     cachedExpiry.Format(time.RFC3339),
			TenantID:       p.tenantID,
			SubscriptionID: p.subscriptionID,
			Location:       p.location,
		}

		// Add Graph API token if available from cache.
		if cachedGraphToken != "" {
			creds.GraphAPIToken = cachedGraphToken
			creds.GraphAPIExpiration = cachedGraphExpiry.Format(time.RFC3339)
		}

		return creds, nil
	}

	// No valid cached token, proceed with device authorization flow.

	// Check if we're in a headless environment - device code flow requires user interaction.
	if !isInteractive() {
		return nil, fmt.Errorf("%w: Azure device code flow requires an interactive terminal (no TTY detected). Use managed identity or service principal authentication in headless environments", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("Starting Azure device code authentication",
		"provider", p.name,
		"tenant", p.tenantID,
		"clientID", p.clientID,
	)

	// Create device code credential with user prompt callback.
	var userCode string
	var verificationURL string

	cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
		TenantID: p.tenantID,
		ClientID: p.clientID,
		UserPrompt: func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
			userCode = msg.UserCode
			verificationURL = msg.VerificationURL
			p.promptDeviceAuth(msg)
			return nil
		},
		ClientOptions: policy.ClientOptions{
			Telemetry: policy.TelemetryOptions{
				ApplicationID: "atmos-auth",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create Azure device code credential: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Get token with spinner (if TTY).
	accessToken, expiresOn, err := p.getTokenWithSpinner(ctx, cred, userCode, verificationURL)
	if err != nil {
		return nil, err
	}

	log.Debug("Authentication successful", "expiration", expiresOn)

	// Request Graph API token for azuread provider.
	// We do this after device code authentication completes, using the same credential.
	log.Debug("Requesting Graph API token for azuread provider")
	graphToken, graphExpiresOn, err := p.getGraphAPIToken(ctx, cred)
	if err != nil {
		log.Debug("Failed to get Graph API token, azuread provider may not work", "error", err)
		// Non-fatal - azurerm backend will still work.
		// Set empty values for Graph API token.
		graphToken = ""
		graphExpiresOn = time.Time{}
	} else {
		log.Debug("Successfully obtained Graph API token",
			"expiresOn", graphExpiresOn,
			"tokenLength", len(graphToken))
	}

	// Request KeyVault token for azurerm provider KeyVault operations.
	// We do this after device code authentication completes, using the same credential.
	log.Debug("Requesting KeyVault token for azurerm provider")
	keyVaultToken, keyVaultExpiresOn, err := p.getKeyVaultToken(ctx, cred)
	if err != nil {
		log.Debug("Failed to get KeyVault token, KeyVault operations may not work", "error", err)
		// Non-fatal - other operations will still work.
		// Set empty values for KeyVault token.
		keyVaultToken = ""
		keyVaultExpiresOn = time.Time{}
	} else {
		log.Debug("Successfully obtained KeyVault token",
			"expiresOn", keyVaultExpiresOn,
			"tokenLength", len(keyVaultToken))
	}

	// Save all tokens to cache for future use (non-fatal if fails).
	if err := p.saveCachedToken(accessToken, "Bearer", expiresOn, graphToken, graphExpiresOn); err != nil {
		log.Debug("Failed to cache Azure device code token", "error", err)
	}

	// Update Azure CLI token cache so Terraform can use it automatically.
	// This makes Atmos auth work exactly like `az login`.
	if err := p.updateAzureCLICache(accessToken, expiresOn, graphToken, graphExpiresOn, keyVaultToken, keyVaultExpiresOn); err != nil {
		log.Debug("Failed to update Azure CLI token cache", "error", err)
	}

	creds := &authTypes.AzureCredentials{
		AccessToken:    accessToken,
		TokenType:      "Bearer",
		Expiration:     expiresOn.Format(time.RFC3339),
		TenantID:       p.tenantID,
		SubscriptionID: p.subscriptionID,
		Location:       p.location,
	}

	// Add Graph API token if available.
	if graphToken != "" {
		creds.GraphAPIToken = graphToken
		creds.GraphAPIExpiration = graphExpiresOn.Format(time.RFC3339)
		log.Debug("Added Graph API token to credentials",
			"graphTokenLength", len(graphToken),
			"graphExpiration", graphExpiresOn.Format(time.RFC3339))
	} else {
		log.Debug("Graph API token is empty, not adding to credentials")
	}

	// Add KeyVault API token if available.
	if keyVaultToken != "" {
		creds.KeyVaultToken = keyVaultToken
		creds.KeyVaultExpiration = keyVaultExpiresOn.Format(time.RFC3339)
		log.Debug("Added KeyVault API token to credentials",
			"keyVaultTokenLength", len(keyVaultToken),
			"keyVaultExpiration", keyVaultExpiresOn.Format(time.RFC3339))
	} else {
		log.Debug("KeyVault API token is empty, not adding to credentials")
	}

	return creds, nil
}

// promptDeviceAuth displays user code and verification URI.
func (p *deviceCodeProvider) promptDeviceAuth(msg azidentity.DeviceCodeMessage) {
	log.Debug("Displaying Azure authentication prompt",
		"url", msg.VerificationURL,
		"code", msg.UserCode,
		"isCI", telemetry.IsCI(),
	)

	// Check if we have a TTY for fancy output.
	if isTTY() && !telemetry.IsCI() {
		displayVerificationDialog(msg.UserCode, msg.VerificationURL)
	} else {
		// Fallback to simple text output for non-TTY or CI environments.
		displayVerificationPlainText(msg.UserCode, msg.VerificationURL)
	}

	// Open browser if not in CI.
	// Use verification_uri_complete if available, otherwise build URL with code parameter.
	if !telemetry.IsCI() && msg.VerificationURL != "" {
		// Azure supports pre-filling the code with ?otc=CODE parameter.
		urlToOpen := fmt.Sprintf("%s?otc=%s", msg.VerificationURL, msg.UserCode)
		if err := utils.OpenUrl(urlToOpen); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		} else {
			log.Debug("Browser opened successfully", "url", urlToOpen)
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
	// Simpler, clearer output without complex borders.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan))

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray))

	codeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorGreen)).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(0, 2)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorBlue))

	// Build simple, readable output.
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, titleStyle.Render("🔐 Azure Authentication Required"))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s  %s\n", labelStyle.Render("Verification Code:"), codeStyle.Render(code))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s  %s\n", labelStyle.Render("Verification URL:"), urlStyle.Render(url))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, labelStyle.Render("Opening browser..."))
	fmt.Fprintln(os.Stderr)
}

// displayVerificationPlainText shows plain text authentication prompt.
func displayVerificationPlainText(code, url string) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "🔐 Azure Authentication Required")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Verification Code: %s\n", code)
	fmt.Fprintf(os.Stderr, "Verification URL:  %s\n", url)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Please open the URL above and enter the verification code to authenticate.")
	fmt.Fprintln(os.Stderr, "")
}

// getTokenWithSpinner polls for the access token with a spinner UI.
func (p *deviceCodeProvider) getTokenWithSpinner(ctx context.Context, cred *azidentity.DeviceCodeCredential, userCode, verificationURL string) (string, time.Time, error) {
	// Create a context with timeout.
	authCtx, cancel := context.WithTimeout(ctx, deviceCodeTimeout)
	defer cancel()

	// If not a TTY or in CI, use simple polling without spinner.
	if !isTTY() || telemetry.IsCI() {
		return p.pollForAccessToken(authCtx, cred)
	}

	// Use spinner for interactive terminals.
	resultCh := make(chan struct {
		token     string
		expiresOn time.Time
		err       error
	}, 1)

	// Start authentication in background.
	go func() {
		token, expiresOn, err := p.pollForAccessToken(authCtx, cred)
		resultCh <- struct {
			token     string
			expiresOn time.Time
			err       error
		}{token, expiresOn, err}
	}()

	// Run spinner.
	model := newSpinnerModel()
	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	go func() {
		result := <-resultCh
		prog.Send(authCompleteMsg{
			token:     result.token,
			expiresOn: result.expiresOn,
			err:       result.err,
		})
	}()

	finalModel, err := prog.Run()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to run spinner: %w", errUtils.ErrAuthenticationFailed, err)
	}

	m := finalModel.(spinnerModel)
	if m.authErr != nil {
		return "", time.Time{}, m.authErr
	}

	return m.token, m.expiresOn, nil
}

// pollForAccessToken polls Azure for the access token.
func (p *deviceCodeProvider) pollForAccessToken(ctx context.Context, cred *azidentity.DeviceCodeCredential) (string, time.Time, error) {
	log.Debug("Polling for Azure access token")

	// Request token with Azure Resource Manager scope.
	// This gives us access to manage Azure resources.
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to get Azure access token: %w", errUtils.ErrAuthenticationFailed, err)
	}

	log.Debug("Successfully obtained Azure access token", "expiresOn", token.ExpiresOn)
	return token.Token, token.ExpiresOn, nil
}

// getGraphAPIToken requests a Graph API token for azuread provider.
// This is called after device code authentication completes.
func (p *deviceCodeProvider) getGraphAPIToken(ctx context.Context, cred *azidentity.DeviceCodeCredential) (string, time.Time, error) {
	log.Debug("Requesting Graph API token for azuread provider")

	// Request token with Microsoft Graph API scope.
	// This gives us access to Azure AD operations.
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://graph.microsoft.com/.default"},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get Graph API token: %w", err)
	}

	log.Debug("Successfully obtained Graph API token", "expiresOn", token.ExpiresOn)
	return token.Token, token.ExpiresOn, nil
}

// getKeyVaultToken requests a KeyVault token for azurerm provider KeyVault operations.
// This is called after device code authentication completes.
func (p *deviceCodeProvider) getKeyVaultToken(ctx context.Context, cred *azidentity.DeviceCodeCredential) (string, time.Time, error) {
	log.Debug("Requesting KeyVault token for azurerm provider")

	// Request token with Azure KeyVault scope.
	// This gives us access to KeyVault operations.
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://vault.azure.net/.default"},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get KeyVault token: %w", err)
	}

	log.Debug("Successfully obtained KeyVault token", "expiresOn", token.ExpiresOn)
	return token.Token, token.ExpiresOn, nil
}

// Validate validates the provider configuration.
func (p *deviceCodeProvider) Validate() error {
	if p.tenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns environment variables for this provider.
func (p *deviceCodeProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if p.tenantID != "" {
		env["AZURE_TENANT_ID"] = p.tenantID
	}
	if p.subscriptionID != "" {
		env["AZURE_SUBSCRIPTION_ID"] = p.subscriptionID
	}
	if p.location != "" {
		env["AZURE_LOCATION"] = p.location
	}
	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
func (p *deviceCodeProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	// Use shared Azure environment preparation.
	// Note: access token is set later by SetEnvironmentVariables which loads from credential store.
	return azureCloud.PrepareEnvironment(
		environ,
		p.subscriptionID,
		p.tenantID,
		p.location,
		"",  // Credentials file path set by identity.
		"",  // Access token loaded from credential store by SetEnvironmentVariables.
	), nil
}

// Logout removes cached device code tokens.
func (p *deviceCodeProvider) Logout(ctx context.Context) error {
	log.Debug("Logout Azure device code provider", "provider", p.name)
	return p.deleteCachedToken()
}

// GetFilesDisplayPath returns the display path for credential files.
func (p *deviceCodeProvider) GetFilesDisplayPath() string {
	return "~/.azure/atmos/" + p.name
}

// Spinner model for authentication polling.

type authCompleteMsg struct {
	token     string
	expiresOn time.Time
	err       error
}

type spinnerModel struct {
	spinner   spinner.Model
	token     string
	expiresOn time.Time
	authErr   error
	quitting  bool
}

func newSpinnerModel() spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	return spinnerModel{spinner: s}
}

func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case authCompleteMsg:
		m.token = msg.token
		m.expiresOn = msg.expiresOn
		m.authErr = msg.err
		m.quitting = true
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			m.authErr = fmt.Errorf("%w: authentication cancelled by user", errUtils.ErrAuthenticationFailed)
			return m, tea.Quit
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m spinnerModel) View() string {
	if m.quitting {
		if m.authErr != nil {
			return ""
		}
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
		return successStyle.Render("✓") + " Authentication successful!\n"
	}
	return m.spinner.View() + " Waiting for authentication...\n"
}
