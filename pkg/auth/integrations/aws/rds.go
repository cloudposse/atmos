package aws

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

func init() {
	integrations.Register(integrations.KindAWSRDS, NewRDSIntegration)
}

// RDSIntegration implements the aws/rds integration type.
//
// It is declarative connection metadata only. An RDS IAM authentication token is short-lived
// (~15 minutes) and single-use per connection, so — unlike ECR (docker credential) or EKS
// (kubeconfig) — there is nothing durable to materialize at login. Execute is therefore a no-op,
// and the connection details are consumed on demand by `atmos aws rds connect <name>`.
type RDSIntegration struct {
	name     string
	identity string
}

// NewRDSIntegration creates an aws/rds integration from config. It validates the declared database
// connection so a malformed integration fails fast at load time rather than at connect time.
func NewRDSIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "aws.NewRDSIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	spec := config.Config.Spec
	if spec == nil || spec.Database == nil {
		return nil, fmt.Errorf("%w: integration '%s': spec.database is required", errUtils.ErrRDSIntegrationConfig, config.Name)
	}
	db := spec.Database
	if db.Host == "" || db.Port == 0 || db.Username == "" {
		return nil, fmt.Errorf("%w: integration '%s': spec.database requires host, port, and username", errUtils.ErrRDSIntegrationConfig, config.Name)
	}

	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	return &RDSIntegration{
		name:     config.Name,
		identity: identity,
	}, nil
}

// Kind returns "aws/rds".
func (r *RDSIntegration) Kind() string {
	return integrations.KindAWSRDS
}

// Execute is a no-op. RDS IAM tokens are ephemeral and minted per connection by
// `atmos aws rds connect`, so there is nothing to provision at login.
func (r *RDSIntegration) Execute(_ context.Context, _ types.ICredentials) error {
	defer perf.Track(nil, "aws.RDSIntegration.Execute")()

	return nil
}

// Cleanup is a no-op because Execute materializes nothing.
func (r *RDSIntegration) Cleanup(_ context.Context) error {
	defer perf.Track(nil, "aws.RDSIntegration.Cleanup")()

	return nil
}

// Environment intentionally contributes nothing at `atmos auth env`/`auth shell`.
//
// Unlike EKS's composable KUBECONFIG, RDS connection coordinates (PGHOST/PGUSER/...) are
// single-valued and would collide across multiple aws/rds integrations linked to one identity, so
// they are not exported globally. `atmos aws rds connect <name>` reads this integration's metadata
// directly instead. The token/password is never exported here under any circumstances.
func (r *RDSIntegration) Environment() (map[string]string, error) {
	defer perf.Track(nil, "aws.RDSIntegration.Environment")()

	return nil, nil
}

// GetIdentity returns the identity name this integration authenticates with.
func (r *RDSIntegration) GetIdentity() string {
	return r.identity
}
