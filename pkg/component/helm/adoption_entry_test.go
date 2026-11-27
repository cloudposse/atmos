package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

func TestHelmEntry(t *testing.T) {
	project := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(project, "unused-kubeconfig"))
	t.Setenv("HELM_NAMESPACE", "")
	base := filepath.Join(project, "charts")
	dir := filepath.Join(base, "web")
	writeAdoptionFile(t, filepath.Join(dir, "Chart.yaml"), "apiVersion: v2\nname: upstream\nversion: 1.0.0\n")
	writeAdoptionFile(t, filepath.Join(project, "config", "values", "base.yaml"), "tag: base\n")
	writeAdoptionFile(t, filepath.Join(project, "config", "values", "dev.yaml"), "tag: dev\n")
	t.Chdir(filepath.Join(project, "config"))
	dir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	r := stackadoption.Registration{Project: project, Dir: dir}
	entry, err := helmEntry(&r, dir, base)
	require.NoError(t, err)
	want := map[string]any{"chart": ".", "name": "web", "namespace": "default", "metadata": map[string]any{"component": "web"}}
	require.Equal(t, want, entry)

	t.Setenv("HELM_NAMESPACE", "team-preview")
	r.Name = "canary"
	entry, err = helmEntry(&r, dir, base)
	require.NoError(t, err)
	require.Equal(t, "canary", entry["name"])
	require.Equal(t, "team-preview", entry["namespace"])
	r.ReleaseName = "web-preview"
	r.Namespace = "preview"
	r.Values = []string{"values/base.yaml", "values/dev.yaml"}
	entry, err = helmEntry(&r, dir, base)
	require.NoError(t, err)
	want["name"], want["namespace"] = "web-preview", "preview"
	want["values_files"] = []string{"../../config/values/base.yaml", "../../config/values/dev.yaml"}
	require.Equal(t, want, entry)
}

func TestHelmEntryInvalidInputs(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, "web")
	chart := filepath.Join(dir, "Chart.yaml")
	const metadata = "apiVersion: v2\nname: web\nversion: 1.0.0\n"
	writeAdoptionFile(t, chart, metadata)
	dir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	r := stackadoption.Registration{Project: project, Dir: dir}
	check := func(want string) {
		t.Helper()
		_, err := helmEntry(&r, dir, project)
		require.ErrorContains(t, err, want)
	}
	r.Name = "Invalid"
	check("release")
	r.Name, r.Namespace = "web", "invalid.namespace"
	check("namespace")
	r.Namespace = "default"
	outside := t.TempDir()
	_, err = helmEntry(&r, dir, outside)
	require.ErrorContains(t, err, "outside component base")
	r.Values = []string{outside}
	check("outside component base")
	r.Values = []string{dir}
	check("regular")
	r.Values = []string{filepath.Join(project, "missing.yaml")}
	check("missing.yaml")
	r.Values = []string{filepath.Join(project, "malformed.yaml")}
	writeAdoptionFile(t, r.Values[0], "key: first\n---\nsecret: [")
	check("malformed.yaml")
	r.Values = []string{filepath.Join(project, "{{values}}.yaml")}
	writeAdoptionFile(t, r.Values[0], "field: example")
	check("template delimiter")
	r.Values = nil
	require.NoError(t, os.Remove(chart))
	check("Chart.yaml")
	writeAdoptionFile(t, chart, metadata+"type: library\n")
	check("library")
	writeAdoptionFile(t, chart, metadata+"dependencies:\n  - name: redis\n    version: 1.0.0\n    repository: file://../redis\n")
	check("helm dependency build")
	writeAdoptionFile(t, chart, metadata)
	writeAdoptionFile(t, filepath.Join(dir, "crds", "custom.yaml"), "kind: CustomResourceDefinition\n")
	check("custom.yaml")
}

func writeAdoptionFile(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}
