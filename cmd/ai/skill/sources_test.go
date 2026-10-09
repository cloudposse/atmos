package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func sourceCommandFixture(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(dir)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	setupSkillCommandUI(t)
	setupSkillListOutput(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("base_path: .\nai:\n  enabled: true\n"), 0o644))
	skillDir := filepath.Join(dir, "local", "demo")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Example\n---\n# Demo\nInstructions.\n"), 0o644))
	return dir
}

func executeSourceCommand(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestSourceCommandsKeepConfigAndInstallationIndependent(t *testing.T) {
	dir := sourceCommandFixture(t)
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("add"), "./local", "--name", "Team"))
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("set"), "Team", "clients", "[claude-code, gemini]"))
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("set"), "Team", "ref", "!version skills"))
	require.NoDirExists(t, filepath.Join(dir, ".atmos"))
	require.Error(t, executeSourceCommand(t, newSyncCommand(), "--check"))
	require.NoDirExists(t, filepath.Join(dir, ".atmos"))
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("set"), "Team", "ref", "main"))
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "--dry-run"))
	require.NoDirExists(t, filepath.Join(dir, ".atmos"))
	require.NoError(t, executeSourceCommand(t, newSyncCommand()))
	target := filepath.Join(dir, ".claude", "skills", "demo", "SKILL.md")
	require.FileExists(t, target)
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "--frozen"))
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "--check"))
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("remove"), "Team"))
	require.FileExists(t, target)
	require.NoError(t, executeSourceCommand(t, newSyncCommand()))
	require.FileExists(t, target)
	require.Error(t, executeSourceCommand(t, newSyncCommand(), "--check"))
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "--prune"))
	require.NoFileExists(t, target)
}

func TestSourceCommandSelectionAndRecoveryGuards(t *testing.T) {
	sourceCommandFixture(t)
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("add"), "./local", "--name", "Team"))
	require.Error(t, executeSourceCommand(t, newSourceEditCommand("add"), "./local", "--name", "Team"))
	require.Error(t, executeSourceCommand(t, newSyncCommand(), "Team", "--source", "other"))
	for _, flag := range []string{"--dry-run", "--check", "--frozen"} {
		require.ErrorContains(t, executeSourceCommand(t, newSyncCommand(), "--recover", flag), "cannot be combined")
	}
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "--recover"))
}
