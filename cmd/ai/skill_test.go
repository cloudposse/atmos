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
	assert.Contains(t, SkillCmd.Long, "info")
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
