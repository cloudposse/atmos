package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardParser handles flag parsing for standard Atmos commands (non-pass-through).
// Returns strongly-typed StandardOptions with all parsed flags.
//
// Standard commands include: describe, list, validate, vendor, workflow, aws, pro, etc.
// These commands don't pass arguments through to external tools.
type StandardParser struct {
	parser *StandardFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewStandardParser creates a parser for standard commands with specified flags.
// Use existing Option functions (WithStringFlag, WithBoolFlag, etc.) to add command-specific flags.
//
// Example:
//
//	parser := NewStandardParser(
//	    WithStringFlag("stack", "s", "", "Stack name"),
//	    WithStringFlag("format", "f", "yaml", "Output format"),
//	    WithBoolFlag("dry-run", "", false, "Dry run mode"),
//	)
func NewStandardParser(opts ...Option) *StandardParser {
	defer perf.Track(nil, "flagparser.NewStandardParser")()

	return &StandardParser{
		parser: NewStandardFlagParser(opts...),
	}
}

// SetPositionalArgs configures positional argument extraction and validation.
// Delegates to the underlying StandardFlagParser.
func (p *StandardParser) SetPositionalArgs(
	specs []*PositionalArgSpec,
	validator cobra.PositionalArgs,
	usage string,
) {
	defer perf.Track(nil, "flagparser.StandardParser.SetPositionalArgs")()

	p.parser.SetPositionalArgs(specs, validator, usage)
}

// RegisterFlags adds flags to the Cobra command.
func (p *StandardParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.StandardParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// RegisterPersistentFlags adds flags as persistent flags (inherited by subcommands).
// This is used for global flags that should be available to all subcommands.
func (p *StandardParser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.StandardParser.RegisterPersistentFlags")()

	p.cmd = cmd
	p.parser.RegisterPersistentFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *StandardParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.StandardParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// BindFlagsToViper binds Cobra flags to Viper for precedence handling.
func (p *StandardParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.StandardParser.BindFlagsToViper")()

	return p.parser.BindFlagsToViper(cmd, v)
}

// Parse processes command-line arguments and returns strongly-typed StandardOptions.
//
// Handles precedence (CLI > ENV > config > defaults) via Viper.
// Extracts positional arguments (e.g., component name).
func (p *StandardParser) Parse(ctx context.Context, args []string) (*StandardOptions, error) {
	defer perf.Track(nil, "flagparser.StandardParser.Parse")()

	// Use underlying parser to parse flags and extract positional args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Extract positional args into struct fields using TargetField mapping.
	// This is configured via WithPositionalArgs() on the builder.
	positionalValues := make(map[string]string)
	if p.parser.positionalArgs != nil {
		specs := p.parser.positionalArgs.specs
		for i, spec := range specs {
			if i < len(parsedConfig.PositionalArgs) {
				// Map positional arg value to its target field
				positionalValues[spec.TargetField] = parsedConfig.PositionalArgs[i]
			}
		}
	}

	// Determine component value.
	// Priority: component flag > positional arg extraction > hardcoded fallback.
	component := getString(parsedConfig.Flags, "component")
	if component == "" {
		// Try positional arg from builder config first
		if val, ok := positionalValues["Component"]; ok {
			component = val
		} else if len(parsedConfig.PositionalArgs) == 1 {
			// Fallback for commands not using builder pattern yet
			component = parsedConfig.PositionalArgs[0]
		}
	}

	// Convert to strongly-typed options.
	opts := StandardOptions{
		GlobalFlags: GlobalFlags{
			Chdir:           getString(parsedConfig.Flags, "chdir"),
			BasePath:        getString(parsedConfig.Flags, "base-path"),
			Config:          getStringSlice(parsedConfig.Flags, "config"),
			ConfigPath:      getStringSlice(parsedConfig.Flags, "config-path"),
			LogsLevel:       getString(parsedConfig.Flags, "logs-level"),
			LogsFile:        getString(parsedConfig.Flags, "logs-file"),
			NoColor:         getBool(parsedConfig.Flags, "no-color"),
			Pager:           getPagerSelector(parsedConfig.Flags, "pager"),
			Identity:        getIdentitySelector(parsedConfig.Flags, "identity"),
			ProfilerEnabled: getBool(parsedConfig.Flags, "profiler-enabled"),
			ProfilerPort:    getInt(parsedConfig.Flags, "profiler-port"),
			ProfilerHost:    getString(parsedConfig.Flags, "profiler-host"),
			ProfileFile:     getString(parsedConfig.Flags, "profile-file"),
			ProfileType:     getString(parsedConfig.Flags, "profile-type"),
			Heatmap:         getBool(parsedConfig.Flags, "heatmap"),
			HeatmapMode:     getString(parsedConfig.Flags, "heatmap-mode"),
			RedirectStderr:  getString(parsedConfig.Flags, "redirect-stderr"),
			Version:         getBool(parsedConfig.Flags, "version"),
		},
		Stack:                       getString(parsedConfig.Flags, "stack"),
		Component:                   component,
		Format:                      getString(parsedConfig.Flags, "format"),
		File:                        getString(parsedConfig.Flags, "file"),
		ProcessTemplates:            getBool(parsedConfig.Flags, "process-templates"),
		ProcessYamlFunctions:        getBool(parsedConfig.Flags, "process-functions"),
		Skip:                        getStringSlice(parsedConfig.Flags, "skip"),
		DryRun:                      getBool(parsedConfig.Flags, "dry-run"),
		Query:                       getString(parsedConfig.Flags, "query"),
		Provenance:                  getBool(parsedConfig.Flags, "provenance"),
		Abstract:                    getBool(parsedConfig.Flags, "abstract"),
		Vars:                        getBool(parsedConfig.Flags, "vars"),
		MaxColumns:                  getInt(parsedConfig.Flags, "max-columns"),
		Delimiter:                   getString(parsedConfig.Flags, "delimiter"),
		Type:                        getString(parsedConfig.Flags, "type"),
		Tags:                        getString(parsedConfig.Flags, "tags"),
		SchemaPath:                  getString(parsedConfig.Flags, "schema-path"),
		SchemaType:                  getString(parsedConfig.Flags, "schema-type"),
		ModulePaths:                 getStringSlice(parsedConfig.Flags, "module-paths"),
		Timeout:                     getInt(parsedConfig.Flags, "timeout"),
		SchemasAtmosManifest:        getString(parsedConfig.Flags, "schemas-atmos-manifest"),
		Login:                       getBool(parsedConfig.Flags, "login"),
		Provider:                    getString(parsedConfig.Flags, "provider"),
		Providers:                   getString(parsedConfig.Flags, "providers"),
		Identities:                  getString(parsedConfig.Flags, "identities"),
		All:                         getBool(parsedConfig.Flags, "all"),
		Everything:                  getBool(parsedConfig.Flags, "everything"),
		Ref:                         getString(parsedConfig.Flags, "ref"),
		Sha:                         getString(parsedConfig.Flags, "sha"),
		RepoPath:                    getString(parsedConfig.Flags, "repo-path"),
		SSHKey:                      getString(parsedConfig.Flags, "ssh-key"),
		SSHKeyPassword:              getString(parsedConfig.Flags, "ssh-key-password"),
		IncludeSpaceliftAdminStacks: getBool(parsedConfig.Flags, "include-spacelift-admin-stacks"),
		IncludeDependents:           getBool(parsedConfig.Flags, "include-dependents"),
		IncludeSettings:             getBool(parsedConfig.Flags, "include-settings"),
		Upload:                      getBool(parsedConfig.Flags, "upload"),
		CloneTargetRef:              getBool(parsedConfig.Flags, "clone-target-ref"),
		Verbose:                     getBool(parsedConfig.Flags, "verbose"),
		ExcludeLocked:               getBool(parsedConfig.Flags, "exclude-locked"),
		Components:                  getStringSlice(parsedConfig.Flags, "components"),
		ComponentTypes:              getStringSlice(parsedConfig.Flags, "component-types"),
		Output:                      getString(parsedConfig.Flags, "output"),
		positionalArgs:              parsedConfig.PositionalArgs,
	}

	return &opts, nil
}
