package testhelpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/utils"
)

// SandboxEnvironment holds the state for a sandboxed test.
type SandboxEnvironment struct {
	TempDir         string
	ComponentsPath  string
	OriginalWorkdir string
}

// SetupSandbox creates an isolated test environment with copied components.
func SetupSandbox(t *testing.T, workdir string) (*SandboxEnvironment, error) {
	t.Helper()

	// Validate and prepare workdir.
	absWorkdir, err := validateWorkdir(workdir)
	if err != nil {
		return nil, err
	}

	// Create sandbox directory.
	tempDir, err := os.MkdirTemp("", "atmos-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// Get component paths from configuration.
	componentPaths := getComponentPaths(absWorkdir)

	// Copy components to sandbox.
	sandboxComponentsPath := filepath.Join(tempDir, "components")
	if err := copyComponentsToSandbox(absWorkdir, sandboxComponentsPath, componentPaths, tempDir); err != nil {
		return nil, err
	}

	return &SandboxEnvironment{
		TempDir:         tempDir,
		ComponentsPath:  sandboxComponentsPath,
		OriginalWorkdir: absWorkdir,
	}, nil
}

// validateWorkdir validates and returns the absolute path of the workdir.
func validateWorkdir(workdir string) (string, error) {
	if workdir == "" {
		return "", errUtils.ErrEmptyWorkdir
	}

	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve workdir: %w", err)
	}

	if _, err := os.Stat(absWorkdir); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s", errUtils.ErrWorkdirNotExist, absWorkdir)
	}

	return absWorkdir, nil
}

// getComponentPaths retrieves component paths from configuration or returns defaults.
func getComponentPaths(workdir string) map[string]string {
	paths, err := extractComponentPaths(workdir)
	if err != nil {
		// Fall back to default paths if config parsing fails.
		return map[string]string{
			"terraform": "../../components/terraform",
			"helmfile":  "../../components/helmfile",
		}
	}
	return paths
}

// copyComponentsToSandbox copies component directories to the sandbox.
func copyComponentsToSandbox(absWorkdir, sandboxComponentsPath string, componentPaths map[string]string, tempDir string) error {
	for componentType, relPath := range componentPaths {
		if relPath == "" {
			continue
		}

		if err := copySingleComponentType(absWorkdir, sandboxComponentsPath, componentType, relPath); err != nil {
			os.RemoveAll(tempDir)
			return err
		}
	}
	return nil
}

// copySingleComponentType copies a single component type to the sandbox.
//
//nolint:nilerr // We intentionally return nil for non-existent components to continue processing other components
func copySingleComponentType(absWorkdir, sandboxComponentsPath, componentType, relPath string) error {
	srcPath := utils.JoinPath(absWorkdir, relPath)
	srcAbsPath, err := filepath.Abs(srcPath)
	if err != nil {
		// Skip if path doesn't resolve, not a critical error for sandbox setup.
		return nil
	}

	if _, err := os.Stat(srcAbsPath); os.IsNotExist(err) {
		// Skip if source doesn't exist, not a critical error for sandbox setup.
		return nil
	}

	dstPath := filepath.Join(sandboxComponentsPath, componentType)
	if err := copyToSandbox(srcAbsPath, dstPath); err != nil {
		return fmt.Errorf("failed to copy %s components: %w", componentType, err)
	}

	return nil
}

// copyToSandbox copies directories excluding terraform artifacts.
func copyToSandbox(src, dst string) error {
	const dirPerm = 0o755
	// Create destination directory.
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return err
	}

	// On Windows, use native Go implementation directly since rsync/cp don't exist.
	if runtime.GOOS == "windows" {
		return copyDir(src, dst)
	}

	// On Unix systems, try rsync first (fastest).
	if err := copyDirWithRsync(src, dst); err == nil {
		return nil
	}

	// Fallback to cp.
	if err := copyDirWithCp(src, dst); err == nil {
		// Clean up terraform artifacts after copy.
		return cleanTerraformArtifacts(dst)
	}

	// Final fallback to native Go implementation.
	return copyDir(src, dst)
}

// copyDirWithRsync attempts to copy using rsync with exclusions.
func copyDirWithRsync(src, dst string) error {
	if _, err := exec.LookPath("rsync"); err != nil {
		return err
	}

	// #nosec G204 -- src and dst are validated paths from test environment setup
	cmd := exec.Command("rsync", "-a",
		"--exclude=.terraform",
		"--exclude=.terraform.lock.hcl",
		"--exclude=*.terraform.tfvars.json",
		"--exclude=terraform.tfstate.d",
		"--exclude=backend.tf.json",
		"--exclude=*.planfile",
		"--exclude=*.planfile.json",
		"--exclude=terraform.tfstate",
		"--exclude=terraform.tfstate.backup",
		src+"/", dst+"/")
	return cmd.Run()
}

// copyDirWithCp attempts to copy using cp command.
func copyDirWithCp(src, dst string) error {
	cmd := exec.Command("cp", "-r", src, dst)
	return cmd.Run()
}

// copyDir copies a directory recursively using native Go, excluding terraform artifacts.
func copyDir(src, dst string) error {
	const dirPerm = 0o755
	// Create destination directory if it doesn't exist.
	if err := os.MkdirAll(dst, dirPerm); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Read source directory entries.
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip terraform artifacts using existing shouldRemoveArtifact function.
		if shouldRemoveArtifact(info.Name()) {
			continue
		}

		if info.IsDir() {
			// Recursively copy subdirectory.
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file.
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file preserving permissions.
func copyFile(src, dst string) error {
	// Get source file info using Lstat to detect symlinks.
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source file %q: %w", src, err)
	}

	// Skip symlinks - they can cause issues especially on Windows.
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	// Read source file.
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file %q: %w", src, err)
	}

	// Write destination file with same permissions.
	if err := os.WriteFile(dst, data, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to write destination file %q: %w", dst, err)
	}

	return nil
}

// cleanTerraformArtifacts removes terraform artifacts from the destination.
func cleanTerraformArtifacts(dst string) error {
	return filepath.Walk(dst, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Skip files we can't access instead of failing the whole operation.
			return filepath.SkipDir
		}

		if shouldRemoveArtifact(info.Name()) {
			os.RemoveAll(path)
			if info.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

// shouldRemoveArtifact checks if a file or directory should be removed.
func shouldRemoveArtifact(name string) bool {
	// Check for terraform artifacts.
	switch name {
	case ".terraform", ".terraform.lock.hcl", "terraform.tfstate.d",
		"backend.tf.json", "terraform.tfstate", "terraform.tfstate.backup":
		return true
	}

	// Check for tfvars.json and planfile files.
	const (
		tfvarsSuffix       = ".terraform.tfvars.json"
		planfileSuffix     = ".planfile"
		planfileJSONSuffix = ".planfile.json"
	)

	if len(name) > len(tfvarsSuffix) && name[len(name)-len(tfvarsSuffix):] == tfvarsSuffix {
		return true
	}
	if len(name) > len(planfileSuffix) && name[len(name)-len(planfileSuffix):] == planfileSuffix {
		return true
	}
	if len(name) > len(planfileJSONSuffix) && name[len(name)-len(planfileJSONSuffix):] == planfileJSONSuffix {
		return true
	}

	return false
}

// extractComponentPaths parses atmos.yaml and extracts component base paths.
func extractComponentPaths(workdir string) (map[string]string, error) {
	atmosYamlPath := filepath.Join(workdir, "atmos.yaml")
	data, err := os.ReadFile(atmosYamlPath)
	if err != nil {
		return nil, err
	}

	var config struct {
		Components struct {
			Terraform struct {
				BasePath string `yaml:"base_path"`
			} `yaml:"terraform"`
			Helmfile struct {
				BasePath string `yaml:"base_path"`
			} `yaml:"helmfile"`
		} `yaml:"components"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	paths := make(map[string]string)
	if config.Components.Terraform.BasePath != "" {
		paths["terraform"] = config.Components.Terraform.BasePath
	}
	if config.Components.Helmfile.BasePath != "" {
		paths["helmfile"] = config.Components.Helmfile.BasePath
	}

	return paths, nil
}

// GetEnvironmentVariables returns environment variables for sandbox execution.
func (s *SandboxEnvironment) GetEnvironmentVariables() map[string]string {
	env := make(map[string]string)

	// Override component paths using Atmos environment variables.
	terraformPath := filepath.Join(s.ComponentsPath, "terraform")
	if _, err := os.Stat(terraformPath); err == nil {
		env["ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"] = terraformPath
	}

	helmfilePath := filepath.Join(s.ComponentsPath, "helmfile")
	if _, err := os.Stat(helmfilePath); err == nil {
		env["ATMOS_COMPONENTS_HELMFILE_BASE_PATH"] = helmfilePath
	}

	return env
}

// Cleanup removes the sandbox directory.
func (s *SandboxEnvironment) Cleanup() {
	if s.TempDir != "" {
		os.RemoveAll(s.TempDir)
	}
}
