// Package cachepaths is the single source of truth for the root-relative
// subdirectories under the Atmos XDG cache root (~/.cache/atmos) that hold
// auth session material (tokens, refresh tokens, client secrets) or
// provisioned identity/role metadata, and therefore must be excluded from
// the CI cache.
//
// pkg/ci/cache caches the whole XDG cache root by default (so vendoring
// caches, remote stack-import clones, and plugin caches inherit caching for
// free), which means these directories need an explicit, unconditional
// carve-out. Each owning package (pkg/auth/providers/aws,
// pkg/auth/providers/azure, pkg/auth/identities/aws,
// pkg/auth/provisioning) defines its subdirectory constant as an alias of
// the corresponding constant here, rather than declaring its own literal, so
// there is exactly one place that spells out the subdirectory name — the CI
// cache exclusion list can never drift from the literal a provider actually
// uses on disk.
//
// This package is deliberately dependency-free: pkg/ci/cache runs on every
// Atmos invocation's startup path (auto-restore/save hooks), and the owning
// packages above pull in heavyweight, provider-specific transitive
// dependencies (AWS SDK, MSAL, Azure SDK) that a generic cache-archiving
// package has no other reason to depend on. Both pkg/ci/cache and the owning
// packages depend on this leaf package instead of on each other.
//
// Deliberately NOT included, because they are not under the XDG *cache*
// root pkg/ci/cache archives:
//   - GCP ADC/config (pkg/auth/cloud/gcp/files.go) resolves via
//     xdg.GetXDGConfigDir ("~/.config/atmos"), a different XDG base entirely.
//   - saml2aws browser storage (pkg/auth/providers/aws/saml.go) lives at
//     "~/.aws/saml2aws/", outside any Atmos XDG directory.
//   - "auth/github-sts" (pkg/auth/integrations/github/sts.go) resolves via
//     xdg.GetXDGDataDir (XDG *data* dir), not GetXDGCacheDir. This is
//     incidental today, not a designed safety boundary — if a future
//     refactor moves it under XDG cache, it will need to be added here.
package cachepaths

const (
	// AWSSSOSubdir is the AWS SSO provider's OAuth token cache directory
	// (AccessToken/RefreshToken/ClientSecret). See
	// pkg/auth/providers/aws/sso_cache.go.
	AWSSSOSubdir = "aws-sso"

	// AzureDeviceCodeSubdir is the Azure device-code provider's token cache
	// directory (AccessToken/GraphAPIToken). See
	// pkg/auth/providers/azure/device_code_cache.go.
	AzureDeviceCodeSubdir = "azure-device-code"

	// AWSWebflowSubdir is the AWS browser/SAML webflow identity's refresh
	// token cache directory. See pkg/auth/identities/aws/webflow.go.
	AWSWebflowSubdir = "aws-webflow"

	// ProvisioningSubdir holds provisioned-identities.yaml metadata (not raw
	// secrets, excluded defensively). See pkg/auth/provisioning/writer.go.
	ProvisioningSubdir = "auth"
)

// All returns the root-relative subdirectories under the Atmos XDG cache
// root that must be excluded from CI cache archiving by default.
func All() []string {
	return []string{AWSSSOSubdir, AzureDeviceCodeSubdir, AWSWebflowSubdir, ProvisioningSubdir}
}
