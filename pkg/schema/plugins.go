package schema

// PluginsConfig declares packages and policy. Commands restore required packages before activating their exports.
type PluginsConfig struct {
	MaxConcurrency int                       `yaml:"max_concurrency,omitempty" json:"max_concurrency,omitempty" mapstructure:"max_concurrency"`
	CacheDir       string                    `yaml:"cache_dir,omitempty" json:"cache_dir,omitempty" mapstructure:"cache_dir"`
	Packages       map[string]PluginPackage  `yaml:"packages,omitempty" json:"packages,omitempty" mapstructure:"packages"`
	Registries     map[string]PluginRegistry `yaml:"registries,omitempty" json:"registries,omitempty" mapstructure:"registries"`
	Discovery      PluginDiscoveryConfig     `yaml:"discovery,omitempty" json:"discovery,omitempty" mapstructure:"discovery"`
	Policy         PluginPolicy              `yaml:"policy,omitempty" json:"policy,omitempty" mapstructure:"policy"`
}

// PluginPackage records a consumer's explicit plugin selection.
type PluginPackage struct {
	Source  string `yaml:"source" json:"source" mapstructure:"source"`
	Version string `yaml:"version" json:"version" mapstructure:"version"`
	// Path runs a plugin built in a local directory instead of the release named by source and version.
	// A relative path is relative to the project directory (the directory of the root atmos.yaml); an
	// absolute path is used as is. The directory holds plugin.yaml and the executable it names.
	// When path is set it wins: source and version stay in the merged configuration but are not used,
	// no lock entry is read or written, and no signature or version check runs. Declare it in a profile
	// so the local build applies only while that profile is active.
	Path    string         `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`
	Scope   string         `yaml:"scope,omitempty" json:"scope,omitempty" mapstructure:"scope"`
	Enabled *bool          `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Config  map[string]any `yaml:"config,omitempty" json:"config,omitempty" mapstructure:"config"`
	Command string         `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	// Timeout bounds every call into the plugin as a Go duration such as 5m; empty means no deadline.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`
}

// PluginDiscoveryConfig controls the local metadata discovery cache.
type PluginDiscoveryConfig struct {
	CacheTTL string `yaml:"cache_ttl,omitempty" json:"cache_ttl,omitempty" mapstructure:"cache_ttl"`
}

// PluginRegistry configures authentication and optional catalog or repository discovery.
type PluginRegistry struct {
	// Identity selects the Atmos auth identity used exclusively for this registry host.
	Identity     string   `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
	Host         string   `yaml:"host,omitempty" json:"host,omitempty" mapstructure:"host"`
	Catalog      string   `yaml:"catalog,omitempty" json:"catalog,omitempty" mapstructure:"catalog"`
	Repositories []string `yaml:"repositories,omitempty" json:"repositories,omitempty" mapstructure:"repositories"`
	Auth         string   `yaml:"auth,omitempty" json:"auth,omitempty" mapstructure:"auth"`
}

// PluginPolicy restricts loading and both automatic and explicit installation.
type PluginPolicy struct {
	// PublisherConstraints retains mandatory signer sets from all loaded config layers.
	PublisherConstraints [][]PluginPublisher `yaml:"-" json:"-" mapstructure:"-"`
	RequireSignature     bool                `yaml:"require_signature,omitempty" json:"require_signature,omitempty" mapstructure:"require_signature"`
	Offline              bool                `yaml:"offline,omitempty" json:"offline,omitempty" mapstructure:"offline"`
	CacheTTL             string              `yaml:"cache_ttl,omitempty" json:"cache_ttl,omitempty" mapstructure:"cache_ttl"`
	TrustedPublishers    []PluginPublisher   `yaml:"trusted_publishers,omitempty" json:"trusted_publishers,omitempty" mapstructure:"trusted_publishers"`
}

// PluginPublisher binds a package namespace to a signing identity and issuer.
type PluginPublisher struct {
	Publisher string `yaml:"publisher" json:"publisher" mapstructure:"publisher"`
	Identity  string `yaml:"identity" json:"identity" mapstructure:"identity"`
	Issuer    string `yaml:"issuer" json:"issuer" mapstructure:"issuer"`
}
