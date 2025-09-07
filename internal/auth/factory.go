package auth

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/auth/identities/aws"
	awsProviders "github.com/cloudposse/atmos/internal/auth/providers/aws"
	githubProviders "github.com/cloudposse/atmos/internal/auth/providers/github"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewProvider creates a new provider instance based on the provider configuration.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrStaticError)
	}
	switch config.Kind {
	case "aws/iam-identity-center":
		p, err := awsProviders.NewSSOProvider(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create provider '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return p, nil
	case "aws/assume-role":
		p, err := awsProviders.NewAssumeRoleProvider(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create provider '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return p, nil
	case "aws/saml":
		p, err := awsProviders.NewSAMLProvider(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create provider '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return p, nil
	case "github/oidc":
		p, err := githubProviders.NewOIDCProvider(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create provider '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrStaticError, config.Kind)
	}
}

// NewIdentity creates a new identity instance based on the identity configuration
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrStaticError)
	}
	switch config.Kind {
	case "aws/permission-set":
		i, err := aws.NewPermissionSetIdentity(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create identity '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return i, nil
	case "aws/assume-role":
		i, err := aws.NewAssumeRoleIdentity(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create identity '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return i, nil
	case "aws/user":
		i, err := aws.NewUserIdentity(name, config)
		if err != nil {
			return nil, fmt.Errorf("%w: create identity '%s' (kind=%s) failed: %v", errUtils.ErrStaticError, name, config.Kind, err)
		}
		return i, nil
	default:
		return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrStaticError, config.Kind)
	}
}
