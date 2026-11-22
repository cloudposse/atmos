package aws

// Tests that AWS identities honor the root provider's spec.files.base_path,
// the directory the provider reports in Paths and cleans up on logout
// (issue #3720, finding 4.9).

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the provider spec shape these tests rely on.
var _ = schema.Provider{Kind: "aws/iam-identity-center", Spec: map[string]any{"files": map[string]any{"base_path": ""}}}

// basePathManager returns an auth manager whose root provider for every
// identity is "aws-sso" with spec.files.base_path set to basePath.
func basePathManager(t *testing.T, basePath string) types.AuthManager {
	t.Helper()

	provider := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-east-1",
		StartURL: "https://example.awsapps.com/start",
		Spec:     map[string]any{"files": map[string]any{"base_path": basePath}},
	}
	manager := types.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().ResolveProviderConfig(gomock.Any()).Return(provider, true).AnyTimes()
	manager.EXPECT().GetProviderForIdentity(gomock.Any()).Return("aws-sso").AnyTimes()
	manager.EXPECT().GetIdentities().Return(nil).AnyTimes()
	manager.EXPECT().ResolvePrincipalSetting(gomock.Any(), gomock.Any()).Return(nil, false).AnyTimes()
	return manager
}

// basePathIdentity is the slice of the AWS identities this test exercises.
type basePathIdentity interface {
	types.Identity
	CredentialsExist() (bool, error)
}

func TestAWSIdentities_HonorProviderFilesBasePath(t *testing.T) {
	newIdentities := map[string]func(t *testing.T) basePathIdentity{
		"permission set": func(t *testing.T) basePathIdentity {
			identity, err := NewPermissionSetIdentity("dev", &schema.Identity{
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]any{
					"name":    "DevAccess",
					"account": map[string]any{"id": "123456789012"},
				},
			})
			require.NoError(t, err)
			return identity.(basePathIdentity)
		},
		"assume role": func(t *testing.T) basePathIdentity {
			identity, err := NewAssumeRoleIdentity("dev", &schema.Identity{
				Kind:      "aws/assume-role",
				Via:       &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]any{"assume_role": "arn:aws:iam::123456789012:role/dev"},
			})
			require.NoError(t, err)
			return identity.(basePathIdentity)
		},
		"assume root": func(t *testing.T) basePathIdentity {
			identity, err := NewAssumeRootIdentity("dev", &schema.Identity{
				Kind: "aws/assume-root",
				Via:  &schema.IdentityVia{Identity: "admin"},
				Principal: map[string]any{
					"target_principal": "123456789012",
					"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				},
			})
			require.NoError(t, err)
			return identity.(basePathIdentity)
		},
	}

	for name, newIdentity := range newIdentities {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
			basePath := filepath.Join(t.TempDir(), "custom-aws")
			manager := basePathManager(t, basePath)

			fileManager, err := awsCloud.NewAWSFileManager(basePath, "")
			require.NoError(t, err)
			wantCredentials := fileManager.GetCredentialsPath("aws-sso")
			wantConfig := fileManager.GetConfigPath("aws-sso")

			identity := newIdentity(t)
			authContext := &schema.AuthContext{}
			stack := &schema.ConfigAndStacksInfo{}
			err = identity.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
				AuthContext:  authContext,
				StackInfo:    stack,
				ProviderName: "aws-sso",
				IdentityName: "dev",
				Credentials:  &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", SessionToken: "TK", Region: "us-east-1"},
				Manager:      manager,
			})
			require.NoError(t, err)

			// Credentials are written where the provider reports and cleans them.
			require.NotNil(t, authContext.AWS)
			assert.Equal(t, wantCredentials, authContext.AWS.CredentialsFile)
			assert.Equal(t, wantConfig, authContext.AWS.ConfigFile)
			assert.FileExists(t, wantCredentials)

			env, err := identity.Environment()
			require.NoError(t, err)
			assert.Equal(t, wantCredentials, env["AWS_SHARED_CREDENTIALS_FILE"])
			assert.Equal(t, wantConfig, env["AWS_CONFIG_FILE"])

			prepared, err := identity.PrepareEnvironment(context.Background(), map[string]string{})
			require.NoError(t, err)
			assert.Equal(t, wantCredentials, prepared["AWS_SHARED_CREDENTIALS_FILE"])

			exists, err := identity.CredentialsExist()
			require.NoError(t, err)
			assert.True(t, exists, "credentials written under base_path must be found there")

			require.NoError(t, identity.Logout(context.Background()))
			exists, err = identity.CredentialsExist()
			require.NoError(t, err)
			assert.False(t, exists, "logout must remove the credentials written under base_path")
		})
	}
}
