// Package gitenv composes the environment `atmos git` runs its Git subprocesses with.
//
// The environment is the process environment plus the environment of the Atmos Auth identity in
// effect. The identity is chosen with this precedence: the global `--identity` flag (or
// `ATMOS_IDENTITY`), then `git.repositories.<name>.auth.identity`, then nothing, in which case the
// process environment is returned unchanged and ambient credentials apply.
//
// `atmos git` is a command the user runs on purpose, so unlike a remote read (see
// pkg/auth/remoteread) it may authenticate interactively when the identity has no session.
package gitenv

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// IdentityManager is the part of the Atmos Auth manager `atmos git` needs.
type IdentityManager interface {
	GetDefaultIdentity(forceSelect bool) (string, error)
	EnsureIdentityEnvironment(ctx context.Context, identityName string) (map[string]string, error)
}

// Composer builds Git subprocess environments from Atmos Auth identities.
type Composer struct {
	newManager func(*schema.AtmosConfiguration) (IdentityManager, error)
}

// Option configures a Composer.
type Option func(*Composer)

// WithManagerFactory replaces how the auth manager is built from the configuration (for tests).
func WithManagerFactory(factory func(*schema.AtmosConfiguration) (IdentityManager, error)) Option {
	return func(c *Composer) { c.newManager = factory }
}

// NewComposer creates a Composer backed by the Atmos Auth manager built from atmos.yaml.
func NewComposer(opts ...Option) *Composer {
	defer perf.Track(nil, "gitenv.NewComposer")()

	c := &Composer{newManager: defaultManager}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// defaultManager builds the same manager the remote-read path uses.
func defaultManager(atmosConfig *schema.AtmosConfiguration) (IdentityManager, error) {
	if atmosConfig == nil {
		return nil, fmt.Errorf("%w: atmos configuration is nil", errUtils.ErrInvalidAuthConfig)
	}
	return auth.NewDefaultManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
}

// Compose returns base with the environment of the effective identity overlaid.
// The flagIdentity argument is the value of the global `--identity` flag (or ATMOS_IDENTITY) and
// wins over repoIdentity, the identity configured for the repository.
// With no effective identity, base is returned unchanged.
func (c *Composer) Compose(ctx context.Context, atmosConfig *schema.AtmosConfiguration, base []string, flagIdentity, repoIdentity string) ([]string, error) {
	defer perf.Track(atmosConfig, "gitenv.Composer.Compose")()

	identity := effectiveIdentity(flagIdentity, repoIdentity)
	if identity == "" {
		return base, nil
	}

	provider := &lazyProvider{atmosConfig: atmosConfig, newManager: c.newManager}
	return atmosgit.ComposeEnvironment(ctx, base, identity, provider)
}

// effectiveIdentity applies the precedence: flag, then repository. `--identity=false` disables
// authentication entirely, including the repository's identity.
func effectiveIdentity(flagIdentity, repoIdentity string) string {
	flagIdentity = cfg.NormalizeIdentityValue(flagIdentity)
	switch flagIdentity {
	case cfg.IdentityFlagDisabledValue:
		return ""
	case "":
		return repoIdentity
	default:
		return flagIdentity
	}
}

// lazyProvider builds the auth manager on first use, resolves an interactive selection
// (`--identity` without a value), then asks the manager for the identity environment. The manager
// authenticates when the identity has no valid session.
type lazyProvider struct {
	atmosConfig *schema.AtmosConfiguration
	newManager  func(*schema.AtmosConfiguration) (IdentityManager, error)
}

// EnsureIdentityEnvironment satisfies git.IdentityEnvironmentProvider.
func (p *lazyProvider) EnsureIdentityEnvironment(ctx context.Context, identity string) (map[string]string, error) {
	manager, err := p.newManager(p.atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: creating the auth manager for identity %q: %w", errUtils.ErrAuthManager, identity, err)
	}

	if identity == cfg.IdentityFlagSelectValue {
		identity, err = manager.GetDefaultIdentity(true)
		if err != nil {
			return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, err)
		}
	}

	return manager.EnsureIdentityEnvironment(ctx, identity)
}
