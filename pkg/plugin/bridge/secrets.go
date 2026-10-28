package bridge

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_secrets_test.go -package=bridge

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/pkg/perf"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

const (
	maxSecretNameBytes = 256
	maxSecretBytes     = 1 << 20
)

// SecretReader grants logical names, never arbitrary store paths, to one provider.
type SecretReader interface {
	ReadSecret(context.Context, string) (string, error)
}

// Read reads an explicitly granted secret without exposing backend diagnostics.
func (s *Server) Read(ctx context.Context, request *apiv1.SecretRequest) (*apiv1.SecretResponse, error) {
	defer perf.Track(nil, "bridge.Server.Read")()
	call, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer call.done()
	if call.Secrets == nil {
		return nil, status.Error(codes.PermissionDenied, "secret access not granted")
	}
	if request.GetName() == "" || len(request.GetName()) > maxSecretNameBytes {
		return nil, status.Error(codes.InvalidArgument, "invalid secret name")
	}
	value, err := call.Secrets.ReadSecret(call.Context, request.GetName())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "secret unavailable or not granted")
	}
	if len(value) > maxSecretBytes {
		return nil, status.Error(codes.ResourceExhausted, "secret exceeds size limit")
	}
	return &apiv1.SecretResponse{Value: value}, nil
}
