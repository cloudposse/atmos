package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/validator"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/xeipuuv/gojsonschema"
)

func TestExecuteAtmosValidateSchemaCmd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		yamlSource    string
		customSchema  string
		mockSetup     func(*validator.MockValidator, *downloader.MockFileDownloader)
		expectedError string
	}{
		{
			name:         "successful validation",
			yamlSource:   "atmos.yaml",
			customSchema: "atmos://schema",
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader) {
				mfd.EXPECT().FetchAndAutoParse("atmos.yaml").Return(map[string]interface{}{"schema": "atmos://schema"}, nil)
				mv.EXPECT().ValidateYAMLSchema("atmos://schema", "atmos.yaml").Return([]gojsonschema.ResultError{}, nil)
			},
			expectedError: "",
		},
		{
			name:         "validation errors",
			yamlSource:   "atmos.yaml",
			customSchema: "atmos://schema",
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader) {
				mfd.EXPECT().FetchAndAutoParse("atmos.yaml").Return(map[string]interface{}{"schema": "atmos://schema"}, nil)
				mv.EXPECT().ValidateYAMLSchema("atmos://schema", "atmos.yaml").Return([]gojsonschema.ResultError{&gojsonschema.AdditionalPropertyNotAllowedError{}}, nil)
			},
			expectedError: "invalid field",
		},
		{
			name:         "missing schema",
			yamlSource:   "invalid.yaml",
			customSchema: "",
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader) {
				mfd.EXPECT().FetchAndAutoParse("invalid.yaml").Return(map[string]interface{}{}, nil)
			},
			expectedError: "schema not found for invalid.yaml file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockValidator := validator.NewMockValidator(ctrl)
			mockFileDownloader := downloader.NewMockFileDownloader(ctrl)

			tt.mockSetup(mockValidator, mockFileDownloader)

			// Mock the Exit function
			mockExit := func(code int) {
				if code != 0 {
					assert.Equal(t, 1, code)
				}
			}

			av := &atmosValidatorExecuter{
				validator:      mockValidator,
				fileDownloader: mockFileDownloader,
				Exit:           mockExit,
			}

			err := av.ExecuteAtmosValidateSchemaCmd(tt.yamlSource, tt.customSchema)
			if tt.expectedError != "" {
				assert.NoError(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
