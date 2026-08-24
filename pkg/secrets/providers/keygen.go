package providers

// KeyGenerator is the optional, backend-agnostic capability a provider implements to generate its
// own key material. It follows the registry pattern: any registered provider that implements it is
// dispatched by `atmos secret keygen` with no backend-specific code in the command. The SOPS (age)
// provider implements it to produce an age key pair; a future provider (e.g. an x509/SSL kind)
// could implement it to produce a certificate + key, reusing the same command and output rendering.
type KeyGenerator interface {
	// HasKey reports whether the vault already has resolvable key material (so callers can decide
	// whether to generate).
	HasKey() bool
	// GenerateKey produces new key material and records it in the vault's configured sinks. basePath
	// roots any relative output. Implementations append/merge and never clobber other vaults'
	// material. The KeygenResult describes what was produced for user-facing output.
	GenerateKey(basePath string) (*KeygenResult, error)
}

// KeygenResult describes what a provider's GenerateKey produced, in a backend-agnostic shape so the
// command can render any provider's output uniformly.
type KeygenResult struct {
	// Vault is the named secrets vault (`secrets.providers.<name>`) the material belongs to.
	Vault string
	// Kind is the provider kind that generated it (e.g. "sops/age", "ssl/x509").
	Kind string
	// Summary is a one-line human description (e.g. "Generated an age key pair.").
	Summary string
	// Outputs are the artifacts written, with where each landed.
	Outputs []KeygenOutput
	// Public is optional public material safe to print/share (an age recipient, a certificate, a
	// public key). Empty when there is none.
	Public string
}

// KeygenOutput is one artifact a provider wrote during key generation.
type KeygenOutput struct {
	// Label names the artifact (e.g. "private identity", "public recipient", "certificate").
	Label string
	// Location is where it was written (a file path or a store reference).
	Location string
	// Sensitive marks private material that must be kept out of version control.
	Sensitive bool
}
