package datafetcher

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaCloudFormationTypeDefaults checks schemas accept supported type-level defaults and reject invalid fields.
// CloudFormation has no generate section: it renders inline templates instead.
func TestSchemaCloudFormationTypeDefaults(t *testing.T) {
	schemas := map[string][]byte{"manifest": loadEmbeddedSchemaBytes(t), "stack-config": loadStackConfigSchemaBytes(t)}
	defaults := map[string]any{
		"locals":       map[string]any{"region": "us-east-1"},
		"auth":         map[string]any{},
		"dependencies": map[string]any{"components": []any{map[string]any{"component": "network"}}},
		"source":       map[string]any{"uri": "https://example.com/template.yaml"},
		"provision":    map[string]any{"default": "deploy", "targets": map[string]any{"deploy": map[string]any{"kind": "aws/cloudformation"}}},
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			for key, value := range defaults {
				t.Run(key, func(t *testing.T) {
					assertSchemaValid(t, schema, map[string]any{"aws/cloudformation": map[string]any{key: value}})
				})
			}
			assertSchemaInvalid(t, schema, map[string]any{"aws/cloudformation": map[string]any{"unknown_setting": true}})
			assertSchemaInvalid(t, schema, map[string]any{"aws/cloudformation": map[string]any{"provision": 42}})
			assertSchemaInvalid(t, schema, map[string]any{"aws/cloudformation": map[string]any{"generate": map[string]any{"template.yaml": "Resources: {}"}}})
		})
	}
}

// cloudFormationComponent wraps a component body in a stack manifest.
func cloudFormationComponent(body map[string]any) map[string]any {
	return map[string]any{"components": map[string]any{"aws/cloudformation": map[string]any{"vpc": body}}}
}

// TestSchemaCloudFormationComponentManifest checks every schema copy accepts the supported component fields
// (including path, locals and an inline stack policy body) and rejects generate.
func TestSchemaCloudFormationComponentManifest(t *testing.T) {
	schemas := map[string][]byte{"manifest": loadEmbeddedSchemaBytes(t), "stack-config": loadStackConfigSchemaBytes(t), "fixture": loadFixtureSchemaBytes(t)}
	policy := map[string]any{"Statement": []any{map[string]any{"Effect": "Allow", "Action": "Update:*", "Principal": "*", "Resource": "*"}}}
	tests := []struct {
		name  string
		body  map[string]any
		valid bool
	}{
		{name: "inline template string", body: map[string]any{"stack_name": "vpc", "template": "Resources: {}"}, valid: true},
		{name: "inline template map", body: map[string]any{"stack_name": "vpc", "template": map[string]any{"Resources": map[string]any{}}}, valid: true},
		{name: "path", body: map[string]any{"stack_name": "vpc", "path": "template.yaml"}, valid: true},
		{name: "locals", body: map[string]any{"stack_name": "vpc", "path": "template.yaml", "locals": map[string]any{"a": "b"}}, valid: true},
		{name: "stack policy file", body: map[string]any{"stack_name": "vpc", "path": "t.yaml", "stack_policy": map[string]any{"file": "policy.json"}}, valid: true},
		{name: "stack policy body string", body: map[string]any{"stack_name": "vpc", "path": "t.yaml", "stack_policy": map[string]any{"body": "{}"}}, valid: true},
		{name: "stack policy body map", body: map[string]any{"stack_name": "vpc", "path": "t.yaml", "stack_policy": map[string]any{"body": policy}}, valid: true},
		{name: "stack policy body wrong type", body: map[string]any{"stack_name": "vpc", "path": "t.yaml", "stack_policy": map[string]any{"body": 42}}},
		{name: "generate is rejected", body: map[string]any{"stack_name": "vpc", "path": "t.yaml", "generate": map[string]any{"t.yaml": "Resources: {}"}}},
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if tt.valid {
						assertSchemaValid(t, schema, cloudFormationComponent(tt.body))
						return
					}
					assertSchemaInvalid(t, schema, cloudFormationComponent(tt.body))
				})
			}
		})
	}
}

// TestSchemaCloudFormationFieldParity guards the schema copies against drifting apart: each must list the
// same CloudFormation component and type-default fields.
func TestSchemaCloudFormationFieldParity(t *testing.T) {
	schemas := map[string][]byte{"manifest": loadEmbeddedSchemaBytes(t), "stack-config": loadStackConfigSchemaBytes(t), "fixture": loadFixtureSchemaBytes(t)}
	for _, definition := range []string{"aws_cloudformation", "aws_cloudformation_component_manifest"} {
		fields := map[string][]string{}
		for name, data := range schemas {
			fields[name] = cloudFormationDefinitionFields(t, data, definition)
			require.NotEmpty(t, fields[name], "%s must define %s properties", name, definition)
		}
		assert.Equal(t, fields["manifest"], fields["stack-config"], "%s: stack-config differs from manifest", definition)
		assert.Equal(t, fields["manifest"], fields["fixture"], "%s: test fixture differs from manifest", definition)
	}
}

// cloudFormationDefinitionFields returns the sorted property names of a definition's object variant.
func cloudFormationDefinitionFields(t *testing.T, data []byte, definition string) []string {
	t.Helper()
	var doc struct {
		Definitions map[string]struct {
			OneOf []struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"oneOf"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	var fields []string
	for _, variant := range doc.Definitions[definition].OneOf {
		for field := range variant.Properties {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}
