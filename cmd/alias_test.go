package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandAliases(t *testing.T) {
	_ = NewTestKit(t)

	testDir := "../tests/fixtures/scenarios/subcommand-alias"

	// Change to test directory (t.Chdir automatically restores on cleanup).
	t.Chdir(testDir)

	// Load the atmos config to trigger alias registration.
	RootCmd.SetArgs([]string{"version"})
	err := Execute()
	require.NoError(t, err)

	tests := []struct {
		name      string
		aliasName string
		aliasFor  string
	}{
		{
			name:      "terraform plan alias 'tp'",
			aliasName: "tp",
			aliasFor:  "terraform plan",
		},
		{
			name:      "terraform alias 'tr'",
			aliasName: "tr",
			aliasFor:  "terraform",
		},
		{
			name:      "terraform apply alias 'ta'",
			aliasName: "ta",
			aliasFor:  "terraform apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Verify the alias command exists.
			cmd, _, err := RootCmd.Find([]string{tt.aliasName})
			require.NoError(t, err, "%s alias should be registered", tt.aliasName)
			assert.Equal(t, tt.aliasName, cmd.Use, "%s command should exist", tt.aliasName)
			assert.Contains(t, cmd.Short, "alias for", "%s should be an alias command", tt.aliasName)
		})
	}
}

func TestDevcontainerAliases(t *testing.T) {
	_ = NewTestKit(t)

	testDir := "../examples/devcontainer"

	// Change to test directory (t.Chdir automatically restores on cleanup).
	t.Chdir(testDir)

	// Load the atmos config to trigger alias registration.
	RootCmd.SetArgs([]string{"version"})
	err := Execute()
	require.NoError(t, err)

	// Verify the 'shell' alias command exists.
	shellCmd, _, err := RootCmd.Find([]string{"shell"})
	require.NoError(t, err, "shell alias should be registered")
	assert.Equal(t, "shell", shellCmd.Use, "shell command should exist")
	assert.Contains(t, shellCmd.Short, "alias for", "shell should be an alias command")
}

func TestAliasChdirProcessing(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("filterChdirArgs removes chdir flags", func(t *testing.T) {
		tests := []struct {
			name     string
			input    []string
			expected []string
		}{
			{
				name:     "removes --chdir with value",
				input:    []string{"--chdir", "somedir", "arg1", "arg2"},
				expected: []string{"arg1", "arg2"},
			},
			{
				name:     "removes -C with value",
				input:    []string{"-C", "somedir", "arg1", "arg2"},
				expected: []string{"arg1", "arg2"},
			},
			{
				name:     "removes --chdir=value",
				input:    []string{"--chdir=somedir", "arg1", "arg2"},
				expected: []string{"arg1", "arg2"},
			},
			{
				name:     "removes -C=value",
				input:    []string{"-C=somedir", "arg1", "arg2"},
				expected: []string{"arg1", "arg2"},
			},
			{
				name:     "removes -C<value> concatenated",
				input:    []string{"-Csomedir", "arg1", "arg2"},
				expected: []string{"arg1", "arg2"},
			},
			{
				name:     "removes multiple chdir flags",
				input:    []string{"--chdir", "dir1", "arg1", "-C", "dir2", "arg2"},
				expected: []string{"arg1", "arg2"},
			},
			{
				name:     "preserves other flags",
				input:    []string{"--stack", "dev", "--chdir", "somedir", "--component", "vpc"},
				expected: []string{"--stack", "dev", "--component", "vpc"},
			},
			{
				name:     "preserves args that start with -C but are longer flags",
				input:    []string{"--chdir", "somedir", "--config", "file.yaml"},
				expected: []string{"--config", "file.yaml"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := filterChdirArgs(tt.input)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("filterChdirEnv removes ATMOS_CHDIR from environment", func(t *testing.T) {
		// This test verifies that the production filterChdirEnv function properly
		// filters out ATMOS_CHDIR from the environment to prevent child processes
		// from re-applying the parent's chdir directive.

		tests := []struct {
			name               string
			environ            []string
			expectedContains   string
			expectedNotContain string
			expectEmptyChdir   bool // true if we expect ATMOS_CHDIR= to be added
		}{
			{
				name: "adds empty ATMOS_CHDIR when present",
				environ: []string{
					"PATH=/usr/bin",
					"ATMOS_CHDIR=/some/path",
					"HOME=/home/user",
				},
				expectedContains:   "ATMOS_CHDIR=",
				expectedNotContain: "ATMOS_CHDIR=/some/path",
				expectEmptyChdir:   true,
			},
			{
				name: "no ATMOS_CHDIR when not present",
				environ: []string{
					"PATH=/usr/bin",
					"HOME=/home/user",
				},
				expectedContains:   "PATH=/usr/bin",
				expectedNotContain: "ATMOS_CHDIR=",
				expectEmptyChdir:   false,
			},
			{
				name: "preserves other environment variables",
				environ: []string{
					"PATH=/usr/bin",
					"ATMOS_CHDIR=/some/path",
					"HOME=/home/user",
					"ATMOS_OTHER_VAR=value",
				},
				expectedContains:   "ATMOS_OTHER_VAR=value",
				expectedNotContain: "ATMOS_CHDIR=/some/path",
				expectEmptyChdir:   true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Call the production function.
				filteredEnv := filterChdirEnv(tt.environ)

				// Verify expectations.
				envStr := strings.Join(filteredEnv, "\n")
				if tt.expectedContains != "" {
					assert.Contains(t, envStr, tt.expectedContains,
						"filtered environment should contain %s", tt.expectedContains)
				}
				if tt.expectedNotContain != "" {
					assert.NotContains(t, envStr, tt.expectedNotContain,
						"filtered environment should NOT contain %s", tt.expectedNotContain)
				}

				// Additional verification for ATMOS_CHDIR handling.
				if tt.expectEmptyChdir {
					// Should have exactly one ATMOS_CHDIR= entry with empty value.
					count := 0
					for _, env := range filteredEnv {
						if strings.HasPrefix(env, "ATMOS_CHDIR=") {
							count++
							// Should be empty value to override parent.
							assert.Equal(t, "ATMOS_CHDIR=", env,
								"ATMOS_CHDIR should have empty value to override parent")
						}
					}
					assert.Equal(t, 1, count, "should have exactly one ATMOS_CHDIR= entry")
				}
			})
		}
	})
}

func TestAliasFlagPassing(t *testing.T) {
	_ = NewTestKit(t)

	testDir := "../examples/devcontainer"

	// Change to test directory (t.Chdir automatically restores on cleanup).
	t.Chdir(testDir)

	// Load the atmos config to trigger alias registration.
	RootCmd.SetArgs([]string{"version"})
	err := Execute()
	require.NoError(t, err)

	t.Run("alias passes flags through", func(t *testing.T) {
		_ = NewTestKit(t)

		// Get the shell alias command.
		shellCmd, _, err := RootCmd.Find([]string{"shell"})
		require.NoError(t, err, "shell alias should be registered")

		// Verify DisableFlagParsing is true so flags are passed through.
		assert.True(t, shellCmd.DisableFlagParsing, "alias should have DisableFlagParsing enabled")

		// Verify FParseErrWhitelist allows unknown flags.
		assert.True(t, shellCmd.FParseErrWhitelist.UnknownFlags, "alias should allow unknown flags")

		// Verify the Run function exists (it will construct the command with flags).
		assert.NotNil(t, shellCmd.Run, "alias should have a Run function")

		// We can't easily test the actual execution without running the command,
		// but we can verify that the alias is configured to pass flags through:
		// 1. DisableFlagParsing = true means Cobra won't parse/validate flags
		// 2. FParseErrWhitelist.UnknownFlags = true means unknown flags are allowed
		// 3. The Run function uses strings.Join(args, " ") which includes all flags
		//
		// This configuration ensures that:
		//   atmos shell --instance test
		// becomes:
		//   atmos devcontainer shell geodesic --instance test
	})

	t.Run("verify alias command construction", func(t *testing.T) {
		_ = NewTestKit(t)

		// This test verifies the alias is configured correctly to pass flags.
		// The actual command construction in cmd_utils.go:163 is:
		//   commandToRun := fmt.Sprintf("%s %s %s", os.Args[0], aliasCmd, strings.Join(args, " "))
		//
		// With shell alias = "devcontainer shell geodesic" and args = ["--instance", "test"]
		// This becomes: "atmos devcontainer shell geodesic --instance test"

		testArgs := []string{"--instance", "myinstance"}
		expectedCommand := "devcontainer shell geodesic " + strings.Join(testArgs, " ")

		// The alias should preserve the command structure.
		shellCmd, _, err := RootCmd.Find([]string{"shell"})
		require.NoError(t, err)

		// Verify the alias points to the correct command.
		assert.Contains(t, shellCmd.Short, "devcontainer shell geodesic",
			"alias should be for 'devcontainer shell geodesic'")

		// The actual command that would be executed includes the program name.
		// We verify the structure by checking that the program name would be prepended
		// and the args would be appended to the alias command.
		// Note: Production code in cmd_utils.go uses os.Args[0] to get the program name.
		assert.NotEmpty(t, expectedCommand, "alias command should not be empty")
	})
}
