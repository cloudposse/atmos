package validator

import (
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/goccy/go-yaml"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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
		key            string
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
				mockFetcher.EXPECT().GetData(gomock.Any(), "data.yaml").
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
		{
			name:         "Valid YAML against schema with key",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			schemaData: []byte(`{
		        "type": "object",
		        "properties": {
		            "name": {"type": "string"}
		        },
		        "required": ["name"]
		    }`),
			yamlData:       []byte("data:\n  name: test"),
			expectedErrors: 0,
			wantErr:        false,
			key:            "data",
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
				mockFetcher.EXPECT().GetData(atmosConfig, tt.yamlSource).
					Return(tt.yamlData, nil)
				mockFetcher.EXPECT().GetData(atmosConfig, tt.schemaSource).
					Return(tt.schemaData, nil)
			}

			// Create validator with mock fetcher
			v := &yamlSchemaValidator{
				atmosConfig: atmosConfig,
				dataFetcher: mockFetcher,
			}

			// Execute the method
			resultErrors, err := v.ValidateYAMLSchema(tt.schemaSource, tt.yamlSource, tt.key)

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
	assert.Error(t, err, ErrSchemaNotFound)
}
