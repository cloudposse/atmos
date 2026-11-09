package source

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
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
