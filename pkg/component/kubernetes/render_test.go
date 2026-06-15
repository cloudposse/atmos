package kubernetes

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
)

// requireReadOnlyDirSupport skips tests that depend on a read-only directory
// rejecting writes. Chmod-based permission enforcement is unreliable on Windows
// and ineffective when running as root.
func requireReadOnlyDirSupport(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions are not enforced on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("read-only directory permissions are bypassed when running as root")
	}
}

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

	// --output is single-file mode and overrides a component-configured split,
	// otherwise validation would reject it ("--split requires --output-dir").
	options = resolveRenderOptions(map[string]any{"output": "flag-output.yaml"}, componentSection)
	require.Equal(t, renderOptions{Output: "flag-output.yaml", Split: false}, options)

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

func TestWriteSingleManifestFileMkdirError(t *testing.T) {
	requireReadOnlyDirSupport(t)

	readOnly := t.TempDir()
	require.NoError(t, os.Chmod(readOnly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	// MkdirAll must fail when creating a subdirectory under the read-only dir.
	err := writeSingleManifestFile(filepath.Join(readOnly, "nested", "manifest.yaml"), testRenderObjects())
	require.ErrorIs(t, err, errUtils.ErrKubernetesRenderOutput)
	require.ErrorContains(t, err, "creating output directory")
}

func TestWriteSingleManifestFileWriteError(t *testing.T) {
	requireReadOnlyDirSupport(t)

	readOnly := t.TempDir()
	require.NoError(t, os.Chmod(readOnly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	// The directory already exists (MkdirAll is a no-op) but WriteFile must fail
	// because the directory is read-only.
	err := writeSingleManifestFile(filepath.Join(readOnly, "manifest.yaml"), testRenderObjects())
	require.ErrorIs(t, err, errUtils.ErrKubernetesRenderOutput)
	require.ErrorContains(t, err, "writing to")
}

func TestWriteSplitManifestFilesMkdirError(t *testing.T) {
	requireReadOnlyDirSupport(t)

	readOnly := t.TempDir()
	require.NoError(t, os.Chmod(readOnly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	err := writeSplitManifestFiles(filepath.Join(readOnly, "nested"), testRenderObjects())
	require.ErrorIs(t, err, errUtils.ErrKubernetesRenderOutput)
	require.ErrorContains(t, err, "creating output directory")
}

func TestWriteSplitManifestFilesWriteError(t *testing.T) {
	requireReadOnlyDirSupport(t)

	readOnly := t.TempDir()
	require.NoError(t, os.Chmod(readOnly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	// The output directory exists but is read-only, so writing the per-object
	// manifest file must fail.
	err := writeSplitManifestFiles(readOnly, testRenderObjects())
	require.ErrorIs(t, err, errUtils.ErrKubernetesRenderOutput)
	require.ErrorContains(t, err, "writing manifest")
}

func TestObjectYAMLReturnsErrorForUnserializableObject(t *testing.T) {
	// A value that the YAML marshaller cannot serialize (a function) must surface
	// as ErrKubernetesRender rather than panicking or being silently dropped.
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "settings"},
		"data":       map[string]any{"bad": func() {}},
	}}

	_, err := objectYAML(obj)
	require.ErrorIs(t, err, errUtils.ErrKubernetesRender)
	require.ErrorContains(t, err, "ConfigMap/settings")
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
