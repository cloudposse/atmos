package embedded_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/skills/embedded"
)

func TestListNames_IncludesAtmosPro(t *testing.T) {
	names, err := embedded.ListNames()
	require.NoError(t, err)
	assert.Contains(t, names, "atmos-pro", "atmos-pro skill must be bundled")
	// Spot-check that the full skill catalog is present — regression guard against
	// the embed directive silently dropping subfolders.
	for _, want := range []string{"atmos-terraform", "atmos-stacks", "atmos-auth", "atmos-gitops"} {
		assert.Contains(t, names, want, "expected %q in embedded skills", want)
	}
}

func TestLoad_AtmosPro(t *testing.T) {
	skill, err := embedded.Load("atmos-pro")
	require.NoError(t, err)

	assert.Equal(t, "atmos-pro", skill.Name)
	assert.True(t, skill.IsBuiltIn, "embedded skills must be marked IsBuiltIn")
	assert.NotEmpty(t, skill.Description)

	// The system prompt combines SKILL.md body with referenced files.
	// SKILL.md must be at the top; reference files should be appended after "## Reference:".
	assert.Contains(t, skill.SystemPrompt, "# Atmos Pro Onboarding",
		"SystemPrompt should start with the SKILL.md H1")
	// Frontmatter must be stripped.
	assert.NotContains(t, skill.SystemPrompt, "---\nname: atmos-pro",
		"frontmatter must not leak into the system prompt")
}

func TestLoad_MissingSkill(t *testing.T) {
	_, err := embedded.Load("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SKILL.md")
}

func TestLoadAll_PopulatesRegistry(t *testing.T) {
	registry := skills.NewRegistry()
	require.NoError(t, embedded.LoadAll(registry))

	atmosPro, err := registry.Get("atmos-pro")
	require.NoError(t, err)
	assert.True(t, atmosPro.IsBuiltIn)

	// All embedded skills must be registered.
	names, err := embedded.ListNames()
	require.NoError(t, err)
	assert.Equal(t, len(names), registry.Count(), "every embedded skill must be registered")
}

func TestLoadAll_DoesNotOverrideExisting(t *testing.T) {
	// Seed the registry with a "marketplace-installed" skill named atmos-pro so
	// the embedded loader must defer to it.
	registry := skills.NewRegistry()
	override := &skills.Skill{
		Name:         "atmos-pro",
		Description:  "user override",
		SystemPrompt: "override content",
		IsBuiltIn:    false,
	}
	require.NoError(t, registry.Register(override))

	require.NoError(t, embedded.LoadAll(registry))

	got, err := registry.Get("atmos-pro")
	require.NoError(t, err)
	assert.False(t, got.IsBuiltIn, "marketplace override must survive")
	assert.Equal(t, "override content", got.SystemPrompt, "embedded must not clobber override")
}

func TestLoader_ImplementsSkillLoader(t *testing.T) {
	// Compile-time: embedded.Loader must satisfy skills.SkillLoader.
	var _ skills.SkillLoader = embedded.Loader{}

	// Runtime: the LoadInstalledSkills method must populate the registry.
	registry := skills.NewRegistry()
	require.NoError(t, embedded.Loader{}.LoadInstalledSkills(registry))
	assert.True(t, registry.Has("atmos-pro"))
}

func TestLoad_AtmosProReferencesIncluded(t *testing.T) {
	skill, err := embedded.Load("atmos-pro")
	require.NoError(t, err)

	// Verify references were concatenated. The atmos-pro skill ships with 8
	// reference docs and each gets a "## Reference: <filename>" section header.
	refs := strings.Count(skill.SystemPrompt, "## Reference: ")
	assert.GreaterOrEqual(t, refs, 1, "at least one reference must be embedded")

	// Specifically confirm the onboarding playbook reference is loaded.
	assert.Contains(t, skill.SystemPrompt, "## Reference: onboarding-playbook.md")
}
