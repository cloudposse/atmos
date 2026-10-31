package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

func out(t *testing.T) string {
	return filepath.Join(t.TempDir(), "out", "pulled")
}

func TestPull_RoundTripEveryPackaging(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			e := newEnv(t, envConfig{})
			e.setInfo("dst", func(info *service.RepositoryInfo) { info.Packaging = packaging })
			src := e.source("hello")
			published := e.publish("dst", src)

			dest := out(t)
			result, err := e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest})
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeRead, result.Outcome)
			assert.Equal(t, dest, result.Location)
			assert.Equal(t, published.Operation.RecordDigest, result.RecordDigest)
			assert.Equal(t, "tools/example:1.0.0", result.Ref)

			state, err := materialize.InspectDestination(dest, &published.Record)
			require.NoError(t, err)
			assert.Equal(t, materialize.DestinationMatches, state)
		})
	}
}

func TestPull_ExistingDestinationRules(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.publish("dst", e.source("hello"))
	dest := out(t)
	request := artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest}
	_, err := e.svc.Pull(context.Background(), request)
	require.NoError(t, err)
	reads := e.repos["dst"].Calls("Read")

	again, err := e.svc.Pull(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome, "an identical destination is a no-op")
	assert.Equal(t, reads, e.repos["dst"].Calls("Read"), "nothing is downloaded for an identical destination")

	fixtures.WriteFile(t, dest, "stale.txt", "stale", 0o644)
	require.NoError(t, os.WriteFile(filepath.Join(dest, "data.txt"), []byte("locally edited"), 0o644))
	_, err = e.svc.Pull(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a different non-empty destination needs force")
	data, readErr := os.ReadFile(filepath.Join(dest, "data.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "locally edited", string(data), "a refused pull leaves the destination untouched")

	request.Force = true
	forced, err := e.svc.Pull(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeRead, forced.Outcome)
	_, statErr := os.Stat(filepath.Join(dest, "stale.txt"))
	assert.True(t, os.IsNotExist(statErr), "stale files are removed, never merged")

	empty := out(t)
	require.NoError(t, os.MkdirAll(empty, 0o755))
	request.Destination, request.Force = empty, false
	_, err = e.svc.Pull(context.Background(), request)
	require.NoError(t, err, "an empty destination needs no force")
}

func TestPull_Failures(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.publish("dst", e.source("hello"))
	tests := []struct {
		name    string
		request artifact.PullRequest
		want    error
	}{
		{"missing repository", artifact.PullRequest{Ref: exampleRef, Destination: out(t)}, errUtils.ErrArtifactConfigInvalid},
		{"missing destination", artifact.PullRequest{Repository: "dst", Ref: exampleRef}, errUtils.ErrArtifactConfigInvalid},
		{"invalid ref", artifact.PullRequest{Repository: "dst", Ref: artifact.Ref{Name: "x"}, Destination: out(t)}, errUtils.ErrArtifactRefInvalid},
		{"not found", artifact.PullRequest{Repository: "dst", Ref: ref("tools/example", "9.9.9"), Destination: out(t)}, errUtils.ErrArtifactNotFound},
		{"unknown repository", artifact.PullRequest{Repository: "nope", Ref: exampleRef, Destination: out(t)}, errUtils.ErrArtifactConfigInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.svc.Pull(context.Background(), tt.request)
			require.ErrorIs(t, err, tt.want)
			require.NotNil(t, result)
			assert.NotEmpty(t, result.OperationID)
			_, statErr := os.Stat(tt.request.Destination)
			assert.True(t, tt.request.Destination == "" || os.IsNotExist(statErr), "a failed pull creates nothing")
		})
	}
}

func TestPull_WriteOnlyRepositoryIsACapabilityError(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.Read = false
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"dst": {Capabilities: &caps}}})
	_, err := e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: out(t)})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Zero(t, e.repos["dst"].TotalCalls())
}

// moveTagOnFirstResolve repoints exampleRef at the decoy the first time it is resolved.
func moveTagOnFirstResolve(t *testing.T, e *env, decoyVersion string) {
	t.Helper()
	decoy, ok := e.repos["dst"].RecordDigestOf(ref("tools/example", decoyVersion))
	require.True(t, ok)
	moved := false
	e.repos["dst"].SetOnResolved(func(resolved artifact.Ref) {
		if resolved == exampleRef && !moved {
			moved = true
			assert.True(t, e.repos["dst"].MoveRef(exampleRef, decoy))
		}
	})
}

func publishDecoy(t *testing.T, e *env) {
	t.Helper()
	request := e.publishRequest("dst", e.source("DECOY CONTENT"))
	request.Version = "2.0.0"
	_, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
}

func TestPull_TagMovedMidReadStillConsumesTheOriginallyPinnedSubject(t *testing.T) {
	e := newEnv(t, envConfig{})
	original := e.publish("dst", e.source("ORIGINAL"))
	publishDecoy(t, e)
	moveTagOnFirstResolve(t, e, "2.0.0")
	resolves := e.repos["dst"].Calls("Resolve")

	dest := out(t)
	result, err := e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest})
	require.NoError(t, err)
	assert.Equal(t, 1, e.repos["dst"].Calls("Resolve")-resolves, "the mutable reference is resolved exactly once")
	assert.Equal(t, original.Operation.RecordDigest, result.RecordDigest)
	data, err := os.ReadFile(filepath.Join(dest, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(data), "the moved tag never substitutes other bytes")
	moved, _ := e.repos["dst"].RecordDigestOf(exampleRef)
	assert.NotEqual(t, original.Operation.RecordDigest, moved, "the tag really moved")
}

func TestInspectAndShowResolveOnce(t *testing.T) {
	e := newEnv(t, envConfig{})
	original := e.publish("dst", e.source("ORIGINAL"))
	publishDecoy(t, e)
	moveTagOnFirstResolve(t, e, "2.0.0")
	before := e.repos["dst"].Calls("Resolve")

	inspected, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err)
	assert.Equal(t, 1, e.repos["dst"].Calls("Resolve")-before)
	assert.Equal(t, original.Operation.RecordDigest, inspected.Operation.RecordDigest)
	assert.Equal(t, original.Operation.RecordDigest, inspected.Inspection.Pinned.RecordDigest)
	require.NotNil(t, inspected.Inspection.Record)
	assert.Equal(t, original.Record.ContentDigest, inspected.Inspection.Record.ContentDigest)
	assert.Nil(t, inspected.Verification, "no trust configuration reports unchecked, never verified")
}

func TestShowDocument(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.publish("dst", e.source("hello"))
	reads := e.repos["dst"].Calls("Read")

	result, err := e.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: artifact.DocumentReadme})
	require.NoError(t, err)
	assert.Equal(t, "package readme\n", string(result.Content))
	assert.Equal(t, artifact.DocumentReadme, result.Document.Role)
	assert.Equal(t, reads, e.repos["dst"].Calls("Read"), "documents are read without the payload")
	assert.Equal(t, int64(len(result.Content)), result.Operation.Size)

	_, err = e.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: artifact.DocumentChangelog})
	require.ErrorIs(t, err, errUtils.ErrArtifactDocumentMissing)

	_, err = e.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: "license"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestInspectAndShow_Failures(t *testing.T) {
	e := newEnv(t, envConfig{})
	_, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Ref: exampleRef})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = e.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Ref: exampleRef, Role: artifact.DocumentReadme})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = e.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: artifact.DocumentReadme})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func seedList(t *testing.T, e *env, alias string, items map[string]string, labels map[string]string) {
	t.Helper()
	for nameVersion, content := range items {
		name, version, _ := splitRef(nameVersion)
		request := artifact.PublishRequest{Repository: alias, Source: e.source(content), Name: name, Version: version}
		request.Metadata.Labels = labels
		_, err := e.svc.Publish(context.Background(), request)
		require.NoError(t, err)
	}
}

func splitRef(nameVersion string) (string, string, bool) {
	parsed, err := artifact.ParseRef(nameVersion)
	if err != nil {
		return "", "", false
	}
	return parsed.Name, parsed.Version, true
}

func TestList_AcrossRepositoriesWithPaginationLabelsAndOrdering(t *testing.T) {
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"alpha": {}, "beta": {}}})
	seedList(t, e, "alpha", map[string]string{"app/a:1": "a1", "app/a:2": "a2", "app/b:1": "b1"}, map[string]string{"team": "platform"})
	seedList(t, e, "beta", map[string]string{"app/a:1": "other", "app/c:1": "c1"}, map[string]string{"team": "data"})

	all, err := e.svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err)
	var keys []string
	for _, entry := range all.Artifacts {
		keys = append(keys, entry.Repository+"/"+entry.Name+":"+entry.Version)
	}
	assert.Equal(t, []string{"alpha/app/a:1", "beta/app/a:1", "alpha/app/a:2", "alpha/app/b:1", "beta/app/c:1"}, keys, "sorted by name, version, repository; aliases stay visible; every page consumed")
	require.Len(t, all.Repositories, 2)
	assert.Equal(t, 3, all.Repositories[0].Count)

	named, err := e.svc.List(context.Background(), artifact.ListRequest{Name: "app/a"})
	require.NoError(t, err)
	assert.Len(t, named.Artifacts, 3)

	one, err := e.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"beta"}})
	require.NoError(t, err)
	assert.Len(t, one.Artifacts, 2)

	platform, err := e.svc.List(context.Background(), artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "platform"}}})
	require.NoError(t, err)
	assert.Len(t, platform.Artifacts, 3)

	none, err := e.svc.List(context.Background(), artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "platform"}, {Key: "team", Value: "data"}}})
	require.NoError(t, err)
	assert.Empty(t, none.Artifacts, "label selectors use AND semantics")
}

func TestSearch_MatchesNamesDescriptionsKindsAndLabels(t *testing.T) {
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"alpha": {}}})
	request := e.publishRequest("alpha", e.source("a"))
	request.Name = "net/vpc-module"
	request.Metadata = artifact.Metadata{Description: "Networking Basics", Labels: map[string]string{"purpose": "backup"}}
	_, err := e.svc.Publish(context.Background(), request)
	require.NoError(t, err)
	other := e.publishRequest("alpha", e.source("b"))
	other.Name = "tools/other"
	_, err = e.svc.Publish(context.Background(), other)
	require.NoError(t, err)

	for query, want := range map[string]int{"vpc": 1, "networking": 1, "BACKUP": 1, "files": 2, "nomatch": 0, "": 2} {
		result, err := e.svc.Search(context.Background(), artifact.SearchRequest{Query: query})
		require.NoError(t, err, query)
		assert.Len(t, result.Artifacts, want, query)
	}
	labeled, err := e.svc.Search(context.Background(), artifact.SearchRequest{Query: "files", Labels: []artifact.Label{{Key: "purpose", Value: "backup"}}})
	require.NoError(t, err)
	assert.Len(t, labeled.Artifacts, 1)
}

func TestList_CapabilityErrorsAreReportedNotHidden(t *testing.T) {
	noCollection := artifacttest.DefaultCapabilities()
	noCollection.EnumerateCollection = false
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"alpha": {}, "registry": {Capabilities: &noCollection}}})
	seedList(t, e, "alpha", map[string]string{"app/a:1": "a1"}, nil)

	all, err := e.svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err, "a default listing tolerates a repository without collection enumeration")
	assert.Len(t, all.Artifacts, 1)
	var diagnostic artifact.RepositoryListing
	for _, listing := range all.Repositories {
		if listing.Repository == "registry" {
			diagnostic = listing
		}
	}
	require.NotEmpty(t, diagnostic.Diagnostics)
	assert.Contains(t, diagnostic.Diagnostics[0].Message, "direct named operations")

	_, err = e.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"registry"}})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "an explicitly requested repository reports the capability error")

	_, err = e.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"registry"}, Name: "app/a"})
	require.NoError(t, err, "named listing works without a catalog")

	_, err = e.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"ghost"}})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestList_OnlyFailingRepositoriesFailTheListing(t *testing.T) {
	noCollection := artifacttest.DefaultCapabilities()
	noCollection.EnumerateCollection = false
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"registry": {Capabilities: &noCollection}}})
	_, err := e.svc.List(context.Background(), artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "when no repository can answer, the listing fails")
}

func TestList_ConcurrencyIsInjectable(t *testing.T) {
	e := newEnv(t, envConfig{
		repos:   map[string]artifacttest.Options{"a": {}, "b": {}, "c": {}},
		options: []service.Option{service.WithConcurrency(1)},
	})
	for _, alias := range []string{"a", "b", "c"} {
		seedList(t, e, alias, map[string]string{"app/x:1": alias}, nil)
	}
	result, err := e.svc.List(context.Background(), artifact.ListRequest{})
	require.NoError(t, err)
	assert.Len(t, result.Artifacts, 3)
}
