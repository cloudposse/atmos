package vendoring

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GenericGitSourceProvider implements VendorSourceProvider for generic Git repositories.
// This provider has limited functionality compared to GitHub provider.
type GenericGitSourceProvider struct{}

// NewGenericGitSourceProvider creates a new generic Git source provider.
func NewGenericGitSourceProvider() VendorSourceProvider {
	return &GenericGitSourceProvider{}
}

// GetAvailableVersions implements VendorSourceProvider.GetAvailableVersions.
func (g *GenericGitSourceProvider) GetAvailableVersions(source string) ([]string, error) {
	gitURI := extractGitURI(source)
	return getGitRemoteTags(gitURI)
}

// VerifyVersion implements VendorSourceProvider.VerifyVersion.
func (g *GenericGitSourceProvider) VerifyVersion(source string, version string) (bool, error) {
	gitURI := extractGitURI(source)
	return checkGitRef(gitURI, version)
}

// GetDiff implements VendorSourceProvider.GetDiff.
// For generic Git providers, diff functionality is not implemented.
//
//nolint:revive // Seven parameters needed for interface compatibility.
func (g *GenericGitSourceProvider) GetDiff(
	atmosConfig *schema.AtmosConfiguration,
	source string,
	fromVersion string,
	toVersion string,
	filePath string,
	contextLines int,
	noColor bool,
) ([]byte, error) {
	return nil, fmt.Errorf("%w: diff functionality is only supported for GitHub sources", errUtils.ErrNotImplemented)
}

// SupportsOperation implements VendorSourceProvider.SupportsOperation.
func (g *GenericGitSourceProvider) SupportsOperation(operation SourceOperation) bool {
	switch operation {
	case OperationListVersions, OperationVerifyVersion, OperationFetchSource:
		return true
	case OperationGetDiff:
		return false // Not implemented for generic Git
	default:
		return false
	}
}

// isGitSource checks if a source URL is a Git repository.
func isGitSource(source string) bool {
	return strings.HasPrefix(source, "git::") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git")
}
