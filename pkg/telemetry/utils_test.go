package telemetry

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	mock_telemetry "github.com/cloudposse/atmos/pkg/telemetry/mock"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestGetTelemetryFromConfig tests the getTelemetryFromConfig function to ensure it properly
// initializes telemetry configuration with default values and maintains consistency across calls.
func TestGetTelemetryFromConfig(t *testing.T) {
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no client creation since telemetry is not actually used in this test.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	// Expect no telemetry events to be sent.
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Test first telemetry instance creation.
	telemetryOne := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetryOne)
	assert.Equal(t, telemetryOne.isEnabled, enabled)
	assert.NotEmpty(t, telemetryOne.token)
	assert.NotEmpty(t, telemetryOne.endpoint)
	assert.NotEmpty(t, telemetryOne.distinctId)
	assert.NotNil(t, telemetryOne.clientProvider)

	// Test second telemetry instance creation - should be identical to first.
	telemetryTwo := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetryTwo)
	assert.Equal(t, telemetryTwo.isEnabled, telemetryOne.isEnabled)
	assert.Equal(t, telemetryTwo.token, telemetryOne.token)
	assert.Equal(t, telemetryTwo.endpoint, telemetryOne.endpoint)
	assert.Equal(t, telemetryTwo.distinctId, telemetryOne.distinctId)
	assert.NotNil(t, telemetryTwo.clientProvider)
}

// TestCaptureCmdString tests capturing command telemetry with string command and CI environment.
func TestCaptureCmdString(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	atmosProWorkspaceID := fmt.Sprintf("ws_%s", uuid.New().String())

	installationId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = installationId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client to be created once.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	// Expect telemetry event to be captured with CI environment details.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: installationId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", "test-cmd").
			Set("ci", true).
			Set("ci_provider", "").
			Set("atmos_pro_workspace_id", atmosProWorkspaceID).
			Set("docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Set up CI environment and workspace ID.
	t.Setenv("CI", "true")
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", atmosProWorkspaceID)
	captureCmdString("test-cmd", nil, mockClientProvider.NewMockClient)
}

// TestCaptureCmdErrorString tests capturing command telemetry when an error occurs.
func TestCaptureCmdErrorString(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	installationId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = installationId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client to be created once.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	// Expect telemetry event to be captured with error flag set to true.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: installationId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", true).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", "test-cmd").
			Set("ci", false).
			Set("ci_provider", "").
			Set("atmos_pro_workspace_id", "").
			Set("docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)
	captureCmdString("test-cmd", errors.New("test-error"), mockClientProvider.NewMockClient)
}

// TestCaptureCmdStringDisabledWithEnvvar tests that telemetry is disabled when ATMOS_TELEMETRY_ENABLED=false.
func TestCaptureCmdStringDisabledWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no client creation since telemetry is disabled.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	// Expect no telemetry events to be sent.
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Disable telemetry via environment variable
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmdString("test-cmd", nil, mockClientProvider.NewMockClient)
}

// TestCaptureCmdFailureStringDisabledWithEnvvar tests that error telemetry is also disabled when ATMOS_TELEMETRY_ENABLED=false.
func TestCaptureCmdFailureStringDisabledWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no client creation since telemetry is disabled.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	// Expect no telemetry events to be sent.
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Disable telemetry via environment variable.
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmdString("test-cmd", errors.New("test-error"), mockClientProvider.NewMockClient)
}

// TestGetTelemetryFromConfigTokenWithEnvvar tests telemetry configuration with custom token, endpoint, and enabled status via environment variables.
func TestGetTelemetryFromConfigTokenWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	enabled := false
	token := uuid.New().String()
	endpoint := uuid.New().String()

	installationId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = installationId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	t.Setenv("ATMOS_TELEMETRY_TOKEN", token)
	t.Setenv("ATMOS_TELEMETRY_ENABLED", strconv.FormatBool(enabled))
	t.Setenv("ATMOS_TELEMETRY_ENDPOINT", endpoint)
	telemetry := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetry)
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, installationId)
	assert.NotNil(t, telemetry.clientProvider)
}

// TestGetTelemetryFromConfigIntergration tests the integration of telemetry configuration
// by creating a telemetry instance with default settings and verifying all required
// fields are properly initialized.
func TestGetTelemetryFromConfigIntergration(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	enabled := true

	// Create telemetry instance with default configuration.
	telemetry := getTelemetryFromConfig()

	// Verify telemetry instance was created successfully with all required fields.
	assert.NotNil(t, telemetry)
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.NotEmpty(t, telemetry.token)
	assert.NotEmpty(t, telemetry.endpoint)
	assert.NotEmpty(t, telemetry.distinctId)
	assert.NotNil(t, telemetry.clientProvider)
}

// TestCaptureCmd tests the captureCmd function for successful command execution
// by setting up mock expectations and verifying telemetry data is captured correctly.
func TestCaptureCmd(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Generate unique installation ID for testing.
	installationId := uuid.New().String()

	// Load and update cache configuration with installation ID.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = installationId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client provider to be called once with any parameters.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	// Create test command with unique name.
	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	// Expect telemetry capture with specific properties for successful command execution.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: installationId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", cmd.CommandPath()).
			Set("ci", true).
			Set("ci_provider", "JENKINS").
			Set("atmos_pro_workspace_id", "").
			Set("docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Set Jenkins CI environment and test command capture.
	t.Setenv("JENKINS_URL", "https://jenkins.example.com")
	captureCmd(cmd, nil, mockClientProvider.NewMockClient)
}

// TestCaptureCmdError tests the captureCmd function for failed command execution
// by setting up mock expectations and verifying error telemetry data is captured correctly.
func TestCaptureCmdError(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create test command with unique name.
	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	// Generate unique Atmos Pro workspace ID for testing.
	atmosProWorkspaceID := fmt.Sprintf("ws_%s", uuid.New().String())

	// Generate unique installation ID for testing.
	installationId := uuid.New().String()

	// Load and update cache configuration with installation ID.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = installationId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client provider to be called once with any parameters.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	// Expect telemetry capture with error properties for failed command execution.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: installationId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", true).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", cmd.CommandPath()).
			Set("ci", true).
			Set("ci_provider", "GITHUB_ACTIONS").
			Set("atmos_pro_workspace_id", atmosProWorkspaceID).
			Set("docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Set CI environment variables and test error command capture.
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", atmosProWorkspaceID)
	captureCmd(cmd, errors.New("test-error"), mockClientProvider.NewMockClient)
}

// TestCaptureCmdDisabledWithEnvvar tests that telemetry is disabled when
// ATMOS_TELEMETRY_ENABLED environment variable is set to false.
func TestCaptureCmdDisabledWithEnvvar(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no calls to client provider or client methods when telemetry is disabled.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Create test command with unique name.
	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	// Disable telemetry via environment variable and test command capture.
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmd(cmd, nil, mockClientProvider.NewMockClient)
}

// TestCaptureCmdFailureDisabledWithEnvvar tests that telemetry is disabled for failed commands
// when ATMOS_TELEMETRY_ENABLED environment variable is set to false.
func TestCaptureCmdFailureDisabledWithEnvvar(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no calls to client provider or client methods when telemetry is disabled.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Create test command with unique name.
	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	// Disable telemetry via environment variable and test error command capture.
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmd(cmd, errors.New("test-error"), mockClientProvider.NewMockClient)
}

// TestTelemetryDisclosureMessage tests the disclosure message functionality when telemetry disclosure
// has not been shown before. It verifies that the first call returns the expected disclosure message
// and subsequent calls return empty strings (indicating the disclosure has been marked as shown).
func TestTelemetryDisclosureMessage(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and ensure telemetry disclosure is set to not shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// First call should return the disclosure message.
	message1 := disclosureMessage()
	assert.NotEmpty(t, message1)
	assert.Equal(t, message1, telemetryNoticeMarkdown)

	// Second call should return empty string since disclosure has been marked as shown.
	message2 := disclosureMessage()
	assert.Empty(t, message2)
}

// TestTelemetryDisclosureMessageShown tests that no disclosure message is returned when
// the telemetry disclosure has already been shown to the user.
func TestTelemetryDisclosureMessageShown(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and set telemetry war	ning as already shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = true
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Should return empty string since disclosure has already been shown.
	message := disclosureMessage()
	assert.Empty(t, message)
}

// TestTelemetryDisclosureMessageHideForCI tests that disclosure messages are suppressed
// when running in a CI environment (when CI environment variable is set to "true").
func TestTelemetryDisclosureMessageHideForCI(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and ensure telemetry disclosure is set to not shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Set CI environment variable to simulate CI environment.
	t.Setenv("CI", "true")
	// Should return empty string when running in CI environment.
	message := disclosureMessage()
	assert.Empty(t, message)
}

// TestTelemetryDisclosureMessageHideIfTelemetryDisabled tests that disclosure messages are suppressed
// when telemetry is disabled.
func TestTelemetryDisclosureMessageHideIfTelemetryDisabled(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and ensure telemetry disclosure is set to not shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Disable telemetry via environment variable.
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	// Should return empty string when telemetry is disabled.
	message := disclosureMessage()
	assert.Empty(t, message)
}

// TestGetTelemetryFromConfigWithLoggingEnabled tests that logging can be enabled via environment variable.
func TestGetTelemetryFromConfigWithLoggingEnabled(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no client creation since we're just testing config loading.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Enable logging via environment variable.
	t.Setenv("ATMOS_TELEMETRY_LOGGING", "true")

	// Get telemetry configuration.
	telemetry := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	// Verify logging is enabled.
	assert.NotNil(t, telemetry)
	assert.True(t, telemetry.logging, "Logging should be enabled when ATMOS_TELEMETRY_LOGGING=true")
}

// TestGetTelemetryFromConfigWithLoggingDisabled tests that logging can be disabled via environment variable.
func TestGetTelemetryFromConfigWithLoggingDisabled(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no client creation since we're just testing config loading.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Explicitly disable logging via environment variable.
	t.Setenv("ATMOS_TELEMETRY_LOGGING", "false")

	// Get telemetry configuration.
	telemetry := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	// Verify logging is disabled.
	assert.NotNil(t, telemetry)
	assert.False(t, telemetry.logging, "Logging should be disabled when ATMOS_TELEMETRY_LOGGING=false")
}

// TestGetTelemetryFromConfigWithLoggingDefault tests the default logging configuration.
func TestGetTelemetryFromConfigWithLoggingDefault(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect no client creation since we're just testing config loading.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Ensure ATMOS_TELEMETRY_LOGGING is not set (not needed with t.Setenv in other tests).

	// Get telemetry configuration.
	telemetry := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	// Verify logging defaults to false (as configured in atmos.yaml).
	assert.NotNil(t, telemetry)
	assert.False(t, telemetry.logging, "Logging should default to false")
}

// TestCaptureCmdWithLoggingEnabled tests that telemetry capture works with logging enabled.
func TestCaptureCmdWithLoggingEnabled(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Set up expectations for successful capture with logging enabled.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Enable logging via environment variable.
	t.Setenv("ATMOS_TELEMETRY_LOGGING", "true")

	// Capture telemetry event.
	captureCmdString("test-cmd-with-logging", nil, mockClientProvider.NewMockClient)
}

// TestCaptureCmdWithLoggingDisabled tests that telemetry capture works with logging disabled.
func TestCaptureCmdWithLoggingDisabled(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Set up expectations for successful capture with logging disabled.
	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)
	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Explicitly disable logging via environment variable.
	t.Setenv("ATMOS_TELEMETRY_LOGGING", "false")

	// Capture telemetry event.
	captureCmdString("test-cmd-no-logging", nil, mockClientProvider.NewMockClient)
}

// TestPrintTelemetryDisclosure tests that the telemetry disclosure message
// is properly printed to stderr with markdown formatting.
func TestPrintTelemetryDisclosure(t *testing.T) {
	// Save original stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Save original CI env vars
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Clean up test cache
	cacheDir := "./.atmos"
	os.RemoveAll(cacheDir)
	defer os.RemoveAll(cacheDir)

	// Initialize markdown renderer for testing
	utils.InitializeMarkdown(schema.AtmosConfiguration{})

	// Call PrintTelemetryDisclosure
	PrintTelemetryDisclosure()

	// Close the writer and restore stderr
	w.Close()
	os.Stderr = oldStderr

	// Read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify the output contains the telemetry disclosure message
	// In a non-TTY environment, it will be plain text
	assert.Contains(t, output, "Notice:")
	assert.Contains(t, output, "Telemetry Enabled")
	assert.Contains(t, output, "Atmos now collects anonymous telemetry")
	assert.Contains(t, output, "https://atmos.tools/cli/telemetry")
}

// TestPrintTelemetryDisclosureOnlyOnce tests that the telemetry disclosure
// message is only shown once and not on subsequent calls.
func TestPrintTelemetryDisclosureOnlyOnce(t *testing.T) {
	// Save original CI env vars
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Clean up test cache - get the actual cache file path
	cacheFilePath, _ := cfg.GetCacheFilePath()
	cacheDir := filepath.Dir(cacheFilePath)
	os.RemoveAll(cacheDir)
	defer os.RemoveAll(cacheDir)

	// Initialize markdown renderer for testing
	utils.InitializeMarkdown(schema.AtmosConfiguration{})

	// First call should show the message
	oldStderr := os.Stderr
	r1, w1, _ := os.Pipe()
	os.Stderr = w1
	PrintTelemetryDisclosure()
	w1.Close()
	os.Stderr = oldStderr

	var buf1 bytes.Buffer
	io.Copy(&buf1, r1)
	firstOutput := buf1.String()

	// Verify first call shows the message
	assert.Contains(t, firstOutput, "Notice:")
	assert.Contains(t, firstOutput, "Telemetry Enabled")

	// Second call should NOT show the message
	r2, w2, _ := os.Pipe()
	os.Stderr = w2
	PrintTelemetryDisclosure()
	w2.Close()
	os.Stderr = oldStderr

	var buf2 bytes.Buffer
	io.Copy(&buf2, r2)
	secondOutput := buf2.String()

	// Verify second call does not show the message
	assert.NotContains(t, secondOutput, "Notice:")
	assert.NotContains(t, secondOutput, "Telemetry Enabled")
}

// TestPrintTelemetryDisclosureDisabledInCI tests that the telemetry disclosure
// message is not shown when running in a CI environment.
func TestPrintTelemetryDisclosureDisabledInCI(t *testing.T) {
	// Save original CI env vars
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Set CI environment variable
	t.Setenv("CI", "true")

	// Clean up test cache
	cacheDir := "./.atmos"
	os.RemoveAll(cacheDir)
	defer os.RemoveAll(cacheDir)

	// Initialize markdown renderer for testing
	utils.InitializeMarkdown(schema.AtmosConfiguration{})

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	PrintTelemetryDisclosure()
	w.Close()
	os.Stderr = oldStderr

	// Read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify no telemetry disclosure is shown in CI
	assert.NotContains(t, output, "Notice:")
	assert.NotContains(t, output, "Telemetry Enabled")
}

// TestPrintTelemetryDisclosureDisabledByConfig tests that the telemetry disclosure
// message is not shown when telemetry is disabled via environment variable.
func TestPrintTelemetryDisclosureDisabledByConfig(t *testing.T) {
	// Save original CI env vars
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Disable telemetry
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")

	// Clean up test cache
	cacheDir := "./.atmos"
	os.RemoveAll(cacheDir)
	defer os.RemoveAll(cacheDir)

	// Initialize markdown renderer for testing
	utils.InitializeMarkdown(schema.AtmosConfiguration{})

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	PrintTelemetryDisclosure()
	w.Close()
	os.Stderr = oldStderr

	// Read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify no telemetry disclosure is shown when disabled
	assert.NotContains(t, output, "Notice:")
	assert.NotContains(t, output, "Telemetry Enabled")
}
