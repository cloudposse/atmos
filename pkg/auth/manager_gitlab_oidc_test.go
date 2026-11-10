package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for configuration fields used in the authentication chain.
var (
	_ = schema.Provider{Kind: types.ProviderKindGitLabOIDC}
	_ = schema.Identity{Kind: "aws/assume-role", Via: &schema.IdentityVia{}, Principal: map[string]any{}, Spec: map[string]any{}}
)

// TestGitLabOIDCAuthenticationChain covers direct and chained AWS roles using the current GitLab token
// instead of ambient AWS credentials.
func TestGitLabOIDCAuthenticationChain(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		actions []string
		chain   []string
	}{
		{
			name: "direct role", target: "runner",
			actions: []string{"AssumeRoleWithWebIdentity"},
			chain:   []string{"gitlab", "runner"},
		},
		{
			name: "chained role", target: "production",
			actions: []string{"AssumeRoleWithWebIdentity", "AssumeRole"},
			chain:   []string{"gitlab", "runner", "production"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetProcessCredentialCache()
			t.Cleanup(resetProcessCredentialCache)
			t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			t.Setenv("ATMOS_AUTH_REALM", "")
			t.Setenv("ATMOS_OIDC_TOKEN", "initial-gitlab-job-token")
			t.Setenv("AWS_ACCESS_KEY_ID", "ambient-access-key")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret-key")
			t.Setenv("AWS_SESSION_TOKEN", "ambient-session-token")
			runGitLabOIDCChain(t, tc.target, tc.actions, tc.chain)
		})
	}
}

// runGitLabOIDCChain exercises mock STS authentication, process-local reuse, token rotation, and explicit
// logout of managed AWS files.
func runGitLabOIDCChain(t *testing.T, target string, wantActions, wantChain []string) {
	t.Helper()
	actions := []string{}
	tokens := []string{}
	expiration := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, r.ParseForm()) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		action := r.Form.Get("Action")
		actions = append(actions, action)
		assert.Equal(t, "gitlab-test", r.Form.Get("RoleSessionName"))
		assert.Equal(t, "900", r.Form.Get("DurationSeconds"))
		key := checkGitLabSTSRequest(t, r, action, &tokens)
		w.Header().Set("Content-Type", "text/xml")
		_, err := fmt.Fprint(w, gitlabSTSResponse(action, key, expiration))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	config := gitlabChainConfig(server.URL)
	if target == "runner" {
		delete(config.Identities, "production")
	}
	store := types.NewMockCredentialStore(gomock.NewController(t))
	// Unexpected Store or Retrieve calls fail: ambient chains must never use the keyring.
	for _, step := range wantChain {
		store.EXPECT().Delete(step, "gitlab-test").Return(nil).MinTimes(1)
		store.EXPECT().Delete(step, "").Return(nil).MinTimes(1)
	}
	newManager := func() types.AuthManager {
		m, err := NewAuthManager(config, store, validation.NewValidator(), &schema.ConfigAndStacksInfo{}, "")
		require.NoError(t, err)
		return m
	}
	m := newManager()
	info, err := m.Authenticate(context.Background(), target)
	require.NoError(t, err)
	assert.Equal(t, wantChain, m.GetChain())
	assert.Equal(t, wantActions, actions)
	assert.Equal(t, []string{"initial-gitlab-job-token"}, tokens)
	assertGitLabToolCredentials(t, info, target, expiration)
	verifyGitLabOIDCRefresh(t, newManager, target, wantActions, &actions, &tokens)
	require.NoError(t, m.LogoutProvider(context.Background(), "gitlab", false))
	// Logging out removes the tool credentials even though the provider owns no files.
	files, err := awsCloud.NewAWSFileManager("", "gitlab-test")
	require.NoError(t, err)
	_, err = os.Stat(files.GetCredentialsPath("gitlab"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// checkGitLabSTSRequest verifies that initial role assumption sends the GitLab token anonymously and chained
// requests use the preceding role.
func checkGitLabSTSRequest(t *testing.T, r *http.Request, action string, tokens *[]string) string {
	t.Helper()
	if action == "AssumeRoleWithWebIdentity" {
		assert.Equal(t, "arn:aws:iam::111111111111:role/GitLabRunnerRole", r.Form.Get("RoleArn"))
		assert.Empty(t, r.Header.Get("Authorization"), "web identity must use anonymous AWS credentials")
		assert.Empty(t, r.Header.Get("X-Amz-Security-Token"))
		*tokens = append(*tokens, r.Form.Get("WebIdentityToken"))
		return "runner-key"
	}
	assert.Equal(t, "AssumeRole", action)
	assert.Equal(t, "arn:aws:iam::222222222222:role/DeployRole", r.Form.Get("RoleArn"))
	assert.Contains(t, r.Header.Get("Authorization"), "Credential=runner-key/")
	assert.Equal(t, "runner-key-session", r.Header.Get("X-Amz-Security-Token"))
	assert.Empty(t, r.Form.Get("WebIdentityToken"))
	return "production-key"
}

func gitlabSTSResponse(action, key string, expiration time.Time) string {
	return fmt.Sprintf(`<%[1]sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <%[1]sResult><Credentials>
    <AccessKeyId>%[2]s</AccessKeyId><SecretAccessKey>%[2]s-secret</SecretAccessKey>
    <SessionToken>%[2]s-session</SessionToken><Expiration>%[3]s</Expiration>
  </Credentials></%[1]sResult>
  <ResponseMetadata><RequestId>gitlab-test</RequestId></ResponseMetadata>
</%[1]sResponse>`, action, key, expiration.Format(time.RFC3339))
}

func gitlabChainConfig(endpoint string) *schema.AuthConfig {
	principal := func(role string) map[string]any {
		return map[string]any{"assume_role": role, "region": "eu-west-1", "session_name": "gitlab-test", "duration": "15m"}
	}
	return &schema.AuthConfig{
		Realm:     "gitlab-test",
		Providers: map[string]schema.Provider{"gitlab": {Kind: types.ProviderKindGitLabOIDC}},
		Identities: map[string]schema.Identity{
			"runner": {
				Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "gitlab"},
				Principal: principal("arn:aws:iam::111111111111:role/GitLabRunnerRole"),
				Spec:      map[string]any{"endpoint_url": endpoint},
			},
			"production": {
				Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: "runner"},
				Principal: principal("arn:aws:iam::222222222222:role/DeployRole"),
				Spec:      map[string]any{"endpoint_url": endpoint},
			},
		},
	}
}

// assertGitLabToolCredentials checks the returned AWS identity and managed tool credentials without
// persisting the GitLab token.
func assertGitLabToolCredentials(t *testing.T, info *types.WhoamiInfo, target string, expiration time.Time) {
	t.Helper()
	key := "runner-key"
	if target == "production" {
		key = "production-key"
	}
	require.IsType(t, &types.AWSCredentials{}, info.Credentials)
	assert.Equal(t, &types.AWSCredentials{
		AccessKeyID: key, SecretAccessKey: key + "-secret", SessionToken: key + "-session",
		Region: "eu-west-1", Expiration: expiration.Format(time.RFC3339),
	}, info.Credentials)
	assert.Equal(t, target, info.Identity)
	assert.Equal(t, target, info.Environment["AWS_PROFILE"])
	files, err := awsCloud.NewAWSFileManager("", "gitlab-test")
	require.NoError(t, err)
	assert.Equal(t, files.GetCredentialsPath("gitlab"), info.Environment["AWS_SHARED_CREDENTIALS_FILE"])
	contents, err := os.ReadFile(files.GetCredentialsPath("gitlab"))
	require.NoError(t, err)
	assert.Contains(t, string(contents), key)
	assert.NotContains(t, string(contents), "gitlab-job-token")
}

func verifyGitLabOIDCRefresh(t *testing.T, newManager func() types.AuthManager, target string, wantActions []string, actions, tokens *[]string) {
	t.Helper()
	_, err := newManager().Authenticate(context.Background(), target)
	require.NoError(t, err)
	assert.Equal(t, wantActions, *actions, "new managers in one command reuse process-cached AWS credentials")
	resetProcessCredentialCache()
	t.Setenv("ATMOS_OIDC_TOKEN", "rotated-gitlab-job-token")
	_, err = newManager().Authenticate(context.Background(), target)
	require.NoError(t, err)
	wantRepeated := append(append([]string{}, wantActions...), wantActions...)
	assert.Equal(t, wantRepeated, *actions)
	assert.Equal(t, []string{"initial-gitlab-job-token", "rotated-gitlab-job-token"}, *tokens)
	// Existing AWS files must not bypass a missing job token on a subsequent run.
	resetProcessCredentialCache()
	t.Setenv("ATMOS_OIDC_TOKEN", "")
	_, err = newManager().Authenticate(context.Background(), target)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Equal(t, wantRepeated, *actions)
}
