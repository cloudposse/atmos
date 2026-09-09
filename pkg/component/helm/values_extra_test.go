package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/action"

	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveChartRef must map a "repo/name" reference to an explicit RepoURL plus a
// bare chart name when a matching declarative repository is configured, and pass
// an explicit RepoURL through unchanged.
func TestResolveChartRef_RepoResolution(t *testing.T) {
	actx := memoryActionContext(t)

	tests := []struct {
		name        string
		spec        *chartSpec
		wantRef     string
		wantRepoURL string
	}{
		{
			name:        "repo/name with matching repository resolves to bare name and RepoURL",
			spec:        &chartSpec{Chart: "bitnami/nginx", Repositories: []chartRepository{{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"}}},
			wantRef:     "nginx",
			wantRepoURL: "https://charts.bitnami.com/bitnami",
		},
		{
			name:        "repo/name with no matching repository passes through",
			spec:        &chartSpec{Chart: "unknown/nginx"},
			wantRef:     "unknown/nginx",
			wantRepoURL: "",
		},
		{
			name:        "explicit RepoURL passes bare chart name through",
			spec:        &chartSpec{Chart: "nginx", RepoURL: "https://example.com/charts"},
			wantRef:     "nginx",
			wantRepoURL: "https://example.com/charts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := action.NewInstall(actx.cfg)
			assert.Equal(t, tt.wantRef, resolveChartRef(client, tt.spec))
			assert.Equal(t, tt.wantRepoURL, client.RepoURL)
		})
	}
}

// mergeRepositories layers component repositories over global ones: same-name
// component entries override the global entry (not duplicated), component-only
// entries are appended, and incomplete entries (missing name or url) are skipped.
func TestMergeRepositories_GlobalOverrideAndSkipEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Helm.Repositories = []schema.HelmRepository{
		{Name: "shared", URL: "https://global.example/shared"},
		{Name: "", URL: ""}, // incomplete global entry -> skipped
	}
	section := map[string]any{
		"repositories": []any{
			map[string]any{"name": "shared", "url": "https://component.example/shared"}, // overrides global by name
			map[string]any{"name": "extra", "url": "https://component.example/extra"},   // new
		},
	}

	repos := mergeRepositories(atmosConfig, section)

	require.Len(t, repos, 2, "the incomplete global entry is skipped and 'shared' is overridden, not duplicated")

	shared, found := findRepository(repos, "shared")
	require.True(t, found)
	assert.Equal(t, "https://component.example/shared", shared.URL)
	assert.Equal(t, repositorySourceComponent, shared.Source)

	extra, found := findRepository(repos, "extra")
	require.True(t, found)
	assert.Equal(t, "https://component.example/extra", extra.URL)
}

// mergeRepositories tolerates a nil global config, returning only the component
// repositories.
func TestMergeRepositories_NilConfigReturnsComponentOnly(t *testing.T) {
	section := map[string]any{
		"repositories": []any{
			map[string]any{"name": "only", "url": "https://component.example/only"},
		},
	}
	repos := mergeRepositories(nil, section)
	require.Len(t, repos, 1)
	assert.Equal(t, "only", repos[0].Name)
	assert.Nil(t, globalRepositories(nil), "globalRepositories must tolerate a nil config")
}

// buildChartSpec must propagate a values-file load error rather than returning a
// partially built spec.
func TestBuildChartSpec_ValuesFileErrorPropagates(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "apps/demo",
		ComponentSection: map[string]any{
			"chart":        "./chart",
			"values_files": []any{filepath.Join(t.TempDir(), "missing.yaml")},
		},
	}
	_, err := buildChartSpec(&schema.AtmosConfiguration{}, info, "testdata")
	require.Error(t, err)
}

func TestLoadValuesFile(t *testing.T) {
	tests := []struct {
		name    string
		write   bool
		content string
		wantErr bool
	}{
		{name: "missing file errors", write: false, wantErr: true},
		{name: "malformed YAML errors", write: true, content: "\tnot: [valid", wantErr: true},
		{name: "empty file yields empty non-nil map", write: true, content: "", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "values.yaml")
			if tt.write {
				require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))
			}
			out, err := loadValuesFile(path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, out)
			assert.Empty(t, out)
		})
	}
}

func TestApplyValueOverridesUsesHelmPrecedence(t *testing.T) {
	dir := t.TempDir()
	valuesFile := filepath.Join(dir, "override.yaml")
	setFile := filepath.Join(dir, "payload.txt")
	require.NoError(t, os.WriteFile(valuesFile, []byte("priority: from-values-file\nnested:\n  file: true\n"), 0o600))
	require.NoError(t, os.WriteFile(setFile, []byte("file payload"), 0o600))

	merged, err := applyValueOverrides(map[string]any{
		"priority": "from-component",
		"nested":   map[string]any{"component": true},
	}, map[string]any{
		flagValues:     []string{valuesFile},
		flagSetJSON:    []string{`json={"enabled":true}`},
		flagSet:        []string{"priority=from-set", "typed=42"},
		flagSetString:  []string{"priority=from-string", "asString=42"},
		flagSetFile:    []string{"payload=" + setFile},
		flagSetLiteral: []string{"priority=from-literal", "literal=a,b"},
	})
	require.NoError(t, err)

	assert.Equal(t, "from-literal", merged["priority"])
	assert.Equal(t, int64(42), merged["typed"])
	assert.Equal(t, "42", merged["asString"])
	assert.Equal(t, "file payload", merged["payload"])
	assert.Equal(t, "a,b", merged["literal"])
	assert.Equal(t, map[string]any{"enabled": true}, merged["json"])
	assert.Equal(t, map[string]any{"component": true, "file": true}, merged["nested"])
}

func TestApplyValueOverridesReportsInvalidInputs(t *testing.T) {
	_, err := applyValueOverrides(map[string]any{}, map[string]any{flagSet: []string{"not-an-assignment"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--set")

	_, err = applyValueOverrides(map[string]any{}, map[string]any{flagValues: []string{filepath.Join(t.TempDir(), "missing.yaml")}})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "missing.yaml"))
}
