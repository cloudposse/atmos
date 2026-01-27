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
