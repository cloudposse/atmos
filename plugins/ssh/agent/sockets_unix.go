//go:build !windows

package agent

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/agent"
)

// ErrWrongUser means a connection came from a different user than the one running the daemon.
var ErrWrongUser = errors.New("connection from a different user")

const (
	socketMode os.FileMode = 0o600
	// staleProbe is how long to wait when checking whether an existing socket is alive.
	staleProbe = 500 * time.Millisecond
	// acceptBackoff is the pause after a temporary accept failure.
	acceptBackoff = 50 * time.Millisecond
)

// socketEntry is one live identity socket and the connections it is serving.
type socketEntry struct {
	id    Identity
	path  string
	ln    net.Listener
	agent *identityAgent

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
}

// track records a connection. It reports false once the socket is closing.
func (e *socketEntry) track(c net.Conn) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false
	}
	e.conns[c] = struct{}{}
	return true
}

func (e *socketEntry) untrack(c net.Conn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.conns, c)
}

// close stops accepting, drops open connections and removes the socket file.
func (e *socketEntry) close() {
	e.mu.Lock()
	e.closed = true
	conns := make([]net.Conn, 0, len(e.conns))
	for c := range e.conns {
		conns = append(conns, c)
	}
	e.mu.Unlock()

	_ = e.ln.Close()
	for _, c := range conns {
		_ = c.Close()
	}
	_ = os.Remove(e.path)
}

// removeStaleSocket deletes a leftover socket file. It refuses to touch anything that is not a
// socket, and fails with ErrSocketInUse if something is still listening on it.
func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket", path)
	}
	if conn, err := net.DialTimeout("unix", path, staleProbe); err == nil {
		_ = conn.Close()
		return fmt.Errorf("%w: %s", ErrSocketInUse, path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale socket %s: %w", path, err)
	}
	return nil
}

// startSocket opens the identity's store and socket and starts serving it.
func (d *daemon) startSocket(id Identity) (*socketEntry, error) {
	sockPath, err := d.opts.Paths.SocketPath(id.Name)
	if err != nil {
		return nil, err
	}
	store, err := d.opts.OpenStore(id.Store, d.opts.Paths.StoreDir(id.Store))
	if err != nil {
		return nil, fmt.Errorf("identity %q: open key store %q: %w", id.Name, id.Store, err)
	}
	if err := os.MkdirAll(d.opts.Paths.SocketsDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create sockets directory: %w", err)
	}
	if err := removeStaleSocket(sockPath); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", sockPath, err)
	}
	if err := os.Chmod(sockPath, socketMode); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("restrict %s: %w", sockPath, err)
	}
	e := &socketEntry{
		id: id, path: sockPath, ln: ln, conns: map[net.Conn]struct{}{},
		agent: &identityAgent{id: id, store: store, audit: d.audit, onAuditError: d.auditError},
	}
	d.wg.Add(1)
	go d.acceptLoop(e)
	return e, nil
}

func (d *daemon) auditError(err error) { d.opts.Logf("audit log: %v", err) }

// acceptLoop serves connections until the socket is closed.
func (d *daemon) acceptLoop(e *socketEntry) {
	defer d.wg.Done()
	for {
		conn, err := e.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			d.opts.Logf("accept on %s: %v", e.path, err)
			time.Sleep(acceptBackoff)
			continue
		}
		if err := d.admit(e, conn); err != nil {
			d.opts.Logf("identity %q: %v", e.id.Name, err)
			e.agent.record(AuditEntry{Operation: OpConnect, Result: ResultRefused, Reason: err.Error()})
			_ = conn.Close()
			continue
		}
		d.wg.Add(1)
		go d.serveConn(e, conn)
	}
}

// admit checks the connecting user and registers the connection.
func (d *daemon) admit(e *socketEntry, conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("%w: not a unix connection", ErrWrongUser)
	}
	uid, err := d.peer(uc)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWrongUser, err)
	}
	if err := verifyPeer(uid, uint32(os.Getuid())); err != nil {
		return err
	}
	if !e.track(conn) {
		return net.ErrClosed
	}
	return nil
}

// verifyPeer accepts only a connection from the daemon's own user.
func verifyPeer(got, want uint32) error {
	if got != want {
		return fmt.Errorf("%w: uid %d, want %d", ErrWrongUser, got, want)
	}
	return nil
}

func (d *daemon) serveConn(e *socketEntry, conn net.Conn) {
	defer d.wg.Done()
	defer e.untrack(conn)
	defer conn.Close()
	if err := agent.ServeAgent(e.agent, conn); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		d.opts.Logf("identity %q: connection ended: %v", e.id.Name, err)
	}
}
