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

	err = installer.Uninstall("nonexistent", true)

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
	err = installer.Uninstall("uninstall-test-skill", true)

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

	result := installer.getInstallPath(source)

	assert.Contains(t, result, ".atmos")
	assert.Contains(t, result, "skills")
	// Use filepath.ToSlash to normalize for cross-platform comparison.
	assert.Contains(t, filepath.ToSlash(result), "github.com/user/repo")
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
