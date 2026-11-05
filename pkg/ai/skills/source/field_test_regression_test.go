package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProjectRootSourceExcludesGeneratedContent(t *testing.T) {
	for _, layout := range []string{"skills/demo", "."} {
		t.Run(layout, func(t *testing.T) {
			e, _ := fixture(t)
			e.Config.AI.Skills["test"].Source = "."
			root := filepath.Join(e.Project, layout)
			write(t, filepath.Join(root, "SKILL.md"), "---\nname: demo\ndescription: Demo\n---\n# Demo\nOriginal\n")
			write(t, filepath.Join(root, "reference.md"), "authored content")
			write(t, filepath.Join(root, ".claude", "authored.md"), "authored hidden content")
			run(t, e, Options{})
			lockBefore, err := os.ReadFile(e.lockPath())
			require.NoError(t, err)
			run(t, e, Options{Check: true})
			run(t, e, Options{})
			lockAfter, err := os.ReadFile(e.lockPath())
			require.NoError(t, err)
			require.Equal(t, lockBefore, lockAfter)
			canonical, _ := e.destination("project", "atmos", "demo")
			require.NoDirExists(t, filepath.Join(canonical, ".atmos"))
			require.NoFileExists(t, filepath.Join(canonical, "skills.lock.yaml"))
			require.NoDirExists(t, filepath.Join(canonical, ".claude", "skills", "demo"))
			require.FileExists(t, filepath.Join(canonical, ".claude", "authored.md"))
			write(t, filepath.Join(root, "reference.md"), "changed authored content")
			_, err = e.Run(context.Background(), Options{Check: true})
			require.ErrorIs(t, err, ErrDrift)
			run(t, e, Options{Update: true})
			run(t, e, Options{Check: true})
		})
	}
}

func TestEditValidatesEffectiveLayeredDeclaration(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	root := filepath.Join(dir, "atmos.yaml")
	fragment := filepath.Join(dir, ".atmos.d", "source.yaml")
	write(t, fragment, "ai:\n  skills:\n    Team:\n      source: org/repo\n      ref: !version dep\n")
	write(t, root, "# preserved\nai:\n  skills:\n    Team:\n      clients: [gemini]\n")
	config := &schema.AtmosConfiguration{BasePath: dir}
	before, err := os.ReadFile(root)
	require.NoError(t, err)
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Team", Field: "ref", Value: "main", DryRun: true})
	require.NoError(t, err)
	after, err := os.ReadFile(root)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Team", Field: "ref", Value: "main"})
	require.NoError(t, err)
	after, err = os.ReadFile(root)
	require.NoError(t, err)
	require.Contains(t, string(after), "ref: main")
	require.NotContains(t, string(after), "source:")
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Team", Field: "source", Value: ""})
	require.ErrorIs(t, err, ErrInvalid)
	unchanged, err := os.ReadFile(root)
	require.NoError(t, err)
	require.Equal(t, after, unchanged)
}

func TestConfigEditPreservesMode(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	file := filepath.Join(dir, "atmos.yaml")
	write(t, file, "ai:\n  skills:\n    Team:\n      source: org/repo\n")
	require.NoError(t, os.Chmod(file, 0o640))
	before, err := os.Stat(file)
	require.NoError(t, err)
	_, err = Edit(&schema.AtmosConfiguration{BasePath: dir}, EditOptions{Operation: "set", Label: "Team", Field: "ref", Value: "main", File: file})
	require.NoError(t, err)
	info, err := os.Stat(file)
	require.NoError(t, err)
	// Windows exposes the read-only attribute rather than Unix permission bits.
	require.Equal(t, before.Mode().Perm(), info.Mode().Perm())
}

func TestExplicitLowerLayerEditRespectsHigherPrecedence(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	root := filepath.Join(dir, "atmos.yaml")
	fragment := filepath.Join(dir, ".atmos.d", "source.yaml")
	write(t, fragment, "ai:\n  skills:\n    Team:\n      source: lower/repo\n")
	write(t, root, "ai:\n  skills:\n    Team:\n      source: higher/repo\n")
	_, err := Edit(&schema.AtmosConfiguration{BasePath: dir}, EditOptions{
		Operation: "set", Label: "Team", Field: "source", Value: "", File: ".atmos.d/source.yaml",
	})
	require.NoError(t, err, "the effective source still comes from the higher-precedence root")
	raw, err := os.ReadFile(root)
	require.NoError(t, err)
	require.Contains(t, string(raw), "higher/repo")
}

func TestUninstallRestrictsExplicitTrack(t *testing.T) {
	e, _ := fixture(t)
	run(t, e, Options{Track: "blue"})
	before, err := os.ReadFile(filepath.Join(e.stateDir("project"), "installations.json"))
	require.NoError(t, err)
	statuses := run(t, e, Options{Uninstall: true, Track: "red"})
	require.Empty(t, statuses)
	after, err := os.ReadFile(filepath.Join(e.stateDir("project"), "installations.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	run(t, e, Options{Check: true, Track: "blue"})
	run(t, e, Options{Uninstall: true, Track: "blue"})
	target, _ := e.destination("project", "atmos", "demo")
	require.NoDirExists(t, target)
}

func TestManualOnlyUpdateDoesNotDiscoverNewClientCopies(t *testing.T) {
	e, repo := fixture(t)
	manual := filepath.Join(e.Project, "manual")
	run(t, e, Options{Path: manual})
	write(t, filepath.Join(repo, "skills", "demo", "new.txt"), "updated")
	run(t, e, Options{Update: true})
	require.FileExists(t, filepath.Join(manual, "demo", "new.txt"))
	for _, client := range []string{"claude-code", "gemini"} {
		p, _ := e.destination("project", client, "demo")
		require.NoDirExists(t, p)
	}
	run(t, e, Options{Update: true, Clients: []string{"gemini"}})
	p, _ := e.destination("project", "gemini", "demo")
	require.FileExists(t, filepath.Join(p, "new.txt"))
	write(t, filepath.Join(repo, "skills", "demo", "new.txt"), "updated again")
	run(t, e, Options{Update: true})
	data, err := os.ReadFile(filepath.Join(p, "new.txt"))
	require.NoError(t, err)
	require.Equal(t, "updated again", string(data), "previously installed clients continue to update")
}
