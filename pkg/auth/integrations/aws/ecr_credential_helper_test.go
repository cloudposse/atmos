package aws

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const helperTestRegistry = "123456789012.dkr.ecr.us-east-1.amazonaws.com"

// stubECRAuth stubs the ECR token seams and uses the real Docker config manager.
func stubECRAuth(t *testing.T) {
	t.Helper()
	origGetAuth, origFactory := ecrGetAuthToken, ecrDockerConfigFactory
	t.Cleanup(func() { ecrGetAuthToken, ecrDockerConfigFactory = origGetAuth, origFactory })
	ecrGetAuthToken = func(context.Context, types.ICredentials, string, string) (*awsCloud.ECRAuthResult, error) {
		return &awsCloud.ECRAuthResult{Username: "AWS", Password: "token", Registry: helperTestRegistry, ExpiresAt: time.Now().Add(12 * time.Hour)}, nil
	}
	ecrDockerConfigFactory = docker.NewConfigManager
}

// newHelperTestECRPublic returns an aws/ecr-public integration with a stubbed
// token and the real Docker config manager.
func newHelperTestECRPublic() *ECRPublicIntegration {
	return &ECRPublicIntegration{
		name: "public",
		getAuthToken: func(context.Context, types.ICredentials) (*awsCloud.ECRPublicAuthResult, error) {
			return &awsCloud.ECRPublicAuthResult{Username: "AWS", Password: "token", ExpiresAt: time.Now().Add(12 * time.Hour)}, nil
		},
	}
}

// TestECRIntegrations_EcrLoginHelperManagesRegistry verifies that when
// config.json routes the registries to docker-credential-ecr-login, the
// aws/ecr and aws/ecr-public integrations succeed without writing anything.
func TestECRIntegrations_EcrLoginHelperManagesRegistry(t *testing.T) {
	const content = `{"credHelpers":{"` + helperTestRegistry + `":"ecr-login","public.ecr.aws":"ecr-login"}}`
	path := installFakeCredentialHelper(t, "ecr-login", "not-implemented", content)
	stubECRAuth(t)

	private := &ECRIntegration{name: "dev/ecr", registry: &schema.Registry{AccountID: "123456789012", Region: "us-east-1"}}
	require.NoError(t, private.Execute(context.Background(), &types.AWSCredentials{}))
	public := newHelperTestECRPublic()
	require.NoError(t, public.Execute(context.Background(), &types.AWSCredentials{}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, content, string(data))
}

// TestECRIntegration_FailingHelperStillFails is the negative path.
func TestECRIntegration_FailingHelperStillFails(t *testing.T) {
	installFakeCredentialHelper(t, "broken", "fail", `{"credsStore":"broken"}`)
	stubECRAuth(t)

	private := &ECRIntegration{name: "dev/ecr", registry: &schema.Registry{AccountID: "123456789012", Region: "us-east-1"}}
	err := private.Execute(context.Background(), &types.AWSCredentials{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)

	err = newHelperTestECRPublic().Execute(context.Background(), &types.AWSCredentials{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
}
