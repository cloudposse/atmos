package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/ini.v1"
)

// DefaultCloudProviderFactory implements CloudProviderFactory
type DefaultCloudProviderFactory struct {
	providers map[string]CloudProvider
	mutex     sync.RWMutex
}

// NewCloudProviderFactory creates a new cloud provider factory with default providers
func NewCloudProviderFactory() CloudProviderFactory {
	factory := &DefaultCloudProviderFactory{
		providers: make(map[string]CloudProvider),
	}

	// Register default cloud providers
	factory.RegisterCloudProvider("aws", newAWSCloudProvider())
	factory.RegisterCloudProvider("azure", newAzureCloudProvider())
	factory.RegisterCloudProvider("gcp", newGCPCloudProvider())

	return factory
}

// Embedded cloud provider implementations to avoid import cycles

// awsCloudProvider implements CloudProvider for AWS
type awsCloudProvider struct {
	homeDir string
}

func (p *awsCloudProvider) GetName() string {
	return "aws"
}

func (p *awsCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *schema.Credentials) error {
	if credentials == nil || credentials.AWS == nil {
		return fmt.Errorf("AWS credentials are required")
	}

	// Create AWS directory structure
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		return fmt.Errorf("failed to create AWS directory: %w", err)
	}

	// Write credentials file
	credPath := filepath.Join(awsDir, "credentials")
	if err := p.writeCredentialsFile(credPath, identityName, credentials.AWS); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	// Write config file
	configPath := filepath.Join(awsDir, "config")
	if err := p.writeConfigFile(configPath, identityName, credentials.AWS.Region); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (p *awsCloudProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)
	return map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": filepath.Join(awsDir, "credentials"),
		"AWS_CONFIG_FILE":             filepath.Join(awsDir, "config"),
		"AWS_PROFILE":                 identityName,
	}
}

func (p *awsCloudProvider) Cleanup(ctx context.Context, providerName, identityName string) error {
	// Currently no cleanup needed - files are persistent for caching
	return nil
}

func (p *awsCloudProvider) ValidateCredentials(ctx context.Context, credentials *schema.Credentials) error {
	if credentials == nil || credentials.AWS == nil {
		return fmt.Errorf("AWS credentials are required")
	}
	if credentials.AWS.AccessKeyID == "" || credentials.AWS.SecretAccessKey == "" {
		return fmt.Errorf("AWS access key ID and secret access key are required")
	}
	return nil
}

func (p *awsCloudProvider) GetCredentialFilePaths(providerName string) map[string]string {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)
	return map[string]string{
		"credentials": filepath.Join(awsDir, "credentials"),
		"config":      filepath.Join(awsDir, "config"),
	}
}

func (p *awsCloudProvider) writeCredentialsFile(path, identityName string, creds *schema.AWSCredentials) error {
	cfg := ini.Empty()
	section, _ := cfg.NewSection(identityName)
	section.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	if creds.SessionToken != "" {
		section.Key("aws_session_token").SetValue(creds.SessionToken)
	}
	return cfg.SaveTo(path)
}

func (p *awsCloudProvider) writeConfigFile(path, identityName, region string) error {
	if region == "" {
		region = "us-east-1"
	}
	cfg := ini.Empty()
	section, _ := cfg.NewSection(fmt.Sprintf("profile %s", identityName))
	section.Key("region").SetValue(region)
	return cfg.SaveTo(path)
}

// azureCloudProvider implements CloudProvider for Azure
type azureCloudProvider struct{}

func (p *azureCloudProvider) GetName() string {
	return "azure"
}

func (p *azureCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *schema.Credentials) error {
	return fmt.Errorf("Azure environment setup not yet implemented")
}

func (p *azureCloudProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string {
	return map[string]string{
		"AZURE_CONFIG_DIR": fmt.Sprintf("~/.azure/atmos/%s", providerName),
	}
}

func (p *azureCloudProvider) Cleanup(ctx context.Context, providerName, identityName string) error {
	return nil
}

func (p *azureCloudProvider) ValidateCredentials(ctx context.Context, credentials *schema.Credentials) error {
	if credentials == nil || credentials.Azure == nil {
		return fmt.Errorf("Azure credentials are required")
	}
	return nil
}

func (p *azureCloudProvider) GetCredentialFilePaths(providerName string) map[string]string {
	return map[string]string{
		"config": fmt.Sprintf("~/.azure/atmos/%s/config", providerName),
	}
}

// gcpCloudProvider implements CloudProvider for GCP
type gcpCloudProvider struct{}

func (p *gcpCloudProvider) GetName() string {
	return "gcp"
}

func (p *gcpCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *schema.Credentials) error {
	return fmt.Errorf("GCP environment setup not yet implemented")
}

func (p *gcpCloudProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string {
	return map[string]string{
		"GOOGLE_APPLICATION_CREDENTIALS": fmt.Sprintf("~/.gcp/atmos/%s/credentials.json", providerName),
	}
}

func (p *gcpCloudProvider) Cleanup(ctx context.Context, providerName, identityName string) error {
	return nil
}

func (p *gcpCloudProvider) ValidateCredentials(ctx context.Context, credentials *schema.Credentials) error {
	if credentials == nil || credentials.GCP == nil {
		return fmt.Errorf("GCP credentials are required")
	}
	return nil
}

func (p *gcpCloudProvider) GetCredentialFilePaths(providerName string) map[string]string {
	return map[string]string{
		"credentials": fmt.Sprintf("~/.gcp/atmos/%s/credentials.json", providerName),
	}
}

// newAWSCloudProvider creates a new AWS cloud provider instance
func newAWSCloudProvider() CloudProvider {
	homeDir, _ := os.UserHomeDir()
	return &awsCloudProvider{
		homeDir: homeDir,
	}
}

// newAzureCloudProvider creates a new Azure cloud provider instance
func newAzureCloudProvider() CloudProvider {
	return &azureCloudProvider{}
}

// newGCPCloudProvider creates a new GCP cloud provider instance
func newGCPCloudProvider() CloudProvider {
	return &gcpCloudProvider{}
}

// GetCloudProvider returns the appropriate cloud provider for the given provider kind
func (f *DefaultCloudProviderFactory) GetCloudProvider(providerKind string) (CloudProvider, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// Extract cloud provider name from provider kind
	// Examples: "aws/sso" -> "aws", "azure/ad" -> "azure", "github/oidc" -> "aws" (for downstream AWS usage)
	cloudName := f.extractCloudName(providerKind)

	provider, exists := f.providers[cloudName]
	if !exists {
		return nil, fmt.Errorf("unsupported cloud provider: %s (from provider kind: %s)", cloudName, providerKind)
	}

	return provider, nil
}

// RegisterCloudProvider allows registration of new cloud providers
func (f *DefaultCloudProviderFactory) RegisterCloudProvider(name string, provider CloudProvider) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if name == "" {
		return fmt.Errorf("cloud provider name cannot be empty")
	}

	if provider == nil {
		return fmt.Errorf("cloud provider cannot be nil")
	}

	f.providers[name] = provider
	return nil
}

// ListCloudProviders returns all registered cloud provider names
func (f *DefaultCloudProviderFactory) ListCloudProviders() []string {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	names := make([]string, 0, len(f.providers))
	for name := range f.providers {
		names = append(names, name)
	}
	return names
}

// extractCloudName extracts the cloud provider name from a provider kind
func (f *DefaultCloudProviderFactory) extractCloudName(providerKind string) string {
	// Handle special cases first
	switch {
	case strings.HasPrefix(providerKind, "github/"):
		// GitHub OIDC providers typically target AWS for downstream usage
		return "aws"
	case strings.HasPrefix(providerKind, "aws/"):
		return "aws"
	case strings.HasPrefix(providerKind, "azure/"):
		return "azure"
	case strings.HasPrefix(providerKind, "gcp/") || strings.HasPrefix(providerKind, "google/"):
		return "gcp"
	default:
		// Default to the first part before the slash
		parts := strings.Split(providerKind, "/")
		if len(parts) > 0 {
			return parts[0]
		}
		return "aws" // Default fallback
	}
}
