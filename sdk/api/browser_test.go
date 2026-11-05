package api

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

type browserFixture struct {
	apiv1.UnimplementedBrowserServer
	events   []*apiv1.CallbackEvent
	terminal error
	opened   string
	path     string
	state    string
}

func (f *browserFixture) Open(_ context.Context, request *apiv1.OpenRequest) (*emptypb.Empty, error) {
	f.opened = request.Url
	return &emptypb.Empty{}, nil
}

func (f *browserFixture) Listen(request *apiv1.ListenRequest, stream grpc.ServerStreamingServer[apiv1.CallbackEvent]) error {
	f.path, f.state = request.Path, request.ExpectedState
	for _, event := range f.events {
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	return f.terminal
}

func TestBrowserCallbackProtocol(t *testing.T) {
	t.Parallel()
	ready := &apiv1.CallbackEvent{Event: &apiv1.CallbackEvent_Ready{Ready: &apiv1.CallbackReady{RedirectUri: "http://127.0.0.1:12345/callback"}}}
	callback := &apiv1.CallbackEvent{Event: &apiv1.CallbackEvent_Callback{Callback: &apiv1.Callback{Code: "authorization", State: "nonce"}}}
	cases := []struct {
		name          string
		events        []*apiv1.CallbackEvent
		terminal      error
		listenError   bool
		protocolError bool
	}{
		{name: "complete", events: []*apiv1.CallbackEvent{ready, callback}},
		{name: "no readiness", events: []*apiv1.CallbackEvent{callback}, listenError: true, protocolError: true},
		{name: "empty readiness", events: []*apiv1.CallbackEvent{{Event: &apiv1.CallbackEvent_Ready{Ready: &apiv1.CallbackReady{}}}}, listenError: true, protocolError: true},
		{name: "listener denied", terminal: status.Error(codes.PermissionDenied, "denied"), listenError: true},
		{name: "missing callback", events: []*apiv1.CallbackEvent{ready}},
		{name: "duplicate readiness", events: []*apiv1.CallbackEvent{ready, ready}, protocolError: true},
		{name: "duplicate callback", events: []*apiv1.CallbackEvent{ready, callback, callback}, protocolError: true},
		{name: "trailing error", events: []*apiv1.CallbackEvent{ready, callback}, terminal: status.Error(codes.Unavailable, "closed"), protocolError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := &browserFixture{events: tc.events, terminal: tc.terminal}
			client, _ := serviceClient(t, func(s *grpc.Server) { apiv1.RegisterBrowserServer(s, fixture) })
			listener, err := client.Listen(t.Context(), "/callback", "nonce")
			if tc.listenError {
				if err == nil {
					t.Fatal("invalid listener accepted")
				}
				if tc.protocolError && !errors.Is(err, ErrProtocol) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			if listener.RedirectURI() != ready.GetReady().GetRedirectUri() {
				t.Fatal("redirect URI changed")
			}
			result, err := listener.Receive()
			if tc.name != "complete" {
				if err == nil || (tc.protocolError && !errors.Is(err, ErrProtocol)) {
					t.Fatalf("bad callback accepted: %v %v", result, err)
				}
				return
			}
			if err != nil || result.GetCode() != "authorization" || result.GetState() != "nonce" {
				t.Fatalf("callback: %v %v", result, err)
			}
			if fixture.path != "/callback" || fixture.state != "nonce" {
				t.Fatal("listener constraints changed")
			}
			if err := client.Open(t.Context(), "https://issuer.example/authorize"); err != nil {
				t.Fatal(err)
			}
			if fixture.opened != "https://issuer.example/authorize" {
				t.Fatal("browser target changed")
			}
		})
	}
}
