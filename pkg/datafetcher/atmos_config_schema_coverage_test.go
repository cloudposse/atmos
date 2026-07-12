package datafetcher

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file is the "you can't forget the atmos.yaml schema" ratchet. Every user-authored
// top-level key on AtmosConfiguration must be modeled in the atmos-config JSON Schema, or
// editors and `atmos validate schema config` silently reject — or fail to validate — the new
// capability.

const (
	atmosConfigStructRelPath     = "../schema/schema.go"
	embeddedAtmosConfigSchemaRel = "schema/config/global/1.0.json"
	websiteAtmosConfigSchemaRel  = "../../website/static/schemas/atmos/atmos-config/1.0/atmos-config.json"
)

// nonUserAtmosConfigFields are AtmosConfiguration struct fields that are not authored in
// atmos.yaml (runtime-derived, embedded defaults, or transient). They are exempt from the
// schema-coverage requirement, but listing them here is mandatory: a new yaml-tagged field
// absent from BOTH expectedUserAtmosConfigFields (via struct parse) AND this set must still
// be classified.
var nonUserAtmosConfigFields = map[string]struct{}{
	"initialized":                     {},
	"default":                         {},
	"basePathAbsolute":                {},
	"stacksBaseAbsolutePath":          {},
	"includeStackAbsolutePaths":       {},
	"excludeStackAbsolutePaths":       {},
	"terraformDirAbsolutePath":        {},
	"helmfileDirAbsolutePath":         {},
	"packerDirAbsolutePath":           {},
	"ansibleDirAbsolutePath":          {},
	"kubernetesDirAbsolutePath":       {},
	"helmDirAbsolutePath":             {},
	"stackConfigFilesRelativePaths":   {},
	"stackConfigFilesAbsolutePaths":   {},
	"stackType":                       {},
	"stores_registry":                 {},
	"cli_config_path":                 {},
}

// knownAtmosConfigSchemaGaps tracks top-level keys that SHOULD be in the atmos-config schema
// but currently are not. Deliberate, reviewed technical debt — NOT an escape hatch for new keys.
var knownAtmosConfigSchemaGaps = map[string]struct{}{}

// TestAtmosConfigSchemaCoversUserFields asserts every user-authored AtmosConfiguration yaml key
// is modeled in both the embedded and website atmos-config JSON schemas.
func TestAtmosConfigSchemaCoversUserFields(t *testing.T) {
	userFields := parseAtmosConfigurationYAMLKeys(t)
	require.NotEmpty(t, userFields, "expected user-authored AtmosConfiguration yaml keys")

	schemas := map[string]map[string]any{
		"embedded": loadAtmosConfigSchemaFile(t, embeddedAtmosConfigSchemaRel),
		"website":  loadAtmosConfigSchemaFile(t, websiteAtmosConfigSchemaRel),
	}

	for schemaName, schema := range schemas {
		props := topLevelProps(t, schema)
		for field := range userFields {
			assertAtmosConfigFieldCovered(t, props, schemaName, field)
		}
	}
}

// TestAtmosConfigSchemaMetadata asserts both schema copies declare the expected identity.
func TestAtmosConfigSchemaMetadata(t *testing.T) {
	for _, path := range []string{embeddedAtmosConfigSchemaRel, websiteAtmosConfigSchemaRel} {
		schema := loadAtmosConfigSchemaFile(t, path)
		require.Equal(t, "https://json.schemastore.org/atmos-config.json", schema["$id"])
		require.Contains(t, schema["title"], "Atmos CLI configuration")
	}
}

func assertAtmosConfigFieldCovered(t *testing.T, props map[string]struct{}, schemaName, field string) {
	t.Helper()
	_, present := props[field]
	gapKey := schemaName + ":" + field
	_, isGap := knownAtmosConfigSchemaGaps[gapKey]

	if isGap {
		require.Falsef(t, present,
			"%q is present in the %s atmos-config schema but is still listed in knownAtmosConfigSchemaGaps — remove the %q entry",
			field, schemaName, gapKey)
		return
	}
	require.Truef(t, present,
		"atmos.yaml key %q is missing from the %s atmos-config schema.\n"+
			"Add a %q property to the schema, or — only if this is reviewed, intentional debt — add %q to knownAtmosConfigSchemaGaps.",
		field, schemaName, field, gapKey)
}

// parseAtmosConfigurationYAMLKeys AST-parses pkg/schema/schema.go and returns yaml tag names
// for exported fields on AtmosConfiguration, excluding yaml:"-" and runtime-only keys.
func parseAtmosConfigurationYAMLKeys(t *testing.T) map[string]struct{} {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, atmosConfigStructRelPath, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", atmosConfigStructRelPath)

	out := map[string]struct{}{}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "AtmosConfiguration" {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				yamlKey := structFieldYAMLKey(field)
				if yamlKey == "" || yamlKey == "-" {
					continue
				}
				if _, excluded := nonUserAtmosConfigFields[yamlKey]; excluded {
					continue
				}
				out[yamlKey] = struct{}{}
			}
		}
	}
	require.NotEmpty(t, out, "expected to find AtmosConfiguration in %s", atmosConfigStructRelPath)
	return out
}

func structFieldYAMLKey(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	tag := strings.Trim(field.Tag.Value, "`")
	for _, part := range strings.Split(tag, " ") {
		if !strings.HasPrefix(part, "yaml:") {
			continue
		}
		value := strings.TrimPrefix(part, "yaml:")
		value = strings.Trim(value, `"`)
		if idx := strings.Index(value, ","); idx >= 0 {
			value = value[:idx]
		}
		return value
	}
	return ""
}

func loadAtmosConfigSchemaFile(t *testing.T, relPath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(relPath))
	require.NoErrorf(t, err, "failed to read atmos-config schema at %s", relPath)
	var schema map[string]any
	require.NoErrorf(t, json.Unmarshal(data, &schema), "failed to parse atmos-config schema at %s", relPath)
	return schema
}
