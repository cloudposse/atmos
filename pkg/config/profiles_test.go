package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDiscoverProfileLocations tests profile location discovery with different configurations.
func TestDiscoverProfileLocations(t *testing.T) {
	tests := []struct {
		name              string
		atmosConfig       schema.AtmosConfiguration
		expectedLocations int
		expectedTypes     []string
	}{
		{
			name: "default locations (no custom base_path)",
			atmosConfig: schema.AtmosConfiguration{
				CliConfigPath: "/test/config",
				Profiles:      schema.ProfilesConfig{},
			},
			expectedLocations: 3, // project-hidden, xdg, project (no configurable when base_path is empty)
			expectedTypes:     []string{"project-hidden", "xdg", "project"},
		},
		{
			name: "custom base_path (absolute)",
			atmosConfig: schema.AtmosConfiguration{
				CliConfigPath: "/test/config",
				Profiles: schema.ProfilesConfig{
					BasePath: "/custom/profiles",
				},
			},
			expectedLocations: 4,
			expectedTypes:     []string{"configurable", "project-hidden", "xdg", "project"},
		},
		{
			name: "custom base_path (relative)",
			atmosConfig: schema.AtmosConfiguration{
				CliConfigPath: "/test/config",
				Profiles: schema.ProfilesConfig{
					BasePath: "custom-profiles",
				},
			},
			expectedLocations: 4,
			expectedTypes:     []string{"configurable", "project-hidden", "xdg", "project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations, err := discoverProfileLocations(&tt.atmosConfig)
			require.NoError(t, err)
			assert.Len(t, locations, tt.expectedLocations)

			for _, expectedType := range tt.expectedTypes {
				found := false
				for _, loc := range locations {
					if loc.Type == expectedType {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected location type %s not found", expectedType)
			}
		})
	}
}

// TestFindProfileDirectory tests profile directory lookup across locations.
func TestFindProfileDirectory(t *testing.T) {
	// Create temporary test directories.
	tmpDir := t.TempDir()

	// Create profile directories in different locations.
	projectProfilesDir := filepath.Join(tmpDir, "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(projectProfilesDir, "developer"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectProfilesDir, "production"), 0o755))

	hiddenProfilesDir := filepath.Join(tmpDir, ".atmos", "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(hiddenProfilesDir, "ci"), 0o755))

	tests := []struct {
		name           string
		profileName    string
		locations      []ProfileLocation
		expectFound    bool
		expectedType   string
		expectedErrMsg string
	}{
		{
			name:        "profile found in project location",
			profileName: "developer",
			locations: []ProfileLocation{
				{Path: projectProfilesDir, Type: "project", Precedence: 4},
			},
			expectFound:  true,
			expectedType: "project",
		},
		{
			name:        "profile found in hidden location (higher precedence)",
			profileName: "ci",
			locations: []ProfileLocation{
				{Path: projectProfilesDir, Type: "project", Precedence: 4},
				{Path: hiddenProfilesDir, Type: "project-hidden", Precedence: 2},
			},
			expectFound:  true,
			expectedType: "project-hidden",
		},
		{
			name:        "profile not found",
			profileName: "nonexistent",
			locations: []ProfileLocation{
				{Path: projectProfilesDir, Type: "project", Precedence: 4},
			},
			expectFound:    false,
			expectedErrMsg: "not found",
		},
		{
			name:        "precedence: hidden wins over project",
			profileName: "developer",
			locations: []ProfileLocation{
				{Path: projectProfilesDir, Type: "project", Precedence: 4},
				{Path: filepath.Join(tmpDir, ".atmos", "profiles"), Type: "project-hidden", Precedence: 2},
			},
			expectFound:  true,
			expectedType: "project", // developer only exists in project, not hidden
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileDir, locType, err := findProfileDirectory(tt.profileName, tt.locations)

			if tt.expectFound {
				require.NoError(t, err)
				assert.NotEmpty(t, profileDir)
				assert.Equal(t, tt.expectedType, locType)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			}
		})
	}
}

// TestListAvailableProfiles tests listing all available profiles.
func TestListAvailableProfiles(t *testing.T) {
	// Create temporary test directories.
	tmpDir := t.TempDir()

	// Create profile directories.
	projectProfilesDir := filepath.Join(tmpDir, "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(projectProfilesDir, "developer"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectProfilesDir, "production"), 0o755))

	hiddenProfilesDir := filepath.Join(tmpDir, ".atmos", "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(hiddenProfilesDir, "ci"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(hiddenProfilesDir, "developer"), 0o755)) // Duplicate profile name

	locations := []ProfileLocation{
		{Path: projectProfilesDir, Type: "project", Precedence: 4},
		{Path: hiddenProfilesDir, Type: "project-hidden", Precedence: 2},
	}

	profiles, err := listAvailableProfiles(locations)
	require.NoError(t, err)

	// Should find 3 unique profile names (developer, production, ci).
	assert.Len(t, profiles, 3)

	// Developer should be found in both locations.
	assert.Contains(t, profiles, "developer")
	assert.Len(t, profiles["developer"], 2)

	// Production only in project.
	assert.Contains(t, profiles, "production")
	assert.Len(t, profiles["production"], 1)

	// CI only in hidden.
	assert.Contains(t, profiles, "ci")
	assert.Len(t, profiles["ci"], 1)
}

// TestLoadProfileFiles tests loading YAML files from a profile directory.
func TestLoadProfileFiles(t *testing.T) {
	// Create temporary profile directory.
	tmpDir := t.TempDir()
	profileDir := filepath.Join(tmpDir, "test-profile")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))

	// Create test YAML files.
	authYAML := `
auth:
  providers:
    test-provider:
      kind: aws/sso
      region: us-east-1
  identities:
    test-identity:
      via:
        provider: test-provider
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "auth.yaml"), []byte(authYAML), 0o644))

	settingsYAML := `
settings:
  terminal:
    color: true
`
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "settings.yaml"), []byte(settingsYAML), 0o644))

	tests := []struct {
		name                string
		profileDir          string
		profileName         string
		expectError         bool
		expectedErrMsg      string
		expectedErrSentinel error
	}{
		{
			name:        "successful profile loading",
			profileDir:  profileDir,
			profileName: "test-profile",
			expectError: false,
		},
		{
			name:           "profile directory does not exist",
			profileDir:     filepath.Join(tmpDir, "nonexistent"),
			profileName:    "nonexistent",
			expectError:    true,
			expectedErrMsg: "does not exist",
		},
		{
			name:        "profile path is not a directory",
			profileDir:  filepath.Join(profileDir, "auth.yaml"), // Point to file instead of directory
			profileName: "invalid",
			expectError: true,
			// Use error sentinel check instead of string matching.
			expectedErrSentinel: errUtils.ErrProfileDirNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("yaml")

			err := loadProfileFiles(v, tt.profileDir, tt.profileName)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrSentinel != nil {
					assert.ErrorIs(t, err, tt.expectedErrSentinel)
				} else if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
			} else {
				require.NoError(t, err)

				// Verify config was loaded correctly.
				var config schema.AtmosConfiguration
				err = v.Unmarshal(&config)
				require.NoError(t, err)

				// Check auth config was loaded.
				assert.Contains(t, config.Auth.Providers, "test-provider")
				assert.Contains(t, config.Auth.Identities, "test-identity")

				// Check settings config was loaded.
				assert.True(t, config.Settings.Terminal.Color)
			}
		})
	}
}

// TestLoadProfiles tests loading multiple profiles with precedence.
func TestLoadProfiles(t *testing.T) {
	// Create temporary directory structure.
	tmpDir := t.TempDir()

	// Isolate XDG_CONFIG_HOME to prevent test from accessing system directories.
	cleanup := withTestXDGConfigHome(t, tmpDir)
	t.Cleanup(cleanup)

	profilesDir := filepath.Join(tmpDir, "profiles")

	// Create base profile.
	baseProfileDir := filepath.Join(profilesDir, "base")
	require.NoError(t, os.MkdirAll(baseProfileDir, 0o755))
	baseYAML := `
settings:
  terminal:
    color: true
    max_width: 100
`
	require.NoError(t, os.WriteFile(filepath.Join(baseProfileDir, "settings.yaml"), []byte(baseYAML), 0o644))

	// Create developer profile (overrides base).
	devProfileDir := filepath.Join(profilesDir, "developer")
	require.NoError(t, os.MkdirAll(devProfileDir, 0o755))
	devYAML := `
settings:
  terminal:
    max_width: 120
logs:
  level: Debug
`
	require.NoError(t, os.WriteFile(filepath.Join(devProfileDir, "settings.yaml"), []byte(devYAML), 0o644))

	tests := []struct {
		name              string
		profileNames      []string
		expectError       bool
		expectedMaxWidth  int
		expectedLogsLevel string
		expectedColor     bool
		checkColor        bool
		expectedErrMsg    string
	}{
		{
			name:             "load single profile",
			profileNames:     []string{"base"},
			expectError:      false,
			expectedMaxWidth: 100,
			expectedColor:    true,
			checkColor:       true,
		},
		{
			name:              "load multiple profiles (rightmost wins)",
			profileNames:      []string{"base", "developer"},
			expectError:       false,
			expectedMaxWidth:  120,  // From developer (overrides base)
			expectedColor:     true, // From base
			checkColor:        true,
			expectedLogsLevel: "Debug", // From developer
		},
		{
			name:           "nonexistent profile",
			profileNames:   []string{"nonexistent"},
			expectError:    true,
			expectedErrMsg: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("yaml")

			atmosConfig := &schema.AtmosConfiguration{
				CliConfigPath: tmpDir,
				Profiles: schema.ProfilesConfig{
					BasePath: "profiles",
				},
			}

			err := loadProfiles(v, tt.profileNames, atmosConfig)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				require.NoError(t, err)

				// Unmarshal and verify merged config.
				var config schema.AtmosConfiguration
				err = v.Unmarshal(&config)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedMaxWidth, config.Settings.Terminal.MaxWidth)

				if tt.checkColor {
					assert.Equal(t, tt.expectedColor, config.Settings.Terminal.Color)
				}

				if tt.expectedLogsLevel != "" {
					assert.Equal(t, tt.expectedLogsLevel, config.Logs.Level)
				}
			}
		})
	}
}

// TestProfileWithImports tests that import: directives work in profile files.
func TestProfileWithImports(t *testing.T) {
	tests := []struct {
		name           string
		profileContent string
		importContent  string
		importSubdir   string // subdirectory for import file (empty = same directory as profile)
		validateConfig func(t *testing.T, v *viper.Viper)
	}{
		{
			name: "profile with import loads imported values",
			profileContent: `import:
  - ./imports/imported.yaml
settings:
  terminal:
    max_width: 100
`,
			importContent: `settings:
  terminal:
    color: true
logs:
  level: debug
`,
			importSubdir: "imports",
			validateConfig: func(t *testing.T, v *viper.Viper) {
				// Profile values should be present.
				assert.Equal(t, 100, v.GetInt("settings.terminal.max_width"))
				// Imported values should be present.
				assert.Equal(t, true, v.GetBool("settings.terminal.color"))
				assert.Equal(t, "debug", v.GetString("logs.level"))
			},
		},
		{
			name: "importing file takes precedence over imported files",
			profileContent: `import:
  - ./imports/imported.yaml
settings:
  terminal:
    max_width: 120
`,
			importContent: `settings:
  terminal:
    max_width: 80
    color: true
`,
			importSubdir: "imports",
			validateConfig: func(t *testing.T, v *viper.Viper) {
				// The importing file (profile) takes precedence over imported files.
				// This aligns with mergeConfig() semantics: "importing file always takes precedence".
				// NOTE: The import file is placed in a subdirectory to ensure it's only loaded
				// via the import mechanism, not via directory scanning.
				assert.Equal(t, 120, v.GetInt("settings.terminal.max_width"))
				// Import value should still be present when not overridden by the importing file.
				assert.Equal(t, true, v.GetBool("settings.terminal.color"))
			},
		},
		{
			name: "profile with no imports works normally",
			profileContent: `settings:
  terminal:
    max_width: 150
`,
			importContent: "", // No import file needed.
			validateConfig: func(t *testing.T, v *viper.Viper) {
				assert.Equal(t, 150, v.GetInt("settings.terminal.max_width"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create profile directory with config file.
			profileDir := filepath.Join(tmpDir, "profiles", "test-profile")
			require.NoError(t, os.MkdirAll(profileDir, 0o755))

			// Write profile config.
			profileFile := filepath.Join(profileDir, "atmos.yaml")
			require.NoError(t, os.WriteFile(profileFile, []byte(tt.profileContent), 0o644))

			// Write imported file if content provided.
			if tt.importContent != "" {
				importDir := profileDir
				if tt.importSubdir != "" {
					importDir = filepath.Join(profileDir, tt.importSubdir)
					require.NoError(t, os.MkdirAll(importDir, 0o755))
				}
				importFile := filepath.Join(importDir, "imported.yaml")
				require.NoError(t, os.WriteFile(importFile, []byte(tt.importContent), 0o644))
			}

			// Load the profile using loadAtmosConfigsFromDirectory.
			// Use pattern without ** to only load files in the profile directory,
			// not subdirectories. Import files in subdirectories are loaded via
			// the import mechanism, not via directory scanning.
			v := viper.New()
			v.SetConfigType("yaml")

			searchPattern := filepath.Join(profileDir, "*")
			err := loadAtmosConfigsFromDirectory(searchPattern, v, "test-profile")
			require.NoError(t, err)

			// Validate the config.
			tt.validateConfig(t, v)
		})
	}
}

// TestProcessFileImportsIfPresent tests the import processing helper function.
func TestProcessFileImportsIfPresent(t *testing.T) {
	tests := []struct {
		name           string
		fileContent    string
		importContent  string
		hasImport      bool
		validateResult func(t *testing.T, v *viper.Viper)
		expectError    bool
	}{
		{
			name: "file with no imports - returns early",
			fileContent: `settings:
  terminal:
    max_width: 100
`,
			hasImport: false,
			validateResult: func(t *testing.T, v *viper.Viper) {
				// Should have no additional values from imports.
			},
			expectError: false,
		},
		{
			name: "file with valid import",
			fileContent: `import:
  - ./settings.yaml
`,
			importContent: `logs:
  level: trace
`,
			hasImport: true,
			validateResult: func(t *testing.T, v *viper.Viper) {
				assert.Equal(t, "trace", v.GetString("logs.level"))
			},
			expectError: false,
		},
		{
			name: "file with missing import file - silently continues (non-fatal)",
			fileContent: `import:
  - ./nonexistent.yaml
`,
			hasImport: true,
			// processConfigImports logs errors but continues - non-fatal behavior.
			expectError: false,
			validateResult: func(t *testing.T, v *viper.Viper) {
				// Nothing should be loaded from the missing import.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Write main file.
			mainFile := filepath.Join(tmpDir, "main.yaml")
			require.NoError(t, os.WriteFile(mainFile, []byte(tt.fileContent), 0o644))

			// Write import file if content provided.
			if tt.importContent != "" {
				importFile := filepath.Join(tmpDir, "settings.yaml")
				require.NoError(t, os.WriteFile(importFile, []byte(tt.importContent), 0o644))
			}

			v := viper.New()
			v.SetConfigType("yaml")

			err := processFileImportsIfPresent(mainFile, tmpDir, v)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateResult != nil {
					tt.validateResult(t, v)
				}
			}
		})
	}
}
