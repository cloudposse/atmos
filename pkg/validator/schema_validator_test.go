package validator

import (
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestValidateYAMLSchema tests the ValidateYAMLSchema method of the yamlSchemaValidator
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
		},
		{
			name:         "Schema fetch error",
			schemaSource: "schema.json",
			yamlSource:   "data.yaml",
			fetcherErr:   errors.New("failed to fetch schema"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			// Create mock data fetcher
			mockFetcher := NewMockDataFetcher(ctrl)
			atmosConfig := &schema.AtmosConfiguration{}
			// Configure mock behavior
			if tt.fetcherErr != nil {
				mockFetcher.EXPECT().GetData(atmosConfig, tt.schemaSource).
					Return(nil, tt.fetcherErr)
			} else {
				mockFetcher.EXPECT().GetData(atmosConfig, tt.schemaSource).
					Return(tt.schemaData, nil)
				mockFetcher.EXPECT().GetData(atmosConfig, tt.yamlSource).
					Return(tt.yamlData, nil)
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
