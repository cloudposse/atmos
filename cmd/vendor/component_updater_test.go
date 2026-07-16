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

	"github.com/cloudposse/atmos/pkg/data"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
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
	calls int
}

type componentUpdaterPublisher struct {
	options *atmosgit.PullRequestOptions
}

func (p *componentUpdaterPublisher) Reconcile(_ context.Context, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	p.options = options
	return &atmosgit.PullRequestResult{Number: 42, URL: "https://github.com/acme/repo/pull/42", Created: true}, nil
}

func (l *componentUpdaterLister) ListTags(context.Context, string) ([]string, error) {
	l.calls++
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
	previous := version.DefaultLister
	version.DefaultLister = &componentUpdaterLister{tags: []string{"1.1.0"}}
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err := runVendorUpdate(v, "terraform", nil, false, nil, "platform", false)
	require.NoError(t, err)
	assert.Empty(t, report.Results)
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
	branch, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	assert.Equal(t, "updates/all", branch)
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
	branch, err := prepareComponentUpdateBranch(context.Background(), v, "all")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("after\n"), 0o644))

	pr, commit, err := publishComponentUpdate(context.Background(), v, "all", branch, &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Status: vendoring.StatusUpdated}}})
	require.NoError(t, err)
	require.NotEmpty(t, commit)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	require.NotNil(t, publisher.options)
	assert.Equal(t, "acme", publisher.options.Owner)
	assert.Equal(t, "repo", publisher.options.Repository)
	assert.Equal(t, branch, publisher.options.Head)
	assert.Equal(t, []string{"component-update"}, publisher.options.Labels)

	pr, commit, err = publishComponentUpdate(context.Background(), v, "all", branch, &vendoring.UpdateReport{})
	require.NoError(t, err)
	assert.Nil(t, pr)
	assert.Empty(t, commit)
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
