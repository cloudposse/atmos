package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name       string
		component  string
		stack      string
		tags       []string
		everything bool
		expectErr  error
	}{
		{
			name:       "no flags - valid",
			component:  "",
			stack:      "",
			tags:       nil,
			everything: false,
			expectErr:  nil,
		},
		{
			name:       "component only - valid",
			component:  "vpc",
			stack:      "",
			tags:       nil,
			everything: false,
			expectErr:  nil,
		},
		{
			name:       "stack only - valid",
			component:  "",
			stack:      "dev-us-east-1",
			tags:       nil,
			everything: false,
			expectErr:  nil,
		},
		{
			name:       "tags only - valid",
			component:  "",
			stack:      "",
			tags:       []string{"networking"},
			everything: false,
			expectErr:  nil,
		},
		{
			name:       "stack and tags - valid",
			component:  "",
			stack:      "dev-us-east-1",
			tags:       []string{"networking"},
			everything: false,
			expectErr:  nil,
		},
		{
			name:       "component and stack - invalid",
			component:  "vpc",
			stack:      "dev-us-east-1",
			tags:       nil,
			everything: false,
			expectErr:  ErrValidateComponentStackFlag,
		},
		{
			name:       "component and tags - invalid",
			component:  "vpc",
			stack:      "",
			tags:       []string{"networking"},
			everything: false,
			expectErr:  ErrValidateComponentFlag,
		},
		{
			name:       "everything with component - invalid",
			component:  "vpc",
			stack:      "",
			tags:       nil,
			everything: true,
			expectErr:  ErrValidateEverythingFlag,
		},
		{
			name:       "everything with stack - invalid",
			component:  "",
			stack:      "dev-us-east-1",
			tags:       nil,
			everything: true,
			expectErr:  ErrValidateEverythingFlag,
		},
		{
			name:       "everything with tags - invalid",
			component:  "",
			stack:      "",
			tags:       []string{"networking"},
			everything: true,
			expectErr:  ErrValidateEverythingFlag,
		},
		{
			name:       "everything alone - valid",
			component:  "",
			stack:      "",
			tags:       nil,
			everything: true,
			expectErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFlags(tt.component, tt.stack, tt.tags, tt.everything)
			if tt.expectErr != nil {
				assert.ErrorIs(t, err, tt.expectErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestShouldSetEverythingDefault(t *testing.T) {
	tests := []struct {
		name      string
		component string
		stack     string
		tags      []string
		expected  bool
	}{
		{
			name:      "no flags - should default to true",
			component: "",
			stack:     "",
			tags:      nil,
			expected:  true,
		},
		{
			name:      "empty tags - should default to true",
			component: "",
			stack:     "",
			tags:      []string{},
			expected:  true,
		},
		{
			name:      "component set - should not default",
			component: "vpc",
			stack:     "",
			tags:      nil,
			expected:  false,
		},
		{
			name:      "stack set - should not default",
			component: "",
			stack:     "dev-us-east-1",
			tags:      nil,
			expected:  false,
		},
		{
			name:      "tags set - should not default",
			component: "",
			stack:     "",
			tags:      []string{"networking"},
			expected:  false,
		},
		{
			name:      "all set - should not default",
			component: "vpc",
			stack:     "dev-us-east-1",
			tags:      []string{"networking"},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldSetEverythingDefault(tt.component, tt.stack, tt.tags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDiff(t *testing.T) {
	err := Diff()
	assert.ErrorIs(t, err, ErrExecuteVendorDiffCmd)
}

func TestFilterByTags(t *testing.T) {
	sources := []schema.AtmosVendorSource{
		{Component: "vpc", Tags: []string{"networking", "core"}},
		{Component: "ecs", Tags: []string{"compute", "core"}},
		{Component: "rds", Tags: []string{"database"}},
		{Component: "s3", Tags: []string{}},
	}

	tests := []struct {
		name          string
		sources       []schema.AtmosVendorSource
		tags          []string
		expectedCount int
		expectedComps []string
	}{
		{
			name:          "no tags returns all sources",
			sources:       sources,
			tags:          nil,
			expectedCount: 4,
			expectedComps: []string{"vpc", "ecs", "rds", "s3"},
		},
		{
			name:          "empty tags returns all sources",
			sources:       sources,
			tags:          []string{},
			expectedCount: 4,
			expectedComps: []string{"vpc", "ecs", "rds", "s3"},
		},
		{
			name:          "single tag filters correctly",
			sources:       sources,
			tags:          []string{"networking"},
			expectedCount: 1,
			expectedComps: []string{"vpc"},
		},
		{
			name:          "multiple tags filters with OR logic",
			sources:       sources,
			tags:          []string{"networking", "database"},
			expectedCount: 2,
			expectedComps: []string{"vpc", "rds"},
		},
		{
			name:          "shared tag returns multiple",
			sources:       sources,
			tags:          []string{"core"},
			expectedCount: 2,
			expectedComps: []string{"vpc", "ecs"},
		},
		{
			name:          "non-existent tag returns empty",
			sources:       sources,
			tags:          []string{"nonexistent"},
			expectedCount: 0,
			expectedComps: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterByTags(tt.sources, tt.tags)
			assert.Len(t, result, tt.expectedCount)

			for i, comp := range tt.expectedComps {
				if i < len(result) {
					assert.Equal(t, comp, result[i].Component)
				}
			}
		})
	}
}

func TestDetermineSourceType(t *testing.T) {
	tests := []struct {
		name                 string
		uri                  string
		vendorConfigFilePath string
		expectedOCI          bool
		expectedLocalFS      bool
		expectedLocalFile    bool
		expectedURIAfter     string
		expectError          bool
	}{
		{
			name:                 "OCI scheme",
			uri:                  "oci://public.ecr.aws/cloudposse/components:latest",
			vendorConfigFilePath: "/tmp",
			expectedOCI:          true,
			expectedLocalFS:      false,
			expectedLocalFile:    false,
			expectedURIAfter:     "public.ecr.aws/cloudposse/components:latest",
		},
		{
			name:                 "remote git URL",
			uri:                  "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
			vendorConfigFilePath: "/tmp",
			expectedOCI:          false,
			expectedLocalFS:      false,
			expectedLocalFile:    false,
			expectedURIAfter:     "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
		},
		{
			name:                 "file scheme",
			uri:                  "file:///path/to/local",
			vendorConfigFilePath: "/tmp",
			expectedOCI:          false,
			expectedLocalFS:      true,
			expectedLocalFile:    false,
			expectedURIAfter:     "path/to/local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := tt.uri
			result, err := determineSourceType(&uri, tt.vendorConfigFilePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOCI, result.useOciScheme)
				assert.Equal(t, tt.expectedLocalFS, result.useLocalFileSystem)
				assert.Equal(t, tt.expectedLocalFile, result.sourceIsLocalFile)
				// Normalize path separators for cross-platform comparison.
				assert.Equal(t, tt.expectedURIAfter, filepath.ToSlash(uri))
			}
		})
	}
}

func TestDeterminePackageType(t *testing.T) {
	tests := []struct {
		name               string
		useOciScheme       bool
		useLocalFileSystem bool
		expected           pkgType
	}{
		{
			name:               "OCI scheme",
			useOciScheme:       true,
			useLocalFileSystem: false,
			expected:           pkgTypeOci,
		},
		{
			name:               "local file system",
			useOciScheme:       false,
			useLocalFileSystem: true,
			expected:           pkgTypeLocal,
		},
		{
			name:               "remote",
			useOciScheme:       false,
			useLocalFileSystem: false,
			expected:           pkgTypeRemote,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determinePackageType(tt.useOciScheme, tt.useLocalFileSystem)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessTargets(t *testing.T) {
	tests := []struct {
		name          string
		params        *processTargetsParams
		expectedCount int
		expectError   bool
	}{
		{
			name: "single target",
			params: &processTargetsParams{
				AtmosConfig: &schema.AtmosConfiguration{},
				IndexSource: 0,
				Source: &schema.AtmosVendorSource{
					Component: "vpc",
					Targets:   []string{"./components/terraform/vpc"},
				},
				TemplateData:         struct{ Component, Version string }{Component: "vpc", Version: "1.0.0"},
				VendorConfigFilePath: "/tmp",
				URI:                  "github.com/example/repo.git//vpc",
				PkgType:              pkgTypeRemote,
				SourceIsLocalFile:    false,
			},
			expectedCount: 1,
		},
		{
			name: "multiple targets",
			params: &processTargetsParams{
				AtmosConfig: &schema.AtmosConfiguration{},
				IndexSource: 0,
				Source: &schema.AtmosVendorSource{
					Component: "vpc",
					Targets:   []string{"./target1", "./target2"},
				},
				TemplateData:         struct{ Component, Version string }{Component: "vpc", Version: "1.0.0"},
				VendorConfigFilePath: "/tmp",
				URI:                  "github.com/example/repo.git//vpc",
				PkgType:              pkgTypeRemote,
				SourceIsLocalFile:    false,
			},
			expectedCount: 2,
		},
		{
			name: "no targets",
			params: &processTargetsParams{
				AtmosConfig: &schema.AtmosConfiguration{},
				IndexSource: 0,
				Source: &schema.AtmosVendorSource{
					Component: "vpc",
					Targets:   []string{},
				},
				TemplateData:         struct{ Component, Version string }{Component: "vpc", Version: "1.0.0"},
				VendorConfigFilePath: "/tmp",
				URI:                  "github.com/example/repo.git//vpc",
				PkgType:              pkgTypeRemote,
				SourceIsLocalFile:    false,
			},
			expectedCount: 0,
		},
		{
			name: "target with template",
			params: &processTargetsParams{
				AtmosConfig: &schema.AtmosConfiguration{},
				IndexSource: 0,
				Source: &schema.AtmosVendorSource{
					Component: "vpc",
					Targets:   []string{"./components/terraform/{{.Component}}"},
				},
				TemplateData:         struct{ Component, Version string }{Component: "vpc", Version: "1.0.0"},
				VendorConfigFilePath: "/tmp",
				URI:                  "github.com/example/repo.git//vpc",
				PkgType:              pkgTypeRemote,
				SourceIsLocalFile:    false,
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processTargets(tt.params)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
			}
		})
	}
}

func TestPkgTypeString(t *testing.T) {
	tests := []struct {
		name     string
		pkgType  pkgType
		expected string
	}{
		{
			name:     "remote type",
			pkgType:  pkgTypeRemote,
			expected: "remote",
		},
		{
			name:     "oci type",
			pkgType:  pkgTypeOci,
			expected: "oci",
		},
		{
			name:     "local type",
			pkgType:  pkgTypeLocal,
			expected: "local",
		},
		{
			name:     "unknown type - negative",
			pkgType:  pkgType(-1),
			expected: "unknown",
		},
		{
			name:     "unknown type - out of range",
			pkgType:  pkgType(100),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pkgType.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateRemoteURI(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		component string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid remote URI",
			uri:       "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
			component: "vpc",
			expectErr: false,
		},
		{
			name:      "valid HTTPS URI",
			uri:       "https://github.com/cloudposse/components.git//vpc",
			component: "vpc",
			expectErr: false,
		},
		{
			name:      "empty URI returns error",
			uri:       "",
			component: "vpc",
			expectErr: true,
			errMsg:    "invalid URI for component vpc",
		},
		{
			name:      "path traversal returns specific error",
			uri:       "../../../etc/passwd",
			component: "malicious",
			expectErr: true,
			errMsg:    "Please ensure the source is a valid local path",
		},
		{
			name:      "URI with spaces returns error",
			uri:       "github.com/some path/repo",
			component: "bad-component",
			expectErr: true,
			errMsg:    "invalid URI for component bad-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRemoteURI(tt.uri, tt.component)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteAtmosVendorInternal(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	tests := []struct {
		name        string
		params      *executeVendorOptions
		expectError error
	}{
		{
			name: "empty sources and imports returns error",
			params: &executeVendorOptions{
				atmosConfig:          atmosConfig,
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				atmosVendorSpec: schema.AtmosVendorSpec{
					Sources: []schema.AtmosVendorSource{},
					Imports: []string{},
				},
			},
			expectError: ErrMissingVendorConfigDefinition,
		},
		{
			name: "non-existent import file returns error",
			params: &executeVendorOptions{
				atmosConfig:          atmosConfig,
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				atmosVendorSpec: schema.AtmosVendorSpec{
					Sources: []schema.AtmosVendorSource{},
					Imports: []string{filepath.Join(tempDir, "non-existent-import.yaml")},
				},
			},
			expectError: nil, // Error occurs but not a specific sentinel.
		},
		{
			name: "non-existent component returns error",
			params: &executeVendorOptions{
				atmosConfig:          atmosConfig,
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				atmosVendorSpec: schema.AtmosVendorSpec{
					Sources: []schema.AtmosVendorSource{
						{
							Component: "vpc",
							Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
							Targets:   []string{"./components/terraform/vpc"},
						},
					},
				},
				component: "non-existent-component",
			},
			expectError: ErrComponentNotDefined,
		},
		{
			name: "non-matching tag returns error",
			params: &executeVendorOptions{
				atmosConfig:          atmosConfig,
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				atmosVendorSpec: schema.AtmosVendorSpec{
					Sources: []schema.AtmosVendorSource{
						{
							Component: "vpc",
							Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
							Targets:   []string{"./components/terraform/vpc"},
							Tags:      []string{"networking"},
						},
					},
				},
				tags: []string{"non-existent-tag"},
			},
			expectError: ErrNoComponentsWithTags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeAtmosVendorInternal(tt.params)
			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
			} else if tt.name == "non-existent import file returns error" {
				assert.Error(t, err)
			}
		})
	}
}

func TestProcessAtmosVendorSource(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	tests := []struct {
		name          string
		params        *vendorSourceParams
		expectedCount int
		expectError   bool
	}{
		{
			name: "single source with single target",
			params: &vendorSourceParams{
				atmosConfig: atmosConfig,
				sources: []schema.AtmosVendorSource{
					{
						Component: "vpc",
						Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
						Targets:   []string{"./components/terraform/vpc"},
					},
				},
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				vendorConfigFilePath: tempDir,
			},
			expectedCount: 1,
			expectError:   false,
		},
		{
			name: "source filtered by component",
			params: &vendorSourceParams{
				atmosConfig: atmosConfig,
				sources: []schema.AtmosVendorSource{
					{
						Component: "vpc",
						Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
						Targets:   []string{"./components/terraform/vpc"},
					},
					{
						Component: "rds",
						Source:    "github.com/cloudposse/terraform-aws-components.git//modules/rds?ref=1.0.0",
						Targets:   []string{"./components/terraform/rds"},
					},
				},
				component:            "vpc",
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				vendorConfigFilePath: tempDir,
			},
			expectedCount: 1, // Only vpc should be included.
			expectError:   false,
		},
		{
			name: "source filtered by tag",
			params: &vendorSourceParams{
				atmosConfig: atmosConfig,
				sources: []schema.AtmosVendorSource{
					{
						Component: "vpc",
						Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
						Targets:   []string{"./components/terraform/vpc"},
						Tags:      []string{"networking"},
					},
					{
						Component: "rds",
						Source:    "github.com/cloudposse/terraform-aws-components.git//modules/rds?ref=1.0.0",
						Targets:   []string{"./components/terraform/rds"},
						Tags:      []string{"database"},
					},
				},
				tags:                 []string{"networking"},
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				vendorConfigFilePath: tempDir,
			},
			expectedCount: 1, // Only vpc has networking tag.
			expectError:   false,
		},
		{
			name: "empty sources returns empty",
			params: &vendorSourceParams{
				atmosConfig:          atmosConfig,
				sources:              []schema.AtmosVendorSource{},
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				vendorConfigFilePath: tempDir,
			},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name: "source with missing targets returns error",
			params: &vendorSourceParams{
				atmosConfig: atmosConfig,
				sources: []schema.AtmosVendorSource{
					{
						Component: "vpc",
						Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
						Targets:   []string{}, // No targets.
					},
				},
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				vendorConfigFilePath: tempDir,
			},
			expectError: true,
		},
		{
			name: "source with missing source field returns error",
			params: &vendorSourceParams{
				atmosConfig: atmosConfig,
				sources: []schema.AtmosVendorSource{
					{
						Component: "vpc",
						Source:    "", // No source.
						Targets:   []string{"./components/terraform/vpc"},
					},
				},
				vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
				vendorConfigFilePath: tempDir,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processAtmosVendorSource(tt.params)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
			}
		})
	}
}

func TestDetermineSourceType_Extended(t *testing.T) {
	// Additional test cases for determineSourceType.
	tempDir := t.TempDir()

	// Create a local file for testing.
	localFile := tempDir + "/local.tf"
	err := os.WriteFile(localFile, []byte("# test"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name              string
		uri               string
		vendorConfigPath  string
		expectedOCI       bool
		expectedLocalFS   bool
		expectedLocalFile bool
	}{
		{
			name:              "OCI with registry path",
			uri:               "oci://ghcr.io/cloudposse/components:v1.0.0",
			vendorConfigPath:  "/tmp",
			expectedOCI:       true,
			expectedLocalFS:   false,
			expectedLocalFile: false,
		},
		{
			name:              "file scheme with path",
			uri:               "file:///var/components/terraform",
			vendorConfigPath:  "/tmp",
			expectedOCI:       false,
			expectedLocalFS:   true,
			expectedLocalFile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := tt.uri
			result, err := determineSourceType(&uri, tt.vendorConfigPath)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedOCI, result.useOciScheme)
			assert.Equal(t, tt.expectedLocalFS, result.useLocalFileSystem)
			assert.Equal(t, tt.expectedLocalFile, result.sourceIsLocalFile)
		})
	}
}

func TestDetermineSourceType_RelativeLocalPath(t *testing.T) {
	// Test relative local paths and existing file detection.
	tempDir := t.TempDir()

	// Create an actual local file.
	localFile := filepath.Join(tempDir, "local-component.tf")
	err := os.WriteFile(localFile, []byte("# local component"), 0o644)
	assert.NoError(t, err)

	// Create a local directory.
	localDir := filepath.Join(tempDir, "local-modules")
	err = os.MkdirAll(localDir, 0o755)
	assert.NoError(t, err)

	tests := []struct {
		name              string
		uri               string
		vendorConfigPath  string
		expectedLocalFS   bool
		expectedLocalFile bool
	}{
		{
			name:              "relative path to existing file",
			uri:               "local-component.tf",
			vendorConfigPath:  tempDir,
			expectedLocalFS:   true,
			expectedLocalFile: true,
		},
		{
			name:              "relative path to existing directory",
			uri:               "local-modules",
			vendorConfigPath:  tempDir,
			expectedLocalFS:   true,
			expectedLocalFile: false,
		},
		{
			name:              "dot-prefixed relative path",
			uri:               "./local-component.tf",
			vendorConfigPath:  tempDir,
			expectedLocalFS:   true,
			expectedLocalFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := tt.uri
			result, err := determineSourceType(&uri, tt.vendorConfigPath)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedLocalFS, result.useLocalFileSystem, "useLocalFileSystem mismatch")
			assert.Equal(t, tt.expectedLocalFile, result.sourceIsLocalFile, "sourceIsLocalFile mismatch")
		})
	}
}

func TestProcessTargets_TemplateError(t *testing.T) {
	// Test error handling for invalid target template.
	params := &processTargetsParams{
		AtmosConfig: &schema.AtmosConfiguration{},
		IndexSource: 0,
		Source: &schema.AtmosVendorSource{
			Component: "vpc",
			Targets:   []string{"./components/terraform/{{.InvalidSyntax"},
		},
		TemplateData:         struct{ Component, Version string }{Component: "vpc", Version: "1.0.0"},
		VendorConfigFilePath: "/tmp",
		URI:                  "github.com/example/repo.git//vpc",
		PkgType:              pkgTypeRemote,
		SourceIsLocalFile:    false,
	}

	_, err := processTargets(params)
	assert.Error(t, err, "Should return error for invalid template")
}

func TestExecuteAtmosVendorInternal_SkippedSource(t *testing.T) {
	// Test skipping sources based on component and tags filters.
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Test with sources that should be skipped due to component filter.
	params := &executeVendorOptions{
		atmosConfig:          atmosConfig,
		vendorConfigFileName: filepath.Join(tempDir, "vendor.yaml"),
		atmosVendorSpec: schema.AtmosVendorSpec{
			Sources: []schema.AtmosVendorSource{
				{
					Component: "vpc",
					Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
					Targets:   []string{"./components/terraform/vpc"},
				},
				{
					Component: "rds",
					Source:    "github.com/cloudposse/terraform-aws-components.git//modules/rds?ref=1.0.0",
					Targets:   []string{"./components/terraform/rds"},
				},
			},
		},
		component: "vpc",
	}

	// This should work and only process vpc.
	err := executeAtmosVendorInternal(params)
	// Since the sources are valid, it should not return an error about component not defined.
	// It may return other errors related to TUI or network, but not ErrComponentNotDefined.
	if err != nil {
		// Only fail if it's the wrong type of error.
		assert.NotErrorIs(t, err, ErrComponentNotDefined)
	}
}

func TestProcessTargets_MultipleTargets(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{}

	params := &processTargetsParams{
		AtmosConfig: atmosConfig,
		IndexSource: 0,
		Source: &schema.AtmosVendorSource{
			Component: "vpc",
			Version:   "1.0.0",
			Targets:   []string{"./components/terraform/vpc", "./modules/vpc"},
		},
		TemplateData:         struct{ Component, Version string }{Component: "vpc", Version: "1.0.0"},
		VendorConfigFilePath: tempDir,
		URI:                  "github.com/example/repo.git//vpc",
		PkgType:              pkgTypeRemote,
		SourceIsLocalFile:    false,
	}

	packages, err := processTargets(params)
	assert.NoError(t, err)
	assert.Len(t, packages, 2)
	assert.Equal(t, "vpc", packages[0].name)
	assert.Equal(t, "1.0.0", packages[0].version)
}

func TestProcessTargets_NoComponent(t *testing.T) {
	// When component is empty, URI should be used as name.
	tempDir := t.TempDir()

	params := &processTargetsParams{
		AtmosConfig: &schema.AtmosConfiguration{},
		IndexSource: 0,
		Source: &schema.AtmosVendorSource{
			Component: "",
			Targets:   []string{"./target"},
		},
		TemplateData:         struct{ Component, Version string }{Component: "", Version: ""},
		VendorConfigFilePath: tempDir,
		URI:                  "github.com/example/repo.git",
		PkgType:              pkgTypeRemote,
		SourceIsLocalFile:    false,
	}

	packages, err := processTargets(params)
	assert.NoError(t, err)
	assert.Len(t, packages, 1)
	assert.Equal(t, "github.com/example/repo.git", packages[0].name)
}

func TestDeterminePackageType_All(t *testing.T) {
	tests := []struct {
		name         string
		useOci       bool
		useLocalFS   bool
		expectedType pkgType
	}{
		{"remote", false, false, pkgTypeRemote},
		{"OCI", true, false, pkgTypeOci},
		{"local", false, true, pkgTypeLocal},
		{"OCI takes precedence over local", true, true, pkgTypeOci},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determinePackageType(tt.useOci, tt.useLocalFS)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

func TestProcessAtmosVendorSource_EmptySources(t *testing.T) {
	params := &vendorSourceParams{
		atmosConfig:          &schema.AtmosConfiguration{},
		sources:              []schema.AtmosVendorSource{},
		component:            "",
		tags:                 nil,
		vendorConfigFileName: "vendor.yaml",
		vendorConfigFilePath: "/tmp",
	}

	packages, err := processAtmosVendorSource(params)
	assert.NoError(t, err)
	assert.Empty(t, packages)
}

func TestProcessAtmosVendorSource_SkipsNonMatchingComponent(t *testing.T) {
	tempDir := t.TempDir()

	params := &vendorSourceParams{
		atmosConfig: &schema.AtmosConfiguration{},
		sources: []schema.AtmosVendorSource{
			{
				Component: "vpc",
				Source:    "github.com/example/repo.git//vpc",
				Targets:   []string{"./vpc"},
			},
			{
				Component: "rds",
				Source:    "github.com/example/repo.git//rds",
				Targets:   []string{"./rds"},
			},
		},
		component:            "vpc",
		tags:                 nil,
		vendorConfigFileName: "vendor.yaml",
		vendorConfigFilePath: tempDir,
	}

	packages, err := processAtmosVendorSource(params)
	assert.NoError(t, err)
	assert.Len(t, packages, 1)
	assert.Equal(t, "vpc", packages[0].name)
}

func TestPull_NoVendorConfig(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Pull without vendor.yaml or component should return error.
	err := Pull(atmosConfig)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorConfigNotExist)
}

func TestPull_WithComponent(t *testing.T) {
	tempDir := t.TempDir()

	// Create component directory with component.yaml.
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	componentConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: vpc
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
	err = os.WriteFile(filepath.Join(componentDir, "component.yaml"), []byte(componentConfig), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Pull with component should attempt to vendor the component.
	err = Pull(atmosConfig, WithComponent("vpc"), WithDryRun(true))
	// May fail due to network issues, but should not return ErrVendorConfigNotExist.
	if err != nil {
		assert.NotErrorIs(t, err, ErrVendorConfigNotExist)
	}
}

func TestHandleVendorConfigNotExist_DefaultComponentType(t *testing.T) {
	tempDir := t.TempDir()

	// Create component directory.
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	componentConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: vpc
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
	err = os.WriteFile(filepath.Join(componentDir, "component.yaml"), []byte(componentConfig), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Empty componentType should default to terraform.
	err = HandleVendorConfigNotExist(atmosConfig, "vpc", "", true)
	// May fail due to network issues, but should at least find the component.
	if err != nil {
		assert.NotErrorIs(t, err, ErrComponentConfigFileNotFound)
	}
}

func TestHandleVendorConfigNotExist_NotFound(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Non-existent component should return error.
	err := HandleVendorConfigNotExist(atmosConfig, "nonexistent", "terraform", true)
	assert.Error(t, err)
}
