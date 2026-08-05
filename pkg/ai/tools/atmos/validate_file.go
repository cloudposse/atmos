package atmos

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	goyaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/filematch"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/validator"
)

// arrayIndexSegment matches a gojsonschema field-path array-index segment
// (e.g. ".0") so it can be rewritten into the bracket-index form
// (e.g. "[0]") pkg/utils.GetYAMLPosition expects.
var arrayIndexSegment = regexp.MustCompile(`\.(\d+)`)

// paramFilePath is the file_path parameter for atmos_validate_file. The
// schema_path parameter reuses paramSchemaPath, already declared in
// validate_component.go.
const paramFilePath = "file_path"

// ValidateFileTool validates a single YAML file against a JSON Schema and
// reports each error's line number, without requiring an LSP server.
type ValidateFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewValidateFileTool creates a new validate file tool.
func NewValidateFileTool(atmosConfig *schema.AtmosConfiguration) *ValidateFileTool {
	return &ValidateFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ValidateFileTool) Name() string {
	return "atmos_validate_file"
}

// Description returns the tool description.
func (t *ValidateFileTool) Description() string {
	return "Validate a single YAML file against a JSON Schema and report each error with its line number in " +
		"the file. Unlike atmos_stack_config_get-style tools, this needs no LSP server configured. When " +
		"schema_path is omitted, the schema is auto-resolved from atmos.yaml's schemas registry by matching " +
		"the file against each entry's glob patterns. Read-only."
}

// Parameters returns the tool parameters.
func (t *ValidateFileTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramFilePath,
			Description: "Path to the YAML file to validate, relative to the project base path or absolute.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name: paramSchemaPath,
			Description: "Path or URI to the JSON Schema to validate against. When omitted, auto-resolved " +
				"from atmos.yaml's schemas registry by matching the file against each entry's glob patterns.",
			Type:     tools.ParamTypeString,
			Required: false,
		},
	}
}

// Execute validates the file and reports errors with line numbers.
func (t *ValidateFileTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	filePath, err := extractRequiredStringParam(params, paramFilePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	schemaPath, _ := params[paramSchemaPath].(string)

	absPath := filePath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.atmosConfig.BasePath, filePath)
	}
	content, err := readAndValidateFile(absPath, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if schemaPath == "" {
		resolved, found := resolveSchemaForFile(t.atmosConfig, absPath)
		if !found {
			err := fmt.Errorf("%w: %s", errUtils.ErrAINoSchemaForFile, filePath)
			return &tools.Result{Success: false, Error: err}, err
		}
		schemaPath = resolved
	}

	validationErrors, err := validator.NewYAMLSchemaValidator(t.atmosConfig).ValidateYAMLSchema(schemaPath, absPath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildValidateFileResult(filePath, schemaPath, content, validationErrors), nil
}

// validateFileFinding is one schema-validation error, enriched with the
// source line/column it corresponds to.
type validateFileFinding struct {
	Field       string `json:"field"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
}

// buildValidateFileResult formats the validator's findings into a tools.Result.
func buildValidateFileResult(filePath, schemaPath string, content []byte, validationErrors []gojsonschema.ResultError) *tools.Result {
	if len(validationErrors) == 0 {
		return &tools.Result{
			Success: true,
			Output:  fmt.Sprintf("✅ %s validates against %s", filePath, schemaPath),
			Data: map[string]interface{}{
				paramFilePath:   filePath,
				paramSchemaPath: schemaPath,
				"valid":         true,
			},
		}
	}

	positions := extractFilePositions(content)

	var out strings.Builder
	fmt.Fprintf(&out, "❌ %s failed validation against %s (%d error(s)):\n\n", filePath, schemaPath, len(validationErrors))

	findings := make([]validateFileFinding, 0, len(validationErrors))
	for _, e := range validationErrors {
		pos := resolveFindingPosition(positions, e.Field())

		finding := validateFileFinding{
			Field:       e.Field(),
			Type:        e.Type(),
			Description: e.Description(),
			Line:        pos.Line,
			Column:      pos.Column,
		}
		findings = append(findings, finding)

		if finding.Line > 0 {
			fmt.Fprintf(&out, "  - %s:%d: %s (%s)\n", filePath, finding.Line, finding.Description, finding.Field)
		} else {
			fmt.Fprintf(&out, "  - %s: %s (%s)\n", filePath, finding.Description, finding.Field)
		}
	}

	return &tools.Result{
		Success: false,
		Output:  out.String(),
		Data: map[string]interface{}{
			paramFilePath:   filePath,
			paramSchemaPath: schemaPath,
			"valid":         false,
			"findings":      findings,
		},
	}
}

// extractFilePositions parses content into a position map keyed by the same
// bracket-indexed dot-path format pkg/utils.GetYAMLPosition expects. Returns
// an empty map (never nil-panics on lookup) if the content fails to parse --
// findings still surface without line numbers rather than erroring out.
func extractFilePositions(content []byte) u.PositionMap {
	var node goyaml.Node
	if err := goyaml.Unmarshal(content, &node); err != nil {
		return u.PositionMap{}
	}
	return u.ExtractYAMLPositions(&node, true)
}

// resolveFindingPosition maps a gojsonschema field path to a source
// position. The underlying library reports the document root as the literal
// string "(root)" (its TrimPrefix only strips the "(root)." form, so a bare
// root-level error -- e.g. a missing required top-level property -- keeps
// the raw "(root)" value) rather than an empty path; that case is mapped to
// the top of the file. Array indices are reported as plain dot-integers
// (e.g. "a.0.b"), translated here into the bracket-index form (e.g.
// "a[0].b") pkg/utils' position map is keyed by.
func resolveFindingPosition(positions u.PositionMap, field string) u.Position {
	if field == "(root)" {
		return u.Position{Line: 1, Column: 1}
	}
	yamlPath := arrayIndexSegment.ReplaceAllString(field, `[$1]`)
	return u.GetYAMLPosition(positions, yamlPath)
}

// resolveSchemaForFile finds the first schemas registry entry (from
// atmosConfig.Schemas) whose glob patterns match absPath, mirroring
// internal/exec/validate_schema.go's prepareSchemaValue/shouldSkipSchema
// defaulting rules via only the public schema.AtmosConfiguration API.
func resolveSchemaForFile(atmosConfig *schema.AtmosConfiguration, absPath string) (string, bool) {
	matcher := filematch.NewGlobMatcher()
	for key := range atmosConfig.Schemas {
		registry, ok := effectiveSchemaRegistry(atmosConfig, key)
		if !ok {
			continue
		}
		if schemaRegistryMatchesFile(matcher, registry, absPath) {
			return registry.Schema, true
		}
	}
	return "", false
}

// effectiveSchemaRegistry resolves key's registry entry with the same
// schema/manifest/matches defaulting rules internal/exec/validate_schema.go
// applies, reporting false when the entry (after defaulting) isn't usable
// for validation.
func effectiveSchemaRegistry(atmosConfig *schema.AtmosConfiguration, key string) (schema.SchemaRegistry, bool) {
	if key == "cue" || key == "opa" || key == "jsonschema" {
		return schema.SchemaRegistry{}, false
	}

	registry := atmosConfig.GetSchemaRegistry(key)
	registry.Schema = defaultSchemaValue(registry, key)
	registry.Matches = defaultSchemaMatches(registry, key)
	if registry.Schema == "" || len(registry.Matches) == 0 {
		return schema.SchemaRegistry{}, false
	}
	return registry, true
}

// defaultSchemaValue applies validate_schema.go's schema/manifest defaulting:
// an explicit schema wins; otherwise fall back to manifest, then to the
// built-in atmos:// schema for key.
func defaultSchemaValue(registry schema.SchemaRegistry, key string) string {
	if registry.Schema != "" {
		return registry.Schema
	}
	if registry.Manifest != "" {
		return registry.Manifest
	}
	return fmt.Sprintf("atmos://schema/%s/manifest/1.0", key)
}

// defaultSchemaMatches applies validate_schema.go's default glob for the
// built-in "atmos" schema key when no matches are configured.
func defaultSchemaMatches(registry schema.SchemaRegistry, key string) []string {
	if len(registry.Matches) == 0 && key == "atmos" {
		return []string{"atmos.yaml", "atmos.yml"}
	}
	return registry.Matches
}

// schemaRegistryMatchesFile reports whether absPath is among the files
// registry.Matches resolves to.
func schemaRegistryMatchesFile(matcher filematch.FileMatcher, registry schema.SchemaRegistry, absPath string) bool {
	files, err := matcher.MatchFiles(registry.Matches)
	if err != nil {
		return false
	}
	for _, f := range files {
		if samePath(f, absPath) {
			return true
		}
	}
	return false
}

// samePath compares two paths by their absolute, cleaned form.
func samePath(a, b string) bool {
	ca, errA := filepath.Abs(a)
	cb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return filepath.Clean(ca) == filepath.Clean(cb)
}

// RequiresPermission returns true if this tool needs permission.
func (t *ValidateFileTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ValidateFileTool) IsRestricted() bool {
	return false
}
