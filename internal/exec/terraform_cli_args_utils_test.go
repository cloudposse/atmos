package exec

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTerraformEnvCliArgs(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "empty environment variable",
			envValue: "",
			expected: []string{},
		},
		{
			name:     "single argument",
			envValue: "-auto-approve",
			expected: []string{"-auto-approve"},
		},
		{
			name:     "multiple arguments",
			envValue: "-var environment=prod -auto-approve -input=false",
			expected: []string{"-var", "environment=prod", "-auto-approve", "-input=false"},
		},
		{
			name:     "quoted arguments",
			envValue: `-var "environment=production with spaces" -auto-approve`,
			expected: []string{"-var", "environment=production with spaces", "-auto-approve"},
		},
		{
			name:     "mixed quotes",
			envValue: `-var "env=prod" -var 'region=us-east-1'`,
			expected: []string{"-var", "env=prod", "-var", "region=us-east-1"},
		},
		{
			name:     "complex example with JSON",
			envValue: `-auto-approve -var 'tags={"Environment":"prod","Team":"devops"}' -var environment=prod`,
			expected: []string{"-auto-approve", "-var", `tags={"Environment":"prod","Team":"devops"}`, "-var", "environment=prod"},
		},
		{
			name:     "terraform plan specific args",
			envValue: "-out=planfile -detailed-exitcode -var environment=test",
			expected: []string{"-out=planfile", "-detailed-exitcode", "-var", "environment=test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variable.
			t.Setenv("TF_CLI_ARGS", tt.envValue)

			// Test the function
			result := GetTerraformEnvCliArgs()

			// Assert results
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("GetTerraformEnvCliArgs() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetTerraformEnvCliVars(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		expected    map[string]any
		expectError bool
	}{
		{
			name:        "empty environment variable",
			envValue:    "",
			expected:    map[string]any{},
			expectError: false,
		},
		{
			name:        "no var arguments",
			envValue:    "-auto-approve -input=false",
			expected:    map[string]any{},
			expectError: false,
		},
		{
			name:     "single var argument",
			envValue: "-var environment=prod",
			expected: map[string]any{
				"environment": "prod",
			},
			expectError: false,
		},
		{
			name:     "multiple var arguments",
			envValue: "-var environment=prod -var region=us-east-1 -var instance_count=3",
			expected: map[string]any{
				"environment":    "prod",
				"region":         "us-east-1",
				"instance_count": float64(3),
			},
			expectError: false,
		},
		{
			name:     "mixed with non-var arguments",
			envValue: "-auto-approve -var environment=staging -input=false -var tag=latest",
			expected: map[string]any{
				"environment": "staging",
				"tag":         "latest",
			},
			expectError: false,
		},
		{
			name:     "JSON values",
			envValue: `-var 'tags={"Environment":"prod","Team":"devops"}' -var 'list=["item1","item2"]'`,
			expected: map[string]any{
				"tags": map[string]any{"Environment": "prod", "Team": "devops"},
				"list": []any{"item1", "item2"},
			},
			expectError: false,
		},
		{
			name:     "quoted string values",
			envValue: `-var "environment=production with spaces" -var 'region=us-west-2'`,
			expected: map[string]any{
				"environment": "production with spaces",
				"region":      "us-west-2",
			},
			expectError: false,
		},
		{
			name:     "complex values with equals signs",
			envValue: `-var database_url="postgres://user:pass@host:5432/db" -var connection_string="server=host;database=db"`,
			expected: map[string]any{
				"database_url":      "postgres://user:pass@host:5432/db",
				"connection_string": "server=host;database=db",
			},
			expectError: false,
		},
		{
			name:     "empty value",
			envValue: "-var empty_var= -var normal_var=value",
			expected: map[string]any{
				"empty_var":  "",
				"normal_var": "value",
			},
			expectError: false,
		},
		{
			name:     "number values",
			envValue: `-var count=5 -var price=19.99 -var config='{"timeout":30,"retries":3}'`,
			expected: map[string]any{
				"count": float64(5),
				"price": float64(19.99),
				"config": map[string]any{
					"timeout": float64(30),
					"retries": float64(3),
				},
			},
			expectError: false,
		},
		{
			name:     "var with equals format",
			envValue: "-var=environment=dev -var=region=eu-west-1 -var=count=42",
			expected: map[string]any{
				"environment": "dev",
				"region":      "eu-west-1",
				"count":       float64(42),
			},
			expectError: false,
		},
		{
			name:        "malformed var arguments are ignored",
			envValue:    "-var -var malformed -var good=value",
			expected:    map[string]any{"good": "value"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variable
			t.Setenv("TF_CLI_ARGS", tt.envValue)

			// Test the function
			result, err := GetTerraformEnvCliVars()

			// Assert error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Assert results using testify for better comparison
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetTerraformEnvCliVars_NoEnvironmentVariable(t *testing.T) {
	// Ensure TF_CLI_ARGS is not set.
	originalValue := os.Getenv("TF_CLI_ARGS")
	os.Unsetenv("TF_CLI_ARGS")
	defer func() {
		if originalValue != "" {
			t.Setenv("TF_CLI_ARGS", originalValue)
		}
	}()

	result, err := GetTerraformEnvCliVars()

	// Should return empty map when environment variable is not set.
	assert.NoError(t, err)
	expected := map[string]any{}
	assert.Equal(t, expected, result)
}

func BenchmarkGetTerraformEnvCliArgs(b *testing.B) {
	// Create a realistic TF_CLI_ARGS string for benchmarking
	largeTFCliArgs := "-auto-approve -input=false"
	for i := 0; i < 10; i++ {
		largeTFCliArgs += fmt.Sprintf(" -var key%d=value%d", i, i)
	}

	originalValue := os.Getenv("TF_CLI_ARGS")
	_ = os.Setenv("TF_CLI_ARGS", largeTFCliArgs)
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("TF_CLI_ARGS", originalValue)
		} else {
			os.Unsetenv("TF_CLI_ARGS")
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetTerraformEnvCliArgs()
	}
}

func BenchmarkGetTerraformEnvCliVars(b *testing.B) {
	// Create a realistic TF_CLI_ARGS string for benchmarking
	largeTFCliArgs := "-auto-approve -input=false"
	for i := 0; i < 10; i++ {
		largeTFCliArgs += fmt.Sprintf(" -var key%d=value%d", i, i)
	}

	originalValue := os.Getenv("TF_CLI_ARGS")
	_ = os.Setenv("TF_CLI_ARGS", largeTFCliArgs)
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("TF_CLI_ARGS", originalValue)
		} else {
			os.Unsetenv("TF_CLI_ARGS")
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetTerraformEnvCliVars()
	}
}
