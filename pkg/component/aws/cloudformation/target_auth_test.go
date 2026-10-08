package cloudformation

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// targetAuthFixture provides global identities with independent component and target defaults.
func targetAuthFixture() (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, map[string]any) {
	ac := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
		"sandbox": {Kind: "aws/user", Default: true},
		"dev":     {Kind: "aws/user"},
	}}}
	info := &schema.ConfigAndStacksInfo{
		Stack: "test", ComponentFromArg: "demo", Identity: "sandbox",
		AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "sandbox"}},
		ComponentSection: map[string]any{"auth": map[string]any{"identities": map[string]any{
			"sandbox": map[string]any{"default": true},
		}}},
	}
	block := map[string]any{
		"kind": kindAwsS3, "bucket": "artifacts", "region": "us-east-2",
		"auth": map[string]any{"identities": map[string]any{"dev": map[string]any{"default": true}}},
	}
	info.ComponentSection[cfg.ProvisionSectionName] = map[string]any{"targets": map[string]any{"artifacts": block}}
	return ac, info, block
}

// stubTargetAuthentication injects observable target credentials without authenticating against AWS.
func stubTargetAuthentication(t *testing.T, endpoint, credentials string) {
	t.Helper()
	original := createTargetAuthManager
	t.Cleanup(func() { createTargetAuthManager = original })
	createTargetAuthManager = func(identity string, merged *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, stack string) (auth.AuthManager, error) {
		require.Equal(t, "test", stack)
		if identity == "" {
			for name := range merged.Identities {
				if merged.Identities[name].Default {
					require.Empty(t, identity, "target merge must clear component default")
					identity = name
				}
			}
		}
		require.NotEmpty(t, identity)
		require.Contains(t, merged.Identities, identity)
		manager := authtypes.NewMockAuthManager(gomock.NewController(t))
		manager.EXPECT().GetChain().Return([]string{identity}).AnyTimes()
		manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{
			Stack: stack, AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{
				Profile: identity, CredentialsFile: credentials, Region: "us-east-2", EndpointURL: endpoint,
			}},
		}).AnyTimes()
		manager.EXPECT().Authenticate(gomock.Any(), identity).Return(nil, nil).AnyTimes()
		return manager, nil
	}
}

// TestResolveTargetAuthPrecedenceAndIsolation checks CLI and target selection without mutating the parent auth scope.
func TestResolveTargetAuthPrecedenceAndIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, requested, want string
	}{
		{"target default replaces component default", "", "dev"},
		{"explicit CLI overrides target", "sandbox", "sandbox"},
		{"prompt selection stays selected", cfg.IdentityFlagSelectValue, "sandbox"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, info, block := targetAuthFixture()
			stubTargetAuthentication(t, "", "")
			resolved, err := ResolveTargetAuth(ac, info, "artifacts", block, tc.requested)
			require.NoError(t, err)
			assert.Equal(t, tc.want, resolved.Identity)
			assert.Equal(t, tc.want, resolved.AuthContext.AWS.Profile)
			assert.Equal(t, "sandbox", info.Identity)
			assert.True(t, ac.Auth.Identities["sandbox"].Default)
			assert.False(t, ac.Auth.Identities["dev"].Default)
			resolved.AuthContext.AWS.Profile = "changed-target"
			assert.Equal(t, "sandbox", info.AuthContext.AWS.Profile)
			info.AuthContext.AWS.Profile = "changed-parent"
			assert.Equal(t, "changed-target", resolved.AuthContext.AWS.Profile)
			assert.True(t, block["auth"].(map[string]any)["identities"].(map[string]any)["dev"].(map[string]any)["default"].(bool))
		})
	}
}

// TestResolveTargetAuthAbsentDisabledAndInvalid checks omitted, disabled and malformed target-auth configuration.
func TestResolveTargetAuthAbsentDisabledAndInvalid(t *testing.T) {
	ac, info, block := targetAuthFixture()
	original := createTargetAuthManager
	t.Cleanup(func() { createTargetAuthManager = original })
	createTargetAuthManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (auth.AuthManager, error) {
		t.Fatal("invalid, absent, or disabled auth must not authenticate")
		return nil, nil
	}
	resolved, err := ResolveTargetAuth(ac, info, "", nil, "")
	require.NoError(t, err)
	assert.Same(t, info, resolved)
	resolved, err = ResolveTargetAuth(ac, info, "artifacts", block, cfg.IdentityFlagDisabledValue)
	require.NoError(t, err)
	assert.True(t, resolved.AuthDisabled)
	assert.Nil(t, resolved.AuthContext)
	assert.Equal(t, "sandbox", info.AuthContext.AWS.Profile)

	for _, raw := range []any{"dev", map[string]any{"identity": 123}, map[string]any{"identities": map[string]any{"missing": map[string]any{"default": true}}}} {
		_, err := ResolveTargetAuth(ac, info, "artifacts", map[string]any{"auth": raw}, "")
		require.Error(t, err)
	}
}

// TestResolveTargetAuthFailureDoesNotFallBack ensures failed target authentication cannot reuse component credentials.
func TestResolveTargetAuthFailureDoesNotFallBack(t *testing.T) {
	ac, info, block := targetAuthFixture()
	original := createTargetAuthManager
	t.Cleanup(func() { createTargetAuthManager = original })
	createTargetAuthManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (auth.AuthManager, error) {
		return nil, errUtils.ErrAuthenticationFailed
	}
	_, err := ResolveTargetAuth(ac, info, "artifacts", block, "")
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationTargetAuthFailed)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetAuthFailed)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	targetName, _ := errUtils.GetContext(err, "target")
	assert.Equal(t, "artifacts", targetName)
	componentName, _ := errUtils.GetContext(err, "component")
	assert.Equal(t, "demo", componentName)
	assert.Equal(t, "sandbox", info.AuthContext.AWS.Profile)
}

// TestTargetAuthReachesS3SignedRequests verifies packaging requests are signed with the target credentials.
func TestTargetAuthReachesS3SignedRequests(t *testing.T) {
	for _, requested := range []string{"", "sandbox"} {
		t.Run("identity="+requested, func(t *testing.T) {
			var mu sync.Mutex
			var signed []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				signed = append(signed, r.Header.Get("Authorization"))
				mu.Unlock()
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			credentials := writeTargetCredentials(t)
			stubTargetAuthentication(t, server.URL, credentials)
			ac, info, block := targetAuthFixture()
			octx := &opContext{Ctx: t.Context(), AtmosConfig: ac, Info: info, RequestedIdentity: requested}
			provision := info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
			summary := map[string]any{}
			err := packageIfNeeded(octx, provision, &target.SelectedTarget{Name: "artifacts", Kind: kindAwsS3, Config: block}, &stackSpec{TemplateBody: "Resources: {}"}, summary)
			require.NoError(t, err)
			want := "dev"
			if requested != "" {
				want = requested
			}
			mu.Lock()
			defer mu.Unlock()
			require.GreaterOrEqual(t, len(signed), 2, "must observe real SDK upload requests")
			for _, header := range signed {
				assert.Contains(t, header, "Credential="+want+"/")
			}
			assert.Contains(t, summary["package_s3_uri"], "s3://artifacts/")
			assert.Equal(t, "sandbox", info.Identity)
		})
	}
}

// writeTargetCredentials writes synthetic credentials for local signing tests.
func writeTargetCredentials(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.WriteFile(path, []byte("[dev]\naws_access_key_id=dev\naws_secret_access_key=secret\n[sandbox]\naws_access_key_id=sandbox\naws_secret_access_key=secret\n"), 0o600))
	return path
}

// TestTargetAuthReachesCloudFormationClient verifies deployment requests use the direct target's signing credentials.
func TestTargetAuthReachesCloudFormationClient(t *testing.T) {
	for _, operation := range []Operation{OperationApply, OperationDiff, OperationOutput, OperationDelete, OperationChangesetExecute, OperationStackSetCreate, OperationStackSetUpdate} {
		t.Run(string(operation), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, r.Header.Get("Authorization"), "Credential=dev/")
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/"><DescribeStacksResult><Stacks/></DescribeStacksResult></DescribeStacksResponse>`)
			}))
			defer server.Close()
			stubTargetAuthentication(t, server.URL, writeTargetCredentials(t))
			ac, info, block := targetAuthFixture()
			block["kind"] = cfg.CloudFormationComponentType
			if operation == OperationStackSetCreate || operation == OperationStackSetUpdate {
				block["kind"] = kindAwsStackSet
			}
			provision := info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
			provision["default"] = "artifacts"
			client, err := clientForOperation(&opContext{Ctx: t.Context(), AtmosConfig: ac, Info: info}, operation)
			require.NoError(t, err)
			_, err = client.DescribeStacks(t.Context(), &cloudformation.DescribeStacksInput{})
			require.NoError(t, err)
			assert.Equal(t, "sandbox", info.AuthContext.AWS.Profile)
		})
	}
}

// TestExternalTargetAuthIdentityAndRepositoryFallback checks Git target identity selection and repository-auth fallback.
func TestExternalTargetAuthIdentityAndRepositoryFallback(t *testing.T) {
	ac, info, block := targetAuthFixture()
	stubTargetAuthentication(t, "", "")
	octx := &opContext{Ctx: t.Context(), AtmosConfig: ac, Info: info}
	resolved, selected, err := externalTargetAuth(octx, &target.SelectedTarget{Kind: "git", Config: block})
	require.NoError(t, err)
	assert.Equal(t, "dev", resolved.Identity)
	assert.Equal(t, "dev", selected["auth"].(map[string]any)["identity"])
	assert.NotContains(t, block["auth"], "identity")

	_, selected, err = externalTargetAuth(octx, &target.SelectedTarget{Kind: "git", Config: map[string]any{"repository": "repo"}})
	require.NoError(t, err)
	assert.NotContains(t, selected, "auth", "repository identity remains authoritative without override")
	octx.RequestedIdentity = "sandbox"
	_, selected, err = externalTargetAuth(octx, &target.SelectedTarget{Kind: "git", Config: block})
	require.NoError(t, err)
	assert.Equal(t, "sandbox", selected["auth"].(map[string]any)["identity"])
	assert.Equal(t, "sandbox", info.Identity)
}

// TestStackSetReadDeleteDoNotRequireTargetAuth keeps existing StackSet operations independent of delivery-target selection.
func TestStackSetReadDeleteDoNotRequireTargetAuth(t *testing.T) {
	ac, info, block := targetAuthFixture()
	block["auth"] = "invalid-but-unselected"
	for _, operation := range []Operation{OperationStackSetDelete, OperationStackSetInstances} {
		_, selected, err := operationTargetConfig(&opContext{AtmosConfig: ac, Info: info}, operation)
		require.NoError(t, err)
		assert.Nil(t, selected)
	}
}

// TestExternalTargetAuthExplicitIdentityWithoutTargetBlock preserves a CLI identity when external delivery has no auth override.
func TestExternalTargetAuthExplicitIdentityWithoutTargetBlock(t *testing.T) {
	ac, info, _ := targetAuthFixture()
	_, block, err := externalTargetAuth(&opContext{AtmosConfig: ac, Info: info, RequestedIdentity: "sandbox"}, &target.SelectedTarget{Kind: "git"})
	require.NoError(t, err)
	assert.Equal(t, "sandbox", block["auth"].(map[string]any)["identity"])
}
