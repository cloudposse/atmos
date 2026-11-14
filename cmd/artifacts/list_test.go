package artifacts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

func listing() *artifact.ListResult {
	return &artifact.ListResult{
		Artifacts: []artifact.ListEntry{
			{
				Repository: "archive", Name: "team/app", Version: "1.0.0", Kind: "files", Description: "demo app", Labels: map[string]string{"team": "platform", "env": "dev"},
				Pinned: artifact.PinnedReference{ResolvedDigest: manifestDigest, RecordDigest: recordDigest, ContentDigest: contentDigest},
			},
			{Repository: "dir", Name: "team/app", Version: "1.1.0", Kind: "files", Pinned: artifact.PinnedReference{ResolvedDigest: recordDigest, RecordDigest: recordDigest}},
		},
		Repositories: []artifact.RepositoryListing{{Repository: "archive", Count: 1}, {Repository: "dir", Count: 1}},
	}
}

func TestListDefaultsToAllConfiguredRepositories(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().List(gomock.Any(), artifact.ListRequest{}).Return(listing(), nil)

	got := h.run("list")
	require.NoError(t, got.err)
	assert.Contains(t, got.stdout, "team/app")
	assert.Contains(t, got.stdout, "env=dev,team=platform", "labels are rendered sorted")
	assert.Contains(t, got.stdout, "archive")
	assert.Empty(t, got.stderr)
}

func TestListMapsNameRepositoriesLabelsAndRefresh(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().List(gomock.Any(), artifact.ListRequest{
		Repositories: []string{"dir", "archive"}, Name: "team/app", Refresh: true,
		Labels: []artifact.Label{{Key: "env", Value: "dev"}, {Key: "team", Value: "platform"}},
	}).Return(listing(), nil)
	svc.EXPECT().Search(gomock.Any(), artifact.SearchRequest{
		Repositories: []string{"dir"}, Query: "vpc", Labels: []artifact.Label{{Key: "purpose", Value: "backup"}},
	}).Return(listing(), nil)

	require.NoError(t, h.run("list", "team/app", "--repository=dir,archive", "--labels=team=platform", "--labels=env:dev", "--refresh").err)
	require.NoError(t, h.run("search", "vpc", "-r", "dir", "--labels=purpose=backup").err)
}

func TestListEnvelopeUsesTheArtifactsArrayWithNamedOutputs(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().List(gomock.Any(), gomock.Any()).Return(listing(), nil).Times(2)

	got := h.run("list", "--format=json")
	require.NoError(t, got.err)
	object := decodeObject(t, got.stdout)
	entries, ok := object["artifacts"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 2)
	first := entries[0].(map[string]any)
	assert.Equal(t, "archive", first["repository"])
	assert.Equal(t, "team/app:1.0.0", first["ref"])
	assert.Equal(t, string(manifestDigest), first["resolved_digest"])
	assert.Equal(t, string(recordDigest), first["record_digest"])
	assert.Equal(t, string(contentDigest), first["content_digest"])
	assert.Equal(t, entries[1].(map[string]any)["version"], "1.1.0", "the service's lexical order is preserved")

	var fromYAML map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(h.run("list", "--format=yaml").stdout), &fromYAML))
	assert.Equal(t, object["artifacts"], fromYAML["artifacts"], "display format never changes semantics")
}

func TestEmptyListingIsAnEmptyArrayAndAFriendlyMessage(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&artifact.ListResult{}, nil).Times(2)

	structured := h.run("search", "none", "--format=json")
	require.NoError(t, structured.err)
	assert.JSONEq(t, `{"artifacts": []}`, structured.stdout, "an empty result is [] and never null")

	text := h.run("search", "none")
	require.NoError(t, text.err)
	assert.Empty(t, text.stdout)
	assert.Contains(t, text.stderr, "No artifacts found.")
}

func TestListWithNoConfiguredRepositoriesSaysSo(t *testing.T) {
	h, svc := newHarness(t)
	h.config.Artifacts.Repositories = nil
	svc.EXPECT().List(gomock.Any(), gomock.Any()).Return(&artifact.ListResult{}, nil)

	got := h.run("list")
	require.NoError(t, got.err)
	assert.Contains(t, got.stderr, "No artifact repositories are configured")
}

func TestListFailureStillEmitsTheEnvelope(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrArtifactConfigInvalid)

	got := h.run("list", "-r", "ghost", "--format=json")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactConfigInvalid)
	object := decodeObject(t, got.stdout)
	assert.Contains(t, object, "error")
	assert.Equal(t, []any{}, object["artifacts"])
}

func TestListRejectsBadFormatsAndLabelsBeforeTheService(t *testing.T) {
	h, _ := newHarness(t)
	require.ErrorIs(t, h.run("list", "--format=text").err, errUtils.ErrInvalidFlag, "text is not a collection format")
	require.ErrorIs(t, h.run("list", "--labels=nope").err, errUtils.ErrInvalidFlag)
	require.ErrorIs(t, h.run("list", "a", "b").err, errUtils.ErrInvalidPositionalArgs)
}

// A dry run lists the files it selected. The real service returns the sealed Record next to the
// operation result; the command renders it as the plan.
type detailedService struct {
	artifact.Service
	outcome *service.PublishOutcome
}

func (d *detailedService) PublishDetailed(context.Context, artifact.PublishRequest) (*service.PublishOutcome, error) {
	return d.outcome, nil
}

func TestDryRunShowsTheSelectedFiles(t *testing.T) {
	h, _ := newHarness(t)
	record := artifact.Record{
		Name: "team/app", Version: "1.0.0", Kind: "files", Metadata: artifact.Metadata{Labels: map[string]string{"team": "platform"}},
		Inventory: []artifact.Entry{{Path: "bin/run.sh", Type: artifact.EntryFile}, {Path: "data.txt", Type: artifact.EntryFile}},
		Documents: []artifact.Document{{Role: artifact.DocumentReadme, SourcePath: ".atmos/artifact/README.md"}},
	}
	op := operation(artifact.ActionPublish, artifact.OutcomeDryRun)
	h.service = &detailedService{outcome: &service.PublishOutcome{Operation: *op, Record: record}}

	text := h.run("publish", "./dist", "-r", "dir", "--version=1.0.0", "--dry-run")
	require.NoError(t, text.err)
	assert.Contains(t, text.stderr, "Dry run: nothing was written")
	assert.Contains(t, text.stdout, "file[0]:")
	assert.Contains(t, text.stdout, "bin/run.sh (file)")
	assert.Contains(t, text.stdout, "readme:")
	assert.Contains(t, text.stdout, "team=platform")

	structured := h.run("publish", "./dist", "-r", "dir", "--version=1.0.0", "--dry-run", "--format=json")
	require.NoError(t, structured.err)
	assert.Equal(t, "dry-run", decodeObject(t, structured.stdout)["outcome"])
	assert.Contains(t, decodeObject(t, structured.stdout), "record")
}
