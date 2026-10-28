package integrations

//go:generate go run go.uber.org/mock/mockgen -destination mock_registry_credentials_test.go -package integrations github.com/cloudposse/atmos/pkg/auth/integrations Integration,RegistryCredentialProvider

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegistryCredentialProvider exchanges identity credentials directly for a registry token.
// It does not publish credentials to shared Docker files or invoke ambient helpers.
type RegistryCredentialProvider interface {
	ExchangeCredentials(context.Context, types.ICredentials) (*types.OCICredentials, error)
}

// ExchangeRegistryCredentials acquires the selected integration's registry credentials
// without round-tripping through a shared Docker configuration that another login can replace.
func ExchangeRegistryCredentials(ctx context.Context, config *IntegrationConfig, credentials types.ICredentials) (*types.OCICredentials, error) {
	defer perf.Track(nil, "integrations.ExchangeRegistryCredentials")()

	integration, err := Create(config)
	if err != nil {
		return nil, err
	}
	provider, ok := integration.(RegistryCredentialProvider)
	if !ok {
		return nil, fmt.Errorf("%w: integration %s cannot exchange registry credentials", errUtils.ErrIntegrationFailed, config.Name)
	}
	return provider.ExchangeCredentials(ctx, credentials)
}
