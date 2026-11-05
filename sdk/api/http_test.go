package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

type httpFixture struct {
	apiv1.UnimplementedHTTPServer
	mu         sync.Mutex
	calls      []*apiv1.HTTPRequest
	invocation string
}

func (f *httpFixture) Do(ctx context.Context, request *apiv1.HTTPRequest) (*apiv1.HTTPResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, request)
	values := metadata.ValueFromIncomingContext(ctx, InvocationMetadata)
	if len(values) == 1 {
		f.invocation = values[0]
	}
	f.mu.Unlock()
	switch request.Url {
	case "https://issuer.example/redirect":
		return &apiv1.HTTPResponse{StatusCode: 302, Headers: map[string]*apiv1.HeaderValues{"Location": {Values: []string{"https://other.example/target"}}}}, nil
	case "https://issuer.example/token":
		return &apiv1.HTTPResponse{StatusCode: 307, Headers: map[string]*apiv1.HeaderValues{"Location": {Values: []string{"https://other.example/token"}}}}, nil
	case "https://issuer.example/wait":
		<-ctx.Done()
		return nil, ctx.Err()
	default:
		return &apiv1.HTTPResponse{StatusCode: 201, Body: []byte("response-body"), Headers: map[string]*apiv1.HeaderValues{"X-Fixture": {Values: []string{"one", "two"}}}}, nil
	}
}

func fixtureClient(t *testing.T) (*Client, *httpFixture) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	fixture := &httpFixture{}
	apiv1.RegisterHTTPServer(server, fixture)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{}
	if err := client.Bind(connection); err != nil {
		t.Fatal(err)
	}
	return client, fixture
}

func TestHTTPTransportPreservesExchangeAndInvocation(t *testing.T) {
	t.Parallel()
	client, fixture := fixtureClient(t)
	request, err := http.NewRequestWithContext(WithInvocation(t.Context(), "call-one"), http.MethodPost, "https://issuer.example/exchange", strings.NewReader("code=fixture&code_verifier=proof"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.HTTPClient().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 || string(body) != "response-body" || len(response.Header.Values("X-Fixture")) != 2 {
		t.Fatal("response metadata or body lost")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.calls) != 1 || fixture.invocation != "call-one" {
		t.Fatal("exchange retried or invocation metadata lost")
	}
	call := fixture.calls[0]
	if call.Method != http.MethodPost || string(call.Body) != "code=fixture&code_verifier=proof" || call.Headers["Content-Type"].Values[0] != "application/x-www-form-urlencoded" {
		t.Fatal("OAuth request changed")
	}
}

func TestHTTPRedirectRemovesCredentialsAndDoesNotReplayTokens(t *testing.T) {
	t.Parallel()
	client, fixture := fixtureClient(t)
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example/redirect", nil)
	request.Header.Set("Authorization", "Bearer private")
	request.Header.Set("Cookie", "session=private")
	request.Header.Set("Proxy-Authorization", "Basic private")
	response, err := client.HTTPClient().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	fixture.mu.Lock()
	if len(fixture.calls) != 2 {
		t.Fatalf("got %d calls", len(fixture.calls))
	}
	target := fixture.calls[1]
	for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
		if len(target.Headers[header].GetValues()) != 0 {
			t.Errorf("credential header %s crossed origins", header)
		}
	}
	fixture.mu.Unlock()
	token, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://issuer.example/token", strings.NewReader("refresh_token=private"))
	response, err = client.HTTPClient().Do(token)
	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("expected unsafe redirect rejection, got %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.calls) != 3 {
		t.Fatal("token body was replayed")
	}
}

func TestHTTPRejectsOversizedBodiesBeforeRequest(t *testing.T) {
	t.Parallel()
	client, fixture := fixtureClient(t)
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://issuer.example/token", strings.NewReader(strings.Repeat("x", MaxBody+1)))
	response, err := client.HTTPClient().Do(request)
	if response != nil {
		defer response.Body.Close()
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.calls) != 0 {
		t.Fatal("oversized body reached the transport")
	}
}

func TestHTTPCancellationRemainsContextError(t *testing.T) {
	t.Parallel()
	client, _ := fixtureClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://issuer.example/wait", nil)
	response, err := client.HTTPClient().Do(request)
	if response != nil {
		defer response.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
