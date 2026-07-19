package manifest

import (
	"encoding/json"
	"fmt"
	"strings"

	invopop "github.com/invopop/jsonschema"
	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
)

// jsonSchemaDraft is the JSON Schema dialect used for generated manifest schemas.
const jsonSchemaDraft = "https://json-schema.org/draft/2020-12/schema"

// generateEnvelopeSchema builds the complete JSON Schema for a manifest kind:
// a Kubernetes-style envelope with constant apiVersion/kind, the shared
// Metadata schema, and the spec schema reflected from the registered
// prototype struct.
func generateEnvelopeSchema(kind, apiVersion string, specPrototype any) (string, error) {
	specSchema, err := reflectToMap(specPrototype)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrManifestSchemaGenerate).
			WithCause(err).
			WithExplanationf("Failed to reflect a JSON Schema for the `%s` spec", kind).
			WithContext("kind", kind).
			Err()
	}

	metadataSchema, err := reflectToMap(&Metadata{})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrManifestSchemaGenerate).
			WithCause(err).
			WithExplanationf("Failed to reflect the metadata JSON Schema for `%s`", kind).
			WithContext("kind", kind).
			Err()
	}

	envelope := map[string]any{
		"$schema":              jsonSchemaDraft,
		"$id":                  schemaID(kind),
		"title":                fmt.Sprintf("Atmos %s manifest", kind),
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"apiVersion", "kind", "metadata"},
		"properties": map[string]any{
			"apiVersion": map[string]any{
				"const":       apiVersion,
				"description": "Atmos manifest API version",
			},
			"kind": map[string]any{
				"const":       kind,
				"description": "Atmos manifest kind",
			},
			"metadata": metadataSchema,
			"spec":     specSchema,
		},
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", errUtils.Build(errUtils.ErrManifestSchemaGenerate).
			WithCause(err).
			WithContext("kind", kind).
			Err()
	}
	return string(data), nil
}

// reflectToMap reflects a Go value into an inline JSON Schema represented as
// a generic map, with document-level keywords stripped so the result can be
// embedded as a subschema.
func reflectToMap(prototype any) (map[string]any, error) {
	reflector := &invopop.Reflector{
		// Inline all definitions so the result is self-contained when
		// embedded as a subschema.
		DoNotReference: true,
		// Do not emit an $id derived from the Go package path.
		Anonymous: true,
	}

	reflected := reflector.Reflect(prototype)

	raw, err := json.Marshal(reflected)
	if err != nil {
		return nil, fmt.Errorf("marshal reflected schema: %w", err)
	}

	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return nil, fmt.Errorf("unmarshal reflected schema: %w", err)
	}

	// Strip document-level keywords; the envelope provides them.
	delete(asMap, "$schema")
	delete(asMap, "$id")

	return asMap, nil
}

// schemaID derives a stable schema $id URL for a manifest kind.
func schemaID(kind string) string {
	return fmt.Sprintf("https://atmos.tools/schemas/%s.json", strings.ToLower(kind))
}

// compileSchema compiles a generated JSON Schema for validation.
func compileSchema(kind, schemaJSON string) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020

	resource := schemaID(kind)
	if err := compiler.AddResource(resource, strings.NewReader(schemaJSON)); err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestSchemaGenerate).
			WithCause(err).
			WithExplanationf("Generated JSON Schema for `%s` could not be registered", kind).
			WithContext("kind", kind).
			Err()
	}

	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestSchemaGenerate).
			WithCause(err).
			WithExplanationf("Generated JSON Schema for `%s` does not compile", kind).
			WithContext("kind", kind).
			Err()
	}
	return compiled, nil
}
