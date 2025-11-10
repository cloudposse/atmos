package types

import (
	"github.com/cloudposse/atmos/pkg/auth/provisioning"
)

// ProvisioningResult is an alias for provisioning.Result.
// This allows the manager to use types.ProvisioningResult while the actual.
// implementation lives in pkg/auth/provisioning.
type ProvisioningResult = provisioning.Result

// ProvisioningWriter is an alias for provisioning.Writer.
type ProvisioningWriter = provisioning.Writer

// ProvisioningMetadata is an alias for provisioning.Metadata.
type ProvisioningMetadata = provisioning.Metadata

// ProvisioningCounts is an alias for provisioning.Counts.
type ProvisioningCounts = provisioning.Counts

// NewProvisioningWriter creates a new provisioning writer.
func NewProvisioningWriter() (*ProvisioningWriter, error) {
	return provisioning.NewWriter()
}
