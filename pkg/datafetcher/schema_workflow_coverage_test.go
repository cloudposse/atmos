package datafetcher

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// This file is the workflow half of the "you can't forget the schema" ratchet (see
// schema_section_coverage_test.go for the top-level/component half). Workflow files are
// commonly annotated with `# yaml-language-server: $schema=...atmos-manifest.json`, so every
// authored field on the WorkflowStep / WorkflowDefinition structs must be modeled in the
// manifest JSON schema. When it is not, editors surface false-positive "Property X is not
// allowed" errors on valid, working workflows (see issue #2708, which shipped 1.222.0 step
// features — `dependencies`, `options`, `default`, and many more — without a schema update).
//
// These tests reflect over the Go structs and fail the build when a field is authored in YAML
// but missing from the schema's `workflow_step` / `workflow_manifest` definitions.

// stepPolymorphicKeys are YAML keys a workflow step accepts that are decoded by WorkflowStep's
// custom UnmarshalYAML rather than by a plain struct field, so they carry `yaml:"-"` and are not
// discovered by tag reflection. They MUST still be modeled in the schema.
//   - `background`: string style color OR boolean async marker (polymorphic).
//   - `for`:        scalar or sequence of target step names (wait/cancel actions).
//   - `with`:       container action parameters (GitHub-Actions `uses`/`with` style).
var stepPolymorphicKeys = []string{"background", "for", "with"}

// TestSchemaCoversWorkflowStepFields asserts every authored WorkflowStep YAML key is modeled as a
// property in the embedded schema's `workflow_step` definition (the single source of truth; the
// website copy is generated from it).
func TestSchemaCoversWorkflowStepFields(t *testing.T) {
	props := workflowDefinitionProps(t, "workflow_step")
	authored := authoredYAMLKeys(reflect.TypeOf(schema.WorkflowStep{}), stepPolymorphicKeys...)

	assertSchemaModelsFields(t, "workflow_step", props, authored)
}

// TestSchemaCoversWorkflowManifestFields asserts every authored WorkflowDefinition YAML key (the
// object under each named workflow, modeled by the schema's `workflow_manifest` definition) is
// present in that definition.
func TestSchemaCoversWorkflowManifestFields(t *testing.T) {
	props := workflowDefinitionProps(t, "workflow_manifest")
	authored := authoredYAMLKeys(reflect.TypeOf(schema.WorkflowDefinition{}))

	assertSchemaModelsFields(t, "workflow_manifest", props, authored)
}

// assertSchemaModelsFields collects every authored field missing from props and fails once with
// the full, sorted list so a single run yields a complete checklist.
func assertSchemaModelsFields(t *testing.T, defName string, props, authored map[string]struct{}) {
	t.Helper()
	var missing []string
	for key := range authored {
		if _, present := props[key]; !present {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	require.Emptyf(t, missing,
		"the %q definition in the manifest JSON schema is missing %d authored field(s): %s.\n"+
			"Add each as a property (with a description for IDE hover text) to\n"+
			"pkg/datafetcher/schema/atmos/manifest/1.0.json, or the schema will\n"+
			"reject valid workflow files annotated with the published $schema (issue #2708).",
		defName, len(missing), strings.Join(missing, ", "))
}

// workflowDefinitionProps returns the property-name set of a workflow definition in the embedded
// schema, handling both the plain object shape (`workflow_step`) and the `oneOf: [string, object]`
// !include shape (`workflow_manifest`).
func workflowDefinitionProps(t *testing.T, defName string) map[string]struct{} {
	t.Helper()
	embeddedSchema := loadEmbeddedSchema(t)
	defs, ok := embeddedSchema["definitions"].(map[string]any)
	require.True(t, ok, "schema should have definitions")
	def, ok := defs[defName].(map[string]any)
	require.Truef(t, ok, "schema should define %q", defName)
	return objectVariantProps(def)
}

// authoredYAMLKeys returns the set of YAML keys a struct authors, read from `yaml` struct tags
// (skipping `-` and empty tags), plus any extra keys decoded by a custom UnmarshalYAML.
func authoredYAMLKeys(rt reflect.Type, extra ...string) map[string]struct{} {
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	out := make(map[string]struct{}, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		key := strings.Split(tag, ",")[0]
		if key == "" || key == "-" {
			continue
		}
		out[key] = struct{}{}
	}
	for _, k := range extra {
		out[k] = struct{}{}
	}
	return out
}
