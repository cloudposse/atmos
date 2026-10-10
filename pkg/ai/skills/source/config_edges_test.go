package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigEditRejectsInvalidRequestsWithoutChangingFile(t *testing.T) {
	for _, edit := range []EditOptions{
		{Operation: "set", Label: "../escape", Field: "source", Value: "org/repo"},
		{Operation: "set", Label: "Team", Field: "ref", Value: "!version "},
		{Operation: "unknown", Label: "Team"},
	} {
		t.Run(edit.Operation+edit.Label+edit.Value, func(t *testing.T) {
			dir := realTemp(t)
			t.Chdir(dir)
			file := filepath.Join(dir, "atmos.yaml")
			const original = "ai:\n  skills:\n    Team:\n      source: org/repo\n"
			write(t, file, original)
			edit.File = file
			_, err := Edit(&schema.AtmosConfiguration{BasePath: dir}, edit)
			require.ErrorIs(t, err, ErrInvalid)
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, original, string(after))
		})
	}
}

func TestConfigEditAddsToDefaultFileAndValidatesExistingMergedConfig(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	file := filepath.Join(dir, "atmos.yaml")
	write(t, file, "# preserve\nai:\n  enabled: true\n")
	config := &schema.AtmosConfiguration{BasePath: dir}
	_, err := Edit(config, EditOptions{Operation: "add", Label: "Team", Value: "org/repo"})
	require.NoError(t, err)
	config.AI.Skills = map[string]*schema.AISkillConfig{"Team": {Source: "org/repo", Clients: []string{"gemini"}}}
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Team", Field: "ref", Value: "main"})
	require.NoError(t, err)
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(raw), "# preserve")
	require.Contains(t, string(raw), "ref: main")
	require.NoDirExists(t, filepath.Join(dir, ".atmos"))
}

func TestConfigEditRejectsMalformedInheritedDeclaration(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	file := filepath.Join(dir, "atmos.yaml")
	const original = "ai:\n  skills:\n    Team:\n      clients: [gemini]\n"
	write(t, file, original)
	write(t, filepath.Join(dir, ".atmos.d", "source.yaml"), "ai:\n  skills:\n    Team:\n      source: [not, a, string]\n")
	_, err := Edit(&schema.AtmosConfiguration{BasePath: dir}, EditOptions{Operation: "set", Label: "Team", Field: "ref", Value: "main"})
	require.ErrorContains(t, err, "cannot unmarshal")
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, original, string(raw))
}

func TestStatusRejectsPendingTransactionAndInvalidDeclaration(t *testing.T) {
	e, _ := fixture(t)
	require.NoError(t, e.saveJournal(journal{Project: e.Project}))
	_, err := e.Status(context.Background(), Options{})
	require.ErrorIs(t, err, ErrRecovery)
	require.NoError(t, e.Recover())
	e.Config.AI.Skills["test"].Kind = "unsupported"
	_, err = e.Status(context.Background(), Options{})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestResolveInstalledRequiresExplicitSourceForAmbiguousName(t *testing.T) {
	e, repo := fixture(t)
	run(t, e, Options{})
	e.Config.AI.Skills["other"] = &schema.AISkillConfig{Source: repo, Scope: scopeUser, Clients: []string{"gemini"}}
	run(t, e, Options{Source: "other"})
	_, err := e.ResolveInstalled("demo", "")
	require.ErrorIs(t, err, ErrInvalid)
	require.ErrorContains(t, err, "use --source")
	source, err := e.ResolveInstalled("demo", scopeUser)
	require.NoError(t, err)
	require.Equal(t, "other", source)
}
