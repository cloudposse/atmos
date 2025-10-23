package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuth_EnvCommand_E2E tests the complete workflow of logging in and
// using the auth env command to retrieve environment variables.
func TestAuth_EnvCommand_E2E(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", filepath.Join(tempDir, "keyring.json"))
	tk.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-for-file-keyring")

	// Step 1: Login to cache credentials.
	t.Run("login", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
		err := RootCmd.Execute()
		require.NoError(t, err, "Login should succeed")
	})

	// Step 2: Test auth env with different output formats.
	formats := []struct {
		name           string
		format         string
		expectedOutput []string
	}{
		{
			name:   "json format",
			format: "json",
			expectedOutput: []string{
				`"AWS_PROFILE"`,
				`"mock-identity"`,
				`"AWS_CONFIG_FILE"`,
			},
		},
		{
			name:   "bash format",
			format: "bash",
			expectedOutput: []string{
				"export AWS_PROFILE=",
				"export AWS_CONFIG_FILE=",
				"mock-identity",
			},
		},
		{
			name:   "dotenv format",
			format: "dotenv",
			expectedOutput: []string{
				"AWS_PROFILE=",
				"AWS_CONFIG_FILE=",
				"mock-identity",
			},
		},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Capture stdout to verify output.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			RootCmd.SetArgs([]string{"auth", "env", "--format", tc.format, "--identity", "mock-identity"})

			start := time.Now()
			err := RootCmd.Execute()
			duration := time.Since(start)

			// Restore stdout.
			w.Close()
			os.Stdout = oldStdout

			// Read captured output.
			var buf [4096]byte
			n, _ := r.Read(buf[:])
			output := string(buf[:n])

			require.NoError(t, err, "Auth env should succeed with cached credentials")

			// Verify output contains expected content.
			for _, expected := range tc.expectedOutput {
				assert.Contains(t, output, expected,
					"Output should contain %q", expected)
			}

			// Verify fast execution (using cached credentials).
			assert.Less(t, duration, 2*time.Second,
				"Auth env with cached credentials should be fast")

			t.Logf("auth env --format %s completed in %v", tc.format, duration)
			t.Logf("Output:\n%s", output)
		})
	}
}

// TestAuth_ExecCommand_E2E tests the complete workflow of logging in and
// using the auth exec command to run commands with authenticated environment.
func TestAuth_ExecCommand_E2E(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", filepath.Join(tempDir, "keyring.json"))
	tk.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-for-file-keyring")

	// Step 1: Login to cache credentials.
	t.Run("login", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
		err := RootCmd.Execute()
		require.NoError(t, err, "Login should succeed")
	})

	// Step 2: Test auth exec with various commands.
	testCases := []struct {
		name           string
		command        []string
		expectedOutput string
	}{
		{
			name:           "echo command",
			command:        []string{"echo", "hello"},
			expectedOutput: "hello",
		},
		{
			name:           "print env var",
			command:        []string{"sh", "-c", "echo $AWS_PROFILE"},
			expectedOutput: "mock-identity",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Build args: auth exec --identity mock-identity -- <command>.
			args := []string{"auth", "exec", "--identity", "mock-identity", "--"}
			args = append(args, tc.command...)

			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			RootCmd.SetArgs(args)

			start := time.Now()
			err := RootCmd.Execute()
			duration := time.Since(start)

			// Restore stdout.
			w.Close()
			os.Stdout = oldStdout

			// Read captured output.
			var buf [4096]byte
			n, _ := r.Read(buf[:])
			output := string(buf[:n])

			require.NoError(t, err, "Auth exec should succeed")

			// Verify command output.
			assert.Contains(t, output, tc.expectedOutput,
				"Command output should contain expected text")

			// Verify fast execution.
			assert.Less(t, duration, 2*time.Second,
				"Auth exec with cached credentials should be fast")

			t.Logf("auth exec %v completed in %v", tc.command, duration)
			t.Logf("Output:\n%s", output)
		})
	}
}

// TestAuth_WhoamiCommand_E2E tests the complete workflow of logging in and
// using the auth whoami command to retrieve identity information.
func TestAuth_WhoamiCommand_E2E(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", filepath.Join(tempDir, "keyring.json"))
	tk.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-for-file-keyring")

	// Step 1: Login to cache credentials.
	t.Run("login", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
		err := RootCmd.Execute()
		require.NoError(t, err, "Login should succeed")
	})

	// Step 2: Test whoami command.
	t.Run("whoami with cached credentials", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "whoami", "--identity", "mock-identity"})

		start := time.Now()
		err := RootCmd.Execute()
		duration := time.Since(start)

		require.NoError(t, err, "Whoami should succeed with cached credentials")

		// Verify fast execution.
		assert.Less(t, duration, 2*time.Second,
			"Whoami with cached credentials should be fast")

		t.Logf("auth whoami completed in %v", duration)
	})

	// Step 3: Test whoami with JSON output.
	t.Run("whoami json output", func(t *testing.T) {
		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		RootCmd.SetArgs([]string{"auth", "whoami", "--identity", "mock-identity", "--output", "json"})

		err := RootCmd.Execute()

		// Restore stdout.
		w.Close()
		os.Stdout = oldStdout

		// Read captured output.
		var buf [4096]byte
		n, _ := r.Read(buf[:])
		output := string(buf[:n])

		require.NoError(t, err, "Whoami with JSON output should succeed")

		// Verify JSON output contains expected fields.
		assert.Contains(t, output, `"provider"`, "JSON should contain provider field")
		assert.Contains(t, output, `"identity"`, "JSON should contain identity field")
		assert.Contains(t, output, `"mock-identity"`, "JSON should contain identity name")
		assert.Contains(t, output, `"region"`, "JSON should contain region field")
		assert.Contains(t, output, `"us-east-1"`, "JSON should contain region value")

		t.Logf("JSON output:\n%s", output)
	})
}

// TestAuth_ShellCommand_E2E is skipped because 'auth shell' launches an interactive shell
// rather than printing shell export commands. Testing interactive shells requires
// different test infrastructure (PTY simulation).
// The auth env command should be used for generating shell export statements.
func TestAuth_ShellCommand_E2E(t *testing.T) {
	t.Skip("auth shell launches interactive shell, not suitable for automated testing")
}

// TestAuth_CompleteWorkflow_E2E tests a complete realistic workflow that
// mimics what a user would do: login, check whoami, get env vars, run command.
func TestAuth_CompleteWorkflow_E2E(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", filepath.Join(tempDir, "keyring.json"))
	tk.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-for-file-keyring")

	workflow := []struct {
		name string
		args []string
	}{
		{"login", []string{"auth", "login", "--identity", "mock-identity"}},
		{"whoami", []string{"auth", "whoami", "--identity", "mock-identity"}},
		{"env", []string{"auth", "env", "--identity", "mock-identity"}},
		{"list", []string{"auth", "list"}},
		{"validate", []string{"auth", "validate"}},
	}

	totalDuration := time.Duration(0)

	for i, step := range workflow {
		t.Run(step.name, func(t *testing.T) {
			RootCmd.SetArgs(step.args)

			start := time.Now()
			err := RootCmd.Execute()
			duration := time.Since(start)

			totalDuration += duration

			require.NoError(t, err, "Step %d (%s) should succeed", i+1, step.name)

			// After first step (login), subsequent steps should be fast.
			if i > 0 {
				assert.Less(t, duration, 2*time.Second,
					"Step %d (%s) should be fast with cached credentials", i+1, step.name)
			}

			t.Logf("Step %d: %s completed in %v", i+1, strings.Join(step.args, " "), duration)
		})
	}

	t.Logf("Complete workflow took %v", totalDuration)

	// Entire workflow should complete quickly.
	assert.Less(t, totalDuration, 10*time.Second,
		"Complete workflow should be fast")
}

// TestAuth_MultipleIdentities_E2E tests using multiple commands with a cached identity.
func TestAuth_MultipleIdentities_E2E(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", filepath.Join(tempDir, "keyring.json"))
	tk.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-for-file-keyring")

	// Login to identity.
	t.Run("login", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
		err := RootCmd.Execute()
		require.NoError(t, err, "Login should succeed")
	})

	// Verify identity works with whoami.
	t.Run("whoami uses cached credentials", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "whoami", "--identity", "mock-identity"})
		start := time.Now()
		err := RootCmd.Execute()
		duration := time.Since(start)
		require.NoError(t, err, "Whoami should succeed with cached credentials")
		assert.Less(t, duration, 2*time.Second, "Whoami should be fast with cached credentials")
	})

	// Verify identity works with env.
	t.Run("env uses cached credentials", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "env", "--identity", "mock-identity"})
		start := time.Now()
		err := RootCmd.Execute()
		duration := time.Since(start)
		require.NoError(t, err, "Env should succeed with cached credentials")
		assert.Less(t, duration, 2*time.Second, "Env should be fast with cached credentials")
	})
}
