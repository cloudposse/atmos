package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
)

func TestManagerAWSFileScope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     string
		explicit bool
		clear    bool
	}{
		{name: "inherited GitLab scope", kind: types.ProviderKindGitLabOIDC},
		{name: "explicit scope overrides inherited scope", kind: types.ProviderKindGitLabOIDC, explicit: true},
		{name: "explicit empty scope clears inherited scope", kind: types.ProviderKindGitLabOIDC, explicit: true, clear: true},
		{name: "GitHub storage and cache unchanged", kind: "github/oidc", explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("ATMOS_AUTH_REALM", "")
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			inherited := t.TempDir()
			t.Setenv(AuthExecAWSFilesBasePathEnv, inherited)
			basePath := inherited
			var options []ManagerOption
			if tc.explicit {
				basePath = t.TempDir()
				if tc.clear {
					basePath = ""
				}
				options = []ManagerOption{WithAWSFilesBasePath(basePath)}
			}
			config := gitlabChainConfig("")
			provider := config.Providers["gitlab"]
			provider.Kind = tc.kind
			config.Providers["gitlab"] = provider
			store := types.NewMockCredentialStore(gomock.NewController(t))
			creds := &types.AWSCredentials{
				AccessKeyID: "current-job-key", SecretAccessKey: "synthetic-secret", SessionToken: "current-session",
				Region: "eu-west-1", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			}
			if tc.kind != types.ProviderKindGitLabOIDC || tc.clear {
				basePath = ""
				store.EXPECT().Retrieve("runner", config.Realm).Return(creds, nil)
			}
			require.NoError(t, awsCloud.SetupFiles("gitlab", "runner", creds, basePath, config.Realm))
			m, err := NewAuthManager(config, store, validation.NewValidator(), nil, "")
			require.NoError(t, err)
			m = applyManagerOptions(m, options...)

			info, err := m.GetCachedCredentials(context.Background(), "runner")

			require.NoError(t, err)
			assert.Equal(t, "current-job-key", info.Credentials.(*types.AWSCredentials).AccessKeyID)
			files, err := awsCloud.NewAWSFileManager(basePath, config.Realm)
			require.NoError(t, err)
			assert.Equal(t, files.GetCredentialsPath("gitlab"), info.Environment["AWS_SHARED_CREDENTIALS_FILE"])
		})
	}
}

func TestGitLabIsolatedAuthenticationKeepsFileScope(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ATMOS_AUTH_REALM", "")
	t.Setenv(AuthExecAWSFilesBasePathEnv, "")
	t.Setenv("ATMOS_OIDC_TOKEN", "synthetic-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, gitlabSTSResponse("AssumeRoleWithWebIdentity", "runner-key", time.Now().Add(time.Hour)))
	}))
	t.Cleanup(server.Close)
	config := gitlabChainConfig(server.URL)
	config.Keyring.Type = types.CredentialStoreTypeMemory
	basePath := t.TempDir()
	m, err := NewDefaultManagerWithOptions(config, "", WithAWSFilesBasePath(basePath))
	require.NoError(t, err)

	result, err := m.AuthenticateIsolated(context.Background(), "runner")

	require.NoError(t, err)
	files, err := awsCloud.NewAWSFileManager(basePath, config.Realm)
	require.NoError(t, err)
	assert.Equal(t, files.GetCredentialsPath("gitlab"), result.AWS.CredentialsFile)
	assert.FileExists(t, result.AWS.CredentialsFile)
	shared, err := awsCloud.NewAWSFileManager("", config.Realm)
	require.NoError(t, err)
	assert.NoFileExists(t, shared.GetCredentialsPath("gitlab"))
}
