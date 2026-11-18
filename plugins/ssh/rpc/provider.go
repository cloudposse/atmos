// Package rpc adapts the SSH plugin's logic to the Atmos plugin protocol.
package rpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

const (
	// TypeSecretValues is the credential type between a provider and the ssh/key identity.
	TypeSecretValues = "secret-values/v1"
	// TypeEnvironment is the credential type the ssh/key identity returns.
	TypeEnvironment = "environment/v1"

	// KindSelf is the provider with no authority.
	KindSelf = "ssh/self"
	// KindKey is the SSH key identity.
	KindKey = "ssh/key"

	// AssertionEntry marks the ssh/self credential as self-asserted. The map cannot be empty,
	// because the host rejects an empty secret-values/v1 credential.
	AssertionEntry = "assertion"
	// AssertionSelf is the value of AssertionEntry for ssh/self.
	AssertionSelf = "self"

	// SelfLifetime is how long the ssh/self credential is valid.
	SelfLifetime = 12 * time.Hour
)

// Provider is the ssh/self provider. It needs no interaction and no network: nothing vouches for
// the key, so the credential only says so.
type Provider struct {
	pluginv1.UnimplementedAuthProviderServer
	// Now is the clock.
	Now func() time.Time
}

// Validate accepts only an empty spec.
func (*Provider) Validate(_ context.Context, request *pluginv1.ProviderRequest) (*pluginv1.ValidateResponse, error) {
	if len(request.GetSpec().GetFields()) != 0 {
		return nil, status.Error(codes.InvalidArgument, "ssh/self takes no configuration")
	}
	return &pluginv1.ValidateResponse{}, nil
}

// Authenticate returns a self-asserted credential.
func (p *Provider) Authenticate(request *pluginv1.ProviderRequest, stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	if len(request.GetSpec().GetFields()) != 0 {
		return status.Error(codes.InvalidArgument, "ssh/self takes no configuration")
	}
	data, err := structpb.NewStruct(map[string]any{AssertionEntry: AssertionSelf})
	if err != nil {
		return status.Error(codes.Internal, "build credential")
	}
	creds := &pluginv1.Credentials{Type: TypeSecretValues, Data: data, ExpiresAt: timestamppb.New(p.Now().Add(SelfLifetime))}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: creds}})
}

// Logout has nothing to undo.
func (*Provider) Logout(context.Context, *pluginv1.ProviderRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
