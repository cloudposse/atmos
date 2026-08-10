package skill

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestUpdateCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "update [name]", updateCmd.Use)
	assert.Equal(t, "Update installed bundled skills to their latest catalog version", updateCmd.Short)
	assert.NotEmpty(t, updateCmd.Long)
	assert.NotNil(t, updateCmd.RunE)
}

func TestUpdateCmd_Flags(t *testing.T) {
	for _, name := range []string{"yes", "path", clientFlag, "all-clients", scopeFlag, "global"} {
		flag := updateCmd.Flags().Lookup(name)
		require.NotNil(t, flag, "%s flag should be registered", name)
	}
}

func TestUpdateCmd_ArgsValidation(t *testing.T) {
	assert.NoError(t, updateCmd.Args(updateCmd, []string{}))
	assert.NoError(t, updateCmd.Args(updateCmd, []string{"atmos-terraform"}))
	assert.Error(t, updateCmd.Args(updateCmd, []string{"a", "b"}))
}

// setupUpdateTestEnv isolates HOME and CWD the same way install_test.go's
// RunE tests do, so a real `atmos ai skill install`/`update` round trip never
// touches the operator's real ~/.atmos or reads this repo's own .claude/.
func setupUpdateTestEnv(t *testing.T) string {
	t.Helper()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	t.Chdir(t.TempDir())

	return tempHome
}

func resetUpdateFlags(t *testing.T) {
	t.Helper()
	resetFlagChangedForTest(t, updateCmd, "yes")
	resetFlagChangedForTest(t, updateCmd, "path")
	resetFlagChangedForTest(t, updateCmd, scopeFlag)
	resetFlagChangedForTest(t, updateCmd, "global")
	resetFlagChangedForTest(t, updateCmd, "all-clients")
	resetStringSliceFlagForTest(t, updateCmd)
}

func TestUpdateCmd_RunE_AlreadyUpToDate(t *testing.T) {
	resetUpdateFlags(t)
	t.Cleanup(func() { resetUpdateFlags(t) })
	tempHome := setupUpdateTestEnv(t)

	require.NoError(t, installCmd.Flags().Set("yes", "true"))
	t.Cleanup(func() { resetFlagChangedForTest(t, installCmd, "yes") })
	require.NoError(t, installCmd.RunE(installCmd, []string{"atmos-terraform"}))
	require.FileExists(t, filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform", "SKILL.md"))

	uiOutput := setupSkillCommandUI(t)
	require.NoError(t, updateCmd.Flags().Set("yes", "true"))
	err := updateCmd.RunE(updateCmd, []string{"atmos-terraform"})
	require.NoError(t, err)
	assert.Contains(t, atmosansi.Strip(uiOutput.String()), "already up to date")
}

func TestUpdateCmd_RunE_NotInstalled(t *testing.T) {
	resetUpdateFlags(t)
	t.Cleanup(func() { resetUpdateFlags(t) })
	setupUpdateTestEnv(t)

	setupSkillCommandUI(t)
	require.NoError(t, updateCmd.Flags().Set("yes", "true"))
	err := updateCmd.RunE(updateCmd, []string{"atmos-terraform"})
	require.Error(t, err, "updating a skill that was never installed must fail, not silently install it")
}

func TestUpdateCmd_RunE_NoArgsAllUpToDate(t *testing.T) {
	resetUpdateFlags(t)
	t.Cleanup(func() { resetUpdateFlags(t) })
	setupUpdateTestEnv(t)

	require.NoError(t, installCmd.Flags().Set("yes", "true"))
	t.Cleanup(func() { resetFlagChangedForTest(t, installCmd, "yes") })
	require.NoError(t, installCmd.RunE(installCmd, []string{"atmos-terraform"}))

	uiOutput := setupSkillCommandUI(t)
	require.NoError(t, updateCmd.Flags().Set("yes", "true"))
	err := updateCmd.RunE(updateCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, atmosansi.Strip(uiOutput.String()), "already up to date")
}

func TestUpdateCmd_RunE_InvalidClientRejected(t *testing.T) {
	resetUpdateFlags(t)
	t.Cleanup(func() { resetUpdateFlags(t) })
	setupUpdateTestEnv(t)

	setupSkillCommandUI(t)
	require.NoError(t, updateCmd.Flags().Set("client", "not-a-real-client"))
	err := updateCmd.RunE(updateCmd, []string{"atmos-terraform"})
	require.Error(t, err, "an unrecognized --client value must be rejected before any work happens")
}

func TestUpdateCmd_CommandRegistration(t *testing.T) {
	assert.NotNil(t, updateCmd)
	assert.NotNil(t, updateCmd.RunE)
	assert.NotNil(t, updateCmd.Flags())
}
