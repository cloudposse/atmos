package integrations

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Integration kind constants.
const (
	KindAWSECR       = "aws/ecr"
	KindAWSECRPublic = "aws/ecr-public"
	KindAWSEKS       = "aws/eks"
	KindAzureACR     = "azure/acr"
	KindAzureAKS     = "azure/aks"
	KindGCPGKE       = "gcp/gke"
	KindGitHubSTS    = "github/sts"
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
	// Realm is the credential isolation realm. Integrations that persist secret
	// material to disk (e.g., github/sts) use it to scope their state paths so
	// minted credentials are isolated per repository/customer environment.
	// Kubeconfig integrations (aws/eks, azure/aks, gcp/gke) use it to scope
	// their kube user and context names, but only when RealmExplicit reports
	// true. Integrations that don't need it (aws/ecr) may ignore it.
	Realm string
	// RealmSource is how Realm was determined: realm.SourceEnv or
	// realm.SourceConfig for a realm set by ATMOS_AUTH_REALM or auth.realm, or
	// realm.SourceAuto for one Atmos derived because none is configured (a hash
	// of the atmos.yaml path). Empty counts as explicit, for callers that set
	// Realm directly.
	RealmSource string
}

// RealmExplicit reports whether Realm was configured explicitly, so names that
// users type (kube contexts) and see (kube users) may include it. A derived
// realm still isolates credentials, but must not change those names.
func (c *IntegrationConfig) RealmExplicit() bool {
	return c.Realm != "" && realm.IsExplicitSource(c.RealmSource)
}

// IntegrationFactory creates integrations from configuration.
type IntegrationFactory func(config *IntegrationConfig) (Integration, error)
