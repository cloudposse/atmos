package schema

// ProSettings contains Atmos Pro integration configuration.
type ProSettings struct {
	BaseURL     string             `yaml:"base_url,omitempty" json:"base_url,omitempty" mapstructure:"base_url"`
	Endpoint    string             `yaml:"endpoint,omitempty" json:"endpoint,omitempty" mapstructure:"endpoint"`
	Token       string             `yaml:"token,omitempty" json:"token,omitempty" mapstructure:"token"`
	WorkspaceID string             `yaml:"workspace_id,omitempty" json:"workspace_id,omitempty" mapstructure:"workspace_id"`
	GithubOIDC  GithubOIDCSettings `yaml:"github_oidc,omitempty" json:"github_oidc,omitempty" mapstructure:"github_oidc"`
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
