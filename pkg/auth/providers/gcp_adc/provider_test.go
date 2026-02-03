package gcp_adc

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestNew(t *testing.T) {
	spec := &types.GCPADCProviderSpec{
		ProjectID: "test-project",
		Region:    "us-central1",
	}
	p, err := New(spec)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, ProviderKind, p.Kind())
}

func TestNew_NilSpec(t *testing.T) {
	p, err := New(nil)
	require.Error(t, err)
	assert.Nil(t, p)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidProviderConfig))
	assert.Contains(t, err.Error(), "nil")
}

func TestProvider_Kind(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	assert.Equal(t, "gcp/adc", p.Kind())
}

func TestAuthenticate_NoCredentials(t *testing.T) {
	// This test verifies that Authenticate fails when no credentials are available.
	// However, ADC can find credentials from multiple sources:
	// 1. GOOGLE_APPLICATION_CREDENTIALS env var
	// 2. gcloud application-default credentials (~/.config/gcloud/application_default_credentials.json)
	// 3. GCP metadata server (on GCP VMs/containers)
	//
	// We can only test the "no credentials" case in environments without any of these.
	// Skip if credentials might be available from any source.

	// Skip if running on GCP (metadata server would provide creds).
	if os.Getenv("GCP_METADATA_HOST") != "" {
		t.Skip("Skipping: GCP_METADATA_HOST is set (metadata server available)")
	}

	// Skip if GOOGLE_APPLICATION_CREDENTIALS is set.
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		t.Skip("Skipping: GOOGLE_APPLICATION_CREDENTIALS is set")
	}

	// Skip if gcloud application-default credentials exist.
	home, _ := os.UserHomeDir()
	if home != "" {
		adcPath := home + "/.config/gcloud/application_default_credentials.json"
		if _, err := os.Stat(adcPath); err == nil {
			t.Skip("Skipping: gcloud application-default credentials exist at " + adcPath)
		}
	}

	spec := &types.GCPADCProviderSpec{ProjectID: "test"}
	p, err := New(spec)
	require.NoError(t, err)

	ctx := context.Background()
	creds, err := p.Authenticate(ctx)

	// Should fail when no credentials available.
	require.Error(t, err)
	assert.Nil(t, creds)
}

func TestAuthenticate_WithScopes(t *testing.T) {
	spec := &types.GCPADCProviderSpec{
		ProjectID: "my-project",
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform.read-only",
		},
	}
	p, err := New(spec)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://www.googleapis.com/auth/cloud-platform.read-only"}, p.spec.Scopes)
}

func TestDefaultScope(t *testing.T) {
	assert.Equal(t, "https://www.googleapis.com/auth/cloud-platform", DefaultScope)
}
