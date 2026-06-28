package marketplace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// Compile-time sentinel: fails the build if any AvailableSkill field is renamed,
// so the catalog tests below cannot silently reference stale fields.
var _ = AvailableSkill{Name: "", DisplayName: "", Description: "", Version: "", Source: ""}

func TestCatalog(t *testing.T) {
	catalog, err := Catalog()
	require.NoError(t, err)

	// The repository bundles exactly the 22 official skills.
	require.Len(t, catalog, 22)

	// Entries are sorted by name; assert first and last by value, not just length.
	first := catalog[0]
	assert.Equal(t, "atmos-ansible", first.Name)
	assert.Equal(t, "1.0.0", first.Version)
	assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-ansible", first.Source)
	assert.NotEmpty(t, first.Description)

	last := catalog[len(catalog)-1]
	assert.Equal(t, "atmos-yaml-functions", last.Name)
	assert.Equal(t, "1.0.0", last.Version)
	assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-yaml-functions", last.Source)
	assert.NotEmpty(t, last.Description)

	// Every entry is fully populated.
	for _, s := range catalog {
		assert.NotEmpty(t, s.Name)
		assert.NotEmpty(t, s.DisplayName)
		assert.NotEmpty(t, s.Description)
		assert.NotEmpty(t, s.Version)
		assert.Contains(t, s.Source, "github.com/cloudposse/atmos//agent-skills/skills/")
	}
}

func TestLookupBundledSkill(t *testing.T) {
	t.Run("known skill resolves", func(t *testing.T) {
		s, ok := LookupBundledSkill("atmos-terraform")
		require.True(t, ok)
		assert.Equal(t, "atmos-terraform", s.Name)
		assert.Equal(t, "1.0.0", s.Version)
		assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform", s.Source)
	})

	t.Run("unknown skill does not resolve", func(t *testing.T) {
		_, ok := LookupBundledSkill("does-not-exist")
		assert.False(t, ok)
	})

	t.Run("a URL-like source is not a bundled name", func(t *testing.T) {
		// Bundled names are bare; anything with slashes must miss so it falls
		// through to the Git install path.
		_, ok := LookupBundledSkill("github.com/cloudposse/atmos")
		assert.False(t, ok)
	})
}

func TestInstall_BundledOffline(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	// Installs entirely from the embedded catalog — no network/Git.
	require.NoError(t, installer.Install(ctx, "atmos-terraform", opts))

	// Registered with the canonical source and version.
	installed, err := installer.Get("atmos-terraform")
	require.NoError(t, err)
	assert.Equal(t, "atmos-terraform", installed.Name)
	assert.Equal(t, "1.0.0", installed.Version)
	assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform", installed.Source)

	// The complete skill was copied to disk, including reference files.
	skillsDir, err := GetSkillsDir()
	require.NoError(t, err)
	installPath := filepath.Join(skillsDir, "atmos-terraform")
	assert.FileExists(t, filepath.Join(installPath, "SKILL.md"))
	assert.FileExists(t, filepath.Join(installPath, "references", "commands-reference.md"))

	t.Run("reinstall without force is rejected", func(t *testing.T) {
		err := installer.Install(ctx, "atmos-terraform", opts)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSkillAlreadyInstalled))
	})

	t.Run("reinstall with force succeeds", func(t *testing.T) {
		forceOpts := InstallOptions{SkipConfirm: true, Force: true}
		require.NoError(t, installer.Install(ctx, "atmos-terraform", forceOpts))
	})
}

func TestInstall_BundledUnknownNameFallsThroughToSourceError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// A bare token that is not a bundled skill must not silently install; it
	// falls through to ParseSource which rejects it as an invalid source.
	err = installer.Install(context.Background(), "definitely-not-a-bundled-skill", InstallOptions{SkipConfirm: true})
	require.Error(t, err)

	// And nothing was registered.
	_, getErr := installer.Get("definitely-not-a-bundled-skill")
	assert.Error(t, getErr)
}
