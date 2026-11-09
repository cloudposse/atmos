package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRuntimePathsProtectResolvedModuleRoot verifies that local backend state is protected under the
// module root Terraform actually runs in, not only under target and target/<component>.
func TestRuntimePathsProtectResolvedModuleRoot(t *testing.T) {
	target := filepath.Join(t.TempDir(), "work")
	backend := map[string]any{"path": filepath.Join("state", "custom.tfstate")}

	t.Run("metadata.component subpath before the workdir key is resolved", func(t *testing.T) {
		protected := runtimePaths(target, map[string]any{
			"component": "vpc",
			"metadata":  map[string]any{"component": "modules/vpc"},
			"backend":   backend,
		})
		root := filepath.Join(target, "modules", "vpc")
		assert.True(t, protected[filepath.Join(root, "state", "custom.tfstate")])
		assert.True(t, protected[filepath.Join(root, "state", "custom.tfstate.backup")])
		assert.True(t, protected[filepath.Join(root, "terraform.tfstate.d")])
		// The existing roots stay protected.
		assert.True(t, protected[filepath.Join(target, "state", "custom.tfstate")])
		assert.True(t, protected[filepath.Join(target, "vpc", "terraform.tfstate")])
	})

	t.Run("resolved workdir path", func(t *testing.T) {
		resolved := filepath.Join(target, "nested", "root")
		protected := runtimePaths(target, map[string]any{"component": "vpc", workdir.WorkdirPathKey: resolved, "backend": backend})
		assert.True(t, protected[filepath.Join(resolved, "state", "custom.tfstate")])
		assert.True(t, protected[filepath.Join(resolved, ".terraform")])
	})

	t.Run("workdir path outside the target is ignored", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "elsewhere")
		protected := runtimePaths(target, map[string]any{workdir.WorkdirPathKey: outside})
		assert.False(t, protected[filepath.Join(outside, "terraform.tfstate")])
	})
}

// TestSourceRefreshRejectsGeneratedBackendCollision verifies a source that ships backend.tf.json is
// rejected when Atmos generates that file, and installed normally when backend generation is off.
func TestSourceRefreshRejectsGeneratedBackendCollision(t *testing.T) {
	src := t.TempDir()
	refreshWrite(t, src, "main.tf", "# source")
	refreshWrite(t, src, "backend.tf.json", `{"terraform":{"backend":{"local":{}}}}`)
	spec := &schema.VendorComponentSource{Uri: src}

	generating := &schema.AtmosConfiguration{}
	generating.Components.Terraform.AutoGenerateBackendFile = true
	target := filepath.Join(t.TempDir(), "target")
	err := VendorSource(t.Context(), generating, spec, target, WithRuntimePreservation(nil))
	require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
	assert.Contains(t, err.Error(), "backend.tf.json")
	assert.NoFileExists(t, filepath.Join(target, "main.tf"), "a rejected refresh must not publish any source file")

	target = filepath.Join(t.TempDir(), "target")
	require.NoError(t, VendorSource(t.Context(), &schema.AtmosConfiguration{}, spec, target, WithRuntimePreservation(nil)))
	assert.FileExists(t, filepath.Join(target, "backend.tf.json"))
}

// TestSourceRefreshRejectsFileDirectoryTypeChange verifies a source update that turns a file into a
// directory, or a directory into a file, fails before publication with an actionable error.
func TestSourceRefreshRejectsFileDirectoryTypeChange(t *testing.T) {
	for _, tc := range []struct{ name, before, after, conflict string }{
		{name: "file becomes directory", before: "modules", after: filepath.Join("modules", "main.tf"), conflict: "modules"},
		{name: "directory becomes file", before: filepath.Join("modules", "main.tf"), after: "modules", conflict: "modules"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, second := t.TempDir(), t.TempDir()
			refreshWrite(t, first, "main.tf", "# first")
			refreshWrite(t, first, tc.before, "# before")
			refreshWrite(t, second, "main.tf", "# second")
			refreshWrite(t, second, tc.after, "# after")
			target := filepath.Join(t.TempDir(), "target")
			require.NoError(t, VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: first}, target, WithRuntimePreservation(nil)))

			err := VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: second}, target, WithRuntimePreservation(nil))
			require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
			assert.Contains(t, err.Error(), "between a file and a directory")
			assert.Contains(t, err.Error(), tc.conflict)
			// Nothing was published: the previous source is intact.
			content, readErr := os.ReadFile(filepath.Join(target, "main.tf"))
			require.NoError(t, readErr)
			assert.Equal(t, "# first", string(content))
		})
	}
}
