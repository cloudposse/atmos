package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
