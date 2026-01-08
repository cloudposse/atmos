package integrations

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Integration kind constants.
const (
	KindAWSECR = "aws/ecr"
	KindAWSEKS = "aws/eks" // Future.
)

// Integration represents a client-only credential materialization.
// Integrations derive credentials from identities for service-specific access
// (e.g., ECR docker login, EKS kubeconfig).
type Integration interface {
	// Kind returns the integration type (e.g., "aws/ecr").
	Kind() string

	// Execute performs the integration using the provided AWS credentials.
	// Returns nil on success, error on failure.
	Execute(ctx context.Context, creds types.ICredentials) error
}

// IntegrationConfig wraps the schema.Integration with the integration name.
type IntegrationConfig struct {
	Name   string
	Config *schema.Integration
}

// IntegrationFactory creates integrations from configuration.
type IntegrationFactory func(config *IntegrationConfig) (Integration, error)
