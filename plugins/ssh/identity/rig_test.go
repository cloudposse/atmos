package identity_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
	"github.com/cloudposse/atmos/plugins/ssh/authority/github"
	"github.com/cloudposse/atmos/plugins/ssh/authority/self"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/service"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

const (
	version  = "0.1.0"
	hostname = "testhost"
	home     = "/home/tester"
	keygen   = "/usr/bin/ssh-keygen"
)

// rig is an identity service wired to fakes, rooted in a temporary directory.
type rig struct {
	t      *testing.T
	paths  *paths.Paths
	fake   *authoritytest.Fake
	mgr    *testfakes.Manager
	state  *testfakes.State
	clock  *testfakes.Clock
	deps   identity.Deps
	checks []string
	// socketErrs makes the first CheckSocket calls fail.
	socketErrs int
	installs   int
	keygenErr  error
	svc        *identity.Service
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("unexpected network call to %s", r.URL)
	return nil, errors.New("network is not available in tests")
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		t: t, paths: testfakes.Paths(t), fake: authoritytest.NewFake("fake"),
		mgr: &testfakes.Manager{Version: version}, state: testfakes.NewState(), clock: testfakes.NewClock(),
	}
	reg := authority.NewRegistry()
	require.NoError(t, reg.Register(authority.Factory{Name: "fake", New: func(authority.Options) (authority.Authority, error) { return r.fake, nil }}))
	require.NoError(t, reg.Register(github.Factory))
	require.NoError(t, reg.Register(self.Factory))
	r.deps = identity.Deps{
		Paths:       func(string) (*paths.Paths, error) { return r.paths, nil },
		OpenStore:   testfakes.FileOpener,
		Authorities: reg,
		Daemon: daemon.Deps{
			Service: func(*paths.Paths) (service.Manager, error) { return r.mgr, nil },
			InstallExecutable: func(p *paths.Paths, v string) (string, error) {
				r.installs++
				return service.StablePath(p, v)
			},
			Version: version,
		},
		CheckSocket: func(_ context.Context, socket string, want ssh.PublicKey) error {
			r.checks = append(r.checks, socket)
			if r.socketErrs > 0 {
				r.socketErrs--
				return errors.New("socket not up yet")
			}
			return nil
		},
		HTTPClient:    &http.Client{Transport: failTransport{t}},
		State:         r.state,
		Now:           r.clock.Now,
		Hostname:      func() (string, error) { return hostname, nil },
		HomeDir:       func() (string, error) { return home, nil },
		FindSSHKeygen: func() (string, error) { return keygen, r.keygenErr },
		Sleep:         r.clock.Sleep,
	}
	r.svc = identity.New(r.deps)
	return r
}

// use rebuilds the service after the test changed r.deps.
func (r *rig) use() *rig {
	r.svc = identity.New(r.deps)
	return r
}

func fakeToken() map[string]string {
	return map[string]string{authority.CredentialToken: authoritytest.FakeToken}
}

func baseSpec(extra map[string]any) map[string]any {
	spec := map[string]any{"authority": "fake", "store": "file", "allow": []any{"signing"}}
	for k, v := range extra {
		spec[k] = v
	}
	return spec
}

func (r *rig) authenticate(name string, spec map[string]any, upstream map[string]string) (identity.Result, error) {
	r.t.Helper()
	return r.svc.Authenticate(context.Background(), name, spec, upstream)
}

func (r *rig) mustAuthenticate(name string, spec map[string]any) identity.Result {
	r.t.Helper()
	res, err := r.authenticate(name, spec, fakeToken())
	require.NoError(r.t, err)
	return res
}

func (r *rig) mustAuthenticateWith(name string, spec map[string]any, upstream map[string]string) identity.Result {
	r.t.Helper()
	res, err := r.authenticate(name, spec, upstream)
	require.NoError(r.t, err)
	return res
}

func (r *rig) store() keystore.Store { return keystore.NewFileStore(r.paths.StoreDir("file")) }

func (r *rig) pub(name string) ssh.PublicKey {
	r.t.Helper()
	return testfakes.PublicKeyOf(r.t, r.store(), name)
}

func (r *rig) status(name string) statusfile.Status {
	r.t.Helper()
	s, err := statusfile.Read(r.paths, name)
	require.NoError(r.t, err)
	return s
}

func (r *rig) receipts() []authority.Receipt {
	r.t.Helper()
	store, err := authority.NewStore(r.paths.ReceiptsDir())
	require.NoError(r.t, err)
	list, err := store.List()
	require.NoError(r.t, err)
	return list
}

func (r *rig) agentConfig() *agent.Config {
	r.t.Helper()
	cfg, err := agent.LoadConfig(r.paths.AgentConfigFile())
	if errors.Is(err, agent.ErrConfigNotFound) {
		return &agent.Config{}
	}
	require.NoError(r.t, err)
	return cfg
}

func (r *rig) readFile(path string) string {
	r.t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(r.t, err)
	return string(data)
}

func (r *rig) gitConfigPath(name string) string {
	r.t.Helper()
	return filepath.Join(r.paths.Root(), "git", name+".gitconfig")
}

func ensureCalls(f *authoritytest.Fake) int { return f.CallCount("ensure") }
