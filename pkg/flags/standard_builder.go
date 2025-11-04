package flags

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardOptionsBuilder provides a type-safe, fluent interface for building StandardParser
// with strongly-typed flag definitions that map directly to StandardOptions fields.
//
// Benefits:
//   - Compile-time guarantee that flags map to StandardOptions fields
//   - Refactoring-safe: renaming struct fields updates flag definitions
//   - Clear intent: method names match struct field names
//   - Testable: each method can be unit tested independently
//
// Example:
//
//	parser := flagparser.NewStandardOptionsBuilder().
//	    WithStack(true).        // Required stack flag → .Stack field
//	    WithFormat("yaml").     // Format flag with default → .Format field
//	    WithQuery().            // Optional query flag → .Query field
//	    Build()
//
//	opts, _ := parser.Parse(ctx, args)
//	fmt.Println(opts.Stack)   // Type-safe!
//	fmt.Println(opts.Format)  // Type-safe!
type StandardOptionsBuilder struct {
	options []Option
}

// NewStandardOptionsBuilder creates a new builder for StandardParser.
func NewStandardOptionsBuilder() *StandardOptionsBuilder {
	defer perf.Track(nil, "flagparser.NewStandardOptionsBuilder")()

	return &StandardOptionsBuilder{
		options: []Option{},
	}
}

// WithStack adds the stack flag.
// Maps to StandardOptions.Stack field.
//
// Parameters:
//   - required: if true, flag is marked as required
func (b *StandardOptionsBuilder) WithStack(required bool) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithStack")()

	if required {
		b.options = append(b.options, WithRequiredStringFlag("stack", "s", "Atmos stack"))
	} else {
		b.options = append(b.options, WithStringFlag("stack", "s", "", "Atmos stack"))
	}
	b.options = append(b.options, WithEnvVars("stack", "ATMOS_STACK"))
	return b
}

// WithComponent adds the component flag.
// Maps to StandardOptions.Component field.
//
// Parameters:
//   - required: if true, flag is marked as required
func (b *StandardOptionsBuilder) WithComponent(required bool) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithComponent")()

	if required {
		b.options = append(b.options, WithRequiredStringFlag("component", "c", "Atmos component"))
	} else {
		b.options = append(b.options, WithStringFlag("component", "c", "", "Atmos component"))
	}
	b.options = append(b.options, WithEnvVars("component", "ATMOS_COMPONENT"))
	return b
}

// WithFormat adds the format output flag with explicit valid values and default.
// Maps to StandardOptions.Format field.
//
// Parameters:
//   - validFormats: List of valid format values (e.g., []string{"json", "yaml", "table"})
//   - defaultValue: Default format to use when flag not provided
//
// Example:
//
//	WithFormat([]string{"json", "yaml"}, "yaml")           // describe stacks
//	WithFormat([]string{"table", "tree", "json"}, "table") // auth list
func (b *StandardOptionsBuilder) WithFormat(validFormats []string, defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithFormat")()

	description := fmt.Sprintf("Output format (valid: %s)", strings.Join(validFormats, ", "))
	b.options = append(b.options, WithStringFlag("format", "f", defaultValue, description))
	b.options = append(b.options, WithEnvVars("format", "ATMOS_FORMAT"))
	b.options = append(b.options, WithValidValues("format", validFormats...))
	return b
}

// WithFile adds the file output flag.
// Maps to StandardOptions.File field.
func (b *StandardOptionsBuilder) WithFile() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithFile")()

	b.options = append(b.options, WithStringFlag("file", "", "", "Write output to file"))
	b.options = append(b.options, WithEnvVars("file", "ATMOS_FILE"))
	return b
}

// WithProcessTemplates adds the process-templates flag with specified default.
// Maps to StandardOptions.ProcessTemplates field.
//
// Parameters:
//   - defaultValue: default value (typically true)
func (b *StandardOptionsBuilder) WithProcessTemplates(defaultValue bool) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithProcessTemplates")()

	b.options = append(b.options, WithBoolFlag("process-templates", "", defaultValue, "Enable/disable Go template processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"))
	return b
}

// WithProcessFunctions adds the process-functions flag with specified default.
// Maps to StandardOptions.ProcessYamlFunctions field.
//
// Parameters:
//   - defaultValue: default value (typically true)
func (b *StandardOptionsBuilder) WithProcessFunctions(defaultValue bool) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithProcessFunctions")()

	b.options = append(b.options, WithBoolFlag("process-functions", "", defaultValue, "Enable/disable YAML functions processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"))
	return b
}

// WithSkip adds the skip flag for skipping YAML functions.
// Maps to StandardOptions.Skip field.
func (b *StandardOptionsBuilder) WithSkip() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSkip")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "skip",
			Shorthand:   "",
			Default:     []string{},
			Description: "Skip executing a YAML function in the Atmos stack manifests",
			EnvVars:     []string{"ATMOS_SKIP"},
		})
	})
	return b
}

// WithDryRun adds the dry-run flag.
// Maps to StandardOptions.DryRun field.
func (b *StandardOptionsBuilder) WithDryRun() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithDryRun")()

	b.options = append(b.options, WithBoolFlag("dry-run", "", false, "Simulate operation without making changes"))
	b.options = append(b.options, WithEnvVars("dry-run", "ATMOS_DRY_RUN"))
	return b
}

// WithQuery adds the query flag for JQ/JMESPath queries.
// Maps to StandardOptions.Query field.
func (b *StandardOptionsBuilder) WithQuery() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithQuery")()

	b.options = append(b.options, WithStringFlag("query", "q", "", "JQ/JMESPath query to filter output"))
	b.options = append(b.options, WithEnvVars("query", "ATMOS_QUERY"))
	return b
}

// WithProvenance adds the provenance tracking flag.
// Maps to StandardOptions.Provenance field.
func (b *StandardOptionsBuilder) WithProvenance() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithProvenance")()

	b.options = append(b.options, WithBoolFlag("provenance", "", false, "Enable provenance tracking to show where configuration values originated"))
	b.options = append(b.options, WithEnvVars("provenance", "ATMOS_PROVENANCE"))
	return b
}

// WithAbstract adds the abstract flag for including abstract components.
// Maps to StandardOptions.Abstract field.
func (b *StandardOptionsBuilder) WithAbstract() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithAbstract")()

	b.options = append(b.options, WithBoolFlag("abstract", "", false, "Include abstract components in output"))
	b.options = append(b.options, WithEnvVars("abstract", "ATMOS_ABSTRACT"))
	return b
}

// WithVars adds the vars flag for showing only the vars section.
// Maps to StandardOptions.Vars field.
func (b *StandardOptionsBuilder) WithVars() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithVars")()

	b.options = append(b.options, WithBoolFlag("vars", "", false, "Show only the vars section"))
	b.options = append(b.options, WithEnvVars("vars", "ATMOS_VARS"))
	return b
}

// WithMaxColumns adds the max-columns flag for table output.
// Maps to StandardOptions.MaxColumns field.
func (b *StandardOptionsBuilder) WithMaxColumns(defaultValue int) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithMaxColumns")()

	b.options = append(b.options, WithIntFlag("max-columns", "", defaultValue, "Maximum number of columns to display in table format"))
	b.options = append(b.options, WithEnvVars("max-columns", "ATMOS_MAX_COLUMNS"))
	return b
}

// WithDelimiter adds the delimiter flag for CSV/TSV output.
// Maps to StandardOptions.Delimiter field.
func (b *StandardOptionsBuilder) WithDelimiter(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithDelimiter")()

	b.options = append(b.options, WithStringFlag("delimiter", "", defaultValue, "Delimiter for CSV/TSV output"))
	b.options = append(b.options, WithEnvVars("delimiter", "ATMOS_DELIMITER"))
	return b
}

// WithType adds the type flag for component type filtering.
// Maps to StandardOptions.Type field.
func (b *StandardOptionsBuilder) WithType(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithType")()

	b.options = append(b.options, WithStringFlag("type", "t", defaultValue, "Component type: terraform or helmfile"))
	b.options = append(b.options, WithEnvVars("type", "ATMOS_TYPE"))
	return b
}

// WithTags adds the tags flag for component tag filtering.
// Maps to StandardOptions.Tags field.
func (b *StandardOptionsBuilder) WithTags(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithTags")()

	b.options = append(b.options, WithStringFlag("tags", "", defaultValue, "Component tag filter"))
	b.options = append(b.options, WithEnvVars("tags", "ATMOS_TAGS"))
	return b
}

// WithSchemaPath adds the schema-path flag.
// Maps to StandardOptions.SchemaPath field.
func (b *StandardOptionsBuilder) WithSchemaPath(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSchemaPath")()

	b.options = append(b.options, WithStringFlag("schema-path", "", defaultValue, "Path to schema file"))
	b.options = append(b.options, WithEnvVars("schema-path", "ATMOS_SCHEMA_PATH"))
	return b
}

// WithSchemaType adds the schema-type flag.
// Maps to StandardOptions.SchemaType field.
func (b *StandardOptionsBuilder) WithSchemaType(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSchemaType")()

	b.options = append(b.options, WithStringFlag("schema-type", "", defaultValue, "Schema type: jsonschema or opa"))
	b.options = append(b.options, WithEnvVars("schema-type", "ATMOS_SCHEMA_TYPE"))
	return b
}

// WithModulePaths adds the module-paths flag.
// Maps to StandardOptions.ModulePaths field.
func (b *StandardOptionsBuilder) WithModulePaths() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithModulePaths")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "module-paths",
			Shorthand:   "",
			Default:     []string{},
			Description: "OPA module paths",
			EnvVars:     []string{"ATMOS_MODULE_PATHS"},
		})
	})
	return b
}

// WithTimeout adds the timeout flag.
// Maps to StandardOptions.Timeout field.
func (b *StandardOptionsBuilder) WithTimeout(defaultValue int) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithTimeout")()

	b.options = append(b.options, WithIntFlag("timeout", "", defaultValue, "Validation timeout in seconds"))
	b.options = append(b.options, WithEnvVars("timeout", "ATMOS_TIMEOUT"))
	return b
}

// WithSchemasAtmosManifest adds the schemas-atmos-manifest flag.
// Maps to StandardOptions.SchemasAtmosManifest field.
func (b *StandardOptionsBuilder) WithSchemasAtmosManifest(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSchemasAtmosManifest")()

	b.options = append(b.options, WithStringFlag("schemas-atmos-manifest", "", defaultValue, "Path to Atmos manifest JSON Schema"))
	b.options = append(b.options, WithEnvVars("schemas-atmos-manifest", "ATMOS_SCHEMAS_ATMOS_MANIFEST"))
	return b
}

// WithLogin adds the login flag.
// Maps to StandardOptions.Login field.
func (b *StandardOptionsBuilder) WithLogin() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithLogin")()

	b.options = append(b.options, WithBoolFlag("login", "", false, "Perform login before executing command"))
	b.options = append(b.options, WithEnvVars("login", "ATMOS_LOGIN"))
	return b
}

// WithProvider adds the provider flag.
// Maps to StandardOptions.Provider field.
func (b *StandardOptionsBuilder) WithProvider() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithProvider")()

	b.options = append(b.options, WithStringFlag("provider", "", "", "Identity provider filter"))
	b.options = append(b.options, WithEnvVars("provider", "ATMOS_PROVIDER"))
	return b
}

// WithProviders adds the providers flag.
// Maps to StandardOptions.Providers field.
func (b *StandardOptionsBuilder) WithProviders() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithProviders")()

	b.options = append(b.options, WithStringFlag("providers", "", "", "Comma-separated providers list"))
	b.options = append(b.options, WithEnvVars("providers", "ATMOS_PROVIDERS"))
	return b
}

// WithIdentities adds the identities flag.
// Maps to StandardOptions.Identities field.
func (b *StandardOptionsBuilder) WithIdentities() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithIdentities")()

	b.options = append(b.options, WithStringFlag("identities", "", "", "Comma-separated identities list"))
	b.options = append(b.options, WithEnvVars("identities", "ATMOS_IDENTITIES"))
	return b
}

// WithAll adds the all flag.
// Maps to StandardOptions.All field.
func (b *StandardOptionsBuilder) WithAll() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithAll")()

	b.options = append(b.options, WithBoolFlag("all", "", false, "Apply operation to all items"))
	b.options = append(b.options, WithEnvVars("all", "ATMOS_ALL"))
	return b
}

// Build creates the StandardParser with all configured flags.
// Returns a parser ready for RegisterFlags() and Parse() operations.
func (b *StandardOptionsBuilder) Build() *StandardParser {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.Build")()

	defer perf.Track(nil, "flagparser.StandardOptionsBuilder.Build")()

	return NewStandardParser(b.options...)
}

// WithEverything adds the everything flag for vendoring all components.
func (b *StandardOptionsBuilder) WithEverything() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithEverything")()

	b.options = append(b.options, WithBoolFlag("everything", "", false, "Vendor all components"))
	b.options = append(b.options, WithEnvVars("everything", "ATMOS_EVERYTHING"))
	return b
}

// WithRef adds the ref flag for Git reference comparison.
func (b *StandardOptionsBuilder) WithRef(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithRef")()

	b.options = append(b.options, WithStringFlag("ref", "", defaultValue, "Git reference for comparison"))
	b.options = append(b.options, WithEnvVars("ref", "ATMOS_REF"))
	return b
}

// WithSha adds the sha flag for Git commit SHA comparison.
func (b *StandardOptionsBuilder) WithSha(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSha")()

	b.options = append(b.options, WithStringFlag("sha", "", defaultValue, "Git commit SHA for comparison"))
	b.options = append(b.options, WithEnvVars("sha", "ATMOS_SHA"))
	return b
}

// WithRepoPath adds the repo-path flag for target repository path.
func (b *StandardOptionsBuilder) WithRepoPath(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithRepoPath")()

	b.options = append(b.options, WithStringFlag("repo-path", "", defaultValue, "Path to cloned target repository"))
	b.options = append(b.options, WithEnvVars("repo-path", "ATMOS_REPO_PATH"))
	return b
}

// WithSSHKey adds the ssh-key flag for SSH private key path.
func (b *StandardOptionsBuilder) WithSSHKey(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSSHKey")()

	b.options = append(b.options, WithStringFlag("ssh-key", "", defaultValue, "Path to SSH private key"))
	b.options = append(b.options, WithEnvVars("ssh-key", "ATMOS_SSH_KEY"))
	return b
}

// WithSSHKeyPassword adds the ssh-key-password flag.
func (b *StandardOptionsBuilder) WithSSHKeyPassword(defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithSSHKeyPassword")()

	b.options = append(b.options, WithStringFlag("ssh-key-password", "", defaultValue, "Password for encrypted SSH key"))
	b.options = append(b.options, WithEnvVars("ssh-key-password", "ATMOS_SSH_KEY_PASSWORD"))
	return b
}

// WithIncludeSpaceliftAdminStacks adds the include-spacelift-admin-stacks flag.
func (b *StandardOptionsBuilder) WithIncludeSpaceliftAdminStacks() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithIncludeSpaceliftAdminStacks")()

	b.options = append(b.options, WithBoolFlag("include-spacelift-admin-stacks", "", false, "Include Spacelift admin stacks"))
	b.options = append(b.options, WithEnvVars("include-spacelift-admin-stacks", "ATMOS_INCLUDE_SPACELIFT_ADMIN_STACKS"))
	return b
}

// WithIncludeDependents adds the include-dependents flag.
func (b *StandardOptionsBuilder) WithIncludeDependents() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithIncludeDependents")()

	b.options = append(b.options, WithBoolFlag("include-dependents", "", false, "Include dependent components"))
	b.options = append(b.options, WithEnvVars("include-dependents", "ATMOS_INCLUDE_DEPENDENTS"))
	return b
}

// WithIncludeSettings adds the include-settings flag.
func (b *StandardOptionsBuilder) WithIncludeSettings() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithIncludeSettings")()

	b.options = append(b.options, WithBoolFlag("include-settings", "", false, "Include settings section"))
	b.options = append(b.options, WithEnvVars("include-settings", "ATMOS_INCLUDE_SETTINGS"))
	return b
}

// WithUpload adds the upload flag for HTTP endpoint upload.
func (b *StandardOptionsBuilder) WithUpload() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithUpload")()

	b.options = append(b.options, WithBoolFlag("upload", "", false, "Upload to HTTP endpoint"))
	b.options = append(b.options, WithEnvVars("upload", "ATMOS_UPLOAD"))
	return b
}

// WithCloneTargetRef adds the clone-target-ref flag.
func (b *StandardOptionsBuilder) WithCloneTargetRef() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithCloneTargetRef")()

	b.options = append(b.options, WithBoolFlag("clone-target-ref", "", false, "Clone target ref instead of checkout"))
	b.options = append(b.options, WithEnvVars("clone-target-ref", "ATMOS_CLONE_TARGET_REF"))
	return b
}

// WithVerbose adds the verbose flag (deprecated).
func (b *StandardOptionsBuilder) WithVerbose() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithVerbose")()

	b.options = append(b.options, WithBoolFlag("verbose", "", false, "Deprecated. Use --logs-level=Debug"))
	b.options = append(b.options, WithEnvVars("verbose", "ATMOS_VERBOSE"))
	return b
}

// WithExcludeLocked adds the exclude-locked flag.
func (b *StandardOptionsBuilder) WithExcludeLocked() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithExcludeLocked")()

	b.options = append(b.options, WithBoolFlag("exclude-locked", "", false, "Exclude locked components"))
	b.options = append(b.options, WithEnvVars("exclude-locked", "ATMOS_EXCLUDE_LOCKED"))
	return b
}

// WithComponents adds the components flag for filtering by specific components.
// Maps to StandardOptions.Components field.
func (b *StandardOptionsBuilder) WithComponents() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithComponents")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "components",
			Shorthand:   "",
			Default:     []string{},
			Description: "Filter by specific components",
			EnvVars:     []string{"ATMOS_COMPONENTS"},
		})
	})
	return b
}

// WithComponentTypes adds the component-types flag for filtering by component types.
// Maps to StandardOptions.ComponentTypes field.
func (b *StandardOptionsBuilder) WithComponentTypes() *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithComponentTypes")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "component-types",
			Shorthand:   "",
			Default:     []string{},
			Description: "Filter by component types (terraform, helmfile)",
			EnvVars:     []string{"ATMOS_COMPONENT_TYPES"},
		})
	})
	return b
}

// WithOutput adds the output flag with explicit valid values and default.
// Maps to StandardOptions.Output field.
//
// Parameters:
//   - validOutputs: List of valid output values (e.g., []string{"list", "map", "all"})
//   - defaultValue: Default output type to use when flag not provided
//
// Example:
//
//	WithOutput([]string{"list", "map", "all"}, "list")  // describe workflows
func (b *StandardOptionsBuilder) WithOutput(validOutputs []string, defaultValue string) *StandardOptionsBuilder {
	defer perf.Track(nil, "flags.StandardOptionsBuilder.WithOutput")()

	description := fmt.Sprintf("Output type (valid: %s)", strings.Join(validOutputs, ", "))
	b.options = append(b.options, WithStringFlag("output", "o", defaultValue, description))
	b.options = append(b.options, WithEnvVars("output", "ATMOS_OUTPUT"))
	b.options = append(b.options, WithValidValues("output", validOutputs...))
	return b
}
