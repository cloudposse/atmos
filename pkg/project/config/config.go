// Package config provides scaffold configuration management: the
// AtmosScaffoldConfig manifest kind, the interactive setup form, and the
// project record written to generated projects.
//
// NOTE: This package uses github.com/charmbracelet/huh v0.6.0 instead of v0.7.0
// due to a Terminal.app layout breaking issue in v0.7.0. See:
// https://github.com/charmbracelet/huh/issues/631
//
// The issue causes line duplication when navigating between fields in Mac OS Terminal.app
// with tab key or arrow keys. This was introduced around commit hash 310cd4a379ac
// and affects all versions up through 0.7.0. Version 0.6.0 and earlier work correctly.
//
// The package is split across a few files by responsibility to keep each one
// focused and under the repo's file-length limit:
//   - config.go: the AtmosScaffoldConfig manifest types and registration.
//   - persistence.go: loading/saving scaffold config and project records.
//   - validation.go: field-value validation and type coercion.
//   - form.go: the interactive huh-based setup form.
package config

import (
	"errors"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Color constants for consistent styling using lipgloss named colors.
const (
	ColorWhite  = "White"
	ColorBlack  = "Black"
	ColorBlue   = "Blue"
	ColorRed    = "Red"
	ColorGrey   = "Gray"
	ColorPurple = "Magenta"
)

// ScaffoldKind is the manifest kind for scaffold templates and project records.
const ScaffoldKind = "AtmosScaffoldConfig"

// ScaffoldConfigFileName is the name of the scaffold configuration file.
const ScaffoldConfigFileName = "scaffold.yaml"

// dirPermissions is the file mode for creating directories.
const dirPermissions = 0o755

// filePermissions is the file mode for writing the project record.
const filePermissions = 0o644

const fieldNameErrorFormat = "%w: %q"

var (
	errInvalidBooleanFieldValue = errors.New("invalid boolean field value")
	errFieldMustBeText          = errors.New("field must be text")
	errFieldMustBeStringOption  = errors.New("field must be a string option")
	errFieldUnsupportedOption   = errors.New("field has unsupported option")
	errFieldMustBeStringOptions = errors.New("field must be a list of string options")
	errFieldMustBeBoolean       = errors.New("field must be true or false")
	errFieldValidationFailed    = errors.New("field validation failed")
	errInvalidFieldPattern      = errors.New("invalid field validation pattern")
)

// ScaffoldConfigDir is the directory name for user scaffold configuration.
const ScaffoldConfigDir = ".atmos"

// SourceEmbedded marks a project record generated from a template embedded in the Atmos binary.
const SourceEmbedded = "embedded"

//go:generate go run gen_schema.go

func init() {
	manifest.MustRegister(ScaffoldKind, manifest.DefaultAPIVersion, &ScaffoldSpec{})
}

// ScaffoldConfig is the AtmosScaffoldConfig manifest. The same document kind
// serves two roles:
//   - In a template: metadata plus spec.fields describe the questionnaire.
//   - In a generated project (.atmos/scaffold.yaml): the same document with
//     spec.values (the user's answers) and provenance (spec.source,
//     spec.baseRef) merged in, making the project self-describing for
//     future updates.
type ScaffoldConfig = manifest.Manifest[ScaffoldSpec]

// ScaffoldSpec is the spec of an AtmosScaffoldConfig manifest.
type ScaffoldSpec struct {
	// Source records where the template came from (e.g. "embedded", a local
	// path, or a git URL once remote sources are supported). Written to
	// project records; ignored in template manifests.
	Source string `yaml:"source,omitempty" json:"source,omitempty" jsonschema:"description=Where the template came from (written to project records)"`
	// BaseRef records the git ref used as the three-way merge base when the
	// project was generated. Written to project records.
	BaseRef string `yaml:"baseRef,omitempty" json:"baseRef,omitempty" jsonschema:"description=Git ref used as the three-way merge base"`
	// Delimiters optionally overrides the Go template delimiters used when
	// rendering template files (exactly two entries: left and right).
	Delimiters []string `yaml:"delimiters,omitempty" json:"delimiters,omitempty" jsonschema:"description=Template delimiters as a two-element list,minItems=2,maxItems=2"`
	// Fields defines the questionnaire shown when generating from this
	// template. Order is preserved: fields prompt in the order listed.
	Fields []FieldDefinition `yaml:"fields,omitempty" json:"fields,omitempty" jsonschema:"description=Ordered questionnaire fields"`
	// Values holds the user's answers. Written to project records; may also
	// provide preset values in template manifests.
	Values map[string]any `yaml:"values,omitempty" json:"values,omitempty" jsonschema:"description=Field values (answers) keyed by field name"`
	// Files optionally gates generation of specific auto-discovered template
	// files based on collected answers. Files not listed here always
	// generate (subject to the existing path-templating skip behavior).
	Files []FileSpec `yaml:"files,omitempty" json:"files,omitempty" jsonschema:"description=Conditional generation overlay keyed by each file's discovered path"`
	// Hooks runs step-backed actions around generation, keyed by hook name.
	// Reuses the exact schema and vocabulary of stack-level lifecycle hooks
	// (events, kind, when, type, with) -- see pkg/hooks.Hook and the
	// atmos-hooks skill. Only kind: step and kind: steps are supported for
	// scaffold hooks; the events are before.scaffold.generate and
	// after.scaffold.generate. Kept as a raw map (not map[string]hooks.Hook)
	// because pkg/hooks.Hook has no json tags -- like the stacks JSON Schema,
	// which hand-authors its own "hooks" definition rather than reflecting
	// the Go struct, this stays untyped for schema purposes and is decoded
	// into hooks.Hook via DecodeHooks at the point of use.
	Hooks map[string]any `yaml:"hooks,omitempty" json:"hooks,omitempty" jsonschema:"description=Step-backed hooks around generation reusing the stack-level hooks vocabulary"`
}

// DecodeHooks converts a ScaffoldSpec's raw Hooks map into typed
// pkg/hooks.Hook values by round-tripping through YAML, the same technique
// pkg/hooks.StepFromHook uses to decode a hook's own `with:` block -- Hook
// is designed to unmarshal from YAML, not to be reflected into JSON Schema.
func DecodeHooks(raw map[string]any) (map[string]hooks.Hook, error) {
	defer perf.Track(nil, "config.DecodeHooks")()

	if len(raw) == 0 {
		return nil, nil
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrInvalidScaffoldSection).
			WithCause(err).
			WithExplanation("Failed to encode the scaffold `hooks:` block").
			Err()
	}

	decoded := make(map[string]hooks.Hook, len(raw))
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return nil, errUtils.Build(errUtils.ErrInvalidScaffoldSection).
			WithCause(err).
			WithExplanation("Failed to decode the scaffold `hooks:` block").
			Err()
	}
	return decoded, nil
}

// FileSpec optionally gates whether an auto-discovered template file is
// generated, based on the collected field answers.
type FileSpec struct {
	// Path is the file's path as discovered in the template's file tree
	// (matched against the file's original, pre-template-rendering path).
	Path string `yaml:"path" json:"path" jsonschema:"description=File path as discovered in the template's file tree"`
	// When gates generation of this file. Evaluated against the collected
	// answers (as the `answers` CEL variable); a false result skips the
	// file entirely. Empty always generates the file. Schema-restricted to
	// a predicate/CEL string or a list (implicit all) -- the {all:/any:/not:}
	// map form is deliberately excluded here (see the comment on
	// FieldDefinition.When for why) even though pkg/condition itself parses
	// it; use CEL's &&/||/! instead.
	When condition.Condition `yaml:"when,omitempty" json:"when,omitempty" jsonschema:"description=Condition (predicate/CEL string or a list treated as 'all'; use CEL &&/||/! instead of the all/any/not map form) gating whether this file is generated,oneof_type=string;array"`
}

// FieldValidation constrains the allowed values for a FieldDefinition.
type FieldValidation struct {
	// Pattern is a regular expression the value must match (for input fields).
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty" jsonschema:"description=Regular expression the value must match"`
	// Message is the error message shown when validation fails.
	Message string `yaml:"message,omitempty" json:"message,omitempty" jsonschema:"description=Error message shown when validation fails"`
}

// FieldDefinition defines a single questionnaire field: its name, prompt
// type, presentation, validation, and default value.
type FieldDefinition struct {
	Name        string           `yaml:"name" json:"name" jsonschema:"description=Field name used as the template variable"`
	Type        string           `yaml:"type,omitempty" json:"type,omitempty" jsonschema:"description=Prompt type,enum=input,enum=text,enum=string,enum=select,enum=multiselect,enum=confirm,enum=bool,enum=boolean"`
	Label       string           `yaml:"label,omitempty" json:"label,omitempty" jsonschema:"description=Short prompt label"`
	Description string           `yaml:"description,omitempty" json:"description,omitempty" jsonschema:"description=Longer help text shown with the prompt"`
	Required    bool             `yaml:"required,omitempty" json:"required,omitempty" jsonschema:"description=Whether a value must be provided"`
	Default     any              `yaml:"default,omitempty" json:"default,omitempty" jsonschema:"description=Default value"`
	Options     []string         `yaml:"options,omitempty" json:"options,omitempty" jsonschema:"description=Choices for select and multiselect fields"`
	Placeholder string           `yaml:"placeholder,omitempty" json:"placeholder,omitempty" jsonschema:"description=Placeholder text for input fields"`
	Validation  *FieldValidation `yaml:"validation,omitempty" json:"validation,omitempty" jsonschema:"description=Optional validation constraints for this field"`
	// When gates whether this field is prompted for, evaluated against
	// answers collected from fields declared earlier in Fields (as the
	// `answers` CEL variable). Empty always prompts. Schema-restricted to a
	// predicate/CEL string or a list (implicit all): invopop reflects
	// Condition's oneOf branches alongside a sibling additionalProperties:
	// false (Condition has no exported fields), so including "object" here
	// would make the {all:/any:/not:} map form fail schema validation even
	// though pkg/condition parses it -- confirmed empirically, not assumed.
	// Use CEL's &&/||/! for compound conditions instead.
	When condition.Condition `yaml:"when,omitempty" json:"when,omitempty" jsonschema:"description=Condition (predicate/CEL string or a list treated as 'all'; use CEL &&/||/! instead of the all/any/not map form) gating whether this field is prompted for,oneof_type=string;array"`
}

// Config represents the user's configuration values as a generic map to support dynamic fields from scaffold.yaml.
type Config map[string]interface{}
