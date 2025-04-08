package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

var (
	ErrParseStacks               = errors.New("could not parse stacks")
	ErrParseComponents           = errors.New("could not parse components")
	ErrParseTerraformComponents  = errors.New("could not parse Terraform components")
	ErrParseComponentsAttributes = errors.New("could not parse component attributes")
	ErrDescribeStack             = errors.New("error describe stacks")
	ErrEmptyPath                 = errors.New("path cannot be empty")
	ErrPathNotExist              = errors.New("path not exist")
	ErrFileStat                  = errors.New("error get file stat")
	ErrMatchPattern              = errors.New("error matching pattern")
	ErrReadDir                   = errors.New("error reading directory")
	ErrFailedFoundStack          = errors.New("failed to find stack folders")
	ErrCollectFiles              = errors.New("failed to collect files")
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
func findFoldersNamesWithPrefix(root, prefix string, atmosConfig schema.AtmosConfiguration) ([]string, error) {
	var folderNames []string
	if root == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}
	// First, read the directories at the root level (level 1)
	level1Dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("%w path %s: error %v", ErrReadDir, root, err)
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
				u.LogWarning(fmt.Sprintf("Error reading subdirectory %s: %v", level2Path, err))
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
		return nil, fmt.Errorf("%w %s", ErrPathNotExist, basePath)
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
			return nil, fmt.Errorf("%w,path %s error %v", ErrFileStat, filePath, err)
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
				return fmt.Errorf("%w %s in folder %s: %v", ErrMatchPattern, pat, folderPath, err)
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
func getStackTerraformStateFolder(componentPath string, stack string, atmosConfig schema.AtmosConfiguration) ([]Directory, error) {
	tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d")
	tfStateFolderNames, err := findFoldersNamesWithPrefix(tfStateFolderPath, stack, atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("%w : %v", ErrFailedFoundStack, err)
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
			return nil, fmt.Errorf("%w in %s: %v", ErrCollectFiles, tfStateFolderPath, err)
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
		xMark := theme.Styles.XMark
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
		xMark := theme.Styles.XMark
		fmt.Printf("%s Error deleting %s", xMark, objectName)
		fmt.Println()
		return err
	}
	checkMark := theme.Styles.Checkmark
	fmt.Printf("%s Deleted %s", checkMark, objectName)
	fmt.Println()
	return nil
}

// confirmDeletion prompts the user for confirmation before deletion.
func confirmDeletion(atmosConfig schema.AtmosConfiguration) (bool, error) {
	message := "Are you sure?"
	confirm, err := confirmDeleteTerraformLocal(message)
	if err != nil {
		return false, err
	}
	if !confirm {
		u.LogWarning("Mission aborted.")
		return false, nil
	}
	return true, nil
}

// deleteFolders handles the deletion of the specified folders and files.
func deleteFolders(folders []Directory, relativePath string, atmosConfig schema.AtmosConfiguration) {
	var errors []error
	for _, folder := range folders {
		for _, file := range folder.Files {
			fileRel, err := getRelativePath(atmosConfig.BasePath, file.FullPath)
			if err != nil {
				log.Debug(fmt.Errorf("failed to get relative path for %s: %w", file.FullPath, err))
				fileRel = filepath.Join(relativePath, file.Name)
			}
			if file.IsDir {
				if err := DeletePathTerraform(file.FullPath, fileRel+"/"); err != nil {
					errors = append(errors, fmt.Errorf("failed to delete %s: %w", fileRel, err))
				}
			} else {
				if err := DeletePathTerraform(file.FullPath, fileRel); err != nil {
					errors = append(errors, fmt.Errorf("failed to delete %s: %w", fileRel, err))
				}
			}
		}
	}
	if len(errors) > 0 {
		for _, err := range errors {
			u.LogWarning(err.Error())
		}
	}
	// check if the folder is empty by using the os.ReadDir function
	for _, folder := range folders {
		entries, err := os.ReadDir(folder.FullPath)
		if err == nil && len(entries) == 0 {
			if err := os.Remove(folder.FullPath); err != nil {
				u.LogWarning(fmt.Sprintf("Error removing directory %s: %v", folder.FullPath, err))
			}
		}
	}
}

// handleTFDataDir handles the deletion of the TF_DATA_DIR if specified.
func handleTFDataDir(componentPath string, relativePath string, atmosConfig schema.AtmosConfiguration) {
	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" {
		return
	}
	if err := IsValidDataDir(tfDataDir); err != nil {
		u.LogWarning(err.Error())
		return
	}
	if _, err := os.Stat(filepath.Join(componentPath, tfDataDir)); os.IsNotExist(err) {
		u.LogWarning(fmt.Sprintf("TF_DATA_DIR '%s' does not exist", tfDataDir))
		return
	}
	if err := DeletePathTerraform(filepath.Join(componentPath, tfDataDir), filepath.Join(relativePath, tfDataDir)); err != nil {
		u.LogWarning(err.Error())
	}
}

func initializeFilesToClear(info schema.ConfigAndStacksInfo, atmosConfig schema.AtmosConfiguration) []string {
	if info.ComponentFromArg == "" {
		return []string{".terraform", ".terraform.lock.hcl", "*.tfvar.json", "terraform.tfstate.d"}
	}
	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)
	files := []string{".terraform", varFile, planFile}

	if !u.SliceContainsString(info.AdditionalArgsAndFlags, skipTerraformLockFileFlag) {
		files = append(files, ".terraform.lock.hcl")
	}

	if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
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
func handleCleanSubCommand(info schema.ConfigAndStacksInfo, componentPath string, atmosConfig schema.AtmosConfiguration) error {
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

	relativePath, err := getRelativePath(atmosConfig.BasePath, componentPath)
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
	filesToClear := initializeFilesToClear(info, atmosConfig)
	var FilterComponents []string
	if info.ComponentFromArg != "" {
		FilterComponents = append(FilterComponents, info.ComponentFromArg)
	}
	stacksMap, err := ExecuteDescribeStacks(
		atmosConfig, info.StackFromArg,
		FilterComponents,
		nil, nil, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrDescribeStack, err)
	}
	allComponentsRelativePaths := getAllStacksComponentsPaths(stacksMap)
	folders, err := CollectComponentsDirectoryObjects(atmosConfig.TerraformDirAbsolutePath, allComponentsRelativePaths, filesToClear)
	if err != nil {
		log.Debug("error collecting folders and files", "error", err)
		return err
	}
	if info.Component != "" && info.Stack != "" {
		stackFolders, err := getStackTerraformStateFolder(cleanPath, info.Stack, atmosConfig)
		if err != nil {
			errMsg := fmt.Errorf("error getting stack terraform state folders: %v", err)
			u.LogTrace(errMsg.Error())
		}
		if stackFolders != nil {
			folders = append(folders, stackFolders...)
		}
	}
	tfDataDir := os.Getenv("TF_DATA_DIR")

	var tfDataDirFolders []Directory
	if tfDataDir != "" {
		if err := IsValidDataDir(tfDataDir); err != nil {
			u.LogTrace(err.Error())
		} else {
			tfDataDirFolders, err = CollectDirectoryObjects(cleanPath, []string{tfDataDir})
			if err != nil {
				u.LogTrace(fmt.Errorf("error collecting folder of ENV TF_DATA_DIR: %v", err).Error())
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
			if info.ComponentFromArg == "" {
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
			if confirm, err := confirmDeletion(atmosConfig); err != nil || !confirm {
				return err
			}
		}

		deleteFolders(folders, relativePath, atmosConfig)
		if len(tfDataDirFolders) > 0 {
			tfDataDirFolder := tfDataDirFolders[0]
			handleTFDataDir(tfDataDirFolder.FullPath, relativePath, atmosConfig)
		}

	}

	return nil
}

// getAllStacksComponentsPaths retrieves all components relatives paths to base Terraform directory from the stacks map.
func getAllStacksComponentsPaths(stacksMap map[string]any) []string {
	var allComponentsPaths []string
	for _, stackData := range stacksMap {
		componentsPath, err := getComponentsPaths(stackData)
		if err != nil {
			continue // Skip invalid components path.
		}
		allComponentsPaths = append(allComponentsPaths, componentsPath...)
	}
	return allComponentsPaths
}

func getComponentsPaths(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, ErrParseComponents
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, ErrParseTerraformComponents
	}
	keys := lo.Keys(terraformComponents)
	var componentPaths []string
	for _, key := range keys {
		components, ok := terraformComponents[key].(map[string]any)
		if !ok {
			return nil, ErrParseTerraformComponents
		}
		// component attributes reference to relative path
		componentPath, ok := components["component"].(string)
		if !ok {
			return nil, ErrParseComponentsAttributes
		}
		componentPaths = append(componentPaths, componentPath)
	}

	return componentPaths, nil
}

func CollectComponentsDirectoryObjects(terraformDirAbsolutePath string, allComponentsRelativePaths []string, filesToClear []string) ([]Directory, error) {
	var allFolders []Directory
	for _, path := range allComponentsRelativePaths {
		componentPath := filepath.Join(terraformDirAbsolutePath, path)
		folders, err := CollectComponentObjects(terraformDirAbsolutePath, componentPath, filesToClear)
		if err != nil {
			log.Debug("collecting folders and files", "error", err)
			return nil, err
		}
		allFolders = append(allFolders, folders...)
	}
	return allFolders, nil
}

func CollectComponentObjects(terraformDirAbsolutePath string, componentPath string, patterns []string) ([]Directory, error) {
	if err := validateInputPath(terraformDirAbsolutePath); err != nil {
		return nil, err
	}

	baseFolder, err := createFolder(terraformDirAbsolutePath, componentPath, filepath.Base(componentPath))
	if err != nil {
		return nil, err
	}

	if err := collectFilesInFolder(baseFolder, componentPath, patterns); err != nil {
		return nil, err
	}
	var folders []Directory
	if len(baseFolder.Files) > 0 {
		folders = append(folders, *baseFolder)
	}

	return folders, nil
}

func validateInputPath(path string) error {
	if path == "" {
		return ErrEmptyPath
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%w %s", ErrPathNotExist, path)

	}
	return nil
}

func createFolder(rootPath, folderPath, folderName string) (*Directory, error) {
	relativePath, err := filepath.Rel(rootPath, folderPath)
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

func collectFilesInFolder(folder *Directory, folderPath string, patterns []string) error {
	for _, pat := range patterns {
		matchedFiles, err := filepath.Glob(filepath.Join(folderPath, pat))
		if err != nil {
			return fmt.Errorf("error matching pattern %s in folder %s: %v", pat, folderPath, err)
		}

		for _, matchedFile := range matchedFiles {
			fileInfo, err := createFileInfo(folder.FullPath, matchedFile)
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

func createFileInfo(rootPath, filePath string) (*ObjectInfo, error) {
	relativePath, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return nil, fmt.Errorf("error determining relative path for %s: %v", filePath, err)
	}

	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, nil // Skip if the file doesn't exist
	}
	if err != nil {
		return nil, fmt.Errorf("error stating file %s: %v", filePath, err)
	}

	return &ObjectInfo{
		FullPath:     filePath,
		RelativePath: relativePath,
		Name:         filepath.Base(filePath),
		IsDir:        info.IsDir(),
	}, nil
}
