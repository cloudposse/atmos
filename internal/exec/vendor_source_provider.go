package exec

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// VendorSourceProvider defines the interface for vendor source operations.
// This interface allows for different implementations based on the source type (GitHub, GitLab, etc.).
type VendorSourceProvider interface {
	// GetAvailableVersions fetches all available versions/tags from the source.
	GetAvailableVersions(source string) ([]string, error)

	// VerifyVersion checks if a specific version exists in the source.
	VerifyVersion(source string, version string) (bool, error)

	// GetDiff generates a diff between two versions of a component.
	// Returns the diff output and an error if the operation is not supported or fails.
	GetDiff(atmosConfig *schema.AtmosConfiguration, source string, fromVersion string, toVersion string, filePath string, contextLines int, noColor bool) ([]byte, error)

	// SupportsOperation checks if the provider supports a specific operation.
	SupportsOperation(operation SourceOperation) bool
}

// SourceOperation represents different operations a vendor source provider can support.
type SourceOperation string

const (
	// OperationListVersions indicates the provider can list available versions.
	OperationListVersions SourceOperation = "list_versions"

	// OperationVerifyVersion indicates the provider can verify version existence.
	OperationVerifyVersion SourceOperation = "verify_version"

	// OperationGetDiff indicates the provider can generate diffs between versions.
	OperationGetDiff SourceOperation = "get_diff"

	// OperationFetchSource indicates the provider can fetch/download source code.
	OperationFetchSource SourceOperation = "fetch_source"
)

// GetProviderForSource returns the appropriate VendorSourceProvider for a given source URL.
func GetProviderForSource(source string) VendorSourceProvider {
	defer perf.Track(nil, "exec.GetProviderForSource")()

	// Determine provider type from source URL
	if isGitHubSource(source) {
		return NewGitHubSourceProvider()
	}

	// For all other Git sources, return a generic Git provider
	// (which has limited functionality compared to GitHub)
	if isGitSource(source) {
		return NewGenericGitSourceProvider()
	}

	// For non-Git sources (OCI, HTTP, local), return unsupported provider
	return NewUnsupportedSourceProvider()
}
