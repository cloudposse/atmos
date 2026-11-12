package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
	"github.com/cloudposse/atmos/pkg/schema"
)

type discard struct{}

func (discard) Write(value []byte) (int, error) { return len(value), nil }

func fakeDefinition(t *testing.T, mode string) (*Definition, string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	directory := t.TempDir()
	require.NoError(t, fakeplugin.WriteMode(directory, mode))
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	return &Definition{Package: "example/fake", Executable: executable, Directory: directory, Services: Services{Keyring: storage, LockDirectory: t.TempDir()}}, directory
}

func TestCancelledInvocationKillsPluginAndReleasesSessionLock(t *testing.T) {
	definition, directory := fakeDefinition(t, fakeplugin.ModeHang)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- definition.invoke(ctx, "corporate", "realm", nil, func(operation *invocation) error {
			return operation.handle.Command(ctx, &pluginv1.CommandRequest{Invocation: operation.request, Name: "hang"}, discard{}, discard{})
		})
	}()
	require.Eventually(t, func() bool {
		pids, err := fakeplugin.PIDs(directory)
		return err == nil && len(pids) > 0
	}, 20*time.Second, 20*time.Millisecond)
	pids, err := fakeplugin.PIDs(directory)
	require.NoError(t, err)
	require.True(t, processAlive(pids[0]))

	cancel()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("a cancelled invocation must return")
	}
	require.Eventually(t, func() bool { return !processAlive(pids[0]) }, 20*time.Second, 20*time.Millisecond, "the plugin must not outlive cancellation")

	namespace, err := definition.sessionNamespace("corporate", "realm", nil)
	require.NoError(t, err)
	lockCtx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	acquired := false
	require.NoError(t, definition.transaction(lockCtx, namespace, func(context.Context) error { acquired = true; return nil }))
	require.True(t, acquired, "the session lock must be released after cancellation")
}

func TestInvocationDeadlineSurfacesPluginTimeoutThroughAuthTranslation(t *testing.T) {
	definition, directory := fakeDefinition(t, fakeplugin.ModeHang)
	// Bound the invocation after startup so a slow race-instrumented handshake
	// cannot prevent the test from reaching the auth error translation boundary.
	definition.Timeout = 500 * time.Millisecond
	err := definition.invoke(t.Context(), "corporate", "realm", nil, func(operation *invocation) error {
		require.NotNil(t, operation.request.Deadline)
		deadline, bounded := operation.ctx.Deadline()
		require.True(t, bounded, "the RPC context must enforce the advertised invocation deadline")
		require.Equal(t, operation.request.Deadline.AsTime(), deadline.UTC())
		return operation.handle.Command(operation.ctx, &pluginv1.CommandRequest{Invocation: operation.request, Name: "hang"}, discard{}, discard{})
	})
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.NotEmpty(t, errUtils.AllHints(err))
	pids, pidErr := fakeplugin.PIDs(directory)
	require.NoError(t, pidErr)
	require.NotEmpty(t, pids, "the test must reach the running plugin command")
	require.Eventually(t, func() bool { return !processAlive(pids[0]) }, 20*time.Second, 20*time.Millisecond)
}

func TestCompletedInvocationIsNotReportedAsTimeout(t *testing.T) {
	definition, _ := fakeDefinition(t, fakeplugin.ModeEcho)
	err := definition.invoke(t.Context(), "corporate", "realm", nil, func(operation *invocation) error {
		return operation.handle.Command(t.Context(), &pluginv1.CommandRequest{Invocation: operation.request, Name: "echo"}, discard{}, discard{})
	})
	require.NoError(t, err)
}

const (
	authDeadlineFixtureName = "deadline-fixture"
	authDeadlineRecordFile  = "auth-rpc-deadline.json"
)

type authDeadlineObservation struct {
	PID                int
	RPCDeadline        time.Time
	InvocationDeadline time.Time
}

// hangUntilAuthDeadline records the deadline received by a real RPC before
// waiting for cancellation. A fallback bounds regressions that omit the deadline.
func hangUntilAuthDeadline(ctx context.Context, invocation *pluginv1.Invocation) error {
	deadline, _ := ctx.Deadline()
	observed := authDeadlineObservation{PID: os.Getpid(), RPCDeadline: deadline}
	if advertised := invocation.GetDeadline(); advertised != nil {
		observed.InvocationDeadline = advertised.AsTime()
	}
	record, err := json.Marshal(observed)
	if err != nil {
		return err
	}
	if err := os.WriteFile(authDeadlineRecordFile, record, 0o600); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return status.Error(codes.FailedPrecondition, "auth RPC did not enforce its invocation deadline")
	}
}

func TestAuthRPCsEnforceInvocationDeadline(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  func(context.Context, *provider, *identity) error
	}{
		{"provider authenticate", func(ctx context.Context, p *provider, _ *identity) error {
			_, err := p.Authenticate(ctx)
			return err
		}},
		{"provider logout", func(ctx context.Context, p *provider, _ *identity) error { return p.Logout(ctx) }},
		{"identity authenticate", func(ctx context.Context, _ *provider, i *identity) error {
			_, err := i.Authenticate(ctx, &types.AWSCredentials{
				AccessKeyID: "fixture-access", SecretAccessKey: "fixture-secret", SessionToken: "fixture-session",
				Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
			return err
		}},
		{"identity logout", func(ctx context.Context, _ *provider, i *identity) error { return i.Logout(ctx) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			definition, directory := fakeDefinition(t, fakeplugin.ModeEcho)
			definition.Timeout = 500 * time.Millisecond
			definition.Credentials = credential.New()
			p := &provider{name: authDeadlineFixtureName, realm: "fixture", definition: definition, config: providerConfig(nil)}
			i := &identity{
				name: authDeadlineFixtureName, realm: "fixture", definition: definition, config: &schema.Identity{Kind: "plugin/broker"},
				accepts: []string{"aws/v1"}, materializer: EnvironmentMaterializer(authDeadlineFixtureName, nil),
			}
			err := tt.run(t.Context(), p, i)
			require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.NotEmpty(t, errUtils.AllHints(err))
			record, err := os.ReadFile(filepath.Join(directory, authDeadlineRecordFile))
			require.NoError(t, err, "the actual auth RPC must run before the timeout")
			var observed authDeadlineObservation
			require.NoError(t, json.Unmarshal(record, &observed))
			require.False(t, observed.RPCDeadline.IsZero(), "the auth RPC must receive a bounded context")
			require.False(t, observed.InvocationDeadline.IsZero())
			require.WithinDuration(t, observed.InvocationDeadline, observed.RPCDeadline, 100*time.Millisecond)
			require.Positive(t, observed.PID)
			require.Eventually(t, func() bool { return !processAlive(observed.PID) }, 20*time.Second, 20*time.Millisecond,
				"the plugin must not outlive its auth RPC deadline")
		})
	}
}
