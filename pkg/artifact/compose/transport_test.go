package compose_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func registryConfig(identity, host string) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindOCIRegistry, Identity: identity,
		Options: map[string]any{"registry": host, "prefix": "acme/atmos"},
	}
}

func TestTransportFactoryBuildsAClientWithoutAuthenticating(t *testing.T) {
	config := authConfig()
	factory := compose.NewTransportFactory(config, nil)

	plain, err := factory(context.Background(), "plain", registryConfig("", "ghcr.io"), nil)
	require.NoError(t, err)
	assert.IsType(t, &atmosoci.Client{}, plain)

	bound, err := factory(context.Background(), "bound", registryConfig("aws-writer", "123456789012.dkr.ecr.us-east-2.amazonaws.com"), nil)
	require.NoError(t, err, "binding an identity does not authenticate; that happens on the first request")
	assert.IsType(t, &atmosoci.Client{}, bound)
}

func TestTransportFactoryRejectsAnIdentityItCannotBind(t *testing.T) {
	factory := compose.NewTransportFactory(authConfig(), nil)

	_, err := factory(context.Background(), "ghost", registryConfig("ghost", "ghcr.io"), nil)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound, "a typo'd identity never silently falls back to ambient credentials")

	_, err = factory(context.Background(), "badhost", registryConfig("aws-writer", "not a host"), nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

// An OCI repository with an identity is opened through the composition's transport factory, which
// is where the identity is bound; the default factory refuses an identity that does not exist.
func TestOpeningAnIdentityBoundRegistryUsesTheDefaultFactory(t *testing.T) {
	repos := map[string]schema.ArtifactRepositoryConfig{"registry": *registryConfig("ghost", "127.0.0.1:1")}
	composition, err := compose.New(&schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: repos}})
	require.NoError(t, err, "configuration is valid; identities resolve lazily")

	_, err = composition.Repositories.Describe("registry")
	require.NoError(t, err, "describing never opens the repository")
	_, err = composition.Service.Inspect(context.Background(), artifact.InspectRequest{Repository: "registry", Ref: appRef()})
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestInjectedTransportFactoryReceivesTheRepositoryIdentity(t *testing.T) {
	var seenAlias, seenIdentity string
	registry := newTestRegistry(t)
	repos := map[string]schema.ArtifactRepositoryConfig{"registry": *registryConfig("registry-writer", registry.host)}
	composition := build(t, repos, nil, compose.WithTransportFactory(func(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, resolve artifact.IdentityResolver) (atmosoci.Transport, error) {
		seenAlias, seenIdentity = alias, cfg.Identity
		assert.NotNil(t, resolve, "the composition's identity resolver is handed through")
		return registry.factory(ctx, alias, cfg, resolve)
	}))

	_, err := composition.Service.Publish(context.Background(), publishRequest("registry", source(t, "hello")))
	require.NoError(t, err)
	assert.Equal(t, "registry", seenAlias)
	assert.Equal(t, "registry-writer", seenIdentity)
}
