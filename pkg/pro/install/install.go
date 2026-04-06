package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// File permission for created files.
	fileMode = 0o644
	// Directory permission for created directories.
	dirMode = 0o755
	// GithubDir is the GitHub configuration directory.
	githubDir = ".github"
	// WorkflowsDir is the GitHub Actions workflows directory name.
	workflowsDir = "workflows"
)

// FileWriter abstracts filesystem operations for testability.
type FileWriter interface {
	// WriteFile writes content to a file with the given permissions.
	WriteFile(path string, content []byte, perm os.FileMode) error
	// MkdirAll creates a directory and all parent directories.
	MkdirAll(path string, perm os.FileMode) error
	// FileExists returns true if the file exists.
	FileExists(path string) bool
	// ReadFile reads the content of a file.
	ReadFile(path string) ([]byte, error)
}

// OSFileWriter implements FileWriter using the real filesystem.
type OSFileWriter struct{}

// WriteFile writes content to a file on disk.
func (w *OSFileWriter) WriteFile(path string, content []byte, perm os.FileMode) error {
	return os.WriteFile(path, content, perm)
}

// MkdirAll creates directories on disk.
func (w *OSFileWriter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// FileExists checks if a file exists on disk.
func (w *OSFileWriter) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadFile reads a file from disk.
func (w *OSFileWriter) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// InstallResult reports what happened during installation.
type InstallResult struct {
	// CreatedFiles lists files that were created.
	CreatedFiles []string
	// SkippedFiles lists files that were skipped because they already exist.
	SkippedFiles []string
	// UpdatedFiles lists files that were updated (e.g., _defaults.yaml import added).
	UpdatedFiles []string
}

// fileSpec describes a file to install.
type fileSpec struct {
	// RelPath is the path relative to BasePath.
	RelPath string
	// Content is the file content.
	Content string
}

// Installer scaffolds Atmos Pro configuration files.
type Installer struct {
	writer FileWriter
	opts   Options
}

// NewInstaller creates a new installer with the given options.
func NewInstaller(writer FileWriter, options ...Option) *Installer {
	opts := Options{
		StacksBasePath: "stacks",
	}
	for _, opt := range options {
		opt(&opts)
	}
	return &Installer{
		writer: writer,
		opts:   opts,
	}
}

// Install creates all Atmos Pro configuration files.
func (i *Installer) Install() (*InstallResult, error) {
	defer perf.Track(nil, "install.Installer.Install")()

	result := &InstallResult{}

	// Define all files to install.
	files := i.buildFileSpecs()

	// Write each file.
	for _, f := range files {
		fullPath := filepath.Join(i.opts.BasePath, f.RelPath)
		if err := i.writeFile(fullPath, f.Content, result); err != nil {
			return result, fmt.Errorf("failed to create %s: %w", f.RelPath, err)
		}
	}

	// Handle _defaults.yaml separately (merge logic).
	if err := i.ensureDefaults(result); err != nil {
		return result, err
	}

	return result, nil
}

// DryRun returns what would be created without writing files.
func (i *Installer) DryRun() *InstallResult {
	defer perf.Track(nil, "install.Installer.DryRun")()

	result := &InstallResult{}
	files := i.buildFileSpecs()

	for _, f := range files {
		fullPath := filepath.Join(i.opts.BasePath, f.RelPath)
		if i.writer.FileExists(fullPath) && !i.opts.Force {
			result.SkippedFiles = append(result.SkippedFiles, f.RelPath)
		} else {
			result.CreatedFiles = append(result.CreatedFiles, f.RelPath)
		}
	}

	// Check _defaults.yaml.
	defaultsPath := i.defaultsPath()
	if i.writer.FileExists(defaultsPath) {
		content, err := i.writer.ReadFile(defaultsPath)
		if err == nil && !hasImport(string(content), "mixins/atmos-pro") {
			result.UpdatedFiles = append(result.UpdatedFiles, i.defaultsRelPath())
		} else {
			result.SkippedFiles = append(result.SkippedFiles, i.defaultsRelPath())
		}
	} else {
		result.CreatedFiles = append(result.CreatedFiles, i.defaultsRelPath())
	}

	return result
}

// buildFileSpecs returns the list of files to install.
func (i *Installer) buildFileSpecs() []fileSpec {
	stacksBase := i.opts.StacksBasePath

	return []fileSpec{
		// GitHub Actions workflows.
		{
			RelPath: filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-plan.yaml"),
			Content: planWorkflowTemplate,
		},
		{
			RelPath: filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-apply.yaml"),
			Content: applyWorkflowTemplate,
		},
		{
			RelPath: filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-drift-detection.yaml"),
			Content: driftDetectionWorkflowTemplate,
		},
		{
			RelPath: filepath.Join(githubDir, workflowsDir, "atmos-pro-terraform-drift-remediation.yaml"),
			Content: driftRemediationWorkflowTemplate,
		},
		// Auth profile.
		{
			RelPath: filepath.Join("profiles", "github", "atmos.yaml"),
			Content: githubProfileTemplate,
		},
		// Stack mixin.
		{
			RelPath: filepath.Join(stacksBase, "mixins", "atmos-pro.yaml"),
			Content: proMixinTemplate,
		},
	}
}

// writeFile creates a file, handling directory creation and force/skip logic.
func (i *Installer) writeFile(fullPath, content string, result *InstallResult) error {
	relPath, _ := filepath.Rel(i.opts.BasePath, fullPath)

	if i.writer.FileExists(fullPath) && !i.opts.Force {
		result.SkippedFiles = append(result.SkippedFiles, relPath)
		return nil
	}

	dir := filepath.Dir(fullPath)
	if err := i.writer.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := i.writer.WriteFile(fullPath, []byte(content), fileMode); err != nil {
		return err
	}

	result.CreatedFiles = append(result.CreatedFiles, relPath)
	return nil
}

// defaultsRelPath returns the relative path to _defaults.yaml.
func (i *Installer) defaultsRelPath() string {
	return filepath.Join(i.opts.StacksBasePath, "deploy", "_defaults.yaml")
}

// defaultsPath returns the absolute path to _defaults.yaml.
func (i *Installer) defaultsPath() string {
	return filepath.Join(i.opts.BasePath, i.defaultsRelPath())
}

// ensureDefaults creates or updates _defaults.yaml with the atmos-pro import.
func (i *Installer) ensureDefaults(result *InstallResult) error {
	fullPath := i.defaultsPath()
	relPath := i.defaultsRelPath()

	if !i.writer.FileExists(fullPath) {
		// Create new _defaults.yaml from template.
		dir := filepath.Dir(fullPath)
		if err := i.writer.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		if err := i.writer.WriteFile(fullPath, []byte(defaultsSnippetTemplate), fileMode); err != nil {
			return fmt.Errorf("failed to create %s: %w", relPath, err)
		}
		result.CreatedFiles = append(result.CreatedFiles, relPath)
		return nil
	}

	// File exists - check if import is already present.
	content, err := i.writer.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", relPath, err)
	}

	if hasImport(string(content), "mixins/atmos-pro") {
		result.SkippedFiles = append(result.SkippedFiles, relPath)
		return nil
	}

	// Add the import to the existing file.
	updated := addImport(string(content), "mixins/atmos-pro")
	if err := i.writer.WriteFile(fullPath, []byte(updated), fileMode); err != nil {
		return fmt.Errorf("failed to update %s: %w", relPath, err)
	}
	result.UpdatedFiles = append(result.UpdatedFiles, relPath)
	return nil
}

// hasImport checks if a YAML file already imports the given path.
func hasImport(content, importPath string) bool {
	// Check for the import in common formats:
	// - mixins/atmos-pro
	// - "mixins/atmos-pro"
	// - 'mixins/atmos-pro'
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "- "+importPath ||
			trimmed == "- \""+importPath+"\"" ||
			trimmed == "- '"+importPath+"'" {
			return true
		}
	}
	return false
}

// addImport adds an import entry to an existing YAML file.
// If the file has an import section, it appends to it.
// If not, it prepends the import section.
func addImport(content, importPath string) string {
	lines := strings.Split(content, "\n")
	importLine := "  - " + importPath

	// Look for an existing import: section.
	for idx, line := range lines {
		if strings.TrimSpace(line) != "import:" {
			continue
		}
		// Insert after the import: line.
		result := make([]string, 0, len(lines)+1)
		result = append(result, lines[:idx+1]...)
		result = append(result, importLine)
		result = append(result, lines[idx+1:]...)
		return strings.Join(result, "\n")
	}

	// No import section found - prepend one.
	importSection := "import:\n" + importLine + "\n\n"
	return importSection + content
}
