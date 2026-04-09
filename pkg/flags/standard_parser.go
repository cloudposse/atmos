package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags/global"
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
	defer perf.Track(nil, "flags.NewStandardParser")()

	return &StandardParser{
		parser: NewStandardFlagParser(opts...),
	}
}

// Registry returns the underlying flag registry.
// This allows access to the registry for operations like SetCompletionFunc()
// that need to modify flags after parser creation.
func (p *StandardParser) Registry() *FlagRegistry {
	defer perf.Track(nil, "flags.StandardParser.Registry")()

	return p.parser.Registry()
}

// SetPositionalArgs configures positional argument extraction and validation.
// Delegates to the underlying StandardFlagParser.
func (p *StandardParser) SetPositionalArgs(
	specs []*PositionalArgSpec,
	validator cobra.PositionalArgs,
	usage string,
) {
	defer perf.Track(nil, "flags.StandardParser.SetPositionalArgs")()

	p.parser.SetPositionalArgs(specs, validator, usage)
}

// RegisterFlags adds flags to the Cobra command.
// Does NOT set DisableFlagParsing - allows Cobra to validate flags normally.
// Commands that need pass-through set DisableFlagParsing=true manually.
func (p *StandardParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.StandardParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// RegisterPersistentFlags adds flags as persistent flags (inherited by subcommands).
// This is used for global flags that should be available to all subcommands.
func (p *StandardParser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.StandardParser.RegisterPersistentFlags")()

	p.cmd = cmd
	p.parser.RegisterPersistentFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *StandardParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.StandardParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// BindFlagsToViper binds Cobra flags to Viper for precedence handling.
func (p *StandardParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flags.StandardParser.BindFlagsToViper")()

	return p.parser.BindFlagsToViper(cmd, v)
}

// Parse processes command-line arguments and returns strongly-typed StandardOptions.
//
// Handles precedence (CLI > ENV > config > defaults) via Viper.
// Extracts positional arguments (e.g., component name).
func (p *StandardParser) Parse(ctx context.Context, args []string) (*StandardOptions, error) {
	defer perf.Track(nil, "flags.StandardParser.Parse")()

	// Use underlying parser to parse flags and extract positional args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Extract and resolve positional arg values.
	positionalValues := p.extractPositionalValues(parsedConfig.PositionalArgs)
	component := p.resolveComponentValue(parsedConfig.Flags, positionalValues, parsedConfig.PositionalArgs)
	schemaType := p.resolveSchemaTypeValue(parsedConfig.Flags, positionalValues)
	key := p.resolveKeyValue(positionalValues)

	// Convert to strongly-typed options.
	opts := p.buildStandardOptions(parsedConfig, component, schemaType, key)

	return &opts, nil
}

// extractPositionalValues extracts positional args into a map using TargetField mapping.
// This is configured via WithPositionalArgs() on the builder.
func (p *StandardParser) extractPositionalValues(positionalArgs []string) map[string]string {
	defer perf.Track(nil, "flags.StandardParser.extractPositionalValues")()

	positionalValues := make(map[string]string)
	if p.parser.positionalArgs != nil {
		specs := p.parser.positionalArgs.specs
		for i, spec := range specs {
			if i < len(positionalArgs) {
				// Map positional arg value to its target field.
				positionalValues[spec.TargetField] = positionalArgs[i]
			}
		}
	}
	return positionalValues
}

// resolveComponentValue determines the component value using the following priority:
// 1. Component flag value.
// 2. Positional arg from builder config.
// 3. Hardcoded fallback for commands not using builder pattern.
func (p *StandardParser) resolveComponentValue(flags map[string]interface{}, positionalValues map[string]string, positionalArgs []string) string {
	defer perf.Track(nil, "flags.StandardParser.resolveComponentValue")()

	component := GetString(flags, "component")
	if component == "" {
		// Try positional arg from builder config first.
		if val, ok := positionalValues["Component"]; ok {
			component = val
		} else if len(positionalArgs) == 1 {
			// Fallback for commands not using builder pattern yet.
			component = positionalArgs[0]
		}
	}
	return component
}

// resolveSchemaTypeValue determines the schema type value from positional args if configured.
func (p *StandardParser) resolveSchemaTypeValue(flags map[string]interface{}, positionalValues map[string]string) string {
	defer perf.Track(nil, "flags.StandardParser.resolveSchemaTypeValue")()

	schemaType := GetString(flags, "schema-type")
	if schemaType == "" {
		if val, ok := positionalValues["SchemaType"]; ok {
			schemaType = val
		}
	}
	return schemaType
}

// resolveKeyValue determines the key value from positional args if configured.
func (p *StandardParser) resolveKeyValue(positionalValues map[string]string) string {
	defer perf.Track(nil, "flags.StandardParser.resolveKeyValue")()

	key := ""
	if val, ok := positionalValues["Key"]; ok {
		key = val
	}
	return key
}

// buildStandardOptions builds a StandardOptions struct from parsed config and resolved values.
//
//nolint:revive,funlen // Function length exceeded due to large struct initialization (68 lines). Splitting would reduce readability.
func (p *StandardParser) buildStandardOptions(parsedConfig *ParsedConfig, component, schemaType, key string) StandardOptions {
	defer perf.Track(nil, "flags.StandardParser.buildStandardOptions")()

	return StandardOptions{
		Flags: global.Flags{
			Chdir:           GetString(parsedConfig.Flags, "chdir"),
			BasePath:        GetString(parsedConfig.Flags, "base-path"),
			Config:          GetStringSlice(parsedConfig.Flags, "config"),
			ConfigPath:      GetStringSlice(parsedConfig.Flags, "config-path"),
			LogsLevel:       GetString(parsedConfig.Flags, "logs-level"),
			LogsFile:        GetString(parsedConfig.Flags, "logs-file"),
			NoColor:         GetBool(parsedConfig.Flags, "no-color"),
			ForceColor:      GetBool(parsedConfig.Flags, "force-color"),
			ForceTTY:        GetBool(parsedConfig.Flags, "force-tty"),
			Mask:            GetBool(parsedConfig.Flags, "mask"),
			Pager:           GetPagerSelector(parsedConfig.Flags, "pager"),
			Identity:        GetIdentitySelector(parsedConfig.Flags, "identity"),
			ProfilerEnabled: GetBool(parsedConfig.Flags, "profiler-enabled"),
			ProfilerPort:    GetInt(parsedConfig.Flags, "profiler-port"),
			ProfilerHost:    GetString(parsedConfig.Flags, "profiler-host"),
			ProfileFile:     GetString(parsedConfig.Flags, "profile-file"),
			ProfileType:     GetString(parsedConfig.Flags, "profile-type"),
			Heatmap:         GetBool(parsedConfig.Flags, "heatmap"),
			HeatmapMode:     GetString(parsedConfig.Flags, "heatmap-mode"),
			RedirectStderr:  GetString(parsedConfig.Flags, "redirect-stderr"),
			Version:         GetBool(parsedConfig.Flags, "version"),
		},
		Stack:                       GetString(parsedConfig.Flags, "stack"),
		Component:                   component,
		Format:                      GetString(parsedConfig.Flags, "format"),
		File:                        GetString(parsedConfig.Flags, "file"),
		ProcessTemplates:            GetBool(parsedConfig.Flags, "process-templates"),
		ProcessYamlFunctions:        GetBool(parsedConfig.Flags, "process-functions"),
		Skip:                        GetStringSlice(parsedConfig.Flags, "skip"),
		DryRun:                      GetBool(parsedConfig.Flags, "dry-run"),
		Query:                       GetString(parsedConfig.Flags, "query"),
		Provenance:                  GetBool(parsedConfig.Flags, "provenance"),
		Abstract:                    GetBool(parsedConfig.Flags, "abstract"),
		Vars:                        GetBool(parsedConfig.Flags, "vars"),
		MaxColumns:                  GetInt(parsedConfig.Flags, "max-columns"),
		Delimiter:                   GetString(parsedConfig.Flags, "delimiter"),
		Type:                        GetString(parsedConfig.Flags, "type"),
		Tags:                        GetString(parsedConfig.Flags, "tags"),
		SchemaPath:                  GetString(parsedConfig.Flags, "schema-path"),
		SchemaType:                  schemaType,
		Key:                         key,
		ModulePaths:                 GetStringSlice(parsedConfig.Flags, "module-paths"),
		Timeout:                     GetInt(parsedConfig.Flags, "timeout"),
		SchemasAtmosManifest:        GetString(parsedConfig.Flags, "schemas-atmos-manifest"),
		Login:                       GetBool(parsedConfig.Flags, "login"),
		Provider:                    GetString(parsedConfig.Flags, "provider"),
		Providers:                   GetString(parsedConfig.Flags, "providers"),
		Identities:                  GetString(parsedConfig.Flags, "identities"),
		All:                         GetBool(parsedConfig.Flags, "all"),
		Everything:                  GetBool(parsedConfig.Flags, "everything"),
		Ref:                         GetString(parsedConfig.Flags, "ref"),
		Sha:                         GetString(parsedConfig.Flags, "sha"),
		RepoPath:                    GetString(parsedConfig.Flags, "repo-path"),
		SSHKey:                      GetString(parsedConfig.Flags, "ssh-key"),
		SSHKeyPassword:              GetString(parsedConfig.Flags, "ssh-key-password"),
		IncludeSpaceliftAdminStacks: GetBool(parsedConfig.Flags, "include-spacelift-admin-stacks"),
		IncludeDependents:           GetBool(parsedConfig.Flags, "include-dependents"),
		IncludeSettings:             GetBool(parsedConfig.Flags, "include-settings"),
		Upload:                      GetBool(parsedConfig.Flags, "upload"),
		CloneTargetRef:              GetBool(parsedConfig.Flags, "clone-target-ref"),
		Verbose:                     GetBool(parsedConfig.Flags, "verbose"),
		ExcludeLocked:               GetBool(parsedConfig.Flags, "exclude-locked"),
		Components:                  GetStringSlice(parsedConfig.Flags, "components"),
		ComponentTypes:              GetStringSlice(parsedConfig.Flags, "component-types"),
		Output:                      GetString(parsedConfig.Flags, "output"),
		positionalArgs:              parsedConfig.PositionalArgs,
	}
}
