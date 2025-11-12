package exec

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/version"
)

// TestNewVersionExec tests the constructor.
func TestNewVersionExec(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	exec := NewVersionExec(config)

	assert.NotNil(t, exec)
	assert.Equal(t, config, exec.atmosConfig)
	assert.NotNil(t, exec.printStyledText)
	assert.NotNil(t, exec.getLatestGitHubRepoRelease)
	assert.NotNil(t, exec.printMessage)
	assert.NotNil(t, exec.printMessageToUpgradeToAtmosLatestRelease)
	assert.NotNil(t, exec.loadCacheConfig)
	assert.NotNil(t, exec.shouldCheckForUpdates)
}

// TestVersionExec_Execute_PrintStyledTextError tests error handling when printing styled text fails.
func TestVersionExec_Execute_PrintStyledTextError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockVersionExecutor(ctrl)

	// Mock printMessage for the initial empty line call.
	mockExec.EXPECT().PrintMessage("").Times(1)
	// Mock printStyledText to return an error.
	expectedError := errors.New("styled text error")
	mockExec.EXPECT().PrintStyledText("ATMOS").Return(expectedError)

	v := versionExec{
		atmosConfig:     &schema.AtmosConfiguration{},
		printStyledText: mockExec.PrintStyledText,
		printMessage:    mockExec.PrintMessage,
	}

	err := v.Execute(false, "")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVersionDisplayFailed)
}

// TestVersionExec_Execute_WithoutCheckFlag tests normal execution without check flag.
func TestVersionExec_Execute_WithoutCheckFlag(t *testing.T) {
	// Save original version.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "v1.2.3"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockVersionExecutor(ctrl)

	// Setup expectations.
	mockExec.EXPECT().PrintStyledText("ATMOS").Return(nil)
	mockExec.EXPECT().PrintMessage(gomock.Any()).Times(3) // Empty line, version line, empty line

	v := versionExec{
		atmosConfig:     &schema.AtmosConfiguration{},
		printStyledText: mockExec.PrintStyledText,
		printMessage:    mockExec.PrintMessage,
		loadCacheConfig: func() (cfg.CacheConfig, error) {
			return cfg.CacheConfig{LastChecked: 0}, nil
		},
		shouldCheckForUpdates: func(lastChecked int64, frequency string) bool {
			return false // Disable auto-check
		},
	}

	err := v.Execute(false, "")
	assert.NoError(t, err)
}

// TestVersionExec_checkRelease tests the checkRelease method.
func TestVersionExec_checkRelease(t *testing.T) {
	// Save original version.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()

	tests := []struct {
		name                   string
		currentVersion         string
		latestRelease          string
		getReleaseErr          error
		expectUpgradeMessage   bool
		expectCheckmarkMessage bool
	}{
		{
			name:                   "same version shows checkmark",
			currentVersion:         "v1.0.0",
			latestRelease:          "v1.0.0",
			expectCheckmarkMessage: true,
		},
		{
			name:                 "newer version shows upgrade message",
			currentVersion:       "v1.0.0",
			latestRelease:        "v1.1.0",
			expectUpgradeMessage: true,
		},
		{
			name:           "error fetching release",
			currentVersion: "v1.0.0",
			getReleaseErr:  errors.New("github error"),
		},
		{
			name:           "empty release tag",
			currentVersion: "v1.0.0",
			latestRelease:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.Version = tt.currentVersion

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := NewMockVersionExecutor(ctrl)

			mockExec.EXPECT().GetLatestGitHubRepoRelease().
				Return(tt.latestRelease, tt.getReleaseErr).AnyTimes()

			if tt.expectUpgradeMessage {
				mockExec.EXPECT().PrintMessageToUpgradeToAtmosLatestRelease(gomock.Any()).Times(1)
			}

			v := versionExec{
				atmosConfig:                               &schema.AtmosConfiguration{},
				getLatestGitHubRepoRelease:                mockExec.GetLatestGitHubRepoRelease,
				printMessageToUpgradeToAtmosLatestRelease: mockExec.PrintMessageToUpgradeToAtmosLatestRelease,
			}

			// Should not panic.
			assert.NotPanics(t, func() {
				v.checkRelease()
			})
		})
	}
}

// TestIsCheckVersionEnabled_CacheLoadError tests error handling when cache loading fails.
func TestIsCheckVersionEnabled_CacheLoadError(t *testing.T) {
	v := versionExec{
		atmosConfig: &schema.AtmosConfiguration{
			Version: schema.Version{
				Check: schema.VersionCheck{
					Enabled: true,
				},
			},
		},
		loadCacheConfig: func() (cfg.CacheConfig, error) {
			return cfg.CacheConfig{}, errors.New("cache load error")
		},
	}

	result := v.isCheckVersionEnabled(false)
	assert.False(t, result, "Should return false when cache load fails")
}

// TestIsCheckVersionEnabled_FrequencyCheck tests frequency-based checking.
func TestIsCheckVersionEnabled_FrequencyCheck(t *testing.T) {
	tests := []struct {
		name              string
		frequency         string
		lastChecked       int64
		shouldCheckReturn bool
		expectedEnabled   bool
	}{
		{
			name:              "frequency check says yes",
			frequency:         "daily",
			lastChecked:       0,
			shouldCheckReturn: true,
			expectedEnabled:   true,
		},
		{
			name:              "frequency check says no",
			frequency:         "daily",
			lastChecked:       999999999,
			shouldCheckReturn: false,
			expectedEnabled:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := versionExec{
				atmosConfig: &schema.AtmosConfiguration{
					Version: schema.Version{
						Check: schema.VersionCheck{
							Enabled:   true,
							Frequency: tt.frequency,
						},
					},
				},
				loadCacheConfig: func() (cfg.CacheConfig, error) {
					return cfg.CacheConfig{LastChecked: tt.lastChecked}, nil
				},
				shouldCheckForUpdates: func(lastChecked int64, frequency string) bool {
					return tt.shouldCheckReturn
				},
			}

			result := v.isCheckVersionEnabled(false)
			assert.Equal(t, tt.expectedEnabled, result)
		})
	}
}

// TestGetLatestVersion_EmptyReleaseTag tests handling of empty release tag.
func TestGetLatestVersion_EmptyReleaseTag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockVersionExecutor(ctrl)
	mockExec.EXPECT().GetLatestGitHubRepoRelease().
		Return("", nil).AnyTimes()

	v := versionExec{
		atmosConfig:                &schema.AtmosConfiguration{},
		getLatestGitHubRepoRelease: mockExec.GetLatestGitHubRepoRelease,
	}

	version, ok := v.GetLatestVersion(true)
	assert.Empty(t, version)
	assert.False(t, ok)
}

// TestGetLatestVersion_WithVersionPrefix tests version comparison with "v" prefix handling.
func TestGetLatestVersion_WithVersionPrefix(t *testing.T) {
	// Save original version.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()

	tests := []struct {
		name            string
		currentVersion  string
		latestRelease   string
		expectedVersion string
		expectedOk      bool
	}{
		{
			name:            "both with v prefix - same",
			currentVersion:  "v1.0.0",
			latestRelease:   "v1.0.0",
			expectedVersion: "",
			expectedOk:      false,
		},
		{
			name:            "both with v prefix - different",
			currentVersion:  "v1.0.0",
			latestRelease:   "v1.1.0",
			expectedVersion: "1.1.0",
			expectedOk:      true,
		},
		{
			name:            "current without v, latest with v",
			currentVersion:  "1.0.0",
			latestRelease:   "v1.1.0",
			expectedVersion: "1.1.0",
			expectedOk:      true,
		},
		{
			name:            "current with v, latest without v",
			currentVersion:  "v1.0.0",
			latestRelease:   "1.1.0",
			expectedVersion: "1.1.0",
			expectedOk:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.Version = tt.currentVersion

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := NewMockVersionExecutor(ctrl)
			mockExec.EXPECT().GetLatestGitHubRepoRelease().
				Return(tt.latestRelease, nil).AnyTimes()

			v := versionExec{
				atmosConfig:                &schema.AtmosConfiguration{},
				getLatestGitHubRepoRelease: mockExec.GetLatestGitHubRepoRelease,
			}

			resultVersion, ok := v.GetLatestVersion(true)
			assert.Equal(t, tt.expectedOk, ok)
			assert.Equal(t, tt.expectedVersion, resultVersion)
		})
	}
}

// TestDisplayVersionInFormat_WithUpdateVersion tests formatted output with update version.
func TestDisplayVersionInFormat_WithUpdateVersion(t *testing.T) {
	// Save original version.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "v1.0.0"

	// Initialize I/O context for tests.
	ioCtx, err := iolib.NewContext()
	assert.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockVersionExecutor(ctrl)
	mockExec.EXPECT().GetLatestGitHubRepoRelease().
		Return("v1.1.0", nil).AnyTimes()

	v := versionExec{
		atmosConfig:                &schema.AtmosConfiguration{},
		getLatestGitHubRepoRelease: mockExec.GetLatestGitHubRepoRelease,
	}

	// Test JSON format with update version available.
	err = v.displayVersionInFormat(true, "json")
	assert.NoError(t, err)

	// Test YAML format with update version available.
	err = v.displayVersionInFormat(true, "yaml")
	assert.NoError(t, err)
}

// TestDisplayVersionInFormat_ErrorContexts tests error contexts are properly set.
func TestDisplayVersionInFormat_ErrorContexts(t *testing.T) {
	// Initialize I/O context for tests.
	ioCtx, err := iolib.NewContext()
	assert.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	v := versionExec{
		atmosConfig: &schema.AtmosConfiguration{},
	}

	// Test invalid format error contains proper context.
	err = v.displayVersionInFormat(false, "xml")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVersionFormatInvalid)

	// Verify context contains the invalid format.
	assert.True(t, errUtils.HasContext(err, "format", "xml"), "Expected error context to contain format=xml")
}

// TestVersionStruct tests the Version struct serialization.
func TestVersionStruct(t *testing.T) {
	v := Version{
		Version:       "1.0.0",
		OS:            "linux",
		Arch:          "amd64",
		UpdateVersion: "1.1.0",
	}

	assert.Equal(t, "1.0.0", v.Version)
	assert.Equal(t, "linux", v.OS)
	assert.Equal(t, "amd64", v.Arch)
	assert.Equal(t, "1.1.0", v.UpdateVersion)
}

// TestExecute_WithFormatFlag tests the format flag bypasses normal output.
func TestExecute_WithFormatFlag(t *testing.T) {
	// Save original version.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "v1.0.0"

	// Initialize I/O context for tests.
	ioCtx, err := iolib.NewContext()
	assert.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := NewMockVersionExecutor(ctrl)

	// When format is specified, printStyledText and printMessage should NOT be called.
	v := versionExec{
		atmosConfig:                &schema.AtmosConfiguration{},
		printStyledText:            mockExec.PrintStyledText,
		printMessage:               mockExec.PrintMessage,
		getLatestGitHubRepoRelease: mockExec.GetLatestGitHubRepoRelease,
	}

	// Test JSON format - should not call printStyledText or printMessage.
	err = v.Execute(false, "json")
	assert.NoError(t, err)

	// Test YAML format - should not call printStyledText or printMessage.
	err = v.Execute(false, "yaml")
	assert.NoError(t, err)
}

// TestGetLatestVersion_DisabledCheck tests that disabled checks return false.
func TestGetLatestVersion_DisabledCheck(t *testing.T) {
	v := versionExec{
		atmosConfig: &schema.AtmosConfiguration{
			Version: schema.Version{
				Check: schema.VersionCheck{
					Enabled: false,
				},
			},
		},
	}

	// With forceCheck=false and check disabled, should return false.
	version, ok := v.GetLatestVersion(false)
	assert.Empty(t, version)
	assert.False(t, ok)
}

// TestCheckRelease_WithVersionTrimming tests version trimming in checkRelease.
func TestCheckRelease_WithVersionTrimming(t *testing.T) {
	// Save original version.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()

	tests := []struct {
		name           string
		currentVersion string
		latestRelease  string
		expectUpgrade  bool
	}{
		{
			name:           "trimming v prefix for comparison",
			currentVersion: "v1.0.0",
			latestRelease:  "v1.0.0",
			expectUpgrade:  false,
		},
		{
			name:           "different versions after trimming",
			currentVersion: "v1.0.0",
			latestRelease:  "v1.1.0",
			expectUpgrade:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.Version = tt.currentVersion

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := NewMockVersionExecutor(ctrl)

			mockExec.EXPECT().GetLatestGitHubRepoRelease().
				Return(tt.latestRelease, nil).AnyTimes()

			if tt.expectUpgrade {
				mockExec.EXPECT().PrintMessageToUpgradeToAtmosLatestRelease(gomock.Any()).Times(1)
			}

			v := versionExec{
				atmosConfig:                               &schema.AtmosConfiguration{},
				getLatestGitHubRepoRelease:                mockExec.GetLatestGitHubRepoRelease,
				printMessageToUpgradeToAtmosLatestRelease: mockExec.PrintMessageToUpgradeToAtmosLatestRelease,
			}

			v.checkRelease()
		})
	}
}
