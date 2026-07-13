//nolint:dupl // Test files contain similar setup code by design for isolation and clarity.
package skill

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestUninstallCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "uninstall [name]", uninstallCmd.Use)
	assert.Equal(t, "Remove an installed skill", uninstallCmd.Short)
	assert.NotEmpty(t, uninstallCmd.Long)
	assert.NotNil(t, uninstallCmd.RunE)
}

func TestUninstallCmd_Flags(t *testing.T) {
	t.Run("has force flag with shorthand", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("force")
		require.NotNil(t, flag, "force flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "f", flag.Shorthand)
	})

	t.Run("has client flag with shorthand", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("client")
		require.NotNil(t, flag, "client flag should be registered")
		assert.Equal(t, "stringSlice", flag.Value.Type())
		assert.Equal(t, "c", flag.Shorthand)
	})

	t.Run("has all-clients flag", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("all-clients")
		require.NotNil(t, flag, "all-clients flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has scope flag", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("scope")
		require.NotNil(t, flag, "scope flag should be registered")
		assert.Equal(t, "string", flag.Value.Type())
		assert.Equal(t, "project", flag.DefValue)
	})

	t.Run("has global flag with shorthand", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("global")
		require.NotNil(t, flag, "global flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "g", flag.Shorthand)
	})
}

func TestUninstallCmd_LongDescription(t *testing.T) {
	// Verify long description contains important information.
	assert.Contains(t, uninstallCmd.Long, "Uninstall a community-contributed skill")
	assert.Contains(t, uninstallCmd.Long, "~/.atmos/skills/")
	assert.Contains(t, uninstallCmd.Long, "registry entry")
	assert.Contains(t, uninstallCmd.Long, "prompted to confirm")
	assert.Contains(t, uninstallCmd.Long, "--force")
}

func TestUninstallCmd_ArgsValidation(t *testing.T) {
	// The command expects exactly 1 argument.
	assert.NotNil(t, uninstallCmd.Args)
}

func TestUninstallCmd_Examples(t *testing.T) {
	// Verify the Example field contains usage examples.
	assert.Contains(t, uninstallCmd.Example, "atmos ai skill uninstall terraform-optimizer")
	assert.Contains(t, uninstallCmd.Example, "--force")
}

func TestUninstallCmd_ReferencesListCommand(t *testing.T) {
	// Verify it references the list command for finding skill names.
	assert.Contains(t, uninstallCmd.Long, "atmos ai skill list")
}

func TestUninstallCmd_ArgsValidation_MaximumNArgs(t *testing.T) {
	// Test MaximumNArgs(1) validation: an omitted <name> means "uninstall
	// everything", so zero args must be accepted, but more than one is still
	// rejected.
	t.Run("accepts zero arguments (uninstall all)", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(uninstallCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("accepts exactly one argument", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(uninstallCmd, []string{"my-skill"})
		assert.NoError(t, err)
	})

	t.Run("rejects two arguments", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(uninstallCmd, []string{"arg1", "arg2"})
		assert.Error(t, err)
	})

	t.Run("rejects multiple arguments", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(uninstallCmd, []string{"arg1", "arg2", "arg3"})
		assert.Error(t, err)
	})
}

func TestUninstallCmd_FlagParsing(t *testing.T) {
	// Reset flags after each test.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	t.Run("force flag defaults to false", func(t *testing.T) {
		resetFlags()
		force, err := uninstallCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.False(t, force)
	})

	t.Run("force flag can be set to true", func(t *testing.T) {
		resetFlags()
		err := uninstallCmd.Flags().Set("force", "true")
		require.NoError(t, err)
		force, err := uninstallCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.True(t, force)
	})

	t.Run("force flag shorthand f works", func(t *testing.T) {
		resetFlags()
		flag := uninstallCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand)
	})
}

func TestUninstallCmd_FlagUsage(t *testing.T) {
	t.Run("force flag has usage description", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "confirmation")
	})

	t.Run("client flag has usage description", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("client")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "AI client")
	})

	t.Run("all-clients flag has usage description", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("all-clients")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "AI clients")
	})

	t.Run("scope flag has usage description", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("scope")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "Distribution scope")
	})

	t.Run("global flag has usage description", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("global")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "--scope user")
	})
}

func TestUninstallCmd_ValidatesFlagTypes(t *testing.T) {
	// Test that invalid flag values are rejected.
	t.Run("force flag rejects non-boolean", func(t *testing.T) {
		err := uninstallCmd.Flags().Set("force", "invalid")
		assert.Error(t, err)
	})
}

func TestUninstallCmd_FlagDefaults(t *testing.T) {
	// Verify default values by checking DefValue.
	t.Run("force default is false", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestUninstallCmd_RunE_NonexistentSkill(t *testing.T) {
	// Reset flags before test.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	tests := []struct {
		name          string
		skillName     string
		force         bool
		expectError   bool
		errorContains string
	}{
		{
			name:          "nonexistent skill without force",
			skillName:     "nonexistent-skill-abc123",
			force:         false,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "nonexistent skill with force flag",
			skillName:     "another-nonexistent-skill",
			force:         true,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "skill name with special characters",
			skillName:     "skill-with-special-chars-!@#",
			force:         true,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "empty-like skill name",
			skillName:     "   ",
			force:         true,
			expectError:   true,
			errorContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()

			if tt.force {
				err := uninstallCmd.Flags().Set("force", "true")
				require.NoError(t, err)
			}

			// Capture stdout to prevent output during tests.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run the command.
			err := uninstallCmd.RunE(uninstallCmd, []string{tt.skillName})

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

func TestUninstallCmd_RunE_WithForceFlag(t *testing.T) {
	// Reset flags.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	t.Run("with force flag set to true", func(t *testing.T) {
		resetFlags()
		err := uninstallCmd.Flags().Set("force", "true")
		require.NoError(t, err)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Try to uninstall a nonexistent skill.
		err = uninstallCmd.RunE(uninstallCmd, []string{"nonexistent-skill-test"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// The command should fail because the skill doesn't exist.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("with force flag set to false (default)", func(t *testing.T) {
		resetFlags()

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Try to uninstall a nonexistent skill.
		err := uninstallCmd.RunE(uninstallCmd, []string{"nonexistent-skill-test2"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// The command should fail because the skill doesn't exist.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestUninstallCmd_CommandRegistration(t *testing.T) {
	// Verify the command is properly registered.
	assert.NotNil(t, uninstallCmd)
	assert.NotNil(t, uninstallCmd.RunE)
	assert.NotNil(t, uninstallCmd.Flags())
}

func TestUninstallCmd_RunENotNil(t *testing.T) {
	require.NotNil(t, uninstallCmd.RunE)
}

func TestUninstallCmd_UseFieldCorrect(t *testing.T) {
	assert.Equal(t, "uninstall [name]", uninstallCmd.Use)
}

func TestUninstallCmd_ShortDescription(t *testing.T) {
	assert.Equal(t, "Remove an installed skill", uninstallCmd.Short)
}

func TestUninstallCmd_LongDescriptionContainsExamples(t *testing.T) {
	// Examples are in the Example field, not Long.
	assert.NotEmpty(t, uninstallCmd.Example, "Example field should contain usage examples")
	assert.Contains(t, uninstallCmd.Example, "atmos ai skill uninstall")
}

func TestUninstallCmd_LongDescriptionContainsInstallationPath(t *testing.T) {
	// Verify long description mentions the installation path.
	assert.Contains(t, uninstallCmd.Long, "~/.atmos/skills/")
}

func TestUninstallCmd_LongDescriptionContainsForceOption(t *testing.T) {
	// Verify the Example field mentions the force option.
	assert.Contains(t, uninstallCmd.Example, "Force uninstall")
}

func TestUninstallCmd_LongDescriptionContainsPromptInfo(t *testing.T) {
	// Verify long description mentions confirmation prompt.
	assert.Contains(t, uninstallCmd.Long, "prompted to confirm")
}

func TestUninstallCmd_FlagShorthand(t *testing.T) {
	// Verify the force flag has the correct shorthand.
	flag := uninstallCmd.Flags().Lookup("force")
	require.NotNil(t, flag)
	assert.Equal(t, "f", flag.Shorthand)
}

func TestUninstallCmd_RunE_FlagParsingInFunction(t *testing.T) {
	// This test verifies that the flag parsing logic in RunE works correctly.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	t.Run("flag parsing succeeds with valid flag value", func(t *testing.T) {
		resetFlags()

		// Set a valid flag value.
		err := uninstallCmd.Flags().Set("force", "true")
		require.NoError(t, err)

		// Verify the flag was set correctly.
		force, err := uninstallCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.True(t, force)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Run the command - it should parse the flag successfully.
		// but fail at skill lookup.
		err = uninstallCmd.RunE(uninstallCmd, []string{"test-skill"})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		// The error should be about skill not found, not about flag parsing.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestUninstallCmd_SkillNameValidation(t *testing.T) {
	// Test various skill name inputs.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	skillNames := []struct {
		name      string
		skillName string
	}{
		{"simple name", "my-skill"},
		{"name with numbers", "skill123"},
		{"name with dashes", "my-awesome-skill"},
		{"name with underscores", "my_skill_name"},
		{"long name", "this-is-a-very-long-skill-name-that-should-still-work"},
		{"single character", "a"},
		{"name with dots", "skill.v1"},
	}

	for _, tt := range skillNames {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()
			err := uninstallCmd.Flags().Set("force", "true")
			require.NoError(t, err)

			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err = uninstallCmd.RunE(uninstallCmd, []string{tt.skillName})

			w.Close()
			os.Stdout = oldStdout

			// Drain the pipe.
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			// All should fail with skill not found, not with name validation error.
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not found")
		})
	}
}

func TestUninstallCmd_ErrorMessages(t *testing.T) {
	// Verify that error messages are informative.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	t.Run("error message contains skill name", func(t *testing.T) {
		resetFlags()
		err := uninstallCmd.Flags().Set("force", "true")
		require.NoError(t, err)

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		skillName := "unique-test-skill-xyz"
		err = uninstallCmd.RunE(uninstallCmd, []string{skillName})

		w.Close()
		os.Stdout = oldStdout

		// Drain the pipe.
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)

		assert.Error(t, err)
		// The error message should reference the skill name.
		assert.Contains(t, err.Error(), skillName)
	})
}

func TestUninstallCmd_Args(t *testing.T) {
	// Test the Args validator directly: MaximumNArgs(1), since an omitted
	// <name> means "uninstall every installed skill".
	t.Run("accepts exactly one argument", func(t *testing.T) {
		err := uninstallCmd.Args(uninstallCmd, []string{"single-arg"})
		assert.NoError(t, err)
	})

	t.Run("accepts empty args (uninstall all)", func(t *testing.T) {
		err := uninstallCmd.Args(uninstallCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("rejects multiple args", func(t *testing.T) {
		err := uninstallCmd.Args(uninstallCmd, []string{"arg1", "arg2"})
		assert.Error(t, err)
	})
}

// TestUninstallCmd_SuccessfulUninstall tests the successful uninstall path.
// This test creates a mock registry with a skill and verifies it can be uninstalled.
func TestUninstallCmd_SuccessfulUninstall(t *testing.T) {
	uiOutput := setupSkillCommandUI(t)

	// Create a temp directory to use as HOME.
	tempHome := t.TempDir()

	// Set HOME to temp directory (t.Setenv auto-restores after test).
	t.Setenv("HOME", tempHome)

	// Reset homedir cache to pick up new HOME.
	homedir.Reset()
	homedir.DisableCache = true

	// Restore cache settings after test.
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})

	// Create the skills directory and a mock skill.
	skillsDir := filepath.Join(tempHome, ".atmos", "skills")
	err := os.MkdirAll(skillsDir, 0o755)
	require.NoError(t, err)

	// Create a skill directory.
	skillPath := filepath.Join(skillsDir, "github.com", "example", "test-skill")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	// Create a simple SKILL.md file for the skill.
	skillMD := `---
name: test-skill
description: A test skill
---

This is a test skill.
`
	err = os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte(skillMD), 0o644)
	require.NoError(t, err)

	now := time.Now()
	registry := map[string]interface{}{
		"version": "1.0.0",
		"skills": map[string]interface{}{
			"test-skill": map[string]interface{}{
				"name":         "test-skill",
				"display_name": "Test Skill",
				"source":       "github.com/example/test-skill",
				"version":      "v1.0.0",
				"installed_at": now.Format(time.RFC3339),
				"updated_at":   now.Format(time.RFC3339),
				"path":         skillPath,
				"is_builtin":   false,
				"enabled":      true,
			},
		},
	}

	registryData, err := json.MarshalIndent(registry, "", "  ")
	require.NoError(t, err)

	registryPath := filepath.Join(skillsDir, "registry.json")
	err = os.WriteFile(registryPath, registryData, 0o600)
	require.NoError(t, err)

	// Use force to skip confirmation prompt.
	require.NoError(t, uninstallCmd.Flags().Set("force", "true"))
	t.Cleanup(func() {
		_ = uninstallCmd.Flags().Set("force", "false")
	})

	// Run the uninstall command.
	err = uninstallCmd.RunE(uninstallCmd, []string{"test-skill"})

	// Verify uninstallation was successful.
	require.NoError(t, err)

	assert.Contains(t, atmosansi.Strip(uiOutput.String()), "uninstalled successfully")

	// Verify the skill directory was removed.
	_, statErr := os.Stat(skillPath)
	assert.True(t, os.IsNotExist(statErr), "Skill directory should be removed after uninstall")
}

// TestUninstallCmd_SuccessfulUninstall_VerifiesRegistryUpdate tests that the registry
// is updated after uninstalling a skill.
func TestUninstallCmd_SuccessfulUninstall_VerifiesRegistryUpdate(t *testing.T) {
	// Create a temp directory to use as HOME.
	tempHome := t.TempDir()

	// Set HOME to temp directory (t.Setenv auto-restores after test).
	t.Setenv("HOME", tempHome)

	// Reset homedir cache to pick up new HOME.
	homedir.Reset()
	homedir.DisableCache = true

	// Restore cache settings after test.
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})

	// Create the skills directory and a mock skill.
	skillsDir := filepath.Join(tempHome, ".atmos", "skills")
	err := os.MkdirAll(skillsDir, 0o755)
	require.NoError(t, err)

	// Create a skill directory.
	skillPath := filepath.Join(skillsDir, "github.com", "example", "another-skill")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	// Create a simple SKILL.md file for the skill.
	skillMD := `---
name: another-skill
description: Another test skill
---

Another test skill.
`
	err = os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte(skillMD), 0o644)
	require.NoError(t, err)

	now := time.Now()
	registry := map[string]interface{}{
		"version": "1.0.0",
		"skills": map[string]interface{}{
			"another-skill": map[string]interface{}{
				"name":         "another-skill",
				"display_name": "Another Skill",
				"source":       "github.com/example/another-skill",
				"version":      "v2.0.0",
				"installed_at": now.Format(time.RFC3339),
				"updated_at":   now.Format(time.RFC3339),
				"path":         skillPath,
				"is_builtin":   false,
				"enabled":      true,
			},
		},
	}

	registryData, err := json.MarshalIndent(registry, "", "  ")
	require.NoError(t, err)

	registryPath := filepath.Join(skillsDir, "registry.json")
	err = os.WriteFile(registryPath, registryData, 0o600)
	require.NoError(t, err)

	// Use force to skip confirmation prompt.
	require.NoError(t, uninstallCmd.Flags().Set("force", "true"))
	t.Cleanup(func() {
		_ = uninstallCmd.Flags().Set("force", "false")
	})

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the uninstall command.
	err = uninstallCmd.RunE(uninstallCmd, []string{"another-skill"})

	w.Close()
	os.Stdout = oldStdout

	// Drain the pipe.
	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)

	// Verify uninstallation was successful.
	require.NoError(t, err)

	// Verify the registry was updated (skill should be removed).
	registryContent, readErr := os.ReadFile(registryPath)
	require.NoError(t, readErr)

	var updatedRegistry map[string]interface{}
	err = json.Unmarshal(registryContent, &updatedRegistry)
	require.NoError(t, err)

	skills, ok := updatedRegistry["skills"].(map[string]interface{})
	require.True(t, ok, "Registry should have skills field")
	_, skillExists := skills["another-skill"]
	assert.False(t, skillExists, "Skill should be removed from registry after uninstall")
}

func TestUninstallCmd_RunE_InstallerInitFailure(t *testing.T) {
	// Reset flags before test.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	t.Run("fails when home directory is unwritable", func(t *testing.T) {
		resetFlags()

		// Create a temp directory and set up an unwritable skills path.
		tempHome := t.TempDir()

		// Create .atmos/skills as a file (not a directory) to cause registry failure.
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
		err = uninstallCmd.RunE(uninstallCmd, []string{"some-skill"})

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

// TestUninstallCmd_RunE_NoArgsUninstallsEverything covers the CLI wiring for
// "atmos ai skill uninstall" with no <name>: it must reach UninstallAll
// rather than erroring on a missing argument.
func TestUninstallCmd_RunE_NoArgsUninstallsEverything(t *testing.T) {
	resetInstallFlags := func() {
		if flag := installCmd.Flags().Lookup("force"); flag != nil {
			_ = flag.Value.Set("false")
		}
		if flag := installCmd.Flags().Lookup("yes"); flag != nil {
			_ = flag.Value.Set("false")
		}
	}
	resetUninstallFlags := func() {
		if flag := uninstallCmd.Flags().Lookup("force"); flag != nil {
			_ = flag.Value.Set("false")
		}
	}
	resetInstallFlags()
	resetUninstallFlags()
	t.Cleanup(func() {
		resetInstallFlags()
		resetUninstallFlags()
	})

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	setupSkillCommandUI(t)

	// Install a couple of bundled skills (offline, fast) to have something to
	// uninstall.
	require.NoError(t, installCmd.Flags().Set("yes", "true"))
	require.NoError(t, installCmd.RunE(installCmd, []string{"atmos-terraform"}))
	require.NoError(t, installCmd.RunE(installCmd, []string{"atmos-git"}))
	require.FileExists(t, filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform", "SKILL.md"))

	uiOutput := setupSkillCommandUI(t)
	require.NoError(t, uninstallCmd.Flags().Set("force", "true"))

	err := uninstallCmd.RunE(uninstallCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, atmosansi.Strip(uiOutput.String()), "skills uninstalled successfully")

	_, statErr := os.Stat(filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform"))
	assert.True(t, os.IsNotExist(statErr), "skill directory should be removed")
}

func TestUninstallCmd_RunE_ForceFlagVariations(t *testing.T) {
	// Reset flags helper.
	resetFlags := func() {
		forceFlag := uninstallCmd.Flags().Lookup("force")
		if forceFlag != nil {
			_ = forceFlag.Value.Set("false")
		}
	}

	tests := []struct {
		name       string
		force      bool
		skillName  string
		wantErrMsg string
	}{
		{
			name:       "without force flag - skill not found",
			force:      false,
			skillName:  "nonexistent-skill-force-test",
			wantErrMsg: "not found",
		},
		{
			name:       "with force flag - skill not found",
			force:      true,
			skillName:  "nonexistent-skill-force-test-2",
			wantErrMsg: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()

			if tt.force {
				require.NoError(t, uninstallCmd.Flags().Set("force", "true"))
			}

			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run with nonexistent skill.
			err := uninstallCmd.RunE(uninstallCmd, []string{tt.skillName})

			w.Close()
			os.Stdout = oldStdout

			// Drain the pipe.
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestUninstallCmd_RunE_MultipleSkills(t *testing.T) {
	// Create a temp directory to use as HOME.
	tempHome := t.TempDir()

	// Set HOME to temp directory.
	t.Setenv("HOME", tempHome)

	// Reset homedir cache to pick up new HOME.
	homedir.Reset()
	homedir.DisableCache = true
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})

	// Create the skills directory and multiple mock skills.
	skillsDir := filepath.Join(tempHome, ".atmos", "skills")
	err := os.MkdirAll(skillsDir, 0o755)
	require.NoError(t, err)

	// Create first skill directory.
	skill1Path := filepath.Join(skillsDir, "github.com", "example", "skill-one")
	err = os.MkdirAll(skill1Path, 0o755)
	require.NoError(t, err)

	skillMD1 := `---
name: skill-one
description: First test skill
---

First test skill.
`
	err = os.WriteFile(filepath.Join(skill1Path, "SKILL.md"), []byte(skillMD1), 0o644)
	require.NoError(t, err)

	// Create second skill directory.
	skill2Path := filepath.Join(skillsDir, "github.com", "example", "skill-two")
	err = os.MkdirAll(skill2Path, 0o755)
	require.NoError(t, err)

	skillMD2 := `---
name: skill-two
description: Second test skill
---

Second test skill.
`
	err = os.WriteFile(filepath.Join(skill2Path, "SKILL.md"), []byte(skillMD2), 0o644)
	require.NoError(t, err)

	now := time.Now()
	registry := map[string]interface{}{
		"version": "1.0.0",
		"skills": map[string]interface{}{
			"skill-one": map[string]interface{}{
				"name":         "skill-one",
				"display_name": "Skill One",
				"source":       "github.com/example/skill-one",
				"version":      "v1.0.0",
				"installed_at": now.Format(time.RFC3339),
				"updated_at":   now.Format(time.RFC3339),
				"path":         skill1Path,
				"is_builtin":   false,
				"enabled":      true,
			},
			"skill-two": map[string]interface{}{
				"name":         "skill-two",
				"display_name": "Skill Two",
				"source":       "github.com/example/skill-two",
				"version":      "v2.0.0",
				"installed_at": now.Format(time.RFC3339),
				"updated_at":   now.Format(time.RFC3339),
				"path":         skill2Path,
				"is_builtin":   false,
				"enabled":      true,
			},
		},
	}

	registryData, err := json.MarshalIndent(registry, "", "  ")
	require.NoError(t, err)

	registryPath := filepath.Join(skillsDir, "registry.json")
	err = os.WriteFile(registryPath, registryData, 0o600)
	require.NoError(t, err)

	// Set force flag.
	forceFlag := uninstallCmd.Flags().Lookup("force")
	if forceFlag != nil {
		_ = forceFlag.Value.Set("true")
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Uninstall first skill.
	err = uninstallCmd.RunE(uninstallCmd, []string{"skill-one"})

	w.Close()
	os.Stdout = oldStdout

	// Drain the pipe.
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	require.NoError(t, err)

	// Verify first skill was removed.
	_, statErr := os.Stat(skill1Path)
	assert.True(t, os.IsNotExist(statErr), "First skill directory should be removed")

	// Verify second skill still exists.
	_, statErr = os.Stat(skill2Path)
	assert.False(t, os.IsNotExist(statErr), "Second skill directory should still exist")

	// Verify registry was updated.
	registryContent, readErr := os.ReadFile(registryPath)
	require.NoError(t, readErr)

	var updatedRegistry map[string]interface{}
	err = json.Unmarshal(registryContent, &updatedRegistry)
	require.NoError(t, err)

	skills, ok := updatedRegistry["skills"].(map[string]interface{})
	require.True(t, ok)
	_, skill1Exists := skills["skill-one"]
	_, skill2Exists := skills["skill-two"]
	assert.False(t, skill1Exists, "skill-one should be removed from registry")
	assert.True(t, skill2Exists, "skill-two should still be in registry")
}

func TestUninstallCmd_ForceFlagGetBool(t *testing.T) {
	// Reset flags.
	forceFlag := uninstallCmd.Flags().Lookup("force")
	if forceFlag != nil {
		_ = forceFlag.Value.Set("false")
	}

	// Verify GetBool works correctly after flag is registered.
	force, err := uninstallCmd.Flags().GetBool("force")
	require.NoError(t, err)
	assert.False(t, force)

	// Set to true and verify.
	err = uninstallCmd.Flags().Set("force", "true")
	require.NoError(t, err)

	force, err = uninstallCmd.Flags().GetBool("force")
	require.NoError(t, err)
	assert.True(t, force)
}

// TestUninstallCmd_StandardParserServer tests that the uninstall command uses StandardParser
// with proper Viper binding for flag precedence (CLI > ENV > defaults).
func TestUninstallCmd_StandardParserServer(t *testing.T) {
	t.Run("uninstallParser is initialized", func(t *testing.T) {
		require.NotNil(t, uninstallParser, "uninstallParser should be initialized by init()")
	})

	t.Run("env var binding for force flag", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_FORCE", "true")

		// Use a fresh Viper instance to avoid global state pollution.
		v := viper.New()
		err := uninstallParser.BindToViper(v)
		require.NoError(t, err)

		assert.True(t, v.GetBool("force"), "force should be true from ATMOS_AI_SKILL_FORCE env var")
	})

	t.Run("CLI flag overrides env var", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_FORCE", "true")

		oldVal := uninstallCmd.Flags().Lookup("force").Value.String()
		t.Cleanup(func() {
			_ = uninstallCmd.Flags().Set("force", oldVal)
		})
		_ = uninstallCmd.Flags().Set("force", "false")

		v := viper.GetViper()
		err := uninstallParser.BindFlagsToViper(uninstallCmd, v)
		require.NoError(t, err)

		assert.False(t, v.GetBool("force"), "CLI flag should override env var")
	})

	t.Run("force flag has shorthand f", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand)
	})

	t.Run("env var binding for client flag", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_CLIENT", "vscode")

		v := viper.New()
		err := uninstallParser.BindToViper(v)
		require.NoError(t, err)

		assert.Equal(t, []string{"vscode"}, v.GetStringSlice("client"))
	})

	t.Run("env var binding for all-clients flag", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_ALL_CLIENTS", "true")

		v := viper.New()
		err := uninstallParser.BindToViper(v)
		require.NoError(t, err)

		assert.True(t, v.GetBool("all-clients"), "all-clients should be true from ATMOS_AI_SKILL_ALL_CLIENTS env var")
	})

	t.Run("scope flag defaults to project via Viper", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("scope")
		require.NotNil(t, flag)
		assert.Equal(t, "project", flag.DefValue)
	})

	t.Run("env var binding for scope flag", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_SCOPE", "user")

		v := viper.New()
		err := uninstallParser.BindToViper(v)
		require.NoError(t, err)

		assert.Equal(t, "user", v.GetString("scope"), "scope should come from ATMOS_AI_SKILL_SCOPE env var")
	})

	t.Run("CLI scope flag overrides env var", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_SCOPE", "user")

		oldVal := uninstallCmd.Flags().Lookup("scope").Value.String()
		t.Cleanup(func() {
			_ = uninstallCmd.Flags().Set("scope", oldVal)
		})
		require.NoError(t, uninstallCmd.Flags().Set("scope", "project"))

		v := viper.GetViper()
		err := uninstallParser.BindFlagsToViper(uninstallCmd, v)
		require.NoError(t, err)

		assert.Equal(t, "project", v.GetString("scope"), "CLI flag should override env var")
	})

	t.Run("env var binding for global flag", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_GLOBAL", "true")

		v := viper.New()
		err := uninstallParser.BindToViper(v)
		require.NoError(t, err)

		assert.True(t, v.GetBool("global"), "global should be true from ATMOS_AI_SKILL_GLOBAL env var")
	})
}

// TestUninstallCmd_RunE_ForceCleansUpUserScopeEvenWithProjectSignalPresent
// guards against the exact bug reported live: installing to user scope, then
// running `uninstall --force` (no explicit --scope) from a project directory
// that happens to have its own project-level client signal (e.g. this repo's
// own .claude/) silently defaulted scope to "project" and skipped the real
// (user-scope) distributed copy entirely -- and, worse, could target
// unrelated real files sitting at the wrong-but-real project path. Uninstall
// must check both scopes so the actual distributed copy always gets cleaned
// up regardless of which scope's signal happens to be detectable from CWD.
func TestUninstallCmd_RunE_ForceCleansUpUserScopeEvenWithProjectSignalPresent(t *testing.T) {
	// clearFlag resets a flag's value to its default AND clears pflag's own
	// Changed bit -- plain Flags().Set() would leave Changed true, which
	// explicitSkillScope/resolveUninstallScopes read to mean "the user
	// explicitly asked for this scope", defeating the very auto-detect
	// fallback this test exercises.
	clearFlag := func(cmd *cobra.Command, name string) {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag)
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}
	// clearStringSliceFlag resets a --client-style repeatable flag. Unlike
	// clearFlag, it must not replay flag.DefValue ("[]") through Set(): pflag's
	// stringSliceValue APPENDS on every Set() call after the first, so once
	// Changed has ever been true, Set("[]") leaves a literal "[]" element
	// instead of an empty slice.
	clearStringSliceFlag := func(cmd *cobra.Command, name string) {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag)
		_ = flag.Value.Set("")
		flag.Changed = false
	}
	resetInstallFlags := func() {
		clearFlag(installCmd, "yes")
		clearStringSliceFlag(installCmd, "client")
		clearFlag(installCmd, "scope")
	}
	resetInstallFlags()
	t.Cleanup(resetInstallFlags)

	resetUninstallFlags := func() {
		clearFlag(uninstallCmd, "force")
		clearStringSliceFlag(uninstallCmd, "client")
		clearFlag(uninstallCmd, "scope")
	}
	resetUninstallFlags()
	t.Cleanup(resetUninstallFlags)

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	// A project directory with its own unrelated .claude signal -- mimicking
	// this exact repo, where running uninstall from inside it would
	// otherwise silently "detect" claude-code at project scope even though
	// the skill was actually distributed to user scope.
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".claude"), 0o755))
	t.Chdir(projectDir)

	setupSkillCommandUI(t)

	// Install one bundled skill to claude-code at USER scope.
	require.NoError(t, installCmd.Flags().Set("yes", "true"))
	require.NoError(t, installCmd.Flags().Set("client", "claude-code"))
	require.NoError(t, installCmd.Flags().Set("scope", "user"))
	require.NoError(t, installCmd.RunE(installCmd, []string{}))

	userDistPath := filepath.Join(tempHome, ".claude", "skills", "atmos-terraform")
	require.DirExists(t, userDistPath,
		"sanity check: the skill must have actually landed in the user-scope claude-code directory")

	// Uninstall everything with --force and no explicit --scope/--client --
	// the exact command from the bug report.
	require.NoError(t, uninstallCmd.Flags().Set("force", "true"))
	require.NoError(t, uninstallCmd.RunE(uninstallCmd, []string{}))

	assert.NoDirExists(t, userDistPath,
		"the real user-scope distributed copy must be cleaned up even though CWD's own project-scope .claude/ signal exists")
}
