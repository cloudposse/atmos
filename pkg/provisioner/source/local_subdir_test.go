package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLocalSubdirCopiesRepositoryRootAndTracksSibling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	app := filepath.Join(root, "modules", "app")
	shared := filepath.Join(root, "modules", "shared")
	require.NoError(t, os.MkdirAll(app, 0o755))
	require.NoError(t, os.MkdirAll(shared, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "main.tf"), []byte(`module "shared" { source = "../shared" }`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(shared, "main.tf"), []byte("output \"id\" {}"), 0o644))
	spec := &schema.VendorComponentSource{Uri: root + "//modules/app"}
	target := filepath.Join(t.TempDir(), "checkout")
	require.NoError(t, VendorSource(t.Context(), nil, spec, target))
	require.FileExists(t, filepath.Join(target, "modules", "app", "main.tf"))
	require.FileExists(t, filepath.Join(target, "modules", "shared", "main.tf"))
	first := localSourceStamp(spec)
	require.NotEmpty(t, first)
	require.NoError(t, os.WriteFile(filepath.Join(shared, "main.tf"), []byte("output \"changed\" {}"), 0o644))
	require.NotEqual(t, first, localSourceStamp(spec))
}

func TestLocalSubdirRejectsTraversalAndOutsideSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	require.NoError(t, os.MkdirAll(root, 0o755))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside")))
	for _, suffix := range []string{"../outside", "modules/../outside", "/outside", "outside"} {
		err := VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: root + "//" + suffix}, filepath.Join(t.TempDir(), "target"))
		require.ErrorIs(t, err, errUtils.ErrSourceInvalidSpec)
	}
}

func TestLocalSubdirRetainsCacheIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	require.True(t, sameLocalSource(root+"//modules/app", root+"//modules/app"))
	require.False(t, sameLocalSource(root+"//modules/app", root+"//modules/shared"))
	require.False(t, sameLocalSource(root+"//modules/app", filepath.Join(root, "modules", "app")))
}

func TestLocalSubdirProtectsGeneratedBackend(t *testing.T) {
	stage := t.TempDir()
	target := filepath.Join(t.TempDir(), "checkout")
	module := filepath.Join(stage, "modules", "app")
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, generatedBackendFile), []byte("{}"), 0o644))
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	section := map[string]any{"metadata": map[string]any{"source_subdir": "modules/app"}}
	require.ErrorIs(t, rejectGeneratedBackendCollision(config, stage, target, section), errUtils.ErrSourceCopyFailed)
}

func TestLocalSubdirRefreshPreservesModuleState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	module := filepath.Join(root, "modules", "app")
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("terraform {}"), 0o644))
	target := filepath.Join(t.TempDir(), "checkout")
	section := map[string]any{"metadata": map[string]any{"source_subdir": "modules/app"}}
	spec := &schema.VendorComponentSource{Uri: root + "//modules/app"}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(section)))
	state := filepath.Join(target, "modules", "app", "terraform.tfstate")
	require.NoError(t, os.WriteFile(state, []byte("retained state"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("terraform { required_version = \">= 1.0\" }"), 0o644))
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(section)))
	content, err := os.ReadFile(state)
	require.NoError(t, err)
	require.Equal(t, "retained state", string(content))
}
