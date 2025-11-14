package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetProfileLocations tests the GetProfileLocations method.
func TestGetProfileLocations(t *testing.T) {
	tests := []struct {
		name          string
		basePath      string
		expectedTypes []string
		expectedCount int
		setupDirs     func(t *testing.T, baseDir string) string
	}{
		{
			name:          "all location types with no base path",
			basePath:      "",
			expectedTypes: []string{"project-hidden", "xdg", "project"},
			expectedCount: 3,
			setupDirs: func(t *testing.T, baseDir string) string {
				return baseDir
			},
		},
		{
			name:          "with configurable base_path",
			basePath:      "custom/profiles",
			expectedTypes: []string{"configurable", "project-hidden", "xdg", "project"},
			expectedCount: 4,
			setupDirs: func(t *testing.T, baseDir string) string {
				customPath := filepath.Join(baseDir, "custom", "profiles")
				require.NoError(t, os.MkdirAll(customPath, 0o755))
				return baseDir
			},
		},
		{
			name:          "with absolute configurable base_path",
			basePath:      "", // Will be set to absolute in test
			expectedTypes: []string{"configurable", "project-hidden", "xdg", "project"},
			expectedCount: 4,
			setupDirs: func(t *testing.T, baseDir string) string {
				return baseDir
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory structure.
			tmpDir := t.TempDir()
			configDir := tt.setupDirs(t, tmpDir)

			basePath := tt.basePath
			if tt.name == "with absolute configurable base_path" {
				basePath = filepath.Join(tmpDir, "absolute", "profiles")
				require.NoError(t, os.MkdirAll(basePath, 0o755))
			}

			atmosConfig := &schema.AtmosConfiguration{
				CliConfigPath: configDir,
				Profiles: schema.ProfilesConfig{
					BasePath: basePath,
				},
			}

			manager := NewProfileManager()
			locations, err := manager.GetProfileLocations(atmosConfig)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(locations), tt.expectedCount)

			// Verify expected location types are present.
			foundTypes := make(map[string]bool)
			for _, loc := range locations {
				foundTypes[loc.Type] = true
			}

			for _, expectedType := range tt.expectedTypes {
				assert.True(t, foundTypes[expectedType], "Expected location type %s not found", expectedType)
			}

			// Verify precedence ordering matches location type.
			for _, loc := range locations {
				// Verify each location has correct precedence based on type.
				switch loc.Type {
				case "configurable":
					assert.Equal(t, 1, loc.Precedence)
				case "project-hidden":
					assert.Equal(t, 2, loc.Precedence)
				case "xdg":
					assert.Equal(t, 3, loc.Precedence)
				case "project":
					assert.Equal(t, 4, loc.Precedence)
				}
			}
		})
	}
}

// TestListProfiles tests the ListProfiles method.
func TestListProfiles(t *testing.T) {
	tests := []struct {
		name          string
		setupProfiles func(t *testing.T, baseDir string) string
		expectedCount int
		expectedNames []string
		expectError   bool
	}{
		{
			name: "no profiles",
			setupProfiles: func(t *testing.T, baseDir string) string {
				return baseDir
			},
			expectedCount: 0,
			expectedNames: []string{},
			expectError:   false,
		},
		{
			name: "single profile in project directory",
			setupProfiles: func(t *testing.T, baseDir string) string {
				profileDir := filepath.Join(baseDir, "profiles", "dev")
				require.NoError(t, os.MkdirAll(profileDir, 0o755))

				// Create a config file in the profile.
				configFile := filepath.Join(profileDir, "atmos.yaml")
				require.NoError(t, os.WriteFile(configFile, []byte("base_path: /stacks"), 0o644))

				return baseDir
			},
			expectedCount: 1,
			expectedNames: []string{"dev"},
			expectError:   false,
		},
		{
			name: "multiple profiles across locations",
			setupProfiles: func(t *testing.T, baseDir string) string {
				// Project profile.
				projectProfile := filepath.Join(baseDir, "profiles", "prod")
				require.NoError(t, os.MkdirAll(projectProfile, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(projectProfile, "atmos.yaml"),
					[]byte("base_path: /stacks/prod"),
					0o644,
				))

				// Hidden profile.
				hiddenProfile := filepath.Join(baseDir, ".atmos", "profiles", "staging")
				require.NoError(t, os.MkdirAll(hiddenProfile, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(hiddenProfile, "atmos.yaml"),
					[]byte("base_path: /stacks/staging"),
					0o644,
				))

				return baseDir
			},
			expectedCount: 2,
			expectedNames: []string{"prod", "staging"},
			expectError:   false,
		},
		{
			name: "profile with metadata",
			setupProfiles: func(t *testing.T, baseDir string) string {
				profileDir := filepath.Join(baseDir, "profiles", "production")
				require.NoError(t, os.MkdirAll(profileDir, 0o755))

				// Create profile with metadata.
				metadataYAML := `
metadata:
  name: Production Environment
  description: Production deployment configuration
  version: 1.0.0
  tags:
    - production
    - aws
  deprecated: false
`
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "atmos.yaml"),
					[]byte(metadataYAML),
					0o644,
				))

				return baseDir
			},
			expectedCount: 1,
			expectedNames: []string{"production"},
			expectError:   false,
		},
		{
			name: "precedence: higher precedence profile overrides lower",
			setupProfiles: func(t *testing.T, baseDir string) string {
				// Create same profile name in two locations.
				// Project location (precedence 4 - lower).
				projectProfile := filepath.Join(baseDir, "profiles", "shared")
				require.NoError(t, os.MkdirAll(projectProfile, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(projectProfile, "atmos.yaml"),
					[]byte("# Project profile"),
					0o644,
				))

				// Hidden location (precedence 2 - higher).
				hiddenProfile := filepath.Join(baseDir, ".atmos", "profiles", "shared")
				require.NoError(t, os.MkdirAll(hiddenProfile, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(hiddenProfile, "atmos.yaml"),
					[]byte("# Hidden profile"),
					0o644,
				))

				return baseDir
			},
			expectedCount: 1,
			expectedNames: []string{"shared"},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := tt.setupProfiles(t, tmpDir)

			atmosConfig := &schema.AtmosConfiguration{
				CliConfigPath: configDir,
				Profiles: schema.ProfilesConfig{
					BasePath: "",
				},
			}

			manager := NewProfileManager()
			profiles, err := manager.ListProfiles(atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(profiles))

			// Verify expected profile names.
			foundNames := make(map[string]bool)
			for _, profile := range profiles {
				foundNames[profile.Name] = true
			}

			for _, expectedName := range tt.expectedNames {
				assert.True(t, foundNames[expectedName], "Expected profile %s not found", expectedName)
			}

			// Verify profiles are sorted by name.
			if len(profiles) > 1 {
				for i := 1; i < len(profiles); i++ {
					assert.True(t, profiles[i-1].Name <= profiles[i].Name,
						"Profiles should be sorted by name")
				}
			}
		})
	}
}

// TestGetProfile tests the GetProfile method.
func TestGetProfile(t *testing.T) {
	tests := []struct {
		name            string
		profileName     string
		setupProfile    func(t *testing.T, baseDir string) string
		expectError     bool
		validateProfile func(t *testing.T, profile *ProfileInfo)
	}{
		{
			name:        "profile not found",
			profileName: "nonexistent",
			setupProfile: func(t *testing.T, baseDir string) string {
				return baseDir
			},
			expectError: true,
		},
		{
			name:        "profile found in project directory",
			profileName: "dev",
			setupProfile: func(t *testing.T, baseDir string) string {
				profileDir := filepath.Join(baseDir, "profiles", "dev")
				require.NoError(t, os.MkdirAll(profileDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "atmos.yaml"),
					[]byte("base_path: /stacks"),
					0o644,
				))
				return baseDir
			},
			expectError: false,
			validateProfile: func(t *testing.T, profile *ProfileInfo) {
				assert.Equal(t, "dev", profile.Name)
				assert.Equal(t, "project", profile.LocationType)
				assert.Greater(t, len(profile.Files), 0)
			},
		},
		{
			name:        "profile with metadata",
			profileName: "production",
			setupProfile: func(t *testing.T, baseDir string) string {
				profileDir := filepath.Join(baseDir, "profiles", "production")
				require.NoError(t, os.MkdirAll(profileDir, 0o755))

				metadataYAML := `
metadata:
  name: Production Environment
  description: Production deployment configuration
  version: 2.1.0
  tags:
    - production
    - aws
    - critical
  deprecated: false
`
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "atmos.yaml"),
					[]byte(metadataYAML),
					0o644,
				))

				return baseDir
			},
			expectError: false,
			validateProfile: func(t *testing.T, profile *ProfileInfo) {
				assert.Equal(t, "production", profile.Name)
				require.NotNil(t, profile.Metadata)
				assert.Equal(t, "Production Environment", profile.Metadata.Name)
				assert.Equal(t, "Production deployment configuration", profile.Metadata.Description)
				assert.Equal(t, "2.1.0", profile.Metadata.Version)
				assert.Equal(t, []string{"production", "aws", "critical"}, profile.Metadata.Tags)
				assert.False(t, profile.Metadata.Deprecated)
			},
		},
		{
			name:        "deprecated profile",
			profileName: "legacy",
			setupProfile: func(t *testing.T, baseDir string) string {
				profileDir := filepath.Join(baseDir, "profiles", "legacy")
				require.NoError(t, os.MkdirAll(profileDir, 0o755))

				metadataYAML := `
metadata:
  deprecated: true
  description: Legacy configuration - use 'modern' instead
`
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "atmos.yaml"),
					[]byte(metadataYAML),
					0o644,
				))

				return baseDir
			},
			expectError: false,
			validateProfile: func(t *testing.T, profile *ProfileInfo) {
				assert.Equal(t, "legacy", profile.Name)
				require.NotNil(t, profile.Metadata)
				assert.True(t, profile.Metadata.Deprecated)
			},
		},
		{
			name:        "precedence: hidden profile overrides project profile",
			profileName: "shared",
			setupProfile: func(t *testing.T, baseDir string) string {
				// Project profile (lower precedence).
				projectProfile := filepath.Join(baseDir, "profiles", "shared")
				require.NoError(t, os.MkdirAll(projectProfile, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(projectProfile, "project.yaml"),
					[]byte("# Project"),
					0o644,
				))

				// Hidden profile (higher precedence).
				hiddenProfile := filepath.Join(baseDir, ".atmos", "profiles", "shared")
				require.NoError(t, os.MkdirAll(hiddenProfile, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(hiddenProfile, "hidden.yaml"),
					[]byte("# Hidden"),
					0o644,
				))

				return baseDir
			},
			expectError: false,
			validateProfile: func(t *testing.T, profile *ProfileInfo) {
				assert.Equal(t, "shared", profile.Name)
				assert.Equal(t, "project-hidden", profile.LocationType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := tt.setupProfile(t, tmpDir)

			atmosConfig := &schema.AtmosConfiguration{
				CliConfigPath: configDir,
				Profiles: schema.ProfilesConfig{
					BasePath: "",
				},
			}

			manager := NewProfileManager()
			profile, err := manager.GetProfile(atmosConfig, tt.profileName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, profile)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, profile)

			if tt.validateProfile != nil {
				tt.validateProfile(t, profile)
			}
		})
	}
}

// TestDirExists tests the dirExists helper function.
func TestDirExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Test existing directory.
	assert.True(t, dirExists(tmpDir))

	// Test non-existent directory.
	assert.False(t, dirExists(filepath.Join(tmpDir, "nonexistent")))

	// Test file (not directory).
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))
	assert.False(t, dirExists(testFile))
}

// TestFileExists tests the fileExists helper function.
func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Test existing file.
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))
	assert.True(t, fileExists(testFile))

	// Test non-existent file.
	assert.False(t, fileExists(filepath.Join(tmpDir, "nonexistent.txt")))

	// Test directory (not file).
	assert.False(t, fileExists(tmpDir))
}

// TestLoadProfileMetadata tests the loadProfileMetadata helper function.
func TestLoadProfileMetadata(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		expectError bool
		validate    func(t *testing.T, metadata *schema.ConfigMetadata)
	}{
		{
			name: "valid metadata",
			yamlContent: `
metadata:
  name: Test Profile
  description: Test description
  version: 1.0.0
  tags:
    - test
    - development
  deprecated: false
`,
			expectError: false,
			validate: func(t *testing.T, metadata *schema.ConfigMetadata) {
				require.NotNil(t, metadata)
				assert.Equal(t, "Test Profile", metadata.Name)
				assert.Equal(t, "Test description", metadata.Description)
				assert.Equal(t, "1.0.0", metadata.Version)
				assert.Equal(t, []string{"test", "development"}, metadata.Tags)
				assert.False(t, metadata.Deprecated)
			},
		},
		{
			name: "partial metadata",
			yamlContent: `
metadata:
  name: Partial Profile
`,
			expectError: false,
			validate: func(t *testing.T, metadata *schema.ConfigMetadata) {
				require.NotNil(t, metadata)
				assert.Equal(t, "Partial Profile", metadata.Name)
				assert.Empty(t, metadata.Description)
				assert.Empty(t, metadata.Version)
			},
		},
		{
			name: "only deprecated flag",
			yamlContent: `
metadata:
  deprecated: true
`,
			expectError: false,
			validate: func(t *testing.T, metadata *schema.ConfigMetadata) {
				require.NotNil(t, metadata)
				assert.True(t, metadata.Deprecated)
			},
		},
		{
			name: "no metadata section",
			yamlContent: `
base_path: /stacks
`,
			expectError: false,
			validate: func(t *testing.T, metadata *schema.ConfigMetadata) {
				assert.Nil(t, metadata)
			},
		},
		{
			name: "empty metadata section",
			yamlContent: `
metadata:
`,
			expectError: false,
			validate: func(t *testing.T, metadata *schema.ConfigMetadata) {
				assert.Nil(t, metadata)
			},
		},
		{
			name:        "invalid yaml",
			yamlContent: `invalid: [yaml: syntax`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			yamlFile := filepath.Join(tmpDir, "atmos.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(tt.yamlContent), 0o644))

			metadata, err := loadProfileMetadata(yamlFile)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, metadata)
			}
		})
	}
}

// TestListProfileFiles tests the listProfileFiles helper function.
func TestListProfileFiles(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    func(t *testing.T, profileDir string)
		expectedFiles []string
		expectError   bool
	}{
		{
			name: "single yaml file",
			setupFiles: func(t *testing.T, profileDir string) {
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "atmos.yaml"),
					[]byte("test: value"),
					0o644,
				))
			},
			expectedFiles: []string{"atmos.yaml"},
			expectError:   false,
		},
		{
			name: "multiple files in nested structure",
			setupFiles: func(t *testing.T, profileDir string) {
				require.NoError(t, os.MkdirAll(filepath.Join(profileDir, "stacks"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "atmos.yaml"),
					[]byte("base: config"),
					0o644,
				))
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "stacks", "dev.yaml"),
					[]byte("stack: dev"),
					0o644,
				))
			},
			expectedFiles: []string{"atmos.yaml", filepath.Join("stacks", "dev.yaml")},
			expectError:   false,
		},
		{
			name: "empty profile directory",
			setupFiles: func(t *testing.T, profileDir string) {
				// Create a YAML file so directory is not completely empty.
				// listProfileFiles uses SearchAtmosConfig which errors on completely empty dirs.
				require.NoError(t, os.WriteFile(
					filepath.Join(profileDir, "empty.yaml"),
					[]byte("# Empty"),
					0o644,
				))
			},
			expectedFiles: []string{"empty.yaml"},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			profileDir := filepath.Join(tmpDir, "profile")
			require.NoError(t, os.MkdirAll(profileDir, 0o755))

			tt.setupFiles(t, profileDir)

			files, err := listProfileFiles(profileDir)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Sort both slices for comparison.
			assert.ElementsMatch(t, tt.expectedFiles, files)
		})
	}
}
