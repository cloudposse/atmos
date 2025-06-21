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

func TestGetTelemetryFromConfig(t *testing.T) {
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	telemetryOne := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetryOne)
	assert.Equal(t, telemetryOne.isEnabled, enabled)
	assert.NotEmpty(t, telemetryOne.token)
	assert.NotEmpty(t, telemetryOne.endpoint)
	assert.NotEmpty(t, telemetryOne.distinctId)
	assert.NotNil(t, telemetryOne.clientProvider)

	telemetryTwo := getTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetryTwo)
	assert.Equal(t, telemetryTwo.isEnabled, telemetryOne.isEnabled)
	assert.Equal(t, telemetryTwo.token, telemetryOne.token)
	assert.Equal(t, telemetryTwo.endpoint, telemetryOne.endpoint)
	assert.Equal(t, telemetryTwo.distinctId, telemetryOne.distinctId)
	assert.NotNil(t, telemetryTwo.clientProvider)
}

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

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

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

	os.Setenv("CI", "true")
	os.Setenv("ATMOS_PRO_WORKSPACE_ID", atmosProWorkspaceID)
	captureCmdString("test-cmd", nil, mockClientProvider.NewMockClient)
	os.Unsetenv("CI")
	os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")
}

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

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

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

func TestCaptureCmdStringDisabledWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmdString("test-cmd", nil, mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

func TestCaptureCmdFailureStringDisabledWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmdString("test-cmd", errors.New("test-error"), mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

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

func TestGetTelemetryFromConfigIntergration(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	enabled := true

	telemetry := getTelemetryFromConfig()

	assert.NotNil(t, telemetry)
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.NotEmpty(t, telemetry.token)
	assert.NotEmpty(t, telemetry.endpoint)
	assert.NotEmpty(t, telemetry.distinctId)
	assert.NotNil(t, telemetry.clientProvider)
}

func TestCaptureCmd(t *testing.T) {
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

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}
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

	os.Setenv("JENKINS_URL", "https://jenkins.example.com")
	captureCmd(cmd, nil, mockClientProvider.NewMockClient)
	os.Unsetenv("JENKINS_URL")
}

func TestCaptureCmdError(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	atmosProWorkspaceID := fmt.Sprintf("ws_%s", uuid.New().String())

	installationId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = installationId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

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

	os.Setenv("CI", "true")
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("ATMOS_PRO_WORKSPACE_ID", atmosProWorkspaceID)
	captureCmd(cmd, errors.New("test-error"), mockClientProvider.NewMockClient)
	os.Unsetenv("CI")
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")
}

func TestCaptureCmdDisabledWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmd(cmd, nil, mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

func TestCaptureCmdFailureDisabledWithEnvvar(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	captureCmd(cmd, errors.New("test-error"), mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

func TestTelemetryWarningMessage(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryWarningShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	message1 := warningMessage()
	assert.NotEmpty(t, message1)
	assert.Equal(t, message1, `
Attention: Atmos now collects completely anonymous telemetry regarding usage.
This information is used to shape Atmos roadmap and prioritize features.
You can learn more, including how to opt-out if you'd not like to participate in this anonymous program, by visiting the following URL:
https://atmos.tools/cli/telemetry
`)

	message2 := warningMessage()
	assert.Empty(t, message2)
}

func TestTelemetryWarningMessageShown(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryWarningShown = true
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	message := warningMessage()
	assert.Empty(t, message)
}

func TestTelemetryWarningMessageHideForCI(t *testing.T) {
	currentEnvVars := PreserveCIEnvVars()
	defer RestoreCIEnvVars(currentEnvVars)

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.TelemetryWarningShown = false
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	os.Setenv("CI", "true")
	message := warningMessage()
	assert.Empty(t, message)
	os.Unsetenv("CI")
}
