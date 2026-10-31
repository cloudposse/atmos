package main

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_auth_test.go -package=main

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

type (
	secretReader interface {
		ReadSecret(context.Context, string) (string, error)
	}
	provider struct {
		pluginv1.UnimplementedAuthProviderServer
		secrets secretReader
	}
	identity struct {
		pluginv1.UnimplementedAuthIdentityServer
	}
)

var environmentKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func invalid() error {
	return status.Error(codes.InvalidArgument, "invalid secret authentication configuration")
}

func credentials(spec *structpb.Struct) (map[string]any, error) {
	refs, ok := spec.AsMap()["credentials"].(map[string]any)
	if !ok || len(refs) == 0 || len(refs) > 256 {
		return nil, invalid()
	}
	for name, raw := range refs {
		ref, ok := raw.(map[string]any)
		if !ok || name == "" || len(name) > 256 {
			return nil, invalid()
		}
		for _, field := range []string{"store", "path"} {
			if value, ok := ref[field].(string); !ok || value == "" {
				return nil, invalid()
			}
		}
		if value, exists := ref["field"]; exists {
			if field, ok := value.(string); !ok || field == "" {
				return nil, invalid()
			}
		}
	}
	return refs, nil
}

func (*provider) Validate(_ context.Context, request *pluginv1.ProviderRequest) (*pluginv1.ValidateResponse, error) {
	if _, err := credentials(request.GetSpec()); err != nil {
		return nil, err
	}
	return &pluginv1.ValidateResponse{}, nil
}

func (p *provider) Authenticate(request *pluginv1.ProviderRequest, stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	result, err := p.authenticate(api.WithInvocation(stream.Context(), request.GetInvocation().GetId()), request)
	if err != nil {
		return err
	}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: result}})
}

func (p *provider) authenticate(ctx context.Context, request *pluginv1.ProviderRequest) (*pluginv1.Credentials, error) {
	refs, err := credentials(request.GetSpec())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	data := make(map[string]any, len(refs))
	for _, name := range names {
		value, err := p.secrets.ReadSecret(ctx, name)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "configured secret unavailable")
		}
		if strings.ContainsRune(value, 0) || len(value) > 1<<20 {
			return nil, invalid()
		}
		data[name] = value
	}
	payload, err := structpb.NewStruct(data)
	if err != nil {
		return nil, invalid()
	}
	// Re-read the stores on every authentication. No secret or refresh state is cached.
	return &pluginv1.Credentials{Type: "secret-values/v1", Data: payload, ExpiresAt: timestamppb.New(time.Now().Add(5 * time.Minute))}, nil
}

func (*identity) Validate(_ context.Context, request *pluginv1.IdentityRequest) (*pluginv1.ValidateResponse, error) {
	if _, err := environmentTemplates(request); err != nil {
		return nil, err
	}
	return &pluginv1.ValidateResponse{}, nil
}

func (i *identity) Authenticate(request *pluginv1.IdentityAuthenticateRequest, stream grpc.ServerStreamingServer[pluginv1.AuthenticateEvent]) error {
	result, err := i.authenticate(request)
	if err != nil {
		return err
	}
	return stream.Send(&pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: result}})
}

func (*identity) authenticate(request *pluginv1.IdentityAuthenticateRequest) (*pluginv1.Credentials, error) {
	env, err := environmentTemplates(request.GetIdentity())
	if err != nil {
		return nil, err
	}
	upstream := request.GetUpstream()
	if upstream.GetType() != "secret-values/v1" || upstream.GetExpiresAt() == nil || upstream.GetExpiresAt().CheckValid() != nil || !upstream.GetExpiresAt().AsTime().After(time.Now()) {
		return nil, invalid()
	}
	values := upstream.GetData().AsMap()
	data, err := renderEnvironment(env, values)
	if err != nil {
		return nil, err
	}
	payload, err := structpb.NewStruct(data)
	if err != nil {
		return nil, invalid()
	}
	return &pluginv1.Credentials{Type: "environment/v1", Data: payload, ExpiresAt: upstream.GetExpiresAt()}, nil
}

func (*provider) Logout(context.Context, *pluginv1.ProviderRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (*identity) Logout(context.Context, *pluginv1.IdentityRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
