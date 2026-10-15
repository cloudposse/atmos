package exec

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

func setupCILabelsStack(t *testing.T, body string) (*schema.AtmosConfiguration, *atomic.Int32) {
	t.Helper()
	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "ci-labels"))
	t.Cleanup(ci.SwapRegistryForTest())
	ci.ResetLabelsCache()
	t.Cleanup(ci.ResetLabelsCache)
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)
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
	}{
		{"preview present", `[{"name":"preview"},{"name":"deploy"}]`, true},
		{"preview absent", `[{"name":"deploy"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, calls := setupCILabelsStack(t, tc.body)
			info, err := ProcessStacks(config, schema.ConfigAndStacksInfo{
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

			selectors, err := tags.ParseLabelsFlag("preview:true")
			require.NoError(t, err)
			current, err := ExecuteDescribeStacksScoped(config, "", nil, nil, nil, false, true, true, false, nil, nil, nil, selectors, DescribeStacksErrorOptions{})
			require.NoError(t, err)
			remote := map[string]any{}
			affected, err := findAffected(&current, &remote, config, nil, false, false, "", AffectedFilter{Labels: selectors}, "")
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
	}{
		{true, []string{"ci.labels"}},
		{false, nil},
	} {
		info, err := ProcessStacks(config, schema.ConfigAndStacksInfo{
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
		result, err := processCustomTagsWithContext(&schema.AtmosConfiguration{}, input, "", nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, input, result)
	}
}
