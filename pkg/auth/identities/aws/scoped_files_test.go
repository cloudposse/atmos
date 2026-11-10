package aws

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.Identity{Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "gitlab"}}

// Both STS identity types must use the same private paths for writing and reading credentials.
func TestAWSIdentityInvocationFiles(t *testing.T) {
	tests := []struct {
		kind   string
		create func(string, *schema.Identity) (types.Identity, error)
	}{
		{kind: "aws/assume-role", create: NewAssumeRoleIdentity},
		{kind: "aws/assume-root", create: NewAssumeRootIdentity},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			id, err := tc.create("target", &schema.Identity{Kind: tc.kind, Via: &schema.IdentityVia{Provider: "gitlab"}})
			require.NoError(t, err)
			id.SetRealm("test-realm")
			base := t.TempDir()
			id.(interface{ SetAWSFilesBasePath(string) }).SetAWSFilesBasePath(base)
			ctx := context.Background()
			authContext := &schema.AuthContext{}
			creds := &types.AWSCredentials{
				AccessKeyID: "test-key", SecretAccessKey: "synthetic-secret", SessionToken: "test-session",
				Region: "eu-west-1", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			}
			require.NoError(t, id.PostAuthenticate(ctx, &types.PostAuthenticateParams{
				ProviderName: "gitlab", IdentityName: "target", Credentials: creds,
				Realm: "test-realm", AuthContext: authContext,
			}))
			want := filepath.Join(base, "test-realm", "aws", "gitlab", "credentials")
			env, err := id.Environment()
			require.NoError(t, err)
			assert.Equal(t, want, env["AWS_SHARED_CREDENTIALS_FILE"])
			assert.Equal(t, want, authContext.AWS.CredentialsFile)
			prepared, err := id.PrepareEnvironment(ctx, map[string]string{"AWS_ACCESS_KEY_ID": "ambient-mock"})
			require.NoError(t, err)
			assert.Equal(t, want, prepared["AWS_SHARED_CREDENTIALS_FILE"])
			assert.NotContains(t, prepared, "AWS_ACCESS_KEY_ID")
			exists, err := id.CredentialsExist()
			require.NoError(t, err)
			require.True(t, exists)
			loaded, err := id.LoadCredentials(ctx)
			require.NoError(t, err)
			assert.Equal(t, "test-key", loaded.(*types.AWSCredentials).AccessKeyID)

			require.NoError(t, id.Logout(ctx))

			assert.NoFileExists(t, want)
			shared, err := awsCloud.NewAWSFileManager("", "test-realm")
			require.NoError(t, err)
			assert.NoFileExists(t, shared.GetCredentialsPath("gitlab"))
		})
	}
}
