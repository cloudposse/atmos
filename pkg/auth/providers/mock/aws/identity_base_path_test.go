package aws

// Tests that the mock AWS identity writes its AWS files under the root provider's
// spec.files.base_path, like the real AWS identities (issue #3720, finding 4.9).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the provider spec shape this test relies on.
var _ = schema.Provider{Kind: "mock/aws", Spec: map[string]any{"files": map[string]any{"base_path": ""}}}

func TestIdentity_PostAuthenticateHonorsProviderFilesBasePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
	basePath := filepath.Join(t.TempDir(), "custom-aws")
	files, err := awsCloud.NewAWSFileManager(basePath, "fixture")
	require.NoError(t, err)
	wantCredentials, wantConfig := files.GetCredentialsPath("mock-provider"), files.GetConfigPath("mock-provider")

	provider := &schema.Provider{Kind: "mock/aws", Spec: map[string]any{"files": map[string]any{"base_path": basePath}}}
	manager := types.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().ResolveProviderConfig("base-path-identity").Return(provider, true).AnyTimes()
	manager.EXPECT().GetProviderForIdentity(gomock.Any()).Return("mock-provider").AnyTimes()
	manager.EXPECT().GetIdentities().Return(nil).AnyTimes()
	manager.EXPECT().ResolvePrincipalSetting(gomock.Any(), gomock.Any()).Return(nil, false).AnyTimes()

	identity := NewIdentity("base-path-identity", &schema.Identity{Kind: "mock/aws"})
	t.Cleanup(func() { _ = os.Remove(identity.getCredentialsFilePath()) })
	authContext := &schema.AuthContext{}

	require.NoError(t, identity.PostAuthenticate(t.Context(), &types.PostAuthenticateParams{
		ProviderName: "mock-provider", IdentityName: "base-path-identity", Realm: "fixture",
		AuthContext: authContext, StackInfo: &schema.ConfigAndStacksInfo{}, Manager: manager,
	}))

	require.NotNil(t, authContext.AWS)
	assert.Equal(t, wantCredentials, authContext.AWS.CredentialsFile, "the auth context must point at the base_path files")
	assert.Equal(t, wantConfig, authContext.AWS.ConfigFile)
	assert.FileExists(t, wantCredentials, "credentials must be written where the provider reports and cleans them")
	assert.FileExists(t, wantConfig)
}
