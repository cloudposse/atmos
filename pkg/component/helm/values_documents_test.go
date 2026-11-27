package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderSavedValuesDocuments(t *testing.T) {
	dir := t.TempDir()
	writeValues := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	base := writeValues("base.yaml", "image:\n  tag: '1.24'\n")
	env := writeValues("env.yaml", "replicaCount: 2\n---\nimage:\n  tag: '1.27'\n")
	cli := writeValues("cli.yaml", "replicaCount: 4\n---\nimage:\n  tag: cli\n")

	for _, tc := range []struct {
		name   string
		files  []string
		inline map[string]any
		flags  map[string]any
		tag    string
		count  string
	}{
		{"saved", []string{base, env}, nil, nil, "1.27", "2"},
		{"reversed", []string{env, base}, nil, nil, "1.24", "2"},
		{"inline", []string{base, env}, map[string]any{"image": map[string]any{"tag": "inline"}}, nil, "inline", "2"},
		{"cli", []string{base, env}, map[string]any{"replicaCount": 3}, map[string]any{flagValues: []string{cli}}, "cli", "4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{ComponentFromArg: "web", ComponentSection: map[string]any{
				cfg.ChartSectionName: ".", cfg.ValuesFilesSectionName: tc.files, cfg.ValuesSectionName: tc.inline,
			}}
			spec, err := buildChartSpec(config, info, filepath.Join("testdata", "chart"), nil)
			require.NoError(t, err)
			spec.Values, err = applyValueOverrides(spec.Values, tc.flags)
			require.NoError(t, err)
			rendered, err := renderManifest(t.Context(), spec)
			require.NoError(t, err)
			require.Contains(t, rendered, "nginx:"+tc.tag)
			require.Contains(t, rendered, `replicas: "`+tc.count+`"`)
		})
	}
	malformed := writeValues("malformed.yaml", "image: nginx\n---\nimage: [")
	_, err := loadValuesFile(malformed)
	require.ErrorContains(t, err, malformed)
}

func TestValidateAdoptionInputs(t *testing.T) {
	dir := t.TempDir()
	const metadata = "apiVersion: v2\nname: web\nversion: 1.0.0\n"
	write := func(name, data string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600))
	}
	require.Error(t, ValidateAdoptionInputs(dir, nil))
	write("Chart.yaml", metadata)
	write("overlay.yaml", "count: 2\n")
	require.NoError(t, ValidateAdoptionInputs(dir, []string{"overlay.yaml"}))
	write("overlay.yaml", "count: 2\n---\ncount: [")
	require.ErrorContains(t, ValidateAdoptionInputs(dir, []string{"overlay.yaml"}), "overlay.yaml")
	write("Chart.yaml", metadata+"type: library\n")
	require.ErrorContains(t, ValidateAdoptionInputs(dir, nil), "library")
	write("Chart.yaml", metadata)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "crds"), 0o700))
	write("crds/web.yaml", "kind: CustomResourceDefinition\n")
	require.ErrorContains(t, ValidateAdoptionInputs(dir, nil), "crds/web.yaml")
}
