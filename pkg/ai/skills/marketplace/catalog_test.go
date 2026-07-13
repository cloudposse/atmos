package marketplace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentskills "github.com/cloudposse/atmos/agent-skills"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// newBundledTestInstaller sets up an isolated HOME with a reset homedir cache and
// returns an installer rooted at that temp HOME. It centralizes the temp-dir +
// homedir.Reset boilerplate shared by the bundled-catalog tests below.
func newBundledTestInstaller(t *testing.T) *Installer {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	return installer
}

// Compile-time sentinel: fails the build if any AvailableSkill field is renamed,
// so the catalog tests below cannot silently reference stale fields.
var _ = AvailableSkill{Name: "", DisplayName: "", Description: "", Version: "", Source: ""}

func TestCatalog(t *testing.T) {
	catalog, err := Catalog()
	require.NoError(t, err)

	// The catalog should expose every valid bundled official skill. Keep this
	// tied to the embedded skill tree so adding a skill does not require updating
	// a stale magic number.
	require.Len(t, catalog, bundledSkillCount(t))

	// Entries are sorted by name; assert first and last by value, not just length.
	first := catalog[0]
	assert.Equal(t, "atmos-ai", first.Name)
	assert.Equal(t, "1.0.0", first.Version)
	assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-ai", first.Source)
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

func bundledSkillCount(t *testing.T) int {
	t.Helper()

	entries, err := fs.ReadDir(agentskills.Skills, bundledSkillsRoot)
	require.NoError(t, err)

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := LookupBundledSkill(entry.Name()); ok {
			count++
		}
	}
	return count
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

// TestInstall_BundledWithCustomName covers the --as path of installBundledSkill:
// the on-disk install name differs from the canonical embedded name, while the
// recorded Source still points at the upstream skill.
func TestInstall_BundledWithCustomName(t *testing.T) {
	installer := newBundledTestInstaller(t)

	opts := InstallOptions{SkipConfirm: true, CustomName: "my-tf"}
	require.NoError(t, installer.Install(context.Background(), "atmos-terraform", opts))

	// Registered under the custom name, not the canonical one.
	installed, err := installer.Get("my-tf")
	require.NoError(t, err)
	assert.Equal(t, "my-tf", installed.Name)
	assert.Equal(t, "1.0.0", installed.Version)
	assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform", installed.Source)
	assert.False(t, installed.IsBuiltIn)
	assert.True(t, installed.Enabled)

	// The canonical name is NOT registered.
	_, err = installer.Get("atmos-terraform")
	assert.Error(t, err)

	// Files materialized under the custom directory name.
	skillsDir, err := GetSkillsDir()
	require.NoError(t, err)
	installPath := filepath.Join(skillsDir, "my-tf")
	assert.FileExists(t, filepath.Join(installPath, "SKILL.md"))
}

// TestInstallAllBundled covers `atmos ai skill install` with no <source>:
// every bundled skill gets installed in one call, with a single upfront
// confirmation rather than one per skill.
func TestInstallAllBundled(t *testing.T) {
	installer := newBundledTestInstaller(t)

	catalog, err := Catalog()
	require.NoError(t, err)
	require.NotEmpty(t, catalog, "the embedded catalog must be non-empty for this test to be meaningful")

	opts := &InstallOptions{SkipConfirm: true}
	require.NoError(t, installer.InstallAllBundled(opts))

	installed := installer.List()
	assert.Len(t, installed, len(catalog))

	// Spot-check a specific skill landed on disk and in the registry, not
	// just that the count matches.
	skill, err := installer.Get("atmos-terraform")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(skill.Path, "SKILL.md"))

	t.Run("re-running without --force skips everything already installed", func(t *testing.T) {
		require.NoError(t, installer.InstallAllBundled(opts))
		assert.Len(t, installer.List(), len(catalog), "no duplicates or new entries")
	})

	t.Run("re-running with --force reinstalls everything", func(t *testing.T) {
		forceOpts := &InstallOptions{SkipConfirm: true, Force: true}
		require.NoError(t, installer.InstallAllBundled(forceOpts))
		assert.Len(t, installer.List(), len(catalog))
	})
}

// TestInstall_BundledForceReplacesOnDiskDir covers the --force branch of
// prepareInstallPath: an existing on-disk directory (with stale content that is
// not in the registry) is removed and replaced rather than rejected.
func TestInstall_BundledForceReplacesOnDiskDir(t *testing.T) {
	installer := newBundledTestInstaller(t)

	skillsDir, err := GetSkillsDir()
	require.NoError(t, err)
	installPath := filepath.Join(skillsDir, "atmos-terraform")

	// Pre-create a stale install dir on disk (not in the registry) with a file
	// that must be gone after a forced reinstall.
	require.NoError(t, os.MkdirAll(installPath, 0o755))
	staleFile := filepath.Join(installPath, "stale.txt")
	require.NoError(t, os.WriteFile(staleFile, []byte("old"), 0o600))

	// Without --force, an existing on-disk dir is rejected.
	err = installer.Install(context.Background(), "atmos-terraform", InstallOptions{SkipConfirm: true})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSkillAlreadyInstalled))

	// With --force, the stale dir is removed and the bundled skill installed.
	require.NoError(t, installer.Install(context.Background(), "atmos-terraform", InstallOptions{SkipConfirm: true, Force: true}))
	assert.NoFileExists(t, staleFile)
	assert.FileExists(t, filepath.Join(installPath, "SKILL.md"))

	// And it is now registered.
	installed, err := installer.Get("atmos-terraform")
	require.NoError(t, err)
	assert.Equal(t, "atmos-terraform", installed.Name)
}

// TestInstall_BundledAlreadyInRegistry covers the registry pre-check in
// installBundledSkill (distinct from the on-disk check in prepareInstallPath):
// a name already present in the local registry is rejected without --force.
func TestInstall_BundledAlreadyInRegistry(t *testing.T) {
	installer := newBundledTestInstaller(t)

	require.NoError(t, installer.Install(context.Background(), "atmos-terraform", InstallOptions{SkipConfirm: true}))

	// Second install with the same name and no --force is rejected.
	err := installer.Install(context.Background(), "atmos-terraform", InstallOptions{SkipConfirm: true})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSkillAlreadyInstalled))
}

// TestInstall_BundledThenListMerges verifies that after installing a bundled
// skill, LoadInstalledSkills returns it alongside the rest, exercising the
// installed half of the available-vs-installed view.
func TestInstall_BundledThenListMerges(t *testing.T) {
	installer := newBundledTestInstaller(t)

	require.NoError(t, installer.Install(context.Background(), "atmos-terraform", InstallOptions{SkipConfirm: true}))
	require.NoError(t, installer.Install(context.Background(), "atmos-helmfile", InstallOptions{SkipConfirm: true}))

	installed := installer.List()
	require.Len(t, installed, 2)

	names := make(map[string]bool, len(installed))
	for _, s := range installed {
		names[s.Name] = true
	}
	assert.True(t, names["atmos-terraform"], "expected installed atmos-terraform in list")
	assert.True(t, names["atmos-helmfile"], "expected installed atmos-helmfile in list")

	// Every catalog entry that is not installed is still discoverable via the
	// bundled catalog, confirming available and installed are independent views.
	catalog, err := Catalog()
	require.NoError(t, err)
	require.NotEmpty(t, catalog)
	_, ok := LookupBundledSkill("atmos-stacks")
	assert.True(t, ok, "a non-installed catalog skill remains available")
}

// TestReadBundledMetadata covers readBundledMetadata for both a present and an
// absent bundled skill.
func TestReadBundledMetadata(t *testing.T) {
	t.Run("known skill parses", func(t *testing.T) {
		md, err := readBundledMetadata("atmos-terraform")
		require.NoError(t, err)
		require.NotNil(t, md)
		assert.NotEmpty(t, md.Description)
		assert.Equal(t, "1.0.0", md.GetVersion())
	})

	t.Run("unknown skill errors", func(t *testing.T) {
		_, err := readBundledMetadata("does-not-exist")
		require.Error(t, err)
	})
}

// TestCopyFS_FromBundledSkill exercises copyFS directly against an embedded
// skill subtree, asserting the complete tree (SKILL.md plus nested references)
// is written to disk and contents are preserved.
func TestCopyFS_FromBundledSkill(t *testing.T) {
	skillFS, err := bundledSkillFS("atmos-terraform")
	require.NoError(t, err)

	dst := t.TempDir()
	require.NoError(t, copyFS(skillFS, dst))

	// Top-level SKILL.md copied with real content.
	skillMD := filepath.Join(dst, "SKILL.md")
	require.FileExists(t, skillMD)
	data, err := os.ReadFile(skillMD)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Nested reference file under references/ was recreated.
	assert.FileExists(t, filepath.Join(dst, "references", "commands-reference.md"))
}
