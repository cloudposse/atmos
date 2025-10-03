package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			name: "string values",
			input: map[string]any{
				"KEY1": "value1",
				"KEY2": "value2",
			},
			expected: []string{"KEY1=value1", "KEY2=value2"},
		},
		{
			name: "mixed types",
			input: map[string]any{
				"KEY1": "value1",
				"KEY2": 123,
				"KEY3": true,
			},
			expected: []string{"KEY1=value1", "KEY2=123", "KEY3=true"},
		},
		{
			name: "null value excluded",
			input: map[string]any{
				"KEY1": "value1",
				"KEY2": "null",
			},
			expected: []string{"KEY1=value1"},
		},
		{
			name: "nil value excluded",
			input: map[string]any{
				"KEY1": "value1",
				"KEY2": nil,
			},
			expected: []string{"KEY1=value1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertEnvVars(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestFindPathIndex(t *testing.T) {
	tests := []struct {
		name        string
		env         []string
		expectedIdx int
		expectedKey string
	}{
		{
			name:        "PATH found",
			env:         []string{"HOME=/home/user", "PATH=/usr/bin", "USER=test"},
			expectedIdx: 1,
			expectedKey: "PATH",
		},
		{
			name:        "Path found (Windows style)",
			env:         []string{"HOME=/home/user", "Path=/usr/bin", "USER=test"},
			expectedIdx: 1,
			expectedKey: "Path",
		},
		{
			name:        "PATH not found",
			env:         []string{"HOME=/home/user", "USER=test"},
			expectedIdx: -1,
			expectedKey: "PATH",
		},
		{
			name:        "empty env",
			env:         []string{},
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
