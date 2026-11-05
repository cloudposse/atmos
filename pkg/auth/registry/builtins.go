package registry

import (
	ambientIdentities "github.com/cloudposse/atmos/pkg/auth/identities/ambient"
	atmosproIdentities "github.com/cloudposse/atmos/pkg/auth/identities/atmospro"
	awsIdentities "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	azureIdentities "github.com/cloudposse/atmos/pkg/auth/identities/azure"
	ociIdentities "github.com/cloudposse/atmos/pkg/auth/identities/oci"
	atmosproProviders "github.com/cloudposse/atmos/pkg/auth/providers/atmospro"
	awsProviders "github.com/cloudposse/atmos/pkg/auth/providers/aws"
	azureProviders "github.com/cloudposse/atmos/pkg/auth/providers/azure"
	githubProviders "github.com/cloudposse/atmos/pkg/auth/providers/github"
	mockProviders "github.com/cloudposse/atmos/pkg/auth/providers/mock"
	mockawsProviders "github.com/cloudposse/atmos/pkg/auth/providers/mock/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// builtinProviders preserves the complete configuration accepted by built-in constructors.
func builtinProviders() map[string]ProviderConfigConstructor {
	return map[string]ProviderConfigConstructor{
		"aws/iam-identity-center": providerConstructor(awsProviders.NewSSOProvider),
		"aws/saml":                providerConstructor(awsProviders.NewSAMLProvider),
		"azure/cli":               providerConstructor(azureProviders.NewCLIProvider),
		"azure/device-code":       providerConstructor(azureProviders.NewDeviceCodeProvider),
		"azure/interactive":       providerConstructor(azureProviders.NewInteractiveProvider),
		"azure/oidc":              providerConstructor(azureProviders.NewOIDCProvider),
		"github/oidc":             providerConstructor(githubProviders.NewOIDCProvider),
		"atmos/pro":               providerConstructor(atmosproProviders.NewProvider),
		"mock": func(name string, config *schema.Provider) (types.Provider, error) {
			return mockProviders.NewProvider(name, config), nil
		},
		"mock/aws": func(name string, config *schema.Provider) (types.Provider, error) {
			return mockawsProviders.NewProvider(name, config), nil
		},
	}
}

// builtinIdentities preserves the complete configuration accepted by built-in constructors.
func builtinIdentities() map[string]IdentityConfigConstructor {
	return map[string]IdentityConfigConstructor{
		"aws/permission-set":      identityConstructor(awsIdentities.NewPermissionSetIdentity),
		"aws/assume-role":         identityConstructor(awsIdentities.NewAssumeRoleIdentity),
		"aws/assume-root":         identityConstructor(awsIdentities.NewAssumeRootIdentity),
		"ambient":                 identityConstructor(ambientIdentities.NewAmbientIdentity),
		"aws/ambient":             identityConstructor(awsIdentities.NewAWSAmbientIdentity),
		"aws/user":                identityConstructor(awsIdentities.NewUserIdentity),
		types.IdentityKindOCIUser: identityConstructor(ociIdentities.NewIdentity),
		"aws/credential-process":  identityConstructor(awsIdentities.NewCredentialProcessIdentity),
		"atmos/pro":               identityConstructor(atmosproIdentities.NewIdentity),
		"azure/subscription":      identityConstructor(azureIdentities.NewSubscriptionIdentity),
		"azure/pim-role":          identityConstructor(azureIdentities.NewPIMRoleIdentity),
		"mock": func(name string, config *schema.Identity) (types.Identity, error) {
			return mockProviders.NewIdentity(name, config), nil
		},
		"mock/aws": func(name string, config *schema.Identity) (types.Identity, error) {
			return mockawsProviders.NewIdentity(name, config), nil
		},
	}
}

// Adapt concrete-returning constructors without losing their full configuration.
func providerConstructor[T types.Provider](constructor func(string, *schema.Provider) (T, error)) ProviderConfigConstructor {
	return func(name string, config *schema.Provider) (types.Provider, error) {
		value, err := constructor(name, config)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
}

func identityConstructor[T types.Identity](constructor func(string, *schema.Identity) (T, error)) IdentityConfigConstructor {
	return func(name string, config *schema.Identity) (types.Identity, error) {
		value, err := constructor(name, config)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
}
