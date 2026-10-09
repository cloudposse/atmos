package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/git/providers/azuredevops"
	"github.com/cloudposse/atmos/pkg/git/providers/github"
	ghendpoints "github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// lsRemoteNoMatch is `git ls-remote --exit-code`'s exit status when no ref matched.
const lsRemoteNoMatch = 2

// Seams over the git and forge calls the pull-request flow makes, so tests can swap in
// doubles (same pattern as newProvider) without a forge API or a github.com remote.
var (
	defaultBranch           = atmosgit.DefaultBranch
	pullRequestAddressFor   = resolvePullRequestAddress
	newPullRequestPublisher = atmosgit.NewPullRequestPublisher
)

// gitRunner runs the few git commands reconcile doesn't cover (remote branch probe, head checkout).
var gitRunner = atmosgit.NewExecRunner()

// pullRequestBranches is the base/head pair for a delivery and whether head is already on the remote.
type pullRequestBranches struct {
	Base       string
	Head       string
	HeadExists bool
}

// deliverPullRequest publishes the artifact on a head branch and creates or updates the pull
// request from it into the base branch (the repository's branch, else the remote default, as
// vendor pull requests resolve it). It reuses reconcile unchanged by choosing which branch to
// reconcile: the head branch when it already exists on the remote (so a new delivery stacks on
// the open pull request), otherwise the base branch, from which the head is then cut. The base
// branch is never pushed and nothing is force-pushed.
func deliverPullRequest(ctx context.Context, s *repoSession, cfg *config, in *target.DeliverInput) error {
	// Resolve the forge first, so misconfiguration fails before any git operation instead of
	// leaving a pushed branch with no pull request.
	providerName, address, err := pullRequestAddressFor(s.resolved.URI)
	if err != nil {
		return err
	}
	publisher, err := newPullRequestPublisher(providerName)
	if err != nil {
		return err
	}

	branches, err := resolvePullRequestBranches(ctx, s, &cfg.PullRequest, in)
	if err != nil {
		return err
	}
	if err := checkoutHead(ctx, s, branches); err != nil {
		return err
	}

	if err := writeArtifact(s.rc.Workdir, cfg.Path, &in.Artifact, resolveSplit(cfg.Split, cfg.Path)); err != nil {
		return err
	}
	committed, err := commitAndPush(ctx, s, cfg, &in.Artifact)
	// An existing head branch is reconciled even without a new commit, so a rerun recovers a
	// delivery whose push succeeded but whose pull request API call failed.
	if err != nil || (!committed && !branches.HeadExists) {
		return err
	}

	pr := &cfg.PullRequest
	title, body := pullRequestText(pr, publisher, in, cfg.Path)
	result, err := publisher.Reconcile(ctx, &atmosgit.PullRequestOptions{
		Owner: address.Owner, Namespace: address.Namespace, Repository: address.Repository,
		Base: branches.Base, Head: branches.Head, Title: title, Body: body,
		Labels: pr.Labels, Draft: pr.Draft, Reviewers: pr.Reviewers, Assignees: pr.Assignees,
	})
	// The pull request may exist even when err is set (e.g. a later label/reviewer step
	// failed), so report it either way.
	reportPullRequest(result)
	return err
}

// reportPullRequest prints the reconciled pull request, if any.
func reportPullRequest(result *atmosgit.PullRequestResult) {
	if result == nil {
		return
	}
	verb := "Updated"
	if result.Created {
		verb = "Opened"
	}
	ui.Successf("%s pull request #%d: %s", verb, result.Number, result.URL)
}

// resolvePullRequestBranches resolves the base branch, the head branch name, and whether the
// head branch already exists on the remote.
func resolvePullRequestBranches(ctx context.Context, s *repoSession, pr *schema.ProvisionTargetPullRequest, in *target.DeliverInput) (pullRequestBranches, error) {
	b := pullRequestBranches{Base: s.resolved.Branch, Head: pr.Branch}
	if b.Base == "" {
		base, err := defaultBranch(ctx, "", s.resolved.URI)
		if err != nil {
			return pullRequestBranches{}, err
		}
		b.Base = base
	}
	if b.Head == "" {
		b.Head = defaultPullRequestBranch(in.TargetName, &in.Artifact.Metadata)
	}
	// A head equal to the base would leave the delivery on the base branch and push to it
	// directly, defeating pull-request mode.
	if b.Head == b.Base {
		return pullRequestBranches{}, fmt.Errorf("%w: pull_request.branch %q must differ from the base branch",
			errUtils.ErrGitTargetPullRequestConfig, b.Head)
	}
	exists, err := remoteBranchExists(ctx, s, b.Head)
	if err != nil {
		return pullRequestBranches{}, err
	}
	b.HeadExists = exists
	return b, nil
}

// checkoutHead reconciles the workdir onto the head branch when it exists on the remote,
// otherwise onto base and then cuts head from it. Because reconcile always checks out an
// explicit branch, the persistent workdir stays correct across pull-request and direct runs.
func checkoutHead(ctx context.Context, s *repoSession, b pullRequestBranches) error {
	s.rc.Branch = b.Base
	if b.HeadExists {
		s.rc.Branch = b.Head
	}
	if err := reconcile(ctx, s); err != nil {
		return err
	}
	if b.HeadExists {
		return nil
	}
	// -B also resets a stale local head left behind by an earlier, already-merged pull request.
	result, err := gitRunner.Run(ctx, "git", []string{"checkout", "-B", b.Head}, atmosgit.RunOptions{Dir: s.rc.Workdir, Env: s.rc.Env})
	if err != nil {
		return atmosgit.WrapOperationError("create pull request branch", s.rc.Workdir, result.StderrTail, err, "")
	}
	s.rc.Branch = b.Head
	return nil
}

// remoteBranchExists reports whether branch exists on the repository's remote.
func remoteBranchExists(ctx context.Context, s *repoSession, branch string) (bool, error) {
	result, err := gitRunner.Run(ctx, "git", []string{"ls-remote", "--exit-code", "--heads", s.resolved.URI, branch}, atmosgit.RunOptions{Env: s.rc.Env})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errUtils.ErrGitCommandExited) && result.ExitCode == lsRemoteNoMatch {
		return false, nil
	}
	return false, atmosgit.WrapOperationError("check remote branch", s.rc.Workdir, result.StderrTail, err, "")
}

// pullRequestAddress is the owner/namespace/repository triple a publisher addresses.
type pullRequestAddress struct {
	Owner      string
	Namespace  []string
	Repository string
}

// resolvePullRequestAddress detects the forge from the repository URI (HTTPS or SSH): an Azure
// DevOps URL yields its organization/project/repository, and a github.com (or GitHub Enterprise
// Server, via GITHUB_SERVER_URL) URL yields its owner/repository. Any other URI is rejected
// rather than guessed at.
func resolvePullRequestAddress(uri string) (string, pullRequestAddress, error) {
	if repo, ok := azuredevops.ParseRepositoryURL(uri); ok {
		return azuredevops.ProviderName, pullRequestAddress{Owner: repo.Organization, Namespace: []string{repo.Project}, Repository: repo.Name}, nil
	}
	parts, ok := atmosgit.ParseGenericGitURL(uri)
	if ok && isGitHubHost(parts.Host) && !strings.Contains(parts.Name, "/") {
		return github.ProviderName, pullRequestAddress{Owner: parts.Owner, Repository: parts.Name}, nil
	}
	return "", pullRequestAddress{}, fmt.Errorf("%w: pull_request supports GitHub (github.com/<owner>/<repository>) and Azure DevOps (dev.azure.com/<organization>/<project>/_git/<repository>) repositories over HTTPS or SSH, got %q",
		errUtils.ErrGitTargetPullRequestConfig, uri)
}

// isGitHubHost reports whether host (portless) is github.com or the configured GitHub
// Enterprise Server host. Ports are ignored, matching atmosgit.GitHubRepository, since
// scp-style SSH remotes carry none.
func isGitHubHost(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || host == strings.ToLower(ghendpoints.RepoEndpoints().Hostname())
}

// pullRequestText returns the configured title and body, falling back to defaults.
func pullRequestText(pr *schema.ProvisionTargetPullRequest, publisher atmosgit.PullRequestPublisher, in *target.DeliverInput, path string) (string, string) {
	title := pr.Title
	if title == "" {
		title = fmt.Sprintf("Atmos: deliver %s to %s (%s)", in.Artifact.Metadata.Component, in.TargetName, in.Artifact.Metadata.Stack)
	}
	body := pr.Body
	if body == "" {
		body = defaultPullRequestBody(publisher, in, path)
	}
	return title, body
}

// defaultPullRequestBranch builds atmos/<target>/<stack>/<component>. Slashes inside a
// segment are flattened to "-" so a nested component (e.g. "eks/addons") can't collide
// with a sibling's ref ("eks"), and empty segments are skipped.
func defaultPullRequestBranch(targetName string, md *target.ArtifactMetadata) string {
	parts := []string{"atmos"}
	for _, part := range []string{targetName, md.Stack, md.Component} {
		if part != "" {
			parts = append(parts, strings.ReplaceAll(part, "/", "-"))
		}
	}
	return strings.Join(parts, "/")
}

// defaultPullRequestBody summarizes the delivery, prefixed by the publisher's own badge when
// it supplies one (see atmosgit.PullRequestBodyBadger).
func defaultPullRequestBody(publisher atmosgit.PullRequestPublisher, in *target.DeliverInput, path string) string {
	var b strings.Builder
	if badger, ok := publisher.(atmosgit.PullRequestBodyBadger); ok {
		b.WriteString(badger.PullRequestBodyBadge())
		b.WriteString("\n")
	}
	b.WriteString("Rendered and delivered by Atmos.\n\n")
	fmt.Fprintf(&b, "- **Stack:** `%s`\n", in.Artifact.Metadata.Stack)
	fmt.Fprintf(&b, "- **Component:** `%s`\n", in.Artifact.Metadata.Component)
	fmt.Fprintf(&b, "- **Target:** `%s`\n", in.TargetName)
	fmt.Fprintf(&b, "- **Path:** `%s`\n", path)
	return b.String()
}
