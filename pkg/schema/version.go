package schema

// VersionCheck configures automatic version checking against GitHub releases.
type VersionCheck struct {
	Enabled   bool   `yaml:"enabled,omitempty" mapstructure:"enabled" json:"enabled,omitempty"`
	Timeout   int    `yaml:"timeout,omitempty" mapstructure:"timeout" json:"timeout,omitempty"`
	Frequency string `yaml:"frequency,omitempty" mapstructure:"frequency" json:"frequency,omitempty"`
}

// VersionConstraint configures version constraint validation for Atmos.
// When specified, Atmos validates that the current version satisfies the constraint
// before executing any command.
type VersionConstraint struct {
	// Require specifies the semver constraint(s) for Atmos version as a single string.
	// Multiple constraints are comma-separated and treated as logical AND.
	// Uses hashicorp/go-version library syntax (same as Terraform).
	//
	// Examples:
	//   ">=1.2.3"                    - Minimum version
	//   "<2.0.0"                     - Maximum version (exclude)
	//   ">=1.2.3, <2.0.0"            - Range (AND logic)
	//   ">=2.5.0, !=2.7.0, <3.0.0"   - Complex (multiple AND constraints)
	//   "~>1.2"                      - Pessimistic constraint (>=1.2.0, <1.3.0)
	//   "~>1.2.3"                    - Pessimistic constraint (>=1.2.3, <1.3.0)
	//   "1.2.3"                      - Exact version
	Require string `yaml:"require,omitempty" mapstructure:"require" json:"require,omitempty"`

	// Enforcement specifies the behavior when version constraint is not satisfied.
	// Values:
	//   "fatal" - Exit immediately with error code 1 (default)
	//   "warn"  - Log warning but continue execution
	//   "silent" - Skip validation entirely (for debugging)
	Enforcement string `yaml:"enforcement,omitempty" mapstructure:"enforcement" json:"enforcement,omitempty"`

	// Message provides a custom message to display when constraint fails.
	// If empty, a default message is shown.
	Message string `yaml:"message,omitempty" mapstructure:"message" json:"message,omitempty"`
}

// VersionProvider configures a concrete backend used to discover versions.
type VersionProvider struct {
	Kind       string `yaml:"kind,omitempty" mapstructure:"kind" json:"kind,omitempty"`
	Type       string `yaml:"type,omitempty" mapstructure:"type" json:"type,omitempty" jsonschema_extras:"deprecated=true,x-atmos-replacement=kind"` // Deprecated: use kind.
	URL        string `yaml:"url,omitempty" mapstructure:"url" json:"url,omitempty"`
	Region     string `yaml:"region,omitempty" mapstructure:"region" json:"region,omitempty"`
	RegistryID string `yaml:"registry_id,omitempty" mapstructure:"registry_id" json:"registry_id,omitempty"`

	// Insecure allows plain-HTTP access to the registry (local/emulated
	// registries only).
	Insecure bool `yaml:"insecure,omitempty" mapstructure:"insecure" json:"insecure,omitempty"`
}

// VersionUpdatePolicy configures update intent for managed versions.
type VersionUpdatePolicy struct {
	Strategy string   `yaml:"strategy,omitempty" mapstructure:"strategy" json:"strategy,omitempty"`
	Cooldown string   `yaml:"cooldown,omitempty" mapstructure:"cooldown" json:"cooldown,omitempty"`
	Schedule []string `yaml:"schedule,omitempty" mapstructure:"schedule" json:"schedule,omitempty"`

	// Pin selects the artifact form emitted for the entry: "digest" (alias
	// "sha") locks and renders the immutable identifier (git commit SHA or
	// OCI sha256 digest) alongside the human-readable version; "none" or
	// empty keeps plain version references. Strategy decides how far updates
	// may advance; Pin decides which form is written.
	Pin string `yaml:"pin,omitempty" mapstructure:"pin" json:"pin,omitempty"`
}

// VersionPolicy configures shared defaults for managed versions.
type VersionPolicy struct {
	Update     VersionUpdatePolicy `yaml:"update,omitempty" mapstructure:"update" json:"update,omitempty"`
	Include    []string            `yaml:"include,omitempty" mapstructure:"include" json:"include,omitempty"`
	Exclude    []string            `yaml:"exclude,omitempty" mapstructure:"exclude" json:"exclude,omitempty"`
	Prerelease *bool               `yaml:"prerelease,omitempty" mapstructure:"prerelease" json:"prerelease,omitempty"`
	Labels     []string            `yaml:"labels,omitempty" mapstructure:"labels" json:"labels,omitempty"`
}

// VersionGroup configures a Dependabot/Renovate-style batch of updates.
type VersionGroup struct {
	Ecosystems      []string            `yaml:"ecosystems,omitempty" mapstructure:"ecosystems" json:"ecosystems,omitempty"`
	Datasources     []string            `yaml:"datasources,omitempty" mapstructure:"datasources" json:"datasources,omitempty"`
	Providers       []string            `yaml:"providers,omitempty" mapstructure:"providers" json:"providers,omitempty"`
	Patterns        []string            `yaml:"patterns,omitempty" mapstructure:"patterns" json:"patterns,omitempty"`
	ExcludePatterns []string            `yaml:"exclude_patterns,omitempty" mapstructure:"exclude_patterns" json:"exclude_patterns,omitempty"`
	Update          VersionUpdatePolicy `yaml:"update,omitempty" mapstructure:"update" json:"update,omitempty"`
	Include         []string            `yaml:"include,omitempty" mapstructure:"include" json:"include,omitempty"`
	Exclude         []string            `yaml:"exclude,omitempty" mapstructure:"exclude" json:"exclude,omitempty"`
	Prerelease      *bool               `yaml:"prerelease,omitempty" mapstructure:"prerelease" json:"prerelease,omitempty"`
	Labels          []string            `yaml:"labels,omitempty" mapstructure:"labels" json:"labels,omitempty"`
}

// VersionEntry configures one externally managed version.
type VersionEntry struct {
	Ecosystem  string              `yaml:"ecosystem,omitempty" mapstructure:"ecosystem" json:"ecosystem,omitempty"`
	Datasource string              `yaml:"datasource,omitempty" mapstructure:"datasource" json:"datasource,omitempty"`
	Provider   string              `yaml:"provider,omitempty" mapstructure:"provider" json:"provider,omitempty"`
	Package    string              `yaml:"package,omitempty" mapstructure:"package" json:"package,omitempty"`
	Desired    string              `yaml:"desired,omitempty" mapstructure:"desired" json:"desired,omitempty"`
	Group      string              `yaml:"group,omitempty" mapstructure:"group" json:"group,omitempty"`
	Update     VersionUpdatePolicy `yaml:"update,omitempty" mapstructure:"update" json:"update,omitempty"`
	Include    []string            `yaml:"include,omitempty" mapstructure:"include" json:"include,omitempty"`
	Exclude    []string            `yaml:"exclude,omitempty" mapstructure:"exclude" json:"exclude,omitempty"`
	Prerelease *bool               `yaml:"prerelease,omitempty" mapstructure:"prerelease" json:"prerelease,omitempty"`
	Labels     []string            `yaml:"labels,omitempty" mapstructure:"labels" json:"labels,omitempty"`
}

// VersionFileRule maps a file manager to the paths it maintains, so a single
// `atmos version track apply` sweeps every version-managed file.
type VersionFileRule struct {
	// Manager is the file manager name (github-actions, marker, template).
	Manager string `yaml:"manager,omitempty" mapstructure:"manager" json:"manager,omitempty"`
	// Paths are glob patterns (doublestar) relative to the project root.
	Paths []string `yaml:"paths,omitempty" mapstructure:"paths" json:"paths,omitempty"`
	// Options carries manager-specific settings.
	Options map[string]any `yaml:"options,omitempty" mapstructure:"options" json:"options,omitempty"`
}

// VersionTrack configures a named version lane such as dev, staging, or prod.
type VersionTrack struct {
	Extends      string                  `yaml:"extends,omitempty" mapstructure:"extends" json:"extends,omitempty"`
	Defaults     VersionPolicy           `yaml:"defaults,omitempty" mapstructure:"defaults" json:"defaults,omitempty"`
	Dependencies map[string]VersionEntry `yaml:"dependencies,omitempty" mapstructure:"dependencies" json:"dependencies,omitempty"`
}

// Version configures version checking, constraint validation, self-management, and managed external versions.
type Version struct {
	Check      VersionCheck      `yaml:"check,omitempty" mapstructure:"check" json:"check,omitempty"`
	Constraint VersionConstraint `yaml:"constraint,omitempty" mapstructure:"constraint" json:"constraint,omitempty"`

	// Use specifies the exact Atmos version to use for this project.
	// When set, Atmos will automatically re-execute itself with the specified version,
	// installing it first if not already present.
	//
	// Examples:
	//   "1.160.0"  - Use exact version
	//   "latest"   - Resolve and use the latest available version
	//
	// The re-exec uses the toolchain installer, storing versions in ~/.atmos/bin/cloudposse/atmos/{version}/.
	Use string `yaml:"use,omitempty" mapstructure:"use" json:"use,omitempty"`

	// Track is the default managed version track.
	Track string `yaml:"track,omitempty" mapstructure:"track" json:"track,omitempty"`

	// LockFile is the path to the managed versions lock file. Relative paths are
	// resolved from the Atmos base path.
	LockFile string `yaml:"lock_file,omitempty" mapstructure:"lock_file" json:"lock_file,omitempty"`

	Providers map[string]VersionProvider `yaml:"providers,omitempty" mapstructure:"providers" json:"providers,omitempty"`
	Defaults  VersionPolicy              `yaml:"defaults,omitempty" mapstructure:"defaults" json:"defaults,omitempty"`
	Groups    map[string]VersionGroup    `yaml:"groups,omitempty" mapstructure:"groups" json:"groups,omitempty"`
	// Dependencies is the base catalog of external dependencies managed by the
	// Version Tracker. Track-level dependencies override these entries.
	Dependencies map[string]VersionEntry `yaml:"dependencies,omitempty" mapstructure:"dependencies" json:"dependencies,omitempty"`
	Tracks       map[string]VersionTrack `yaml:"tracks,omitempty" mapstructure:"tracks" json:"tracks,omitempty"`

	// Files declares which project files the file managers maintain. When
	// empty, managers with default paths (github-actions, template) run over
	// those defaults.
	Files []VersionFileRule `yaml:"files,omitempty" mapstructure:"files" json:"files,omitempty"`
}
