package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filematch"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
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
		sourceKey     string
		schemas       map[string]any
		mockSetup     func(*validator.MockValidator, *downloader.MockFileDownloader, *filematch.MockFileMatcher)
		expectedError error
	}{
		{
			name: "successful validation",
			schemas: map[string]any{
				"something": schema.SchemaRegistry{Schema: "atmos://schema", Matches: []string{"atmos.yaml"}},
			},
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcher) {
				fmi.EXPECT().MatchFiles([]string{"atmos.yaml"}).Return([]string{"atmos.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema("atmos://schema", "atmos.yaml").Return([]gojsonschema.ResultError{}, nil)
				// The built-in config entry is seeded alongside configured schemas.
				fmi.EXPECT().MatchFiles(builtinConfigSchemaMatches()).Return([]string{}, nil)
			},
			expectedError: nil,
		},
		{
			name: "validation errors",
			schemas: map[string]any{
				"something": schema.SchemaRegistry{Schema: "atmos://schema", Matches: []string{"atmos.yaml"}},
			},
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcher) {
				fmi.EXPECT().MatchFiles([]string{"atmos.yaml"}).Return([]string{"atmos.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema("atmos://schema", "atmos.yaml").Return([]gojsonschema.ResultError{&mockResultError{}}, nil)
				fmi.EXPECT().MatchFiles(builtinConfigSchemaMatches()).Return([]string{}, nil)
			},
			expectedError: ErrInvalidYAML,
		},
		{
			name:    "built-in config entry validates atmos.yaml by default",
			schemas: nil,
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcher) {
				fmi.EXPECT().MatchFiles(builtinConfigSchemaMatches()).Return([]string{"atmos.yaml", ".atmos.d/extra.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema(configSchemaSource, "atmos.yaml").Return([]gojsonschema.ResultError{}, nil)
				mv.EXPECT().ValidateYAMLSchema(configSchemaSource, ".atmos.d/extra.yaml").Return([]gojsonschema.ResultError{}, nil)
			},
			expectedError: nil,
		},
		{
			name:      "source key config targets only the built-in entry",
			sourceKey: "config",
			schemas: map[string]any{
				"something": schema.SchemaRegistry{Schema: "atmos://schema", Matches: []string{"atmos.yaml"}},
			},
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcher) {
				fmi.EXPECT().MatchFiles(builtinConfigSchemaMatches()).Return([]string{"atmos.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema(configSchemaSource, "atmos.yaml").Return([]gojsonschema.ResultError{}, nil)
			},
			expectedError: nil,
		},
		{
			name: "user-configured config entry overrides the built-in defaults",
			schemas: map[string]any{
				"config": schema.SchemaRegistry{Schema: "https://example.com/custom.json", Matches: []string{"conf/atmos.yaml"}},
			},
			mockSetup: func(mv *validator.MockValidator, mfd *downloader.MockFileDownloader, fmi *filematch.MockFileMatcher) {
				fmi.EXPECT().MatchFiles([]string{"conf/atmos.yaml"}).Return([]string{"conf/atmos.yaml"}, nil)
				mv.EXPECT().ValidateYAMLSchema("https://example.com/custom.json", "conf/atmos.yaml").Return([]gojsonschema.ResultError{}, nil)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockValidator := validator.NewMockValidator(ctrl)
			mockFileDownloader := downloader.NewMockFileDownloader(ctrl)
			mockFileMatcher := filematch.NewMockFileMatcher(ctrl)
			tt.mockSetup(mockValidator, mockFileDownloader, mockFileMatcher)

			av := &atmosValidatorExecutor{
				validator:      mockValidator,
				fileDownloader: mockFileDownloader,
				fileMatcher:    mockFileMatcher,
				atmosConfig:    &schema.AtmosConfiguration{Schemas: tt.schemas},
			}

			err := av.ExecuteAtmosValidateSchemaCmd(tt.sourceKey, "")
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateAtmosYamlEndToEnd validates a real atmos.yaml fixture against the
// embedded generated schema through the full validator stack (data fetcher +
// gojsonschema), proving the atmos:// source and the 2020-12 document work with
// the shipping validation engine.
func TestValidateAtmosYamlEndToEnd(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fixture := filepath.Join(cwd, "..", "..", "examples", "demo-stacks", "atmos.yaml")
	require.FileExists(t, fixture)

	yamlValidator := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})

	validationErrors, err := yamlValidator.ValidateYAMLSchema("atmos://schema/atmos/config/1.0", fixture)
	require.NoError(t, err)
	assert.Empty(t, validationErrors, "examples/demo-stacks/atmos.yaml must validate against the embedded config schema")
}

// TestValidateAtmosYamlEndToEndRejectsInvalid confirms the embedded schema
// actually catches type errors (the negative path for the recovery above).
func TestValidateAtmosYamlEndToEndRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(invalid, []byte("logs:\n  level: 42\n  file: [not, a, string]\n"), 0o644))

	yamlValidator := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})

	validationErrors, err := yamlValidator.ValidateYAMLSchema("atmos://schema/atmos/config/1.0", invalid)
	require.NoError(t, err)
	assert.NotEmpty(t, validationErrors, "a mistyped logs section must fail validation against the embedded config schema")
}

// TestDisplayPath verifies user-facing validation output never leaks
// machine-specific absolute paths for files inside the working directory.
func TestDisplayPath(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "file inside cwd becomes relative",
			file: filepath.Join(cwd, "config.yaml"),
			want: "config.yaml",
		},
		{
			name: "nested file inside cwd becomes relative",
			file: filepath.Join(cwd, "stacks", "dev.yaml"),
			want: filepath.Join("stacks", "dev.yaml"),
		},
		{
			name: "file outside cwd stays absolute",
			file: filepath.Join(filepath.Dir(cwd), "elsewhere", "x.yaml"),
			want: filepath.Join(filepath.Dir(cwd), "elsewhere", "x.yaml"),
		},
		{
			name: "relative path passes through",
			file: "config.yaml",
			want: "config.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, displayPath(tt.file))
		})
	}
}

func TestBuiltinConfigSchemaMatchesIncludesExistingOptionalDirectories(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, "atmos.d"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".atmos", "profiles"), 0o700))
	t.Chdir(project)

	matches := builtinConfigSchemaMatches()
	assert.Contains(t, matches, "atmos.yaml")
	assert.Contains(t, matches, filepath.Join("atmos.d", "**", "*.yaml"))
	assert.Contains(t, matches, filepath.Join(".atmos", "profiles", "**", "*.yml"))
	assert.NotContains(t, matches, filepath.Join("profiles", "**", "*.yaml"))
}

func TestSchemaRegistryHelpers(t *testing.T) {
	av := &atmosValidatorExecutor{atmosConfig: &schema.AtmosConfiguration{Schemas: map[string]any{
		"custom": schema.SchemaRegistry{Manifest: "manifest.json"},
	}}}
	assert.ElementsMatch(t, []string{"custom", builtinConfigSchemaKey}, av.schemaKeys())
	assert.True(t, av.shouldSkipSchema("cue", ""))
	assert.True(t, av.shouldSkipSchema("custom", "config"))
	assert.False(t, av.shouldSkipSchema("custom", ""))

	custom := av.prepareSchemaValue("custom", "", "")
	assert.Equal(t, "manifest.json", custom.Schema)
	builtin := av.prepareSchemaValue(builtinConfigSchemaKey, "", "")
	assert.Equal(t, configSchemaSource, builtin.Schema)
	assert.Equal(t, builtinConfigSchemaMatches(), builtin.Matches)
	override := av.prepareSchemaValue("custom", "custom", "override.json")
	assert.Equal(t, "override.json", override.Schema)
}

func TestSchemaFilePositionsHandlesInvalidInput(t *testing.T) {
	assert.Nil(t, schemaFilePositions(filepath.Join(t.TempDir(), "missing.yaml")))
	invalid := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(invalid, []byte("not: [valid"), 0o600))
	assert.Nil(t, schemaFilePositions(invalid))
}

func TestValidateAtmosSchemaReport(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("field: value\n"), 0o600))

	t.Run("collects diagnostics with source positions", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		matcher := filematch.NewMockFileMatcher(ctrl)
		validatorMock := validator.NewMockValidator(ctrl)
		matcher.EXPECT().MatchFiles([]string{"config.yaml"}).Return([]string{file}, nil)
		validatorMock.EXPECT().ValidateYAMLSchema("schema.json", file).Return([]gojsonschema.ResultError{&mockResultError{
			field: "field", errType: "invalid_type", description: "must be a string",
		}}, nil)

		av := &atmosValidatorExecutor{
			validator: validatorMock, fileMatcher: matcher,
			atmosConfig: &schema.AtmosConfiguration{Schemas: map[string]any{
				"config": schema.SchemaRegistry{Schema: "schema.json", Matches: []string{"config.yaml"}},
			}},
		}
		report, err := av.ValidateAtmosSchemaReport("", "")
		require.NoError(t, err)
		require.Len(t, report.Diagnostics, 1)
		assert.Equal(t, 1, report.FilesChecked)
		assert.Equal(t, "schema", report.Diagnostics[0].Source)
		assert.Equal(t, "invalid_type", report.Diagnostics[0].RuleID)
		assert.Equal(t, file, report.Diagnostics[0].File)
		assert.Positive(t, report.Diagnostics[0].Line)
	})

	t.Run("returns validator errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		matcher := filematch.NewMockFileMatcher(ctrl)
		validatorMock := validator.NewMockValidator(ctrl)
		matcher.EXPECT().MatchFiles([]string{"config.yaml"}).Return([]string{file}, nil)
		validatorMock.EXPECT().ValidateYAMLSchema("schema.json", file).Return(nil, assert.AnError)
		av := &atmosValidatorExecutor{
			validator: validatorMock, fileMatcher: matcher,
			atmosConfig: &schema.AtmosConfiguration{Schemas: map[string]any{
				"config": schema.SchemaRegistry{Schema: "schema.json", Matches: []string{"config.yaml"}},
			}},
		}
		_, err := av.ValidateAtmosSchemaReport("", "")
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestFilterValidationSchemaFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	require.NoError(t, os.WriteFile(first, []byte("first: true\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("second: true\n"), 0o600))

	filtered := filterValidationSchemaFiles(map[string][]string{
		"schema.json": {first, second},
	}, []string{second})
	assert.Equal(t, map[string][]string{"schema.json": {second}}, filtered)
}
