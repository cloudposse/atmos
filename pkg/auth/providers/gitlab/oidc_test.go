package gitlab

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the provider configuration fields used below.
var _ = schema.Provider{Kind: types.ProviderKindGitLabOIDC, Spec: map[string]any{}}

func TestNewOIDCProvider(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config *schema.Provider
	}{
		{name: "gitlab"},
		{config: &schema.Provider{Kind: types.ProviderKindGitLabOIDC}},
	} {
		provider, err := NewOIDCProvider(tc.name, tc.config)
		require.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
		assert.Nil(t, provider)
	}
}

func TestOIDCProvider_Validate(t *testing.T) {
	t.Setenv(defaultTokenEnvironmentVariable, "")
	tests := []struct {
		name string
		spec map[string]any
		fail bool
	}{
		{name: "default"},
		{name: "empty spec", spec: map[string]any{}},
		{name: "custom variable", spec: map[string]any{"token_environment_variable": "GITLAB_ID_TOKEN"}},
		{name: "equals in variable", spec: map[string]any{"token_environment_variable": "TOKEN=NAME"}, fail: true},
		{name: "leading equals", spec: map[string]any{"token_environment_variable": "=TOKEN"}, fail: true},
		{name: "NUL in variable", spec: map[string]any{"token_environment_variable": "TOKEN\x00NAME"}, fail: true},
		{name: "empty variable", spec: map[string]any{"token_environment_variable": ""}, fail: true},
		{name: "blank variable", spec: map[string]any{"token_environment_variable": " \t"}, fail: true},
		{name: "wrong type", spec: map[string]any{"token_environment_variable": 42}, fail: true},
		{name: "null variable", spec: map[string]any{"token_environment_variable": nil}, fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewOIDCProvider("gitlab", &schema.Provider{Kind: types.ProviderKindGitLabOIDC, Spec: tc.spec})
			require.NoError(t, err)
			err = provider.Validate()
			if tc.fail {
				require.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
				creds, authErr := provider.Authenticate(context.Background())
				assert.ErrorIs(t, authErr, errUtils.ErrInvalidProviderConfig)
				assert.Nil(t, creds)
				return
			}
			require.NoError(t, err, "validation must not require a runtime token")
		})
	}
}

func TestOIDCProvider_Authenticate(t *testing.T) {
	// An explicitly configured token source also works outside a GitLab runner.
	t.Setenv("GITLAB_CI", "false")
	tests := []struct {
		name  string
		spec  map[string]any
		token string
		want  string
	}{
		{name: "default variable", token: "job-token", want: "job-token"},
		{name: "trimmed token", token: " \tjob-token\n", want: "job-token"},
		{name: "custom variable", spec: map[string]any{"token_environment_variable": "GITLAB_ID_TOKEN"}, token: "custom-token", want: "custom-token"},
		{name: "trimmed variable", spec: map[string]any{"token_environment_variable": " GITLAB_ID_TOKEN "}, token: "custom-token", want: "custom-token"},
		{name: "empty token"},
		{name: "blank token", token: " \t\n"},
		{name: "empty custom source does not fall back", spec: map[string]any{"token_environment_variable": "GITLAB_ID_TOKEN"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(defaultTokenEnvironmentVariable, "other-token")
			envVar := defaultTokenEnvironmentVariable
			if tc.spec != nil {
				envVar = "GITLAB_ID_TOKEN"
			}
			t.Setenv(envVar, tc.token)
			provider, err := NewOIDCProvider("gitlab", &schema.Provider{Kind: types.ProviderKindGitLabOIDC, Spec: tc.spec})
			require.NoError(t, err)
			creds, err := provider.Authenticate(context.Background())
			if tc.want == "" {
				require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				assert.Equal(t, []string{fmt.Sprintf("GitLab OIDC token environment variable %q is missing or empty", envVar)}, errUtils.AllDetails(err))
				assert.Equal(t, []string{fmt.Sprintf("Configure id_tokens.%s.aud in .gitlab-ci.yml to match the AWS IAM OIDC provider audience", envVar)}, errUtils.AllHints(err))
				assert.Nil(t, creds)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, &types.OIDCCredentials{Token: tc.want, Provider: "gitlab"}, creds)
		})
	}
}

func TestOIDCProvider_ReadsCurrentToken(t *testing.T) {
	provider, err := NewOIDCProvider("gitlab", &schema.Provider{Kind: types.ProviderKindGitLabOIDC})
	require.NoError(t, err)
	for _, token := range []string{"first-job-token", "rotated-job-token", ""} {
		t.Setenv(defaultTokenEnvironmentVariable, token)
		creds, authErr := provider.Authenticate(context.Background())
		if token == "" {
			require.ErrorIs(t, authErr, errUtils.ErrAuthenticationFailed)
			assert.Nil(t, creds)
			continue
		}
		require.NoError(t, authErr)
		assert.Equal(t, &types.OIDCCredentials{Token: token, Provider: "gitlab"}, creds)
	}
}

func TestOIDCProvider_PrepareEnvironmentIsolation(t *testing.T) {
	provider, err := NewOIDCProvider("gitlab", &schema.Provider{Kind: types.ProviderKindGitLabOIDC})
	require.NoError(t, err)
	environ := map[string]string{"AWS_REGION": "eu-west-1"}
	prepared, err := provider.PrepareEnvironment(context.Background(), environ)
	require.NoError(t, err)
	assert.Equal(t, environ, prepared)
	prepared["AWS_REGION"] = "eu-west-2"
	assert.Equal(t, "eu-west-1", environ["AWS_REGION"])
	environ["AWS_REGION"] = "eu-west-3"
	assert.Equal(t, "eu-west-2", prepared["AWS_REGION"])
}
