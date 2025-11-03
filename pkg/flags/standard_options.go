package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardOptions provides strongly-typed access to standard Atmos command flags.
// Used for commands that don't pass through arguments to external tools (describe, list, validate, vendor, etc.).
//
// Embeds GlobalFlags for global Atmos flags (identity, chdir, config, logs, etc.).
// Provides common command fields (Stack, Component, Format, File, etc.).
//
// Commands with additional flags should embed StandardOptions and add their own fields.
type StandardOptions struct {
	GlobalFlags // Embedded global flags (identity, chdir, config, logs, pager, profiling, etc.)

	// Common command flags.
	Stack     string // Stack to operate on (--stack, -s)
	Component string // Component to operate on (--component, -c)

	// Output formatting flags.
	Format string // Output format (--format, -f): yaml, json, etc.
	File   string // Write output to file (--file)

	// Processing flags (common across describe/terraform/helmfile commands).
	ProcessTemplates     bool     // Enable Go template processing (--process-templates)
	ProcessYamlFunctions bool     // Enable YAML functions processing (--process-functions)
	Skip                 []string // Skip YAML functions (--skip)

	// Dry run flag (common across vendor, workflow, pro commands).
	DryRun bool // Simulate operation without making changes (--dry-run)

	// Query flag (for describe commands with jq/jmespath).
	Query string // JQ/JMESPath query string (--query)

	// Additional common flags.
	Provenance bool // Enable provenance tracking (--provenance)

	// List command specific flags.
	Abstract   bool   // Include abstract components (--abstract)
	Vars       bool   // Show only vars section (--vars)
	MaxColumns int    // Maximum columns for table output (--max-columns)
	Delimiter  string // Delimiter for CSV/TSV output (--delimiter)

	// Vendor command specific flags.
	Type string // Component type filter: terraform or helmfile (--type)
	Tags string // Component tag filter (--tags)

	// Validate command specific flags.
	SchemaPath           string   // Path to schema file (--schema-path)
	SchemaType           string   // Schema type: jsonschema or opa (--schema-type)
	ModulePaths          []string // OPA module paths (--module-paths)
	Timeout              int      // Validation timeout in seconds (--timeout)
	SchemasAtmosManifest string   // Path to Atmos manifest schema (--schemas-atmos-manifest)

	// Auth command specific flags.
	Login      bool   // Perform login before command (--login)
	Provider   string // Identity provider filter (--provider)
	Providers  string // Comma-separated providers list (--providers)
	Identities string // Comma-separated identities list (--identities)
	All        bool   // Apply to all items (--all)
	Everything bool   // Vendor all components (--everything)

	// Describe affected command specific flags.
	Ref                         string // Git reference for comparison (--ref)
	Sha                         string // Git commit SHA for comparison (--sha)
	RepoPath                    string // Path to cloned target repository (--repo-path)
	SSHKey                      string // Path to SSH private key (--ssh-key)
	SSHKeyPassword              string // Password for encrypted SSH key (--ssh-key-password)
	IncludeSpaceliftAdminStacks bool   // Include Spacelift admin stacks (--include-spacelift-admin-stacks)
	IncludeDependents           bool   // Include dependent components (--include-dependents)
	IncludeSettings             bool   // Include settings section (--include-settings)
	Upload                      bool   // Upload to HTTP endpoint (--upload)
	CloneTargetRef              bool   // Clone target ref instead of checkout (--clone-target-ref)
	Verbose                     bool   // Deprecated verbose flag (--verbose)
	ExcludeLocked               bool   // Exclude locked components (--exclude-locked)

	// Describe workflows command specific flags.
	Components     []string // Filter by specific components (--components)
	ComponentTypes []string // Filter by component types (--component-types)
	Output         string   // Output type: list, detail (--output)

	// Positional arguments (component name, etc.).
	positionalArgs []string
}

// GetGlobalFlags returns a pointer to the embedded GlobalFlags.
// Implements CommandOptions interface.
func (s *StandardOptions) GetGlobalFlags() *GlobalFlags {
	defer perf.Track(nil, "flagparser.StandardOptions.GetGlobalFlags")()

	return &s.GlobalFlags
}

// GetPositionalArgs returns positional arguments extracted by the parser.
// For standard commands: typically component name or other required args.
func (s *StandardOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.StandardOptions.GetPositionalArgs")()

	return s.positionalArgs
}

// GetPassThroughArgs returns pass-through arguments.
// For standard commands: always empty (no pass-through).
func (s *StandardOptions) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.StandardOptions.GetPassThroughArgs")()

	return []string{}
}
