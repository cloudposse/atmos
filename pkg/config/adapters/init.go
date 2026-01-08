// Package adapters contains import adapter implementations for the config package.
// Import this package to register all built-in adapters.
package adapters

import (
	"github.com/cloudposse/atmos/pkg/config"
)

func init() {
	// Set the function pointer to register adapters lazily.
	config.SetBuiltinAdaptersInitializer(registerBuiltinAdapters)
}

// registerBuiltinAdapters registers all built-in import adapters.
func registerBuiltinAdapters() {
	// Register GoGetterAdapter for remote imports.
	config.RegisterImportAdapter(&GoGetterAdapter{})

	// Register MockAdapter for testing (using the global singleton).
	config.RegisterImportAdapter(GetGlobalMockAdapter())

	// Register LocalAdapter as the default fallback.
	config.SetDefaultAdapter(&LocalAdapter{})
}
