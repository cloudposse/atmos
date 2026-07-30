package datafetcher

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// The `settings.depends_on` entry schema (`depends_on_manifest`) sets `additionalProperties: false`,
// so every key Atmos actually honors must be modeled or IDEs flag valid manifests as invalid.
// `stack` was missing (https://github.com/cloudposse/atmos/issues/2104) even though it is documented,
// parsed by `internal/exec/dependency_parser.go`, and carried on `schema.Dependent`.

// Compile-time sentinel: if `Stack` is ever renamed or removed from `schema.Dependent`, this fails
// the build so the schema property below is revisited alongside it.
var _ = schema.Dependent{Stack: "tenant1-ue2-prod"}

// dependsOnSchemas returns every schema copy that models `depends_on_manifest`. They are separate
// files that drift independently, so each one is asserted.
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
