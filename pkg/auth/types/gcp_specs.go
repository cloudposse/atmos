package types

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GCPADCProviderSpec defines the spec for gcp/adc provider.
type GCPADCProviderSpec struct {
	ProjectID string   `json:"project_id,omitempty" yaml:"project_id,omitempty" mapstructure:"project_id"`
	Region    string   `json:"region,omitempty" yaml:"region,omitempty" mapstructure:"region"`
	Scopes    []string `json:"scopes,omitempty" yaml:"scopes,omitempty" mapstructure:"scopes"`
}

// WIFTokenSource defines where to obtain the OIDC token for workload identity federation.
type WIFTokenSource struct {
	// Type is "environment", "file", or "url".
	Type string `mapstructure:"type" json:"type" yaml:"type"`
	// EnvironmentVariable is the env var containing the token (for type=environment).
	EnvironmentVariable string `mapstructure:"environment_variable" json:"environment_variable,omitempty" yaml:"environment_variable,omitempty"`
	// FilePath is the path to the token file (for type=file).
	FilePath string `mapstructure:"file_path" json:"file_path,omitempty" yaml:"file_path,omitempty"`
	// URL is for fetching token via HTTP (e.g., GitHub Actions OIDC).
	URL string `mapstructure:"url" json:"url,omitempty" yaml:"url,omitempty"`
	// RequestToken is the bearer token for URL requests (e.g., GitHub Actions).
	RequestToken string `mapstructure:"request_token" json:"request_token,omitempty" yaml:"request_token,omitempty"`
	// Audience for the OIDC token request.
	Audience string `mapstructure:"audience" json:"audience,omitempty" yaml:"audience,omitempty"`
}

// GCPWorkloadIdentityFederationProviderSpec defines the spec for gcp/workload-identity-federation provider.
type GCPWorkloadIdentityFederationProviderSpec struct {
	// ProjectID is the GCP project ID.
	ProjectID string `mapstructure:"project_id" json:"project_id" yaml:"project_id"`
	// ProjectNumber is the GCP project number (required for WIF).
	ProjectNumber string `mapstructure:"project_number" json:"project_number" yaml:"project_number"`
	// WorkloadIdentityPoolID is the WIF pool ID (preferred over WorkloadIdentityPool).
	WorkloadIdentityPoolID string `mapstructure:"workload_identity_pool_id" json:"workload_identity_pool_id" yaml:"workload_identity_pool_id"`
	// WorkloadIdentityProviderID is the WIF provider ID (preferred over WorkloadIdentityProvider).
	WorkloadIdentityProviderID string `mapstructure:"workload_identity_provider_id" json:"workload_identity_provider_id" yaml:"workload_identity_provider_id"`
	// WorkloadIdentityPool is the WIF pool ID (legacy; use WorkloadIdentityPoolID).
	WorkloadIdentityPool string `mapstructure:"workload_identity_pool" json:"workload_identity_pool,omitempty" yaml:"workload_identity_pool,omitempty"`
	// WorkloadIdentityProvider is the WIF provider ID (legacy; use WorkloadIdentityProviderID).
	WorkloadIdentityProvider string `mapstructure:"workload_identity_provider" json:"workload_identity_provider,omitempty" yaml:"workload_identity_provider,omitempty"`
	// ServiceAccountEmail is the SA to impersonate (optional but typical).
	ServiceAccountEmail string `mapstructure:"service_account_email" json:"service_account_email" yaml:"service_account_email"`
	// TokenSource defines where to get the OIDC token.
	TokenSource *WIFTokenSource `mapstructure:"token_source" json:"token_source" yaml:"token_source"`
	// Scopes for the final access token.
	Scopes []string `mapstructure:"scopes" json:"scopes,omitempty" yaml:"scopes,omitempty"`
	// Region for regional endpoints (optional).
	Region string `mapstructure:"region" json:"region,omitempty" yaml:"region,omitempty"`
}

// GCPServiceAccountIdentityPrincipal defines the principal for gcp/service-account identity.
type GCPServiceAccountIdentityPrincipal struct {
	// ServiceAccountEmail is the target service account to impersonate.
	ServiceAccountEmail string `json:"service_account_email" yaml:"service_account_email" mapstructure:"service_account_email"`
	// Delegates is an optional chain of service accounts for delegation.
	// The provider's identity impersonates delegates[0], which impersonates delegates[1], etc.
	Delegates []string `json:"delegates,omitempty" yaml:"delegates,omitempty" mapstructure:"delegates"`
	// Scopes for the generated access token.
	Scopes []string `json:"scopes,omitempty" yaml:"scopes,omitempty" mapstructure:"scopes"`
	// Lifetime is the token lifetime (e.g., "3600s"). Default is 1 hour.
	Lifetime string `json:"lifetime,omitempty" yaml:"lifetime,omitempty" mapstructure:"lifetime"`
	// ProjectID to set in the auth context (optional, derived from SA email if not set).
	ProjectID string `json:"project_id,omitempty" yaml:"project_id,omitempty" mapstructure:"project_id"`
}

// GCPProjectIdentityPrincipal defines the principal for gcp/project identity.
type GCPProjectIdentityPrincipal struct {
	// ProjectID is the GCP project ID to target (required).
	ProjectID string `json:"project_id" yaml:"project_id" mapstructure:"project_id"`
	// Region is the default GCP region (optional).
	Region string `json:"region,omitempty" yaml:"region,omitempty" mapstructure:"region"`
	// Zone is the default GCP zone (optional).
	Zone string `json:"zone,omitempty" yaml:"zone,omitempty" mapstructure:"zone"`
	// Location is the GCP location (optional; legacy, prefer Region/Zone).
	Location string `json:"location,omitempty" yaml:"location,omitempty" mapstructure:"location"`
}

// ParseGCPADCProviderSpec parses a map into GCPADCProviderSpec.
func ParseGCPADCProviderSpec(spec map[string]any) (*GCPADCProviderSpec, error) {
	defer perf.Track(nil, "types.ParseGCPADCProviderSpec")()

	if spec == nil {
		return nil, fmt.Errorf("%w: spec is nil", errUtils.ErrInvalidAuthConfig)
	}

	var out GCPADCProviderSpec
	if err := mapstructure.Decode(spec, &out); err != nil {
		return nil, fmt.Errorf("%w: failed to decode GCP ADC provider spec: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return &out, nil
}

// ParseGCPWorkloadIdentityFederationProviderSpec parses the WIF provider spec.
func ParseGCPWorkloadIdentityFederationProviderSpec(spec map[string]any) (*GCPWorkloadIdentityFederationProviderSpec, error) {
	defer perf.Track(nil, "types.ParseGCPWorkloadIdentityFederationProviderSpec")()

	if spec == nil {
		return nil, fmt.Errorf("%w: spec is nil", errUtils.ErrInvalidAuthConfig)
	}

	var out GCPWorkloadIdentityFederationProviderSpec
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &out,
		WeaklyTypedInput: false,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create decoder: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	if err := decoder.Decode(spec); err != nil {
		return nil, fmt.Errorf("%w: decode spec: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	return &out, nil
}

// ParseGCPServiceAccountIdentityPrincipal parses the service account identity principal.
func ParseGCPServiceAccountIdentityPrincipal(principal map[string]any) (*GCPServiceAccountIdentityPrincipal, error) {
	defer perf.Track(nil, "types.ParseGCPServiceAccountIdentityPrincipal")()

	if principal == nil {
		return nil, fmt.Errorf("%w: principal is nil", errUtils.ErrInvalidAuthConfig)
	}

	var out GCPServiceAccountIdentityPrincipal
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &out,
		WeaklyTypedInput: false,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create decoder: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	if err := decoder.Decode(principal); err != nil {
		return nil, fmt.Errorf("%w: decode principal: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	if out.ServiceAccountEmail == "" {
		return nil, fmt.Errorf("%w: service_account_email is required for GCP service account identity", errUtils.ErrInvalidAuthConfig)
	}

	return &out, nil
}

// ParseGCPProjectIdentityPrincipal parses the project identity principal.
func ParseGCPProjectIdentityPrincipal(principal map[string]any) (*GCPProjectIdentityPrincipal, error) {
	defer perf.Track(nil, "types.ParseGCPProjectIdentityPrincipal")()

	if principal == nil {
		return nil, fmt.Errorf("%w: principal is nil", errUtils.ErrInvalidAuthConfig)
	}

	var out GCPProjectIdentityPrincipal
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &out,
		WeaklyTypedInput: false,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create decoder: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	if err := decoder.Decode(principal); err != nil {
		return nil, fmt.Errorf("%w: decode principal: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	if out.ProjectID == "" {
		return nil, fmt.Errorf("%w: project_id is required for GCP project identity", errUtils.ErrInvalidAuthConfig)
	}

	return &out, nil
}
