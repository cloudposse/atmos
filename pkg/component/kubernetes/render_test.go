package kubernetes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderObjectsWritesSingleManifestFile(t *testing.T) {
	output := filepath.Join(t.TempDir(), "rendered.yaml")

	err := renderObjects(testRenderObjects(), renderOptions{Output: output})

	require.NoError(t, err)
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(data), "kind: ConfigMap")
	require.Contains(t, string(data), "---")
	require.Contains(t, string(data), "kind: Service")
}

func TestRenderObjectsWritesSplitManifestFiles(t *testing.T) {
	outputDir := t.TempDir()

	err := renderObjects(testRenderObjects(), renderOptions{OutputDir: outputDir, Split: true})

	require.NoError(t, err)
	files, err := os.ReadDir(outputDir)
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "001_v1_ConfigMap_settings.yaml", files[0].Name())
	require.Equal(t, "002_v1_Service_default_app.yaml", files[1].Name())
}

func TestRenderObjectsRequiresOutputDirForSplit(t *testing.T) {
	err := renderObjects(testRenderObjects(), renderOptions{Split: true})
	require.ErrorContains(t, err, "--split requires --output-dir")
}

func TestRenderObjectsRejectsConflictingOutputOptions(t *testing.T) {
	err := renderObjects(testRenderObjects(), renderOptions{Output: "manifest.yaml", OutputDir: "manifests"})
	require.ErrorContains(t, err, "--output and --output-dir are mutually exclusive")

	err = renderObjects(testRenderObjects(), renderOptions{Output: "manifest.yaml", Split: true})
	require.ErrorContains(t, err, "--split requires --output-dir and cannot be used with --output")
}

func TestRenderObjectsWritesManifestFileInOutputDir(t *testing.T) {
	outputDir := t.TempDir()

	err := renderObjects(testRenderObjects(), renderOptions{OutputDir: outputDir})

	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(outputDir, "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "kind: ConfigMap")
	require.Contains(t, string(data), "kind: Service")
}

func TestRenderObjectsWritesToStdoutByDefault(t *testing.T) {
	output := captureKubernetesStdout(t, func() {
		require.NoError(t, renderObjects(testRenderObjects(), renderOptions{}))
	})

	require.Contains(t, output, "kind: ConfigMap")
	require.Contains(t, output, "---")
	require.Contains(t, output, "kind: Service")
}

func TestResolveRenderOptions(t *testing.T) {
	componentSection := map[string]any{
		"render": map[string]any{
			"output": map[string]any{
				"path":  "component-output",
				"split": true,
			},
		},
	}

	options := resolveRenderOptions(nil, componentSection)
	require.Equal(t, renderOptions{OutputDir: "component-output", Split: true}, options)

	options = resolveRenderOptions(map[string]any{"output": "flag-output.yaml"}, componentSection)
	require.Equal(t, renderOptions{Output: "flag-output.yaml", Split: true}, options)

	options = resolveRenderOptions(map[string]any{"output_dir": "flag-dir", "split": true}, map[string]any{})
	require.Equal(t, renderOptions{OutputDir: "flag-dir", Split: true}, options)
}

func TestRenderOptionsFromComponentSingleFile(t *testing.T) {
	options := renderOptionsFromComponent(map[string]any{
		"render": map[string]any{
			"output": map[string]any{
				"path": "rendered.yaml",
			},
		},
	})

	require.Equal(t, renderOptions{Output: "rendered.yaml"}, options)
	require.Empty(t, renderOptionsFromComponent(nil))
	require.Empty(t, renderOptionsFromComponent(map[string]any{"render": "invalid"}))
}

func testRenderObjects() []*unstructured.Unstructured {
	return []*unstructured.Unstructured{
		{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "settings",
				},
			},
		},
		{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]any{
					"name":      "app",
					"namespace": "default",
				},
			},
		},
	}
}
