package auth

// Tests that native plugin identities materialize AWS credentials under the root provider's
// spec.files.base_path, the directory the provider reports in Paths and cleans up on logout
// (issue #3720, finding 4.9).

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the provider spec shape these tests rely on.
var _ = schema.Provider{Kind: "aws/iam-identity-center", Spec: map[string]any{"files": map[string]any{"base_path": ""}}}

// basePathAuthManager resolves every identity to the "corporate" root provider with
// spec.files.base_path set to basePath.
func basePathAuthManager(t *testing.T, basePath string) types.AuthManager {
	t.Helper()
	provider := &schema.Provider{Kind: "aws/iam-identity-center", Spec: map[string]any{"files": map[string]any{"base_path": basePath}}}
	manager := types.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().ResolveProviderConfig("broker").Return(provider, true).AnyTimes()
	manager.EXPECT().GetProviderForIdentity(gomock.Any()).Return("corporate").AnyTimes()
	manager.EXPECT().GetIdentities().Return(nil).AnyTimes()
	manager.EXPECT().ResolvePrincipalSetting(gomock.Any(), gomock.Any()).Return(nil, false).AnyTimes()
	return manager
}

// isolateAWSFiles keeps the default AWS file location inside the test.
func isolateAWSFiles(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
}

// basePathFiles returns the credentials and config paths under basePath.
func basePathFiles(t *testing.T, basePath, realm string) (credentials, config string) {
	t.Helper()
	files, err := awsCloud.NewAWSFileManager(basePath, realm)
	require.NoError(t, err)
	return files.GetCredentialsPath("corporate"), files.GetConfigPath("corporate")
}

func TestAWSMaterializerHonorsProviderFilesBasePath(t *testing.T) {
	isolateAWSFiles(t)
	basePath := filepath.Join(t.TempDir(), "custom-aws")
	wantCredentials, wantConfig := basePathFiles(t, basePath, "fixture")
	materializer := AWSMaterializer("broker", &schema.Identity{Via: &schema.IdentityVia{Provider: "corporate"}})
	materializer.SetRealm("fixture")
	authContext := &schema.AuthContext{}
	credentials := &types.AWSCredentials{
		AccessKeyID: "access-key", SecretAccessKey: "secret-key", SessionToken: "session-token",
		Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Region: "us-west-2",
	}

	require.NoError(t, materializer.Apply(t.Context(), &types.PostAuthenticateParams{
		ProviderName: "corporate", Realm: "fixture", Credentials: credentials,
		AuthContext: authContext, StackInfo: &schema.ConfigAndStacksInfo{}, Manager: basePathAuthManager(t, basePath),
	}))

	require.NotNil(t, authContext.AWS)
	assert.Equal(t, wantCredentials, authContext.AWS.CredentialsFile, "the auth context must point at the base_path files")
	assert.Equal(t, wantConfig, authContext.AWS.ConfigFile)
	assert.FileExists(t, wantCredentials, "credentials must be written where the provider reports and cleans them")
	environment, err := materializer.Environment()
	require.NoError(t, err)
	assert.Equal(t, wantCredentials, environment["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, wantConfig, environment["AWS_CONFIG_FILE"])
	paths, err := materializer.Paths()
	require.NoError(t, err)
	require.Len(t, paths, 2)
	assert.Equal(t, wantCredentials, paths[0].Location)
	assert.Equal(t, wantConfig, paths[1].Location)
	loaded, err := materializer.Load(t.Context())
	require.NoError(t, err)
	require.IsType(t, &types.AWSCredentials{}, loaded)
	assert.Equal(t, "access-key", loaded.(*types.AWSCredentials).AccessKeyID)
	require.NoError(t, materializer.Logout(t.Context()))
	_, err = os.Stat(wantCredentials)
	require.ErrorIs(t, err, os.ErrNotExist, "logout must remove the credentials written under base_path")
}

// TestPluginIdentityResolvesBasePathBeforeAuthentication covers a fresh process that reuses cached
// credentials: the auth manager hands the identity its manager before Environment, Paths, or
// Logout run, without a PostAuthenticate call.
func TestPluginIdentityResolvesBasePathBeforeAuthentication(t *testing.T) {
	isolateAWSFiles(t)
	basePath := filepath.Join(t.TempDir(), "custom-aws")
	wantCredentials, wantConfig := basePathFiles(t, basePath, "")
	constructed, err := IdentityConstructor(&Definition{}, []string{"aws/v1"}, AWSMaterializer)("broker", &schema.Identity{
		Kind: "plugin/broker", Via: &schema.IdentityVia{Provider: "corporate"},
	})
	require.NoError(t, err)
	aware, ok := constructed.(interface {
		SetManagerAndProvider(types.AuthManager, string)
	})
	require.True(t, ok, "the auth manager binds AWS identities through SetManagerAndProvider")

	aware.SetManagerAndProvider(basePathAuthManager(t, basePath), "corporate")

	environment, err := constructed.Environment()
	require.NoError(t, err)
	assert.Equal(t, wantCredentials, environment["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, wantConfig, environment["AWS_CONFIG_FILE"])
	paths, err := constructed.Paths()
	require.NoError(t, err)
	require.Len(t, paths, 2)
	assert.Equal(t, wantCredentials, paths[0].Location)
	assert.Equal(t, wantConfig, paths[1].Location)
	provider, err := constructed.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "corporate", provider)
}

// TestPluginIdentityWithoutAWSMaterializerIgnoresManager keeps non-AWS materializers unaffected.
func TestPluginIdentityWithoutAWSMaterializerIgnoresManager(t *testing.T) {
	constructed, err := IdentityConstructor(&Definition{}, []string{"oidc/v1"}, EnvironmentMaterializer)("broker", &schema.Identity{
		Kind: "plugin/broker", Via: &schema.IdentityVia{Provider: "corporate"},
	})
	require.NoError(t, err)
	aware, ok := constructed.(interface {
		SetManagerAndProvider(types.AuthManager, string)
	})
	require.True(t, ok)
	aware.SetManagerAndProvider(types.NewMockAuthManager(gomock.NewController(t)), "root")
	provider, err := constructed.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "root", provider, "the root provider is still bound")
}
