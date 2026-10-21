// Package api binds Go clients to the Atmos API served over the plugin broker.
package api

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

const (
	InvocationMetadata = "atmos-invocation-id"
	MaxBody            = 8 << 20
)

var (
	ErrNotConfigured   = errors.New("Atmos API is not configured")
	ErrProtocol        = errors.New("invalid Atmos API response")
	ErrPayloadTooLarge = errors.New("HTTP body exceeds limit")
	ErrRedirect        = errors.New("unsafe HTTP redirect")
)

// Client is configured once by the Plugin SDK after the broker handshake.
type Client struct {
	mu         sync.RWMutex
	connection grpc.ClientConnInterface
}

// Bind installs the authenticated broker connection before invocations start.
func (c *Client) Bind(connection grpc.ClientConnInterface) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connection != nil || connection == nil {
		return ErrProtocol
	}
	c.connection = connection
	return nil
}

func (c *Client) conn() (grpc.ClientConnInterface, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.connection == nil {
		return nil, ErrNotConfigured
	}
	return c.connection, nil
}

// WithInvocation scopes API requests to the active plugin invocation.
func WithInvocation(ctx context.Context, id string) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(InvocationMetadata, id)
	return metadata.NewOutgoingContext(ctx, md)
}

// Get reads private state scoped by Atmos to this package and configured auth session.
func (c *Client) Get(ctx context.Context, key string) ([]byte, bool, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, false, err
	}
	result, err := apiv1.NewCredentialsClient(conn).Get(ctx, &apiv1.GetRequest{Key: key})
	if err != nil {
		return nil, false, err
	}
	return result.GetData(), result.GetFound(), nil
}

// Put persists a complete replacement of private session state.
func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	conn, err := c.conn()
	if err != nil {
		return err
	}
	_, err = apiv1.NewCredentialsClient(conn).Put(ctx, &apiv1.PutRequest{Key: key, Data: data})
	return err
}

// Delete removes private state from this invocation's session namespace.
func (c *Client) Delete(ctx context.Context, key string) error {
	conn, err := c.conn()
	if err != nil {
		return err
	}
	_, err = apiv1.NewCredentialsClient(conn).Delete(ctx, &apiv1.DeleteRequest{Key: key})
	return err
}
