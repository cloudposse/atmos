package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

func init() {
	integrations.Register(integrations.KindAWSECR, NewECRIntegration)
}

// ECRRegistry represents a single ECR registry configuration.
type ECRRegistry struct {
	AccountID string `mapstructure:"account_id"`
	Region    string `mapstructure:"region"`
}

// ECRIntegration implements the aws/ecr integration type.
type ECRIntegration struct {
	name       string
	identity   string
	registries []ECRRegistry
}

// NewECRIntegration creates an ECR integration from config.
func NewECRIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "aws.NewECRIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	// Extract registries from spec.
	var registries []ECRRegistry
	if config.Config.Spec != nil {
		if regList, ok := config.Config.Spec["registries"]; ok {
			if err := mapstructure.Decode(regList, &registries); err != nil {
				return nil, fmt.Errorf("%w: invalid registries config: %w", errUtils.ErrIntegrationFailed, err)
			}
		}
	}

	return &ECRIntegration{
		name:       config.Name,
		identity:   config.Config.Identity,
		registries: registries,
	}, nil
}

// Kind returns "aws/ecr".
func (e *ECRIntegration) Kind() string {
	return integrations.KindAWSECR
}

// Execute performs ECR login for all configured registries.
func (e *ECRIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.ECRIntegration.Execute")()

	if len(e.registries) == 0 {
		log.Warn("ECR integration has no registries configured", "integration", e.name)
		return nil
	}

	// Create Docker config manager.
	dockerConfig, err := docker.NewConfigManager()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	// Login to each registry.
	for _, reg := range e.registries {
		if err := e.loginToRegistry(ctx, creds, dockerConfig, reg); err != nil {
			return err
		}
	}

	// Set DOCKER_CONFIG environment variable.
	if err := os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir()); err != nil {
		log.Warn("Failed to set DOCKER_CONFIG environment variable", "error", err)
	}

	return nil
}

// loginToRegistry performs ECR login for a single registry.
func (e *ECRIntegration) loginToRegistry(ctx context.Context, creds types.ICredentials, dockerConfig *docker.ConfigManager, reg ECRRegistry) error {
	defer perf.Track(nil, "aws.ECRIntegration.loginToRegistry")()

	log.Debug("Logging in to ECR registry", "account_id", reg.AccountID, "region", reg.Region)

	// Get authorization token from ECR.
	result, err := awsCloud.GetAuthorizationToken(ctx, creds, reg.AccountID, reg.Region)
	if err != nil {
		return fmt.Errorf("%w: failed to get ECR token for %s: %w", errUtils.ErrECRAuthFailed, reg.AccountID, err)
	}

	// Build registry URL.
	registryURL := awsCloud.BuildRegistryURL(reg.AccountID, reg.Region)

	// Write credentials to Docker config.
	if err := dockerConfig.WriteAuth(registryURL, result.Username, result.Password); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	// Log success.
	_ = ui.Success(fmt.Sprintf("ECR login: %s (expires in 12h)", registryURL))
	log.Debug("ECR login successful", "registry", registryURL, "expires_at", result.ExpiresAt)

	return nil
}

// GetIdentity returns the identity name this integration uses.
func (e *ECRIntegration) GetIdentity() string {
	return e.identity
}

// GetRegistries returns the configured registries.
func (e *ECRIntegration) GetRegistries() []ECRRegistry {
	return e.registries
}
