package updater

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// azureDevOpsProviderName matches azuredevops.ProviderName. Compared by string, not by importing
// the provider package, so this package (and the production binary paths that don't otherwise
// need pull-request publishing) doesn't gain a hard dependency on every registered provider --
// mirrors how the "github" default just below is also a literal, not githubprovider.ProviderName.
const azureDevOpsProviderName = "azuredevops"

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
		// commit already succeeded (checked above) and pr may be partially populated -- return
		// both alongside the error so the caller can still report what actually happened.
		return pr, commit, err
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

	address, err := resolveRepositoryAddress(ctx, workdir, remote, prConfig, githubRepository)
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
	pr, err := publisher.Reconcile(ctx, &atmosgit.PullRequestOptions{
		Owner: address.Owner, Namespace: address.Namespace, Repository: address.Repository, Base: publication.Base, Head: publication.Branch,
		Title: title, Body: body, Labels: labels, Draft: prConfig.Draft, Reviewers: prConfig.Reviewers, Assignees: prConfig.Assignees,
	})
	if pr == nil {
		return nil, err
	}
	// The pull request itself may already exist even when err is set (e.g. it was created but a
	// later label/assignee/reviewer step failed) -- surface it either way so the caller isn't left
	// with no way to find a pull request that was, in fact, created.
	return &PullRequest{Number: pr.Number, URL: pr.URL}, err
}

// repositoryAddress is the owner/namespace/repository triple ReconcileComponentUpdatePullRequest
// hands the selected publisher (bundled into one return value: CLAUDE.md's revive
// function-result-limit caps functions at 3 return values).
type repositoryAddress struct {
	Owner      string
	Namespace  []string
	Repository string
}

// resolveRepositoryAddress resolves the repositoryAddress a publisher.Reconcile call needs.
// GitHub derives owner/repository from workdir's Git remote (it doesn't need explicit config);
// Azure DevOps has no equivalent remote-parsing convention (see pkg/git.GitHubRepository's own
// github.com-specific parsing), so it addresses its organization/project/repository directly from
// prConfig instead.
func resolveRepositoryAddress(ctx context.Context, workdir, remote string, prConfig *schema.VendorPullRequestConfig, githubRepository GitHubRepositoryFunc) (repositoryAddress, error) {
	defer perf.Track(nil, "updater.resolveRepositoryAddress")()

	if prConfig.Provider == azureDevOpsProviderName {
		if prConfig.Organization == "" || prConfig.Project == "" || prConfig.Repository == "" {
			return repositoryAddress{}, fmt.Errorf("%w: the azuredevops provider requires vendor.ci.pull_request.organization, .project, and .repository to be set",
				errUtils.ErrComponentUpdaterConfig)
		}
		return repositoryAddress{Owner: prConfig.Organization, Namespace: []string{prConfig.Project}, Repository: prConfig.Repository}, nil
	}
	owner, repository, err := githubRepository(ctx, workdir, remote)
	return repositoryAddress{Owner: owner, Repository: repository}, err
}
