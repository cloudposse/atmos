package auth

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/auth/identities/aws"
	awsProviders "github.com/cloudposse/atmos/internal/auth/providers/aws"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewProvider creates a new provider instance based on the provider configuration
func NewProvider(name string, config *schema.Provider) (Provider, error) {
	switch config.Kind {
	case "aws/iam-identity-center":
		return awsProviders.NewSSOProvider(name, config)
	case "aws/assume-role":
		return awsProviders.NewAssumeRoleProvider(name, config)
	case "aws/user":
		return awsProviders.NewUserProvider(name, config)
	default:
		return nil, fmt.Errorf("unsupported provider kind: %s", config.Kind)
	}
}

// NewIdentity creates a new identity instance based on the identity configuration
func NewIdentity(name string, config *schema.Identity) (Identity, error) {
	switch config.Kind {
	case "aws/permission-set":
		return aws.NewPermissionSetIdentity(name, config)
	case "aws/assume-role":
		return aws.NewAssumeRoleIdentity(name, config)
	case "aws/user":
		return aws.NewUserIdentity(name, config)
	default:
		return nil, fmt.Errorf("unsupported identity kind: %s", config.Kind)
	}
}
