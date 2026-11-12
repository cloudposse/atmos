// Package testfakes holds the fakes the SSH plugin's tests share: a service manager that never
// touches launchd or systemd, a private-state store, a settable clock, and key stores that model
// requirements and a locked screen.
package testfakes

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

// ShortDir makes a temporary directory whose path is short enough for Unix sockets.
func ShortDir(t *testing.T) string {
	t.Helper()
	base := ""
	if _, err := os.Stat("/tmp"); err == nil {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "sshp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// Paths returns Paths under a short temporary directory.
func Paths(t *testing.T) *paths.Paths {
	t.Helper()
	p, err := paths.New(filepath.Join(ShortDir(t), "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Clock is a clock that only moves when told to.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock returns a clock set to a fixed instant.
func NewClock() *Clock {
	return &Clock{now: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
}

// Now returns the current instant.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Sleep advances the clock instead of waiting, and counts the calls.
func (c *Clock) Sleep(_ context.Context, d time.Duration) error {
	c.Advance(d)
	return nil
}

// Manager is a service.Manager that records calls and keeps its state in memory.
type Manager struct {
	mu         sync.Mutex
	St         service.Status
	Version    string
	Installs   []service.Spec
	Starts     int
	Stops      int
	StatusErr  error
	InstallErr error
}

// Status implements service.Manager.
func (m *Manager) Status(context.Context) (service.Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.St, m.StatusErr
}

// Install implements service.Manager. The service ends up installed and running the plugin version.
func (m *Manager) Install(_ context.Context, spec service.Spec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.InstallErr != nil {
		return m.InstallErr
	}
	m.Installs = append(m.Installs, spec)
	m.St = service.Status{
		Installed: true, Loaded: true, Running: true, PID: 4242, Executable: spec.Executable,
		InstalledVersion: m.Version, DaemonVersion: m.Version,
	}
	return nil
}

// Start implements service.Manager.
func (m *Manager) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Starts++
	return nil
}

// Stop implements service.Manager.
func (m *Manager) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Stops++
	m.St.Running, m.St.Loaded = false, false
	return nil
}

// Uninstall implements service.Manager.
func (m *Manager) Uninstall(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.St = service.Status{}
	return nil
}

// InstallCount returns how many times Install succeeded.
func (m *Manager) InstallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Installs)
}

// State is an in-memory private state.
type State struct {
	mu      sync.Mutex
	Data    map[string][]byte
	Puts    int
	Gets    int
	PutErr  error
	GetErr  error
	Deleted []string
}

// NewState returns an empty state.
func NewState() *State { return &State{Data: map[string][]byte{}} }

// Get implements the private-state interface.
func (s *State) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Gets++
	if s.GetErr != nil {
		return nil, false, s.GetErr
	}
	v, ok := s.Data[key]
	return v, ok, nil
}

// Put implements the private-state interface.
func (s *State) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Puts++
	if s.PutErr != nil {
		return s.PutErr
	}
	s.Data[key] = data
	return nil
}

// Delete implements the private-state interface.
func (s *State) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Deleted = append(s.Deleted, key)
	delete(s.Data, key)
	return nil
}

// FileOpener opens the real "file" store, and reports every other kind as unsupported, as a
// machine without that hardware would.
func FileOpener(kind, dir string) (keystore.Store, error) {
	if kind == keystore.KindFile {
		return keystore.NewFileStore(dir), nil
	}
	return nil, fmt.Errorf("%w: %s", keystore.ErrUnsupportedPlatform, kind)
}

// RequirementOpener opens a "file" store that claims it can enforce "unlocked" and remembers the
// requirements each key was created with, as the Secure Enclave store does. The memory is shared
// by every store the opener returns.
func RequirementOpener() func(kind, dir string) (keystore.Store, error) {
	reqs := &requirementMemory{m: map[string][]keystore.Requirement{}}
	return func(kind, dir string) (keystore.Store, error) {
		if kind != keystore.KindFile {
			return nil, fmt.Errorf("%w: %s", keystore.ErrUnsupportedPlatform, kind)
		}
		return &requirementStore{Store: keystore.NewFileStore(dir), mem: reqs, dir: dir}, nil
	}
}

type requirementMemory struct {
	mu sync.Mutex
	m  map[string][]keystore.Requirement
}

type requirementStore struct {
	keystore.Store
	mem *requirementMemory
	dir string
}

func (s *requirementStore) Supports(keystore.Requirement) bool { return true }

func (s *requirementStore) id(name string) string { return s.dir + "|" + name }

func (s *requirementStore) Create(ctx context.Context, name string, reqs []keystore.Requirement) (keystore.Key, error) {
	k, err := s.Store.Create(ctx, name, nil)
	if err != nil {
		return nil, err
	}
	s.mem.mu.Lock()
	s.mem.m[s.id(name)] = keystore.Normalize(reqs)
	s.mem.mu.Unlock()
	return &requirementKey{Key: k, reqs: keystore.Normalize(reqs)}, nil
}

func (s *requirementStore) Load(ctx context.Context, name string) (keystore.Key, error) {
	k, err := s.Store.Load(ctx, name)
	if err != nil {
		return nil, err
	}
	s.mem.mu.Lock()
	defer s.mem.mu.Unlock()
	return &requirementKey{Key: k, reqs: s.mem.m[s.id(name)]}, nil
}

func (s *requirementStore) List(ctx context.Context) ([]keystore.Info, error) {
	infos, err := s.Store.List(ctx)
	s.mem.mu.Lock()
	defer s.mem.mu.Unlock()
	for i := range infos {
		infos[i].Requirements = s.mem.m[s.id(infos[i].Name)]
	}
	return infos, err
}

type requirementKey struct {
	keystore.Key
	reqs []keystore.Requirement
}

func (k *requirementKey) Requirements() []keystore.Requirement { return k.reqs }

// LockedOpener opens a "file" store whose keys refuse to sign, the way the Secure Enclave does
// while the screen is locked (errSecInteractionNotAllowed).
func LockedOpener(kind, dir string) (keystore.Store, error) {
	if kind != keystore.KindFile {
		return nil, fmt.Errorf("%w: %s", keystore.ErrUnsupportedPlatform, kind)
	}
	return &lockedStore{Store: keystore.NewFileStore(dir)}, nil
}

type lockedStore struct{ keystore.Store }

func (s *lockedStore) Load(ctx context.Context, name string) (keystore.Key, error) {
	k, err := s.Store.Load(ctx, name)
	if err != nil {
		return nil, err
	}
	return &lockedKey{Key: k}, nil
}

type lockedKey struct{ keystore.Key }

// ErrLocked is what a locked key returns when asked to sign.
var ErrLocked = &keystore.OSStatusError{Op: "sign", Code: keystore.CodeInteractionNotAllowed, Message: "interaction not allowed"}

func (k *lockedKey) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) { return nil, ErrLocked }

// FailingLoad wraps a store so that Load fails with err.
func FailingLoad(inner keystore.Store, err error) keystore.Store {
	return &failingLoad{Store: inner, err: err}
}

type failingLoad struct {
	keystore.Store
	err error
}

func (s *failingLoad) Load(context.Context, string) (keystore.Key, error) { return nil, s.err }

// PublicKeyOf returns the public key of the named key in the store at dir.
func PublicKeyOf(t *testing.T, store keystore.Store, name string) ssh.PublicKey {
	t.Helper()
	k, err := store.Load(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	return k.SSHPublicKey()
}

// ErrBoom is a generic failure for tests.
var ErrBoom = errors.New("boom")
