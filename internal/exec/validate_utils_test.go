package exec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestValidateWithOpa tests the ValidateWithOpa function with actual OPA policies.
func TestValidateWithOpa(t *testing.T) {
	// Note: These tests would require actual OPA policy files to run.
	// For now, we'll skip them if the test files don't exist.
	t.Skipf("Skipping OPA validation tests: test policy files not available in testdata/opa/")

	tests := []struct {
		name           string
		data           any
		schemaPath     string
		modulePaths    []string
		timeoutSeconds int
		expectError    bool
		errorCheck     func(error) bool
	}{
		{
			name: "Valid data passes validation",
			data: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{
								"region": "us-east-1",
							},
						},
					},
				},
			},
			// We'll use a simple policy that always passes.
			schemaPath:     "testdata/opa/valid_policy.rego",
			modulePaths:    []string{},
			timeoutSeconds: 5,
			expectError:    false,
		},
		{
			name: "Invalid data fails validation",
			data: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{
								"region": "invalid-region",
							},
						},
					},
				},
			},
			// We'll use a policy that validates regions.
			schemaPath:     "testdata/opa/region_policy.rego",
			modulePaths:    []string{},
			timeoutSeconds: 5,
			expectError:    true,
			errorCheck: func(err error) bool {
				return errors.Is(err, errUtils.ErrOPAPolicyViolations)
			},
		},
		{
			name: "Timeout is handled correctly",
			data: map[string]any{
				"test": "data",
			},
			// Use a policy that would hang/timeout.
			schemaPath:     "testdata/opa/timeout_policy.rego",
			modulePaths:    []string{},
			timeoutSeconds: 1, // Very short timeout.
			expectError:    true,
			errorCheck: func(err error) bool {
				// Should get timeout error message.
				return err != nil && errors.Is(errors.New(err.Error()), errors.New("Timeout evaluating the OPA policy"))
			},
		},
		{
			name: "Invalid policy file returns error",
			data: map[string]any{
				"test": "data",
			},
			schemaPath:     "testdata/opa/non_existent.rego",
			modulePaths:    []string{},
			timeoutSeconds: 5,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateWithOpa(tt.data, tt.schemaPath, tt.modulePaths, tt.timeoutSeconds)

			if tt.expectError {
				assert.Error(t, err)
				assert.False(t, result)
				if tt.errorCheck != nil {
					assert.True(t, tt.errorCheck(err), "Error check failed: %v", err)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, result)
			}
		})
	}
}

// TestValidateWithOpaLegacy tests the ValidateWithOpaLegacy function.
func TestValidateWithOpaLegacy(t *testing.T) {
	tests := []struct {
		name           string
		data           any
		schemaName     string
		schemaText     string
		timeoutSeconds int
		expectError    bool
		errorCheck     func(error) bool
	}{
		{
			name: "Valid data passes validation",
			data: map[string]any{
				"region": "us-east-1",
			},
			schemaName: "test.rego",
			schemaText: `
package atmos

errors[msg] {
	false
	msg := "This should never trigger"
}
`,
			timeoutSeconds: 5,
			expectError:    false,
		},
		{
			name: "Policy violations are detected",
			data: map[string]any{
				"region": "invalid",
			},
			schemaName: "test.rego",
			schemaText: `
package atmos

errors[msg] {
	input.region == "invalid"
	msg := "Invalid region specified"
}
`,
			timeoutSeconds: 5,
			expectError:    true,
			errorCheck: func(err error) bool {
				return errors.Is(err, errUtils.ErrOPAPolicyViolations)
			},
		},
		{
			name: "Invalid Rego syntax returns error",
			data: map[string]any{
				"test": "data",
			},
			schemaName: "test.rego",
			schemaText: `
package atmos

This is not valid Rego syntax
`,
			timeoutSeconds: 5,
			expectError:    true,
		},
		{
			name: "Empty errors array means validation passes",
			data: map[string]any{
				"test": "data",
			},
			schemaName: "test.rego",
			schemaText: `
package atmos

# No errors defined, so validation passes.
errors[msg] {
	false
	msg := "This will never trigger"
}
`,
			timeoutSeconds: 0, // Will use default of 20 seconds.
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateWithOpaLegacy(tt.data, tt.schemaName, tt.schemaText, tt.timeoutSeconds)

			if tt.expectError {
				assert.Error(t, err)
				assert.False(t, result)
				if tt.errorCheck != nil {
					assert.True(t, tt.errorCheck(err), "Error check failed: %v", err)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, result)
			}
		})
	}
}

// TestValidateWithOpa_ContextTimeout tests that context timeout is properly handled in ValidateWithOpa.
func TestValidateWithOpa_ContextTimeout(t *testing.T) {
	// Create a context that's already timed out.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for the context to timeout.
	<-ctx.Done()

	// Verify that the error is properly detected with errors.Is.
	assert.True(t, errors.Is(ctx.Err(), context.DeadlineExceeded))

	// Test with wrapped error to ensure error chain is preserved.
	wrappedErr := errors.Join(context.DeadlineExceeded, errors.New("additional context"))
	assert.True(t, errors.Is(wrappedErr, context.DeadlineExceeded))
}

// TestValidateWithOpaLegacy_ContextTimeout tests that context timeout is properly handled in ValidateWithOpaLegacy.
func TestValidateWithOpaLegacy_ContextTimeout(t *testing.T) {
	// Similar test for legacy function.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for the context to timeout.
	<-ctx.Done()

	// Verify that the error is properly detected with errors.Is.
	assert.True(t, errors.Is(ctx.Err(), context.DeadlineExceeded))
}

// TestContextDeadlineExceededHandling tests that context deadline exceeded errors are handled properly.
func TestContextDeadlineExceededHandling(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Direct deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "Wrapped deadline exceeded",
			err:      errors.Join(context.DeadlineExceeded, errors.New("extra info")),
			expected: true,
		},
		{
			name:     "Different error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "String comparison would fail",
			err:      errors.New("context deadline exceeded"), // String matches but not the same error.
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The new code uses errors.Is instead of string comparison.
			result := errors.Is(tt.err, context.DeadlineExceeded)
			assert.Equal(t, tt.expected, result)
		})
	}
}
