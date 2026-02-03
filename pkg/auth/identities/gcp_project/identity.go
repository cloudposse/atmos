package gcp_project

import (
	"context"
	"fmt"
	"maps"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// IdentityKind is the kind identifier for this identity.
	IdentityKind = types.IdentityKindGCPProject // "gcp/project"
)

// Identity implements the gcp/project identity.
// This identity sets project context without authentication.
type Identity struct {
	name      string
	principal *types.GCPProjectIdentityPrincipal
	config    *schema.Identity
}

// New creates a new project identity.
func New(principal *types.GCPProjectIdentityPrincipal) (*Identity, error) {
	defer perf.Track(nil, "gcp_project.New")()

	if principal == nil {
		return nil, fmt.Errorf("%w: project principal cannot be nil", errUtils.ErrInvalidIdentityConfig)
	}
	return &Identity{principal: principal}, nil
}

// SetName sets the identity name.
func (i *Identity) SetName(name string) {
	i.name = name
}

// Kind returns the identity kind.
func (i *Identity) Kind() string {
	return IdentityKind
}

// Name returns the identity name.
func (i *Identity) Name() string {
	if i.name != "" {
		return i.name
	}
	return i.Kind()
}

// SetConfig sets the identity configuration (for Via.Provider resolution).
func (i *Identity) SetConfig(config *schema.Identity) {
	i.config = config
}

// GetProviderName returns the provider name from config, or empty string.
// GCP project identities may or may not have a provider (via.provider is optional).
func (i *Identity) GetProviderName() (string, error) {
	if i.config != nil && i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	return "", nil
}

// Validate validates the identity configuration.
func (i *Identity) Validate() error {
	defer perf.Track(nil, "gcp_project.Validate")()

	if i.principal == nil {
		return fmt.Errorf("%w: principal is nil", errUtils.ErrInvalidIdentityConfig)
	}
	if i.principal.ProjectID == "" {
		return fmt.Errorf("%w: project_id is required", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// Authenticate returns credentials with project context (no authentication performed).
// The project identity only sets context; it passes through base credentials if provided.
func (i *Identity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "gcp_project.Authenticate")()

	if err := i.Validate(); err != nil {
		return nil, err
	}

	// If base credentials provided, pass them through with updated project.
	if baseCreds != nil {
		if gcpCreds, ok := baseCreds.(*types.GCPCredentials); ok {
			return &types.GCPCredentials{
				AccessToken:         gcpCreds.AccessToken,
				TokenExpiry:         gcpCreds.TokenExpiry,
				ProjectID:           i.principal.ProjectID,
				ServiceAccountEmail: gcpCreds.ServiceAccountEmail,
				Scopes:              gcpCreds.Scopes,
			}, nil
		}
	}

	// No base credentials - return minimal credentials with just project info.
	return &types.GCPCredentials{
		ProjectID: i.principal.ProjectID,
	}, nil
}

// Environment returns environment variables for this identity.
func (i *Identity) Environment() (map[string]string, error) {
	defer perf.Track(nil, "gcp_project.Environment")()

	if i.principal == nil {
		return nil, nil
	}

	env := make(map[string]string)

	if i.principal.ProjectID != "" {
		env["GOOGLE_CLOUD_PROJECT"] = i.principal.ProjectID
		env["GCLOUD_PROJECT"] = i.principal.ProjectID
		env["CLOUDSDK_CORE_PROJECT"] = i.principal.ProjectID
	}

	if i.principal.Region != "" {
		env["GOOGLE_CLOUD_REGION"] = i.principal.Region
		env["CLOUDSDK_COMPUTE_REGION"] = i.principal.Region
	}

	if i.principal.Zone != "" {
		env["GOOGLE_CLOUD_ZONE"] = i.principal.Zone
		env["CLOUDSDK_COMPUTE_ZONE"] = i.principal.Zone
	}

	// Location is a legacy field that maps to zone if zone is not set.
	if i.principal.Location != "" && i.principal.Zone == "" {
		env["GOOGLE_CLOUD_ZONE"] = i.principal.Location
		env["CLOUDSDK_COMPUTE_ZONE"] = i.principal.Location
	}

	return env, nil
}

// Paths returns empty (no credential files).
func (i *Identity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PostAuthenticate sets up environment variables and populates auth context.
func (i *Identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "gcp_project.PostAuthenticate")()

	env, err := i.Environment()
	if err != nil {
		return err
	}
	for k, v := range env {
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("set environment variable %s: %w", k, err)
		}
	}

	if params != nil && params.AuthContext != nil && i.principal != nil {
		params.AuthContext.GCP = &schema.GCPAuthContext{
			ProjectID: i.principal.ProjectID,
			Region:    i.principal.Region,
		}
		if params.Credentials != nil {
			if gcpCreds, ok := params.Credentials.(*types.GCPCredentials); ok {
				params.AuthContext.GCP.AccessToken = gcpCreds.AccessToken
				params.AuthContext.GCP.TokenExpiry = gcpCreds.TokenExpiry
				params.AuthContext.GCP.ServiceAccountEmail = gcpCreds.ServiceAccountEmail
			}
		}
	}

	return nil
}

// PrepareEnvironment merges identity environment into the given map.
func (i *Identity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "gcp_project.PrepareEnvironment")()

	result := maps.Clone(environ)
	if result == nil {
		result = make(map[string]string)
	}

	env, err := i.Environment()
	if err != nil {
		return nil, err
	}
	for k, v := range env {
		result[k] = v
	}

	return result, nil
}

// Logout is a no-op (no credentials to clean up).
func (i *Identity) Logout(ctx context.Context) error {
	return nil
}

// CredentialsExist returns true (no credentials needed).
func (i *Identity) CredentialsExist() (bool, error) {
	return true, nil
}

// LoadCredentials returns minimal credentials with project info (no stored credentials).
func (i *Identity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	if i.principal == nil {
		return nil, nil
	}
	return &types.GCPCredentials{
		ProjectID: i.principal.ProjectID,
	}, nil
}
