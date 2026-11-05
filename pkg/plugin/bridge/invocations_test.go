package bridge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/pkg/keyring"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

func TestPrivateSessionsAndInvocationLifetime(t *testing.T) {
	t.Parallel()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	server := New()
	first, err := server.Begin("one", Invocation{Context: t.Context(), Namespace: "plugin/provider/realm-one", Keyring: storage})
	require.NoError(t, err)
	second, err := server.Begin("two", Invocation{Context: t.Context(), Namespace: "plugin/provider/realm-two", Keyring: storage})
	require.NoError(t, err)
	defer second()
	firstContext := metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "one"))
	secondContext := metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "two"))
	_, err = server.Put(firstContext, &apiv1.PutRequest{Key: "session", Data: []byte("private-refresh-token")})
	require.NoError(t, err)
	own, err := server.Get(firstContext, &apiv1.GetRequest{Key: "session"})
	require.NoError(t, err)
	assert.True(t, own.GetFound())
	assert.Equal(t, []byte("private-refresh-token"), own.GetData())
	other, err := server.Get(secondContext, &apiv1.GetRequest{Key: "session"})
	require.NoError(t, err)
	assert.False(t, other.GetFound())
	first()
	_, err = server.Get(firstContext, &apiv1.GetRequest{Key: "session"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	// A fresh invocation can restore the same namespace after the previous call ends.
	cleanup, err := server.Begin("three", Invocation{Context: t.Context(), Namespace: "plugin/provider/realm-one", Keyring: storage})
	require.NoError(t, err)
	defer cleanup()
	fresh := metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "three"))
	own, err = server.Get(fresh, &apiv1.GetRequest{Key: "session"})
	require.NoError(t, err)
	assert.Equal(t, []byte("private-refresh-token"), own.GetData())
	_, err = server.Delete(fresh, &apiv1.DeleteRequest{Key: "session"})
	require.NoError(t, err)
	own, err = server.Get(fresh, &apiv1.GetRequest{Key: "session"})
	require.NoError(t, err)
	assert.False(t, own.GetFound())
}

func TestRejectsMissingDuplicateAndCanceledInvocations(t *testing.T) {
	t.Parallel()
	server := New()
	ctx, cancel := context.WithCancel(t.Context())
	cleanup, err := server.Begin("active", Invocation{Context: ctx, Namespace: "session"})
	require.NoError(t, err)
	defer cleanup()
	_, err = server.Begin("active", Invocation{Context: ctx, Namespace: "session"})
	assert.Equal(t, codes.AlreadyExists, status.Code(err))
	for _, values := range [][]string{nil, {"unknown"}, {"active", "active"}} {
		request := metadata.NewIncomingContext(t.Context(), metadata.MD{InvocationMetadata: values})
		_, err := server.Get(request, &apiv1.GetRequest{Key: "session"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	}
	cancel()
	request := metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "active"))
	_, err = server.Get(request, &apiv1.GetRequest{Key: "session"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPrivateSessionCleanupWithoutKeyEnumeration(t *testing.T) {
	t.Parallel()
	memory, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	storage := keyring.NewMockKeyring(gomock.NewController(t))
	storage.EXPECT().Get(gomock.Any()).DoAndReturn(memory.Get).AnyTimes()
	storage.EXPECT().Set(gomock.Any(), gomock.Any()).DoAndReturn(memory.Set).AnyTimes()
	storage.EXPECT().Delete(gomock.Any()).DoAndReturn(memory.Delete).AnyTimes()
	// No List expectation: OS keychains cannot enumerate keys.
	server := New()
	cleanup, err := server.Begin("fixture", Invocation{Context: t.Context(), Namespace: "provider", Keyring: storage})
	require.NoError(t, err)
	defer cleanup()
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "fixture"))
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "one", Data: []byte("first-token")})
	require.NoError(t, err)
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "two", Data: []byte("second-token")})
	require.NoError(t, err)
	value, err := server.Get(ctx, &apiv1.GetRequest{Key: "one"})
	require.NoError(t, err)
	assert.Equal(t, []byte("first-token"), value.Data)
	require.NoError(t, ClearPrivateSession(storage, "unrelated"))
	value, err = server.Get(ctx, &apiv1.GetRequest{Key: "two"})
	require.NoError(t, err)
	assert.Equal(t, []byte("second-token"), value.Data)
	require.NoError(t, ClearPrivateSession(storage, "provider"))
	value, err = server.Get(ctx, &apiv1.GetRequest{Key: "one"})
	require.NoError(t, err)
	assert.False(t, value.Found)
	keys, err := memory.List()
	require.NoError(t, err)
	assert.Empty(t, keys)
}
