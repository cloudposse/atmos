package rpc_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/github"
	"github.com/cloudposse/atmos/plugins/ssh/authority/self"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/rpc"
	"github.com/cloudposse/atmos/plugins/ssh/service"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

type server struct {
	provider pluginv1.AuthProviderClient
	identity pluginv1.AuthIdentityClient
	commands pluginv1.CommandsClient
	paths    *paths.Paths
	clock    *testfakes.Clock
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("unexpected network call to %s", r.URL)
	return nil, errors.New("no network in tests")
}

func start(t *testing.T, runner rpc.Runner) *server {
	t.Helper()
	p := testfakes.Paths(t)
	clock := testfakes.NewClock()
	mgr := &testfakes.Manager{Version: "0.1.0"}
	reg := authority.NewRegistry()
	require.NoError(t, reg.Register(self.Factory))
	require.NoError(t, reg.Register(github.Factory))
	svc := identity.New(identity.Deps{
		Paths:       func(string) (*paths.Paths, error) { return p, nil },
		OpenStore:   testfakes.FileOpener,
		Authorities: reg,
		Daemon: daemon.Deps{
			Service:           func(*paths.Paths) (service.Manager, error) { return mgr, nil },
			InstallExecutable: service.StablePath,
			Version:           "0.1.0",
		},
		CheckSocket:   func(context.Context, string, ssh.PublicKey) error { return nil },
		HTTPClient:    &http.Client{Transport: failTransport{t}},
		State:         testfakes.NewState(),
		Now:           clock.Now,
		Hostname:      func() (string, error) { return "host", nil },
		HomeDir:       func() (string, error) { return "/home/u", nil },
		FindSSHKeygen: func() (string, error) { return "/usr/bin/ssh-keygen", nil },
		Sleep:         clock.Sleep,
	})
	grpcServer := grpc.NewServer()
	pluginv1.RegisterAuthProviderServer(grpcServer, &rpc.Provider{Now: clock.Now})
	pluginv1.RegisterAuthIdentityServer(grpcServer, rpc.NewIdentity(svc))
	pluginv1.RegisterCommandsServer(grpcServer, rpc.NewCommands(runner))
	listener := bufconn.Listen(1 << 20)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///ssh", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &server{
		provider: pluginv1.NewAuthProviderClient(conn), identity: pluginv1.NewAuthIdentityClient(conn),
		commands: pluginv1.NewCommandsClient(conn), paths: p, clock: clock,
	}
}

func object(t *testing.T, v map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(v)
	require.NoError(t, err)
	return s
}

func identitySpec(t *testing.T, extra map[string]any) *structpb.Struct {
	spec := map[string]any{"store": "file", "allow": []any{"signing", "authentication"}}
	for k, v := range extra {
		spec[k] = v
	}
	return object(t, spec)
}

func only(t *testing.T, stream grpc.ServerStreamingClient[pluginv1.AuthenticateEvent]) *pluginv1.Credentials {
	t.Helper()
	event, err := stream.Recv()
	require.NoError(t, err)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
	return event.GetResult()
}

func TestSelfProviderThenIdentityAcrossRPC(t *testing.T) {
	s := start(t, nil)
	ctx := t.Context()
	provider := &pluginv1.ProviderRequest{Invocation: &pluginv1.Invocation{Id: "p"}, Name: "keys", Kind: rpc.KindSelf, Spec: object(t, nil)}
	_, err := s.provider.Validate(ctx, provider)
	require.NoError(t, err)
	stream, err := s.provider.Authenticate(ctx, provider)
	require.NoError(t, err)
	upstream := only(t, stream)

	assert.Equal(t, "secret-values/v1", upstream.GetType())
	assert.Equal(t, map[string]any{"assertion": "self"}, upstream.GetData().AsMap(), "never empty, and says it is self-asserted")
	assert.Equal(t, s.clock.Now().Add(rpc.SelfLifetime), upstream.GetExpiresAt().AsTime())

	request := &pluginv1.IdentityRequest{Invocation: &pluginv1.Invocation{Id: "i"}, Name: "ops", Kind: rpc.KindKey, Spec: identitySpec(t, map[string]any{"user": "deploy"})}
	_, err = s.identity.Validate(ctx, request)
	require.NoError(t, err)
	out, err := s.identity.Authenticate(ctx, &pluginv1.IdentityAuthenticateRequest{Identity: request, Upstream: upstream})
	require.NoError(t, err)
	creds := only(t, out)

	assert.Equal(t, "environment/v1", creds.GetType())
	env := creds.GetData().AsMap()
	assert.Contains(t, env, "SSH_AUTH_SOCK")
	assert.Contains(t, env, "GIT_CONFIG_GLOBAL")
	assert.Equal(t, "deploy", env["ANSIBLE_REMOTE_USER"])
	assert.Equal(t, s.clock.Now().Add(identity.CredentialLifetime), creds.GetExpiresAt().AsTime())
	assert.False(t, creds.GetExpiresAt().AsTime().IsZero(), "the host rejects a credential without an expiry")

	_, err = s.identity.Logout(ctx, request)
	require.NoError(t, err)
	_, err = s.provider.Logout(ctx, provider)
	require.NoError(t, err)
}

func TestSelfProviderTakesNoConfiguration(t *testing.T) {
	s := start(t, nil)
	request := &pluginv1.ProviderRequest{Spec: object(t, map[string]any{"anything": "x"})}

	_, err := s.provider.Validate(t.Context(), request)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	stream, err := s.provider.Authenticate(t.Context(), request)
	require.NoError(t, err)
	_, err = stream.Recv()
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestIdentityValidateRejectsABadSpec(t *testing.T) {
	s := start(t, nil)

	_, err := s.identity.Validate(t.Context(), &pluginv1.IdentityRequest{Name: "ops", Spec: object(t, map[string]any{"store": "file"})})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestIdentityRejectsAnUpstreamOfTheWrongType(t *testing.T) {
	s := start(t, nil)
	request := &pluginv1.IdentityRequest{Name: "ops", Spec: identitySpec(t, nil)}

	out, err := s.identity.Authenticate(t.Context(), &pluginv1.IdentityAuthenticateRequest{
		Identity: request, Upstream: &pluginv1.Credentials{Type: "oidc/v1", Data: object(t, nil)},
	})
	require.NoError(t, err)
	_, err = out.Recv()

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestARejectedTokenIsUnauthenticatedAndTheDetailIsInTheStatusFile(t *testing.T) {
	s := start(t, nil)
	request := &pluginv1.IdentityRequest{Name: "ops", Spec: identitySpec(t, map[string]any{"authority": "github"})}

	out, err := s.identity.Authenticate(t.Context(), &pluginv1.IdentityAuthenticateRequest{
		Identity: request, Upstream: &pluginv1.Credentials{Type: "secret-values/v1", Data: object(t, map[string]any{"assertion": "self"})},
	})
	require.NoError(t, err)
	_, err = out.Recv()

	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "atmos ssh key status")
	assert.NotContains(t, status.Convert(err).Message(), "no token", "the detail is for the status file, not the wire")
}

func TestStatusErrorMapsEveryCodeToWhatTheHostUnderstands(t *testing.T) {
	tests := map[identity.Code]codes.Code{
		identity.CodeTokenRejected:          codes.Unauthenticated,
		identity.CodeMissingScope:           codes.Unauthenticated,
		identity.CodeKeyLocked:              codes.FailedPrecondition,
		identity.CodeInvalidSpec:            codes.InvalidArgument,
		identity.CodeRequirementMismatch:    codes.InvalidArgument,
		identity.CodeUnsupportedRequirement: codes.InvalidArgument,
		identity.CodeUnknownAuthority:       codes.InvalidArgument,
		identity.CodeUnsupportedUse:         codes.InvalidArgument,
		identity.CodeAuthorityUnreachable:   codes.Unavailable,
		identity.CodeAgentUnavailable:       codes.Unavailable,
		identity.CodeUnsupportedPlatform:    codes.Unimplemented,
		identity.CodeCanceled:               codes.Canceled,
		identity.CodeInternal:               codes.Internal,
	}
	for code, want := range tests {
		t.Run(string(code), func(t *testing.T) {
			err := rpc.StatusError(&identity.Error{Code: code, Message: "secret detail"})

			assert.Equal(t, want, status.Code(err))
			assert.NotContains(t, err.Error(), "secret detail")
		})
	}
	assert.Equal(t, codes.Internal, status.Code(rpc.StatusError(errors.New("anything else"))))
}

type fakeRunner struct {
	stdout, stderr []byte
	code           int
	gotArgs        []string
	gotSettings    map[string]any
}

func (f *fakeRunner) Run(_ context.Context, args []string, settings map[string]any, stdout, stderr io.Writer) int {
	f.gotArgs, f.gotSettings = args, settings
	_, _ = stdout.Write(f.stdout)
	_, _ = stderr.Write(f.stderr)
	_, _ = stdout.Write(nil)
	return f.code
}

func collect(t *testing.T, stream grpc.ServerStreamingClient[pluginv1.CommandEvent]) (stdout, stderr []byte, exits []uint32, maxChunk int) {
	t.Helper()
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stdout, stderr, exits, maxChunk
		}
		require.NoError(t, err)
		switch e := event.GetEvent().(type) {
		case *pluginv1.CommandEvent_Stdout:
			require.NotEmpty(t, e.Stdout, "an empty chunk is a protocol error")
			stdout = append(stdout, e.Stdout...)
			maxChunk = max(maxChunk, len(e.Stdout))
		case *pluginv1.CommandEvent_Stderr:
			require.NotEmpty(t, e.Stderr)
			stderr = append(stderr, e.Stderr...)
			maxChunk = max(maxChunk, len(e.Stderr))
		case *pluginv1.CommandEvent_Exit:
			exits = append(exits, e.Exit.GetCode())
		}
	}
}

func TestCommandStreamsOutputInProtocolSizedChunksAndEndsWithOneExit(t *testing.T) {
	big := make([]byte, 150_000)
	for i := range big {
		big[i] = byte('a' + i%26)
	}
	runner := &fakeRunner{stdout: big, stderr: []byte("warning\n"), code: 3}
	s := start(t, runner)

	stream, err := s.commands.Run(t.Context(), &pluginv1.CommandRequest{
		Name: "ssh", Arguments: []string{"key", "list"}, Settings: object(t, map[string]any{"root": "/data"}),
	})
	require.NoError(t, err)
	stdout, stderr, exits, maxChunk := collect(t, stream)

	assert.Equal(t, big, stdout)
	assert.Equal(t, "warning\n", string(stderr))
	assert.Equal(t, []uint32{3}, exits, "exactly one exit event")
	assert.LessOrEqual(t, maxChunk, 64<<10)
	assert.Equal(t, []string{"key", "list"}, runner.gotArgs)
	assert.Equal(t, map[string]any{"root": "/data"}, runner.gotSettings)
}

func TestCommandExitCodesAreClampedToTheProtocolRange(t *testing.T) {
	for in, want := range map[int]uint32{0: 0, 1: 1, 255: 255, 256: 255, -1: 255, 4000: 255} {
		s := start(t, &fakeRunner{code: in})

		stream, err := s.commands.Run(t.Context(), &pluginv1.CommandRequest{})
		require.NoError(t, err)
		_, _, exits, _ := collect(t, stream)

		assert.Equal(t, []uint32{want}, exits, "exit %d", in)
	}
}

func TestACommandWithNoOutputStillEndsWithAnExit(t *testing.T) {
	s := start(t, &fakeRunner{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	stream, err := s.commands.Run(ctx, &pluginv1.CommandRequest{})
	require.NoError(t, err)
	_, _, exits, _ := collect(t, stream)

	assert.Equal(t, []uint32{0}, exits)
}
