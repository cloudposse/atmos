package source

import (
	"context"
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

// TestSourceRefreshYieldsShippedBackendToGeneration verifies that a source shipping backend.tf.json
// provisions when Atmos generates that file, as it did before source ownership existed: the shipped
// copy is not installed or recorded as source-owned, so the generated backend wins on every run and a
// later refresh neither replaces nor removes it. With backend generation off, the shipped file is
// ordinary source content.
func TestSourceRefreshYieldsShippedBackendToGeneration(t *testing.T) {
	src := t.TempDir()
	refreshWrite(t, src, "main.tf", "# source")
	refreshWrite(t, src, generatedBackendFile, `{"terraform":{"backend":{"s3":{}}}}`)
	spec := &schema.VendorComponentSource{Uri: src}

	generating := &schema.AtmosConfiguration{}
	generating.Components.Terraform.AutoGenerateBackendFile = true
	var warned []string
	stubShippedBackendWarning(t, &warned)
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, VendorSource(t.Context(), generating, spec, target, WithRuntimePreservation(nil)))
	assert.FileExists(t, filepath.Join(target, "main.tf"))
	assert.NoFileExists(t, filepath.Join(target, generatedBackendFile), "the shipped backend must not be installed")
	inventory, err := readSourceInventory(target)
	require.NoError(t, err)
	assert.Equal(t, []string{"main.tf"}, inventory.Files, "the shipped backend must not become source-owned")
	assert.Equal(t, []string{generatedBackendFile}, warned)

	// Atmos generates its backend; a forced refresh of an existing workdir keeps it untouched.
	generated := `{"terraform":{"backend":{"local":{}}}}`
	refreshWrite(t, target, generatedBackendFile, generated)
	refreshWrite(t, src, "main.tf", "# updated")
	require.NoError(t, VendorSource(t.Context(), generating, spec, target, WithRuntimePreservation(nil)))
	content, err := os.ReadFile(filepath.Join(target, generatedBackendFile))
	require.NoError(t, err)
	assert.Equal(t, generated, string(content))
	content, err = os.ReadFile(filepath.Join(target, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# updated", string(content))

	// Without backend generation the module's own backend file is source content.
	target = filepath.Join(t.TempDir(), "target")
	require.NoError(t, VendorSource(t.Context(), &schema.AtmosConfiguration{}, spec, target, WithRuntimePreservation(nil)))
	assert.FileExists(t, filepath.Join(target, generatedBackendFile))
}

// TestSourceRefreshTakesOverPreviouslyOwnedBackend verifies that enabling backend generation for a
// workdir whose receipt recorded the module's backend.tf.json as source-owned hands the path to
// generation: the obsolete source copy is removed so Atmos writes its own backend there.
func TestSourceRefreshTakesOverPreviouslyOwnedBackend(t *testing.T) {
	src := t.TempDir()
	refreshWrite(t, src, "main.tf", "# source")
	refreshWrite(t, src, generatedBackendFile, `{"terraform":{"backend":{"s3":{}}}}`)
	spec := &schema.VendorComponentSource{Uri: src}
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, VendorSource(t.Context(), &schema.AtmosConfiguration{}, spec, target, WithRuntimePreservation(nil)))
	require.FileExists(t, filepath.Join(target, generatedBackendFile))

	generating := &schema.AtmosConfiguration{}
	generating.Components.Terraform.AutoGenerateBackendFile = true
	stubShippedBackendWarning(t, new([]string))
	require.NoError(t, VendorSource(t.Context(), generating, spec, target, WithRuntimePreservation(nil)))
	assert.NoFileExists(t, filepath.Join(target, generatedBackendFile))
	assert.FileExists(t, filepath.Join(target, "main.tf"))
}

// TestSourceRefreshStillRejectsOtherCollisions is the negative path of the backend exemption: a
// shipped file that collides with caller-owned content is still refused, and a directory named
// like the backend file is not silently discarded.
func TestSourceRefreshStillRejectsOtherCollisions(t *testing.T) {
	generating := &schema.AtmosConfiguration{}
	generating.Components.Terraform.AutoGenerateBackendFile = true
	stubShippedBackendWarning(t, new([]string))

	t.Run("runtime state", func(t *testing.T) {
		src := t.TempDir()
		refreshWrite(t, src, "main.tf", "# source")
		refreshWrite(t, src, "terraform.tfstate", `{"version":4}`)
		target := filepath.Join(t.TempDir(), "target")
		err := VendorSource(t.Context(), generating, &schema.VendorComponentSource{Uri: src}, target, WithRuntimePreservation(nil))
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
		assert.NoFileExists(t, filepath.Join(target, "main.tf"))
	})

	t.Run("backend path is a directory", func(t *testing.T) {
		src := t.TempDir()
		refreshWrite(t, src, "main.tf", "# source")
		refreshWrite(t, src, filepath.Join(generatedBackendFile, "nested.tf"), "# nested")
		target := filepath.Join(t.TempDir(), "target")
		err := VendorSource(t.Context(), generating, &schema.VendorComponentSource{Uri: src}, target, WithRuntimePreservation(nil))
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
		assert.Contains(t, err.Error(), generatedBackendFile)
		assert.NoFileExists(t, filepath.Join(target, "main.tf"))
	})
}

// stubShippedBackendWarning records the shipped-backend warnings of one test.
func stubShippedBackendWarning(t *testing.T, warned *[]string) {
	t.Helper()
	original := warnShippedBackendReplaced
	warnShippedBackendReplaced = func(_ context.Context, path string) { *warned = append(*warned, path) }
	t.Cleanup(func() { warnShippedBackendReplaced = original })
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
			assert.NotContains(t, err.Error(), "source delete")

			// The documented recovery moves only the conflicting path aside; refresh then succeeds.
			require.NoError(t, os.Rename(filepath.Join(target, tc.conflict), filepath.Join(t.TempDir(), "aside")))
			require.NoError(t, VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: second}, target, WithRuntimePreservation(nil)))
			content, readErr = os.ReadFile(filepath.Join(target, tc.after))
			require.NoError(t, readErr)
			assert.Equal(t, "# after", string(content))
		})
	}
}

// TestSourceRefreshRejectsRuntimePathsOnFreshCheckout verifies that a source cannot plant initial
// local state or Terraform runtime files when the destination does not exist yet, while ordinary
// source files and an absent provider lock file still install.
func TestSourceRefreshRejectsRuntimePathsOnFreshCheckout(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		config  map[string]any
		wantErr bool
	}{
		{name: "case-variant-state", file: "Terraform.tfstate", wantErr: true},
		{name: "mixed-case-backend", file: "STATE/Custom.tfstate", config: map[string]any{"backend": map[string]any{"path": "state/custom.tfstate"}}, wantErr: true},
		{name: "default-state", file: "terraform.tfstate", wantErr: true},
		{name: "state-backup", file: "terraform.tfstate.backup", wantErr: true},
		{name: "workspace-state", file: "terraform.tfstate.d/dev/terraform.tfstate", wantErr: true},
		{name: "terraform-dir", file: ".terraform/terraform.tfstate", wantErr: true},
		{
			name:    "configured-backend-path",
			file:    "state/custom.tfstate",
			config:  map[string]any{"backend": map[string]any{"local": map[string]any{"path": "state/custom.tfstate"}}},
			wantErr: true,
		},
		{name: "provider-lock", file: ".terraform.lock.hcl"},
		{name: "ordinary-file", file: "data/custom.tfstate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
			refreshWrite(t, src, "main.tf", "# source")
			refreshWrite(t, src, tc.file, `{"version":4,"lineage":"shipped-by-source","serial":1}`)
			spec := &schema.VendorComponentSource{Uri: src}
			err := VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(tc.config))
			_, statErr := os.Lstat(filepath.Join(target, filepath.FromSlash(tc.file)))
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
				require.ErrorContains(t, err, "collides with runtime artifact "+tc.file)
				require.True(t, os.IsNotExist(statErr), "the source file must not be published at a runtime path")
				return
			}
			require.NoError(t, err)
			require.NoError(t, statErr, "a non-runtime source file must still be installed")
			require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(tc.config)))
			if tc.name == "ordinary-file" {
				refreshWrite(t, target, tc.file, `{"version":4,"lineage":"runtime","serial":2}`)
				require.ErrorIs(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(tc.config)), errUtils.ErrSourceCopyFailed)
			}
		})
	}
}

// TestRuntimeOwnedAbsentDestination verifies the missing-destination branch directly: absent paths under a
// protected root are runtime-owned, and absent paths elsewhere, including the provider lock file, are not.
func TestRuntimeOwnedAbsentDestination(t *testing.T) {
	target := t.TempDir()
	protected := runtimePaths(target, nil)
	for relative, want := range map[string]bool{
		"terraform.tfstate":            true,
		".terraform/modules/x/main.tf": true,
		".terraform.lock.hcl":          false,
		"modules/terraform.tfstate":    false,
		"terraform.tfstate.d.notes":    false,
	} {
		owned, err := runtimeOwned(filepath.Join(target, filepath.FromSlash(relative)), relative, protected)
		require.NoError(t, err)
		assert.Equal(t, want, owned, relative)
	}
}

// TestReadSourceInventoryWrapsFailures verifies that unreadable and unparseable ownership manifests
// carry ErrSourceCopyFailed and keep their cause.
func TestReadSourceInventoryWrapsFailures(t *testing.T) {
	t.Run("unparseable", func(t *testing.T) {
		target := t.TempDir()
		refreshWrite(t, target, sourceFilesManifest, "not json")
		_, err := readSourceInventory(target)
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
		require.ErrorContains(t, err, "read source ownership")
	})
	t.Run("unreadable", func(t *testing.T) {
		target := t.TempDir()
		// A directory at the manifest path makes the read fail with an error other than not-exist.
		require.NoError(t, os.MkdirAll(filepath.Join(target, filepath.FromSlash(sourceFilesManifest)), 0o755))
		_, err := readSourceInventory(target)
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
		require.ErrorContains(t, err, "read source ownership")
	})
}

func TestProtectedRuntimePathCaseAndBoundary(t *testing.T) {
	root := t.TempDir()
	protected := map[string]bool{filepath.Join(root, "State"): true}
	for _, name := range []string{"STATE", "state/child", ".ATMOS/Metadata.JSON", ".ATMOS/Source.JSON", ".Terraform.Lock.hcl"} {
		assert.True(t, protectedRuntimePath(filepath.Join(root, filepath.FromSlash(name)), name, protected), name)
	}
	assert.False(t, protectedRuntimePath(filepath.Join(root, "StateOther"), "StateOther", protected))
}
