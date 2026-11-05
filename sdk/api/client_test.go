package api

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

// credentialFixture implements a private key-value service over the real wire protocol.
type credentialFixture struct {
	apiv1.UnimplementedCredentialsServer
	values     map[string][]byte
	invocation string
}

func (f *credentialFixture) Get(ctx context.Context, request *apiv1.GetRequest) (*apiv1.GetResponse, error) {
	if request.Key == "denied" {
		return nil, status.Error(codes.PermissionDenied, "denied")
	}
	f.invocation = metadata.ValueFromIncomingContext(ctx, InvocationMetadata)[0]
	data, found := f.values[request.Key]
	return &apiv1.GetResponse{Data: data, Found: found}, nil
}

func (f *credentialFixture) Put(_ context.Context, request *apiv1.PutRequest) (*emptypb.Empty, error) {
	f.values[request.Key] = append([]byte(nil), request.Data...)
	return &emptypb.Empty{}, nil
}

func (f *credentialFixture) Delete(_ context.Context, request *apiv1.DeleteRequest) (*emptypb.Empty, error) {
	delete(f.values, request.Key)
	return &emptypb.Empty{}, nil
}

func serviceClient(t *testing.T, register func(*grpc.Server)) (*Client, *grpc.ClientConn) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	register(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///api-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{}
	if err := client.Bind(connection); err != nil {
		t.Fatal(err)
	}
	return client, connection
}

func TestPrivateCredentialsLifecycleAndInvocationIsolation(t *testing.T) {
	t.Parallel()
	fixture := &credentialFixture{values: map[string][]byte{}}
	client, conn := serviceClient(t, func(s *grpc.Server) { apiv1.RegisterCredentialsServer(s, fixture) })
	original := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("other", "retained", InvocationMetadata, "old"))
	ctx := WithInvocation(original, "new")
	before, _ := metadata.FromOutgoingContext(original)
	after, _ := metadata.FromOutgoingContext(ctx)
	if before.Get(InvocationMetadata)[0] != "old" || after.Get("other")[0] != "retained" {
		t.Fatal("invocation metadata modified the parent or lost unrelated values")
	}
	if err := client.Put(ctx, "refresh", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := client.Put(ctx, "refresh", []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	raw, found, err := client.Get(ctx, "refresh")
	if err != nil || !found || string(raw) != "rotated" || fixture.invocation != "new" {
		t.Fatalf("get: %q %v %v", raw, found, err)
	}
	if err := client.Delete(ctx, "refresh"); err != nil {
		t.Fatal(err)
	}
	raw, found, err = client.Get(ctx, "refresh")
	if err != nil || found || len(raw) != 0 {
		t.Fatalf("deleted state returned: %q %v %v", raw, found, err)
	}
	if _, _, err := client.Get(ctx, "denied"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("RPC error lost: %v", err)
	}
	if err := client.Bind(conn); !errors.Is(err, ErrProtocol) {
		t.Fatalf("rebinding allowed: %v", err)
	}
}

func TestUnconfiguredClientsRejectOperations(t *testing.T) {
	t.Parallel()
	client := &Client{}
	if err := client.Bind(nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("nil connection accepted: %v", err)
	}
	checks := []struct {
		name string
		call func() error
	}{
		{"get", func() error { _, _, err := client.Get(t.Context(), "key"); return err }},
		{"put", func() error { return client.Put(t.Context(), "key", nil) }},
		{"delete", func() error { return client.Delete(t.Context(), "key") }},
		{"open", func() error { return client.Open(t.Context(), "https://example.com") }},
		{"listen", func() error { _, err := client.Listen(t.Context(), "/callback", "state"); return err }},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(); !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
