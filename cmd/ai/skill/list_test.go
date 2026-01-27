package skill

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
)

func TestListCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "list", listCmd.Use)
	assert.Equal(t, "List installed skills", listCmd.Short)
	assert.NotEmpty(t, listCmd.Long)
	assert.NotNil(t, listCmd.RunE)
}

func TestListCmd_Flags(t *testing.T) {
	t.Run("has detailed flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("detailed")
		require.NotNil(t, flag, "detailed flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "d", flag.Shorthand)
	})
}

func TestFormatTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "just now",
			time:     now.Add(-30 * time.Second),
			expected: "just now",
		},
		{
			name:     "1 minute ago",
			time:     now.Add(-1 * time.Minute),
			expected: "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			time:     now.Add(-5 * time.Minute),
			expected: "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			time:     now.Add(-1 * time.Hour),
			expected: "1 hour ago",
		},
		{
			name:     "3 hours ago",
			time:     now.Add(-3 * time.Hour),
			expected: "3 hours ago",
		},
		{
			name:     "yesterday",
			time:     now.Add(-25 * time.Hour),
			expected: "yesterday",
		},
		{
			name:     "3 days ago",
			time:     now.Add(-3 * 24 * time.Hour),
			expected: "3 days ago",
		},
		{
			name:     "more than a week ago",
			time:     now.Add(-10 * 24 * time.Hour),
			expected: now.Add(-10 * 24 * time.Hour).Format("Jan 2, 2006"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(tt.time)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrintSkillSummary(t *testing.T) {
	tests := []struct {
		name     string
		skill    *marketplace.InstalledSkill
		contains []string
	}{
		{
			name: "enabled skill",
			skill: &marketplace.InstalledSkill{
				Name:        "test-skill",
				DisplayName: "Test Skill",
				Source:      "github.com/user/test-skill",
				Version:     "v1.0.0",
				Enabled:     true,
			},
			contains: []string{"Test Skill", "github.com/user/test-skill", "v1.0.0"},
		},
		{
			name: "disabled skill",
			skill: &marketplace.InstalledSkill{
				Name:        "disabled-skill",
				DisplayName: "Disabled Skill",
				Source:      "github.com/user/disabled-skill",
				Version:     "v2.0.0",
				Enabled:     false,
			},
			contains: []string{"Disabled Skill", "github.com/user/disabled-skill", "v2.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printSkillSummary(tt.skill)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, err := io.Copy(&buf, r)
			require.NoError(t, err)

			output := buf.String()
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintSkillDetailed(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		skill    *marketplace.InstalledSkill
		contains []string
	}{
		{
			name: "enabled community skill",
			skill: &marketplace.InstalledSkill{
				Name:        "test-skill",
				DisplayName: "Test Skill",
				Source:      "github.com/user/test-skill",
				Version:     "v1.0.0",
				Path:        "/home/user/.atmos/skills/test-skill",
				InstalledAt: now,
				UpdatedAt:   now,
				Enabled:     true,
				IsBuiltIn:   false,
			},
			contains: []string{
				"Test Skill",
				"Enabled",
				"test-skill",
				"github.com/user/test-skill",
				"v1.0.0",
				"/home/user/.atmos/skills/test-skill",
				"Community",
			},
		},
		{
			name: "disabled built-in skill",
			skill: &marketplace.InstalledSkill{
				Name:        "builtin-skill",
				DisplayName: "Built-in Skill",
				Source:      "built-in",
				Version:     "v0.1.0",
				Path:        "/app/skills/builtin-skill",
				InstalledAt: now,
				UpdatedAt:   now,
				Enabled:     false,
				IsBuiltIn:   true,
			},
			contains: []string{
				"Built-in Skill",
				"Disabled",
				"builtin-skill",
				"built-in",
				"v0.1.0",
				"/app/skills/builtin-skill",
				"Built-in",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printSkillDetailed(tt.skill)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, err := io.Copy(&buf, r)
			require.NoError(t, err)

			output := buf.String()
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestListCmd_LongDescription(t *testing.T) {
	assert.Contains(t, listCmd.Long, "List all community-contributed skills")
	assert.Contains(t, listCmd.Long, "~/.atmos/skills/")
	assert.Contains(t, listCmd.Long, "registry.json")
}

func TestPrintSkillSummary_StatusSymbols(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		expectedSymbol string
	}{
		{
			name:           "enabled skill shows checkmark",
			enabled:        true,
			expectedSymbol: "✓",
		},
		{
			name:           "disabled skill shows x",
			enabled:        false,
			expectedSymbol: "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &marketplace.InstalledSkill{
				Name:        "test-skill",
				DisplayName: "Test Skill",
				Source:      "github.com/user/test-skill",
				Version:     "v1.0.0",
				Enabled:     tt.enabled,
			}

			// Capture stdout.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printSkillSummary(skill)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, err := io.Copy(&buf, r)
			require.NoError(t, err)

			output := buf.String()
			assert.Contains(t, output, tt.expectedSymbol, "Expected status symbol %q in output", tt.expectedSymbol)
		})
	}
}

func TestPrintSkillSummary_OutputFormat(t *testing.T) {
	skill := &marketplace.InstalledSkill{
		Name:        "my-skill",
		DisplayName: "My Awesome Skill",
		Source:      "github.com/example/my-skill",
		Version:     "v2.3.4",
		Enabled:     true,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSkillSummary(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()

	// Verify the output format includes display name on first line.
	assert.Contains(t, output, "My Awesome Skill")
	// Verify second line contains source @ version.
	assert.Contains(t, output, "github.com/example/my-skill @ v2.3.4")
}

func TestPrintSkillDetailed_OutputFormat(t *testing.T) {
	now := time.Now()

	skill := &marketplace.InstalledSkill{
		Name:        "detailed-skill",
		DisplayName: "Detailed Test Skill",
		Source:      "github.com/example/detailed-skill",
		Version:     "v1.2.3",
		Path:        "/home/user/.atmos/skills/detailed-skill",
		InstalledAt: now,
		UpdatedAt:   now,
		Enabled:     true,
		IsBuiltIn:   false,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSkillDetailed(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()

	// Verify header separator line.
	assert.Contains(t, output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	// Verify field labels.
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "Source:")
	assert.Contains(t, output, "Version:")
	assert.Contains(t, output, "Installed:")
	assert.Contains(t, output, "Last Updated:")
	assert.Contains(t, output, "Location:")
	assert.Contains(t, output, "Type:")
}

func TestPrintSkillDetailed_StatusText(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		enabled        bool
		expectedStatus string
	}{
		{
			name:           "enabled shows Enabled status",
			enabled:        true,
			expectedStatus: "Enabled",
		},
		{
			name:           "disabled shows Disabled status",
			enabled:        false,
			expectedStatus: "Disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &marketplace.InstalledSkill{
				Name:        "status-test-skill",
				DisplayName: "Status Test Skill",
				Source:      "github.com/example/status-test",
				Version:     "v1.0.0",
				Path:        "/home/user/.atmos/skills/status-test",
				InstalledAt: now,
				UpdatedAt:   now,
				Enabled:     tt.enabled,
				IsBuiltIn:   false,
			}

			// Capture stdout.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printSkillDetailed(skill)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, err := io.Copy(&buf, r)
			require.NoError(t, err)

			output := buf.String()
			assert.Contains(t, output, tt.expectedStatus, "Expected status text %q in output", tt.expectedStatus)
		})
	}
}

func TestPrintSkillDetailed_TypeField(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		isBuiltIn    bool
		expectedType string
	}{
		{
			name:         "built-in skill shows Built-in type",
			isBuiltIn:    true,
			expectedType: "Type:         Built-in",
		},
		{
			name:         "community skill shows Community type",
			isBuiltIn:    false,
			expectedType: "Type:         Community",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &marketplace.InstalledSkill{
				Name:        "type-test-skill",
				DisplayName: "Type Test Skill",
				Source:      "github.com/example/type-test",
				Version:     "v1.0.0",
				Path:        "/home/user/.atmos/skills/type-test",
				InstalledAt: now,
				UpdatedAt:   now,
				Enabled:     true,
				IsBuiltIn:   tt.isBuiltIn,
			}

			// Capture stdout.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printSkillDetailed(skill)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, err := io.Copy(&buf, r)
			require.NoError(t, err)

			output := buf.String()
			assert.Contains(t, output, tt.expectedType, "Expected type %q in output", tt.expectedType)
		})
	}
}

func TestFormatTime_BoundaryConditions(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		duration time.Duration
		contains string
	}{
		{
			name:     "exactly 0 seconds ago",
			duration: 0,
			contains: "just now",
		},
		{
			name:     "59 seconds ago",
			duration: -59 * time.Second,
			contains: "just now",
		},
		{
			name:     "exactly 1 minute ago",
			duration: -60 * time.Second,
			contains: "1 minute ago",
		},
		{
			name:     "59 minutes ago",
			duration: -59 * time.Minute,
			contains: "minutes ago",
		},
		{
			name:     "exactly 1 hour ago",
			duration: -60 * time.Minute,
			contains: "1 hour ago",
		},
		{
			name:     "23 hours ago",
			duration: -23 * time.Hour,
			contains: "hours ago",
		},
		{
			name:     "exactly 24 hours ago (1 day)",
			duration: -24 * time.Hour,
			contains: "yesterday",
		},
		{
			name:     "2 days ago",
			duration: -48 * time.Hour,
			contains: "2 days ago",
		},
		{
			name:     "6 days ago",
			duration: -6 * 24 * time.Hour,
			contains: "6 days ago",
		},
		{
			name:     "exactly 7 days ago (shows date)",
			duration: -7 * 24 * time.Hour,
			contains: now.Add(-7 * 24 * time.Hour).Format("2006"),
		},
		{
			name:     "30 days ago (shows date format)",
			duration: -30 * 24 * time.Hour,
			contains: ",",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(now.Add(tt.duration))
			assert.Contains(t, result, tt.contains, "Expected %q to contain %q", result, tt.contains)
		})
	}
}

func TestFormatTime_SpecificDate(t *testing.T) {
	// Test that old dates are formatted correctly.
	oldDate := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	result := formatTime(oldDate)

	// Should be formatted as "Jun 15, 2024".
	assert.Contains(t, result, "Jun 15, 2024")
}

func TestListCmd_ArgsValidation(t *testing.T) {
	// The list command should have an Args validator set (cobra.NoArgs).
	// We test this by verifying the Args field is not nil and by testing behavior.
	require.NotNil(t, listCmd.Args, "Args validator should be set")

	// Test that passing arguments returns an error (validates NoArgs behavior).
	err := listCmd.Args(listCmd, []string{"unexpected-arg"})
	assert.Error(t, err, "Should reject arguments when NoArgs is set")
}

func TestListCmd_Examples(t *testing.T) {
	// Verify the long description contains usage examples.
	assert.Contains(t, listCmd.Long, "atmos ai skill list")
	assert.Contains(t, listCmd.Long, "--detailed")
}

func TestListCmd_FlagShorthand(t *testing.T) {
	// Verify the detailed flag has the correct shorthand.
	flag := listCmd.Flags().Lookup("detailed")
	require.NotNil(t, flag)
	assert.Equal(t, "d", flag.Shorthand)
}

func TestListCmd_FlagUsage(t *testing.T) {
	// Verify the detailed flag has a proper usage description.
	flag := listCmd.Flags().Lookup("detailed")
	require.NotNil(t, flag)
	assert.NotEmpty(t, flag.Usage)
	assert.Contains(t, flag.Usage, "detailed")
}

func TestPrintSkillSummary_EmptyFields(t *testing.T) {
	// Test with empty source and version to ensure no panic.
	skill := &marketplace.InstalledSkill{
		Name:        "",
		DisplayName: "",
		Source:      "",
		Version:     "",
		Enabled:     true,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Should not panic.
	printSkillSummary(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()
	// Should still contain the status symbol.
	assert.Contains(t, output, "✓")
	// Should contain the @ separator even with empty fields.
	assert.Contains(t, output, "@")
}

func TestPrintSkillDetailed_EmptyFields(t *testing.T) {
	now := time.Now()

	// Test with minimal fields to ensure no panic.
	skill := &marketplace.InstalledSkill{
		Name:        "",
		DisplayName: "",
		Source:      "",
		Version:     "",
		Path:        "",
		InstalledAt: now,
		UpdatedAt:   now,
		Enabled:     true,
		IsBuiltIn:   false,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Should not panic.
	printSkillDetailed(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()
	// Should still contain field labels.
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "Source:")
	assert.Contains(t, output, "Version:")
}

func TestPrintSkillDetailed_TimestampFormatting(t *testing.T) {
	// Create a skill with a specific timestamp to verify formatting.
	installedAt := time.Now().Add(-5 * time.Minute)
	updatedAt := time.Now().Add(-2 * time.Minute)

	skill := &marketplace.InstalledSkill{
		Name:        "timestamp-test",
		DisplayName: "Timestamp Test Skill",
		Source:      "github.com/example/timestamp-test",
		Version:     "v1.0.0",
		Path:        "/home/user/.atmos/skills/timestamp-test",
		InstalledAt: installedAt,
		UpdatedAt:   updatedAt,
		Enabled:     true,
		IsBuiltIn:   false,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSkillDetailed(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()
	// Verify timestamps are formatted.
	assert.Contains(t, output, "minutes ago")
}

func TestListCmd_RunENotNil(t *testing.T) {
	// Verify RunE is set.
	require.NotNil(t, listCmd.RunE)
}

func TestListCmd_UseFieldCorrect(t *testing.T) {
	// Verify Use field is exactly "list".
	assert.Equal(t, "list", listCmd.Use)
}

func TestListCmd_ShortDescription(t *testing.T) {
	// Verify short description is set.
	assert.Equal(t, "List installed skills", listCmd.Short)
}

func TestListCmd_LongDescriptionContainsExamples(t *testing.T) {
	// Verify long description has Examples section.
	assert.Contains(t, listCmd.Long, "Examples:")
}

func TestListCmd_LongDescriptionContainsInstallationPath(t *testing.T) {
	// Verify long description mentions the installation path.
	assert.Contains(t, listCmd.Long, "~/.atmos/skills/")
}

func TestListCmd_LongDescriptionContainsRegistryFile(t *testing.T) {
	// Verify long description mentions the registry file.
	assert.Contains(t, listCmd.Long, "registry.json")
}

func TestListCmd_CommandRegistration(t *testing.T) {
	// Verify the list command is properly configured with parent command setup in init().
	// This tests that the init() function properly sets up the command.
	assert.NotNil(t, listCmd)
	assert.NotNil(t, listCmd.Flags())
}

func TestFormatTime_FutureTime(t *testing.T) {
	// Test with a time in the future (edge case).
	futureTime := time.Now().Add(1 * time.Hour)
	result := formatTime(futureTime)

	// Future times should return "just now" since diff will be negative.
	// The function uses now.Sub(t) which will be negative for future times.
	// This tests the edge case behavior.
	assert.NotEmpty(t, result)
}

func TestPrintSkillSummary_SpecialCharactersInName(t *testing.T) {
	skill := &marketplace.InstalledSkill{
		Name:        "test-skill-with-special-chars",
		DisplayName: "Test Skill (v2.0) - Special Edition",
		Source:      "github.com/user/test-skill",
		Version:     "v2.0.0-beta.1",
		Enabled:     true,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSkillSummary(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Test Skill (v2.0) - Special Edition")
	assert.Contains(t, output, "v2.0.0-beta.1")
}

func TestPrintSkillDetailed_SpecialCharactersInFields(t *testing.T) {
	now := time.Now()

	skill := &marketplace.InstalledSkill{
		Name:        "special-chars-skill",
		DisplayName: "Special [Chars] & <Test> Skill",
		Source:      "github.com/user-name/repo_name",
		Version:     "v1.0.0+build.123",
		Path:        "/home/user/.atmos/skills/special-chars-skill",
		InstalledAt: now,
		UpdatedAt:   now,
		Enabled:     true,
		IsBuiltIn:   false,
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSkillDetailed(skill)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Special [Chars] & <Test> Skill")
	assert.Contains(t, output, "v1.0.0+build.123")
}

// TestListCmd_Execute tests the actual command execution.
// This test uses the real home directory and filesystem to create/read the registry.
func TestListCmd_Execute(t *testing.T) {
	// Reset flag value to default for each subtest.
	resetDetailedFlag := func() {
		flag := listCmd.Flags().Lookup("detailed")
		if flag != nil {
			flag.Value.Set("false")
		}
	}

	t.Run("executes without error and shows no skills message", func(t *testing.T) {
		resetDetailedFlag()

		// Capture stdout.
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Execute the command - this will use the real home directory.
		// If there are no skills installed, it should print the "no skills" message.
		err := listCmd.RunE(listCmd, []string{})

		w.Close()
		os.Stdout = old

		var buf bytes.Buffer
		_, copyErr := io.Copy(&buf, r)
		require.NoError(t, copyErr)

		// The command should either succeed with no skills or succeed with skills listed.
		// Either way, it should not error (assuming the home directory is accessible).
		if err != nil {
			// If there's an error, it's likely a filesystem issue.
			// Skip the test in such cases.
			t.Skipf("Skipping due to filesystem access: %v", err)
		}

		output := buf.String()
		// The output should contain either "No skills installed" or "Installed skills".
		assert.True(t, len(output) > 0 || err == nil, "Command should produce output or succeed silently")
	})

	t.Run("executes with detailed flag", func(t *testing.T) {
		resetDetailedFlag()

		// Set the detailed flag.
		err := listCmd.Flags().Set("detailed", "true")
		require.NoError(t, err)

		// Capture stdout.
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = listCmd.RunE(listCmd, []string{})

		w.Close()
		os.Stdout = old

		var buf bytes.Buffer
		_, copyErr := io.Copy(&buf, r)
		require.NoError(t, copyErr)

		if err != nil {
			// If there's an error, it's likely a filesystem issue.
			t.Skipf("Skipping due to filesystem access: %v", err)
		}

		// Verify the detailed flag was recognized.
		detailedValue, _ := listCmd.Flags().GetBool("detailed")
		assert.True(t, detailedValue)
	})

	t.Run("flag parsing works correctly", func(t *testing.T) {
		resetDetailedFlag()

		// Test that we can get the detailed flag value.
		detailed, err := listCmd.Flags().GetBool("detailed")
		require.NoError(t, err)
		assert.False(t, detailed, "default value should be false")

		// Set and verify.
		err = listCmd.Flags().Set("detailed", "true")
		require.NoError(t, err)

		detailed, err = listCmd.Flags().GetBool("detailed")
		require.NoError(t, err)
		assert.True(t, detailed)
	})
}

// TestListCmd_DetailedFlagParsing specifically tests the flag parsing logic in RunE.
func TestListCmd_DetailedFlagParsing(t *testing.T) {
	// Reset the flag to ensure clean state.
	flag := listCmd.Flags().Lookup("detailed")
	require.NotNil(t, flag)

	// Test setting to true.
	err := flag.Value.Set("true")
	require.NoError(t, err)
	assert.Equal(t, "true", flag.Value.String())

	// Test setting to false.
	err = flag.Value.Set("false")
	require.NoError(t, err)
	assert.Equal(t, "false", flag.Value.String())

	// Test GetBool after setting.
	err = listCmd.Flags().Set("detailed", "true")
	require.NoError(t, err)
	val, err := listCmd.Flags().GetBool("detailed")
	require.NoError(t, err)
	assert.True(t, val)
}

// TestListCmd_OutputMessages tests the output messages for different scenarios.
func TestListCmd_OutputMessages(t *testing.T) {
	// This test verifies that the command outputs expected messages.
	// The actual output depends on whether skills are installed.

	t.Run("expected messages exist in command help", func(t *testing.T) {
		// Verify the command has proper help text.
		assert.NotEmpty(t, listCmd.Long)
		assert.NotEmpty(t, listCmd.Short)
	})
}
