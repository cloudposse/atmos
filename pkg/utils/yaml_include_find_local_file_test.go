package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// writeIncludeFixture creates a file with the given content, creating parent directories.
func writeIncludeFixture(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// TestFindLocalFileResolutionOrder verifies the documented resolution order of local
// include paths: absolute paths as-is, `./` and `../` paths relative to the including
// manifest only, all other relative paths relative to `base_path`. The process working
// directory is only a last resort for the latter. Checking it before `base_path` made
// `describe affected` read the current checkout's file for both the current and the
// base stacks whenever the working directory was the repository root, which hid
// edits to included files.
func TestFindLocalFileResolutionOrder(t *testing.T) {
	baseDir := t.TempDir()
	workDir := t.TempDir()

	manifestFile := filepath.Join(baseDir, "stacks", "deploy", "demo.yaml")
	writeIncludeFixture(t, manifestFile, "vars: {}\n")

	// The same relative path exists under base_path and under the working directory.
	writeIncludeFixture(t, filepath.Join(baseDir, "docs", "shared.yaml"), "kind: base\n")
	writeIncludeFixture(t, filepath.Join(workDir, "docs", "shared.yaml"), "kind: cwd\n")
	// A file next to the manifest.
	writeIncludeFixture(t, filepath.Join(baseDir, "stacks", "deploy", "demo", "host.yaml"), "kind: host\n")
	// A file that exists only in the working directory.
	writeIncludeFixture(t, filepath.Join(workDir, "cwd-only.yaml"), "kind: cwd-only\n")

	t.Chdir(workDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:         baseDir,
		BasePathAbsolute: baseDir,
	}

	tests := []struct {
		name    string
		include string
		want    string
	}{
		{
			name:    "bare relative path resolves against base_path, not the working directory",
			include: filepath.Join("docs", "shared.yaml"),
			want:    filepath.Join(baseDir, "docs", "shared.yaml"),
		},
		{
			name:    "dot-slash path resolves against the manifest directory",
			include: "./demo/host.yaml",
			want:    filepath.Join(baseDir, "stacks", "deploy", "demo", "host.yaml"),
		},
		{
			name:    "dot-dot path resolves against the manifest directory",
			include: "../../docs/shared.yaml",
			want:    filepath.Join(baseDir, "docs", "shared.yaml"),
		},
		{
			name:    "dot-slash path missing beside the manifest does not fall back to base_path or the working directory",
			include: "./docs/shared.yaml",
			want:    "",
		},
		{
			name:    "absolute path is used as-is",
			include: filepath.Join(workDir, "docs", "shared.yaml"),
			want:    filepath.Join(workDir, "docs", "shared.yaml"),
		},
		{
			name:    "working directory remains the last resort",
			include: "cwd-only.yaml",
			want:    filepath.Join(workDir, "cwd-only.yaml"),
		},
		{
			name:    "missing file resolves to empty",
			include: filepath.Join("docs", "missing.yaml"),
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findLocalFile(tt.include, manifestFile, atmosConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFindLocalFileFollowsRebasedBasePath reproduces the `describe affected` flow:
// the configuration is re-based onto a second checkout while the working directory
// stays in the first one. A `base_path`-relative include must resolve inside the
// checkout named by the configuration, so that the two checkouts can differ.
func TestFindLocalFileFollowsRebasedBasePath(t *testing.T) {
	currentCheckout := t.TempDir()
	baseCheckout := t.TempDir()
	relative := filepath.Join("docs", "shared.yaml")
	writeIncludeFixture(t, filepath.Join(currentCheckout, relative), "kind: edited\n")
	writeIncludeFixture(t, filepath.Join(baseCheckout, relative), "kind: original\n")

	t.Chdir(currentCheckout)

	for _, checkout := range []string{currentCheckout, baseCheckout} {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath:         checkout,
			BasePathAbsolute: checkout,
		}
		manifestFile := filepath.Join(checkout, "stacks", "deploy", "demo.yaml")

		got := findLocalFile(relative, manifestFile, atmosConfig)

		assert.Equal(t, filepath.Join(checkout, relative), got,
			"include must resolve inside the checkout named by base_path")
	}
}
