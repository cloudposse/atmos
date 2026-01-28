//nolint:dupl // Test files contain similar setup code by design for isolation and clarity.
package skill

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestInstallCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "install <source>", installCmd.Use)
	assert.Equal(t, "Install a skill from a GitHub repository", installCmd.Short)
	assert.NotEmpty(t, installCmd.Long)
	assert.NotNil(t, installCmd.RunE)
}

func TestInstallCmd_Flags(t *testing.T) {
	t.Run("has force flag", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("force")
		require.NotNil(t, flag, "force flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has yes flag with shorthand", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("yes")
		require.NotNil(t, flag, "yes flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "y", flag.Shorthand)
	})
}

func TestInstallCmd_LongDescription(t *testing.T) {
	// Verify long description contains important information.
	assert.Contains(t, installCmd.Long, "Install a community-contributed skill")
	assert.Contains(t, installCmd.Long, "~/.atmos/skills/")
	assert.Contains(t, installCmd.Long, "agentskills.io")
	assert.Contains(t, installCmd.Long, "SKILL.md")
	assert.Contains(t, installCmd.Long, "github.com/user/repo")
	assert.Contains(t, installCmd.Long, "@v1.2.3")
}

func TestInstallCmd_ArgsValidation(t *testing.T) {
	// The command expects exactly 1 argument.
	assert.NotNil(t, installCmd.Args)

	// Test ExactArgs(1) validation.
	t.Run("rejects zero arguments", func(t *testing.T) {
		err := cobra.ExactArgs(1)(installCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s)")
	})

	t.Run("accepts exactly one argument", func(t *testing.T) {
		err := cobra.ExactArgs(1)(installCmd, []string{"github.com/user/repo"})
		assert.NoError(t, err)
	})

	t.Run("rejects two arguments", func(t *testing.T) {
		err := cobra.ExactArgs(1)(installCmd, []string{"arg1", "arg2"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s)")
	})
}

func TestInstallCmd_Examples(t *testing.T) {
	// Verify the long description contains examples.
	assert.Contains(t, installCmd.Long, "atmos ai skill install github.com/cloudposse/atmos-skill-terraform")
	assert.Contains(t, installCmd.Long, "--force")
	assert.Contains(t, installCmd.Long, "--yes")
}

func TestInstallCmd_SecuritySection(t *testing.T) {
	// Verify security information is documented.
	assert.Contains(t, installCmd.Long, "Security")
	assert.Contains(t, installCmd.Long, "cannot execute arbitrary code")
	assert.Contains(t, installCmd.Long, "prompted to confirm")
}

func TestInstallCmd_SourceFormats(t *testing.T) {
	// Verify all documented source formats are mentioned.
	assert.Contains(t, installCmd.Long, "github.com/user/repo")
	assert.Contains(t, installCmd.Long, "github.com/user/repo@v1.2.3")
	assert.Contains(t, installCmd.Long, "github.com/user/repo@branch")
	assert.Contains(t, installCmd.Long, "https://github.com/user/repo.git")
}

func TestInstallCmd_FlagParsing(t *testing.T) {
	// Reset flags after each test.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	t.Run("force flag defaults to false", func(t *testing.T) {
		resetFlags()
		force, err := installCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.False(t, force)
	})

	t.Run("force flag can be set to true", func(t *testing.T) {
		resetFlags()
		err := installCmd.Flags().Set("force", "true")
		require.NoError(t, err)
		force, err := installCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.True(t, force)
	})

	t.Run("yes flag defaults to false", func(t *testing.T) {
		resetFlags()
		yes, err := installCmd.Flags().GetBool("yes")
		require.NoError(t, err)
		assert.False(t, yes)
	})

	t.Run("yes flag can be set to true", func(t *testing.T) {
		resetFlags()
		err := installCmd.Flags().Set("yes", "true")
		require.NoError(t, err)
		yes, err := installCmd.Flags().GetBool("yes")
		require.NoError(t, err)
		assert.True(t, yes)
	})

	t.Run("yes flag shorthand y works", func(t *testing.T) {
		resetFlags()
		flag := installCmd.Flags().Lookup("yes")
		require.NotNil(t, flag)
		assert.Equal(t, "y", flag.Shorthand)
	})
}

func TestInstallCmd_FlagUsage(t *testing.T) {
	t.Run("force flag has usage description", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "Reinstall")
	})

	t.Run("yes flag has usage description", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("yes")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "confirmation")
	})
}

func TestInstallCmd_RunE_InvalidSource(t *testing.T) {
	// Reset flags before test.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	tests := []struct {
		name          string
		source        string
		expectError   bool
		errorContains string
	}{
		{
			name:          "invalid source format",
			source:        "invalid-source-format",
			expectError:   true,
			errorContains: "invalid skill source",
		},
		{
			name:          "missing repo in github shorthand",
			source:        "github.com/user",
			expectError:   true,
			errorContains: "invalid skill source",
		},
		{
			name:          "too many path parts",
			source:        "github.com/user/repo/extra/path",
			expectError:   true,
			errorContains: "invalid skill source",
		},
		{
			name:          "unsupported host",
			source:        "gitlab.com/user/repo",
			expectError:   true,
			errorContains: "invalid skill source",
		},
		{
			name:          "empty source",
			source:        "",
			expectError:   true,
			errorContains: "invalid skill source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()

			// Capture stdout to prevent output during tests.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run the command.
			err := installCmd.RunE(installCmd, []string{tt.source})

			w.Close()
			os.Stdout = oldStdout

			// Drain the pipe.
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInstallCmd_RunE_WithFlags(t *testing.T) {
	// Reset flags after test.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	t.Run("with force flag set", func(t *testing.T) {
		resetFlags()
		err := installCmd.Flags().Set("force", "true")
		require.NoError(t, err)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Use valid source format that will fail at download stage.
		err = installCmd.RunE(installCmd, []string{"github.com/nonexistent-user/nonexistent-repo"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// The command should proceed past flag parsing but fail at download.
		// This exercises the flag-getting code paths.
		assert.Error(t, err)
		// Should fail at download, not at flag parsing.
		assert.Contains(t, err.Error(), "download")
	})

	t.Run("with yes flag set", func(t *testing.T) {
		resetFlags()
		err := installCmd.Flags().Set("yes", "true")
		require.NoError(t, err)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Use valid source format.
		err = installCmd.RunE(installCmd, []string{"github.com/nonexistent-user/nonexistent-repo"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// The command should proceed past flag parsing but fail at download.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "download")
	})

	t.Run("with both flags set", func(t *testing.T) {
		resetFlags()
		err := installCmd.Flags().Set("force", "true")
		require.NoError(t, err)
		err = installCmd.Flags().Set("yes", "true")
		require.NoError(t, err)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Use valid source format.
		err = installCmd.RunE(installCmd, []string{"github.com/nonexistent-user/nonexistent-repo"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// The command should proceed past flag parsing but fail at download.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "download")
	})
}

func TestInstallCmd_RunE_ValidSourceFormats(t *testing.T) {
	// Reset flags before tests.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	// These tests verify that valid source formats are parsed correctly
	// and the command progresses to the download stage (where it will fail).
	validSources := []struct {
		name   string
		source string
	}{
		{
			name:   "github shorthand",
			source: "github.com/user/repo",
		},
		{
			name:   "github shorthand with version tag",
			source: "github.com/user/repo@v1.2.3",
		},
		{
			name:   "github shorthand with branch",
			source: "github.com/user/repo@main",
		},
		{
			name:   "https URL",
			source: "https://github.com/user/repo.git",
		},
		{
			name:   "https URL without .git suffix",
			source: "https://github.com/user/repo",
		},
	}

	for _, tt := range validSources {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()
			// Skip confirmation.
			err := installCmd.Flags().Set("yes", "true")
			require.NoError(t, err)

			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err = installCmd.RunE(installCmd, []string{tt.source})

			w.Close()
			os.Stdout = oldStdout

			// Drain the pipe.
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			// The command should proceed past source parsing.
			// It will fail at download since the repo doesn't exist.
			assert.Error(t, err)
			// Should fail at download, not at source parsing.
			assert.Contains(t, err.Error(), "download")
		})
	}
}

func TestInstallCmd_CommandRegistration(t *testing.T) {
	// Verify the command is properly registered.
	assert.NotNil(t, installCmd)
	assert.NotNil(t, installCmd.RunE)
	assert.NotNil(t, installCmd.Flags())
}

func TestInstallCmd_OutputDuringInstall(t *testing.T) {
	// Reset flags.
	forceFlag := installCmd.Flags().Lookup("force")
	if forceFlag != nil {
		_ = forceFlag.Value.Set("false")
	}
	yesFlag := installCmd.Flags().Lookup("yes")
	if yesFlag != nil {
		_ = yesFlag.Value.Set("false")
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run with valid source format.
	_ = installCmd.RunE(installCmd, []string{"github.com/cloudposse/test-skill"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Should print "Downloading skill from..." message.
	assert.Contains(t, output, "Downloading skill from")
}

func TestInstallCmd_RunENotNil(t *testing.T) {
	require.NotNil(t, installCmd.RunE)
}

func TestInstallCmd_UseFieldCorrect(t *testing.T) {
	assert.Equal(t, "install <source>", installCmd.Use)
}

func TestInstallCmd_ShortDescription(t *testing.T) {
	assert.Equal(t, "Install a skill from a GitHub repository", installCmd.Short)
}

func TestInstallCmd_LongDescriptionContainsExamples(t *testing.T) {
	assert.Contains(t, installCmd.Long, "Examples:")
}

func TestInstallCmd_LongDescriptionContainsSourceFormats(t *testing.T) {
	assert.Contains(t, installCmd.Long, "Source formats:")
}

func TestInstallCmd_LongDescriptionContainsSecurityInfo(t *testing.T) {
	assert.Contains(t, installCmd.Long, "Security:")
}

func TestInstallCmd_ValidatesFlagTypes(t *testing.T) {
	// Test that invalid flag values are rejected.
	t.Run("force flag rejects non-boolean", func(t *testing.T) {
		err := installCmd.Flags().Set("force", "invalid")
		assert.Error(t, err)
	})

	t.Run("yes flag rejects non-boolean", func(t *testing.T) {
		err := installCmd.Flags().Set("yes", "invalid")
		assert.Error(t, err)
	})
}

func TestInstallCmd_FlagDefaults(t *testing.T) {
	// Verify default values by checking DefValue.
	t.Run("force default is false", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("yes default is false", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("yes")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestInstallCmd_RunE_InstallerInitFailure(t *testing.T) {
	// Reset flags before test.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	t.Run("fails when home directory is unwritable", func(t *testing.T) {
		resetFlags()

		// Create a temp directory and make it unwritable.
		tempHome := t.TempDir()

		// Create .atmos/skills as a file (not a directory) to cause write failure.
		atmosDir := filepath.Join(tempHome, ".atmos")
		err := os.MkdirAll(atmosDir, 0o755)
		require.NoError(t, err)

		// Create skills as a file, not a directory, to cause registry creation to fail.
		skillsFile := filepath.Join(atmosDir, "skills")
		err = os.WriteFile(skillsFile, []byte("not a directory"), 0o644)
		require.NoError(t, err)

		// Set HOME to temp directory.
		t.Setenv("HOME", tempHome)

		// Reset homedir cache to pick up new HOME.
		homedir.Reset()
		homedir.DisableCache = true
		t.Cleanup(func() {
			homedir.Reset()
			homedir.DisableCache = false
		})

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Run the command - should fail during installer initialization.
		err = installCmd.RunE(installCmd, []string{"github.com/user/repo"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// Verify we get an error about initialization.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize installer")
	})
}

func TestInstallCmd_RunE_ContextUsage(t *testing.T) {
	// Reset flags before test.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	t.Run("uses context in installer", func(t *testing.T) {
		resetFlags()
		_ = installCmd.Flags().Set("yes", "true")

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Run with a valid source - it will fail at download but shows context is used.
		err := installCmd.RunE(installCmd, []string{"github.com/nonexistent/repo@v1.0.0"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		assert.Error(t, err)
		// Verify output shows downloading started.
		assert.Contains(t, buf.String(), "Downloading skill from")
	})
}

func TestInstallCmd_RunE_AllFlagCombinations(t *testing.T) {
	// Reset flags helper.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	tests := []struct {
		name  string
		force bool
		yes   bool
	}{
		{"neither flag", false, false},
		{"force only", true, false},
		{"yes only", false, true},
		{"both flags", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()

			if tt.force {
				require.NoError(t, installCmd.Flags().Set("force", "true"))
			}
			if tt.yes {
				require.NoError(t, installCmd.Flags().Set("yes", "true"))
			}

			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run with valid source.
			err := installCmd.RunE(installCmd, []string{"github.com/test-user/test-repo"})

			w.Close()
			os.Stdout = oldStdout

			// Drain the pipe.
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			// All should fail at download.
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "download")
		})
	}
}

func TestInstallCmd_RunE_InstallOptionsPassthrough(t *testing.T) {
	// Reset flags before test.
	forceFlag := installCmd.Flags().Lookup("force")
	if forceFlag != nil {
		_ = forceFlag.Value.Set("true")
	}
	yesFlag := installCmd.Flags().Lookup("yes")
	if yesFlag != nil {
		_ = yesFlag.Value.Set("true")
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the command.
	err := installCmd.RunE(installCmd, []string{"github.com/cloudposse/test-skill@v2.0.0"})

	w.Close()
	os.Stdout = oldStdout

	// Drain the pipe.
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	// Should proceed to download stage.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "download")

	// Verify download message was printed.
	output := buf.String()
	assert.Contains(t, output, "Downloading skill from")
}

// TestInstallCmd_RunE_SuccessfulInstall tests the full successful install path.
// This is more of an integration test since it sets up a mock download.
// Note: This test requires mocking the downloader which is complex, so we focus
// on testing other aspects of the RunE function.
func TestInstallCmd_RunE_FlagParsingSuccess(t *testing.T) {
	// Reset flags helper.
	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	t.Run("force flag parsing succeeds", func(t *testing.T) {
		resetFlags()

		// Verify GetBool works correctly.
		force, err := installCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.False(t, force)

		// Set to true and verify.
		require.NoError(t, installCmd.Flags().Set("force", "true"))

		force, err = installCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.True(t, force)
	})

	t.Run("yes flag parsing succeeds", func(t *testing.T) {
		resetFlags()

		// Verify GetBool works correctly.
		yes, err := installCmd.Flags().GetBool("yes")
		require.NoError(t, err)
		assert.False(t, yes)

		// Set to true and verify.
		require.NoError(t, installCmd.Flags().Set("yes", "true"))

		yes, err = installCmd.Flags().GetBool("yes")
		require.NoError(t, err)
		assert.True(t, yes)
	})
}

func TestInstallCmd_RunE_SuccessPathCoverage(t *testing.T) {
	// This test exercises the path up to the point where installer.Install is called.
	// Since installer.Install requires network access to download the skill,
	// the successful return path (line 86) cannot be covered without mocking.

	resetFlags := func() {
		forceFlag := installCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
		yesFlag := installCmd.Flags().Lookup("yes")
		if yesFlag != nil {
			_ = yesFlag.Value.Set("false")
		}
	}

	t.Run("successful flag parsing leads to install attempt", func(t *testing.T) {
		resetFlags()
		require.NoError(t, installCmd.Flags().Set("force", "true"))
		require.NoError(t, installCmd.Flags().Set("yes", "true"))

		// Verify flags are correctly parsed.
		force, err := installCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.True(t, force)

		yes, err := installCmd.Flags().GetBool("yes")
		require.NoError(t, err)
		assert.True(t, yes)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Run the command - it will fail at download, but exercises all code paths.
		err = installCmd.RunE(installCmd, []string{"github.com/testorg/testskill"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// Verify it proceeded past flag parsing and installer creation.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "download")
	})
}

// TestInstallCmd_RunE_MissingFlags tests the error paths when flags are missing.
// These paths (lines 61, 66) are normally unreachable because flags are registered
// in init(). To test them, we create a fresh command without flags.
func TestInstallCmd_RunE_MissingFlags(t *testing.T) {
	t.Run("missing force flag returns error", func(t *testing.T) {
		// Create a fresh command without the force flag.
		cmd := &cobra.Command{
			Use:  "install <source>",
			Args: cobra.ExactArgs(1),
			RunE: installCmd.RunE, // Reuse the same RunE function.
		}
		// Only add the yes flag, not force.
		cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := cmd.RunE(cmd, []string{"github.com/user/repo"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// Should fail when trying to get the missing force flag.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get --force flag")
	})

	t.Run("missing yes flag returns error", func(t *testing.T) {
		// Create a fresh command without the yes flag.
		cmd := &cobra.Command{
			Use:  "install <source>",
			Args: cobra.ExactArgs(1),
			RunE: installCmd.RunE, // Reuse the same RunE function.
		}
		// Only add the force flag, not yes.
		cmd.Flags().Bool("force", false, "Reinstall if skill is already installed")

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := cmd.RunE(cmd, []string{"github.com/user/repo"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// Should fail when trying to get the missing yes flag.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get --yes flag")
	})
}
