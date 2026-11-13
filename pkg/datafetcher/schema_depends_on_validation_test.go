package datafetcher

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// The `settings.depends_on` entry schema (`depends_on_manifest`) sets `additionalProperties: false`,
// so every key Atmos actually honors must be modeled or IDEs flag valid manifests as invalid.
// `stack` was missing (https://github.com/cloudposse/atmos/issues/2104) even though it is documented,
// parsed by `internal/exec/dependency_parser.go`, and carried on `schema.Dependent`.

// Compile-time sentinel: if `Stack` is ever renamed or removed from `schema.Dependent`, this fails
// the build so the schema property below is revisited alongside it.
var _ = schema.Dependent{Stack: "tenant1-ue2-prod"}

// dependsOnSchemas returns every schema copy that models `depends_on_manifest`, so a fix applied to
// one but not another fails CI. `website` currently resolves to the same bytes as `embedded` (the
// published copy is generated from the embedded schema), and is listed anyway — matching the sibling
// schema tests — so coverage is already in place if the two ever diverge again.
func dependsOnSchemas(t *testing.T) map[string][]byte {
	t.Helper()

	return map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}
}

func TestManifestSchema_DependsOnAcceptsValidEntries(t *testing.T) {
	validEntries := map[string]map[string]any{
		"stack": {
			"component": "tgw/hub",
			"stack":     "tenant1-ue2-prod",
		},
		"stack-template": {
			"component": "vpc",
			"stack":     "{{ .settings.context.tenant }}-ue1-{{ .settings.context.stage }}",
		},
		"stack-with-context": {
			"component":   "vpc",
			"stack":       "tenant1-ue2-prod",
			"namespace":   "acme",
			"tenant":      "tenant1",
			"environment": "ue2",
			"stage":       "prod",
		},
		"component-only": {
			"component": "vpc",
		},
		"context-vars": {
			"component":   "component4",
			"tenant":      "tenant1",
			"environment": "ue2",
			"stage":       "staging",
		},
		"file":   {"file": "./config.json"},
		"folder": {"folder": "./modules"},
	}

	for schemaName, schemaData := range dependsOnSchemas(t) {
		for name, entry := range validEntries {
			t.Run(schemaName+"/top-level/"+name, func(t *testing.T) {
				assertSchemaValid(t, schemaData, topLevelDependsOnManifest(entry))
			})

			t.Run(schemaName+"/component/"+name, func(t *testing.T) {
				assertSchemaValid(t, schemaData, componentDependsOnManifest(entry))
			})
		}
	}
}

// TestManifestSchema_DependsOnRejectsInvalidEntries pins the constraints that make the `stack`
// addition meaningful: `additionalProperties: false` must still reject typos, and one of
// `component`/`file`/`folder` is still required.
func TestManifestSchema_DependsOnRejectsInvalidEntries(t *testing.T) {
	invalidEntries := map[string]map[string]any{
		"unknown-property": {
			"component": "vpc",
			"stacks":    "tenant1-ue2-prod",
		},
		"stack-without-target": {
			"stack": "tenant1-ue2-prod",
		},
		"empty": {},
	}

	for schemaName, schemaData := range dependsOnSchemas(t) {
		for name, entry := range invalidEntries {
			t.Run(schemaName+"/top-level/"+name, func(t *testing.T) {
				assertSchemaInvalid(t, schemaData, topLevelDependsOnManifest(entry))
			})

			t.Run(schemaName+"/component/"+name, func(t *testing.T) {
				assertSchemaInvalid(t, schemaData, componentDependsOnManifest(entry))
			})
		}
	}
}

// dependsOnDeprecationReplacement is the modern section that supersedes `settings.depends_on`.
const dependsOnDeprecationReplacement = "dependencies.components"

// TestManifestSchema_DependsOnIsMarkedDeprecated pins where the migration metadata lives.
// `settings.depends_on` is deprecated in favor of `dependencies.components` but still supported, so
// the schema must keep saying so — editors surface `deprecated` as a strikethrough, which is the
// only in-IDE signal practitioners get.
//
// The marker belongs on the `depends_on` container and nowhere else: the entry shape
// (`depends_on_manifest`) is asserted to stay unmarked, so both halves of that decision are pinned
// and neither can drift.
func TestManifestSchema_DependsOnIsMarkedDeprecated(t *testing.T) {
	for schemaName, schemaData := range dependsOnSchemas(t) {
		t.Run(schemaName+"/depends_on", func(t *testing.T) {
			def := schemaDefinition(t, schemaData, "depends_on")

			deprecated, ok := def["deprecated"].(bool)
			require.Truef(t, ok, "%s.depends_on must declare a boolean `deprecated`", schemaName)
			assert.Truef(t, deprecated, "%s.depends_on must be marked deprecated", schemaName)

			assert.Equalf(t, dependsOnDeprecationReplacement, def["x-atmos-replacement"],
				"%s.depends_on must point at the replacement section", schemaName)

			description, _ := def["description"].(string)
			assert.Containsf(t, description, dependsOnDeprecationReplacement,
				"%s.depends_on description must name the replacement for editors without `deprecated` support", schemaName)
		})

		// The entry shape is deliberately NOT deprecated. `depends_on_manifest` models a single
		// legacy entry and is reachable only through the deprecated `depends_on` container, so
		// marking it too would strike through every entry in an editor on top of the already-struck
		// container. Migration metadata belongs on the container and its call sites only.
		t.Run(schemaName+"/depends_on_manifest-not-independently-deprecated", func(t *testing.T) {
			def := schemaDefinition(t, schemaData, "depends_on_manifest")

			assert.NotContainsf(t, def, "deprecated",
				"%s.depends_on_manifest must not carry its own `deprecated` marker", schemaName)
			assert.NotContainsf(t, def, "x-atmos-replacement",
				"%s.depends_on_manifest must not carry its own replacement pointer", schemaName)
		})
	}
}

// TestManifestSchema_DependsOnCallSitesAreMarkedDeprecated checks every `depends_on` property that
// `$ref`s the deprecated definition, since editors differ in whether they follow a `$ref` for
// annotations. Both the container definition and each call site must carry the marker.
func TestManifestSchema_DependsOnCallSitesAreMarkedDeprecated(t *testing.T) {
	for schemaName, schemaData := range dependsOnSchemas(t) {
		t.Run(schemaName, func(t *testing.T) {
			var doc any
			require.NoError(t, json.Unmarshal(schemaData, &doc))

			sites := findDependsOnCallSites(doc, "")
			require.NotEmptyf(t, sites, "%s must reference #/definitions/depends_on somewhere", schemaName)

			for path, site := range sites {
				assert.Equalf(t, true, site["deprecated"], "%s: call site %s must be marked deprecated", schemaName, path)
				assert.Equalf(t, dependsOnDeprecationReplacement, site["x-atmos-replacement"],
					"%s: call site %s must point at the replacement section", schemaName, path)
			}
		})
	}
}

// findDependsOnCallSites walks a decoded schema and returns every `depends_on` property object that
// `$ref`s the `depends_on` definition, keyed by its JSON path for readable failure messages.
func findDependsOnCallSites(node any, path string) map[string]map[string]any {
	sites := map[string]map[string]any{}

	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			childPath := path + "/" + key
			if key == "depends_on" {
				if obj, ok := value.(map[string]any); ok {
					if ref, _ := obj["$ref"].(string); ref == "#/definitions/depends_on" {
						sites[childPath] = obj
					}
				}
			}
			for k, v := range findDependsOnCallSites(value, childPath) {
				sites[k] = v
			}
		}
	case []any:
		for i, value := range typed {
			for k, v := range findDependsOnCallSites(value, path+"["+strconv.Itoa(i)+"]") {
				sites[k] = v
			}
		}
	}

	return sites
}

// schemaDefinition returns a named entry from the schema's `definitions` map.
func schemaDefinition(t *testing.T, schemaData []byte, name string) map[string]any {
	t.Helper()

	var doc struct {
		Definitions map[string]map[string]any `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(schemaData, &doc))

	def, ok := doc.Definitions[name]
	require.Truef(t, ok, "schema must define %q", name)
	return def
}

func topLevelDependsOnManifest(entry map[string]any) map[string]any {
	return map[string]any{
		"settings": dependsOnSettings(entry),
	}
}

func componentDependsOnManifest(entry map[string]any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"tgw/attachment": map[string]any{
					"settings": dependsOnSettings(entry),
				},
			},
		},
	}
}

// dependsOnSettings wraps a single dependency in the map-of-descriptions shape that
// `settings.depends_on` uses (keys are arbitrary labels, not component names).
func dependsOnSettings(entry map[string]any) map[string]any {
	return map[string]any{
		"depends_on": map[string]any{
			"1": entry,
		},
	}
}
