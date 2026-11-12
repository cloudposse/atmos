// Package remoteread resolves the Atmos Auth identity named on a remote read (a vendored source, a
// component `source:`, or a remote stack import) into the environment its Git subprocess runs with.
//
// It registers itself with pkg/downloader from init(), the same way credential brokers register,
// so the downloader stays free of a dependency on pkg/auth. The command layer blank-imports this
// package. The identity environment is composed with git.ComposeEnvironment, the primitive
// `atmos git` uses, and is handed to the fetch as a per-fetch environment: it is never exported
// with os.Setenv, so concurrent fetches can each use their own identity.
package remoteread

import (
	"context"
	"errors"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/downloader"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func init() {
	downloader.RegisterIdentityEnvironmentResolver(defaultResolver.Resolve)
}

var defaultResolver = NewResolver()

// identityManager is the part of the Atmos Auth manager a remote read needs.
type identityManager interface {
	GetCachedCredentials(ctx context.Context, identityName string) (*types.WhoamiInfo, error)
	EnsureIdentityEnvironment(ctx context.Context, identityName string) (map[string]string, error)
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithManagerFactory replaces how the auth manager is built from the configuration (for tests).
func WithManagerFactory(factory func(*schema.AtmosConfiguration) (identityManager, error)) Option {
	return func(r *Resolver) { r.newManager = factory }
}

// WithCIDetector replaces CI detection (for tests).
func WithCIDetector(inCI func() bool) Option {
	return func(r *Resolver) { r.inCI = inCI }
}

// Resolver turns identity names into per-fetch environments. Each identity is resolved at most once
// per Resolver, however many sources name it and however concurrently they are fetched.
type Resolver struct {
	newManager func(*schema.AtmosConfiguration) (identityManager, error)
	inCI       func() bool

	managerOnce sync.Once
	manager     identityManager
	managerErr  error

	mu      sync.Mutex
	results map[string]*resolution
}

type resolution struct {
	once sync.Once
	env  []string
	err  error
}

// NewResolver creates a Resolver backed by the Atmos Auth manager built from atmos.yaml.
func NewResolver(opts ...Option) *Resolver {
	r := &Resolver{
		newManager: defaultManager,
		inCI:       telemetry.IsCI,
		results:    map[string]*resolution{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func defaultManager(atmosConfig *schema.AtmosConfiguration) (identityManager, error) {
	if atmosConfig == nil {
		return nil, fmt.Errorf("%w: atmos configuration is nil", errUtils.ErrInvalidAuthConfig)
	}
	return auth.NewDefaultManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
}

// Resolve returns the environment overlay for identity, as "KEY=value" entries.
func (r *Resolver) Resolve(ctx context.Context, atmosConfig *schema.AtmosConfiguration, identity string) ([]string, error) {
	defer perf.Track(atmosConfig, "remoteread.Resolver.Resolve")()

	r.mu.Lock()
	res, ok := r.results[identity]
	if !ok {
		res = &resolution{}
		r.results[identity] = res
	}
	r.mu.Unlock()

	res.once.Do(func() {
		res.env, res.err = r.compose(ctx, atmosConfig, identity)
	})
	if res.err != nil {
		return nil, res.err
	}
	return append([]string(nil), res.env...), nil
}

func (r *Resolver) compose(ctx context.Context, atmosConfig *schema.AtmosConfiguration, identity string) ([]string, error) {
	r.managerOnce.Do(func() {
		r.manager, r.managerErr = r.newManager(atmosConfig)
	})
	if r.managerErr != nil {
		return nil, fmt.Errorf("%w: creating the auth manager for identity %q: %w", errUtils.ErrAuthManager, identity, r.managerErr)
	}
	// A nil base keeps the process environment out of the overlay: the downloader applies the
	// overlay to the process environment itself, per subprocess.
	return atmosgit.ComposeEnvironment(ctx, nil, identity, &noPromptProvider{manager: r.manager, inCI: r.inCI})
}

// noPromptProvider guards identity resolution so a remote read never prompts mid-fetch. Locally an
// identity must already hold a valid session (`atmos auth login`); in CI there is nobody to prompt
// and providers authenticate from the ambient environment (OIDC), so resolution may authenticate.
// Prompting is also disabled on the context for the identity kinds that honor it.
type noPromptProvider struct {
	manager identityManager
	inCI    func() bool
}

// EnsureIdentityEnvironment satisfies git.IdentityEnvironmentProvider.
func (p *noPromptProvider) EnsureIdentityEnvironment(ctx context.Context, identity string) (map[string]string, error) {
	ctx = types.WithAllowPrompts(ctx, false)

	if !p.inCI() {
		if _, err := p.manager.GetCachedCredentials(ctx, identity); err != nil {
			return nil, loginRequired(identity, err)
		}
	}

	env, err := p.manager.EnsureIdentityEnvironment(ctx, identity)
	if err != nil {
		return nil, loginRequired(identity, err)
	}
	return env, nil
}

// loginRequired maps an authentication failure to the static sentinel plus the login hint. Errors
// that are not about missing credentials (an unknown identity, an invalid configuration) pass
// through unchanged because logging in would not fix them.
func loginRequired(identity string, cause error) error {
	for _, passthrough := range []error{
		errUtils.ErrIdentityNotFound, errUtils.ErrInvalidAuthConfig, errUtils.ErrInvalidIdentityConfig,
		errUtils.ErrInvalidIdentityKind, errUtils.ErrProviderNotFound, errUtils.ErrCircularDependency,
	} {
		if errors.Is(cause, passthrough) {
			return cause
		}
	}
	return errUtils.Build(errUtils.ErrRemoteIdentityLoginRequired).
		WithCause(cause).
		WithExplanation(fmt.Sprintf("Identity `%s` is used for a remote read but has no valid session. Atmos never prompts in the middle of a fetch.", identity)).
		WithHintf("Run `atmos auth login --identity %s`, then retry", identity).
		WithContext("identity", identity).
		Err()
}
