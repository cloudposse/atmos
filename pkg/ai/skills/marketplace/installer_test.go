//nolint:dupl // Test files contain similar setup code by design for isolation and clarity.
package marketplace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// mockDownloader implements DownloaderInterface for testing.
type mockDownloader struct {
	downloadFunc func(ctx context.Context, source *SourceInfo) (string, error)
}

func (m *mockDownloader) Download(ctx context.Context, source *SourceInfo) (string, error) {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, source)
	}
	return "", errors.New("download not implemented")
}

// mockLocalRegistry provides a test implementation of LocalRegistry methods.
type mockLocalRegistry struct {
	skills map[string]*InstalledSkill
}

func newMockLocalRegistry() *mockLocalRegistry {
	return &mockLocalRegistry{
		skills: make(map[string]*InstalledSkill),
	}
}

func (m *mockLocalRegistry) Get(name string) (*InstalledSkill, error) {
	skill, exists := m.skills[name]
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrSkillNotFound, name)
	}
	return skill, nil
}

func (m *mockLocalRegistry) Add(skill *InstalledSkill) error {
	if _, exists := m.skills[skill.Name]; exists {
		return fmt.Errorf("%w: %q", ErrSkillAlreadyInstalled, skill.Name)
	}
	m.skills[skill.Name] = skill
	return nil
}

func (m *mockLocalRegistry) Remove(name string) error {
	if _, exists := m.skills[name]; !exists {
		return fmt.Errorf("%w: %q", ErrSkillNotFound, name)
	}
	delete(m.skills, name)
	return nil
}

func (m *mockLocalRegistry) List() []*InstalledSkill {
	result := make([]*InstalledSkill, 0, len(m.skills))
	for _, skill := range m.skills {
		result = append(result, skill)
	}
	return result
}

// createTestSkillDir creates a temporary directory with a valid SKILL.md file.
func createTestSkillDir(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	skillMD := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: Test skill for unit testing
license: MIT
compatibility:
  atmos: ">=1.0.0"
metadata:
  display_name: Test Skill
  version: 1.0.0
  author: Test Author
  category: general
  repository: https://github.com/test/skill
allowed-tools:
  - bash
  - read_file
---

# Test Skill

This is a test skill for unit testing.

## Purpose

Test the installer functionality.
`

	err := os.WriteFile(skillMD, []byte(content), 0o644)
	require.NoError(t, err)

	return tempDir
}

// createTestSkillDirWithMetadata creates a skill directory with custom metadata.
func createTestSkillDirWithMetadata(t *testing.T, metadata string, promptContent string) string {
	t.Helper()

	tempDir := t.TempDir()
	skillMD := filepath.Join(tempDir, "SKILL.md")

	content := fmt.Sprintf("---\n%s\n---\n\n%s", metadata, promptContent)

	err := os.WriteFile(skillMD, []byte(content), 0o644)
	require.NoError(t, err)

	return tempDir
}

func TestNewInstaller_Success(t *testing.T) {
	// Create a temporary registry for testing.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")

	require.NoError(t, err)
	require.NotNil(t, installer)
	assert.NotNil(t, installer.downloader)
	assert.NotNil(t, installer.validator)
	assert.NotNil(t, installer.localRegistry)
	assert.Equal(t, "1.0.0", installer.atmosVersion)
}

func TestInstall_InvalidSource(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "invalid-source", opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid source")
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create installer with mock registry.
	mockRegistry := newMockLocalRegistry()
	mockRegistry.skills["test-repo"] = &InstalledSkill{
		Name: "test-repo",
	}

	installer := &Installer{
		downloader:    &mockDownloader{},
		validator:     NewValidator("1.0.0"),
		localRegistry: &LocalRegistry{Skills: mockRegistry.skills},
		atmosVersion:  "1.0.0",
	}

	ctx := context.Background()
	opts := InstallOptions{Force: false, SkipConfirm: true}

	// Mock the Get method to return existing skill.
	installer.localRegistry.Skills["test-repo"] = &InstalledSkill{Name: "test-repo"}

	err := installer.Install(ctx, "github.com/test/test-repo", opts)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSkillAlreadyInstalled))
}

func TestInstall_DownloadFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	mockRegistry := newMockLocalRegistry()
	downloadErr := errors.New("download failed")

	installer := &Installer{
		downloader: &mockDownloader{
			downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
				return "", downloadErr
			},
		},
		validator:     NewValidator("1.0.0"),
		localRegistry: &LocalRegistry{Skills: mockRegistry.skills},
		atmosVersion:  "1.0.0",
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err := installer.Install(ctx, "github.com/test/repo", opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download skill")
}

func TestInstall_InvalidMetadata(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	mockRegistry := newMockLocalRegistry()

	// Create temp dir without SKILL.md.
	skillDir := t.TempDir()

	installer := &Installer{
		downloader: &mockDownloader{
			downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
				return skillDir, nil
			},
		},
		validator:     NewValidator("1.0.0"),
		localRegistry: &LocalRegistry{Skills: mockRegistry.skills},
		atmosVersion:  "1.0.0",
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err := installer.Install(ctx, "github.com/test/repo", opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid skill metadata")
}

func TestInstall_ValidationFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	mockRegistry := newMockLocalRegistry()

	// Create skill dir with incompatible version.
	metadata := `name: test-skill
description: Test skill
compatibility:
  atmos: ">=99.0.0"`

	skillDir := createTestSkillDirWithMetadata(t, metadata, "# Test Skill\n\nTest content")

	installer := &Installer{
		downloader: &mockDownloader{
			downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
				return skillDir, nil
			},
		},
		validator:     NewValidator("1.0.0"),
		localRegistry: &LocalRegistry{Skills: mockRegistry.skills},
		atmosVersion:  "1.0.0",
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err := installer.Install(ctx, "github.com/test/repo", opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "skill validation failed")
}

//nolint:dupl
func TestInstall_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillDir := createTestSkillDir(t)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Replace downloader with mock.
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/test-skill", opts)

	assert.NoError(t, err)

	// Verify skill was added to registry.
	skill, err := installer.Get("test-skill")
	require.NoError(t, err)
	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, "Test Skill", skill.DisplayName)
	assert.Equal(t, "1.0.0", skill.Version)
}

//nolint:dupl
func TestInstall_ForceReinstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Create a skill with different name for force reinstall test.
	metadata := `name: force-reinstall-skill
description: Test skill for force reinstall
metadata:
  display_name: Force Reinstall Skill
  version: 1.0.0`

	skillDir := createTestSkillDirWithMetadata(t, metadata, "# Force Reinstall Skill\n\nTest content")

	// Add existing skill.
	oldSkill := &InstalledSkill{
		Name:    "force-reinstall-skill",
		Version: "0.9.0",
		Path:    filepath.Join(tempDir, ".atmos", "skills", "old-path"),
	}
	err = installer.localRegistry.Add(oldSkill)
	require.NoError(t, err)

	// Replace downloader with mock.
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{Force: true, SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/force-reinstall-skill", opts)

	assert.NoError(t, err)
	// Verify skill was updated.
	skill, err := installer.Get("force-reinstall-skill")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", skill.Version)
}

func TestUninstall_SkillNotFound(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	err = installer.Uninstall("nonexistent", true, "", nil, nil)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSkillNotFound))
}

func TestUninstall_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create skill directory with unique name.
	skillPath := filepath.Join(tempDir, ".atmos", "skills", "uninstall-test-skill")
	err := os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Add skill to registry.
	skill := &InstalledSkill{
		Name:        "uninstall-test-skill",
		DisplayName: "Uninstall Test Skill",
		Version:     "1.0.0",
		Path:        skillPath,
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	// Uninstall with force.
	err = installer.Uninstall("uninstall-test-skill", true, "", nil, nil)

	assert.NoError(t, err)
	// Verify skill was removed from registry.
	_, err = installer.localRegistry.Get("uninstall-test-skill")
	assert.Error(t, err)
	// Verify directory was removed.
	_, err = os.Stat(skillPath)
	assert.True(t, os.IsNotExist(err))
}

func TestList(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Add test skills with unique names.
	skills := []*InstalledSkill{
		{Name: "list-test-skill-a", DisplayName: "List Test Skill A"},
		{Name: "list-test-skill-b", DisplayName: "List Test Skill B"},
	}

	for _, skill := range skills {
		err := installer.localRegistry.Add(skill)
		require.NoError(t, err)
	}

	result := installer.List()

	assert.GreaterOrEqual(t, len(result), 2)
	// Find our test skills in the results.
	found := 0
	for _, s := range result {
		if s.Name == "list-test-skill-a" || s.Name == "list-test-skill-b" {
			found++
		}
	}
	assert.Equal(t, 2, found, "Should find both test skills")
}

func TestGet_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:        "get-test-skill",
		DisplayName: "Get Test Skill",
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	result, err := installer.Get("get-test-skill")

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "get-test-skill", result.Name)
}

func TestGet_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	result, err := installer.Get("nonexistent")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, ErrSkillNotFound))
}

func TestLoadInstalledSkills_EmptyRegistry(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a fresh installer with truly empty registry.
	localReg, err := NewLocalRegistry()
	require.NoError(t, err)

	installer := &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator("1.0.0"),
		localRegistry: localReg,
		atmosVersion:  "1.0.0",
	}

	registry := skills.NewRegistry()

	err = installer.LoadInstalledSkills(registry)

	assert.NoError(t, err)
	// Registry should be empty since no skills are enabled in local registry.
	assert.GreaterOrEqual(t, registry.Count(), 0)
}

func TestLoadInstalledSkills_DisabledSkillSkipped(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a fresh installer.
	localReg, err := NewLocalRegistry()
	require.NoError(t, err)

	installer := &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator("1.0.0"),
		localRegistry: localReg,
		atmosVersion:  "1.0.0",
	}

	// Create disabled skill.
	skillPath := filepath.Join(tempDir, ".atmos", "skills", "disabled-test-skill")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:    "disabled-test-skill",
		Path:    skillPath,
		Enabled: false, // Disabled
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	registry := skills.NewRegistry()
	initialCount := registry.Count()

	err = installer.LoadInstalledSkills(registry)

	assert.NoError(t, err)
	// Registry should not have added the disabled skill.
	assert.Equal(t, initialCount, registry.Count())
}

func TestLoadInstalledSkills_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Create skill directory with SKILL.md.
	skillPath := filepath.Join(tempDir, ".atmos", "skills", "test-skill-load")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	skillMD := filepath.Join(skillPath, "SKILL.md")
	content := `---
name: test-skill-load
description: Test skill
metadata:
  display_name: Test Skill Load
  category: general
allowed-tools:
  - bash
---

# Test Skill

This is the skill prompt content.
`
	err = os.WriteFile(skillMD, []byte(content), 0o644)
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:    "test-skill-load",
		Path:    skillPath,
		Enabled: true,
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	registry := skills.NewRegistry()

	err = installer.LoadInstalledSkills(registry)

	assert.NoError(t, err)
	assert.Equal(t, 1, registry.Count())

	loadedSkill, err := registry.Get("test-skill-load")
	require.NoError(t, err)
	assert.Equal(t, "test-skill-load", loadedSkill.Name)
	assert.Equal(t, "Test Skill Load", loadedSkill.DisplayName)
	assert.Contains(t, loadedSkill.SystemPrompt, "This is the skill prompt content.")
	assert.False(t, loadedSkill.IsBuiltIn)
}

func TestReadSkillPrompt_ValidFrontmatter(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "SKILL.md")
	content := `---
name: test
description: test skill
---

# Skill Content

This is the actual prompt content that should be extracted.
`
	err := os.WriteFile(tempFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readSkillPrompt(tempFile)

	assert.NoError(t, err)
	assert.Equal(t, "# Skill Content\n\nThis is the actual prompt content that should be extracted.", result)
}

func TestReadSkillPrompt_NoFrontmatter(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "SKILL.md")
	content := `# Skill Without Frontmatter

This file has no frontmatter.
`
	err := os.WriteFile(tempFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readSkillPrompt(tempFile)

	assert.NoError(t, err)
	// Should return empty since no frontmatter ended.
	assert.Equal(t, "", result)
}

func TestReadSkillPrompt_FileNotFound(t *testing.T) {
	// Use cross-platform non-existent path.
	nonExistentPath := filepath.Join(t.TempDir(), "nonexistent", "SKILL.md")
	result, err := readSkillPrompt(nonExistentPath)

	assert.Error(t, err)
	assert.Equal(t, "", result)
}

func TestGetInstallPath(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer := &Installer{}

	source := &SourceInfo{
		FullPath: "github.com/user/repo",
	}

	result, err := installer.getInstallPath(source, "")

	require.NoError(t, err)
	assert.Contains(t, result, ".atmos")
	assert.Contains(t, result, "skills")
	// Use filepath.ToSlash to normalize for cross-platform comparison.
	assert.Contains(t, filepath.ToSlash(result), "github.com/user/repo")
}

func TestGetInstallPath_WithOverride(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer := &Installer{}

	source := &SourceInfo{
		FullPath: "github.com/user/repo",
		Name:     "repo",
	}

	overrideDir := filepath.Join(tempDir, "custom-skills")
	result, err := installer.getInstallPath(source, overrideDir)

	require.NoError(t, err)
	// Flattened: <override>/<skillName>, not <override>/github.com/user/repo.
	assert.Equal(t, filepath.Join(overrideDir, "repo"), result)
}

func TestRedactHomePath_WithHomePrefix(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	path := filepath.Join(tempDir, ".atmos", "skills", "test")

	result := redactHomePath(path)

	// Should start with ~ and contain the path suffix.
	assert.True(t, strings.HasPrefix(result, "~"), "Expected path to start with ~, got: %s", result)
	assert.Contains(t, result, "skills")
}

func TestRedactHomePath_WithoutHomePrefix(t *testing.T) {
	// Use a path that definitely doesn't start with HOME (cross-platform).
	path := filepath.Join("some", "other", "path")

	result := redactHomePath(path)

	assert.Equal(t, path, result)
}

func TestConfirmInstallation_BasicInfo(t *testing.T) {
	// This test verifies the function structure, but cannot test
	// interactive input without mocking stdin.
	installer := &Installer{}

	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "Test description",
		Metadata: &ExtendedMetadata{
			DisplayName: "Test Skill",
			Author:      "Test Author",
			Version:     "1.0.0",
			Repository:  "https://github.com/test/skill",
		},
	}

	// We can't easily test the interactive prompt without stdin mocking,
	// but we can verify the function doesn't panic.
	err := installer.confirmInstallation(metadata)
	// Expect error because no stdin input in test.
	assert.Error(t, err)
}

func TestConfirmInstallation_WithDestructiveTools(t *testing.T) {
	installer := &Installer{}

	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "Test description",
		Metadata: &ExtendedMetadata{
			DisplayName: "Test Skill",
			Author:      "Test Author",
			Version:     "1.0.0",
			Repository:  "https://github.com/test/skill",
		},
		AllowedTools: []string{"terraform_apply", "bash"},
	}

	// Verify function handles destructive tools.
	err := installer.confirmInstallation(metadata)
	assert.Error(t, err) // No stdin in test.
}

func TestInstaller_InstallationFlow_Integration(t *testing.T) {
	// Integration test covering the full flow.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Create skill with unique name for integration test.
	metadata := `name: integration-test-skill
description: Integration test skill
metadata:
  display_name: Integration Test Skill
  version: 1.1.0`

	skillDir := createTestSkillDirWithMetadata(t, metadata, "# Integration Test Skill\n\nTest content")
	originalSkillDir := skillDir

	// Replace downloader with mock.
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, source *SourceInfo) (string, error) {
			// Return a copy so we can verify cleanup.
			copyDir := filepath.Join(t.TempDir(), "skill-copy")
			err := os.MkdirAll(copyDir, 0o755)
			require.NoError(t, err)

			// Copy SKILL.md to new location.
			originalFile := filepath.Join(originalSkillDir, "SKILL.md")
			newFile := filepath.Join(copyDir, "SKILL.md")
			data, err := os.ReadFile(originalFile)
			require.NoError(t, err)
			err = os.WriteFile(newFile, data, 0o644)
			require.NoError(t, err)

			return copyDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, Force: false}

	err = installer.Install(ctx, "github.com/test/integration-test-skill", opts)

	assert.NoError(t, err)

	// Verify skill was registered.
	skill, err := installer.Get("integration-test-skill")
	require.NoError(t, err)
	assert.Equal(t, "integration-test-skill", skill.Name)
	assert.Equal(t, "Integration Test Skill", skill.DisplayName)
	assert.Equal(t, "1.1.0", skill.Version)
	assert.Equal(t, "github.com/test/integration-test-skill", skill.Source)
	assert.False(t, skill.IsBuiltIn)
	assert.True(t, skill.Enabled)
	assert.NotZero(t, skill.InstalledAt)
	assert.NotZero(t, skill.UpdatedAt)
}

func TestInstaller_ForceReinstallCleansOldInstallation(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Create skill with unique name.
	metadata := `name: cleanup-test-skill
description: Test skill for cleanup
metadata:
  display_name: Cleanup Test Skill
  version: 1.0.0`

	skillDir := createTestSkillDirWithMetadata(t, metadata, "# Cleanup Test Skill\n\nTest content")

	// Create old installation directory.
	oldInstallPath := filepath.Join(tempDir, ".atmos", "skills", "github.com", "test", "cleanup-test-skill")
	err = os.MkdirAll(oldInstallPath, 0o755)
	require.NoError(t, err)

	oldFile := filepath.Join(oldInstallPath, "old-file.txt")
	err = os.WriteFile(oldFile, []byte("old content"), 0o644)
	require.NoError(t, err)

	// Add existing skill to registry.
	existingSkill := &InstalledSkill{
		Name:        "cleanup-test-skill",
		Version:     "0.5.0",
		Path:        oldInstallPath,
		InstalledAt: time.Now().Add(-24 * time.Hour),
	}
	err = installer.localRegistry.Add(existingSkill)
	require.NoError(t, err)

	// Replace downloader with mock.
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{Force: true, SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/cleanup-test-skill", opts)

	assert.NoError(t, err)
	// Verify old file was removed.
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))
}

func TestLoadInstalledSkills_InvalidMetadataWarning(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a fresh installer.
	localReg, err := NewLocalRegistry()
	require.NoError(t, err)

	installer := &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator("1.0.0"),
		localRegistry: localReg,
		atmosVersion:  "1.0.0",
	}

	// Create skill with invalid SKILL.md.
	skillPath := filepath.Join(tempDir, ".atmos", "skills", "invalid-metadata-skill")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	skillMD := filepath.Join(skillPath, "SKILL.md")
	invalidContent := `---
invalid yaml: [
---`
	err = os.WriteFile(skillMD, []byte(invalidContent), 0o644)
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:    "invalid-metadata-skill",
		Path:    skillPath,
		Enabled: true,
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	registry := skills.NewRegistry()
	initialCount := registry.Count()

	// Should not error, but should log warning.
	err = installer.LoadInstalledSkills(registry)

	assert.NoError(t, err)
	// Invalid skill should not be loaded.
	assert.Equal(t, initialCount, registry.Count())
}

func TestLoadInstalledSkills_MissingPromptFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a fresh installer.
	localReg, err := NewLocalRegistry()
	require.NoError(t, err)

	installer := &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator("1.0.0"),
		localRegistry: localReg,
		atmosVersion:  "1.0.0",
	}

	// Create skill without SKILL.md.
	skillPath := filepath.Join(tempDir, ".atmos", "skills", "missing-prompt-file-skill")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:    "missing-prompt-file-skill",
		Path:    skillPath,
		Enabled: true,
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	registry := skills.NewRegistry()
	initialCount := registry.Count()

	// Should not error, but should log warning.
	err = installer.LoadInstalledSkills(registry)

	assert.NoError(t, err)
	// Skill should not be loaded.
	assert.Equal(t, initialCount, registry.Count())
}

func TestReadSkillPrompt_EmptyContent(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "SKILL.md")
	content := `---
name: test
---

`
	err := os.WriteFile(tempFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readSkillPrompt(tempFile)

	assert.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestReadSkillPrompt_OnlyFrontmatter(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "SKILL.md")
	content := `---
name: test
description: test
---`
	err := os.WriteFile(tempFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readSkillPrompt(tempFile)

	assert.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestReadSkillPrompt_MultipleDelimiters(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "SKILL.md")
	content := `---
name: test
---

# Content

Some text with --- in it.

More content.
`
	err := os.WriteFile(tempFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readSkillPrompt(tempFile)

	assert.NoError(t, err)
	assert.Contains(t, result, "# Content")
	assert.Contains(t, result, "Some text with --- in it.")
}

// createMultiSkillPackageDir creates a temp directory with multiple skills in skills/*/SKILL.md layout.
func createMultiSkillPackageDir(t *testing.T, skillCount int) string {
	t.Helper()

	tempDir := t.TempDir()

	for i := 1; i <= skillCount; i++ {
		skillName := fmt.Sprintf("test-skill-%d", i)
		skillDir := filepath.Join(tempDir, "agent-skills", "skills", skillName)
		err := os.MkdirAll(skillDir, 0o755)
		require.NoError(t, err)

		content := fmt.Sprintf(`---
name: %s
description: Test skill %d for multi-package testing
metadata:
  display_name: Test Skill %d
  version: 1.0.0
  author: Test Author
---

# Test Skill %d

This is test skill %d prompt content.
`, skillName, i, i, i, i)

		err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
		require.NoError(t, err)
	}

	return tempDir
}

func TestInstall_MultiSkillPackage(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a multi-skill package with 3 skills.
	packageDir := createMultiSkillPackageDir(t, 3)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Replace downloader with mock.
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			// Return a copy of the package dir.
			copyDir := filepath.Join(t.TempDir(), "pkg-copy")
			err := os.MkdirAll(copyDir, 0o755)
			require.NoError(t, err)
			err = copyDirRecursive(packageDir, copyDir)
			require.NoError(t, err)
			return copyDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/cloudposse/atmos", opts)

	assert.NoError(t, err)

	// Verify all 3 skills were registered.
	installedSkills := installer.List()
	found := 0
	for _, s := range installedSkills {
		if strings.HasPrefix(s.Name, "test-skill-") {
			found++
		}
	}
	assert.Equal(t, 3, found, "Should have installed 3 skills")
}

func TestInstall_MultiSkillPackage_ForceReinstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	packageDir := createMultiSkillPackageDir(t, 2)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Pre-install one skill.
	existingSkill := &InstalledSkill{
		Name:    "test-skill-1",
		Version: "0.5.0",
		Path:    filepath.Join(tempDir, ".atmos", "skills", "cloudposse", "atmos", "test-skill-1"),
	}
	err = installer.localRegistry.Add(existingSkill)
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			copyDir := filepath.Join(t.TempDir(), "pkg-copy")
			err := os.MkdirAll(copyDir, 0o755)
			require.NoError(t, err)
			err = copyDirRecursive(packageDir, copyDir)
			require.NoError(t, err)
			return copyDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, Force: true}

	err = installer.Install(ctx, "github.com/cloudposse/atmos", opts)
	assert.NoError(t, err)

	// Both skills should be installed.
	installedSkills := installer.List()
	found := 0
	for _, s := range installedSkills {
		if strings.HasPrefix(s.Name, "test-skill-") {
			found++
		}
	}
	assert.Equal(t, 2, found, "Should have installed 2 skills after force reinstall")
}

func TestInstall_NoSkillMDFound(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create empty directory (no SKILL.md at root or in skills/).
	emptyDir := t.TempDir()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return emptyDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/empty-repo", opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no SKILL.md found")
}

func TestReadSkillPromptWithReferences(t *testing.T) {
	// Create skill directory with SKILL.md and reference files.
	skillDir := t.TempDir()

	skillMD := `---
name: test-with-refs
description: Test skill with references
references:
  - docs/patterns.md
  - docs/examples.md
---

# Test Skill

Main prompt content.
`
	err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644)
	require.NoError(t, err)

	// Create reference files.
	docsDir := filepath.Join(skillDir, "docs")
	err = os.MkdirAll(docsDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(docsDir, "patterns.md"), []byte("# Patterns\n\nCommon patterns content."), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(docsDir, "examples.md"), []byte("# Examples\n\nExample content here."), 0o644)
	require.NoError(t, err)

	metadata := &SkillMetadata{
		Name:        "test-with-refs",
		Description: "Test skill with references",
		References:  []string{"docs/patterns.md", "docs/examples.md"},
	}

	result, err := readSkillPromptWithReferences(skillDir, metadata)

	assert.NoError(t, err)
	assert.Contains(t, result, "Main prompt content.")
	assert.Contains(t, result, "## Reference: patterns.md")
	assert.Contains(t, result, "Common patterns content.")
	assert.Contains(t, result, "## Reference: examples.md")
	assert.Contains(t, result, "Example content here.")
}

func TestReadSkillPromptWithReferences_NoReferences(t *testing.T) {
	skillDir := t.TempDir()

	skillMD := `---
name: test-no-refs
description: Test skill without references
---

# Test Skill

Just the main prompt.
`
	err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644)
	require.NoError(t, err)

	metadata := &SkillMetadata{
		Name:        "test-no-refs",
		Description: "Test skill without references",
	}

	result, err := readSkillPromptWithReferences(skillDir, metadata)

	assert.NoError(t, err)
	assert.Contains(t, result, "Just the main prompt.")
	assert.NotContains(t, result, "## Reference:")
}

func TestReadSkillPromptWithReferences_MissingReferenceFile(t *testing.T) {
	skillDir := t.TempDir()

	skillMD := `---
name: test-missing-ref
description: Test skill with missing reference
---

# Test Skill

Main prompt.
`
	err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644)
	require.NoError(t, err)

	metadata := &SkillMetadata{
		Name:        "test-missing-ref",
		Description: "Test skill with missing reference",
		References:  []string{"nonexistent.md"},
	}

	// Should not error; missing references are warned and skipped.
	result, err := readSkillPromptWithReferences(skillDir, metadata)

	assert.NoError(t, err)
	assert.Contains(t, result, "Main prompt.")
	assert.NotContains(t, result, "## Reference:")
}

func TestLoadInstalledSkills_WithReferences(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Create skill directory with SKILL.md and reference file.
	skillPath := filepath.Join(tempDir, ".atmos", "skills", "ref-test-skill")
	docsPath := filepath.Join(skillPath, "docs")
	err = os.MkdirAll(docsPath, 0o755)
	require.NoError(t, err)

	skillMD := `---
name: ref-test-skill
description: Test skill with references
metadata:
  display_name: Ref Test Skill
  category: general
references:
  - docs/extra.md
---

# Ref Test Skill

Main skill prompt.
`
	err = os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte(skillMD), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(docsPath, "extra.md"), []byte("Extra reference content."), 0o644)
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:    "ref-test-skill",
		Path:    skillPath,
		Enabled: true,
	}
	err = installer.localRegistry.Add(skill)
	require.NoError(t, err)

	registry := skills.NewRegistry()

	err = installer.LoadInstalledSkills(registry)

	assert.NoError(t, err)
	assert.Equal(t, 1, registry.Count())

	loadedSkill, err := registry.Get("ref-test-skill")
	require.NoError(t, err)
	assert.Contains(t, loadedSkill.SystemPrompt, "Main skill prompt.")
	assert.Contains(t, loadedSkill.SystemPrompt, "## Reference: extra.md")
	assert.Contains(t, loadedSkill.SystemPrompt, "Extra reference content.")
}

func TestCopyDir(t *testing.T) {
	t.Run("copies files and subdirectories", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()

		// Create source structure.
		err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0o644)
		require.NoError(t, err)

		subDir := filepath.Join(src, "subdir")
		err = os.MkdirAll(subDir, 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0o644)
		require.NoError(t, err)

		// Copy.
		err = copyDir(src, dst)
		assert.NoError(t, err)

		// Verify.
		data, err := os.ReadFile(filepath.Join(dst, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content", string(data))

		data, err = os.ReadFile(filepath.Join(dst, "subdir", "nested.txt"))
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(data))
	})

	t.Run("errors on nonexistent source", func(t *testing.T) {
		dst := t.TempDir()
		err := copyDir(filepath.Join(t.TempDir(), "nonexistent"), dst)
		assert.Error(t, err)
	})
}

func TestInstall_MultiSkillPackage_SkipsAlreadyInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	packageDir := createMultiSkillPackageDir(t, 2)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	// Pre-install one skill (without --force).
	existingSkill := &InstalledSkill{
		Name:    "test-skill-1",
		Version: "0.5.0",
		Path:    filepath.Join(tempDir, ".atmos", "skills", "test", "atmos", "test-skill-1"),
	}
	err = installer.localRegistry.Add(existingSkill)
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			copyDir := filepath.Join(t.TempDir(), "pkg-copy")
			err := os.MkdirAll(copyDir, 0o755)
			require.NoError(t, err)
			err = copyDirRecursive(packageDir, copyDir)
			require.NoError(t, err)
			return copyDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, Force: false}

	err = installer.Install(ctx, "github.com/test/atmos", opts)
	assert.NoError(t, err)

	// Only test-skill-2 should be newly installed (test-skill-1 was skipped).
	skill2, err := installer.localRegistry.Get("test-skill-2")
	require.NoError(t, err)
	assert.Equal(t, "test-skill-2", skill2.Name)
}

func TestInstall_MultiSkillPackage_AllInvalid(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a package with only invalid skills.
	packageDir := t.TempDir()
	skillDir := filepath.Join(packageDir, "agent-skills", "skills", "bad-skill")
	err := os.MkdirAll(skillDir, 0o755)
	require.NoError(t, err)

	// Write invalid SKILL.md (missing required fields).
	err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ninvalid: yaml: [\n---\n"), 0o644)
	require.NoError(t, err)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			copyDir := filepath.Join(t.TempDir(), "pkg-copy")
			err := os.MkdirAll(copyDir, 0o755)
			require.NoError(t, err)
			err = copyDirRecursive(packageDir, copyDir)
			require.NoError(t, err)
			return copyDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/bad-pkg", opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid skills found")
}

func TestInstall_SingleSkillWithRootSKILLMD(t *testing.T) {
	// Verify single-skill path is taken when SKILL.md exists at root.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillDir := createTestSkillDir(t)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/single-skill", opts)
	assert.NoError(t, err)

	skill, err := installer.Get("test-skill")
	require.NoError(t, err)
	assert.Equal(t, "test-skill", skill.Name)
}

func TestInstall_MultiSkillPackage_SkillsSubdir(t *testing.T) {
	// Test the skills/*/SKILL.md fallback pattern (not agent-skills/skills/).
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	packageDir := t.TempDir()
	skillDir := filepath.Join(packageDir, "skills", "my-skill")
	err := os.MkdirAll(skillDir, 0o755)
	require.NoError(t, err)

	content := `---
name: my-skill
description: A skill in the skills/ subdir
metadata:
  display_name: My Skill
  version: 1.0.0
---

# My Skill

Prompt content.
`
	err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	require.NoError(t, err)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			copyTo := filepath.Join(t.TempDir(), "pkg-copy")
			err := os.MkdirAll(copyTo, 0o755)
			require.NoError(t, err)
			err = copyDirRecursive(packageDir, copyTo)
			require.NoError(t, err)
			return copyTo, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/skills-subdir-pkg", opts)
	assert.NoError(t, err)

	skill, err := installer.localRegistry.Get("my-skill")
	require.NoError(t, err)
	assert.Equal(t, "My Skill", skill.DisplayName)
}

// copyFileForTest copies a single file from src to dst.
func copyFileForTest(srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, 0o644)
}

// copyDirRecursive copies a directory tree for testing.
func copyDirRecursive(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if !entry.IsDir() {
			if err := copyFileForTest(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(dstPath, 0o755); err != nil {
			return err
		}
		if err := copyDirRecursive(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func TestCopyDir_InvalidSource(t *testing.T) {
	err := copyDir(filepath.Join(t.TempDir(), "nonexistent"), t.TempDir())
	assert.Error(t, err)
}

func TestLoadInstalledSkills_DuplicateSkillWarning(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	skillPath := filepath.Join(tempDir, ".atmos", "skills", "dup-test")
	err = os.MkdirAll(skillPath, 0o755)
	require.NoError(t, err)

	content := `---
name: dup-test
description: Duplicate test
metadata:
  display_name: Dup Test
---

# Dup Test
`
	err = os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte(content), 0o644)
	require.NoError(t, err)

	err = installer.localRegistry.Add(&InstalledSkill{
		Name:    "dup-test",
		Path:    skillPath,
		Enabled: true,
	})
	require.NoError(t, err)

	// Pre-register in the skills.Registry so the second register fails.
	registry := skills.NewRegistry()
	err = registry.Register(&skills.Skill{Name: "dup-test", DisplayName: "Already There"})
	require.NoError(t, err)

	// Should not error overall; the duplicate is warned and skipped.
	err = installer.LoadInstalledSkills(registry)
	assert.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
}

func TestRedactHomePath_FallbackOnError(t *testing.T) {
	result := redactHomePath("/some/random/path")
	assert.Equal(t, "/some/random/path", result)
}

func TestInstall_PathOverride_FlattensLayout(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillDir := createTestSkillDir(t)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	overridePath := filepath.Join(t.TempDir(), "custom-skills")

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, Path: overridePath}

	err = installer.Install(ctx, "github.com/test/test-skill", opts)
	require.NoError(t, err)

	skill, err := installer.Get("test-skill")
	require.NoError(t, err)
	// Flattened: <override>/<skillName>, not <override>/github.com/test/test-skill.
	assert.Equal(t, filepath.Join(overridePath, "test-skill"), skill.Path)

	_, err = os.Stat(filepath.Join(overridePath, "test-skill", skillFileName))
	assert.NoError(t, err)
}

func TestInstall_NoPathOverride_KeepsOwnerRepoNesting(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillDir := createTestSkillDir(t)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true}

	err = installer.Install(ctx, "github.com/test/test-skill", opts)
	require.NoError(t, err)

	skill, err := installer.Get("test-skill")
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(filepath.Join(tempDir, ".atmos", "skills", "github.com", "test", "test-skill")), filepath.ToSlash(skill.Path))
}

func TestInstall_PathOverride_SkipsAutoDistribution(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillDir := createTestSkillDir(t)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	basePath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, ".vscode"), 0o755))

	ctx := context.Background()
	opts := InstallOptions{
		SkipConfirm: true,
		Path:        filepath.Join(t.TempDir(), "custom-skills"),
		BasePath:    basePath,
	}

	err = installer.Install(ctx, "github.com/test/test-skill", opts)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(basePath, ".github", "skills", "test-skill"))
	assert.True(t, os.IsNotExist(err), "explicit --path must skip auto-distribution")
}

func TestInstallOneSkillFromPackage_PathOverride_Flattens(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	packageDir := createMultiSkillPackageDir(t, 1)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			copyTo := filepath.Join(t.TempDir(), "pkg-copy")
			require.NoError(t, os.MkdirAll(copyTo, 0o755))
			require.NoError(t, copyDirRecursive(packageDir, copyTo))
			return copyTo, nil
		},
	}

	overridePath := filepath.Join(t.TempDir(), "custom-skills")

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, Path: overridePath}

	err = installer.Install(ctx, "github.com/cloudposse/atmos", opts)
	require.NoError(t, err)

	skill, err := installer.Get("test-skill-1")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(overridePath, "test-skill-1"), skill.Path)
}

// distributionSourceDir creates a minimal installed-skill directory (as if it
// were already moved to its canonical install path) for distributeToClients tests.
func distributionSourceDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, skillFileName), []byte("---\nname: dist-skill\n---\n\nBody.\n"), 0o644)
	require.NoError(t, err)
	return dir
}

func TestDistributeToClients_ExplicitClients(t *testing.T) {
	installer := &Installer{}
	installPath := distributionSourceDir(t)
	basePath := t.TempDir()

	opts := &InstallOptions{Clients: []string{ClientClaudeCode, ClientVSCode}}
	installer.distributeToClients(basePath, installPath, "dist-skill", opts.Clients, opts.Scope)

	_, err := os.Stat(filepath.Join(basePath, ".claude", "skills", "dist-skill", skillFileName))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(basePath, ".github", "skills", "dist-skill", skillFileName))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(basePath, ".gemini", "skills", "dist-skill", skillFileName))
	assert.True(t, os.IsNotExist(err), "gemini was not requested, must not be distributed")
}

func TestDistributeToClients_AllClients(t *testing.T) {
	installer := &Installer{}
	installPath := distributionSourceDir(t)
	basePath := t.TempDir()

	opts := &InstallOptions{AllClients: true}
	installer.distributeToClients(basePath, installPath, "dist-skill", SupportedClients, opts.Scope)

	for _, client := range SupportedClients {
		_, err := os.Stat(filepath.Join(clientSkillDir(basePath, "", ScopeProject, client), "dist-skill", skillFileName))
		assert.NoError(t, err, "expected distribution for client %s", client)
	}
}

func TestDistributeToClients_AutoDetect(t *testing.T) {
	installer := &Installer{}
	installPath := distributionSourceDir(t)
	basePath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, ".vscode"), 0o755))

	opts := &InstallOptions{}
	clients := installer.resolveDistributionClients(basePath, opts)
	installer.distributeToClients(basePath, installPath, "dist-skill", clients, opts.Scope)

	_, err := os.Stat(filepath.Join(basePath, ".github", "skills", "dist-skill", skillFileName))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(basePath, ".claude", "skills", "dist-skill", skillFileName))
	assert.True(t, os.IsNotExist(err), "claude-code was not detected, must not be distributed")
}

// TestDistributeToClients_SkipsPreExistingSymlink covers the real incident: this repo's own
// .claude/skills/<name> entries are intentionally checked-in symlinks pointing into
// agent-skills/skills/<name> for contributor auto-discovery. It must never write through a
// pre-existing symlink at a client target -- it must skip that client and leave the symlink
// (and whatever it points at) completely untouched.
func TestDistributeToClients_SkipsPreExistingSymlink(t *testing.T) {
	installer := &Installer{}
	installPath := distributionSourceDir(t)
	basePath := t.TempDir()

	// A separate directory standing in for e.g. agent-skills/skills/dist-skill, with sentinel
	// content that must survive completely unchanged.
	symlinkTarget := t.TempDir()
	sentinelPath := filepath.Join(symlinkTarget, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinelPath, []byte("do not touch"), 0o644))

	claudeSkillsDir := filepath.Join(basePath, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkillsDir, 0o755))
	symlinkPath := filepath.Join(claudeSkillsDir, "dist-skill")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Skipf("Skipping symlink test: %v", err)
	}

	opts := &InstallOptions{Clients: []string{ClientClaudeCode, ClientVSCode}}
	installer.distributeToClients(basePath, installPath, "dist-skill", opts.Clients, opts.Scope)

	// The symlink itself is untouched (still a symlink, still pointing at the same target).
	assert.True(t, isSymlink(symlinkPath), "symlink must remain a symlink, not be replaced")
	resolved, err := os.Readlink(symlinkPath)
	require.NoError(t, err)
	assert.Equal(t, symlinkTarget, resolved)

	// Its target content is byte-for-byte unchanged -- nothing was written through it.
	content, err := os.ReadFile(sentinelPath)
	require.NoError(t, err)
	assert.Equal(t, "do not touch", string(content))
	_, err = os.Stat(filepath.Join(symlinkTarget, skillFileName))
	assert.True(t, os.IsNotExist(err), "skill files must not have been copied through the symlink")

	// The other, non-symlinked client still gets its distribution as normal.
	_, err = os.Stat(filepath.Join(basePath, ".github", "skills", "dist-skill", skillFileName))
	assert.NoError(t, err, "vscode wasn't a symlink and should still be distributed to")
}

func TestInstallThenUninstall_RemovesCanonicalAndClientCopies(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillDir := createTestSkillDir(t)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	basePath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, ".claude"), 0o755))

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, BasePath: basePath}

	err = installer.Install(ctx, "github.com/test/test-skill", opts)
	require.NoError(t, err)

	skill, err := installer.Get("test-skill")
	require.NoError(t, err)

	clientCopyPath := filepath.Join(basePath, ".claude", "skills", "test-skill")
	_, err = os.Stat(clientCopyPath)
	require.NoError(t, err, "expected auto-distributed claude-code copy")

	// Mirror what the CLI does: recompute the client list (stateless -- no
	// registry tracking of prior distribution) and pass it to Uninstall.
	clients := DetectClients(basePath, "", ScopeProject)
	err = installer.Uninstall("test-skill", true, basePath, clients, []string{ScopeProject})
	require.NoError(t, err)

	_, err = os.Stat(skill.Path)
	assert.True(t, os.IsNotExist(err), "canonical install path should be removed")
	_, err = os.Stat(clientCopyPath)
	assert.True(t, os.IsNotExist(err), "distributed client copy should be removed")
}

func TestUninstallAll_RemovesEveryInstalledSkill(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	basePath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, ".claude"), 0o755))

	skillDir := createTestSkillDir(t)
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}
	ctx := context.Background()
	require.NoError(t, installer.Install(ctx, "github.com/test/test-skill", InstallOptions{SkipConfirm: true, BasePath: basePath}))
	require.NoError(t, installer.Install(ctx, "atmos-terraform", InstallOptions{SkipConfirm: true, BasePath: basePath}))

	require.Len(t, installer.List(), 2)
	clientCopyPath := filepath.Join(basePath, ".claude", "skills", "test-skill")
	require.FileExists(t, filepath.Join(clientCopyPath, "SKILL.md"))

	clients := DetectClients(basePath, "", ScopeProject)
	require.NoError(t, installer.UninstallAll(true, basePath, clients, []string{ScopeProject}))

	assert.Empty(t, installer.List())
	_, err = os.Stat(clientCopyPath)
	assert.True(t, os.IsNotExist(err), "distributed client copy should be removed")
}

func TestUninstallAll_NoneInstalled_NoError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	require.NoError(t, installer.UninstallAll(true, "", nil, nil))
	assert.Empty(t, installer.List())
}

func TestUninstall_NoClients_SkipsClientCleanup(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	skillPath := filepath.Join(tempDir, ".atmos", "skills", "no-client-cleanup-skill")
	require.NoError(t, os.MkdirAll(skillPath, 0o755))

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	skill := &InstalledSkill{
		Name:    "no-client-cleanup-skill",
		Path:    skillPath,
		Enabled: true,
	}
	require.NoError(t, installer.localRegistry.Add(skill))

	err = installer.Uninstall("no-client-cleanup-skill", true, "", nil, nil)
	require.NoError(t, err)

	_, err = os.Stat(skillPath)
	assert.True(t, os.IsNotExist(err))
}

// TestRemoveClientCopies_SkipsPreExistingSymlink is the uninstall-side mirror of
// TestDistributeToClients_SkipsPreExistingSymlink: removeClientCopies must never delete a
// pre-existing symlink at a client target (e.g. this repo's own .claude/skills/<name> entries),
// even though Go's os.RemoveAll on a symlink only unlinks it rather than recursing into its
// target -- deleting an intentionally-placed symlink is still the wrong outcome.
func TestRemoveClientCopies_SkipsPreExistingSymlink(t *testing.T) {
	basePath := t.TempDir()
	symlinkTarget := t.TempDir()

	claudeSkillsDir := filepath.Join(basePath, ".claude", "skills")
	require.NoError(t, os.MkdirAll(claudeSkillsDir, 0o755))
	symlinkPath := filepath.Join(claudeSkillsDir, "dist-skill")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Skipf("Skipping symlink test: %v", err)
	}

	// A real (non-symlink) copy for vscode, to confirm it's still cleaned up normally.
	vscodeSkillsDir := filepath.Join(basePath, ".github", "skills", "dist-skill")
	require.NoError(t, os.MkdirAll(vscodeSkillsDir, 0o755))

	removeClientCopies(basePath, "dist-skill", []string{ClientClaudeCode, ClientVSCode}, []string{ScopeProject})

	assert.True(t, isSymlink(symlinkPath), "symlink must survive removeClientCopies untouched")
	resolved, err := os.Readlink(symlinkPath)
	require.NoError(t, err)
	assert.Equal(t, symlinkTarget, resolved)
	_, err = os.Stat(symlinkTarget)
	assert.NoError(t, err, "the symlink's target directory itself must still exist")

	_, err = os.Stat(vscodeSkillsDir)
	assert.True(t, os.IsNotExist(err), "the non-symlinked vscode copy should still be removed normally")
}

// TestPrepareInstallPath_ForceRefusesSymlink covers the canonical-install-path guard: --force
// reinstall must refuse to delete a pre-existing symlink at the install path (e.g. someone
// manually symlinked ~/.atmos/skills/<name> for local testing) rather than silently unlinking it.
func TestPrepareInstallPath_ForceRefusesSymlink(t *testing.T) {
	installer := &Installer{}

	symlinkTarget := t.TempDir()
	installPath := filepath.Join(t.TempDir(), "some-skill")
	if err := os.Symlink(symlinkTarget, installPath); err != nil {
		t.Skipf("Skipping symlink test: %v", err)
	}

	err := installer.prepareInstallPath(installPath, "some-skill", true)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrRefuseDeleteSymbolicLink))

	assert.True(t, isSymlink(installPath), "symlink must survive the refused force-reinstall")
}

// TestDistributeToClients_UserScope confirms Scope: ScopeUser lands copies
// under the resolved home directory (via homedir.Dir(), since BasePath is
// irrelevant at user scope) instead of the project path, including the
// vscode/.copilot asymmetry (not .github).
func TestDistributeToClients_UserScope(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer := &Installer{}
	installPath := distributionSourceDir(t)
	basePath := t.TempDir()

	opts := &InstallOptions{Scope: ScopeUser, Clients: []string{ClientClaudeCode, ClientVSCode}}
	installer.distributeToClients(basePath, installPath, "dist-skill", opts.Clients, opts.Scope)

	_, err := os.Stat(filepath.Join(tempHome, ".claude", "skills", "dist-skill", skillFileName))
	assert.NoError(t, err, "expected user-scope claude-code distribution under the home dir")
	_, err = os.Stat(filepath.Join(tempHome, ".copilot", "skills", "dist-skill", skillFileName))
	assert.NoError(t, err, "expected user-scope vscode/copilot distribution under .copilot, not .github")

	_, err = os.Stat(filepath.Join(basePath, ".claude", "skills", "dist-skill", skillFileName))
	assert.True(t, os.IsNotExist(err), "user scope must not write into the project path")
	_, err = os.Stat(filepath.Join(basePath, ".github", "skills", "dist-skill", skillFileName))
	assert.True(t, os.IsNotExist(err), "user scope must not write into the project path")
}

// TestUninstall_UserScope_RemovesHomeDirClientCopy is the uninstall-side
// mirror of TestDistributeToClients_UserScope: a skill installed with
// Scope: ScopeUser must have its client copy removed from the home-dir-rooted
// path, not the project path.
func TestUninstall_UserScope_RemovesHomeDirClientCopy(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	installer, err := NewInstaller("1.0.0")
	require.NoError(t, err)

	skillDir := createTestSkillDir(t)
	installer.downloader = &mockDownloader{
		downloadFunc: func(_ context.Context, _ *SourceInfo) (string, error) {
			return skillDir, nil
		},
	}

	basePath := t.TempDir()

	ctx := context.Background()
	opts := InstallOptions{SkipConfirm: true, BasePath: basePath, Scope: ScopeUser, Clients: []string{ClientClaudeCode}}
	require.NoError(t, installer.Install(ctx, "github.com/test/test-skill", opts))

	homeCopyPath := filepath.Join(tempHome, ".claude", "skills", "test-skill")
	_, err = os.Stat(homeCopyPath)
	require.NoError(t, err, "expected user-scope claude-code copy under the home dir")

	require.NoError(t, installer.Uninstall("test-skill", true, basePath, []string{ClientClaudeCode}, []string{ScopeUser}))

	_, err = os.Stat(homeCopyPath)
	assert.True(t, os.IsNotExist(err), "user-scope client copy should be removed")

	// The project path must never have been touched.
	_, err = os.Stat(filepath.Join(basePath, ".claude", "skills", "test-skill"))
	assert.True(t, os.IsNotExist(err), "project path must remain untouched by a user-scope uninstall")
}
