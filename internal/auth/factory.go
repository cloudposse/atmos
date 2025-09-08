package auth

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/identities/aws"
	awsProviders "github.com/cloudposse/atmos/internal/auth/providers/aws"
	githubProviders "github.com/cloudposse/atmos/internal/auth/providers/github"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewProvider creates a new provider instance based on the provider configuration.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidAuthConfig)
	}
	switch config.Kind {
	case "aws/iam-identity-center":
		return awsProviders.NewSSOProvider(name, config)
	case "aws/assume-role":
		return awsProviders.NewAssumeRoleProvider(name, config)
	case "aws/saml":
		return awsProviders.NewSAMLProvider(name, config)
	case "github/oidc":
		return githubProviders.NewOIDCProvider(name, config)
	default:
		return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}
}

// NewIdentity creates a new identity instance based on the identity configuration.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidAuthConfig)
	}
	switch config.Kind {
	case "aws/permission-set":
		return aws.NewPermissionSetIdentity(name, config)
	case "aws/assume-role":
		return aws.NewAssumeRoleIdentity(name, config)
	case "aws/user":
		return aws.NewUserIdentity(name, config)
	default:
		return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
}
