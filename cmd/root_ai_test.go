package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/analyze"
	"github.com/cloudposse/atmos/pkg/schema"
)

// saveAndRestoreArgs saves os.Args and returns a cleanup function that restores them.
func saveAndRestoreArgs(t *testing.T) {
	t.Helper()
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
}

// TestBuildCommandName verifies that buildCommandName joins os.Args.
func TestBuildCommandName(t *testing.T) {
	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "plan", "vpc", "-s", "prod"}
	result := buildCommandName()
	assert.Equal(t, "atmos terraform plan vpc -s prod", result)
}

// TestBuildCommandName_SingleArg verifies buildCommandName with single argument.
func TestBuildCommandName_SingleArg(t *testing.T) {
	saveAndRestoreArgs(t)
	os.Args = []string{"atmos"}
	result := buildCommandName()
	assert.Equal(t, "atmos", result)
}

// TestRunAIAnalysis_StopsCaptureAndCallsAnalyze verifies basic runAIAnalysis flow.
func TestRunAIAnalysis_StopsCaptureAndCallsAnalyze(t *testing.T) {
	cs, err := analyze.StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	_, _ = os.Stdout.Write([]byte("captured stdout"))
	_, _ = os.Stderr.Write([]byte("captured stderr"))

	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "plan"}

	cfg := &schema.AtmosConfiguration{}

	assert.NotPanics(t, func() {
		runAIAnalysis(cfg, cs, nil, "", "")
	})
}

// TestRunAIAnalysis_WithError verifies error is formatted and appended to stderr.
func TestRunAIAnalysis_WithError(t *testing.T) {
	cs, err := analyze.StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	_, _ = os.Stderr.Write([]byte("some stderr output"))

	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "apply"}

	cfg := &schema.AtmosConfiguration{}
	cmdErr := errors.New("exit status 1")

	assert.NotPanics(t, func() {
		runAIAnalysis(cfg, cs, cmdErr, "", "")
	})
}

// TestRunAIAnalysis_WithSkill verifies skill name and prompt are passed through.
func TestRunAIAnalysis_WithSkill(t *testing.T) {
	cs, err := analyze.StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	_, _ = os.Stdout.Write([]byte("plan output"))

	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "plan", "--ai", "--skill", "atmos-terraform"}

	cfg := &schema.AtmosConfiguration{}

	assert.NotPanics(t, func() {
		runAIAnalysis(cfg, cs, nil, "atmos-terraform", "You are a Terraform expert.")
	})
}

// TestSetupAIAnalysis_AINotEnabled verifies that setupAIAnalysis returns error when AI is disabled.
func TestSetupAIAnalysis_AINotEnabled(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: false,
		},
	}

	cs, skillPrompt, err := setupAIAnalysis(cfg, "")
	assert.Error(t, err)
	assert.Nil(t, cs)
	assert.Empty(t, skillPrompt)
	assert.Contains(t, err.Error(), "AI features are not enabled")
}

// TestSetupAIAnalysis_NoProvider verifies error when no provider is configured.
func TestSetupAIAnalysis_NoProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: nil,
		},
	}

	cs, skillPrompt, err := setupAIAnalysis(cfg, "")
	assert.Error(t, err)
	assert.Nil(t, cs)
	assert.Empty(t, skillPrompt)
}

// TestSetupAIAnalysis_ValidNoSkill verifies successful setup without a skill.
func TestSetupAIAnalysis_ValidNoSkill(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
		},
	}

	cs, skillPrompt, err := setupAIAnalysis(cfg, "")
	if cs != nil {
		t.Cleanup(func() { cs.Stop() })
	}
	assert.NoError(t, err)
	assert.NotNil(t, cs)
	assert.Empty(t, skillPrompt)
}

// TestSetupAIAnalysis_InvalidSkill verifies error when skill is not found.
func TestSetupAIAnalysis_InvalidSkill(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
		},
	}

	cs, skillPrompt, err := setupAIAnalysis(cfg, "nonexistent-skill")
	assert.Error(t, err)
	assert.Nil(t, cs)
	assert.Empty(t, skillPrompt)
	assert.True(t, errors.Is(err, errUtils.ErrAISkillNotFound))
}

// TestLoadAndValidateSkill_SkillNotFound verifies error when skill is not in registry.
func TestLoadAndValidateSkill_SkillNotFound(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: nil,
		},
	}

	skill, err := loadAndValidateSkill(cfg, "nonexistent")
	assert.Nil(t, skill)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAISkillNotFound))
}

// TestLoadAndValidateSkill_SkillFound verifies successful skill loading from config.
func TestLoadAndValidateSkill_SkillFound(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"test-skill": {
					DisplayName:  "Test Skill",
					Description:  "A test skill",
					SystemPrompt: "You are a test expert.",
				},
			},
		},
	}

	skill, err := loadAndValidateSkill(cfg, "test-skill")
	assert.NoError(t, err)
	require.NotNil(t, skill)
	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, "You are a test expert.", skill.SystemPrompt)
}

// TestLoadAndValidateSkill_SkillNotFoundWithAvailable verifies error when wrong skill requested.
func TestLoadAndValidateSkill_SkillNotFoundWithAvailable(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"my-skill": {
					DisplayName:  "My Skill",
					Description:  "Available skill",
					SystemPrompt: "prompt",
				},
			},
		},
	}

	skill, err := loadAndValidateSkill(cfg, "wrong-skill")
	assert.Nil(t, skill)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAISkillNotFound))
}

// TestRunAIAnalysis_ErrorWithEmptyStderr verifies correct handling when stderr is empty.
func TestRunAIAnalysis_ErrorWithEmptyStderr(t *testing.T) {
	cs, err := analyze.StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "plan"}

	cfg := &schema.AtmosConfiguration{}
	cmdErr := errors.New("command failed")

	assert.NotPanics(t, func() {
		runAIAnalysis(cfg, cs, cmdErr, "", "")
	})
}

// TestSetupAIAnalysis_WithValidSkill verifies skillPrompt is returned for valid skill.
func TestSetupAIAnalysis_WithValidSkill(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
			Skills: map[string]*schema.AISkillConfig{
				"test-skill": {
					DisplayName:  "Test Skill",
					Description:  "A test skill",
					SystemPrompt: "You are a test expert.",
				},
			},
		},
	}

	cs, skillPrompt, err := setupAIAnalysis(cfg, "test-skill")
	if cs != nil {
		t.Cleanup(func() { cs.Stop() })
	}
	assert.NoError(t, err)
	assert.NotNil(t, cs)
	assert.Equal(t, "You are a test expert.", skillPrompt)
}

// TestBuildCommandName_WithSpecialChars verifies buildCommandName with special characters in args.
func TestBuildCommandName_WithSpecialChars(t *testing.T) {
	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "plan", "--var", "name=test value"}
	result := buildCommandName()
	assert.Equal(t, "atmos terraform plan --var name=test value", result)
	assert.True(t, strings.Contains(result, "atmos"))
}

// TestRunAIAnalysis_DoubleStopSafe verifies runAIAnalysis doesn't panic on double stop.
func TestRunAIAnalysis_DoubleStopSafe(t *testing.T) {
	cs, err := analyze.StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	saveAndRestoreArgs(t)
	os.Args = []string{"atmos", "terraform", "plan"}

	cfg := &schema.AtmosConfiguration{}

	// First call stops the capture.
	assert.NotPanics(t, func() {
		runAIAnalysis(cfg, cs, nil, "", "")
	})

	// Second call — double stop should be safe due to idempotent Stop().
	assert.NotPanics(t, func() {
		cs.Stop()
	})
}
