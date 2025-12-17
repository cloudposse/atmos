package exec

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	tfclean "github.com/cloudposse/atmos/pkg/terraform/clean"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

var ErrRelPath = errors.New("error determining relative path")

// Type aliases from pkg/terraform/clean for use in internal/exec.
type (
	ObjectInfo = tfclean.ObjectInfo
	Directory  = tfclean.Directory
)

// Error aliases from pkg/terraform/clean.
var (
	ErrParseTerraformComponents  = tfclean.ErrParseTerraformComponents
	ErrParseComponentsAttributes = tfclean.ErrParseComponentsAttributes
	ErrEmptyPath                 = tfclean.ErrEmptyPath
	ErrPathNotExist              = tfclean.ErrPathNotExist
	ErrFileStat                  = tfclean.ErrFileStat
	ErrMatchPattern              = tfclean.ErrMatchPattern
	ErrRootPath                  = tfclean.ErrRootPath
	ErrReadDir                   = tfclean.ErrReadDir
	ErrFailedFoundStack          = tfclean.ErrFailedFoundStack
	ErrEmptyEnvDir               = tfclean.ErrEmptyEnvDir
	ErrRefusingToDeleteDir       = tfclean.ErrRefusingToDeleteDir
)

// getAllStacksComponentsPaths retrieves all components relatives paths to base Terraform directory from the stacks map.
func getAllStacksComponentsPaths(stacksMap map[string]any) []string {
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

func getComponentsPaths(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, errUtils.ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, errUtils.ErrParseComponents
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
		// component attributes reference to relative path.
		componentPath, ok := components["component"].(string)
		if !ok {
			return nil, ErrParseComponentsAttributes
		}
		componentPaths = append(componentPaths, componentPath)
	}

	return componentPaths, nil
}

func CollectComponentsDirectoryObjects(terraformDirAbsolutePath string, allComponentsRelativePaths []string, filesToClear []string) ([]Directory, error) {
	defer perf.Track(nil, "exec.CollectComponentsDirectoryObjects")()

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
	defer perf.Track(nil, "exec.CollectComponentObjects")()

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
		return nil, fmt.Errorf("%w %s: %v", ErrRelPath, folderPath, err)
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
			return fmt.Errorf("%w %s in folder %s: %v", ErrMatchPattern, pat, folderPath, err)
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
		return nil, fmt.Errorf("%w: %s error %v", ErrRelPath, filePath, err)
	}

	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, nil // Skip if the file doesn't exist
	}
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrFileStat, filePath, err)
	}

	return &ObjectInfo{
		FullPath:     filePath,
		RelativePath: relativePath,
		Name:         filepath.Base(filePath),
		IsDir:        info.IsDir(),
	}, nil
}
