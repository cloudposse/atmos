package updater

import (
	"context"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// GitHubRepositoryFunc resolves the owner/repository for workdir's remote -- matches
// atmosgit.GitHubRepository's signature. An explicit parameter (not a package-level var) so tests
// can substitute a fixture resolver -- this replaces today's package-level gitHubRepository
// test-seam var in cmd/vendor.
type GitHubRepositoryFunc func(ctx context.Context, workdir, remote string) (string, string, error)

// Publication bundles PublishComponentUpdate's inputs.
type Publication struct {
	Scope  string
	Branch string
	Base   string
	Report *vendoring.UpdateReport
}

// PublishComponentUpdate commits and pushes publication's workdir changes, then reconciles the
// pull request for the resulting commit. It returns a nil PullRequest and empty commit SHA (with
// no error) when the worktree had nothing to commit.
//
//nolint:revive // argument-limit: workdir/remote (repo location), publication (scope/branch/report), prConfig, and githubRepository are each independently meaningful; publication already bundles the fields specific to this call.
func PublishComponentUpdate(ctx context.Context, workdir, remote string, publication Publication, prConfig *schema.VendorPullRequestConfig, githubRepository GitHubRepositoryFunc) (*PullRequest, string, error) {
	defer perf.Track(nil, "updater.PublishComponentUpdate")()

	commit, err := CommitAndPushComponentUpdate(ctx, workdir, remote, publication.Branch)
	if err != nil || commit == "" {
		return nil, commit, err
	}
	pr, err := ReconcileComponentUpdatePullRequest(ctx, workdir, remote, publication, prConfig, githubRepository)
	if err != nil {
		return nil, "", err
	}
	return pr, commit, nil
}

// CommitAndPushComponentUpdate commits every changed path in workdir on branch and pushes it to
// remote, returning the empty string (with no error) when the worktree was already clean or
// nothing was actually committed.
func CommitAndPushComponentUpdate(ctx context.Context, workdir, remote, branch string) (string, error) {
	defer perf.Track(nil, "updater.CommitAndPushComponentUpdate")()

	provider, err := atmosgit.NewProvider("cli")
	if err != nil {
		return "", err
	}
	rc := atmosgit.RepoContext{Workdir: workdir, Remote: remote, Branch: branch}
	status, err := provider.Status(ctx, &atmosgit.StatusOptions{RepoContext: rc})
	if err != nil {
		return "", err
	}
	if status.Clean {
		return "", nil
	}
	paths := make([]string, 0, len(status.Entries))
	for _, entry := range status.Entries {
		paths = append(paths, entry.Path)
	}
	commit, err := provider.Commit(ctx, &atmosgit.CommitOptions{RepoContext: rc, Paths: paths, Message: "chore(components): update vendored components", Author: &atmosgit.Author{Name: "atmos[bot]", Email: "atmos-bot@users.noreply.github.com"}})
	if err != nil {
		return "", err
	}
	if !commit.Committed {
		return "", nil
	}
	if err := provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rc, Retries: 1}); err != nil {
		return "", err
	}
	return commit.SHA, nil
}

// ReconcileComponentUpdatePullRequest creates or updates the pull request for publication's branch
// against publication's base, using prConfig for the provider/labels/reviewers/etc. and
// githubRepository to resolve workdir's remote owner/repository.
//
//nolint:revive // argument-limit: workdir/remote (repo location), publication (scope/branch/report), prConfig, and githubRepository are each independently meaningful; publication already bundles the fields specific to this call.
func ReconcileComponentUpdatePullRequest(ctx context.Context, workdir, remote string, publication Publication, prConfig *schema.VendorPullRequestConfig, githubRepository GitHubRepositoryFunc) (*PullRequest, error) {
	defer perf.Track(nil, "updater.ReconcileComponentUpdatePullRequest")()

	owner, repository, err := githubRepository(ctx, workdir, remote)
	if err != nil {
		return nil, err
	}
	title, body, err := RenderPRTemplates(PRTemplates{Title: prConfig.Title, Body: prConfig.Body}, publication.Scope, publication.Report)
	if err != nil {
		return nil, err
	}
	publisherName := prConfig.Provider
	if publisherName == "" {
		publisherName = "github"
	}
	publisher, err := atmosgit.NewPullRequestPublisher(publisherName)
	if err != nil {
		return nil, err
	}
	labels := prConfig.Labels
	if len(labels) == 0 {
		labels = []string{"component-update"}
	}
	pr, err := publisher.Reconcile(ctx, &atmosgit.PullRequestOptions{Owner: owner, Repository: repository, Base: publication.Base, Head: publication.Branch, Title: title, Body: body, Labels: labels, Draft: prConfig.Draft, Reviewers: prConfig.Reviewers, Assignees: prConfig.Assignees})
	if err != nil {
		return nil, err
	}
	return &PullRequest{Number: pr.Number, URL: pr.URL}, nil
}
