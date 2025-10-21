package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNoValuesFoundError tests the NoValuesFoundError type.
func TestNoValuesFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      *NoValuesFoundError
		expected string
	}{
		{
			name:     "With query",
			err:      &NoValuesFoundError{Component: "vpc", Query: "region"},
			expected: "no values found for component 'vpc' with query 'region'",
		},
		{
			name:     "Without query",
			err:      &NoValuesFoundError{Component: "vpc"},
			expected: "no values found for component 'vpc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

// TestInvalidFormatError tests the InvalidFormatError type.
func TestInvalidFormatError(t *testing.T) {
	err := &InvalidFormatError{
		Format: "xml",
		Valid:  []string{"yaml", "json"},
	}
	expected := "invalid format 'xml'. Valid formats are: yaml, json"
	assert.Equal(t, expected, err.Error())
}

// TestQueryError tests the QueryError type.
func TestQueryError(t *testing.T) {
	cause := errors.New("parse error")
	err := &QueryError{
		Query: "$.components",
		Cause: cause,
	}

	assert.Contains(t, err.Error(), "error processing query '$.components'")
	assert.Contains(t, err.Error(), "parse error")
	assert.Equal(t, cause, err.Unwrap())
}

// TestStackPatternError tests the StackPatternError type.
func TestStackPatternError(t *testing.T) {
	cause := errors.New("invalid pattern")
	err := &StackPatternError{
		Pattern: "***/invalid",
		Cause:   cause,
	}

	assert.Contains(t, err.Error(), "invalid stack pattern '***/invalid'")
	assert.Contains(t, err.Error(), "invalid pattern")
	assert.Equal(t, cause, err.Unwrap())
}

// TestNoMetadataFoundError tests the NoMetadataFoundError type.
func TestNoMetadataFoundError(t *testing.T) {
	err := &NoMetadataFoundError{
		Query: "$.metadata",
	}
	assert.Contains(t, err.Error(), "no metadata found")
	assert.Contains(t, err.Error(), "$.metadata")
}

// TestMetadataFilteringError tests the MetadataFilteringError type.
func TestMetadataFilteringError(t *testing.T) {
	cause := errors.New("filter error")
	err := &MetadataFilteringError{
		Cause: cause,
	}

	assert.Contains(t, err.Error(), "error filtering and listing metadata")
	assert.Contains(t, err.Error(), "filter error")
	assert.Equal(t, cause, err.Unwrap())
}

// TestCommonFlagsError tests the CommonFlagsError type.
func TestCommonFlagsError(t *testing.T) {
	cause := errors.New("flag error")
	err := &CommonFlagsError{
		Cause: cause,
	}

	assert.Contains(t, err.Error(), "error getting common flags")
	assert.Contains(t, err.Error(), "flag error")
	assert.Equal(t, cause, err.Unwrap())
}

// TestInitConfigError tests the InitConfigError type.
func TestInitConfigError(t *testing.T) {
	cause := errors.New("config error")
	err := &InitConfigError{
		Cause: cause,
	}

	assert.Contains(t, err.Error(), "error initializing CLI config")
	assert.Contains(t, err.Error(), "config error")
	assert.Equal(t, cause, err.Unwrap())
}

// TestDescribeStacksError tests the DescribeStacksError type.
func TestDescribeStacksError(t *testing.T) {
	cause := errors.New("describe error")
	err := &DescribeStacksError{
		Cause: cause,
	}

	assert.Contains(t, err.Error(), "error describing stacks")
	assert.Contains(t, err.Error(), "describe error")
	assert.Equal(t, cause, err.Unwrap())
}

// TestNoSettingsFoundError tests the NoSettingsFoundError type.
func TestNoSettingsFoundError(t *testing.T) {
	err := &NoSettingsFoundError{
		Query: "$.settings",
	}
	assert.Contains(t, err.Error(), "no settings found")
	assert.Contains(t, err.Error(), "$.settings")
}

// TestSettingsFilteringError tests the SettingsFilteringError type.
func TestSettingsFilteringError(t *testing.T) {
	cause := errors.New("filter error")
	err := &SettingsFilteringError{
		Cause: cause,
	}

	assert.Contains(t, err.Error(), "error filtering and listing settings")
	assert.Contains(t, err.Error(), "filter error")
	assert.Equal(t, cause, err.Unwrap())
}

// TestNoComponentSettingsFoundError tests the NoComponentSettingsFoundError type.
func TestNoComponentSettingsFoundError(t *testing.T) {
	err := &NoComponentSettingsFoundError{
		Component: "vpc",
	}
	assert.Contains(t, err.Error(), "no settings found for component 'vpc'")
}

// TestNoSettingsFoundForComponentError tests the NoSettingsFoundForComponentError type.
func TestNoSettingsFoundForComponentError(t *testing.T) {
	tests := []struct {
		name     string
		err      *NoSettingsFoundForComponentError
		expected string
	}{
		{
			name:     "With query",
			err:      &NoSettingsFoundForComponentError{Component: "vpc", Query: "region"},
			expected: "no settings found for component 'vpc' with query 'region'",
		},
		{
			name:     "Without query",
			err:      &NoSettingsFoundForComponentError{Component: "vpc"},
			expected: "no settings found for component 'vpc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

// TestComponentVarsNotFoundError tests the ComponentVarsNotFoundError type.
func TestComponentVarsNotFoundError(t *testing.T) {
	err := &ComponentVarsNotFoundError{
		Component: "vpc",
	}
	assert.Contains(t, err.Error(), "no vars found for component 'vpc'")
}

// TestComponentMetadataNotFoundError tests the ComponentMetadataNotFoundError type.
func TestComponentMetadataNotFoundError(t *testing.T) {
	err := &ComponentMetadataNotFoundError{
		Component: "vpc",
	}
	assert.Contains(t, err.Error(), "no metadata found for component 'vpc'")
}

// TestComponentDefinitionNotFoundError tests the ComponentDefinitionNotFoundError type.
func TestComponentDefinitionNotFoundError(t *testing.T) {
	err := &ComponentDefinitionNotFoundError{
		Component: "vpc",
	}
	assert.Contains(t, err.Error(), "component 'vpc' does not exist")
}
