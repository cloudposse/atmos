package source

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// UnsupportedProvider implements Provider for unsupported source types.
// This includes OCI registries, local files, HTTP sources, etc.
type UnsupportedProvider struct{}

// NewUnsupportedProvider creates a new unsupported source provider.
func NewUnsupportedProvider() Provider {
	defer perf.Track(nil, "source.NewUnsupportedProvider")()

	return &UnsupportedProvider{}
}

// GetAvailableVersions implements Provider.GetAvailableVersions.
func (u *UnsupportedProvider) GetAvailableVersions(source string) ([]string, error) {
	defer perf.Track(nil, "source.UnsupportedProvider.GetAvailableVersions")()

	return nil, fmt.Errorf("%w: version listing not supported for this source type", errUtils.ErrUnsupportedVendorSource)
}

// VerifyVersion implements Provider.VerifyVersion.
func (u *UnsupportedProvider) VerifyVersion(source string, version string) (bool, error) {
	defer perf.Track(nil, "source.UnsupportedProvider.VerifyVersion")()

	return false, fmt.Errorf("%w: version verification not supported for this source type", errUtils.ErrUnsupportedVendorSource)
}

// GetDiff implements Provider.GetDiff.
//
//nolint:revive // Seven parameters needed for interface compatibility.
func (u *UnsupportedProvider) GetDiff(
	atmosConfig *schema.AtmosConfiguration,
	source string,
	fromVersion string,
	toVersion string,
	filePath string,
	contextLines int,
	noColor bool,
) ([]byte, error) {
	defer perf.Track(atmosConfig, "source.UnsupportedProvider.GetDiff")()

	return nil, fmt.Errorf("%w: diff functionality not supported for this source type", errUtils.ErrUnsupportedVendorSource)
}

// SupportsOperation implements Provider.SupportsOperation.
func (u *UnsupportedProvider) SupportsOperation(operation Operation) bool {
	defer perf.Track(nil, "source.UnsupportedProvider.SupportsOperation")()

	return false
}
