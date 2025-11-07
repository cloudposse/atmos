package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cliProvider implements Azure CLI-based authentication.
// This provider uses `az account get-access-token` to obtain Azure credentials.
type cliProvider struct {
	name           string
	config         *schema.Provider
	tenantID       string
	subscriptionID string
	location       string
}

// azureCliTokenResponse represents the response from `az account get-access-token`.
type azureCliTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	ExpiresOn    string `json:"expiresOn"`    // ISO 8601 format
	Tenant       string `json:"tenant"`       // Tenant ID
	Subscription string `json:"subscription"` // Subscription ID
	TokenType    string `json:"tokenType"`    // Usually "Bearer"
}

// NewCLIProvider creates a new Azure CLI provider.
func NewCLIProvider(name string, config *schema.Provider) (*cliProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "azure/cli" {
		return nil, fmt.Errorf("%w: invalid provider kind for Azure CLI provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract Azure-specific config from Spec.
	tenantID := ""
	subscriptionID := ""
	location := ""

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
	}

	// Tenant ID is required.
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required in spec for Azure CLI provider", errUtils.ErrInvalidProviderConfig)
	}

	return &cliProvider{
		name:           name,
		config:         config,
		tenantID:       tenantID,
		subscriptionID: subscriptionID,
		location:       location,
	}, nil
}

// Kind returns the provider kind.
func (p *cliProvider) Kind() string {
	return "azure/cli"
}

// Name returns the configured provider name.
func (p *cliProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for Azure CLI provider.
func (p *cliProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// Authenticate performs Azure CLI authentication.
func (p *cliProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.cliProvider.Authenticate")()

	log.Debug("Authenticating with Azure CLI",
		"provider", p.name,
		"tenant", p.tenantID,
	)

	// Build az command args.
	args := []string{"account", "get-access-token", "--tenant", p.tenantID}
	if p.subscriptionID != "" {
		args = append(args, "--subscription", p.subscriptionID)
	}
	args = append(args, "--output", "json")

	// Execute az command.
	cmd := exec.CommandContext(ctx, "az", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		if strings.Contains(outputStr, "az login") {
			return nil, fmt.Errorf("%w: Azure CLI not logged in. Run 'az login' first: %s", errUtils.ErrAuthenticationFailed, outputStr)
		}
		return nil, fmt.Errorf("%w: failed to get Azure CLI access token: %s", errUtils.ErrAuthenticationFailed, outputStr)
	}

	// Parse response.
	var tokenResp azureCliTokenResponse
	if err := json.Unmarshal(output, &tokenResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse Azure CLI token response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Parse expiration time (Azure CLI returns ISO 8601 format).
	expiresOn, err := time.Parse(time.RFC3339, tokenResp.ExpiresOn)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse Azure CLI token expiration: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	// Use subscription from response if not configured.
	subscriptionID := p.subscriptionID
	if subscriptionID == "" && tokenResp.Subscription != "" {
		subscriptionID = tokenResp.Subscription
	}

	// Create Azure credentials.
	creds := &authTypes.AzureCredentials{
		AccessToken:    tokenResp.AccessToken,
		TokenType:      tokenResp.TokenType,
		Expiration:     expiresOn.Format(time.RFC3339),
		TenantID:       p.tenantID,
		SubscriptionID: subscriptionID,
		Location:       p.location,
	}

	log.Debug("Successfully authenticated with Azure CLI",
		"provider", p.name,
		"tenant", p.tenantID,
		"subscription", subscriptionID,
	)

	return creds, nil
}

// Validate validates the provider configuration.
func (p *cliProvider) Validate() error {
	if p.tenantID == "" {
		return fmt.Errorf("%w: tenant_id is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns environment variables for this provider.
func (p *cliProvider) Environment() (map[string]string, error) {
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
func (p *cliProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	// Use shared Azure environment preparation.
	// Note: access token is set later by SetEnvironmentVariables which loads from credential store.
	return azureCloud.PrepareEnvironment(
		environ,
		p.subscriptionID,
		p.tenantID,
		p.location,
		"",  // No credentials file for CLI provider.
		"",  // Access token loaded from credential store by SetEnvironmentVariables.
	), nil
}

// Logout is a no-op for Azure CLI provider (credentials are managed by az CLI).
func (p *cliProvider) Logout(ctx context.Context) error {
	log.Debug("Azure CLI provider logout is managed by 'az logout'", "provider", p.name)
	return nil
}

// GetFilesDisplayPath returns empty string (no files managed by this provider).
func (p *cliProvider) GetFilesDisplayPath() string {
	return "" // CLI provider doesn't manage files.
}
