package auth

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestCredentialStreamRequiresOneTerminalResult(t *testing.T) {
	t.Parallel()
	result := &pluginv1.Credentials{Type: "oidc/v1"}
	event := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: result}}
	for _, tt := range []struct {
		name     string
		events   []*pluginv1.AuthenticateEvent
		terminal error
		valid    bool
	}{
		{"valid with progress", []*pluginv1.AuthenticateEvent{{}, event}, io.EOF, true},
		{"missing result", nil, io.EOF, false},
		{"duplicate result", []*pluginv1.AuthenticateEvent{event, event}, io.EOF, false},
		{"transport failure after result", []*pluginv1.AuthenticateEvent{event}, context.Canceled, false},
		{"unbounded progress", make([]*pluginv1.AuthenticateEvent, maxCredentialEvents), io.EOF, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := NewMockcredentialStream(gomock.NewController(t))
			calls := make([]any, 0, len(tt.events)+1)
			for _, entry := range tt.events {
				calls = append(calls, stream.EXPECT().Recv().Return(entry, nil))
			}
			if len(tt.events) < maxCredentialEvents && tt.name != "duplicate result" {
				calls = append(calls, stream.EXPECT().Recv().Return(nil, tt.terminal))
			}
			gomock.InOrder(calls...)
			actual, err := receiveCredentials(stream)
			if tt.valid {
				require.NoError(t, err)
				assert.Equal(t, result, actual)
			} else {
				require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
				assert.Nil(t, actual)
			}
		})
	}
}

func TestTranslateErrorPreservesSafeClassification(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ input, target error }{
		{nil, nil},
		{status.Error(codes.FailedPrecondition, "secret"), errUtils.ErrPluginInteractionRequired},
		{status.Error(codes.Canceled, "secret"), context.Canceled},
		{status.Error(codes.DeadlineExceeded, "secret"), context.DeadlineExceeded},
		{status.Error(codes.Unauthenticated, "secret"), errUtils.ErrAuthenticationFailed},
		{status.Error(codes.Internal, "secret"), errUtils.ErrPluginAPI},
		{errors.Join(errUtils.ErrPluginProtocol, io.EOF), errUtils.ErrPluginProtocol},
		{context.Canceled, context.Canceled},
		{context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		result := translateError(tt.input)
		if tt.target == nil {
			require.NoError(t, result)
		} else {
			require.ErrorIs(t, result, tt.target)
			assert.NotContains(t, result.Error(), "secret")
		}
	}
}

func TestInvocationRejectsInvalidSettingsAndLockDirectory(t *testing.T) {
	t.Parallel()
	invalid := map[string]any{"unsupported": make(chan int)}
	definition := &Definition{}
	never := func(*invocation) error { t.Fatal("invalid settings must never launch a plugin"); return nil }
	require.ErrorIs(t, definition.invoke(t.Context(), "provider", "realm", invalid, never), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, definition.logout(t.Context(), "provider", "realm", invalid, never), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, definition.invoke(t.Context(), "provider", "realm", nil, never), errUtils.ErrPluginInvalidConfig)
	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o600))
	definition.Services.LockDirectory = filepath.Join(blocked, "locks")
	require.ErrorIs(t, definition.invoke(t.Context(), "provider", "realm", nil, never), errUtils.ErrPluginAPI)
	p := &provider{definition: definition, config: providerConfig(invalid)}
	require.ErrorIs(t, p.WithCredentialTransaction(t.Context(), func(context.Context) error { t.Fatal("invalid settings must not lock"); return nil }), errUtils.ErrPluginInvalidConfig)
}
