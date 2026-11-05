package api

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

// Open launches the external browser through Atmos.
func (c *Client) Open(ctx context.Context, url string) error {
	conn, err := c.conn()
	if err != nil {
		return err
	}
	_, err = apiv1.NewBrowserClient(conn).Open(ctx, &apiv1.OpenRequest{Url: url})
	return err
}

// CallbackListener owns a ready loopback callback stream.
type CallbackListener struct {
	redirectURI string
	stream      grpc.ServerStreamingClient[apiv1.CallbackEvent]
	cancel      context.CancelFunc
}

// Listen waits for readiness before returning the exact redirect URI to use.
func (c *Client) Listen(ctx context.Context, path, state string) (*CallbackListener, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	stream, err := apiv1.NewBrowserClient(conn).Listen(lifetime, &apiv1.ListenRequest{Path: path, ExpectedState: state})
	if err != nil {
		cancel()
		return nil, err
	}
	first, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, err
	}
	if first.GetReady().GetRedirectUri() == "" {
		cancel()
		return nil, ErrProtocol
	}
	return &CallbackListener{redirectURI: first.GetReady().GetRedirectUri(), stream: stream, cancel: cancel}, nil
}

func (l *CallbackListener) RedirectURI() string { return l.redirectURI }

// Receive accepts exactly one terminal callback and verifies stream completion.
func (l *CallbackListener) Receive() (*apiv1.Callback, error) {
	event, err := l.stream.Recv()
	if err != nil {
		return nil, err
	}
	callback := event.GetCallback()
	if callback == nil {
		return nil, ErrProtocol
	}
	if _, err := l.stream.Recv(); !errors.Is(err, io.EOF) {
		return nil, ErrProtocol
	}
	return callback, nil
}

// Close cancels the stream and releases Atmos's listener.
func (l *CallbackListener) Close() { l.cancel() }
