package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

const (
	// The slow device fixture shows its prompt and then takes three call timeouts before the
	// user "completes" the sign-in, the way a real device flow polls the identity provider.
	slowDeviceFixtureName = "device-slow-fixture"
	// The stalled device fixture shows its prompt and the user never completes the sign-in.
	stalledDeviceFixtureName = "device-stalled-fixture"
	slowDeviceRecordFile     = "device-wait.json"
	interactiveCallTimeout   = 500 * time.Millisecond
	slowDeviceWait           = 3 * interactiveCallTimeout
	// A stalled fixture gives up on its own after this long so a regression cannot hang the suite.
	stalledDeviceFallback = 30 * time.Second
)

// slowDeviceFixtures maps provider names to how long the user takes to complete the sign-in
// after the prompt is shown. Zero means the user never completes it.
var slowDeviceFixtures = map[string]time.Duration{
	slowDeviceFixtureName:    slowDeviceWait,
	stalledDeviceFixtureName: 0,
}

type deviceWaitObservation struct {
	PID         int
	RPCDeadline time.Time
}

// sendDeviceThenWait emits a device prompt, records the deadline the plugin received, waits for
// the simulated user, and then returns the credential result.
func sendDeviceThenWait(stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent], invocation *pluginv1.Invocation, wait time.Duration) error {
	if !invocation.GetInteractive() {
		return status.Error(codes.InvalidArgument, "the slow device fixture requires an interactive invocation")
	}
	deadline, _ := stream.Context().Deadline()
	record, err := json.Marshal(deviceWaitObservation{PID: os.Getpid(), RPCDeadline: deadline})
	if err != nil {
		return err
	}
	if err := os.WriteFile(slowDeviceRecordFile, record, 0o600); err != nil {
		return err
	}
	if err := stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Interaction{Interaction: &pluginv1.Interaction{
		Kind: deviceCodeInteractionKind, Fields: map[string]string{"verification_uri": deviceFixtureURI, "user_code": deviceFixtureCode},
	}}}); err != nil {
		return err
	}
	if wait == 0 {
		wait = stalledDeviceFallback
	}
	select {
	case <-stream.Context().Done():
		return stream.Context().Err()
	case <-time.After(wait):
	}
	return sendDeviceCredentials(stream)
}

// interactiveProvider builds a provider over the real plugin transport with an interactive host.
func interactiveProvider(t *testing.T, name string) (*provider, string) {
	t.Helper()
	captureUIStreams(t)
	definition, directory := fakeDefinition(t, fakeplugin.ModeEcho)
	definition.Outputs = []string{"oidc/v1"}
	definition.Credentials = credential.New()
	definition.Services.Interactive = func() bool { return true }
	definition.Timeout = interactiveCallTimeout
	return &provider{name: name, realm: "fixture", definition: definition, config: providerConfig(nil)}, directory
}

func readDeviceWait(t *testing.T, directory string) deviceWaitObservation {
	t.Helper()
	record, err := os.ReadFile(filepath.Join(directory, slowDeviceRecordFile))
	require.NoError(t, err, "the device fixture must run before the call ends")
	var observed deviceWaitObservation
	require.NoError(t, json.Unmarshal(record, &observed))
	require.Positive(t, observed.PID)
	return observed
}

// TestDeviceSignInLongerThanTimeoutSucceeds proves the user's sign-in time does not count
// against the package timeout: the user takes three timeouts and the login still succeeds.
func TestDeviceSignInLongerThanTimeoutSucceeds(t *testing.T) {
	p, directory := interactiveProvider(t, slowDeviceFixtureName)

	started := time.Now()
	credentials, err := p.Authenticate(t.Context())

	require.NoError(t, err, "time spent waiting for the user must not count against the timeout")
	require.GreaterOrEqual(t, time.Since(started), slowDeviceWait, "the fixture must have waited longer than the timeout")
	require.IsType(t, &types.PluginCredentials{}, credentials)
	require.Equal(t, "device-access-token", credentials.(*types.PluginCredentials).Data["access_token"])
	observed := readDeviceWait(t, directory)
	require.False(t, observed.RPCDeadline.IsZero(), "an interactive call must still carry a hard deadline")
}

// TestInteractiveHangWithoutPromptIsStillCutOffByTimeout proves the timeout still bounds plugin
// work in an interactive call: a plugin that hangs without showing a prompt is stopped after the
// timeout, long before the call's hard deadline.
func TestInteractiveHangWithoutPromptIsStillCutOffByTimeout(t *testing.T) {
	p, directory := interactiveProvider(t, authDeadlineFixtureName)
	p.definition.InteractionLimit = time.Hour

	started := time.Now()
	_, err := p.Authenticate(t.Context())

	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 10*time.Second, "the timeout, not the hard deadline, must stop a hung plugin")
	record, readErr := os.ReadFile(filepath.Join(directory, authDeadlineRecordFile))
	require.NoError(t, readErr, "the auth RPC must run before the timeout")
	var observed authDeadlineObservation
	require.NoError(t, json.Unmarshal(record, &observed))
	require.Greater(t, time.Until(observed.RPCDeadline), 30*time.Minute,
		"an interactive call advertises the hard deadline, so only the host-side budget can stop it this early")
	require.WithinDuration(t, observed.InvocationDeadline, observed.RPCDeadline, 100*time.Millisecond)
	require.Eventually(t, func() bool { return !processAlive(observed.PID) }, 20*time.Second, 20*time.Millisecond,
		"the plugin must not outlive the timeout")
}

// TestStalledSignInIsCutOffByInteractionLimit proves the hard ceiling: a user who never finishes
// signing in is not waited on forever, and the error explains what happened.
func TestStalledSignInIsCutOffByInteractionLimit(t *testing.T) {
	const limit = 2 * interactiveCallTimeout
	p, directory := interactiveProvider(t, stalledDeviceFixtureName)
	p.definition.InteractionLimit = limit
	p.definition.Alias = "corporate"

	started := time.Now()
	credentials, err := p.Authenticate(t.Context())
	elapsed := time.Since(started)

	require.Nil(t, credentials)
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, elapsed, limit, "the user wait must outlast the timeout before the ceiling applies")
	require.Less(t, elapsed, stalledDeviceFallback, "the ceiling, not the fixture's own fallback, must end the wait")
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "sign-in")
	require.Contains(t, hints, "`plugins.packages.corporate.interaction_timeout`", "the hint names the key of this package")
	observed := readDeviceWait(t, directory)
	require.WithinDuration(t, started.Add(interactiveCallTimeout+limit), observed.RPCDeadline, 5*time.Second,
		"the plugin sees the hard deadline: the timeout plus the interaction limit")
	require.Eventually(t, func() bool { return !processAlive(observed.PID) }, 20*time.Second, 20*time.Millisecond,
		"the plugin must not outlive the interaction limit")
}
