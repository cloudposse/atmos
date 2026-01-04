package source

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// GenericGitProvider implements Provider for generic Git repositories.
// This provider has limited functionality compared to GitHub provider.
type GenericGitProvider struct{}

// NewGenericGitProvider creates a new generic Git source provider.
func NewGenericGitProvider() Provider {
	defer perf.Track(nil, "source.NewGenericGitProvider")()

	return &GenericGitProvider{}
}

// GetAvailableVersions implements Provider.GetAvailableVersions.
func (g *GenericGitProvider) GetAvailableVersions(source string) ([]string, error) {
	defer perf.Track(nil, "source.GenericGitProvider.GetAvailableVersions")()

	gitURI := version.ExtractGitURI(source)
	return version.GetGitRemoteTags(gitURI)
}

// VerifyVersion implements Provider.VerifyVersion.
func (g *GenericGitProvider) VerifyVersion(source string, ver string) (bool, error) {
	defer perf.Track(nil, "source.GenericGitProvider.VerifyVersion")()

	gitURI := version.ExtractGitURI(source)
	return version.CheckGitRef(gitURI, ver)
}

// GetDiff implements Provider.GetDiff.
// For generic Git providers, diff functionality is not implemented.
//
//nolint:revive // Seven parameters needed for interface compatibility.
func (g *GenericGitProvider) GetDiff(
	atmosConfig *schema.AtmosConfiguration,
	source string,
	fromVersion string,
	toVersion string,
	filePath string,
	contextLines int,
	noColor bool,
) ([]byte, error) {
	defer perf.Track(atmosConfig, "source.GenericGitProvider.GetDiff")()

	return nil, fmt.Errorf("%w: diff functionality is only supported for GitHub sources", errUtils.ErrNotImplemented)
}

// SupportsOperation implements Provider.SupportsOperation.
func (g *GenericGitProvider) SupportsOperation(operation Operation) bool {
	defer perf.Track(nil, "source.GenericGitProvider.SupportsOperation")()

	switch operation {
	case OperationListVersions, OperationVerifyVersion, OperationFetchSource:
		return true
	case OperationGetDiff:
		return false // Not implemented for generic Git.
	default:
		return false
	}
}

// IsGitSource checks if a source URL is a Git repository.
func IsGitSource(source string) bool {
	defer perf.Track(nil, "source.IsGitSource")()

	return strings.HasPrefix(source, "git::") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git")
}
