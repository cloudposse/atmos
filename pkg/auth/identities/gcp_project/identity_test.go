package gcp_project

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNew(t *testing.T) {
	principal := &types.GCPProjectIdentityPrincipal{
		ProjectID: "test-project",
	}
	id, err := New(principal)
	require.NoError(t, err)
	require.NotNil(t, id)
	assert.Equal(t, IdentityKind, id.Kind())
}

func TestNew_NilPrincipal(t *testing.T) {
	id, err := New(nil)
	require.Error(t, err)
	assert.Nil(t, id)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestIdentity_Kind(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{}}
	assert.Equal(t, "gcp/project", id.Kind())
}

func TestIdentity_Name(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{}}
	assert.Equal(t, IdentityKind, id.Name())

	id.SetName("custom-project")
	assert.Equal(t, "custom-project", id.Name())
}

func TestGetProviderName(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{}}
	name, err := id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "", name)
}

func TestGetProviderName_WithConfig(t *testing.T) {
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "test-project",
		},
	}

	// Without config, returns empty string.
	name, err := id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "", name)

	// With config containing Via.Provider, returns the provider name.
	id.SetConfig(&schema.Identity{
		Kind: "gcp/project",
		Via: &schema.IdentityVia{
			Provider: "my-gcp-provider",
		},
	})
	name, err = id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "my-gcp-provider", name)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		principal *types.GCPProjectIdentityPrincipal
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "nil principal",
			principal: nil,
			wantErr:   true,
			errMsg:    "principal is nil",
		},
		{
			name:      "empty project_id",
			principal: &types.GCPProjectIdentityPrincipal{},
			wantErr:   true,
			errMsg:    "project_id is required",
		},
		{
			name: "valid - project only",
			principal: &types.GCPProjectIdentityPrincipal{
				ProjectID: "my-project",
			},
			wantErr: false,
		},
		{
			name: "valid - with region and zone",
			principal: &types.GCPProjectIdentityPrincipal{
				ProjectID: "my-project",
				Region:    "us-central1",
				Zone:      "us-central1-a",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := &Identity{principal: tt.principal}
			err := id.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAuthenticate_NoBaseCreds(t *testing.T) {
	principal := &types.GCPProjectIdentityPrincipal{
		ProjectID: "test-project",
	}
	id, _ := New(principal)

	ctx := context.Background()
	creds, err := id.Authenticate(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds, ok := creds.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "test-project", gcpCreds.ProjectID)
	assert.Empty(t, gcpCreds.AccessToken)
}

func TestAuthenticate_WithBaseCreds(t *testing.T) {
	principal := &types.GCPProjectIdentityPrincipal{
		ProjectID: "override-project",
	}
	id, _ := New(principal)

	baseCreds := &types.GCPCredentials{
		AccessToken:         "original-token",
		TokenExpiry:         time.Now().Add(1 * time.Hour),
		ProjectID:           "original-project",
		ServiceAccountEmail: "sa@original.iam.gserviceaccount.com",
		Scopes:              []string{"scope1"},
	}

	ctx := context.Background()
	creds, err := id.Authenticate(ctx, baseCreds)

	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds, ok := creds.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "override-project", gcpCreds.ProjectID)
	assert.Equal(t, "original-token", gcpCreds.AccessToken)
	assert.Equal(t, "sa@original.iam.gserviceaccount.com", gcpCreds.ServiceAccountEmail)
}

func TestAuthenticate_InvalidPrincipal(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{}}

	ctx := context.Background()
	creds, err := id.Authenticate(ctx, nil)

	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), "project_id is required")
}

func TestEnvironment(t *testing.T) {
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "env-project",
			Region:    "us-west1",
			Zone:      "us-west1-b",
		},
	}

	env, err := id.Environment()
	require.NoError(t, err)

	assert.Equal(t, "env-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "env-project", env["GCLOUD_PROJECT"])
	assert.Equal(t, "env-project", env["CLOUDSDK_CORE_PROJECT"])
	assert.Equal(t, "us-west1", env["GOOGLE_CLOUD_REGION"])
	assert.Equal(t, "us-west1", env["CLOUDSDK_COMPUTE_REGION"])
	assert.Equal(t, "us-west1-b", env["GOOGLE_CLOUD_ZONE"])
	assert.Equal(t, "us-west1-b", env["CLOUDSDK_COMPUTE_ZONE"])
}

func TestEnvironment_ProjectOnly(t *testing.T) {
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "project-only",
		},
	}

	env, err := id.Environment()
	require.NoError(t, err)

	assert.Equal(t, "project-only", env["GOOGLE_CLOUD_PROJECT"])
	assert.NotContains(t, env, "GOOGLE_CLOUD_REGION")
	assert.NotContains(t, env, "GOOGLE_CLOUD_ZONE")
}

func TestEnvironment_LocationFallbackToZone(t *testing.T) {
	// Location is a legacy field that maps to zone if zone is not set.
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "loc-project",
			Region:    "us-east1",
			Location:  "us-east1-b", // Legacy location field
		},
	}

	env, err := id.Environment()
	require.NoError(t, err)

	assert.Equal(t, "loc-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "us-east1", env["GOOGLE_CLOUD_REGION"])
	// Location should be applied as zone since zone is not set.
	assert.Equal(t, "us-east1-b", env["GOOGLE_CLOUD_ZONE"])
	assert.Equal(t, "us-east1-b", env["CLOUDSDK_COMPUTE_ZONE"])
}

func TestEnvironment_ZoneTakesPrecedenceOverLocation(t *testing.T) {
	// When both zone and location are set, zone takes precedence.
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "zone-project",
			Zone:      "us-west1-a",
			Location:  "us-east1-b", // Should be ignored
		},
	}

	env, err := id.Environment()
	require.NoError(t, err)

	assert.Equal(t, "us-west1-a", env["GOOGLE_CLOUD_ZONE"])
	assert.Equal(t, "us-west1-a", env["CLOUDSDK_COMPUTE_ZONE"])
}

func TestPrepareEnvironment(t *testing.T) {
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "prep-project",
			Region:    "europe-west1",
		},
	}

	existing := map[string]string{
		"PATH":                 "/usr/bin",
		"GOOGLE_CLOUD_PROJECT": "old-project",
	}

	env, err := id.PrepareEnvironment(context.Background(), existing)
	require.NoError(t, err)

	assert.Equal(t, "/usr/bin", env["PATH"])
	assert.Equal(t, "prep-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "europe-west1", env["GOOGLE_CLOUD_REGION"])
}

func TestPostAuthenticate(t *testing.T) {
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GOOGLE_CLOUD_REGION")
	defer func() {
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("GOOGLE_CLOUD_REGION")
	}()

	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "post-auth-project",
			Region:    "asia-east1",
		},
	}

	authContext := &schema.AuthContext{}
	creds := &types.GCPCredentials{
		AccessToken: "test-token",
		ProjectID:   "post-auth-project",
	}

	err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: authContext,
		Credentials: creds,
	})
	require.NoError(t, err)

	assert.Equal(t, "post-auth-project", os.Getenv("GOOGLE_CLOUD_PROJECT"))
	assert.Equal(t, "asia-east1", os.Getenv("GOOGLE_CLOUD_REGION"))

	require.NotNil(t, authContext.GCP)
	assert.Equal(t, "post-auth-project", authContext.GCP.ProjectID)
	assert.Equal(t, "asia-east1", authContext.GCP.Region)
	assert.Equal(t, "test-token", authContext.GCP.AccessToken)
}

func TestCredentialsExist(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{ProjectID: "p"}}

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestLoadCredentials(t *testing.T) {
	id := &Identity{
		principal: &types.GCPProjectIdentityPrincipal{
			ProjectID: "load-project",
		},
	}

	creds, err := id.LoadCredentials(context.Background())
	require.NoError(t, err)

	gcpCreds, ok := creds.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "load-project", gcpCreds.ProjectID)
}

func TestLoadCredentials_NilPrincipal(t *testing.T) {
	id := &Identity{principal: nil}

	creds, err := id.LoadCredentials(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds)
}

func TestLogout(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{ProjectID: "p"}}

	err := id.Logout(context.Background())
	require.NoError(t, err)
}

func TestPaths(t *testing.T) {
	id := &Identity{principal: &types.GCPProjectIdentityPrincipal{ProjectID: "p"}}

	paths, err := id.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}
