package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/keyring"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

func bridgeScope(t *testing.T, scope *Invocation) (*Server, context.Context) {
	t.Helper()
	scope.Context = t.Context()
	scope.Namespace = "failure-fixture"
	server := New()
	cleanup, err := server.Begin("fixture", *scope)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return server, metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, "fixture"))
}

func TestAPIRejectsUnauthorizedAndUnavailableCapabilities(t *testing.T) {
	t.Parallel()
	server, ctx := bridgeScope(t, &Invocation{})
	for _, authorized := range []bool{false, true} {
		expected := codes.PermissionDenied
		callContext := t.Context()
		if authorized {
			expected, callContext = codes.Unimplemented, ctx
		}
		_, err := server.Get(callContext, &apiv1.GetRequest{})
		assert.Equal(t, expected, status.Code(err))
		_, err = server.Put(callContext, &apiv1.PutRequest{})
		assert.Equal(t, expected, status.Code(err))
		_, err = server.Delete(callContext, &apiv1.DeleteRequest{})
		assert.Equal(t, expected, status.Code(err))
		_, err = server.Open(callContext, &apiv1.OpenRequest{})
		assert.Equal(t, expected, status.Code(err))
		_, err = server.Do(callContext, &apiv1.HTTPRequest{})
		assert.Equal(t, expected, status.Code(err))
		stream := NewMockcallbackStream(gomock.NewController(t))
		stream.EXPECT().Context().Return(callContext)
		assert.Equal(t, expected, status.Code(server.Listen(&apiv1.ListenRequest{}, stream)))
	}
}

func TestAPIErrorsRedactSecrets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cause error
		code  codes.Code
	}{
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{errUtils.ErrPluginInteractionRequired, codes.FailedPrecondition},
		{errUtils.ErrPluginInvalidConfig, codes.InvalidArgument},
		{errUtils.ErrPluginPayloadTooLarge, codes.ResourceExhausted},
		{io.ErrUnexpectedEOF, codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			t.Parallel()
			err := apiError(fmt.Errorf("secret-refresh-token: %w", tt.cause))
			assert.Equal(t, tt.code, status.Code(err))
			assert.NotContains(t, err.Error(), "secret-refresh-token")
		})
	}
}

func TestPrivateSessionRejectsCorruptionAndLimits(t *testing.T) {
	t.Parallel()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	server, ctx := bridgeScope(t, &Invocation{Keyring: storage})
	for _, raw := range []string{"not-base64!", base64.StdEncoding.EncodeToString([]byte("not-json")), base64.StdEncoding.EncodeToString([]byte(`{"version":2,"values":{}}`)), base64.StdEncoding.EncodeToString([]byte(`{"version":1}`))} {
		require.NoError(t, storage.Set(sessionRecordKey("failure-fixture"), raw))
		_, err = server.Get(ctx, &apiv1.GetRequest{Key: "session"})
		assert.Equal(t, codes.Unavailable, status.Code(err))
		_, err = server.Put(ctx, &apiv1.PutRequest{Key: "session"})
		assert.Equal(t, codes.Unavailable, status.Code(err))
		_, err = server.Delete(ctx, &apiv1.DeleteRequest{Key: "session"})
		assert.Equal(t, codes.Unavailable, status.Code(err))
	}
	require.NoError(t, ClearPrivateSession(storage, "failure-fixture"))
	for _, key := range []string{"", strings.Repeat("k", maxSessionKeyBytes+1)} {
		_, err = server.Put(ctx, &apiv1.PutRequest{Key: key})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "session", Data: make([]byte, api.MaxHTTPBody+1)})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	for index := range maxSessionEntries {
		_, err = server.Put(ctx, &apiv1.PutRequest{Key: fmt.Sprint(index), Data: []byte("value")})
		require.NoError(t, err)
	}
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "overflow"})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	existing, err := server.Get(ctx, &apiv1.GetRequest{Key: "0"})
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), existing.Data)
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "0", Data: make([]byte, api.MaxHTTPBody)})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.NoError(t, storage.Set(sessionRecordKey("failure-fixture"), strings.Repeat("x", base64.StdEncoding.EncodedLen(api.MaxHTTPBody)+1)))
	_, err = readSession(storage, "failure-fixture")
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
}

func TestPrivateSessionStorageFailures(t *testing.T) {
	t.Parallel()
	storage := keyring.NewMockKeyring(gomock.NewController(t))
	server, ctx := bridgeScope(t, &Invocation{Keyring: storage})
	storage.EXPECT().Get(gomock.Any()).Return("", io.ErrUnexpectedEOF)
	_, err := server.Get(ctx, &apiv1.GetRequest{Key: "session"})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	storage.EXPECT().Get(gomock.Any()).Return("", keyring.ErrNotFound)
	storage.EXPECT().Delete(gomock.Any()).Return(io.ErrUnexpectedEOF)
	_, err = server.Delete(ctx, &apiv1.DeleteRequest{Key: "session"})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	storage.EXPECT().Get(gomock.Any()).Return("", keyring.ErrNotFound)
	storage.EXPECT().Set(gomock.Any(), gomock.Any()).Return(io.ErrUnexpectedEOF)
	_, err = server.Put(ctx, &apiv1.PutRequest{Key: "session"})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	require.ErrorIs(t, ClearPrivateSession(nil, "scope"), errUtils.ErrPluginAPI)
	storage.EXPECT().Delete(gomock.Any()).Return(io.ErrUnexpectedEOF)
	require.ErrorIs(t, ClearPrivateSession(storage, "scope"), io.ErrUnexpectedEOF)
}

func TestHTTPBridgeValidationAndTransportFailures(t *testing.T) {
	t.Parallel()
	transport := NewMockHTTP(gomock.NewController(t))
	server, ctx := bridgeScope(t, &Invocation{HTTP: transport})
	_, err := server.Do(ctx, &apiv1.HTTPRequest{Url: "%"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = server.Do(ctx, &apiv1.HTTPRequest{Body: make([]byte, api.MaxHTTPBody+1)})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	for _, response := range []*http.Response{nil, {}, {StatusCode: 99, Body: io.NopCloser(strings.NewReader(""))}, {StatusCode: 600, Body: io.NopCloser(strings.NewReader(""))}} {
		transport.EXPECT().Do(gomock.Any(), gomock.Any()).Return(response, nil)
		_, err = server.Do(ctx, &apiv1.HTTPRequest{Url: "https://example.com"})
		assert.Equal(t, codes.Unavailable, status.Code(err))
	}
	transport.EXPECT().Do(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)
	_, err = server.Do(ctx, &apiv1.HTTPRequest{Url: "https://example.com"})
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	transport.EXPECT().Do(gomock.Any(), gomock.Any()).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(make([]byte, api.MaxHTTPBody+1)))}, nil)
	_, err = server.Do(ctx, &apiv1.HTTPRequest{Url: "https://example.com"})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestBrowserBridgeFailureClosesListener(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"open", "listen", "ready", "callback"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			controller := gomock.NewController(t)
			browser := NewMockBrowser(controller)
			server, ctx := bridgeScope(t, &Invocation{Browser: browser, Interactive: true})
			if stage == "open" {
				browser.EXPECT().Open(gomock.Any(), "https://issuer.example").Return(context.Canceled)
				_, err := server.Open(ctx, &apiv1.OpenRequest{Url: "https://issuer.example"})
				assert.Equal(t, codes.Canceled, status.Code(err))
				return
			}
			stream := NewMockcallbackStream(controller)
			stream.EXPECT().Context().Return(ctx)
			if stage == "listen" {
				browser.EXPECT().Listen(gomock.Any(), "/callback", "state").Return(nil, errUtils.ErrPluginInteractionRequired)
				assert.Equal(t, codes.FailedPrecondition, status.Code(server.Listen(&apiv1.ListenRequest{Path: "/callback", ExpectedState: "state"}, stream)))
				return
			}
			listener := NewMockCallbackListener(controller)
			browser.EXPECT().Listen(gomock.Any(), "/callback", "state").Return(listener, nil)
			listener.EXPECT().RedirectURI().Return("http://127.0.0.1/callback")
			listener.EXPECT().Close().Return(nil)
			if stage == "ready" {
				stream.EXPECT().Send(gomock.Any()).Return(io.ErrClosedPipe)
			} else {
				stream.EXPECT().Send(gomock.Any()).Return(nil)
				listener.EXPECT().Receive(gomock.Any()).Return(api.Callback{}, context.DeadlineExceeded)
			}
			err := server.Listen(&apiv1.ListenRequest{Path: "/callback", ExpectedState: "state"}, stream)
			if stage == "ready" {
				require.ErrorIs(t, err, io.ErrClosedPipe)
			} else {
				assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
			}
		})
	}
}

func TestBrowserBridgeRejectsNoninteractiveInvocation(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"open", "listen"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			controller := gomock.NewController(t)
			browser := NewMockBrowser(controller)
			server, ctx := bridgeScope(t, &Invocation{Browser: browser})
			// No browser calls are expected, regardless of the implementation's own policy.
			var err error
			if operation == "open" {
				_, err = server.Open(ctx, &apiv1.OpenRequest{Url: "https://issuer.example"})
			} else {
				stream := NewMockcallbackStream(controller)
				stream.EXPECT().Context().Return(ctx)
				err = server.Listen(&apiv1.ListenRequest{Path: "/callback", ExpectedState: "state"}, stream)
			}
			assert.Equal(t, codes.FailedPrecondition, status.Code(err))
			assert.Equal(t, "INTERACTION_REQUIRED", status.Convert(err).Message())
		})
	}
}

func TestBrowserBridgeInteractiveInvocation(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	browser := NewMockBrowser(controller)
	server, ctx := bridgeScope(t, &Invocation{Browser: browser, Interactive: true})
	browser.EXPECT().Open(gomock.Any(), "https://issuer.example").Return(nil)
	_, err := server.Open(ctx, &apiv1.OpenRequest{Url: "https://issuer.example"})
	require.NoError(t, err)
	listener := NewMockCallbackListener(controller)
	browser.EXPECT().Listen(gomock.Any(), "/callback", "state").Return(listener, nil)
	listener.EXPECT().RedirectURI().Return("http://127.0.0.1/callback")
	listener.EXPECT().Receive(gomock.Any()).Return(api.Callback{Code: "code", State: "state"}, nil)
	listener.EXPECT().Close().Return(nil)
	stream := NewMockcallbackStream(controller)
	stream.EXPECT().Context().Return(ctx)
	gomock.InOrder(
		stream.EXPECT().Send(&apiv1.CallbackEvent{Event: &apiv1.CallbackEvent_Ready{Ready: &apiv1.CallbackReady{RedirectUri: "http://127.0.0.1/callback"}}}).Return(nil),
		stream.EXPECT().Send(&apiv1.CallbackEvent{Event: &apiv1.CallbackEvent_Callback{Callback: &apiv1.Callback{Code: "code", State: "state"}}}).Return(nil),
	)
	require.NoError(t, server.Listen(&apiv1.ListenRequest{Path: "/callback", ExpectedState: "state"}, stream))
}
