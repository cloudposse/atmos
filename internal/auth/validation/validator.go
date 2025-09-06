package validation

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validator implements the Validator interface
type validator struct{}

// NewValidator creates a new validator instance
func NewValidator() types.Validator {
	return &validator{}
}

// ValidateAuthConfig validates the entire auth configuration
func (v *validator) ValidateAuthConfig(config *schema.AuthConfig) error {
	if config == nil {
		return fmt.Errorf("auth config cannot be nil")
	}

	// Validate logs configuration
	if config.Logs != nil {
		if err := v.ValidateLogsConfig(config.Logs); err != nil {
			return fmt.Errorf("logs configuration validation failed: %w", err)
		}
	}

	// Validate providers
	for name, provider := range config.Providers {
		if err := v.ValidateProvider(name, &provider); err != nil {
			return fmt.Errorf("provider %q validation failed: %w", name, err)
		}
	}

	// Validate identities
	for name, identity := range config.Identities {
		if err := v.ValidateIdentity(name, &identity, convertProviders(config.Providers)); err != nil {
			return fmt.Errorf("identity %q validation failed: %w", name, err)
		}
	}

	// Validate chains
	if err := v.ValidateChains(convertIdentities(config.Identities), convertProviders(config.Providers)); err != nil {
		return fmt.Errorf("identity chain validation failed: %w", err)
	}

	return nil
}

// ValidateLogsConfig validates the logs configuration
func (v *validator) ValidateLogsConfig(logs *schema.LogsConfig) error {
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

	return fmt.Errorf("invalid log level %q, must be one of: %s", logs.Level, strings.Join(validLevels, ", "))
}

// ValidateProvider validates a provider configuration
func (v *validator) ValidateProvider(name string, provider *schema.Provider) error {
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	if provider.Kind == "" {
		return fmt.Errorf("provider kind is required")
	}

	// Validate based on provider kind
	switch provider.Kind {
	case "aws/iam-identity-center":
		return v.validateSSOProvider(provider)
	case "aws/assume-role":
		return v.validateAssumeRoleProvider(provider)
	case "aws/saml":
		return v.validateSAMLProvider(provider)
	case "github/oidc":
		return v.validateGitHubOIDCProvider(provider)
	default:
		return fmt.Errorf("unsupported provider kind: %s", provider.Kind)
	}
}

// ValidateIdentity validates an identity configuration
func (v *validator) ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error {
	if name == "" {
		return fmt.Errorf("identity name cannot be empty")
	}

	if identity.Kind == "" {
		return fmt.Errorf("identity kind is required")
	}

	// Validate via configuration - AWS User identities don't require via provider
	if identity.Kind != "aws/user" && identity.Via != nil {
		if identity.Via.Provider != "" {
			if _, exists := providers[identity.Via.Provider]; !exists {
				return fmt.Errorf("referenced provider %q does not exist", identity.Via.Provider)
			}
		}
	}

	// Validate based on identity kind
	switch identity.Kind {
	case "aws/permission-set":
		return v.validatePermissionSetIdentity(identity)
	case "aws/assume-role":
		return v.validateAssumeRoleIdentity(identity)
	case "aws/user":
		return v.validateUserIdentity(identity)
	default:
		return fmt.Errorf("unsupported identity kind: %s", identity.Kind)
	}
}

// ValidateChains validates identity chains for cycles and invalid references
func (v *validator) ValidateChains(identities map[string]*schema.Identity, providers map[string]*schema.Provider) error {
	// Build dependency graph
	graph := make(map[string][]string)

	for name, identity := range identities {
		if identity.Via != nil {
			if identity.Via.Identity != "" {
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
				return fmt.Errorf("circular dependency detected in identity chain involving %q", name)
			}
		}
	}

	return nil
}

// validateSSOProvider validates AWS SSO provider configuration
func (v *validator) validateSSOProvider(provider *schema.Provider) error {
	if provider.StartURL == "" {
		return fmt.Errorf("start_url is required for AWS SSO provider")
	}

	if provider.Region == "" {
		return fmt.Errorf("region is required for AWS SSO provider")
	}

	return nil
}

// validateAssumeRoleProvider validates AWS assume role provider configuration
func (v *validator) validateAssumeRoleProvider(provider *schema.Provider) error {
	if provider.Spec == nil {
		return fmt.Errorf("spec is required for assume role provider")
	}

	roleArn, ok := provider.Spec["role_arn"].(string)
	if !ok || roleArn == "" {
		return fmt.Errorf("role_arn is required in spec")
	}

	if !strings.HasPrefix(roleArn, "arn:aws:iam::") {
		return fmt.Errorf("invalid role ARN format")
	}

	return nil
}

// validateSAMLProvider validates AWS SAML provider configuration
func (v *validator) validateSAMLProvider(provider *schema.Provider) error {
	if provider.URL == "" {
		return fmt.Errorf("url is required for AWS SAML provider")
	}

	if provider.Region == "" {
		return fmt.Errorf("region is required for AWS SAML provider")
	}

	// Validate URL format
	if _, err := url.Parse(provider.URL); err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	return nil
}

// validateGitHubOIDCProvider validates GitHub OIDC provider configuration
func (v *validator) validateGitHubOIDCProvider(provider *schema.Provider) error {
	if provider.Region == "" {
		return fmt.Errorf("region is required for GitHub OIDC provider")
	}

	return nil
}

// validatePermissionSetIdentity validates AWS permission set identity configuration
func (v *validator) validatePermissionSetIdentity(identity *schema.Identity) error {
	if identity.Principal == nil {
		return fmt.Errorf("principal is required for permission set identity")
	}

	name, ok := identity.Principal["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("permission set name is required in principal")
	}

	accountSpec, ok := identity.Principal["account"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("account specification is required")
	}

	accountName, ok := accountSpec["name"].(string)
	if !ok || accountName == "" {
		return fmt.Errorf("account name is required")
	}

	return nil
}

// validateAssumeRoleIdentity validates AWS assume role identity configuration
func (v *validator) validateAssumeRoleIdentity(identity *schema.Identity) error {
	if identity.Principal == nil {
		return fmt.Errorf("principal is required for assume role identity")
	}

	roleArn, ok := identity.Principal["assume_role"].(string)
	if !ok || roleArn == "" {
		return fmt.Errorf("assume_role is required in principal")
	}

	if !strings.HasPrefix(roleArn, "arn:aws:iam::") {
		return fmt.Errorf("invalid role ARN format")
	}

	return nil
}

// validateUserIdentity validates AWS user identity configuration
func (v *validator) validateUserIdentity(identity *schema.Identity) error {
	// User identities typically don't require additional validation
	return nil
}

// hasCycle performs DFS to detect cycles in the dependency graph
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

// Helper functions to convert map types
func convertProviders(providers map[string]schema.Provider) map[string]*schema.Provider {
	result := make(map[string]*schema.Provider)
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
