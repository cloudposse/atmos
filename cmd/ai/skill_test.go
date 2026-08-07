package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSkillCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "skill", SkillCmd.Use)
	assert.Equal(t, "Manage AI skills", SkillCmd.Short)
	assert.NotEmpty(t, SkillCmd.Long)
}

func TestSkillCmd_LongDescription(t *testing.T) {
	// Verify long description contains important information.
	assert.Contains(t, SkillCmd.Long, "Manage community and custom AI skills")
	assert.Contains(t, SkillCmd.Long, "agentskills.io")
	assert.Contains(t, SkillCmd.Long, "GitHub repositories")
}

func TestSkillCmd_AvailableCommands(t *testing.T) {
	// Verify the long description lists available commands.
	assert.Contains(t, SkillCmd.Long, "install")
	assert.Contains(t, SkillCmd.Long, "list")
	assert.Contains(t, SkillCmd.Long, "uninstall")
	// "info" is not a registered subcommand (only install/list/uninstall are);
	// the help text must not advertise a command that doesn't exist.
	assert.NotContains(t, SkillCmd.Long, "info")
}

// TestSkillCmd_SubcommandDescriptionsMatchCobraShort covers that the embedded
// help markdown's one-line subcommand descriptions match what install/list's own
// Cobra Short fields actually say (see cmd/ai/skill/install.go, list.go), instead
// of the stale "Install a skill from a GitHub repository" (undersold the
// bundled-offline path) and "List installed skills" (wrong -- list shows
// available AND installed by default) wording that used to drift from them.
func TestSkillCmd_SubcommandDescriptionsMatchCobraShort(t *testing.T) {
	assert.Contains(t, SkillCmd.Long, "Install bundled or GitHub-hosted AI skills")
	assert.Contains(t, SkillCmd.Long, "List available and installed skills")
}

func TestSkillCmd_Examples(t *testing.T) {
	// Verify the long description contains examples.
	assert.Contains(t, SkillCmd.Long, "atmos ai skill install github.com/user/skill-name")
	assert.Contains(t, SkillCmd.Long, "@v1.2.3")
	assert.Contains(t, SkillCmd.Long, "atmos ai skill list")
	assert.Contains(t, SkillCmd.Long, "atmos ai skill uninstall skill-name")
}

func TestSkillCmd_CanHaveSubcommands(t *testing.T) {
	// SkillCmd is designed to have subcommands registered via init() in skill package.
	// The actual subcommands (install, list, uninstall) are tested in the skill package.
	// Here we just verify the command is set up correctly to receive subcommands.
	assert.NotNil(t, SkillCmd)
	assert.Equal(t, "skill", SkillCmd.Name())
}

func TestSkillCmd_ParentCommand(t *testing.T) {
	// SkillCmd should be attached to aiCmd.
	parent := SkillCmd.Parent()
	assert.NotNil(t, parent)
	assert.Equal(t, "ai", parent.Name())
}
