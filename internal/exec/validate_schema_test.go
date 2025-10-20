package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filematch"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
	"go.uber.org/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/xeipuuv/gojsonschema"
)

type mockResultError struct {
	field             string
	errType           string // Renamed to avoid conflict with Type() method
	description       string
	descriptionFormat string
	value             any
	context           *gojsonschema.JsonContext
	details           gojsonschema.ErrorDetails
}

// Field returns the field name without the root context.
func (m *mockResultError) Field() string {
	return m.field
}

// SetType sets the error-type.
func (m *mockResultError) SetType(t string) {
	m.errType = t
}

// Type returns the error-type.
func (m *mockResultError) Type() string {
	return m.errType
}

// SetContext sets the JSON-context for the error.
func (m *mockResultError) SetContext(ctx *gojsonschema.JsonContext) {
	m.context = ctx
}

// Context returns the JSON-context of the error.
func (m *mockResultError) Context() *gojsonschema.JsonContext {
	return m.context
}

// SetDescription sets a description for the error.
func (m *mockResultError) SetDescription(desc string) {
	m.description = desc
}

// Description returns the description of the error.
func (m *mockResultError) Description() string {
	return m.description
}

// SetDescriptionFormat sets the format for the description.
func (m *mockResultError) SetDescriptionFormat(format string) {
	m.descriptionFormat = format
}

// DescriptionFormat returns the format for the description.
func (m *mockResultError) DescriptionFormat() string {
	return m.descriptionFormat
}

// SetValue sets the value related to the error.
func (m *mockResultError) SetValue(val any) {
	m.value = val
}

// Value returns the value related to the error.
func (m *mockResultError) Value() any {
	return m.value
}

// SetDetails sets the details specific to the error.
func (m *mockResultError) SetDetails(details gojsonschema.ErrorDetails) {
	m.details = details
}

// Details returns details about the error.
func (m *mockResultError) Details() gojsonschema.ErrorDetails {
	return m.details
}

// String returns a string representation of the error.
func (m *mockResultError) String() string {
	return m.field + ": " + m.description
}

// func (m *mockResultError) Context

func TestExecuteAtmosValidateSchemaCmd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		yamlSource    string
		customSchema  string
		mockSetup     func(*validator.MockValidator, *downloader.MockFileDownloader, *filematch.MockFileMatcherInterface)
		expectedError error
	}{
		{
			name:         "successful validation",
			yamlSource:   "atmos.yaml",
			customSchema: "atmos://schema",
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcherInterface) {
				fmi.EXPECT().MatchFiles([]string{"atmos.yaml"}).Return([]string{"atmos.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema("atmos://schema", "atmos.yaml").Return([]gojsonschema.ResultError{}, nil)
			},
			expectedError: nil,
		},
		{
			name:         "validation errors",
			yamlSource:   "atmos.yaml",
			customSchema: "atmos://schema",
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcherInterface) {
				fmi.EXPECT().MatchFiles([]string{"atmos.yaml"}).Return([]string{"atmos.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema("atmos://schema", "atmos.yaml").Return([]gojsonschema.ResultError{&mockResultError{}}, nil)
			},
			expectedError: ErrInvalidYAML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockValidator := validator.NewMockValidator(ctrl)
			mockFileDownloader := downloader.NewMockFileDownloader(ctrl)
			mockFileMatcher := filematch.NewMockFileMatcherInterface(ctrl)
			tt.mockSetup(mockValidator, mockFileDownloader, mockFileMatcher)

			av := &atmosValidatorExecutor{
				validator:      mockValidator,
				fileDownloader: mockFileDownloader,
				fileMatcher:    mockFileMatcher,
				atmosConfig: &schema.AtmosConfiguration{
					Schemas: map[string]interface{}{
						"something": schema.SchemaRegistry{
							Schema:  tt.customSchema,
							Matches: []string{tt.yamlSource},
						},
					},
				},
			}

			err := av.ExecuteAtmosValidateSchemaCmd("", "")
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
