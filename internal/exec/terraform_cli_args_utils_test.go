package exec

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTFCliArgsVars(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected map[string]string
	}{
		{
			name:     "empty environment variable",
			envValue: "",
			expected: map[string]string{},
		},
		{
			name:     "single -var argument",
			envValue: "-var environment=prod",
			expected: map[string]string{
				"environment": "prod",
			},
		},
		{
			name:     "multiple -var arguments",
			envValue: "-var environment=prod -var region=us-east-1 -var instance_count=3",
			expected: map[string]string{
				"environment":    "prod",
				"region":         "us-east-1",
				"instance_count": "3",
			},
		},
		{
			name:     "mixed with other arguments",
			envValue: "-auto-approve -var environment=staging -input=false -var tag=latest",
			expected: map[string]string{
				"environment": "staging",
				"tag":         "latest",
			},
		},
		{
			name:     "quoted values",
			envValue: `-var "environment=production with spaces" -var 'region=us-west-2'`,
			expected: map[string]string{
				"environment": "production with spaces",
				"region":      "us-west-2",
			},
		},
		{
			name:     "var with equals format",
			envValue: "-var=environment=dev -var=region=eu-west-1",
			expected: map[string]string{
				"environment": "dev",
				"region":      "eu-west-1",
			},
		},
		{
			name:     "complex values with equals signs",
			envValue: `-var database_url="postgres://user:pass@host:5432/db" -var connection_string="server=host;database=db"`,
			expected: map[string]string{
				"database_url":      "postgres://user:pass@host:5432/db",
				"connection_string": "server=host;database=db",
			},
		},
		{
			name:     "JSON-like values",
			envValue: `-var 'tags={"Environment":"prod","Team":"devops"}' -var list='["item1","item2"]'`,
			expected: map[string]string{
				"tags": `{"Environment":"prod","Team":"devops"}`,
				"list": `["item1","item2"]`,
			},
		},
		{
			name:     "empty value",
			envValue: "-var empty_var= -var normal_var=value",
			expected: map[string]string{
				"empty_var":  "",
				"normal_var": "value",
			},
		},
		{
			name:     "special characters in values",
			envValue: `-var path="/tmp/test file" -var command="echo 'hello world'"`,
			expected: map[string]string{
				"path":    "/tmp/test file",
				"command": "echo 'hello world'",
			},
		},
		{
			name:     "malformed var arguments are ignored",
			envValue: "-var -var malformed -var good=value",
			expected: map[string]string{
				"good": "value",
			},
		},
		{
			name:     "var without value is ignored",
			envValue: "-var key_without_equals -var good=value",
			expected: map[string]string{
				"good": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original value to restore later.
			originalValue := os.Getenv("TF_CLI_ARGS")
			defer func() {
				if originalValue != "" {
					os.Setenv("TF_CLI_ARGS", originalValue)
				} else {
					os.Unsetenv("TF_CLI_ARGS")
				}
			}()

			// Set test environment variable
			os.Setenv("TF_CLI_ARGS", tt.envValue)

			// Test the function
			result := ParseTFCliArgsVars()

			// Assert results
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseTFCliArgsVars() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestParseTFCliArgsVars_NoEnvironmentVariable(t *testing.T) {
	// Ensure TF_CLI_ARGS is not set.
	originalValue := os.Getenv("TF_CLI_ARGS")
	os.Unsetenv("TF_CLI_ARGS")
	defer func() {
		if originalValue != "" {
			os.Setenv("TF_CLI_ARGS", originalValue)
		}
	}()

	result := ParseTFCliArgsVars()

	// Should return empty map when environment variable is not set.
	expected := map[string]string{}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ParseTFCliArgsVars() with no env var = %v, expected %v", result, expected)
	}
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single argument",
			input:    "plan",
			expected: []string{"plan"},
		},
		{
			name:     "multiple arguments",
			input:    "terraform plan -auto-approve",
			expected: []string{"terraform", "plan", "-auto-approve"},
		},
		{
			name:     "quoted arguments",
			input:    `terraform plan -var "environment=production"`,
			expected: []string{"terraform", "plan", "-var", "environment=production"},
		},
		{
			name:     "single quoted arguments",
			input:    `terraform plan -var 'region=us-west-2'`,
			expected: []string{"terraform", "plan", "-var", "region=us-west-2"},
		},
		{
			name:     "mixed quotes",
			input:    `terraform plan -var "env=prod" -var 'region=us-east-1'`,
			expected: []string{"terraform", "plan", "-var", "env=prod", "-var", "region=us-east-1"},
		},
		{
			name:     "quotes within quotes",
			input:    `terraform plan -var 'command=echo "hello world"'`,
			expected: []string{"terraform", "plan", "-var", `command=echo "hello world"`},
		},
		{
			name:     "extra spaces",
			input:    "  terraform   plan   -auto-approve  ",
			expected: []string{"terraform", "plan", "-auto-approve"},
		},
		{
			name:     "complex example",
			input:    `-auto-approve -var "database_url=postgres://user:pass@host:5432/db" -var environment=prod`,
			expected: []string{"-auto-approve", "-var", "database_url=postgres://user:pass@host:5432/db", "-var", "environment=prod"},
		},
		{
			name:     "empty quotes",
			input:    `terraform plan -var "empty=" -var normal=value`,
			expected: []string{"terraform", "plan", "-var", "empty=", "-var", "normal=value"},
		},
		{
			name:     "unclosed quotes handled gracefully",
			input:    `terraform plan -var "unclosed`,
			expected: []string{"terraform", "plan", "-var", "unclosed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommandArgs(tt.input)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseCommandArgs(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractVarFromNextArg(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		startIndex     int
		expectedResult map[string]string
		expectedIndex  int
	}{
		{
			name:           "valid key-value pair",
			args:           []string{"-var", "key=value"},
			startIndex:     0,
			expectedResult: map[string]string{"key": "value"},
			expectedIndex:  1,
		},
		{
			name:           "key-value with complex value",
			args:           []string{"-var", "instance_type=t3.large"},
			startIndex:     0,
			expectedResult: map[string]string{"instance_type": "t3.large"},
			expectedIndex:  1,
		},
		{
			name:           "key-value with multiple equals",
			args:           []string{"-var", "config=key=value=more"},
			startIndex:     0,
			expectedResult: map[string]string{"config": "key=value=more"},
			expectedIndex:  1,
		},
		{
			name:           "key-value with empty value",
			args:           []string{"-var", "empty="},
			startIndex:     0,
			expectedResult: map[string]string{"empty": ""},
			expectedIndex:  1,
		},
		{
			name:           "no next argument",
			args:           []string{"-var"},
			startIndex:     0,
			expectedResult: map[string]string{},
			expectedIndex:  0,
		},
		{
			name:           "invalid format without equals",
			args:           []string{"-var", "invalidformat"},
			startIndex:     0,
			expectedResult: map[string]string{},
			expectedIndex:  1,
		},
		{
			name:           "index at end of array",
			args:           []string{"-var", "key=value"},
			startIndex:     1,
			expectedResult: map[string]string{},
			expectedIndex:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]string)
			index := extractVarFromNextArg(tt.args, tt.startIndex, result)

			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, tt.expectedIndex, index)
		})
	}
}

func TestExtractVarFromEqualFormat(t *testing.T) {
	tests := []struct {
		name           string
		arg            string
		expectedResult map[string]string
	}{
		{
			name:           "valid key-value pair",
			arg:            "-var=key=value",
			expectedResult: map[string]string{"key": "value"},
		},
		{
			name:           "key-value with complex value",
			arg:            "-var=instance_type=t3.large",
			expectedResult: map[string]string{"instance_type": "t3.large"},
		},
		{
			name:           "key-value with multiple equals",
			arg:            "-var=config=key=value=more",
			expectedResult: map[string]string{"config": "key=value=more"},
		},
		{
			name:           "key-value with empty value",
			arg:            "-var=empty=",
			expectedResult: map[string]string{"empty": ""},
		},
		{
			name:           "invalid format without value equals",
			arg:            "-var=invalidformat",
			expectedResult: map[string]string{},
		},
		{
			name:           "just the var flag",
			arg:            "-var=",
			expectedResult: map[string]string{},
		},
		{
			name:           "key with spaces in value",
			arg:            "-var=message=hello world",
			expectedResult: map[string]string{"message": "hello world"},
		},
		{
			name:           "key with special characters",
			arg:            "-var=special=!@#$%^&*()",
			expectedResult: map[string]string{"special": "!@#$%^&*()"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]string)
			extractVarFromEqualFormat(tt.arg, result)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestIsQuoteChar(t *testing.T) {
	tests := []struct {
		name     string
		char     rune
		expected bool
	}{
		{
			name:     "double quote is quote char",
			char:     '"',
			expected: true,
		},
		{
			name:     "single quote is quote char",
			char:     '\'',
			expected: true,
		},
		{
			name:     "regular letter is not quote char",
			char:     'a',
			expected: false,
		},
		{
			name:     "number is not quote char",
			char:     '1',
			expected: false,
		},
		{
			name:     "space is not quote char",
			char:     ' ',
			expected: false,
		},
		{
			name:     "equals is not quote char",
			char:     '=',
			expected: false,
		},
		{
			name:     "dash is not quote char",
			char:     '-',
			expected: false,
		},
		{
			name:     "backtick is not quote char",
			char:     '`',
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isQuoteChar(tt.char)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark test to ensure the function performs well with large inputs.
func BenchmarkParseTFCliArgsVars(b *testing.B) {
	// Create a large TF_CLI_ARGS string for benchmarking
	largeTFCliArgs := ""
	for i := 0; i < 100; i++ {
		largeTFCliArgs += fmt.Sprintf(" -var key%d=value%d", i, i)
	}

	originalValue := os.Getenv("TF_CLI_ARGS")
	os.Setenv("TF_CLI_ARGS", largeTFCliArgs)
	defer func() {
		if originalValue != "" {
			os.Setenv("TF_CLI_ARGS", originalValue)
		} else {
			os.Unsetenv("TF_CLI_ARGS")
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseTFCliArgsVars()
	}
}

func BenchmarkExtractVarFromNextArg(b *testing.B) {
	args := []string{"-var", "key=value", "-var", "other=test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := make(map[string]string)
		extractVarFromNextArg(args, 0, result)
	}
}

func BenchmarkExtractVarFromEqualFormat(b *testing.B) {
	arg := "-var=key=value"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := make(map[string]string)
		extractVarFromEqualFormat(arg, result)
	}
}

func BenchmarkIsQuoteChar(b *testing.B) {
	chars := []rune{'"', '\'', 'a', '1', ' '}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		char := chars[i%len(chars)]
		isQuoteChar(char)
	}
}
