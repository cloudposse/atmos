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
	Objects      []ObjectInfo
}

// createDirectory creates a Directory entry with its objects (files and sub-directories).
func createDirectory(basePath, dirPath, dirName string) (*Directory, error) {
	relativePath, err := filepath.Rel(basePath, dirPath)
	if err != nil {
		return nil, fmt.Errorf("error determining relative path for directory %s: %v", dirPath, err)
	}

	return &Directory{
		Name:         dirName,
		FullPath:     dirPath,
		RelativePath: relativePath,
		Objects:      []ObjectInfo{},
	}, nil
}

// addObjectInfo adds an ObjectInfo for a given path.
func addObjectInfo(basePath, objPath string) (*ObjectInfo, error) {
	relativePath, err := filepath.Rel(basePath, objPath)
	if err != nil {
		return nil, fmt.Errorf("error determining relative path for %s: %v", objPath, err)
	}

	info, err := os.Stat(objPath)
	if os.IsNotExist(err) {
		return nil, nil // Skip if the object doesn't exist
	} else if err != nil {
		return nil, fmt.Errorf("error stating object %s: %v", objPath, err)
	}

	return &ObjectInfo{
		FullPath:     objPath,
		RelativePath: relativePath,
		Name:         filepath.Base(objPath),
		IsDir:        info.IsDir(),
	}, nil
}

// collectObjectsInDirectory collects all objects within a given directory based on patterns.
func collectObjectsInDirectory(dir *Directory, dirPath string, patterns []string) error {
	for _, pat := range patterns {
		matchedPaths, err := filepath.Glob(filepath.Join(dirPath, pat))
		if err != nil {
			return fmt.Errorf("error matching pattern %s in directory %s: %v", pat, dirPath, err)
		}

		// Add matched objects (files or subdirectories) to the directory
		for _, matchedPath := range matchedPaths {
			objectInfo, err := addObjectInfo(dir.FullPath, matchedPath)
			if err != nil {
				return err
			}
			if objectInfo != nil {
				dir.Objects = append(dir.Objects, *objectInfo)
			}
		}
	}
	return nil
}

// CollectDirectoriesWithObjects collects all directories and their contained objects (files and subdirectories) based on patterns.
func CollectDirectoriesWithObjects(basePath string, patterns []string) ([]Directory, error) {
	if basePath == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	var directories []Directory

	// Collect objects for the base path itself
	baseDir, err := createDirectory(basePath, basePath, filepath.Base(basePath))
	if err != nil {
		return nil, err
	}
	err = collectObjectsInDirectory(baseDir, basePath, patterns)
	if err != nil {
		return nil, err
	}
	if len(baseDir.Objects) > 0 {
		directories = append(directories, *baseDir)
	}

	// Now, search for subdirectories and their objects
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("error reading the base path %s: %v", basePath, err)
	}

	for _, entry := range entries {
		// Only proceed if the entry is a directory
		if entry.IsDir() {
			subDirPath := filepath.Join(basePath, entry.Name())

			// Create the directory entry
			dir, err := createDirectory(basePath, subDirPath, entry.Name())
			if err != nil {
				return nil, err
			}

			// Collect objects in the subdirectory
			err = collectObjectsInDirectory(dir, subDirPath, patterns)
			if err != nil {
				return nil, err
			}

			// Add the directory to the list only if it contains objects
			if len(dir.Objects) > 0 {
				directories = append(directories, *dir)
			}
		}
	}

	return directories, nil
}

type FileInfo struct {
	FullPath     string
	RelativePath string
	Name         string
	IsDir        bool
}

type Folder struct {
	Name         string
	FullPath     string
	RelativePath string
	Files        []FileInfo
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

func CollectFoldersWithFiles(basePath string, patterns []string) ([]Folder, error) {
	if basePath == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	var folders []Folder

	// Helper function to add file information if it exists
	addFileInfo := func(filePath string) (*FileInfo, error) {
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

		return &FileInfo{
			FullPath:     filePath,
			RelativePath: relativePath,
			Name:         filepath.Base(filePath),
			IsDir:        info.IsDir(),
		}, nil
	}

	// Function to create a folder entry with its files
	createFolder := func(folderPath string, folderName string) (*Folder, error) {
		relativePath, err := filepath.Rel(basePath, folderPath)
		if err != nil {
			return nil, fmt.Errorf("error determining relative path for folder %s: %v", folderPath, err)
		}

		return &Folder{
			Name:         folderName,
			FullPath:     folderPath,
			RelativePath: relativePath,
			Files:        []FileInfo{},
		}, nil
	}

	// Function to collect files for a given folder path
	collectFilesInFolder := func(folder *Folder, folderPath string) error {
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
func getStackTerraformStateFolder(componentPath string, stack string) ([]Folder, error) {
	tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d")
	tfStateFolderNames, err := findFoldersNamesWithPrefix(tfStateFolderPath, stack)
	if err != nil {
		return nil, fmt.Errorf("failed to find stack folders: %w", err)
	}
	var stackTfStateFolders []Folder
	for _, folderName := range tfStateFolderNames {
		tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d", folderName)
		// Check if exists
		if _, err := os.Stat(tfStateFolderPath); os.IsNotExist(err) {
			continue
		}
		folder, err := CollectFoldersWithFiles(tfStateFolderPath, []string{"*.tfstate", "*.tfstate.backup"})
		if err != nil {
			return nil, fmt.Errorf("failed to collect files in %s: %w", tfStateFolderPath, err)
		}
		for i := range folder {
			if folder[i].Files != nil {
				for j := range folder[i].Files {
					folder[i].Files[j].Name = folderName + "/" + folder[i].Files[j].Name

				}

			}
		}
		stackTfStateFolders = append(stackTfStateFolders, folder...)
	}

	return stackTfStateFolders, nil
}

// determineCleanPath determines the path to clean based on component and stack arguments.
func determineCleanPath(info schema.ConfigAndStacksInfo, componentPath string) (string, error) {
	cleanPath := componentPath
	if info.ComponentFromArg != "" && info.StackFromArg == "" {
		if info.Context.BaseComponent == "" {
			return "", fmt.Errorf("could not find the component '%s'", info.ComponentFromArg)
		}
		cleanPath = filepath.Join(componentPath, info.Context.BaseComponent)
	}
	return cleanPath, nil
}

// getRelativePath computes the relative path from basePath to componentPath.
func getRelativePath(basePath, componentPath string) (string, error) {
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		fmt.Printf("Error getting absolute path for basePath: %v\n", err)
		return "", err
	}

	relPath, err := filepath.Rel(absBasePath, componentPath)
	if err != nil {
		fmt.Printf("Error getting relative path: %v\n", err)
		return "", err
	}

	return filepath.Base(absBasePath) + "/" + relPath, nil
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
			return confirm, fmt.Errorf("Mission aborted")
		}
		return confirm, err
	}

	return confirm, nil
}

// DeletePathTerraform deletes the specified file or folder. with a checkmark or xmark
func DeletePathTerraform(fullPath string, objectName string) error {
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		xMark := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("x")
		fmt.Printf("%s Error deleting %s", xMark, objectName)
		return err
	}
	// Attempt to delete the file or folder If the path does not exist, RemoveAll returns nil (no error)
	err := os.RemoveAll(fullPath)
	if err != nil {
		xMark := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("x")
		fmt.Printf("%s Error deleting %s", xMark, objectName)
		return err
	}

	checkMark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	fmt.Printf("%s deleted %s", checkMark, objectName)
	println()
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
func deleteFolders(folders []Folder, relativePath string) {
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
}

// handleTFDataDir handles the deletion of the TF_DATA_DIR if specified.
func handleTFDataDir(componentPath string, cliConfig schema.CliConfiguration) {
	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" || tfDataDir == "." || tfDataDir == "/" || tfDataDir == "./" {
		return
	}

	u.PrintMessage(fmt.Sprintf("Found ENV var TF_DATA_DIR=%s", tfDataDir))
	u.PrintMessage(fmt.Sprintf("Do you want to delete the folder '%s'? (only 'yes' will be accepted to approve)\n", tfDataDir))
	fmt.Print("Enter a value: ")

	var userAnswer string
	if _, err := fmt.Scanln(&userAnswer); err != nil {
		u.LogWarning(cliConfig, fmt.Sprintf("Error reading input: %v", err))
		return
	}

	if userAnswer == "yes" {
		u.PrintMessage(fmt.Sprintf("Deleting folder '%s'\n", tfDataDir))
		if err := os.RemoveAll(filepath.Join(componentPath, tfDataDir)); err != nil {
			u.LogWarning(cliConfig, err.Error())
		}
	}
}
func initializeFilesToClear(info schema.ConfigAndStacksInfo, cliConfig schema.CliConfiguration, everything bool) []string {
	if everything {
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

// handleCleanSubCommand handles the 'clean' subcommand logic.
func handleCleanSubCommand(info schema.ConfigAndStacksInfo, componentPath string, cliConfig schema.CliConfiguration) error {
	if info.SubCommand != "clean" {
		return nil
	}

	cleanPath, err := determineCleanPath(info, componentPath)
	if err != nil {
		return err
	}

	relativePath, err := getRelativePath(cliConfig.BasePath, componentPath)
	if err != nil {
		return err
	}

	force := u.SliceContainsString(info.AdditionalArgsAndFlags, forceFlag)
	everything := u.SliceContainsString(info.AdditionalArgsAndFlags, everythingFlag)
	filesToClear := initializeFilesToClear(info, cliConfig, everything)
	folders, err := CollectFoldersWithFiles(cleanPath, filesToClear)
	if err != nil {
		u.LogTrace(cliConfig, fmt.Errorf("error collecting folders and files: %v", err).Error())
		return err
	}

	if info.Component != "" && info.Stack != "" {
		stackFolders, err := getStackTerraformStateFolder(cleanPath, info.Stack)
		if err != nil {
			fmt.Println("Error getting stack folders:", err)
			return err
		}
		folders = append(folders, stackFolders...)
	}

	if len(folders) == 0 {
		u.LogWarning(cliConfig, "Nothing to delete")
		return nil
	}

	if !force {
		u.LogInfo(cliConfig, "This will delete all terraform state files for all components")
		if confirm, err := confirmDeletion(cliConfig); err != nil || !confirm {
			return err
		}
	}

	deleteFolders(folders, relativePath)

	if everything {
		return nil
	}

	handleTFDataDir(componentPath, cliConfig)

	return nil
}
