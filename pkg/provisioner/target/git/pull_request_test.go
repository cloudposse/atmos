package git

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cockroacherrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

const testPRBranch = "atmos/deployment-repo/dev/argocd"

// fakePublisher records Reconcile calls instead of talking to a forge API.
type fakePublisher struct {
	calls []atmosgit.PullRequestOptions
}

func (f *fakePublisher) Reconcile(_ context.Context, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	f.calls = append(f.calls, *options)
	return &atmosgit.PullRequestResult{Number: 7, URL: "https://example.invalid/pr/7", Created: len(f.calls) == 1}, nil
}

func (f *fakePublisher) PullRequestBodyBadge() string { return "BADGE" }

// installPRFakes swaps the forge-facing seams for doubles: the publisher registry, forge
// address detection (the test remote is a local bare repo, not an https forge URL), and
// default-branch discovery. Git itself (ls-remote, clone, checkout, commit, push) stays real.
func installPRFakes(t *testing.T) *fakePublisher {
	t.Helper()
	publisher := &fakePublisher{}
	prevPublisher, prevAddress, prevDefault := newPullRequestPublisher, pullRequestAddressFor, defaultBranch
	newPullRequestPublisher = func(string) (atmosgit.PullRequestPublisher, error) { return publisher, nil }
	pullRequestAddressFor = func(string) (string, pullRequestAddress, error) {
		return "github", pullRequestAddress{Owner: "acme", Repository: "deployments"}, nil
	}
	defaultBranch = func(context.Context, string, string) (string, error) { return "main", nil }
	t.Cleanup(func() { newPullRequestPublisher, pullRequestAddressFor, defaultBranch = prevPublisher, prevAddress, prevDefault })
	return publisher
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	isolatedGitEnv(t)
}

// prDeliverInput builds a git-target DeliverInput for a deployments repository at bare,
// with pull_request set to pr (nil omits the block entirely).
func prDeliverInput(bare, workdir, branch string, pr map[string]any, files map[string][]byte) *target.DeliverInput {
	targetConfig := map[string]any{
		"repository": "deployments",
		"path":       "clusters/dev/argocd",
		"commit":     map[string]any{"message": "Render argocd for dev", "signing": "never"},
	}
	if pr != nil {
		targetConfig["pull_request"] = pr
	}
	return &target.DeliverInput{
		AtmosConfig: &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
			"deployments": {URI: bare, Branch: branch, Workdir: workdir},
		}}},
		TargetName:   "deployment-repo",
		TargetConfig: targetConfig,
		Artifact: target.ProvisionArtifact{
			Kind:     target.ArtifactKindKubernetesManifests,
			Format:   target.FormatYAML,
			Files:    files,
			Metadata: target.ArtifactMetadata{Stack: "dev", Component: "argocd"},
		},
	}
}

func remoteRefCount(t *testing.T, bare, ref string) string {
	t.Helper()
	return strings.TrimSpace(gitCmd(t, bare, "rev-list", "--count", ref))
}

func manifest(content string) map[string][]byte {
	return map[string][]byte{"namespace.yaml": []byte(content)}
}

// TestDeliverPullRequestFromDefaultBranch covers the full lifecycle with no repository branch
// configured: the base comes from the remote default, the head is cut from it, re-deliveries
// stack onto the open head, and the base is never pushed.
func TestDeliverPullRequestFromDefaultBranch(t *testing.T) {
	requireGit(t)
	publisher := installPRFakes(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	mainBefore := remoteRefCount(t, bare, "main")
	pr := map[string]any{"enabled": true, "labels": []any{"gitops"}, "draft": true}
	g := &gitProvisioner{}

	in := prDeliverInput(bare, filepath.Join(root, "workdir"), "", pr, manifest("kind: Namespace\n"))
	require.NoError(t, g.Deliver(context.Background(), in))

	assert.Equal(t, mainBefore, remoteRefCount(t, bare, "main"), "the base branch must never be pushed")
	assert.Equal(t, "2", remoteRefCount(t, bare, testPRBranch))
	require.Len(t, publisher.calls, 1)
	got := publisher.calls[0]
	assert.Equal(t, "acme", got.Owner)
	assert.Equal(t, "deployments", got.Repository)
	assert.Empty(t, got.Namespace)
	assert.Equal(t, "main", got.Base)
	assert.Equal(t, testPRBranch, got.Head)
	assert.Equal(t, "Atmos: deliver argocd to deployment-repo (dev)", got.Title)
	assert.True(t, strings.HasPrefix(got.Body, "BADGE\n"), "the publisher's badge must lead the default body")
	assert.Contains(t, got.Body, "`clusters/dev/argocd`")
	assert.Equal(t, []string{"gitops"}, got.Labels)
	assert.True(t, got.Draft)

	// Identical re-delivery: no new commit, but the head already exists, so the PR is reconciled
	// again (this is what recovers a run whose PR API call failed after the push).
	require.NoError(t, g.Deliver(context.Background(), in))
	assert.Equal(t, "2", remoteRefCount(t, bare, testPRBranch))
	require.Len(t, publisher.calls, 2)

	// Changed delivery: stacks a commit on the open head branch (no force-push).
	in.Artifact.Files = manifest("kind: Namespace\nmetadata: {}\n")
	require.NoError(t, g.Deliver(context.Background(), in))
	assert.Equal(t, "3", remoteRefCount(t, bare, testPRBranch))
	assert.Equal(t, mainBefore, remoteRefCount(t, bare, "main"))
	require.Len(t, publisher.calls, 3)
	assert.Equal(t, testPRBranch, publisher.calls[2].Head)
}

// TestDeliverPullRequestThenDirectDeliveryTargetsBase verifies the persistent workdir, left on
// the head branch by a PR delivery, does not leak into a later direct delivery to the same
// repository: reconcile checks out the base explicitly, so the direct commit lands on main.
func TestDeliverPullRequestThenDirectDeliveryTargetsBase(t *testing.T) {
	requireGit(t)
	installPRFakes(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	workdir := filepath.Join(root, "workdir")
	g := &gitProvisioner{}

	require.NoError(t, g.Deliver(context.Background(), prDeliverInput(bare, workdir, "main", map[string]any{"enabled": true}, manifest("pr: true\n"))))
	headCount := remoteRefCount(t, bare, testPRBranch)
	mainCount := remoteRefCount(t, bare, "main")

	require.NoError(t, g.Deliver(context.Background(), prDeliverInput(bare, workdir, "main", nil, manifest("direct: true\n"))))

	assert.Equal(t, headCount, remoteRefCount(t, bare, testPRBranch), "a direct delivery must not touch the PR head")
	assert.NotEqual(t, mainCount, remoteRefCount(t, bare, "main"), "a direct delivery must commit to the base")
}

// TestDeliverPullRequestRecreatesHeadAfterMerge verifies that once the remote head is gone
// (merged and deleted), the stale local head is reset onto the current base, not reused.
func TestDeliverPullRequestRecreatesHeadAfterMerge(t *testing.T) {
	requireGit(t)
	installPRFakes(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	workdir := filepath.Join(root, "workdir")
	g := &gitProvisioner{}

	require.NoError(t, g.Deliver(context.Background(), prDeliverInput(bare, workdir, "main", map[string]any{"enabled": true}, manifest("v: 1\n"))))
	gitCmd(t, bare, "branch", "-D", testPRBranch)

	require.NoError(t, g.Deliver(context.Background(), prDeliverInput(bare, workdir, "main", map[string]any{"enabled": true}, manifest("v: 2\n"))))

	assert.Equal(t, "1", remoteRefCount(t, bare, "main"))
	assert.Equal(t, "2", remoteRefCount(t, bare, testPRBranch), "the new head must be one commit on top of base, not stacked on the stale head")
}

func TestDeliverPullRequestSkipsReconcileWhenNothingChanged(t *testing.T) {
	requireGit(t)
	publisher := installPRFakes(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	workdir := filepath.Join(root, "workdir")
	files := manifest("kind: Namespace\n")
	g := &gitProvisioner{}

	// Land the content directly on main, then deliver the same content via a PR.
	require.NoError(t, g.Deliver(context.Background(), prDeliverInput(bare, workdir, "main", nil, files)))
	require.NoError(t, g.Deliver(context.Background(), prDeliverInput(bare, workdir, "main", map[string]any{"enabled": true}, files)))

	assert.Empty(t, publisher.calls, "a head with nothing to deliver must not open a pull request")
	assert.Empty(t, strings.TrimSpace(gitCmd(t, bare, "branch", "--list", testPRBranch)), "an empty head must not be pushed")
}

func TestResolvePullRequestAddress(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com")
	tests := []struct {
		name     string
		uri      string
		provider string
		address  pullRequestAddress
	}{
		{"azure devops", "https://dev.azure.com/acme/platform/_git/deployments", "azuredevops", pullRequestAddress{Owner: "acme", Namespace: []string{"platform"}, Repository: "deployments"}},
		{"github.com", "https://github.com/acme/deployments.git", "github", pullRequestAddress{Owner: "acme", Repository: "deployments"}},
		{"github enterprise server", "https://ghe.example.com/acme/deployments.git", "github", pullRequestAddress{Owner: "acme", Repository: "deployments"}},
		{"azure devops ssh", "git@ssh.dev.azure.com:v3/acme/platform/deployments", "azuredevops", pullRequestAddress{Owner: "acme", Namespace: []string{"platform"}, Repository: "deployments"}},
		{"github ssh", "git@github.com:acme/deployments.git", "github", pullRequestAddress{Owner: "acme", Repository: "deployments"}},
		{"github enterprise server ssh", "git@ghe.example.com:acme/deployments.git", "github", pullRequestAddress{Owner: "acme", Repository: "deployments"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, address, err := resolvePullRequestAddress(tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.provider, provider)
			assert.Equal(t, tt.address, address)
		})
	}
}

func TestResolvePullRequestAddressRejectsUnsupportedURIs(t *testing.T) {
	for _, uri := range []string{
		"/tmp/origin.git",                                 // Local path.
		"https://gitlab.com/acme/deployments.git",         // Unsupported forge is not guessed as GitHub.
		"git@gitlab.com:acme/deployments.git",             // Unsupported forge over SSH.
		"https://dev.azure.com/acme/platform/deployments", // Azure host, but not a _git URL.
		"git@ssh.dev.azure.com:acme/platform/deployments", // Azure SSH host, but no v3 prefix.
		"https://github.com/acme/group/deployments.git",   // Not owner/repository.
	} {
		t.Run(uri, func(t *testing.T) {
			_, _, err := resolvePullRequestAddress(uri)
			require.ErrorIs(t, err, errUtils.ErrGitTargetPullRequestConfig)
		})
	}
}

// TestDeliverPullRequestRejectsUnsupportedForgeBeforeAnyGitOperation verifies the forge is
// validated before cloning: the workdir is never created.
func TestDeliverPullRequestRejectsUnsupportedForgeBeforeAnyGitOperation(t *testing.T) {
	workdir := filepath.Join(t.TempDir(), "workdir")
	err := (&gitProvisioner{}).Deliver(context.Background(),
		prDeliverInput("https://gitlab.com/acme/deployments.git", workdir, "main", map[string]any{"enabled": true}, manifest("v: 1\n")))
	require.ErrorIs(t, err, errUtils.ErrGitTargetPullRequestConfig)
	assert.NoDirExists(t, workdir)
}

func TestParseConfigPullRequest(t *testing.T) {
	cfg, err := parseConfig(map[string]any{"pull_request": map[string]any{
		"enabled": true, "branch": "deploy/argocd", "title": "T", "body": "B",
		"labels": []any{"a", "b"}, "draft": true, "reviewers": []any{"r"}, "assignees": []any{"x"},
	}})
	require.NoError(t, err)
	assert.Equal(t, schema.ProvisionTargetPullRequest{
		Enabled: true, Branch: "deploy/argocd", Title: "T", Body: "B",
		Labels: []string{"a", "b"}, Draft: true, Reviewers: []string{"r"}, Assignees: []string{"x"},
	}, cfg.PullRequest)

	_, err = parseConfig(map[string]any{"pull_request": "yes"})
	require.ErrorIs(t, err, errUtils.ErrGitTargetPullRequestConfig)
}

func TestDefaultPullRequestBranch(t *testing.T) {
	tests := []struct {
		name   string
		target string
		md     target.ArtifactMetadata
		want   string
	}{
		{"all segments", "repo", target.ArtifactMetadata{Stack: "dev", Component: "argocd"}, "atmos/repo/dev/argocd"},
		{"nested component and stack are flattened", "repo", target.ArtifactMetadata{Stack: "orgs/dev", Component: "eks/addons"}, "atmos/repo/orgs-dev/eks-addons"},
		{"empty segments skipped", "repo", target.ArtifactMetadata{Component: "argocd"}, "atmos/repo/argocd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, defaultPullRequestBranch(tt.target, &tt.md))
		})
	}
}

// errPublisher is a publisher whose Reconcile partially succeeds: it returns the pull request
// together with an error, as a provider does when a later label/reviewer step fails.
type errPublisher struct{}

func (errPublisher) Reconcile(context.Context, *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	return &atmosgit.PullRequestResult{Number: 9, URL: "https://example.invalid/pr/9"}, errPRMetadata
}

var errPRMetadata = errors.New("add labels failed")

func TestDeliverPullRequestSurfacesReconcileErrorAfterPush(t *testing.T) {
	requireGit(t)
	installPRFakes(t)
	newPullRequestPublisher = func(string) (atmosgit.PullRequestPublisher, error) { return errPublisher{}, nil }
	root := t.TempDir()
	bare := seedBareRepo(t, root)

	err := (&gitProvisioner{}).Deliver(context.Background(),
		prDeliverInput(bare, filepath.Join(root, "workdir"), "main", map[string]any{"enabled": true, "title": "T", "body": "B"}, manifest("v: 1\n")))

	require.ErrorIs(t, err, errPRMetadata)
	assert.Equal(t, "2", remoteRefCount(t, bare, testPRBranch), "the head is pushed before the PR API call")
}

func TestDeliverPullRequestFailsBeforeGitOperations(t *testing.T) {
	errFactory := errors.New("publisher unavailable")
	errDefault := errors.New("no default branch")
	tests := []struct {
		name    string
		branch  string
		setup   func()
		wantErr error
	}{
		{
			name:    "publisher factory error",
			branch:  "main",
			setup:   func() { newPullRequestPublisher = func(string) (atmosgit.PullRequestPublisher, error) { return nil, errFactory } },
			wantErr: errFactory,
		},
		{
			name:    "default branch error",
			branch:  "",
			setup:   func() { defaultBranch = func(context.Context, string, string) (string, error) { return "", errDefault } },
			wantErr: errDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installPRFakes(t)
			tt.setup()
			workdir := filepath.Join(t.TempDir(), "workdir")

			err := (&gitProvisioner{}).Deliver(context.Background(),
				prDeliverInput("https://github.com/acme/deployments.git", workdir, tt.branch, map[string]any{"enabled": true}, manifest("v: 1\n")))

			require.ErrorIs(t, err, tt.wantErr)
			assert.NoDirExists(t, workdir)
		})
	}
}

func TestDeliverPullRequestRemoteProbeError(t *testing.T) {
	requireGit(t)
	installPRFakes(t)
	missing := filepath.Join(t.TempDir(), "missing.git")
	workdir := filepath.Join(t.TempDir(), "workdir")

	err := (&gitProvisioner{}).Deliver(context.Background(),
		prDeliverInput(missing, workdir, "main", map[string]any{"enabled": true}, manifest("v: 1\n")))

	require.ErrorIs(t, err, errUtils.ErrGitCommandExited, "an unreachable remote is an error, not an absent branch")
	assert.NoDirExists(t, workdir)
}

func TestDeliverPullRequestReconcileError(t *testing.T) {
	requireGit(t)
	installPRFakes(t)
	t.Cleanup(setTestProvider(&stubCloneFailureProvider{}))
	root := t.TempDir()
	bare := seedBareRepo(t, root)

	err := (&gitProvisioner{}).Deliver(context.Background(),
		prDeliverInput(bare, filepath.Join(root, "workdir"), "main", map[string]any{"enabled": true}, manifest("v: 1\n")))

	require.Error(t, err)
	assert.Contains(t, strings.Join(cockroacherrors.GetAllHints(err), "\n"), "Confirm the configured branch exists")
}

func TestDeliverPullRequestInvalidHeadBranch(t *testing.T) {
	requireGit(t)
	publisher := installPRFakes(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)

	err := (&gitProvisioner{}).Deliver(context.Background(),
		prDeliverInput(bare, filepath.Join(root, "workdir"), "main", map[string]any{"enabled": true, "branch": "bad..name"}, manifest("v: 1\n")))

	require.ErrorIs(t, err, errUtils.ErrGitCommandExited)
	assert.Empty(t, publisher.calls)
}

func TestReportPullRequestNil(t *testing.T) {
	assert.NotPanics(t, func() { reportPullRequest(nil) })
	assert.NotPanics(t, func() { reportPullRequest(&atmosgit.PullRequestResult{Number: 1, URL: "u", Created: true}) })
}

func TestDeliverPullRequestWriteArtifactError(t *testing.T) {
	requireGit(t)
	publisher := installPRFakes(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	in := prDeliverInput(bare, filepath.Join(root, "workdir"), "main", map[string]any{"enabled": true}, manifest("v: 1\n"))
	in.TargetConfig["path"] = "."

	err := (&gitProvisioner{}).Deliver(context.Background(), in)

	require.ErrorIs(t, err, errUtils.ErrGitTargetPathInvalid)
	assert.Empty(t, publisher.calls)
	assert.Empty(t, strings.TrimSpace(gitCmd(t, bare, "branch", "--list", testPRBranch)), "nothing may be pushed")
}

// stubCommitFailureProvider clones fine but fails every commit.
type stubCommitFailureProvider struct{ stubCloneFailureProvider }

var errCommitStub = errors.New("commit failed")

func (s *stubCommitFailureProvider) Commit(context.Context, *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
	return nil, errCommitStub
}

func TestCommitAndPushReportsCommitError(t *testing.T) {
	s := &repoSession{provider: &stubCommitFailureProvider{}, resolved: &atmosgit.ResolvedRepository{}}

	committed, err := commitAndPush(context.Background(), s, &config{Path: "clusters/dev"}, &target.ProvisionArtifact{})

	require.ErrorIs(t, err, errCommitStub)
	assert.False(t, committed)
}
