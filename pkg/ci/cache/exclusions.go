package cache

import "github.com/cloudposse/atmos/pkg/perf"

// defaultExcludedPaths are root-relative subpaths that are pruned from the CI
// cache unconditionally (regardless of ci.cache.paths), unless the user
// explicitly opts out via ci.cache.allow_unsafe_auth_cache. Each entry is a
// well-known Atmos auth cache directory that lives under the same XDG cache
// root (~/.cache/atmos) that ci.cache archives by default, and persists real
// session material (tokens, refresh tokens, client secrets) or provisioned
// identity/role metadata to disk. Caching the whole root by default is
// intentional (it lets vendoring caches, remote stack-import clones, and
// plugin caches inherit caching for free), but that means these directories
// need an explicit, unconditional carve-out rather than relying on every
// ci.cache.paths configuration to know to avoid them.
//
// This list intentionally duplicates literal strings rather than importing
// the owning packages' constants (pkg/auth/providers/aws, .../azure,
// pkg/auth/identities/aws, pkg/auth/provisioning). This package runs on
// every Atmos invocation's startup path (auto-restore/save hooks), and
// those owning packages pull in heavyweight, provider-specific transitive
// dependencies (AWS SDK, MSAL, etc.) that a generic cache-archiving package
// has no other reason to depend on. Each owning package instead carries a
// drift-guard test asserting its literal subdir constant is present in
// DefaultExcludedPaths(), so renaming/repathing a subdir there without
// updating this list fails a test immediately.
//
// Deliberately NOT included, because they are not under the XDG *cache*
// root ci.cache archives:
//   - GCP ADC/config (pkg/auth/cloud/gcp/files.go) resolves via
//     xdg.GetXDGConfigDir ("~/.config/atmos"), a different XDG base entirely.
//   - saml2aws browser storage (pkg/auth/providers/aws/saml.go) lives at
//     "~/.aws/saml2aws/", outside any Atmos XDG directory.
//   - "auth/github-sts" (pkg/auth/integrations/github/sts.go) resolves via
//     xdg.GetXDGDataDir (XDG *data* dir), not GetXDGCacheDir. This is
//     incidental today, not a designed safety boundary — if a future
//     refactor moves it under XDG cache, it will need to be added here.
var defaultExcludedPaths = []string{
	"aws-sso",           // pkg/auth/providers/aws/sso_cache.go: ssoTokenCacheSubdir (AccessToken/RefreshToken/ClientSecret).
	"azure-device-code", // pkg/auth/providers/azure/device_code_cache.go: deviceCodeTokenCacheSubdir (AccessToken/GraphAPIToken).
	"aws-webflow",       // pkg/auth/identities/aws/webflow.go: webflowCacheSubdir (refresh token).
	"auth",              // pkg/auth/provisioning/writer.go: authSubDir (provisioned-identities.yaml; metadata, not raw secrets, excluded defensively).
}

// DefaultExcludedPaths returns a copy of the root-relative subpaths excluded
// from the CI cache by default. Exported so cmd/ci/cache (the `atmos ci cache
// paths` command, which feeds the native actions/cache action instead of
// Atmos's own backend) can render the same exclusions.
func DefaultExcludedPaths() []string {
	defer perf.Track(nil, "cache.DefaultExcludedPaths")()

	out := make([]string, len(defaultExcludedPaths))
	copy(out, defaultExcludedPaths)
	return out
}
