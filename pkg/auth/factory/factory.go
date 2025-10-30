package factory

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	awsIdentities "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	awsProviders "github.com/cloudposse/atmos/pkg/auth/providers/aws"
	githubProviders "github.com/cloudposse/atmos/pkg/auth/providers/github"
	mockProviders "github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewProvider creates a new provider instance based on the provider configuration.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	defer perf.Track(nil, "factory.NewProvider")()
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidAuthConfig)
	}
	switch config.Kind {
	case "aws/iam-identity-center":
		return awsProviders.NewSSOProvider(name, config)
	case "aws/saml":
		return awsProviders.NewSAMLProvider(name, config)
	case "github/oidc":
		return githubProviders.NewOIDCProvider(name, config)
	case "mock":
		return mockProviders.NewProvider(name, config), nil
	default:
		return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}
}

// NewIdentity creates a new identity instance based on the identity configuration.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "factory.NewIdentity")()
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidAuthConfig)
	}
	switch config.Kind {
	case "aws/permission-set":
		return awsIdentities.NewPermissionSetIdentity(name, config)
	case "aws/assume-role":
		return awsIdentities.NewAssumeRoleIdentity(name, config)
	case "aws/user":
		return awsIdentities.NewUserIdentity(name, config)
	case "mock":
		return mockProviders.NewIdentity(name, config), nil
	default:
		return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
}
