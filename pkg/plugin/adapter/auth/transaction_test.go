package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
	"google.golang.org/grpc/metadata"
)

func TestSessionTransactionSerializationAndLeaseLifetime(t *testing.T) {
	t.Parallel()
	definition := Definition{Services: Services{LockDirectory: t.TempDir()}}
	var escaped context.Context
	require.NoError(t, definition.transaction(t.Context(), "provider", func(ctx context.Context) error {
		escaped = ctx
		return definition.transaction(ctx, "provider", func(context.Context) error { return nil })
	}))
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- definition.transaction(t.Context(), "provider", func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	defer func() { close(release); require.NoError(t, <-done) }()
	ctx, cancel := context.WithTimeout(escaped, 100*time.Millisecond)
	defer cancel()
	var ran atomic.Bool
	err := definition.transaction(ctx, "provider", func(context.Context) error { ran.Store(true); return nil })
	require.Error(t, err)
	assert.False(t, ran.Load(), "expired request lease must not bypass another operation's lock")
	require.NoError(t, definition.transaction(t.Context(), "other-provider", func(context.Context) error { return nil }))
}

func TestLogoutCleansPrivateSessionWhenBinaryCannotStart(t *testing.T) {
	t.Parallel()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	definition := Definition{Package: "example/test", Executable: "missing-native-plugin", Services: Services{Keyring: storage, LockDirectory: t.TempDir()}}
	namespace, err := definition.sessionNamespace("corporate", "realm", nil)
	require.NoError(t, err)
	server := bridge.New()
	done, err := server.Begin("fixture", bridge.Invocation{Context: t.Context(), Namespace: namespace, Keyring: storage})
	require.NoError(t, err)
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(bridge.InvocationMetadata, "fixture"))
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "refresh", Data: []byte("private-token")})
	require.NoError(t, err)
	done()
	err = definition.logout(t.Context(), "corporate", "realm", nil, func(*invocation) error { t.Fatal("missing executable cannot invoke handler"); return nil })
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	keys, err := storage.List()
	require.NoError(t, err)
	assert.Empty(t, keys)
}
