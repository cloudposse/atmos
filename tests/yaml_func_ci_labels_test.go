package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

func setupCILabelsStack(t *testing.T, body string) (*schema.AtmosConfiguration, *atomic.Int32) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join("fixtures", "scenarios", "ci-labels"))
	require.NoError(t, err)
	t.Chdir(createCILabelsRepo(t, fixture))
	t.Cleanup(ci.SwapRegistryForTest())
	ci.ResetLabelsCache()
	t.Cleanup(ci.ResetLabelsCache)
	e.ClearFindStacksMapCache()
	t.Cleanup(e.ClearFindStacksMapCache)
	calls := &atomic.Int32{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/repos/owner/repo/issues/42/labels", r.URL.Path)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(s.Close)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_REF", "refs/pull/42/merge")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "labels-test-token")
	t.Setenv("ATMOS_CI_GITHUB_API_URL", s.URL)
	ci.Register(github.NewProvider())
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return &config, calls
}

func TestCILabelsStackInheritanceAndAffected(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
		queue      bool
	}{
		{"preview present", `[{"name":"preview"},{"name":"deploy"}]`, true, false},
		{"preview absent", `[{"name":"deploy"}]`, false, false},
		{"queue preview present", `[{"name":"preview"},{"name":"deploy"}]`, true, true},
		{"queue preview absent", `[{"name":"deploy"}]`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, calls := setupCILabelsStack(t, tc.body)
			if tc.queue {
				t.Setenv("GITHUB_EVENT_NAME", "merge_group")
				t.Setenv("GITHUB_REF", "refs/heads/gh-readonly-queue/main/pr-42-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			}
			info, err := e.ProcessStacks(config, schema.ConfigAndStacksInfo{
				ComponentFromArg: "preview", Stack: "test", ComponentType: cfg.TerraformComponentType,
			}, true, true, true, nil, nil)
			require.NoError(t, err)
			metadata := info.ComponentSection["metadata"].(map[string]any)
			assert.Equal(t, tc.want, metadata["labels"].(map[string]any)["preview"])
			assert.Equal(t, tc.want, metadata["labels"].(map[string]any)["ci_preview"])
			assert.Equal(t, tc.want, info.ComponentSection["settings"].(map[string]any)["pro"].(map[string]any)["enabled"])
			wantLabels := []any{"deploy"}
			if tc.want {
				wantLabels = append(wantLabels, "preview")
			}
			assert.Equal(t, wantLabels, info.ComponentSection["vars"].(map[string]any)["ci_labels"])

			selectors, err := tags.ParseLabelsFlag([]string{"preview:true"})
			require.NoError(t, err)
			affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPathWithOptions(
				config, createCILabelsRepo(t, ""), false, false, "", true, true, nil,
				e.AffectedFilter{Labels: selectors}, nil, false, e.DescribeStacksErrorOptions{},
			)
			require.NoError(t, err)
			if tc.want {
				require.Len(t, affected, 1)
				assert.Equal(t, "preview", affected[0].Component)
			} else {
				assert.Empty(t, affected)
			}
			assert.EqualValues(t, 1, calls.Load(), "all components and both evaluation paths must share one snapshot")
		})
	}
}

func TestCILabelsStackSkip(t *testing.T) {
	config, calls := setupCILabelsStack(t, `[{"name":"preview"}]`)
	for _, tc := range []struct {
		process bool
		skip    []string
		queue   bool
	}{
		{true, []string{"ci.labels"}, false},
		{false, nil, false},
		{true, []string{"ci.labels"}, true},
		{false, nil, true},
	} {
		if tc.queue {
			t.Setenv("GITHUB_EVENT_NAME", "merge_group")
			t.Setenv("GITHUB_REF", "refs/heads/gh-readonly-queue/main/pr-42-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		}
		info, err := e.ProcessStacks(config, schema.ConfigAndStacksInfo{
			ComponentFromArg: "preview", Stack: "test", ComponentType: cfg.TerraformComponentType,
		}, true, true, tc.process, tc.skip, nil)
		require.NoError(t, err)
		assert.Equal(t, `!ci.labels ["preview"]`, info.ComponentSection["settings"].(map[string]any)["pro"].(map[string]any)["enabled"])
	}
	assert.Zero(t, calls.Load())
}

func TestCILabelsDispatchBoundaries(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"!ci.labels.other", "!ci.labels-extra", "!ci.labels[preview]"} {
		result, err := e.ProcessCustomYamlTags(&schema.AtmosConfiguration{}, map[string]any{"value": input}, "", nil, nil)
		require.NoError(t, err)
		assert.Equal(t, input, result["value"])
	}
}

// createCILabelsRepo provides a self-contained comparison repository without network access.
func createCILabelsRepo(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	if fixture != "" {
		require.NoError(t, cp.Copy(fixture, dir))
	} else {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Empty base\n"), 0o600))
	}
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{"https://github.com/owner/repo.git"}})
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(".")
	require.NoError(t, err)
	_, err = worktree.Commit("fixture", &gogit.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.com", When: time.Unix(1_000_000, 0),
	}})
	require.NoError(t, err)
	return dir
}
