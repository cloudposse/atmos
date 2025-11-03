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
	if required {
		b.options = append(b.options, WithRequiredStringFlag("component", "c", "Atmos component"))
	} else {
		b.options = append(b.options, WithStringFlag("component", "c", "", "Atmos component"))
	}
	b.options = append(b.options, WithEnvVars("component", "ATMOS_COMPONENT"))
	return b
}

// WithFormat adds the format flag with specified default value and optional valid values.
// Maps to StandardOptions.Format field.
//
// Parameters:
//   - defaultValue: default format (e.g., "yaml", "json", "table")
//   - validFormats: optional list of valid format values for help text (e.g., []string{"json", "yaml"})
//
// Example:
//
//	WithFormat("yaml", []string{"json", "yaml"})  // describe stacks
//	WithFormat("table", []string{"table", "tree", "json", "yaml", "graphviz", "mermaid", "markdown"})  // auth list
//	WithFormat("json")  // backward compatible - no validation list
func (b *StandardOptionsBuilder) WithFormat(defaultValue string, validFormats ...string) *StandardOptionsBuilder {
	description := "Output format"
	if len(validFormats) > 0 {
		description = fmt.Sprintf("Output format (valid: %s)", strings.Join(validFormats, ", "))
	}
	b.options = append(b.options, WithStringFlag("format", "f", defaultValue, description))
	b.options = append(b.options, WithEnvVars("format", "ATMOS_FORMAT"))

	// Add validation for valid formats if provided.
	// TODO: Implement WithValidValues for validation.
	// if len(validFormats) > 0 {
	// 	b.options = append(b.options, WithValidValues("format", validFormats))
	// }

	return b
}

// WithFile adds the file output flag.
// Maps to StandardOptions.File field.
func (b *StandardOptionsBuilder) WithFile() *StandardOptionsBuilder {
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
	b.options = append(b.options, WithBoolFlag("process-functions", "", defaultValue, "Enable/disable YAML functions processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"))
	return b
}

// WithSkip adds the skip flag for skipping YAML functions.
// Maps to StandardOptions.Skip field.
func (b *StandardOptionsBuilder) WithSkip() *StandardOptionsBuilder {
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
	b.options = append(b.options, WithBoolFlag("dry-run", "", false, "Simulate operation without making changes"))
	b.options = append(b.options, WithEnvVars("dry-run", "ATMOS_DRY_RUN"))
	return b
}

// WithQuery adds the query flag for JQ/JMESPath queries.
// Maps to StandardOptions.Query field.
func (b *StandardOptionsBuilder) WithQuery() *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("query", "q", "", "JQ/JMESPath query to filter output"))
	b.options = append(b.options, WithEnvVars("query", "ATMOS_QUERY"))
	return b
}

// WithProvenance adds the provenance tracking flag.
// Maps to StandardOptions.Provenance field.
func (b *StandardOptionsBuilder) WithProvenance() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("provenance", "", false, "Enable provenance tracking to show where configuration values originated"))
	b.options = append(b.options, WithEnvVars("provenance", "ATMOS_PROVENANCE"))
	return b
}

// WithAbstract adds the abstract flag for including abstract components.
// Maps to StandardOptions.Abstract field.
func (b *StandardOptionsBuilder) WithAbstract() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("abstract", "", false, "Include abstract components in output"))
	b.options = append(b.options, WithEnvVars("abstract", "ATMOS_ABSTRACT"))
	return b
}

// WithVars adds the vars flag for showing only the vars section.
// Maps to StandardOptions.Vars field.
func (b *StandardOptionsBuilder) WithVars() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("vars", "", false, "Show only the vars section"))
	b.options = append(b.options, WithEnvVars("vars", "ATMOS_VARS"))
	return b
}

// WithMaxColumns adds the max-columns flag for table output.
// Maps to StandardOptions.MaxColumns field.
func (b *StandardOptionsBuilder) WithMaxColumns(defaultValue int) *StandardOptionsBuilder {
	b.options = append(b.options, WithIntFlag("max-columns", "", defaultValue, "Maximum number of columns to display in table format"))
	b.options = append(b.options, WithEnvVars("max-columns", "ATMOS_MAX_COLUMNS"))
	return b
}

// WithDelimiter adds the delimiter flag for CSV/TSV output.
// Maps to StandardOptions.Delimiter field.
func (b *StandardOptionsBuilder) WithDelimiter(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("delimiter", "", defaultValue, "Delimiter for CSV/TSV output"))
	b.options = append(b.options, WithEnvVars("delimiter", "ATMOS_DELIMITER"))
	return b
}

// WithType adds the type flag for component type filtering.
// Maps to StandardOptions.Type field.
func (b *StandardOptionsBuilder) WithType(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("type", "t", defaultValue, "Component type: terraform or helmfile"))
	b.options = append(b.options, WithEnvVars("type", "ATMOS_TYPE"))
	return b
}

// WithTags adds the tags flag for component tag filtering.
// Maps to StandardOptions.Tags field.
func (b *StandardOptionsBuilder) WithTags(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("tags", "", defaultValue, "Component tag filter"))
	b.options = append(b.options, WithEnvVars("tags", "ATMOS_TAGS"))
	return b
}

// WithSchemaPath adds the schema-path flag.
// Maps to StandardOptions.SchemaPath field.
func (b *StandardOptionsBuilder) WithSchemaPath(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("schema-path", "", defaultValue, "Path to schema file"))
	b.options = append(b.options, WithEnvVars("schema-path", "ATMOS_SCHEMA_PATH"))
	return b
}

// WithSchemaType adds the schema-type flag.
// Maps to StandardOptions.SchemaType field.
func (b *StandardOptionsBuilder) WithSchemaType(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("schema-type", "", defaultValue, "Schema type: jsonschema or opa"))
	b.options = append(b.options, WithEnvVars("schema-type", "ATMOS_SCHEMA_TYPE"))
	return b
}

// WithModulePaths adds the module-paths flag.
// Maps to StandardOptions.ModulePaths field.
func (b *StandardOptionsBuilder) WithModulePaths() *StandardOptionsBuilder {
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
	b.options = append(b.options, WithIntFlag("timeout", "", defaultValue, "Validation timeout in seconds"))
	b.options = append(b.options, WithEnvVars("timeout", "ATMOS_TIMEOUT"))
	return b
}

// WithSchemasAtmosManifest adds the schemas-atmos-manifest flag.
// Maps to StandardOptions.SchemasAtmosManifest field.
func (b *StandardOptionsBuilder) WithSchemasAtmosManifest(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("schemas-atmos-manifest", "", defaultValue, "Path to Atmos manifest JSON Schema"))
	b.options = append(b.options, WithEnvVars("schemas-atmos-manifest", "ATMOS_SCHEMAS_ATMOS_MANIFEST"))
	return b
}

// WithLogin adds the login flag.
// Maps to StandardOptions.Login field.
func (b *StandardOptionsBuilder) WithLogin() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("login", "", false, "Perform login before executing command"))
	b.options = append(b.options, WithEnvVars("login", "ATMOS_LOGIN"))
	return b
}

// WithProvider adds the provider flag.
// Maps to StandardOptions.Provider field.
func (b *StandardOptionsBuilder) WithProvider() *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("provider", "", "", "Identity provider filter"))
	b.options = append(b.options, WithEnvVars("provider", "ATMOS_PROVIDER"))
	return b
}

// WithProviders adds the providers flag.
// Maps to StandardOptions.Providers field.
func (b *StandardOptionsBuilder) WithProviders() *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("providers", "", "", "Comma-separated providers list"))
	b.options = append(b.options, WithEnvVars("providers", "ATMOS_PROVIDERS"))
	return b
}

// WithIdentities adds the identities flag.
// Maps to StandardOptions.Identities field.
func (b *StandardOptionsBuilder) WithIdentities() *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("identities", "", "", "Comma-separated identities list"))
	b.options = append(b.options, WithEnvVars("identities", "ATMOS_IDENTITIES"))
	return b
}

// WithAll adds the all flag.
// Maps to StandardOptions.All field.
func (b *StandardOptionsBuilder) WithAll() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("all", "", false, "Apply operation to all items"))
	b.options = append(b.options, WithEnvVars("all", "ATMOS_ALL"))
	return b
}

// Build creates the StandardParser with all configured flags.
// Returns a parser ready for RegisterFlags() and Parse() operations.
func (b *StandardOptionsBuilder) Build() *StandardParser {
	defer perf.Track(nil, "flagparser.StandardOptionsBuilder.Build")()

	return NewStandardParser(b.options...)
}

// WithEverything adds the everything flag for vendoring all components.
func (b *StandardOptionsBuilder) WithEverything() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("everything", "", false, "Vendor all components"))
	b.options = append(b.options, WithEnvVars("everything", "ATMOS_EVERYTHING"))
	return b
}

// WithRef adds the ref flag for Git reference comparison.
func (b *StandardOptionsBuilder) WithRef(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("ref", "", defaultValue, "Git reference for comparison"))
	b.options = append(b.options, WithEnvVars("ref", "ATMOS_REF"))
	return b
}

// WithSha adds the sha flag for Git commit SHA comparison.
func (b *StandardOptionsBuilder) WithSha(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("sha", "", defaultValue, "Git commit SHA for comparison"))
	b.options = append(b.options, WithEnvVars("sha", "ATMOS_SHA"))
	return b
}

// WithRepoPath adds the repo-path flag for target repository path.
func (b *StandardOptionsBuilder) WithRepoPath(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("repo-path", "", defaultValue, "Path to cloned target repository"))
	b.options = append(b.options, WithEnvVars("repo-path", "ATMOS_REPO_PATH"))
	return b
}

// WithSSHKey adds the ssh-key flag for SSH private key path.
func (b *StandardOptionsBuilder) WithSSHKey(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("ssh-key", "", defaultValue, "Path to SSH private key"))
	b.options = append(b.options, WithEnvVars("ssh-key", "ATMOS_SSH_KEY"))
	return b
}

// WithSSHKeyPassword adds the ssh-key-password flag.
func (b *StandardOptionsBuilder) WithSSHKeyPassword(defaultValue string) *StandardOptionsBuilder {
	b.options = append(b.options, WithStringFlag("ssh-key-password", "", defaultValue, "Password for encrypted SSH key"))
	b.options = append(b.options, WithEnvVars("ssh-key-password", "ATMOS_SSH_KEY_PASSWORD"))
	return b
}

// WithIncludeSpaceliftAdminStacks adds the include-spacelift-admin-stacks flag.
func (b *StandardOptionsBuilder) WithIncludeSpaceliftAdminStacks() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("include-spacelift-admin-stacks", "", false, "Include Spacelift admin stacks"))
	b.options = append(b.options, WithEnvVars("include-spacelift-admin-stacks", "ATMOS_INCLUDE_SPACELIFT_ADMIN_STACKS"))
	return b
}

// WithIncludeDependents adds the include-dependents flag.
func (b *StandardOptionsBuilder) WithIncludeDependents() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("include-dependents", "", false, "Include dependent components"))
	b.options = append(b.options, WithEnvVars("include-dependents", "ATMOS_INCLUDE_DEPENDENTS"))
	return b
}

// WithIncludeSettings adds the include-settings flag.
func (b *StandardOptionsBuilder) WithIncludeSettings() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("include-settings", "", false, "Include settings section"))
	b.options = append(b.options, WithEnvVars("include-settings", "ATMOS_INCLUDE_SETTINGS"))
	return b
}

// WithUpload adds the upload flag for HTTP endpoint upload.
func (b *StandardOptionsBuilder) WithUpload() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("upload", "", false, "Upload to HTTP endpoint"))
	b.options = append(b.options, WithEnvVars("upload", "ATMOS_UPLOAD"))
	return b
}

// WithCloneTargetRef adds the clone-target-ref flag.
func (b *StandardOptionsBuilder) WithCloneTargetRef() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("clone-target-ref", "", false, "Clone target ref instead of checkout"))
	b.options = append(b.options, WithEnvVars("clone-target-ref", "ATMOS_CLONE_TARGET_REF"))
	return b
}

// WithVerbose adds the verbose flag (deprecated).
func (b *StandardOptionsBuilder) WithVerbose() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("verbose", "", false, "Deprecated. Use --logs-level=Debug"))
	b.options = append(b.options, WithEnvVars("verbose", "ATMOS_VERBOSE"))
	return b
}

// WithExcludeLocked adds the exclude-locked flag.
func (b *StandardOptionsBuilder) WithExcludeLocked() *StandardOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("exclude-locked", "", false, "Exclude locked components"))
	b.options = append(b.options, WithEnvVars("exclude-locked", "ATMOS_EXCLUDE_LOCKED"))
	return b
}

// WithComponents adds the components flag for filtering by specific components.
// Maps to StandardOptions.Components field.
func (b *StandardOptionsBuilder) WithComponents() *StandardOptionsBuilder {
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

// WithOutput adds the output flag for output type selection with optional validation.
// Maps to StandardOptions.Output field.
//
// Parameters:
//   - defaultValue: default output type (e.g., "list", "map")
//   - validOutputs: optional list of valid output values for validation (e.g., []string{"list", "map", "all"})
//
// Example:
//
//	WithOutput("list", []string{"list", "map", "all"})  // describe workflows
//	WithOutput("table")  // backward compatible - no validation list
func (b *StandardOptionsBuilder) WithOutput(defaultValue string, validOutputs ...string) *StandardOptionsBuilder {
	description := "Output type"
	if len(validOutputs) > 0 {
		description = fmt.Sprintf("Output type (valid: %s)", strings.Join(validOutputs, ", "))
	}
	b.options = append(b.options, WithStringFlag("output", "o", defaultValue, description))
	b.options = append(b.options, WithEnvVars("output", "ATMOS_OUTPUT"))

	// Add validation for valid outputs if provided.
	// TODO: Implement WithValidValues for validation.
	// if len(validOutputs) > 0 {
	// 	b.options = append(b.options, WithValidValues("output", validOutputs))
	// }

	return b
}
