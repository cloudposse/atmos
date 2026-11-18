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

type identityNameKey struct{}

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
// environment (see WithEnvironment) and are attributed to that identity (see IdentityFromContext).
// When no identity applies it returns ctx unchanged.
//
// Prefer WithIdentityForSource when the source is known: an identity only applies to the
// transports that can use it.
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
	return WithEnvironment(context.WithValue(ctx, identityNameKey{}, name), env), nil
}

// WithIdentityForSource is WithIdentity for one known source. The identity is resolved only when
// the source's transport can use it (see SourceUsesIdentity). For any other source it returns ctx
// unchanged, so a registry, bucket, or local read is never refused because an identity it would
// not have used has no session.
func WithIdentityForSource(ctx context.Context, atmosConfig *schema.AtmosConfiguration, src, entry, sectionDefault string) (context.Context, error) {
	defer perf.Track(atmosConfig, "downloader.WithIdentityForSource")()

	if !SourceUsesIdentity(src) {
		return ctx, nil
	}
	return WithIdentity(ctx, atmosConfig, entry, sectionDefault)
}

// IdentityFromContext returns the name of the Atmos Auth identity the fetches made with ctx run as
// (set by WithIdentity), or "" when they name none and ambient credentials apply.
func IdentityFromContext(ctx context.Context) string {
	defer perf.Track(nil, "downloader.IdentityFromContext")()

	if ctx == nil {
		return ""
	}
	name, _ := ctx.Value(identityNameKey{}).(string)
	return name
}

// FetchUsesIdentity reports whether the fetches made with ctx run with a named identity or a
// per-fetch environment of their own, as opposed to ambient credentials.
func FetchUsesIdentity(ctx context.Context) bool {
	defer perf.Track(nil, "downloader.FetchUsesIdentity")()

	return IdentityFromContext(ctx) != "" || len(EnvironmentFromContext(ctx)) > 0
}

// identitySkippedPrefixes are the source forms whose getters authenticate through their own
// credential chain (a registry keychain, a cloud SDK) or need no credentials at all. An identity
// environment never reaches them.
var identitySkippedPrefixes = []string{
	"oci://", "oci::",
	"s3://", "s3::",
	"gcs::", "gs://",
	"file://", "file::",
}

// identitySkippedHosts mark the scheme-less bucket URLs go-getter's own detectors turn into S3 and
// GCS sources.
var identitySkippedHosts = []string{".amazonaws.com/", "googleapis.com/"}

// SourceUsesIdentity reports whether an identity named for src can take part in fetching it. An
// identity is delivered as an environment for the fetch, which Git subprocesses run with. Sources
// fetched from an OCI registry, an S3 or GCS bucket, or the local filesystem authenticate on their
// own and report false. Everything else (Git in any form, and HTTP) reports true, so an identity
// named for it must resolve.
func SourceUsesIdentity(src string) bool {
	defer perf.Track(nil, "downloader.SourceUsesIdentity")()

	trimmed := strings.TrimSpace(src)
	lowered := strings.ToLower(trimmed)
	for _, prefix := range identitySkippedPrefixes {
		if strings.HasPrefix(lowered, prefix) {
			return false
		}
	}
	if strings.Contains(lowered, schemeSeparator) || strings.HasPrefix(lowered, strings.ToLower(GitPrefix)) {
		return true
	}

	// No scheme and not forced to Git: a bucket URL, a Git shorthand, or a local path.
	for _, host := range identitySkippedHosts {
		if strings.Contains(lowered, host) {
			return false
		}
	}
	// isRemoteSource recognizes the two scheme-less remote forms: an SCP-style Git URL
	// (git@host:org/repo) and the shorthand for a supported host (github.com/org/repo). go-getter
	// treats any other scheme-less source as a filesystem path.
	return isRemoteSource(trimmed)
}
