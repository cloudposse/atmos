package authbridge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	"github.com/cloudposse/atmos/pkg/store/providers"
)

func ambientStoreManager(t *testing.T, source string) types.AuthManager {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_CACHE_HOME", root)
	for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	configPath, credentialsPath := filepath.Join(root, "config"), filepath.Join(root, "credentials")
	require.NoError(t, os.WriteFile(configPath, []byte("[profile selected]\nregion = eu-west-1\n"), 0o600))
	require.NoError(t, os.WriteFile(credentialsPath, []byte("[selected]\naws_access_key_id = ambient-access\naws_secret_access_key = ambient-secret\naws_session_token = ambient-token\n"), 0o600))
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
	if source == "profile" {
		t.Setenv("AWS_PROFILE", "selected")
	} else {
		t.Setenv("AWS_ACCESS_KEY_ID", "ambient-access")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
		t.Setenv("AWS_SESSION_TOKEN", "ambient-token")
		t.Setenv("AWS_REGION", "eu-west-1")
	}
	ctrl := gomock.NewController(t)
	credentialStore := types.NewMockCredentialStore(ctrl)
	credentialStore.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	manager, err := auth.NewAuthManager(&schema.AuthConfig{
		Realm:      "ambient-store-test",
		Identities: map[string]schema.Identity{"default": {Kind: "aws/ambient", Default: true}},
	}, credentialStore, types.NewMockValidator(ctrl), &schema.ConfigAndStacksInfo{}, "")
	require.NoError(t, err)
	return manager
}

// TestAWSAmbientSecretStores exercises the real ambient identity and auth manager
// through both resolver paths, then signs real SDK requests to a local backend.
func TestAWSAmbientSecretStores(t *testing.T) {
	for _, source := range []string{"profile", "environment"} {
		t.Run(source, func(t *testing.T) {
			manager := ambientStoreManager(t, source)
			for _, adapter := range []string{"manager", "resolved", "lazy"} {
				t.Run(adapter, func(t *testing.T) {
					resolver := store.AuthContextResolver(authbridge.NewResolver(manager, &schema.ConfigAndStacksInfo{}))
					if adapter != "manager" {
						_, err := manager.Authenticate(t.Context(), "default")
						require.NoError(t, err)
						resolver = authbridge.NewResolvedContext(manager.GetStackInfo().AuthContext)
						if adapter == "lazy" {
							resolver = authbridge.NewContextResolver(func(context.Context, string) (*schema.AuthContext, error) {
								return manager.GetStackInfo().AuthContext, nil
							})
						}
					}
					resolved, err := resolver.ResolveAWSAuthContext(t.Context(), "default")
					require.NoError(t, err)
					assert.Equal(t, &store.AWSAuthConfig{Region: "eu-west-1"}, resolved)
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						assert.Contains(t, r.Header.Get("Authorization"), "Credential=ambient-access/")
						assert.Contains(t, r.Header.Get("Authorization"), "/eu-west-1/")
						assert.Equal(t, "ambient-token", r.Header.Get("X-Amz-Security-Token"))
						w.Header().Set("Content-Type", "application/x-amz-json-1.1")
						switch r.Header.Get("X-Amz-Target") {
						case "AmazonSSM.GetParameter":
							var input struct {
								Name           string
								WithDecryption bool
							}
							if err := json.NewDecoder(r.Body).Decode(&input); !assert.NoError(t, err) {
								return
							}
							assert.Contains(t, input.Name, "demo")
							assert.True(t, input.WithDecryption)
							_, _ = w.Write([]byte(`{"Parameter":{"Name":"demo","Type":"SecureString","Value":"secret-value"}}`))
						case "secretsmanager.GetSecretValue":
							_, _ = w.Write([]byte(`{"Name":"demo","SecretString":"secret-value"}`))
						default:
							t.Errorf("unexpected AWS operation %q", r.Header.Get("X-Amz-Target"))
							w.WriteHeader(http.StatusBadRequest)
						}
					}))
					t.Cleanup(server.Close)
					endpoint := server.URL
					ssm, err := providers.NewSSMStore(providers.SSMStoreOptions{Region: "eu-west-1", Endpoint: &endpoint}, "default")
					require.NoError(t, err)
					asm, err := providers.NewSecretsManagerStore(providers.SecretsManagerStoreOptions{Region: "eu-west-1", Endpoint: &endpoint}, "default")
					require.NoError(t, err)
					for _, backend := range []store.Store{ssm, asm} {
						backend.(store.IdentityAwareStore).SetAuthContext(resolver, "default")
						value, err := backend.Get("", "", "demo")
						require.NoError(t, err)
						assert.Equal(t, "secret-value", value)
					}
					assert.Equal(t, int32(2), requests.Load())
				})
			}
		})
	}
}
