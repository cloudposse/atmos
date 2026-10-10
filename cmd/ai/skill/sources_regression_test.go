package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/ai/skills/source"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/flags"
)

// sourceCLI creates fresh command flags so regression tests do not leak singleton state.
func sourceCLI(t *testing.T, original *cobra.Command, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: original.Use, RunE: original.RunE, Args: original.Args, SilenceErrors: true, SilenceUsage: true}
	var parser *flags.StandardParser
	switch original.Name() {
	case "install":
		parser = installParser
	case "update":
		parser = updateParser
	case "uninstall":
		parser = uninstallParser
	}
	parser.RegisterFlags(cmd)
	sourceFlags().RegisterFlags(cmd)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func declaredFixture(t *testing.T, label string) string {
	t.Helper()
	dir := sourceCommandFixture(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("add"), "./local", "--name", label))
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("set"), label, "clients", "[gemini]"))
	return dir
}

func TestSourceRegressionLegacyUpdateGuards(t *testing.T) {
	for _, flag := range []string{"--dry-run", "--check", "--frozen"} {
		t.Run(flag, func(t *testing.T) {
			sourceCommandFixture(t)
			installer, err := marketplace.NewInstaller("1.0.0")
			require.NoError(t, err)
			require.NoError(t, installer.Install(context.Background(), "atmos-terraform", marketplace.InstallOptions{SkipConfirm: true}))
			installed, err := installer.Get("atmos-terraform")
			require.NoError(t, err)
			registry, err := marketplace.NewLocalRegistry()
			require.NoError(t, err)
			require.NoError(t, registry.Update(installed.Name, func(skill *marketplace.InstalledSkill) error {
				skill.Version = "0.0.1"
				return nil
			}))
			local := filepath.Join(installed.Path, "local.txt")
			require.NoError(t, os.WriteFile(local, []byte("keep"), 0o600))
			err = sourceCLI(t, updateCmd, "atmos-terraform", flag, "--yes")
			require.ErrorIs(t, err, source.ErrInvalid)
			require.FileExists(t, local)
		})
	}
}

func TestSourceRegressionNamedMutationsRequireConfirmation(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			dir := declaredFixture(t, "Team")
			if operation == "uninstall" {
				require.NoError(t, sourceCLI(t, installCmd, "--source", "Team", "--yes"))
			}
			cmd := installCmd
			if operation == "uninstall" {
				cmd = uninstallCmd
			}
			err := sourceCLI(t, cmd, "--source", "Team", "--scope", "project")
			require.ErrorIs(t, err, errUtils.ErrInteractiveNotAvailable)
			target := filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md")
			if operation == "install" {
				require.NoFileExists(t, target)
			} else {
				require.FileExists(t, target)
			}
		})
	}
}

func TestSourceRegressionUninstallDefaultScope(t *testing.T) {
	dir := declaredFixture(t, "Team")
	home, err := homedir.Dir()
	require.NoError(t, err)
	for _, scope := range []string{"project", "user"} {
		require.NoError(t, sourceCLI(t, installCmd, "--source", "Team", "--scope", scope, "--yes"))
	}
	require.NoError(t, sourceCLI(t, uninstallCmd, "demo", "--force"))
	require.NoDirExists(t, filepath.Join(dir, ".atmos", "skills", "content", "demo"))
	require.FileExists(t, filepath.Join(home, ".atmos", "skills", "content", "demo", "SKILL.md"))
}

func TestSourceRegressionSyncEnvironmentAndCompletedStatus(t *testing.T) {
	dir := declaredFixture(t, "Team")
	t.Setenv("ATMOS_AI_SKILL_SCOPE", "user")
	t.Setenv("ATMOS_AI_SKILL_CLIENT", "vscode")
	output := setupSkillCommandUI(t)
	require.NoError(t, executeSourceCommand(t, newSyncCommand()))
	home, err := homedir.Dir()
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(home, ".copilot", "skills", "demo", "SKILL.md"))
	require.NoDirExists(t, filepath.Join(dir, ".atmos", "skills", "content"))
	require.Contains(t, output.String(), "installed:")
	require.NotContains(t, output.String(), "missing:")
}

func TestSourceRegressionManualPathIgnoresDistribution(t *testing.T) {
	dir := sourceCommandFixture(t)
	manual := filepath.Join(dir, "manual")
	require.NoError(t, sourceCLI(t, installCmd, "./local", "--path", manual, "--global", "--client", "gemini", "--yes"))
	require.FileExists(t, filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md"))
	require.NoError(t, sourceCLI(t, updateCmd, "demo", "--yes"))
	home, err := homedir.Dir()
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(home, ".gemini"))
	require.NoDirExists(t, filepath.Join(dir, ".gemini"))
	require.FileExists(t, filepath.Join(manual, "demo", "SKILL.md"))
}

func TestSourceRegressionUpdateForceAndSameOwner(t *testing.T) {
	dir := declaredFixture(t, "demo")
	require.NoError(t, sourceCLI(t, installCmd, "demo", "--yes"))
	require.NoError(t, sourceCLI(t, installCmd, "demo", "--yes"))
	target := filepath.Join(dir, ".gemini", "skills", "demo", "SKILL.md")
	require.NoError(t, os.WriteFile(target, []byte("local edit"), 0o600))
	require.NoError(t, sourceCLI(t, updateCmd, "demo", "--force", "--yes"))
	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Contains(t, string(raw), "Instructions.")
	require.NoError(t, sourceCLI(t, uninstallCmd, "demo", "--force"))
	require.NoFileExists(t, target)
}

func TestSourceRegressionEditNameRejected(t *testing.T) {
	for _, operation := range []string{"set", "remove"} {
		t.Run(operation, func(t *testing.T) {
			dir := declaredFixture(t, "Team")
			before, err := os.ReadFile(filepath.Join(dir, "atmos.yaml"))
			require.NoError(t, err)
			args := []string{"Team"}
			if operation == "set" {
				args = append(args, "scope", "user")
			}
			args = append(args, "--name", "Other")
			require.Error(t, executeSourceCommand(t, newSourceEditCommand(operation), args...))
			after, err := os.ReadFile(filepath.Join(dir, "atmos.yaml"))
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSourceRegressionPreviewNeverPrompts(t *testing.T) {
	declaredFixture(t, "Team")
	for _, flag := range []string{"--dry-run", "--check"} {
		err := sourceCLI(t, installCmd, "--source", "Team", flag)
		if flag == "--check" {
			require.ErrorIs(t, err, source.ErrDrift)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestSourceRegressionInvalidUpdateDoesNotPrompt(t *testing.T) {
	declaredFixture(t, "Team")
	require.ErrorIs(t, sourceCLI(t, updateCmd, "--source", "Team", "--frozen"), source.ErrInvalid)
}

func TestSourceRegressionSyncForceEnvironment(t *testing.T) {
	t.Setenv("ATMOS_AI_SKILL_FORCE", "true")
	dir := declaredFixture(t, "Team")
	require.NoError(t, executeSourceCommand(t, newSyncCommand()))
	target := filepath.Join(dir, ".gemini", "skills", "demo", "SKILL.md")
	require.NoError(t, os.WriteFile(target, []byte("changed"), 0o600))
	require.NoError(t, executeSourceCommand(t, newSyncCommand()))
}
