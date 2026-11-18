package command_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/command"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

const version = "0.1.0"

type rig struct {
	t     *testing.T
	paths *paths.Paths
	mgr   *testfakes.Manager
	clock *testfakes.Clock
	deps  command.Deps
	roots []string
	// socketErr, when set, is what CheckSocket returns.
	socketErr error
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, paths: testfakes.Paths(t), mgr: &testfakes.Manager{Version: version}, clock: testfakes.NewClock()}
	r.deps = command.Deps{
		Paths: func(root string) (*paths.Paths, error) {
			r.roots = append(r.roots, root)
			if root != "" {
				return paths.New(root)
			}
			return r.paths, nil
		},
		OpenStore: testfakes.FileOpener,
		Daemon: daemon.Deps{
			Service:           func(*paths.Paths) (service.Manager, error) { return r.mgr, nil },
			InstallExecutable: service.StablePath,
			Version:           version,
		},
		CheckSocket: func(context.Context, string, ssh.PublicKey) error { return r.socketErr },
		Dial: func(ctx context.Context, socket string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
		Now:      r.clock.Now,
		Sleep:    r.clock.Sleep,
		Hostname: func() (string, error) { return "testhost", nil },
	}
	return r
}

func (r *rig) run(args ...string) (code int, stdout, stderr string) {
	r.t.Helper()
	return r.runWith(nil, args...)
}

func (r *rig) runWith(settings map[string]any, args ...string) (code int, stdout, stderr string) {
	r.t.Helper()
	var out, errw bytes.Buffer
	code = command.New(r.deps).Run(context.Background(), args, settings, &out, &errw)
	return code, out.String(), errw.String()
}

func (r *rig) store() keystore.Store { return keystore.NewFileStore(r.paths.StoreDir("file")) }

func (r *rig) createKey(name string) ssh.PublicKey {
	r.t.Helper()
	k, err := r.store().Create(context.Background(), name, nil)
	require.NoError(r.t, err)
	defer k.Close()
	return k.SSHPublicKey()
}

func (r *rig) hasKey(name string) bool {
	r.t.Helper()
	_, err := r.store().Load(context.Background(), name)
	if errors.Is(err, keystore.ErrKeyNotFound) {
		return false
	}
	require.NoError(r.t, err)
	return true
}

func (r *rig) configure(id agent.Identity) {
	r.t.Helper()
	require.NoError(r.t, agent.UpsertIdentity(r.paths.AgentConfigFile(), id))
}

func (r *rig) saveReceipt(rec authority.Receipt) {
	r.t.Helper()
	store, err := authority.NewStore(r.paths.ReceiptsDir())
	require.NoError(r.t, err)
	require.NoError(r.t, store.Save(rec))
}

func (r *rig) writeFile(path, content string) {
	r.t.Helper()
	require.NoError(r.t, os.MkdirAll(dirOf(path), 0o700))
	require.NoError(r.t, os.WriteFile(path, []byte(content), 0o600))
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if os.IsPathSeparator(path[i]) {
			return path[:i]
		}
	}
	return "."
}
