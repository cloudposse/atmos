package source

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNeedsVendoring(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			expected: true,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				emptyDir := filepath.Join(dir, "empty")
				err := os.MkdirAll(emptyDir, 0o755)
				require.NoError(t, err)
				return emptyDir
			},
			expected: true,
		},
		{
			name: "directory with files",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				populatedDir := filepath.Join(dir, "populated")
				err := os.MkdirAll(populatedDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(populatedDir, "main.tf"), []byte("# test"), 0o644)
				require.NoError(t, err)
				return populatedDir
			},
			expected: false,
		},
		{
			name: "file instead of directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				filePath := filepath.Join(dir, "file.txt")
				err := os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := tt.setup(t)
			result := needsVendoring(targetDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineTargetDirectory(t *testing.T) {
	tests := []struct {
		name            string
		atmosConfig     *schema.AtmosConfiguration
		componentType   string
		component       string
		componentConfig map[string]any
		expectedDir     string
		expectError     error
	}{
		{
			name: "working_directory in metadata takes priority",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"working_directory": "/custom/path/vpc",
				},
			},
			expectedDir: "/custom/path/vpc",
			expectError: nil,
		},
		{
			name: "working_directory in settings takes priority over default",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"settings": map[string]any{
					"working_directory": "/settings/path/vpc",
				},
			},
			expectedDir: "/settings/path/vpc",
			expectError: nil,
		},
		{
			name: "default terraform base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     filepath.Join("components", "terraform", "vpc"),
			expectError:     nil,
		},
		{
			name: "default helmfile base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			componentType:   "helmfile",
			component:       "nginx",
			componentConfig: map[string]any{},
			expectedDir:     filepath.Join("components", "helmfile", "nginx"),
			expectError:     nil,
		},
		{
			name: "default packer base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "components/packer",
					},
				},
			},
			componentType:   "packer",
			component:       "ami",
			componentConfig: map[string]any{},
			expectedDir:     filepath.Join("components", "packer", "ami"),
			expectError:     nil,
		},
		{
			name: "no base path configured for terraform",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "",
					},
				},
			},
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrInvalidConfig,
		},
		{
			name:            "nil atmos config",
			atmosConfig:     nil,
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrInvalidConfig,
		},
		{
			name: "unknown component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "unknown",
			component:       "test",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrInvalidConfig,
		},
		{
			// Regression test: the non-workdir (default vendoring) fallback used
			// filepath.Join(componentBasePath, component) with zero containment guard.
			// A component named "../escape-test-nowd" (source: set, workdir NOT enabled -
			// the default JIT-vendoring path) resolves outside components/terraform/ into a
			// sibling components/escape-test-nowd/ directory. Confirmed on disk via
			// tests/fixtures/scenarios/source-provisioner-workdir-nested. Two other BuildPath
			// callers (internal/terraform_backend/terraform_backend_local.go and
			// pkg/terraform/output/config.go) already guard against exactly this; this
			// caller must too.
			name: "component name with .. escapes component base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "terraform",
			component:       "../escape-test-nowd",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrPathTraversal,
		},
		{
			// Regression test: filepath.Join(componentBasePath, ".") cleans to
			// componentBasePath itself. isWithinBase's filepath.Rel-based check treats
			// rel == "." as "within base" (it is, technically -- it *is* the base), so a
			// naive containment guard would let a component named "." silently vendor
			// into the shared components/terraform/ directory instead of a per-component
			// subdirectory. This must be a hard error, not a fallback, since
			// DetermineTargetDirectory's default branch has no further fallback to try.
			name: "component name of . resolves to the component base path itself",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "terraform",
			component:       ".",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrPathTraversal,
		},
		{
			// Regression test: same root cause as the "." case above, reached via a
			// component name that lexically cancels itself out --
			// filepath.Join(componentBasePath, "child", "..") also cleans to
			// componentBasePath itself.
			name: "component name of child/.. resolves to the component base path itself",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			// A literal "child/.." component name, exactly as it could appear in a stack
			// manifest -- not built with filepath.Join, since the value under test is the
			// raw (potentially attacker-controlled) string, and filepath.Clean treats '/'
			// as a separator on all platforms including Windows.
			component:       "child/..",
			componentConfig: map[string]any{},
			expectedDir:     "",
			expectError:     errUtils.ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DetermineTargetDirectory(tt.atmosConfig, tt.componentType, tt.component, tt.componentConfig)

			if tt.expectError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedDir, result)
			}
		})
	}
}

// TestValidateWithinComponentBasePath_RootBase is a regression test for the naive
// absBase+separator prefix check rejecting every valid descendant when
// componentBasePath resolves to a filesystem root ("/" on Unix): absBase already
// ends in the separator there, so the literal absBase+sep prefix ("//") never
// matches any real target, incorrectly returning ErrPathTraversal for legitimate
// paths. The filepath.Rel-based check does not have this edge case.
func TestValidateWithinComponentBasePath_RootBase(t *testing.T) {
	tests := []struct {
		name        string
		targetDir   string
		base        string
		expectError bool
	}{
		{
			name:        "descendant of filesystem root base is allowed",
			targetDir:   "/vpc",
			base:        "/",
			expectError: false,
		},
		{
			name:        "nested descendant of filesystem root base is allowed",
			targetDir:   "/terraform/vpc",
			base:        "/",
			expectError: false,
		},
		{
			name:        "root base equals target",
			targetDir:   "/",
			base:        "/",
			expectError: false,
		},
		{
			// Nothing is "above" the filesystem root -- filepath.Clean collapses a
			// leading ".." at the root back to "/" (e.g. "/../outside" -> "/outside"),
			// so an escape-from-root case isn't constructible. Confirm instead that
			// the filepath.Rel-based rewrite didn't weaken rejection in general by
			// checking a true escape against a non-root base still fails.
			name:        "true escape from a non-root base is still rejected",
			targetDir:   "/base/../../outside",
			base:        "/base",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWithinComponentBasePath(tt.targetDir, tt.base)
			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// trySymlink attempts to create a symlink and skips the test if unsupported (e.g. Windows
// without Developer Mode / SeCreateSymbolicLinkPrivilege, or a locked-down CI sandbox).
func trySymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("skipping symlink test: cannot create symlink (%v)", err)
	}
}

// TestValidateWithinComponentBasePath_SymlinkEscape is a regression test for a gap where a
// symlink under componentBasePath pointing outside it was lexically contained (the literal path
// string starts with componentBasePath) but resolved outside componentBasePath on the real
// filesystem, defeating the containment guard; validateWithinComponentBasePath must resolve
// symlinks in whatever portion of the path already exists and reject based on where it actually
// points.
func TestValidateWithinComponentBasePath_SymlinkEscape(t *testing.T) {
	componentBasePath := t.TempDir()
	outsideDir := t.TempDir() // Sibling directory, NOT under componentBasePath.

	symlinkPath := filepath.Join(componentBasePath, "evil")
	trySymlink(t, outsideDir, symlinkPath)

	// targetDir itself does not exist yet -- only the "evil" symlink ancestor does -- mirroring
	// the pre-creation call site in DetermineTargetDirectory.
	targetDir := filepath.Join(symlinkPath, "vpc")

	err := validateWithinComponentBasePath(targetDir, componentBasePath)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestValidateWithinComponentBasePath_SymlinkWithinBase confirms a symlink under
// componentBasePath that points to another location *inside* componentBasePath is still
// allowed -- the new symlink resolution must not become overly strict.
func TestValidateWithinComponentBasePath_SymlinkWithinBase(t *testing.T) {
	componentBasePath := t.TempDir()
	realDir := filepath.Join(componentBasePath, "real-vpc")
	require.NoError(t, os.MkdirAll(realDir, 0o755))

	linkPath := filepath.Join(componentBasePath, "link-vpc")
	trySymlink(t, realDir, linkPath)

	err := validateWithinComponentBasePath(linkPath, componentBasePath)
	assert.NoError(t, err)
}

// TestValidateWithinComponentBasePath_BaseItselfIsSymlink confirms componentBasePath itself
// being reached through a symlink (e.g. macOS /tmp -> /private/tmp) does not cause a false
// positive: both target and base resolve to the same real location.
func TestValidateWithinComponentBasePath_BaseItselfIsSymlink(t *testing.T) {
	realBase := t.TempDir()
	linkBase := filepath.Join(t.TempDir(), "base-link")
	trySymlink(t, realBase, linkBase)

	targetDir := filepath.Join(linkBase, "vpc")
	err := validateWithinComponentBasePath(targetDir, linkBase)
	assert.NoError(t, err)
}

// TestValidateWithinComponentBasePath_SymlinkResolutionErrorPropagates verifies that a
// non-ENOENT failure while resolving symlinks in targetDir's ancestors (e.g. permission denied
// on an intermediate directory) is surfaced as a wrapped ErrPathTraversal rather than being
// silently treated the same as "ancestor does not exist yet" and skipped. This exercises
// resolveExistingSymlinks's `!os.IsNotExist(err)` branch: EvalSymlinks cannot even determine
// whether "sub" exists under an unsearchable "guarded" directory, so it fails with a permission
// error rather than ENOENT.
func TestValidateWithinComponentBasePath_SymlinkResolutionErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod cannot deny directory traversal on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("test relies on POSIX permission denial; root bypasses chmod")
	}
	componentBasePath := t.TempDir()
	guardedDir := filepath.Join(componentBasePath, "guarded")
	require.NoError(t, os.MkdirAll(guardedDir, 0o755))
	require.NoError(t, os.Chmod(guardedDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(guardedDir, 0o755) })

	// targetDir does not exist; its "guarded" ancestor does, but with no search (execute)
	// permission, so resolving whether "sub" exists underneath it fails with EACCES rather
	// than ENOENT.
	targetDir := filepath.Join(guardedDir, "sub", "vpc")

	err := validateWithinComponentBasePath(targetDir, componentBasePath)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestValidateTargetIsComponentSubdirectory covers the stricter, equality-rejecting check used
// only by DetermineTargetDirectory's default vendoring-target branch. Unlike
// validateWithinComponentBasePath (which intentionally allows target == base, per
// TestValidateWithinComponentBasePath_RootBase's "root base equals target" case),
// validateTargetIsComponentSubdirectory must reject target == base, since on this call path that
// only happens when the component name collapsed to nothing (e.g. "." or "child/..").
func TestValidateTargetIsComponentSubdirectory(t *testing.T) {
	componentBasePath := t.TempDir()

	t.Run("target equal to base is rejected", func(t *testing.T) {
		err := validateTargetIsComponentSubdirectory(componentBasePath, componentBasePath)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
	})

	t.Run("target equal to base via dot is rejected", func(t *testing.T) {
		dotTarget := filepath.Join(componentBasePath, ".")
		err := validateTargetIsComponentSubdirectory(dotTarget, componentBasePath)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
	})

	t.Run("target equal to base via canceling segments is rejected", func(t *testing.T) {
		cancelingTarget := filepath.Join(componentBasePath, "child", "..")
		err := validateTargetIsComponentSubdirectory(cancelingTarget, componentBasePath)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
	})

	t.Run("genuine subdirectory of base is allowed", func(t *testing.T) {
		target := filepath.Join(componentBasePath, "vpc")
		err := validateTargetIsComponentSubdirectory(target, componentBasePath)
		assert.NoError(t, err)
	})

	t.Run("escape from base is still rejected", func(t *testing.T) {
		target := filepath.Join(componentBasePath, "..", "outside")
		err := validateTargetIsComponentSubdirectory(target, componentBasePath)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
	})
}

func TestGetComponentBasePath(t *testing.T) {
	tests := []struct {
		name          string
		atmosConfig   *schema.AtmosConfiguration
		componentType string
		expected      string
	}{
		{
			name: "terraform component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			expected:      "components/terraform",
		},
		{
			name: "helmfile component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			componentType: "helmfile",
			expected:      "components/helmfile",
		},
		{
			name: "packer component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "components/packer",
					},
				},
			},
			componentType: "packer",
			expected:      "components/packer",
		},
		{
			name: "unknown component type",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "unknown",
			expected:      "",
		},
		{
			name:          "nil config",
			atmosConfig:   nil,
			componentType: "terraform",
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentBasePath(tt.atmosConfig, tt.componentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProvision_NilParams tests that Provision returns an error when params is nil.
func TestProvision_NilParams(t *testing.T) {
	ctx := context.Background()

	err := Provision(ctx, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestProvision_NoSource tests that Provision returns nil when no source is configured.
func TestProvision_NoSource(t *testing.T) {
	ctx := context.Background()

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		},
		ComponentType:   "terraform",
		Component:       "vpc",
		Stack:           "dev",
		ComponentConfig: map[string]any{}, // No source configured.
		Force:           false,
	}

	err := Provision(ctx, params)
	assert.NoError(t, err, "Provision should return nil when no source is configured")
}

// TestProvision_InvalidSource tests that Provision returns an error for invalid source spec.
func TestProvision_InvalidSource(t *testing.T) {
	ctx := context.Background()

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			// Invalid: source is a number, not a string or map.
			"source": 12345,
		},
		Force: false,
	}

	err := Provision(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}

// TestProvision_TargetDirectoryError tests that Provision returns an error when target directory cannot be determined.
func TestProvision_TargetDirectoryError(t *testing.T) {
	ctx := context.Background()

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "", // Empty base path should cause error.
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/cloudposse/terraform-aws-vpc",
			},
		},
		Force: false,
	}

	err := Provision(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}

// TestProvision_AlreadyExists tests that Provision skips when target already exists and force=false.
func TestProvision_AlreadyExists(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with content.
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "main.tf"), []byte("# existing"), 0o644)
	require.NoError(t, err)

	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/cloudposse/terraform-aws-vpc",
			},
		},
		Force: false, // Not forcing re-vendor.
	}

	// Should not error - just skip.
	err = Provision(ctx, params)
	assert.NoError(t, err, "Provision should skip when target exists and force=false")

	// Verify the existing file was not modified.
	content, err := os.ReadFile(filepath.Join(targetDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# existing", string(content), "Existing file should not be modified")
}

// TestProvision_ForceOverwritesExisting tests that Provision re-vendors when Force=true.
func TestProvision_ForceOverwritesExisting(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with existing content.
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "main.tf"), []byte("# old content"), 0o644)
	require.NoError(t, err)

	// Use a URI that will definitely fail to download (non-existent repo).
	params := &ProvisionParams{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/cloudposse/nonexistent-repo-that-does-not-exist-12345",
			},
		},
		Force: true, // Force re-vendor even if target exists.
	}

	// Provision will attempt to download (which will fail because repo doesn't exist).
	// The key validation is that it doesn't skip due to existing directory.
	err = Provision(ctx, params)

	// We expect an error because the repo doesn't exist.
	// But the error should be a download error, not a "skipped" situation.
	// This confirms Force=true triggers the download path instead of skipping.
	require.Error(t, err, "Expected error from download attempt, not skip")
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision, "Error should be from provisioning attempt")
}

// TestDetermineTargetDirectory_WorkdirEnabled tests workdir path when provision.workdir.enabled is true.
func TestDetermineTargetDirectory_WorkdirEnabled(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	result, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", componentConfig)
	require.NoError(t, err)
	// Expecting: <tempDir>/.workdir/terraform/dev-vpc/
	expected := filepath.Join(tempDir, workdir.WorkdirPath, "terraform", "dev-vpc-bb03116d")
	assert.Equal(t, expected, result)
}

// TestDetermineTargetDirectory_WorkdirUsesAtmosComponent tests that workdir path uses
// atmos_component (instance name) instead of the passed component (base name) when available.
// This ensures JIT vendoring and source pull use the same workdir path as terraform plan/init.
func TestDetermineTargetDirectory_WorkdirUsesAtmosComponent(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"atmos_stack":     "demo-dev",
		"atmos_component": "demo-cluster-codepipeline-iac",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	// Pass the base component name, but expect the workdir to use atmos_component (instance name).
	result, err := DetermineTargetDirectory(atmosConfig, "terraform", "demo-cluster-codepipeline", componentConfig)
	require.NoError(t, err)
	expected := filepath.Join(tempDir, workdir.WorkdirPath, "terraform", "demo-dev-demo-cluster-codepipeline-iac-9d3a9da4")
	assert.Equal(t, expected, result)
}

// TestDetermineTargetDirectory_WorkdirEnabledEmptyBasePath verifies buildWorkdirPath's default
// of "." when atmosConfig.BasePath is empty: the resulting workdir path must be relative
// (rooted at "."), not silently error out or resolve to an absolute path derived from an empty
// string. Exercises buildWorkdirPath's `if basePath == "" { basePath = "." }` branch.
func TestDetermineTargetDirectory_WorkdirEnabledEmptyBasePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		// BasePath intentionally left empty.
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	result, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", componentConfig)
	require.NoError(t, err)
	// workdir.BuildPath(".", ...) resolves relative to "." rather than erroring.
	expected := filepath.Join(workdir.WorkdirPath, "terraform", "dev-vpc-bb03116d")
	assert.Equal(t, expected, result)
}

// TestDetermineTargetDirectory_WorkdirEnabledNoStack tests workdir path error when stack is missing.
func TestDetermineTargetDirectory_WorkdirEnabledNoStack(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// workdir enabled but no atmos_stack in config.
	componentConfig := map[string]any{
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	_, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}

// TestGetResolvedAbsPath tests retrieval of pre-resolved absolute paths.
func TestGetResolvedAbsPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: "/abs/path/terraform",
		HelmfileDirAbsolutePath:  "/abs/path/helmfile",
		PackerDirAbsolutePath:    "/abs/path/packer",
	}

	assert.Equal(t, "/abs/path/terraform", getResolvedAbsPath(atmosConfig, "terraform"))
	assert.Equal(t, "/abs/path/helmfile", getResolvedAbsPath(atmosConfig, "helmfile"))
	assert.Equal(t, "/abs/path/packer", getResolvedAbsPath(atmosConfig, "packer"))
	assert.Equal(t, "", getResolvedAbsPath(atmosConfig, "unknown"))
}

// TestDetermineTargetDirectory_PreResolvedAbsPath tests using pre-resolved absolute paths.
func TestDetermineTargetDirectory_PreResolvedAbsPath(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "terraform")
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: absPath,
	}

	result, err := DetermineTargetDirectory(atmosConfig, "terraform", "vpc", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(absPath, "vpc"), result)
}

// TestBuildComponentPath_AbsolutePath tests building path when config base path is absolute.
func TestBuildComponentPath_AbsolutePath(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "components", "terraform")
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: absPath,
			},
		},
	}

	result, err := buildComponentPath(atmosConfig, "terraform")
	require.NoError(t, err)
	assert.Equal(t, absPath, result)
}

// TestBuildComponentPath_RelativePathWithBasePath tests building path with relative config and base path.
func TestBuildComponentPath_RelativePathWithBasePath(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	result, err := buildComponentPath(atmosConfig, "terraform")
	require.NoError(t, err)
	expected := filepath.Join(tempDir, "components", "terraform")
	assert.Equal(t, expected, result)
}

// TestBuildComponentPath_RelativePathNoBasePath tests building path with relative config and no base path.
func TestBuildComponentPath_RelativePathNoBasePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	result, err := buildComponentPath(atmosConfig, "terraform")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "components", "terraform"), result)
}
