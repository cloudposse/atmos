package schema

// AuthConfig defines the authentication configuration structure.
type AuthConfig struct {
	Logs       Logs                `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`
	Providers  map[string]Provider `yaml:"providers" json:"providers" mapstructure:"providers"`
	Identities map[string]Identity `yaml:"identities" json:"identities" mapstructure:"identities"`
}

// Provider defines an authentication provider configuration.
type Provider struct {
	Kind                  string                 `yaml:"kind" json:"kind" mapstructure:"kind"`
	StartURL              string                 `yaml:"start_url,omitempty" json:"start_url,omitempty" mapstructure:"start_url"`
	URL                   string                 `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`
	Region                string                 `yaml:"region,omitempty" json:"region,omitempty" mapstructure:"region"`
	Username              string                 `yaml:"username,omitempty" json:"username,omitempty" mapstructure:"username"`
	Password              string                 `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`
	SAMLDriver            string                 `yaml:"saml_driver,omitempty" json:"saml_driver,omitempty" mapstructure:"saml_driver"`
	ProviderType          string                 `yaml:"provider_type,omitempty" json:"provider_type,omitempty" mapstructure:"provider_type"` // Deprecated: use saml_driver.
	DownloadBrowserDriver bool                   `yaml:"download_browser_driver,omitempty" json:"download_browser_driver,omitempty" mapstructure:"download_browser_driver"`
	Session               *SessionConfig         `yaml:"session,omitempty" json:"session,omitempty" mapstructure:"session"`
	Default               bool                   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
	Spec                  map[string]interface{} `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"`
}

// SessionConfig defines session configuration for providers.
type SessionConfig struct {
	Duration string `yaml:"duration,omitempty" json:"duration,omitempty" mapstructure:"duration"`
}

// Identity defines an authentication identity configuration.
type Identity struct {
	Kind        string                 `yaml:"kind" json:"kind" mapstructure:"kind"`
	Default     bool                   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
	Via         *IdentityVia           `yaml:"via,omitempty" json:"via,omitempty" mapstructure:"via"`
	Principal   map[string]interface{} `yaml:"principal,omitempty" json:"principal,omitempty" mapstructure:"principal"`
	Credentials map[string]interface{} `yaml:"credentials,omitempty" json:"credentials,omitempty" mapstructure:"credentials"`
	Alias       string                 `yaml:"alias,omitempty" json:"alias,omitempty" mapstructure:"alias"`
	Env         []EnvironmentVariable  `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
}

// IdentityVia defines how an identity connects to a provider or other identity.
type IdentityVia struct {
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

// EnvironmentVariable defines an environment variable with preserved case sensitivity.
type EnvironmentVariable struct {
	Key   string `yaml:"key" json:"key" mapstructure:"key"`
	Value string `yaml:"value" json:"value" mapstructure:"value"`
}

// ComponentAuthConfig defines auth configuration at the component level.
type ComponentAuthConfig struct {
	Providers  map[string]Provider `yaml:"providers,omitempty" json:"providers,omitempty" mapstructure:"providers"`
	Identities map[string]Identity `yaml:"identities,omitempty" json:"identities,omitempty" mapstructure:"identities"`
}
