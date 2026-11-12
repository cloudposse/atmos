package rpc

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// Identity is the ssh/key identity server.
type Identity struct {
	pluginv1.UnimplementedAuthIdentityServer
	service *identity.Service
}

// NewIdentity returns the identity server.
func NewIdentity(service *identity.Service) *Identity { return &Identity{service: service} }

// bind scopes API calls (private state, HTTP) to the invocation.
func bind(ctx context.Context, invocation *pluginv1.Invocation) context.Context {
	return api.WithInvocation(ctx, invocation.GetId())
}

// Validate checks the identity's spec.
func (i *Identity) Validate(_ context.Context, request *pluginv1.IdentityRequest) (*pluginv1.ValidateResponse, error) {
	if err := i.service.Validate(request.GetName(), request.GetSpec().AsMap()); err != nil {
		return nil, StatusError(err)
	}
	return &pluginv1.ValidateResponse{}, nil
}

// Authenticate brings the identity up and returns its environment.
func (i *Identity) Authenticate(request *pluginv1.IdentityAuthenticateRequest, stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	req := request.GetIdentity()
	upstream, err := upstreamStrings(request.GetUpstream())
	if err != nil {
		return err
	}
	ctx := bind(stream.Context(), req.GetInvocation())
	result, err := i.service.Authenticate(ctx, req.GetName(), req.GetSpec().AsMap(), upstream)
	if err != nil {
		return StatusError(err)
	}
	env := make(map[string]any, len(result.Environment))
	for k, v := range result.Environment {
		env[k] = v
	}
	data, err := structpb.NewStruct(env)
	if err != nil {
		return status.Error(codes.Internal, "build credential")
	}
	creds := &pluginv1.Credentials{Type: TypeEnvironment, Data: data, ExpiresAt: timestamppb.New(result.ExpiresAt)}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: creds}})
}

// Logout withdraws enrollments of keys that are gone and removes the identity from the agent.
func (i *Identity) Logout(ctx context.Context, request *pluginv1.IdentityRequest) (*emptypb.Empty, error) {
	ctx = bind(ctx, request.GetInvocation())
	if err := i.service.Logout(ctx, request.GetName(), request.GetSpec().AsMap()); err != nil {
		return nil, StatusError(err)
	}
	return &emptypb.Empty{}, nil
}

// upstreamStrings reads the provider's credential as a map of strings.
func upstreamStrings(creds *pluginv1.Credentials) (map[string]string, error) {
	if creds.GetType() != TypeSecretValues {
		return nil, status.Error(codes.InvalidArgument, "ssh/key accepts only secret-values/v1 credentials")
	}
	out := map[string]string{}
	for k, v := range creds.GetData().AsMap() {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out, nil
}

// StatusError maps a failure to the gRPC code the host understands: Unauthenticated for a rejected
// token, FailedPrecondition only for "interaction required" (a locked screen), and codes that the
// host reports as a generic failure for the rest. The text is the code's name, because the host
// discards plugin error text; "atmos ssh key status" holds the details.
func StatusError(err error) error {
	var failure *identity.Error
	if !errors.As(err, &failure) {
		return status.Error(codes.Internal, "ssh identity failed")
	}
	return status.Error(grpcCode(failure.Code), fmt.Sprintf("ssh identity failed: %s; run \"atmos ssh key status\" for details", failure.Code))
}

func grpcCode(code identity.Code) codes.Code {
	switch code {
	case identity.CodeTokenRejected, identity.CodeMissingScope:
		return codes.Unauthenticated
	case identity.CodeKeyLocked:
		return codes.FailedPrecondition
	case identity.CodeInvalidSpec, identity.CodeRequirementMismatch, identity.CodeUnsupportedRequirement,
		identity.CodeUnknownAuthority, identity.CodeUnsupportedUse:
		return codes.InvalidArgument
	case identity.CodeAuthorityUnreachable, identity.CodeAgentUnavailable:
		return codes.Unavailable
	case identity.CodeUnsupportedPlatform:
		return codes.Unimplemented
	case identity.CodeCanceled:
		return codes.Canceled
	default:
		return codes.Internal
	}
}
