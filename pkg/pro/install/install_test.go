package install

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileWriter tracks filesystem operations for testing.
type mockFileWriter struct {
	files   map[string][]byte
	dirs    map[string]bool
	written []string
}

func newMockFileWriter() *mockFileWriter {
	return &mockFileWriter{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *mockFileWriter) WriteFile(path string, content []byte, _ os.FileMode) error {
	m.files[path] = content
	m.written = append(m.written, path)
	return nil
}

func (m *mockFileWriter) MkdirAll(path string, _ os.FileMode) error {
	m.dirs[path] = true
	return nil
}

func (m *mockFileWriter) FileExists(path string) bool {
	_, ok := m.files[path]
	return ok
}

func (m *mockFileWriter) ReadFile(path string) ([]byte, error) {
	content, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return content, nil
}

func TestInstaller_Install_FreshProject(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()
	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
	)

	result, err := installer.Install()
	require.NoError(t, err)

	// Should create all files.
	assert.Empty(t, result.SkippedFiles)
	assert.Empty(t, result.UpdatedFiles)

	// 1 atmos.yaml + 4 workflows + 2 profiles + 1 profiles README + 1 mixin + 2 .atmos.d configs = 11 files.
	assert.Len(t, result.CreatedFiles, 11)

	// Verify atmos.yaml.
	atmosPath := filepath.Join(base, "atmos.yaml")
	assert.True(t, writer.FileExists(atmosPath))
	assert.Contains(t, string(writer.files[atmosPath]), "atmos.d")

	// Verify workflow files exist.
	expectedWorkflows := []string{
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"),
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-apply.yaml"),
		filepath.Join(githubDir, workflowsDir, "atmos-pro-affected-stacks.yaml"),
		filepath.Join(githubDir, workflowsDir, "atmos-pro-list-instances.yaml"),
	}
	for _, wf := range expectedWorkflows {
		assert.Contains(t, result.CreatedFiles, wf)
		fullPath := filepath.Join(base, wf)
		assert.True(t, writer.FileExists(fullPath), "expected file to exist: %s", fullPath)
	}

	// Verify auth profiles.
	planProfilePath := filepath.Join(base, "profiles", "github-plan", "atmos.yaml")
	assert.True(t, writer.FileExists(planProfilePath))
	planContent := string(writer.files[planProfilePath])
	assert.Contains(t, planContent, "github-oidc")
	assert.Contains(t, planContent, "<region>")
	assert.Contains(t, planContent, "planner")

	applyProfilePath := filepath.Join(base, "profiles", "github-apply", "atmos.yaml")
	assert.True(t, writer.FileExists(applyProfilePath))
	applyContent := string(writer.files[applyProfilePath])
	assert.Contains(t, applyContent, "github-oidc")
	assert.Contains(t, applyContent, "terraform")

	// Verify profiles README.
	readmePath := filepath.Join(base, "profiles", "README.md")
	assert.True(t, writer.FileExists(readmePath))

	// Verify mixin.
	mixinPath := filepath.Join(base, "stacks", "mixins", "atmos-pro.yaml")
	assert.True(t, writer.FileExists(mixinPath))
	mixinContent := string(writer.files[mixinPath])
	assert.Contains(t, mixinContent, "drift_detection:")
	assert.Contains(t, mixinContent, "github_environment:")

	// Verify .atmos.d/ drop-in configs.
	ciPath := filepath.Join(base, ".atmos.d", "ci.yaml")
	assert.True(t, writer.FileExists(ciPath))
	ciContent := string(writer.files[ciPath])
	assert.Contains(t, ciContent, "ci:")
	assert.Contains(t, ciContent, "enabled: true")

	proPath := filepath.Join(base, ".atmos.d", "atmos-pro.yaml")
	assert.True(t, writer.FileExists(proPath))
	proContent := string(writer.files[proPath])
	assert.Contains(t, proContent, "settings:")
	assert.Contains(t, proContent, "pro:")
	assert.Contains(t, proContent, "base_url:")
}

func TestInstaller_Install_ExistingFiles_NoForce(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()

	// Pre-populate a workflow file.
	planPath := filepath.Join(base, githubDir, "workflows", "atmos-pro-terraform-plan.yaml")
	writer.files[planPath] = []byte("existing content")

	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
	)

	result, err := installer.Install()
	require.NoError(t, err)

	// The plan workflow should be skipped.
	assert.Contains(t, result.SkippedFiles,
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"))

	// Existing content should be preserved.
	assert.Equal(t, "existing content", string(writer.files[planPath]))
}

func TestInstaller_Install_ExistingFiles_WithForce(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()

	// Pre-populate a workflow file.
	planPath := filepath.Join(base, githubDir, "workflows", "atmos-pro-terraform-plan.yaml")
	writer.files[planPath] = []byte("existing content")

	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
		WithForce(true),
	)

	result, err := installer.Install()
	require.NoError(t, err)

	// The plan workflow should be created (overwritten), not skipped.
	assert.Contains(t, result.CreatedFiles,
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"))
	assert.NotContains(t, result.SkippedFiles,
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"))

	// Content should be overwritten.
	assert.NotEqual(t, "existing content", string(writer.files[planPath]))
}

func TestInstaller_Install_OnConflict_Overwrite(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()

	// Pre-populate a workflow file.
	planPath := filepath.Join(base, githubDir, "workflows", "atmos-pro-terraform-plan.yaml")
	writer.files[planPath] = []byte("existing content")

	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
		WithOnConflict(func(_ string) (bool, error) {
			return true, nil // Always overwrite.
		}),
	)

	result, err := installer.Install()
	require.NoError(t, err)

	// The plan workflow should be overwritten.
	assert.Contains(t, result.CreatedFiles,
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"))
	assert.NotEqual(t, "existing content", string(writer.files[planPath]))
}

func TestInstaller_Install_OnConflict_Skip(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()

	// Pre-populate a workflow file.
	planPath := filepath.Join(base, githubDir, "workflows", "atmos-pro-terraform-plan.yaml")
	writer.files[planPath] = []byte("existing content")

	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
		WithOnConflict(func(_ string) (bool, error) {
			return false, nil // Always skip.
		}),
	)

	result, err := installer.Install()
	require.NoError(t, err)

	// The plan workflow should be skipped.
	assert.Contains(t, result.SkippedFiles,
		filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"))
	assert.Equal(t, "existing content", string(writer.files[planPath]))
}

func TestInstaller_Install_OnConflict_Error(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()

	// Pre-populate a workflow file.
	planPath := filepath.Join(base, githubDir, "workflows", "atmos-pro-terraform-plan.yaml")
	writer.files[planPath] = []byte("existing content")

	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
		WithOnConflict(func(_ string) (bool, error) {
			return false, fmt.Errorf("non-interactive mode")
		}),
	)

	_, err := installer.Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-interactive mode")
}

func TestInstaller_DryRun(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()
	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("stacks"),
	)

	result := installer.DryRun()

	// Nothing should be written.
	assert.Empty(t, writer.written)

	// Should report what would be created.
	assert.NotEmpty(t, result.CreatedFiles)
	assert.Empty(t, result.SkippedFiles)

	// Verify atmos.yaml and .atmos.d/ files are included.
	assert.Contains(t, result.CreatedFiles, "atmos.yaml")
	assert.Contains(t, result.CreatedFiles, filepath.Join(".atmos.d", "ci.yaml"))
	assert.Contains(t, result.CreatedFiles, filepath.Join(".atmos.d", "atmos-pro.yaml"))
}

func TestNewInstaller_Defaults(t *testing.T) {
	writer := newMockFileWriter()
	installer := NewInstaller(writer)

	assert.Equal(t, "stacks", installer.opts.StacksBasePath)
	assert.Equal(t, "", installer.opts.BasePath)
	assert.False(t, installer.opts.Force)
	assert.False(t, installer.opts.DryRun)
}

func TestNewInstaller_WithOptions(t *testing.T) {
	base := t.TempDir()
	writer := newMockFileWriter()
	installer := NewInstaller(writer,
		WithBasePath(base),
		WithStacksBasePath("custom-stacks"),
		WithForce(true),
		WithDryRun(true),
	)

	assert.Equal(t, base, installer.opts.BasePath)
	assert.Equal(t, "custom-stacks", installer.opts.StacksBasePath)
	assert.True(t, installer.opts.Force)
	assert.True(t, installer.opts.DryRun)
}
