package source

//go:generate mockgen -source=$GOFILE -destination=mock_provider.go -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider defines the interface for vendor source operations.
// This interface allows for different implementations based on the source type (GitHub, GitLab, etc.).
type Provider interface {
	// GetAvailableVersions fetches all available versions/tags from the source.
	GetAvailableVersions(source string) ([]string, error)

	// VerifyVersion checks if a specific version exists in the source.
	VerifyVersion(source string, version string) (bool, error)

	// GetDiff generates a diff between two versions of a component.
	// Returns the diff output and an error if the operation is not supported or fails.
	GetDiff(atmosConfig *schema.AtmosConfiguration, source string, fromVersion string, toVersion string, filePath string, contextLines int, noColor bool) ([]byte, error)

	// SupportsOperation checks if the provider supports a specific operation.
	SupportsOperation(operation Operation) bool
}

// Operation represents different operations a vendor source provider can support.
type Operation string

const (
	// OperationListVersions indicates the provider can list available versions.
	OperationListVersions Operation = "list_versions"

	// OperationVerifyVersion indicates the provider can verify version existence.
	OperationVerifyVersion Operation = "verify_version"

	// OperationGetDiff indicates the provider can generate diffs between versions.
	OperationGetDiff Operation = "get_diff"

	// OperationFetchSource indicates the provider can fetch/download source code.
	OperationFetchSource Operation = "fetch_source"
)

// GetProviderForSource returns the appropriate Provider for a given source URL.
func GetProviderForSource(source string) Provider {
	defer perf.Track(nil, "source.GetProviderForSource")()

	// Determine provider type from source URL.
	if IsGitHubSource(source) {
		return NewGitHubProvider()
	}

	// For all other Git sources, return a generic Git provider
	// (which has limited functionality compared to GitHub).
	if IsGitSource(source) {
		return NewGenericGitProvider()
	}

	// For non-Git sources (OCI, HTTP, local), return unsupported provider.
	return NewUnsupportedProvider()
}
