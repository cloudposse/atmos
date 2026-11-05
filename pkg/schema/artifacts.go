package schema

// Developer note: these types describe configuration only. Backend-specific `options` stay a
// generic map so this package never depends on a backend; typed decoding and validation live in
// pkg/artifact and pkg/artifact/config. This package must not import pkg/artifact (pkg/perf
// imports pkg/schema and pkg/artifact imports pkg/perf).

// ArtifactConfig is the `artifacts` section of atmos.yaml. It holds the single list of artifact
// repositories (destinations that are also discovery sources) and the optional global signature
// verification policy.
type ArtifactConfig struct {
	// Repositories maps a repository alias to its definition. The alias is case-sensitive and is
	// what `--repository` and the artifact step `repository` field select.
	Repositories map[string]ArtifactRepositoryConfig `yaml:"repositories,omitempty" json:"repositories,omitempty" mapstructure:"repositories"`

	// Verification is the global signature verification policy. Repository policies can add
	// requirements but never weaken it.
	Verification *ArtifactVerificationConfig `yaml:"verification,omitempty" json:"verification,omitempty" mapstructure:"verification"`
}

// ArtifactRepositoryConfig defines one artifact repository: a collection root that holds many named
// artifacts of possibly different kinds.
type ArtifactRepositoryConfig struct {
	// Kind selects the backend: local/dir, local/archive, aws/s3, oci/registry, or oci/layout.
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty" mapstructure:"kind"`

	// Identity names an Atmos auth identity used for this repository. Credentials never belong in
	// options. The identity is resolved lazily, only when an operation needs credentials.
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`

	// Options are backend-specific settings, for example `path` for local kinds, `bucket`, `prefix`,
	// `region`, `tags`, and `key_pattern` for aws/s3, or `registry` and `prefix` for oci/registry.
	// The `key_pattern` template is never expanded while the configuration is loaded.
	Options map[string]any `yaml:"options,omitempty" json:"options,omitempty" mapstructure:"options"`

	// Packaging selects how file payloads are stored: none, tar, tgz, or zip. The supported values
	// and the default depend on the repository kind.
	Packaging string `yaml:"packaging,omitempty" json:"packaging,omitempty" mapstructure:"packaging" jsonschema:"enum=,enum=none,enum=tar,enum=tgz,enum=zip"`

	// Permissions is the quoted octal mode of the resulting local archive file, for example "0600".
	// It is accepted only for local/archive repositories and must be a quoted string.
	Permissions string `yaml:"permissions,omitempty" json:"permissions,omitempty" mapstructure:"permissions" jsonschema:"pattern=^[0-7][0-7][0-7][0-7]?$"`

	// Catalog is the optional OCI catalog URI of this repository, for example
	// oci://ghcr.io/acme/atmos/catalog:stable. It is valid only for oci/registry repositories and
	// must name the repository's own registry host.
	Catalog string `yaml:"catalog,omitempty" json:"catalog,omitempty" mapstructure:"catalog"`

	// Defaults supplies defaults for newly published artifacts.
	Defaults *ArtifactRepositoryDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty" mapstructure:"defaults"`

	// Verification is this repository's signature verification policy. It is combined with the
	// global policy: both must be satisfied.
	Verification *ArtifactVerificationConfig `yaml:"verification,omitempty" json:"verification,omitempty" mapstructure:"verification"`

	// Locking configures mutation coordination for this repository.
	Locking *ArtifactLockingConfig `yaml:"locking,omitempty" json:"locking,omitempty" mapstructure:"locking"`

	// PathOrigin records the configuration file that declared options.path. It is populated by the
	// configuration loader, is never read from or written to YAML, and is tracked independently of
	// the other (possibly merged) repository fields.
	PathOrigin ArtifactConfigOrigin `yaml:"-" json:"-" mapstructure:"-"`
}

// ArtifactRepositoryDefaults holds defaults applied to artifacts newly published to a repository.
type ArtifactRepositoryDefaults struct {
	// Metadata supplies default artifact metadata. Explicit publication metadata overrides matching keys.
	Metadata *ArtifactMetadataDefaults `yaml:"metadata,omitempty" json:"metadata,omitempty" mapstructure:"metadata"`
}

// ArtifactMetadataDefaults is the default description and labels of newly published artifacts.
// Labels belong to each published artifact version, not to the repository itself.
type ArtifactMetadataDefaults struct {
	// Description is the default artifact description.
	Description string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`

	// Labels are default exact key/value labels for newly published artifact versions.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" mapstructure:"labels"`
}

// ArtifactVerificationConfig is a signature verification policy, allowed globally under
// `artifacts.verification` and per repository.
type ArtifactVerificationConfig struct {
	// RequireSignature makes a missing or invalid signature a failure. Unset means false for new
	// general artifacts. A repository cannot set it to false when the global policy requires it.
	RequireSignature *bool `yaml:"require_signature,omitempty" json:"require_signature,omitempty" mapstructure:"require_signature"`

	// TrustedIdentities are certificate identity and OIDC issuer pairs. Any one of them satisfies
	// this policy level's trust set.
	TrustedIdentities []ArtifactTrustedIdentity `yaml:"trusted_identities,omitempty" json:"trusted_identities,omitempty" mapstructure:"trusted_identities"`

	// TrustedKeys are operator-managed public-key file paths. Relative paths resolve against the
	// configuration file that declared them.
	TrustedKeys []string `yaml:"trusted_keys,omitempty" json:"trusted_keys,omitempty" mapstructure:"trusted_keys"`

	// KeysOrigin records the configuration file that declared trusted_keys. It is populated by the
	// configuration loader and is never read from or written to YAML.
	KeysOrigin ArtifactConfigOrigin `yaml:"-" json:"-" mapstructure:"-"`
}

// ArtifactTrustedIdentity is a trusted signer certificate identity and the OIDC issuer that vouches for it.
type ArtifactTrustedIdentity struct {
	// Identity is the expected certificate identity, for example a workflow URL or an email address.
	Identity string `yaml:"identity" json:"identity" mapstructure:"identity"`

	// Issuer is the expected OIDC issuer URL, for example https://token.actions.githubusercontent.com.
	Issuer string `yaml:"issuer" json:"issuer" mapstructure:"issuer"`
}

// ArtifactLockingConfig configures mutation coordination for a repository.
type ArtifactLockingConfig struct {
	// RequiredScope is the minimum lock guarantee mutations need: host (the default) or repository.
	RequiredScope string `yaml:"required_scope,omitempty" json:"required_scope,omitempty" mapstructure:"required_scope" jsonschema:"enum=,enum=host,enum=repository"`

	// Repository delegates locking to another configured repository that advertises repository scope.
	// The delegate keeps its own identity and storage root. Cycles are rejected.
	Repository string `yaml:"repository,omitempty" json:"repository,omitempty" mapstructure:"repository"`
}

// ArtifactConfigOrigin records which configuration file declared a path-valued setting, so that
// relative paths resolve against the declaring file even after imports have been merged.
type ArtifactConfigOrigin struct {
	// File is the absolute path of the declaring configuration file. Empty when unknown.
	File string `yaml:"-" json:"-" mapstructure:"-"`

	// Dir is the absolute directory of the declaring file. Empty when unknown.
	Dir string `yaml:"-" json:"-" mapstructure:"-"`

	// Remote reports that the value was declared by a remote (downloaded) import, whose local
	// path is ephemeral and must never anchor a relative destination.
	Remote bool `yaml:"-" json:"-" mapstructure:"-"`
}

// IsZero reports whether no declaring file is known.
func (o ArtifactConfigOrigin) IsZero() bool {
	return o.File == "" && o.Dir == "" && !o.Remote
}
