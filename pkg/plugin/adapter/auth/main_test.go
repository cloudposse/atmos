package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

// TestMain lets the test binary act as the plugin process (see fakeplugin).
func TestMain(m *testing.M) {
	if fakeplugin.MaybeServe(func(server *grpc.Server) {
		pluginv1.RegisterAuthProviderServer(server, &providerFixture{})
		pluginv1.RegisterAuthIdentityServer(server, &identityFixture{})
	}) {
		return
	}
	os.Exit(m.Run())
}

// providerFixture exercises the real adapter transport with a versioned OIDC envelope.
type providerFixture struct {
	pluginv1.UnimplementedAuthProviderServer
}

func (*providerFixture) Authenticate(request *pluginv1.ProviderRequest, stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	if request.Name != "corporate" || request.Kind != "plugin/test" || request.Realm != "fixture" {
		return status.Error(codes.InvalidArgument, "provider configuration was not forwarded")
	}
	expiry, err := time.Parse(time.RFC3339, request.Spec.GetFields()["access_token_expires_at"].GetStringValue())
	if err != nil {
		return err
	}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: &pluginv1.Credentials{
		Type: "oidc/v1", Data: request.Spec, ExpiresAt: timestamppb.New(expiry),
	}}})
}

func (*providerFixture) Logout(context.Context, *pluginv1.ProviderRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, os.WriteFile("provider-logout", []byte("revoked"), 0o600)
}

// identityFixture validates forwarded configuration and exchanges an upstream token.
type identityFixture struct {
	pluginv1.UnimplementedAuthIdentityServer
}

func (*identityFixture) Authenticate(request *pluginv1.IdentityAuthenticateRequest, stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	identity := request.Identity
	if identity.Name != "broker" || identity.Kind != "plugin/broker" || identity.Realm != "fixture" ||
		identity.Principal.GetFields()["endpoint"].GetStringValue() != "https://broker.example" ||
		identity.Spec.GetFields()["tenant"].GetStringValue() != "test" ||
		request.Upstream.GetType() != "oidc/v1" || request.Upstream.GetData().GetFields()["access_token"].GetStringValue() != "fixture-access-token" {
		return status.Error(codes.InvalidArgument, "identity configuration or upstream credentials were not forwarded")
	}
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	data, err := structpb.NewStruct(map[string]any{
		"access_key_id": "access", "secret_access_key": "secret", "session_token": "session", "expiration": expiry.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: &pluginv1.Credentials{
		Type: "aws/v1", Data: data, ExpiresAt: timestamppb.New(expiry),
	}}})
}

func (*identityFixture) Logout(context.Context, *pluginv1.IdentityRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, os.WriteFile("identity-logout", []byte("revoked"), 0o600)
}
