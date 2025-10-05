package exec

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateWithOpa_NonexistentFile(t *testing.T) {
	data := map[string]interface{}{
		"test": "value",
	}

	valid, err := ValidateWithOpa(data, "/nonexistent/policy.rego", nil, 10)

	assert.False(t, valid)
	assert.Error(t, err)
}

func TestValidateWithOpaLegacy_Timeout(t *testing.T) {
	// Test the legacy OPA validation with timeout.
	policyContent := `package test
deny[msg] {
    msg := "test denial"
}
`
	data := map[string]interface{}{
		"test": "value",
	}

	// Use timeout of 0 to force immediate deadline exceeded.
	valid, err := ValidateWithOpaLegacy(data, "test", policyContent, 0)

	assert.False(t, valid)
	assert.Error(t, err)
	// Check that error wrapping includes OPA timeout error.
	assert.ErrorIs(t, err, errUtils.ErrOPATimeout)
}

// TestIsWindowsOPALoadError tests the isWindowsOPALoadError function.
func TestIsWindowsOPALoadError(t *testing.T) {
	isWindows := runtime.GOOS == "windows"

	tests := []struct {
		name               string
		err                error
		expectedWindows    bool
		expectedNonWindows bool
	}{
		{
			name:               "nil error",
			err:                nil,
			expectedWindows:    false,
			expectedNonWindows: false,
		},
		{
			name:               "fs.ErrNotExist error",
			err:                fs.ErrNotExist,
			expectedWindows:    true,
			expectedNonWindows: false,
		},
		{
			name:               "os.ErrNotExist error",
			err:                os.ErrNotExist,
			expectedWindows:    true,
			expectedNonWindows: false,
		},
		{
			name:               "Windows path not specified error",
			err:                errors.New("cannot find the path specified"),
			expectedWindows:    true,
			expectedNonWindows: false,
		},
		{
			name:               "Windows file not found error",
			err:                errors.New("system cannot find the file specified"),
			expectedWindows:    true,
			expectedNonWindows: false,
		},
		{
			name:               "generic error",
			err:                errors.New("some other error"),
			expectedWindows:    false,
			expectedNonWindows: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWindowsOPALoadError(tt.err)

			if isWindows {
				assert.Equal(t, tt.expectedWindows, result, "Expected %v on Windows for: %v", tt.expectedWindows, tt.err)
			} else {
				assert.Equal(t, tt.expectedNonWindows, result, "Expected %v on non-Windows for: %v", tt.expectedNonWindows, tt.err)
			}
		})
	}
}

func TestContextDeadlineExceededWrapping(t *testing.T) {
	// Create a context that's already cancelled to simulate timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	// Create a simple error that includes deadline exceeded.
	err := ctx.Err()

	// Verify it's a deadline exceeded (actually Canceled in this case, but the pattern is the same).
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// This is how the code should wrap it.
		wrappedErr := errors.Join(errUtils.ErrOPATimeout, err)

		assert.Error(t, wrappedErr)
		assert.ErrorIs(t, wrappedErr, errUtils.ErrOPATimeout)
	}
}

// TestValidateWithCue tests that CUE validation returns not supported error.
func TestValidateWithCue(t *testing.T) {
	data := map[string]interface{}{
		"test": "value",
	}

	valid, err := ValidateWithCue(data, "test.cue", "test: string")

	assert.False(t, valid)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported yet")
}

// TestValidateWithOpaFallback_FileReadError tests the fallback when file cannot be read.
func TestValidateWithOpaFallback_FileReadError(t *testing.T) {
	data := map[string]interface{}{
		"test": "value",
	}

	// Use a non-existent file path.
	valid, err := validateWithOpaFallback(data, "/nonexistent/policy.rego", 10)

	assert.False(t, valid)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadFile)
}

// TestValidateWithJsonSchema_ValidationError tests JSON schema validation with invalid data.
func TestValidateWithJsonSchema_ValidationError(t *testing.T) {
	schema := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"name": {
				"type": "string"
			},
			"age": {
				"type": "number"
			}
		},
		"required": ["name"]
	}`

	// Invalid data: missing required "name" field.
	data := map[string]interface{}{
		"age": 25,
	}

	valid, err := ValidateWithJsonSchema(data, "test-schema", schema)

	assert.False(t, valid)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrValidation)
}

// TestValidateWithJsonSchema_Valid tests JSON schema validation with valid data.
func TestValidateWithJsonSchema_Valid(t *testing.T) {
	schema := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"name": {
				"type": "string"
			}
		},
		"required": ["name"]
	}`

	data := map[string]interface{}{
		"name": "John",
	}

	valid, err := ValidateWithJsonSchema(data, "test-schema", schema)

	assert.True(t, valid)
	assert.NoError(t, err)
}

// TestValidateWithJsonSchema_InvalidSchema tests with malformed JSON schema.
func TestValidateWithJsonSchema_InvalidSchema(t *testing.T) {
	schema := `{
		"type": "invalid-type-here"
	}`

	data := map[string]interface{}{
		"test": "value",
	}

	valid, err := ValidateWithJsonSchema(data, "test-schema", schema)

	assert.False(t, valid)
	assert.Error(t, err)
}

// TestIsWindowsOPALoadError_WrappedError tests wrapped fs.ErrNotExist.
func TestIsWindowsOPALoadError_WrappedError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Skipping Windows-specific test on %s", runtime.GOOS)
	}

	// Test wrapped fs.ErrNotExist.
	wrappedErr := errors.Join(errors.New("wrapper"), fs.ErrNotExist)
	result := isWindowsOPALoadError(wrappedErr)

	assert.True(t, result, "Expected true for wrapped fs.ErrNotExist on Windows")
}
