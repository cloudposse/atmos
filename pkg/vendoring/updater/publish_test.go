package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
	githubprovider "github.com/cloudposse/atmos/pkg/git/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

type fakePullRequestPublisher struct {
	options *atmosgit.PullRequestOptions
	err     error
	// partialResult is returned alongside err when set, simulating a pull request that was
	// created but whose labels/assignees/reviewers step then failed.
	partialResult *atmosgit.PullRequestResult
}

func (p *fakePullRequestPublisher) Reconcile(_ context.Context, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	p.options = options
	if p.err != nil {
		return p.partialResult, p.err
	}
	return &atmosgit.PullRequestResult{Number: 42, URL: "https://github.com/acme/repo/pull/42", Created: true}, nil
}

func fakeGitHubRepository(_ context.Context, _, _ string) (string, string, error) {
	return "acme", "repo", nil
}

func TestPublishComponentUpdateCommitsPushesAndReconciles(t *testing.T) {
	_, workdir := newGitFixture(t)

	publisher := &fakePullRequestPublisher{}
	publisherName := t.Name()
	atmosgit.RegisterPullRequestPublisher(publisherName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	prConfig := schema.VendorPullRequestConfig{Provider: publisherName, Labels: []string{"component-update"}}
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Status: vendoring.StatusUpdated}}}}
	pr, commit, err := PublishComponentUpdate(context.Background(), workdir, "origin", publication, &prConfig, fakeGitHubRepository)
	require.NoError(t, err)
	require.NotEmpty(t, commit)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	require.NotNil(t, publisher.options)
	assert.Equal(t, "acme", publisher.options.Owner)
	assert.Equal(t, "repo", publisher.options.Repository)
	assert.Equal(t, branch, publisher.options.Head)
	assert.Equal(t, []string{"component-update"}, publisher.options.Labels)

	// An empty report means nothing to commit: the worktree is clean, so publish must be a no-op
	// rather than creating an empty commit or an empty-diff pull request.
	publication.Report = &vendoring.UpdateReport{}
	pr, commit, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &prConfig, fakeGitHubRepository)
	require.NoError(t, err)
	assert.Nil(t, pr)
	assert.Empty(t, commit)
}

// TestCommitAndPushComponentUpdateStatusError proves a git-status failure (here, workdir not even
// being a git repository) is returned as-is rather than panicking or being swallowed.
func TestCommitAndPushComponentUpdateStatusError(t *testing.T) {
	_, err := CommitAndPushComponentUpdate(context.Background(), t.TempDir(), "origin", "some-branch")
	require.Error(t, err)
}

func TestPublishComponentUpdatePushError(t *testing.T) {
	remote, workdir := newGitFixture(t)

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	// Break the remote after branch prep (which needs it) but before publish's push.
	require.NoError(t, os.RemoveAll(remote))

	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &schema.VendorPullRequestConfig{}, fakeGitHubRepository)
	assert.Error(t, err)
}

func TestPublishComponentUpdateGitHubRepositoryError(t *testing.T) {
	_, workdir := newGitFixture(t)

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	failingRepository := func(context.Context, string, string) (string, string, error) { return "", "", assert.AnError }
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &schema.VendorPullRequestConfig{}, failingRepository)
	assert.ErrorIs(t, err, assert.AnError)
}

// TestPublishComponentUpdateDefaultsProviderAndLabels proves the "github" provider and
// "component-update" label defaults both apply when prConfig.Provider/Labels are left unset. It
// temporarily swaps the real "github" pull-request publisher registration for a fake so no live
// API call is made, restoring it afterward.
func TestPublishComponentUpdateDefaultsProviderAndLabels(t *testing.T) {
	_, workdir := newGitFixture(t)

	t.Cleanup(func() {
		atmosgit.RegisterPullRequestPublisher(githubprovider.ProviderName, func() (atmosgit.PullRequestPublisher, error) { return githubprovider.New(), nil })
	})
	publisher := &fakePullRequestPublisher{}
	atmosgit.RegisterPullRequestPublisher(githubprovider.ProviderName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	// prConfig.Provider and .Labels are intentionally left unset.
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &schema.VendorPullRequestConfig{}, fakeGitHubRepository)
	require.NoError(t, err)
	require.NotNil(t, publisher.options)
	assert.Equal(t, []string{"component-update"}, publisher.options.Labels)
}

func TestPublishComponentUpdateUnknownProviderError(t *testing.T) {
	_, workdir := newGitFixture(t)

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	prConfig := schema.VendorPullRequestConfig{Provider: "nonexistent-provider"}
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &prConfig, fakeGitHubRepository)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-provider")
}

func TestPublishComponentUpdateReconcileError(t *testing.T) {
	_, workdir := newGitFixture(t)

	publisher := &fakePullRequestPublisher{err: assert.AnError}
	publisherName := t.Name()
	atmosgit.RegisterPullRequestPublisher(publisherName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	prConfig := schema.VendorPullRequestConfig{Provider: publisherName}
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &prConfig, fakeGitHubRepository)
	assert.ErrorIs(t, err, assert.AnError)
}

// TestPublishComponentUpdatePartialReconcileErrorStillReturnsPullRequest proves that when the
// underlying publisher creates a pull request but then fails a later step (a real example: GitHub
// rejects requesting a review from the PR's own author), PublishComponentUpdate still returns the
// pull request's number/URL alongside the error, rather than discarding it -- confirmed against a
// real end-to-end run against cloudposse/infra-live, where this previously left the user with an
// opaque "reconciliation failed" error and no way to find the pull request that was, in fact,
// created.
func TestPublishComponentUpdatePartialReconcileErrorStillReturnsPullRequest(t *testing.T) {
	_, workdir := newGitFixture(t)

	publisher := &fakePullRequestPublisher{
		err:           assert.AnError,
		partialResult: &atmosgit.PullRequestResult{Number: 99, URL: "https://github.com/acme/repo/pull/99", Created: true},
	}
	publisherName := t.Name()
	atmosgit.RegisterPullRequestPublisher(publisherName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	prConfig := schema.VendorPullRequestConfig{Provider: publisherName}
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	pr, commit, err := PublishComponentUpdate(context.Background(), workdir, "origin", publication, &prConfig, fakeGitHubRepository)

	assert.ErrorIs(t, err, assert.AnError)
	assert.NotEmpty(t, commit, "the commit already succeeded and must still be reported")
	require.NotNil(t, pr, "the already-created pull request must still be reported")
	assert.Equal(t, 99, pr.Number)
	assert.Equal(t, "https://github.com/acme/repo/pull/99", pr.URL)
}

func TestPublishComponentUpdateInvalidTemplateError(t *testing.T) {
	_, workdir := newGitFixture(t)

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	prConfig := schema.VendorPullRequestConfig{Title: "{{"}
	publication := Publication{Scope: "all", Branch: branch, Base: base, Report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = PublishComponentUpdate(context.Background(), workdir, "origin", publication, &prConfig, fakeGitHubRepository)
	require.Error(t, err)
}
