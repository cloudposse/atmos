package integrations

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Integration kind constants.
const (
	KindAWSECR = "aws/ecr"
	KindAWSEKS = "aws/eks"
)

// Integration represents a client-only credential materialization.
// Integrations derive credentials from identities for service-specific access
// (e.g., ECR docker login, EKS kubeconfig).
type Integration interface {
	// Kind returns the integration type (e.g., "aws/ecr", "aws/eks").
	Kind() string

	// Execute performs the integration using the provided credentials.
	// Returns nil on success, error on failure.
	Execute(ctx context.Context, creds types.ICredentials) error

	// Cleanup reverses the effects of Execute (e.g., removes kubeconfig entries, docker logout).
	// Called during identity/provider logout to clean up integration artifacts.
	// Idempotent — returns nil if nothing to clean up.
	// Errors are non-fatal during logout (logged as warnings, do not block logout).
	Cleanup(ctx context.Context) error

	// Environment returns environment variables contributed by this integration.
	// Returns vars based on configuration (deterministic), not Execute() output.
	// Called by the manager when composing env vars for atmos auth env / auth shell.
	Environment() (map[string]string, error)
}

// IntegrationConfig wraps the schema.Integration with the integration name.
type IntegrationConfig struct {
	Name   string
	Config *schema.Integration
}

// IntegrationFactory creates integrations from configuration.
type IntegrationFactory func(config *IntegrationConfig) (Integration, error)
