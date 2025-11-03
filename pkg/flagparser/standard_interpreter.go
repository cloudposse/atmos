package flagparser

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardInterpreter provides strongly-typed access to standard Atmos command flags.
// Used for commands that don't pass through arguments to external tools (describe, list, validate, vendor, etc.).
//
// Embeds GlobalFlags for global Atmos flags (identity, chdir, config, logs, etc.).
// Provides common command fields (Stack, Component, Format, File, etc.).
//
// Commands with additional flags should embed StandardInterpreter and add their own fields.
type StandardInterpreter struct {
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

	// Positional arguments (component name, etc.).
	positionalArgs []string
}

// GetGlobalFlags returns a pointer to the embedded GlobalFlags.
// Implements CommandInterpreter interface.
func (s *StandardInterpreter) GetGlobalFlags() *GlobalFlags {
	defer perf.Track(nil, "flagparser.StandardInterpreter.GetGlobalFlags")()

	return &s.GlobalFlags
}

// GetPositionalArgs returns positional arguments extracted by the parser.
// For standard commands: typically component name or other required args.
func (s *StandardInterpreter) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.StandardInterpreter.GetPositionalArgs")()

	return s.positionalArgs
}

// GetPassThroughArgs returns pass-through arguments.
// For standard commands: always empty (no pass-through).
func (s *StandardInterpreter) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.StandardInterpreter.GetPassThroughArgs")()

	return []string{}
}
