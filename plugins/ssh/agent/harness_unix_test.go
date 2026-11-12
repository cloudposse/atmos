//go:build !windows

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	xagent "golang.org/x/crypto/ssh/agent"

	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// shortDir makes a temporary directory with a path short enough for Unix sockets.
func shortDir(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS != "windows" {
		if _, err := os.Stat("/tmp"); err == nil {
			base = "/tmp"
		}
	}
	dir, err := os.MkdirTemp(base, "ssha")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// harness is a running daemon under a temporary root.
type harness struct {
	t      *testing.T
	paths  *paths.Paths
	opts   Options
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
}

// newPaths returns Paths under a short temporary directory.
func newPaths(t *testing.T) *paths.Paths {
	t.Helper()
	p, err := paths.New(filepath.Join(shortDir(t), "ssh"))
	require.NoError(t, err)
	require.NoError(t, p.EnsureDirs())
	return p
}

// createFileKey makes a key in the "file" store and returns its public key.
func createFileKey(t *testing.T, p *paths.Paths, name string) ssh.PublicKey {
	t.Helper()
	store := keystore.NewFileStore(p.StoreDir(keystore.KindFile))
	key, err := store.Create(context.Background(), name, nil)
	require.NoError(t, err)
	defer key.Close()
	return key.SSHPublicKey()
}

// startDaemon writes the identities to the config file and runs Serve until the test ends.
func startDaemon(t *testing.T, p *paths.Paths, mutate func(*Options), ids ...Identity) *harness {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, UpsertIdentity(p.AgentConfigFile(), id))
	}
	ready := make(chan struct{})
	opts := Options{
		Paths: p, Version: "test", PollInterval: 25 * time.Millisecond,
		Logf:  func(format string, args ...any) { t.Logf("daemon: "+format, args...) },
		Ready: func() { close(ready) },
	}
	if mutate != nil {
		mutate(&opts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{t: t, paths: p, opts: opts, cancel: cancel, done: make(chan error, 1)}
	go func() { h.done <- Serve(ctx, opts) }()
	select {
	case <-ready:
	case err := <-h.done:
		t.Fatalf("Serve returned early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not become ready")
	}
	t.Cleanup(h.Stop)
	return h
}

// Stop ends the daemon and waits for it.
func (h *harness) Stop() {
	h.once.Do(func() {
		h.cancel()
		select {
		case err := <-h.done:
			require.NoError(h.t, err)
		case <-time.After(10 * time.Second):
			h.t.Fatal("Serve did not stop")
		}
	})
}

// socket returns the socket path of an identity.
func (h *harness) socket(name string) string {
	h.t.Helper()
	s, err := h.paths.SocketPath(name)
	require.NoError(h.t, err)
	return s
}

// client connects to an identity's socket.
func (h *harness) client(name string) xagent.ExtendedAgent {
	h.t.Helper()
	conn, err := net.Dial("unix", h.socket(name))
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = conn.Close() })
	return xagent.NewClient(conn)
}

// auditEntries reads the audit log.
func (h *harness) auditEntries() []AuditEntry {
	h.t.Helper()
	f, err := os.Open(h.paths.AuditLogFile())
	require.NoError(h.t, err)
	defer f.Close()
	var out []AuditEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e AuditEntry
		require.NoError(h.t, json.Unmarshal(sc.Bytes(), &e))
		out = append(out, e)
	}
	return out
}

// lastAudit returns the newest audit entry.
func (h *harness) lastAudit() AuditEntry {
	h.t.Helper()
	entries := h.auditEntries()
	require.NotEmpty(h.t, entries)
	return entries[len(entries)-1]
}
