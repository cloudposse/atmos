package main

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func object(t *testing.T, value map[string]any) *structpb.Struct {
	t.Helper()
	result, err := structpb.NewStruct(value)
	require.NoError(t, err)
	return result
}

func spec(t *testing.T) *structpb.Struct {
	return object(t, map[string]any{"credentials": map[string]any{"token": map[string]any{"store": "team", "key": "github"}}})
}

func principal(t *testing.T) *structpb.Struct {
	return object(t, map[string]any{"environment": map[string]any{"GITHUB_TOKEN": "token"}})
}

func TestReadAndMapAcrossRPC(t *testing.T) {
	mock := NewMocksecretReader(gomock.NewController(t))
	mock.EXPECT().ReadSecret(gomock.Any(), "token").DoAndReturn(func(ctx context.Context, _ string) (string, error) {
		md, _ := metadata.FromOutgoingContext(ctx)
		require.Equal(t, []string{"call"}, md.Get(api.InvocationMetadata))
		return "rotating-secret", nil
	})
	server := grpc.NewServer()
	pluginv1.RegisterAuthProviderServer(server, &provider{secrets: mock})
	pluginv1.RegisterAuthIdentityServer(server, &identity{})
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///plugin", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pluginv1.NewAuthProviderClient(conn)
	request := &pluginv1.ProviderRequest{Invocation: &pluginv1.Invocation{Id: "call"}, Spec: spec(t)}
	_, err = client.Validate(t.Context(), request)
	require.NoError(t, err)
	stream, err := client.Authenticate(t.Context(), request)
	require.NoError(t, err)
	event, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "secret-values/v1", event.GetResult().GetType())
	require.Equal(t, "rotating-secret", event.GetResult().GetData().AsMap()["token"])
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
	identityClient := pluginv1.NewAuthIdentityClient(conn)
	identityRequest := &pluginv1.IdentityRequest{Principal: principal(t)}
	_, err = identityClient.Validate(t.Context(), identityRequest)
	require.NoError(t, err)
	output, err := identityClient.Authenticate(t.Context(), &pluginv1.IdentityAuthenticateRequest{Identity: identityRequest, Upstream: event.GetResult()})
	require.NoError(t, err)
	mapped, err := output.Recv()
	require.NoError(t, err)
	require.Equal(t, "environment/v1", mapped.GetResult().GetType())
	require.Equal(t, map[string]any{"GITHUB_TOKEN": "rotating-secret"}, mapped.GetResult().GetData().AsMap())
	require.Equal(t, event.GetResult().GetExpiresAt().AsTime(), mapped.GetResult().GetExpiresAt().AsTime())
	_, err = output.Recv()
	require.ErrorIs(t, err, io.EOF)
	_, err = client.Logout(t.Context(), request)
	require.NoError(t, err)
	_, err = identityClient.Logout(t.Context(), identityRequest)
	require.NoError(t, err)
	_, err = client.Validate(t.Context(), &pluginv1.ProviderRequest{})
	require.Error(t, err)
	bad, err := client.Authenticate(t.Context(), &pluginv1.ProviderRequest{})
	require.NoError(t, err)
	_, err = bad.Recv()
	require.Error(t, err)
	_, err = identityClient.Validate(t.Context(), &pluginv1.IdentityRequest{})
	require.Error(t, err)
	bad, err = identityClient.Authenticate(t.Context(), &pluginv1.IdentityAuthenticateRequest{})
	require.NoError(t, err)
	_, err = bad.Recv()
	require.Error(t, err)
}

func TestConfigurationAndSecretFailures(t *testing.T) {
	cases := []map[string]any{
		{},
		{"credentials": map[string]any{}},
		{"credentials": "invalid"},
		{"credentials": map[string]any{"token": 42}},
		{"credentials": map[string]any{"": map[string]any{}}},
		{"credentials": map[string]any{"token": map[string]any{"store": "team", "key": ""}}},
	}
	for _, value := range cases {
		_, err := credentials(object(t, value))
		require.Error(t, err)
	}
	for _, failure := range []struct {
		value string
		err   error
	}{{err: errors.New("must-not-leak")}, {value: "bad\x00value"}, {value: strings.Repeat("a", (1<<20)+1)}, {value: string([]byte{0xff})}} {
		mock := NewMocksecretReader(gomock.NewController(t))
		mock.EXPECT().ReadSecret(gomock.Any(), "token").Return(failure.value, failure.err)
		_, err := (&provider{secrets: mock}).authenticate(t.Context(), &pluginv1.ProviderRequest{Spec: spec(t)})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "must-not-leak")
	}
}

func TestMappingRejectsMissingExpiredAndInvalidValues(t *testing.T) {
	for _, value := range []map[string]any{{}, {"environment": map[string]any{}}, {"environment": map[string]any{"BAD-NAME": "token"}}, {"environment": map[string]any{"TOKEN": 42}}, {"environment": map[string]any{"TOKEN": ""}}, {"environment": map[string]any{"TOKEN": "token", "token": "token"}}} {
		_, err := mapping(object(t, value))
		require.Error(t, err)
	}
	for _, upstream := range []*pluginv1.Credentials{
		nil,
		{Type: "unknown"},
		{Type: "secret-values/v1"},
		{Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(-time.Minute))},
		{Type: "secret-values/v1", ExpiresAt: &timestamppb.Timestamp{Seconds: 1 << 62}},
		{Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)), Data: object(t, map[string]any{})},
		{Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)), Data: object(t, map[string]any{"token": "nul\x00"})},
		{Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)), Data: object(t, map[string]any{"token": strings.Repeat("a", (1<<20)+1)})},
	} {
		_, err := (&identity{}).authenticate(&pluginv1.IdentityAuthenticateRequest{Identity: &pluginv1.IdentityRequest{Principal: principal(t)}, Upstream: upstream})
		require.Error(t, err)
	}
}

func TestProviderReadsRotatedSecretsOnEachAuthentication(t *testing.T) {
	mock := NewMocksecretReader(gomock.NewController(t))
	gomock.InOrder(
		mock.EXPECT().ReadSecret(gomock.Any(), "token").Return("first-token", nil),
		mock.EXPECT().ReadSecret(gomock.Any(), "token").Return("rotated-token", nil),
	)
	p := &provider{secrets: mock}
	request := &pluginv1.ProviderRequest{Spec: spec(t)}
	first, err := p.authenticate(t.Context(), request)
	require.NoError(t, err)
	before := time.Now()
	second, err := p.authenticate(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "first-token", first.GetData().AsMap()["token"])
	require.Equal(t, "rotated-token", second.GetData().AsMap()["token"])
	require.WithinRange(t, second.GetExpiresAt().AsTime(), before.Add(5*time.Minute), time.Now().Add(5*time.Minute))
}

func TestIdentityExportsOnlyExplicitlyMappedFields(t *testing.T) {
	upstream := &pluginv1.Credentials{
		Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)),
		Data: object(t, map[string]any{"token": "selected", "unselected": "private"}),
	}
	result, err := (&identity{}).authenticate(&pluginv1.IdentityAuthenticateRequest{
		Identity: &pluginv1.IdentityRequest{Principal: principal(t)}, Upstream: upstream,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"GITHUB_TOKEN": "selected"}, result.GetData().AsMap())
	require.Equal(t, map[string]any{"token": "selected", "unselected": "private"}, upstream.GetData().AsMap())
}

func TestNilRequestsFailWithoutPanicking(t *testing.T) {
	_, err := (&provider{}).Validate(t.Context(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = (&provider{}).authenticate(t.Context(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = (&identity{}).Validate(t.Context(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = (&identity{}).authenticate(nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestDescriptionUsesManifestIdentityAndExports(t *testing.T) {
	info := description()
	require.Equal(t, "cloudposse/auth-secrets", info.PluginName)
	require.Equal(t, "1.0.2", info.PluginVersion)
	require.Len(t, info.Services, 2)
	require.Equal(t, "secrets/store", info.Exports[0].Kind)
	require.Equal(t, "secrets/env", info.Exports[1].Kind)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, info.ManifestDigest)
}
