package vendor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	githubprovider "github.com/cloudposse/atmos/pkg/git/providers/github"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/updater"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

type componentUpdaterFailingContext struct {
	iolib.Context
	err error
}

func (c componentUpdaterFailingContext) Write(iolib.Stream, string) error {
	return c.err
}

type componentUpdaterLister struct {
	tags  []string
	err   error
	calls int
}

type componentUpdaterPublisher struct {
	options *atmosgit.PullRequestOptions
	err     error
}

func (p *componentUpdaterPublisher) Reconcile(_ context.Context, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	p.options = options
	if p.err != nil {
		return nil, p.err
	}
	return &atmosgit.PullRequestResult{Number: 42, URL: "https://github.com/acme/repo/pull/42", Created: true}, nil
}

func (l *componentUpdaterLister) ListTags(context.Context, string) ([]string, error) {
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return l.tags, nil
}

func TestComponentUpdaterScopeIsStable(t *testing.T) {
	assert.Equal(t, "all", updateScope("", nil))
	assert.Equal(t, "group-platform", updateScope("platform", nil))
	assert.Equal(t, updateScope("", []string{"vpc", "eks"}), updateScope("", []string{"eks", "vpc"}))
}

func TestComponentUpdaterGroupFiltering(t *testing.T) {
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "terraform/vpc", Status: vendoring.StatusUpdated},
		{Component: "terraform/eks/blue", Status: vendoring.StatusUpdated},
		{Component: "terraform/eks/legacy", Status: vendoring.StatusUpdated},
		{Component: "terraform/rds", Status: vendoring.StatusUpdated},
	}}
	assert.Equal(t, []string{"terraform/eks/blue", "terraform/vpc"}, filterGroupComponents(report, []string{"terraform/vpc", "terraform/eks/*"}, []string{"terraform/eks/legacy"}))
	filtered := filterReport(report, []string{"terraform/vpc"})
	require.Len(t, filtered.Results, 1)
	assert.Equal(t, "terraform/vpc", filtered.Results[0].Component)
	assert.Equal(t, []string{"terraform/vpc", "terraform/eks/blue", "terraform/eks/legacy", "terraform/rds"}, updatedComponents(report))
}

func TestComponentUpdaterTemplates(t *testing.T) {
	v := viper.New()
	v.Set("vendor.ci.pull_request.title", "update {{ .scope.name }}")
	v.Set("vendor.ci.pull_request.body", "{{ .updates | markdownTable }}")
	title, body, err := renderPRTemplates(v, "all", &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1", LatestVersion: "2"}}})
	require.NoError(t, err)
	assert.Equal(t, "update all", title)
	assert.Contains(t, body, "| vpc | 1 | 2 |")
}

func TestNormalizeComponentSelectors(t *testing.T) {
	assert.Nil(t, normalizeComponentSelectors(nil))
	assert.Equal(t, []string{"vpc", "eks"}, normalizeComponentSelectors([]string{" vpc ", "[]", "", "eks"}))
}

func TestRunVendorUpdateDoesNotWidenEmptyGroupSelection(t *testing.T) {
	report, err := runVendorUpdate(viper.New(), "terraform", nil, false, []string{}, "platform", false)
	require.NoError(t, err)
	assert.Empty(t, report.Results)
}

func TestRunVendorUpdateUpdatesSelectedAndGroupedComponents(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))

	lister := &componentUpdaterLister{tags: []string{"1.0.0", "1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	report, err := runVendorUpdate(v, "terraform", nil, false, []string{"vpc"}, "", true)
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, vendoring.StatusUpdated, report.Results[0].Status)

	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err = runVendorUpdate(v, "terraform", nil, false, nil, "platform", false)
	require.NoError(t, err)
	assert.Equal(t, 1, report.UpdatedCount())
	assert.Equal(t, 3, lister.calls, "group discovery occurs once and selected mutation runs once")
}

// TestRunVendorUpdateGroupDiscoveryError proves a discovery-phase failure inside the
// group-scoped branch (no manifest resolvable at all) is returned as-is rather than being
// swallowed or panicking.
func TestRunVendorUpdateGroupDiscoveryError(t *testing.T) {
	chdirTest(t, t.TempDir()) // no vendor.yaml or component.yaml anywhere.

	v := viper.New()
	v.Set("file", filepath.Join(t.TempDir(), "missing-vendor.yaml"))
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})

	_, err := runVendorUpdate(v, "terraform", nil, false, nil, "platform", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read vendor manifest")
}

// TestRunVendorUpdateComponentResolvedError proves a per-component UpdateResolved failure (here,
// the remote tag lister erroring) during the explicit --component selector path is returned as-is.
func TestRunVendorUpdateComponentResolvedError(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))

	lister := &componentUpdaterLister{err: errors.New("remote unavailable")}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	_, err := runVendorUpdate(v, "terraform", nil, false, []string{"vpc"}, "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote unavailable")
}

func TestRunVendorUpdateGroupCheckIsDryRun(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))

	lister := &componentUpdaterLister{tags: []string{"1.0.0", "1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err := runVendorUpdate(v, "terraform", nil, false, nil, "platform", true)
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "vpc", report.Results[0].Component)
	assert.Equal(t, vendoring.StatusUpdated, report.Results[0].Status)
	assert.Equal(t, 1, lister.calls, "a --group --check dry run must stop after discovery and never mutate")
}

func TestRunVendorUpdateReturnsNoOpForGroupWithoutUpdates(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.1.0
      targets: [components/terraform/vpc]
`), 0o644))
	lister := &componentUpdaterLister{tags: []string{"1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err := runVendorUpdate(v, "terraform", nil, false, nil, "platform", false)
	require.NoError(t, err)
	assert.Empty(t, report.Results)
	assert.Equal(t, 1, lister.calls, "an empty group must stop after discovery")
}

func TestPrepareComponentUpdateBranch(t *testing.T) {
	root := t.TempDir()
	remote, workdir := filepath.Join(root, "remote.git"), filepath.Join(root, "workdir")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, output)
	}
	run(root, "init", "--bare", remote)
	run(remote, "symbolic-ref", "HEAD", "refs/heads/main")
	run(root, "clone", remote, workdir)
	run(workdir, "config", "user.name", "Atmos Test")
	run(workdir, "config", "user.email", "atmos-test@example.com")
	run(workdir, "config", "commit.gpgSign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "README.md"), []byte("base\n"), 0o644))
	run(workdir, "add", "README.md")
	run(workdir, "commit", "-m", "base")
	run(workdir, "branch", "-M", "main")
	run(workdir, "push", "-u", "origin", "main")

	previous := currentWorkdir
	currentWorkdir = workdir
	t.Cleanup(func() { currentWorkdir = previous })
	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	v.Set("vendor.ci.pull_request.branch_prefix", "updates")
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	assert.Equal(t, "updates/all", branch)
	assert.Equal(t, "main", base)
}

func TestPublishComponentUpdateCommitsPushesAndReconciles(t *testing.T) {
	root := t.TempDir()
	remote, workdir := filepath.Join(root, "remote.git"), filepath.Join(root, "workdir")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, output)
	}
	run(root, "init", "--bare", remote)
	run(remote, "symbolic-ref", "HEAD", "refs/heads/main")
	run(root, "clone", remote, workdir)
	run(workdir, "config", "user.name", "Atmos Test")
	run(workdir, "config", "user.email", "atmos-test@example.com")
	run(workdir, "config", "commit.gpgSign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("before\n"), 0o644))
	run(workdir, "add", "vendor.yaml")
	run(workdir, "commit", "-m", "base")
	run(workdir, "branch", "-M", "main")
	run(workdir, "push", "-u", "origin", "main")

	previous := currentWorkdir
	currentWorkdir = workdir
	previousRepository := gitHubRepository
	gitHubRepository = func(context.Context, string, string) (string, string, error) { return "acme", "repo", nil }
	t.Cleanup(func() {
		currentWorkdir = previous
		gitHubRepository = previousRepository
	})

	publisher := &componentUpdaterPublisher{}
	publisherName := t.Name()
	atmosgit.RegisterPullRequestPublisher(publisherName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })
	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	v.Set("vendor.ci.pull_request.provider", publisherName)
	v.Set("vendor.ci.pull_request.labels", []string{"component-update"})
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Status: vendoring.StatusUpdated}}}}
	pr, commit, err := publishComponentUpdate(context.Background(), v, publication)
	require.NoError(t, err)
	require.NotEmpty(t, commit)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	require.NotNil(t, publisher.options)
	assert.Equal(t, "acme", publisher.options.Owner)
	assert.Equal(t, "repo", publisher.options.Repository)
	assert.Equal(t, branch, publisher.options.Head)
	assert.Equal(t, []string{"component-update"}, publisher.options.Labels)

	publication.report = &vendoring.UpdateReport{}
	pr, commit, err = publishComponentUpdate(context.Background(), v, publication)
	require.NoError(t, err)
	assert.Nil(t, pr)
	assert.Empty(t, commit)
}

// newComponentUpdaterGitFixture stands up a bare remote plus a cloned workdir
// with an initial "vendor.yaml" commit already pushed to "main", the same
// shape TestPrepareComponentUpdateBranch and
// TestPublishComponentUpdateCommitsPushesAndReconciles each set up inline.
func newComponentUpdaterGitFixture(t *testing.T) (remote, workdir string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	workdir = filepath.Join(root, "workdir")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, output)
	}
	run(root, "init", "--bare", remote)
	run(remote, "symbolic-ref", "HEAD", "refs/heads/main")
	run(root, "clone", remote, workdir)
	run(workdir, "config", "user.name", "Atmos Test")
	run(workdir, "config", "user.email", "atmos-test@example.com")
	run(workdir, "config", "commit.gpgSign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("before\n"), 0o644))
	run(workdir, "add", "vendor.yaml")
	run(workdir, "commit", "-m", "base")
	run(workdir, "branch", "-M", "main")
	run(workdir, "push", "-u", "origin", "main")
	return remote, workdir
}

func TestPrepareComponentUpdateBranchResolvesDefaultBase(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)

	previous := currentWorkdir
	currentWorkdir = workdir
	t.Cleanup(func() { currentWorkdir = previous })

	// vendor.ci.pull_request.base_branch is intentionally left unset so
	// prepareComponentUpdateBranch must resolve it via atmosgit.DefaultBranch.
	branch, base, err := prepareComponentUpdateBranch(context.Background(), viper.New(), "all")
	require.NoError(t, err)
	assert.Equal(t, "main", base)
	assert.Equal(t, "atmos/component-updater/all", branch)
}

func TestPrepareComponentUpdateBranchDefaultBranchError(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)
	cmd := exec.Command("git", "remote", "remove", "origin")
	cmd.Dir = workdir
	require.NoError(t, cmd.Run())

	previous := currentWorkdir
	currentWorkdir = workdir
	t.Cleanup(func() { currentWorkdir = previous })

	_, _, err := prepareComponentUpdateBranch(context.Background(), viper.New(), "all")
	assert.Error(t, err)
}

func TestPrepareComponentUpdateBranchPrepareBranchError(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "dirty.txt"), []byte("dirty\n"), 0o644))

	previous := currentWorkdir
	currentWorkdir = workdir
	t.Cleanup(func() { currentWorkdir = previous })

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	_, _, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterDirtyWorktree)
}

// TestCommitAndPushComponentUpdateStatusError proves a git-status failure (here, currentWorkdir
// not even being a git repository) is returned as-is rather than panicking or being swallowed.
func TestCommitAndPushComponentUpdateStatusError(t *testing.T) {
	previous := currentWorkdir
	currentWorkdir = t.TempDir() // not a git repository at all.
	t.Cleanup(func() { currentWorkdir = previous })

	_, err := commitAndPushComponentUpdate(context.Background(), "some-branch")
	require.Error(t, err)
}

func TestPublishComponentUpdatePushError(t *testing.T) {
	remote, workdir := newComponentUpdaterGitFixture(t)

	previous := currentWorkdir
	currentWorkdir = workdir
	t.Cleanup(func() { currentWorkdir = previous })

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	// Break the remote after branch prep (which needs it) but before publish's push.
	require.NoError(t, os.RemoveAll(remote))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = publishComponentUpdate(context.Background(), v, publication)
	assert.Error(t, err)
}

func TestPublishComponentUpdateGitHubRepositoryError(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)

	previousWorkdir := currentWorkdir
	currentWorkdir = workdir
	previousRepository := gitHubRepository
	gitHubRepository = func(context.Context, string, string) (string, string, error) { return "", "", assert.AnError }
	t.Cleanup(func() {
		currentWorkdir = previousWorkdir
		gitHubRepository = previousRepository
	})

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = publishComponentUpdate(context.Background(), v, publication)
	assert.ErrorIs(t, err, assert.AnError)
}

// TestPublishComponentUpdateDefaultsProviderAndLabels proves the "github"
// provider and "component-update" label defaults both apply when
// vendor.ci.pull_request.provider/labels are left unset. It temporarily
// swaps the real "github" pull-request publisher registration for a fake so
// no live API call is made, restoring it afterward.
func TestPublishComponentUpdateDefaultsProviderAndLabels(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)

	previousWorkdir := currentWorkdir
	currentWorkdir = workdir
	previousRepository := gitHubRepository
	gitHubRepository = func(context.Context, string, string) (string, string, error) { return "acme", "repo", nil }
	t.Cleanup(func() {
		currentWorkdir = previousWorkdir
		gitHubRepository = previousRepository
		atmosgit.RegisterPullRequestPublisher(githubprovider.ProviderName, func() (atmosgit.PullRequestPublisher, error) { return githubprovider.New(), nil })
	})

	publisher := &componentUpdaterPublisher{}
	atmosgit.RegisterPullRequestPublisher(githubprovider.ProviderName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	// vendor.ci.pull_request.provider and .labels are intentionally left unset.
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = publishComponentUpdate(context.Background(), v, publication)
	require.NoError(t, err)
	require.NotNil(t, publisher.options)
	assert.Equal(t, []string{"component-update"}, publisher.options.Labels)
}

func TestPublishComponentUpdateUnknownProviderError(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)

	previousWorkdir := currentWorkdir
	currentWorkdir = workdir
	previousRepository := gitHubRepository
	gitHubRepository = func(context.Context, string, string) (string, string, error) { return "acme", "repo", nil }
	t.Cleanup(func() {
		currentWorkdir = previousWorkdir
		gitHubRepository = previousRepository
	})

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	v.Set("vendor.ci.pull_request.provider", "nonexistent-provider")
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = publishComponentUpdate(context.Background(), v, publication)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-provider")
}

func TestPublishComponentUpdateReconcileError(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)

	previousWorkdir := currentWorkdir
	currentWorkdir = workdir
	previousRepository := gitHubRepository
	gitHubRepository = func(context.Context, string, string) (string, string, error) { return "acme", "repo", nil }
	t.Cleanup(func() {
		currentWorkdir = previousWorkdir
		gitHubRepository = previousRepository
	})

	publisher := &componentUpdaterPublisher{err: assert.AnError}
	publisherName := t.Name()
	atmosgit.RegisterPullRequestPublisher(publisherName, func() (atmosgit.PullRequestPublisher, error) { return publisher, nil })

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	v.Set("vendor.ci.pull_request.provider", publisherName)
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = publishComponentUpdate(context.Background(), v, publication)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestPublishComponentUpdateInvalidTemplateError(t *testing.T) {
	_, workdir := newComponentUpdaterGitFixture(t)

	previousWorkdir := currentWorkdir
	currentWorkdir = workdir
	previousRepository := gitHubRepository
	gitHubRepository = func(context.Context, string, string) (string, string, error) { return "acme", "repo", nil }
	t.Cleanup(func() {
		currentWorkdir = previousWorkdir
		gitHubRepository = previousRepository
	})

	v := viper.New()
	v.Set("vendor.ci.pull_request.base_branch", "main")
	v.Set("vendor.ci.pull_request.title", "{{")
	branch, base, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	publication := componentUpdatePublication{scope: "all", branch: branch, base: base, report: &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}}}
	_, _, err = publishComponentUpdate(context.Background(), v, publication)
	require.Error(t, err)
}

func TestValidateUpdateInvocation(t *testing.T) {
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("check", false, "")
		return cmd
	}

	tests := []struct {
		name       string
		configure  func(*viper.Viper, *cobra.Command)
		invocation updateInvocation
		wantError  string
	}{
		{name: "conflicting all selector", invocation: updateInvocation{All: true, Group: "platform"}, wantError: "--all cannot"},
		{name: "conflicting group and component selectors", invocation: updateInvocation{Group: "platform", Components: []string{"vpc"}}, wantError: "--group and --component"},
		{name: "missing group", invocation: updateInvocation{Group: "platform"}, wantError: "is not configured"},
		{name: "invalid format", configure: func(v *viper.Viper, _ *cobra.Command) { v.Set("format", "yaml") }, wantError: "--format"},
		{name: "invalid execution mode", configure: func(v *viper.Viper, _ *cobra.Command) {
			v.Set("format", "table")
			v.Set("vendor.update.execution.mode", "invalid")
		}, wantError: "execution.mode"},
		{name: "component batching requires worktree", configure: func(v *viper.Viper, _ *cobra.Command) {
			v.Set("format", "table")
			v.Set("vendor.update.batching.mode", "component")
		}, wantError: "requires"},
		{name: "invalid batching mode", configure: func(v *viper.Viper, _ *cobra.Command) {
			v.Set("format", "table")
			v.Set("vendor.update.batching.mode", "invalid")
		}, wantError: "batching.mode"},
		{name: "component batching not available in this release", configure: func(v *viper.Viper, _ *cobra.Command) {
			v.Set("format", "table")
			v.Set("vendor.update.execution.mode", "worktree")
			v.Set("vendor.update.batching.mode", "component")
		}, wantError: "not available in this release"},
		{name: "invalid template", configure: func(v *viper.Viper, _ *cobra.Command) {
			v.Set("format", "table")
			v.Set("vendor.ci.pull_request.title", "{{")
		}, invocation: updateInvocation{PullRequest: true}, wantError: "invalid pull request template"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, cmd := viper.New(), newCommand()
			v.Set("format", "table")
			if tt.configure != nil {
				tt.configure(v, cmd)
			}
			err := validateUpdateInvocation(v, cmd, tt.invocation)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestValidateUpdateInvocationPullRequestCheckIsDryRun(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("check", false, "")
	require.NoError(t, cmd.Flags().Set("check", "true"))

	v := viper.New()
	v.Set("format", "table")
	v.Set("check", true)
	// An invalid template must NOT surface here: a dry-run PR request never
	// renders (or validates) PR templates, since it will never publish.
	v.Set("vendor.ci.pull_request.title", "{{")

	assert.NoError(t, validateUpdateInvocation(v, cmd, updateInvocation{PullRequest: true}))
}

func TestApplyComponentUpdaterReport(t *testing.T) {
	result := updater.Result{Status: "updated"}
	applyComponentUpdaterReport(&result, &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpToDate}}})
	assert.Equal(t, "no_updates", result.Status)
	assert.Zero(t, result.Updated)

	applyComponentUpdaterReport(&result, &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}})
	assert.Equal(t, "updated", result.Status)
	assert.Equal(t, 1, result.Updated)
}

func TestVendorUpdateCommandReportsNoUpdatesAsJSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	stdout := captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
	assert.Contains(t, stdout.String(), `"status": "no_updates"`)
}

func TestVendorUpdatePullRequestNoUpdatesDoesNotPublish(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	stdout := captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull-request", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
	assert.Contains(t, stdout.String(), `"status": "no_updates"`)
	assert.NotContains(t, stdout.String(), `"branch":`)
}

// TestVendorUpdatePullRequestDiscoveryError proves a discovery-phase failure (the initial dry
// run runVendorUpdate call --pull-request makes before ever touching git) is returned as-is,
// without attempting to prepare a branch or publish anything.
func TestVendorUpdatePullRequestDiscoveryError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir()) // no vendor.yaml or component.yaml anywhere.

	_ = captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", filepath.Join(t.TempDir(), "missing-vendor.yaml")))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull-request", "true"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read vendor manifest")
}

// NOTE on the --pull-request "found updates" success path (vendorUpdateCmd's RunE, update.go
// lines ~112-121 branch preparation and ~149-157 publish): --pull-request unconditionally forces
// v.Set("pull", true), so exercising that branch through vendorUpdateCmd.RunE end-to-end also
// drives the auto-pull's real ExecuteVendorPullCmd materialization step for the "updated"
// component. Doing that without a real network-hosted Git source would require standing up a
// second, separately-tagged local bare Git repo to serve as the vendored *component* source (on
// top of the "origin" repo already used for the PR branch/commit/push flow here), purely to
// satisfy this one branch - a disproportionate amount of integration scaffolding for coverage
// that's already exercised at the unit level: prepareComponentUpdateBranch,
// commitAndPushComponentUpdate, and publishComponentUpdate (the actual logic these RunE lines
// call into) are each already covered directly above and in TestPrepareComponentUpdateBranch* /
// TestPublishComponentUpdate* (100% coverage per `go tool cover -func`). Skipped here rather than
// forcing a flaky or disproportionately heavy test; see TestVendorUpdatePullRequestDiscoveryError
// and TestVendorUpdatePullRequestNoUpdatesDoesNotPublish above for the branches of this same
// RunE block that are covered.

// TestVendorUpdateCommand_JSONRenderErrorPropagates proves a data-writer failure while rendering
// the final JSON summary surfaces as a command error instead of being silently swallowed.
func TestVendorUpdateCommand_JSONRenderErrorPropagates(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(componentUpdaterFailingContext{Context: ioCtx, err: errors.New("broken pipe")})
	t.Cleanup(data.Reset)

	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err = vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken pipe")
}

// TestVendorUpdatePullRequestNoUpdatesJSONRenderErrorPropagates proves a data-writer failure
// while rendering the discovery-phase "no updates" JSON summary (the --pull-request early-return
// branch, distinct from the final renderComponentUpdaterJSON call covered by
// TestVendorUpdateCommand_JSONRenderErrorPropagates above) also surfaces as a command error
// instead of being silently swallowed.
func TestVendorUpdatePullRequestNoUpdatesJSONRenderErrorPropagates(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(componentUpdaterFailingContext{Context: ioCtx, err: errors.New("broken pipe")})
	t.Cleanup(data.Reset)

	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull-request", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err = vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken pipe")
}

func TestVendorUpdateCommandSurfacesValidationError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	_ = captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("all", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("group", "platform"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot")
}

func TestVendorUpdateSummaryRespectsEnabledFlag(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		wantWritten bool
	}{
		{name: "enabled by default", enabled: true, wantWritten: true},
		{name: "disabled", enabled: false, wantWritten: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			resetCommandFlags(t, vendorUpdateCmd)
			file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
			_ = captureVendorStdout(t)
			require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
			require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
			require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

			summaryPath := filepath.Join(t.TempDir(), "summary.md")
			t.Setenv("GITHUB_ACTIONS", "true")
			t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
			viper.GetViper().Set("vendor.ci.summary.enabled", tt.enabled)

			require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))

			content, readErr := os.ReadFile(summaryPath)
			if tt.wantWritten {
				require.NoError(t, readErr)
				assert.NotEmpty(t, content)
				return
			}
			if readErr == nil {
				assert.Empty(t, content, "summary must not be written when vendor.ci.summary.enabled=false")
			}
		})
	}
}

func TestRenderComponentUpdaterJSON(t *testing.T) {
	t.Run("writes JSON", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		ioCtx, err := iolib.NewContext(iolib.WithStreams(&testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}))
		require.NoError(t, err)
		data.InitWriter(ioCtx)
		t.Cleanup(data.Reset)

		require.NoError(t, renderComponentUpdaterJSON(&updater.Result{Status: "no_updates"}, "json"))
		assert.Contains(t, stdout.String(), `"status": "no_updates"`)
		assert.NoError(t, renderComponentUpdaterJSON(&updater.Result{}, "table"))
	})

	t.Run("propagates write error", func(t *testing.T) {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(componentUpdaterFailingContext{Context: ioCtx, err: errors.New("broken pipe")})
		t.Cleanup(data.Reset)

		assert.EqualError(t, renderComponentUpdaterJSON(&updater.Result{}, "json"), "broken pipe")
	})
}
