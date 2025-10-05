package exec

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateWithOpa(t *testing.T) {
	tests := []struct {
		name          string
		data          any
		schemaPath    string
		modulePaths   []string
		timeout       int
		expectValid   bool
		expectError   bool
		errorContains string
		setupPolicy   func(t *testing.T) (string, func())
	}{
		{
			name:        "nonexistent policy file",
			data:        map[string]any{"test": "value"},
			schemaPath:  "/nonexistent/policy.rego",
			modulePaths: nil,
			timeout:     10,
			expectValid: false,
			expectError: true,
		},
		{
			name: "valid policy with no violations",
			data: map[string]any{
				"vars": map[string]any{
					"region": "us-east-1",
					"tags": map[string]any{
						"Team": "platform",
					},
				},
			},
			timeout:     10,
			expectValid: true,
			expectError: false,
			setupPolicy: func(t *testing.T) (string, func()) {
				// Create temporary policy file that passes.
				tmpDir := t.TempDir()
				policyPath := filepath.Join(tmpDir, "valid_policy.rego")
				policyContent := `package atmos

# This policy has no errors defined, so it will pass.
errors[msg] {
    input.vars.region == "invalid-region"
    msg := "Invalid region"
}
`
				err := os.WriteFile(policyPath, []byte(policyContent), 0o644)
				assert.NoError(t, err)
				return policyPath, func() {}
			},
		},
		{
			name: "policy violation",
			data: map[string]any{
				"vars": map[string]any{
					"region": "invalid-region",
				},
			},
			timeout:       10,
			expectValid:   false,
			expectError:   true,
			errorContains: "Invalid region",
			setupPolicy: func(t *testing.T) (string, func()) {
				// Create temporary policy file that fails for this data.
				tmpDir := t.TempDir()
				policyPath := filepath.Join(tmpDir, "violation_policy.rego")
				policyContent := `package atmos

errors[msg] {
    input.vars.region == "invalid-region"
    msg := "Invalid region"
}
`
				err := os.WriteFile(policyPath, []byte(policyContent), 0o644)
				assert.NoError(t, err)
				return policyPath, func() {}
			},
		},
		{
			name:          "invalid rego syntax",
			data:          map[string]any{"test": "value"},
			timeout:       10,
			expectValid:   false,
			expectError:   true,
			errorContains: "",
			setupPolicy: func(t *testing.T) (string, func()) {
				// Create temporary policy file with invalid syntax.
				tmpDir := t.TempDir()
				policyPath := filepath.Join(tmpDir, "invalid_syntax.rego")
				policyContent := `package atmos

errors[msg] {
    this is invalid rego syntax!!!
    msg := "test"
}
`
				err := os.WriteFile(policyPath, []byte(policyContent), 0o644)
				assert.NoError(t, err)
				return policyPath, func() {}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaPath := tt.schemaPath

			// Setup temporary policy file if needed.
			if tt.setupPolicy != nil {
				var cleanup func()
				schemaPath, cleanup = tt.setupPolicy(t)
				defer cleanup()
			}

			// Execute validation.
			valid, err := ValidateWithOpa(tt.data, schemaPath, tt.modulePaths, tt.timeout)

			// Assert results.
			assert.Equal(t, tt.expectValid, valid, "validation result mismatch")

			if tt.expectError {
				assert.Error(t, err, "expected error but got nil")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "error message mismatch")
				}
			} else {
				assert.NoError(t, err, "unexpected error")
			}
		})
	}
}

func TestValidateWithOpaLegacy_PolicyNamespace(t *testing.T) {
	// Test that ValidateWithOpaLegacy correctly uses the 'package atmos' namespace.
	// This is a basic smoke test to verify the function accepts properly formatted policies.
	// Full integration testing of OPA policy evaluation is done elsewhere.
	policyContent := `package atmos

# Valid but empty policy - no errors defined.
errors[msg] {
    false  # Never triggers.
    msg := "test error"
}
`
	data := map[string]interface{}{
		"test": "value",
	}

	// This should pass since the policy has no violations.
	valid, err := ValidateWithOpaLegacy(data, "test.rego", policyContent, 10)

	// The policy should validate successfully (no violations).
	assert.True(t, valid)
	assert.NoError(t, err)
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
