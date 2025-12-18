package vendor

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

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
				assert.Equal(t, tt.expectedURIAfter, uri)
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
