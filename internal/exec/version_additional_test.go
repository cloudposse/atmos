package exec

import (
	"bytes"
	"encoding/json"
	"runtime/debug"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/version"
)

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

// TestFipsSettingEnabled covers every branch of the GOFIPS140 build-setting
// decision: unset entirely, present but "off", present and empty, and
// present with a real value (e.g. "latest", or a pinned module version).
func TestFipsSettingEnabled(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     bool
	}{
		{
			name:     "no GOFIPS140 setting at all",
			settings: []debug.BuildSetting{{Key: "CGO_ENABLED", Value: "0"}},
			want:     false,
		},
		{
			name:     "GOFIPS140=off",
			settings: []debug.BuildSetting{{Key: "GOFIPS140", Value: "off"}},
			want:     false,
		},
		{
			name:     "GOFIPS140 present but empty",
			settings: []debug.BuildSetting{{Key: "GOFIPS140", Value: ""}},
			want:     false,
		},
		{
			name:     "GOFIPS140=latest",
			settings: []debug.BuildSetting{{Key: "GOFIPS140", Value: "latest"}},
			want:     true,
		},
		{
			name:     "GOFIPS140 pinned to a specific module version",
			settings: []debug.BuildSetting{{Key: "GOFIPS140", Value: "v1.0.0"}},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fipsSettingEnabled(tt.settings))
		})
	}
}

// TestIsFIPSBuild exercises the debug.ReadBuildInfo() integration itself; it
// can only assert that it runs without panicking and returns a bool, since
// whether the running test binary was built with GOFIPS140 depends on how
// the test suite was invoked (see .atmos.d/test.yaml vs a bare `go test`).
// The decision logic itself is covered deterministically by
// TestFipsSettingEnabled above.
func TestIsFIPSBuild(t *testing.T) {
	assert.IsType(t, false, isFIPSBuild())
}

// TestDisplayVersionInFormat_FIPSField verifies the JSON/YAML output's fips
// field reflects isFIPSBuild(), rather than silently carrying the zero value.
func TestDisplayVersionInFormat_FIPSField(t *testing.T) {
	var stdout bytes.Buffer
	streams := &vendorModelTestStreams{stdout: &stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	v := versionExec{
		atmosConfig: &schema.AtmosConfiguration{},
		getLatestGitHubRepoRelease: func() (string, error) {
			return "", errors.New("no update check in this test")
		},
	}

	require.NoError(t, v.displayVersionInFormat(false, "json"))

	var decoded Version
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
	assert.Equal(t, isFIPSBuild(), decoded.FIPS)
}
