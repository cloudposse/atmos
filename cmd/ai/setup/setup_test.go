package setup

import (
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosErrors "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// saveAndRestoreArgs saves os.Args and restores them during test cleanup.
func saveAndRestoreArgs(t *testing.T) {
	t.Helper()
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
}

// validAIConfig returns a minimal valid AI configuration for testing.
func validAIConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
		},
	}
}

// TestIsAISubcommand tests that AI subcommand detection works via Cobra command tree.
func TestIsAISubcommand(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	aiCmd := &cobra.Command{Use: "ai"}
	chatCmd := &cobra.Command{Use: "chat"}
	terraformCmd := &cobra.Command{Use: "terraform"}
	planCmd := &cobra.Command{Use: "plan"}

	aiCmd.AddCommand(chatCmd)
	root.AddCommand(aiCmd, terraformCmd)
	terraformCmd.AddCommand(planCmd)

	tests := []struct {
		name     string
		cmd      *cobra.Command
		expected bool
	}{
		{name: "ai command is AI subcommand", cmd: aiCmd, expected: true},
		{name: "ai chat is AI subcommand", cmd: chatCmd, expected: true},
		{name: "terraform is not AI subcommand", cmd: terraformCmd, expected: false},
		{name: "terraform plan is not AI subcommand", cmd: planCmd, expected: false},
		{name: "root is not AI subcommand", cmd: root, expected: false},
		{name: "nil is not AI subcommand", cmd: nil, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAISubcommand(tt.cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestInitAI_NoAIFlag returns disabled context when --ai is not present.
func TestInitAI_NoAIFlag(t *testing.T) {
	saveAndRestoreArgs(t)
	//nolint:tenv // Must set os.Args directly to test pre-Cobra flag parsing.
	os.Args = []string{"atmos", "terraform", "plan"}
	t.Setenv("ATMOS_AI", "")

	cfg := validAIConfig()
	ctx, err := InitAI(cfg)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.False(t, ctx.Enabled(), "should return disabled context when --ai is not present")
}

// TestInitAI_SkillWithoutAI returns error when --skill is used without --ai.
func TestInitAI_SkillWithoutAI(t *testing.T) {
	saveAndRestoreArgs(t)
	//nolint:tenv // Must set os.Args directly to test pre-Cobra flag parsing.
	os.Args = []string{"atmos", "--skill", "atmos-terraform", "terraform", "plan"}
	t.Setenv("ATMOS_AI", "")

	cfg := validAIConfig()
	_, err := InitAI(cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, atmosErrors.ErrAISkillRequiresAIFlag))
}

// TestInitAI_SkillWithoutAI_EnvVar tests env var --skill without --ai.
func TestInitAI_SkillWithoutAI_EnvVar(t *testing.T) {
	saveAndRestoreArgs(t)
	//nolint:tenv // Must set os.Args directly to test pre-Cobra flag parsing.
	os.Args = []string{"atmos", "terraform", "plan"}
	t.Setenv("ATMOS_AI", "")
	t.Setenv("ATMOS_SKILL", "atmos-terraform")

	cfg := validAIConfig()
	_, err := InitAI(cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, atmosErrors.ErrAISkillRequiresAIFlag))
}

// TestInitAI_HelpWithSkillEnvVar verifies --help short-circuits before skill validation.
func TestInitAI_HelpWithSkillEnvVar(t *testing.T) {
	saveAndRestoreArgs(t)
	//nolint:tenv // Must set os.Args directly to test pre-Cobra flag parsing.
	os.Args = []string{"atmos", "terraform", "plan", "--help"}
	t.Setenv("ATMOS_SKILL", "atmos-terraform")
	t.Setenv("ATMOS_AI", "")

	cfg := validAIConfig()
	ctx, err := InitAI(cfg)
	require.NoError(t, err, "ATMOS_SKILL with --help should not return ErrAISkillRequiresAIFlag")
	require.NotNil(t, ctx)
	assert.False(t, ctx.Enabled(), "--help should return disabled context")
}

// TestInitAI_HelpWithAIEnvVar verifies --help short-circuits before provider validation.
func TestInitAI_HelpWithAIEnvVar(t *testing.T) {
	saveAndRestoreArgs(t)
	//nolint:tenv // Must set os.Args directly to test pre-Cobra flag parsing.
	os.Args = []string{"atmos", "--help"}
	t.Setenv("ATMOS_AI", "true")

	// Use a config with no providers to ensure validation would fail.
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{Enabled: true},
	}
	ctx, err := InitAI(cfg)
	require.NoError(t, err, "ATMOS_AI=true with --help should not fail provider validation")
	require.NotNil(t, ctx)
	assert.False(t, ctx.Enabled(), "--help should return disabled context")
}

// TestInitAI_ShortHelpFlag verifies -h short-circuits like --help.
func TestInitAI_ShortHelpFlag(t *testing.T) {
	saveAndRestoreArgs(t)
	//nolint:tenv // Must set os.Args directly to test pre-Cobra flag parsing.
	os.Args = []string{"atmos", "terraform", "-h"}
	t.Setenv("ATMOS_AI", "true")
	t.Setenv("ATMOS_SKILL", "atmos-terraform")

	cfg := validAIConfig()
	ctx, err := InitAI(cfg)
	require.NoError(t, err, "-h should short-circuit AI setup")
	require.NotNil(t, ctx)
	assert.False(t, ctx.Enabled(), "-h should return disabled context")
}
