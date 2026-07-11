package validator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	goccyyaml "github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/schema"
)

var ErrFailedToFetchSchema = errors.New("failed to fetch schema")

// TestValidateYAMLSchema tests the ValidateYAMLSchema method of the yamlSchemaValidator.
func TestValidateYAMLSchema(t *testing.T) {
	tests := []struct {
		name           string
		schemaSource   string
		yamlSource     string
		schemaData     []byte
		yamlData       []byte
		fetcherErr     error
		expectedErrors int
		wantErr        bool
		setMockExpect  func(*datafetcher.MockDataFetcher)
	}{
		{
			name:         "Valid YAML against schema",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			schemaData: []byte(`{
                "type": "object",
                "properties": {
                    "name": {"type": "string"}
                },
                "required": ["name"]
            }`),
			yamlData:       []byte("name: test"),
			expectedErrors: 0,
			wantErr:        false,
		},
		{
			name:         "Invalid YAML against schema",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			schemaData: []byte(`{
		        "type": "object",
		        "properties": {
		            "name": {"type": "string"}
		        },
		        "required": ["name"]
		    }`),
			yamlData:       []byte("age: 25"),
			expectedErrors: 1, // Missing required property "name"
			wantErr:        false,
		},
		{
			name:         "Valid YAML include tag against schema",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			schemaData: []byte(`{
				"type": "object",
				"properties": {
					"env": {
						"oneOf": [
							{"type": "string", "pattern": "^!include"},
							{"type": "object", "additionalProperties": true}
						]
					}
				},
				"required": ["env"]
			}`),
			yamlData:       []byte("env: !include .env"),
			expectedErrors: 0,
			wantErr:        false,
		},
		{
			name:         "Invalid YAML format",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			schemaData:   []byte(`{"type": "object"}`),
			yamlData: []byte(`
key: value
: malformed
`), // Invalid YAML
			wantErr: true,
			setMockExpect: func(mockFetcher *datafetcher.MockDataFetcher) {
				mockFetcher.EXPECT().GetData("data.yaml").
					Return(nil, goccyyaml.ErrExceededMaxDepth) // Return nil data to trigger YAML unmarshal error
			},
		},
		{
			name:         "Schema fetch error",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			fetcherErr:   ErrFailedToFetchSchema,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			// Create mock data fetcher
			mockFetcher := datafetcher.NewMockDataFetcher(ctrl)
			atmosConfig := &schema.AtmosConfiguration{}
			// Configure mock behavior
			if tt.setMockExpect != nil {
				tt.setMockExpect(mockFetcher)
			} else {
				mockFetcher.EXPECT().GetData(tt.yamlSource).
					Return(tt.yamlData, nil)
				mockFetcher.EXPECT().GetData(tt.schemaSource).
					Return(tt.schemaData, nil)
			}

			// Create validator with mock fetcher
			v := &yamlSchemaValidator{
				atmosConfig: atmosConfig,
				dataFetcher: mockFetcher,
			}

			// Execute the method
			resultErrors, err := v.ValidateYAMLSchema(tt.schemaSource, tt.yamlSource)

			// Assertions
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resultErrors)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedErrors, len(resultErrors))
			}
		})
	}
}

func TestValidateYAMLSchema_AtmosManifestEnvInclude(t *testing.T) {
	schemaData, err := os.ReadFile("../../tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json")
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := datafetcher.NewMockDataFetcher(ctrl)
	mockFetcher.EXPECT().GetData("stack.yaml").
		Return([]byte("env: !include .env"), nil)
	mockFetcher.EXPECT().GetData("atmos-manifest.json").
		Return(schemaData, nil)

	v := &yamlSchemaValidator{
		atmosConfig: &schema.AtmosConfiguration{},
		dataFetcher: mockFetcher,
	}

	resultErrors, err := v.ValidateYAMLSchema("atmos-manifest.json", "stack.yaml")
	require.NoError(t, err)
	assert.Empty(t, resultErrors)
}

func TestYAMLToJSONPreservesCustomTagsAndNodeTypes(t *testing.T) {
	data, err := yamlToJSON([]byte(`defaults: &defaults
  name: test
  enabled: true
copy: *defaults
list:
  - !include .env
  - 3
empty:
`))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, map[string]any{
		"name":    "test",
		"enabled": true,
	}, decoded["defaults"])
	assert.Equal(t, decoded["defaults"], decoded["copy"])
	assert.Equal(t, []any{"!include .env", float64(3)}, decoded["list"])
	assert.Nil(t, decoded["empty"])
}

func TestYAMLNodeToInterfaceEdges(t *testing.T) {
	assert.Nil(t, yamlNodeToInterface(nil))
	assert.Nil(t, yamlNodeToInterface(&yamlv3.Node{Kind: yamlv3.DocumentNode}))
	assert.Nil(t, yamlNodeToInterface(&yamlv3.Node{Kind: 999}))

	assert.True(t, isCustomYAMLTag("!include"))
	assert.False(t, isCustomYAMLTag(""))
	assert.False(t, isCustomYAMLTag("!!str"))
}

func TestSchemaExtractor_Success(t *testing.T) {
	// Create validator with mock fetcher
	v := &yamlSchemaValidator{}
	// Execute the method
	schemaSource, err := v.getSchemaSourceFromYAML([]byte(`{"schema": "schema.json"}`))
	assert.NoError(t, err)
	assert.Equal(t, "schema.json", schemaSource)
}

func TestSchemaExtractor_Failure(t *testing.T) {
	// Create validator with mock fetcher
	v := &yamlSchemaValidator{}
	// Execute the method
	_, err := v.getSchemaSourceFromYAML([]byte(`{}`))
	assert.ErrorIs(t, err, ErrSchemaNotFound)
}

func TestValidateYAMLSchema_AtmosConfigSchema(t *testing.T) {
	v := NewYAMLSchemaValidator(&schema.AtmosConfiguration{})
	schemaSource := "atmos://schema/config/global/1.0"

	validConfig, err := os.ReadFile("../../tests/fixtures/scenarios/invalid-config-schema/atmos.yaml")
	require.NoError(t, err)

	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid-atmos.yaml")
	require.NoError(t, os.WriteFile(validPath, []byte(`base_path: "."
stacks:
  base_path: stacks
  included_paths:
    - orgs/**/*
components:
  terraform:
    base_path: components/terraform
`), 0o644))

	invalidPath := filepath.Join(tmpDir, "invalid-atmos.yaml")
	require.NoError(t, os.WriteFile(invalidPath, validConfig, 0o644))

	validErrors, err := v.ValidateYAMLSchema(schemaSource, validPath)
	require.NoError(t, err)
	assert.Empty(t, validErrors)

	invalidErrors, err := v.ValidateYAMLSchema(schemaSource, invalidPath)
	require.NoError(t, err)
	assert.NotEmpty(t, invalidErrors)
}
