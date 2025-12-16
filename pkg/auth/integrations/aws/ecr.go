package aws

import (
	"context"
	"fmt"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func init() {
	integrations.Register(integrations.KindAWSECR, NewECRIntegration)
}

// ECRIntegration implements the aws/ecr integration type.
type ECRIntegration struct {
	name     string
	identity string
	registry *schema.ECRRegistry
}

// NewECRIntegration creates an ECR integration from config.
func NewECRIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "aws.NewECRIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	// Extract identity from via.identity.
	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	// Extract registry from spec.registry.
	var registry *schema.ECRRegistry
	if config.Config.Spec != nil && config.Config.Spec.Registry != nil {
		registry = config.Config.Spec.Registry
	}

	return &ECRIntegration{
		name:     config.Name,
		identity: identity,
		registry: registry,
	}, nil
}

// Kind returns "aws/ecr".
func (e *ECRIntegration) Kind() string {
	return integrations.KindAWSECR
}

// Execute performs ECR login for the configured registry.
func (e *ECRIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.ECRIntegration.Execute")()

	if e.registry == nil {
		log.Warn("ECR integration has no registry configured", "integration", e.name)
		return nil
	}

	// Create Docker config manager.
	dockerConfig, err := docker.NewConfigManager()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	log.Debug("Logging in to ECR registry", "account_id", e.registry.AccountID, "region", e.registry.Region)

	// Get authorization token from ECR.
	result, err := awsCloud.GetAuthorizationToken(ctx, creds, e.registry.AccountID, e.registry.Region)
	if err != nil {
		return fmt.Errorf("%w: failed to get ECR token for %s: %w", errUtils.ErrECRAuthFailed, e.registry.AccountID, err)
	}

	// Build registry URL.
	registryURL := awsCloud.BuildRegistryURL(e.registry.AccountID, e.registry.Region)

	// Write credentials to Docker config.
	if err := dockerConfig.WriteAuth(registryURL, result.Username, result.Password); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	// Set DOCKER_CONFIG environment variable.
	if err := os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir()); err != nil {
		log.Warn("Failed to set DOCKER_CONFIG environment variable", "error", err)
	}

	// Log success with actual expiration time.
	expiresIn := time.Until(result.ExpiresAt).Round(time.Minute)
	_ = ui.Success(fmt.Sprintf("ECR login: %s (expires in %s)", registryURL, expiresIn))
	log.Debug("ECR login successful", "registry", registryURL, "expires_at", result.ExpiresAt)

	return nil
}

// GetIdentity returns the identity name this integration uses.
func (e *ECRIntegration) GetIdentity() string {
	return e.identity
}

// GetRegistry returns the configured registry.
func (e *ECRIntegration) GetRegistry() *schema.ECRRegistry {
	return e.registry
}
