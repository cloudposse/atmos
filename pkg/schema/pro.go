package schema

// ProSettings contains Atmos Pro integration configuration.
type ProSettings struct {
	BaseURL         string             `yaml:"base_url,omitempty" json:"base_url,omitempty" mapstructure:"base_url"`
	Endpoint        string             `yaml:"endpoint,omitempty" json:"endpoint,omitempty" mapstructure:"endpoint"`
	Token           string             `yaml:"token,omitempty" json:"token,omitempty" mapstructure:"token"`
	WorkspaceID     string             `yaml:"workspace_id,omitempty" json:"workspace_id,omitempty" mapstructure:"workspace_id"`
	GithubOIDC      GithubOIDCSettings `yaml:"github_oidc,omitempty" json:"github_oidc,omitempty" mapstructure:"github_oidc"`
	MaxPayloadBytes int                `yaml:"max_payload_bytes,omitempty" json:"max_payload_bytes,omitempty" mapstructure:"max_payload_bytes"`
	GitHubHeadRef   string             `yaml:"-" json:"-" mapstructure:"github_head_ref"`
	// GitSTS holds global defaults for the github/sts auth integration.
	GitSTS GitSTSSettings `yaml:"git_sts,omitempty" json:"git_sts,omitempty" mapstructure:"git_sts"`
	// Exec holds settings for the command-execution metadata upload feature.
	Exec ExecSettings `yaml:"exec,omitempty" json:"exec,omitempty" mapstructure:"exec"`
}

// ExecSettings contains configuration for command-execution metadata upload
// (POST /v1/atmos/exec). This only lengthens the default synchronous-upload
// wait timeout; it is not an opt-out/opt-in switch (see FR-008a).
type ExecSettings struct {
	// SyncTimeoutSeconds bounds how long a synchronous command (terraform
	// plan/apply, describe affected) waits for its execution-record upload to
	// be confirmed. Defaults to 10 seconds when unset/zero; values below 10
	// are clamped up to 10 rather than allowed to shorten the default.
	SyncTimeoutSeconds int `yaml:"sync_timeout_seconds,omitempty" json:"sync_timeout_seconds,omitempty" mapstructure:"sync_timeout_seconds"`
}

// GitSTSSettings contains global defaults for the github/sts auth integration.
// Per-integration spec fields override these.
type GitSTSSettings struct {
	// GitConfigMode controls how minted tokens reach the child process git config:
	// "env" (inline GIT_CONFIG_KEY_n/VALUE_n) or "file" (write a 0600 gitconfig and emit include.path).
	// Defaults to "env" when unset.
	GitConfigMode string `yaml:"git_config_mode,omitempty" json:"git_config_mode,omitempty" mapstructure:"git_config_mode"`
	// RevokeOnExit controls command-end auto-revocation of minted tokens (in CI). Defaults to true when unset.
	RevokeOnExit *bool `yaml:"revoke_on_exit,omitempty" json:"revoke_on_exit,omitempty" mapstructure:"revoke_on_exit"`
}

// GithubOIDCSettings contains GitHub OIDC token configuration.
type GithubOIDCSettings struct {
	RequestURL   string `yaml:"request_url,omitempty" json:"request_url,omitempty" mapstructure:"request_url"`
	RequestToken string `yaml:"request_token,omitempty" json:"request_token,omitempty" mapstructure:"request_token"`
}

// StackLockActionParams holds the parameters for stack lock/unlock operations.
type StackLockActionParams struct {
	Method  string `yaml:"method,omitempty" json:"method,omitempty" mapstructure:"method"`
	URL     string `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`
	Body    any    `yaml:"body,omitempty" json:"body,omitempty" mapstructure:"body"`
	Out     any    `yaml:"out,omitempty" json:"out,omitempty" mapstructure:"out"`
	Op      string `yaml:"op,omitempty" json:"op,omitempty" mapstructure:"op"`
	WrapErr error  `yaml:"wrap_err,omitempty" json:"wrap_err,omitempty" mapstructure:"wrap_err"`
}
