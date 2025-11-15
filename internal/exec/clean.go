package exec

import (
	"fmt"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/filematch"
	"github.com/hashicorp/go-multierror"
)

// GetFilesToBeDeleted retrieves file paths to be deleted based on stack and component filters.
func GetFilesToBeDeleted(stackMap map[string]any, component, stack string) ([]string, error) {
	var paths []string
	for stackName, stackInfo := range stackMap {
		if !isValidStack(stackInfo, stackName, stack) {
			continue
		}
		newPaths, err := processStackInfo(stackInfo, component)
		if err != nil {
			return nil, err
		}
		paths = append(paths, newPaths...)
	}
	return paths, nil
}

// isValidStack checks if the stack info is valid and matches the stack filter.
func isValidStack(stackInfo any, stackName, stack string) bool {
	if stackInfo == nil {
		return false
	}
	if stack != "" && stackName != stack {
		return false
	}
	return true
}

// processStackInfo processes stack information to extract file paths.
func processStackInfo(stackInfo any, component string) ([]string, error) {
	info, ok := stackInfo.(map[string]any)
	if !ok {
		return nil, nil
	}
	components, ok := info["components"].(map[string]any)
	if !ok {
		return nil, nil
	}
	return processComponents(components, component)
}

// processComponents processes component types to extract file paths.
func processComponents(components map[string]any, component string) ([]string, error) {
	var paths []string
	for componentType, componentTypeMap := range components {
		if componentType != "terraform" {
			continue
		}
		newPaths, err := processComponentType(componentTypeMap, component)
		if err != nil {
			return nil, err
		}
		paths = append(paths, newPaths...)
	}
	return paths, nil
}

// processComponentType processes a component type map to extract file paths.
func processComponentType(componentTypeMap any, component string) ([]string, error) {
	typeMap, ok := componentTypeMap.(map[string]any)
	if !ok {
		return nil, nil
	}
	var paths []string
	for componentName, componentValue := range typeMap {
		if component != "" && componentName != component {
			continue
		}
		newPaths, err := extractCleanPatterns(componentValue)
		if err != nil {
			return nil, err
		}
		paths = append(paths, newPaths...)
	}
	return paths, nil
}

// extractCleanPatterns extracts and matches clean patterns from component settings.
func extractCleanPatterns(componentValue any) ([]string, error) {
	componentMap, ok := componentValue.(map[string]any)
	if !ok {
		return nil, nil
	}
	settings, ok := componentMap["settings"].(map[string]any)
	if !ok {
		return nil, nil
	}
	cleanSetting, ok := settings["clean"].(map[string]any)
	if !ok {
		return nil, nil
	}
	cleanPatterns, ok := cleanSetting["paths"].([]any)
	if !ok {
		return nil, nil
	}
	return filematch.NewGlobMatcher().MatchFiles(convertToStringArray(cleanPatterns))
}

// convertToStringArray converts an array of any to an array of strings.
func convertToStringArray(arr []any) []string {
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// Filesystem defines the filesystem operations required for DeletePaths.
//
//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type Filesystem interface {
	Lstat(name string) (os.FileInfo, error)
	RemoveAll(name string) error
	Stat(name string) (os.FileInfo, error)
}

// DeletePaths deletes all files and directories in the provided paths slice, logging detailed information.
// It skips empty paths, logs non-existent paths, and aggregates errors encountered during deletion.
// The function checks if paths still exist after deletion attempts and logs warnings or errors accordingly.
// Returns a combined error if any operations fail, or nil if all succeed or no actions are taken.
func deletePaths(fs Filesystem, paths []string) error {
	var result *multierror.Error

	for _, path := range paths {
		if path == "" {
			log.Info("Skipping empty path")
			continue
		}

		// Check if path exists
		info, err := fs.Lstat(path)
		if os.IsNotExist(err) {
			log.Infof("Path does not exist: %s", path)
			continue
		}
		if err != nil {
			log.Errorf("Failed to stat path %s: %v", path, err)
			result = multierror.Append(result, fmt.Errorf("stat %s: %w", path, err))
			continue
		}

		// Log deletion attempt with permissions
		log.Infof("Attempting to delete: %s (mode: %s)", path, info.Mode())

		// Attempt deletion
		if err := fs.RemoveAll(path); err != nil {
			log.Errorf("Failed to delete path %s: %v", path, err)
			result = multierror.Append(result, fmt.Errorf("delete %s: %w", path, err))
			continue
		}

		// Verify deletion
		_, err = fs.Stat(path)
		switch {
		case err == nil:
			log.Warnf("Path still exists after deletion: %s", path)
			result = multierror.Append(result, fmt.Errorf("path %s still exists after deletion", path))
		case !os.IsNotExist(err):
			log.Errorf("Post-deletion stat error for %s: %v", path, err)
			result = multierror.Append(result, fmt.Errorf("post-deletion stat %s: %w", path, err))
		default:
			log.Infof("Successfully deleted: %s", path)
		}
	}

	return result.ErrorOrNil()
}

// DeletePaths deletes all files and directories in the provided paths slice, logging detailed information.
// It skips empty paths, logs non-existent paths, and aggregates errors encountered during deletion.
// The function checks if paths still exist after deletion attempts and logs warnings or errors to the standard logger.
// Returns a combined error if any operations fail, or nil if all succeed or no actions are taken.
func DeletePaths(paths []string) error {
	// Use os as the filesystem implementation
	fs := &osFilesystem{}
	return deletePaths(fs, paths)
}

// osFilesystem adapts the os package to the Filesystem interface.
type osFilesystem struct{}

func (fs *osFilesystem) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (fs *osFilesystem) RemoveAll(name string) error {
	return os.RemoveAll(name)
}

func (fs *osFilesystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
