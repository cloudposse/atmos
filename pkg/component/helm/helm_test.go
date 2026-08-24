package helm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: a rename of these schema fields should fail the build.
var (
	_ = schema.Helm{BasePath: "components/helm"}
	_ = schema.Components{Helm: schema.Helm{}}
)

func TestComponentProvider_Identity(t *testing.T) {
	p := &ComponentProvider{}
	assert.Equal(t, cfg.HelmComponentType, p.GetType())
	assert.Equal(t, "Kubernetes", p.GetGroup())
	assert.Equal(t, []string{"template", "diff", "plan", "apply", "deploy", "delete"}, p.GetAvailableCommands())
}

func TestComponentProvider_RegisteredInRegistry(t *testing.T) {
	provider, ok := component.GetProvider(cfg.HelmComponentType)
	require.True(t, ok, "helm provider must be registered via init()")
	assert.Equal(t, cfg.HelmComponentType, provider.GetType())
}

func TestComponentProvider_GetBasePath(t *testing.T) {
	p := &ComponentProvider{}
	assert.Equal(t, "components/helm", p.GetBasePath(nil))

	custom := &schema.AtmosConfiguration{}
	custom.Components.Helm.BasePath = "charts"
	assert.Equal(t, "charts", p.GetBasePath(custom))
}

func TestComponentProvider_ListComponents(t *testing.T) {
	stackConfig := map[string]any{
		"components": map[string]any{
			"helm": map[string]any{
				"redis": map[string]any{},
				"nginx": map[string]any{},
			},
		},
	}
	names, err := (&ComponentProvider{}).ListComponents(context.Background(), "dev", stackConfig)
	require.NoError(t, err)
	require.Len(t, names, 2)
	assert.Equal(t, []string{"nginx", "redis"}, names) // sorted

	empty, err := (&ComponentProvider{}).ListComponents(context.Background(), "dev", map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestComponentProvider_ValidateComponent(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr error
	}{
		{name: "nil config", config: nil, wantErr: nil},
		{name: "abstract skips chart check", config: map[string]any{"metadata": map[string]any{"type": "abstract"}}, wantErr: nil},
		{name: "missing chart", config: map[string]any{"namespace": "x"}, wantErr: errUtils.ErrHelmChartNotConfigured},
		{name: "empty chart", config: map[string]any{"chart": ""}, wantErr: errUtils.ErrHelmChartNotConfigured},
		{name: "valid chart", config: map[string]any{"chart": "bitnami/nginx"}, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&ComponentProvider{}).ValidateComponent(tt.config)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestExecute_UnsupportedSubcommand(t *testing.T) {
	err := (&ComponentProvider{}).Execute(&component.ExecutionContext{SubCommand: "frobnicate"})
	assert.ErrorIs(t, err, errUtils.ErrHelmUnsupportedSubcommand)
}

func TestExecute_DispatchesToOperation(t *testing.T) {
	original := executeOperation
	t.Cleanup(func() { executeOperation = original })

	var got Operation
	executeOperation = func(_ *component.ExecutionContext, op Operation) error {
		got = op
		return nil
	}

	cases := map[string]Operation{
		"template": OperationTemplate,
		"render":   OperationTemplate,
		"diff":     OperationDiff,
		"plan":     OperationDiff,
		"apply":    OperationApply,
		"deploy":   OperationApply,
		"delete":   OperationDelete,
		"destroy":  OperationDelete,
	}
	for sub, want := range cases {
		t.Run(sub, func(t *testing.T) {
			require.NoError(t, (&ComponentProvider{}).Execute(&component.ExecutionContext{SubCommand: sub}))
			assert.Equal(t, want, got)
		})
	}
}

func TestCutRepoRef(t *testing.T) {
	name, chart, ok := cutRepoRef("prometheus-community/kube-prometheus-stack")
	assert.True(t, ok)
	assert.Equal(t, "prometheus-community", name)
	assert.Equal(t, "kube-prometheus-stack", chart)

	_, _, ok = cutRepoRef("nochart")
	assert.False(t, ok)
}

func TestIsLocalOrOCI(t *testing.T) {
	assert.True(t, isLocalOrOCI("oci://ghcr.io/acme/chart"))
	assert.True(t, isLocalOrOCI("./charts/foo"))
	assert.True(t, isLocalOrOCI("/abs/charts/foo"))
	assert.False(t, isLocalOrOCI("bitnami/nginx"))
}

func TestRepositoriesMap(t *testing.T) {
	section := map[string]any{
		"repositories": []any{
			map[string]any{"name": "bitnami", "url": "https://charts.bitnami.com/bitnami"},
			map[string]any{"name": "incomplete"}, // no url -> skipped
			"not-a-map",                          // skipped
		},
	}
	got := repositoriesMap(section)
	assert.Equal(t, map[string]string{"bitnami": "https://charts.bitnami.com/bitnami"}, got)
}

func TestResolveReleaseNameAndNamespace(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "platform/redis"}
	assert.Equal(t, "redis", resolveReleaseName(map[string]any{}, info))
	assert.Equal(t, "custom", resolveReleaseName(map[string]any{"name": "custom"}, info))

	assert.Equal(t, defaultNamespace, resolveNamespace(map[string]any{}))
	assert.Equal(t, "prod", resolveNamespace(map[string]any{"namespace": "prod"}))
}

func TestBuildValues_MergeAndFiles(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	dir := t.TempDir()
	valuesFile := filepath.Join(dir, "extra.yaml")
	require.NoError(t, os.WriteFile(valuesFile, []byte("replicaCount: 1\nimage:\n  tag: base\n"), 0o600))

	section := map[string]any{
		"values_files": []any{valuesFile},
		"values": map[string]any{
			"replicaCount": 3,
			"image":        map[string]any{"tag": "override"},
		},
	}

	values, err := buildValues(atmosConfig, section, "")
	require.NoError(t, err)
	assert.Equal(t, 3, values["replicaCount"])
	img, ok := values["image"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "override", img["tag"]) // inline values win over values_files
}

func TestResolveRenderOptions_FlagsOverrideComponent(t *testing.T) {
	componentSection := map[string]any{
		"render": map[string]any{"output": map[string]any{"path": "from-component.yaml"}},
	}
	// Component default.
	opts := resolveRenderOptions(map[string]any{}, componentSection)
	assert.Equal(t, "from-component.yaml", opts.Output)
	assert.Equal(t, renderNoun, opts.Noun)

	// CLI flag overrides.
	opts = resolveRenderOptions(map[string]any{"output_dir": "out", "split": true}, componentSection)
	assert.Equal(t, "out", opts.OutputDir)
	assert.True(t, opts.Split)
	assert.Empty(t, opts.Output)
}

// TestRenderManifest_LocalChart exercises the Helm SDK render path against a local
// chart fixture with no cluster access (client-side dry run).
func TestRenderManifest_LocalChart(t *testing.T) {
	chartPath, err := filepath.Abs(filepath.Join("testdata", "chart"))
	require.NoError(t, err)

	rendered, err := renderManifest(context.Background(), &chartSpec{
		Chart:       chartPath,
		ReleaseName: "unit",
		Namespace:   "testns",
		Values:      map[string]any{"replicaCount": 5, "image": map[string]any{"tag": "9.9"}},
		IncludeCRDs: true,
	})
	require.NoError(t, err)
	assert.Contains(t, rendered, "kind: ConfigMap")
	assert.Contains(t, rendered, "name: unit")
	assert.Contains(t, rendered, "namespace: testns")
	assert.Contains(t, rendered, `replicas: "5"`)
	assert.Contains(t, rendered, "nginx:9.9")
	assert.True(t, strings.Contains(rendered, "ConfigMap"))
}
