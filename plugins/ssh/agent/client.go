package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ErrUnexpectedKeys means a socket answered but did not list exactly the expected key.
var ErrUnexpectedKeys = errors.New("socket does not list the expected key")

// dialTimeout bounds how long CheckSocket waits when the context has no deadline.
const dialTimeout = 5 * time.Second

// CheckSocket verifies that an identity's socket is alive and lists exactly the key want. When
// want is nil it only checks that the socket answers a list request.
func CheckSocket(ctx context.Context, socketPath string, want ssh.PublicKey) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dialTimeout)
		defer cancel()
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to agent socket %s: %w", socketPath, err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	keys, err := agent.NewClient(conn).List()
	if err != nil {
		return fmt.Errorf("list keys on %s: %w", socketPath, err)
	}
	if want == nil {
		return nil
	}
	if len(keys) != 1 || string(keys[0].Blob) != string(want.Marshal()) {
		return fmt.Errorf("%w: %s lists %d key(s)", ErrUnexpectedKeys, socketPath, len(keys))
	}
	return nil
}
