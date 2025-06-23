package telemetry

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"strconv"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	mock_telemetry "github.com/cloudposse/atmos/pkg/telemetry/mock"
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
			Set("is_docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Set up CI environment and workspace ID.
	os.Setenv("CI", "true")
	os.Setenv("ATMOS_PRO_WORKSPACE_ID", atmosProWorkspaceID)
	captureCmdString("test-cmd", nil, mockClientProvider.NewMockClient)
	os.Unsetenv("CI")
	os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")
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
			Set("is_docker", isDocker()),
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
	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmdString("test-cmd", nil, mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
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
	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmdString("test-cmd", errors.New("test-error"), mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
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

	os.Setenv("ATMOS_TELEMETRY_TOKEN", token)
	os.Setenv("ATMOS_TELEMETRY_ENABLED", strconv.FormatBool(enabled))
	os.Setenv("ATMOS_TELEMETRY_ENDPOINT", endpoint)
	telemetry := getTelemetryFromConfig(mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_TOKEN")
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
	os.Unsetenv("ATMOS_TELEMETRY_ENDPOINT")

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
			Set("is_docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Set Jenkins CI environment and test command capture.
	os.Setenv("JENKINS_URL", "https://jenkins.example.com")
	captureCmd(cmd, nil, mockClientProvider.NewMockClient)
	os.Unsetenv("JENKINS_URL")
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
			Set("is_docker", isDocker()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Set CI environment variables and test error command capture.
	os.Setenv("CI", "true")
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("ATMOS_PRO_WORKSPACE_ID", atmosProWorkspaceID)
	captureCmd(cmd, errors.New("test-error"), mockClientProvider.NewMockClient)
	os.Unsetenv("CI")
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")
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
	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmd(cmd, nil, mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
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
	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmd(cmd, errors.New("test-error"), mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

// TestTelemetryDisclosureMessage tests the warning message functionality when telemetry warning
// has not been shown before. It verifies that the first call returns the expected warning message
// and subsequent calls return empty strings (indicating the warning has been marked as shown).
func TestTelemetryDisclosureMessage(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and ensure telemetry warning is set to not shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// First call should return the warning message.
	message1 := disclosureMessage()
	assert.NotEmpty(t, message1)
	assert.Equal(t, message1, `
**Attention:** Atmos now collects completely anonymous telemetry regarding usage.
This information is used to shape Atmos roadmap and prioritize features.
You can learn more, including how to opt-out if you'd not like to participate in this anonymous program, by visiting the following URL:
https://atmos.tools/cli/telemetry
`)

	// Second call should return empty string since warning has been marked as shown.
	message2 := disclosureMessage()
	assert.Empty(t, message2)
}

// TestTelemetryDisclosureMessageShown tests that no warning message is returned when
// the telemetry warning has already been shown to the user.
func TestTelemetryDisclosureMessageShown(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and set telemetry warning as already shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = true
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Should return empty string since warning has already been shown.
	message := disclosureMessage()
	assert.Empty(t, message)
}

// TestTelemetryDisclosureMessageHideForCI tests that warning messages are suppressed
// when running in a CI environment (when CI environment variable is set to "true").
func TestTelemetryDisclosureMessageHideForCI(t *testing.T) {
	// Preserve and restore CI environment variables to avoid interference.
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	// Load cache configuration and ensure telemetry warning is set to not shown.
	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryDisclosureShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	// Set CI environment variable to simulate CI environment.
	os.Setenv("CI", "true")
	// Should return empty string when running in CI environment.
	message := disclosureMessage()
	assert.Empty(t, message)
	os.Unsetenv("CI")
}
