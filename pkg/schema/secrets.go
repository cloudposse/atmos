package schema

// SecretsConfig is the top-level `secrets:` block in atmos.yaml. It holds non-store
// secret backends (track 2), such as SOPS encrypted files. Store-backed secret backends
// (track 1) are configured as regular `stores:` entries with `secret: true` instead.
type SecretsConfig struct {
	// Providers maps a provider name to its configuration. These are non-store backends
	// (e.g. SOPS) that do not fit the key-value store interface.
	Providers map[string]SecretProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty" mapstructure:"providers"`
}

// SecretProviderConfig configures a single non-store secret provider (track 2).
type SecretProviderConfig struct {
	// Kind selects the provider implementation (e.g. "sops/age", "sops/aws-kms",
	// "sops/gcp-kms", "sops/gpg").
	Kind string `yaml:"kind" json:"kind" mapstructure:"kind"`
	// Identity optionally names an auth identity (resolved via pkg/auth) used by KMS-backed
	// SOPS kinds (sops/aws-kms, sops/gcp-kms). Local-key kinds (sops/age, sops/gpg) ignore it.
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
	// Spec carries provider-specific options (e.g. file, age_recipients, kms arn).
	Spec map[string]any `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"`
}

// SecretDeclaration is a single declared secret under a component's `secrets.vars` map.
// Declarations are GitOps-friendly: a secret must be declared before it can be set, read
// via `!secret`, or pulled/pushed. Exactly one backend reference (Store or Sops) is set.
type SecretDeclaration struct {
	// Description is a human-readable description of the secret.
	Description string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	// Store names a `secret: true` store (track 1) this secret resolves from.
	Store string `yaml:"store,omitempty" json:"store,omitempty" mapstructure:"store"`
	// Sops names a `secrets.providers` SOPS provider (track 2) this secret resolves from.
	Sops string `yaml:"sops,omitempty" json:"sops,omitempty" mapstructure:"sops"`
	// Reference is an optional backend-specific address for the secret. Reference-based backends
	// (e.g. a 1Password store) resolve it directly instead of composing a key from the secret
	// name; it may contain Go-template vars ({{ .atmos_stack }}, {{ .atmos_component }}). Example:
	// "op://Production/{{ .atmos_component }}/password". Name-keyed backends ignore it.
	Reference string `yaml:"reference,omitempty" json:"reference,omitempty" mapstructure:"reference"`
	// Required marks the secret as required; validation fails if it is not initialized.
	Required bool `yaml:"required,omitempty" json:"required,omitempty" mapstructure:"required"`
}
