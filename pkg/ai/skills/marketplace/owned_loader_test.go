package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills"
)

type ownedLoaderRecord struct {
	Name   string `json:"name"`
	Client string `json:"client"`
	Path   string `json:"path"`
}

func ownedLoaderFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func ownedLoaderState(t *testing.T, base string, records ...ownedLoaderRecord) string {
	t.Helper()
	raw, err := json.Marshal(struct {
		Version int                 `json:"version"`
		Records []ownedLoaderRecord `json:"records"`
	}{Version: 1, Records: records})
	require.NoError(t, err)
	path := filepath.Join(base, ".atmos", "skills", "installations.json")
	ownedLoaderFile(t, path, string(raw))
	return path
}

func ownedLoaderBase(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return base
}

func TestLoadProjectSkillsMetadataReferencesAndPrecedence(t *testing.T) {
	project, user := ownedLoaderBase(t), ownedLoaderBase(t)
	registry := skills.NewRegistry()
	installer := &Installer{}
	for _, fixture := range []struct{ base, body string }{{project, "PROJECT INSTRUCTIONS"}, {user, "USER INSTRUCTIONS"}} {
		dir := filepath.Join(fixture.base, ".atmos", "skills", "content", "demo")
		ownedLoaderFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: demo\ndescription: Deployment guidance\nmetadata:\n  display_name: Deployment Expert\n  category: operations\nallowed-tools: [Read]\nrestricted-tools: [Bash]\nreferences: [reference.md, ../../../../outside.txt]\n---\n"+fixture.body+"\n")
		ownedLoaderFile(t, filepath.Join(dir, "reference.md"), "LOCAL REFERENCE")
		ownedLoaderFile(t, filepath.Join(fixture.base, "outside.txt"), "SECRET OUTSIDE SKILL")
		ownedLoaderState(t, fixture.base,
			ownedLoaderRecord{Name: "demo", Client: "gemini", Path: filepath.Join(fixture.base, ".gemini", "skills", "demo")},
			ownedLoaderRecord{Name: "demo", Client: "atmos", Path: dir})
	}

	// The default base resolves against the caller's project, and the first loaded
	// project skill must retain precedence when user installations are loaded later.
	t.Chdir(project)
	require.NoError(t, installer.LoadProjectSkills(registry, ""))
	require.NoError(t, loadOwnedSkills(registry, filepath.Join(user, ".atmos", "skills")))
	loaded, err := registry.Get("demo")
	require.NoError(t, err)
	require.Equal(t, "Deployment Expert", loaded.DisplayName)
	require.Equal(t, "Deployment guidance", loaded.Description)
	require.Equal(t, "operations", loaded.Category)
	require.Equal(t, []string{"Read"}, loaded.AllowedTools)
	require.Equal(t, []string{"Bash"}, loaded.RestrictedTools)
	require.Contains(t, loaded.SystemPrompt, "PROJECT INSTRUCTIONS")
	require.Contains(t, loaded.SystemPrompt, "## Reference: reference.md\n\nLOCAL REFERENCE")
	require.NotContains(t, loaded.SystemPrompt, "USER INSTRUCTIONS")
	require.NotContains(t, loaded.SystemPrompt, "SECRET OUTSIDE SKILL")
	require.Len(t, registry.List(), 1)
}

func TestLoadProjectSkillsSkipsUnavailableSkills(t *testing.T) {
	base := ownedLoaderBase(t)
	dir := filepath.Join(base, ".atmos", "skills", "content")
	ownedLoaderFile(t, filepath.Join(dir, "broken", "SKILL.md"), "not skill metadata")
	ownedLoaderFile(t, filepath.Join(dir, "healthy", "SKILL.md"), "---\nname: healthy\ndescription: Still usable\n---\nHEALTHY INSTRUCTIONS\n")
	ownedLoaderState(t, base,
		ownedLoaderRecord{Name: "missing", Client: "atmos", Path: filepath.Join(dir, "missing")},
		ownedLoaderRecord{Name: "broken", Client: "atmos", Path: filepath.Join(dir, "broken")},
		ownedLoaderRecord{Name: "healthy", Client: "atmos", Path: filepath.Join(dir, "healthy")})
	registry := skills.NewRegistry()
	require.NoError(t, (&Installer{}).LoadProjectSkills(registry, base))
	loaded := registry.List()
	require.Len(t, loaded, 1)
	require.Equal(t, "healthy", loaded[0].Name)
	require.Equal(t, "HEALTHY INSTRUCTIONS", loaded[0].SystemPrompt)
}

func TestLoadProjectSkillsRejectsInvalidState(t *testing.T) {
	for _, tc := range []struct {
		name          string
		state         string
		metadataError bool
	}{
		{name: "malformed JSON", state: "{"},
		{name: "unsupported version", state: `{"version":2}`, metadataError: true},
		{name: "empty name", state: `{"version":1,"records":[{"client":"atmos"}]}`, metadataError: true},
		{name: "traversal name", state: `{"version":1,"records":[{"client":"atmos","name":"../outside"}]}`, metadataError: true},
		{name: "wrong destination", state: `{"version":1,"records":[{"client":"atmos","name":"demo","path":"elsewhere"}]}`, metadataError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := ownedLoaderBase(t)
			ownedLoaderFile(t, filepath.Join(base, ".atmos", "skills", "installations.json"), tc.state)
			registry := skills.NewRegistry()
			err := (&Installer{}).LoadProjectSkills(registry, base)
			if tc.metadataError {
				require.ErrorIs(t, err, ErrInvalidMetadata)
			} else {
				var syntax *json.SyntaxError
				require.ErrorAs(t, err, &syntax)
			}
			require.Empty(t, registry.List())
		})
	}
}

func TestLoadProjectSkillsStateReadFailures(t *testing.T) {
	for _, tc := range []string{"missing project", "state is directory", "missing state"} {
		t.Run(tc, func(t *testing.T) {
			base := ownedLoaderBase(t)
			switch tc {
			case "missing project":
				base = filepath.Join(base, "missing")
			case "state is directory":
				require.NoError(t, os.MkdirAll(filepath.Join(base, ".atmos", "skills", "installations.json"), 0o755))
			}
			registry := skills.NewRegistry()
			err := (&Installer{}).LoadProjectSkills(registry, base)
			if tc == "missing state" {
				require.NoError(t, err)
			} else {
				var pathError *os.PathError
				require.ErrorAs(t, err, &pathError)
			}
			require.Empty(t, registry.List())
		})
	}
}

func TestLoadProjectSkillsRejectsSymlinkedStateAndContent(t *testing.T) {
	for _, component := range []string{"installations.json", "content", "SKILL.md"} {
		t.Run(component, func(t *testing.T) {
			base := ownedLoaderBase(t)
			dir := filepath.Join(base, ".atmos", "skills", "content", "demo")
			ownedLoaderFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: demo\ndescription: Example\n---\nINSTRUCTIONS\n")
			state := ownedLoaderState(t, base, ownedLoaderRecord{Name: "demo", Client: "atmos", Path: dir})
			target := state
			switch component {
			case "content":
				target = filepath.Dir(dir)
			case "SKILL.md":
				target = filepath.Join(dir, "SKILL.md")
			}
			moved := target + ".original"
			require.NoError(t, os.Rename(target, moved))
			if err := os.Symlink(moved, target); err != nil && runtime.GOOS == "windows" {
				t.Skipf("symlinks require Windows developer mode or privilege: %v", err)
			} else {
				require.NoError(t, err)
			}
			registry := skills.NewRegistry()
			require.ErrorIs(t, (&Installer{}).LoadProjectSkills(registry, base), ErrInvalidMetadata)
			require.Empty(t, registry.List())
		})
	}
}
