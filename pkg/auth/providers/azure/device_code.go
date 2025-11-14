package azure

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
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

// createMSALClient creates a MSAL public client with persistent token cache.
// The cache is stored in ~/.azure/msal_token_cache.json for Azure CLI compatibility.
func (p *deviceCodeProvider) createMSALClient() (public.Client, error) {
	// Create MSAL cache for token persistence.
	msalCache, err := azureCloud.NewMSALCache("")
	if err != nil {
		return public.Client{}, fmt.Errorf("failed to create MSAL cache: %w", err)
	}

	// Create MSAL public client with cache.
	// This client will automatically persist refresh tokens.
	client, err := public.New(
		p.clientID,
		public.WithAuthority(fmt.Sprintf("https://login.microsoftonline.com/%s", p.tenantID)),
		public.WithCache(msalCache),
	)
	if err != nil {
		return public.Client{}, fmt.Errorf("failed to create MSAL client: %w", err)
	}

	log.Debug("Created MSAL client",
		"clientID", p.clientID,
		azureCloud.LogFieldTenantID, p.tenantID)

	return client, nil
}

// acquireTokenByDeviceCode performs device code authentication flow using MSAL.
// It displays the device code to the user and waits for authentication to complete.
func (p *deviceCodeProvider) acquireTokenByDeviceCode(ctx context.Context, client *public.Client, scopes []string) (string, time.Time, error) {
	// Create a context with timeout.
	authCtx, cancel := context.WithTimeout(ctx, deviceCodeTimeout)
	defer cancel()

	// Start device code flow.
	deviceCode, err := client.AcquireTokenByDeviceCode(authCtx, scopes)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to start device code flow: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Display device code to user.
	displayDeviceCodePrompt(deviceCode.Result.UserCode, deviceCode.Result.VerificationURL)

	// If not a TTY or in CI, use simple polling without spinner.
	if !isTTY() || telemetry.IsCI() {
		result, err := deviceCode.AuthenticationResult(authCtx)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("%w: device code authentication failed: %w", errUtils.ErrAuthenticationFailed, err)
		}
		return result.AccessToken, result.ExpiresOn, nil
	}

	// Use spinner for interactive terminals.
	return waitForAuthWithSpinner(authCtx, &deviceCode)
}

// waitForAuthWithSpinner waits for device code authentication with a spinner UI.
func waitForAuthWithSpinner(authCtx context.Context, deviceCode *public.DeviceCode) (string, time.Time, error) {
	resultCh := make(chan struct {
		token     string
		expiresOn time.Time
		err       error
	}, 1)

	// Start authentication in background.
	go func() {
		result, err := deviceCode.AuthenticationResult(authCtx)
		if err != nil {
			resultCh <- struct {
				token     string
				expiresOn time.Time
				err       error
			}{"", time.Time{}, err}
			return
		}
		resultCh <- struct {
			token     string
			expiresOn time.Time
			err       error
		}{result.AccessToken, result.ExpiresOn, nil}
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

	m := finalModel.(*spinnerModel)
	if m.authErr != nil {
		return "", time.Time{}, m.authErr
	}

	return m.token, m.expiresOn, nil
}

// findAccountForTenant finds the account that matches the configured tenant ID.
// Returns the matching account or an error if no match is found.
func (p *deviceCodeProvider) findAccountForTenant(accounts []public.Account) (public.Account, error) {
	if len(accounts) == 0 {
		return public.Account{}, errUtils.ErrAzureNoAccountsInCache
	}

	// Try to find account matching the tenant ID.
	for i := range accounts {
		// Match by tenant ID in the home account ID (format: objectId.tenantId).
		if accounts[i].Realm == p.tenantID {
			log.Debug("Found account matching tenant ID",
				"username", accounts[i].PreferredUsername,
				azureCloud.LogFieldTenantID, p.tenantID)
			return accounts[i], nil
		}
	}

	return public.Account{}, fmt.Errorf("%w: %s", errUtils.ErrAzureNoAccountForTenant, p.tenantID)
}

// displayDeviceCodePrompt displays the device code and verification URL to the user.
func displayDeviceCodePrompt(userCode, verificationURL string) {
	log.Debug("Displaying Azure authentication prompt",
		"url", verificationURL,
		"code", userCode,
		"isCI", telemetry.IsCI(),
	)

	// Check if we have a TTY for fancy output.
	if isTTY() && !telemetry.IsCI() {
		displayVerificationDialog(userCode, verificationURL)
	} else {
		// Fallback to simple text output for non-TTY or CI environments.
		displayVerificationPlainText(userCode, verificationURL)
	}

	// Open browser if not in CI.
	// Azure supports pre-filling the code with ?otc=CODE parameter.
	if !telemetry.IsCI() && verificationURL != "" {
		urlToOpen := fmt.Sprintf("%s?otc=%s", verificationURL, userCode)
		if err := utils.OpenUrl(urlToOpen); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		} else {
			log.Debug("Browser opened successfully", "url", urlToOpen)
		}
	}
	log.Debug("Finished displaying device code prompt, waiting for user authentication")
}

// Authenticate performs Azure device code authentication using MSAL.
// This implementation uses MSAL directly to enable refresh token persistence,
// making it a true drop-in replacement for `az login`.
func (p *deviceCodeProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.deviceCodeProvider.Authenticate")()

	// Create MSAL client with persistent cache.
	// This client automatically manages token caching and refresh tokens.
	client, err := p.createMSALClient()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create MSAL client: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Try silent authentication first (uses cached tokens/refresh tokens).
	accounts, err := client.Accounts(ctx)
	if err != nil {
		log.Debug("Failed to get cached accounts, will proceed with device code flow", "error", err)
	}

	// Try silent token acquisition from cached account.
	tokens := p.trySilentTokenAcquisition(ctx, &client, accounts)

	// If silent acquisition failed, use device code flow.
	if tokens.accessToken == "" {
		tokens, err = p.acquireTokensViaDeviceCode(ctx, &client)
		if err != nil {
			return nil, err
		}
	}

	// Update Azure CLI token cache so Terraform can use it automatically.
	// This makes Atmos auth work exactly like `az login`.
	// Note: MSAL already persisted tokens (including refresh tokens) to ~/.azure/msal_token_cache.json.
	if err := p.updateAzureCLICache(tokenCacheUpdate{
		AccessToken:       tokens.accessToken,
		ExpiresAt:         tokens.expiresOn,
		GraphToken:        tokens.graphToken,
		GraphExpiresAt:    tokens.graphExpiresOn,
		KeyVaultToken:     tokens.keyVaultToken,
		KeyVaultExpiresAt: tokens.keyVaultExpiresOn,
	}); err != nil {
		log.Debug("Failed to update Azure CLI token cache", "error", err)
	}

	return p.createCredentials(tokens)
}

// tokenAcquisitionResult holds tokens acquired from Azure.
type tokenAcquisitionResult struct {
	accessToken       string
	graphToken        string
	keyVaultToken     string
	expiresOn         time.Time
	graphExpiresOn    time.Time
	keyVaultExpiresOn time.Time
}

// trySilentTokenAcquisition attempts to acquire tokens silently from cached account.
func (p *deviceCodeProvider) trySilentTokenAcquisition(ctx context.Context, client *public.Client, accounts []public.Account) tokenAcquisitionResult {
	result := tokenAcquisitionResult{}

	if len(accounts) == 0 {
		return result
	}

	// Find the account that matches our configured tenant ID.
	account, err := p.findAccountForTenant(accounts)
	if err != nil {
		log.Debug("No matching account found for tenant, will proceed with device code flow",
			azureCloud.LogFieldTenantID, p.tenantID,
			"error", err)
		return result
	}

	log.Debug("Found cached account, attempting silent token acquisition",
		"account", account.PreferredUsername,
		azureCloud.LogFieldTenantID, p.tenantID)

	// Try to get management token silently.
	mgmtResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://management.azure.com/.default"},
		public.WithSilentAccount(account),
	)
	if err != nil {
		log.Debug("Silent token acquisition failed, will proceed with device code flow", "error", err)
		return result
	}

	result.accessToken = mgmtResult.AccessToken
	result.expiresOn = mgmtResult.ExpiresOn
	log.Debug("Successfully acquired management token silently", "expiresOn", result.expiresOn)

	// Try to get Graph token silently.
	graphResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://graph.microsoft.com/.default"},
		public.WithSilentAccount(account),
	)
	if err == nil {
		result.graphToken = graphResult.AccessToken
		result.graphExpiresOn = graphResult.ExpiresOn
		log.Debug("Successfully acquired Graph token silently", azureCloud.LogFieldExpiresOn, result.graphExpiresOn)
	} else {
		log.Debug("Failed to get Graph token silently, will skip", "error", err)
	}

	// Try to get KeyVault token silently.
	kvResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://vault.azure.net/.default"},
		public.WithSilentAccount(account),
	)
	if err == nil {
		result.keyVaultToken = kvResult.AccessToken
		result.keyVaultExpiresOn = kvResult.ExpiresOn
		log.Debug("Successfully acquired KeyVault token silently", "expiresOn", result.keyVaultExpiresOn)
	} else {
		log.Debug("Failed to get KeyVault token silently, will skip", "error", err)
	}

	return result
}

// acquireTokensViaDeviceCode performs device code flow and acquires additional tokens.
func (p *deviceCodeProvider) acquireTokensViaDeviceCode(ctx context.Context, client *public.Client) (tokenAcquisitionResult, error) {
	result := tokenAcquisitionResult{}

	// Check if we're in a headless environment - device code flow requires user interaction.
	if !isInteractive() {
		return result, fmt.Errorf("%w: Azure device code flow requires an interactive terminal (no TTY detected). Use managed identity or service principal authentication in headless environments", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("Starting Azure device code authentication",
		"provider", p.name,
		"tenant", p.tenantID,
		"clientID", p.clientID,
	)

	// Start device code flow for management scope.
	accessToken, expiresOn, err := p.acquireTokenByDeviceCode(ctx, client,
		[]string{"https://management.azure.com/.default"})
	if err != nil {
		return result, err
	}

	result.accessToken = accessToken
	result.expiresOn = expiresOn
	log.Debug("Authentication successful", "expiration", expiresOn)

	// Get the authenticated account for subsequent silent acquisitions.
	accounts, err := client.Accounts(ctx)
	if err != nil || len(accounts) == 0 {
		log.Debug("Failed to get authenticated account, will skip Graph and KeyVault tokens", "error", err)
		return result, nil
	}

	// Find the account that matches our tenant ID.
	account, err := p.findAccountForTenant(accounts)
	if err != nil {
		log.Debug("No matching account found after device code authentication, will skip Graph and KeyVault tokens",
			azureCloud.LogFieldTenantID, p.tenantID,
			"error", err)
		return result, nil
	}

	// Request Graph API token for azuread provider (silently, using refresh token).
	log.Debug("Requesting Graph API token for azuread provider")
	graphResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://graph.microsoft.com/.default"},
		public.WithSilentAccount(account),
	)
	if err != nil {
		log.Debug("Failed to get Graph API token, azuread provider may not work", "error", err)
	} else {
		result.graphToken = graphResult.AccessToken
		result.graphExpiresOn = graphResult.ExpiresOn
		log.Debug("Successfully obtained Graph API token",
			azureCloud.LogFieldExpiresOn, result.graphExpiresOn,
			"tokenLength", len(result.graphToken))
	}

	// Request KeyVault token for azurerm provider KeyVault operations (silently).
	log.Debug("Requesting KeyVault token for azurerm provider")
	kvResult, err := client.AcquireTokenSilent(ctx,
		[]string{"https://vault.azure.net/.default"},
		public.WithSilentAccount(account),
	)
	if err != nil {
		log.Debug("Failed to get KeyVault token, KeyVault operations may not work", "error", err)
	} else {
		result.keyVaultToken = kvResult.AccessToken
		result.keyVaultExpiresOn = kvResult.ExpiresOn
		log.Debug("Successfully obtained KeyVault token",
			"expiresOn", result.keyVaultExpiresOn,
			"tokenLength", len(result.keyVaultToken))
	}

	return result, nil
}

// createCredentials creates Azure credentials from acquired tokens.
func (p *deviceCodeProvider) createCredentials(tokens tokenAcquisitionResult) (authTypes.ICredentials, error) {
	creds := &authTypes.AzureCredentials{
		AccessToken:    tokens.accessToken,
		TokenType:      "Bearer",
		Expiration:     tokens.expiresOn.Format(time.RFC3339),
		TenantID:       p.tenantID,
		SubscriptionID: p.subscriptionID,
		Location:       p.location,
	}

	// Add Graph API token if available.
	if tokens.graphToken != "" {
		creds.GraphAPIToken = tokens.graphToken
		creds.GraphAPIExpiration = tokens.graphExpiresOn.Format(time.RFC3339)
		log.Debug("Added Graph API token to credentials",
			"graphTokenLength", len(tokens.graphToken),
			"graphExpiration", tokens.graphExpiresOn.Format(time.RFC3339))
	} else {
		log.Debug("Graph API token is empty, not adding to credentials")
	}

	// Add KeyVault API token if available.
	if tokens.keyVaultToken != "" {
		creds.KeyVaultToken = tokens.keyVaultToken
		creds.KeyVaultExpiration = tokens.keyVaultExpiresOn.Format(time.RFC3339)
		log.Debug("Added KeyVault API token to credentials",
			"keyVaultTokenLength", len(tokens.keyVaultToken),
			"keyVaultExpiration", tokens.keyVaultExpiresOn.Format(time.RFC3339))
	} else {
		log.Debug("KeyVault API token is empty, not adding to credentials")
	}

	return creds, nil
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

// Validate checks the provider configuration and returns an error if required fields
// (tenant_id, client_id) are missing or invalid.
func (p *deviceCodeProvider) Validate() error {
	if p.tenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns Azure-specific environment variables for this provider,
// including AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID, and AZURE_LOCATION if configured.
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

// PrepareEnvironment prepares environment variables for external processes (Terraform, etc.)
// by merging provider configuration with the base environment and setting Azure-specific variables
// (ARM_TENANT_ID, ARM_SUBSCRIPTION_ID, ARM_USE_OIDC). Returns the prepared environment map and error.
// Note: access token is set later by SetEnvironmentVariables which loads from credential store.
func (p *deviceCodeProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	// Use shared Azure environment preparation.
	return azureCloud.PrepareEnvironment(azureCloud.PrepareEnvironmentConfig{
		Environ:        environ,
		SubscriptionID: p.subscriptionID,
		TenantID:       p.tenantID,
		Location:       p.location,
	}), nil
}

// Logout removes cached device code tokens from disk by deleting the MSAL token cache file.
// Returns an error if the cache deletion fails.
func (p *deviceCodeProvider) Logout(ctx context.Context) error {
	log.Debug("Logout Azure device code provider", "provider", p.name)
	return p.deleteCachedToken()
}

// GetFilesDisplayPath returns the user-facing display path for credential files
// stored by this provider (e.g., "~/.azure/atmos/provider-name").
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

func newSpinnerModel() *spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	return &spinnerModel{spinner: s}
}

func (m *spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m *spinnerModel) View() string {
	if m.quitting {
		if m.authErr != nil {
			return ""
		}
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
		return successStyle.Render("✓") + " Authentication successful!\n"
	}
	return m.spinner.View() + " Waiting for authentication...\n"
}
