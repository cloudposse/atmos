package provisioning

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ICredentials is an interface for credentials.
// This is a minimal interface to avoid circular dependencies.
type ICredentials interface {
	GetProvider() string
	GetExpiration() (*time.Time, error)
}

// Provisioner is an optional interface that auth providers can implement
// to auto-provision identities from external sources.
type Provisioner interface {
	// ProvisionIdentities provisions identities from the external source.
	// Returns provisioned identities and metadata, or error if provisioning fails.
	// Implementations should be non-fatal - errors are logged but don't block authentication.
	ProvisionIdentities(ctx context.Context, creds ICredentials) (*Result, error)
}

// Result contains the provisioned identities and metadata.
type Result struct {
	// Identities maps identity names to their configuration.
	Identities map[string]*schema.Identity

	// Provider is the name of the auth provider that provisioned these identities.
	Provider string

	// ProvisionedAt is when the identities were provisioned.
	ProvisionedAt time.Time

	// Metadata contains provider-specific provisioning information.
	Metadata Metadata
}

// Metadata contains provider-specific metadata about the provisioning operation.
type Metadata struct {
	// Source identifies the external source (e.g., "aws-sso", "okta", "azure-ad").
	Source string

	// Counts provides statistics about provisioned identities.
	Counts *Counts

	// Extra holds provider-specific metadata (e.g., AWS account IDs, Okta org ID).
	Extra map[string]interface{}
}

// Counts provides statistics about the provisioning operation.
type Counts struct {
	// Accounts is the number of accounts/organizations discovered.
	Accounts int

	// Roles is the number of permission sets/roles discovered.
	Roles int

	// Identities is the total number of identities provisioned.
	Identities int
}
