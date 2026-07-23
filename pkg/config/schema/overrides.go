package configschema

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	schemaTitle = "JSON Schema for the Atmos CLI configuration (atmos.yaml). Version 1.0. https://atmos.tools"

	schemaDescription = "Configuration for the Atmos CLI, authored in atmos.yaml (or atmos.d fragments). " +
		"Generated from the Go configuration structs, so it always matches what Atmos actually reads."

	// The name of the shared `$defs` entry that models an Atmos YAML function
	// invocation in its authored (string) form.
	yamlFunctionDef = "yamlFunction"

	// JSON Schema type names used when constructing and inspecting subschemas.
	typeString  = "string"
	typeObject  = "object"
	typeArray   = "array"
	typeBoolean = "boolean"
	typeInteger = "integer"
	typeNumber  = "number"
	typeNull    = "null"
)

// excludedRootFields are AtmosConfiguration fields that carry yaml tags (so they
// survive serialization in `atmos describe config`) but are computed at runtime
// and must never appear in the authored-config schema. Fields tagged `yaml:"-"`
// are skipped by reflection automatically and do not need to be listed.
// TestEveryRootFieldIsClassified fails whenever a new AtmosConfiguration field is
// neither `yaml:"-"`, listed here, nor present in the generated schema.
var excludedRootFields = []string{
	"initialized",
	"basePathAbsolute",
	"stacksBaseAbsolutePath",
	"includeStackAbsolutePaths",
	"excludeStackAbsolutePaths",
	"terraformDirAbsolutePath",
	"helmfileDirAbsolutePath",
	"packerDirAbsolutePath",
	"ansibleDirAbsolutePath",
	"kubernetesDirAbsolutePath",
	"helmDirAbsolutePath",
	"stackConfigFilesRelativePaths",
	"stackConfigFilesAbsolutePaths",
	"stackType",
	"default",
	"cli_config_path",
	"stores_registry",
}

// atmosConfigYamlFunctions are the YAML functions supported when loading
// atmos.yaml (the dispatch in pkg/config/process_yaml.go). Stack-manifest-only
// functions (!terraform.output, !store, !secret, …) are intentionally absent;
// the classification is enforced by TestEveryYamlFunctionIsClassified.
var atmosConfigYamlFunctions = []string{
	u.AtmosYamlFuncAppend,
	u.AtmosYamlFuncCwd,
	u.AtmosYamlFuncEnv,
	u.AtmosYamlFuncExec,
	u.AtmosYamlFuncGitBranch,
	u.AtmosYamlFuncGitHost,
	u.AtmosYamlFuncGitName,
	u.AtmosYamlFuncGitOwner,
	u.AtmosYamlFuncGitRef,
	u.AtmosYamlFuncGitRepository,
	u.AtmosYamlFuncGitRoot,
	u.AtmosYamlFuncGitRootAlias,
	u.AtmosYamlFuncGitSha,
	u.AtmosYamlFuncGitUrl,
	u.AtmosYamlFuncInclude,
	u.AtmosYamlFuncIncludeRaw,
	u.AtmosYamlFuncRandom,
	u.AtmosYamlFuncUnset,
}

// typeMapper overrides reflection for types whose authored YAML forms differ
// from their Go representation. A fresh schema is returned per call because the
// reflector annotates the returned instance with field doc comments.
func typeMapper(t reflect.Type) *jsonschema.Schema {
	switch t {
	case reflect.TypeOf(time.Duration(0)):
		// Durations are authored as Go duration strings; the decode hook also
		// accepts bare integers (nanoseconds).
		return &jsonschema.Schema{AnyOf: []*jsonschema.Schema{
			{Type: typeString, Description: "Go duration string (e.g. \"500ms\", \"30s\", \"5m\")."},
			{Type: typeInteger, Description: "Duration in nanoseconds."},
		}}
	case reflect.TypeOf(schema.Condition{}):
		// `when:` supports scalar, list, and object forms
		// (see pkg/condition Condition.UnmarshalYAML).
		return &jsonschema.Schema{AnyOf: []*jsonschema.Schema{
			{Type: typeString, Description: "Predicate (always, never, ci, local, success, failure) or CEL expression."},
			{Type: typeBoolean},
			{Type: typeArray, Description: "Conditions that must all hold."},
			{Type: typeObject, Description: "Structured condition (all/any/not/cel)."},
		}}
	}
	return nil
}

// applyOverrides applies the curated delta between what reflection produces and
// what users may author in atmos.yaml. Every part of this delta is guarded by a
// ratchet test, so new configuration fields and new YAML functions must be
// classified here before the build goes green.
func applyOverrides(r *jsonschema.Reflector, root *jsonschema.Schema) {
	root.ID = jsonschema.ID(SchemaID)
	root.Title = schemaTitle
	root.Description = schemaDescription

	for _, field := range excludedRootFields {
		root.Properties.Delete(field)
	}

	if root.Definitions == nil {
		root.Definitions = jsonschema.Definitions{}
	}
	overrideSchemasSection(r, root)
	applyPolymorphicOverrides(root)
	allowYamlFunctions(root)
}

// applyPolymorphicOverrides relaxes shapes that custom unmarshalers accept
// beyond what the Go field types declare. Polymorphic keys backed by `yaml:"-"`
// fields (e.g. Task `with:`, `background:`, `for:`) need no override — they are
// absent from the schema and admitted by permissive additionalProperties. The
// corpus test over examples/ is the backstop that surfaces missing entries.
func applyPolymorphicOverrides(root *jsonschema.Schema) {
	// Custom command steps accept a plain-string shell shorthand
	// (see schema.Tasks.UnmarshalYAML).
	if tasks, ok := root.Definitions["Tasks"]; ok && tasks.Items != nil {
		tasks.Items = anyOfWith(tasks.Items, &jsonschema.Schema{
			Type:        "string",
			Description: "Shell command shorthand for a step.",
		})
	}

	// Definition properties whose decode hooks or weakly-typed decoding (see
	// schema.Task.UnmarshalYAML, commandEnvMapDecodeHook, Terminal.IsPagerEnabled)
	// accept more shapes than the Go field declares.
	propertyAlternatives := map[string]map[string][]*jsonschema.Schema{
		"Task": {
			// output/prompt: scalar or structured object.
			"output": {{Type: typeObject}},
			"prompt": {{Type: typeObject}},
			// default: string, boolean, or number (depends on the step type).
			"default": {{Type: typeBoolean}, {Type: typeNumber}},
			// padding/margin: "1 2" string or bare integer shorthand.
			"padding": {{Type: typeInteger}},
			"margin":  {{Type: typeInteger}},
		},
		"Command": {
			// env accepts the map form ({KEY: value}) in addition to the list
			// of {key, value} entries (commandEnvMapDecodeHook).
			"env": {{Type: typeObject}},
		},
		"Terminal": {
			// pager is a string flag ("less", "on", "false"); booleans are
			// accepted and coerced (Terminal.IsPagerEnabled).
			"pager": {{Type: typeBoolean}},
		},
	}
	for defName, alternativesByProperty := range propertyAlternatives {
		def, ok := root.Definitions[defName]
		if !ok || def.Properties == nil {
			continue
		}
		for name, alternatives := range alternativesByProperty {
			if existing, found := def.Properties.Get(name); found && existing != nil {
				def.Properties.Set(name, anyOfWith(existing, alternatives...))
			}
		}
	}
}

// anyOfWith returns an anyOf of the original subschema and the alternatives,
// hoisting the description so editors keep surfacing it on hover.
func anyOfWith(s *jsonschema.Schema, alternatives ...*jsonschema.Schema) *jsonschema.Schema {
	description := s.Description
	s.Description = ""
	return &jsonschema.Schema{
		AnyOf:       append([]*jsonschema.Schema{s}, alternatives...),
		Description: description,
	}
}

// overrideSchemasSection replaces the weakly-typed `schemas` property
// (map[string]any with a custom unmarshaler — see
// schema.AtmosConfiguration.UnmarshalYAML) with its real contract: each entry is
// a plain path/URL string, a ResourcePath ({base_path}), or a SchemaRegistry
// ({manifest, schema, matches}).
func overrideSchemasSection(r *jsonschema.Reflector, root *jsonschema.Schema) {
	description := ""
	if existing, ok := root.Properties.Get("schemas"); ok && existing != nil {
		description = existing.Description
	}
	root.Properties.Set("schemas", &jsonschema.Schema{
		Type:        "object",
		Description: description,
		AdditionalProperties: &jsonschema.Schema{
			AnyOf: []*jsonschema.Schema{
				{Type: typeString, Description: "Path or URL of the schema resource."},
				reflectDefinitionRef(r, root, &schema.ResourcePath{}),
				reflectDefinitionRef(r, root, &schema.SchemaRegistry{}),
			},
		},
	})
}

// reflectDefinitionRef reflects v into root's `$defs` and returns a `$ref` to it,
// so polymorphic sections stay derived from the Go structs instead of being
// hand-written here.
func reflectDefinitionRef(r *jsonschema.Reflector, root *jsonschema.Schema, v any) *jsonschema.Schema {
	name := reflect.TypeOf(v).Elem().Name()
	sub := r.Reflect(v)
	for n, d := range sub.Definitions {
		root.Definitions[n] = d
	}
	sub.Definitions = nil
	sub.Version = ""
	sub.ID = ""
	root.Definitions[name] = sub
	return &jsonschema.Schema{Ref: "#/$defs/" + name}
}

// allowYamlFunctions rewrites the schema so any constrained value may
// alternatively be an Atmos YAML function invocation (e.g.
// `logs: !include shared.yaml`): every subschema that would reject the authored
// string form gains an anyOf alternative referencing the yamlFunction
// definition. This mirrors the atmos-manifest schema's `^!include` pattern
// approach and works both for `atmos validate schema` (which stringifies custom
// tags) and for editors (yaml-language-server treats custom-tagged nodes as
// scalars).
func allowYamlFunctions(root *jsonschema.Schema) {
	root.Definitions[yamlFunctionDef] = &jsonschema.Schema{
		Type:        "string",
		Pattern:     yamlFunctionPattern(),
		Description: "Atmos YAML function invocation whose result replaces this value when atmos.yaml is loaded (e.g. `!include`, `!env`, `!exec`).",
	}
	for _, def := range root.Definitions {
		wrapChildren(def)
	}
	wrapChildren(root)
}

// yamlFunctionPattern builds a regular expression matching any supported YAML
// function invocation in its stringified form ("!include cfg.yaml"). Longer
// names sort first so prefixes ("!include") cannot shadow extensions
// ("!include.raw") in engines that report the first alternative.
func yamlFunctionPattern() string {
	tags := make([]string, len(atmosConfigYamlFunctions))
	copy(tags, atmosConfigYamlFunctions)
	sort.Slice(tags, func(i, j int) bool {
		if len(tags[i]) != len(tags[j]) {
			return len(tags[i]) > len(tags[j])
		}
		return tags[i] < tags[j]
	})
	escaped := make([]string, 0, len(tags))
	for _, tag := range tags {
		escaped = append(escaped, regexp.QuoteMeta(tag))
	}
	return "^(?:" + strings.Join(escaped, "|") + ")(?:\\s|$)"
}

// wrapChildren recursively rewrites every constrained child subschema into an
// anyOf of itself and the authored alternatives (null, and a YAML function
// where the original would reject its string form). Definitions are wrapped at
// their reference sites, never at the definition root, so `$ref` targets stay
// plain objects. Pre-existing anyOf wrappers (polymorphic overrides) gain a
// null branch in place instead of being nested.
func wrapChildren(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if s.Properties != nil {
		for pair := s.Properties.Oldest(); pair != nil; pair = pair.Next() {
			child := pair.Value
			wrapChildren(child)
			s.Properties.Set(pair.Key, wrapped(child))
		}
	}
	if s.Items != nil {
		wrapChildren(s.Items)
		s.Items = wrapped(s.Items)
	}
	if s.AdditionalProperties != nil {
		wrapChildren(s.AdditionalProperties)
		s.AdditionalProperties = wrapped(s.AdditionalProperties)
	}
	for key, sub := range s.PatternProperties {
		wrapChildren(sub)
		s.PatternProperties[key] = wrapped(sub)
	}
	for _, sub := range s.AnyOf {
		wrapChildren(sub)
	}
	for _, sub := range s.OneOf {
		wrapChildren(sub)
	}
	for _, sub := range s.AllOf {
		wrapChildren(sub)
	}
}

// wrapped returns the subschema rewritten to accept its authored alternatives:
// null (an empty value like `env:` decodes to null and means "unset" to Viper)
// and, where the original would reject it, the string form of a YAML function
// invocation. Unconstrained subschemas are returned as-is; a pre-existing anyOf
// wrapper gains a null branch in place.
func wrapped(s *jsonschema.Schema) *jsonschema.Schema {
	switch {
	case s == nil:
		return nil
	case isAnyOfWrapper(s):
		if !anyOfAcceptsNull(s.AnyOf) {
			s.AnyOf = append(s.AnyOf, &jsonschema.Schema{Type: typeNull})
		}
		if !anyOfAcceptsYamlFunction(s.AnyOf) {
			s.AnyOf = append(s.AnyOf, &jsonschema.Schema{Ref: "#/$defs/" + yamlFunctionDef})
		}
		return s
	case s.Ref == "" && s.Type == "" || s.Type == typeNull:
		return s
	}
	return anyOfWith(s, authoredAlternatives(s)...)
}

// isAnyOfWrapper reports whether the subschema is a bare anyOf wrapper (a
// polymorphic override or a wrapper produced by this pass).
func isAnyOfWrapper(s *jsonschema.Schema) bool {
	return len(s.AnyOf) > 0 && s.Type == "" && s.Ref == ""
}

// authoredAlternatives returns the alternative shapes a constrained value also
// accepts when authored in atmos.yaml.
func authoredAlternatives(s *jsonschema.Schema) []*jsonschema.Schema {
	alternatives := []*jsonschema.Schema{{Type: typeNull}}
	if rejectsYamlFunctionString(s) {
		alternatives = append(alternatives, &jsonschema.Schema{Ref: "#/$defs/" + yamlFunctionDef})
	}
	// String-list fields also accept a scalar string: the loader's
	// StringToSliceHookFunc coerces `import: "path"` into a one-element list.
	if s.Type == typeArray && arrayItemsAcceptString(s.Items) {
		alternatives = append(alternatives, &jsonschema.Schema{Type: typeString})
	}
	return alternatives
}

// arrayItemsAcceptString reports whether an array's items admit plain strings
// (directly, or via an anyOf branch added by this generator).
func arrayItemsAcceptString(items *jsonschema.Schema) bool {
	if items == nil {
		return false
	}
	if items.Type == typeString {
		return true
	}
	for _, branch := range items.AnyOf {
		if branch != nil && branch.Type == typeString {
			return true
		}
	}
	return false
}

// anyOfAcceptsNull reports whether one of the branches already admits null.
func anyOfAcceptsNull(branches []*jsonschema.Schema) bool {
	for _, branch := range branches {
		if branch != nil && branch.Type == typeNull {
			return true
		}
	}
	return false
}

// anyOfAcceptsYamlFunction reports whether one of the branches already admits
// the authored string form of a YAML function invocation (an unconstrained
// string branch, or the yamlFunction reference itself).
func anyOfAcceptsYamlFunction(branches []*jsonschema.Schema) bool {
	for _, branch := range branches {
		if branch == nil {
			continue
		}
		if branch.Ref == "#/$defs/"+yamlFunctionDef {
			return true
		}
		if branch.Type == typeString && !rejectsYamlFunctionString(branch) {
			return true
		}
	}
	return false
}

// rejectsYamlFunctionString reports whether a subschema would reject the
// authored string form of a YAML function invocation. Unconstrained strings
// already accept it.
func rejectsYamlFunctionString(s *jsonschema.Schema) bool {
	if s.Ref != "" {
		return s.Ref != "#/$defs/"+yamlFunctionDef
	}
	switch s.Type {
	case typeObject, typeArray, typeBoolean, typeInteger, typeNumber:
		return true
	case typeString:
		return len(s.Enum) > 0 || s.Pattern != "" || s.Format != "" || s.Const != nil
	default:
		return false
	}
}
