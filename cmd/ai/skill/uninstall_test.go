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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestUninstallCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "uninstall <name>", uninstallCmd.Use)
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
	// Verify the long description contains examples.
	assert.Contains(t, uninstallCmd.Long, "atmos ai skill uninstall terraform-optimizer")
	assert.Contains(t, uninstallCmd.Long, "--force")
}

func TestUninstallCmd_ReferencesListCommand(t *testing.T) {
	// Verify it references the list command for finding skill names.
	assert.Contains(t, uninstallCmd.Long, "atmos ai skill list")
}

func TestUninstallCmd_ArgsValidation_ExactArgs(t *testing.T) {
	// Test ExactArgs(1) validation.
	t.Run("rejects zero arguments", func(t *testing.T) {
		err := cobra.ExactArgs(1)(uninstallCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s)")
	})

	t.Run("accepts exactly one argument", func(t *testing.T) {
		err := cobra.ExactArgs(1)(uninstallCmd, []string{"my-skill"})
		assert.NoError(t, err)
	})

	t.Run("rejects two arguments", func(t *testing.T) {
		err := cobra.ExactArgs(1)(uninstallCmd, []string{"arg1", "arg2"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s)")
	})

	t.Run("rejects multiple arguments", func(t *testing.T) {
		err := cobra.ExactArgs(1)(uninstallCmd, []string{"arg1", "arg2", "arg3"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s)")
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
	assert.Equal(t, "uninstall <name>", uninstallCmd.Use)
}

func TestUninstallCmd_ShortDescription(t *testing.T) {
	assert.Equal(t, "Remove an installed skill", uninstallCmd.Short)
}

func TestUninstallCmd_LongDescriptionContainsExamples(t *testing.T) {
	assert.Contains(t, uninstallCmd.Long, "Examples:")
}

func TestUninstallCmd_LongDescriptionContainsInstallationPath(t *testing.T) {
	// Verify long description mentions the installation path.
	assert.Contains(t, uninstallCmd.Long, "~/.atmos/skills/")
}

func TestUninstallCmd_LongDescriptionContainsForceOption(t *testing.T) {
	// Verify long description mentions the force option.
	assert.Contains(t, uninstallCmd.Long, "Force uninstall")
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
	// Test the Args validator directly.
	t.Run("validates that Args is ExactArgs(1)", func(t *testing.T) {
		// ExactArgs(1) should pass for exactly 1 argument.
		err := uninstallCmd.Args(uninstallCmd, []string{"single-arg"})
		assert.NoError(t, err)
	})

	t.Run("rejects empty args", func(t *testing.T) {
		err := uninstallCmd.Args(uninstallCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("rejects multiple args", func(t *testing.T) {
		err := uninstallCmd.Args(uninstallCmd, []string{"arg1", "arg2"})
		assert.Error(t, err)
	})
}

// TestUninstallCmd_SuccessfulUninstall tests the successful uninstall path.
// This test creates a mock registry with a skill and verifies it can be uninstalled.
func TestUninstallCmd_SuccessfulUninstall(t *testing.T) {
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

	// Reset the force flag.
	forceFlag := uninstallCmd.Flags().Lookup("force")
	if forceFlag != nil {
		_ = forceFlag.Value.Set("true") // Use force to skip confirmation prompt.
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the uninstall command.
	err = uninstallCmd.RunE(uninstallCmd, []string{"test-skill"})

	w.Close()
	os.Stdout = oldStdout

	// Drain the pipe.
	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)

	// Verify uninstallation was successful.
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "uninstalled successfully")

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

	// Reset the force flag.
	forceFlag := uninstallCmd.Flags().Lookup("force")
	if forceFlag != nil {
		_ = forceFlag.Value.Set("true") // Use force to skip confirmation prompt.
	}

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
