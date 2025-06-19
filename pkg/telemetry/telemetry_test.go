package telemetry

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"

	mock_telemetry "github.com/cloudposse/atmos/pkg/telemetry/mock"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/golang/mock/gomock"
	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/assert"
)

const (
	TestPosthogIntegrationToken = "phc_5Z678901234567890123456789012345"
)

func TestTelemetryConstructor(t *testing.T) {
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)
}

func TestTelemetryCaptureMethod(t *testing.T) {
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.True(t, captured)
}

func TestTelemetryDisabledCaptureMethod(t *testing.T) {
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := false

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

func TestTelemetryEmptyTokenCaptureMethod(t *testing.T) {
	token := ""
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

func TestTelemetryProviderErrorCaptureMethod(t *testing.T) {
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Return(mockClient, errors.New("provider error")).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

func TestTelemetryEnqueueErrorCaptureMethod(t *testing.T) {
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(errors.New("enqueue error")).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

func TestTelemetryPosthogIntegrationCaptureMethod(t *testing.T) {
	token := TestPosthogIntegrationToken
	endpoint := "https://us.i.posthog.com/"
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	var realPosthogClient posthog.Client

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Do(func(token string, config posthog.Config) {
		var err error
		realPosthogClient, err = posthog.NewWithConfig(token, config)
		if err != nil {
			t.Fatalf("Failed to create real PostHog client: %v", err)
		}
	}).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).DoAndReturn(func(capture posthog.Capture) error {
		return realPosthogClient.Enqueue(capture)
	}).Times(1)
	mockClient.EXPECT().Close().Do(func() {
		realPosthogClient.Close()
	}).Return(nil).Times(1)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)

	assert.NotNil(t, realPosthogClient)
	assert.True(t, captured)
}

func TestTelemetryPosthogIntegrationWrongEndpointCaptureMethod(t *testing.T) {
	token := TestPosthogIntegrationToken
	endpoint := fmt.Sprintf("https://us.i.posthog.com/wrong/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	var realPosthogClient posthog.Client

	ctrl := gomock.NewController(t)

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	mockClientProvider.EXPECT().NewMockClient(token, posthog.Config{
		Endpoint: endpoint,
	}).Do(func(token string, config posthog.Config) {
		var err error
		realPosthogClient, err = posthog.NewWithConfig(token, config)
		if err != nil {
			t.Fatalf("Failed to create real PostHog client: %v", err)
		}
	}).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).DoAndReturn(func(capture posthog.Capture) error {
		return realPosthogClient.Enqueue(capture)
	}).Times(1)
	mockClient.EXPECT().Close().Do(func() {
		realPosthogClient.Close()
	}).Return(nil).Times(1)

	telemetry := NewTelemetry(enabled, token, endpoint, distinctId, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)

	assert.NotNil(t, realPosthogClient)
	// TODO: PostHog Enqueue always returns nil, but we still check errors
	// to handle them if posthog go lib will return them in the future
	assert.True(t, captured)
}
