package install

import "github.com/cloudposse/atmos/pkg/auth/registryauth"

// Option configures optional composition services for installation.
type Option func(*serviceOptions)

type serviceOptions struct{ registryPreparation registryauth.Preparation }

// WithRegistryAuthPreparation allows verified local auth plugins to bootstrap registry access.
//
//nolint:lintroller // Functional option only.
func WithRegistryAuthPreparation(prepare registryauth.Preparation) Option {
	return func(options *serviceOptions) { options.registryPreparation = prepare }
}
