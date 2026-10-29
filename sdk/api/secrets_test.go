package api

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

// secretsFixture exercises the SDK against the real Secrets wire service.
type secretsFixture struct {
	apiv1.UnimplementedSecretsServer
}

func (*secretsFixture) Read(ctx context.Context, request *apiv1.SecretRequest) (*apiv1.SecretResponse, error) {
	ids := metadata.ValueFromIncomingContext(ctx, InvocationMetadata)
	if len(ids) != 1 || ids[0] != "provider-invocation" || request.GetName() != "token" {
		return nil, status.Error(codes.PermissionDenied, "not granted")
	}
	return &apiv1.SecretResponse{Value: "private-token"}, nil
}

func TestReadSecretInvocationAndErrors(t *testing.T) {
	t.Parallel()
	client, _ := serviceClient(t, func(s *grpc.Server) { apiv1.RegisterSecretsServer(s, &secretsFixture{}) })
	ctx := WithInvocation(t.Context(), "provider-invocation")
	value, err := client.ReadSecret(ctx, "token")
	if err != nil || value != "private-token" {
		t.Fatalf("ReadSecret: value %q, err %v", value, err)
	}
	for _, request := range []struct {
		ctx  context.Context
		name string
	}{{ctx, "not-granted"}, {t.Context(), "token"}} {
		value, err = client.ReadSecret(request.ctx, request.name)
		if status.Code(err) != codes.PermissionDenied || value != "" {
			t.Fatalf("unauthorized request: value %q, err %v", value, err)
		}
	}
	var unconfigured Client
	if _, err := unconfigured.ReadSecret(ctx, "token"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured client: %v", err)
	}
}
