package config

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ImportAdapter handles import resolution for specific URL schemes.
// All import handling goes through the adapter registry - there are no special cases.
//
// Built-in adapters:
//   - GoGetterAdapter: http://, https://, git::, s3::, oci://, github.com/, etc.
//   - LocalAdapter: Local filesystem paths (default fallback)
//   - MockAdapter: mock:// scheme for testing
//
// Future adapters:
//   - TerragruntAdapter: terragrunt:// for HCL→YAML transformation
type ImportAdapter interface {
	// Schemes returns the URL schemes/prefixes this adapter handles.
	// Examples: []string{"http://", "https://", "git::", "github.com/"}
	//
	// Return nil or empty slice for the default adapter (LocalAdapter),
	// which handles paths without recognized schemes.
	Schemes() []string

	// Resolve processes an import path and returns resolved file paths.
	//
	// Parameters:
	//   - ctx: Context for cancellation and deadlines
	//   - importPath: The full import path (e.g., "git::https://github.com/org/repo//path")
	//   - basePath: The base path for resolving relative references
	//   - tempDir: Temporary directory for downloaded/generated files
	//   - currentDepth: Current recursion depth for nested imports
	//   - maxDepth: Maximum allowed recursion depth
	//   - atmosConfig: The Atmos configuration being assembled, so adapters can honor
	//     auth settings (e.g., GitHub token injection and the Atmos Pro credential
	//     broker) when fetching remote imports. May be nil.
	//
	// Returns:
	//   - []ResolvedPaths: List of resolved file paths to merge
	//   - error: Any error encountered during resolution
	//
	// Adapters are responsible for handling nested imports by calling
	// processImports() recursively when the resolved config contains
	// further import statements.
	Resolve(
		ctx context.Context,
		importPath string,
		basePath string,
		tempDir string,
		currentDepth int,
		maxDepth int,
		atmosConfig *schema.AtmosConfiguration,
	) ([]ResolvedPaths, error)
}
