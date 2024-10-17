package exec

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

// constructTerraformComponentWorkingDir constructs the working dir for a terraform component in a stack
func constructTerraformComponentWorkingDir(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return path.Join(
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
	return path.Join(
		constructTerraformComponentWorkingDir(Config, info),
		constructTerraformComponentVarfileName(info),
	)
}

// constructTerraformComponentPlanfilePath constructs the planfile path for a terraform component in a stack
func constructTerraformComponentPlanfilePath(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return path.Join(
		constructTerraformComponentWorkingDir(cliConfig, info),
		constructTerraformComponentPlanfileName(info),
	)
}

// constructHelmfileComponentWorkingDir constructs the working dir for a helmfile component in a stack
func constructHelmfileComponentWorkingDir(cliConfig schema.CliConfiguration, info schema.ConfigAndStacksInfo) string {
	return path.Join(
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
	return path.Join(
		constructHelmfileComponentWorkingDir(cliConfig, info),
		constructHelmfileComponentVarfileName(info),
	)
}

// findFoldersNamesWithPrefix finds all the folders with the specified prefix return the list of folder names
// If prefix is empty, it returns all the folders in the root
func findFoldersNamesWithPrefix(root, prefix string) ([]string, error) {
	var folderNames []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check if the current file is a directory and has the specified prefix
		if info.IsDir() && prefix == "" {
			folderNames = append(folderNames, info.Name()) // Collect all folder names
		} else {
			if info.IsDir() && strings.HasPrefix(info.Name(), prefix) {
				folderNames = append(folderNames, info.Name()) // Collect folder names only
			}
		}

		return nil
	})
	return folderNames, err
}

// deleteFilesAndFoldersRecursive deletes files and folders from the given list if they exist under the specified path,
// including files and folders in the second-level subdirectories.
func deleteFilesAndFoldersRecursive(basePath string, items []string) error {

	// First, delete files and folders directly under the base path
	for _, item := range items {
		fullPath := filepath.Join(basePath, item)

		// Check if the file or folder exists
		if _, err := os.Stat(fullPath); err == nil {
			// File or folder exists, attempt to delete
			err := os.RemoveAll(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("failed to delete %s: %v", fullPath, err)
			}
			lastFolderName := filepath.Base(basePath)

			fmt.Printf("Deleted: %s/%s\n", lastFolderName, item)
		}
	}

	// Now, check second-level directories and delete matching files and folders
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		// Skip the base directory itself
		if path == basePath {
			return nil
		}

		// Check if it's a directory and proceed with deletion in second-level directories
		if info.IsDir() {
			for _, item := range items {
				fullPath := filepath.Join(path, item)

				// Check if the file or folder exists in the subdirectory
				if _, err := os.Stat(fullPath); err == nil {

					// File or folder exists, attempt to delete
					err := os.RemoveAll(fullPath)
					if err != nil {
						if os.IsNotExist(err) {
							continue
						}
						return fmt.Errorf("failed to delete %s: %v", item, err)
					}
					lastFolderName := filepath.Base(basePath)

					fmt.Printf("Deleted: %s/%s\n", lastFolderName, item)
				}
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking the path %s: %v", basePath, err)
	}

	return nil
}
