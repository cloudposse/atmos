package source

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstalledSkillPrecedenceAndSourceExclusion(t *testing.T) {
	e, repo := fixture(t)
	t.Setenv("HOME", e.Home)
	t.Setenv("USERPROFILE", e.Home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	run(t, e, Options{Scope: "user"})
	write(t, filepath.Join(repo, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: Demo\n---\n# Demo\nProject prompt\n")
	run(t, e, Options{Update: true, Scope: "project"})
	e.Config.AI.Skills["demo"] = &schema.AISkillConfig{SystemPrompt: "inline prompt"}
	installer, err := marketplace.NewInstaller("1.0.0")
	require.NoError(t, err)
	registry, err := skills.LoadSkills(e.Config, installer)
	require.NoError(t, err)
	demo, err := registry.Get("demo")
	require.NoError(t, err)
	require.Contains(t, demo.SystemPrompt, "Project prompt")
	_, err = registry.Get("test")
	require.Error(t, err, "source declarations must not become inline skills")
	run(t, e, Options{Uninstall: true, Scope: "project"})
	registry, err = skills.LoadSkills(e.Config, installer)
	require.NoError(t, err)
	demo, err = registry.Get("demo")
	require.NoError(t, err)
	require.Contains(t, demo.SystemPrompt, "Original")
	run(t, e, Options{Uninstall: true, Scope: "user"})
	registry, err = skills.LoadSkills(e.Config, installer)
	require.NoError(t, err)
	demo, err = registry.Get("demo")
	require.NoError(t, err)
	require.Equal(t, "inline prompt", demo.SystemPrompt)
}
