package marketplace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestSkillVersionOutdated(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		catalog   string
		want      bool
	}{
		{"same version", "1.0.0", "1.0.0", false},
		{"different version", "1.0.0", "1.1.0", true},
		{"installed version empty", "", "1.0.0", false},
		{"catalog version empty", "1.0.0", "", false},
		{"both empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SkillVersionOutdated(tt.installed, tt.catalog))
		})
	}
}

func TestUpdateSkill_NotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	err = installer.UpdateSkill(context.Background(), "atmos-terraform", &InstallOptions{SkipConfirm: true})
	assert.ErrorIs(t, err, ErrSkillNotFound)
}

func TestUpdateSkill_AlreadyUpToDate(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, installer.Install(ctx, "atmos-terraform", InstallOptions{SkipConfirm: true}))

	err = installer.UpdateSkill(ctx, "atmos-terraform", &InstallOptions{SkipConfirm: true})
	assert.NoError(t, err, "updating an already-current bundled skill must be a no-op, not an error")
}

func TestUpdateSkill_ReinstallsWhenOutdated(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, installer.Install(ctx, "atmos-terraform", InstallOptions{SkipConfirm: true}))

	// Simulate staleness: record an older version than the catalog actually has.
	require.NoError(t, installer.localRegistry.Update("atmos-terraform", func(s *InstalledSkill) error {
		s.Version = "0.0.1-old"
		return nil
	}))

	err = installer.UpdateSkill(ctx, "atmos-terraform", &InstallOptions{SkipConfirm: true})
	require.NoError(t, err)

	skill, err := installer.Get("atmos-terraform")
	require.NoError(t, err)
	available, ok := LookupBundledSkill("atmos-terraform")
	require.True(t, ok)
	assert.Equal(t, available.Version, skill.Version, "update must reinstall to the current catalog version")
}

func TestUpdateSkill_NonBundledSkillUnsupported(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	require.NoError(t, installer.localRegistry.Add(&InstalledSkill{
		Name:    "some-git-skill",
		Source:  "github.com/someone/some-git-skill",
		Version: "1.0.0",
		Path:    tempDir,
	}))

	err = installer.UpdateSkill(context.Background(), "some-git-skill", &InstallOptions{SkipConfirm: true})
	require.ErrorIs(t, err, ErrSkillUpdateNotSupported)
	assert.Contains(t, err.Error(), "github.com/someone/some-git-skill",
		"the error must point at the recorded source so the user knows what to --force reinstall")
}

func TestUpdateAllBundled_NothingInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	err = installer.UpdateAllBundled(&InstallOptions{SkipConfirm: true, BasePath: tempDir})
	assert.NoError(t, err, "no installed bundled skills means nothing to update, not an error")
}

func TestUpdateAllBundled_UpdatesOnlyOutdatedSkills(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, installer.Install(ctx, "atmos-terraform", InstallOptions{SkipConfirm: true}))

	require.NoError(t, installer.localRegistry.Update("atmos-terraform", func(s *InstalledSkill) error {
		s.Version = "0.0.1-old"
		return nil
	}))

	err = installer.UpdateAllBundled(&InstallOptions{SkipConfirm: true, BasePath: tempDir})
	require.NoError(t, err)

	skill, err := installer.Get("atmos-terraform")
	require.NoError(t, err)
	available, ok := LookupBundledSkill("atmos-terraform")
	require.True(t, ok)
	assert.Equal(t, available.Version, skill.Version)
}
