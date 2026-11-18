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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

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

// Resolver turns identity names into per-fetch environments. A successful resolution is kept for
// the life of the Resolver, so an identity is resolved once however many sources name it and
// however concurrently they are fetched. A failed resolution is not kept: the next fetch that names
// the identity tries again, so one transient error does not fail every later source.
//
// Resolutions are scoped to the auth configuration they were made under (see scopeKey). An
// identity name resolved for one configuration is never served to another, and each configuration
// gets its own auth manager.
type Resolver struct {
	newManager func(*schema.AtmosConfiguration) (identityManager, error)
	inCI       func() bool

	mu     sync.Mutex
	scopes map[string]*scope
}

// scope holds what was resolved under one auth configuration.
type scope struct {
	// The mu mutex serializes every use of manager. The Atmos Auth manager keeps the chain it is
	// authenticating in a field, so two identities must not be resolved through it at once.
	mu      sync.Mutex
	manager identityManager
	envs    map[string][]string
}

// NewResolver creates a Resolver backed by the Atmos Auth manager built from atmos.yaml.
func NewResolver(opts ...Option) *Resolver {
	defer perf.Track(nil, "remoteread.NewResolver")()

	r := &Resolver{
		newManager: defaultManager,
		inCI:       telemetry.IsCI,
		scopes:     map[string]*scope{},
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

	sc := r.scopeFor(atmosConfig)
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if env, ok := sc.envs[identity]; ok {
		return append([]string(nil), env...), nil
	}

	env, err := r.compose(ctx, sc, atmosConfig, identity)
	if err != nil {
		return nil, err
	}
	sc.envs[identity] = env
	return append([]string(nil), env...), nil
}

// scopeFor returns the scope for the auth configuration of atmosConfig, creating it on first use.
// A configuration that cannot be keyed gets a scope of its own that is not kept, so it is resolved
// afresh every time and never shares a manager or an environment with another configuration.
func (r *Resolver) scopeFor(atmosConfig *schema.AtmosConfiguration) *scope {
	key, ok := scopeKey(atmosConfig)
	if !ok {
		return &scope{envs: map[string][]string{}}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	sc, found := r.scopes[key]
	if !found {
		sc = &scope{envs: map[string][]string{}}
		r.scopes[key] = sc
	}
	return sc
}

// scopeKey identifies the auth configuration a resolution belongs to: the `auth` section (which
// carries the realm, providers, and identities) and the directory of the atmos.yaml it came from
// (which the default realm and the profile fallback are derived from). These are exactly the inputs
// the auth manager is built from.
func scopeKey(atmosConfig *schema.AtmosConfiguration) (string, bool) {
	if atmosConfig == nil {
		return "", false
	}
	authSection, err := yaml.Marshal(&atmosConfig.Auth)
	if err != nil {
		return "", false
	}
	digest := sha256.New()
	digest.Write(authSection)
	digest.Write([]byte{0})
	digest.Write([]byte(atmosConfig.Auth.RealmSource))
	digest.Write([]byte{0})
	digest.Write([]byte(atmosConfig.CliConfigPath))
	return hex.EncodeToString(digest.Sum(nil)), true
}

// compose resolves identity through the scope's manager. The caller holds sc.mu.
func (r *Resolver) compose(ctx context.Context, sc *scope, atmosConfig *schema.AtmosConfiguration, identity string) ([]string, error) {
	if sc.manager == nil {
		manager, err := r.newManager(atmosConfig)
		if err != nil {
			return nil, fmt.Errorf("%w: creating the auth manager for identity %q: %w", errUtils.ErrAuthManager, identity, err)
		}
		sc.manager = manager
	}
	// A nil base keeps the process environment out of the overlay: the downloader applies the
	// overlay to the process environment itself, per subprocess.
	return atmosgit.ComposeEnvironment(ctx, nil, identity, &noPromptProvider{manager: sc.manager, inCI: r.inCI})
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
