package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// constructTerraformComponentWorkingDir constructs the working dir for a terraform component in a stack
func constructTerraformComponentWorkingDir(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		cliConfig.BasePath,
		cliConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// constructTerraformComponentPlanfileName constructs the planfile name for a terraform component in a stack
func constructTerraformComponentPlanfileName(info schema.ConfigAndStacksInfo) string {
	var planFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", info.ContextPrefix, info.Component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return planFile
}

// constructTerraformComponentVarfileName constructs the varfile name for a terraform component in a stack
func constructTerraformComponentVarfileName(info schema.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return varFile
}

// constructTerraformComponentVarfilePath constructs the varfile path for a terraform component in a stack
func constructTerraformComponentVarfilePath(Config schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructTerraformComponentWorkingDir(Config, info),
		constructTerraformComponentVarfileName(info),
	)
}

// constructTerraformComponentPlanfilePath constructs the planfile path for a terraform component in a stack
func constructTerraformComponentPlanfilePath(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructTerraformComponentWorkingDir(cliConfig, info),
		constructTerraformComponentPlanfileName(info),
	)
}

// constructHelmfileComponentWorkingDir constructs the working dir for a helmfile component in a stack
func constructHelmfileComponentWorkingDir(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		cliConfig.BasePath,
		cliConfig.Components.Helmfile.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// constructHelmfileComponentVarfileName constructs the varfile name for a helmfile component in a stack
func constructHelmfileComponentVarfileName(info schema.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}
	return varFile
}

// constructHelmfileComponentVarfilePath constructs the varfile path for a helmfile component in a stack
func constructHelmfileComponentVarfilePath(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructHelmfileComponentWorkingDir(cliConfig, info),
		constructHelmfileComponentVarfileName(info),
	)
}

// findFoldersNamesWithPrefix finds the names of folders that match the given prefix under the specified root path.
// The search is performed at the root level (level 1) and one level deeper (level 2).
func findFoldersNamesWithPrefix(root, prefix string) ([]string, error) {
	var folderNames []string
	if root == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}
	// First, read the directories at the root level (level 1)
	level1Dirs, err := os.ReadDir(root)
	if err != nil {
		u.LogWarning(schema.CliConfiguration{}, fmt.Sprintf("Error reading root directory %s: %v", root, err))
		return nil, err
	}

	for _, dir := range level1Dirs {
		if dir.IsDir() {
			// If the directory at level 1 matches the prefix, add it
			if prefix == "" || strings.HasPrefix(dir.Name(), prefix) {
				folderNames = append(folderNames, dir.Name())
			}

			// Now, explore one level deeper (level 2)
			level2Path := filepath.Join(root, dir.Name())
			level2Dirs, err := os.ReadDir(level2Path)
			if err != nil {
				u.LogWarning(schema.CliConfiguration{}, fmt.Sprintf("Error reading subdirectory %s: %v", level2Path, err))
				continue
			}

			for _, subDir := range level2Dirs {
				if subDir.IsDir() && (prefix == "" || strings.HasPrefix(subDir.Name(), prefix)) {
					folderNames = append(folderNames, filepath.Join(dir.Name(), subDir.Name()))
				}
			}
		}
	}

	return folderNames, nil
}

// DeleteFilesAndFoldersRecursive deletes specified files and folders from the base path,
// including those found in immediate subdirectories.
func deleteFilesAndFoldersRecursive(basePath string, items []string) error {
	// First, delete files and folders directly under the base path
	for _, item := range items {
		fullPath := filepath.Join(basePath, item)

		// Attempt to delete the file or folder
		err := os.RemoveAll(fullPath)
		if err != nil {
			xMark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("x")
			u.LogInfo(schema.CliConfiguration{}, fmt.Sprintf("%s Error deleting %s", xMark, item))
			continue
		}
		checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
		u.LogInfo(schema.CliConfiguration{}, fmt.Sprintf("%s Deleted %s", checkMark, item))

	}

	// Now, delete matching files and folders from immediate subdirectories
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("error reading the base path %s: %v", basePath, err)
	}

	for _, entry := range entries {
		// Only proceed if the entry is a directory
		if entry.IsDir() {
			subDirPath := filepath.Join(basePath, entry.Name())

			for _, item := range items {
				fullPath := filepath.Join(subDirPath, item)
				// Attempt to delete the file or folder
				err := os.RemoveAll(fullPath)
				if err != nil {
					u.LogWarning(schema.CliConfiguration{}, fmt.Sprintf("Error deleting %s: %v", item, err))
					continue
				}
				checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
				u.LogInfo(schema.CliConfiguration{}, fmt.Sprintf("%s Deleted %s", checkMark, item))
			}
		}
	}

	return nil
}

// Helper functions to improve readability and maintainability
func cleanAllComponents(cliConfig schema.CliConfiguration, filesToClear []string, componentPath string, force bool) error {
	if componentPath == "" {
		return fmt.Errorf("component path cannot be empty")
	}
	if !force {
		message := "Are you sure"
		confirm, err := confirmDeleteTerraformLocal(message)
		if err != nil {
			return err
		}
		if !confirm {
			u.LogWarning(cliConfig, "mission aborted.")
			return nil
		}
	}
	// get relative path form base path

	err := deleteFilesAndFoldersRecursive(componentPath, filesToClear)
	if err != nil {
		u.LogWarning(cliConfig, err.Error())
	}

	return nil
}

func cleanSpecificComponent(cliConfig schema.CliConfiguration, componentPath string, filesToClear []string, force bool, componentName string, baseComponentName string) error {
	if componentPath == "" {
		return fmt.Errorf("component path cannot be empty")
	}
	if componentName == "" || baseComponentName == "" {
		return fmt.Errorf("component name cannot be empty")
	}

	if !force {
		message := "Are you sure"
		confirm, err := confirmDeleteTerraformLocal(message)
		if err != nil {
			return err
		}
		if !confirm {
			u.LogWarning(cliConfig, "mission aborted.")
			return nil
		}
	}

	return deleteFilesAndFoldersRecursive(componentPath, filesToClear)
}

func cleanStackComponent(cliConfig schema.CliConfiguration, componentPath, stack string, force bool, component string) error {
	if componentPath == "" {
		return fmt.Errorf("component path cannot be empty")
	}
	if stack == "" {
		return fmt.Errorf("stack name cannot be empty")
	}
	if component == "" {
		return fmt.Errorf("component name cannot be empty")
	}
	if !force {
		message := "Are you sure"
		confirm, err := confirmDeleteTerraformLocal(message)
		if err != nil {
			return err
		}
		if !confirm {
			u.LogWarning(cliConfig, "mission aborted.")
			return nil
		}
	}

	tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d")
	tfStateFolderNames, err := findFoldersNamesWithPrefix(tfStateFolderPath, stack)
	if err != nil {
		return fmt.Errorf("failed to find stack folders: %w", err)
	}

	for _, folderName := range tfStateFolderNames {
		tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d", folderName)
		if err := os.RemoveAll(tfStateFolderPath); err != nil {
			return fmt.Errorf("failed to delete stack folder %s: %w", folderName, err)
		}
		checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
		u.LogInfo(schema.CliConfiguration{}, fmt.Sprintf("%s Deleted 'terraform.tfstate.d/%s' folder", checkMark, folderName))
	}
	return nil
}

func confirmDeleteTerraformLocal(message string) (confirm bool, err error) {
	confirm = false
	t := huh.ThemeCharm()
	cream := lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}
	purple := lipgloss.Color("#5B00FF")
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(cream).Background(purple)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(purple)
	t.Blurred.Title = t.Blurred.Title.Foreground(purple)
	confirmPrompt := huh.NewConfirm().
		Title(message).
		Affirmative("Yes!").
		Negative("No.").
		Value(&confirm).WithTheme(t)
	if err := confirmPrompt.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return confirm, fmt.Errorf("mission aborted")
		}
		return confirm, err
	}

	return confirm, nil
}
