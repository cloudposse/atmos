package discovery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/internal/testkit"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

func TestScopeFailureSurfacesBeforeAnyRegistryAccess(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	failure := errors.New("realm is invalid")
	e.disc = discovery.New(func() (string, error) { return "", failure }, discovery.WithDirectory(e.dir))
	svc, err := service.New(e.repos, nil, service.WithEnumerator(e.disc))
	require.NoError(t, err)
	e.registry.Reset()
	_, err = svc.List(context.Background(), artifact.ListRequest{})
	require.ErrorIs(t, err, failure)
	assert.Zero(t, e.registry.Total())
}

func TestOversizedSnapshotFailsInsteadOfBeingTruncated(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	e.disc = discovery.New(func() (string, error) { return "scope", nil }, discovery.WithDirectory(e.dir), discovery.WithMaxSize(64), discovery.WithTTL(time.Hour))
	svc, err := service.New(e.repos, nil, service.WithEnumerator(e.disc))
	require.NoError(t, err)
	result, err := svc.List(context.Background(), artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryTooLarge)
	assert.Empty(t, result.Artifacts)
	assert.Empty(t, e.snapshots())
}

// denyAll rejects every source, including live ones.
type denyAll struct{ discovery.LocationPolicy }

func (denyAll) AuthorizeSource(context.Context, *discovery.SourceCheck) error { return errPolicy }

func TestPolicyRejectionAppliesToLiveRepositoriesToo(t *testing.T) {
	backups := newLocalRepo(t)
	e := newEnv(t, envOptions{
		extraRepos: map[string]artifact.Repository{"backups": backups},
		extraInfo:  map[string]service.RepositoryInfo{"backups": {Capabilities: backups.Capabilities(), Discovery: service.DiscoveryInfo{Kind: artifact.RepositoryKindLocalDir, Location: "x"}}},
		policy:     denyAll{},
	})
	testkit.Publish(t, backups, testkit.Spec{Name: "app", Version: "1.0.0", Content: "x", Packaging: artifact.PackagingNone})
	_, err := e.list(artifact.ListRequest{Repositories: []string{"backups"}})
	require.ErrorIs(t, err, errPolicy)
}

func TestTTLOptionControlsFreshness(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	e.disc = discovery.New(func() (string, error) { return "scope", nil }, discovery.WithDirectory(e.dir), discovery.WithClock(e.clock.Now), discovery.WithTTL(time.Minute))
	svc, err := service.New(e.repos, nil, service.WithEnumerator(e.disc))
	require.NoError(t, err)
	_, err = svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err)
	e.registry.Reset()
	e.clock.advance(30 * time.Second)
	_, err = svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err)
	assert.Zero(t, e.registry.Total())
	e.clock.advance(time.Minute)
	_, err = svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err)
	assert.Positive(t, e.registry.Total(), "a snapshot older than the configured TTL is revalidated")
}

func TestListingWithoutAnEnumeratorReadsRepositoriesDirectly(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", map[string]string{"team": "a"})
	e.publish("app", "1.1.0", map[string]string{"team": "b"})
	svc, err := service.New(e.repos, nil)
	require.NoError(t, err)
	result, err := svc.List(context.Background(), artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "b"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.1.0"}, refs(result))
	assert.Empty(t, result.Repositories[0].Source, "without discovery there is no cache provenance to report")
	e.registry.Reset()
	_, err = svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err)
	assert.Positive(t, e.registry.Total(), "no cache: every listing reads the registry")
}
