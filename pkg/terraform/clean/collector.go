package clean

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Error format strings for consistent error wrapping.
const errFmtPathWithCause = "%w %s: %w"

// CollectDirectoryObjects collects files matching patterns in the given base path.
//
//nolint:gocognit,revive,cyclop,funlen // complexity is inherent in recursive directory traversal with pattern matching
func CollectDirectoryObjects(basePath string, patterns []string) ([]Directory, error) {
	defer perf.Track(nil, "clean.CollectDirectoryObjects")()

	if basePath == "" {
		return nil, ErrEmptyPath
	}
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w %s", ErrPathNotExist, basePath)
	}
	var folders []Directory

	// Helper function to add file information if it exists.
	addFileInfo := func(filePath string) (*ObjectInfo, error) {
		relativePath, err := filepath.Rel(basePath, filePath)
		if err != nil {
			return nil, fmt.Errorf(errFmtPathWithCause, ErrRelPath, filePath, err)
		}
		info, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			return nil, nil // Skip if the file doesn't exist.
		} else if err != nil {
			return nil, fmt.Errorf("%w path %s: %w", ErrFileStat, filePath, err)
		}

		return &ObjectInfo{
			FullPath:     filePath,
			RelativePath: relativePath,
			Name:         filepath.Base(filePath),
			IsDir:        info.IsDir(),
		}, nil
	}

	// Function to create a folder entry with its files.
	createFolder := func(folderPath string, folderName string) (*Directory, error) {
		relativePath, err := filepath.Rel(basePath, folderPath)
		if err != nil {
			return nil, fmt.Errorf(errFmtPathWithCause, ErrRelPath, folderPath, err)
		}

		return &Directory{
			Name:         folderName,
			FullPath:     folderPath,
			RelativePath: relativePath,
			Files:        []ObjectInfo{},
		}, nil
	}

	// Function to collect files for a given folder path.
	collectFilesInFolder := func(folder *Directory, folderPath string) error {
		for _, pat := range patterns {
			matchedFiles, err := filepath.Glob(filepath.Join(folderPath, pat))
			if err != nil {
				return fmt.Errorf("%w %s in folder %s: %w", ErrMatchPattern, pat, folderPath, err)
			}

			// Add matched files to folder.
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

	// Collect files for the base path itself.
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

	// Now, search for folders and their files from immediate subdirectories.
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("%w path %s: %w", ErrReadDir, basePath, err)
	}

	for _, entry := range entries {
		// Skip non-directories.
		if !entry.IsDir() {
			continue
		}

		subDirPath := filepath.Join(basePath, entry.Name())

		// Create the folder entry.
		folder, err := createFolder(subDirPath, entry.Name())
		if err != nil {
			return nil, err
		}

		// Collect files in the subdirectory.
		err = collectFilesInFolder(folder, subDirPath)
		if err != nil {
			return nil, err
		}

		// Add folder to the list only if it contains files.
		if len(folder.Files) > 0 {
			folders = append(folders, *folder)
		}
	}

	return folders, nil
}

// FindFoldersNamesWithPrefix finds the names of folders that match the given prefix under the specified root path.
// The search is performed at the root level (level 1) and one level deeper (level 2).
//
//nolint:revive // cyclomatic: complexity is inherent in two-level directory search with prefix matching
func FindFoldersNamesWithPrefix(root, prefix string) ([]string, error) {
	defer perf.Track(nil, "clean.FindFoldersNamesWithPrefix")()

	var folderNames []string
	if root == "" {
		return nil, ErrRootPath
	}
	// First, read the directories at the root level (level 1).
	level1Dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("%w path %s: %w", ErrReadDir, root, err)
	}

	for _, dir := range level1Dirs {
		// Skip non-directories.
		if !dir.IsDir() {
			continue
		}

		// If the directory at level 1 matches the prefix, add it.
		if prefix == "" || strings.HasPrefix(dir.Name(), prefix) {
			folderNames = append(folderNames, dir.Name())
		}

		// Now, explore one level deeper (level 2).
		level2Path := filepath.Join(root, dir.Name())
		level2Dirs, err := os.ReadDir(level2Path)
		if err != nil {
			log.Debug("Error reading subdirectory", "directory", level2Path, "error", err)
			continue
		}

		for _, subDir := range level2Dirs {
			if subDir.IsDir() && (prefix == "" || strings.HasPrefix(subDir.Name(), prefix)) {
				folderNames = append(folderNames, filepath.Join(dir.Name(), subDir.Name()))
			}
		}
	}

	return folderNames, nil
}

// GetStackTerraformStateFolder gets stack terraform state files.
func GetStackTerraformStateFolder(componentPath string, stack string) ([]Directory, error) {
	defer perf.Track(nil, "clean.GetStackTerraformStateFolder")()

	tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d")
	tfStateFolderNames, err := FindFoldersNamesWithPrefix(tfStateFolderPath, stack)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedFoundStack, err)
	}
	var stackTfStateFolders []Directory
	for _, folderName := range tfStateFolderNames {
		tfStateFolderPath := filepath.Join(componentPath, "terraform.tfstate.d", folderName)
		// Check if exists.
		if _, err := os.Stat(tfStateFolderPath); os.IsNotExist(err) {
			continue
		}
		directories, err := CollectDirectoryObjects(tfStateFolderPath, []string{"*.tfstate", "*.tfstate.backup"})
		if err != nil {
			return nil, fmt.Errorf("%w in %s: %w", ErrCollectFiles, tfStateFolderPath, err)
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

	return filepath.Join(filepath.Base(absBasePath), relPath), nil
}

// collectTFDataDirFolders collects folders from TF_DATA_DIR environment variable.
func collectTFDataDirFolders(cleanPath string) ([]Directory, string) {
	tfDataDir := os.Getenv(EnvTFDataDir) //nolint:forbidigo // TF_DATA_DIR is a Terraform runtime env var, not an Atmos config option.
	if tfDataDir == "" {
		return nil, ""
	}

	if err := IsValidDataDir(tfDataDir); err != nil {
		log.Debug("error validating TF_DATA_DIR", "error", err)
		return nil, tfDataDir
	}

	folders, err := CollectDirectoryObjects(cleanPath, []string{tfDataDir})
	if err != nil {
		log.Debug("error collecting folder of ENV TF_DATA_DIR", "error", err)
		return nil, tfDataDir
	}
	return folders, tfDataDir
}

// GetAllStacksComponentsPaths retrieves all component relative paths to base Terraform directory from the stacks map.
// It deduplicates paths to avoid processing the same component multiple times when multiple stacks reference it.
func GetAllStacksComponentsPaths(stacksMap map[string]any) []string {
	defer perf.Track(nil, "clean.GetAllStacksComponentsPaths")()

	var allComponentsPaths []string
	uniquePaths := make(map[string]bool)

	for _, stackData := range stacksMap {
		componentsPath, err := getComponentsPaths(stackData)
		if err != nil {
			log.Debug("Skip invalid components path", "path", componentsPath)
			continue // Skip invalid components path.
		}
		// Add only unique paths to avoid duplicates when multiple stacks reference the same component.
		for _, path := range componentsPath {
			if !uniquePaths[path] {
				uniquePaths[path] = true
				allComponentsPaths = append(allComponentsPaths, path)
			}
		}
	}
	return allComponentsPaths
}

// getComponentsPaths extracts component paths from a single stack's data.
func getComponentsPaths(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, ErrParseTerraformComponents
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, ErrParseTerraformComponents
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, ErrParseTerraformComponents
	}

	var componentPaths []string
	for _, componentData := range terraformComponents {
		components, ok := componentData.(map[string]any)
		if !ok {
			return nil, ErrParseTerraformComponents
		}
		// component attributes reference to relative path.
		componentPath, ok := components["component"].(string)
		if !ok {
			return nil, ErrParseComponentsAttributes
		}
		componentPaths = append(componentPaths, componentPath)
	}

	return componentPaths, nil
}

// CollectComponentsDirectoryObjects collects files matching patterns for multiple component paths.
// It handles deduplication of component paths to avoid processing the same component multiple times.
func CollectComponentsDirectoryObjects(terraformDirAbsolutePath string, allComponentsRelativePaths []string, patterns []string) ([]Directory, error) {
	defer perf.Track(nil, "clean.CollectComponentsDirectoryObjects")()

	// Validate input path.
	if terraformDirAbsolutePath == "" {
		return nil, ErrEmptyPath
	}
	if _, err := os.Stat(terraformDirAbsolutePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w %s", ErrPathNotExist, terraformDirAbsolutePath)
	}

	// Deduplicate paths to avoid processing the same component multiple times.
	uniquePaths := make(map[string]bool)
	var deduplicatedPaths []string
	for _, path := range allComponentsRelativePaths {
		if !uniquePaths[path] {
			uniquePaths[path] = true
			deduplicatedPaths = append(deduplicatedPaths, path)
		}
	}

	var allFolders []Directory
	for _, path := range deduplicatedPaths {
		componentPath := filepath.Join(terraformDirAbsolutePath, path)
		folders, err := collectComponentObjects(terraformDirAbsolutePath, componentPath, patterns)
		if err != nil {
			log.Debug("collecting folders and files", "error", err)
			return nil, err
		}
		allFolders = append(allFolders, folders...)
	}
	return allFolders, nil
}

// collectComponentObjects collects files matching patterns for a single component path.
func collectComponentObjects(terraformDirAbsolutePath string, componentPath string, patterns []string) ([]Directory, error) {
	defer perf.Track(nil, "clean.collectComponentObjects")()

	baseFolder, err := createComponentFolder(terraformDirAbsolutePath, componentPath, filepath.Base(componentPath))
	if err != nil {
		return nil, err
	}

	if err := collectFilesInComponentFolder(baseFolder, componentPath, patterns); err != nil {
		return nil, err
	}

	var folders []Directory
	if len(baseFolder.Files) > 0 {
		folders = append(folders, *baseFolder)
	}

	return folders, nil
}

// createComponentFolder creates a Directory struct for a component path.
func createComponentFolder(rootPath, folderPath, folderName string) (*Directory, error) {
	relativePath, err := filepath.Rel(rootPath, folderPath)
	if err != nil {
		return nil, fmt.Errorf(errFmtPathWithCause, ErrRelPath, folderPath, err)
	}

	return &Directory{
		Name:         folderName,
		FullPath:     folderPath,
		RelativePath: relativePath,
		Files:        []ObjectInfo{},
	}, nil
}

// collectFilesInComponentFolder collects files matching patterns in a folder.
func collectFilesInComponentFolder(folder *Directory, folderPath string, patterns []string) error {
	for _, pat := range patterns {
		matchedFiles, err := filepath.Glob(filepath.Join(folderPath, pat))
		if err != nil {
			return fmt.Errorf("%w %s in folder %s: %w", ErrMatchPattern, pat, folderPath, err)
		}

		for _, matchedFile := range matchedFiles {
			fileInfo, err := createComponentFileInfo(folder.FullPath, matchedFile)
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

// createComponentFileInfo creates an ObjectInfo struct for a file.
func createComponentFileInfo(rootPath, filePath string) (*ObjectInfo, error) {
	relativePath, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s error %w", ErrRelPath, filePath, err)
	}

	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, nil // Skip if the file doesn't exist.
	}
	if err != nil {
		return nil, fmt.Errorf(errFmtPathWithCause, ErrFileStat, filePath, err)
	}

	return &ObjectInfo{
		FullPath:     filePath,
		RelativePath: relativePath,
		Name:         filepath.Base(filePath),
		IsDir:        info.IsDir(),
	}, nil
}
