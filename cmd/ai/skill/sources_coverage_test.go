package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/source"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestSourceCommandAmbiguousLabelRequiresExplicitSelector(t *testing.T) {
	for _, conflict := range []string{"local directory", "bundled name", "foreign owner"} {
		t.Run(conflict, func(t *testing.T) {
			label := "Team"
			if conflict == "bundled name" {
				label = "atmos-terraform"
			}
			dir := declaredFixture(t, label)
			switch conflict {
			case "local directory":
				require.NoError(t, os.Mkdir(filepath.Join(dir, label), 0o755))
			case "foreign owner":
				require.NoError(t, sourceCLI(t, installCmd, "Team", "--yes"))
				label = "demo"
				require.NoError(t, executeSourceCommand(t, newSourceEditCommand("add"), "./local", "--name", label))
			}
			err := sourceCLI(t, installCmd, label, "--dry-run")
			require.ErrorIs(t, err, source.ErrInvalid)
			require.ErrorContains(t, err, "use --source")
			if conflict != "foreign owner" {
				require.NoDirExists(t, filepath.Join(dir, ".atmos", "skills", "content"))
				require.NoError(t, sourceCLI(t, installCmd, "--source", label, "--yes"))
				require.FileExists(t, filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md"))
			}
		})
	}
}

func TestSourceCommandHonorsDeclaredUserScopeAndAllClients(t *testing.T) {
	dir := declaredFixture(t, "Team")
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("set"), "Team", "scope", "user"))
	require.NoError(t, sourceCLI(t, installCmd, "Team", "--all-clients", "--yes"))
	home, err := homedir.Dir()
	require.NoError(t, err)
	for _, clientDir := range []string{".claude", ".copilot", ".gemini"} {
		require.FileExists(t, filepath.Join(home, clientDir, "skills", "demo", "SKILL.md"))
		require.NoDirExists(t, filepath.Join(dir, clientDir))
	}
	require.NoDirExists(t, filepath.Join(dir, ".atmos", "skills", "content"))
}

func TestSourceCommandUpdateAllDeclaredAndAdHoc(t *testing.T) {
	dir := declaredFixture(t, "Team")
	adHoc := filepath.Join(dir, "extra")
	require.NoError(t, os.Mkdir(adHoc, 0o755))
	before := "---\nname: extra\ndescription: Additional skill\n---\n# Extra\nBefore update.\n"
	require.NoError(t, os.WriteFile(filepath.Join(adHoc, "SKILL.md"), []byte(before), 0o644))
	require.NoError(t, sourceCLI(t, installCmd, "Team", "--yes"))
	require.NoError(t, sourceCLI(t, installCmd, adHoc, "--yes"))
	for _, path := range []string{filepath.Join(dir, "local", "demo", "SKILL.md"), filepath.Join(adHoc, "SKILL.md")} {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(raw, []byte("Updated source instructions.\n")...), 0o644))
	}
	require.NoError(t, sourceCLI(t, updateCmd, "--yes"))
	for _, name := range []string{"demo", "extra"} {
		raw, err := os.ReadFile(filepath.Join(dir, ".atmos", "skills", "content", name, "SKILL.md"))
		require.NoError(t, err)
		require.Contains(t, string(raw), "Updated source instructions.")
	}
}

func TestSourceCommandUpdateAllDoesNotSelectOtherScopes(t *testing.T) {
	dir := sourceCommandFixture(t)
	require.NoError(t, sourceCLI(t, installCmd, "./local", "--global", "--yes"))
	home, err := homedir.Dir()
	require.NoError(t, err)
	target := filepath.Join(home, ".atmos", "skills", "content", "demo", "SKILL.md")
	before, err := os.ReadFile(target)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local", "demo", "SKILL.md"), append(before, []byte("New user source content.\n")...), 0o644))
	require.NoError(t, sourceCLI(t, updateCmd, "--yes"))
	after, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoDirExists(t, filepath.Join(dir, ".atmos", "skills", "content"))
	require.NoError(t, sourceCLI(t, updateCmd, "--global", "--yes"))
	after, err = os.ReadFile(target)
	require.NoError(t, err)
	require.Contains(t, string(after), "New user source content.")
}

func TestSourceCommandAllOperationsPreserveModifiedCopies(t *testing.T) {
	for _, declared := range []bool{true, false} {
		for _, operation := range []string{"update", "uninstall"} {
			t.Run(fmt.Sprintf("%s declared=%t", operation, declared), func(t *testing.T) {
				var dir string
				selector := "./local"
				if declared {
					dir = declaredFixture(t, "Team")
					selector = "Team"
				} else {
					dir = sourceCommandFixture(t)
				}
				require.NoError(t, sourceCLI(t, installCmd, selector, "--yes"))
				target := filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md")
				require.NoError(t, os.WriteFile(target, []byte("KEEP LOCAL EDITS"), 0o600))
				command, args := updateCmd, []string{"--yes"}
				if operation == "uninstall" {
					command, args = uninstallCmd, []string{"--dry-run"}
				}
				require.ErrorIs(t, sourceCLI(t, command, args...), source.ErrDrift)
				raw, err := os.ReadFile(target)
				require.NoError(t, err)
				require.Equal(t, "KEEP LOCAL EDITS", string(raw))
			})
		}
	}
}

func TestSourceCommandInvalidConfigDoesNotMutateInstallations(t *testing.T) {
	for _, operation := range []string{"edit", "sync", "install"} {
		t.Run(operation, func(t *testing.T) {
			dir := sourceCommandFixture(t)
			path := filepath.Join(dir, "atmos.yaml")
			invalid := []byte("ai: [invalid yaml")
			require.NoError(t, os.WriteFile(path, invalid, 0o600))
			var err error
			switch operation {
			case "edit":
				err = executeSourceCommand(t, newSourceEditCommand("add"), "./local", "--name", "Team")
			case "sync":
				err = executeSourceCommand(t, newSyncCommand())
			case "install":
				err = sourceCLI(t, installCmd, "./local", "--yes")
			}
			require.Error(t, err)
			raw, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, invalid, raw)
			require.NoDirExists(t, filepath.Join(dir, ".atmos"))
		})
	}
}

func TestSourceCommandCorruptStateFailsClosed(t *testing.T) {
	for _, command := range []*cobra.Command{installCmd, updateCmd} {
		t.Run(command.Name(), func(t *testing.T) {
			dir := declaredFixture(t, "Team")
			require.NoError(t, sourceCLI(t, installCmd, "Team", "--yes"))
			path := filepath.Join(dir, ".atmos", "skills", "installations.json")
			require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
			args := []string{"--yes"}
			if command.Name() == "install" {
				args = append(args, "Team")
			}
			var syntax *json.SyntaxError
			require.ErrorAs(t, sourceCLI(t, command, args...), &syntax)
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "{", string(raw))
			require.FileExists(t, filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md"))
		})
	}
}

func TestSourceCommandSyncSelectsSingleLabel(t *testing.T) {
	dir := declaredFixture(t, "Team")
	require.NoError(t, executeSourceCommand(t, newSourceEditCommand("add"), "./does-not-exist", "--name", "Unavailable"))
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "Team"))
	require.FileExists(t, filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md"))
	require.NoError(t, executeSourceCommand(t, newSyncCommand(), "Team", "--check"))
}

func TestSourceListRowsAndDetailsExposeInstallationContext(t *testing.T) {
	entry := listEntry{
		name: "demo", displayName: "Demo", displaySource: "Team", source: "Team", installed: true,
		sourceStatus: &source.Status{Status: "drifted", Scope: "user", Track: "stable", Path: "/owned/demo"},
	}
	rows := skillListRows([]listEntry{entry})
	require.Equal(t, []map[string]any{{"status_marker": markerInstalled, "name": "demo", "source": "Team", "state": "drifted", "category": "", "scope": "user", "track": "stable", "destination": "/owned/demo"}}, rows)
	details := renderEntryDetails([]listEntry{entry})
	for _, value := range []string{"State: drifted", "Scope: user", "Track: stable", "Destination: /owned/demo"} {
		require.Contains(t, strings.Join(strings.Fields(details), " "), value)
	}
}

func TestSourceCommandMissingAdHocResolutionFailsClosed(t *testing.T) {
	for _, named := range []bool{true, false} {
		t.Run(fmt.Sprintf("named=%t", named), func(t *testing.T) {
			dir := sourceCommandFixture(t)
			require.NoError(t, sourceCLI(t, installCmd, "./local", "--yes"))
			lock := filepath.Join(dir, ".atmos", "skills", "ad-hoc.lock.yaml")
			require.NoError(t, os.Remove(lock))
			target := filepath.Join(dir, ".atmos", "skills", "content", "demo", "SKILL.md")
			before, err := os.ReadFile(target)
			require.NoError(t, err)
			args := []string{"--yes"}
			if named {
				args = append(args, "demo")
			}
			err = sourceCLI(t, updateCmd, args...)
			require.ErrorIs(t, err, source.ErrDrift)
			require.ErrorContains(t, err, "missing local resolution")
			after, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoFileExists(t, lock)
		})
	}
}

func TestSourceCommandLegacyUninstallPreviewRequiresOwnership(t *testing.T) {
	for _, flag := range []string{"--dry-run", "--check", "--frozen"} {
		t.Run(flag, func(t *testing.T) {
			dir := sourceCommandFixture(t)
			err := sourceCLI(t, uninstallCmd, "atmos-terraform", flag, "--force")
			require.ErrorIs(t, err, source.ErrInvalid)
			require.ErrorContains(t, err, "reinstall legacy skills to establish ownership")
			require.NoDirExists(t, filepath.Join(dir, ".atmos"))
		})
	}
}
