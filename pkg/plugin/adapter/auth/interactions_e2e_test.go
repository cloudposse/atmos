package auth

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	deviceFixtureName          = "device-fixture"
	deviceFixtureQuietName     = "device-fixture-noninteractive"
	deviceFixtureMalformedName = "device-fixture-malformed"
	deviceFixtureURI           = "https://issuer.example/device"
	deviceFixtureCode          = "WXYZ-2345"
	deviceFixtureLeak          = "synthetic-private-device-secret"
)

// deviceFixtureFields maps provider names to the interaction fields the fake
// plugin sends before its credential result.
var deviceFixtureFields = map[string]map[string]string{
	deviceFixtureName:      {"verification_uri": deviceFixtureURI, "user_code": deviceFixtureCode},
	deviceFixtureQuietName: {"verification_uri": deviceFixtureURI, "user_code": deviceFixtureCode},
	// A complete, token-bearing URI must never be rendered or echoed in errors.
	deviceFixtureMalformedName: {"verification_uri": deviceFixtureURI + "?code=" + deviceFixtureLeak, "user_code": deviceFixtureCode},
}

// deviceFixtureInteractive is the interactive policy the host must forward to the plugin in the
// invocation for each provider name. The child process checks it, so a dropped or altered
// invocation fails the call instead of being masked by the host's local gate.
var deviceFixtureInteractive = map[string]bool{
	deviceFixtureName:          true,
	deviceFixtureQuietName:     false,
	deviceFixtureMalformedName: true,
}

// sendDeviceThenCredentials emits one device-code interaction and then an
// OIDC credential result, the sequence a real device-flow plugin produces.
func sendDeviceThenCredentials(stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent], fields map[string]string) error {
	if err := stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Interaction{Interaction: &pluginv1.Interaction{
		Kind: deviceCodeInteractionKind, Title: deviceFixtureLeak, Fields: fields,
	}}}); err != nil {
		return err
	}
	return sendDeviceCredentials(stream)
}

// sendDeviceCredentials emits the OIDC credential result that completes a device flow.
func sendDeviceCredentials(stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	// The envelope lifetime must not outlive the token, so both share one expiry.
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	data, err := structpb.NewStruct(map[string]any{
		"issuer": "https://issuer.example", "subject": "device-subject", "client_id": "device-client",
		"access_token": "device-access-token", "token_type": "Bearer",
		"access_token_expires_at": expiry.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: &pluginv1.Credentials{
		Type: "oidc/v1", Data: data, ExpiresAt: timestamppb.New(expiry),
	}}})
}

// captureUIStreams routes the global data and UI writers to buffers for one test.
func captureUIStreams(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "false")
	t.Setenv("ATMOS_FORCE_COLOR", "false")
	t.Setenv("NO_COLOR", "1")
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	previous := iolib.GetContext()
	streams := NewMockStreams(gomock.NewController(t))
	streams.EXPECT().RawOutput().Return(stdout).AnyTimes()
	streams.EXPECT().RawError().Return(stderr).AnyTimes()
	ctx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(func() {
		data.InitWriter(previous)
		ui.InitFormatter(previous)
	})
	return stdout, stderr
}

// TestDeviceInteractionThroughRealPluginTransport drives a device-code
// interaction over the real gRPC plugin transport, so the host's interactive
// policy must reach the invocation and gate what is rendered.
func TestDeviceInteractionThroughRealPluginTransport(t *testing.T) {
	for _, tt := range []struct {
		name        string
		provider    string
		interactive bool
		wantErr     error
		wantPrompt  bool
	}{
		{name: "interactive host renders the prompt on stderr", provider: deviceFixtureName, interactive: true, wantPrompt: true},
		{name: "noninteractive host refuses the prompt", provider: deviceFixtureQuietName, interactive: false, wantErr: errUtils.ErrPluginInteractionRequired},
		{name: "token-bearing URI is rejected without echoing it", provider: deviceFixtureMalformedName, interactive: true, wantErr: errUtils.ErrPluginProtocol},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureUIStreams(t)
			definition, _ := fakeDefinition(t, fakeplugin.ModeEcho)
			definition.Outputs = []string{"oidc/v1"}
			definition.Credentials = credential.New()
			interactive := tt.interactive
			definition.Services.Interactive = func() bool { return interactive }
			p := &provider{name: tt.provider, realm: "fixture", definition: definition, config: providerConfig(nil)}

			credentials, err := p.Authenticate(t.Context())

			assert.Empty(t, stdout.String(), "device prompts must never reach the data channel")
			assert.NotContains(t, stderr.String(), deviceFixtureLeak, "plugin titles and complete URIs must not be rendered")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.NotContains(t, err.Error(), deviceFixtureLeak)
				assert.Nil(t, credentials)
				assert.NotContains(t, stderr.String(), deviceFixtureCode, "a refused prompt must not be rendered")
				return
			}
			require.NoError(t, err)
			require.IsType(t, &types.PluginCredentials{}, credentials)
			assert.Equal(t, "device-access-token", credentials.(*types.PluginCredentials).Data["access_token"])
			assert.Contains(t, stderr.String(), deviceFixtureURI)
			assert.Contains(t, stderr.String(), deviceFixtureCode)
		})
	}
}
