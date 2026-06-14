package kubernetes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestLoaderLoadsInlineAndPathManifests(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: app
spec:
  ports:
    - port: 80
`), 0o644))

	loader := manifestLoader{
		componentPath: dir,
		provider:      ProviderKubectl,
	}

	objects, err := loader.Load(map[string]any{
		"manifests": []any{map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "settings",
			},
		}},
		"paths": []any{"service.yaml"},
	})

	require.NoError(t, err)
	require.Len(t, objects, 2)
	require.Equal(t, "ConfigMap", objects[0].GetKind())
	require.Equal(t, "settings", objects[0].GetName())
	require.Equal(t, "Service", objects[1].GetKind())
	require.Equal(t, "app", objects[1].GetName())
}

func TestManifestLoaderRendersKustomizeDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte(`
resources:
  - deployment.yaml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec:
  selector:
    matchLabels:
      app: app
  template:
    metadata:
      labels:
        app: app
    spec:
      containers:
        - name: app
          image: nginx
`), 0o644))

	loader := manifestLoader{
		componentPath: dir,
		provider:      ProviderKustomize,
	}

	objects, err := loader.Load(map[string]any{
		"paths": []any{"."},
	})

	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.Equal(t, "Deployment", objects[0].GetKind())
	require.Equal(t, "app", objects[0].GetName())
}

func TestManifestLoaderUsesComponentPathWhenNoInputsConfigured(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "namespace.yaml"), []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: demo
`), 0o644))

	objects, err := (manifestLoader{componentPath: dir, provider: ProviderKubectl}).Load(map[string]any{})

	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.Equal(t, "Namespace", objects[0].GetKind())
}

func TestManifestFilesInDirFiltersAndSortsManifestFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "ignored.yaml"), []byte("ignored"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0o644))

	files, err := manifestFilesInDir(dir)

	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(dir, "a.yaml"),
		filepath.Join(dir, "b.yml"),
		filepath.Join(dir, "c.json"),
	}, files)
}

func TestManifestLoaderErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "missing-kind.yaml"), []byte(`
apiVersion: v1
metadata:
  name: broken
`), 0o644))

	_, err := (manifestLoader{componentPath: dir}).Load(map[string]any{"manifests": []any{42}})
	require.ErrorContains(t, err, "manifest entries must be YAML strings or maps")

	_, err = (manifestLoader{componentPath: dir}).Load(map[string]any{"paths": []any{"does-not-exist.yaml"}})
	require.ErrorContains(t, err, "failed to stat Kubernetes path")

	_, err = loadManifestFile(filepath.Join(dir, "missing-kind.yaml"))
	require.ErrorContains(t, err, "manifest is missing apiVersion or kind")

	_, err = decodeObjects([]byte("apiVersion: v1\nkind: ["))
	require.ErrorContains(t, err, "failed to decode Kubernetes manifest")
}

func TestDecodeObjectsHandlesEmptyDocumentsAndLists(t *testing.T) {
	objects, err := decodeObjects([]byte(`
---
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: settings
---
`))

	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.Equal(t, "ConfigMap", objects[0].GetKind())
	require.Equal(t, "settings", objects[0].GetName())
}

func TestManifestPathHelpers(t *testing.T) {
	assert.True(t, isManifestFile("app.yaml"))
	assert.True(t, isManifestFile("app.yml"))
	assert.True(t, isManifestFile("app.json"))
	assert.False(t, isManifestFile("app.txt"))

	dir := t.TempDir()
	assert.False(t, hasKustomizationFile(dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Kustomization"), []byte("resources: []"), 0o644))
	assert.True(t, hasKustomizationFile(dir))

	assert.Equal(t, filepath.Clean(filepath.Join(dir, "relative.yaml")), resolvePath(dir, "relative.yaml"))
	assert.Equal(t, filepath.Clean("/tmp/absolute.yaml"), resolvePath(dir, "/tmp/absolute.yaml"))
	assert.Equal(t, []any{"one", "two"}, asAnySlice([]string{"one", "two"}))
	assert.Equal(t, []any{"one"}, asAnySlice("one"))
	assert.Nil(t, asAnySlice(42))
	assert.Equal(t, []string{"one"}, asStringSlice([]any{"one", "", 2}))
}
