package cmd

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/analyze"
	"github.com/cloudposse/atmos/pkg/schema"
)

// saveAndRestoreArgs saves os.Args and restores them during test cleanup.
func saveAndRestoreArgs(t *testing.T) {
	t.Helper()
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
}

// validAIConfig returns a minimal valid AI configuration for testing.
func validAIConfig() schema.AISettings {
	return schema.AISettings{
		Enabled:         true,
		DefaultProvider: "anthropic",
		Providers: map[string]*schema.AIProviderConfig{
			"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
		},
	}
}

// TestBuildCommandName verifies that buildCommandName joins os.Args.
func TestBuildCommandName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "multiple args",
			args:     []string{"atmos", "terraform", "plan", "vpc", "-s", "prod"},
			expected: "atmos terraform plan vpc -s prod",
		},
		{
			name:     "single arg",
			args:     []string{"atmos"},
			expected: "atmos",
		},
		{
			name:     "args with special characters",
			args:     []string{"atmos", "terraform", "plan", "--var", "name=test value"},
			expected: "atmos terraform plan --var name=test value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveAndRestoreArgs(t)
			os.Args = tt.args
			result := buildCommandName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRunAIAnalysis verifies runAIAnalysis across different scenarios.
// Each case asserts on observable effects: stream restoration, capture buffer
// contents, and formatted error output — not just "no panic".
func TestRunAIAnalysis(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		stdout      string
		stderr      string
		cmdErr      error
		skillName   string
		skillPrompt string
	}{
		{
			name:   "captures stdout and restores streams",
			args:   []string{"atmos", "terraform", "plan"},
			stdout: "plan output",
			stderr: "debug info",
		},
		{
			name:   "formats command error to stderr",
			args:   []string{"atmos", "terraform", "apply"},
			stderr: "provider error",
			cmdErr: errors.New("exit status 1"),
		},
		{
			name:        "passes skill through to analysis",
			args:        []string{"atmos", "terraform", "plan", "--ai", "--skill", "atmos-terraform"},
			stdout:      "plan output",
			skillName:   "atmos-terraform",
			skillPrompt: "You are a Terraform expert.",
		},
		{
			name:   "handles error with empty captured stderr",
			args:   []string{"atmos", "plan"},
			cmdErr: errors.New("command failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect stderr to a temp file so we can inspect post-Stop output.
			// The tee goroutine and the error formatter both write to "old stderr",
			// which becomes this temp file.
			tmpFile, fileErr := os.CreateTemp(t.TempDir(), "stderr-*")
			require.NoError(t, fileErr)
			realStderr := os.Stderr
			os.Stderr = tmpFile
			// Register FIRST so it runs LAST in LIFO — after cs.Stop() restores os.Stderr.
			// Close the temp file before TempDir cleanup to avoid Windows "file in use" errors.
			t.Cleanup(func() {
				os.Stderr = realStderr
				tmpFile.Close()
			})

			// Save original stdout to verify restoration.
			origStdout := os.Stdout

			cs, err := analyze.StartCapture()
			require.NoError(t, err)
			// Register SECOND so it runs FIRST in LIFO — before stderr is fully restored.
			t.Cleanup(func() { cs.Stop() })

			if tt.stdout != "" {
				_, _ = os.Stdout.Write([]byte(tt.stdout))
			}
			if tt.stderr != "" {
				_, _ = os.Stderr.Write([]byte(tt.stderr))
			}

			saveAndRestoreArgs(t)
			os.Args = tt.args

			cfg := &schema.AtmosConfiguration{}
			runAIAnalysis(cfg, cs, tt.cmdErr, tt.skillName, tt.skillPrompt)

			// Verify streams are restored to pre-capture values.
			assert.Same(t, origStdout, os.Stdout, "os.Stdout should be restored after Stop")
			assert.Same(t, tmpFile, os.Stderr, "os.Stderr should be restored to pre-capture value")

			// Verify capture buffers contain the data written during capture.
			capturedStdout, capturedStderr := cs.Stop()
			if tt.stdout != "" {
				assert.Contains(t, capturedStdout, tt.stdout,
					"captured stdout buffer should contain data written during capture")
			}
			if tt.stderr != "" {
				assert.Contains(t, capturedStderr, tt.stderr,
					"captured stderr buffer should contain data written during capture")
			}

			// For error cases, verify the formatted error was written to real stderr.
			if tt.cmdErr != nil {
				_, seekErr := tmpFile.Seek(0, 0)
				require.NoError(t, seekErr)
				stderrOutput, readErr := io.ReadAll(tmpFile)
				require.NoError(t, readErr)
				assert.Contains(t, string(stderrOutput), tt.cmdErr.Error(),
					"formatted error should be written to stderr after capture stops")
			}
		})
	}
}

// TestRunAIAnalysis_DoubleStopSafe verifies idempotent Stop preserves buffered data.
func TestRunAIAnalysis_DoubleStopSafe(t *testing.T) {
	cs, err := analyze.StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	_, _ = os.Stdout.Write([]byte("buffered stdout"))

	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "plan"}

	cfg := &schema.AtmosConfiguration{}

	// First Stop happens inside runAIAnalysis.
	runAIAnalysis(cfg, cs, nil, "", "")

	// Second Stop should return the same buffered data without panic.
	stdout, _ := cs.Stop()
	assert.Contains(t, stdout, "buffered stdout",
		"second Stop should still return buffered data")
}

// TestSetupAIAnalysis verifies setupAIAnalysis across different configurations.
func TestSetupAIAnalysis(t *testing.T) {
	tests := []struct {
		name           string
		config         schema.AtmosConfiguration
		skillName      string
		expectErr      bool
		expectErrIs    error
		expectCapture  bool
		expectedPrompt string
	}{
		{
			name:      "AI not enabled",
			config:    schema.AtmosConfiguration{AI: schema.AISettings{Enabled: false}},
			expectErr: true,
		},
		{
			name:      "no provider configured",
			config:    schema.AtmosConfiguration{AI: schema.AISettings{Enabled: true, Providers: nil}},
			expectErr: true,
		},
		{
			name:          "valid config without skill",
			config:        schema.AtmosConfiguration{AI: validAIConfig()},
			expectCapture: true,
		},
		{
			name:        "invalid skill name",
			config:      schema.AtmosConfiguration{AI: validAIConfig()},
			skillName:   "nonexistent-skill",
			expectErr:   true,
			expectErrIs: errUtils.ErrAISkillNotFound,
		},
		{
			name: "valid config with skill",
			config: schema.AtmosConfiguration{AI: func() schema.AISettings {
				ai := validAIConfig()
				ai.Skills = map[string]*schema.AISkillConfig{
					"test-skill": {
						DisplayName:  "Test Skill",
						Description:  "A test skill",
						SystemPrompt: "You are a test expert.",
					},
				}
				return ai
			}()},
			skillName:      "test-skill",
			expectCapture:  true,
			expectedPrompt: "You are a test expert.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, skillPrompt, err := setupAIAnalysis(&tt.config, tt.skillName)
			if cs != nil {
				t.Cleanup(func() { cs.Stop() })
			}

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, cs)
				assert.Empty(t, skillPrompt)
				if tt.expectErrIs != nil {
					assert.True(t, errors.Is(err, tt.expectErrIs))
				}
			} else {
				assert.NoError(t, err)
				if tt.expectCapture {
					assert.NotNil(t, cs)
				}
				assert.Equal(t, tt.expectedPrompt, skillPrompt)
			}
		})
	}
}

// TestLoadAndValidateSkill verifies skill loading and validation.
func TestLoadAndValidateSkill(t *testing.T) {
	tests := []struct {
		name         string
		skills       map[string]*schema.AISkillConfig
		skillName    string
		expectErr    bool
		expectName   string
		expectPrompt string
	}{
		{
			name:      "skill not found with no skills installed",
			skills:    nil,
			skillName: "nonexistent",
			expectErr: true,
		},
		{
			name: "skill not found with other skills available",
			skills: map[string]*schema.AISkillConfig{
				"my-skill": {
					DisplayName:  "My Skill",
					Description:  "Available skill",
					SystemPrompt: "prompt",
				},
			},
			skillName: "wrong-skill",
			expectErr: true,
		},
		{
			name: "skill found",
			skills: map[string]*schema.AISkillConfig{
				"test-skill": {
					DisplayName:  "Test Skill",
					Description:  "A test skill",
					SystemPrompt: "You are a test expert.",
				},
			},
			skillName:    "test-skill",
			expectName:   "test-skill",
			expectPrompt: "You are a test expert.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &schema.AtmosConfiguration{
				AI: schema.AISettings{Skills: tt.skills},
			}

			skill, err := loadAndValidateSkill(cfg, tt.skillName)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, skill)
				assert.True(t, errors.Is(err, errUtils.ErrAISkillNotFound))
			} else {
				assert.NoError(t, err)
				require.NotNil(t, skill)
				assert.Equal(t, tt.expectName, skill.Name)
				assert.Equal(t, tt.expectPrompt, skill.SystemPrompt)
			}
		})
	}
}
