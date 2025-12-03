package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestConvertEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected []string
	}{
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name:     "nil values are skipped",
			input:    map[string]any{"KEY1": nil, "KEY2": "value2"},
			expected: []string{"KEY2=value2"},
		},
		{
			name:     "null string values are skipped",
			input:    map[string]any{"KEY1": "null", "KEY2": "value2"},
			expected: []string{"KEY2=value2"},
		},
		{
			name:     "various types converted to string",
			input:    map[string]any{"STR": "value", "INT": 42, "BOOL": true},
			expected: []string{"STR=value", "INT=42", "BOOL=true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertEnvVars(tt.input)
			// Sort for consistent comparison since map iteration order is not guaranteed.
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEnvironToMap(t *testing.T) {
	// Set a known env var for deterministic testing.
	t.Setenv("TEST_ENVIRON_TO_MAP", "test_value")

	result := EnvironToMap()

	// Should have at least some environment variables.
	assert.NotEmpty(t, result, "Environment should not be empty")

	// Verify our test variable is present.
	assert.Equal(t, "test_value", result["TEST_ENVIRON_TO_MAP"])
}

func TestSplitStringAtFirstOccurrence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected [2]string
	}{
		{
			name:     "normal split",
			input:    "KEY=value",
			sep:      "=",
			expected: [2]string{"KEY", "value"},
		},
		{
			name:     "value contains separator",
			input:    "KEY=value=with=equals",
			sep:      "=",
			expected: [2]string{"KEY", "value=with=equals"},
		},
		{
			name:     "no separator found",
			input:    "KEYVALUE",
			sep:      "=",
			expected: [2]string{"KEYVALUE", ""},
		},
		{
			name:     "empty value",
			input:    "KEY=",
			sep:      "=",
			expected: [2]string{"KEY", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitStringAtFirstOccurrence(tt.input, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrependToPath(t *testing.T) {
	tests := []struct {
		name        string
		currentPath string
		newDir      string
		expected    string
	}{
		{
			name:        "empty path",
			currentPath: "",
			newDir:      "/test/bin",
			expected:    "/test/bin",
		},
		{
			name:        "existing path",
			currentPath: "/usr/bin:/bin",
			newDir:      "/test/bin",
			expected:    "/test/bin" + string(os.PathListSeparator) + "/usr/bin:/bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrependToPath(tt.currentPath, tt.newDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPathFromEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		expected string
	}{
		{
			name:     "no PATH variable",
			env:      []string{"HOME=/home/user", "USER=testuser"},
			expected: "",
		},
		{
			name:     "PATH exists",
			env:      []string{"HOME=/home/user", "PATH=/usr/bin:/bin", "USER=testuser"},
			expected: "/usr/bin:/bin",
		},
		{
			name:     "empty PATH",
			env:      []string{"PATH=", "USER=testuser"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPathFromEnvironment(tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateEnvironmentPath(t *testing.T) {
	// Use cross-platform paths.
	homeDir := filepath.Join(string(filepath.Separator), "home", "user")
	testBinDir := filepath.Join(string(filepath.Separator), "test", "bin")
	usrBinDir := filepath.Join(string(filepath.Separator), "usr", "bin")
	binDir := filepath.Join(string(filepath.Separator), "bin")
	existingPath := usrBinDir + string(os.PathListSeparator) + binDir

	tests := []struct {
		name     string
		env      []string
		newDir   string
		expected []string
	}{
		{
			name:   "add PATH to empty environment",
			env:    []string{"HOME=" + homeDir},
			newDir: testBinDir,
			expected: []string{
				"HOME=" + homeDir,
				"PATH=" + testBinDir,
			},
		},
		{
			name:   "update existing PATH",
			env:    []string{"HOME=" + homeDir, "PATH=" + existingPath},
			newDir: testBinDir,
			expected: []string{
				"HOME=" + homeDir,
				"PATH=" + testBinDir + string(os.PathListSeparator) + existingPath,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UpdateEnvironmentPath(tt.env, tt.newDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnsureBinaryInPath(t *testing.T) {
	// Use cross-platform paths.
	binDir1 := filepath.Join(string(filepath.Separator), "usr", "bin")
	binDir2 := filepath.Join(string(filepath.Separator), "bin")
	testBinDir := filepath.Join(string(filepath.Separator), "test", "bin")
	testBinary := filepath.Join(testBinDir, "atmos")

	// Create cross-platform PATH values.
	existingPath := binDir1 + string(os.PathListSeparator) + binDir2
	pathWithTestDir := testBinDir + string(os.PathListSeparator) + existingPath

	tests := []struct {
		name       string
		env        []string
		binaryPath string
		expected   func([]string) bool // Function to validate result
	}{
		{
			name:       "binary directory not in PATH",
			env:        []string{"PATH=" + existingPath},
			binaryPath: testBinary,
			expected: func(result []string) bool {
				path := GetPathFromEnvironment(result)
				return strings.HasPrefix(path, testBinDir+string(os.PathListSeparator))
			},
		},
		{
			name:       "binary directory already in PATH",
			env:        []string{"PATH=" + pathWithTestDir},
			binaryPath: testBinary,
			expected: func(result []string) bool {
				path := GetPathFromEnvironment(result)
				return path == pathWithTestDir
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureBinaryInPath(tt.env, tt.binaryPath)
			assert.True(t, tt.expected(result))
		})
	}
}

func TestUpdateEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		key      string
		value    string
		expected func([]string) bool
	}{
		{
			name:  "add new variable to empty environment",
			env:   []string{},
			key:   "TEST_VAR",
			value: "test_value",
			expected: func(result []string) bool {
				return len(result) == 1 && result[0] == "TEST_VAR=test_value"
			},
		},
		{
			name:  "add new variable to existing environment",
			env:   []string{"HOME=/home/user", "USER=testuser"},
			key:   "TEST_VAR",
			value: "test_value",
			expected: func(result []string) bool {
				return len(result) == 3 && result[2] == "TEST_VAR=test_value"
			},
		},
		{
			name:  "update existing variable",
			env:   []string{"HOME=/home/user", "TEST_VAR=old_value", "USER=testuser"},
			key:   "TEST_VAR",
			value: "new_value",
			expected: func(result []string) bool {
				for _, envVar := range result {
					if envVar == "TEST_VAR=new_value" {
						return true
					}
					if strings.HasPrefix(envVar, "TEST_VAR=old_value") {
						return false // Old value should not exist
					}
				}
				return false
			},
		},
		{
			name:  "update variable with empty value",
			env:   []string{"HOME=/home/user", "TEST_VAR=old_value"},
			key:   "TEST_VAR",
			value: "",
			expected: func(result []string) bool {
				for _, envVar := range result {
					if envVar == "TEST_VAR=" {
						return true
					}
				}
				return false
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UpdateEnvVar(tt.env, tt.key, tt.value)
			assert.True(t, tt.expected(result), "UpdateEnvVar result validation failed for test: %s", tt.name)
		})
	}
}

func TestCommandEnvToMap(t *testing.T) {
	type args struct {
		envs []schema.CommandEnv
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "empty input returns empty map",
			args: args{envs: []schema.CommandEnv{}},
			want: map[string]string{},
		},
		{
			name: "single entry maps key to value",
			args: args{envs: []schema.CommandEnv{{Key: "FOO", Value: "bar"}}},
			want: map[string]string{"FOO": "bar"},
		},
		{
			name: "multiple entries map correctly",
			args: args{envs: []schema.CommandEnv{
				{Key: "FOO", Value: "bar"},
				{Key: "HELLO", Value: "world"},
			}},
			want: map[string]string{
				"FOO":   "bar",
				"HELLO": "world",
			},
		},
		{
			name: "duplicate keys: last wins",
			args: args{envs: []schema.CommandEnv{
				{Key: "FOO", Value: "first"},
				{Key: "FOO", Value: "second"},
				{Key: "FOO", Value: "third"},
			}},
			want: map[string]string{"FOO": "third"},
		},
		{
			name: "key casing is preserved and ValueCommand is ignored",
			args: args{envs: []schema.CommandEnv{
				{Key: "Aws_Profile", Value: "val1", ValueCommand: "echo ignored"},
				{Key: "aws_profile", Value: "val2"},
			}},
			want: map[string]string{
				// Both keys should exist independently due to casing differences.
				"Aws_Profile": "val1",
				"aws_profile": "val2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, CommandEnvToMap(tt.args.envs), "CommandEnvToMap(%v)", tt.args.envs)
		})
	}
}

func TestEnvironmentPathIntegration(t *testing.T) {
	// Integration test: simulate the full AtmosRunner workflow.
	// Use cross-platform paths.
	homeDir := filepath.Join(string(filepath.Separator), "home", "user")
	usrBinDir := filepath.Join(string(filepath.Separator), "usr", "bin")
	binDir := filepath.Join(string(filepath.Separator), "bin")
	originalPath := usrBinDir + string(os.PathListSeparator) + binDir

	originalEnv := []string{
		"HOME=" + homeDir,
		"PATH=" + originalPath,
		"USER=testuser",
	}

	// Simulate test binary in temp directory.
	testBinaryPath := filepath.Join(os.TempDir(), "atmos-test-12345", "atmos")

	// Update environment with test binary.
	updatedEnv := EnsureBinaryInPath(originalEnv, testBinaryPath)

	// Verify test binary directory is first in PATH.
	updatedPath := GetPathFromEnvironment(updatedEnv)
	expectedPrefix := filepath.Join(os.TempDir(), "atmos-test-12345") + string(os.PathListSeparator)
	assert.True(t, strings.HasPrefix(updatedPath, expectedPrefix),
		"PATH should start with test binary directory: %s", updatedPath)

	// Verify original PATH components are preserved.
	assert.Contains(t, updatedPath, originalPath,
		"Original PATH should be preserved: %s", updatedPath)
}

func TestFindPathIndex(t *testing.T) {
	tests := []struct {
		name        string
		env         []string
		expectedIdx int
		expectedKey string
	}{
		{
			name:        "PATH in uppercase",
			env:         []string{"HOME=/home", "PATH=/usr/bin", "USER=test"},
			expectedIdx: 1,
			expectedKey: "PATH",
		},
		{
			name:        "Path in mixed case",
			env:         []string{"HOME=/home", "Path=/usr/bin", "USER=test"},
			expectedIdx: 1,
			expectedKey: "Path",
		},
		{
			name:        "path in lowercase",
			env:         []string{"HOME=/home", "path=/usr/bin", "USER=test"},
			expectedIdx: 1,
			expectedKey: "path",
		},
		{
			name:        "no PATH",
			env:         []string{"HOME=/home", "USER=test"},
			expectedIdx: -1,
			expectedKey: "PATH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, key := findPathIndex(tt.env)
			assert.Equal(t, tt.expectedIdx, idx)
			assert.Equal(t, tt.expectedKey, key)
		})
	}
}
