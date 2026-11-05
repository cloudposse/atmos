package types

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// OCICredentials holds host-scoped registry credentials in the Atmos auth keyring.
// It is never included in serialized WhoamiInfo; consumers request RegistryAuth explicitly.
type OCICredentials struct {
	Registry      string     `json:"registry"`
	Username      string     `json:"username,omitempty"`
	Password      string     `json:"password,omitempty"`
	IdentityToken string     `json:"identity_token,omitempty"`
	RegistryToken string     `json:"registry_token,omitempty"`
	Expiration    *time.Time `json:"expiration,omitempty"`
}

// RegistryHost returns the sole registry authorized to receive these credentials.
func (c *OCICredentials) RegistryHost() string { return c.Registry }

// RegistryAuth exposes the registry authentication fields to the OCI transport.
//
//nolint:revive // function-result-limit: preserves the registry credential transport interface.
func (c *OCICredentials) RegistryAuth() (username, password, identityToken, registryToken string) {
	return c.Username, c.Password, c.IdentityToken, c.RegistryToken
}

// IsExpired reports invalid credentials or an elapsed optional expiry.
func (c *OCICredentials) IsExpired() bool {
	return c.Registry == "" || (c.Password == "" && c.IdentityToken == "" && c.RegistryToken == "") || (c.Expiration != nil && !time.Now().Before(*c.Expiration))
}

// GetExpiration returns the optional expiry (PATs generally do not expose one).
func (c *OCICredentials) GetExpiration() (*time.Time, error) { return c.Expiration, nil }

// BuildWhoamiInfo adds only the non-secret registry and username.
func (c *OCICredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info != nil {
		info.Account, info.Principal, info.Expiration = c.Registry, c.Username, c.Expiration
	}
}

// Validate cannot verify registry permissions without a target repository operation.
func (c *OCICredentials) Validate(context.Context) (*ValidationInfo, error) {
	return nil, errUtils.ErrNotImplemented
}

// String permits safe diagnostic formatting without exposing registry secrets.
func (c *OCICredentials) String() string {
	if c == nil {
		return "OCICredentials(nil)"
	}
	return fmt.Sprintf("OCICredentials{Registry: %q, Username: %q}", c.Registry, c.Username)
}
