// Package providers contains the concrete store backend implementations
// (AWS SSM Parameter Store, AWS Secrets Manager, Azure Key Vault, Google Secret
// Manager, HashiCorp Vault, Redis, Artifactory, 1Password, Keychain, GitHub
// Actions). Each backend registers itself with pkg/store via store.Register in
// its init() function, so importing this package — typically with a blank
// import — makes the built-in store kinds available to store.NewStoreRegistry.
package providers

import "github.com/go-viper/mapstructure/v2"

// Error format constants shared across store provider implementations.
const (
	errFormat           = "%w: %v"
	errWrapFormat       = "%w: %s"
	errWrapFormatWithID = "%w '%s': %s"
	// The errParseFmt format wraps an option-parsing error so callers can use
	// errors.Is on both the sentinel and the underlying error.
	errParseFmt = "%w: %w"
)

// parseOptions decodes a raw options map into the typed options struct for a store backend.
func parseOptions(options map[string]interface{}, target interface{}) error {
	return mapstructure.Decode(options, target)
}
