package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseGCPADCProviderSpec_Valid(t *testing.T) {
	spec := map[string]any{
		"project_id": "my-project",
		"region":     "us-central1",
	}

	out, err := ParseGCPADCProviderSpec(spec)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, "my-project", out.ProjectID)
	assert.Equal(t, "us-central1", out.Region)
}

func TestParseGCPADCProviderSpec_NilSpec(t *testing.T) {
	out, err := ParseGCPADCProviderSpec(nil)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestParseGCPADCProviderSpec_EmptySpec(t *testing.T) {
	out, err := ParseGCPADCProviderSpec(map[string]any{})
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Empty(t, out.ProjectID)
	assert.Empty(t, out.Region)
}

func TestParseGCPWorkloadIdentityFederationProviderSpec_Valid(t *testing.T) {
	spec := map[string]any{
		"project_id":                 "my-project",
		"project_number":             "123456789",
		"workload_identity_pool":     "my-pool",
		"workload_identity_provider": "my-provider",
		"service_account_email":      "sa@my-project.iam.gserviceaccount.com",
	}

	out, err := ParseGCPWorkloadIdentityFederationProviderSpec(spec)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, "my-project", out.ProjectID)
	assert.Equal(t, "123456789", out.ProjectNumber)
	assert.Equal(t, "my-pool", out.WorkloadIdentityPool)
	assert.Equal(t, "my-provider", out.WorkloadIdentityProvider)
	assert.Equal(t, "sa@my-project.iam.gserviceaccount.com", out.ServiceAccountEmail)
}

func TestParseGCPWorkloadIdentityFederationProviderSpec_ValidWithPoolIDProviderID(t *testing.T) {
	spec := map[string]any{
		"project_id":                    "my-project",
		"project_number":                "123456789",
		"workload_identity_pool_id":     "github-pool",
		"workload_identity_provider_id": "github-provider",
		"service_account_email":         "deploy@my-project.iam.gserviceaccount.com",
		"token_source": map[string]any{
			"type":                "environment",
			"environment_variable": "ACTIONS_ID_TOKEN_REQUEST_TOKEN",
		},
		"scopes": []any{"https://www.googleapis.com/auth/cloud-platform"},
	}

	out, err := ParseGCPWorkloadIdentityFederationProviderSpec(spec)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, "my-project", out.ProjectID)
	assert.Equal(t, "123456789", out.ProjectNumber)
	assert.Equal(t, "github-pool", out.WorkloadIdentityPoolID)
	assert.Equal(t, "github-provider", out.WorkloadIdentityProviderID)
	assert.NotNil(t, out.TokenSource)
	assert.Equal(t, "environment", out.TokenSource.Type)
	assert.Equal(t, "ACTIONS_ID_TOKEN_REQUEST_TOKEN", out.TokenSource.EnvironmentVariable)
	assert.Len(t, out.Scopes, 1)
}

func TestParseGCPWorkloadIdentityFederationProviderSpec_OptionalServiceAccount(t *testing.T) {
	// Parser allows optional service_account_email; provider validates required WIF fields.
	spec := map[string]any{
		"project_number":                "123",
		"workload_identity_pool_id":      "p",
		"workload_identity_provider_id": "pr",
	}

	out, err := ParseGCPWorkloadIdentityFederationProviderSpec(spec)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, "123", out.ProjectNumber)
	assert.Equal(t, "p", out.WorkloadIdentityPoolID)
	assert.Equal(t, "pr", out.WorkloadIdentityProviderID)
	assert.Empty(t, out.ServiceAccountEmail)
}

func TestParseGCPWorkloadIdentityFederationProviderSpec_NilSpec(t *testing.T) {
	out, err := ParseGCPWorkloadIdentityFederationProviderSpec(nil)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestParseGCPServiceAccountIdentityPrincipal_Valid(t *testing.T) {
	principal := map[string]any{
		"service_account_email": "sa@my-project.iam.gserviceaccount.com",
		"scopes":                 []any{"https://www.googleapis.com/auth/cloud-platform"},
		"lifetime":               "3600s",
		"delegates":              []any{"delegate@project.iam.gserviceaccount.com"},
	}

	out, err := ParseGCPServiceAccountIdentityPrincipal(principal)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, "sa@my-project.iam.gserviceaccount.com", out.ServiceAccountEmail)
	assert.Len(t, out.Scopes, 1)
	assert.Equal(t, "3600s", out.Lifetime)
	assert.Len(t, out.Delegates, 1)
}

func TestParseGCPServiceAccountIdentityPrincipal_MissingServiceAccountEmail(t *testing.T) {
	principal := map[string]any{"scopes": []any{}}

	out, err := ParseGCPServiceAccountIdentityPrincipal(principal)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestParseGCPServiceAccountIdentityPrincipal_NilPrincipal(t *testing.T) {
	out, err := ParseGCPServiceAccountIdentityPrincipal(nil)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestParseGCPProjectIdentityPrincipal_Valid(t *testing.T) {
	principal := map[string]any{
		"project_id": "my-project",
		"region":     "us-central1",
		"location":   "us",
	}

	out, err := ParseGCPProjectIdentityPrincipal(principal)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Equal(t, "my-project", out.ProjectID)
	assert.Equal(t, "us-central1", out.Region)
	assert.Equal(t, "us", out.Location)
}

func TestParseGCPProjectIdentityPrincipal_MissingProjectID(t *testing.T) {
	principal := map[string]any{"region": "us-central1"}

	out, err := ParseGCPProjectIdentityPrincipal(principal)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestParseGCPProjectIdentityPrincipal_NilPrincipal(t *testing.T) {
	out, err := ParseGCPProjectIdentityPrincipal(nil)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestParseGCPADCProviderSpec_InvalidType(t *testing.T) {
	// Invalid type for a field (e.g. region as slice) should return an error.
	// mapstructure correctly rejects unconvertible types.
	spec := map[string]any{"project_id": "p", "region": []any{"a", "b"}}

	out, err := ParseGCPADCProviderSpec(spec)
	assert.Error(t, err)
	assert.Nil(t, out)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}
