package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const reviewSkill = "---\nname: demo\ndescription: Example\n---\n# Demo\nContent\n"

func TestDiscoveryRejectsInvalidPackagesWithoutInstalling(t *testing.T) {
	cases := []struct{ name, manifest, pluginManifest, skill, want string }{
		{name: "malformed marketplace", manifest: `{`, want: "unexpected end"},
		{name: "duplicate plugin", manifest: `{"plugins":[{"name":"one","source":"."},{"name":"one","source":"."}]}`, want: "duplicate plugin"},
		{name: "invalid source object", manifest: `{"plugins":[{"name":"one","source":12}]}`, want: "cannot unmarshal"},
		{name: "unsupported source", manifest: `{"plugins":[{"name":"one","source":{"source":"npm"}}]}`, want: "unsupported plugin source"},
		{name: "source traversal", manifest: `{"plugins":[{"name":"one","source":"../outside"}]}`, want: "traversal"},
		{name: "malformed plugin", manifest: `{"plugins":[{"name":"one","source":"."}]}`, pluginManifest: `{`, want: "unexpected end"},
		{name: "invalid skill paths", manifest: `{"plugins":[{"name":"one","source":".","skills":42}]}`, want: "cannot unmarshal"},
		{name: "skill path traversal", manifest: `{"plugins":[{"name":"one","source":".","skills":["../outside"]}]}`, want: "traversal"},
		{name: "malformed skill metadata", skill: "---\nname: [\n---\n# Demo\n", want: "parse"},
		{name: "invalid skill name", skill: "---\nname: Bad_Name\ndescription: Example\n---\n# Demo\n", want: "skill name"},
		{name: "invalid skill compatibility", skill: "---\nname: demo\ndescription: Example\ncompatibility:\n  atmos: garbage\n---\n# Demo\n", want: "compatibility"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, repo := fixture(t)
			if tc.manifest != "" {
				write(t, filepath.Join(repo, ".claude-plugin", "marketplace.json"), tc.manifest)
			}
			if tc.pluginManifest != "" {
				write(t, filepath.Join(repo, ".claude-plugin", "plugin.json"), tc.pluginManifest)
			}
			if tc.skill != "" {
				write(t, filepath.Join(repo, "skills", "demo", "SKILL.md"), tc.skill)
			}
			_, err := e.Run(context.Background(), Options{})
			require.ErrorContains(t, err, tc.want)
			require.NoFileExists(t, e.lockPath())
			target, err := e.destination(scopeProject, canonicalClient, "demo")
			require.NoError(t, err)
			require.NoDirExists(t, target)
			require.NoError(t, e.pending())
		})
	}
}

func TestMarketplacePrefixAndRepeatedPathsSelectOneSkill(t *testing.T) {
	e, repo := fixture(t)
	require.NoError(t, os.RemoveAll(filepath.Join(repo, "skills")))
	write(t, filepath.Join(repo, "README.md"), "not a skill")
	write(t, filepath.Join(repo, ".claude-plugin", "marketplace.json"), `{"metadata":{"pluginRoot":"plugins"},"plugins":[{"name":"one","source":"one","skills":["custom","./custom"]}]}`)
	write(t, filepath.Join(repo, "plugins", "one", "custom", "demo", "SKILL.md"), reviewSkill)
	run(t, e, Options{})
	var lock Lock
	require.NoError(t, readYAML(e.lockPath(), &lock))
	skills := lock.Tracks["default"]["test"].Skills
	require.Len(t, skills, 1)
	require.Equal(t, "one", skills[0].Plugin)
	require.Equal(t, "plugins/one/custom/demo", skills[0].Path)
	run(t, e, Options{Check: true})
}

func TestDiscoveryRejectsEmptyAndDuplicateSkills(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "duplicate"}[duplicate], func(t *testing.T) {
			e, repo := fixture(t)
			if duplicate {
				write(t, filepath.Join(repo, "skills", "other", "SKILL.md"), reviewSkill)
			} else {
				require.NoError(t, os.RemoveAll(filepath.Join(repo, "skills")))
				write(t, filepath.Join(repo, "README.md"), "only docs")
			}
			_, err := e.Run(context.Background(), Options{})
			if duplicate {
				require.ErrorContains(t, err, "duplicate skill demo")
			} else {
				require.ErrorContains(t, err, "no selected skills")
			}
			require.NoFileExists(t, e.lockPath())
		})
	}
}

func TestDiscoveryRejectsEscapingSubpathAndFileSkillCollection(t *testing.T) {
	for _, subpath := range []string{"../outside", "README.md"} {
		t.Run(subpath, func(t *testing.T) {
			e, repo := fixture(t)
			write(t, filepath.Join(repo, "README.md"), "not a directory")
			e.Config.AI.Skills["test"].Subpath = subpath
			_, err := e.Run(context.Background(), Options{})
			require.Error(t, err)
			require.NoFileExists(t, e.lockPath())
		})
	}
}
