package source

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        string
	}{
		{
			name: "component field present",
			componentConfig: map[string]any{
				"component": "vpc",
			},
			expected: "vpc",
		},
		{
			name: "metadata.component present",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "s3-bucket",
				},
			},
			expected: "s3-bucket",
		},
		{
			name: "component field takes priority over metadata",
			componentConfig: map[string]any{
				"component": "vpc",
				"metadata": map[string]any{
					"component": "s3-bucket",
				},
			},
			expected: "vpc",
		},
		{
			name:            "empty config returns empty string",
			componentConfig: map[string]any{},
			expected:        "",
		},
		{
			name:            "nil config returns empty string",
			componentConfig: nil,
			expected:        "",
		},
		{
			name: "empty component field returns empty string",
			componentConfig: map[string]any{
				"component": "",
			},
			expected: "",
		},
		{
			name: "metadata without component field",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"other": "value",
				},
			},
			expected: "",
		},
		{
			name: "metadata is not a map",
			componentConfig: map[string]any{
				"metadata": "not-a-map",
			},
			expected: "",
		},
		{
			name: "component is not a string",
			componentConfig: map[string]any{
				"component": 12345,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWorkdirEnabled(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        bool
	}{
		{
			name: "workdir enabled",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "workdir disabled",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name:            "no provision section",
			componentConfig: map[string]any{},
			expected:        false,
		},
		{
			name: "no workdir section",
			componentConfig: map[string]any{
				"provision": map[string]any{},
			},
			expected: false,
		},
		{
			name: "workdir without enabled field",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"other": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "provision is not a map",
			componentConfig: map[string]any{
				"provision": "not-a-map",
			},
			expected: false,
		},
		{
			name: "workdir is not a map",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": "not-a-map",
				},
			},
			expected: false,
		},
		{
			name: "enabled is not a bool",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name:            "nil config",
			componentConfig: nil,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := workdir.IsWorkdirEnabled(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetermineSourceTargetDirectory_WorkdirUsesAtmosComponent tests that the JIT source
// provisioner uses atmos_component (instance name) for the workdir path, not the base component.
// This prevents workdir mismatch when metadata.component differs from the instance name.
func TestDetermineSourceTargetDirectory_WorkdirUsesAtmosComponent(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
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

	// Pass base component name, expect workdir to use atmos_component (instance name).
	targetDir, isWorkdir, err := determineSourceTargetDirectory(
		atmosConfig, "terraform", "demo-cluster-codepipeline", componentConfig,
	)
	require.NoError(t, err)
	assert.True(t, isWorkdir)
	expected := filepath.Join(tempDir, workdir.WorkdirPath, "terraform", "demo-dev-demo-cluster-codepipeline-iac")
	assert.Equal(t, expected, targetDir)
}

// TestDetermineSourceTargetDirectory_WorkdirFallsBackToComponent tests that when
// atmos_component is not set, the workdir path falls back to the passed component name.
func TestDetermineSourceTargetDirectory_WorkdirFallsBackToComponent(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	componentConfig := map[string]any{
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	targetDir, isWorkdir, err := determineSourceTargetDirectory(
		atmosConfig, "terraform", "vpc", componentConfig,
	)
	require.NoError(t, err)
	assert.True(t, isWorkdir)
	expected := filepath.Join(tempDir, workdir.WorkdirPath, "terraform", "dev-vpc")
	assert.Equal(t, expected, targetDir)
}

func TestNeedsProvisioning(t *testing.T) {
	sourceSpec := &schema.VendorComponentSource{
		Uri:     "github.com/test/repo//src",
		Version: "1.0.0",
	}

	tests := []struct {
		name           string
		setup          func(t *testing.T) string // Returns path to test.
		sourceSpec     *schema.VendorComponentSource
		expected       bool
		expectedReason string
	}{
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				return filepath.Join(tempDir, "nonexistent")
			},
			sourceSpec:     sourceSpec,
			expected:       true,
			expectedReason: "",
		},
		{
			name: "path is a file not directory",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "file.txt")
				err := os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
			sourceSpec:     sourceSpec,
			expected:       true,
			expectedReason: "",
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				emptyDir := filepath.Join(tempDir, "empty")
				err := os.MkdirAll(emptyDir, 0o755)
				require.NoError(t, err)
				return emptyDir
			},
			sourceSpec:     sourceSpec,
			expected:       true,
			expectedReason: "",
		},
		{
			name: "directory with files but no metadata",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				dirPath := filepath.Join(tempDir, "component")
				err := os.MkdirAll(dirPath, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644)
				require.NoError(t, err)
				return dirPath
			},
			sourceSpec:     sourceSpec,
			expected:       true,
			expectedReason: "No metadata found, re-provisioning",
		},
		{
			name: "directory with metadata and matching version",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				dirPath := filepath.Join(tempDir, "component")
				err := os.MkdirAll(dirPath, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644)
				require.NoError(t, err)
				// Write metadata with matching version.
				metadata := &workdir.WorkdirMetadata{
					SourceURI:     "github.com/test/repo//src",
					SourceVersion: "1.0.0",
				}
				err = workdir.WriteMetadata(dirPath, metadata)
				require.NoError(t, err)
				return dirPath
			},
			sourceSpec:     sourceSpec,
			expected:       false,
			expectedReason: "",
		},
		{
			name: "directory with metadata but version changed",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				dirPath := filepath.Join(tempDir, "component")
				err := os.MkdirAll(dirPath, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644)
				require.NoError(t, err)
				// Write metadata with old version.
				metadata := &workdir.WorkdirMetadata{
					SourceURI:     "github.com/test/repo//src",
					SourceVersion: "0.9.0",
				}
				err = workdir.WriteMetadata(dirPath, metadata)
				require.NoError(t, err)
				return dirPath
			},
			sourceSpec:     sourceSpec,
			expected:       true,
			expectedReason: "Source version changed (0.9.0 → 1.0.0)",
		},
		{
			name: "directory with metadata but URI changed",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				dirPath := filepath.Join(tempDir, "component")
				err := os.MkdirAll(dirPath, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644)
				require.NoError(t, err)
				// Write metadata with different URI.
				metadata := &workdir.WorkdirMetadata{
					SourceURI:     "github.com/other/repo//src",
					SourceVersion: "1.0.0",
				}
				err = workdir.WriteMetadata(dirPath, metadata)
				require.NoError(t, err)
				return dirPath
			},
			sourceSpec:     sourceSpec,
			expected:       true,
			expectedReason: "Source URI changed (github.com/other/repo//src → github.com/test/repo//src)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			// Test with isWorkdir=true since these tests verify metadata behavior.
			result, reason := needsProvisioning(path, tt.sourceSpec, true)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.expectedReason, reason)
		})
	}
}

// TestNeedsProvisioning_TTL verifies TTL-based cache expiration triggers re-provisioning.
func TestNeedsProvisioning_TTL(t *testing.T) {
	tests := []struct {
		name          string
		ttl           string
		updatedAt     time.Time
		expected      bool
		expectExpired bool
	}{
		{
			name:          "TTL zero always expires",
			ttl:           "0s",
			updatedAt:     time.Now(),
			expected:      true,
			expectExpired: true,
		},
		{
			name:          "TTL 1h with recent update does not expire",
			ttl:           "1h",
			updatedAt:     time.Now().Add(-30 * time.Minute),
			expected:      false,
			expectExpired: false,
		},
		{
			name:          "TTL 1h with old update expires",
			ttl:           "1h",
			updatedAt:     time.Now().Add(-2 * time.Hour),
			expected:      true,
			expectExpired: true,
		},
		{
			name:          "no TTL does not expire",
			ttl:           "",
			updatedAt:     time.Now().Add(-24 * 365 * time.Hour),
			expected:      false,
			expectExpired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			dirPath := filepath.Join(tempDir, "component")
			require.NoError(t, os.MkdirAll(dirPath, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644))

			// Write metadata with matching version/URI so only TTL matters.
			metadata := &workdir.WorkdirMetadata{
				SourceURI:     "github.com/test/repo//src",
				SourceVersion: "main",
				UpdatedAt:     tt.updatedAt,
			}
			require.NoError(t, workdir.WriteMetadata(dirPath, metadata))

			sourceSpec := &schema.VendorComponentSource{
				Uri:     "github.com/test/repo//src",
				Version: "main",
				TTL:     tt.ttl,
			}

			result, reason := needsProvisioning(dirPath, sourceSpec, true)
			assert.Equal(t, tt.expected, result)
			if tt.expectExpired {
				assert.Contains(t, reason, "Source cache expired")
			}
		})
	}
}

func TestDetermineSourceTargetDirectory(t *testing.T) {
	tests := []struct {
		name            string
		atmosConfig     *schema.AtmosConfiguration
		componentType   string
		component       string
		componentConfig map[string]any
		expectedDir     string
		expectedWorkdir bool
		expectError     bool
	}{
		{
			name: "standard terraform component path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "/base/components/terraform/vpc",
			expectedWorkdir: false,
			expectError:     false,
		},
		{
			name: "workdir enabled with stack",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"atmos_stack": "dev-us-east-1",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedDir:     "/base/.workdir/terraform/dev-us-east-1-vpc",
			expectedWorkdir: true,
			expectError:     false,
		},
		{
			name: "workdir enabled without stack returns error",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedDir:     "",
			expectedWorkdir: false,
			expectError:     true,
		},
		{
			name: "empty base path defaults to current dir",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedDir:     ".workdir/terraform/dev-vpc",
			expectedWorkdir: true,
			expectError:     false,
		},
		{
			name: "helmfile component type",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			componentType:   "helmfile",
			component:       "nginx",
			componentConfig: map[string]any{},
			expectedDir:     "/base/components/helmfile/nginx",
			expectedWorkdir: false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, isWorkdir, err := determineSourceTargetDirectory(tt.atmosConfig, tt.componentType, tt.component, tt.componentConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, filepath.FromSlash(tt.expectedDir), dir)
				assert.Equal(t, tt.expectedWorkdir, isWorkdir)
			}
		})
	}
}

func TestExtractSourceAndComponent(t *testing.T) {
	tests := []struct {
		name              string
		componentConfig   map[string]any
		expectedSource    bool
		expectedComponent string
		expectError       bool
	}{
		{
			name: "valid source and component",
			componentConfig: map[string]any{
				"component": "vpc",
				"source": map[string]any{
					"uri":     "github.com/cloudposse/terraform-aws-vpc",
					"version": "1.0.0",
				},
			},
			expectedSource:    true,
			expectedComponent: "vpc",
			expectError:       false,
		},
		{
			name: "no source returns nil without error",
			componentConfig: map[string]any{
				"component": "vpc",
			},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       false,
		},
		{
			name: "source but no component returns error",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri": "github.com/cloudposse/terraform-aws-vpc",
				},
			},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       true,
		},
		{
			name: "invalid source type returns error",
			componentConfig: map[string]any{
				"component": "vpc",
				"source":    12345,
			},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       true,
		},
		{
			name:              "empty config",
			componentConfig:   map[string]any{},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, component, err := extractSourceAndComponent(tt.componentConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectedSource {
					assert.NotNil(t, source)
					assert.Equal(t, tt.expectedComponent, component)
				} else {
					assert.Nil(t, source)
				}
			}
		})
	}
}

func TestWrapProvisionError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		explanation string
		component   string
	}{
		{
			name:        "basic error wrapping",
			err:         assert.AnError,
			explanation: "Failed to provision",
			component:   "vpc",
		},
		{
			name:        "nil error",
			err:         nil,
			explanation: "No underlying error",
			component:   "test-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapProvisionError(tt.err, tt.explanation, tt.component)
			require.Error(t, result)
			// Verify error is of expected type.
			assert.ErrorIs(t, result, errUtils.ErrSourceProvision)
			// Note: Explanation and context are stored in ErrorBuilder enrichments
			// but not included in the .Error() string representation.
		})
	}
}

func TestIsLocalSource(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		// Local sources.
		{
			name:     "relative path with dot",
			uri:      "./components/terraform/vpc",
			expected: true,
		},
		{
			name:     "parent directory path",
			uri:      "../demo-library/weather",
			expected: true,
		},
		{
			name:     "absolute path",
			uri:      "/home/user/components/vpc",
			expected: true,
		},
		{
			name:     "file scheme",
			uri:      "file:///path/to/component",
			expected: true,
		},
		{
			name:     "simple relative path",
			uri:      "components/terraform/vpc",
			expected: true,
		},
		// Remote sources.
		{
			name:     "github.com URL",
			uri:      "github.com/cloudposse/terraform-aws-vpc//src",
			expected: false,
		},
		{
			name:     "gitlab.com URL",
			uri:      "gitlab.com/org/repo//module",
			expected: false,
		},
		{
			name:     "bitbucket URL",
			uri:      "bitbucket.org/org/repo//module",
			expected: false,
		},
		{
			name:     "https URL",
			uri:      "https://example.com/path/to/module",
			expected: false,
		},
		{
			name:     "git:: prefix",
			uri:      "git::https://github.com/org/repo.git",
			expected: false,
		},
		{
			name:     "s3:: prefix",
			uri:      "s3::s3://bucket/path/to/module",
			expected: false,
		},
		{
			name:     "gcs:: prefix",
			uri:      "gcs::gs://bucket/path/to/module",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLocalSource(tt.uri)
			assert.Equal(t, tt.expected, result, "isLocalSource(%q) should return %v", tt.uri, tt.expected)
		})
	}
}

// Tests for checkMetadataChanges with various version scenarios.

func TestCheckMetadataChanges(t *testing.T) {
	tests := []struct {
		name           string
		metadata       *workdir.WorkdirMetadata
		sourceSpec     *schema.VendorComponentSource
		expected       bool
		expectedReason string
	}{
		{
			name: "no changes - same version and URI",
			metadata: &workdir.WorkdirMetadata{
				SourceURI:     "github.com/test/repo",
				SourceVersion: "1.0.0",
			},
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/test/repo",
				Version: "1.0.0",
			},
			expected:       false,
			expectedReason: "",
		},
		{
			name: "version changed",
			metadata: &workdir.WorkdirMetadata{
				SourceURI:     "github.com/test/repo",
				SourceVersion: "1.0.0",
			},
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/test/repo",
				Version: "2.0.0",
			},
			expected:       true,
			expectedReason: "Source version changed (1.0.0 → 2.0.0)",
		},
		{
			name: "version added",
			metadata: &workdir.WorkdirMetadata{
				SourceURI:     "github.com/test/repo",
				SourceVersion: "",
			},
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/test/repo",
				Version: "1.0.0",
			},
			expected:       true,
			expectedReason: "Source version changed ((none) → 1.0.0)",
		},
		{
			name: "version removed",
			metadata: &workdir.WorkdirMetadata{
				SourceURI:     "github.com/test/repo",
				SourceVersion: "1.0.0",
			},
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/test/repo",
				Version: "",
			},
			expected:       true,
			expectedReason: "Source version changed (1.0.0 → (none))",
		},
		{
			name: "URI changed",
			metadata: &workdir.WorkdirMetadata{
				SourceURI:     "github.com/old/repo",
				SourceVersion: "1.0.0",
			},
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/new/repo",
				Version: "1.0.0",
			},
			expected:       true,
			expectedReason: "Source URI changed (github.com/old/repo → github.com/new/repo)",
		},
		{
			name: "both empty versions",
			metadata: &workdir.WorkdirMetadata{
				SourceURI:     "github.com/test/repo",
				SourceVersion: "",
			},
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/test/repo",
				Version: "",
			},
			expected:       false,
			expectedReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, reason := checkMetadataChanges(tt.metadata, tt.sourceSpec)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.expectedReason, reason)
		})
	}
}

// Tests for isNonEmptyDir.

func TestIsNonEmptyDir(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "non-existent path",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expected: false,
		},
		{
			name: "path is a file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "file.txt")
				require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))
				return filePath
			},
			expected: false,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				emptyDir := filepath.Join(tmpDir, "empty")
				require.NoError(t, os.MkdirAll(emptyDir, 0o755))
				return emptyDir
			},
			expected: false,
		},
		{
			name: "directory with only .atmos",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				dir := filepath.Join(tmpDir, "only-atmos")
				require.NoError(t, os.MkdirAll(filepath.Join(dir, workdir.AtmosDir), 0o755))
				return dir
			},
			expected: false,
		},
		{
			name: "directory with files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				dir := filepath.Join(tmpDir, "with-files")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# test"), 0o644))
				return dir
			},
			expected: true,
		},
		{
			name: "directory with .atmos and other files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				dir := filepath.Join(tmpDir, "mixed")
				require.NoError(t, os.MkdirAll(filepath.Join(dir, workdir.AtmosDir), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# test"), 0o644))
				return dir
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			result := isNonEmptyDir(path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test needsProvisioning for non-workdir targets.

func TestNeedsProvisioning_NonWorkdir(t *testing.T) {
	sourceSpec := &schema.VendorComponentSource{
		Uri:     "github.com/test/repo//src",
		Version: "1.0.0",
	}

	// Create directory with content.
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "component")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644))

	// For non-workdir targets, existence is sufficient - no metadata check.
	result, reason := needsProvisioning(dirPath, sourceSpec, false)
	assert.False(t, result, "existing non-workdir should not need provisioning")
	assert.Empty(t, reason)
}

// Test writeWorkdirMetadata.

func TestWriteWorkdirMetadata(t *testing.T) {
	tests := []struct {
		name              string
		uri               string
		version           string
		existingMetadata  bool
		expectedType      workdir.SourceType
		preserveCreatedAt bool
	}{
		{
			name:         "remote source",
			uri:          "github.com/cloudposse/terraform-aws-vpc//src",
			version:      "1.0.0",
			expectedType: workdir.SourceTypeRemote,
		},
		{
			name:         "local source",
			uri:          "./components/terraform/vpc",
			version:      "",
			expectedType: workdir.SourceTypeLocal,
		},
		{
			name:              "preserves CreatedAt from existing metadata",
			uri:               "github.com/test/repo",
			version:           "2.0.0",
			existingMetadata:  true,
			expectedType:      workdir.SourceTypeRemote,
			preserveCreatedAt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			workdirPath := filepath.Join(tmpDir, "workdir")
			require.NoError(t, os.MkdirAll(workdirPath, 0o755))

			var originalCreatedAt time.Time
			if tt.existingMetadata {
				originalCreatedAt = time.Now().Add(-24 * time.Hour).Truncate(time.Second)
				existing := &workdir.WorkdirMetadata{
					Component: "existing",
					CreatedAt: originalCreatedAt,
				}
				require.NoError(t, workdir.WriteMetadata(workdirPath, existing))
			}

			sourceSpec := &schema.VendorComponentSource{
				Uri:     tt.uri,
				Version: tt.version,
			}

			err := writeWorkdirMetadata(workdirPath, "test-component", "test-stack", sourceSpec)
			require.NoError(t, err)

			// Read and verify.
			metadata, err := workdir.ReadMetadata(workdirPath)
			require.NoError(t, err)
			require.NotNil(t, metadata)

			assert.Equal(t, "test-component", metadata.Component)
			assert.Equal(t, "test-stack", metadata.Stack)
			assert.Equal(t, tt.expectedType, metadata.SourceType)
			assert.Equal(t, tt.uri, metadata.SourceURI)
			assert.Equal(t, tt.version, metadata.SourceVersion)

			if tt.preserveCreatedAt {
				assert.True(t, originalCreatedAt.Equal(metadata.CreatedAt),
					"CreatedAt should be preserved from existing metadata")
			}
		})
	}
}

// Test writeWorkdirMetadata preserves ContentHash for local sources.

// TestIsSourceCacheExpired tests the TTL cache expiration logic directly.
func TestIsSourceCacheExpired(t *testing.T) {
	tests := []struct {
		name           string
		ttl            string
		updatedAt      time.Time
		expected       bool
		expectedReason string
	}{
		{
			name:           "zero TTL (0s) always expired",
			ttl:            "0s",
			updatedAt:      time.Now(),
			expected:       true,
			expectedReason: "Source cache expired (TTL: 0s, always re-pull)",
		},
		{
			name:           "zero TTL (0) always expired",
			ttl:            "0",
			updatedAt:      time.Now(),
			expected:       true,
			expectedReason: "Source cache expired (TTL: 0, always re-pull)",
		},
		{
			name:           "zero TTL (0m) always expired",
			ttl:            "0m",
			updatedAt:      time.Now(),
			expected:       true,
			expectedReason: "Source cache expired (TTL: 0m, always re-pull)",
		},
		{
			name:           "zero TTL (0h) always expired",
			ttl:            "0h",
			updatedAt:      time.Now(),
			expected:       true,
			expectedReason: "Source cache expired (TTL: 0h, always re-pull)",
		},
		{
			name:           "zero TTL (0d) always expired",
			ttl:            "0d",
			updatedAt:      time.Now(),
			expected:       true,
			expectedReason: "Source cache expired (TTL: 0d, always re-pull)",
		},
		{
			name:      "1h TTL with recent update not expired",
			ttl:       "1h",
			updatedAt: time.Now().Add(-30 * time.Minute),
			expected:  false,
		},
		{
			name:           "1h TTL with old update expired",
			ttl:            "1h",
			updatedAt:      time.Now().Add(-2 * time.Hour),
			expected:       true,
			expectedReason: "Source cache expired",
		},
		{
			name:           "7d TTL with old update expired",
			ttl:            "7d",
			updatedAt:      time.Now().Add(-8 * 24 * time.Hour),
			expected:       true,
			expectedReason: "Source cache expired",
		},
		{
			name:      "7d TTL with recent update not expired",
			ttl:       "7d",
			updatedAt: time.Now().Add(-3 * 24 * time.Hour),
			expected:  false,
		},
		{
			name:           "invalid TTL forces re-provision",
			ttl:            "not-a-duration",
			updatedAt:      time.Now(),
			expected:       true,
			expectedReason: "Invalid source TTL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expired, reason := isSourceCacheExpired(tt.ttl, tt.updatedAt)
			assert.Equal(t, tt.expected, expired)
			if tt.expectedReason != "" {
				assert.Contains(t, reason, tt.expectedReason)
			}
		})
	}
}

// TestIsZeroTTL tests zero TTL detection.
func TestIsZeroTTL(t *testing.T) {
	tests := []struct {
		ttl      string
		expected bool
	}{
		{"0", true},
		{"0s", true},
		{"0m", true},
		{"0h", true},
		{"0d", true},
		{"1s", false},
		{"1h", false},
		{"", false},
		{"daily", false},
		{"0x", false},
	}

	for _, tt := range tests {
		t.Run(tt.ttl, func(t *testing.T) {
			assert.Equal(t, tt.expected, isZeroTTL(tt.ttl))
		})
	}
}

// TestNeedsProvisioning_TTLWithInvalidValue verifies that invalid TTL forces re-provisioning.
func TestNeedsProvisioning_TTLWithInvalidValue(t *testing.T) {
	tempDir := t.TempDir()
	dirPath := filepath.Join(tempDir, "component")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "main.tf"), []byte("# test"), 0o644))

	// Write metadata with matching version/URI.
	metadata := &workdir.WorkdirMetadata{
		SourceURI:     "github.com/test/repo//src",
		SourceVersion: "main",
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, workdir.WriteMetadata(dirPath, metadata))

	sourceSpec := &schema.VendorComponentSource{
		Uri:     "github.com/test/repo//src",
		Version: "main",
		TTL:     "not-a-valid-duration",
	}

	result, reason := needsProvisioning(dirPath, sourceSpec, true)
	assert.True(t, result, "invalid TTL should force re-provisioning")
	assert.Contains(t, reason, "Invalid source TTL")
}

// TestExtractSource_TTLVariousFormats verifies various valid TTL format strings are accepted.
func TestExtractSource_TTLVariousFormats(t *testing.T) {
	formats := []string{"0s", "0", "30s", "5m", "1h", "7d", "30m"}

	for _, ttl := range formats {
		t.Run(ttl, func(t *testing.T) {
			componentConfig := map[string]any{
				"source": map[string]any{
					"uri":     "github.com/example/repo//module",
					"version": "main",
					"ttl":     ttl,
				},
			}
			result, err := ExtractSource(componentConfig)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, ttl, result.TTL)
		})
	}
}

// TestApplyGlobalTTLDefault verifies global TTL default is applied per component type.
func TestApplyGlobalTTLDefault(t *testing.T) {
	tests := []struct {
		name          string
		sourceTTL     string
		componentType string
		atmosConfig   *schema.AtmosConfiguration
		expectedTTL   string
	}{
		{
			name:          "terraform global TTL applied when source TTL empty",
			sourceTTL:     "",
			componentType: "terraform",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Source: &schema.SourceSettings{TTL: "1h"},
					},
				},
			},
			expectedTTL: "1h",
		},
		{
			name:          "helmfile global TTL applied when source TTL empty",
			sourceTTL:     "",
			componentType: "helmfile",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						Source: &schema.SourceSettings{TTL: "30m"},
					},
				},
			},
			expectedTTL: "30m",
		},
		{
			name:          "packer global TTL applied when source TTL empty",
			sourceTTL:     "",
			componentType: "packer",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						Source: &schema.SourceSettings{TTL: "7d"},
					},
				},
			},
			expectedTTL: "7d",
		},
		{
			name:          "per-component TTL takes precedence over global",
			sourceTTL:     "0s",
			componentType: "terraform",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Source: &schema.SourceSettings{TTL: "1h"},
					},
				},
			},
			expectedTTL: "0s",
		},
		{
			name:          "no global TTL configured leaves source TTL empty",
			sourceTTL:     "",
			componentType: "terraform",
			atmosConfig:   &schema.AtmosConfiguration{},
			expectedTTL:   "",
		},
		{
			name:          "nil source settings leaves source TTL empty",
			sourceTTL:     "",
			componentType: "terraform",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Source: nil,
					},
				},
			},
			expectedTTL: "",
		},
		{
			name:          "unknown component type leaves source TTL empty",
			sourceTTL:     "",
			componentType: "unknown",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Source: &schema.SourceSettings{TTL: "1h"},
					},
				},
			},
			expectedTTL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceSpec := &schema.VendorComponentSource{
				Uri:     "github.com/example/repo//module",
				Version: "1.0.0",
				TTL:     tt.sourceTTL,
			}
			applyGlobalTTLDefault(sourceSpec, tt.atmosConfig, tt.componentType)
			assert.Equal(t, tt.expectedTTL, sourceSpec.TTL)
		})
	}
}

func TestWriteWorkdirMetadata_PreservesContentHash(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create existing metadata with ContentHash for local source.
	existing := &workdir.WorkdirMetadata{
		Component:   "vpc",
		SourceType:  workdir.SourceTypeLocal,
		ContentHash: "abc123hash",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
	}
	require.NoError(t, workdir.WriteMetadata(workdirPath, existing))

	// Write new metadata for local source.
	sourceSpec := &schema.VendorComponentSource{
		Uri:     "./components/terraform/vpc",
		Version: "",
	}

	err := writeWorkdirMetadata(workdirPath, "vpc", "dev", sourceSpec)
	require.NoError(t, err)

	// Read and verify ContentHash is preserved.
	metadata, err := workdir.ReadMetadata(workdirPath)
	require.NoError(t, err)
	assert.Equal(t, "abc123hash", metadata.ContentHash,
		"ContentHash should be preserved for local sources")
}

// TestAutoProvisionSource_InvocationGuard_PreventsDoubleProvisioning verifies that once
// AutoProvisionSource completes within a command invocation (marked via invocationDoneKey),
// a second call with the same componentConfig is a no-op even with ttl:"0s".
//
// This is the regression test for the JIT TTL bug: without the guard, a zero-TTL would
// cause AutoProvisionSource to delete the workdir on its second call (via the
// before.terraform.init hook), wiping out varfiles and backend configs written between
// the two calls and causing the subprocess to fail with "file does not exist" errors.
func TestAutoProvisionSource_InvocationGuard_PreventsDoubleProvisioning(t *testing.T) {
	// The guard is checked before any provisioning when invocationDoneKey is set.
	// A component config with a valid source but the done-key present should return nil
	// immediately without touching the filesystem.
	componentConfig := map[string]any{
		"component":   "null-label",
		"atmos_stack": "demo",
		"source": map[string]any{
			"uri":     "github.com/cloudposse/terraform-null-label.git//",
			"version": "0.25.0",
			"ttl":     "0s",
		},
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
		// Simulate that this invocation already ran AutoProvisionSource once.
		invocationDoneKey: struct{}{},
	}

	// Use a real atmosConfig with base path pointing to a temp dir.
	// The function should return nil without ever trying to resolve a target dir
	// or call VendorSource (which would require network access).
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	ctx := t.Context()
	err := AutoProvisionSource(ctx, atmosConfig, "terraform", componentConfig, nil)
	require.NoError(t, err, "second AutoProvisionSource call with invocationDoneKey set should be a no-op")
}

// TestAutoProvisionSource_InvocationGuard_SetAfterProvisioning verifies that the
// invocationDoneKey is written into componentConfig when provisioning is skipped
// (cache still valid). This ensures the before.terraform.init hook will be a no-op.
func TestAutoProvisionSource_InvocationGuard_SetAfterProvisioning(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workdir with valid metadata so needsProvisioning returns false
	// (TTL not expired, version unchanged).
	workdirPath := filepath.Join(tmpDir, ".workdir", "terraform", "demo-null-label")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))
	// Place a dummy file so isNonEmptyDir returns true.
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# test"), 0o644))
	// Write metadata matching the source spec so TTL check is the only gate.
	meta := &workdir.WorkdirMetadata{
		SourceURI:     "github.com/cloudposse/terraform-null-label.git//",
		SourceVersion: "0.25.0",
		UpdatedAt:     time.Now(), // Fresh — 1h TTL not expired.
	}
	require.NoError(t, workdir.WriteMetadata(workdirPath, meta))

	componentConfig := map[string]any{
		"component":   "null-label",
		"atmos_stack": "demo",
		"source": map[string]any{
			"uri":     "github.com/cloudposse/terraform-null-label.git//",
			"version": "0.25.0",
			"ttl":     "1h",
		},
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	ctx := t.Context()
	err := AutoProvisionSource(ctx, atmosConfig, "terraform", componentConfig, nil)
	require.NoError(t, err)

	// The guard marker must now be present in componentConfig.
	_, done := componentConfig[invocationDoneKey]
	assert.True(t, done, "invocationDoneKey should be set in componentConfig after a skipped provision")
}
