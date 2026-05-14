package gcp_adc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// staticTokenSource returns a fixed token for testing.
type staticTokenSource struct {
	token *oauth2.Token
	err   error
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	return s.token, s.err
}

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

func TestProvider_Name(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	// Default name is the kind.
	assert.Equal(t, ProviderKind, p.Name())

	// Custom name.
	p.SetName("my-adc-provider")
	assert.Equal(t, "my-adc-provider", p.Name())
}

func TestProvider_SetName(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	p.SetName("custom")
	assert.Equal(t, "custom", p.name)
}

// TestSetRealm_RealmIndependent verifies that SetRealm stores the value
// (for interface compliance) but ADC behavior is unaffected since it performs
// no credential file I/O.
func TestSetRealm_RealmIndependent(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{ProjectID: "proj"}}
	p.SetRealm("test-realm")
	assert.Equal(t, "test-realm", p.realm)

	// Paths is always empty regardless of realm.
	paths, err := p.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)

	// Environment is unaffected by realm.
	env, err := p.Environment()
	require.NoError(t, err)
	assert.Equal(t, "proj", env["GOOGLE_CLOUD_PROJECT"])
}

func TestPreAuthenticate(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	err := p.PreAuthenticate(nil)
	assert.NoError(t, err)
}

func TestValidate(t *testing.T) {
	// Valid spec.
	p := &Provider{spec: &types.GCPADCProviderSpec{ProjectID: "proj"}}
	assert.NoError(t, p.Validate())

	// Nil spec.
	p = &Provider{}
	err := p.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestEnvironment_WithProject(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{ProjectID: "env-project"}}
	env, err := p.Environment()
	require.NoError(t, err)
	assert.Equal(t, "env-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "env-project", env["GCLOUD_PROJECT"])
	assert.Equal(t, "env-project", env["CLOUDSDK_CORE_PROJECT"])
}

func TestEnvironment_EmptyProject(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	env, err := p.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestPaths(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	paths, err := p.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestPrepareEnvironment(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{ProjectID: "prep-project"}}
	env, err := p.PrepareEnvironment(context.Background(), map[string]string{"PATH": "/usr/bin"})
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin", env["PATH"])
	assert.Equal(t, "prep-project", env["GOOGLE_CLOUD_PROJECT"])
}

func TestPrepareEnvironment_NilInput(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{ProjectID: "proj"}}
	env, err := p.PrepareEnvironment(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "proj", env["GOOGLE_CLOUD_PROJECT"])
}

func TestPrepareEnvironment_NoProject(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	env, err := p.PrepareEnvironment(context.Background(), map[string]string{"FOO": "bar"})
	require.NoError(t, err)
	assert.Equal(t, "bar", env["FOO"])
	_, hasProject := env["GOOGLE_CLOUD_PROJECT"]
	assert.False(t, hasProject)
}

func TestLogout(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	err := p.Logout(context.Background())
	assert.NoError(t, err)
}

func TestGetFilesDisplayPath(t *testing.T) {
	p := &Provider{spec: &types.GCPADCProviderSpec{}}
	assert.Equal(t, "", p.GetFilesDisplayPath())
}

func TestIsADCReauthError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil error", nil, false},
		{"unrelated error", errors.New("connection refused"), false},
		{"invalid_grant only", errors.New("invalid_grant"), false},
		{"invalid_rapt only", errors.New("invalid_rapt"), false},
		{"both invalid_grant and invalid_rapt", errors.New("oauth2: cannot fetch token: 400 Bad Request\nResponse: {\"error\":\"invalid_grant\",\"error_description\":\"reauth related error: invalid_rapt\"}"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, isADCReauthError(tt.err))
		})
	}
}

func TestAuthenticate_NoCredentials(t *testing.T) {
	// This test verifies that Authenticate fails when no credentials are available.
	// ADC can find credentials from multiple sources:
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
		adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
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

func TestAuthenticate_Success_DefaultScopes(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, scopes ...string) (*google.Credentials, error) {
			assert.Equal(t, []string{DefaultScope}, scopes)
			return &google.Credentials{
				ProjectID: "adc-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{
						AccessToken: "test-access-token",
						Expiry:      expiry,
					},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, accessToken string) (string, error) {
			assert.Equal(t, "test-access-token", accessToken)
			return "sa@project.iam.gserviceaccount.com", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds, ok := creds.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "test-access-token", gcpCreds.AccessToken)
	assert.Equal(t, expiry, gcpCreds.TokenExpiry)
	assert.Equal(t, "adc-project", gcpCreds.ProjectID)
	assert.Equal(t, "sa@project.iam.gserviceaccount.com", gcpCreds.ServiceAccountEmail)
	assert.Equal(t, []string{DefaultScope}, gcpCreds.Scopes)
}

func TestAuthenticate_Success_CustomScopes(t *testing.T) {
	customScopes := []string{"https://www.googleapis.com/auth/compute.readonly"}
	p := &Provider{
		spec: &types.GCPADCProviderSpec{
			Scopes: customScopes,
		},
		findCredentials: func(_ context.Context, scopes ...string) (*google.Credentials, error) {
			assert.Equal(t, customScopes, scopes)
			return &google.Credentials{
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{AccessToken: "tok"},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)
	gcpCreds := creds.(*types.GCPCredentials)
	assert.Equal(t, customScopes, gcpCreds.Scopes)
}

func TestAuthenticate_Success_SpecProjectOverridesADC(t *testing.T) {
	p := &Provider{
		spec: &types.GCPADCProviderSpec{
			ProjectID: "spec-project",
		},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				ProjectID: "adc-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{AccessToken: "tok"},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	gcpCreds := creds.(*types.GCPCredentials)
	assert.Equal(t, "spec-project", gcpCreds.ProjectID)
}

func TestAuthenticate_Success_ADCProjectUsedWhenNoSpec(t *testing.T) {
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				ProjectID: "adc-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{AccessToken: "tok"},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	gcpCreds := creds.(*types.GCPCredentials)
	assert.Equal(t, "adc-project", gcpCreds.ProjectID)
}

func TestAuthenticate_FindCredentialsFails(t *testing.T) {
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return nil, fmt.Errorf("no credentials found")
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "find default credentials")
}

func TestAuthenticate_TokenFails(t *testing.T) {
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				TokenSource: &staticTokenSource{
					err: fmt.Errorf("token expired"),
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "get token from ADC")
}

func TestAuthenticate_ReauthError(t *testing.T) {
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				TokenSource: &staticTokenSource{
					err: fmt.Errorf("oauth2: cannot fetch token: 400\nResponse: {\"error\":\"invalid_grant\",\"error_description\":\"invalid_rapt\"}"),
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	// The reauth hint is in the error builder's explanation, not the base message.
	assert.Contains(t, err.Error(), "get token from ADC")
}

func TestAuthenticate_NilSpec(t *testing.T) {
	p := &Provider{
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return nil, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestAuthenticate_EmptyAccessToken_SkipsEmailFetch(t *testing.T) {
	emailFetchCalled := false
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{AccessToken: ""},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			emailFetchCalled = true
			return "should-not-be-called", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	assert.False(t, emailFetchCalled)
	gcpCreds := creds.(*types.GCPCredentials)
	assert.Empty(t, gcpCreds.ServiceAccountEmail)
}

// --- ADC Critical Path Tests ---
//
// These tests verify the ADC provider's stateless nature and realm-independence,
// which is critical because:
// 1. ADC relies on external credential sources (gcloud, env vars, metadata server).
// 2. ADC does NOT store or manage any credential files.
// 3. ADC must work with empty realm (no auth.realm configured).
// 4. ADC works with both gcp/project (no SA) and gcp/service-account (impersonation).

// TestADC_StatelessNoFileStorage verifies that ADC provider does not store any
// credential files. This is critical because ADC delegates all credential management
// to Google's SDK and external tools (gcloud, metadata server).
func TestADC_StatelessNoFileStorage(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	p := &Provider{
		spec: &types.GCPADCProviderSpec{ProjectID: "stateless-project"},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				ProjectID: "stateless-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{
						AccessToken: "stateless-token",
						Expiry:      expiry,
					},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "user@example.com", nil
		},
	}

	// Authenticate successfully.
	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)

	// Verify Paths() is always empty — no credential files managed.
	paths, err := p.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths, "ADC provider must not manage any credential files")

	// Verify Logout() is a no-op — nothing to clean up.
	err = p.Logout(context.Background())
	require.NoError(t, err)

	// Verify GetFilesDisplayPath() is empty — no files to display.
	assert.Empty(t, p.GetFilesDisplayPath(), "ADC provider has no credential files to display")
}

// TestADC_RealmIndependentBehavior verifies that ADC provider behavior is completely
// independent of the realm setting. Authentication, environment, and paths must all
// work identically regardless of realm value (empty, explicit, or any string).
func TestADC_RealmIndependentBehavior(t *testing.T) {
	realms := []string{"", "test-realm", "customer-acme", "auto-a1b2c3d4"}

	for _, realmValue := range realms {
		t.Run(fmt.Sprintf("realm=%q", realmValue), func(t *testing.T) {
			expiry := time.Now().Add(time.Hour)
			p := &Provider{
				spec: &types.GCPADCProviderSpec{ProjectID: "realm-test-project"},
				findCredentials: func(_ context.Context, scopes ...string) (*google.Credentials, error) {
					return &google.Credentials{
						ProjectID: "realm-test-project",
						TokenSource: &staticTokenSource{
							token: &oauth2.Token{
								AccessToken: "realm-test-token",
								Expiry:      expiry,
							},
						},
					}, nil
				},
				fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
					return "sa@realm-test-project.iam.gserviceaccount.com", nil
				},
			}

			// Set realm — should not affect behavior.
			p.SetRealm(realmValue)

			// Authenticate must succeed regardless of realm.
			creds, err := p.Authenticate(context.Background())
			require.NoError(t, err)
			require.NotNil(t, creds)

			gcpCreds := creds.(*types.GCPCredentials)
			assert.Equal(t, "realm-test-token", gcpCreds.AccessToken)
			assert.Equal(t, "realm-test-project", gcpCreds.ProjectID)

			// Paths always empty regardless of realm.
			paths, err := p.Paths()
			require.NoError(t, err)
			assert.Empty(t, paths)

			// Environment always the same regardless of realm.
			env, err := p.Environment()
			require.NoError(t, err)
			assert.Equal(t, "realm-test-project", env["GOOGLE_CLOUD_PROJECT"])

			// PrepareEnvironment always the same regardless of realm.
			prepEnv, err := p.PrepareEnvironment(context.Background(), map[string]string{"PATH": "/usr/bin"})
			require.NoError(t, err)
			assert.Equal(t, "realm-test-project", prepEnv["GOOGLE_CLOUD_PROJECT"])
			assert.Equal(t, "/usr/bin", prepEnv["PATH"])
		})
	}
}

// TestADC_AuthenticateReturnsUserEmail verifies that when ADC returns a user
// email (not a service account), it is correctly captured. This is the case when
// a developer runs `gcloud auth application-default login` — the token belongs
// to their Google user account, not a service account.
func TestADC_AuthenticateReturnsUserEmail(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	p := &Provider{
		spec: &types.GCPADCProviderSpec{ProjectID: "user-project"},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				ProjectID: "user-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{
						AccessToken: "user-token",
						Expiry:      expiry,
					},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, accessToken string) (string, error) {
			assert.Equal(t, "user-token", accessToken)
			return "developer@company.com", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds := creds.(*types.GCPCredentials)
	assert.Equal(t, "developer@company.com", gcpCreds.ServiceAccountEmail,
		"ADC should capture user email from tokeninfo API")
	assert.Equal(t, "user-project", gcpCreds.ProjectID)
	assert.Equal(t, "user-token", gcpCreds.AccessToken)
}

// TestADC_AuthenticateEmailFetchFailure verifies that authentication succeeds
// even when the tokeninfo email fetch fails. Email is best-effort only.
func TestADC_AuthenticateEmailFetchFailure(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	p := &Provider{
		spec: &types.GCPADCProviderSpec{},
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				ProjectID: "resilient-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{
						AccessToken: "resilient-token",
						Expiry:      expiry,
					},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("tokeninfo API unavailable")
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err, "Authentication must succeed even when email fetch fails")
	require.NotNil(t, creds)

	gcpCreds := creds.(*types.GCPCredentials)
	assert.Equal(t, "resilient-token", gcpCreds.AccessToken)
	assert.Equal(t, "resilient-project", gcpCreds.ProjectID)
	assert.Empty(t, gcpCreds.ServiceAccountEmail,
		"Email should be empty when tokeninfo fetch fails")
}

// TestADC_AuthenticateNoProjectID verifies ADC works when neither spec nor
// ADC chain provides a project ID. This is valid when the developer's environment
// doesn't have a default project configured.
func TestADC_AuthenticateNoProjectID(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	p := &Provider{
		spec: &types.GCPADCProviderSpec{}, // No ProjectID in spec.
		findCredentials: func(_ context.Context, _ ...string) (*google.Credentials, error) {
			return &google.Credentials{
				ProjectID: "", // No project in ADC chain either.
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{
						AccessToken: "no-project-token",
						Expiry:      expiry,
					},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "dev@company.com", nil
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err, "ADC should authenticate even without project ID")
	require.NotNil(t, creds)

	gcpCreds := creds.(*types.GCPCredentials)
	assert.Empty(t, gcpCreds.ProjectID, "Project ID should be empty when not available")
	assert.Equal(t, "no-project-token", gcpCreds.AccessToken)
	assert.Equal(t, "dev@company.com", gcpCreds.ServiceAccountEmail)
}

// TestADC_PrepareEnvironment_EmptyRealm verifies that PrepareEnvironment works
// correctly with empty realm, which is the default when auth.realm is not configured.
func TestADC_PrepareEnvironment_EmptyRealm(t *testing.T) {
	p := &Provider{
		spec: &types.GCPADCProviderSpec{ProjectID: "env-project"},
	}
	// Empty realm — the default.
	p.SetRealm("")

	env, err := p.PrepareEnvironment(context.Background(), map[string]string{
		"PATH":                           "/usr/bin",
		"GOOGLE_APPLICATION_CREDENTIALS": "/some/path/to/key.json",
	})
	require.NoError(t, err)

	// Existing env vars preserved.
	assert.Equal(t, "/usr/bin", env["PATH"])
	// ADC does not clear GOOGLE_APPLICATION_CREDENTIALS — it's the external credential source.
	assert.Equal(t, "/some/path/to/key.json", env["GOOGLE_APPLICATION_CREDENTIALS"])
	// Project env vars set.
	assert.Equal(t, "env-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "env-project", env["GCLOUD_PROJECT"])
	assert.Equal(t, "env-project", env["CLOUDSDK_CORE_PROJECT"])
}

// TestADC_FullAuthenticateLifecycle verifies the complete ADC lifecycle:
// authenticate → get credentials → verify stateless (no files) → logout (no-op).
func TestADC_FullAuthenticateLifecycle(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	p := &Provider{
		spec: &types.GCPADCProviderSpec{
			ProjectID: "lifecycle-project",
			Scopes:    []string{"https://www.googleapis.com/auth/compute.readonly"},
		},
		findCredentials: func(_ context.Context, scopes ...string) (*google.Credentials, error) {
			assert.Equal(t, []string{"https://www.googleapis.com/auth/compute.readonly"}, scopes)
			return &google.Credentials{
				ProjectID: "lifecycle-project",
				TokenSource: &staticTokenSource{
					token: &oauth2.Token{
						AccessToken: "lifecycle-token",
						Expiry:      expiry,
					},
				},
			}, nil
		},
		fetchTokenEmail: func(_ context.Context, _ string) (string, error) {
			return "sa@lifecycle-project.iam.gserviceaccount.com", nil
		},
	}

	ctx := context.Background()

	// Step 1: Authenticate.
	creds, err := p.Authenticate(ctx)
	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds := creds.(*types.GCPCredentials)
	assert.Equal(t, "lifecycle-token", gcpCreds.AccessToken)
	assert.Equal(t, expiry, gcpCreds.TokenExpiry)
	assert.Equal(t, "lifecycle-project", gcpCreds.ProjectID)
	assert.Equal(t, "sa@lifecycle-project.iam.gserviceaccount.com", gcpCreds.ServiceAccountEmail)
	assert.Equal(t, []string{"https://www.googleapis.com/auth/compute.readonly"}, gcpCreds.Scopes)

	// Step 2: Verify no files stored.
	paths, err := p.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)

	// Step 3: Verify environment variables.
	env, err := p.Environment()
	require.NoError(t, err)
	assert.Equal(t, "lifecycle-project", env["GOOGLE_CLOUD_PROJECT"])

	// Step 4: Logout (no-op).
	err = p.Logout(ctx)
	require.NoError(t, err)

	// Step 5: Can authenticate again (stateless — no state to corrupt).
	creds2, err := p.Authenticate(ctx)
	require.NoError(t, err)
	require.NotNil(t, creds2)
	gcpCreds2 := creds2.(*types.GCPCredentials)
	assert.Equal(t, "lifecycle-token", gcpCreds2.AccessToken)
}
