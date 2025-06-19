package telemetry

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"

	"math/rand/v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	mock_telemetry "github.com/cloudposse/atmos/pkg/telemetry/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/gruntwork-io/go-commons/version"
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

	telemetryOne := GetTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetryOne)
	assert.Equal(t, telemetryOne.isEnabled, enabled)
	assert.NotEmpty(t, telemetryOne.token)
	assert.NotEmpty(t, telemetryOne.endpoint)
	assert.NotEmpty(t, telemetryOne.distinctId)
	assert.NotNil(t, telemetryOne.clientProvider)

	telemetryTwo := GetTelemetryFromConfig(mockClientProvider.NewMockClient)

	assert.NotNil(t, telemetryTwo)
	assert.Equal(t, telemetryTwo.isEnabled, telemetryOne.isEnabled)
	assert.Equal(t, telemetryTwo.token, telemetryOne.token)
	assert.Equal(t, telemetryTwo.endpoint, telemetryOne.endpoint)
	assert.Equal(t, telemetryTwo.distinctId, telemetryOne.distinctId)
	assert.NotNil(t, telemetryTwo.clientProvider)
}

func TestCaptureCmdString(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosInstanceId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = atmosInstanceId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: atmosInstanceId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.GetVersion()).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", "test-cmd"),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)
	CaptureCmdString("test-cmd", mockClientProvider.NewMockClient)
}

func TestCaptureCmdErrorString(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosInstanceId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = atmosInstanceId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: atmosInstanceId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", true).
			Set("version", version.GetVersion()).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", "test-cmd"),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)
	CaptureCmdFailureString("test-cmd", mockClientProvider.NewMockClient)
}

func TestCaptureCmdStringDisabledWithEnvvar(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	CaptureCmdString("test-cmd", mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

func TestCaptureCmdFailureStringDisabledWithEnvvar(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	os.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	CaptureCmdFailureString("test-cmd", mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

func TestGetTelemetryFromConfigTokenWithEnvvar(t *testing.T) {
	enabled := false
	token := uuid.New().String()
	endpoint := uuid.New().String()

	atmosInstanceId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = atmosInstanceId
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
	telemetry := GetTelemetryFromConfig(mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_TOKEN")
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
	os.Unsetenv("ATMOS_TELEMETRY_ENDPOINT")

	assert.NotNil(t, telemetry)
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, atmosInstanceId)
	assert.NotNil(t, telemetry.clientProvider)
}

func TestGetTelemetryFromConfigIntergration(t *testing.T) {
	enabled := true

	telemetry := GetTelemetryFromConfig()

	assert.NotNil(t, telemetry)
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.NotEmpty(t, telemetry.token)
	assert.NotEmpty(t, telemetry.endpoint)
	assert.NotEmpty(t, telemetry.distinctId)
	assert.NotNil(t, telemetry.clientProvider)
}

func TestCaptureCmd(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosInstanceId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = atmosInstanceId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: atmosInstanceId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.GetVersion()).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", cmd.CommandPath()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)
	CaptureCmd(cmd, mockClientProvider.NewMockClient)
}

func TestCaptureCmdError(t *testing.T) {
	ctrl := gomock.NewController(t)

	cmd := &cobra.Command{
		Use: fmt.Sprintf("test-cmd-%d", rand.IntN(10000)),
	}

	atmosInstanceId := uuid.New().String()

	cacheCfg, err := cfg.LoadCache()
	assert.NoError(t, err)

	cacheCfg.InstallationId = atmosInstanceId
	saveErr := cfg.SaveCache(cacheCfg)
	assert.NoError(t, saveErr)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(gomock.Any(), gomock.Any()).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: atmosInstanceId,
		Event:      "command",
		Properties: posthog.NewProperties().
			Set("error", true).
			Set("version", version.GetVersion()).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("command", cmd.CommandPath()),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)
	CaptureCmdFailure(cmd, mockClientProvider.NewMockClient)
}

func TestCaptureCmdDisabledWithEnvvar(t *testing.T) {
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
	CaptureCmd(cmd, mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}

func TestCaptureCmdFailureDisabledWithEnvvar(t *testing.T) {
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
	CaptureCmdFailure(cmd, mockClientProvider.NewMockClient)
	os.Unsetenv("ATMOS_TELEMETRY_ENABLED")
}
