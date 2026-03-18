package aws

import (
	"context"
	"fmt"
	"time"

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
	integrations.Register(integrations.KindAWSECRPublic, NewECRPublicIntegration)
}

// ECRPublicIntegration implements the aws/ecr-public integration type.
type ECRPublicIntegration struct {
	name     string
	identity string
}

// NewECRPublicIntegration creates an ECR Public integration from config.
func NewECRPublicIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "aws.NewECRPublicIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	// Extract identity from via.identity.
	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	// Validate region if user specified one in spec.registry.
	if config.Config.Spec != nil && config.Config.Spec.Registry != nil && config.Config.Spec.Registry.Region != "" {
		if err := awsCloud.ValidateECRPublicRegion(config.Config.Spec.Registry.Region); err != nil {
			return nil, fmt.Errorf("%w: integration '%s': %w", errUtils.ErrIntegrationFailed, config.Name, err)
		}
	}

	return &ECRPublicIntegration{
		name:     config.Name,
		identity: identity,
	}, nil
}

// Kind returns "aws/ecr-public".
func (e *ECRPublicIntegration) Kind() string {
	return integrations.KindAWSECRPublic
}

// Execute performs ECR Public login.
func (e *ECRPublicIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.ECRPublicIntegration.Execute")()

	// Create Docker config manager.
	dockerConfig, err := docker.NewConfigManager()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	log.Debug("Logging in to ECR Public registry", "registry", awsCloud.ECRPublicRegistryURL)

	// Get authorization token from ECR Public (always uses us-east-1).
	result, err := awsCloud.GetPublicAuthorizationToken(ctx, creds)
	if err != nil {
		return fmt.Errorf("%w: failed to get ECR Public token: %w", errUtils.ErrECRPublicAuthFailed, err)
	}

	// Write credentials to Docker config.
	if err := dockerConfig.WriteAuth(awsCloud.ECRPublicRegistryURL, result.Username, result.Password); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	// Log success with actual expiration time.
	expiresIn := time.Until(result.ExpiresAt).Round(time.Minute)
	ui.Success(fmt.Sprintf("ECR Public login: %s (expires in %s)", awsCloud.ECRPublicRegistryURL, expiresIn))
	log.Debug("ECR Public login successful", "registry", awsCloud.ECRPublicRegistryURL, "expires_at", result.ExpiresAt)

	return nil
}

// GetIdentity returns the identity name this integration uses.
func (e *ECRPublicIntegration) GetIdentity() string {
	return e.identity
}
