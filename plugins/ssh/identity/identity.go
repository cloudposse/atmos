// Package identity implements the ssh/key identity: it creates the key, has an authority vouch
// for it, keeps the agent daemon running, and returns the environment that makes tools use it.
//
// Authenticate runs on every use of the identity, each time in a fresh process, so it is
// idempotent and does nothing expensive when nothing changed. Every outside dependency is injected
// through Deps.
package identity

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

const (
	// CredentialLifetime is how long the environment credential is valid. The host rejects a
	// credential without an expiry, and every use authenticates again anyway.
	CredentialLifetime = 12 * time.Hour

	// StateTokenKey is the private-state entry that holds the upstream token for logout.
	StateTokenKey = "upstream-token"

	// socketAttempts and socketInterval bound the wait for the daemon to serve a new socket. The
	// daemon notices configuration changes by polling, so a new socket takes a few seconds.
	socketAttempts = 40
	socketInterval = 250 * time.Millisecond
)

// Environment variable names the identity sets.
const (
	EnvAuthSock   = "SSH_AUTH_SOCK"
	EnvGitConfig  = "GIT_CONFIG_GLOBAL"
	EnvAnsibleUsr = "ANSIBLE_REMOTE_USER"
)

// PrivateState is the host's private session state for this identity. It survives between
// invocations and is cleared by the host after logout.
type PrivateState interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, data []byte) error
	Delete(ctx context.Context, key string) error
}

// Deps are the outside dependencies of the identity.
type Deps struct {
	// Paths returns the data root; an empty root means the default location.
	Paths func(root string) (*paths.Paths, error)
	// OpenStore builds a key store.
	OpenStore agent.StoreOpener
	// Authorities builds authorities by name.
	Authorities *authority.Registry
	// Daemon makes sure the agent daemon is running the current version.
	Daemon daemon.Deps
	// CheckSocket verifies that an identity's socket serves the expected key.
	CheckSocket func(ctx context.Context, socket string, want ssh.PublicKey) error
	// HTTPClient is the client authorities use. The caller binds the context to the invocation.
	HTTPClient *http.Client
	// State is the identity's private state.
	State PrivateState
	// Now is the clock.
	Now func() time.Time
	// Hostname returns the machine's name, for the key's title.
	Hostname func() (string, error)
	// HomeDir returns the user's home directory, for the Git configuration includes.
	HomeDir func() (string, error)
	// FindSSHKeygen returns the absolute path of ssh-keygen.
	FindSSHKeygen func() (string, error)
	// Sleep waits for d, or until ctx ends.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Service is the ssh/key identity.
type Service struct {
	deps Deps
}

// New returns the identity service.
func New(deps Deps) *Service { return &Service{deps: deps} }

// Result is what Authenticate returns.
type Result struct {
	// Environment is the variables to set.
	Environment map[string]string
	// ExpiresAt is when the credential stops being valid.
	ExpiresAt time.Time
}

// Validate checks an identity's spec without touching anything.
func (s *Service) Validate(name string, raw map[string]any) error {
	if err := paths.ValidateIdentity(name); err != nil {
		return classify(name, name, err)
	}
	if _, err := ParseSpec(raw); err != nil {
		return classify(name, name, err)
	}
	return nil
}
