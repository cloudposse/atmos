package bridge

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

func TestSecretBrokerInvocationGrants(t *testing.T) {
	server := New()
	reader := NewMockSecretReader(gomock.NewController(t))
	done, err := server.Begin("provider", Invocation{Context: t.Context(), Namespace: "provider", Secrets: reader})
	require.NoError(t, err)
	t.Cleanup(done)
	endIdentity, err := server.Begin("identity", Invocation{Context: t.Context(), Namespace: "identity"})
	require.NoError(t, err)
	t.Cleanup(endIdentity)
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "provider"))
	reader.EXPECT().ReadSecret(gomock.Any(), "token").Return("private-token", nil)
	value, err := server.Read(ctx, &apiv1.SecretRequest{Name: "token"})
	require.NoError(t, err)
	require.Equal(t, "private-token", value.GetValue())
	for _, id := range []string{"identity", "unknown", ""} {
		_, err := server.Read(metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, id)), &apiv1.SecretRequest{Name: "token"})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	}
	for _, request := range []*apiv1.SecretRequest{nil, {}, {Name: strings.Repeat("x", 257)}} {
		_, err := server.Read(ctx, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	reader.EXPECT().ReadSecret(gomock.Any(), "denied").Return("", errors.New("sensitive-backend-path"))
	_, err = server.Read(ctx, &apiv1.SecretRequest{Name: "denied"})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.NotContains(t, err.Error(), "sensitive-backend-path")
	reader.EXPECT().ReadSecret(gomock.Any(), "large").Return(strings.Repeat("x", (1<<20)+1), nil)
	_, err = server.Read(ctx, &apiv1.SecretRequest{Name: "large"})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}
