package agent

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// ErrUnsupportedPlatform means the agent daemon cannot run on this operating system. Windows needs
// named pipes, which come in a later phase.
var ErrUnsupportedPlatform = errors.New("the SSH agent is not supported on this platform")

// ErrSocketInUse means another process is already listening on an identity's socket.
var ErrSocketInUse = errors.New("socket is already in use")

// DefaultPollInterval is how often the daemon checks the configuration file's modification time.
const DefaultPollInterval = 2 * time.Second

// StoreOpener builds the key store of the given kind that keeps its key material in dir.
type StoreOpener func(kind, dir string) (keystore.Store, error)

// Options configures Serve.
type Options struct {
	// Paths locates the configuration, sockets, keys, audit log and state file. Required.
	Paths *paths.Paths
	// Version is recorded in the daemon state file.
	Version string
	// OpenStore builds key stores. It defaults to keystore.New. Tests inject fakes here.
	OpenStore StoreOpener
	// PollInterval is how often the configuration file is checked for changes. It defaults to
	// DefaultPollInterval.
	PollInterval time.Duration
	// Logf receives the daemon's own diagnostics. It defaults to discarding them.
	Logf func(format string, args ...any)
	// peer overrides the peer credential lookup. Tests use it to simulate another user.
	peer func(*net.UnixConn) (uint32, error)
	// Ready, when set, is called once after the first configuration has been applied.
	Ready func()
}

// withDefaults returns o with every unset optional field filled in.
func (o Options) withDefaults() (Options, error) {
	if o.Paths == nil {
		return o, fmt.Errorf("%w: Options.Paths is required", ErrInvalidConfig)
	}
	if o.OpenStore == nil {
		o.OpenStore = func(kind, dir string) (keystore.Store, error) { return keystore.New(kind, dir) }
	}
	if o.PollInterval <= 0 {
		o.PollInterval = DefaultPollInterval
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	return o, nil
}
