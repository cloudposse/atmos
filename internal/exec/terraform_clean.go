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

type ObjectInfo struct {
	FullPath     string
	RelativePath string
	Name         string
	IsDir        bool
}

type Directory struct {
	Name         string
	FullPath     string
	RelativePath string
	Files        []ObjectInfo
}

// findFoldersNamesWithPrefix finds the names of folders that match the given prefix under the specified root path.
// The search is performed at the root level (level 1) and one level deeper (level 2).
func findFoldersNamesWithPrefix(root, prefix string, cliConfig schema.CliConfiguration) ([]string, error) {
	var folderNames []string
	if root == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}
	// First, read the directories at the root level (level 1)
	level1Dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("error reading root directory %s: %w", root, err)
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
				u.LogWarning(cliConfig, fmt.Sprintf("Error reading subdirectory %s: %v", level2Path, err))
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

func CollectDirectoryObjects(basePath string, patterns []string) ([]Directory, error) {
	if basePath == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("path %s does not exist", basePath)
	}
	var folders []Directory

	// Helper function to add file information if it exists
	addFileInfo := func(filePath string) (*ObjectInfo, error) {
		relativePath, err := filepath.Rel(basePath, filePath)
		if err != nil {
			return nil, fmt.Errorf("error determining relative path for %s: %v", filePath, err)
		}
		info, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			return nil, nil // Skip if the file doesn't exist
		} else if err != nil {
			return nil, fmt.Errorf("error stating file %s: %v", filePath, err)
		}

		return &ObjectInfo{
			FullPath:     filePath,
			RelativePath: relativePath,
			Name:         filepath.Base(filePath),
			IsDir:        info.IsDir(),
		}, nil
	}

	// Function to create a folder entry with its files
	createFolder := func(folderPath string, folderName string) (*Directory, error) {
		relativePath, err := filepath.Rel(basePath, folderPath)
		if err != nil {
			return nil, fmt.Errorf("error determining relative path for folder %s: %v", folderPath, err)
		}

		return &Directory{
			Name:         folderName,
			FullPath:     folderPath,
			RelativePath: relativePath,
			Files:        []ObjectInfo{},
		}, nil
	}

	// Function to collect files for a given folder path
	collectFilesInFolder := func(folder *Directory, folderPath string) error {
		for _, pat := range patterns {
			matchedFiles, err := filepath.Glob(filepath.Join(folderPath, pat))
			if err != nil {
				return fmt.Errorf("error matching pattern %s in folder %s: %v", pat, folderPath, err)
			}

			// Add matched files to folder
			for _, matchedFile := range matchedFiles {
				fileInfo, err := addFileInfo(matchedFile)
				if err != nil {
					return err
				}
				if fileInfo != nil {
					folder.Files = append(folder.Files, *fileInfo)
				}
			}
		}
		return nil
	}

	// Collect files for the base path itself
	baseFolder, err := createFolder(basePath, filepath.Base(basePath))
	if err != nil {
		return nil, err
	}
	err = collectFilesInFolder(baseFolder, basePath)
	if err != nil {
		return nil, err
	}
	if len(baseFolder.Files) > 0 {
		folders = append(folders, *baseFolder)
	}

	// Now, search for folders and their files from immediate subdirectories
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("error reading the base path %s: %v", basePath, err)
	}

	for _, entry := range entries {
		// Only proceed if the entry is a directory
		if entry.IsDir() {
			subDirPath := filepath.Join(basePath, entry.Name())

			// Create the folder entry
			folder, err := createFolder(subDirPath, entry.Name())
			if err != nil {
				return nil, err
			}

			// Collect files in the subdirectory
			err = collectFilesInFolder(folder, subDirPath)
			if err != nil {
				return nil, err
			}

			// Add folder to the list only if it contains files
			if len(folder.Files) > 0 {
				folders = append(folders, *folder)
			}
		}
	}

	return folders, nil
}

// get stack terraform state files
func getStackTerraformStateFolder(componentPath string, stack string, cliConfig schema.CliConfiguration) ([]Directory, error) {
	tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d")
	tfStateFolderNames, err := findFoldersNamesWithPrefix(tfStateFolderPath, stack, cliConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to find stack folders: %w", err)
	}
	var stackTfStateFolders []Directory
	for _, folderName := range tfStateFolderNames {
		tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d", folderName)
		// Check if exists
		if _, err := os.Stat(tfStateFolderPath); os.IsNotExist(err) {
			continue
		}
		directories, err := CollectDirectoryObjects(tfStateFolderPath, []string{"*.tfstate", "*.tfstate.backup"})
		if err != nil {
			return nil, fmt.Errorf("failed to collect files in %s: %w", tfStateFolderPath, err)
		}
		for i := range directories {
			if directories[i].Files != nil {
				for j := range directories[i].Files {
					directories[i].Files[j].Name = folderName + "/" + directories[i].Files[j].Name

				}

			}
		}
		stackTfStateFolders = append(stackTfStateFolders, directories...)
	}

	return stackTfStateFolders, nil
}

// getRelativePath computes the relative path from basePath to componentPath.
func getRelativePath(basePath, componentPath string) (string, error) {
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	absComponentPath, err := filepath.Abs(componentPath)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(absBasePath, absComponentPath)
	if err != nil {
		return "", err
	}

	return filepath.Base(absBasePath) + "/" + relPath, nil
}
func confirmDeleteTerraformLocal(message string) (confirm bool, err error) {
	confirm = false
	t := huh.ThemeCharm()
	cream := lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}
	purple := lipgloss.AdaptiveColor{Light: "#5B00FF", Dark: "#5B00FF"}
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
			return confirm, fmt.Errorf("Mission aborted")
		}
		return confirm, err
	}

	return confirm, nil
}

// DeletePathTerraform deletes the specified file or folder. with a checkmark or xmark
func DeletePathTerraform(fullPath string, objectName string) error {
	fileInfo, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		xMark := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("x")
		fmt.Printf("%s Cannot delete %s: path does not exist", xMark, objectName)
		fmt.Println()
		return err
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to delete symbolic link: %s", objectName)
	}
	// Proceed with deletion
	err = os.RemoveAll(fullPath)
	if err != nil {
		xMark := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("x")
		fmt.Printf("%s Error deleting %s", xMark, objectName)
		fmt.Println()
		return err
	}
	checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	fmt.Printf("%s Deleted %s", checkMark, objectName)
	fmt.Println()
	return nil
}

// confirmDeletion prompts the user for confirmation before deletion.
func confirmDeletion(cliConfig schema.CliConfiguration) (bool, error) {
	message := "Are you sure?"
	confirm, err := confirmDeleteTerraformLocal(message)
	if err != nil {
		return false, err
	}
	if !confirm {
		u.LogWarning(cliConfig, "Mission aborted.")
		return false, nil
	}
	return true, nil
}

// deleteFolders handles the deletion of the specified folders and files.
func deleteFolders(folders []Directory, relativePath string, cliConfig schema.CliConfiguration) {
	for _, folder := range folders {
		for _, file := range folder.Files {
			path := filepath.ToSlash(filepath.Join(relativePath, file.Name))
			if file.IsDir {
				DeletePathTerraform(file.FullPath, path+"/")
			} else {
				DeletePathTerraform(file.FullPath, path)
			}
		}
	}
	// check if the folder is empty by using the os.ReadDir function
	for _, folder := range folders {
		entries, err := os.ReadDir(folder.FullPath)
		if err == nil && len(entries) == 0 {
			if err := os.Remove(folder.FullPath); err != nil {
				u.LogWarning(cliConfig, fmt.Sprintf("Error removing directory %s: %v", folder.FullPath, err))
			}
		}
	}

}

// handleTFDataDir handles the deletion of the TF_DATA_DIR if specified.
func handleTFDataDir(componentPath string, relativePath string, cliConfig schema.CliConfiguration) {

	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" {
		return
	}
	if err := IsValidDataDir(tfDataDir); err != nil {
		u.LogWarning(cliConfig, err.Error())
		return
	}
	if _, err := os.Stat(filepath.Join(componentPath, tfDataDir)); os.IsNotExist(err) {
		u.LogWarning(cliConfig, fmt.Sprintf("TF_DATA_DIR '%s' does not exist", tfDataDir))
		return
	}
	if err := DeletePathTerraform(filepath.Join(componentPath, tfDataDir), filepath.Join(relativePath, tfDataDir)); err != nil {
		u.LogWarning(cliConfig, err.Error())
	}

}
func initializeFilesToClear(info schema.ConfigAndStacksInfo, cliConfig schema.CliConfiguration, everything bool) []string {
	if everything && info.Stack == "" {
		return []string{".terraform", ".terraform.lock.hcl", "*.tfvar.json", "terraform.tfstate.d"}
	}
	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)
	files := []string{".terraform", varFile, planFile}

	if !u.SliceContainsString(info.AdditionalArgsAndFlags, skipTerraformLockFileFlag) {
		files = append(files, ".terraform.lock.hcl")
	}

	if cliConfig.Components.Terraform.AutoGenerateBackendFile {
		files = append(files, "backend.tf.json")
	}

	return files
}
func IsValidDataDir(tfDataDir string) error {
	if tfDataDir == "" {
		return fmt.Errorf("ENV TF_DATA_DIR is empty")
	}
	absTFDataDir, err := filepath.Abs(tfDataDir)
	if err != nil {
		return fmt.Errorf("error resolving TF_DATA_DIR path: %v", err)
	}
	if absTFDataDir == "/" || absTFDataDir == filepath.Clean("/") {
		return fmt.Errorf("refusing to delete root directory '/'")
	}
	if strings.Contains(absTFDataDir, "..") {
		return fmt.Errorf("refusing to delete directory containing '..'")
	}
	return nil
}

// handleCleanSubCommand handles the 'clean' subcommand logic.
func handleCleanSubCommand(info schema.ConfigAndStacksInfo, componentPath string, cliConfig schema.CliConfiguration) error {
	if info.SubCommand != "clean" {
		return nil
	}
	cleanPath := componentPath
	if info.ComponentFromArg != "" && info.StackFromArg == "" {
		if info.Context.BaseComponent == "" {
			return fmt.Errorf("could not find the component '%s'", info.ComponentFromArg)
		}
		cleanPath = filepath.Join(componentPath, info.Context.BaseComponent)
	}

	relativePath, err := getRelativePath(cliConfig.BasePath, componentPath)
	if err != nil {
		return err
	}
	if info.Context.BaseComponent != "" {
		// remove the base component from the relative path
		relativePath = strings.Replace(relativePath, info.Context.BaseComponent, "", 1)
		// remove the leading slash
		relativePath = strings.TrimPrefix(relativePath, "/")
	}

	force := u.SliceContainsString(info.AdditionalArgsAndFlags, forceFlag)
	everything := u.SliceContainsString(info.AdditionalArgsAndFlags, everythingFlag)
	filesToClear := initializeFilesToClear(info, cliConfig, everything)
	folders, err := CollectDirectoryObjects(cleanPath, filesToClear)
	if err != nil {
		u.LogTrace(cliConfig, fmt.Errorf("error collecting folders and files: %v", err).Error())
		return err
	}

	if info.Component != "" && info.Stack != "" {
		stackFolders, err := getStackTerraformStateFolder(cleanPath, info.Stack, cliConfig)
		if err != nil {
			errMsg := fmt.Errorf("error getting stack terraform state folders: %v", err)
			u.LogTrace(cliConfig, errMsg.Error())
		}
		if stackFolders != nil {
			folders = append(folders, stackFolders...)
		}
	}
	tfDataDir := os.Getenv("TF_DATA_DIR")

	var tfDataDirFolders []Directory
	if tfDataDir != "" {
		if err := IsValidDataDir(tfDataDir); err != nil {
			u.LogTrace(cliConfig, err.Error())
		} else {
			tfDataDirFolders, err = CollectDirectoryObjects(cleanPath, []string{tfDataDir})
			if err != nil {
				u.LogTrace(cliConfig, fmt.Errorf("error collecting folder of ENV TF_DATA_DIR: %v", err).Error())
			}
		}
	}
	objectCount := 0
	for _, folder := range folders {
		objectCount += len(folder.Files)
	}
	total := objectCount + len(tfDataDirFolders)

	if total == 0 {
		u.PrintMessage("Nothing to delete")
		return nil
	}

	if total > 0 {
		if !force {
			if len(tfDataDirFolders) > 0 {
				u.PrintMessage(fmt.Sprintf("Found ENV var TF_DATA_DIR=%s", tfDataDir))
				u.PrintMessage(fmt.Sprintf("Do you want to delete the folder '%s'? ", tfDataDir))
			}
			var message string
			if everything && info.ComponentFromArg == "" {
				message = fmt.Sprintf("This will delete %v local terraform state files affecting all components", total)
			} else if info.Component != "" && info.Stack != "" {
				message = fmt.Sprintf("This will delete %v local terraform state files for component '%s' in stack '%s'", total, info.Component, info.Stack)
			} else if info.ComponentFromArg != "" {
				message = fmt.Sprintf("This will delete %v local terraform state files for component '%s'", total, info.ComponentFromArg)
			} else {
				message = "This will delete selected terraform state files"
			}
			u.PrintMessage(message)
			println()
			if confirm, err := confirmDeletion(cliConfig); err != nil || !confirm {
				return err
			}
		}

		deleteFolders(folders, relativePath, cliConfig)
		if len(tfDataDirFolders) > 0 {
			tfDataDirFolder := tfDataDirFolders[0]
			handleTFDataDir(tfDataDirFolder.FullPath, relativePath, cliConfig)
		}

	}

	return nil
}
