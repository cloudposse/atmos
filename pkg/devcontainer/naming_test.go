package devcontainer

import (
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateContainerName(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		instance      string
		expected      string
		expectError   bool
		errorContains string
	}{
		{
			name:     "valid name and instance",
			devName:  "geodesic",
			instance: "default",
			expected: "atmos-devcontainer-geodesic-default",
		},
		{
			name:     "valid name with empty instance uses default",
			devName:  "terraform",
			instance: "",
			expected: "atmos-devcontainer-terraform-default",
		},
		{
			name:     "valid name with hyphens",
			devName:  "my-dev-env",
			instance: "test-1",
			expected: "atmos-devcontainer-my-dev-env-test-1",
		},
		{
			name:     "valid name with underscores",
			devName:  "my_dev_env",
			instance: "test_1",
			expected: "atmos-devcontainer-my_dev_env-test_1",
		},
		{
			name:     "valid name with mixed separators",
			devName:  "my-dev_env",
			instance: "test-1_2",
			expected: "atmos-devcontainer-my-dev_env-test-1_2",
		},
		{
			name:          "empty devcontainer name",
			devName:       "",
			instance:      "default",
			expectError:   true,
			errorContains: "empty",
		},
		{
			name:          "invalid devcontainer name starting with hyphen",
			devName:       "-invalid",
			instance:      "default",
			expectError:   true,
			errorContains: "invalid",
		},
		{
			name:          "invalid devcontainer name with special characters",
			devName:       "my@dev",
			instance:      "default",
			expectError:   true,
			errorContains: "invalid",
		},
		{
			name:          "invalid instance name starting with hyphen",
			devName:       "valid",
			instance:      "-invalid",
			expectError:   true,
			errorContains: "invalid",
		},
		{
			name:          "devcontainer name too long",
			devName:       strings.Repeat("a", maxNameLength+1),
			instance:      "default",
			expectError:   true,
			errorContains: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateContainerName(tt.devName, tt.instance)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseContainerName(t *testing.T) {
	tests := []struct {
		name             string
		containerName    string
		expectedName     string
		expectedInstance string
	}{
		{
			name:             "valid container name",
			containerName:    "atmos-devcontainer-geodesic-default",
			expectedName:     "geodesic",
			expectedInstance: "default",
		},
		{
			name:             "container name with hyphens in devcontainer name",
			containerName:    "atmos-devcontainer-my-dev-env-test-1",
			expectedName:     "my-dev-env-test",
			expectedInstance: "1",
		},
		{
			name:             "container name with underscores",
			containerName:    "atmos-devcontainer-my_dev_env-test_1",
			expectedName:     "my_dev_env",
			expectedInstance: "test_1",
		},
		{
			name:             "container name with multiple hyphens",
			containerName:    "atmos-devcontainer-a-b-c-d-e",
			expectedName:     "a-b-c-d",
			expectedInstance: "e",
		},
		{
			name:             "invalid prefix",
			containerName:    "other-container-name-instance",
			expectedName:     "",
			expectedInstance: "",
		},
		{
			name:             "missing prefix",
			containerName:    "geodesic-default",
			expectedName:     "",
			expectedInstance: "",
		},
		{
			name:             "prefix only",
			containerName:    "atmos-devcontainer",
			expectedName:     "",
			expectedInstance: "",
		},
		{
			name:             "prefix with single part",
			containerName:    "atmos-devcontainer-name",
			expectedName:     "",
			expectedInstance: "",
		},
		{
			name:             "empty string",
			containerName:    "",
			expectedName:     "",
			expectedInstance: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, instance := ParseContainerName(tt.containerName)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedInstance, instance)
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorType   error
	}{
		{
			name:        "valid simple name",
			input:       "geodesic",
			expectError: false,
		},
		{
			name:        "valid name with hyphen",
			input:       "my-dev",
			expectError: false,
		},
		{
			name:        "valid name with underscore",
			input:       "my_dev",
			expectError: false,
		},
		{
			name:        "valid name with mixed separators",
			input:       "my-dev_env",
			expectError: false,
		},
		{
			name:        "valid name starting with number",
			input:       "1dev",
			expectError: false,
		},
		{
			name:        "valid name with numbers",
			input:       "dev123",
			expectError: false,
		},
		{
			name:        "empty name",
			input:       "",
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameEmpty,
		},
		{
			name:        "name starting with hyphen",
			input:       "-dev",
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameInvalid,
		},
		{
			name:        "name starting with underscore",
			input:       "_dev",
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameInvalid,
		},
		{
			name:        "name with special characters",
			input:       "my@dev",
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameInvalid,
		},
		{
			name:        "name with spaces",
			input:       "my dev",
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameInvalid,
		},
		{
			name:        "name with dots",
			input:       "my.dev",
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameInvalid,
		},
		{
			name:        "name too long",
			input:       strings.Repeat("a", maxNameLength+1),
			expectError: true,
			errorType:   errUtils.ErrDevcontainerNameTooLong,
		},
		{
			name:        "name exactly at max length",
			input:       strings.Repeat("a", maxNameLength),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errorType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsAtmosDevcontainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expected      bool
	}{
		{
			name:          "valid atmos devcontainer",
			containerName: "atmos-devcontainer-geodesic-default",
			expected:      true,
		},
		{
			name:          "valid atmos devcontainer with hyphens",
			containerName: "atmos-devcontainer-my-dev-instance-1",
			expected:      true,
		},
		{
			name:          "prefix only",
			containerName: "atmos-devcontainer",
			expected:      false,
		},
		{
			name:          "non-atmos container",
			containerName: "other-container-name",
			expected:      false,
		},
		{
			name:          "partial prefix match",
			containerName: "atmos-other-container",
			expected:      false,
		},
		{
			name:          "empty string",
			containerName: "",
			expected:      false,
		},
		{
			name:          "prefix in middle",
			containerName: "prefix-atmos-devcontainer-name",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAtmosDevcontainer(tt.containerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
