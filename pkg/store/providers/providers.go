// Package providers contains the concrete store backend implementations
// (AWS SSM Parameter Store, Azure Key Vault, Google Secret Manager, Redis,
// Artifactory). Each backend registers itself with pkg/store via store.Register
// in its init() function, so importing this package — typically with a blank
// import — makes the built-in store types available to store.NewStoreRegistry.
package providers

import "github.com/go-viper/mapstructure/v2"

// Error format constants shared across store provider implementations.
const (
	errFormat           = "%w: %v"
	errWrapFormat       = "%w: %s"
	errWrapFormatWithID = "%w '%s': %s"
)

// parseOptions decodes a raw options map into the typed options struct for a store backend.
func parseOptions(options map[string]interface{}, target interface{}) error {
	return mapstructure.Decode(options, target)
}
