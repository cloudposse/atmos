package utils

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteShellAndReturnOutput(t *testing.T) {
	// Save and restore ATMOS_SHLVL
	origShellLevel := os.Getenv("ATMOS_SHLVL")
	defer os.Setenv("ATMOS_SHLVL", origShellLevel)

	tests := []struct {
		name        string
		command     string
		cmdName     string
		dir         string
		env         []string
		dryRun      bool
		setup       func()
		expected    string
		expectError bool
		skipOnOS    []string
	}{
		{
			name:     "simple echo command",
			command:  "echo hello",
			cmdName:  "test-echo",
			dir:      ".",
			env:      []string{},
			dryRun:   false,
			expected: "hello\n",
		},
		{
			name:     "command with environment variable",
			command:  "echo $TEST_VAR",
			cmdName:  "test-env",
			dir:      ".",
			env:      []string{"TEST_VAR=test_value"},
			dryRun:   false,
			expected: "test_value\n",
		},
		{
			name:     "multiple environment variables",
			command:  "echo $VAR1 $VAR2",
			cmdName:  "test-multi-env",
			dir:      ".",
			env:      []string{"VAR1=hello", "VAR2=world"},
			dryRun:   false,
			expected: "hello world\n",
		},
		{
			name:     "dry run mode",
			command:  "echo should_not_execute",
			cmdName:  "test-dry-run",
			dir:      ".",
			env:      []string{},
			dryRun:   true,
			expected: "", // No output in dry run
		},
		{
			name:    "command in different directory",
			command: "pwd",
			cmdName: "test-pwd",
			dir:     t.TempDir(),
			env:     []string{},
			dryRun:  false,
			// expected will be checked differently
		},
		{
			name:        "invalid command syntax",
			command:     "echo 'unclosed quote",
			cmdName:     "test-invalid",
			dir:         ".",
			env:         []string{},
			dryRun:      false,
			expectError: true,
		},
		{
			name:        "command failure",
			command:     "exit 1",
			cmdName:     "test-failure",
			dir:         ".",
			env:         []string{},
			dryRun:      false,
			expectError: true,
		},
		{
			name:     "multiline command",
			command:  "echo line1\necho line2\necho line3",
			cmdName:  "test-multiline",
			dir:      ".",
			env:      []string{},
			dryRun:   false,
			expected: "line1\nline2\nline3\n",
		},
		{
			name:    "shell level incremented",
			command: "echo $ATMOS_SHLVL",
			cmdName: "test-shlvl",
			dir:     ".",
			env:     []string{},
			dryRun:  false,
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "2")
			},
			expected: "3\n",
		},
		{
			name:    "max shell depth exceeded",
			command: "echo test",
			cmdName: "test-max-depth",
			dir:     ".",
			env:     []string{},
			dryRun:  false,
			setup: func() {
				os.Setenv("ATMOS_SHLVL", fmt.Sprintf("%d", MaxShellDepth))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests on certain OS if specified
			if len(tt.skipOnOS) > 0 {
				for _, skipOS := range tt.skipOnOS {
					if runtime.GOOS == skipOS {
						t.Skipf("Skipping test on %s", skipOS)
					}
				}
			}

			// Clear ATMOS_SHLVL before each test
			os.Unsetenv("ATMOS_SHLVL")

			if tt.setup != nil {
				tt.setup()
			}

			output, err := ExecuteShellAndReturnOutput(tt.command, tt.cmdName, tt.dir, tt.env, tt.dryRun)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Special handling for pwd command
				if tt.command == "pwd" {
					assert.Contains(t, output, tt.dir)
				} else if tt.expected != "" {
					assert.Equal(t, tt.expected, output)
				}
			}
		})
	}
}

func TestShellRunner(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		cmdName     string
		dir         string
		env         []string
		expected    string
		expectError bool
	}{
		{
			name:     "simple echo",
			command:  "echo hello world",
			cmdName:  "echo-test",
			dir:      ".",
			env:      []string{},
			expected: "hello world\n",
		},
		{
			name:     "command with variables",
			command:  "VAR=test; echo $VAR",
			cmdName:  "var-test",
			dir:      ".",
			env:      []string{},
			expected: "test\n",
		},
		{
			name:     "command with environment",
			command:  "echo $MY_ENV_VAR",
			cmdName:  "env-test",
			dir:      ".",
			env:      []string{"MY_ENV_VAR=from_environment"},
			expected: "from_environment\n",
		},
		{
			name:     "command with pipes",
			command:  "echo 'hello world' | tr ' ' '-'",
			cmdName:  "pipe-test",
			dir:      ".",
			env:      []string{},
			expected: "hello-world\n",
		},
		{
			name:     "command with conditionals",
			command:  "if [ 1 -eq 1 ]; then echo true; else echo false; fi",
			cmdName:  "conditional-test",
			dir:      ".",
			env:      []string{},
			expected: "true\n",
		},
		{
			name:        "syntax error",
			command:     "echo 'unclosed",
			cmdName:     "syntax-error-test",
			dir:         ".",
			env:         []string{},
			expectError: true,
		},
		{
			name:        "command failure",
			command:     "false",
			cmdName:     "failure-test",
			dir:         ".",
			env:         []string{},
			expectError: true,
		},
		{
			name:     "for loop",
			command:  "for i in 1 2 3; do echo $i; done",
			cmdName:  "loop-test",
			dir:      ".",
			env:      []string{},
			expected: "1\n2\n3\n",
		},
		{
			name:     "function definition and call",
			command:  "greet() { echo Hello $1; }; greet World",
			cmdName:  "function-test",
			dir:      ".",
			env:      []string{},
			expected: "Hello World\n",
		},
		{
			name:    "command substitution",
			command: "echo Today is $(date +%A 2>/dev/null || echo unknown)",
			cmdName: "substitution-test",
			dir:     ".",
			env:     []string{},
			// Output will vary, just check it doesn't error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := ShellRunner(tt.command, tt.cmdName, tt.dir, tt.env, &buf)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				output := buf.String()

				// For command substitution test, just check we got something
				if strings.Contains(tt.command, "date") {
					assert.NotEmpty(t, output)
				} else if tt.expected != "" {
					assert.Equal(t, tt.expected, output)
				}
			}
		})
	}
}

func TestGetNextShellLevelExtended(t *testing.T) {
	// Save original ATMOS_SHLVL
	origShellLevel := os.Getenv("ATMOS_SHLVL")
	defer os.Setenv("ATMOS_SHLVL", origShellLevel)

	tests := []struct {
		name        string
		setup       func()
		expected    int
		expectError bool
		errorType   error
	}{
		{
			name: "no existing shell level",
			setup: func() {
				os.Unsetenv("ATMOS_SHLVL")
			},
			expected: 1,
		},
		{
			name: "existing shell level 0",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "0")
			},
			expected: 1,
		},
		{
			name: "existing shell level 1",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "1")
			},
			expected: 2,
		},
		{
			name: "existing shell level 5",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "5")
			},
			expected: 6,
		},
		{
			name: "shell level at maximum",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", fmt.Sprintf("%d", MaxShellDepth-1))
			},
			expected: MaxShellDepth,
		},
		{
			name: "shell level exceeds maximum",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", fmt.Sprintf("%d", MaxShellDepth))
			},
			expectError: true,
			errorType:   ErrMaxShellDepthExceeded,
		},
		{
			name: "invalid shell level string",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "not_a_number")
			},
			expectError: true,
			errorType:   ErrConvertingShellLevel,
		},
		{
			name: "negative shell level",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "-1")
			},
			expected: 0, // -1 + 1 = 0
		},
		{
			name: "very large shell level",
			setup: func() {
				os.Setenv("ATMOS_SHLVL", "999")
			},
			expectError: true,
			errorType:   ErrMaxShellDepthExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			level, err := GetNextShellLevel()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestShellRunnerWithDifferentDirectories(t *testing.T) {
	// Create temporary directories for testing
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	// Create a test file in tmpDir1
	testFile := "test.txt"
	err := os.WriteFile(fmt.Sprintf("%s/%s", tmpDir1, testFile), []byte("test content"), 0o644)
	require.NoError(t, err)

	tests := []struct {
		name     string
		command  string
		dir      string
		expected func(string) bool
	}{
		{
			name:    "list files in tmpDir1",
			command: fmt.Sprintf("ls %s", testFile),
			dir:     tmpDir1,
			expected: func(output string) bool {
				return strings.Contains(output, testFile)
			},
		},
		{
			name:    "list files in tmpDir2 (empty)",
			command: fmt.Sprintf("ls %s 2>/dev/null || echo 'not found'", testFile),
			dir:     tmpDir2,
			expected: func(output string) bool {
				return strings.Contains(output, "not found")
			},
		},
		{
			name:    "pwd shows correct directory",
			command: "pwd",
			dir:     tmpDir1,
			expected: func(output string) bool {
				return strings.Contains(output, tmpDir1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := ShellRunner(tt.command, "dir-test", tt.dir, []string{}, &buf)
			assert.NoError(t, err)

			output := buf.String()
			assert.True(t, tt.expected(output), "Output: %s", output)
		})
	}
}

func TestExecuteShellWithComplexScripts(t *testing.T) {
	// Save and restore ATMOS_SHLVL
	origShellLevel := os.Getenv("ATMOS_SHLVL")
	defer os.Setenv("ATMOS_SHLVL", origShellLevel)
	os.Unsetenv("ATMOS_SHLVL")

	tests := []struct {
		name     string
		command  string
		env      []string
		expected string
	}{
		{
			name: "multi-line script with variables",
			command: `
				VAR1="hello"
				VAR2="world"
				echo "$VAR1 $VAR2"
			`,
			expected: "hello world\n",
		},
		{
			name: "script with functions",
			command: `
				add() {
					echo $(($1 + $2))
				}
				add 5 3
			`,
			expected: "8\n",
		},
		{
			name: "script with arrays",
			command: `
				arr=(one two three)
				for item in "${arr[@]}"; do
					echo $item
				done
			`,
			expected: "one\ntwo\nthree\n",
		},
		{
			name: "script with case statement",
			command: `
				VAR="apple"
				case $VAR in
					apple) echo "fruit";;
					carrot) echo "vegetable";;
					*) echo "unknown";;
				esac
			`,
			expected: "fruit\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ExecuteShellAndReturnOutput(tt.command, "complex-test", ".", tt.env, false)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, output)
		})
	}
}

func TestShellRunnerEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		expectError bool
	}{
		{
			name:    "empty command",
			command: "",
		},
		{
			name:    "whitespace only",
			command: "   \t\n   ",
		},
		{
			name:    "comment only",
			command: "# This is just a comment",
		},
		{
			name:    "multiple comments",
			command: "# Comment 1\n# Comment 2\n# Comment 3",
		},
		{
			name:        "unmatched quote",
			command:     "echo 'hello",
			expectError: true,
		},
		{
			name:        "unmatched parenthesis",
			command:     "echo $(echo hello",
			expectError: true,
		},
		{
			name:        "unmatched brace",
			command:     "if true; then echo hello",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := ShellRunner(tt.command, "edge-test", ".", []string{}, &buf)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
