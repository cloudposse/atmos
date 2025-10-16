package exec

import (
	"path/filepath"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// isComponentFolderChangedIndexed checks if the component folder changed using the indexed files.
// This is an optimized version that only checks relevant files.
// Now with pattern caching for additional improvement.
func isComponentFolderChangedIndexed(
	component string,
	componentType string,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
) (bool, error) {
	// Get cached pattern.
	componentPathPattern, err := patternCache.getComponentPathPattern(component, componentType, atmosConfig)
	if err != nil {
		return false, err
	}

	// Only check files relevant to this component type.
	relevantFiles := filesIndex.getRelevantFiles(componentType, atmosConfig)

	for _, changedFile := range relevantFiles {
		// Files are already absolute paths from the index.
		match, err := u.PathMatch(componentPathPattern, changedFile)
		if err != nil {
			return false, err
		}

		if match {
			return true, nil
		}
	}

	return false, nil
}

// areTerraformComponentModulesChangedIndexed checks if terraform modules changed using indexed files.
// This is an optimized version with cached module patterns to avoid expensive tfconfig.LoadModule() calls.
func areTerraformComponentModulesChangedIndexed(
	component string,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
) (bool, error) {
	// Get cached module patterns (avoids expensive tfconfig.LoadModule()).
	modulePatterns, err := patternCache.getTerraformModulePatterns(component, atmosConfig)
	if err != nil {
		return false, err
	}

	// If no modules, return early.
	if len(modulePatterns) == 0 {
		return false, nil
	}

	// Only check terraform files.
	relevantFiles := filesIndex.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)

	// Check each file against all patterns (inverted loop order for better performance).
	for _, changedFile := range relevantFiles {
		for _, pattern := range modulePatterns {
			match, err := u.PathMatch(pattern, changedFile)
			if err != nil {
				return false, err
			}

			if match {
				return true, nil
			}
		}
	}

	return false, nil
}

// isComponentDependentFolderOrFileChangedIndexed checks dependencies using indexed files.
// This is an optimized version that reduces file iterations.
//
//nolint:gocognit,revive // Dependency checking requires nested loops and multiple return values for complete status
func isComponentDependentFolderOrFileChangedIndexed(
	filesIndex *changedFilesIndex,
	deps schema.DependsOn,
) (bool, string, string, error) {
	hasDependencies := false
	isChanged := false
	changedType := ""
	changedFileOrFolder := ""
	pathPatternSuffix := ""

	// Get all files once (dependencies can span multiple component types).
	allChangedFiles := filesIndex.getAllFiles()

	for _, dep := range deps { //nolint:gocritic // DependsOn value copy is acceptable; refactoring would require schema changes
		if isChanged {
			break
		}

		if dep.File != "" {
			changedType = "file"
			changedFileOrFolder = dep.File
			pathPatternSuffix = ""
			hasDependencies = true
		} else if dep.Folder != "" {
			changedType = "folder"
			changedFileOrFolder = dep.Folder
			pathPatternSuffix = "/**"
			hasDependencies = true
		}

		if hasDependencies {
			changedFileOrFolderAbs, err := filepath.Abs(changedFileOrFolder)
			if err != nil {
				return false, "", "", err
			}

			pathPattern := changedFileOrFolderAbs + pathPatternSuffix

			for _, changedFile := range allChangedFiles {
				// Files are already absolute from index.
				match, err := u.PathMatch(pathPattern, changedFile)
				if err != nil {
					return false, "", "", err
				}

				if match {
					isChanged = true
					break
				}
			}
		}
	}

	return isChanged, changedType, changedFileOrFolder, nil
}
