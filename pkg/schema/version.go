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

// Version configures version checking, constraint validation, and self-management.
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
}
