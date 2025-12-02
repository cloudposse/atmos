package vendoring

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// UnsupportedSourceProvider implements VendorSourceProvider for unsupported source types.
// This includes OCI registries, local files, HTTP sources, etc.
type UnsupportedSourceProvider struct{}

// NewUnsupportedSourceProvider creates a new unsupported source provider.
func NewUnsupportedSourceProvider() VendorSourceProvider {
	return &UnsupportedSourceProvider{}
}

// GetAvailableVersions implements VendorSourceProvider.GetAvailableVersions.
func (u *UnsupportedSourceProvider) GetAvailableVersions(source string) ([]string, error) {
	defer perf.Track(nil, "exec.UnsupportedSourceProvider.GetAvailableVersions")()

	return nil, fmt.Errorf("%w: version listing not supported for this source type", errUtils.ErrUnsupportedVendorSource)
}

// VerifyVersion implements VendorSourceProvider.VerifyVersion.
func (u *UnsupportedSourceProvider) VerifyVersion(source string, version string) (bool, error) {
	defer perf.Track(nil, "exec.UnsupportedSourceProvider.VerifyVersion")()

	return false, fmt.Errorf("%w: version verification not supported for this source type", errUtils.ErrUnsupportedVendorSource)
}

// GetDiff implements VendorSourceProvider.GetDiff.
//
//nolint:revive // Seven parameters needed for interface compatibility.
func (u *UnsupportedSourceProvider) GetDiff(
	atmosConfig *schema.AtmosConfiguration,
	source string,
	fromVersion string,
	toVersion string,
	filePath string,
	contextLines int,
	noColor bool,
) ([]byte, error) {
	defer perf.Track(atmosConfig, "exec.UnsupportedSourceProvider.GetDiff")()

	return nil, fmt.Errorf("%w: diff functionality not supported for this source type", errUtils.ErrUnsupportedVendorSource)
}

// SupportsOperation implements VendorSourceProvider.SupportsOperation.
func (u *UnsupportedSourceProvider) SupportsOperation(operation SourceOperation) bool {
	return false
}
