package schema

// AuthConfig defines the authentication configuration structure.
type AuthConfig struct {
	Logs       Logs                `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`
	Keyring    KeyringConfig       `yaml:"keyring,omitempty" json:"keyring,omitempty" mapstructure:"keyring"`
	Providers  map[string]Provider `yaml:"providers" json:"providers" mapstructure:"providers"`
	Identities map[string]Identity `yaml:"identities" json:"identities" mapstructure:"identities"`
	// IdentityCaseMap maps lowercase identity names to their original case.
	// This is populated during config loading to work around Viper's case-insensitive behavior.
	IdentityCaseMap map[string]string `yaml:"-" json:"-" mapstructure:"-"`
}

// KeyringConfig defines keyring backend configuration for credential storage.
type KeyringConfig struct {
	Type string                 `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"` // "system", "file", or "memory"
	Spec map[string]interface{} `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"` // Type-specific configuration
}

// Provider defines an authentication provider configuration.
type Provider struct {
	Kind                    string                 `yaml:"kind" json:"kind" mapstructure:"kind"`
	StartURL                string                 `yaml:"start_url,omitempty" json:"start_url,omitempty" mapstructure:"start_url"`
	URL                     string                 `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`
	Region                  string                 `yaml:"region,omitempty" json:"region,omitempty" mapstructure:"region"`
	Username                string                 `yaml:"username,omitempty" json:"username,omitempty" mapstructure:"username"`
	Password                string                 `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`
	Driver                  string                 `yaml:"driver,omitempty" json:"driver,omitempty" mapstructure:"driver"`
	ProviderType            string                 `yaml:"provider_type,omitempty" json:"provider_type,omitempty" mapstructure:"provider_type"` // Deprecated: use driver.
	DownloadBrowserDriver   bool                   `yaml:"download_browser_driver,omitempty" json:"download_browser_driver,omitempty" mapstructure:"download_browser_driver"`
	AutoProvisionIdentities *bool                  `yaml:"auto_provision_identities,omitempty" json:"auto_provision_identities,omitempty" mapstructure:"auto_provision_identities"`
	Session                 *SessionConfig         `yaml:"session,omitempty" json:"session,omitempty" mapstructure:"session"`
	Console                 *ConsoleConfig         `yaml:"console,omitempty" json:"console,omitempty" mapstructure:"console"`
	Default                 bool                   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
	Spec                    map[string]interface{} `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"`
}

// SessionConfig defines session configuration for providers.
type SessionConfig struct {
	Duration string `yaml:"duration,omitempty" json:"duration,omitempty" mapstructure:"duration"`
}

// ConsoleConfig defines web console access configuration for providers.
type ConsoleConfig struct {
	SessionDuration string `yaml:"session_duration,omitempty" json:"session_duration,omitempty" mapstructure:"session_duration"` // Duration string (e.g., "12h"). Max: 12h for AWS.
}

// Identity defines an authentication identity configuration.
type Identity struct {
	Kind        string                 `yaml:"kind" json:"kind" mapstructure:"kind"`
	Default     bool                   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
	Provider    string                 `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"` // Provider name for direct provider association (for provisioned identities).
	Via         *IdentityVia           `yaml:"via,omitempty" json:"via,omitempty" mapstructure:"via"`
	Principal   map[string]interface{} `yaml:"principal,omitempty" json:"principal,omitempty" mapstructure:"principal"` // Principal information (role name, account, etc.). For AWS permission sets: {name: string, account: {name: string, id: string}}.
	Credentials map[string]interface{} `yaml:"credentials,omitempty" json:"credentials,omitempty" mapstructure:"credentials"`
	Alias       string                 `yaml:"alias,omitempty" json:"alias,omitempty" mapstructure:"alias"`
	Env         []EnvironmentVariable  `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
	Session     *SessionConfig         `yaml:"session,omitempty" json:"session,omitempty" mapstructure:"session"`
}

// IdentityVia defines how an identity connects to a provider or other identity.
type IdentityVia struct {
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

// Principal defines the principal information for an identity (for provisioned identities).
// This is a helper struct for creating provisioned identities programmatically.
type Principal struct {
	Name    string   `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Account *Account `yaml:"account,omitempty" json:"account,omitempty" mapstructure:"account"`
}

// Account defines account information for a principal.
type Account struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	ID   string `yaml:"id,omitempty" json:"id,omitempty" mapstructure:"id"`
}

// ToMap converts a Principal struct to a map[string]interface{} for use in Identity.Principal.
func (p *Principal) ToMap() map[string]interface{} {
	if p == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})
	if p.Name != "" {
		result["name"] = p.Name
	}
	if p.Account != nil {
		account := make(map[string]interface{})
		if p.Account.Name != "" {
			account["name"] = p.Account.Name
		}
		if p.Account.ID != "" {
			account["id"] = p.Account.ID
		}
		if len(account) > 0 {
			result["account"] = account
		}
	}
	return result
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
