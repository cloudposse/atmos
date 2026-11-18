package downloader

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/broker"
	"github.com/cloudposse/atmos/pkg/perf"
)

// envSeparator separates a variable name from its value in a "KEY=value" entry.
const envSeparator = "="

type fetchEnvironmentKey struct{}

// WithEnvironment scopes an environment overlay to the fetches made with the returned context.
// Every Git subprocess a fetch spawns (clone, fetch, checkout, submodules) runs with the process
// environment plus this overlay, with overlay entries replacing process entries of the same name.
// The process environment is never modified, so concurrent fetches can use different overlays
// (for example, one Atmos Auth identity per vendored source).
//
// Entries are "KEY=value" strings, the form returned by git.ComposeEnvironment. Calling it again on
// a derived context merges the new entries over the earlier ones. An empty overlay returns ctx.
func WithEnvironment(ctx context.Context, env []string) context.Context {
	defer perf.Track(nil, "downloader.WithEnvironment")()

	if len(env) == 0 {
		return ctx
	}
	merged := overlayEnvironment(EnvironmentFromContext(ctx), env)
	return context.WithValue(ctx, fetchEnvironmentKey{}, merged)
}

// EnvironmentFromContext returns a copy of the per-fetch environment overlay carried by ctx, or nil.
func EnvironmentFromContext(ctx context.Context) []string {
	defer perf.Track(nil, "downloader.EnvironmentFromContext")()

	if ctx == nil {
		return nil
	}
	env, _ := ctx.Value(fetchEnvironmentKey{}).([]string)
	if len(env) == 0 {
		return nil
	}
	return append([]string(nil), env...)
}

// fetchEnvironmentValue looks a variable up in the per-fetch overlay first and falls back to the
// process environment, mirroring how a Git subprocess of this fetch would see it.
func fetchEnvironmentValue(ctx context.Context, name string) string {
	overlay := EnvironmentFromContext(ctx)
	for i := len(overlay) - 1; i >= 0; i-- {
		key, value, found := strings.Cut(overlay[i], envSeparator)
		if found && envKeysEqual(key, name) {
			return value
		}
	}
	// A named identity never sees what an ambient broker exported: see applyFetchEnvironment.
	if IdentityFromContext(ctx) != "" {
		if prior, exported := broker.PreBrokerValue(name); exported {
			return prior
		}
	}
	//nolint:forbidigo // Live env read of broker-exported GIT_CONFIG_*; mirrors pkg/http/client.go.
	return os.Getenv(name)
}

// applyFetchEnvironment gives cmd the process environment overlaid with the per-fetch environment
// carried by ctx. It is a no-op for a fetch that names no identity and carries no overlay, so such
// a fetch keeps inheriting the process environment exactly as before.
//
// A fetch that names an identity runs with that identity's credentials only. An ambient credential
// broker exports its Git rewrites (GIT_CONFIG_*) and tokens into the process environment, where an
// earlier fetch in the same process may have put them. Left in place, Git would rewrite this
// fetch's URL to carry the broker's token instead of using the identity. So for a named identity
// every broker-exported variable is first restored to what it was before the broker ran, and the
// identity's own environment is applied on top.
func applyFetchEnvironment(ctx context.Context, cmd *exec.Cmd) {
	overlay := EnvironmentFromContext(ctx)
	namedIdentity := IdentityFromContext(ctx) != ""
	if len(overlay) == 0 && !namedIdentity {
		return
	}
	base := cmd.Env
	if base == nil {
		base = os.Environ()
	}
	if namedIdentity {
		base = broker.WithoutBrokeredEnvironment(base)
	}
	cmd.Env = overlayEnvironment(base, overlay)
}

// overlayEnvironment returns base with every overlay entry applied: an overlay entry replaces
// base entries of the same name, and later overlay entries win over earlier ones. Neither input
// is modified.
func overlayEnvironment(base, overlay []string) []string {
	// The index of the last overlay entry for each name.
	last := make(map[string]int, len(overlay))
	for i, entry := range overlay {
		if key, ok := envEntryKey(entry); ok {
			last[key] = i
		}
	}

	// Sized to one operand: a sum of two lengths is flagged as a possible allocation overflow.
	merged := make([]string, 0, len(base))
	for _, entry := range base {
		if key, ok := envEntryKey(entry); ok {
			if _, replaced := last[key]; replaced {
				continue
			}
		}
		merged = append(merged, entry)
	}
	for i, entry := range overlay {
		if key, ok := envEntryKey(entry); ok && last[key] == i {
			merged = append(merged, entry)
		}
	}
	return merged
}

// envEntryKey returns the normalized variable name of a "KEY=value" entry. Entries without an
// equals sign are not variables and report false.
func envEntryKey(entry string) (string, bool) {
	key, _, found := strings.Cut(entry, envSeparator)
	if !found {
		return "", false
	}
	return normalizeEnvKey(key), true
}

// normalizeEnvKey folds case on Windows, where environment names are case-insensitive.
func normalizeEnvKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func envKeysEqual(a, b string) bool {
	return normalizeEnvKey(a) == normalizeEnvKey(b)
}
