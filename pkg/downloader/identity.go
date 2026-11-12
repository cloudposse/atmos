package downloader

import (
	"context"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The sentinel values of the global --identity flag (mirrors pkg/config, which this package cannot
// import without a cycle): "--identity=false" disables identities and a bare "--identity" asks for
// an interactive selection, which a remote read never does.
const (
	identityFlagDisabled = "__DISABLED__"
	identityFlagSelect   = "__SELECT__"
)

// IdentityEnvironmentResolver turns an Atmos Auth identity name into the environment ("KEY=value"
// entries) its Git subprocess needs. Implementations must not modify the process environment and
// must never prompt: a remote read happens in the middle of a command.
type IdentityEnvironmentResolver func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, identity string) ([]string, error)

type identityOverrideKey struct{}

var (
	identityResolverMu sync.RWMutex
	identityResolver   IdentityEnvironmentResolver
)

// RegisterIdentityEnvironmentResolver installs the resolver WithIdentity uses. The auth layer
// registers it from init(), the same way credential brokers register, so this package stays free
// of a dependency on pkg/auth.
func RegisterIdentityEnvironmentResolver(resolver IdentityEnvironmentResolver) {
	defer perf.Track(nil, "downloader.RegisterIdentityEnvironmentResolver")()

	identityResolverMu.Lock()
	defer identityResolverMu.Unlock()
	identityResolver = resolver
}

// WithIdentityOverride records the global --identity flag value on ctx. A non-empty value takes
// precedence over every identity: field and section default (see ResolveIdentityName).
func WithIdentityOverride(ctx context.Context, identity string) context.Context {
	defer perf.Track(nil, "downloader.WithIdentityOverride")()

	if identity == "" {
		return ctx
	}
	return context.WithValue(ctx, identityOverrideKey{}, identity)
}

// ResolveIdentityName applies the remote-read precedence, matching `atmos git`: the global
// --identity flag (carried on ctx by WithIdentityOverride), then the entry's own identity field,
// then the section default. It returns "" when none applies (or when the flag disables identities),
// which keeps today's behavior: the ambient credential broker, then the process environment.
func ResolveIdentityName(ctx context.Context, entry, sectionDefault string) string {
	defer perf.Track(nil, "downloader.ResolveIdentityName")()

	override, _ := ctx.Value(identityOverrideKey{}).(string)
	switch override {
	case identityFlagDisabled:
		return ""
	case "", identityFlagSelect:
		// No usable flag value: fall through to the configuration.
	default:
		return override
	}
	if entry = strings.TrimSpace(entry); entry != "" {
		return entry
	}
	return strings.TrimSpace(sectionDefault)
}

// WithIdentity resolves which identity (if any) applies to one remote read, turns it into an
// environment through the registered resolver, and returns a context whose fetches run with that
// environment (see WithEnvironment). When no identity applies it returns ctx unchanged.
func WithIdentity(ctx context.Context, atmosConfig *schema.AtmosConfiguration, entry, sectionDefault string) (context.Context, error) {
	defer perf.Track(atmosConfig, "downloader.WithIdentity")()

	name := ResolveIdentityName(ctx, entry, sectionDefault)
	if name == "" {
		return ctx, nil
	}

	identityResolverMu.RLock()
	resolver := identityResolver
	identityResolverMu.RUnlock()
	if resolver == nil {
		return ctx, errUtils.Build(errUtils.ErrRemoteIdentityResolverUnavailable).
			WithExplanation(fmt.Sprintf("Identity `%s` is named for a remote read, but no identity resolver is registered.", name)).
			WithHint("This is a build problem: the auth layer registers the resolver at startup.").
			Err()
	}

	env, err := resolver(ctx, atmosConfig, name)
	if err != nil {
		return ctx, err
	}
	return WithEnvironment(ctx, env), nil
}
