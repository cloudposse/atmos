package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
)

const twoDocYAML = `apiVersion: v1
kind: Service
metadata:
  name: svc
  namespace: demo
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dep
  namespace: demo
`

func TestDecodeObjects_MultiDoc(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	require.Len(t, objects, 2)
	assert.Equal(t, "Service", objects[0].GetKind())
	assert.Equal(t, "svc", objects[0].GetName())
	assert.Equal(t, "Deployment", objects[1].GetKind())
	assert.Equal(t, "dep", objects[1].GetName())
}

func TestDecodeObjects_MissingAPIVersion(t *testing.T) {
	_, err := DecodeObjects([]byte("kind: Service\nmetadata:\n  name: x\n"))
	assert.ErrorIs(t, err, errUtils.ErrManifestMissingAPIVersionKind)
}

func TestObjectFileName_Deterministic(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web", "namespace": "prod"},
	}}
	name := ObjectFileName(0, obj)
	assert.Equal(t, "001_apps_apps_v1_Deployment_prod_web.yaml", name)
}

func TestArtifactFiles(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	files, err := ArtifactFiles(objects)
	require.NoError(t, err)
	require.Len(t, files, 2)
	_, hasSvc := files["001_v1_Service_demo_svc.yaml"]
	_, hasDep := files["002_apps_apps_v1_Deployment_demo_dep.yaml"]
	assert.True(t, hasSvc)
	assert.True(t, hasDep)
}

func TestMultiDocumentYAML_RoundTrip(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	data, err := MultiDocumentYAML(objects)
	require.NoError(t, err)
	again, err := DecodeObjects(data)
	require.NoError(t, err)
	require.Len(t, again, 2)
	assert.Equal(t, "svc", again[0].GetName())
	assert.Equal(t, "dep", again[1].GetName())
}

func TestValidateRenderOptions(t *testing.T) {
	assert.NoError(t, ValidateRenderOptions(RenderOptions{}))
	assert.NoError(t, ValidateRenderOptions(RenderOptions{Output: "a.yaml"}))
	assert.NoError(t, ValidateRenderOptions(RenderOptions{OutputDir: "d", Split: true}))
	assert.Error(t, ValidateRenderOptions(RenderOptions{Output: "a.yaml", OutputDir: "d"}))
	assert.Error(t, ValidateRenderOptions(RenderOptions{Output: "a.yaml", Split: true}))
	assert.Error(t, ValidateRenderOptions(RenderOptions{Split: true}))
}

func TestWriteObjects_SplitFiles(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, WriteObjects(objects, RenderOptions{OutputDir: dir, Split: true, Noun: "Helm"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "001_v1_Service_demo_svc.yaml", entries[0].Name())
	assert.Equal(t, "002_apps_apps_v1_Deployment_demo_dep.yaml", entries[1].Name())
}

func TestWriteObjects_SingleFile(t *testing.T) {
	objects, err := DecodeObjects([]byte(twoDocYAML))
	require.NoError(t, err)
	dir := t.TempDir()
	out := filepath.Join(dir, "all.yaml")
	require.NoError(t, WriteObjects(objects, RenderOptions{Output: out, Noun: "Helm"}))

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	decoded, err := DecodeObjects(data)
	require.NoError(t, err)
	require.Len(t, decoded, 2)
}
