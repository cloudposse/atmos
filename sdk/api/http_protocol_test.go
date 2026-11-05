package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"testing/iotest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

type responseFixture struct {
	apiv1.UnimplementedHTTPServer
	response *apiv1.HTTPResponse
	err      error
}

func (f responseFixture) Do(context.Context, *apiv1.HTTPRequest) (*apiv1.HTTPResponse, error) {
	return f.response, f.err
}

func TestHTTPResponseValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		response *apiv1.HTTPResponse
		rpcError error
	}{
		{name: "invalid low status", response: &apiv1.HTTPResponse{StatusCode: 99}},
		{name: "invalid high status", response: &apiv1.HTTPResponse{StatusCode: 600}},
		{name: "permission error", rpcError: status.Error(codes.PermissionDenied, "denied")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, _ := serviceClient(t, func(s *grpc.Server) {
				apiv1.RegisterHTTPServer(s, responseFixture{response: tc.response, err: tc.rpcError})
			})
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := (Transport{API: client}).RoundTrip(request)
			if response != nil {
				defer response.Body.Close()
			}
			if tc.rpcError != nil {
				if status.Code(err) != codes.PermissionDenied {
					t.Fatalf("lost RPC status: %v", err)
				}
			} else if !errors.Is(err, ErrProtocol) {
				t.Fatalf("malformed response accepted: %v %v", response, err)
			}
		})
	}
}

func TestHTTPRejectsUnreadableRequestAndUnconfiguredAPI(t *testing.T) {
	t.Parallel()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com", io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF)))
	if err != nil {
		t.Fatal(err)
	}
	response, err := (Transport{API: &Client{}}).RoundTrip(request)
	if response != nil {
		defer response.Body.Close()
	}
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
	client, fixture := fixtureClient(t)
	response, err = (Transport{API: client}).RoundTrip(request)
	if response != nil {
		defer response.Body.Close()
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error lost: %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.calls) != 0 {
		t.Fatal("incomplete body sent to network")
	}
}

func TestRedirectRejectsDowngradeAndExcessiveHops(t *testing.T) {
	t.Parallel()
	previous, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/a", nil)
	next, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/b", nil)
	if err := safeRedirect(next, []*http.Request{previous}); !errors.Is(err, ErrRedirect) {
		t.Fatalf("downgrade allowed: %v", err)
	}
	if err := safeRedirect(previous, make([]*http.Request, 10)); !errors.Is(err, ErrRedirect) {
		t.Fatalf("redirect loop allowed: %v", err)
	}
}
