package validator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"

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
					Return(nil, yaml.ErrExceededMaxDepth) // Return nil data to trigger YAML unmarshal error
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

func TestValidateYAMLSchema_ComponentTypeAliases(t *testing.T) {
	schemaData, err := os.ReadFile(filepath.Join("..", "datafetcher", "schema", "atmos", "manifest", "1.0.json"))
	assert.NoError(t, err)

	tests := []struct {
		name     string
		yamlData []byte
	}{
		{
			name: "canonical section aliases scalar",
			yamlData: []byte(`
components:
  terraform:
    aliases: opentofu
`),
		},
		{
			name: "canonical section aliases list",
			yamlData: []byte(`
components:
  terraform:
    aliases: [opentofu, tofu]
`),
		},
		{
			name: "alias envelope remains schema-compatible",
			yamlData: []byte(`
components:
  opentofu:
    vpc:
      vars:
        name: vpc
`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFetcher := datafetcher.NewMockDataFetcher(ctrl)
			mockFetcher.EXPECT().GetData("data.yaml").Return(tt.yamlData, nil)
			mockFetcher.EXPECT().GetData("schema.json").Return(schemaData, nil)

			v := &yamlSchemaValidator{
				atmosConfig: &schema.AtmosConfiguration{},
				dataFetcher: mockFetcher,
			}

			resultErrors, err := v.ValidateYAMLSchema("schema.json", "data.yaml")
			assert.NoError(t, err)
			assert.Empty(t, resultErrors)
		})
	}
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
