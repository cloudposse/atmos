package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasContext(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		key      string
		value    string
		expected bool
	}{
		{
			name: "context exists",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				Err(),
			key:      "format",
			value:    "xml",
			expected: true,
		},
		{
			name: "context with multiple pairs",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				WithContext("version", "1.0.0").
				Err(),
			key:      "version",
			value:    "1.0.0",
			expected: true,
		},
		{
			name: "context key exists but value doesn't match",
			err: Build(errors.New("test error")).
				WithContext("format", "json").
				Err(),
			key:      "format",
			value:    "xml",
			expected: false,
		},
		{
			name: "context key doesn't exist",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				Err(),
			key:      "version",
			value:    "1.0.0",
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			key:      "format",
			value:    "xml",
			expected: false,
		},
		{
			name:     "error without context",
			err:      errors.New("plain error"),
			key:      "format",
			value:    "xml",
			expected: false,
		},
		{
			name: "false positive - similar key prefix",
			err: Build(errors.New("test error")).
				WithContext("myformat", "xml").
				Err(),
			key:      "format",
			value:    "xml",
			expected: false,
		},
		{
			name: "false positive - similar value prefix",
			err: Build(errors.New("test error")).
				WithContext("format", "xml2").
				Err(),
			key:      "format",
			value:    "xml",
			expected: false,
		},
		{
			name: "false positive - value contains search",
			err: Build(errors.New("test error")).
				WithContext("format", "xml_extended").
				Err(),
			key:      "format",
			value:    "xml",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasContext(tt.err, tt.key, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContext(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		key           string
		expectedValue string
		expectedOk    bool
	}{
		{
			name: "context exists",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				Err(),
			key:           "format",
			expectedValue: "xml",
			expectedOk:    true,
		},
		{
			name: "context with multiple pairs - get first",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				WithContext("version", "1.0.0").
				Err(),
			key:           "format",
			expectedValue: "xml",
			expectedOk:    true,
		},
		{
			name: "context with multiple pairs - get second",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				WithContext("version", "1.0.0").
				Err(),
			key:           "version",
			expectedValue: "1.0.0",
			expectedOk:    true,
		},
		{
			name: "context key doesn't exist",
			err: Build(errors.New("test error")).
				WithContext("format", "xml").
				Err(),
			key:           "version",
			expectedValue: "",
			expectedOk:    false,
		},
		{
			name:          "nil error",
			err:           nil,
			key:           "format",
			expectedValue: "",
			expectedOk:    false,
		},
		{
			name:          "error without context",
			err:           errors.New("plain error"),
			key:           "format",
			expectedValue: "",
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := GetContext(tt.err, tt.key)
			assert.Equal(t, tt.expectedOk, ok)
			assert.Equal(t, tt.expectedValue, value)
		})
	}
}

func TestGetContext_WithComplexValues(t *testing.T) {
	// Test with values that might contain special characters.
	err := Build(errors.New("test error")).
		WithContext("path", "/some/file/path.txt").
		WithContext("component", "vpc-prod").
		Err()

	path, ok := GetContext(err, "path")
	assert.True(t, ok)
	assert.Equal(t, "/some/file/path.txt", path)

	component, ok := GetContext(err, "component")
	assert.True(t, ok)
	assert.Equal(t, "vpc-prod", component)
}
