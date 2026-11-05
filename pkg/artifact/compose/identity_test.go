package compose_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// recordingBridge is the credential bridge a test injects in place of a real auth manager.
type recordingBridge struct {
	aws, azure, gcp atomic.Int64
	fail            error
}

func (b *recordingBridge) ResolveAWSAuthContext(_ context.Context, name string) (*store.AWSAuthConfig, error) {
	b.aws.Add(1)
	return &store.AWSAuthConfig{Profile: name, Region: "us-east-2"}, b.fail
}

func (b *recordingBridge) ResolveAzureAuthContext(_ context.Context, name string) (*store.AzureAuthConfig, error) {
	b.azure.Add(1)
	return &store.AzureAuthConfig{TenantID: name}, b.fail
}

func (b *recordingBridge) ResolveGCPAuthContext(_ context.Context, name string) (*store.GCPAuthConfig, error) {
	b.gcp.Add(1)
	return &store.GCPAuthConfig{ProjectID: name}, b.fail
}

func authConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
		"aws-writer":   {Kind: "aws/assume-role"},
		"azure-writer": {Kind: "azure/subscription"},
		"gcp-writer":   {Kind: "gcp/service-account"},
		"token":        {Kind: "github/token"},
	}}}
}

func TestIdentityResolverReturnsTheCredentialTypeOfTheIdentitysCloud(t *testing.T) {
	bridge := &recordingBridge{}
	var builds atomic.Int64
	resolve := compose.NewIdentityResolverForTest(authConfig(), func(*schema.AtmosConfiguration) (store.AuthContextResolver, error) {
		builds.Add(1)
		return bridge, nil
	})
	assert.Zero(t, builds.Load(), "constructing the resolver builds no auth manager")

	value, err := resolve(context.Background(), "aws-writer")
	require.NoError(t, err)
	assert.Equal(t, &store.AWSAuthConfig{Profile: "aws-writer", Region: "us-east-2"}, value)
	value, err = resolve(context.Background(), "azure-writer")
	require.NoError(t, err)
	assert.Equal(t, &store.AzureAuthConfig{TenantID: "azure-writer"}, value)
	value, err = resolve(context.Background(), "gcp-writer")
	require.NoError(t, err)
	assert.Equal(t, &store.GCPAuthConfig{ProjectID: "gcp-writer"}, value)
	assert.Equal(t, int64(1), builds.Load(), "the auth bridge is built once, on first use")
}

func TestIdentityResolverFailsClosed(t *testing.T) {
	bridge := &recordingBridge{}
	resolve := compose.NewIdentityResolverForTest(authConfig(), func(*schema.AtmosConfiguration) (store.AuthContextResolver, error) { return bridge, nil })

	_, err := resolve(context.Background(), "nobody")
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	_, err = resolve(context.Background(), "token")
	require.ErrorIs(t, err, errUtils.ErrBlobIdentityUnresolved, "an identity without cloud credentials is never accepted")
	assert.Zero(t, bridge.aws.Load()+bridge.azure.Load()+bridge.gcp.Load(), "unknown and unsupported identities never authenticate")

	boom := errors.New("sso expired")
	failing := compose.NewIdentityResolverForTest(authConfig(), func(*schema.AtmosConfiguration) (store.AuthContextResolver, error) {
		return &recordingBridge{fail: boom}, nil
	})
	_, err = failing(context.Background(), "aws-writer")
	require.ErrorIs(t, err, boom)

	broken := compose.NewIdentityResolverForTest(authConfig(), func(*schema.AtmosConfiguration) (store.AuthContextResolver, error) { return nil, boom })
	_, err = broken(context.Background(), "aws-writer")
	require.ErrorIs(t, err, boom)
	_, err = broken(context.Background(), "aws-writer")
	require.ErrorIs(t, err, boom, "a failed build is remembered, not retried per call")
}

// An S3 repository that names an identity receives that identity's resolver result, lazily.
func TestRepositoriesGetTheIdentityResolverAndUseItOnDemand(t *testing.T) {
	var asked atomic.Value
	repos := map[string]schema.ArtifactRepositoryConfig{
		"bucket": {Kind: artifact.RepositoryKindS3, Identity: "aws-writer", Options: map[string]any{"bucket": "acme-artifacts", "endpoint": "http://127.0.0.1:1", "region": "us-east-2"}},
	}
	composition := build(t, repos, nil, compose.WithIdentityResolver(func(_ context.Context, name string) (any, error) {
		asked.Store(name)
		return nil, errors.New("stop here")
	}))
	assert.Nil(t, asked.Load(), "nothing resolved yet")

	_, err := composition.Service.Pull(context.Background(), artifact.PullRequest{Repository: "bucket", Ref: appRef(), Destination: t.TempDir()})
	require.Error(t, err)
	assert.Equal(t, "aws-writer", asked.Load(), "the repository's own identity is the one resolved")
}
