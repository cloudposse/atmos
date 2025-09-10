package validation

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validator implements the Validator interface.
type validator struct{}

// NewValidator creates a new validator instance.
func NewValidator() types.Validator {
	return &validator{}
}

// ErrIdentityCycle signals a circular dependency in an identity chain.
var ErrIdentityCycle = errors.New("identity cycle detected")

// ValidateAuthConfig validates the entire auth configuration.
func (v *validator) ValidateAuthConfig(config *schema.AuthConfig) error {
	if config == nil {
		return fmt.Errorf("%w: auth config cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	// Validate logs.
	if err := v.ValidateLogsConfig(&config.Logs); err != nil {
		return fmt.Errorf("logs configuration validation failed: %w", errors.Join(errUtils.ErrInvalidAuthConfig, err))
	}

	// Validate providers.
	//nolint:gocritic // rangeValCopy: map stores structs; address of map element can't be taken. Passing copy to factory is intended.
	for name, provider := range config.Providers {
		if err := v.ValidateProvider(name, &provider); err != nil {
			return fmt.Errorf("provider %q validation failed: %w", name, errors.Join(errUtils.ErrInvalidAuthConfig, err))
		}
	}

	// Validate identities

	for name, identity := range config.Identities {
		if err := v.ValidateIdentity(name, &identity, convertProviders(config.Providers)); err != nil {
			return fmt.Errorf("identity %q validation failed: %w", name, errors.Join(errUtils.ErrInvalidAuthConfig, err))
		}
	}

	// Validate chains
	if err := v.ValidateChains(convertIdentities(config.Identities), convertProviders(config.Providers)); err != nil {
		return fmt.Errorf("identity chain validation failed: %w", errors.Join(errUtils.ErrInvalidAuthConfig, err))
	}

	return nil
}

// ValidateLogsConfig validates the logs configuration.
func (v *validator) ValidateLogsConfig(logs *schema.Logs) error {
	if logs.Level == "" {
		// Default to Info if not specified
		return nil
	}

	validLevels := []string{"Debug", "Info", "Warn", "Error"}
	for _, validLevel := range validLevels {
		if logs.Level == validLevel {
			return nil
		}
	}

	return fmt.Errorf("%w: invalid log level %q, must be one of: %s", errUtils.ErrInvalidAuthConfig, logs.Level, strings.Join(validLevels, ", "))
}

// ValidateProvider validates a provider configuration.
func (v *validator) ValidateProvider(name string, provider *schema.Provider) error {
	if name == "" {
		return fmt.Errorf("%w: provider name cannot be empty", errUtils.ErrInvalidProviderConfig)
	}

	if provider.Kind == "" {
		return fmt.Errorf("%w: provider kind is required", errUtils.ErrInvalidProviderConfig)
	}

	// TODO replace with Provider Interface Validate()
	// Validate based on provider kind
	switch provider.Kind {
	case "aws/iam-identity-center":
		return v.validateSSOProvider(provider)
	case "aws/saml":
		return v.validateSAMLProvider(provider)
	case "github/oidc":
		return v.validateGitHubOIDCProvider(provider)
	default:
		return fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, provider.Kind)
	}
}

// ValidateIdentity validates an identity configuration.
func (v *validator) ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error {
	if name == "" {
		return fmt.Errorf("%w: identity name cannot be empty", errUtils.ErrInvalidIdentityConfig)
	}

	if identity.Kind == "" {
		return fmt.Errorf("%w: identity kind is required", errUtils.ErrInvalidIdentityConfig)
	}

	// Validate via configuration - AWS User identities don't require via provider
	if err := v.validateViaConfiguration(identity, providers); err != nil {
		return err
	}

	// TODO replace with Identity Interface Validate()
	// Validate based on identity kind
	switch identity.Kind {
	case "aws/permission-set":
		return v.validatePermissionSetIdentity(identity)
	case "aws/assume-role":
		return v.validateAssumeRoleIdentity(identity)
	case "aws/user":
		return v.validateUserIdentity(identity)
	default:
		return fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidAuthConfig, identity.Kind)
	}
}

// validateViaConfiguration validates the optional Via provider/identity references for an identity.
func (v *validator) validateViaConfiguration(identity *schema.Identity, providers map[string]*schema.Provider) error {
	if identity.Kind == "aws/user" || identity.Via == nil {
		return nil
	}
	if identity.Via.Provider != "" {
		if _, exists := providers[identity.Via.Provider]; !exists {
			return fmt.Errorf("%w: referenced provider %q does not exist", errUtils.ErrInvalidAuthConfig, identity.Via.Provider)
		}
	}
	return nil
}

// ValidateChains validates identity chains for cycles and invalid references.
func (v *validator) ValidateChains(identities map[string]*schema.Identity, providers map[string]*schema.Provider) error {
	// Build dependency graph
	graph := make(map[string][]string)

	for name, identity := range identities {
		if identity.Via != nil {
			if identity.Via.Identity != "" {
				if _, ok := identities[identity.Via.Identity]; !ok {
					return fmt.Errorf("%w: referenced identity %q does not exist", errUtils.ErrInvalidAuthConfig, identity.Via.Identity)
				}
				graph[name] = append(graph[name], identity.Via.Identity)
			}
		}
	}

	// Check for cycles using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for name := range identities {
		if !visited[name] {
			if v.hasCycle(name, graph, visited, recStack) {
				// Return a domain-specific sentinel to enable precise error checks by callers/tests
				return fmt.Errorf("%w: circular dependency detected in identity chain involving %q", ErrIdentityCycle, name)
			}
		}
	}

	return nil
}

// validateSSOProvider validates AWS SSO provider configuration.
func (v *validator) validateSSOProvider(provider *schema.Provider) error {
	if provider.StartURL == "" {
		return fmt.Errorf("%w: start_url is required for AWS SSO provider", errUtils.ErrInvalidAuthConfig)
	}

	if provider.Region == "" {
		return fmt.Errorf("%w: region is required for AWS SSO provider", errUtils.ErrInvalidAuthConfig)
	}

	return nil
}

// (aws/assume-role provider removed)

// validateSAMLProvider validates AWS SAML provider configuration.
func (v *validator) validateSAMLProvider(provider *schema.Provider) error {
	if provider.URL == "" {
		return fmt.Errorf("%w: url is required for AWS SAML provider", errUtils.ErrInvalidAuthConfig)
	}

	if provider.Region == "" {
		return fmt.Errorf("%w: region is required for AWS SAML provider", errUtils.ErrInvalidAuthConfig)
	}

	// Validate URL format
	if _, err := url.Parse(provider.URL); err != nil {
		return fmt.Errorf("invalid URL format: %w", errors.Join(errUtils.ErrInvalidAuthConfig, err))
	}

	return nil
}

// validateGitHubOIDCProvider validates GitHub OIDC provider configuration.
func (v *validator) validateGitHubOIDCProvider(provider *schema.Provider) error {
	return nil
}

// validatePermissionSetIdentity validates AWS permission set identity configuration.
func (v *validator) validatePermissionSetIdentity(identity *schema.Identity) error {
	if identity.Principal == nil {
		return fmt.Errorf("%w: principal is required for permission set identity", errUtils.ErrInvalidAuthConfig)
	}

	name, ok := identity.Principal["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("%w: permission set name is required in principal", errUtils.ErrInvalidAuthConfig)
	}

	accountSpec, ok := identity.Principal["account"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("%w: account specification is required", errUtils.ErrInvalidAuthConfig)
	}

	accountName, ok := accountSpec["name"].(string)
	accountId, okId := accountSpec["id"].(string)
	if (!ok && !okId) || (accountName == "" && accountId == "") {
		return fmt.Errorf("%w: account name or account ID is required", errUtils.ErrInvalidAuthConfig)
	}

	return nil
}

// validateAssumeRoleIdentity validates AWS assume role identity configuration.
func (v *validator) validateAssumeRoleIdentity(identity *schema.Identity) error {
	if identity.Principal == nil {
		return fmt.Errorf("%w: principal is required for assume role identity", errUtils.ErrInvalidAuthConfig)
	}

	roleArn, ok := identity.Principal["assume_role"].(string)
	if !ok || roleArn == "" {
		return fmt.Errorf("%w: assume_role is required in principal", errUtils.ErrInvalidAuthConfig)
	}

	if !awsarn.IsARN(roleArn) {
		return fmt.Errorf("%w: invalid role ARN format %q", errUtils.ErrInvalidAuthConfig, roleArn)
	}

	return nil
}

// validateUserIdentity validates AWS user identity configuration.
func (v *validator) validateUserIdentity(identity *schema.Identity) error {
	// User identities typically don't require additional validation
	return nil
}

// hasCycle performs DFS to detect cycles in the dependency graph.
func (v *validator) hasCycle(node string, graph map[string][]string, visited, recStack map[string]bool) bool {
	visited[node] = true
	recStack[node] = true

	for _, neighbor := range graph[node] {
		if !visited[neighbor] {
			if v.hasCycle(neighbor, graph, visited, recStack) {
				return true
			}
		} else if recStack[neighbor] {
			return true
		}
	}

	recStack[node] = false
	return false
}

// Helper functions to convert map types.
func convertProviders(providers map[string]schema.Provider) map[string]*schema.Provider {
	result := make(map[string]*schema.Provider)
	//nolint:gocritic // rangeValCopy: map stores structs; address of map element can't be taken. Passing copy to factory is intended.
	for k, v := range providers {
		provider := v
		result[k] = &provider
	}
	return result
}

func convertIdentities(identities map[string]schema.Identity) map[string]*schema.Identity {
	result := make(map[string]*schema.Identity)
	for k, v := range identities {
		identity := v
		result[k] = &identity
	}
	return result
}
